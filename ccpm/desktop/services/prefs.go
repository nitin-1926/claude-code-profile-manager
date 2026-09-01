//go:build darwin

package services

import (
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/nitin-1926/claude-code-profile-manager/ccpm/internal/atomicwrite"
	"github.com/nitin-1926/claude-code-profile-manager/ccpm/internal/config"
)

// Desktop-only preferences, kept in ~/.ccpm/desktop.json rather than in
// config.Settings.
//
// Two reasons for the separate file. `ccpm config` enumerates the keys of
// config.Settings in its CLI help, and GUI-only preferences leaking into the
// CLI's surface would be noise. And the rail is native AppKit that has to read
// its placement before any webview exists, so localStorage — where the theme
// preference lives — cannot own this.

// Rail visibility modes.
const (
	RailModeAlways = "always"
	RailModeHover  = "hover"
	RailModeHidden = "hidden"
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
	RailMode string `json:"railMode"`
	RailEdge string `json:"railEdge"`
	// RailProfiles holds explicit per-profile choices only. A profile absent
	// from this map is enabled — so a newly created profile shows up on the
	// rail without the user having to go and find a switch for it.
	RailProfiles map[string]bool `json:"railProfiles"`
}

// DefaultPrefs is what a machine with no preferences file gets. Hover rather
// than always-on so the rail introduces itself without immediately occupying
// screen edge for someone who did not ask for it.
func DefaultPrefs() DesktopPrefs {
	return DesktopPrefs{
		RailMode:     RailModeHover,
		RailEdge:     RailEdgeRight,
		RailProfiles: map[string]bool{},
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
	case RailModeAlways, RailModeHover, RailModeHidden:
	default:
		p.RailMode = RailModeHover
	}
	switch p.RailEdge {
	case RailEdgeRight, RailEdgeLeft, RailEdgeTop, RailEdgeBottom:
	default:
		p.RailEdge = RailEdgeRight
	}
	if p.RailProfiles == nil {
		p.RailProfiles = map[string]bool{}
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
	var p DesktopPrefs
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
type PrefsService struct{}

func NewPrefs() *PrefsService { return &PrefsService{} }

// Get returns the current preferences, defaults included.
func (s *PrefsService) Get() (DesktopPrefs, error) { return LoadPrefs(), nil }

// Set replaces the preferences and returns what was actually stored, so the
// frontend renders the normalized values rather than its own optimistic guess.
func (s *PrefsService) Set(p DesktopPrefs) (DesktopPrefs, error) {
	if err := SavePrefs(p); err != nil {
		return LoadPrefs(), err
	}
	return LoadPrefs(), nil
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
