//go:build darwin

// Package rail draws the floating usage rail: a borderless always-on-top panel
// pinned to a screen edge, one ring per ccpm profile.
//
// It exists as hand-written cgo because Wails v2 cannot open a second window —
// pkg/runtime/window.go takes no window handle on any of its functions, the
// darwin frontend holds exactly one mainWindow, and NewWindow lives under
// internal/ where an app cannot reach it. Upstream defers multi-window to v3.
//
// Everything in this file is deliberately free of AppKit so it can be tested
// without a display. The cgo lives in rail_darwin.m and its thin Go bridge.
package rail

// Rect is a macOS screen rectangle: origin bottom-left, points not pixels.
type Rect struct {
	X, Y, W, H float64
}

// Edge is where the rail sits.
type Edge string

const (
	EdgeRight  Edge = "right"
	EdgeLeft   Edge = "left"
	EdgeTop    Edge = "top"
	EdgeBottom Edge = "bottom"
)

// Vertical reports whether the rail stacks its rings top-to-bottom (left and
// right edges) rather than side-by-side (top and bottom edges).
func (e Edge) Vertical() bool { return e == EdgeRight || e == EdgeLeft }

// ParseEdge maps a stored preference string to an Edge, defaulting to the right
// edge for anything unrecognised so a hand-edited preferences file cannot leave
// the panel unplaceable.
func ParseEdge(s string) Edge {
	switch Edge(s) {
	case EdgeRight, EdgeLeft, EdgeTop, EdgeBottom:
		return Edge(s)
	default:
		return EdgeRight
	}
}

// Layout constants, in points.
const (
	// Thickness is the rail's short dimension — wide enough for a ring plus its
	// percentage label.
	Thickness = 64
	// SlotLength is the space one profile occupies along the rail's long axis.
	SlotLength = 76
	// EndPadding is the breathing room at each end of the stack.
	EndPadding = 10
	// EdgeMargin holds the rail just off the screen edge so its shadow reads.
	EdgeMargin = 8
)

// StackLength returns the rail's long dimension for n profiles. A rail with no
// profiles still returns a positive size — callers hide it rather than trying
// to place a zero-area window, which AppKit handles poorly.
func StackLength(n int) float64 {
	if n < 1 {
		n = 1
	}
	return float64(n)*SlotLength + 2*EndPadding
}

// PanelRect places the rail within a screen's visible frame.
//
// visible is NSScreen.visibleFrame — the area excluding the menu bar and Dock,
// which is what keeps the rail from sliding under either. The rail is centred
// on its long axis and clamped so it never exceeds the visible area, since a
// panel taller than the screen would push its first ring off the top where no
// pointer could ever reach it.
func PanelRect(visible Rect, edge Edge, profiles int) Rect {
	length := StackLength(profiles)

	if edge.Vertical() {
		maxLen := visible.H - 2*EdgeMargin
		if maxLen > 0 && length > maxLen {
			length = maxLen
		}
		y := visible.Y + (visible.H-length)/2
		x := visible.X + visible.W - Thickness - EdgeMargin
		if edge == EdgeLeft {
			x = visible.X + EdgeMargin
		}
		return Rect{X: x, Y: y, W: Thickness, H: length}
	}

	maxLen := visible.W - 2*EdgeMargin
	if maxLen > 0 && length > maxLen {
		length = maxLen
	}
	x := visible.X + (visible.W-length)/2
	y := visible.Y + EdgeMargin
	if edge == EdgeTop {
		y = visible.Y + visible.H - Thickness - EdgeMargin
	}
	return Rect{X: x, Y: y, W: length, H: Thickness}
}

// SlotRect returns the sub-rectangle of the panel occupied by ring i, in
// panel-local coordinates (origin bottom-left).
//
// Vertical rails read top-to-bottom, matching the reference design and how a
// person scans a list — which means index 0 sits at the HIGHEST y, not the
// lowest, because macOS y grows upward.
func SlotRect(panel Rect, edge Edge, i, total int) Rect {
	if total < 1 {
		total = 1
	}
	if edge.Vertical() {
		slot := (panel.H - 2*EndPadding) / float64(total)
		top := panel.H - EndPadding
		return Rect{X: 0, Y: top - slot*float64(i+1), W: panel.W, H: slot}
	}
	slot := (panel.W - 2*EndPadding) / float64(total)
	return Rect{X: EndPadding + slot*float64(i), Y: 0, W: slot, H: panel.H}
}

// HitIndex maps a point in panel-local coordinates to a ring index, or -1 when
// the point falls outside every slot.
func HitIndex(panel Rect, edge Edge, total int, x, y float64) int {
	if total < 1 {
		return -1
	}
	if x < 0 || y < 0 || x > panel.W || y > panel.H {
		return -1
	}
	for i := range total {
		s := SlotRect(panel, edge, i, total)
		if x >= s.X && x <= s.X+s.W && y >= s.Y && y <= s.Y+s.H {
			return i
		}
	}
	return -1
}

// CalloutAnchor returns where the callout's pointer should meet the rail for
// ring i, in screen coordinates, plus which side the callout body extends
// toward. The callout always grows away from the screen edge the rail is on,
// so it never renders off-screen.
func CalloutAnchor(panel Rect, edge Edge, i, total int) (x, y float64, grows Edge) {
	s := SlotRect(panel, edge, i, total)
	cx := panel.X + s.X + s.W/2
	cy := panel.Y + s.Y + s.H/2

	switch edge {
	case EdgeRight:
		return panel.X, cy, EdgeLeft
	case EdgeLeft:
		return panel.X + panel.W, cy, EdgeRight
	case EdgeTop:
		return cx, panel.Y, EdgeBottom
	default:
		return cx, panel.Y + panel.H, EdgeTop
	}
}
