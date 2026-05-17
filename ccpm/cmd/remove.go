package cmd

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/fatih/color"
	"github.com/spf13/cobra"

	"github.com/nitin-1926/claude-code-profile-manager/ccpm/internal/config"
	"github.com/nitin-1926/claude-code-profile-manager/ccpm/internal/keystore"
	"github.com/nitin-1926/claude-code-profile-manager/ccpm/internal/profile"
	"github.com/nitin-1926/claude-code-profile-manager/ccpm/internal/vault"
)

var (
	forceRemove bool
)

var removeCmd = &cobra.Command{
	Use:   "remove <name>",
	Short: "Delete a profile",
	Aliases: []string{"rm"},
	Args:              cobra.ExactArgs(1),
	RunE:              runRemove,
	ValidArgsFunction: completeProfileNames,
}

func init() {
	removeCmd.Flags().BoolVarP(&forceRemove, "force", "f", false, "skip confirmation")
	rootCmd.AddCommand(removeCmd)
}

func runRemove(cmd *cobra.Command, args []string) error {
	name := args[0]

	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	p, exists := cfg.Profiles[name]
	if !exists {
		return fmt.Errorf("profile %q not found", name)
	}

	if cfg.DefaultProfile == name {
		color.New(color.FgYellow).Fprintf(os.Stderr, "Warning: %q is the default profile\n", name)
	}

	if !forceRemove {
		fmt.Printf("Remove profile %q? This deletes all profile data. [y/N]: ", name)
		reader := bufio.NewReader(os.Stdin)
		input, _ := reader.ReadString('\n')
		if strings.TrimSpace(strings.ToLower(input)) != "y" {
			fmt.Println("Cancelled.")
			return nil
		}
	}

	// Hold the global lock only for the mutation phase — not the y/N prompt
	// above — so a user pondering the confirmation doesn't block other ccpm
	// commands. Re-load config inside the lock so we delete from the freshest
	// state instead of clobbering a concurrent profile add/rename.
	return withConfigLock(func() error {
		// Remove profile directory
		if err := profile.Remove(name); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: %v\n", err)
		}

		// Remove API key from keychain if applicable
		if p.AuthMethod == "api_key" {
			store := keystore.New()
			if err := store.DeleteAPIKey(name); err != nil {
				fmt.Fprintf(os.Stderr, "Warning: could not remove API key from keychain: %v\n", err)
			}
		}

		// Remove vault backup
		v := vault.New(keystore.New())
		if err := v.Remove(name); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: could not remove vault backup: %v\n", err)
		}

		// Update config (re-loaded under lock)
		freshCfg, err := config.Load()
		if err != nil {
			return fmt.Errorf("reloading config: %w", err)
		}
		freshCfg.RemoveProfile(name)
		if err := config.Save(freshCfg); err != nil {
			return fmt.Errorf("saving config: %w", err)
		}

		color.New(color.FgGreen, color.Bold).Printf("✓ Profile %q removed\n", name)
		return nil
	})
}
