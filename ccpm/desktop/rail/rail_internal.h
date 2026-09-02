// Shared between rail_darwin.m (panel lifecycle) and render_darwin.m (drawing
// and hover). Not part of the Go-facing surface — rail.h is that.
#ifndef CCPM_RAIL_INTERNAL_H
#define CCPM_RAIL_INTERNAL_H

#import <Cocoa/Cocoa.h>

// Runs block on the main thread: inline when already there, dispatch_async
// otherwise. AppKit is main-thread-only and Wails runs our Go entry points on a
// background goroutine, so every AppKit touch in this package goes through it.
void ccpmRailOnMain(dispatch_block_t block);

// The live panel, or nil before Start and after Stop.
NSPanel *CCPMRailPanelRef(void);

// Installs the hover tracking area on the panel's content view.
void CCPMRailUpdateTracking(void);

#endif
