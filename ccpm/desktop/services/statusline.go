//go:build darwin

package services

import (
	"strings"

	"github.com/nitin-1926/claude-code-profile-manager/ccpm/internal/config"
	"github.com/nitin-1926/claude-code-profile-manager/ccpm/internal/statusline"
)

// StatusLineSegment is one configurable field's catalog entry: what it is
// called and what it shows. It carries no position — where a segment sits is
// StatusLineBuckets' job, because position is ordered and a per-segment tag
// could not express the order of segments within a row.
type StatusLineSegment struct {
	Key         string `json:"key"`
	Label       string `json:"label"`
	Description string `json:"description"`
}

// StatusLineBuckets is one layout: the keys on each row, in render order, plus
// the ones switched off. Order is significant — it is the order the status line
// prints — so these are lists, not sets.
type StatusLineBuckets struct {
	Row1 []string `json:"row1"`
	Row2 []string `json:"row2"`
	Off  []string `json:"off"`
}

// StatusLineConfig is everything the Settings tab needs to draw the section.
//
// Segments is the catalog, in canonical order, for labels and descriptions.
// Layout is what this profile actually renders; Global is the default it would
// fall back to, carried so the UI can switch scope without a second round trip.
// HasOverride decides which scope the section opens on.
type StatusLineConfig struct {
	Profile     string              `json:"profile"`
	HasOverride bool                `json:"hasOverride"`
	Enabled     bool                `json:"enabled"`
	Segments    []StatusLineSegment `json:"segments"`
	Layout      StatusLineBuckets   `json:"layout"`
	Global      StatusLineBuckets   `json:"global"`
}

// StatusLineService reads and writes the segment layout for `ccpm statusline`.
//
// Reads go straight to internal/config, as UsageService.Get does. Writes shell
// out to `ccpm statusline configure`, which is the same funnel the CLI and any
// script uses — the validation that every segment be accounted for exactly once
// lives there, so the GUI cannot save a layout the CLI would reject.
type StatusLineService struct{}

func NewStatusLine() *StatusLineService { return &StatusLineService{} }

// Get returns the layout in force for profile, plus the global default.
// An unknown or empty profile is not an error: it reports the global default
// with no override, which is what a freshly-launched window should show.
func (s *StatusLineService) Get(profile string) StatusLineConfig {
	out := StatusLineConfig{
		Profile:  profile,
		Enabled:  true,
		Segments: catalog(),
	}
	cfg, err := config.Load()
	if err != nil {
		// Still describe the built-in layout rather than blanking the section.
		out.Layout = buckets(statusline.Default())
		out.Global = buckets(statusline.Default())
		return out
	}
	out.Enabled = cfg.Settings.StatusLineEnabled()
	out.HasOverride = statusline.HasOverride(cfg, profile)
	out.Layout = buckets(statusline.Resolve(cfg, profile))
	out.Global = buckets(statusline.Global(cfg))
	return out
}

// Set writes a layout. profile "" targets the global default; otherwise it
// writes that profile's override. The three slices must together account for
// every known segment exactly once — `ccpm statusline configure` rejects
// anything else rather than guessing, so a UI bug surfaces as a visible error
// instead of a silently reshuffled status line.
func (s *StatusLineService) Set(profile string, row1, row2, off []string) CmdResult {
	return runCCPM(statusLineSetArgs(profile, row1, row2, off)...)
}

// statusLineSetArgs builds the argv for a Set. Split out from Set so the
// composition is testable without shelling out — dropping --off entirely, or
// swapping row1 and row2, produced a working binary and a silently wrong write
// with nothing to catch it.
func statusLineSetArgs(profile string, row1, row2, off []string) []string {
	args := []string{
		"statusline", "configure",
		"--row1", strings.Join(row1, ","),
		"--row2", strings.Join(row2, ","),
		// Always sent, even when empty: the CLI requires every segment to be
		// accounted for exactly once, so omitting the flag turns a deliberate
		// "nothing hidden" into an incomplete layout it will reject.
		"--off", strings.Join(off, ","),
	}
	if profile != "" {
		args = append(args, "--profile", profile)
	}
	return args
}

// Reset drops profile's override so it follows the global default again, or
// restores the built-in layout when profile is "".
func (s *StatusLineService) Reset(profile string) CmdResult {
	args := []string{"statusline", "configure", "--reset"}
	if profile != "" {
		args = append(args, "--profile", profile)
	}
	return runCCPM(args...)
}

// catalog is the full segment list with its labels, in canonical order. Every
// segment appears whatever its position, because a switched-off one still needs
// a row in the table for the user to switch it back on.
//
// Always non-nil: a nil slice marshals to JSON null and makes the frontend's
// .map throw, which is what nonnil_test.go guards.
func catalog() []StatusLineSegment {
	out := make([]StatusLineSegment, 0, len(statusline.Segments))
	for _, s := range statusline.Segments {
		out = append(out, StatusLineSegment{Key: s.Key, Label: s.Label, Description: s.Description})
	}
	return out
}

// buckets converts a resolved layout into the wire shape, preserving the order
// within each row — that order is what the status line prints, and it is the
// thing the UI's move-up/move-down controls exist to change.
func buckets(l statusline.Layout) StatusLineBuckets {
	return StatusLineBuckets{
		Row1: nonNil(l.Row1),
		Row2: nonNil(l.Row2),
		Off:  nonNil(l.Off),
	}
}

func nonNil(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}
