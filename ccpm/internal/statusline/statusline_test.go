package statusline

import (
	"reflect"
	"testing"

	"github.com/nitin-1926/claude-code-profile-manager/ccpm/internal/config"
)

// TestDefaultCoversEverySegment is the guard that makes adding a segment to the
// catalog safe: every key must land somewhere, or the renderer would never be
// asked for it and nobody would notice until a user asked where it went.
func TestDefaultCoversEverySegment(t *testing.T) {
	l := Default()
	got := map[string]int{}
	for _, k := range append(append(append([]string{}, l.Row1...), l.Row2...), l.Off...) {
		got[k]++
	}
	if len(got) != len(Segments) {
		t.Fatalf("Default() covers %d segments, catalog has %d", len(got), len(Segments))
	}
	for _, s := range Segments {
		if got[s.Key] != 1 {
			t.Errorf("segment %q appears %d times in Default(), want exactly 1", s.Key, got[s.Key])
		}
	}
}

func TestDefaultLayout(t *testing.T) {
	l := Default()
	wantRow1 := []string{Profile, Workspace, Branch, Model, Context}
	wantRow2 := []string{Effort, FiveHour, SevenDay, Cost}
	if !reflect.DeepEqual(l.Row1, wantRow1) {
		t.Errorf("Row1 = %v, want %v", l.Row1, wantRow1)
	}
	if !reflect.DeepEqual(l.Row2, wantRow2) {
		t.Errorf("Row2 = %v, want %v", l.Row2, wantRow2)
	}
	if len(l.Off) != 0 {
		t.Errorf("Off = %v, want nothing hidden by default", l.Off)
	}
}

func TestNormalize(t *testing.T) {
	cases := []struct {
		name string
		in   config.StatusLineLayout
		want Layout
	}{
		{
			name: "an empty layout is the default layout",
			in:   config.StatusLineLayout{},
			want: Default(),
		},
		{
			name: "unknown keys are dropped, not rendered and not errored on",
			in: config.StatusLineLayout{
				Row1: []string{Profile, "weather", Model},
				Row2: []string{Cost},
				Off:  []string{Workspace, Branch, Context, Effort, FiveHour, SevenDay},
			},
			want: Layout{
				Row1: []string{Profile, Model},
				Row2: []string{Cost},
				Off:  []string{Workspace, Branch, Context, Effort, FiveHour, SevenDay},
			},
		},
		{
			name: "a duplicate keeps its first occurrence, scanning row1 then row2 then off",
			in: config.StatusLineLayout{
				Row1: []string{Profile, Model},
				Row2: []string{Model, Cost},
				Off:  []string{Profile, Workspace, Branch, Context, Effort, FiveHour, SevenDay},
			},
			want: Layout{
				Row1: []string{Profile, Model},
				Row2: []string{Cost},
				Off:  []string{Workspace, Branch, Context, Effort, FiveHour, SevenDay},
			},
		},
		{
			name: "stored order within a row is preserved, not re-sorted to catalog order",
			in: config.StatusLineLayout{
				Row1: []string{Cost, Profile},
				Row2: []string{Model},
				Off:  []string{Workspace, Branch, Context, Effort, FiveHour, SevenDay},
			},
			want: Layout{
				Row1: []string{Cost, Profile},
				Row2: []string{Model},
				Off:  []string{Workspace, Branch, Context, Effort, FiveHour, SevenDay},
			},
		},
		{
			name: "an empty row survives as an empty row",
			in: config.StatusLineLayout{
				Row1: []string{Profile, Workspace, Branch, Model, Context, Effort, FiveHour, SevenDay, Cost},
			},
			want: Layout{
				Row1: []string{Profile, Workspace, Branch, Model, Context, Effort, FiveHour, SevenDay, Cost},
				Row2: []string{},
				Off:  []string{},
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := Normalize(tc.in)
			if !reflect.DeepEqual(got.Row1, tc.want.Row1) {
				t.Errorf("Row1 = %v, want %v", got.Row1, tc.want.Row1)
			}
			if !reflect.DeepEqual(got.Row2, tc.want.Row2) {
				t.Errorf("Row2 = %v, want %v", got.Row2, tc.want.Row2)
			}
			if !reflect.DeepEqual(got.Off, tc.want.Off) {
				t.Errorf("Off = %v, want %v", got.Off, tc.want.Off)
			}
		})
	}
}

// TestNormalizePlacesSegmentsAddedAfterTheLayoutWasSaved is the reason Off is
// stored explicitly. A layout written before a segment existed cannot mention
// it; if "absent" meant "hidden", every existing user would silently never see
// a segment added in a later release.
//
// This simulates that by writing a layout that accounts for every segment
// EXCEPT one, exactly as a config saved by the previous version would look.
func TestNormalizePlacesSegmentsAddedAfterTheLayoutWasSaved(t *testing.T) {
	for _, missing := range Segments {
		t.Run(missing.Key, func(t *testing.T) {
			var stored config.StatusLineLayout
			// Everything the "older" ccpm knew about goes to Off, the most
			// hostile case: if the new segment were treated as absent-means-off
			// it would be indistinguishable from these.
			for _, s := range Segments {
				if s.Key != missing.Key {
					stored.Off = append(stored.Off, s.Key)
				}
			}
			got := Normalize(stored)
			if row := got.Row(missing.Key); row != missing.Default {
				t.Errorf("segment %q added after the layout was saved landed on row %v, want its default %v",
					missing.Key, row, missing.Default)
			}
		})
	}
}

// TestNormalizeFillsInAPartialHandWrittenLayout pins the direction of the
// fill-in rule, which is easy to get backwards. Someone editing config.json who
// writes only `row1` has NOT hidden everything else — the unlisted segments go
// to their catalog defaults, so a half-finished edit degrades toward showing
// too much rather than toward a status line that silently lost most of itself.
//
// Hiding a segment therefore requires naming it in `off`, which is exactly what
// both the picker and the desktop app write.
func TestNormalizeFillsInAPartialHandWrittenLayout(t *testing.T) {
	got := Normalize(config.StatusLineLayout{Row1: []string{Cost}})

	// Cost keeps the row it was explicitly given, ahead of the fill-ins.
	if len(got.Row1) == 0 || got.Row1[0] != Cost {
		t.Fatalf("Row1 = %v, want %q first", got.Row1, Cost)
	}
	// Everything else lands at its default, not in Off.
	if len(got.Off) != 0 {
		t.Errorf("Off = %v, want nothing hidden by a partial layout", got.Off)
	}
	for _, s := range Segments {
		if s.Key == Cost {
			continue
		}
		if row := got.Row(s.Key); row != s.Default {
			t.Errorf("unlisted segment %q landed on row %v, want its default %v", s.Key, row, s.Default)
		}
	}
}

// TestNormalizeIsIdempotent — the layout the renderer walks must survive a
// round trip through storage unchanged, or saving from the desktop app would
// slowly rewrite what the user chose.
func TestNormalizeIsIdempotent(t *testing.T) {
	start := Normalize(config.StatusLineLayout{
		Row1: []string{Cost, Profile},
		Row2: []string{Model},
	})
	again := Normalize(start.Store())
	if !reflect.DeepEqual(start, again) {
		t.Errorf("round trip changed the layout:\n first %+v\nsecond %+v", start, again)
	}
}

func TestResolvePrecedence(t *testing.T) {
	// Both fixtures account for every segment, so Normalize's fill-in rule
	// (covered separately below) cannot quietly change what these assert.
	globalLayout := &config.StatusLineLayout{
		Row1: []string{Profile},
		Off:  []string{Workspace, Branch, Model, Context, Effort, FiveHour, SevenDay, Cost},
	}
	profileLayout := &config.StatusLineLayout{
		Row1: []string{Model},
		Off:  []string{Profile, Workspace, Branch, Context, Effort, FiveHour, SevenDay, Cost},
	}

	cfg := func(global, override *config.StatusLineLayout) *config.Config {
		return &config.Config{
			Profiles: map[string]config.ProfileConfig{
				"work": {Name: "work", StatusLine: override},
			},
			Settings: config.Settings{StatusLine: global},
		}
	}

	cases := []struct {
		name    string
		cfg     *config.Config
		profile string
		want    []string // expected Row1
	}{
		{"profile override wins over the global default", cfg(globalLayout, profileLayout), "work", []string{Model}},
		{"global default applies when the profile has no override", cfg(globalLayout, nil), "work", []string{Profile}},
		{"built-in applies when neither is configured", cfg(nil, nil), "work", Default().Row1},
		{"an unknown profile falls through to the global default", cfg(globalLayout, profileLayout), "nope", []string{Profile}},
		{"no profile resolved skips the override lookup", cfg(globalLayout, profileLayout), "", []string{Profile}},
		{"a nil config still renders", nil, "work", Default().Row1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := Resolve(tc.cfg, tc.profile).Row1; !reflect.DeepEqual(got, tc.want) {
				t.Errorf("Row1 = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestHasOverrideAndGlobal(t *testing.T) {
	override := &config.StatusLineLayout{Row1: []string{Model}}
	globalOnlyProfile := &config.StatusLineLayout{
		Row1: []string{Profile},
		Off:  []string{Workspace, Branch, Model, Context, Effort, FiveHour, SevenDay, Cost},
	}
	cfg := &config.Config{
		Profiles: map[string]config.ProfileConfig{
			"work": {Name: "work", StatusLine: override},
			"solo": {Name: "solo"},
		},
		Settings: config.Settings{StatusLine: globalOnlyProfile},
	}

	if !HasOverride(cfg, "work") {
		t.Error("work has an override")
	}
	for _, p := range []string{"solo", "nope", ""} {
		if HasOverride(cfg, p) {
			t.Errorf("%q must not report an override", p)
		}
	}
	if HasOverride(nil, "work") {
		t.Error("a nil config has no overrides")
	}

	// Global ignores the override even when asked about a profile that has one.
	if got := Global(cfg).Row1; !reflect.DeepEqual(got, []string{Profile}) {
		t.Errorf("Global Row1 = %v, want the global default", got)
	}
	if got := Global(nil).Row1; !reflect.DeepEqual(got, Default().Row1) {
		t.Errorf("Global(nil) Row1 = %v, want the built-in", got)
	}
}

func TestKnown(t *testing.T) {
	for _, s := range Segments {
		if !Known(s.Key) {
			t.Errorf("catalog segment %q is not Known", s.Key)
		}
	}
	for _, k := range []string{"", "weather", "PROFILE", "profile "} {
		if Known(k) {
			t.Errorf("Known(%q) must be false", k)
		}
	}
}
