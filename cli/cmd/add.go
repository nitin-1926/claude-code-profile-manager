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

	// Auto set-default on first profile
	isFirstProfile := len(cfg.Profiles) == 0

	// Save to config
	cfg.AddProfile(name, dir, authMethod)
	if isFirstProfile {
		cfg.DefaultProfile = name
	}
	if err := config.Save(cfg); err != nil {
		return fmt.Errorf("saving config: %w", err)
	}

	if isFirstProfile {
		green.Printf("✓ Set as default profile (first profile)\n")
	}

	// Apply global installs (skills, etc.) to the new profile
	if err := profilesync.ApplyGlobals(dir, name); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: could not apply global installs: %v\n", err)
	}

	fmt.Printf("\nRun claude with this profile:\n  ccpm run %s\n", name)
	return nil
}

// pickAuthMethod asks the user to choose OAuth vs API key. Uses the interactive
// picker in a TTY; falls back to the legacy 1/2 numeric prompt when not, so CI
// and piped-stdin callers keep working.
func pickAuthMethod(scanner *bufio.Scanner) (string, error) {
	choice, err := picker.Select("Choose authentication method", []picker.Option{
		{Value: "oauth", Label: "OAuth", Description: "browser login via `claude /login`"},
		{Value: "api_key", Label: "API Key", Description: "paste an Anthropic API key"},
	})
	if err == nil {
		return choice, nil
	}
	if !errors.Is(err, picker.ErrNonInteractive) {
		return "", err
	}

	fmt.Println("Choose authentication method:")
	fmt.Println("  1) OAuth (browser login via claude /login)")
	fmt.Println("  2) API Key (enter your Anthropic API key)")
	fmt.Print("Enter choice [1/2]: ")
	if !scanner.Scan() {
		return "", fmt.Errorf("no input received")
	}
	raw := strings.TrimSpace(scanner.Text())
	switch raw {
	case "1", "oauth", "":
		return "oauth", nil
	case "2", "api_key", "api-key", "apikey":
		return "api_key", nil
	default:
		return "", fmt.Errorf("invalid choice %q, expected 1 or 2", raw)
	}
}

// applyImportDecision runs the import the wizard selected against the newly
// created profile directory. After any successful import we re-materialize
// settings + MCP so the new profile is launch-ready.
func applyImportDecision(profileDir, profileName string, d wizard.Decision, cfg *config.Config) error {
	switch d.Source {
	case wizard.SourceDefault:
		if _, err := defaultclaude.Import(profileDir, defaultclaude.ImportOptions{
			Targets:     d.Targets,
			Dedupe:      true,
			ProfileName: profileName,
		}); err != nil {
			return err
		}
		if err := mergeImportedSettings(profileDir); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: settings merge failed: %v\n", err)
		}
	case wizard.SourceProfile:
		srcProfile, ok := cfg.Profiles[d.ProfileName]
		if !ok {
			return fmt.Errorf("source profile %q not found", d.ProfileName)
		}
		if err := importFromProfile(srcProfile.Dir, profileDir, d.Targets, false); err != nil {
			return err
		}
	}

	if err := settingsmerge.Materialize(profileDir, profileName); err != nil {
		return fmt.Errorf("materializing settings: %w", err)
	}
	if err := settingsmerge.MaterializeMCP(profileDir, profileName); err != nil {
		return fmt.Errorf("materializing MCP: %w", err)
	}
	return nil
}
