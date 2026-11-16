//go:build darwin

#import <Cocoa/Cocoa.h>
#import "rail_internal.h"
#import "tray.h"
#include "_cgo_export.h"

// Prefixed CCPMTray for the reason at the top of rail_darwin.m: duplicate
// Objective-C class names are a link-time collision with Wails' own.

@interface CCPMTrayMenu : NSObject <NSMenuDelegate>
- (void)runProfile:(NSMenuItem *)sender;
- (void)showWindow:(NSMenuItem *)sender;
@end

// Main thread only. All three are strong on purpose: a status item vanishes
// from the menu bar when it deallocates, and NSMenu.delegate is weak.
static NSStatusItem *gTrayItem = nil;
static CCPMTrayMenu *gTrayMenu = nil;
static NSArray *gTrayRows = nil;

@implementation CCPMTrayMenu

// Built from the cached rows and nothing else: no disk, no call into Go.
- (void)menuNeedsUpdate:(NSMenu *)menu {
  [menu removeAllItems];

  for (NSDictionary *row in gTrayRows) {
    NSMenuItem *item = [[NSMenuItem alloc] initWithTitle:row[@"title"]
                                                  action:@selector(runProfile:)
                                           keyEquivalent:@""];
    item.target = self;
    item.representedObject = row[@"profile"];
    [menu addItem:item];
  }
  if (gTrayRows.count == 0) {
    NSMenuItem *none = [[NSMenuItem alloc] initWithTitle:@"No profiles yet" action:nil keyEquivalent:@""];
    none.enabled = NO;
    [menu addItem:none];
  }
  [menu addItem:[NSMenuItem separatorItem]];

  NSMenuItem *show = [[NSMenuItem alloc] initWithTitle:@"Show Window"
                                                action:@selector(showWindow:)
                                         keyEquivalent:@""];
  show.target = self;
  [menu addItem:show];

  // terminate: is the entry Cmd-Q and the Dock's Quit use. Wails' delegate turns
  // it into its own quit, so OnShutdown runs exactly as it does for those.
  NSMenuItem *quit = [[NSMenuItem alloc] initWithTitle:@"Quit"
                                                action:@selector(terminate:)
                                         keyEquivalent:@""];
  quit.target = NSApp;
  [menu addItem:quit];
}

- (void)runProfile:(NSMenuItem *)sender {
  NSString *profile = sender.representedObject;
  ccpmTrayRun((char *)profile.UTF8String);
}

- (void)showWindow:(NSMenuItem *)sender {
  (void)sender;
  ccpmTrayShowWindow();
}

@end

// Two arcs, the notch's big ring and small ring. Black is ignored: as a template
// image AppKit tints it to suit a light or dark menu bar.
static NSImage *ccpmTrayIcon(void) {
  NSImage *icon = [NSImage imageWithSize:NSMakeSize(18, 18)
                                 flipped:NO
                          drawingHandler:^BOOL(NSRect rect) {
                            NSPoint c = NSMakePoint(NSMidX(rect), NSMidY(rect));
                            [[NSColor blackColor] setStroke];
                            NSBezierPath *outer = [NSBezierPath bezierPath];
                            [outer appendBezierPathWithArcWithCenter:c radius:7 startAngle:90 endAngle:0 clockwise:NO];
                            outer.lineWidth = 2;
                            outer.lineCapStyle = NSLineCapStyleRound;
                            [outer stroke];
                            NSBezierPath *inner = [NSBezierPath bezierPath];
                            [inner appendBezierPathWithArcWithCenter:c radius:3 startAngle:90 endAngle:270 clockwise:NO];
                            inner.lineWidth = 2;
                            inner.lineCapStyle = NSLineCapStyleRound;
                            [inner stroke];
                            return YES;
                          }];
  icon.template = YES;
  return icon;
}

void CCPMTrayShow(void) {
  ccpmRailOnMain(^{
    if (gTrayItem != nil) {
      return;
    }
    if (gTrayMenu == nil) {
      gTrayMenu = [[CCPMTrayMenu alloc] init];
    }
    NSMenu *menu = [[NSMenu alloc] initWithTitle:@"CCPM"];
    menu.autoenablesItems = NO;
    menu.delegate = gTrayMenu;

    gTrayItem = [[NSStatusBar systemStatusBar] statusItemWithLength:NSSquareStatusItemLength];
    gTrayItem.button.image = ccpmTrayIcon();
    gTrayItem.button.toolTip = @"CCPM";
    gTrayItem.button.accessibilityLabel = @"CCPM";
    gTrayItem.menu = menu;
  });
}

void CCPMTrayHide(void) {
  ccpmRailOnMain(^{
    if (gTrayItem == nil) {
      return;
    }
    [[NSStatusBar systemStatusBar] removeStatusItem:gTrayItem];
    gTrayItem = nil;
  });
}

void CCPMTraySetRows(const char *json) {
  if (json == NULL) {
    return;
  }
  // Copy before the hop: Go frees the C string as soon as this returns.
  NSData *data = [[NSString stringWithUTF8String:json] dataUsingEncoding:NSUTF8StringEncoding];

  ccpmRailOnMain(^{
    NSArray *rows = [NSJSONSerialization JSONObjectWithData:data options:0 error:NULL];
    if ([rows isKindOfClass:[NSArray class]]) {
      gTrayRows = rows;
    }
  });
}
