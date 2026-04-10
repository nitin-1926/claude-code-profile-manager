package cmd

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"

	"github.com/fatih/color"
	"github.com/spf13/cobra"

	"github.com/nitin-1926/ccpm/internal/config"
)

var setDefaultCmd = &cobra.Command{
	Use:   "set-default <name>",
	Short: "Set profile as default for VS Code / IDE extension",
	Args:  cobra.ExactArgs(1),
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
	name := args[0]

	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	p, exists := cfg.Profiles[name]
	if !exists {
		return fmt.Errorf("profile %q not found", name)
	}

	// On Linux/Windows, copy .credentials.json to ~/.claude/
	if runtime.GOOS != "darwin" && p.AuthMethod == "oauth" {
		if err := copyCredentialsToDefault(p.Dir); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: could not copy credentials to ~/.claude/: %v\n", err)
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

	cfg.DefaultProfile = ""
	if err := config.Save(cfg); err != nil {
		return fmt.Errorf("saving config: %w", err)
	}

	fmt.Println("Default profile cleared.")
	return nil
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
