package cmd

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/fatih/color"
	"github.com/spf13/cobra"

	"github.com/nitin-1926/claude-code-profile-manager/ccpm/internal/config"
)

var uninstallCmd = &cobra.Command{
	Use:   "uninstall",
	Short: "Remove ccpm and all its data from your system",
	Long: `Completely removes ccpm from your system:
  - Deletes all profiles and their data (~/.ccpm/)
  - Removes API keys and OAuth logins from your OS keychain
  - Removes vault master key from keychain
  - Clears the system-wide CLAUDE_CONFIG_DIR set by set-default (macOS)
  - Prints instructions to remove the binary and shell hook`,
	RunE: runUninstall,
}

func init() {
	rootCmd.AddCommand(uninstallCmd)
}

func runUninstall(cmd *cobra.Command, args []string) error {
	red := color.New(color.FgRed, color.Bold)
	green := color.New(color.FgGreen, color.Bold)

	red.Println("This will permanently delete ALL ccpm data:")
	fmt.Println("  - All profile directories and their config")
	fmt.Println("  - All API keys and OAuth logins from your OS keychain")
	fmt.Println("  - All encrypted vault backups")
	fmt.Println("  - The ccpm config directory (~/.ccpm/)")
	fmt.Println()

	if !forceRemove {
		fmt.Print("Are you sure? Type 'yes' to confirm: ")
		reader := bufio.NewReader(os.Stdin)
		input, _ := reader.ReadString('\n')
		if strings.TrimSpace(input) != "yes" {
			fmt.Println("Cancelled.")
			return nil
		}
	}

	fmt.Println()

	// Load config to find all profiles
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: could not load config: %v\n", err)
	}

	// Remove API keys and the path-namespaced OAuth logins from the keychain,
	// and undo set-default's launchd CLAUDE_CONFIG_DIR, which would otherwise
	// keep every GUI/IDE claude pointed into the deleted ~/.ccpm across reboots.
	if cfg != nil {
		store := newKeystore()
		for name, p := range cfg.Profiles {
			if p.AuthMethod == "api_key" {
				if err := store.DeleteAPIKey(name); err != nil {
					fmt.Fprintf(os.Stderr, "  Warning: could not remove API key for %q: %v\n", name, err)
				} else {
					fmt.Printf("  Removed API key for profile %q from keychain\n", name)
				}
			}
			if err := deleteOAuthKeychain(p.Dir); err != nil {
				fmt.Fprintf(os.Stderr, "  Warning: could not remove OAuth keychain entry for %q: %v\n", name, err)
			}
		}
		def, hasDefault := cfg.Profiles[cfg.DefaultProfile]
		releaseSystemDefault(hasDefault && def.AuthMethod == "api_key")
	} else {
		releaseSystemDefault(false)
	}

	// Remove the entire ~/.ccpm directory
	baseDir, err := config.BaseDir()
	if err != nil {
		return fmt.Errorf("could not determine config directory: %w", err)
	}

	if err := os.RemoveAll(baseDir); err != nil {
		return fmt.Errorf("could not remove %s: %w", baseDir, err)
	}
	fmt.Printf("  Removed %s\n", baseDir)

	// The master key only decrypts vault/*.enc, which are gone with ~/.ccpm —
	// so it is deleted only after that removal succeeded.
	if err := newKeystore().DeleteVaultMasterKey(); err != nil {
		fmt.Fprintf(os.Stderr, "  Warning: could not remove vault master key from keychain: %v\n", err)
	} else {
		fmt.Println("  Removed vault master key from keychain")
	}

	fmt.Println()
	green.Println("ccpm data removed.")
	fmt.Println()
	fmt.Println("To finish uninstalling, remove the binary and shell hook:")
	fmt.Println()

	// Find where the binary is
	binaryPath, _ := os.Executable()
	if binaryPath != "" {
		fmt.Printf("  rm %s\n", binaryPath)
	} else {
		fmt.Println("  rm $(which ccpm)")
	}
	fmt.Println()
	fmt.Println("  # Remove this line from your ~/.zshrc or ~/.bashrc:")
	fmt.Println("  # eval \"$(ccpm shell-init)\"")
	fmt.Println()

	return nil
}
