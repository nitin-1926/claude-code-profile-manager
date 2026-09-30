//go:build darwin

package rail

import (
	"math"
	"testing"
)

// A 16" MacBook's visible frame: full screen minus menu bar and Dock.
var laptop = Rect{X: 0, Y: 0, W: 1728, H: 1000}

func closeTo(a, b float64) bool { return math.Abs(a-b) < 0.001 }

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

// The collapsed peek must hug the screen edge — flush, not inset — and cover
// exactly the same span as the full panel, or the rail appears to jump sideways
// as it reveals rather than sliding out.
