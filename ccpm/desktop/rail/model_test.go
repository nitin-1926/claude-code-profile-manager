//go:build darwin

package rail

import (
	"testing"

	"github.com/nitin-1926/claude-code-profile-manager/ccpm/desktop/services"
	"github.com/nitin-1926/claude-code-profile-manager/ccpm/internal/usage"
)

func TestFillFractionClamps(t *testing.T) {
	cases := []struct {
		in   float64
		want float64
	}{
		{0, 0},
		{50, 0.5},
		{99.5, 0.995},
		{100, 1},
		// An overage must read as completely full. Left unclamped the arc
		// sweeps past a full turn and draws as a thin sliver — the visual
		// opposite of what it means.
		{140, 1},
		{-10, 0},
	}
	for _, c := range cases {
		if got := FillFraction(c.in); got != c.want {
			t.Errorf("FillFraction(%v) = %v, want %v", c.in, got, c.want)
		}
	}
}

func available(profile string, fiveHour, sevenDay float64) services.ProfileLimits {
	return services.ProfileLimits{
		Profile:   profile,
		Available: true,
		Windows: []services.LimitWindowDTO{
			{Key: usage.KeyFiveHour, UsedPercentage: fiveHour},
			{Key: usage.KeySevenDay, UsedPercentage: sevenDay},
		},
	}
}

func TestBuildModelMapsBothWindows(t *testing.T) {
	m := BuildModel([]services.ProfileLimits{available("work", 80, 30)}, ThemeGraphite, usage.KeyFiveHour, now)
	if len(m.Slots) != 1 {
		t.Fatalf("got %d slots, want 1", len(m.Slots))
	}
	s := m.Slots[0]
	if s.Outer != 0.8 {
		t.Errorf("outer (five-hour) = %v, want 0.8", s.Outer)
	}
	if s.Inner != 0.3 {
		t.Errorf("inner (seven-day) = %v, want 0.3", s.Inner)
	}
	if s.Percent != "80%" {
		t.Errorf("percent = %q, want the five-hour figure", s.Percent)
	}
	// 80% used is 20% left: tightening, per the statusline thresholds.
	if s.OuterRGB != NotchWatch {
		t.Errorf("outer colour = %#06x, want tightening for 20%% headroom", s.OuterRGB)
	}
	// 30% used is 70% left: healthy.
	if s.InnerRGB != NotchAmple {
		t.Errorf("inner colour = %#06x, want healthy for 70%% headroom", s.InnerRGB)
	}
}

// The whole point of tracking availability separately: an unavailable profile
// must never be drawn as an empty ring, because an empty ring reads as "plenty
// of headroom" when the truth is "we have no idea".
func TestUnavailableProfileDrawsNoFill(t *testing.T) {
	for _, reason := range []string{services.ReasonNoData, services.ReasonNotSubscribed} {
		m := BuildModel([]services.ProfileLimits{{
			Profile:   "cin",
			Available: false,
			Reason:    reason,
		}}, ThemeGraphite, usage.KeyFiveHour, now)
		s := m.Slots[0]
		if s.Available {
			t.Errorf("%s: slot reported available", reason)
		}
		if s.Outer != 0 || s.Inner != 0 {
			t.Errorf("%s: unavailable slot has fill %v/%v", reason, s.Outer, s.Inner)
		}
		if s.Percent != "—" {
			t.Errorf("%s: percent = %q, want an em-dash rather than a number", reason, s.Percent)
		}
		// Colours still have to be readable values, not zero-value black.
		if s.OuterRGB == 0 || s.InnerRGB == 0 {
			t.Errorf("%s: unavailable slot has zero-value colours", reason)
		}
	}
}

// A profile reporting only one of the two windows must not leave the other arc
// filled with a stale or zero-value colour.
func TestPartialWindowsLeaveTheOtherArcEmpty(t *testing.T) {
	m := BuildModel([]services.ProfileLimits{{
		Profile:   "labs",
		Available: true,
		Windows:   []services.LimitWindowDTO{{Key: usage.KeyFiveHour, UsedPercentage: 10}},
	}}, ThemeGraphite, usage.KeyFiveHour, now)
	s := m.Slots[0]
	if s.Outer != 0.1 {
		t.Errorf("outer = %v, want 0.1", s.Outer)
	}
	if s.Inner != 0 {
		t.Errorf("inner = %v, want no seven-day arc when no seven-day window was reported", s.Inner)
	}
}

func TestPercentRoundsRatherThanTruncates(t *testing.T) {
	cases := map[float64]string{
		0:    "0%",
		49.4: "49%",
		49.6: "50%",
		99.9: "100%",
		100:  "100%",
	}
	for used, want := range cases {
		m := BuildModel([]services.ProfileLimits{available("work", used, 0)}, ThemeGraphite, usage.KeyFiveHour, now)
		if got := m.Slots[0].Percent; got != want {
			t.Errorf("%v%% used rendered as %q, want %q", used, got, want)
		}
	}
}

func TestBuildModelPreservesOrder(t *testing.T) {
	in := []services.ProfileLimits{available("cin", 1, 1), available("labs", 2, 2), available("work", 3, 3)}
	m := BuildModel(in, ThemeGraphite, usage.KeyFiveHour, now)
	for i, want := range []string{"cin", "labs", "work"} {
		if m.Slots[i].Profile != want {
			t.Errorf("slot %d is %q, want %q — ring order must not reshuffle between refreshes", i, m.Slots[i].Profile, want)
		}
	}
}

func TestBuildModelCarriesTheTheme(t *testing.T) {
	if got := BuildModel(nil, ThemeLight, usage.KeyFiveHour, now).Theme; got != PaletteFor(ThemeLight) {
		t.Errorf("theme = %+v, want the light palette", got)
	}
	// No profiles is a legitimate state (all toggled off); it must not panic
	// and must still carry a usable palette.
	if BuildModel(nil, ThemeGraphite, usage.KeyFiveHour, now).Theme.Foreground == 0 {
		t.Error("an empty model has no usable foreground colour")
	}
}

// Picking the weekly window as the main ring swaps which window is the big
// outer arc and whose figure is the label, and nothing else: the five-hour
// window becomes the inner arc, each keeping its own colour.
func TestBuildModelWeeklyMainRingSwapsTheArcs(t *testing.T) {
	m := BuildModel([]services.ProfileLimits{available("work", 80, 30)}, ThemeGraphite, usage.KeySevenDay, now)
	s := m.Slots[0]
	if s.Outer != 0.3 || s.Inner != 0.8 {
		t.Errorf("weekly main: outer %v inner %v, want the seven-day 0.3 outside and the five-hour 0.8 inside", s.Outer, s.Inner)
	}
	if s.Percent != "30%" {
		t.Errorf("weekly main: percent = %q, want the seven-day figure", s.Percent)
	}
	if s.OuterRGB != NotchAmple || s.InnerRGB != NotchWatch {
		t.Errorf("weekly main: colours %#06x/%#06x did not travel with their windows", s.OuterRGB, s.InnerRGB)
	}

	// Anything that is not the weekly key is the five-hour default, so a
	// stale or empty preference can never leave the ring blank.
	for _, main := range []string{usage.KeyFiveHour, "", "monthly"} {
		if got := BuildModel([]services.ProfileLimits{available("work", 80, 30)}, ThemeGraphite, main, now).Slots[0]; got.Outer != 0.8 || got.Percent != "80%" {
			t.Errorf("main %q: outer %v percent %q, want the five-hour default", main, got.Outer, got.Percent)
		}
	}
}
