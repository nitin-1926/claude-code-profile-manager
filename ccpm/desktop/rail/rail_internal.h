// Shared between rail_darwin.m (panel lifecycle) and render_darwin.m (drawing
// and the cursor watcher). Not part of the Go-facing surface — rail.h is that.
#ifndef CCPM_RAIL_INTERNAL_H
#define CCPM_RAIL_INTERNAL_H

#import <Cocoa/Cocoa.h>

// Runs block on the main thread: inline when already there, dispatch_async
// otherwise. AppKit is main-thread-only and Wails runs our Go entry points on a
// background goroutine, so every AppKit touch in this package goes through it.
void ccpmRailOnMain(dispatch_block_t block);

// The live panel, or nil before Start and after Stop.
NSPanel *CCPMNotchPanelRef(void);

// Unpacks a 0xRRGGBB value from the model into an sRGB colour.
NSColor *ccpmRailColor(unsigned int rgb, CGFloat alpha);

// Starts and stops the cursor watcher. Implemented in render_darwin.m and
// called from the panel's lifecycle, because a monitor outliving its panel is a
// retain cycle with a crash at the end of it.
void ccpmNotchStartWatching(void);
void ccpmNotchStopWatching(void);

// Recomputes which regions take the mouse and updates the panel's
// ignoresMouseEvents accordingly. Called on every cursor event and whenever the
// geometry or the expansion state changes.
void ccpmNotchUpdateInteractive(void);

#endif
