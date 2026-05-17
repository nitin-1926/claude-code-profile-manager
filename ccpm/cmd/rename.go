package cmd

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/fatih/color"
	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/nitin-1926/claude-code-profile-manager/ccpm/internal/atomicwrite"
	"github.com/nitin-1926/claude-code-profile-manager/ccpm/internal/config"
	"github.com/nitin-1926/claude-code-profile-manager/ccpm/internal/credentials"
	"github.com/nitin-1926/claude-code-profile-manager/ccpm/internal/keystore"
	"github.com/nitin-1926/claude-code-profile-manager/ccpm/internal/profile"
	"github.com/nitin-1926/claude-code-profile-manager/ccpm/internal/vault"
)

var renameCmd = &cobra.Command{
	Use:   "rename <old-name> <new-name>",
	Short: "Rename a profile",
	Args:              cobra.ExactArgs(2),
	RunE:              lockedRunE(runRename),
	ValidArgsFunction: completeProfileNames,
}

func init() {
	rootCmd.AddCommand(renameCmd)
}

func runRename(cmd *cobra.Command, args []string) error {
	oldName, newName := args[0], args[1]

	if err := profile.ValidateName(newName); err != nil {
		return err
	}

	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	p, exists := cfg.Profiles[oldName]
	if !exists {
		return fmt.Errorf("profile %q not found", oldName)
	}
	if _, exists := cfg.Profiles[newName]; exists {
		return fmt.Errorf("profile %q already exists", newName)
	}

	// If newName isn't in the registry but a directory by that name still
	// exists on disk, it's an orphan from an earlier rename/import that didn't
	// clean up — `claude` running inside a previously-renamed dir can also
	// re-create it. profile.Rename would refuse with a misleading "profile
	// already exists" message, so handle the case here with a clear summary
	// and an explicit confirmation.
	newDirPath, err := profile.GetDir(newName)
	if err != nil {
		return err
	}
	if info, statErr := os.Stat(newDirPath); statErr == nil && info.IsDir() {
		fileCount, totalBytes := summarizeDir(newDirPath)
		fmt.Fprintf(cmd.ErrOrStderr(),
			"Directory %s exists on disk but is not a registered profile.\n"+
				"  contents: %d file(s), %s\n",
			newDirPath, fileCount, humanizeBytes(totalBytes))
		if !term.IsTerminal(int(os.Stdin.Fd())) {
			return fmt.Errorf("orphan directory %q present; refusing to clobber in non-interactive mode (remove it manually first)", newDirPath)
		}
		fmt.Fprintf(cmd.ErrOrStderr(), "Remove this directory and continue with the rename? [y/N]: ")
		reader := bufio.NewReader(os.Stdin)
		input, _ := reader.ReadString('\n')
		if strings.TrimSpace(strings.ToLower(input)) != "y" {
			fmt.Fprintln(cmd.ErrOrStderr(), "Cancelled.")
			return nil
		}
		// Best-effort: also delete any orphan keychain entry whose namespace
		// hashes from this exact path. If `claude` had ever logged in here,
		// the entry is stale anyway.
		_ = credentials.DeleteMacKeychainOAuth(newDirPath)
		if err := os.RemoveAll(newDirPath); err != nil {
			return fmt.Errorf("removing orphan directory: %w", err)
		}
	}

	// The macOS keychain entry for OAuth tokens is keyed by sha256(absPath of
	// profile dir). Renaming the directory changes the path -> new hash -> the
	// keychain entry becomes orphaned and `claude` prompts re-login. Read the
	// payload from the OLD dir's namespace before we rename, so we can replay
	// it under the NEW dir's namespace afterwards. Read errors here are
	// non-fatal: api_key profiles and OAuth profiles that pre-date the v2.1.56
	// keychain layout will simply have no payload to migrate.
	oldDir, err := profile.GetDir(oldName)
	if err != nil {
		return err
	}
	var oauthPayload string
	if p.AuthMethod == "oauth" {
		if kc, err := credentials.ReadMacKeychainOAuth(oldDir); err != nil {
			fmt.Fprintf(cmd.ErrOrStderr(), "Warning: could not read OAuth keychain entry: %v\n", err)
		} else if kc != nil {
			oauthPayload = kc.Raw
		}
	}

	// Rename profile directory
	if err := profile.Rename(oldName, newName); err != nil {
		return fmt.Errorf("renaming profile directory: %w", err)
	}

	newDir, err := profile.GetDir(newName)
	if err != nil {
		return err
	}

	// Order matters for safe rollback. The keychain migrations below are the
	// steps that can fail and force us to undo the directory rename. We run them
	// FIRST, while the only thing to roll back is the `os.Rename` itself. The
	// plugin-metadata rewrite — which bakes the *new* dir path into JSON on disk
	// — is deliberately deferred until after the keychain steps succeed, so a
	// keychain failure never leaves plugin metadata pointing at a directory we
	// then move back. (A rename profile is either oauth or api_key, so at most
	// one migration block runs.)

	// Replay the OAuth keychain entry under the new dir's hash. If write fails,
	// roll back the directory rename so the user isn't left with a profile dir
	// that no longer matches its keychain namespace.
	if oauthPayload != "" {
		if err := credentials.WriteMacKeychainOAuth(newDir, oauthPayload); err != nil {
			_ = profile.Rename(newName, oldName)
			return fmt.Errorf("migrating OAuth keychain entry to new name: %w", err)
		}
		if err := credentials.DeleteMacKeychainOAuth(oldDir); err != nil {
			fmt.Fprintf(cmd.ErrOrStderr(), "Warning: could not remove old OAuth keychain entry: %v\n", err)
		}
	}

	// Move API key in keychain if applicable
	if p.AuthMethod == "api_key" {
		store := keystore.New()
		key, err := store.GetAPIKey(oldName)
		if err != nil {
			fmt.Fprintf(cmd.ErrOrStderr(), "Warning: could not read API key from keychain: %v\n", err)
		} else {
			if err := store.SetAPIKey(newName, key); err != nil {
				// Roll back directory rename before returning. No plugin
				// metadata has been rewritten yet, so the dir and its on-disk
				// references stay consistent after the rollback.
				_ = profile.Rename(newName, oldName)
				return fmt.Errorf("storing API key under new name: %w", err)
			}
			if err := store.DeleteAPIKey(oldName); err != nil {
				fmt.Fprintf(cmd.ErrOrStderr(), "Warning: could not remove old API key from keychain: %v\n", err)
			}
		}
	}

	// Plugin metadata files (`installed_plugins.json`, `known_marketplaces.json`)
	// store absolute paths under the old profile dir. `os.Rename` only moves the
	// directory itself; those embedded paths still point at <oldDir>, so Claude
	// Code silently fails to load every affected plugin under the new name.
	// Rewrite them in place. Done only after the fallible keychain migrations
	// above committed, so we never bake newDir into metadata for a rename we
	// then roll back. A failure here is non-fatal (plugins may not load until a
	// `ccpm sync`), but the profile identity itself is already consistent.
	if err := rewritePluginMetadataPaths(newDir, oldDir, newDir); err != nil {
		fmt.Fprintf(cmd.ErrOrStderr(), "Warning: could not rewrite plugin metadata paths: %v\n", err)
	}

	// Rename vault backup if present
	v := vault.New(keystore.New())
	if err := v.Rename(oldName, newName); err != nil {
		fmt.Fprintf(cmd.ErrOrStderr(), "Warning: could not rename vault backup: %v\n", err)
	}

	// Update config
	cfg.RenameProfile(oldName, newName, newDir)
	if err := config.Save(cfg); err != nil {
		return fmt.Errorf("saving config: %w", err)
	}

	color.New(color.FgGreen, color.Bold).Printf("✓ Profile %q renamed to %q\n", oldName, newName)
	return nil
}

// summarizeDir walks dir and returns (file count, total bytes). Walk errors
// are silently skipped — this is purely informational ahead of a confirm
// prompt, not a measurement we'd act on programmatically.
func summarizeDir(dir string) (int, int64) {
	var count int
	var bytes int64
	_ = filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		count++
		if info, ierr := d.Info(); ierr == nil {
			bytes += info.Size()
		}
		return nil
	})
	return count, bytes
}

// rewritePluginMetadataPaths rewrites every absolute path beginning with oldDir
// inside profileDir/plugins/{installed_plugins.json,known_marketplaces.json} to
// begin with newDir instead. Each file is parsed as untyped JSON so that
// fields the schema picks up later (added by Claude Code or new ccpm versions)
// are preserved verbatim. Returns nil silently when a file is absent.
func rewritePluginMetadataPaths(profileDir, oldDir, newDir string) error {
	files := []string{
		filepath.Join(profileDir, "plugins", "installed_plugins.json"),
		filepath.Join(profileDir, "plugins", "known_marketplaces.json"),
	}
	for _, path := range files {
		if err := rewriteAbsPathsInJSON(path, oldDir, newDir); err != nil {
			return fmt.Errorf("rewriting %s: %w", path, err)
		}
	}
	return nil
}

// rewriteAbsPathsInJSON loads path as a generic JSON value, walks it, and for
// every string that is exactly oldPrefix or starts with oldPrefix + "/" rewrites
// the prefix to newPrefix. Writes the result back atomically with 0600 perms
// only if a rewrite actually occurred (so a touch-rename remains a no-op for
// untouched files). Missing files are treated as success.
func rewriteAbsPathsInJSON(path, oldPrefix, newPrefix string) error {
	raw, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	var doc interface{}
	if err := json.Unmarshal(raw, &doc); err != nil {
		return fmt.Errorf("parse: %w", err)
	}
	rewrote := false
	doc = walkRewrite(doc, oldPrefix, newPrefix, &rewrote)
	if !rewrote {
		return nil
	}
	out, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return err
	}
	out = append(out, '\n')
	if err := atomicwrite.Apply([]atomicwrite.FileChange{
		atomicwrite.WriteFile(path, out, config.FilePerm),
	}); err != nil {
		return err
	}
	return nil
}

func walkRewrite(v interface{}, oldPrefix, newPrefix string, rewrote *bool) interface{} {
	switch t := v.(type) {
	case map[string]interface{}:
		for k, sub := range t {
			t[k] = walkRewrite(sub, oldPrefix, newPrefix, rewrote)
		}
		return t
	case []interface{}:
		for i, sub := range t {
			t[i] = walkRewrite(sub, oldPrefix, newPrefix, rewrote)
		}
		return t
	case string:
		if t == oldPrefix {
			*rewrote = true
			return newPrefix
		}
		if strings.HasPrefix(t, oldPrefix+string(os.PathSeparator)) {
			*rewrote = true
			return newPrefix + t[len(oldPrefix):]
		}
		return t
	default:
		return v
	}
}

func humanizeBytes(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := int64(unit), 0
	for x := n / unit; x >= unit; x /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %ciB", float64(n)/float64(div), "KMGTPE"[exp])
}
