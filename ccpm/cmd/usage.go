package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/fatih/color"
	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/nitin-1926/claude-code-profile-manager/ccpm/internal/config"
	"github.com/nitin-1926/claude-code-profile-manager/ccpm/internal/usage"
)

var (
	usageAll       bool
	usageJSON      bool
	usageSince     string
	usageByModel   bool
	usageByProject bool
	usageSessions  bool
	usagePlain     bool
)

var usageCmd = &cobra.Command{
	Use:   "usage [profile]",
	Short: "Show token usage for a profile from its Claude Code transcripts",
	Long: `Report token usage for a profile, read from the Claude Code session
transcripts it accumulates under <profileDir>/projects/.

With no argument it uses the active profile ($CCPM_ACTIVE_PROFILE or the profile
bound to $CLAUDE_CONFIG_DIR), then the configured default. Pass a name to target
one profile, or --all to aggregate every profile.

Counts are raw token totals (input, output, cache-write, cache-read) — no dollar
cost is computed. The default view shows totals, a contribution heatmap, and a
per-model breakdown; --by-project, --by-model, and --sessions swap the body.

Usage data is maintained incrementally in <profileDir>/usage/; each run reads
only the new transcript bytes since the last one.`,
	Args: cobra.MaximumNArgs(1),
	RunE: runUsage,
}

// usageSyncCmd is the hook entry point: it refreshes the store and stays silent
// so it can run from a SessionEnd hook without polluting the session.
var usageSyncCmd = &cobra.Command{
	Use:           "sync [profile]",
	Short:         "Refresh the usage store for a profile (used by the SessionEnd hook)",
	Hidden:        true,
	Args:          cobra.MaximumNArgs(1),
	SilenceErrors: true,
	SilenceUsage:  true,
	RunE:          runUsageSync,
}

func init() {
	usageCmd.Flags().BoolVar(&usageAll, "all", false, "aggregate every profile")
	usageCmd.Flags().BoolVar(&usageJSON, "json", false, "machine-readable JSON output")
	usageCmd.Flags().StringVar(&usageSince, "since", "", "limit to a window: duration (168h), days (30d), or date (2026-06-01)")
	usageCmd.Flags().BoolVar(&usageByModel, "by-model", false, "break usage down by model")
	usageCmd.Flags().BoolVar(&usageByProject, "by-project", false, "break usage down by project (cwd)")
	usageCmd.Flags().BoolVar(&usageSessions, "sessions", false, "list sessions with per-session tokens (resume-style)")
	usageCmd.Flags().BoolVar(&usagePlain, "plain", false, "print a static report instead of the interactive dashboard")
	usageCmd.AddCommand(usageSyncCmd)
	rootCmd.AddCommand(usageCmd)
}

// usageProfileName resolves the single target profile: explicit arg, then the
// active session's profile, then the configured default.
func usageProfileName(cfg *config.Config, args []string) (string, error) {
	if len(args) == 1 {
		return args[0], nil
	}
	if n := statusLineProfileName(); n != "" {
		return n, nil
	}
	if cfg.DefaultProfile != "" {
		return cfg.DefaultProfile, nil
	}
	return "", fmt.Errorf("no profile given and none active — pass a profile name or set a default with `ccpm set-default`")
}

func runUsage(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	var names []string
	if usageAll {
		for n := range cfg.Profiles {
			names = append(names, n)
		}
		sort.Strings(names)
		if len(names) == 0 {
			return fmt.Errorf("no profiles exist yet")
		}
	} else {
		name, err := usageProfileName(cfg, args)
		if err != nil {
			return err
		}
		names = []string{name}
	}

	sinceDate, err := usage.ParseSince(usageSince, time.Now())
	if err != nil {
		return err
	}

	if usageJSON {
		return emitUsageJSON(cmd, cfg, names, sinceDate)
	}

	// Interactive dashboard by default on a real terminal — unless a flag asks
	// for a specific static view, output is piped, or --plain is set.
	if !usagePlain && !usageByModel && !usageByProject && !usageSessions && !usageAll &&
		usageSince == "" && term.IsTerminal(int(os.Stdout.Fd())) {
		return runUsageTUI(cfg, sortedProfileNames(cfg), names[0])
	}

	for i, name := range names {
		p, ok := cfg.Profiles[name]
		if !ok {
			return fmt.Errorf("profile %q not found", name)
		}
		sess, day, serr := usage.Sync(p.Dir)
		if serr != nil {
			// Sync may be contended or the dir unwritable; fall back to whatever
			// is already on disk so the report still renders.
			if sess, day, err = usage.Load(p.Dir); err != nil {
				return fmt.Errorf("reading usage for %q: %w", name, err)
			}
		}
		if i > 0 {
			fmt.Fprintln(cmd.OutOrStdout())
		}
		renderUsage(cmd.OutOrStdout(), name, usage.BuildView(sess, day, sinceDate), day, sinceDate)
	}
	return nil
}

// sortedProfileNames returns every profile name, sorted — the profile axis the
// interactive dashboard cycles through.
func sortedProfileNames(cfg *config.Config) []string {
	names := make([]string, 0, len(cfg.Profiles))
	for n := range cfg.Profiles {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

