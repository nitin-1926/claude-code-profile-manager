package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	claudepkg "github.com/nitin-1926/ccpm/internal/claude"
	"github.com/nitin-1926/ccpm/internal/config"
	"github.com/nitin-1926/ccpm/internal/keystore"
)

var runCmd = &cobra.Command{
	Use:   "run <name> [-- claude-args...]",
	Short: "Launch Claude Code with the given profile",
	Long:  "Starts Claude Code with CLAUDE_CONFIG_DIR set to the profile's directory. Pass additional args to claude after --.",
	Args:  cobra.MinimumNArgs(1),
	RunE:  runRun,
}

func init() {
	rootCmd.AddCommand(runCmd)
}

func runRun(cmd *cobra.Command, args []string) error {
	name := args[0]
	claudeArgs := args[1:]

	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	p, exists := cfg.Profiles[name]
	if !exists {
		return fmt.Errorf("profile %q not found. Run 'ccpm list' to see available profiles", name)
	}

	// Update last used
	cfg.UpdateLastUsed(name)
	_ = config.Save(cfg)

	// Get API key if needed
	var apiKey string
	if p.AuthMethod == "api_key" {
		store := keystore.New()
		apiKey, err = store.GetAPIKey(name)
		if err != nil {
			return fmt.Errorf("retrieving API key: %w\nRun 'ccpm auth refresh %s' to re-enter your key", err, name)
		}
	}

	// Exec replaces this process with claude
	return claudepkg.Exec(p.Dir, apiKey, claudeArgs)
}
