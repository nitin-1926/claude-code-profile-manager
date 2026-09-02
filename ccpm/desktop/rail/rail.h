// C surface for the floating usage rail panel.
//
// Every function here is safe to call from any goroutine: each hops to the main
// queue internally, because AppKit is main-thread-only and Wails runs our
// OnStartup on a background goroutine while [NSApp run] owns the locked main
// thread. Calls are also safe before Start and after Stop — startup ordering is
// not guaranteed, so a no-op beats a nil dereference.
#ifndef CCPM_RAIL_H
#define CCPM_RAIL_H

// Creates the panel if it does not exist. Idempotent.
void CCPMRailStart(void);

// Destroys the panel. Idempotent.
void CCPMRailStop(void);

// Publishes the two frames the hover reveal moves between, in screen
// coordinates (origin bottom-left): the collapsed peek and the full panel.
// Both live on the C side so a mouse-enter can animate without a cgo hop.
void CCPMRailSetFrames(double px, double py, double pw, double ph,
                       double fx, double fy, double fw, double fh);

// Replaces the drawn contents. json is the rail.Model encoding: theme tokens
// plus one entry per profile ring.
void CCPMRailSetModel(const char *json);

// hover != 0 means collapse to the peek when the pointer leaves. Off means the
// rail stays fully revealed.
void CCPMRailSetHoverMode(int hover);

// Expands or collapses now, animating unless Reduce Motion is on.
void CCPMRailSetReveal(int expanded);

void CCPMRailShow(void);
void CCPMRailHide(void);

// Reports whether the panel is currently on screen.
int CCPMRailIsVisible(void);

// Writes the main screen's visible frame (excluding menu bar and Dock) into the
// out-params. Reads a cache refreshed on the main thread at start and whenever
// the screen configuration changes, so callers never block on AppKit.
void CCPMRailVisibleFrame(double *x, double *y, double *w, double *h);

#endif
