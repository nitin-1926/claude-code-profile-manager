//go:build darwin

#import <Cocoa/Cocoa.h>
#import <QuartzCore/QuartzCore.h>
#import "rail.h"
#import "rail_internal.h"

// Drawing and interaction for the notch.
//
// The look is codenotch's (MIT, github.com/vinzdg/codenotch): a solid black
// body with concave shoulders where it meets the bezel, so it reads as part of
// the hardware; 44pt rings with the five-hour arc running down the middle of a
// soft track and a thin seven-day ring inside it; a black card with a tail
// that points at the ring it describes; spring motion throughout.
//
// EVERY rectangle and radius here arrives from Go (notch.go), where it is
// tested. The parked rail divided the ring stack twice — once in Go and once in
// this file — and kept the two in step by comment. This file draws what it is
// given and reports where the pointer is; it derives no geometry of its own.
// The one piece of geometry that lives here is the path construction, because
// CGPath is the only thing that can hold it, and its inputs are still Go's.

// ---- Palette (codenotch's dark values; the body is black whatever the theme)

static NSColor *ccpmTextPrimary(void) { return [NSColor whiteColor]; }
static NSColor *ccpmTextSecondary(void) {
  return [NSColor colorWithSRGBRed:0x80 / 255.0 green:0x80 / 255.0 blue:0x80 / 255.0 alpha:1];
}
// Track alphas are chosen to composite to the reference's #303030 / #2D2D2D
// over black.
static NSColor *ccpmRingTrack(void) { return [NSColor colorWithWhite:1 alpha:0.188]; }
static NSColor *ccpmBarTrack(void) { return [NSColor colorWithWhite:1 alpha:0.176]; }

// ---- Measurements that are drawing-only (font sizes, strokes). Layout is Go's.

static const CGFloat kScale = 44.0 / 117.0;
static inline CGFloat px(CGFloat v) { return v * kScale; }
static inline CGFloat capFont(CGFloat capPx) { return px(capPx) / 0.714; }

static const CGFloat kRingDiameter = 44.0;
#define kTrackStroke px(15.5)
#define kProgressStroke px(8)
#define kWeeklyStroke px(5)
#define kWeeklyRadius px(28)
#define kPercentSize capFont(27)
#define kPercentAcrossSize capFont(47)
#define kCardTitleSize capFont(26)
#define kCardBodySize capFont(18)
#define kCardCorner px(49.5)
#define kCardPadding px(32)
#define kBarHeight px(10.5)
#define kHeaderToBlock px(21)
#define kLabelToBar px(16.8)
#define kBarToUsed px(17.8)
#define kBlockSpacing px(20)
// How far a cell slides toward the bezel as it folds away, so the closing
// outline appears to swallow it.
#define kFoldSlide px(28)

// ---- Motion (codenotch's NotchMotion vocabulary)
//
// SwiftUI's spring(response:dampingFraction:) mapped onto CASpringAnimation's
// physical parameters for a unit mass: stiffness = (2*pi/response)^2 and
// damping = 4*pi*dampingFraction/response.
typedef struct {
  CGFloat response, damping;
} Spring;
// The fold is heavier and looser than codenotch's first 0.42/0.78, which read
// as a switch being thrown: slow enough to have mass, damped just under the
// point where it would stop dead, so it settles rather than arrives. The cells
// stay a little quicker so the readings keep up with the black.
static const Spring kUnfold = {0.62, 0.72};   // the shape opening and closing
static const Spring kContents = {0.48, 0.80}; // cells arriving
static const Spring kGlide = {0.50, 0.86};    // the card moving between rings
static const Spring kReading = {0.90, 0.90};  // a percentage changing
static const NSTimeInterval kCrossfade = 0.16;

// Grace periods, deliberately asymmetric. Opening is instant; closing waits,
// because the pointer has to cross the gap between the notch and a card, and
// folding waits longest because it is the biggest movement.
static const NSTimeInterval kCardGrace = 0.25;
static const NSTimeInterval kFoldGrace = 0.45;

// How often the cursor is polled in addition to the event monitors. Not
// redundancy: a pointer that does not move produces no events, so a notch that
// appears or is re-placed under a parked pointer would otherwise keep stale
// hover state forever.
static const NSTimeInterval kCursorPoll = 0.3;

enum { kVisHover = 0, kVisAlways = 1, kVisHidden = 2 };
enum { kEdgeRight = 0, kEdgeLeft = 1, kEdgeTop = 2, kEdgeBottom = 3 };

// ---- State. gPanelFrame is screen space; every other rect is PANEL-LOCAL.
//
// gPanelFrame does not change when the notch expands. The previous
// implementation animated the panel between a peek frame and a full frame,
// which moved the hit region out from under the pointer that had triggered the
// reveal. Here only the path inside the panel morphs.

static int gEdge = kEdgeRight;
static NSRect gPanelFrame = {{0, 0}, {0, 0}};
static NSRect gCollapsed = {{0, 0}, {0, 0}};
static NSRect gExpandedRect = {{0, 0}, {0, 0}};
static NSRect gWake = {{0, 0}, {0, 0}};
static CGFloat gColCorner = 0, gColCurl = 0, gExpCorner = 0, gExpCurl = 0;

static BOOL gExpanded = NO;
static int gVisibility = kVisHover;
static NSInteger gHovered = -1;

static NSArray *gSlots = nil; // model: one per profile
static NSArray *gCells = nil; // Go's layout: ring, label, slot, card, body, tail

static CAShapeLayer *gBody = nil;  // the black shape
static CAShapeLayer *gClip = nil;  // the same path, masking gContent
static CALayer *gContent = nil;    // holds the cell layers
static NSMutableArray *gCellLayers = nil;
static CALayer *gCard = nil;
static NSInteger gCardFor = -1;

// Last drawn arc per profile, so a refresh animates the change instead of
// snapping — codenotch's "reading" motion.
static NSMutableDictionary *gLastFraction = nil;

static NSMutableArray *gMonitors = nil;
static NSTimer *gPollTimer = nil;
static dispatch_block_t gFoldWork = nil;
static dispatch_block_t gCardWork = nil;

static void ccpmNotchSetExpandedInternal(BOOL wanted);
static void ccpmNotchSyncCard(void);
static void ccpmNotchShowCard(NSInteger want, BOOL animated);

NSColor *ccpmRailColor(unsigned int rgb, CGFloat alpha) {
  return [NSColor colorWithSRGBRed:((rgb >> 16) & 0xFF) / 255.0
                             green:((rgb >> 8) & 0xFF) / 255.0
                              blue:(rgb & 0xFF) / 255.0
                             alpha:alpha];
}

// Reduce Motion is an accessibility setting, not a preference to weigh against
// how nice the animation looks. When it is on, every change is a jump.
static BOOL ccpmReduceMotion(void) {
  NSWorkspace *ws = [NSWorkspace sharedWorkspace];
  if ([ws respondsToSelector:@selector(accessibilityDisplayShouldReduceMotion)]) {
    return [ws accessibilityDisplayShouldReduceMotion];
  }
  return NO;
}

static CASpringAnimation *ccpmSpring(NSString *keyPath, Spring s) {
  CASpringAnimation *a = [CASpringAnimation animationWithKeyPath:keyPath];
  a.mass = 1;
  a.stiffness = pow(2 * M_PI / s.response, 2);
  a.damping = 4 * M_PI * s.damping / s.response;
  a.initialVelocity = 0;
  a.duration = a.settlingDuration;
  return a;
}

// Sets a layer property, animating it with a spring from its CURRENT on-screen
// value. Reading the presentation layer is what lets a spring interrupted
// mid-flight carry on from where it visibly is rather than jumping back.
static void ccpmAnimate(CALayer *layer, NSString *keyPath, id to, Spring s, CFTimeInterval delay) {
  id from = [(layer.presentationLayer ?: layer) valueForKeyPath:keyPath];
  [CATransaction begin];
  [CATransaction setDisableActions:YES];
  [layer setValue:to forKeyPath:keyPath];
  [CATransaction commit];
  if (ccpmReduceMotion() || from == nil) {
    return;
  }
  CASpringAnimation *a = ccpmSpring(keyPath, s);
  a.fromValue = from;
  a.toValue = to;
  if (delay > 0) {
    a.beginTime = CACurrentMediaTime() + delay;
    a.fillMode = kCAFillModeBackwards;
  }
  [layer addAnimation:a forKey:keyPath];
}

static NSRect ccpmRect(NSDictionary *d) {
  if (![d isKindOfClass:[NSDictionary class]]) {
    return NSZeroRect;
  }
  return NSMakeRect([d[@"X"] doubleValue], [d[@"Y"] doubleValue], [d[@"W"] doubleValue],
                    [d[@"H"] doubleValue]);
}

static NSRect ccpmCellRect(NSInteger i, NSString *key) {
  if (![gCells isKindOfClass:[NSArray class]] || i < 0 || i >= (NSInteger)gCells.count) {
    return NSZeroRect;
  }
  NSDictionary *c = gCells[i];
  return [c isKindOfClass:[NSDictionary class]] ? ccpmRect(c[key]) : NSZeroRect;
}

// ---- The shape path

// Appends one quarter-circle as a single cubic Bezier.
//
// Hand-built rather than CGPathAddArc on purpose. The open and folded shapes
// are morphed by animating the path, and Core Animation interpolates paths
// element by element: if CGPathAddArc splits one state's arc into two curves
// and not the other's, the morph jumps instead of flowing. A fixed element
// count per quarter makes the two paths structurally identical by
// construction. The current point must already be on the arc's start.
static void ccpmQuarter(CGMutablePathRef p, CGFloat cx, CGFloat cy, CGFloat r, CGFloat a0,
                        CGFloat a1) {
  const CGFloat k = 0.5522847498; // 4/3 * tan(pi/8)
  CGFloat dir = (a1 > a0) ? 1 : -1;
  CGFloat x0 = cx + r * cos(a0), y0 = cy + r * sin(a0);
  CGFloat x3 = cx + r * cos(a1), y3 = cy + r * sin(a1);
  CGFloat x1 = x0 + k * r * dir * -sin(a0), y1 = y0 + k * r * dir * cos(a0);
  CGFloat x2 = x3 - k * r * dir * -sin(a1), y2 = y3 - k * r * dir * cos(a1);
  CGPathAddCurveToPoint(p, NULL, x1, y1, x2, y2, x3, y3);
}

// The notch outline for rect r, with the given inner-corner radius and bezel
// shoulder, oriented for the current edge.
//
// Built once in a canonical space — d runs from the inner face (0) to the
// bezel (D), s runs along the edge (0..L) — and then transformed, exactly as
// the reference does, so all four edges share one path and cannot disagree.
// The shape is symmetric in s, so the direction s runs does not matter.
//
// Traced: bezel corner -> concave shoulder -> along the body's end -> convex
// inner corner -> down the inner face -> convex corner -> back along the other
// end -> concave shoulder -> close along the bezel.
static CGPathRef ccpmNotchPath(NSRect r, CGFloat corner, CGFloat curl, int edge) {
  BOOL vertical = (edge == kEdgeRight || edge == kEdgeLeft);
  CGFloat D = vertical ? NSWidth(r) : NSHeight(r);
  CGFloat L = vertical ? NSHeight(r) : NSWidth(r);
  CGFloat c = MAX(0.01, curl), k = MAX(0.01, corner);

  CGMutablePathRef p = CGPathCreateMutable();
  CGPathMoveToPoint(p, NULL, D, 0);
  ccpmQuarter(p, D - c, 0, c, 0, M_PI_2);
  CGPathAddLineToPoint(p, NULL, k, c);
  ccpmQuarter(p, k, c + k, k, 3 * M_PI_2, M_PI);
  CGPathAddLineToPoint(p, NULL, 0, L - c - k);
  ccpmQuarter(p, k, L - c - k, k, M_PI, M_PI_2);
  CGPathAddLineToPoint(p, NULL, D - c, L - c);
  ccpmQuarter(p, D - c, L, c, 3 * M_PI_2, 2 * M_PI);
  CGPathCloseSubpath(p);

  // (d, s) -> panel-local. x' = a*d + c*s + tx, y' = b*d + dd*s + ty.
  CGAffineTransform t;
  switch (edge) {
  case kEdgeLeft:
    t = CGAffineTransformMake(-1, 0, 0, 1, NSMinX(r) + D, NSMinY(r));
    break;
  case kEdgeTop:
    t = CGAffineTransformMake(0, 1, 1, 0, NSMinX(r), NSMinY(r));
    break;
  case kEdgeBottom:
    t = CGAffineTransformMake(0, -1, 1, 0, NSMinX(r), NSMinY(r) + D);
    break;
  default:
    t = CGAffineTransformMake(1, 0, 0, 1, NSMinX(r), NSMinY(r));
    break;
  }
  CGPathRef out = CGPathCreateCopyByTransformingPath(p, &t);
  CGPathRelease(p);
  return out;
}

// Unit vector pointing from the body toward the bezel, in panel space. Folding
// cells slide this way, into the shoulder, as the outline closes over them.
static CGPoint ccpmTowardBezel(void) {
  switch (gEdge) {
  case kEdgeLeft:
    return CGPointMake(-1, 0);
  case kEdgeTop:
    return CGPointMake(0, 1);
  case kEdgeBottom:
    return CGPointMake(0, -1);
  default:
    return CGPointMake(1, 0);
  }
}

// ---- Hit regions and the cursor watcher

static NSPoint ccpmNotchLocalCursor(void) {
  NSPoint m = [NSEvent mouseLocation];
  return NSMakePoint(m.x - NSMinX(gPanelFrame), m.y - NSMinY(gPanelFrame));
}

// The regions that take the mouse right now: the wake band when folded; the
// shape, plus the hovered ring's card, when open. The card's region spans the
// gap between the shape and the card body, so the pointer never crosses a
// strip that belongs to neither on its way to the card.
static NSInteger ccpmNotchLiveRects(NSRect *out, NSInteger cap) {
  NSInteger n = 0;
  if (!gExpanded) {
    if (n < cap) out[n++] = gWake;
    return n;
  }
  if (n < cap) out[n++] = gExpandedRect;
  if (gHovered >= 0) {
    NSRect card = ccpmCellRect(gHovered, @"card");
    if (!NSIsEmptyRect(card) && n < cap) out[n++] = card;
  }
  return n;
}

static BOOL ccpmNotchPointerIsLive(void) {
  NSRect rects[4];
  NSInteger n = ccpmNotchLiveRects(rects, 4);
  NSPoint p = ccpmNotchLocalCursor();
  for (NSInteger i = 0; i < n; i++) {
    if (NSPointInRect(p, rects[i])) {
      return YES;
    }
  }
  return NO;
}

void ccpmNotchUpdateInteractive(void) {
  NSPanel *panel = CCPMNotchPanelRef();
  if (panel == nil) {
    return;
  }
  BOOL ignores = !ccpmNotchPointerIsLive();
  // Guarded write. AppKit does not skip an unchanged value: each assignment
  // re-sends the window's event mask to the window server, and this runs on
  // every mouse move anywhere on screen.
  if (panel.ignoresMouseEvents != ignores) {
    panel.ignoresMouseEvents = ignores;
  }
}

static NSInteger ccpmNotchSlotAt(NSPoint local) {
  // Only inside the open shape. The end rings' hit bands reach past its
  // rounded ends, so a pointer resting there showed a card while the notch,
  // finding the pointer outside every live rect, folded under it.
  if (!gExpanded || !NSPointInRect(local, gExpandedRect)) {
    return -1;
  }
  for (NSInteger i = 0; i < (NSInteger)gCells.count; i++) {
    if (NSPointInRect(local, ccpmCellRect(i, @"slot"))) {
      return i;
    }
  }
  return -1;
}

// __strong, not the default __autoreleasing for an out-parameter: these are
// globals holding a block, and ARC refuses to write back into one through an
// autoreleasing pointer.
static void ccpmCancel(dispatch_block_t __strong *slot) {
  if (*slot != nil) {
    dispatch_block_cancel(*slot);
    *slot = nil;
  }
}

// The single, idempotent answer to "where is the pointer now". The global
// monitor, the local monitor and the poll all funnel here, and calling it twice
// for the same position does nothing the first call did not — which is what
// makes three overlapping event sources safe.
static void ccpmNotchCursorMoved(void) {
  // Ordered out (switched off, or no profile shown): nothing to track. The
  // poll and the global monitor would otherwise keep expanding the invisible
  // panel and building cards off screen.
  NSPanel *panel = CCPMNotchPanelRef();
  if (panel == nil || !panel.isVisible) {
    return;
  }
  if (gVisibility == kVisHover) {
    ccpmNotchSetExpandedInternal(ccpmNotchPointerIsLive());
  }
  ccpmNotchSyncCard();
  ccpmNotchUpdateInteractive();
}

void ccpmNotchStartWatching(void) {
  if (gMonitors != nil) {
    return;
  }
  gMonitors = [NSMutableArray array];
  NSEventMask mask = NSEventMaskMouseMoved | NSEventMaskLeftMouseDragged;

  // No NSTrackingArea anywhere in this file. A tracking area lives on a view
  // that only receives events once ignoresMouseEvents is NO, and that is only
  // turned off once the pointer is known to be over a live rect — so a
  // tracking area can never see the crossing that would enable it. The global
  // monitor sees movement over other apps, which is where the pointer arrives
  // from; mouse-movement monitors need no accessibility grant.
  id global = [NSEvent addGlobalMonitorForEventsMatchingMask:mask
                                                     handler:^(NSEvent *e) {
    (void)e;
    ccpmNotchCursorMoved();
  }];
  if (global != nil) {
    [gMonitors addObject:global];
  }
  // The local monitor catches the way back out, once the panel takes events.
  id local = [NSEvent addLocalMonitorForEventsMatchingMask:mask
                                                   handler:^NSEvent *(NSEvent *e) {
    ccpmNotchCursorMoved();
    return e;
  }];
  if (local != nil) {
    [gMonitors addObject:local];
  }

  gPollTimer = [NSTimer timerWithTimeInterval:kCursorPoll
                                      repeats:YES
                                        block:^(NSTimer *t) {
    (void)t;
    ccpmNotchCursorMoved();
  }];
  // Common modes, so the poll runs while a menu is open or a window is being
  // resized — both states in which the notch can end up under a still pointer.
  [[NSRunLoop mainRunLoop] addTimer:gPollTimer forMode:NSRunLoopCommonModes];
}

void ccpmNotchStopWatching(void) {
  for (id m in gMonitors) {
    [NSEvent removeMonitor:m];
  }
  gMonitors = nil;
  [gPollTimer invalidate];
  gPollTimer = nil;
  ccpmCancel(&gFoldWork);
  ccpmCancel(&gCardWork);
  // The panel is about to go. Drop every layer and state flag tied to it, so a
  // later Start builds fresh ones on the new panel: EnsureLayers returns early
  // while gBody is set, which left a restarted notch empty, its layers still
  // attached to the dead panel.
  [gBody removeFromSuperlayer];
  [gContent removeFromSuperlayer];
  [gCard removeFromSuperlayer];
  gBody = nil;
  gClip = nil;
  gContent = nil;
  gCard = nil;
  gCardFor = -1;
  gCellLayers = nil;
  gLastFraction = nil;
  gHovered = -1;
  gExpanded = NO;
}

// ---- Layers

static CATextLayer *ccpmText(NSString *s, CGRect frame, CGFloat size, NSColor *color,
                             NSFontWeight weight, NSString *align) {
  CATextLayer *t = [CATextLayer layer];
  NSFont *font = [NSFont systemFontOfSize:size weight:weight];
  t.string = s ?: @"";
  t.font = (__bridge CFTypeRef)font;
  t.fontSize = size;
  t.foregroundColor = color.CGColor;
  t.alignmentMode = align;
  t.truncationMode = kCATruncationEnd;
  t.frame = frame;
  // The panel's own display, not the key window's: on a mixed-DPI setup the
  // two differ and text rendered at the wrong scale comes out blurry.
  t.contentsScale = CCPMNotchPanelRef().backingScaleFactor ?: 2.0;
  return t;
}

// A circle stroke starting at 12 o'clock and running clockwise, so strokeEnd
// sweeps the way a clock hand does.
static CAShapeLayer *ccpmArc(CGPoint c, CGFloat r, CGFloat width, NSColor *color, CGFloat end,
                             BOOL round) {
  CGMutablePathRef path = CGPathCreateMutable();
  CGPathAddArc(path, NULL, c.x, c.y, r, M_PI_2, M_PI_2 - 2 * M_PI, true);
  CAShapeLayer *l = [CAShapeLayer layer];
  l.path = path;
  CGPathRelease(path);
  l.fillColor = NULL;
  l.strokeColor = color.CGColor;
  l.lineWidth = width;
  l.lineCap = round ? kCALineCapRound : kCALineCapButt;
  l.strokeStart = 0;
  l.strokeEnd = end;
  return l;
}

static NSString *ccpmInitial(NSString *profile) {
  if (![profile isKindOfClass:[NSString class]] || profile.length == 0) {
    return @"";
  }
  NSRange r = [profile rangeOfComposedCharacterSequenceAtIndex:0];
  return [[profile substringWithRange:r] uppercaseString];
}

// One profile: the main ring, the thin secondary ring inside it (which window
// is which was settled in Go), the profile's initial where the reference puts
// its provider mark, and the percentage beneath.
//
// Returned as a container spanning the whole panel so the cell's ring and
// label rects — both panel-local, from Go — are used as they arrive.
static CALayer *ccpmCellLayer(NSDictionary *slot, NSInteger i) {
  CALayer *cell = [CALayer layer];
  cell.frame = gContent.bounds;

  NSRect ring = ccpmCellRect(i, @"ring");
  NSRect label = ccpmCellRect(i, @"label");
  BOOL available = [slot[@"available"] boolValue];
  NSString *profile = slot[@"profile"];
  CGPoint c = CGPointMake(NSMidX(ring), NSMidY(ring));
  // The scale the ring is drawn at, read off the box Go sized: 1 everywhere
  // but a hardware notch, where the whole notch is drawn at the size that
  // lands its depth on the hole's. Strokes, radii and type follow it so the
  // ring keeps its proportions rather than thinning to a wire.
  CGFloat k = NSWidth(ring) > 0 ? NSWidth(ring) / kRingDiameter : 1;

  // The track is inset by half its own stroke, so its outer edge lands on the
  // 44pt diameter exactly — SwiftUI's strokeBorder, which the reference uses.
  CGFloat trackR = k * (kRingDiameter / 2 - kTrackStroke / 2);
  [cell addSublayer:ccpmArc(c, trackR, k * kTrackStroke, ccpmRingTrack(), 1, NO)];

  CAShapeLayer *weeklyTrack =
      ccpmArc(c, k * kWeeklyRadius, k * kWeeklyStroke, ccpmRingTrack(), 1, NO);
  weeklyTrack.opacity = 0.7;
  [cell addSublayer:weeklyTrack];

  // Unavailable gets track only. Never a 0% arc: an empty ring reads as
  // "plenty of headroom" when the truth is "we have no reading".
  if (available) {
    CGFloat outer = [slot[@"outer"] doubleValue];
    CGFloat inner = [slot[@"inner"] doubleValue];
    NSArray *was = [profile isKindOfClass:[NSString class]] ? gLastFraction[profile] : nil;

    CAShapeLayer *progress =
        ccpmArc(c, trackR, k * kProgressStroke,
                ccpmRailColor([slot[@"outerRGB"] unsignedIntValue], 1), outer, YES);
    [cell addSublayer:progress];

    CAShapeLayer *weekly =
        ccpmArc(c, k * kWeeklyRadius, k * kWeeklyStroke,
                ccpmRailColor([slot[@"innerRGB"] unsignedIntValue], 1), inner, YES);
    weekly.opacity = 0.8;
    [cell addSublayer:weekly];

    // A refresh moves the arc from where it was rather than redrawing it.
    if (was.count == 2 && !ccpmReduceMotion()) {
      CASpringAnimation *a = ccpmSpring(@"strokeEnd", kReading);
      a.fromValue = was[0];
      a.toValue = @(outer);
      [progress addAnimation:a forKey:@"reading"];
      CASpringAnimation *b = ccpmSpring(@"strokeEnd", kReading);
      b.fromValue = was[1];
      b.toValue = @(inner);
      [weekly addAnimation:b forKey:@"reading"];
    }
    if ([profile isKindOfClass:[NSString class]]) {
      gLastFraction[profile] = @[ @(outer), @(inner) ];
    }
  }

  CGFloat glyph = 11.0 * k;
  [cell addSublayer:ccpmText(ccpmInitial(profile),
                             CGRectMake(c.x - 10 * k, c.y - glyph * 0.62, 20 * k, glyph * 1.3),
                             glyph, [NSColor colorWithWhite:1 alpha:0.9], NSFontWeightSemibold,
                             kCAAlignmentCenter)];

  // With percentages hidden Go sends no label box, and the geometry has
  // already given its room back to the rings.
  if (NSIsEmptyRect(label)) {
    return cell;
  }
  // Across a hardware notch the one ring's reading has the other side to
  // itself: the larger size, held against the hole it reads across.
  NSDictionary *layout = gCells[i];
  BOOL across = [layout isKindOfClass:[NSDictionary class]] && [layout[@"across"] boolValue];
  NSString *pct = [slot[@"percent"] isKindOfClass:[NSString class]] ? slot[@"percent"] : @"—";
  [cell addSublayer:ccpmText(pct, label, k * (across ? kPercentAcrossSize : kPercentSize),
                             available ? ccpmTextPrimary() : ccpmTextSecondary(),
                             NSFontWeightSemibold,
                             across ? kCAAlignmentRight : kCAAlignmentCenter)];
  return cell;
}

// Positions every cell for the current state: in place and opaque when open,
// slid toward the bezel and transparent when folded. Staggered on the way in,
// so the rings arrive one after another rather than as a block.
static void ccpmNotchApplyCells(BOOL animated) {
  CGPoint away = ccpmTowardBezel();
  for (NSUInteger i = 0; i < gCellLayers.count; i++) {
    CALayer *cell = gCellLayers[i];
    // transform.translation takes a SIZE, not a point — Core Animation reads
    // the NSValue as CGSize for that key path.
    NSValue *offset = [NSValue valueWithSize:gExpanded ? NSZeroSize
                                                       : NSMakeSize(away.x * kFoldSlide,
                                                                    away.y * kFoldSlide)];
    NSNumber *opacity = @(gExpanded ? 1.0 : 0.0);
    if (!animated) {
      [CATransaction begin];
      [CATransaction setDisableActions:YES];
      [cell setValue:offset forKeyPath:@"transform.translation"];
      cell.opacity = opacity.floatValue;
      [CATransaction commit];
      continue;
    }
    CFTimeInterval delay = gExpanded ? MIN(i * 0.045, 0.18) : 0;
    ccpmAnimate(cell, @"transform.translation", offset, kContents, delay);
    ccpmAnimate(cell, @"opacity", opacity, kContents, delay);
  }
}

// Morphs the shape to the current state.
static void ccpmNotchApplyShape(BOOL animated) {
  if (gBody == nil) {
    return;
  }
  NSRect r = gExpanded ? gExpandedRect : gCollapsed;
  if (NSIsEmptyRect(r)) {
    return;
  }
  CGPathRef path = ccpmNotchPath(r, gExpanded ? gExpCorner : gColCorner,
                                 gExpanded ? gExpCurl : gColCurl, gEdge);
  id value = (__bridge id)path;
  if (animated) {
    ccpmAnimate(gBody, @"path", value, kUnfold, 0);
    ccpmAnimate(gClip, @"path", value, kUnfold, 0);
  } else {
    [CATransaction begin];
    [CATransaction setDisableActions:YES];
    gBody.path = path;
    gClip.path = path;
    [CATransaction commit];
  }
  CGPathRelease(path);
  ccpmNotchApplyCells(animated);
}

// The expand/collapse state machine. Opening is immediate and cancels a
// pending fold; closing is deferred by kFoldGrace and re-checks at fire time,
// so a pointer that leaves and comes straight back never sees the notch move.
static void ccpmNotchSetExpandedInternal(BOOL wanted) {
  if (wanted) {
    ccpmCancel(&gFoldWork);
    if (gExpanded) {
      return;
    }
    gExpanded = YES;
    ccpmNotchApplyShape(YES);
    ccpmNotchUpdateInteractive();
    return;
  }
  if (!gExpanded || gFoldWork != nil || gVisibility == kVisAlways) {
    return;
  }
  dispatch_block_t work = dispatch_block_create(0, ^{
    gFoldWork = nil;
    if (gVisibility == kVisAlways || ccpmNotchPointerIsLive()) {
      return;
    }
    gExpanded = NO;
    gHovered = -1;
    ccpmNotchSyncCard();
    ccpmNotchApplyShape(YES);
    ccpmNotchUpdateInteractive();
  });
  gFoldWork = work;
  dispatch_after(dispatch_time(DISPATCH_TIME_NOW, (int64_t)(kFoldGrace * NSEC_PER_SEC)),
                 dispatch_get_main_queue(), work);
}

void CCPMNotchSetVisibility(int mode) {
  ccpmRailOnMain(^{
    gVisibility = mode;
    if (mode == kVisAlways) {
      ccpmCancel(&gFoldWork);
      ccpmNotchSetExpandedInternal(YES);
      return;
    }
    if (mode == kVisHidden) {
      ccpmCancel(&gFoldWork);
      gExpanded = NO;
      gHovered = -1;
      ccpmNotchSyncCard();
      ccpmNotchApplyShape(NO);
      ccpmNotchUpdateInteractive();
      return;
    }
    // Hover: settle it from the pointer's real position, which may already be
    // on the notch.
    ccpmNotchCursorMoved();
  });
}

// ---- The card

// The card body plus its tail, as one path. The tail's tip is the side of its
// box that faces the shape, centred along it.
static CGPathRef ccpmCardPath(NSRect body, NSRect tail) {
  CGMutablePathRef p = CGPathCreateMutable();
  CGPathAddRoundedRect(p, NULL, body, kCardCorner, kCardCorner);
  if (!NSIsEmptyRect(tail)) {
    CGPoint tip, a, b;
    switch (gEdge) {
    case kEdgeLeft: // card to the right of the shape; tip points left
      tip = CGPointMake(NSMinX(tail), NSMidY(tail));
      a = CGPointMake(NSMaxX(tail) + 1, NSMinY(tail));
      b = CGPointMake(NSMaxX(tail) + 1, NSMaxY(tail));
      break;
    case kEdgeTop: // card below the shape; tip points up
      tip = CGPointMake(NSMidX(tail), NSMaxY(tail));
      a = CGPointMake(NSMinX(tail), NSMinY(tail) - 1);
      b = CGPointMake(NSMaxX(tail), NSMinY(tail) - 1);
      break;
    case kEdgeBottom: // card above the shape; tip points down
      tip = CGPointMake(NSMidX(tail), NSMinY(tail));
      a = CGPointMake(NSMinX(tail), NSMaxY(tail) + 1);
      b = CGPointMake(NSMaxX(tail), NSMaxY(tail) + 1);
      break;
    default: // right edge: card to the left of the shape; tip points right
      tip = CGPointMake(NSMaxX(tail), NSMidY(tail));
      a = CGPointMake(NSMinX(tail) - 1, NSMinY(tail));
      b = CGPointMake(NSMinX(tail) - 1, NSMaxY(tail));
      break;
    }
    CGPathMoveToPoint(p, NULL, a.x, a.y);
    CGPathAddLineToPoint(p, NULL, tip.x, tip.y);
    CGPathAddLineToPoint(p, NULL, b.x, b.y);
    CGPathCloseSubpath(p);
  }
  return p;
}

// Builds one profile's card: its name, plan and freshness, then a block per
// usage window — label and percentage, a bar, and when it resets. Laid out top
// down, since that is how it is read, converting to AppKit's bottom-up y as it
// goes.
static CALayer *ccpmCardLayer(NSDictionary *callout, NSInteger i) {
  NSRect body = ccpmCellRect(i, @"body");
  NSRect tail = ccpmCellRect(i, @"tail");

  CALayer *card = [CALayer layer];
  card.frame = gContent.bounds;

  CAShapeLayer *bg = [CAShapeLayer layer];
  CGPathRef path = ccpmCardPath(body, tail);
  bg.path = path;
  CGPathRelease(path);
  bg.fillColor = [NSColor colorWithWhite:0 alpha:0.96].CGColor;
  [card addSublayer:bg];

  CGFloat pad = kCardPadding;
  CGFloat x = NSMinX(body) + pad, w = NSWidth(body) - 2 * pad;
  __block CGFloat top = NSMaxY(body) - pad; // moves downward
  CGFloat titleH = ceil(kCardTitleSize * 1.25), bodyH = ceil(kCardBodySize * 1.3);
  void (^line)(NSString *, CGFloat, CGFloat, NSColor *, NSFontWeight, NSString *) =
      ^(NSString *s, CGFloat size, CGFloat h, NSColor *color, NSFontWeight weight,
        NSString *align) {
        [card addSublayer:ccpmText(s, CGRectMake(x, top - h, w, h), size, color, weight, align)];
      };

  line(callout[@"profile"], kCardTitleSize, titleH, ccpmTextPrimary(), NSFontWeightSemibold,
       kCAAlignmentLeft);
  top -= titleH;

  NSMutableArray *sub = [NSMutableArray array];
  for (NSString *k in @[ @"plan", @"freshness" ]) {
    NSString *v = callout[k];
    if ([v isKindOfClass:[NSString class]] && v.length > 0) {
      [sub addObject:v];
    }
  }
  if (sub.count > 0) {
    line([sub componentsJoinedByString:@" · "], kCardBodySize, bodyH, ccpmTextSecondary(),
         NSFontWeightRegular, kCAAlignmentLeft);
    top -= bodyH;
  }
  top -= kHeaderToBlock;

  NSArray *windows = [callout[@"windows"] isKindOfClass:[NSArray class]] ? callout[@"windows"] : @[];
  if (windows.count == 0) {
    NSString *why = callout[@"explanation"];
    if ([why isKindOfClass:[NSString class]] && why.length > 0) {
      CATextLayer *t = ccpmText(why, CGRectMake(x, NSMinY(body) + pad, w, top - NSMinY(body) - pad),
                                kCardBodySize, ccpmTextSecondary(), NSFontWeightRegular,
                                kCAAlignmentLeft);
      t.wrapped = YES;
      [card addSublayer:t];
    }
    return card;
  }

  for (NSDictionary *win in windows) {
    if (![win isKindOfClass:[NSDictionary class]]) {
      continue;
    }
    line(win[@"label"], kCardBodySize, bodyH, ccpmTextSecondary(), NSFontWeightRegular,
         kCAAlignmentLeft);
    line(win[@"percent"], kCardBodySize, bodyH, ccpmTextPrimary(), NSFontWeightSemibold,
         kCAAlignmentRight);
    top -= bodyH + kLabelToBar;

    CALayer *track = [CALayer layer];
    track.frame = CGRectMake(x, top - kBarHeight, w, kBarHeight);
    track.backgroundColor = ccpmBarTrack().CGColor;
    track.cornerRadius = kBarHeight / 2;
    [card addSublayer:track];
    CGFloat frac = MAX(0, MIN(1, [win[@"fraction"] doubleValue]));
    if (frac > 0) {
      CALayer *fill = [CALayer layer];
      fill.frame = CGRectMake(x, top - kBarHeight, MAX(kBarHeight, w * frac), kBarHeight);
      fill.backgroundColor = ccpmRailColor([win[@"rgb"] unsignedIntValue], 1).CGColor;
      fill.cornerRadius = kBarHeight / 2;
      [card addSublayer:fill];
    }
    top -= kBarHeight + kBarToUsed;

    NSString *reset = win[@"reset"];
    if ([reset isKindOfClass:[NSString class]] && reset.length > 0) {
      line(reset, kCardBodySize, bodyH, ccpmTextSecondary(), NSFontWeightRegular,
           kCAAlignmentLeft);
      top -= bodyH;
    }
    top -= kBlockSpacing;
  }
  return card;
}

static void ccpmNotchRemoveCard(BOOL animated) {
  CALayer *old = gCard;
  gCard = nil;
  gCardFor = -1;
  if (old == nil) {
    return;
  }
  if (!animated || ccpmReduceMotion()) {
    [old removeFromSuperlayer];
    return;
  }
  [CATransaction begin];
  [CATransaction setAnimationDuration:kCrossfade];
  [CATransaction setCompletionBlock:^{
    [old removeFromSuperlayer];
  }];
  old.opacity = 0;
  [CATransaction commit];
}

// Draws the card for ring `want`, replacing whatever card is up. Animated when
// the pointer moves onto a ring (glide from the previous card, or fade in);
// not when a refresh rebuilds the card the pointer is already resting on.
static void ccpmNotchShowCard(NSInteger want, BOOL animated) {
  NSPanel *panel = CCPMNotchPanelRef();
  ccpmCancel(&gCardWork);
  NSInteger from = gCardFor;
  gHovered = want;
  NSDictionary *callout = nil;
  if (gSlots != nil && want >= 0 && want < (NSInteger)gSlots.count &&
      [gSlots[want] isKindOfClass:[NSDictionary class]]) {
    callout = gSlots[want][@"callout"];
  }
  if (panel == nil || ![callout isKindOfClass:[NSDictionary class]]) {
    // Nothing to show for this ring: take the previous ring's card down rather
    // than leave it up while gHovered already points somewhere else.
    ccpmNotchRemoveCard(NO);
    ccpmNotchUpdateInteractive();
    return;
  }

  CALayer *next = ccpmCardLayer(callout, want);
  [panel.contentView.layer addSublayer:next];

  if (animated && from >= 0 && gCard != nil && !ccpmReduceMotion()) {
    // Moving between rings: the new card glides in from where the old one was,
    // rather than one card vanishing and another appearing.
    NSRect a = ccpmCellRect(from, @"body"), b = ccpmCellRect(want, @"body");
    CGPoint pos = next.position;
    CASpringAnimation *glide = ccpmSpring(@"position", kGlide);
    glide.fromValue = [NSValue valueWithPoint:NSMakePoint(pos.x + NSMinX(a) - NSMinX(b),
                                                          pos.y + NSMinY(a) - NSMinY(b))];
    glide.toValue = [NSValue valueWithPoint:pos];
    [next addAnimation:glide forKey:@"glide"];
    [gCard removeFromSuperlayer];
  } else {
    ccpmNotchRemoveCard(NO);
    if (animated && !ccpmReduceMotion()) {
      CABasicAnimation *fade = [CABasicAnimation animationWithKeyPath:@"opacity"];
      fade.fromValue = @0;
      fade.toValue = @1;
      fade.duration = kCrossfade;
      [next addAnimation:fade forKey:@"fade"];
    }
  }
  gCard = next;
  gCardFor = want;
  ccpmNotchUpdateInteractive();
}

// Shows, moves or removes the card for whatever the pointer is over.
static void ccpmNotchSyncCard(void) {
  NSPanel *panel = CCPMNotchPanelRef();
  if (panel == nil) {
    return;
  }
  NSPoint p = ccpmNotchLocalCursor();
  NSInteger want = ccpmNotchSlotAt(p);

  // Being over the card itself must not dismiss it: the card is a live region
  // but not a slot, so without this, moving off the ring onto the card reads
  // as "no ring hovered" and takes the card away from under the pointer.
  if (want < 0 && gHovered >= 0 && NSPointInRect(p, ccpmCellRect(gHovered, @"card"))) {
    want = gHovered;
  }
  if (want == gHovered) {
    // Back on the ring (or card) a pending removal was scheduled for: keep it.
    // Returning without cancelling let that removal fire under the pointer.
    ccpmCancel(&gCardWork);
    // Nothing hovered, yet a card is still drawn. The fold and Hidden paths
    // reset gHovered before calling here, so comparing the pointer against
    // gHovered alone saw "no change" and never took the card down: it stayed
    // on screen with no way to dismiss it.
    if (want < 0 && gCard != nil) {
      ccpmNotchRemoveCard(YES);
      ccpmNotchUpdateInteractive();
    }
    return;
  }

  if (want < 0) {
    // Deferred: the pointer is usually in transit to the card. Once scheduled,
    // further moves must not push it back, or a pointer that keeps moving
    // (anywhere on screen, since the global monitor reports every move) holds
    // the card up indefinitely in Always mode.
    if (gCardWork != nil) {
      return;
    }
    NSInteger was = gHovered;
    dispatch_block_t work = dispatch_block_create(0, ^{
      gCardWork = nil;
      if (gHovered != was) {
        return;
      }
      gHovered = -1;
      ccpmNotchRemoveCard(YES);
      ccpmNotchUpdateInteractive();
    });
    gCardWork = work;
    dispatch_after(dispatch_time(DISPATCH_TIME_NOW, (int64_t)(kCardGrace * NSEC_PER_SEC)),
                   dispatch_get_main_queue(), work);
    return;
  }

  ccpmNotchShowCard(want, YES);
}

// ---- Entry points

// Creates the shape layers once, on first use.
static void ccpmNotchEnsureLayers(void) {
  NSPanel *panel = CCPMNotchPanelRef();
  if (panel == nil || gBody != nil) {
    return;
  }
  CALayer *root = panel.contentView.layer;

  gBody = [CAShapeLayer layer];
  gBody.frame = root.bounds;
  gBody.autoresizingMask = kCALayerWidthSizable | kCALayerHeightSizable;
  // Solid black, never glass, folded or open: it has to read as part of the
  // bezel. This is the reference's "solid" surface, which is also what it
  // falls back to under Reduce Transparency.
  gBody.fillColor = [NSColor blackColor].CGColor;
  [root addSublayer:gBody];

  gContent = [CALayer layer];
  gContent.frame = root.bounds;
  gContent.autoresizingMask = kCALayerWidthSizable | kCALayerHeightSizable;
  // The cells are clipped by the same outline, so as the notch folds they are
  // swallowed by it rather than left floating outside a shrinking shape.
  gClip = [CAShapeLayer layer];
  gClip.frame = root.bounds;
  gClip.autoresizingMask = kCALayerWidthSizable | kCALayerHeightSizable;
  gClip.fillColor = [NSColor blackColor].CGColor;
  gContent.mask = gClip;
  [root addSublayer:gContent];

  gCellLayers = [NSMutableArray array];
  gLastFraction = [NSMutableDictionary dictionary];
}

void CCPMNotchSetModel(const char *json) {
  if (json == NULL) {
    return;
  }
  // Copy before the hop: Go frees the C string as soon as this returns.
  NSData *data = [[NSString stringWithUTF8String:json] dataUsingEncoding:NSUTF8StringEncoding];

  ccpmRailOnMain(^{
    if (CCPMNotchPanelRef() == nil) {
      return;
    }
    ccpmNotchEnsureLayers();
    NSDictionary *model = [NSJSONSerialization JSONObjectWithData:data options:0 error:NULL];
    if (![model isKindOfClass:[NSDictionary class]]) {
      return;
    }
    NSArray *slots = model[@"slots"];
    if (![slots isKindOfClass:[NSArray class]]) {
      return;
    }
    gSlots = slots;
    gCells = [model[@"cells"] isKindOfClass:[NSArray class]] ? model[@"cells"] : @[];

    // The stack is rebuilt underneath whatever is hovered. Remember the ring
    // so its card comes back with the new numbers: this runs on every file
    // change and once a minute, and dropping the card took it away from under
    // a pointer resting on it (after which the notch folded).
    NSInteger keep = gHovered;
    gHovered = -1;
    ccpmCancel(&gCardWork);
    ccpmNotchRemoveCard(NO);

    for (CALayer *l in gCellLayers) {
      [l removeFromSuperlayer];
    }
    [gCellLayers removeAllObjects];
    NSUInteger n = MIN(slots.count, gCells.count);
    for (NSUInteger i = 0; i < n; i++) {
      NSDictionary *slot = slots[i];
      if (![slot isKindOfClass:[NSDictionary class]]) {
        continue;
      }
      CALayer *cell = ccpmCellLayer(slot, (NSInteger)i);
      [gContent addSublayer:cell];
      [gCellLayers addObject:cell];
    }
    ccpmNotchApplyCells(NO);
    if (gExpanded && keep >= 0 && keep < (NSInteger)n) {
      ccpmNotchShowCard(keep, NO);
    }
    ccpmNotchUpdateInteractive();
  });
}

void CCPMNotchSetGeometry(int edge, double panelX, double panelY, double panelW, double panelH,
                          double colX, double colY, double colW, double colH, double expX,
                          double expY, double expW, double expH, double wakeX, double wakeY,
                          double wakeW, double wakeH, double colCorner, double colCurl,
                          double expCorner, double expCurl) {
  ccpmRailOnMain(^{
    gEdge = edge;
    gPanelFrame = NSMakeRect(panelX, panelY, panelW, panelH);
    gCollapsed = NSMakeRect(colX, colY, colW, colH);
    gExpandedRect = NSMakeRect(expX, expY, expW, expH);
    gWake = NSMakeRect(wakeX, wakeY, wakeW, wakeH);
    gColCorner = colCorner;
    gColCurl = colCurl;
    gExpCorner = expCorner;
    gExpCurl = expCurl;

    NSPanel *panel = CCPMNotchPanelRef();
    if (panel == nil) {
      return;
    }
    // The ONE place the panel's frame is set. Not animated: it changes only
    // with the screen, the edge or the profile count — never on expand.
    [panel setFrame:gPanelFrame display:YES];
    ccpmNotchEnsureLayers();
    ccpmNotchApplyShape(NO);

    // The panel may have moved under a parked pointer, which produces no
    // events; settle the state from where the pointer actually is.
    ccpmNotchCursorMoved();
  });
}
