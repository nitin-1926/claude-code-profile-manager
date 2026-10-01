//go:build darwin

package rail

import "testing"

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
