// C surface for the floating usage notch.
//
// Every function here is safe to call from any goroutine: each hops to the main
// queue internally, because AppKit is main-thread-only and Wails runs our
// OnStartup on a background goroutine while [NSApp run] owns the locked main
// thread. Calls are also safe before Start and after Stop — startup ordering is
// not guaranteed, so a no-op beats a nil dereference.
//
// The division of labour changed with the notch rework, and the split is the
// point: GO OWNS ALL GEOMETRY. Every rectangle below is computed and tested in
// notch.go, where it can be asserted without a display. The C side draws what
// it is given and reports where the pointer is; it derives nothing. The parked
// rail had the slot division implemented twice — once in geometry.go and once
// in render_darwin.m — and keeping the two in step by comment was exactly as
// reliable as it sounds.
#ifndef CCPM_RAIL_H
#define CCPM_RAIL_H

// Creates the panel if it does not exist. Idempotent.
void CCPMNotchStart(void);

// Destroys the panel. Idempotent.
void CCPMNotchStop(void);

// Publishes the panel frame (screen coordinates) and the three panel-local
// rectangles the interaction runs on.
//
// The panel frame is FIXED for a given screen, edge and profile count — it does
// NOT change when the notch expands. That is what stops an expansion from
// moving the hit region out from under the pointer that triggered it. See the
// header of notch.go for why the previous two-frame model could not be made to
// work.
//
// collapsed and expanded are the drawn shape's bounds in each state; wake is
// the region that opens the notch while collapsed, and is guaranteed by
// TestWakeIsContainedInExpanded to lie inside expanded. The corner and curl
// pairs are each state's resolved inner-corner radius and bezel shoulder, from
// Spec.Shape — the renderer builds the path from them and resolves nothing.
// edge orients the path: 0 right, 1 left, 2 top, 3 bottom.
void CCPMNotchSetGeometry(int edge,
                          double panelX, double panelY, double panelW, double panelH,
                          double colX, double colY, double colW, double colH,
                          double expX, double expY, double expW, double expH,
                          double wakeX, double wakeY, double wakeW, double wakeH,
                          double colCorner, double colCurl, double expCorner, double expCurl);

// Replaces the drawn contents. json is the rail.Model encoding: theme tokens,
// one entry per profile ring, and the panel-local slot and card rectangles Go
// computed for them.
void CCPMNotchSetModel(const char *json);

// visibility: 0 reveal on hover, 1 always expanded, 2 hidden — rail.Visibility.
void CCPMNotchSetVisibility(int mode);

// Expands or collapses now, animating unless Reduce Motion is on. Used by the
// always-on mode and by a peek; ordinary hover drives itself from the cursor
// watcher inside the C layer, because a cgo hop per mouse-move is latency for
// a question that can be answered from published geometry.
void CCPMNotchSetExpanded(int expanded);

void CCPMNotchShow(void);
void CCPMNotchHide(void);

// Writes the main screen's FULL frame into the out-params — deliberately not
// visibleFrame. Anchoring to the visible frame moves the notch whenever the
// Dock is shown, hidden or repositioned, and a notch that drifts does not read
// as part of the bezel.
//
// Reads a cache refreshed on the main thread at start and whenever the screen
// configuration changes, so callers never block on AppKit.
void CCPMNotchScreenFrame(double *x, double *y, double *w, double *h);

// Writes the main screen's hardware notch size into the out-params, or 0x0 when
// the display has none. Measured from the two menu-bar strips either side of
// the cutout.
void CCPMNotchHardwareSize(double *w, double *h);

#endif
