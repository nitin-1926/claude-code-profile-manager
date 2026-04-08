package cmd

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/fatih/color"
	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/nitin-1926/ccpm/internal/claude"
	"github.com/nitin-1926/ccpm/internal/config"
	"github.com/nitin-1926/ccpm/internal/keystore"
	"github.com/nitin-1926/ccpm/internal/profile"
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

	// Prompt for auth method
	fmt.Println("Choose authentication method:")
	fmt.Println("  1) OAuth (browser login via claude /login)")
	fmt.Println("  2) API Key (enter your Anthropic API key)")
	fmt.Print("Enter choice [1/2]: ")

	var authMethod string
	if scanner.Scan() {
		choice := strings.TrimSpace(scanner.Text())
		switch choice {
		case "1", "oauth", "":
			authMethod = "oauth"
		case "2", "api_key", "api-key", "apikey":
			authMethod = "api_key"
		default:
			return fmt.Errorf("invalid choice %q, expected 1 or 2", choice)
		}
	} else {
		return fmt.Errorf("no input received")
	}

	// Create profile directory
	dir, err := profile.Create(name)
	if err != nil {
		return err
	}

	bold := color.New(color.Bold)
	green := color.New(color.FgGreen, color.Bold)

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
			key = strings.TrimSpace(string(keyBytes))
		} else {
			if scanner.Scan() {
				key = strings.TrimSpace(scanner.Text())
			}
		}

		if key == "" {
			profile.Remove(name)
			return fmt.Errorf("API key cannot be empty")
		}

		store := keystore.New()
		if err := store.SetAPIKey(name, key); err != nil {
			profile.Remove(name)
			return fmt.Errorf("storing API key: %w", err)
		}

		green.Printf("✓ Profile %q authenticated via API key\n", name)
	}

	// Save to config
	cfg.AddProfile(name, dir, authMethod)
	if err := config.Save(cfg); err != nil {
		return fmt.Errorf("saving config: %w", err)
	}

	fmt.Printf("\nRun claude with this profile:\n  ccpm run %s\n", name)
	return nil
}
