//go:build darwin

package services

import (
	"strings"

	"github.com/nitin-1926/claude-code-profile-manager/ccpm/internal/config"
	"github.com/nitin-1926/claude-code-profile-manager/ccpm/internal/statusline"
)

// StatusLineSegment is one configurable field, with the labels the UI shows and
// where it currently sits. Row is "off", "row1" or "row2" — strings rather than
// the Go enum so the frontend never has to know the numbering.
type StatusLineSegment struct {
	Key         string `json:"key"`
	Label       string `json:"label"`
	Description string `json:"description"`
	Row         string `json:"row"`
}

// StatusLineConfig is everything the Settings tab needs to draw the section.
//
// Segments reflects Effective, so the checkboxes render straight from it.
// Global is carried separately so the UI can show what "Use the global default"
// would look like without a second round trip, and HasOverride decides which
// scope the section opens on.
type StatusLineConfig struct {
	Profile     string              `json:"profile"`
	HasOverride bool                `json:"hasOverride"`
	Enabled     bool                `json:"enabled"`
	Segments    []StatusLineSegment `json:"segments"`
	Global      []StatusLineSegment `json:"global"`
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
		Segments: []StatusLineSegment{},
		Global:   []StatusLineSegment{},
	}
	cfg, err := config.Load()
	if err != nil {
		// Still describe the built-in layout rather than blanking the section.
		out.Segments = describeLayout(statusline.Default())
		out.Global = describeLayout(statusline.Default())
		return out
	}
	out.Enabled = cfg.Settings.StatusLineEnabled()
	out.HasOverride = statusline.HasOverride(cfg, profile)
	out.Segments = describeLayout(statusline.Resolve(cfg, profile))
	out.Global = describeLayout(statusline.Global(cfg))
	return out
}

// Set writes a layout. profile "" targets the global default; otherwise it
// writes that profile's override. The three slices must together account for
// every known segment exactly once — `ccpm statusline configure` rejects
// anything else rather than guessing, so a UI bug surfaces as a visible error
// instead of a silently reshuffled status line.
func (s *StatusLineService) Set(profile string, row1, row2, off []string) CmdResult {
	args := []string{
		"statusline", "configure",
		"--row1", strings.Join(row1, ","),
		"--row2", strings.Join(row2, ","),
		"--off", strings.Join(off, ","),
	}
	if profile != "" {
		args = append(args, "--profile", profile)
	}
	return runCCPM(args...)
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

// describeLayout pairs the catalog with a layout. It walks Segments rather than
// the layout's rows so the UI always lists every segment in a stable order,
// including the ones switched off — a hidden segment still needs a row in the
// table for the user to switch it back on.
//
// Always returns a non-nil slice: a nil one marshals to JSON null and makes the
// frontend's .map throw, which is what nonnil_test.go guards.
func describeLayout(l statusline.Layout) []StatusLineSegment {
	out := make([]StatusLineSegment, 0, len(statusline.Segments))
	for _, s := range statusline.Segments {
		out = append(out, StatusLineSegment{
			Key:         s.Key,
			Label:       s.Label,
			Description: s.Description,
			Row:         rowName(l.Row(s.Key)),
		})
	}
	return out
}

func rowName(r statusline.Row) string {
	switch r {
	case statusline.Row1:
		return "row1"
	case statusline.Row2:
		return "row2"
	default:
		return "off"
	}
}
