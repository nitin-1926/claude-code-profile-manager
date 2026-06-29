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

