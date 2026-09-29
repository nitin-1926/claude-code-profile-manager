//go:build darwin

package rail

import "testing"

// Wails runs OnStartup on a background goroutine while [NSApp run] owns the
// main thread, so the ordering between "the app is up" and "the rail was asked
// to do something" is not guaranteed. Every entry point must therefore tolerate
// being called before Start and after Stop. A rail that appears a moment late
// is a cosmetic bug; one that dereferences a nil panel takes the app down.
//
// These run without a display: the C layer's nil-panel guards return early, and
// the geometry falls back to a placeable default.
func TestControllerCallsAreSafeBeforeStartAndAfterStop(t *testing.T) {
	c := New()

	// Before Start.
	c.SetLayout(EdgeLeft, 3)
	c.SetVisible(true)
	c.SetVisible(false)
	_ = c.Visible()
	_ = c.Frame()
	if got := c.Edge(); got != EdgeLeft {
		t.Errorf("SetLayout before Start did not stick: edge = %q", got)
	}

	// Stop without Start.
	c.Stop()

	// And again after an explicit Stop.
	c.Stop()
	c.SetLayout(EdgeTop, 1)
	c.SetVisible(true)
	_ = c.Frame()
}

func TestControllerDefaults(t *testing.T) {
	c := New()
	if c.Edge() != EdgeRight {
		t.Errorf("default edge = %q, want right", c.Edge())
	}
	// A fresh controller must produce a placeable frame even with no screen
	// information published yet.
	f := c.Frame()
	if f.W <= 0 || f.H <= 0 {
		t.Errorf("default frame is zero-area: %+v", f)
	}
}

func TestSetLayoutRecomputesFrame(t *testing.T) {
	c := New()

	c.SetLayout(EdgeRight, 1)
	one := c.Frame()
	c.SetLayout(EdgeRight, 4)
	four := c.Frame()

	if four.H <= one.H {
		t.Errorf("four profiles (h=%v) should need more height than one (h=%v)", four.H, one.H)
	}

	c.SetLayout(EdgeBottom, 4)
	bottom := c.Frame()
	if bottom.W <= bottom.H {
		t.Errorf("a bottom-edge rail should be wider than it is tall: %+v", bottom)
	}
}

// ScreenFrame reads a cache the main thread publishes. Without a running
// NSApplication it is legitimately zero — the contract is that it never panics
// and never returns garbage, not that it returns a real screen.
func TestScreenFrameIsSafeWithoutAppKitRunning(t *testing.T) {
	v := ScreenFrame()
	if v.W < 0 || v.H < 0 {
		t.Errorf("negative screen frame: %+v", v)
	}
	// Whatever it returns, it must feed PanelRect without producing a
	// zero-area or NaN panel.
	r := PanelRect(v, Spec{Edge: EdgeRight, Profiles: 3})
	if r.W <= 0 || r.H <= 0 {
		t.Errorf("screen frame %+v produced an unplaceable panel %+v", v, r)
	}
}

// HardwareNotch is likewise cache-backed and must be answerable with no AppKit
// running. A display without a notch reports 0x0, which is not an error.
func TestHardwareNotchIsSafeWithoutAppKitRunning(t *testing.T) {
	r, ok := HardwareNotch()
	if r.W < 0 || r.H < 0 {
		t.Errorf("negative hardware notch: %+v", r)
	}
	if ok != (r.W > 0 && r.H > 0) {
		t.Errorf("hardware notch %+v disagrees with its own ok=%v", r, ok)
	}
}
