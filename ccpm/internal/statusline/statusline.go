// Package statusline owns the segment catalog for `ccpm statusline` and the
// rules for turning a stored config.StatusLineLayout into a layout the renderer
// can walk.
//
// It lives in internal/ rather than in package cmd because the desktop app
// needs the same catalog to draw its checkboxes, and ccpm/desktop is
// //go:build darwin — it cannot import package cmd. This mirrors the existing
// internal/usage + desktop/services/usage.go split.
//
// Nothing here formats a segment. The renderer in package cmd decides what
// "⎇ main" or "5h 42%" looks like; this package only decides which keys appear
// and in what order.
package statusline

import (
	"slices"

	"github.com/nitin-1926/claude-code-profile-manager/ccpm/internal/config"
)

// Segment keys. These are the stable identifiers written into config.json and
// accepted by `ccpm statusline configure --row1/--row2/--off`, so renaming one
// is a breaking change to a user's saved layout.
//
// FiveHour and SevenDay are spelled out rather than "5h"/"7d" to match the
// field names in Claude Code's own status JSON (rate_limits.five_hour), which
// is what someone hand-editing config.json will have seen.
const (
	Profile   = "profile"
	Workspace = "workspace"
	Branch    = "branch"
	Model     = "model"
	Context   = "context"
	Effort    = "effort"
	FiveHour  = "five_hour"
	SevenDay  = "seven_day"
	Cost      = "cost"
)

// Row identifies where a segment sits. RowOff is the zero value so an
// unset Segment.Row would read as "hidden" rather than silently landing on a
// row, but nothing relies on that — the catalog sets every Row explicitly.
type Row int

const (
	RowOff Row = iota
	Row1
	Row2
)

// Segment describes one renderable field for the pickers and the desktop UI.
// Label and Description are user-facing; Key is what reaches config.json.
type Segment struct {
	Key         string
	Label       string
	Description string
	// Default is where this segment sits before anyone configures anything,
	// and where it is placed when it appears in a layout that predates it.
	Default Row
}

// Segments is the catalog, in canonical order. That order is also the default
// order within a row and the order both UIs write, so a segment moved between
// rows lands in a predictable place rather than at the end.
//
// Adding an entry here is all it takes to introduce a new segment: existing
// saved layouts do not mention it, Normalize therefore treats it as new, and it
// appears at its Default row for users who have already configured a layout.
var Segments = []Segment{
	{Profile, "ccpm profile", "Which profile this session is running under", Row1},
	{Workspace, "repo / directory", "Repository name, plus the subdirectory you are in", Row1},
	{Branch, "git branch", "Current branch, read from .git/HEAD", Row1},
	{Model, "model", "The model answering, e.g. Opus 5", Row1},
	{Context, "context used", "How much of the model's context window is consumed", Row1},
	{Effort, "reasoning effort", "Reasoning-effort level, for models that report one", Row2},
	{FiveHour, "5h usage window", "Percent of the rolling 5-hour limit used, and when it renews", Row2},
	{SevenDay, "7d usage window", "Percent of the rolling 7-day limit used, and when it renews", Row2},
	{Cost, "session cost", "Estimated API-equivalent cost of this session", Row2},
}

// Layout is a resolved, normalized assignment of segments to rows. Every key in
// it is known, appears exactly once, and every known key appears somewhere —
// which is what lets the renderer walk it without re-validating.
type Layout struct {
	Row1 []string
	Row2 []string
	Off  []string
}

// Known reports whether key names a segment this build understands.
func Known(key string) bool {
	for _, s := range Segments {
		if s.Key == key {
			return true
		}
	}
	return false
}

// Default returns the built-in layout: identity and model on row 1, the budget
// on row 2, nothing hidden.
//
// The slices are non-nil even when empty. A Layout crosses the Wails bridge to
// the desktop app, where a nil slice marshals to JSON null and makes the
// frontend's .map throw — the invariant desktop/services/nonnil_test.go exists
// to guard. Keeping it true at the source means every producer inherits it.
func Default() Layout {
	l := Layout{Row1: []string{}, Row2: []string{}, Off: []string{}}
	for _, s := range Segments {
		switch s.Default {
		case Row1:
			l.Row1 = append(l.Row1, s.Key)
		case Row2:
			l.Row2 = append(l.Row2, s.Key)
		default:
			l.Off = append(l.Off, s.Key)
		}
	}
	return l
}

// Normalize turns a stored layout into a usable one. It never fails and never
// reports a problem: this runs on the path that renders a status line on every
// assistant message, where the only acceptable behaviour for a malformed
// config is to render something sensible.
//
// Unknown keys are dropped (a layout written by a newer ccpm, or a typo).
// Duplicates keep their first occurrence, scanning row 1, then row 2, then off.
// Known keys the layout never mentions are new since it was written, and go to
// their catalog default. Order within a row is otherwise preserved.
func Normalize(in config.StatusLineLayout) Layout {
	seen := map[string]bool{}
	keep := func(src []string) []string {
		out := []string{}
		for _, k := range src {
			if !Known(k) || seen[k] {
				continue
			}
			seen[k] = true
			out = append(out, k)
		}
		return out
	}
	l := Layout{Row1: keep(in.Row1), Row2: keep(in.Row2), Off: keep(in.Off)}

	// Anything the stored layout never mentioned did not exist when it was
	// written. Place it where a fresh install would.
	for _, s := range Segments {
		if seen[s.Key] {
			continue
		}
		switch s.Default {
		case Row1:
			l.Row1 = append(l.Row1, s.Key)
		case Row2:
			l.Row2 = append(l.Row2, s.Key)
		default:
			l.Off = append(l.Off, s.Key)
		}
	}
	return l
}

// Resolve returns the layout in force for a profile: its own override if it has
// one, else the global default, else the built-in. profile may be "" (no
// profile resolved for this session), which simply skips the override lookup.
//
// cfg may be nil, so a caller whose config.Load failed can still render.
func Resolve(cfg *config.Config, profile string) Layout {
	if cfg == nil {
		return Default()
	}
	if profile != "" {
		if p, ok := cfg.Profiles[profile]; ok && p.StatusLine != nil {
			return Normalize(*p.StatusLine)
		}
	}
	if cfg.Settings.StatusLine != nil {
		return Normalize(*cfg.Settings.StatusLine)
	}
	return Default()
}

// HasOverride reports whether profile carries its own layout rather than
// inheriting the global default. The desktop app uses this to decide between
// showing "Global default" and "This profile".
func HasOverride(cfg *config.Config, profile string) bool {
	if cfg == nil || profile == "" {
		return false
	}
	p, ok := cfg.Profiles[profile]
	return ok && p.StatusLine != nil
}

// Global returns the configured global default, or the built-in when none has
// been set. Unlike Resolve it ignores profile overrides entirely.
func Global(cfg *config.Config) Layout {
	if cfg == nil || cfg.Settings.StatusLine == nil {
		return Default()
	}
	return Normalize(*cfg.Settings.StatusLine)
}

// Store converts a resolved layout back into the serializable shape. Round
// tripping through Normalize then Store is stable.
func (l Layout) Store() config.StatusLineLayout {
	return config.StatusLineLayout{Row1: l.Row1, Row2: l.Row2, Off: l.Off}
}

// Row reports which row key sits on, or RowOff when it is hidden or unknown.
func (l Layout) Row(key string) Row {
	switch {
	case slices.Contains(l.Row1, key):
		return Row1
	case slices.Contains(l.Row2, key):
		return Row2
	}
	return RowOff
}
