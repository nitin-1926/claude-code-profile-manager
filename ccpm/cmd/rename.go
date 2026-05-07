package cmd

import (
	"fmt"

	"github.com/fatih/color"
	"github.com/spf13/cobra"

	"github.com/nitin-1926/claude-code-profile-manager/ccpm/internal/config"
	"github.com/nitin-1926/claude-code-profile-manager/ccpm/internal/credentials"
	"github.com/nitin-1926/claude-code-profile-manager/ccpm/internal/keystore"
	"github.com/nitin-1926/claude-code-profile-manager/ccpm/internal/profile"
	"github.com/nitin-1926/claude-code-profile-manager/ccpm/internal/vault"
)

var renameCmd = &cobra.Command{
	Use:   "rename <old-name> <new-name>",
	Short: "Rename a profile",
	Args:  cobra.ExactArgs(2),
	RunE:  runRename,
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
				// Roll back directory rename before returning
				_ = profile.Rename(newName, oldName)
				return fmt.Errorf("storing API key under new name: %w", err)
			}
			if err := store.DeleteAPIKey(oldName); err != nil {
				fmt.Fprintf(cmd.ErrOrStderr(), "Warning: could not remove old API key from keychain: %v\n", err)
			}
		}
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
