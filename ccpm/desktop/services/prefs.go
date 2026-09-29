//go:build darwin

package services

import (
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/nitin-1926/claude-code-profile-manager/ccpm/internal/atomicwrite"
	"github.com/nitin-1926/claude-code-profile-manager/ccpm/internal/config"
	"github.com/nitin-1926/claude-code-profile-manager/ccpm/internal/usage"
)

// Desktop-only preferences, kept in ~/.ccpm/desktop.json rather than in
// config.Settings.
//
// Two reasons for the separate file. `ccpm config` enumerates the keys of
// config.Settings in its CLI help, and GUI-only preferences leaking into the
// CLI's surface would be noise. And the rail is native AppKit that has to read
// its placement before any webview exists, so localStorage — where the theme
// preference lives — cannot own this.

// Rail reveal modes. Whether the notch is shown at all is RailOn, not a third
// mode: keeping the two apart is what lets switching the notch back on restore
// the reveal the user had picked instead of resetting it.
const (
	RailModeAlways = "always"
	RailModeHover  = "hover"
	// RailModeHidden is only ever READ, from a desktop.json written before
	// RailOn existed; normalize turns it into RailOn=false.
	RailModeHidden = "hidden"
)

// Rail main-ring choices: which usage window is the big outer ring. They are
// the usage window keys themselves, so nothing has to translate between the
// preference and the reading.
const (
	RailMainFiveHour = usage.KeyFiveHour
	RailMainSevenDay = usage.KeySevenDay
)

// Theme names, matching the `data-theme` values in
// desktop/frontend/src/globals.css. Duplicated as constants here rather than
// imported from the rail package so services stays free of cgo.
const (
	ThemeGraphite = "graphite"
	ThemeMidnight = "midnight"
	ThemeLight    = "light"
)

// Rail edges.
const (
	RailEdgeRight  = "right"
	RailEdgeLeft   = "left"
	RailEdgeTop    = "top"
	RailEdgeBottom = "bottom"
)

// DesktopPrefs is the whole desktop-local preference set.
type DesktopPrefs struct {
	// RailOn is the master switch. RailMode keeps the reveal choice while the
	// notch is off, so turning it back on returns to hover or always as left.
	RailOn   bool   `json:"railOn"`
	RailMode string `json:"railMode"`
	RailEdge string `json:"railEdge"`
	// RailPercent draws the percentage under each ring. Worth turning off on
	// the top edge with several profiles, where the labels scale down to a few
	// points tall and the rings get the room back.
	RailPercent bool `json:"railPercent"`
	// RailMain is the usage window drawn as the big ring: RailMainFiveHour or
	// RailMainSevenDay. The other window is the small inner ring.
	RailMain string `json:"railMain"`
	// RailProfiles holds explicit per-profile choices only. A profile absent
	// from this map is enabled — so a newly created profile shows up on the
	// rail without the user having to go and find a switch for it.
	RailProfiles map[string]bool `json:"railProfiles"`
	// Theme mirrors the `data-theme` the frontend sets on <html>. The rail is a
	// native panel and cannot read the frontend's CSS custom properties or its
	// localStorage, so the theme has to reach Go through the preferences file.
	Theme string `json:"theme"`
}

// DefaultPrefs is what a machine with no preferences file gets. Hover rather
// than always-on so the rail introduces itself without immediately occupying
// screen edge for someone who did not ask for it.
//
// It is also what LoadPrefs decodes the file ON TOP of, so a key the file does
// not have keeps its default. That is how a desktop.json written before
// RailOn and RailPercent existed still loads with the notch and its
// percentages on, rather than with Go's false for a missing bool.
func DefaultPrefs() DesktopPrefs {
	return DesktopPrefs{
		RailOn:       true,
		RailMode:     RailModeHover,
		RailEdge:     RailEdgeRight,
		RailPercent:  true,
		RailMain:     RailMainFiveHour,
		RailProfiles: map[string]bool{},
		Theme:        ThemeGraphite,
	}
}

// RailEnabled reports whether a profile should appear on the rail.
func (p DesktopPrefs) RailEnabled(profile string) bool {
	if p.RailProfiles == nil {
		return true
	}
	on, ok := p.RailProfiles[profile]
	return !ok || on
}

// normalize repairs anything unrecognised. A hand-edited or future-written
// file must never leave the rail in an undrawable state, so unknown values
// fall back to the default rather than propagating.
func (p DesktopPrefs) normalize() DesktopPrefs {
	switch p.RailMode {
	case RailModeAlways, RailModeHover:
	case RailModeHidden:
		// A file from before the master switch. It never recorded which reveal
		// preceded "hidden", so switching back on gets the default one.
		p.RailOn = false
		p.RailMode = RailModeHover
	default:
		p.RailMode = RailModeHover
	}
	switch p.RailMain {
	case RailMainFiveHour, RailMainSevenDay:
	default:
		p.RailMain = RailMainFiveHour
	}
	switch p.RailEdge {
	case RailEdgeRight, RailEdgeLeft, RailEdgeTop, RailEdgeBottom:
	default:
		p.RailEdge = RailEdgeRight
	}
	if p.RailProfiles == nil {
		p.RailProfiles = map[string]bool{}
	}
	switch p.Theme {
	case ThemeGraphite, ThemeMidnight, ThemeLight:
	default:
		p.Theme = ThemeGraphite
	}
	return p
}

func prefsPath() (string, error) {
	base, err := config.BaseDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, "desktop.json"), nil
}

// LoadPrefs reads the preference file. A missing or unreadable file yields the
// defaults, never an error — the absence of preferences is the normal state on
// first run, and no caller should have to distinguish it.
func LoadPrefs() DesktopPrefs {
	path, err := prefsPath()
	if err != nil {
		return DefaultPrefs()
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return DefaultPrefs()
	}
	p := DefaultPrefs()
	if json.Unmarshal(data, &p) != nil {
		return DefaultPrefs()
	}
	return p.normalize()
}

// SavePrefs writes the preference file atomically.
func SavePrefs(p DesktopPrefs) error {
	path, err := prefsPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), config.DirPerm); err != nil {
		return err
	}
	data, err := json.Marshal(p.normalize())
	if err != nil {
		return err
	}
	return atomicwrite.Apply([]atomicwrite.FileChange{
		atomicwrite.WriteFile(path, data, config.FilePerm),
	})
}

// PrefsService exposes the desktop preferences to the frontend.
// PrefsService is the frontend's door to the preferences file.
//
// OnChange fires after every successful write. The rail has to be reshaped when
// preferences change, and making that a second call the frontend must remember
// is a desync waiting to happen — one forgotten call and the rail silently
// disagrees with the settings that produced it.
type PrefsService struct {
	OnChange func()
}

func NewPrefs() *PrefsService { return &PrefsService{} }

// notify runs the change hook if one is wired. Never on the caller's error
// path: a failed write must not make the rail redraw as though it succeeded.
func (s *PrefsService) notify() {
	if s.OnChange != nil {
		s.OnChange()
	}
}

// Get returns the current preferences, defaults included.
func (s *PrefsService) Get() (DesktopPrefs, error) { return LoadPrefs(), nil }

// Set replaces the preferences and returns what was actually stored, so the
// frontend renders the normalized values rather than its own optimistic guess.
func (s *PrefsService) Set(p DesktopPrefs) (DesktopPrefs, error) {
	if err := SavePrefs(p); err != nil {
		return LoadPrefs(), err
	}
	s.notify()
	return LoadPrefs(), nil
}

// SetTheme records the palette the frontend is showing. The rail is a native
// panel and cannot read the frontend's localStorage, so this is how the two
// stay in the same theme.
func (s *PrefsService) SetTheme(theme string) (DesktopPrefs, error) {
	p := LoadPrefs()
	p.Theme = theme
	return s.Set(p)
}

// SetRailProfile toggles one profile without the frontend having to
// read-modify-write the whole map, which would race with a concurrent edit.
func (s *PrefsService) SetRailProfile(profile string, enabled bool) (DesktopPrefs, error) {
	p := LoadPrefs()
	if p.RailProfiles == nil {
		p.RailProfiles = map[string]bool{}
	}
	p.RailProfiles[profile] = enabled
	return s.Set(p)
}
