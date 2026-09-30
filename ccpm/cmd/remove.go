package cmd

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/fatih/color"
	"github.com/spf13/cobra"

	"github.com/nitin-1926/claude-code-profile-manager/ccpm/internal/config"
	"github.com/nitin-1926/claude-code-profile-manager/ccpm/internal/profile"
	"github.com/nitin-1926/claude-code-profile-manager/ccpm/internal/profilelife"
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

	if _, exists := cfg.Profiles[name]; !exists {
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
		freshCfg, err := config.Load()
		if err != nil {
			return fmt.Errorf("reloading config: %w", err)
		}
		p, exists := freshCfg.Profiles[name]
		if !exists {
			return fmt.Errorf("profile %q not found", name)
		}

		// Name-keyed stores first (settings/MCP fragments, manifest refs): if
		// this transaction fails nothing has been deleted yet. Left behind,
		// they would be inherited by the next profile created with this name.
		if err := profilelife.Remove(name); err != nil {
			return fmt.Errorf("removing profile settings/MCP fragments and manifest refs: %w", err)
		}

		// Remove profile directory
		if err := profile.Remove(name); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: %v\n", err)
		}

		// Remove API key from keychain if applicable
		if p.AuthMethod == "api_key" {
			store := newKeystore()
			if err := store.DeleteAPIKey(name); err != nil {
				fmt.Fprintf(os.Stderr, "Warning: could not remove API key from keychain: %v\n", err)
			}
		}

		// Remove the OAuth login Claude Code keyed to this dir's path. A
		// re-added profile with the same name gets the same dir → the same
		// keychain slot, and would silently be logged in as this account.
		// Deleted for api_key profiles too: `claude /login` run inside one
		// writes the same slot, and nothing else can ever use it.
		if err := deleteOAuthKeychain(p.Dir); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: could not remove OAuth keychain entry: %v\n", err)
		}

		// Remove vault backup
		v := vault.New(newKeystore())
		if err := v.Remove(name); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: could not remove vault backup: %v\n", err)
		}

		freshCfg.RemoveProfile(name)
		if err := config.Save(freshCfg); err != nil {
			return fmt.Errorf("saving config: %w", err)
		}

		color.New(color.FgGreen, color.Bold).Printf("✓ Profile %q removed\n", name)
		return nil
	})
}
