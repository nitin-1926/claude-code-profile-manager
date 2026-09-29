//go:build darwin

package rail

/*
#cgo CFLAGS: -x objective-c -fobjc-arc -Wno-unused-parameter
#cgo LDFLAGS: -framework Cocoa -framework QuartzCore
#include <stdlib.h>
#include "rail.h"
*/
import "C"

import (
	"encoding/json"
	"sync"
	"time"
	"unsafe"
)

// Controller owns the panel's lifecycle and placement.
//
// Safe to drive from any goroutine: the C layer hops to the main queue itself,
// and this struct's own state is mutex-guarded. Every method is a no-op before
// Start and after Stop, because Wails' OnStartup ordering is not guaranteed and
// a rail that panics on an early call is worse than one that appears a moment
// late.
type Controller struct {
	mu      sync.Mutex
	started bool
	// retrying is set while a re-layout is scheduled for a frame AppKit has
	// not published yet; see applyLocked.
	retrying bool
	edge     Edge
	profiles int
	// hidePercent drops the percentage labels and the room reserved for them.
	hidePercent bool
	visible     bool
	// visibility is the tri-state from prefs: hidden, hover-reveal, always
	// expanded. It replaces the old hover bool, which could not express
	// "always" without a second flag the C layer had to combine itself.
	visibility Visibility
	model      Model
}

// Visibility mirrors the RailMode preference. The zero value is hover-reveal,
// which is what a fresh install gets.
type Visibility int

const (
	VisibilityHover Visibility = iota
	VisibilityAlways
	VisibilityHidden
)

// New returns an unstarted controller.
func New() *Controller {
	return &Controller{edge: EdgeRight, profiles: 1}
}

// Start creates the panel. Idempotent.
func (c *Controller) Start() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.started {
		return
	}
	C.CCPMNotchStart()
	c.started = true
	C.CCPMNotchSetVisibility(C.int(c.visibility))
	c.applyLocked()
	c.pushModelLocked()
}

// Stop tears the panel down. Idempotent.
func (c *Controller) Stop() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.started {
		return
	}
	C.CCPMNotchStop()
	c.started = false
	c.visible = false
}

// SetLayout updates the edge, profile count and whether percentages are
// drawn, re-placing the panel: hiding them changes the notch's size.
func (c *Controller) SetLayout(edge Edge, profiles int, percent bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.edge = edge
	c.profiles = profiles
	c.hidePercent = !percent
	c.applyLocked()
	// Slot and card rects are panel-local but derived from the placement, so a
	// re-place makes the ones the renderer is holding wrong.
	c.pushModelLocked()
}

// SetVisibility chooses between hidden, reveal-on-hover and always expanded.
func (c *Controller) SetVisibility(v Visibility) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.visibility = v
	if !c.started {
		return
	}
	C.CCPMNotchSetVisibility(C.int(v))
}

// SetModel replaces what the rail draws. Held so a later Start can redraw
// without the caller having to remember to push the model again.
func (c *Controller) SetModel(m Model) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.model = m
	c.pushModelLocked()
}

// specLocked is the geometry input for the current layout. Caller holds mu.
//
// The hardware notch is only consulted on the top edge, the one place the
// notch can join to it.
func (c *Controller) specLocked() Spec {
	s := Spec{Edge: c.edge, Profiles: c.profiles, HidePercent: c.hidePercent}
	if c.edge == EdgeTop {
		if hw, ok := HardwareNotch(); ok {
			s.Hardware = hw
		}
	}
	return s
}

// pushModelLocked hands the model to the renderer. Caller holds mu.
//
// Slot and card rectangles travel WITH the model, panel-local, rather than
// being re-derived in Objective-C. The parked rail divided the stack in two
// places — SlotRect here and an open-coded copy in render_darwin.m — and kept
// them in step with a comment. Sending the rects means the drawing, the
// hit-testing and the tests are all reading the same numbers.
func (c *Controller) pushModelLocked() {
	if !c.started {
		return
	}
	spec := c.specLocked()
	panel := PanelRect(ScreenFrame(), spec)
	cells := Cells(panel, spec)
	// A model can briefly carry a different count than the layout while a
	// refresh is in flight; never hand the renderer fewer cells than slots.
	if len(cells) > len(c.model.Slots) {
		cells = cells[:len(c.model.Slots)]
	}

	payload := struct {
		Model
		Edge  string `json:"edge"`
		Cells []Cell `json:"cells"`
	}{Model: c.model, Edge: string(c.edge), Cells: cells}

	b, err := json.Marshal(payload)
	if err != nil {
		return // a model that will not marshal is a bug, but not one worth a panic in the UI
	}
	cs := C.CString(string(b))
	defer C.free(unsafe.Pointer(cs))
	C.CCPMNotchSetModel(cs)
}

// SetVisible shows or hides the panel.
func (c *Controller) SetVisible(v bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.visible = v
	if !c.started {
		return
	}
	if v {
		c.applyLocked()
		C.CCPMNotchShow()
		return
	}
	C.CCPMNotchHide()
}

// Visible reports the panel's actual on-screen state, asked of AppKit rather
// than of our own bookkeeping, so a panel the system dismissed is not reported
// as showing.
func (c *Controller) Visible() bool {
	return C.CCPMNotchIsVisible() != 0
}

// Frame returns where the panel would be placed for the current layout.
func (c *Controller) Frame() Rect {
	c.mu.Lock()
	defer c.mu.Unlock()
	return PanelRect(ScreenFrame(), c.specLocked())
}

// Edge returns the current edge.
func (c *Controller) Edge() Edge {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.edge
}

// applyLocked pushes the panel frame and the three interaction rects down to
// the C layer. Caller holds mu.
//
// Note there is exactly ONE panel frame. The collapsed and expanded rects are
// panel-LOCAL: the panel itself never moves or resizes between them, which is
// the property the whole rework exists to establish.
func (c *Controller) applyLocked() {
	if !c.started {
		return
	}
	spec := c.specLocked()
	screen := ScreenFrame()
	if screen.W <= 0 || screen.H <= 0 {
		// AppKit has not published a frame: the main thread was still busy
		// launching past the C layer's bounded wait. Place against the
		// fallback now and look again shortly; nothing else re-lays the notch
		// out until an unrelated preference or usage change.
		c.retryLocked()
	}
	panel := PanelRect(screen, spec)
	col := NotchRect(panel, spec, false)
	exp := NotchRect(panel, spec, true)
	wake := WakeRect(panel, spec)
	cs, es := spec.Shape(false), spec.Shape(true)

	C.CCPMNotchSetGeometry(
		C.int(edgeCode(spec.Edge)),
		C.double(panel.X), C.double(panel.Y), C.double(panel.W), C.double(panel.H),
		C.double(col.X), C.double(col.Y), C.double(col.W), C.double(col.H),
		C.double(exp.X), C.double(exp.Y), C.double(exp.W), C.double(exp.H),
		C.double(wake.X), C.double(wake.Y), C.double(wake.W), C.double(wake.H),
		C.double(cs.Corner), C.double(cs.Curl), C.double(es.Corner), C.double(es.Curl))
}

// retryLocked schedules one re-layout, unless one is already pending. Caller
// holds mu. applyLocked is a no-op once stopped, so a retry that fires after
// Stop does nothing and schedules nothing further.
func (c *Controller) retryLocked() {
	if c.retrying {
		return
	}
	c.retrying = true
	time.AfterFunc(250*time.Millisecond, func() {
		c.mu.Lock()
		defer c.mu.Unlock()
		c.retrying = false
		c.applyLocked()
	})
}

// edgeCode is the C-side edge enum. Kept in step with kEdge* in
// render_darwin.m, which uses it to orient the one canonical shape path.
func edgeCode(e Edge) int {
	switch e {
	case EdgeLeft:
		return 1
	case EdgeTop:
		return 2
	case EdgeBottom:
		return 3
	default:
		return 0
	}
}

// ScreenFrame returns the main screen's FULL frame, menu bar and Dock included.
//
// Deliberately not visibleFrame. The rail anchored to the visible frame, which
// meant showing or hiding the Dock slid it along the edge; a notch that moves
// when an unrelated thing appears does not read as part of the bezel.
//
// Reads a cache the main thread refreshes at start and on every screen-
// configuration change. A zero rect means AppKit has not published one yet
// (called before Start, or mid-reconfiguration); PanelRect degrades to a
// placeable default rather than producing a zero-area window.
func ScreenFrame() Rect {
	var x, y, w, h C.double
	C.CCPMNotchScreenFrame(&x, &y, &w, &h)
	return Rect{X: float64(x), Y: float64(y), W: float64(w), H: float64(h)}
}

// HardwareNotch returns the main display's physical notch size, and whether it
// has one. Only meaningful on the top edge; the notch joins to it there, and
// the joined state drops the wake band entirely (see WakeRect).
func HardwareNotch() (Rect, bool) {
	var w, h C.double
	C.CCPMNotchHardwareSize(&w, &h)
	r := Rect{W: float64(w), H: float64(h)}
	return r, r.W > 0 && r.H > 0
}
