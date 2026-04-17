package cmd

import (
	"fmt"

	"github.com/fatih/color"
	"github.com/spf13/cobra"

	"github.com/nitin-1926/ccpm/internal/config"
	"github.com/nitin-1926/ccpm/internal/settingsmerge"
	profilesync "github.com/nitin-1926/ccpm/internal/sync"
)

var syncProfile string

var syncCmd = &cobra.Command{
	Use:   "sync",
	Short: "Sync global installs into a profile",
	Long: `Apply all globally installed skills, MCP servers, and settings to
a specific profile. This happens automatically on 'ccpm add' and 'ccpm run',
but you can run it manually to force a sync.`,
	RunE: runSync,
}

func init() {
	syncCmd.Flags().StringVar(&syncProfile, "profile", "", "profile to sync (syncs all if omitted)")
	rootCmd.AddCommand(syncCmd)
}

func runSync(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	var targets []string
	if syncProfile != "" {
		if _, exists := cfg.Profiles[syncProfile]; !exists {
			return fmt.Errorf("profile %q not found", syncProfile)
		}
		targets = []string{syncProfile}
	} else {
		targets = config.ProfileNames(cfg)
	}

	green := color.New(color.FgGreen, color.Bold)

	for _, name := range targets {
		p := cfg.Profiles[name]

		if err := profilesync.ApplyGlobals(p.Dir, name); err != nil {
			fmt.Printf("  Warning: skills sync failed for %q: %v\n", name, err)
		}

		if err := settingsmerge.Materialize(p.Dir, name); err != nil {
			fmt.Printf("  Warning: settings materialization failed for %q: %v\n", name, err)
		}
		if err := settingsmerge.MaterializeMCP(p.Dir, name); err != nil {
			fmt.Printf("  Warning: MCP materialization failed for %q: %v\n", name, err)
		}

		green.Printf("✓ Synced profile %q\n", name)
	}

	return nil
}
