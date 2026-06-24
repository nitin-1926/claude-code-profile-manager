package cmd

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/fatih/color"
	"github.com/spf13/cobra"

	"github.com/nitin-1926/claude-code-profile-manager/ccpm/internal/config"
	"github.com/nitin-1926/claude-code-profile-manager/ccpm/internal/picker"
	profilesync "github.com/nitin-1926/claude-code-profile-manager/ccpm/internal/sync"
)

var (
	syncProfile     string
	syncAll         bool
	syncNoAutoAdopt bool
	syncDryRun      bool
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
	syncCmd.Flags().BoolVar(&syncNoAutoAdopt, "no-auto-adopt", false, "skip the host-asset cascade scan for this run (does not change cascade_auto_adopt)")
	syncCmd.Flags().BoolVar(&syncDryRun, "dry-run", false, "print what sync would link/adopt without changing anything")
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

	if syncDryRun {
		// Cascade links and host adoption are GLOBAL — driven by the shared
		// manifest and the ~/.claude scan — so the preview below is identical
		// for every listed profile (that's why PreviewApplyGlobals ignores the
		// profile name). Only settings/MCP materialization differs per profile,
		// summarized at the end.
		cascade, adoptable, err := profilesync.PreviewApplyGlobals(targets[0])
		if err != nil {
			return err
		}
		fmt.Printf("Dry-run for profiles: %s\n", strings.Join(targets, ", "))
		if len(cascade) == 0 && len(adoptable) == 0 {
			fmt.Println("  nothing to link or adopt (settings + MCP would still be re-materialized per profile)")
			return nil
		}
		if len(cascade) > 0 {
			fmt.Println("  would (re-)link into every listed profile:")
			for _, c := range cascade {
				fmt.Printf("    → %s\n", c)
			}
		}
		if len(adoptable) > 0 {
			fmt.Println("  would adopt from ~/.claude into every listed profile:")
			for _, a := range adoptable {
				fmt.Printf("    + %s\n", a)
			}
		}
		fmt.Println("  settings + MCP would be re-materialized separately for each profile")
		return nil
	}

	// Locked after the interactive profile picker: the cascade mutates the
	// shared manifest (host adoption appends entries), so two concurrent
	// syncs — or a sync racing a `ccpm skill add` — would lose updates (H7).
	return withConfigLock(func() error {
		var failed []string
		for _, name := range targets {
			p := cfg.Profiles[name]

			if err := profilesync.ApplyGlobalsWithOptions(p.Dir, name, profilesync.Options{
				SkipHostAdoption: syncNoAutoAdopt,
			}); err != nil {
				fmt.Printf("  Warning: sync failed for %q: %v\n", name, err)
				failed = append(failed, name)
				continue
			}

			// MaterializeAll already runs inside ApplyGlobalsWithOptions; the
			// extra call here was double work and is intentionally removed.

			green.Printf("✓ Synced profile %q\n", name)
		}

		// Garbage-collect plugin cache entries no profile references. Hooked into
		// sync so users don't accumulate disk usage from removed plugins; failure
		// is non-fatal because GC is opportunistic.
		if err := runPluginGC(); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: plugin gc failed: %v\n", err)
		}

		// Per-profile failures surface as exit 3 (partial failure) so scripts can
		// tell "some profiles synced, some didn't" apart from a clean run —
		// matching `ccpm import`. A total failure where none synced is still 3
		// here because at least the GC/preview ran; callers key off the message.
		if len(failed) > 0 {
			return partialFailure("sync failed for %d profile(s): %s", len(failed), strings.Join(failed, ", "))
		}
		return nil
	})
}
