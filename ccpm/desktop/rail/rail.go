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
	mu       sync.Mutex
	started  bool
	edge     Edge
	profiles int
	visible  bool
	hover    bool
	model    Model
}

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
	C.CCPMRailStart()
	c.started = true
	C.CCPMRailSetHoverMode(cbool(c.hover))
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
	C.CCPMRailStop()
	c.started = false
	c.visible = false
}

// SetLayout updates the edge and profile count, re-placing the panel.
func (c *Controller) SetLayout(edge Edge, profiles int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.edge = edge
	c.profiles = profiles
	c.applyLocked()
}

// SetHoverMode chooses between collapsing to a peek when the pointer leaves and
// staying fully revealed.
func (c *Controller) SetHoverMode(hover bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.hover = hover
	if !c.started {
		return
	}
	C.CCPMRailSetHoverMode(cbool(hover))
}

// SetModel replaces what the rail draws. Held so a later Start can redraw
// without the caller having to remember to push the model again.
func (c *Controller) SetModel(m Model) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.model = m
	c.pushModelLocked()
}

// pushModelLocked hands the model to the renderer. Caller holds mu.
func (c *Controller) pushModelLocked() {
	if !c.started {
		return
	}
	// EndPadding travels with the model so the C layer divides the stack with
	// the same number geometry.go does, instead of keeping its own copy to
	// drift out of step.
	payload := struct {
		Model
		EndPadding float64 `json:"endPadding"`
	}{Model: c.model, EndPadding: EndPadding}

	b, err := json.Marshal(payload)
	if err != nil {
		return // a model that will not marshal is a bug, but not one worth a panic in the UI
	}
	cs := C.CString(string(b))
	defer C.free(unsafe.Pointer(cs))
	C.CCPMRailSetModel(cs)
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
		C.CCPMRailShow()
		return
	}
	C.CCPMRailHide()
}

// Visible reports the panel's actual on-screen state, asked of AppKit rather
// than of our own bookkeeping, so a panel the system dismissed is not reported
// as showing.
func (c *Controller) Visible() bool {
	return C.CCPMRailIsVisible() != 0
}

// Frame returns where the panel would be placed for the current layout.
// Exposed so the callout can anchor to the same geometry the panel uses.
func (c *Controller) Frame() Rect {
	c.mu.Lock()
	defer c.mu.Unlock()
	return PanelRect(VisibleFrame(), c.edge, c.profiles)
}

// Edge returns the current edge.
func (c *Controller) Edge() Edge {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.edge
}

// applyLocked pushes the computed frames down to the panel. Caller holds mu.
func (c *Controller) applyLocked() {
	if !c.started {
		return
	}
	v := VisibleFrame()
	full := PanelRect(v, c.edge, c.profiles)
	peek := PeekRect(v, c.edge, c.profiles)
	C.CCPMRailSetFrames(
		C.double(peek.X), C.double(peek.Y), C.double(peek.W), C.double(peek.H),
		C.double(full.X), C.double(full.Y), C.double(full.W), C.double(full.H))
}

func cbool(b bool) C.int {
	if b {
		return 1
	}
	return 0
}

// VisibleFrame returns the main screen's usable area — the full screen minus
// the menu bar and the Dock, so the rail never tucks underneath either.
//
// Reads a cache the main thread refreshes at start and on every screen-
// configuration change. A zero rect means AppKit has not published one yet
// (called before Start, or mid-reconfiguration); PanelRect degrades to a
// placeable default rather than producing a zero-area window.
func VisibleFrame() Rect {
	var x, y, w, h C.double
	C.CCPMRailVisibleFrame(&x, &y, &w, &h)
	return Rect{X: float64(x), Y: float64(y), W: float64(w), H: float64(h)}
}
