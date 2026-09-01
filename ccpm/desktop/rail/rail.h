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

// Moves and resizes the panel, in screen coordinates (origin bottom-left).
void CCPMRailSetFrame(double x, double y, double w, double h);

void CCPMRailShow(void);
void CCPMRailHide(void);

// Reports whether the panel is currently on screen.
int CCPMRailIsVisible(void);

// Writes the main screen's visible frame (excluding menu bar and Dock) into the
// out-params. Reads a cache refreshed on the main thread at start and whenever
// the screen configuration changes, so callers never block on AppKit.
void CCPMRailVisibleFrame(double *x, double *y, double *w, double *h);

#endif
