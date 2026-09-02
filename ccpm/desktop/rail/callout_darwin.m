//go:build darwin

#import <Cocoa/Cocoa.h>
#import <QuartzCore/QuartzCore.h>
#import "rail.h"
#import "rail_internal.h"

// The hover callout: a second borderless panel showing one profile's detail
// while its ring is under the pointer.
//
// A separate panel rather than an expanded rail, because the rail is pinned to
// a screen edge and the detail has to grow inward across whatever is beneath
// it. A view inside the rail would be clipped by the rail's own frame.

static const CGFloat kCalloutWidth = 244.0;
static const CGFloat kCalloutPad = 14.0;
static const CGFloat kCalloutGap = 10.0;
static const CGFloat kHeaderGap = 12.0;
static const CGFloat kBarHeight = 4.0;
// Space between one window's block and the next. Without it the previous
// block's "80% used" sits directly on the next block's label and the two read
// as one run-on paragraph.
static const CGFloat kBlockGap = 16.0;

static NSPanel *gCallout = nil;

// Text metrics are needed before layout to size the panel, so both the
// measuring and the drawing go through this.
static NSAttributedString *ccpmCalloutString(NSString *s, CGFloat size,
                                             NSColor *color, NSFontWeight weight,
                                             NSTextAlignment align) {
  NSMutableParagraphStyle *para = [[NSMutableParagraphStyle alloc] init];
  para.alignment = align;
  para.lineBreakMode = NSLineBreakByWordWrapping;
  return [[NSAttributedString alloc]
      initWithString:(s ?: @"")
          attributes:@{NSFontAttributeName: [NSFont systemFontOfSize:size weight:weight],
                       NSForegroundColorAttributeName: color,
                       NSParagraphStyleAttributeName: para}];
}

static CGFloat ccpmCalloutHeightOf(NSAttributedString *s, CGFloat width) {
  return ceil([s boundingRectWithSize:NSMakeSize(width, CGFLOAT_MAX)
                              options:NSStringDrawingUsesLineFragmentOrigin].size.height);
}

static NSTextField *ccpmCalloutLabel(NSAttributedString *s, NSRect frame) {
  NSTextField *f = [[NSTextField alloc] initWithFrame:frame];
  f.attributedStringValue = s;
  f.bezeled = NO;
  f.drawsBackground = NO;
  f.editable = NO;
  f.selectable = NO;
  return f;
}

// Builds the callout's content view and returns its height. Laid out top-down
// into a view whose height is computed first, because AppKit's origin is at the
// bottom and a panel that resizes after its subviews are placed puts everything
// in the wrong place.
static NSView *ccpmCalloutBody(NSDictionary *callout, NSDictionary *theme, CGFloat *outHeight) {
  NSColor *fg = ccpmRailColor([theme[@"foreground"] unsignedIntValue], 1.0);
  NSColor *muted = ccpmRailColor([theme[@"mutedForeground"] unsignedIntValue], 1.0);
  NSColor *track = ccpmRailColor([theme[@"track"] unsignedIntValue], 1.0);

  CGFloat inner = kCalloutWidth - 2 * kCalloutPad;
  NSArray *windows = callout[@"windows"];
  NSString *explanation = callout[@"explanation"];
  BOOL stale = [callout[@"stale"] boolValue];

  NSAttributedString *name =
      ccpmCalloutString(callout[@"profile"], 13, fg, NSFontWeightSemibold, NSTextAlignmentLeft);
  // Account and plan on one line: which login this ring is about is the whole
  // point of per-profile limits.
  NSString *sub = callout[@"account"];
  if ([callout[@"plan"] length] > 0) {
    sub = [NSString stringWithFormat:@"%@ · %@", sub, callout[@"plan"]];
  }
  NSAttributedString *account =
      ccpmCalloutString(sub, 10, muted, NSFontWeightRegular, NSTextAlignmentLeft);
  // Stale data must LOOK stale, not merely be labelled somewhere.
  NSAttributedString *fresh = ccpmCalloutString(
      callout[@"freshness"], 10,
      stale ? ccpmRailColor(0xFFAF5F, 1.0) : muted,
      stale ? NSFontWeightMedium : NSFontWeightRegular, NSTextAlignmentLeft);

  CGFloat nameH = ccpmCalloutHeightOf(name, inner);
  CGFloat accountH = ccpmCalloutHeightOf(account, inner);
  CGFloat freshH = ccpmCalloutHeightOf(fresh, inner);
  CGFloat explainH = 0;
  NSAttributedString *explain = nil;
  if (windows.count == 0) {
    explain = ccpmCalloutString(explanation, 11, muted, NSFontWeightRegular, NSTextAlignmentLeft);
    explainH = ccpmCalloutHeightOf(explain, inner);
  }

  // Measure a block rather than assuming one. A fixed guess that comes out
  // shorter than the content makes every block overlap the next, which is
  // exactly what a hardcoded 46pt did here.
  CGFloat labelH = ccpmCalloutHeightOf(
      ccpmCalloutString(@"Ag", 11, fg, NSFontWeightMedium, NSTextAlignmentLeft), inner);
  CGFloat pctH = ccpmCalloutHeightOf(
      ccpmCalloutString(@"Ag", 10, muted, NSFontWeightRegular, NSTextAlignmentLeft), inner);
  CGFloat blockH = labelH + kCalloutGap + kBarHeight + kCalloutGap + pctH;

  CGFloat body_h = explainH;
  if (windows.count > 0) {
    body_h = windows.count * blockH + (windows.count - 1) * kBlockGap;
  }
  CGFloat height = kCalloutPad * 2 + nameH + accountH + freshH + kHeaderGap + body_h;

  NSView *body = [[NSView alloc] initWithFrame:NSMakeRect(0, 0, kCalloutWidth, height)];
  body.wantsLayer = YES;

  CGFloat y = height - kCalloutPad;

  y -= nameH;
  [body addSubview:ccpmCalloutLabel(name, NSMakeRect(kCalloutPad, y, inner, nameH))];
  y -= accountH;
  [body addSubview:ccpmCalloutLabel(account, NSMakeRect(kCalloutPad, y, inner, accountH))];
  y -= freshH;
  [body addSubview:ccpmCalloutLabel(fresh, NSMakeRect(kCalloutPad, y, inner, freshH))];
  y -= kHeaderGap;

  if (windows.count == 0) {
    y -= explainH;
    [body addSubview:ccpmCalloutLabel(explain, NSMakeRect(kCalloutPad, y, inner, explainH))];
    if (outHeight) *outHeight = height;
    return body;
  }

  for (NSUInteger wi = 0; wi < windows.count; wi++) {
    NSDictionary *w = windows[wi];
    if (wi > 0) {
      y -= kBlockGap;
    }
    NSAttributedString *label =
        ccpmCalloutString(w[@"label"], 11, fg, NSFontWeightMedium, NSTextAlignmentLeft);
    NSAttributedString *reset =
        ccpmCalloutString(w[@"reset"], 10, muted, NSFontWeightRegular, NSTextAlignmentRight);
    NSAttributedString *pct =
        ccpmCalloutString(w[@"percent"], 10, muted, NSFontWeightRegular, NSTextAlignmentLeft);

    y -= labelH;
    [body addSubview:ccpmCalloutLabel(label, NSMakeRect(kCalloutPad, y, inner * 0.5, labelH))];
    [body addSubview:ccpmCalloutLabel(reset, NSMakeRect(kCalloutPad + inner * 0.5, y,
                                                        inner * 0.5, labelH))];
    y -= kCalloutGap;

    y -= kBarHeight;
    CALayer *barTrack = [CALayer layer];
    barTrack.frame = CGRectMake(kCalloutPad, y, inner, kBarHeight);
    barTrack.backgroundColor = track.CGColor;
    barTrack.cornerRadius = kBarHeight / 2;
    [body.layer addSublayer:barTrack];

    CGFloat frac = [w[@"fraction"] doubleValue];
    if (frac > 0) {
      CALayer *fill = [CALayer layer];
      // A visible minimum: a 1% window drawn to scale is a 2pt smudge that
      // reads as nothing at all rather than as "barely used".
      CGFloat fw = MAX(kBarHeight, inner * frac);
      fill.frame = CGRectMake(kCalloutPad, y, fw, kBarHeight);
      fill.backgroundColor = ccpmRailColor([w[@"rgb"] unsignedIntValue], 1.0).CGColor;
      fill.cornerRadius = kBarHeight / 2;
      [body.layer addSublayer:fill];
    }
    y -= kCalloutGap;

    y -= pctH;
    [body addSubview:ccpmCalloutLabel(pct, NSMakeRect(kCalloutPad, y, inner, pctH))];
  }

  if (outHeight) *outHeight = height;
  return body;
}

static void ccpmCalloutEnsurePanel(void) {
  if (gCallout != nil) {
    return;
  }
  gCallout = [[NSPanel alloc]
      initWithContentRect:NSMakeRect(0, 0, kCalloutWidth, 120)
                styleMask:(NSWindowStyleMaskBorderless | NSWindowStyleMaskNonactivatingPanel)
                  backing:NSBackingStoreBuffered
                    defer:NO];
  // Above the rail, and with the same never-steal-focus posture: a detail panel
  // that took the caret would be worse than no detail panel.
  gCallout.level = NSStatusWindowLevel + 1;
  gCallout.collectionBehavior = NSWindowCollectionBehaviorCanJoinAllSpaces |
                                NSWindowCollectionBehaviorStationary |
                                NSWindowCollectionBehaviorFullScreenAuxiliary;
  gCallout.hidesOnDeactivate = NO;
  gCallout.canHide = NO;
  gCallout.releasedWhenClosed = NO;
  gCallout.becomesKeyOnlyIfNeeded = YES;
  gCallout.opaque = NO;
  gCallout.backgroundColor = [NSColor clearColor];
  gCallout.hasShadow = YES;
  // The pointer must be able to travel from the ring to whatever is underneath
  // without the callout intercepting it; it is a readout, not a control.
  gCallout.ignoresMouseEvents = YES;
}

void ccpmCalloutShow(NSDictionary *callout, NSDictionary *theme,
                     double anchorX, double anchorY, NSString *grows) {
  if (callout == nil || theme == nil) {
    return;
  }
  ccpmCalloutEnsurePanel();

  CGFloat height = 0;
  NSView *body = ccpmCalloutBody(callout, theme, &height);

  NSVisualEffectView *fx =
      [[NSVisualEffectView alloc] initWithFrame:NSMakeRect(0, 0, kCalloutWidth, height)];
  if (@available(macOS 10.14, *)) {
    fx.material = NSVisualEffectMaterialHUDWindow;
  }
  fx.blendingMode = NSVisualEffectBlendingModeBehindWindow;
  fx.state = NSVisualEffectStateActive;
  fx.wantsLayer = YES;
  fx.layer.cornerRadius = 14.0;
  fx.layer.masksToBounds = YES;
  [fx addSubview:body];
  gCallout.contentView = fx;

  // Grow away from the screen edge the rail is on, or half the callout renders
  // off-screen. The grows side comes from CalloutAnchor in geometry.go, so the
  // orientation rule lives in one tested place rather than being re-derived.
  CGFloat x = anchorX - kCalloutWidth - kCalloutGap;
  CGFloat y = anchorY - height / 2;
  if ([grows isEqualToString:@"right"]) {
    x = anchorX + kCalloutGap;
  } else if ([grows isEqualToString:@"bottom"]) {
    x = anchorX - kCalloutWidth / 2;
    y = anchorY - height - kCalloutGap;
  } else if ([grows isEqualToString:@"top"]) {
    x = anchorX - kCalloutWidth / 2;
    y = anchorY + kCalloutGap;
  }

  // Clamp inside the screen so the topmost and bottommost rings still get a
  // whole panel rather than one running off the edge.
  NSScreen *screen = [NSScreen mainScreen];
  if (screen != nil) {
    NSRect v = screen.visibleFrame;
    x = MAX(NSMinX(v), MIN(x, NSMaxX(v) - kCalloutWidth));
    y = MAX(NSMinY(v), MIN(y, NSMaxY(v) - height));
  }

  [gCallout setFrame:NSMakeRect(x, y, kCalloutWidth, height) display:YES];
  [gCallout orderFrontRegardless];
}

void ccpmCalloutHide(void) {
  ccpmRailOnMain(^{
    [gCallout orderOut:nil];
  });
}

void ccpmCalloutStop(void) {
  ccpmRailOnMain(^{
    if (gCallout == nil) {
      return;
    }
    [gCallout orderOut:nil];
    [gCallout close];
    gCallout = nil;
  });
}
