package cmd

import (
	"fmt"

	"github.com/fatih/color"
	"github.com/spf13/cobra"

	"github.com/nitin-1926/claude-code-profile-manager/ccpm/internal/config"
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

	// Rename profile directory
	if err := profile.Rename(oldName, newName); err != nil {
		return fmt.Errorf("renaming profile directory: %w", err)
	}

	newDir, err := profile.GetDir(newName)
	if err != nil {
		return err
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
