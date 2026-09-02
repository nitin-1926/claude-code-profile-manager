//go:build darwin

package services

import (
	"os"
	"path/filepath"
	"testing"
)

// withTempHome points config.BaseDir at a scratch directory so preference tests
// never read or clobber the developer's real ~/.ccpm.
func withTempHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	return filepath.Join(home, ".ccpm")
}

func TestPrefsRoundTrip(t *testing.T) {
	withTempHome(t)

	want := DesktopPrefs{
		RailMode:     RailModeAlways,
		RailEdge:     RailEdgeBottom,
		RailProfiles: map[string]bool{"work": true, "cin": false},
	}
	if err := SavePrefs(want); err != nil {
		t.Fatalf("SavePrefs: %v", err)
	}

	got := LoadPrefs()
	if got.RailMode != RailModeAlways || got.RailEdge != RailEdgeBottom {
		t.Errorf("mode/edge lost: %+v", got)
	}
	if !got.RailProfiles["work"] || got.RailProfiles["cin"] {
		t.Errorf("per-profile choices lost: %+v", got.RailProfiles)
	}
}

func TestPrefsDefaultsWhenAbsent(t *testing.T) {
	withTempHome(t)

	got := LoadPrefs()
	if got.RailMode != RailModeHover {
		t.Errorf("default mode = %q, want %q", got.RailMode, RailModeHover)
	}
	if got.RailEdge != RailEdgeRight {
		t.Errorf("default edge = %q, want %q", got.RailEdge, RailEdgeRight)
	}
	if got.RailProfiles == nil {
		t.Error("default RailProfiles is nil; callers would panic on assignment")
	}
}

func TestPrefsCorruptFileFallsBackToDefaults(t *testing.T) {
	base := withTempHome(t)
	if err := os.MkdirAll(base, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(base, "desktop.json"), []byte("{broken"), 0o644); err != nil {
		t.Fatal(err)
	}

	// A hand-mangled preferences file must not leave the rail undrawable.
	got := LoadPrefs()
	if got.RailMode != RailModeHover || got.RailEdge != RailEdgeRight {
		t.Errorf("corrupt file did not fall back to defaults: %+v", got)
	}
}

func TestPrefsNormalizesUnknownValues(t *testing.T) {
	withTempHome(t)

	if err := SavePrefs(DesktopPrefs{RailMode: "sideways", RailEdge: "diagonal"}); err != nil {
		t.Fatalf("SavePrefs: %v", err)
	}
	got := LoadPrefs()
	if got.RailMode != RailModeHover {
		t.Errorf("unknown mode survived as %q", got.RailMode)
	}
	if got.RailEdge != RailEdgeRight {
		t.Errorf("unknown edge survived as %q", got.RailEdge)
	}
}

// A profile the user has never touched must be on the rail by default,
// otherwise a freshly created profile is invisible until it is hunted down in
// settings.
func TestRailEnabledDefaultsOnForUnknownProfiles(t *testing.T) {
	cases := []struct {
		name  string
		prefs DesktopPrefs
		want  bool
	}{
		{"nil map", DesktopPrefs{}, true},
		{"empty map", DesktopPrefs{RailProfiles: map[string]bool{}}, true},
		{"absent key", DesktopPrefs{RailProfiles: map[string]bool{"other": false}}, true},
		{"explicitly on", DesktopPrefs{RailProfiles: map[string]bool{"work": true}}, true},
		{"explicitly off", DesktopPrefs{RailProfiles: map[string]bool{"work": false}}, false},
	}
	for _, c := range cases {
		if got := c.prefs.RailEnabled("work"); got != c.want {
			t.Errorf("%s: RailEnabled(work) = %v, want %v", c.name, got, c.want)
		}
	}
}

func TestSetRailProfileTogglesOne(t *testing.T) {
	withTempHome(t)
	var s PrefsService

	if _, err := s.Set(DesktopPrefs{RailMode: RailModeAlways, RailEdge: RailEdgeLeft}); err != nil {
		t.Fatalf("Set: %v", err)
	}
	got, err := s.SetRailProfile("cin", false)
	if err != nil {
		t.Fatalf("SetRailProfile: %v", err)
	}

	if got.RailEnabled("cin") {
		t.Error("cin should be disabled after SetRailProfile(false)")
	}
	// Toggling one profile must not disturb the rest of the preferences.
	if got.RailMode != RailModeAlways || got.RailEdge != RailEdgeLeft {
		t.Errorf("toggling a profile clobbered mode/edge: %+v", got)
	}
	if !got.RailEnabled("work") {
		t.Error("an untouched profile should still be enabled")
	}
}

// Set returns what was stored, not what was asked for, so the UI cannot end up
// rendering a value the file does not hold.
func TestSetReturnsNormalizedValues(t *testing.T) {
	withTempHome(t)
	var s PrefsService

	got, err := s.Set(DesktopPrefs{RailMode: "nonsense", RailEdge: RailEdgeTop})
	if err != nil {
		t.Fatalf("Set: %v", err)
	}
	if got.RailMode != RailModeHover {
		t.Errorf("Set returned an unnormalized mode: %q", got.RailMode)
	}
	if got.RailEdge != RailEdgeTop {
		t.Errorf("Set discarded a valid edge: %q", got.RailEdge)
	}
}

// The rail is a native panel and cannot read the frontend's localStorage, so
// the theme reaches it through this file. A hand-edited or future-written value
// must not leave the rail drawing in an unknown palette.
func TestThemeNormalizesToAKnownValue(t *testing.T) {
	if got := DefaultPrefs().Theme; got != ThemeGraphite {
		t.Errorf("default theme = %q, want %q", got, ThemeGraphite)
	}
	for _, valid := range []string{ThemeGraphite, ThemeMidnight, ThemeLight} {
		if got := (DesktopPrefs{Theme: valid}).normalize().Theme; got != valid {
			t.Errorf("normalize dropped the valid theme %q, got %q", valid, got)
		}
	}
	for _, bad := range []string{"", "solarized", "GRAPHITE", "dark"} {
		if got := (DesktopPrefs{Theme: bad}).normalize().Theme; got != ThemeGraphite {
			t.Errorf("normalize(%q) = %q, want the graphite fallback", bad, got)
		}
	}
}
