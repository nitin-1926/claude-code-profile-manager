package cmd

import (
	"fmt"
	"os"
	"time"

	"github.com/fatih/color"

	"github.com/nitin-1926/ccpm/internal/config"
	"github.com/nitin-1926/ccpm/internal/defaultclaude"
)

// nudgeInterval debounces the drift reminder so users who leave the setting
// on don't see the same warning on every launch.
const nudgeInterval = 24 * time.Hour

// maybeNudgeDefaultDrift writes a short, non-blocking reminder to stderr
// when ~/.claude has changed since the last fingerprint was taken.
//
// It is a no-op unless Settings.CheckDefaultDrift is true and the default
// tree exists. Failures are swallowed to keep the diagnostic path harmless.
func maybeNudgeDefaultDrift(cfg *config.Config) {
	if cfg == nil || !cfg.Settings.CheckDefaultDrift {
		return
	}
	if !defaultclaude.Exists() {
		return
	}
	stored, err := defaultclaude.LoadFingerprint()
	if err != nil || stored == nil {
		return
	}
	if !defaultclaude.ShouldNudge(stored, nudgeInterval) {
		return
	}

	current, err := defaultclaude.Snapshot(defaultclaude.DefaultTargets())
	if err != nil {
		return
	}
	drift := defaultclaude.Compare(stored, current)
	if !drift.HasChanges() {
		return
	}

	yellow := color.New(color.FgYellow, color.Bold)
	yellow.Fprintf(os.Stderr, "ccpm: ~/.claude drift (+%d ~%d -%d) — run 'ccpm doctor' or 'ccpm import default --dry-run'\n",
		len(drift.Added), len(drift.Modified), len(drift.Removed))
	fmt.Fprintln(os.Stderr, "  (silence with 'ccpm config set check_default_drift false')")

	_ = defaultclaude.MarkNudged()
}
