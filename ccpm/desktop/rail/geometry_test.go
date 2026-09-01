//go:build darwin

package rail

import (
	"math"
	"testing"
)

// A 16" MacBook's visible frame: full screen minus menu bar and Dock.
var laptop = Rect{X: 0, Y: 0, W: 1728, H: 1000}

func closeTo(a, b float64) bool { return math.Abs(a-b) < 0.001 }

func TestPanelRectPlacesEachEdge(t *testing.T) {
	const profiles = 3
	length := StackLength(profiles)

	t.Run("right", func(t *testing.T) {
		r := PanelRect(laptop, EdgeRight, profiles)
		if !closeTo(r.X, laptop.W-Thickness-EdgeMargin) {
			t.Errorf("x = %v, want the rail just inside the right edge", r.X)
		}
		if !closeTo(r.W, Thickness) || !closeTo(r.H, length) {
			t.Errorf("size = %vx%v, want %vx%v", r.W, r.H, Thickness, length)
		}
		// Centred vertically: equal gap above and below.
		if !closeTo(r.Y, (laptop.H-length)/2) {
			t.Errorf("y = %v, want vertically centred", r.Y)
		}
	})

	t.Run("left", func(t *testing.T) {
		r := PanelRect(laptop, EdgeLeft, profiles)
		if !closeTo(r.X, EdgeMargin) {
			t.Errorf("x = %v, want the rail just inside the left edge", r.X)
		}
		if !closeTo(r.W, Thickness) {
			t.Errorf("w = %v, want a vertical rail", r.W)
		}
	})

	t.Run("bottom", func(t *testing.T) {
		r := PanelRect(laptop, EdgeBottom, profiles)
		if !closeTo(r.Y, EdgeMargin) {
			t.Errorf("y = %v, want the rail just above the bottom edge", r.Y)
		}
		if !closeTo(r.H, Thickness) || !closeTo(r.W, length) {
			t.Errorf("size = %vx%v, want a horizontal rail", r.W, r.H)
		}
		if !closeTo(r.X, (laptop.W-length)/2) {
			t.Errorf("x = %v, want horizontally centred", r.X)
		}
	})

	t.Run("top", func(t *testing.T) {
		r := PanelRect(laptop, EdgeTop, profiles)
		if !closeTo(r.Y, laptop.H-Thickness-EdgeMargin) {
			t.Errorf("y = %v, want the rail just below the top edge", r.Y)
		}
	})
}

// The visible frame does not always start at the origin — an external display
// sits at a non-zero offset in the global coordinate space, and the Dock or
// menu bar shifts the origin on the primary one too.
func TestPanelRectRespectsOffsetScreens(t *testing.T) {
	external := Rect{X: 1728, Y: 200, W: 2560, H: 1400}
	r := PanelRect(external, EdgeRight, 3)

	if r.X < external.X {
		t.Errorf("panel x = %v landed left of the screen origin %v", r.X, external.X)
	}
	if !closeTo(r.X, external.X+external.W-Thickness-EdgeMargin) {
		t.Errorf("panel x = %v, want it relative to the screen's own frame", r.X)
	}
	if r.Y < external.Y {
		t.Errorf("panel y = %v landed below the screen origin %v", r.Y, external.Y)
	}
}

// A user with many profiles must not get a rail taller than the screen — the
// first ring would sit off the top edge where no pointer can reach it.
func TestPanelRectClampsToVisibleArea(t *testing.T) {
	small := Rect{X: 0, Y: 0, W: 1440, H: 400}
	r := PanelRect(small, EdgeRight, 20)

	if r.H > small.H {
		t.Fatalf("panel height %v exceeds the visible height %v", r.H, small.H)
	}
	if r.Y < small.Y {
		t.Errorf("clamped panel starts at y = %v, above the visible origin %v", r.Y, small.Y)
	}
	if r.Y+r.H > small.Y+small.H {
		t.Errorf("clamped panel bottom %v overflows the visible area", r.Y+r.H)
	}
}

// Degenerate inputs must still yield something placeable. AppKit handles a
// zero-area window badly, and startup ordering means we can be asked to place
// the rail before a screen frame is known.
func TestPanelRectDegenerateInputs(t *testing.T) {
	for _, c := range []struct {
		name    string
		visible Rect
		n       int
	}{
		{"zero screen", Rect{}, 3},
		{"zero profiles", laptop, 0},
		{"negative profiles", laptop, -5},
		{"both zero", Rect{}, 0},
	} {
		r := PanelRect(c.visible, EdgeRight, c.n)
		if r.W <= 0 || r.H <= 0 {
			t.Errorf("%s: produced a zero-area panel %+v", c.name, r)
		}
		if math.IsNaN(r.X) || math.IsNaN(r.Y) || math.IsNaN(r.W) || math.IsNaN(r.H) {
			t.Errorf("%s: produced NaN geometry %+v", c.name, r)
		}
	}
}

func TestStackLengthGrowsWithProfiles(t *testing.T) {
	one, three := StackLength(1), StackLength(3)
	if three <= one {
		t.Errorf("three profiles (%v) should need more room than one (%v)", three, one)
	}
	if StackLength(0) <= 0 {
		t.Error("an empty rail must still have a positive length")
	}
}

// Vertical rails read top-to-bottom. Because macOS y grows upward, index 0
// must sit at the HIGHEST y — getting this backwards silently inverts the
// whole stack against the reference design.
func TestSlotRectVerticalOrderIsTopDown(t *testing.T) {
	panel := PanelRect(laptop, EdgeRight, 3)
	first := SlotRect(panel, EdgeRight, 0, 3)
	last := SlotRect(panel, EdgeRight, 2, 3)

	if first.Y <= last.Y {
		t.Errorf("slot 0 (y=%v) should sit above slot 2 (y=%v)", first.Y, last.Y)
	}
	if !closeTo(first.H, last.H) {
		t.Errorf("slots differ in height: %v vs %v", first.H, last.H)
	}
	for i := 0; i < 3; i++ {
		s := SlotRect(panel, EdgeRight, i, 3)
		if s.Y < 0 || s.Y+s.H > panel.H {
			t.Errorf("slot %d escapes the panel: %+v in %+v", i, s, panel)
		}
	}
}

func TestSlotRectHorizontalOrderIsLeftToRight(t *testing.T) {
	panel := PanelRect(laptop, EdgeBottom, 3)
	first := SlotRect(panel, EdgeBottom, 0, 3)
	last := SlotRect(panel, EdgeBottom, 2, 3)

	if first.X >= last.X {
		t.Errorf("slot 0 (x=%v) should sit left of slot 2 (x=%v)", first.X, last.X)
	}
}

func TestHitIndexMapsPointerToRing(t *testing.T) {
	panel := PanelRect(laptop, EdgeRight, 3)

	for i := 0; i < 3; i++ {
		s := SlotRect(panel, EdgeRight, i, 3)
		cx, cy := s.X+s.W/2, s.Y+s.H/2
		if got := HitIndex(panel, EdgeRight, 3, cx, cy); got != i {
			t.Errorf("centre of slot %d hit %d", i, got)
		}
	}

	// Outside the panel in every direction.
	for _, p := range [][2]float64{{-1, 10}, {10, -1}, {panel.W + 1, 10}, {10, panel.H + 1}} {
		if got := HitIndex(panel, EdgeRight, 3, p[0], p[1]); got != -1 {
			t.Errorf("point %v outside the panel hit %d", p, got)
		}
	}

	if got := HitIndex(panel, EdgeRight, 0, 10, 10); got != -1 {
		t.Errorf("an empty rail hit %d", got)
	}
}

// The callout must grow away from the screen edge, or half of it renders
// off-screen.
func TestCalloutAnchorGrowsInward(t *testing.T) {
	cases := []struct {
		edge  Edge
		grows Edge
	}{
		{EdgeRight, EdgeLeft},
		{EdgeLeft, EdgeRight},
		{EdgeTop, EdgeBottom},
		{EdgeBottom, EdgeTop},
	}
	for _, c := range cases {
		panel := PanelRect(laptop, c.edge, 3)
		x, y, grows := CalloutAnchor(panel, c.edge, 1, 3)
		if grows != c.grows {
			t.Errorf("edge %s: callout grows %s, want %s", c.edge, grows, c.grows)
		}
		// The anchor sits on the panel's boundary, not floating away from it.
		if x < panel.X-0.001 || x > panel.X+panel.W+0.001 {
			t.Errorf("edge %s: anchor x %v is off the panel [%v,%v]", c.edge, x, panel.X, panel.X+panel.W)
		}
		if y < panel.Y-0.001 || y > panel.Y+panel.H+0.001 {
			t.Errorf("edge %s: anchor y %v is off the panel [%v,%v]", c.edge, y, panel.Y, panel.Y+panel.H)
		}
	}
}

func TestParseEdgeFallsBackToRight(t *testing.T) {
	for _, s := range []string{"right", "left", "top", "bottom"} {
		if got := ParseEdge(s); string(got) != s {
			t.Errorf("ParseEdge(%q) = %q", s, got)
		}
	}
	for _, s := range []string{"", "diagonal", "RIGHT"} {
		if got := ParseEdge(s); got != EdgeRight {
			t.Errorf("ParseEdge(%q) = %q, want the right-edge fallback", s, got)
		}
	}
}

func TestEdgeVertical(t *testing.T) {
	if !EdgeRight.Vertical() || !EdgeLeft.Vertical() {
		t.Error("left and right rails stack vertically")
	}
	if EdgeTop.Vertical() || EdgeBottom.Vertical() {
		t.Error("top and bottom rails stack horizontally")
	}
}
