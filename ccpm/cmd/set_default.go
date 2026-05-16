package cmd

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"

	"github.com/fatih/color"
	"github.com/spf13/cobra"

	"github.com/nitin-1926/claude-code-profile-manager/ccpm/internal/config"
	"github.com/nitin-1926/claude-code-profile-manager/ccpm/internal/credentials"
	"github.com/nitin-1926/claude-code-profile-manager/ccpm/internal/keystore"
	"github.com/nitin-1926/claude-code-profile-manager/ccpm/internal/picker"
	"github.com/nitin-1926/claude-code-profile-manager/ccpm/internal/settingsmerge"
)

var setDefaultCmd = &cobra.Command{
	Use:               "set-default [name]",
	Short:             "Set profile as default for VS Code / IDE extension",
	Args:              cobra.MaximumNArgs(1),
	RunE:              lockedRunE(runSetDefault),
	ValidArgsFunction: completeProfileNames,
}

var unsetDefaultCmd = &cobra.Command{
	Use:   "unset-default",
	Short: "Clear default profile",
	RunE:  lockedRunE(runUnsetDefault),
}

func init() {
	rootCmd.AddCommand(setDefaultCmd)
	rootCmd.AddCommand(unsetDefaultCmd)
}

func runSetDefault(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	var name string
	if len(args) == 1 {
		name = args[0]
	} else {
		// No name given — prompt if interactive, else error with a hint.
		names := config.ProfileNames(cfg)
		if len(names) == 0 {
			return fmt.Errorf("no profiles exist yet — create one with `ccpm add <name>`")
		}
		opts := make([]picker.Option, len(names))
		for i, n := range names {
			desc := ""
			if p := cfg.Profiles[n]; p.AuthMethod != "" {
				desc = p.AuthMethod
			}
			if n == cfg.DefaultProfile {
				desc += " (current default)"
			}
			opts[i] = picker.Option{Value: n, Label: n, Description: desc}
		}
		choice, err := picker.Select("Which profile should be the VSCode default?", opts)
		if err != nil {
			if errors.Is(err, picker.ErrNonInteractive) {
				return fmt.Errorf("profile name is required (e.g. `ccpm set-default %s`)", names[0])
			}
			return err
		}
		name = choice
	}

	p, exists := cfg.Profiles[name]
	if !exists {
		return fmt.Errorf("profile %q not found", name)
	}

	yellow := color.New(color.FgYellow)

	// Capture whatever has accumulated in the default slot (and ~/.claude.json
	// identity) back into the *previously* default profile's own slot, before
	// we overwrite the default with the new selection. Without this step,
	// set-default is a one-way copy: plain `claude` / VSCode invocations
	// rotate the refresh token in the default slot only, the per-profile slot
	// stays frozen, and a later `set-default` of that profile would copy a
	// now-rotated, invalidated refresh token back into the default slot —
	// reliably 401-ing the next `claude`. Best-effort: failures warn but do
	// not block the new set-default.
	if cfg.DefaultProfile != "" {
		if prev, ok := cfg.Profiles[cfg.DefaultProfile]; ok && prev.AuthMethod == "oauth" {
			saveDefaultBackToProfile(prev.Dir)
		}
	}

	// Applying credentials to the default slot is intentionally best-effort:
	// each step warns on failure but does not abort. This matters because the
	// steps are partially-applied side effects (keychain default slot, ~/.claude
	// identity, launchctl env) — bailing out midway would leave them disagreeing
	// with config.json (the "wrong account / re-login" failure mode). Saving the
	// new default unconditionally keeps config, keychain, and launchd in
	// agreement; a warning tells the user if a piece needs a manual retry.
	switch p.AuthMethod {
	case "oauth":
		if err := applyOAuthDefault(p.Dir); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: %v\n", err)
			yellow.Println("  → IDE extensions on macOS may keep using the previous default until the next `set-default`.")
		}
		// Make this profile the system-wide CLAUDE_CONFIG_DIR for every
		// newly-launched claude process — terminal, IDE extensions, GUI apps.
		// Works around the Claude Code v2.1.x startup-refresh path that 401s
		// when CLAUDE_CONFIG_DIR resolves to bare ~/.claude. Best-effort: a
		// failure here doesn't undo the keychain/identity sync above.
		if err := setSystemDefaultConfigDir(p.Dir); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: could not register system-wide CLAUDE_CONFIG_DIR: %v\n", err)
			yellow.Println("  → IDE extensions may not pick up this profile until you restart them with the env set manually.")
		}
	case "api_key":
		// Switching to an API-key profile: undo any system-wide
		// CLAUDE_CONFIG_DIR we previously set for an OAuth profile. claude
		// then reads ANTHROPIC_API_KEY from ~/.claude/settings.json's env
		// block as before.
		if err := clearSystemDefaultConfigDir(); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: could not clear system-wide CLAUDE_CONFIG_DIR: %v\n", err)
		}
		if err := applyAPIKeyDefault(name); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: %v\n", err)
		} else {
			yellow.Println("  Note: VSCode's sidebar cannot display API-key logins as \"signed in,\"")
			yellow.Println("        but `claude` invocations (integrated terminal, agents) now use this profile's key.")
		}
	}

	cfg.DefaultProfile = name
	if err := config.Save(cfg); err != nil {
		return fmt.Errorf("saving config: %w", err)
	}

	color.New(color.FgGreen, color.Bold).Printf("✓ Profile %q is now the default\n", name)
	if runtime.GOOS == "darwin" && p.AuthMethod == "oauth" {
		fmt.Println("Restart any open Cursor/VSCode/Antigravity windows to pick up the new default.")
	} else {
		fmt.Println("VS Code extension will use this account on next restart.")
	}
	return nil
}

func runUnsetDefault(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	if err := clearAPIKeyEnv(); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: could not strip ANTHROPIC_API_KEY from ~/.claude/settings.json: %v\n", err)
	}
	// Remove the system-wide CLAUDE_CONFIG_DIR we may have set during a
	// previous `set-default` for an OAuth profile, so IDE extensions stop
	// being pinned to any specific profile on next launch.
	if err := clearSystemDefaultConfigDir(); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: could not clear system-wide CLAUDE_CONFIG_DIR: %v\n", err)
	}

	cfg.DefaultProfile = ""
	if err := config.Save(cfg); err != nil {
		return fmt.Errorf("saving config: %w", err)
	}

	fmt.Println("Default profile cleared.")
	if runtime.GOOS == "darwin" {
		fmt.Println("Restart Cursor/VSCode/Antigravity windows for the change to take effect.")
	}
	return nil
}

// saveDefaultBackToProfile is the inverse of applyOAuthDefault: it folds the
// current default slot (and ~/.claude.json identity) back into the previous
// default profile's namespaced storage. Called before applying a new default
// so that token rotations Claude Code performed in the default slot are
// preserved in the per-profile slot they "belong to".
//
// IMPORTANT directional rule: save-back only fires when the default slot's
// content is *strictly fresher* than the profile slot's content. Without that
// guard, a re-login done via `ccpm run <profile> claude /login` (which writes
// fresh tokens directly into the profile slot, never touching the default slot)
// would be wiped out by the next set-default: we would copy the *stale* default
// content back over the fresh profile tokens, sending the user right back to
// the 401 they just escaped. Freshness is compared via `expiresAt`, which
// Claude Code stamps from Anthropic's token endpoint.
//
// Best-effort everywhere: each step warns and continues; never blocks the
// caller.
func saveDefaultBackToProfile(prevDir string) {
	yellow := color.New(color.FgYellow)

	if runtime.GOOS == "darwin" || runtime.GOOS == "windows" {
		home, err := os.UserHomeDir()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: could not resolve home dir for credential save-back: %v\n", err)
			return
		}
		defaultDir := filepath.Join(home, ".claude")
		cur, err := credentials.ReadMacKeychainOAuth(defaultDir)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: could not read default OAuth slot to save back to previous profile: %v\n", err)
			yellow.Println("  → previous profile's keychain entry may hold a stale refresh token; re-login if `claude` 401s after switching back.")
			return
		}
		if cur == nil || cur.Raw == "" {
			return // nothing to save back; default slot was empty.
		}
		prev, _ := credentials.ReadMacKeychainOAuth(prevDir)
		if !shouldSaveBack(prev, cur) {
			// Profile slot is the freshest source of truth; do not touch
			// either the keychain or ~/.claude.json identity (they're
			// paired with the keychain pair we're choosing to preserve).
			return
		}
		// The default slot is fresher than the profile slot, so the
		// identity in ~/.claude.json is the one paired with the tokens
		// we're about to copy. Sync both together to keep them coupled.
		if err := syncOAuthIdentityFromDefault(prevDir); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: could not save identity back to previous default profile: %v\n", err)
			yellow.Println("  → previous profile's banner identity may be stale on next launch.")
		}
		if err := credentials.WriteMacKeychainOAuth(prevDir, cur.Raw); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: could not save default OAuth slot back into previous profile keychain entry: %v\n", err)
			yellow.Println("  → previous profile's keychain entry may hold a stale refresh token; re-login if `claude` 401s after switching back.")
		}
		return
	}

	// Linux: mirror copyCredentialsToDefault in reverse. No keychain
	// freshness check on Linux yet — the file mtime would be the only
	// signal and that's noisier than `expiresAt`. Filed as a follow-up.
	if err := syncOAuthIdentityFromDefault(prevDir); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: could not save identity back to previous default profile: %v\n", err)
	}
	if err := copyCredentialsFromDefault(prevDir); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: could not save default credentials back to previous profile: %v\n", err)
	}
}

// shouldSaveBack decides whether the default-slot OAuth payload (`cur`) should
// be written over the previous-default profile's namespaced slot (`prev`).
//
// The default slot is *not* unconditionally authoritative: if the user has
// just done `ccpm run <prev> claude /login`, the profile slot holds the
// freshest tokens and the default slot is stale. Save-back must not clobber
// that. Heuristic: profile slot is preserved when its `expiresAt` is at least
// as late as the default slot's (i.e. it was issued at or after the default
// slot's pair). Equal `Raw` content is also a no-op.
//
// Pure function so it can be exercised in tests without touching the real
// macOS keychain.
func shouldSaveBack(prev, cur *credentials.MacKeychainOAuth) bool {
	if cur == nil || cur.Raw == "" {
		return false
	}
	if prev == nil {
		return true
	}
	if prev.Raw == cur.Raw {
		return false
	}
	if !prev.ExpiresAt.IsZero() && !cur.ExpiresAt.IsZero() && !prev.ExpiresAt.Before(cur.ExpiresAt) {
		return false
	}
	return true
}

// syncOAuthIdentityFromDefault is the inverse of syncOAuthIdentityToDefault.
// Copies oauthAccount and userID from ~/.claude.json into <prevDir>/.claude.json,
// preserving every other key. Silent no-op when ~/.claude.json is absent or
// holds no identity.
func syncOAuthIdentityFromDefault(prevDir string) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	src, err := readClaudeJSON(filepath.Join(home, ".claude.json"))
	if err != nil {
		return err
	}
	if src == nil {
		return nil
	}
	dstPath := filepath.Join(prevDir, ".claude.json")
	dst, err := readClaudeJSON(dstPath)
	if err != nil {
		return err
	}
	if dst == nil {
		dst = map[string]interface{}{}
	}
	wrote := false
	for _, key := range []string{"oauthAccount", "userID"} {
		if v, ok := src[key]; ok {
			dst[key] = v
			wrote = true
		}
	}
	if !wrote {
		return nil
	}
	if err := os.MkdirAll(prevDir, config.DirPerm); err != nil {
		return err
	}
	return writeClaudeJSON(dstPath, dst)
}

// copyCredentialsFromDefault mirrors copyCredentialsToDefault in reverse, for
// the Linux save-back path. Silent no-op when ~/.claude/.credentials.json is
// absent.
func copyCredentialsFromDefault(prevDir string) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	src := filepath.Join(home, ".claude", ".credentials.json")
	srcFile, err := os.Open(src)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("opening default credentials: %w", err)
	}
	defer srcFile.Close()
	if err := os.MkdirAll(prevDir, config.DirPerm); err != nil {
		return err
	}
	dstFile, err := os.OpenFile(filepath.Join(prevDir, ".credentials.json"), os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return fmt.Errorf("opening destination: %w", err)
	}
	defer dstFile.Close()
	_, err = io.Copy(dstFile, srcFile)
	return err
}

// applyOAuthDefault puts the profile's OAuth credentials into whatever storage
// the IDE extension reads: the macOS keychain default slot on darwin, or
// ~/.claude/.credentials.json elsewhere.
func applyOAuthDefault(profileDir string) error {
	// When switching to an OAuth profile, any stale API-key env block in
	// ~/.claude/settings.json must go — otherwise the CLI picks up the wrong
	// key even though the keychain has fresh OAuth.
	if err := clearAPIKeyEnv(); err != nil {
		return fmt.Errorf("clearing stale API-key env: %w", err)
	}

	if runtime.GOOS == "darwin" || runtime.GOOS == "windows" {
		if err := copyKeychainToDefaultMac(profileDir); err != nil {
			return fmt.Errorf("could not copy OS-credential entry into the default slot: %w", err)
		}
	} else {
		if err := copyCredentialsToDefault(profileDir); err != nil {
			return fmt.Errorf("could not copy credentials to ~/.claude/: %w", err)
		}
	}

	// Claude Code displays the welcome banner identity (email, org name) from
	// the cached `oauthAccount` block in ~/.claude.json, NOT from the keychain
	// token. Without syncing it here, `claude` runs with the new profile's
	// credentials but shows the previous default's email until the next /me
	// fetch overwrites it — which looks like set-default did nothing.
	if err := syncOAuthIdentityToDefault(profileDir); err != nil {
		return fmt.Errorf("syncing identity into ~/.claude.json: %w", err)
	}
	return nil
}

// syncOAuthIdentityToDefault copies the identity fields Claude Code shows in
// its welcome banner (`oauthAccount`, `userID`) from the profile's
// `.claude.json` into `~/.claude.json`, leaving every other key untouched.
// Returns nil silently when the source has no identity to copy (e.g. a
// freshly-added profile that has not started Claude Code yet).
func syncOAuthIdentityToDefault(profileDir string) error {
	src, err := readClaudeJSON(filepath.Join(profileDir, ".claude.json"))
	if err != nil {
		return err
	}
	if src == nil {
		return nil
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	dstPath := filepath.Join(home, ".claude.json")
	dst, err := readClaudeJSON(dstPath)
	if err != nil {
		return err
	}
	if dst == nil {
		dst = map[string]interface{}{}
	}

	wrote := false
	for _, key := range []string{"oauthAccount", "userID"} {
		if v, ok := src[key]; ok {
			dst[key] = v
			wrote = true
		}
	}
	if !wrote {
		return nil
	}
	return writeClaudeJSON(dstPath, dst)
}

func readClaudeJSON(path string) (map[string]interface{}, error) {
	raw, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if len(raw) == 0 {
		return nil, nil
	}
	var m map[string]interface{}
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", path, err)
	}
	return m, nil
}

// writeClaudeJSON serializes the map back to disk atomically with 0600 perms,
// matching how Claude Code itself writes the file.
func writeClaudeJSON(path string, data map[string]interface{}) error {
	bytes, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return err
	}
	bytes = append(bytes, '\n')
	// NOTE: deliberately NOT routed through atomicwrite. This targets the
	// Claude-Code-owned ~/.claude.json (and a profile's .claude.json), which a
	// user may have symlinked into a dotfiles repo. atomicwrite refuses to
	// overwrite a symlink (a guard meant for ccpm-owned files), which would turn
	// set-default into a hard failure for those users. The temp-file + rename
	// below preserves the long-standing behavior of replacing the target.
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, bytes, 0600); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}

// applyAPIKeyDefault makes an API-key profile the de-facto default that CLI
// invocations rooted at ~/.claude will use.
//
// The VSCode/Antigravity extension has no API-key sign-in path today
// (see https://github.com/anthropics/claude-code/issues/8386) so we cannot
// make the sidebar light up. What we *can* do is:
//  1. Delete any OAuth entry in the macOS keychain default slot so the
//     extension cannot silently keep using a previous account.
//  2. Write ANTHROPIC_API_KEY into ~/.claude/settings.json under `env`, which
//     Claude Code honors for every invocation with CLAUDE_CONFIG_DIR=~/.claude
//     — covering the integrated terminal, agent subprocesses, etc.
func applyAPIKeyDefault(profileName string) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	claudeDir := filepath.Join(home, ".claude")

	if runtime.GOOS == "darwin" || runtime.GOOS == "windows" {
		if err := credentials.DeleteMacKeychainOAuthDefault(claudeDir); err != nil {
			return fmt.Errorf("clearing default-slot OAuth: %w", err)
		}
	} else {
		// Linux fallback: remove the plaintext default credentials file so
		// the extension can't keep using it either.
		credsPath := filepath.Join(claudeDir, ".credentials.json")
		if err := os.Remove(credsPath); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("removing %s: %w", credsPath, err)
		}
	}

	key, err := keystore.New().GetAPIKey(profileName)
	if err != nil {
		return fmt.Errorf("retrieving API key (run `ccpm auth refresh %s`): %w", profileName, err)
	}

	return writeAPIKeyEnv(claudeDir, key)
}

// writeAPIKeyEnv merges {"env": {"ANTHROPIC_API_KEY": key}} into
// <claudeDir>/settings.json, preserving all other keys.
func writeAPIKeyEnv(claudeDir, key string) error {
	if err := os.MkdirAll(claudeDir, config.DirPerm); err != nil {
		return err
	}
	settingsPath := filepath.Join(claudeDir, "settings.json")
	data, err := settingsmerge.LoadJSON(settingsPath)
	if err != nil {
		return fmt.Errorf("reading settings: %w", err)
	}
	if data == nil {
		data = map[string]interface{}{}
	}
	envRaw, _ := data["env"].(map[string]interface{})
	if envRaw == nil {
		envRaw = map[string]interface{}{}
	}
	envRaw["ANTHROPIC_API_KEY"] = key
	data["env"] = envRaw
	return settingsmerge.WriteJSON(settingsPath, data)
}

// clearAPIKeyEnv strips ANTHROPIC_API_KEY from ~/.claude/settings.json's env
// block. Safe to call when the file doesn't exist.
func clearAPIKeyEnv() error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	settingsPath := filepath.Join(home, ".claude", "settings.json")
	if _, err := os.Stat(settingsPath); os.IsNotExist(err) {
		return nil
	} else if err != nil {
		return err
	}
	data, err := settingsmerge.LoadJSON(settingsPath)
	if err != nil {
		return err
	}
	envRaw, _ := data["env"].(map[string]interface{})
	if envRaw == nil {
		return nil
	}
	if _, has := envRaw["ANTHROPIC_API_KEY"]; !has {
		return nil
	}
	delete(envRaw, "ANTHROPIC_API_KEY")
	if len(envRaw) == 0 {
		delete(data, "env")
	} else {
		data["env"] = envRaw
	}
	return settingsmerge.WriteJSON(settingsPath, data)
}

func copyCredentialsToDefault(profileDir string) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}

	src := filepath.Join(profileDir, ".credentials.json")
	dst := filepath.Join(home, ".claude", ".credentials.json")

	srcFile, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("opening source credentials: %w", err)
	}
	defer srcFile.Close()

	if err := os.MkdirAll(filepath.Join(home, ".claude"), config.DirPerm); err != nil {
		return err
	}

	dstFile, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return fmt.Errorf("opening destination: %w", err)
	}
	defer dstFile.Close()

	_, err = io.Copy(dstFile, srcFile)
	return err
}

// copyKeychainToDefaultMac copies a profile's namespaced keychain OAuth entry
// into the namespace IDE extensions read (the one derived from ~/.claude).
// Used on macOS *and* Windows — both go through go-keyring (Security
// framework / wincred respectively) so the call sites are identical.
func copyKeychainToDefaultMac(profileDir string) error {
	if runtime.GOOS != "darwin" && runtime.GOOS != "windows" {
		return nil
	}
	kc, err := credentials.ReadMacKeychainOAuth(profileDir)
	if err != nil {
		return fmt.Errorf("reading namespaced keychain entry: %w", err)
	}
	if kc == nil || kc.Raw == "" {
		return fmt.Errorf("profile has no OAuth entry in the keychain — login first with `ccpm auth refresh`")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	defaultDir := filepath.Join(home, ".claude")
	return credentials.WriteMacKeychainOAuth(defaultDir, kc.Raw)
}
