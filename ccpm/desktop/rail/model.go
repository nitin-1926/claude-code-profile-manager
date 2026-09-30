//go:build darwin

package rail

import (
	"fmt"
	"time"

	"github.com/nitin-1926/claude-code-profile-manager/ccpm/desktop/services"
	"github.com/nitin-1926/claude-code-profile-manager/ccpm/internal/usage"
)

// Model is everything the Objective-C renderer needs to draw a frame. It
// crosses the cgo boundary as JSON rather than as a C struct array: the shape
// has strings and nesting, and NSJSONSerialization decodes it in three lines,
// which is a great deal less code than hand-marshalling char** and freeing it.
type Model struct {
	Slots []Slot  `json:"slots"`
	Theme Palette `json:"theme"`
}

// Slot is one profile's ring.
//
// Two concentric arcs: the outer is the main window — five-hour unless the
// user picked the weekly one — and the inner the other. Both matter and a
// person watching the rail wants the shape of both without hovering — that is
// the entire reason the rail exists rather than a menu item.
type Slot struct {
	Profile string `json:"profile"`
	// Percent is the label under the ring: the main window's figure, or an
	// em-dash when there is nothing honest to show.
	Percent string `json:"percent"`
	// Available is false when this profile has no usable reading. The renderer
	// draws track only — never a filled arc, because a 0% arc reads as "plenty
	// of headroom" when the truth is "we do not know".
	Available bool `json:"available"`

	Outer    float64 `json:"outer"` // main window's fill, 0..1
	Inner    float64 `json:"inner"` // the other window's fill, 0..1
	OuterRGB RGB     `json:"outerRGB"`
	InnerRGB RGB     `json:"innerRGB"`

	// Callout travels with the ring rather than being fetched on hover. The
	// pointer is already moving when it is needed, and re-reading three
	// profiles' limit files at that moment would put disk I/O on the hover
	// path for data we already have.
	Callout Callout `json:"callout"`
}

// FillFraction converts a used-percentage to an arc sweep.
//
// Clamped at both ends: the API has no contract forbidding a value above 100
// (an overage, or a rounding artefact), and an arc sweeping past a full turn
// draws as a thin sliver — the visual opposite of the "completely full" it
// actually means.
func FillFraction(usedPercentage float64) float64 {
	switch {
	case usedPercentage <= 0:
		return 0
	case usedPercentage >= 100:
		return 1
	}
	return usedPercentage / 100
}

// BuildModel turns the service-layer limits into a drawable frame.
//
// Order is the caller's; it is already name-sorted by LimitsService.All so the
// rings do not reshuffle between refreshes.
//
// main is the usage window drawn as the big outer ring (usage.KeySevenDay, or
// anything else for the five-hour default). Swapping here rather than in the
// renderer keeps Outer/Inner meaning "big ring / small ring" all the way down,
// so the Objective-C side has no second copy of the choice to keep in step.
func BuildModel(limits []services.ProfileLimits, theme, main string, now time.Time) Model {
	m := Model{Theme: PaletteFor(theme), Slots: make([]Slot, 0, len(limits))}
	for _, l := range limits {
		m.Slots = append(m.Slots, buildSlot(l, main == usage.KeySevenDay, now))
	}
	return m
}

func buildSlot(l services.ProfileLimits, weeklyMain bool, now time.Time) Slot {
	s := Slot{
		Callout:   BuildCallout(l, now),
		Profile:   l.Profile,
		Percent:   "—",
		Available: l.Available,
		// Unavailable still needs colours: the renderer tints the track with
		// them, and a zero value would be black on black.
		OuterRGB: NotchAmple,
		InnerRGB: NotchAmple,
	}
	if !l.Available {
		return s
	}

	for _, w := range l.Windows {
		frac := FillFraction(w.UsedPercentage)
		// HeadroomColor grades on what is LEFT, so invert here rather than
		// teaching it about used-percentages and having two conventions.
		color := NotchColor(int(100 - w.UsedPercentage))
		var isMain bool
		switch w.Key {
		case usage.KeyFiveHour:
			isMain = !weeklyMain
		case usage.KeySevenDay:
			isMain = weeklyMain
		default:
			continue
		}
		if isMain {
			s.Outer, s.OuterRGB = frac, color
			s.Percent = fmt.Sprintf("%d%%", int(w.UsedPercentage+0.5))
		} else {
			s.Inner, s.InnerRGB = frac, color
		}
	}
	return s
}
