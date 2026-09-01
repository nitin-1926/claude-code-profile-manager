//go:build darwin

#import <Cocoa/Cocoa.h>
#import <os/lock.h>
#import "rail.h"

// Every Objective-C symbol in this file is prefixed CCPMRail. Wails already
// defines WailsWindow, AppDelegate and WindowDelegate in the same process, and
// duplicate Objective-C class names are a link-time collision, not a warning.

@interface CCPMRailPanel : NSPanel
@end

@implementation CCPMRailPanel
// A rail that took key focus would steal the caret out of whatever the user is
// typing in. NonactivatingPanel gets us clicks without activation; these two
// close the door on the panel ever becoming key or main by another route.
- (BOOL)canBecomeKeyWindow {
  return NO;
}
- (BOOL)canBecomeMainWindow {
  return NO;
}
@end

// Watches for displays being attached, detached, or rearranged so the cached
// visible frame never goes stale. Registered additively on NSNotificationCenter
// — deliberately NOT via [NSApp setDelegate:], which would unhook Wails' own
// delegate and break quit handling and single-instance behaviour.
@interface CCPMRailScreenObserver : NSObject
- (void)screensChanged:(NSNotification *)note;
@end

static CCPMRailPanel *gPanel = nil;
static CCPMRailScreenObserver *gObserver = nil;

// The cached visible frame. AppKit is main-thread-only, so instead of making Go
// block on a main-queue round trip every layout, the main thread publishes the
// frame here and Go reads it under a lock.
static NSRect gVisible = {{0, 0}, {0, 0}};
static os_unfair_lock gVisibleLock = OS_UNFAIR_LOCK_INIT;

// Runs block on the main thread.
//
// dispatch_async, never dispatch_sync: a dispatch_sync to the main queue from
// the main thread deadlocks instantly, and this file is called from both.
// The isMainThread check keeps ordering intuitive when we are already there.
static void ccpmRailOnMain(dispatch_block_t block) {
  if ([NSThread isMainThread]) {
    block();
  } else {
    dispatch_async(dispatch_get_main_queue(), block);
  }
}

// Must run on the main thread.
static void ccpmRailRefreshVisible(void) {
  NSScreen *screen = [NSScreen mainScreen];
  if (screen == nil) {
    screen = [[NSScreen screens] firstObject];
  }
  if (screen == nil) {
    return; // headless or mid-reconfiguration; keep the last known frame
  }
  NSRect v = [screen visibleFrame];
  os_unfair_lock_lock(&gVisibleLock);
  gVisible = v;
  os_unfair_lock_unlock(&gVisibleLock);
}

@implementation CCPMRailScreenObserver
- (void)screensChanged:(NSNotification *)note {
  (void)note;
  ccpmRailRefreshVisible();
}
@end

void CCPMRailStart(void) {
  ccpmRailOnMain(^{
    ccpmRailRefreshVisible();

    if (gObserver == nil) {
      gObserver = [[CCPMRailScreenObserver alloc] init];
      [[NSNotificationCenter defaultCenter]
          addObserver:gObserver
             selector:@selector(screensChanged:)
                 name:NSApplicationDidChangeScreenParametersNotification
               object:nil];
    }
    if (gPanel != nil) {
      return;
    }

    NSRect initial = NSMakeRect(0, 0, 64, 240);
    gPanel = [[CCPMRailPanel alloc]
        initWithContentRect:initial
                  styleMask:(NSWindowStyleMaskBorderless |
                             NSWindowStyleMaskNonactivatingPanel)
                    backing:NSBackingStoreBuffered
                      defer:NO];

    // NSStatusWindowLevel (25), not NSFloatingWindowLevel (3): floating sits
    // BELOW the menu bar (24), which is exactly where a notch-adjacent rail
    // needs to be visible.
    gPanel.level = NSStatusWindowLevel;

    // CanJoinAllSpaces  — follows the user between Spaces instead of stranding
    //                     itself on the Space it was born in.
    // Stationary        — does not slide during Space-switch animations.
    // FullScreenAuxiliary — stays visible over another app in fullscreen.
    gPanel.collectionBehavior = NSWindowCollectionBehaviorCanJoinAllSpaces |
                                NSWindowCollectionBehaviorStationary |
                                NSWindowCollectionBehaviorFullScreenAuxiliary;

    // NSPanel defaults hidesOnDeactivate to YES, which would make the rail
    // vanish the moment the user clicked into any other app — i.e. almost
    // always. canHide=NO keeps it alive through [NSApp hide:], which is what
    // Wails' HideWindowOnClose path calls when the main window closes.
    gPanel.hidesOnDeactivate = NO;
    gPanel.canHide = NO;
    gPanel.releasedWhenClosed = NO;
    gPanel.becomesKeyOnlyIfNeeded = YES;
    gPanel.opaque = NO;
    gPanel.backgroundColor = [NSColor clearColor];
    gPanel.hasShadow = YES;

    NSVisualEffectView *fx =
        [[NSVisualEffectView alloc] initWithFrame:initial];
    fx.material = NSVisualEffectMaterialHUDWindow;
    fx.blendingMode = NSVisualEffectBlendingModeBehindWindow;
    fx.state = NSVisualEffectStateActive;
    fx.wantsLayer = YES;
    fx.layer.cornerRadius = 18.0;
    fx.layer.masksToBounds = YES;
    fx.autoresizingMask = NSViewWidthSizable | NSViewHeightSizable;
    gPanel.contentView = fx;
  });
}

void CCPMRailStop(void) {
  ccpmRailOnMain(^{
    if (gObserver != nil) {
      [[NSNotificationCenter defaultCenter] removeObserver:gObserver];
      gObserver = nil;
    }
    if (gPanel == nil) {
      return;
    }
    [gPanel orderOut:nil];
    [gPanel close];
    gPanel = nil;
  });
}

void CCPMRailSetFrame(double x, double y, double w, double h) {
  ccpmRailOnMain(^{
    if (gPanel == nil) {
      return;
    }
    [gPanel setFrame:NSMakeRect(x, y, w, h) display:YES];
  });
}

void CCPMRailShow(void) {
  ccpmRailOnMain(^{
    if (gPanel == nil) {
      return;
    }
    // orderFrontRegardless, not makeKeyAndOrderFront: the rail must appear
    // without activating ccpm or pulling focus from the user's frontmost app.
    [gPanel orderFrontRegardless];
  });
}

void CCPMRailHide(void) {
  ccpmRailOnMain(^{
    if (gPanel == nil) {
      return;
    }
    [gPanel orderOut:nil];
  });
}

int CCPMRailIsVisible(void) {
  // Read-only and cheap; safe to answer from any thread without a hop.
  return (gPanel != nil && [gPanel isVisible]) ? 1 : 0;
}

void CCPMRailVisibleFrame(double *x, double *y, double *w, double *h) {
  os_unfair_lock_lock(&gVisibleLock);
  NSRect v = gVisible;
  os_unfair_lock_unlock(&gVisibleLock);
  if (x) *x = v.origin.x;
  if (y) *y = v.origin.y;
  if (w) *w = v.size.width;
  if (h) *h = v.size.height;
}
