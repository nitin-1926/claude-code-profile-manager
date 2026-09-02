//go:build darwin

#import <Cocoa/Cocoa.h>
#import <QuartzCore/QuartzCore.h>
#import "rail.h"
#import "rail_internal.h"

// Rendering for the rail: one stack of rings, drawn with CAShapeLayer arcs over
// the panel's NSVisualEffectView. Layers rather than drawRect: because the
// hover reveal animates the panel's frame, and Core Animation reshapes stroked
// arcs on the render server instead of asking the main thread to repaint every
// frame.
//
// The model arrives from Go as JSON. That is deliberate: the shape has strings
// and nesting, and NSJSONSerialization decodes it in three lines, where
// hand-marshalling a C struct array of char* means allocation and free calls on
// both sides of cgo for no benefit.

// Ring metrics, in points. Sized against the 64pt panel thickness and the 76pt
// slot from geometry.go.
static const CGFloat kOuterRadius = 21.0;
static const CGFloat kOuterWidth = 4.0;
static const CGFloat kInnerRadius = 13.0;
static const CGFloat kInnerWidth = 3.0;
static const CGFloat kPercentSize = 12.0;
static const CGFloat kNameSize = 9.0;

// How long the reveal takes. Short enough to feel immediate, long enough to
// read as a movement rather than a jump.
static const NSTimeInterval kRevealDuration = 0.18;

// Frames the hover reveal animates between, published by Go. Both are held here
// so the mouse handlers can animate without a round trip back into Go — the
// pointer is already moving and a cgo hop per event is latency for nothing.
static NSRect gPeekFrame = {{0, 0}, {0, 0}};
static NSRect gFullFrame = {{0, 0}, {0, 0}};
static CGFloat gEndPadding = 0;
static BOOL gHoverMode = NO;   // collapse when the pointer leaves
static BOOL gExpanded = NO;
static NSTrackingArea *gTracking = nil;
static CALayer *gStack = nil;  // holds every ring layer; replaced wholesale

// The parsed model, kept so a hover can answer from memory. The pointer is
// already moving when the callout is needed; re-reading three profiles' limit
// files at that moment would put disk I/O on the hover path for data we have.
static NSArray *gSlots = nil;
static NSArray *gAnchors = nil;
static NSDictionary *gTheme = nil;
static NSInteger gHovered = -1;

// Owns the mouse-entered callback. A separate object rather than a subclassed
// view so the NSVisualEffectView stays exactly what AppKit gave us.
//
// Entering is tracked with an NSTrackingArea; LEAVING deliberately is not.
// Revealing resizes the panel under a stationary pointer, and AppKit answers
// that by re-evaluating the tracking area and sending exited/entered churn.
// Wired to mouseExited, the rail expands a few points, is told the pointer
// left, collapses, and settles into a stutter just wider than the peek —
// measured at 15pt against a 64pt target before this was changed.
//
// So the exit condition is polled from the pointer's actual location instead,
// which has no such feedback loop: the monitor runs only while revealed, and
// asks the one question that matters — is the pointer still over the rail.
@interface CCPMRailTracker : NSObject
@end

static id gMouseMonitor = nil;

// Defined below, next to the hit-testing it depends on; forward-declared
// because the pointer-leave watch installed here also drives it.
static void ccpmRailSyncCallout(void);

// Stops watching for the pointer to leave.
static void ccpmRailEndWatch(void) {
  if (gMouseMonitor != nil) {
    [NSEvent removeMonitor:gMouseMonitor];
    gMouseMonitor = nil;
  }
}

// Watches for the pointer to leave the revealed panel, then collapses.
//
// A global monitor sees movement over other applications, which is exactly the
// case that matters: the pointer usually leaves the rail by moving onto
// whatever is underneath it. Mouse-movement monitors need no accessibility
// grant — only keyboard ones do.
static void ccpmRailBeginWatch(void) {
  if (gMouseMonitor != nil) {
    return;
  }
  gMouseMonitor = [NSEvent addGlobalMonitorForEventsMatchingMask:
                               (NSEventMaskMouseMoved | NSEventMaskLeftMouseDragged)
                                                        handler:^(NSEvent *e) {
    (void)e;
    if (!gHoverMode || !gExpanded) {
      ccpmRailEndWatch();
      return;
    }
    if (!NSPointInRect([NSEvent mouseLocation], gFullFrame)) {
      CCPMRailSetReveal(0);
      return;
    }
    // Global monitors see movement the tracking area does not, and the pointer
    // can cross between rings without AppKit sending us a mouseMoved.
    ccpmRailSyncCallout();
  }];
}

// Which ring the pointer is over, or -1. Divides the stack the same way the
// renderer lays it out, and index 0 is the TOP ring because macOS y grows
// upward — matching SlotRect in geometry.go.
static NSInteger ccpmRailSlotAt(NSPoint screenPoint) {
  NSUInteger n = gSlots.count;
  if (n == 0 || !NSPointInRect(screenPoint, gFullFrame)) {
    return -1;
  }
  CGFloat pad = gEndPadding;
  CGFloat usable = NSHeight(gFullFrame) - 2 * pad;
  if (usable <= 0) {
    return -1;
  }
  CGFloat fromTop = NSMaxY(gFullFrame) - pad - screenPoint.y;
  if (fromTop < 0 || fromTop >= usable) {
    return -1;
  }
  NSInteger i = (NSInteger)(fromTop / (usable / n));
  return (i < 0 || i >= (NSInteger)n) ? -1 : i;
}

// Shows, moves, or hides the callout for wherever the pointer currently is.
static void ccpmRailSyncCallout(void) {
  NSInteger i = ccpmRailSlotAt([NSEvent mouseLocation]);
  // Collapsed, there is nothing to point at.
  if (gHoverMode && !gExpanded) {
    i = -1;
  }
  if (i == gHovered) {
    return; // no churn while the pointer moves within one ring
  }
  gHovered = i;
  if (i < 0 || i >= (NSInteger)gAnchors.count) {
    ccpmCalloutHide();
    return;
  }
  NSDictionary *a = gAnchors[i];
  ccpmCalloutShow(gSlots[i][@"callout"], gTheme,
                  [a[@"x"] doubleValue], [a[@"y"] doubleValue], a[@"grows"]);
}

@implementation CCPMRailTracker
- (void)mouseEntered:(NSEvent *)event {
  (void)event;
  CCPMRailSetReveal(1);
}
- (void)mouseMoved:(NSEvent *)event {
  (void)event;
  ccpmRailSyncCallout();
}
@end

static CCPMRailTracker *gTracker = nil;

static CCPMRailTracker *CCPMRailTrackerRef(void) {
  if (gTracker == nil) {
    gTracker = [[CCPMRailTracker alloc] init];
  }
  return gTracker;
}

NSColor *ccpmRailColor(unsigned int rgb, CGFloat alpha) {
  return [NSColor colorWithSRGBRed:((rgb >> 16) & 0xFF) / 255.0
                             green:((rgb >> 8) & 0xFF) / 255.0
                              blue:(rgb & 0xFF) / 255.0
                             alpha:alpha];
}

// Reduce Motion is an accessibility setting, not a preference we get to weigh
// against how nice the animation looks. When it is on, every reveal is a jump.
static BOOL ccpmRailReduceMotion(void) {
  NSWorkspace *ws = [NSWorkspace sharedWorkspace];
  if ([ws respondsToSelector:@selector(accessibilityDisplayShouldReduceMotion)]) {
    return [ws accessibilityDisplayShouldReduceMotion];
  }
  return NO;
}

// A stroked arc starting at 12 o'clock and sweeping clockwise. fraction is
// 0..1; strokeEnd does the sweeping so the same path serves track and fill.
static CAShapeLayer *ccpmRailArc(CGPoint center, CGFloat radius, CGFloat width,
                                 NSColor *color, CGFloat fraction, CGFloat alpha) {
  CGMutablePathRef path = CGPathCreateMutable();
  // -M_PI_2 is 12 o'clock; clockwise=true in a bottom-left origin space sweeps
  // the way a clock hand does on screen.
  CGPathAddArc(path, NULL, center.x, center.y, radius, -M_PI_2, -M_PI_2 - 2 * M_PI, true);

  CAShapeLayer *l = [CAShapeLayer layer];
  l.path = path;
  CGPathRelease(path);
  l.fillColor = NULL;
  l.strokeColor = color.CGColor;
  l.lineWidth = width;
  l.lineCap = kCALineCapRound;
  l.strokeStart = 0.0;
  l.strokeEnd = fraction;
  l.opacity = alpha;
  return l;
}

static CATextLayer *ccpmRailText(NSString *s, CGRect frame, CGFloat size,
                                 NSColor *color, NSFontWeight weight) {
  CATextLayer *t = [CATextLayer layer];
  NSFont *font = [NSFont systemFontOfSize:size weight:weight];
  t.string = s;
  t.font = (__bridge CFTypeRef)font;
  t.fontSize = size;
  t.foregroundColor = color.CGColor;
  t.alignmentMode = kCAAlignmentCenter;
  t.truncationMode = kCATruncationEnd;
  t.frame = frame;
  t.contentsScale = [NSScreen mainScreen].backingScaleFactor ?: 2.0;
  return t;
}

// Builds one profile's rings into a layer occupying slot.
static CALayer *ccpmRailSlotLayer(NSDictionary *slot, NSDictionary *theme, CGRect frame) {
  CALayer *l = [CALayer layer];
  l.frame = frame;

  NSColor *track = ccpmRailColor([theme[@"track"] unsignedIntValue], 1.0);
  NSColor *fg = ccpmRailColor([theme[@"foreground"] unsignedIntValue], 1.0);
  NSColor *muted = ccpmRailColor([theme[@"mutedForeground"] unsignedIntValue], 1.0);

  BOOL available = [slot[@"available"] boolValue];
  CGFloat outer = [slot[@"outer"] doubleValue];
  CGFloat inner = [slot[@"inner"] doubleValue];

  // The ring sits in the upper part of the slot; the profile name goes under it.
  CGFloat nameH = kNameSize + 4;
  CGPoint c = CGPointMake(CGRectGetWidth(frame) / 2,
                          nameH + (CGRectGetHeight(frame) - nameH) / 2);

  [l addSublayer:ccpmRailArc(c, kOuterRadius, kOuterWidth, track, 1.0, 1.0)];
  [l addSublayer:ccpmRailArc(c, kInnerRadius, kInnerWidth, track, 1.0, 0.75)];

  // An unavailable profile gets track only. Never a 0% arc: an empty ring reads
  // as "plenty of headroom" when the truth is "we have no reading".
  if (available) {
    [l addSublayer:ccpmRailArc(c, kOuterRadius, kOuterWidth,
                               ccpmRailColor([slot[@"outerRGB"] unsignedIntValue], 1.0),
                               outer, 1.0)];
    [l addSublayer:ccpmRailArc(c, kInnerRadius, kInnerWidth,
                               ccpmRailColor([slot[@"innerRGB"] unsignedIntValue], 1.0),
                               inner, 0.9)];
  }

  CGRect pct = CGRectMake(0, c.y - kPercentSize * 0.72, CGRectGetWidth(frame), kPercentSize * 1.4);
  [l addSublayer:ccpmRailText(slot[@"percent"], pct, kPercentSize,
                              available ? fg : muted, NSFontWeightMedium)];

  CGRect name = CGRectMake(2, 1, CGRectGetWidth(frame) - 4, nameH);
  [l addSublayer:ccpmRailText(slot[@"profile"], name, kNameSize, muted, NSFontWeightRegular)];
  return l;
}

void CCPMRailSetModel(const char *json) {
  if (json == NULL) {
    return;
  }
  // Copy before the hop: Go frees the C string as soon as this returns, and the
  // block runs later.
  NSData *data = [[NSString stringWithUTF8String:json] dataUsingEncoding:NSUTF8StringEncoding];

  ccpmRailOnMain(^{
    NSPanel *panel = CCPMRailPanelRef();
    if (panel == nil) {
      return;
    }
    NSDictionary *model = [NSJSONSerialization JSONObjectWithData:data options:0 error:NULL];
    if (![model isKindOfClass:[NSDictionary class]]) {
      return;
    }
    NSArray *slots = model[@"slots"];
    NSDictionary *theme = model[@"theme"];
    if (![slots isKindOfClass:[NSArray class]] || ![theme isKindOfClass:[NSDictionary class]]) {
      return;
    }
    gSlots = slots;
    gTheme = theme;
    gAnchors = [model[@"anchors"] isKindOfClass:[NSArray class]] ? model[@"anchors"] : @[];
    gEndPadding = [model[@"endPadding"] doubleValue];
    // The stack was rebuilt underneath whatever was hovered, so re-resolve
    // rather than leaving a callout describing a ring that has moved.
    gHovered = -1;

    NSView *content = panel.contentView;
    content.wantsLayer = YES;

    [gStack removeFromSuperlayer];
    gStack = [CALayer layer];
    gStack.frame = content.bounds;
    gStack.autoresizingMask = kCALayerWidthSizable | kCALayerHeightSizable;

    NSUInteger n = slots.count;
    if (n > 0) {
      CGFloat pad = gEndPadding;
      CGFloat slotH = (CGRectGetHeight(content.bounds) - 2 * pad) / n;
      for (NSUInteger i = 0; i < n; i++) {
        // Index 0 is the TOP ring, and macOS y grows upward — mirroring
        // SlotRect in geometry.go, which the hit-testing also follows.
        CGFloat y = CGRectGetHeight(content.bounds) - pad - slotH * (i + 1);
        CGRect frame = CGRectMake(0, y, CGRectGetWidth(content.bounds), slotH);
        [gStack addSublayer:ccpmRailSlotLayer(slots[i], theme, frame)];
      }
    }
    [content.layer addSublayer:gStack];

    // Collapsed, the rings are off the visible sliver; fading the stack keeps
    // the peek reading as a tab rather than as a clipped window.
    gStack.opacity = (gHoverMode && !gExpanded) ? 0.0 : 1.0;
  });
}

void CCPMRailSetFrames(double px, double py, double pw, double ph,
                       double fx, double fy, double fw, double fh) {
  ccpmRailOnMain(^{
    gPeekFrame = NSMakeRect(px, py, pw, ph);
    gFullFrame = NSMakeRect(fx, fy, fw, fh);
    NSPanel *panel = CCPMRailPanelRef();
    if (panel == nil) {
      return;
    }
    [panel setFrame:(gHoverMode && !gExpanded) ? gPeekFrame : gFullFrame display:YES];
  });
}

void CCPMRailSetHoverMode(int hover) {
  ccpmRailOnMain(^{
    gHoverMode = hover ? YES : NO;
    if (!gHoverMode) {
      // Leaving hover mode means "always visible", so stop being collapsed.
      gExpanded = YES;
    } else {
      // Entering it means collapse — unless the pointer is already over the
      // rail, in which case no mouseEntered is coming to re-reveal it.
      gExpanded = !NSIsEmptyRect(gFullFrame) &&
                  NSPointInRect([NSEvent mouseLocation], gFullFrame);
    }
    CCPMRailSetReveal(gExpanded ? 1 : 0);
  });
}

void CCPMRailSetReveal(int expanded) {
  ccpmRailOnMain(^{
    NSPanel *panel = CCPMRailPanelRef();
    if (panel == nil || NSIsEmptyRect(gFullFrame)) {
      return;
    }
    gExpanded = expanded ? YES : NO;
    NSRect target = (gHoverMode && !gExpanded) ? gPeekFrame : gFullFrame;
    CGFloat opacity = (gHoverMode && !gExpanded) ? 0.0 : 1.0;

    if (gHoverMode && gExpanded) {
      ccpmRailBeginWatch();
    } else {
      ccpmRailEndWatch();
    }
    if (!gExpanded) {
      gHovered = -1;
      ccpmCalloutHide();
    }

    if (ccpmRailReduceMotion()) {
      [panel setFrame:target display:YES];
      gStack.opacity = opacity;
      return;
    }
    [NSAnimationContext runAnimationGroup:^(NSAnimationContext *ctx) {
      ctx.duration = kRevealDuration;
      ctx.timingFunction =
          [CAMediaTimingFunction functionWithName:kCAMediaTimingFunctionEaseOut];
      [panel.animator setFrame:target display:YES];
      gStack.opacity = opacity;
    } completionHandler:nil];
  });
}

// Tracking lives on the content view and is rebuilt whenever the panel resizes,
// because an NSTrackingArea's rect does not follow its view.
//
// NSTrackingActiveAlways, not ActiveInKeyWindow: the rail is a nonactivating
// panel that is never key, so ActiveInKeyWindow would mean it never tracks.
void CCPMRailUpdateTracking(void) {
  ccpmRailOnMain(^{
    NSPanel *panel = CCPMRailPanelRef();
    if (panel == nil) {
      return;
    }
    NSView *content = panel.contentView;
    if (gTracking != nil) {
      [content removeTrackingArea:gTracking];
      gTracking = nil;
    }
    gTracking = [[NSTrackingArea alloc]
        initWithRect:content.bounds
             options:(NSTrackingMouseEnteredAndExited | NSTrackingMouseMoved |
                      NSTrackingActiveAlways | NSTrackingInVisibleRect)
               owner:CCPMRailTrackerRef()
            userInfo:nil];
    [content addTrackingArea:gTracking];
  });
}
