//go:build darwin

package services

import (
	"encoding/json"
	"strings"
	"testing"
)

// firstProfile returns a real profile name to exercise, or skips.
func firstProfile(t *testing.T) string {
	t.Helper()
	ps, err := NewProfiles().List()
	if err != nil || len(ps) == 0 {
		t.Skip("no profiles registered on this machine")
	}
	return ps[0].Name
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
	name := firstProfile(t)
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
	name := firstProfile(t)
	for _, win := range []string{"all", "7d", "30d"} {
		u, err := NewUsage().Get(name, win)
		if err != nil {
			t.Fatalf("Usage.Get(%s): %v", win, err)
		}
		assertNoNullArrays(t, u, "byDay", "byModel", "byProject", "sessions")
	}
}

func TestCascadeNoNullArrays(t *testing.T) {
	name := firstProfile(t)
	c, err := NewCascade().Get(name)
	if err != nil {
		t.Fatalf("Cascade.Get: %v", err)
	}
	assertNoNullArrays(t, c, "assets", "settings")
}

func TestSettingsReturnsSlice(t *testing.T) {
	name := firstProfile(t)
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
}
