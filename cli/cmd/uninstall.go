package cmd

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/fatih/color"
	"github.com/spf13/cobra"

	"github.com/nitin-1926/ccpm/internal/config"
	"github.com/nitin-1926/ccpm/internal/keystore"
)

var uninstallCmd = &cobra.Command{
	Use:   "uninstall",
	Short: "Remove ccpm and all its data from your system",
	Long: `Completely removes ccpm from your system:
  - Deletes all profiles and their data (~/.ccpm/)
  - Removes API keys from your OS keychain
  - Removes vault master key from keychain
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
	fmt.Println("  - All API keys from your OS keychain")
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

	// Remove API keys from keychain
	if cfg != nil {
		store := keystore.New()
		for name, p := range cfg.Profiles {
			if p.AuthMethod == "api_key" {
				if err := store.DeleteAPIKey(name); err != nil {
					fmt.Fprintf(os.Stderr, "  Warning: could not remove API key for %q: %v\n", name, err)
				} else {
					fmt.Printf("  Removed API key for profile %q from keychain\n", name)
				}
			}
		}
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
