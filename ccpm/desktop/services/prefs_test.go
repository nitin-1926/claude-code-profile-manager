//go:build darwin

package services

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
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

	if _, err := s.SetNotch(DesktopPrefs{RailMode: RailModeAlways, RailEdge: RailEdgeLeft}); err != nil {
		t.Fatalf("SetNotch: %v", err)
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

// SetNotch returns what was stored, not what was asked for, so the UI cannot end up
// rendering a value the file does not hold.
func TestSetReturnsNormalizedValues(t *testing.T) {
	withTempHome(t)
	var s PrefsService

	got, err := s.SetNotch(DesktopPrefs{RailMode: "nonsense", RailEdge: RailEdgeTop})
	if err != nil {
		t.Fatalf("SetNotch: %v", err)
	}
	if got.RailMode != RailModeHover {
		t.Errorf("SetNotch returned an unnormalized mode: %q", got.RailMode)
	}
	if got.RailEdge != RailEdgeTop {
		t.Errorf("SetNotch discarded a valid edge: %q", got.RailEdge)
	}
}

// The rail is a native panel and cannot read the frontend's localStorage, so
// the theme reaches it through this file. A hand-edited or future-written value
// must not leave the rail drawing in an unknown palette.
func TestThemeNormalizesToAKnownValue(t *testing.T) {
	if got := DefaultPrefs().Theme; got != ThemeGraphite {
		t.Errorf("default theme = %q, want %q", got, ThemeGraphite)
	}
	for _, valid := range []string{ThemeGraphite, ThemeMidnight, "light"} {
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

// writePrefsFile puts raw JSON where LoadPrefs reads it, standing in for a
// desktop.json written by an earlier build.
func writePrefsFile(t *testing.T, base, body string) {
	t.Helper()
	if err := os.MkdirAll(base, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(base, "desktop.json"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

// A desktop.json from before the notch settings existed has no railOn,
// railPercent or railMain. Missing keys must keep their defaults — on, shown,
// five-hour — not decode as Go's false and switch the notch off on upgrade.
func TestPrefsFromBeforeTheNotchSettingsKeepDefaults(t *testing.T) {
	base := withTempHome(t)
	writePrefsFile(t, base, `{"railMode":"always","railEdge":"top","railProfiles":{"cin":false},"theme":"light"}`)

	got := LoadPrefs()
	if !got.RailOn {
		t.Error("an old file without railOn loaded with the notch switched off")
	}
	if !got.RailPercent {
		t.Error("an old file without railPercent loaded with the percentages hidden")
	}
	if got.RailMain != RailMainFiveHour {
		t.Errorf("an old file without railMain loaded main ring %q, want %q", got.RailMain, RailMainFiveHour)
	}
	if got.RailMode != RailModeAlways || got.RailEdge != RailEdgeTop || got.Theme != "light" || got.RailEnabled("cin") {
		t.Errorf("an old file's own values were lost: %+v", got)
	}
}

// "hidden" used to be a reveal mode. It now means the master switch is off,
// with the reveal kept separately so switching back on restores it.
func TestPrefsMigrateHiddenModeToSwitchedOff(t *testing.T) {
	base := withTempHome(t)
	writePrefsFile(t, base, `{"railMode":"hidden","railEdge":"left"}`)

	got := LoadPrefs()
	if got.RailOn {
		t.Error(`railMode "hidden" loaded with the notch switched on`)
	}
	if got.RailMode != RailModeHover {
		t.Errorf(`railMode "hidden" migrated to reveal %q, want %q`, got.RailMode, RailModeHover)
	}
	if got.RailEdge != RailEdgeLeft {
		t.Errorf("migration clobbered the edge: %q", got.RailEdge)
	}
}

// The master switch must not touch the reveal choice: off and back on returns
// to "always" rather than resetting to the default.
func TestSwitchingTheNotchOffKeepsTheReveal(t *testing.T) {
	withTempHome(t)
	var s PrefsService

	on, err := s.SetNotch(DesktopPrefs{RailOn: true, RailMode: RailModeAlways, RailPercent: true})
	if err != nil {
		t.Fatalf("SetNotch: %v", err)
	}
	on.RailOn = false
	off, err := s.SetNotch(on)
	if err != nil {
		t.Fatalf("SetNotch off: %v", err)
	}
	if off.RailOn || off.RailMode != RailModeAlways {
		t.Errorf("switched off: on=%v mode=%q, want off with always kept", off.RailOn, off.RailMode)
	}
	off.RailOn = true
	back, err := s.SetNotch(off)
	if err != nil {
		t.Fatalf("SetNotch on: %v", err)
	}
	if !back.RailOn || back.RailMode != RailModeAlways {
		t.Errorf("switched back on: on=%v mode=%q, want on with always restored", back.RailOn, back.RailMode)
	}
}

func TestRailMainNormalizesToAKnownWindow(t *testing.T) {
	for _, valid := range []string{RailMainFiveHour, RailMainSevenDay} {
		if got := (DesktopPrefs{RailMain: valid}).normalize().RailMain; got != valid {
			t.Errorf("normalize dropped the valid main ring %q, got %q", valid, got)
		}
	}
	for _, bad := range []string{"", "weekly", "FIVE_HOUR"} {
		if got := (DesktopPrefs{RailMain: bad}).normalize().RailMain; got != RailMainFiveHour {
			t.Errorf("normalize(%q) = %q, want the five-hour fallback", bad, got)
		}
	}
}

// Concurrent writers each load, edit and save desktop.json. Unserialised, two
// that load the same file drop each other's edit.
func TestConcurrentPrefWritesKeepEveryEdit(t *testing.T) {
	withTempHome(t)
	s := NewPrefs()
	const n = 40
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			if _, err := s.SetRailProfile(fmt.Sprintf("p%02d", i), false); err != nil {
				t.Error(err)
			}
		}(i)
	}
	wg.Wait()
	if got := len(LoadPrefs().RailProfiles); got != n {
		t.Fatalf("%d of %d concurrent profile edits survived", got, n)
	}
}

// The notch section sends its whole copy of the preferences. A theme changed
// after that copy was read must not be put back by the notch write.
func TestSetNotchLeavesTheThemeAlone(t *testing.T) {
	withTempHome(t)
	s := NewPrefs()
	stale := LoadPrefs() // the section's copy, read before the theme change
	if _, err := s.SetTheme("light"); err != nil {
		t.Fatal(err)
	}
	stale.RailEdge = RailEdgeTop
	got, err := s.SetNotch(stale)
	if err != nil {
		t.Fatal(err)
	}
	if got.Theme != "light" {
		t.Errorf("theme = %q after a notch write, want %q: the stale copy put the old theme back", got.Theme, "light")
	}
	if got.RailEdge != RailEdgeTop {
		t.Errorf("railEdge = %q, want %q", got.RailEdge, RailEdgeTop)
	}
}
