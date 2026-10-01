//go:build darwin

// Package rail draws the floating usage notch: a borderless always-on-top panel
// pinned to a screen edge, one ring per ccpm profile.
//
// This file holds the vocabulary — rects, edges, the stack metrics. The
// placement model lives in notch.go, which is where the reasoning about why the
// panel never resizes belongs.
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

// fallbackScreen stands in when AppKit has not published a screen frame yet.
//
// Every dimension of PanelRect is computed relative to the screen it is given,
// so a zero rect does not merely produce a small panel — it produces a panel
// placed off the bottom-left corner, where nothing can see or reach it. The
// origin is the main screen's own bottom-left, and 1280x720 is smaller than any
// display macOS ships on, so the result is always inside the real screen.
var fallbackScreen = Rect{X: 0, Y: 0, W: 1280, H: 720}
