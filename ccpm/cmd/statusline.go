package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/spf13/cobra"

	"github.com/nitin-1926/claude-code-profile-manager/ccpm/internal/config"
)

// statusLineInput is the subset of the JSON Claude Code pipes to a statusLine
// command on stdin that ccpm renders. Claude Code sends many more fields; we
// decode only what we display so newer keys are ignored rather than erroring.
// See https://code.claude.com/docs/en/statusline for the full schema.
type statusLineInput struct {
	// Cwd duplicates workspace.current_dir; Claude Code sends both and
	// documents current_dir as preferred. Kept as a fallback for older clients.
	Cwd       string `json:"cwd"`
	Workspace struct {
		CurrentDir string `json:"current_dir"`
		ProjectDir string `json:"project_dir"`
		// Repo is present only inside a git repository that has an `origin`
		// remote configured.
		Repo *struct {
			Host  string `json:"host"`
			Owner string `json:"owner"`
			Name  string `json:"name"`
		} `json:"repo"`
	} `json:"workspace"`
	// Worktree is present only inside a Claude Code managed worktree, and is the
	// one place the payload names a git branch — everywhere else we read it off
	// disk ourselves.
	Worktree *struct {
		Branch string `json:"branch"`
	} `json:"worktree"`
	// Effort is present only when the active model supports the reasoning-effort
	// parameter, so the segment is absent for models that do not.
	Effort *struct {
		Level string `json:"level"`
	} `json:"effort"`
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
	Long: `Reads Claude Code's status JSON on stdin and prints a two-row status.

  Row 1  the active ccpm profile, the repo (with the subdirectory you are in),
         and the git branch.
  Row 2  the model, context usage, reasoning effort, the subscription usage
         windows (5h / 7d used, Pro/Max accounts only), and session cost.

Segments drop out when their data is absent, and a row with nothing to say is
not printed at all — so an API-key profile outside a repo collapses to a single
row. Output is ANSI-coloured unless NO_COLOR is set.

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

	// Claude Code renders each printed line as its own row.
	for _, row := range renderStatusLine(in, statusLineProfileName(), time.Now(), statusLineColorEnabled()) {
		fmt.Fprintln(cmd.OutOrStdout(), row)
	}
	return nil
}

// ANSI colors for the status line. Claude Code renders the statusLine command's
// stdout with ANSI interpreted, so these show up in the TUI. We honour the
// NO_COLOR convention (https://no-color.org) and TERM=dumb by emitting plain
// text instead — see statusLineColorEnabled.
const (
	cReset   = "\033[0m"
	cBold    = "\033[1m"
	cProfile = "\033[38;5;75m"  // profile name
	cModel   = "\033[38;5;176m" // model
	cGrey    = "\033[38;5;244m" // separators
	cGreen   = "\033[38;5;42m"  // healthy headroom / low ctx
	cAmber   = "\033[38;5;215m" // tightening
	cRed     = "\033[38;5;203m" // near the limit
	cOrange  = "\033[38;5;208m" // 5h / 7d window labels
	cYellow  = "\033[38;5;221m" // reset clock
	cCost    = "\033[38;5;109m" // estimated cost
	cDir     = "\033[38;5;110m" // repo / directory
	cBranch  = "\033[38;5;114m" // git branch
	cEffort  = "\033[38;5;180m" // reasoning effort
)

// statusLineColorEnabled reports whether to emit ANSI color, following the
// standard NO_COLOR convention and disabling on dumb terminals.
func statusLineColorEnabled() bool {
	if _, ok := os.LookupEnv("NO_COLOR"); ok {
		return false
	}
	return os.Getenv("TERM") != "dumb"
}

// paint wraps s in an ANSI color when on; otherwise returns s unchanged so the
// renderer stays a pure string builder that tests can assert against plainly.
func paint(on bool, code, s string) string {
	if !on {
		return s
	}
	return code + s + cReset
}

// headroomColor grades a usage window by how much is LEFT: healthy at ≥50%
// remaining, tightening at ≥20%, near-limit below that.
func headroomColor(remaining int) string {
	switch {
	case remaining >= 50:
		return cGreen
	case remaining >= 20:
		return cAmber
	default:
		return cRed
	}
}

// ctxColor grades the context window by how full it is (percent used).
func ctxColor(used int) string {
	switch {
	case used >= 80:
		return cRed
	case used >= 50:
		return cAmber
	default:
		return cGreen
	}
}

// renderStatusLine builds the status line from decoded input, as one row per
// returned string — Claude Code renders each printed line as its own row.
//
// Row 1 is where you are: profile, repo/directory, branch.
// Row 2 is what it is costing: model, context, effort, the usage windows, spend.
//
// Splitting them this way keeps the volatile numbers on their own row, so the
// identity line stays still while usage ticks. Pure (the clock and the color
// toggle are injected) so it is unit-testable. Segments drop out when their data
// is absent, and a row with no segments is omitted entirely rather than printed
// blank — an API-key profile outside a repo collapses to a single row.
func renderStatusLine(in statusLineInput, profile string, now time.Time, color bool) []string {
	sep := " · "
	if color {
		sep = " " + cGrey + "·" + cReset + " "
	}

	var where []string
	if profile != "" {
		where = append(where, paint(color, cBold+cProfile, "⬢ "+profile))
	}
	if loc := workspaceLabel(in); loc != "" {
		where = append(where, paint(color, cDir, loc))
	}
	if br := statusLineBranch(in); br != "" {
		where = append(where, paint(color, cBranch, "⎇ "+br))
	}

	var usage []string
	if name := in.Model.DisplayName; name != "" {
		usage = append(usage, paint(color, cModel, name))
	} else if in.Model.ID != "" {
		usage = append(usage, paint(color, cModel, in.Model.ID))
	}
	if in.ContextWindow.UsedPercentage > 0 {
		pct := roundPct(in.ContextWindow.UsedPercentage)
		usage = append(usage, paint(color, ctxColor(pct), fmt.Sprintf("ctx %d%%", pct)))
	}
	if in.Effort != nil && in.Effort.Level != "" {
		usage = append(usage, paint(color, cEffort, "effort "+in.Effort.Level))
	}
	if in.RateLimits != nil {
		if w := in.RateLimits.FiveHour; w != nil {
			usage = append(usage, formatWindow("5h", w, now, color))
		}
		if w := in.RateLimits.SevenDay; w != nil {
			usage = append(usage, formatWindow("7d", w, now, color))
		}
	}
	if in.Cost.TotalCostUSD > 0 {
		usage = append(usage, paint(color, cCost, fmt.Sprintf("$%.2f", in.Cost.TotalCostUSD)))
	}

	rows := make([]string, 0, 2)
	for _, segs := range [][]string{where, usage} {
		if len(segs) > 0 {
			rows = append(rows, strings.Join(segs, sep))
		}
	}
	return rows
}

// workspaceLabel renders "repo/subdir", or just the directory name when there is
// no repo. The subdirectory is included only when the session has moved below
// the launch directory, which is exactly when the repo name alone stops being
// enough to say where you are.
func workspaceLabel(in statusLineInput) string {
	cur := in.Workspace.CurrentDir
	if cur == "" {
		cur = in.Cwd
	}
	root := in.Workspace.ProjectDir

	name := ""
	if in.Workspace.Repo != nil {
		name = safeLabel(in.Workspace.Repo.Name)
	}
	if name == "" && root != "" {
		name = safeLabel(filepath.Base(root))
	}
	if name == "" {
		if cur == "" {
			return ""
		}
		return safeLabel(filepath.Base(cur))
	}
	if cur == "" || root == "" {
		return name
	}
	rel, err := filepath.Rel(root, cur)
	if err != nil || rel == "." || rel == "" || strings.HasPrefix(rel, "..") {
		return name
	}
	// Forward slashes on every platform: this is a display label in the shape
	// "repo/subdir", not a path anyone will open. Joining with the OS separator
	// rendered the same session as repo\sub on Windows and repo/sub elsewhere,
	// for a string whose whole job is to read the same to everyone.
	return name + "/" + safeLabel(filepath.ToSlash(rel))
}

// statusLineBranch resolves the current git branch.
//
// The status-line payload carries a branch only for Claude Code's own
// worktrees, so everywhere else it is read straight off disk. That is
// deliberate: the documented approach is to shell out to `git branch
// --show-current`, but this command runs on every assistant message, and
// spawning a process each time is exactly the cost the docs warn about. Reading
// .git/HEAD is a couple of stats and one small read, with no subprocess and
// nothing to cache.
func statusLineBranch(in statusLineInput) string {
	if in.Worktree != nil && in.Worktree.Branch != "" {
		return safeLabel(in.Worktree.Branch)
	}
	dir := in.Workspace.CurrentDir
	if dir == "" {
		dir = in.Cwd
	}
	return gitBranchAt(dir)
}

// gitBranchAt walks up from dir looking for a .git entry and reads the branch
// out of its HEAD. Returns "" when there is no repository, and a short SHA when
// HEAD is detached.
func gitBranchAt(dir string) string {
	if dir == "" {
		return ""
	}
	gitPath := findGitEntry(dir)
	if gitPath == "" {
		return ""
	}
	head, err := os.ReadFile(filepath.Join(gitPath, "HEAD"))
	if err != nil {
		return ""
	}
	return branchFromHead(string(head))
}

// findGitEntry returns the git directory governing dir, or "".
//
// A .git FILE rather than a directory means a linked worktree or a submodule;
// it holds "gitdir: <path>" pointing at the real one, which is where HEAD lives.
func findGitEntry(dir string) string {
	// Bounded so a pathological path cannot walk forever.
	for range 64 {
		candidate := filepath.Join(dir, ".git")
		fi, err := os.Stat(candidate)
		switch {
		case err == nil && fi.IsDir():
			return candidate
		case err == nil:
			b, rerr := os.ReadFile(candidate)
			if rerr != nil {
				return ""
			}
			target := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(string(b)), "gitdir:"))
			if target == "" {
				return ""
			}
			if !filepath.IsAbs(target) {
				target = filepath.Join(dir, target)
			}
			return target
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return ""
		}
		dir = parent
	}
	return ""
}

// branchFromHead parses a .git/HEAD payload: a symbolic ref for a branch, or a
// raw object id when detached.
func branchFromHead(head string) string {
	h := strings.TrimSpace(head)
	if rest, ok := strings.CutPrefix(h, "ref:"); ok {
		ref := strings.TrimSpace(rest)
		// refs/heads/feat/x -> feat/x, keeping slashes inside the branch name.
		if name, ok := strings.CutPrefix(ref, "refs/heads/"); ok {
			return safeLabel(name)
		}
		return safeLabel(filepath.Base(ref))
	}
	if len(h) >= 7 && isHex(h) {
		return h[:7] // detached HEAD
	}
	return ""
}

// labelRunes caps a display label. Git's practical ref limit is far below this;
// anything longer is not a branch name.
const labelRunes = 96

// safeLabel makes a string read from disk safe to print into a terminal.
//
// This output is rendered by Claude Code with ANSI interpreted, on every
// assistant message. .git/HEAD is just a file — git's own check-ref-format
// forbids control characters, but nothing writes that file through git when a
// .git arrives out of band (an extracted archive, a synced tree), and
// findGitEntry additionally follows a `gitdir:` pointer to an arbitrary path.
// Without this, a crafted HEAD injects escape sequences that can retitle the
// window, clear lines, or repaint the rows above — including the ones carrying
// permission prompts.
//
// Rejects rather than strips: a branch name containing control bytes is not a
// branch name, and showing a silently-mangled one is worse than showing none.
func safeLabel(s string) string {
	if s == "" || utf8.RuneCountInString(s) > labelRunes {
		return ""
	}
	// Validity first, and not merely for tidiness: a RAW 0x9b byte is a
	// single-byte CSI on many emulators and is not valid UTF-8 on its own, so
	// ranging over runes decodes it to RuneError (0xFFFD) — which sails past a
	// C1 range check. Rejecting invalid encoding is what actually catches it.
	if !utf8.ValidString(s) {
		return ""
	}
	for _, r := range s {
		// C0, DEL, and the C1 range. Filtering only ESC is not enough.
		if r < 0x20 || r == 0x7f || (r >= 0x80 && r <= 0x9f) {
			return ""
		}
	}
	return s
}

func isHex(s string) bool {
	for i := 0; i < len(s); i++ {
		c := s[i]
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') && (c < 'A' || c > 'F') {
			return false
		}
	}
	return len(s) > 0
}

// formatWindow renders a rate-limit window as label + percent-USED + reset
// clock, e.g. "5h 42% ↺16:15". The JSON reports percent used; we show it
// directly so the number matches Claude Code's own /usage panel (which reports
// used, not remaining). The percent is coloured by remaining headroom, the
// label orange, and the clock yellow.
func formatWindow(label string, w *rateWindow, now time.Time, color bool) string {
	used := roundPct(w.UsedPercentage)
	remaining := 100 - used
	if remaining < 0 {
		remaining = 0
	}
	s := paint(color, cOrange, label) + " " + paint(color, headroomColor(remaining), fmt.Sprintf("%d%%", used))
	if w.ResetsAt > 0 {
		if reset := time.Unix(w.ResetsAt, 0); reset.After(now) {
			s += " " + paint(color, cYellow, "↺"+resetClock(reset, now))
		}
	}
	return s
}

// resetClock formats a reset time, naming the weekday once it is not today.
// A bare "08:25" on the seven-day window reads as this morning when it is
// actually two days out, which is the opposite of the reassurance it should
// give; the five-hour window always lands today and stays a plain clock.
func resetClock(reset, now time.Time) string {
	if reset.YearDay() == now.YearDay() && reset.Year() == now.Year() {
		return reset.Format("15:04")
	}
	if reset.Sub(now) < 7*24*time.Hour {
		return reset.Format("Mon 15:04")
	}
	return reset.Format("2 Jan 15:04")
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
