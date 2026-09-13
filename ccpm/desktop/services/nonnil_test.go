//go:build darwin

package services

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// firstProfile returns a real profile name to exercise, or skips.
//
// Use this only where the test genuinely needs a profile with REAL data on the
// machine (transcripts, usage). For tests that only check DTO shape, use
// syntheticProfile — skipping there means the guard does not run on CI.
func firstProfile(t *testing.T) string {
	t.Helper()
	ps, err := NewProfiles().List()
	if err != nil || len(ps) == 0 {
		t.Skip("no profiles registered on this machine")
	}
	return ps[0].Name
}

// syntheticProfile points $HOME at a scratch directory containing exactly one
// registered, empty profile, and returns its name.
//
// The DTO-shape guards below used to call firstProfile and skip when the
// machine had no profiles — so on a clean CI runner 12 of the package's tests
// skipped, including the nil-slice guard itself. The invariant they protect
// (a nil slice marshals to null and blanks the frontend) is exactly the kind
// that regresses unnoticed, and it was being checked only on the maintainer's
// laptop.
//
// An empty profile is also the stronger fixture for this: every list really is
// empty, so any path that forgets to initialise a slice returns nil rather than
// being accidentally covered by real data.
func syntheticProfile(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home) // os.UserHomeDir reads this on Windows

	const name = "synthetic"
	dir := filepath.Join(home, ".ccpm", "profiles", name)
	if err := os.MkdirAll(filepath.Join(dir, "projects"), 0o700); err != nil {
		t.Fatal(err)
	}
	cfg := map[string]any{
		"version":         "1",
		"default_profile": name,
		"profiles": map[string]any{
			name: map[string]any{
				"name": name, "dir": dir, "auth_method": "oauth",
				"created_at": "2026-01-01T00:00:00Z", "last_used": "2026-01-01T00:00:00Z",
			},
		},
	}
	b, err := json.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, ".ccpm", "config.json"), b, 0o600); err != nil {
		t.Fatal(err)
	}
	// Confirm the fixture is actually visible through the same path the
	// services use, rather than trusting the layout.
	ps, err := NewProfiles().List()
	if err != nil || len(ps) != 1 || ps[0].Name != name {
		t.Fatalf("synthetic profile not registered: %v %+v", err, ps)
	}
	return name
}

// assertNoNullArrays fails if any of the named JSON array fields serialized to
// null. Go nil slices marshal to null, which makes the frontend's .length/.map
// throw and blank the window — this is the regression guard for that bug.
func assertNoNullArrays(t *testing.T, v interface{}, fields ...string) {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	s := string(b)
	for _, f := range fields {
		if strings.Contains(s, `"`+f+`":null`) {
			t.Errorf("field %q serialized to null (nil slice) — frontend would crash. JSON: %s", f, s)
		}
	}
}

func TestDetailsNoNullArrays(t *testing.T) {
	name := syntheticProfile(t)
	d, err := NewDetails().Get(name)
	if err != nil {
		t.Fatalf("Details.Get: %v", err)
	}
	assertNoNullArrays(t, d, "plugins", "env", "mcp", "allow", "ask", "deny")
	if d.Permissions.Allow == nil || d.Permissions.Ask == nil || d.Permissions.Deny == nil {
		t.Error("permission buckets must be non-nil slices")
	}
	if d.Plugins == nil || d.Env == nil || d.Mcp == nil {
		t.Error("plugins/env/mcp must be non-nil slices")
	}
}

func TestUsageNoNullArrays(t *testing.T) {
	name := syntheticProfile(t)
	for _, win := range []string{"all", "7d", "30d"} {
		u, err := NewUsage().Get(name, win)
		if err != nil {
			t.Fatalf("Usage.Get(%s): %v", win, err)
		}
		assertNoNullArrays(t, u, "byDay", "byModel", "byProject", "sessions")
	}
}

func TestCascadeNoNullArrays(t *testing.T) {
	name := syntheticProfile(t)
	c, err := NewCascade().Get(name)
	if err != nil {
		t.Fatalf("Cascade.Get: %v", err)
	}
	assertNoNullArrays(t, c, "assets", "settings")
}

func TestSettingsReturnsSlice(t *testing.T) {
	name := syntheticProfile(t)
	s, err := NewSettings().Get(name)
	if err != nil {
		t.Fatalf("Settings.Get: %v", err)
	}
	if s == nil {
		t.Error("Settings.Get must return a non-nil slice")
	}
	b, _ := json.Marshal(s)
	if string(b) == "null" {
		t.Error("Settings.Get serialized to null")
	}
}

// TestUnknownProfileSafe ensures services return safe empty values (not nil
// slices, not errors) for a profile that doesn't exist.
func TestUnknownProfileSafe(t *testing.T) {
	const ghost = "definitely-not-a-real-profile-xyz"
	d, err := NewDetails().Get(ghost)
	if err != nil {
		t.Fatalf("Details.Get(ghost): %v", err)
	}
	assertNoNullArrays(t, d, "plugins", "env", "mcp", "allow", "ask", "deny")

	c, err := NewCascade().Get(ghost)
	if err != nil {
		t.Fatalf("Cascade.Get(ghost): %v", err)
	}
	assertNoNullArrays(t, c, "assets", "settings")

	u, err := NewUsage().Get(ghost, "all")
	if err != nil {
		t.Fatalf("Usage.Get(ghost): %v", err)
	}
	_ = u

	h := NewHistory()
	rows, err := h.Sessions(ghost)
	if err != nil {
		t.Fatalf("History.Sessions(ghost): %v", err)
	}
	if rows == nil {
		t.Error("History.Sessions must return a non-nil slice")
	}
	page, err := h.Transcript(ghost, "any", "", 0, 10)
	if err != nil {
		t.Fatalf("History.Transcript(ghost): %v", err)
	}
	if page.Turns == nil {
		t.Error("History.Transcript must return a non-nil Turns slice")
	}
	res, err := h.Search(ghost, "q", "tok", false)
	if err != nil {
		t.Fatalf("History.Search(ghost): %v", err)
	}
	if res.Hits == nil {
		t.Error("History.Search must return a non-nil Hits slice")
	}

	sl := NewStatusLine().Get(ghost)
	assertNoNullArrays(t, sl, "segments", "row1", "row2", "off")
	if sl.HasOverride {
		t.Error("a profile that does not exist cannot have a status line override")
	}
}

func TestStatusLineNoNullArrays(t *testing.T) {
	for _, name := range []string{syntheticProfile(t), ""} {
		c := NewStatusLine().Get(name)
		// row1/row2/off appear inside both Layout and Global, so naming them
		// once covers both nested objects.
		assertNoNullArrays(t, c, "segments", "row1", "row2", "off")
		if len(c.Segments) == 0 {
			t.Errorf("StatusLine.Get(%q) returned no segments — the Settings section would render empty", name)
		}
		for _, s := range c.Segments {
			if s.Label == "" {
				t.Errorf("segment %q has no label", s.Key)
			}
		}
		// Every catalog segment must be placed exactly once, in each layout.
		// A segment in none would be invisible in the UI with no way to switch
		// it back on; one in two would render twice.
		for _, b := range []StatusLineBuckets{c.Layout, c.Global} {
			placed := map[string]int{}
			for _, bucket := range [][]string{b.Row1, b.Row2, b.Off} {
				for _, k := range bucket {
					placed[k]++
				}
			}
			for _, s := range c.Segments {
				if placed[s.Key] != 1 {
					t.Errorf("StatusLine.Get(%q): segment %q placed %d times, want exactly 1", name, s.Key, placed[s.Key])
				}
			}
			if len(placed) != len(c.Segments) {
				t.Errorf("StatusLine.Get(%q): layout names %d keys, catalog has %d", name, len(placed), len(c.Segments))
			}
		}
	}
}
