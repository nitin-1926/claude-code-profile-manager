//go:build darwin

package rail

import (
	"sync"
	"testing"
	"time"

	"github.com/nitin-1926/claude-code-profile-manager/ccpm/desktop/services"
	"github.com/nitin-1926/claude-code-profile-manager/ccpm/internal/usage"
)

func TestBuildDockMeter(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	future := now.Add(time.Hour).Unix()
	past := now.Add(-time.Minute).Unix()
	reading := func(profile string, windows ...services.LimitWindowDTO) services.ProfileLimits {
		return services.ProfileLimits{Profile: profile, Available: true, Windows: windows}
	}
	five := func(used float64, resets int64) services.LimitWindowDTO {
		return services.LimitWindowDTO{Key: usage.KeyFiveHour, UsedPercentage: used, ResetsAt: resets}
	}
	seven := func(used float64) services.LimitWindowDTO {
		return services.LimitWindowDTO{Key: usage.KeySevenDay, UsedPercentage: used, ResetsAt: future}
	}
	noReading := services.ProfileLimits{Profile: "api", Available: false, Windows: []services.LimitWindowDTO{}}

	tests := []struct {
		name   string
		limits []services.ProfileLimits
		main   string
		want   DockMeter
	}{
		{"nothing used", []services.ProfileLimits{reading("w", five(0, future))}, usage.KeyFiveHour, DockMeter{true, 100, NotchAmple}},
		{"half is the ample boundary", []services.ProfileLimits{reading("w", five(50, future))}, usage.KeyFiveHour, DockMeter{true, 50, NotchAmple}},
		{"tightening", []services.ProfileLimits{reading("w", five(58, future))}, usage.KeyFiveHour, DockMeter{true, 42, NotchWatch}},
		{"a fifth is the watch boundary", []services.ProfileLimits{reading("w", five(80, future))}, usage.KeyFiveHour, DockMeter{true, 20, NotchWatch}},
		{"near limit", []services.ProfileLimits{reading("w", five(90, future))}, usage.KeyFiveHour, DockMeter{true, 10, NotchCritical}},
		{"over 100 is empty, not negative", []services.ProfileLimits{reading("w", five(120, future))}, usage.KeyFiveHour, DockMeter{true, 0, NotchCritical}},
		{"past its reset is full again", []services.ProfileLimits{reading("w", five(90, past))}, usage.KeyFiveHour, DockMeter{true, 100, NotchAmple}},
		{"main picks the window", []services.ProfileLimits{reading("w", five(10, future), seven(90))}, usage.KeySevenDay, DockMeter{true, 10, NotchCritical}},
		{"skips a profile with no reading", []services.ProfileLimits{noReading, reading("w", five(30, future))}, usage.KeyFiveHour, DockMeter{true, 70, NotchAmple}},
		{"skips a profile without that window", []services.ProfileLimits{reading("a", seven(99)), reading("b", five(30, future))}, usage.KeyFiveHour, DockMeter{true, 70, NotchAmple}},
		{"first profile with a reading wins", []services.ProfileLimits{reading("a", five(10, future)), reading("b", five(90, future))}, usage.KeyFiveHour, DockMeter{true, 90, NotchAmple}},
		{"no reading anywhere is the plain icon", []services.ProfileLimits{noReading}, usage.KeyFiveHour, DockMeter{}},
		{"no profiles is the plain icon", nil, usage.KeyFiveHour, DockMeter{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := BuildDockMeter(tt.limits, "", tt.main, now); got != tt.want {
				t.Errorf("got %+v, want %+v", got, tt.want)
			}
		})
	}

	// The default profile outranks name order, and a default with no reading
	// falls back rather than leaving the plain icon.
	both := []services.ProfileLimits{reading("a", five(10, future)), reading("work", five(30, future))}
	if got, want := BuildDockMeter(both, "work", usage.KeyFiveHour, now), (DockMeter{true, 70, NotchAmple}); got != want {
		t.Errorf("preferred: got %+v, want %+v", got, want)
	}
	unread := []services.ProfileLimits{reading("a", five(10, future)), noReading}
	if got, want := BuildDockMeter(unread, "api", usage.KeyFiveHour, now), (DockMeter{true, 90, NotchAmple}); got != want {
		t.Errorf("preferred without a reading: got %+v, want %+v", got, want)
	}
}

// The meter and the notch grade the same reading, including a fractional one
// right on a threshold, where rounding the remainder would make them disagree.
func TestDockMeterColourMatchesTheNotch(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	for _, used := range []float64{0, 49.6, 50, 50.4, 79.6, 80, 80.4, 100, 130} {
		l := []services.ProfileLimits{{Profile: "w", Available: true, Windows: []services.LimitWindowDTO{
			{Key: usage.KeyFiveHour, UsedPercentage: used},
		}}}
		notch := BuildModel(l, ThemeGraphite, usage.KeyFiveHour, now).Slots[0].OuterRGB
		if got := BuildDockMeter(l, "", usage.KeyFiveHour, now).RGB; got != notch {
			t.Errorf("used %v: dock colour %06x, notch colour %06x", used, got, notch)
		}
	}
}

func TestDockTileRedrawsOnlyOnChange(t *testing.T) {
	var drawn []DockMeter
	d := &DockTile{draw: func(m DockMeter) { drawn = append(drawn, m) }}

	d.Set(DockMeter{}) // default off at startup: the icon is never touched
	if len(drawn) != 0 {
		t.Fatalf("drew %v for the plain icon it already shows", drawn)
	}

	m := DockMeter{Shown: true, Remaining: 58, RGB: NotchAmple}
	d.Set(m)
	d.Set(m)
	if len(drawn) != 1 {
		t.Fatalf("same percentage drew %d times, want 1", len(drawn))
	}

	d.Set(DockMeter{Shown: true, Remaining: 57, RGB: NotchAmple})
	if len(drawn) != 2 {
		t.Fatalf("new percentage did not redraw: %v", drawn)
	}

	d.Set(DockMeter{})
	if len(drawn) != 3 || drawn[2].Shown {
		t.Fatalf("turning off did not ask for the plain icon: %v", drawn)
	}
}

// Set runs from the ticker, the watcher and preference writes at once.
func TestDockTileSetIsSafeConcurrently(t *testing.T) {
	d := &DockTile{draw: func(DockMeter) {}}
	var wg sync.WaitGroup
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			d.Set(DockMeter{Shown: true, Remaining: i})
		}(i)
	}
	wg.Wait()
}
