package cmd

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/fatih/color"
	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/nitin-1926/ccpm/internal/claude"
	"github.com/nitin-1926/ccpm/internal/config"
	"github.com/nitin-1926/ccpm/internal/defaultclaude"
	"github.com/nitin-1926/ccpm/internal/keystore"
	"github.com/nitin-1926/ccpm/internal/picker"
	"github.com/nitin-1926/ccpm/internal/profile"
	"github.com/nitin-1926/ccpm/internal/settingsmerge"
	profilesync "github.com/nitin-1926/ccpm/internal/sync"
	"github.com/nitin-1926/ccpm/internal/wizard"
)

var addCmd = &cobra.Command{
	Use:   "add <name>",
	Short: "Create a new profile and set up authentication",
	Long:  "Creates a new Claude Code profile directory and guides you through OAuth or API key authentication.",
	Args:  cobra.ExactArgs(1),
	RunE:  runAdd,
}

func init() {
	rootCmd.AddCommand(addCmd)
}

func runAdd(cmd *cobra.Command, args []string) error {
	name := args[0]

	if err := profile.ValidateName(name); err != nil {
		return err
	}

	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	if _, exists := cfg.Profiles[name]; exists {
		return fmt.Errorf("profile %q already exists", name)
	}

	scanner := bufio.NewScanner(os.Stdin)

	authMethod, err := pickAuthMethod(scanner)
	if err != nil {
		return err
	}

	// Create profile directory
	dir, err := profile.Create(name)
	if err != nil {
		return err
	}

	bold := color.New(color.Bold)
	green := color.New(color.FgGreen, color.Bold)

	// Import-source wizard: offer to seed the new profile from ~/.claude or
	// another existing profile before we hit the auth step. Failures here
	// are surfaced as warnings — the profile is already created and the
	// user can still import later.
	existing := config.ProfileNames(cfg)
	if defaultclaude.Exists() || len(existing) > 0 {
		fmt.Println()
		decision, err := wizard.PromptImportSource(os.Stdin, os.Stdout, existing, defaultclaude.Exists())
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: import wizard failed: %v\n", err)
		} else if decision.Source != wizard.SourceScratch {
			if err := applyImportDecision(dir, name, decision, cfg); err != nil {
				fmt.Fprintf(os.Stderr, "Warning: import step failed: %v\n", err)
			}
		}
	}

	switch authMethod {
	case "oauth":
		fmt.Println()
		bold.Println("OAuth Authentication")
		fmt.Println("Claude Code will launch now. Run /login inside to authenticate.")
		fmt.Println("After logging in, type /exit or press Ctrl+C to return to ccpm.")
		fmt.Println()

		if err := claude.Spawn(dir); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: claude exited with error: %v\n", err)
		}

		// Verify credentials landed
		credFile := dir + "/.credentials.json"
		if _, err := os.Stat(credFile); os.IsNotExist(err) {
			fmt.Fprintf(os.Stderr, "Warning: no credentials file found at %s\n", credFile)
			fmt.Fprintln(os.Stderr, "You may need to run: ccpm auth refresh", name)
		} else {
			green.Printf("✓ Profile %q authenticated via OAuth\n", name)
		}

	case "api_key":
		fmt.Println()
		bold.Println("API Key Authentication")
		fmt.Print("Enter your Anthropic API key: ")

		var key string
		if term.IsTerminal(int(os.Stdin.Fd())) {
			keyBytes, err := term.ReadPassword(int(os.Stdin.Fd()))
			fmt.Println()
			if err != nil {
				profile.Remove(name)
				return fmt.Errorf("reading API key: %w", err)
			}
