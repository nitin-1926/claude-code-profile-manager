package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/nitin-1926/claude-code-profile-manager/ccpm/internal/config"
)

// statusLineInput is the subset of the JSON Claude Code pipes to a statusLine
// command on stdin that ccpm renders. Claude Code sends many more fields; we
// decode only what we display so newer keys are ignored rather than erroring.
// See https://code.claude.com/docs/en/statusline for the full schema.
type statusLineInput struct {
	Model struct {
		ID          string `json:"id"`
		DisplayName string `json:"display_name"`
	} `json:"model"`
	Cost struct {
		TotalCostUSD float64 `json:"total_cost_usd"`
	} `json:"cost"`
	ContextWindow struct {
		UsedPercentage float64 `json:"used_percentage"`
	} `json:"context_window"`
	// RateLimits is present only for Claude.ai Pro/Max subscription accounts,
	// and only after the first API response in a session. It is absent for
	// API-key profiles — the window segments simply don't render.
	RateLimits *struct {
		FiveHour *rateWindow `json:"five_hour"`
		SevenDay *rateWindow `json:"seven_day"`
	} `json:"rate_limits"`
}

type rateWindow struct {
	UsedPercentage float64 `json:"used_percentage"`
	ResetsAt       int64   `json:"resets_at"`
}

var statusLineRenderCmd = &cobra.Command{
	Use:   "statusline",
	Short: "Render the Claude Code status line for the active ccpm profile",
	Long: `Reads Claude Code's status JSON on stdin and prints a one-line status
showing the active ccpm profile, model, context usage, subscription usage
windows (5h / 7d remaining, Pro/Max accounts only), and session cost.

You don't normally run this yourself — Claude Code invokes it as the
configured statusLine command. 'ccpm run' wires it in automatically for
profiles that have no statusLine of their own. Turn that off with
'ccpm config set statusline false' or per-launch with
'ccpm run <name> --no-statusline'; remove an injected one with
'ccpm settings statusline "" --profile <name>'.`,
	Args:          cobra.NoArgs,
	SilenceErrors: true,
	SilenceUsage:  true,
	RunE:          runStatusLineRender,
}

func init() {
	rootCmd.AddCommand(statusLineRenderCmd)
}

func runStatusLineRender(cmd *cobra.Command, args []string) error {
	// A status line must never be noisy: on any problem print nothing (or just
	// the profile) and exit 0 so Claude Code's TUI isn't polluted with errors.
	raw, err := io.ReadAll(io.LimitReader(cmd.InOrStdin(), 1<<20))
	if err != nil {
		return nil
	}
	var in statusLineInput
	_ = json.Unmarshal(raw, &in) // best-effort; missing fields just don't render

	line := renderStatusLine(in, statusLineProfileName(), time.Now())
	if line != "" {
		fmt.Fprintln(cmd.OutOrStdout(), line)
	}
	return nil
}

// renderStatusLine builds the status line from decoded input. Pure (now is
// injected) so it is unit-testable. Segments drop out when their data is
// absent, so an API-key profile collapses to "⬢ work · Opus 4.8 · $0.12".
func renderStatusLine(in statusLineInput, profile string, now time.Time) string {
	var segs []string
	if profile != "" {
		segs = append(segs, "⬢ "+profile)
	}
	if name := in.Model.DisplayName; name != "" {
		segs = append(segs, name)
	} else if in.Model.ID != "" {
		segs = append(segs, in.Model.ID)
	}
	if in.ContextWindow.UsedPercentage > 0 {
		segs = append(segs, fmt.Sprintf("ctx %d%%", roundPct(in.ContextWindow.UsedPercentage)))
	}
	if in.RateLimits != nil {
		if w := in.RateLimits.FiveHour; w != nil {
			segs = append(segs, formatWindow("5h", w, now))
		}
		if w := in.RateLimits.SevenDay; w != nil {
			segs = append(segs, formatWindow("7d", w, now))
		}
	}
	if in.Cost.TotalCostUSD > 0 {
		segs = append(segs, fmt.Sprintf("$%.2f", in.Cost.TotalCostUSD))
	}
	return strings.Join(segs, " · ")
}

// formatWindow renders a rate-limit window as remaining-percent plus reset
// clock, e.g. "5h 58% ↺16:15". The JSON reports percent USED; we show what is
// left because that's the number users actually watch.
func formatWindow(label string, w *rateWindow, now time.Time) string {
	remaining := 100 - roundPct(w.UsedPercentage)
	if remaining < 0 {
		remaining = 0
	}
	s := fmt.Sprintf("%s %d%%", label, remaining)
	if w.ResetsAt > 0 {
		if reset := time.Unix(w.ResetsAt, 0); reset.After(now) {
			s += " ↺" + reset.Format("15:04")
		}
	}
	return s
}

func roundPct(p float64) int {
	return int(p + 0.5)
}

// statusLineProfileName resolves which ccpm profile this session belongs to,
// cheaply and without ever failing (it runs on every status render). Order:
// $CCPM_ACTIVE_PROFILE, then $CLAUDE_CONFIG_DIR matched back to a known profile
// dir. Unlike `ccpm prompt`, it never falls back to the configured default —
// the status line reflects the session's actual binding, not a guess.
func statusLineProfileName() string {
	if n := strings.TrimSpace(os.Getenv("CCPM_ACTIVE_PROFILE")); n != "" {
		return n
	}
	dir := strings.TrimSpace(os.Getenv("CLAUDE_CONFIG_DIR"))
	if dir == "" {
		return ""
	}
	cfg, err := config.Load()
	if err != nil {
		return ""
	}
	want := filepath.Clean(dir)
	for name, p := range cfg.Profiles {
		if filepath.Clean(p.Dir) == want {
			return name
		}
	}
	return ""
}
