//go:build darwin

package services

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/nitin-1926/claude-code-profile-manager/ccpm/internal/statusline"
)

// writeStatusLineConfig points $HOME at a scratch dir holding one profile, with
// an optional global layout and an optional per-profile override, and returns
// the profile name. Writing the JSON by hand rather than through the Go structs
// is deliberate: it is the on-disk shape a user or an older ccpm produces, so
// a field-name or tag mistake shows up here instead of round-tripping cleanly.
func writeStatusLineConfig(t *testing.T, global, override map[string][]string) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)

	const name = "sl-test"
	dir := filepath.Join(home, ".ccpm", "profiles", name)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	prof := map[string]any{
		"name": name, "dir": dir, "auth_method": "oauth",
		"created_at": "2026-01-01T00:00:00Z", "last_used": "2026-01-01T00:00:00Z",
	}
	if override != nil {
		prof["statusline"] = override
	}
	settings := map[string]any{}
	if global != nil {
		settings["statusline"] = global
	}
	cfg := map[string]any{
		"version": "1", "default_profile": name,
		"profiles": map[string]any{name: prof},
		"settings": settings,
	}
	b, err := json.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, ".ccpm", "config.json"), b, 0o600); err != nil {
		t.Fatal(err)
	}
	return name
}

func allNineExcept(keys ...string) []string {
	drop := map[string]bool{}
	for _, k := range keys {
		drop[k] = true
	}
	var out []string
	for _, s := range statusline.Segments {
		if !drop[s.Key] {
			out = append(out, s.Key)
		}
	}
	return out
}

// TestStatusLineGetHonoursTheProfileOverride is the precedence check AT THE
// BRIDGE. internal/statusline tests precedence thoroughly, but nothing verified
// the service actually asks for the resolved layout: replacing Resolve with
// Global in Get produced a build that passed every test while quietly ignoring
// every profile override the UI can create.
func TestStatusLineGetHonoursTheProfileOverride(t *testing.T) {
	global := map[string][]string{
		"row1": {statusline.Profile},
		"row2": {},
		"off":  allNineExcept(statusline.Profile),
	}
	override := map[string][]string{
		"row1": {statusline.Model},
		"row2": {statusline.Cost},
		"off":  allNineExcept(statusline.Model, statusline.Cost),
	}
	name := writeStatusLineConfig(t, global, override)

	c := NewStatusLine().Get(name)

	if !c.HasOverride {
		t.Error("HasOverride = false for a profile whose config carries a statusline block")
	}
	if !reflect.DeepEqual(c.Layout.Row1, []string{statusline.Model}) {
		t.Errorf("Layout.Row1 = %v, want the profile's override %v", c.Layout.Row1, []string{statusline.Model})
	}
	if !reflect.DeepEqual(c.Layout.Row2, []string{statusline.Cost}) {
		t.Errorf("Layout.Row2 = %v, want the profile's override", c.Layout.Row2)
	}
	// Global must still report the global, unaffected by the override.
	if !reflect.DeepEqual(c.Global.Row1, []string{statusline.Profile}) {
		t.Errorf("Global.Row1 = %v, want the global default %v", c.Global.Row1, []string{statusline.Profile})
	}
	// Row1 and Row2 must not be transposed on the way across.
	if len(c.Layout.Row1) > 0 && c.Layout.Row1[0] == statusline.Cost {
		t.Error("Row1 and Row2 look transposed")
	}
}

// TestStatusLineGetWithoutOverrideFallsBackToGlobal is the other half: without
// a profile block, Layout must equal Global rather than the built-in.
func TestStatusLineGetWithoutOverrideFallsBackToGlobal(t *testing.T) {
	global := map[string][]string{
		"row1": {statusline.Model, statusline.Profile}, // deliberately not catalog order
		"row2": {},
		"off":  allNineExcept(statusline.Model, statusline.Profile),
	}
	name := writeStatusLineConfig(t, global, nil)

	c := NewStatusLine().Get(name)
	if c.HasOverride {
		t.Error("HasOverride = true for a profile with no statusline block")
	}
	want := []string{statusline.Model, statusline.Profile}
	if !reflect.DeepEqual(c.Layout.Row1, want) {
		t.Errorf("Layout.Row1 = %v, want the global %v", c.Layout.Row1, want)
	}
	if !reflect.DeepEqual(c.Layout, c.Global) {
		t.Errorf("with no override Layout must equal Global:\n layout %+v\n global %+v", c.Layout, c.Global)
	}
	// Stored order must survive the trip — the arrows in the UI exist to set it.
	if c.Layout.Row1[0] != statusline.Model {
		t.Error("the stored within-row order was re-sorted on the way to the frontend")
	}
}

// TestStatusLineGetReportsHiddenSegments guards the Off bucket specifically.
// Returning an empty Off survived every existing test, because nothing was
// hidden in the config those tests happened to read.
func TestStatusLineGetReportsHiddenSegments(t *testing.T) {
	hidden := []string{statusline.Branch, statusline.Cost}
	global := map[string][]string{
		"row1": {statusline.Profile, statusline.Model},
		"row2": allNineExcept(append(hidden, statusline.Profile, statusline.Model)...),
		"off":  hidden,
	}
	name := writeStatusLineConfig(t, global, nil)

	c := NewStatusLine().Get(name)
	if !reflect.DeepEqual(c.Layout.Off, hidden) {
		t.Errorf("Layout.Off = %v, want %v", c.Layout.Off, hidden)
	}
}

// TestStatusLineGetReportsEnabled pins the injection switch, which is separate
// from the layout and drives a warning in the UI.
func TestStatusLineGetReportsEnabled(t *testing.T) {
	for _, want := range []bool{true, false} {
		home := t.TempDir()
		t.Setenv("HOME", home)
		dir := filepath.Join(home, ".ccpm", "profiles", "p")
		if err := os.MkdirAll(dir, 0o700); err != nil {
			t.Fatal(err)
		}
		cfg := map[string]any{
			"version": "1", "default_profile": "p",
			"profiles": map[string]any{"p": map[string]any{"name": "p", "dir": dir, "auth_method": "oauth"}},
			"settings": map[string]any{"default_statusline": want},
		}
		b, _ := json.Marshal(cfg)
		if err := os.WriteFile(filepath.Join(home, ".ccpm", "config.json"), b, 0o600); err != nil {
			t.Fatal(err)
		}
		if got := NewStatusLine().Get("p").Enabled; got != want {
			t.Errorf("Enabled = %v, want %v", got, want)
		}
	}
}

// TestStatusLineSetArgs pins the argv the GUI sends. --off must always be
// present: the CLI requires every segment accounted for exactly once, so
// dropping an empty --off turns "nothing hidden" into a rejected write.
func TestStatusLineSetArgs(t *testing.T) {
	row1 := []string{statusline.Profile, statusline.Model}
	row2 := []string{statusline.Cost}
	off := []string{statusline.Branch}

	got := statusLineSetArgs("", row1, row2, off)
	want := []string{"statusline", "configure", "--row1", "profile,model", "--row2", "cost", "--off", "branch"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("global args =\n %v\nwant\n %v", got, want)
	}

	got = statusLineSetArgs("work", row1, row2, off)
	want = append(want, "--profile", "work")
	if !reflect.DeepEqual(got, want) {
		t.Errorf("profile args =\n %v\nwant\n %v", got, want)
	}

	// An empty bucket still sends its flag, with an empty value.
	got = statusLineSetArgs("", row1, nil, nil)
	if !reflect.DeepEqual(got, []string{"statusline", "configure", "--row1", "profile,model", "--row2", "", "--off", ""}) {
		t.Errorf("empty buckets dropped their flags: %v", got)
	}

	// Order within a row is preserved verbatim — it is the render order.
	got = statusLineSetArgs("", []string{statusline.Cost, statusline.Profile}, nil, nil)
	if got[3] != "cost,profile" {
		t.Errorf("--row1 = %q, want the given order preserved", got[3])
	}
}

// TestStatusLineGetUnreadableConfigStillDescribesTheDefault covers the branch
// where config.Load fails: the section must render the built-in rather than
// blank out.
func TestStatusLineGetUnreadableConfigStillDescribesTheDefault(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	if err := os.MkdirAll(filepath.Join(home, ".ccpm"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, ".ccpm", "config.json"), []byte("{ not json"), 0o600); err != nil {
		t.Fatal(err)
	}

	c := NewStatusLine().Get("anything")
	if len(c.Segments) != len(statusline.Segments) {
		t.Fatalf("got %d segments, want the full catalog", len(c.Segments))
	}
	if !reflect.DeepEqual(c.Layout.Row1, statusline.Default().Row1) {
		t.Errorf("Layout.Row1 = %v, want the built-in default", c.Layout.Row1)
	}
	assertNoNullArrays(t, c, "segments", "row1", "row2", "off")
}
