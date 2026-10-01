package cmd

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"

	"github.com/fatih/color"
	"github.com/spf13/cobra"

	"github.com/nitin-1926/claude-code-profile-manager/ccpm/internal/config"
	"github.com/nitin-1926/claude-code-profile-manager/ccpm/internal/defaultclaude"
	"github.com/nitin-1926/claude-code-profile-manager/ccpm/internal/profile"
	"github.com/nitin-1926/claude-code-profile-manager/ccpm/internal/profilelife"
	"github.com/nitin-1926/claude-code-profile-manager/ccpm/internal/settingsmerge"
	profilesync "github.com/nitin-1926/claude-code-profile-manager/ccpm/internal/sync"
)

var cloneNoAuth bool

var cloneCmd = &cobra.Command{
	Use:   "clone <source> <new-name>",
	Short: "Duplicate an existing profile (assets, settings, and auth)",
	Long: `Creates <new-name> as a copy of <source>: its skills, agents, commands,
rules, hooks, MCP servers, plugins, and settings are duplicated, and (unless
--no-auth) its credentials are copied too.

Useful for a throwaway/scratch copy of a profile you don't want to disturb.

Note on OAuth: a cloned OAuth profile shares the source account's tokens. When
Claude rotates the refresh token in one, the other goes stale — so for a clone
you intend to use long-term against the same account, prefer --no-auth and run
'ccpm auth refresh <new-name>' to give it its own login. API-key clones have no
such caveat.`,
	Args:              cobra.ExactArgs(2),
	ValidArgsFunction: completeProfileNames,
	RunE:              lockedRunE(runClone),
}

func init() {
	cloneCmd.Flags().BoolVar(&cloneNoAuth, "no-auth", false, "copy assets/settings only; leave the clone unauthenticated")
	rootCmd.AddCommand(cloneCmd)
}

func runClone(cmd *cobra.Command, args []string) error {
	src, dst := args[0], args[1]

	if err := profile.ValidateName(dst); err != nil {
		return err
	}

	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	srcProfile, ok := cfg.Profiles[src]
	if !ok {
		return fmt.Errorf("source profile %q not found", src)
	}
	if _, exists := cfg.Profiles[dst]; exists {
		return fmt.Errorf("profile %q already exists", dst)
	}

	dstDir, err := profile.Create(dst)
	if err != nil {
		return fmt.Errorf("creating clone directory: %w", err)
	}
	// Undo every store the clone may have written, so a failed clone leaves
	// nothing under dst's name for a later profile to inherit.
	rollback := func() {
		_ = profilelife.Remove(dst)
		_ = profile.Remove(dst)
	}

	// Copy the source's directory, its shared/profile-scoped asset links, and
	// its name-keyed stores (settings/MCP fragments, manifest entries).
	if err := importFromProfile(src, srcProfile.Dir, dst, dstDir, defaultclaude.AllTargets(), false, true); err != nil {
		rollback()
		return fmt.Errorf("copying profile assets: %w", err)
	}
	if err := settingsmerge.MaterializeAll(dstDir, dst, ""); err != nil {
		rollback()
		return fmt.Errorf("materializing clone settings: %w", err)
	}
	// Re-link the global and host-cascaded assets, so the clone is
	// immediately launch-ready (mirrors `ccpm add`).
	if err := profilesync.ApplyGlobals(dstDir, dst); err != nil {
		fmt.Fprintf(cmd.ErrOrStderr(), "Warning: could not apply shared assets to clone: %v\n", err)
	}

	// --no-auth keeps the source's auth method but copies no secrets.
	authMethod := srcProfile.AuthMethod
	if !cloneNoAuth {
		if err := copyProfileAuth(src, srcProfile.Dir, dst, dstDir, srcProfile.AuthMethod); err != nil {
			// Auth copy is best-effort: the clone's assets are already in place,
			// so don't tear it down — just warn and let the user authenticate it.
			fmt.Fprintf(cmd.ErrOrStderr(), "Warning: could not copy credentials (run `ccpm auth refresh %s`): %v\n", dst, err)
		}
	}

	cfg.AddProfile(dst, dstDir, authMethod)
	if err := config.Save(cfg); err != nil {
		// The clone never registered: drop the credentials copied for it too,
		// as `ccpm add` does, so nothing orphaned outlives the failed clone.
		if !cloneNoAuth {
			_ = newKeystore().DeleteAPIKey(dst)
			_ = deleteOAuthKeychain(dstDir)
		}
		rollback()
		return fmt.Errorf("saving config: %w", err)
	}

	green := color.New(color.FgGreen, color.Bold)
	green.Printf("✓ Cloned %q to %q\n", src, dst)
	if cloneNoAuth || authMethod == "" {
		fmt.Printf("Authenticate it with:\n  ccpm auth refresh %s\n", dst)
	} else if authMethod == "oauth" {
		fmt.Printf("Run it with:\n  ccpm run %s\n", dst)
		color.New(color.FgYellow).Println("Note: this OAuth clone shares the source account's tokens; see `ccpm clone --help`.")
	} else {
		fmt.Printf("Run it with:\n  ccpm run %s\n", dst)
	}
	return nil
}

// copyProfileAuth replicates the source profile's credentials into the clone.
// For api_key it copies the keychain entry; for oauth it copies the on-disk
// credential/identity files and replays the OS-keychain OAuth entry under the
// clone dir's namespace (mirroring how `ccpm rename` migrates keychain state).
func copyProfileAuth(srcName, srcDir, dstName, dstDir, authMethod string) error {
	switch authMethod {
	case "api_key":
		store := newKeystore()
		key, err := store.GetAPIKey(srcName)
		if err != nil {
			return fmt.Errorf("reading source API key: %w", err)
		}
		if err := store.SetAPIKey(dstName, key); err != nil {
			return fmt.Errorf("storing cloned API key: %w", err)
		}
	case "oauth":
		// On-disk credential + identity files (Linux primary path, and useful
		// metadata on macOS so `ccpm status` shows the account).
		for _, f := range []string{".credentials.json", ".claude.json"} {
			_ = copyFileIfExists(filepath.Join(srcDir, f), filepath.Join(dstDir, f))
		}
		// macOS/Windows OS-keychain OAuth entry, replayed under the clone's
		// path-derived namespace.
		if runtime.GOOS == "darwin" || runtime.GOOS == "windows" {
			if kc, err := readOAuthKeychain(srcDir); err == nil && kc != nil && kc.Raw != "" {
				if err := writeOAuthKeychain(dstDir, kc.Raw); err != nil {
					return fmt.Errorf("replaying OAuth keychain entry: %w", err)
				}
			}
		}
	}
	return nil
}

// copyFileIfExists copies src to dst with 0600 perms. A missing src is a no-op.
func copyFileIfExists(src, dst string) error {
	in, err := os.Open(src)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, config.FilePerm)
	if err != nil {
		return err
	}
	defer out.Close()

	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return out.Close()
}
