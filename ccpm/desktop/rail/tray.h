// C surface for the menu bar item and the Dock meter. As with rail.h, every
// function is safe to call from any goroutine: each hops to the main queue
// internally.
#ifndef CCPM_TRAY_H
#define CCPM_TRAY_H

// Adds the status item, or removes it. Both are idempotent.
void CCPMTrayShow(void);
void CCPMTrayHide(void);

// Replaces the cached rows the menu is built from when it opens. json is an
// array of rail.MenuRow.
void CCPMTraySetRows(const char *json);

// Draws a meter over the Dock icon, filled to fraction (0..1) in rgb, a 0xRRGGBB
// value. Restore puts back the icon from before the first meter.
void CCPMDockShowMeter(double fraction, unsigned int rgb);
void CCPMDockRestore(void);

#endif
