package cmd

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"

	"github.com/fatih/color"
	"github.com/spf13/cobra"

	"github.com/nitin-1926/ccpm/internal/config"
	"github.com/nitin-1926/ccpm/internal/credentials"
	"github.com/nitin-1926/ccpm/internal/keystore"
	"github.com/nitin-1926/ccpm/internal/picker"
	"github.com/nitin-1926/ccpm/internal/settingsmerge"
)

var setDefaultCmd = &cobra.Command{
	Use:   "set-default [name]",
	Short: "Set profile as default for VS Code / IDE extension",
	Args:  cobra.MaximumNArgs(1),
	RunE:  runSetDefault,
}

var unsetDefaultCmd = &cobra.Command{
	Use:   "unset-default",
	Short: "Clear default profile",
	RunE:  runUnsetDefault,
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

	switch p.AuthMethod {
	case "oauth":
		if err := applyOAuthDefault(p.Dir); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: %v\n", err)
			yellow.Println("  → IDE extensions on macOS may keep using the previous default until the next `set-default`.")
		}
	case "api_key":
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
	fmt.Println("VS Code extension will use this account on next restart.")
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

	cfg.DefaultProfile = ""
	if err := config.Save(cfg); err != nil {
		return fmt.Errorf("saving config: %w", err)
	}

	fmt.Println("Default profile cleared.")
	return nil
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

	if runtime.GOOS == "darwin" {
		if err := copyKeychainToDefaultMac(profileDir); err != nil {
			return fmt.Errorf("could not copy macOS keychain entry into the default slot: %w", err)
		}
		return nil
	}
	if err := copyCredentialsToDefault(profileDir); err != nil {
		return fmt.Errorf("could not copy credentials to ~/.claude/: %w", err)
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

	if runtime.GOOS == "darwin" {
		if err := credentials.DeleteMacKeychainOAuthDefault(claudeDir); err != nil {
			return fmt.Errorf("clearing default-slot OAuth: %w", err)
		}
	} else {
		// Non-darwin: remove the plaintext default credentials file so the
		// extension can't keep using it either.
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
	if err := os.MkdirAll(claudeDir, 0755); err != nil {
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

	if err := os.MkdirAll(filepath.Join(home, ".claude"), 0755); err != nil {
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
// into the "plain" Claude Code-credentials entry that IDE extensions read.
// Both entries are written via go-keyring so ACLs stay permissive.
func copyKeychainToDefaultMac(profileDir string) error {
	if runtime.GOOS != "darwin" {
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
