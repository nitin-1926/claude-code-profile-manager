package cmd

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/fatih/color"
	"github.com/spf13/cobra"

	"github.com/nitin-1926/claude-code-profile-manager/ccpm/internal/config"
	"github.com/nitin-1926/claude-code-profile-manager/ccpm/internal/picker"
	"github.com/nitin-1926/claude-code-profile-manager/ccpm/internal/statusline"
)

var (
	slConfigureProfile string
	slConfigureRow1    string
	slConfigureRow2    string
	slConfigureOff     string
	slConfigureReset   bool
)

var statusLineConfigureCmd = &cobra.Command{
	Use:   "configure",
	Short: "Choose which segments the status line shows, and on which row",
	Long: `Pick the segments ` + "`ccpm statusline`" + ` renders and which of its two rows
each one sits on.

Run with no flags in a terminal and it prompts: row 1 first, then row 2 from
what is left. Anything you do not pick is switched off. The chosen layout is
printed against sample data so you can see the result immediately.

Without --profile this sets the global default for every profile. With
--profile it sets an override for that profile alone, which wins over the
global default; --reset --profile <name> removes the override again.

Segments whose data Claude Code did not send are still skipped at render time,
so enabling the 5h window on an API-key profile shows nothing rather than a
blank segment.

For scripts and for the desktop app, pass the rows explicitly instead of
prompting. Every segment must be accounted for exactly once across the three
flags — an incomplete list is refused rather than filled in, because a segment
named nowhere is treated as newly introduced and placed at its default:

  ccpm statusline configure --row1 profile,workspace,branch,model,context \
                            --row2 effort,five_hour,seven_day,cost
  ccpm statusline configure --row1 profile,model --row2 five_hour,seven_day \
                            --off workspace,branch,context,effort,cost --profile work
  ccpm statusline configure --reset`,
	Args:         cobra.NoArgs,
	SilenceUsage: true,
	RunE:         runStatusLineConfigure,
}

func init() {
	f := statusLineConfigureCmd.Flags()
	f.StringVar(&slConfigureProfile, "profile", "", "configure this profile only, instead of the global default")
	f.StringVar(&slConfigureRow1, "row1", "", "comma-separated segments for row 1 (non-interactive)")
	f.StringVar(&slConfigureRow2, "row2", "", "comma-separated segments for row 2 (non-interactive)")
	f.StringVar(&slConfigureOff, "off", "", "comma-separated segments to hide (non-interactive)")
	f.BoolVar(&slConfigureReset, "reset", false, "restore the default layout, or drop a profile's override")

	statusLineRenderCmd.AddCommand(statusLineConfigureCmd)
}

func runStatusLineConfigure(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}
	if slConfigureProfile != "" {
		if _, ok := cfg.Profiles[slConfigureProfile]; !ok {
			return fmt.Errorf("profile %q not found", slConfigureProfile)
		}
	}

	explicit := slConfigureRow1 != "" || slConfigureRow2 != "" || slConfigureOff != ""
	if slConfigureReset && explicit {
		return errors.New("--reset cannot be combined with --row1/--row2/--off")
	}

	switch {
	case slConfigureReset:
		return applyStatusLineLayout(cfg, slConfigureProfile, nil)
	case explicit:
		layout, err := layoutFromFlags(slConfigureRow1, slConfigureRow2, slConfigureOff)
		if err != nil {
			return err
		}
		stored := layout.Store()
		return applyStatusLineLayout(cfg, slConfigureProfile, &stored)
	}

	return promptStatusLineLayout(cfg, slConfigureProfile)
}

// layoutFromFlags parses the three comma-separated flag values into a layout,
// insisting that every known segment appears exactly once across them.
//
// The strictness is deliberate. This is the path the desktop app writes
// through, and an omitted segment there would be ambiguous: Normalize treats a
// segment mentioned nowhere as newly-introduced and places it at its default,
// which is right for a config written by an older ccpm and wrong for a UI that
// simply forgot to send it. Requiring completeness means the caller's intent is
// never guessed.
func layoutFromFlags(row1, row2, off string) (statusline.Layout, error) {
	l := statusline.Layout{
		Row1: splitSegments(row1),
		Row2: splitSegments(row2),
		Off:  splitSegments(off),
	}

	seen := map[string]int{}
	for _, bucket := range [][]string{l.Row1, l.Row2, l.Off} {
		for _, k := range bucket {
			if !statusline.Known(k) {
				return statusline.Layout{}, fmt.Errorf("unknown segment %q (known: %s)", k, strings.Join(allSegments(), ", "))
			}
			seen[k]++
		}
	}

	var dupes, missing []string
	for _, s := range statusline.Segments {
		switch seen[s.Key] {
		case 1:
		case 0:
			missing = append(missing, s.Key)
		default:
			dupes = append(dupes, s.Key)
		}
	}
	if len(dupes) > 0 {
		sort.Strings(dupes)
		return statusline.Layout{}, fmt.Errorf("segment listed more than once: %s", strings.Join(dupes, ", "))
	}
	if len(missing) > 0 {
		return statusline.Layout{}, fmt.Errorf("every segment must be listed exactly once across --row1/--row2/--off; missing: %s", strings.Join(missing, ", "))
	}
	return l, nil
}

func splitSegments(s string) []string {
	out := []string{}
	for _, part := range strings.Split(s, ",") {
		if p := strings.TrimSpace(part); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func allSegments() []string {
	out := make([]string, 0, len(statusline.Segments))
	for _, s := range statusline.Segments {
		out = append(out, s.Key)
	}
	return out
}

// promptStatusLineLayout runs the interactive picker: row 1 first, then row 2
// over whatever is left, with the remainder switched off.
func promptStatusLineLayout(cfg *config.Config, profile string) error {
	if !picker.IsInteractive() {
		return errors.New("`ccpm statusline configure` needs a terminal; pass --row1/--row2/--off to set a layout non-interactively, or --reset to restore the default")
	}

	current := statusline.Global(cfg)
	scope := "every profile"
	if profile != "" {
		current = statusline.Resolve(cfg, profile)
		scope = fmt.Sprintf("profile %q", profile)
	}
	fmt.Printf("Configuring the status line for %s. Current layout:\n\n", scope)
	printStatusLinePreview(current)
	fmt.Println()

	row1, err := pickRow("Row 1 — which segments?", allSegments(), current.Row1)
	if err != nil {
		return err
	}

	// Row 2 is offered only what row 1 did not take, so a segment cannot end up
	// on both rows and the second prompt shrinks as the first grows.
	remaining := except(allSegments(), row1)
	row2, err := pickRow("Row 2 — which of the rest?", remaining, intersect(current.Row2, remaining))
	if err != nil {
		return err
	}

	layout := statusline.Layout{
		// huh returns selections in the order they were OFFERED, which is
		// catalog order — so a picker run would otherwise silently re-sort a row
		// the user had ordered by hand in config.json. Keeping the previous
		// order for segments that stayed put means opening the picker to toggle
		// one thing does not quietly rearrange everything else.
		Row1: keepOrder(row1, current.Row1),
		Row2: keepOrder(row2, current.Row2),
		Off:  except(remaining, row2),
	}
	stored := layout.Store()
	return applyStatusLineLayout(cfg, profile, &stored)
}

// keepOrder returns chosen, ordered to match prev where the two overlap, with
// anything new appended in the order chosen already had (catalog order).
//
// It is deliberately not a sort: prev is the user's own arrangement and may be
// in no order the catalog knows about, so entries present in both keep their
// relative positions from prev and the rest follow.
func keepOrder(chosen, prev []string) []string {
	want := make(map[string]bool, len(chosen))
	for _, k := range chosen {
		want[k] = true
	}
	out := make([]string, 0, len(chosen))
	seen := make(map[string]bool, len(chosen))
	for _, k := range prev {
		if want[k] && !seen[k] {
			seen[k] = true
			out = append(out, k)
		}
	}
	for _, k := range chosen {
		if !seen[k] {
			seen[k] = true
			out = append(out, k)
		}
	}
	return out
}

func pickRow(title string, choices, defaults []string) ([]string, error) {
	opts := make([]picker.Option, 0, len(choices))
	for _, key := range choices {
		for _, s := range statusline.Segments {
			if s.Key == key {
				opts = append(opts, picker.Option{Value: s.Key, Label: s.Label, Description: s.Description})
				break
			}
		}
	}
	chosen, err := picker.MultiSelect(title, opts, defaults)
	if err != nil {
		return nil, err
	}
	// MultiSelect returns values in the order they were offered, which is
	// catalog order. The caller restores any order the user had already set
	// (keepOrder), so a picker run does not re-sort a hand-arranged row.
	return chosen, nil
}

func except(all, remove []string) []string {
	drop := map[string]bool{}
	for _, k := range remove {
		drop[k] = true
	}
	out := []string{}
	for _, k := range all {
		if !drop[k] {
			out = append(out, k)
		}
	}
	return out
}

func intersect(want, available []string) []string {
	ok := map[string]bool{}
	for _, k := range available {
		ok[k] = true
	}
	out := []string{}
	for _, k := range want {
		if ok[k] {
			out = append(out, k)
		}
	}
	return out
}

// applyStatusLineLayout writes layout to the global settings or to one
// profile's override, then reports the result. A nil layout clears.
//
// The config passed in is NOT the one written. It was loaded before the picker
// ran, and a picker waits on a human — long enough for `ccpm add` in another
// terminal to create a profile, or `ccpm run` to stamp last_used. config.Save
// serialises the whole struct, so writing that stale snapshot would erase them.
// So this takes the lock, re-reads, applies just the status-line field, and
// writes; the caller's copy is only used to answer "does this profile exist".
//
// The lock is deliberately NOT held across the prompt. AGENTS.md's lock policy
// says config mutations must lock and that long-running work must not be
// wrapped, and a prompt is unbounded — holding it there would block every other
// ccpm command until the user answered.
func applyStatusLineLayout(cfg *config.Config, profile string, layout *config.StatusLineLayout) error {
	if profile != "" {
		if _, ok := cfg.Profiles[profile]; !ok {
			return fmt.Errorf("profile %q not found", profile)
		}
	}

	var fresh *config.Config
	err := withConfigLock(func() error {
		c, err := config.Load()
		if err != nil {
			return err
		}
		if profile == "" {
			c.Settings.StatusLine = layout
		} else {
			p, ok := c.Profiles[profile]
			if !ok {
				// Removed while the picker was open.
				return fmt.Errorf("profile %q no longer exists", profile)
			}
			p.StatusLine = layout
			c.Profiles[profile] = p
		}
		if err := config.Save(c); err != nil {
			return err
		}
		fresh = c
		return nil
	})
	if err != nil {
		return err
	}

	green := color.New(color.FgGreen, color.Bold)
	if profile == "" {
		if layout == nil {
			green.Println("✓ Status line restored to the default layout")
		} else {
			green.Println("✓ Status line layout saved for every profile")
		}
		printStatusLinePreview(statusline.Global(fresh))
		return nil
	}
	if layout == nil {
		green.Printf("✓ Profile %q now follows the global status line\n", profile)
	} else {
		green.Printf("✓ Status line layout saved for profile %q\n", profile)
	}
	printStatusLinePreview(statusline.Resolve(fresh, profile))
	return nil
}

// printStatusLinePreview renders layout against sample data so the effect of a
// change is visible without starting a session. It uses the real renderer, so
// the preview cannot drift from what Claude Code will actually show — only the
// input is invented.
func printStatusLinePreview(layout statusline.Layout) {
	rows := renderStatusLine(statusLinePreviewInput(), "work", time.Now(), statusLineColorEnabled(), layout)
	faint := color.New(color.Faint)
	fmt.Println()
	if len(rows) == 0 {
		faint.Println("  (every segment is switched off — the status line prints nothing)")
		fmt.Println()
		return
	}
	for _, row := range rows {
		fmt.Println("  " + row)
	}
	fmt.Println()
	faint.Println("  sample data — your own session fills these in")
}

// statusLinePreviewInput is a plausible payload covering every segment, so a
// preview never omits a field merely because the sample lacked it.
func statusLinePreviewInput() statusLineInput {
	var in statusLineInput
	in.Workspace.CurrentDir = "/w/ccpm/cmd"
	in.Workspace.ProjectDir = "/w/ccpm"
	in.Workspace.Repo = &struct {
		Host  string `json:"host"`
		Owner string `json:"owner"`
		Name  string `json:"name"`
	}{Host: "github.com", Owner: "nitin-1926", Name: "ccpm"}
	in.Worktree = &struct {
		Branch string `json:"branch"`
	}{Branch: "main"}
	in.Model.DisplayName = "Opus 5"
	in.ContextWindow.UsedPercentage = 34
	in.Effort = &struct {
		Level string `json:"level"`
	}{Level: "high"}
	in.Cost.TotalCostUSD = 1.23
	in.RateLimits = &struct {
		FiveHour *rateWindow `json:"five_hour"`
		SevenDay *rateWindow `json:"seven_day"`
	}{
		FiveHour: &rateWindow{UsedPercentage: 42, ResetsAt: time.Now().Add(3 * time.Hour).Unix()},
		SevenDay: &rateWindow{UsedPercentage: 78, ResetsAt: time.Now().Add(4 * 24 * time.Hour).Unix()},
	}
	return in
}
