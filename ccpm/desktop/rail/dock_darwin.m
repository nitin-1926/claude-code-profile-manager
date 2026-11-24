//go:build darwin

#import <Cocoa/Cocoa.h>
#import "rail_internal.h"
#import "tray.h"

// The icon as it was before the first meter. Main thread only.
static NSImage *gDockPlainIcon = nil;

// The icon with a bar over its lower third, below the glyph and inside the
// tile's rounded square (build/appicon.png). A fixed large size, not plain.size:
// the app icon reports a small nominal size whatever reps it holds, and the Dock
// tile is bigger than that.
static NSImage *ccpmDockIcon(NSImage *plain, double fraction, unsigned int rgb) {
  return [NSImage imageWithSize:NSMakeSize(512, 512)
                        flipped:NO
                 drawingHandler:^BOOL(NSRect rect) {
                   [plain drawInRect:rect];

                   NSRect track = NSMakeRect(NSWidth(rect) * 0.24, NSHeight(rect) * 0.205,
                                             NSWidth(rect) * 0.52, NSHeight(rect) * 0.085);
                   CGFloat radius = NSHeight(track) / 2;
                   NSBezierPath *capsule = [NSBezierPath bezierPathWithRoundedRect:track
                                                                           xRadius:radius
                                                                           yRadius:radius];
                   // Near-black, like the notch its colours are tuned for.
                   [[NSColor colorWithWhite:0.06 alpha:0.92] setFill];
                   [capsule fill];

                   // Clipped, not a shorter capsule: a sliver of fill would
                   // otherwise be wider than it is long.
                   [NSGraphicsContext saveGraphicsState];
                   [capsule addClip];
                   NSRect fill = track;
                   fill.size.width *= fraction;
                   [ccpmRailColor(rgb, 1) setFill];
                   NSRectFill(fill);
                   [NSGraphicsContext restoreGraphicsState];
                   return YES;
                 }];
}

void CCPMDockShowMeter(double fraction, unsigned int rgb) {
  ccpmRailOnMain(^{
    if (gDockPlainIcon == nil) {
      gDockPlainIcon = NSApp.applicationIconImage;
    }
    NSApp.applicationIconImage = ccpmDockIcon(gDockPlainIcon, fraction, rgb);
  });
}

void CCPMDockRestore(void) {
  ccpmRailOnMain(^{
    NSApp.applicationIconImage = gDockPlainIcon;
  });
}
