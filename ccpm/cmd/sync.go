package cmd

import (
	"errors"
	"fmt"
	"os"

	"github.com/fatih/color"
	"github.com/spf13/cobra"

	"github.com/nitin-1926/claude-code-profile-manager/ccpm/internal/config"
	"github.com/nitin-1926/claude-code-profile-manager/ccpm/internal/picker"
	"github.com/nitin-1926/claude-code-profile-manager/ccpm/internal/settingsmerge"
	profilesync "github.com/nitin-1926/claude-code-profile-manager/ccpm/internal/sync"
)

var (
	syncProfile string
	syncAll     bool
)

var syncCmd = &cobra.Command{
	Use:   "sync",
	Short: "Sync global installs into a profile",
	Long: `Apply all globally installed skills, MCP servers, and settings to
a specific profile. This happens automatically on 'ccpm add' and 'ccpm run',
but you can run it manually to force a sync.`,
	RunE: runSync,
}

func init() {
	syncCmd.Flags().StringVar(&syncProfile, "profile", "", "profile to sync (prompts when omitted in a TTY)")
	syncCmd.Flags().BoolVar(&syncAll, "all", false, "sync all profiles without prompting")
	rootCmd.AddCommand(syncCmd)
}

func runSync(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	var targets []string
	switch {
	case syncProfile != "":
		if _, exists := cfg.Profiles[syncProfile]; !exists {
			return fmt.Errorf("profile %q not found", syncProfile)
		}
		targets = []string{syncProfile}
	case syncAll:
		targets = config.ProfileNames(cfg)
	default:
		names := config.ProfileNames(cfg)
		if len(names) == 0 {
			return fmt.Errorf("no profiles exist yet — create one with `ccpm add <name>`")
		}
		opts := make([]picker.Option, len(names))
		for i, n := range names {
			opts[i] = picker.Option{Value: n, Label: n}
		}
		chosen, err := picker.MultiSelect("Which profiles should we sync?", opts, names)
		if err != nil {
			if errors.Is(err, picker.ErrNonInteractive) {
				// Preserve historical behavior: sync all profiles non-interactively.
				targets = names
			} else {
				return err
			}
		} else if len(chosen) == 0 {
			return fmt.Errorf("no profiles selected")
		} else {
			targets = chosen
		}
	}

	green := color.New(color.FgGreen, color.Bold)

	for _, name := range targets {
		p := cfg.Profiles[name]

		if err := profilesync.ApplyGlobals(p.Dir, name); err != nil {
			fmt.Printf("  Warning: skills sync failed for %q: %v\n", name, err)
		}

		if err := settingsmerge.MaterializeAll(p.Dir, name, ""); err != nil {
			fmt.Printf("  Warning: profile materialization failed for %q: %v\n", name, err)
		}

		green.Printf("✓ Synced profile %q\n", name)
	}

	// Garbage-collect plugin cache entries no profile references. Hooked into
	// sync so users don't accumulate disk usage from removed plugins; failure
	// is non-fatal because GC is opportunistic.
	if err := runPluginGC(); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: plugin gc failed: %v\n", err)
	}

	return nil
}
