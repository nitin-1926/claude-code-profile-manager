//go:build darwin

#import <Cocoa/Cocoa.h>
#import <os/lock.h>
#import "rail.h"
#import "rail_internal.h"

// Every Objective-C symbol in this file is prefixed CCPMNotch. Wails already
// defines WailsWindow, AppDelegate and WindowDelegate in the same process, and
// duplicate Objective-C class names are a link-time collision, not a warning.

@interface CCPMNotchPanel : NSPanel
@end

@implementation CCPMNotchPanel
// A notch that took key focus would steal the caret out of whatever the user is
// typing in. NonactivatingPanel gets us clicks without activation; these two
// close the door on the panel ever becoming key or main by another route.
- (BOOL)canBecomeKeyWindow {
  return NO;
}
- (BOOL)canBecomeMainWindow {
  return NO;
}
@end

// The panel's content view: a plain, fully transparent container that spans the
// whole panel and never claims a hit for itself.
//
// This is load-bearing. The panel is deliberately much larger than the visible
// notch — it reserves room for the fully expanded shape AND a hover card at
// either end, so that expanding never has to resize the window. That means most
// of the panel is empty space sitting over the user's other windows. A content
// view whose hitTest returns self turns all of it into a wall that swallows
// every click meant for whatever is underneath.
//
// Returning the subview's answer, or nil, is what makes the transparent region
// genuinely transparent to the mouse. The coarse gate is the panel's
// ignoresMouseEvents, recomputed per event in render_darwin.m; this is the fine
// one, for the case where the pointer is inside a live rect but over a part of
// the container that no subview covers.
@interface CCPMNotchContainer : NSView
@end

@implementation CCPMNotchContainer
- (NSView *)hitTest:(NSPoint)point {
  NSView *hit = [super hitTest:point];
  return (hit == self) ? nil : hit;
}
@end

// Watches for displays being attached, detached, or rearranged so the cached
// screen frame never goes stale. Registered additively on NSNotificationCenter
// — deliberately NOT via [NSApp setDelegate:], which would unhook Wails' own
// delegate and break quit handling and single-instance behaviour.
@interface CCPMNotchScreenObserver : NSObject
- (void)screensChanged:(NSNotification *)note;
@end

static CCPMNotchPanel *gPanel = nil;
static CCPMNotchScreenObserver *gObserver = nil;

// The cached screen frame and hardware notch size. AppKit is main-thread-only,
// so instead of making Go block on a main-queue round trip every layout, the
// main thread publishes them here and Go reads them under a lock.
static NSRect gScreen = {{0, 0}, {0, 0}};
static NSSize gHardware = {0, 0};
static os_unfair_lock gScreenLock = OS_UNFAIR_LOCK_INIT;

// Runs block on the main thread. Declared in rail_internal.h; render_darwin.m
// uses it too, so it is deliberately not static.
//
// dispatch_async, never dispatch_sync: a dispatch_sync to the main queue from
// the main thread deadlocks instantly, and this file is called from both.
// The isMainThread check keeps ordering intuitive when we are already there.
void ccpmRailOnMain(dispatch_block_t block) {
  if ([NSThread isMainThread]) {
    block();
  } else {
    dispatch_async(dispatch_get_main_queue(), block);
  }
}

// Must run on the main thread.
//
// Caches screen.frame, NOT visibleFrame. The rail anchored to the visible
// frame, so showing or hiding the Dock slid it along the edge — and a notch
// that moves when an unrelated thing appears stops reading as part of the
// bezel. Go clamps against this frame; the menu bar is not in the way because
// the panel sits at NSStatusWindowLevel, above it.
static void ccpmNotchRefreshScreen(void) {
  NSScreen *screen = [NSScreen mainScreen];
  if (screen == nil) {
    screen = [[NSScreen screens] firstObject];
  }
  if (screen == nil) {
    return; // headless or mid-reconfiguration; keep the last known frame
  }
  NSRect f = [screen frame];

  // The hardware notch is not exposed directly. It is the gap between the two
  // menu-bar strips either side of the cutout. Both APIs are 12.0+; on
  // anything older, and on any display without a notch, the auxiliary areas
  // are nil and the size stays zero.
  //
  // Its depth is the deepest of the top safe-area inset and the two strips,
  // not the inset alone (codenotch 1.18): the inset is the area the system
  // asks apps to keep clear, and it collapses when the menu bar is hidden or
  // auto-hides. The hole does not move when that happens, and a notch sized
  // from the collapsed inset comes out shallower than the cutout it joins — a
  // step along its foot. The strips are the hole's own height either way.
  NSSize hw = NSMakeSize(0, 0);
  if (@available(macOS 12.0, *)) {
    NSRect left = screen.auxiliaryTopLeftArea;
    NSRect right = screen.auxiliaryTopRightArea;
    if (!NSIsEmptyRect(left) && !NSIsEmptyRect(right)) {
      CGFloat w = NSWidth(f) - NSWidth(left) - NSWidth(right);
      CGFloat h = MAX(screen.safeAreaInsets.top, MAX(NSHeight(left), NSHeight(right)));
      if (w > 0 && h > 0) {
        hw = NSMakeSize(w, h);
      }
    }
  }

  os_unfair_lock_lock(&gScreenLock);
  gScreen = f;
  gHardware = hw;
  os_unfair_lock_unlock(&gScreenLock);
}

@implementation CCPMNotchScreenObserver
- (void)screensChanged:(NSNotification *)note {
  (void)note;
  ccpmNotchRefreshScreen();
}
@end

void CCPMNotchStart(void) {
  ccpmRailOnMain(^{
    ccpmNotchRefreshScreen();

    if (gObserver == nil) {
      gObserver = [[CCPMNotchScreenObserver alloc] init];
      [[NSNotificationCenter defaultCenter]
          addObserver:gObserver
             selector:@selector(screensChanged:)
                 name:NSApplicationDidChangeScreenParametersNotification
               object:nil];
    }
    if (gPanel != nil) {
      return;
    }

    NSRect initial = NSMakeRect(0, 0, 320, 480);
    gPanel = [[CCPMNotchPanel alloc]
        initWithContentRect:initial
                  styleMask:(NSWindowStyleMaskBorderless |
                             NSWindowStyleMaskNonactivatingPanel)
                    backing:NSBackingStoreBuffered
                      defer:NO];

    // NSStatusWindowLevel (25), not NSFloatingWindowLevel (3): floating sits
    // BELOW the menu bar (24), which is exactly where a notch needs to be
    // visible.
    gPanel.level = NSStatusWindowLevel;

    // CanJoinAllSpaces  — follows the user between Spaces instead of stranding
    //                     itself on the Space it was born in.
    // Stationary        — does not slide during Space-switch animations.
    // FullScreenAuxiliary — stays visible over another app in fullscreen.
    gPanel.collectionBehavior = NSWindowCollectionBehaviorCanJoinAllSpaces |
                                NSWindowCollectionBehaviorStationary |
                                NSWindowCollectionBehaviorFullScreenAuxiliary;

    // NSPanel defaults hidesOnDeactivate to YES, which would make the notch
    // vanish the moment the user clicked into any other app — i.e. almost
    // always. canHide=NO keeps it alive through [NSApp hide:], which is what
    // Wails' HideWindowOnClose path calls when the main window closes.
    gPanel.hidesOnDeactivate = NO;
    gPanel.canHide = NO;
    gPanel.releasedWhenClosed = NO;
    gPanel.becomesKeyOnlyIfNeeded = YES;
    gPanel.opaque = NO;
    gPanel.backgroundColor = [NSColor clearColor];
    gPanel.movable = NO;
    gPanel.movableByWindowBackground = NO;

    // No window shadow. The panel is mostly empty space, and AppKit draws the
    // shadow around the WINDOW, not around the drawn shape — so a shadow here
    // outlines a large invisible rectangle hanging off the screen edge. The
    // black body is meant to look welded to the bezel and wants none anyway.
    gPanel.hasShadow = NO;

    CCPMNotchContainer *container =
        [[CCPMNotchContainer alloc] initWithFrame:initial];
    container.wantsLayer = YES;
    container.layer.backgroundColor = NSColor.clearColor.CGColor;
    container.autoresizingMask = NSViewWidthSizable | NSViewHeightSizable;
    gPanel.contentView = container;

    // Everything visible is drawn as layers by render_darwin.m, on top of
    // this container: the black body as a CAShapeLayer whose path morphs
    // between the folded and open shapes, the rings clipped by the same path,
    // and the hover card. No NSVisualEffectView — codenotch's notch is solid
    // black, which is what lets it read as part of the bezel rather than as a
    // translucent window laid over the screen.
    ccpmNotchStartWatching();
  });
}

void CCPMNotchStop(void) {
  ccpmRailOnMain(^{
    ccpmNotchStopWatching();
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

// Main thread only, which every caller in render_darwin.m already is.
NSPanel *CCPMNotchPanelRef(void) { return gPanel; }

void CCPMNotchShow(void) {
  ccpmRailOnMain(^{
    if (gPanel == nil) {
      return;
    }
    // orderFrontRegardless, not makeKeyAndOrderFront: the notch must appear
    // without activating ccpm or pulling focus from the user's frontmost app.
    [gPanel orderFrontRegardless];
    ccpmNotchUpdateInteractive();
  });
}

void CCPMNotchHide(void) {
  ccpmRailOnMain(^{
    if (gPanel == nil) {
      return;
    }
    // orderOut, deliberately not alphaValue = 0. An invisible panel that is
    // still on screen keeps taking the mouse over its live rects, so "hidden"
    // would silently go on swallowing clicks at the screen edge.
    [gPanel orderOut:nil];
  });
}

int CCPMNotchIsVisible(void) {
  // Read-only and cheap; safe to answer from any thread without a hop.
  return (gPanel != nil && [gPanel isVisible]) ? 1 : 0;
}

// Warms the cache synchronously the first time, then answers from it.
//
// CCPMNotchStart hands its work to the main queue and returns immediately, so
// Go's very first placement lands here before the main thread has published
// anything. Warming once beats shipping a panel placed against a zero screen.
//
// The wait is bounded, never a bare dispatch_sync. In the app the main thread
// runs [NSApp run] and answers within microseconds; with no run loop servicing
// the main queue (go test, a headless CI runner) a dispatch_sync blocked
// forever. After the timeout the frame stays zero and Go places against its
// fallback screen, as it does for any unpublished frame.
static NSRect ccpmNotchCachedScreen(NSSize *hardware) {
  os_unfair_lock_lock(&gScreenLock);
  NSRect v = gScreen;
  NSSize hw = gHardware;
  os_unfair_lock_unlock(&gScreenLock);

  if (v.size.width <= 0 || v.size.height <= 0) {
    if ([NSThread isMainThread]) {
      ccpmNotchRefreshScreen();
    } else {
      dispatch_semaphore_t done = dispatch_semaphore_create(0);
      dispatch_async(dispatch_get_main_queue(), ^{
        ccpmNotchRefreshScreen();
        dispatch_semaphore_signal(done);
      });
      dispatch_semaphore_wait(
          done, dispatch_time(DISPATCH_TIME_NOW, 250 * NSEC_PER_MSEC));
    }
    os_unfair_lock_lock(&gScreenLock);
    v = gScreen;
    hw = gHardware;
    os_unfair_lock_unlock(&gScreenLock);
  }
  if (hardware) {
    *hardware = hw;
  }
  return v;
}

void CCPMNotchScreenFrame(double *x, double *y, double *w, double *h) {
  NSRect v = ccpmNotchCachedScreen(NULL);
  if (x) *x = v.origin.x;
  if (y) *y = v.origin.y;
  if (w) *w = v.size.width;
  if (h) *h = v.size.height;
}

void CCPMNotchHardwareSize(double *w, double *h) {
  NSSize hw = NSMakeSize(0, 0);
  ccpmNotchCachedScreen(&hw);
  if (w) *w = hw.width;
  if (h) *h = hw.height;
}
