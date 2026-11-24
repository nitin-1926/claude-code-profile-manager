//go:build darwin

package rail

/*
#include "tray.h"
*/
import "C"

import (
	"sync"
	"time"

	"github.com/nitin-1926/claude-code-profile-manager/ccpm/desktop/services"
)

// DockMeter is what the Dock icon shows. The zero value is the plain icon.
type DockMeter struct {
	Shown bool
	// Remaining is the percentage of the window left, 0..100: the meter drains.
	Remaining int
	RGB       RGB
}

// BuildDockMeter picks what the Dock shows: main is the window the notch's big
// ring uses, read from the preferred profile, or failing that the first profile
// in limits that has that window. No such profile means the plain icon, because
// an empty meter would say "out of headroom" when the truth is "we do not know".
func BuildDockMeter(limits []services.ProfileLimits, preferred, main string, now time.Time) DockMeter {
	for _, l := range limits {
		if l.Profile == preferred {
			if m, ok := meterFor(l, main, now); ok {
				return m
			}
		}
	}
	for _, l := range limits {
		if m, ok := meterFor(l, main, now); ok {
			return m
		}
	}
	return DockMeter{}
}

// meterFor reads one profile's window. The colour is the notch's, computed the
// way buildSlot computes it, so the two can never grade one reading differently.
func meterFor(l services.ProfileLimits, main string, now time.Time) (DockMeter, bool) {
	if !l.Available {
		return DockMeter{}, false
	}
	for _, w := range l.Windows {
		if w.Key != main {
			continue
		}
		used := usedNow(w, now)
		return DockMeter{
			Shown:     true,
			Remaining: int((1-FillFraction(used))*100 + 0.5),
			RGB:       NotchColor(int(100 - used)),
		}, true
	}
	return DockMeter{}, false
}

// DockTile holds the meter on screen and only redraws when it changes. Refreshes
// arrive every minute and on every file change, and each redraw makes the Dock
// re-render the tile.
type DockTile struct {
	mu    sync.Mutex
	shown DockMeter
	draw  func(DockMeter)
}

// NewDockTile returns a tile showing the plain icon.
func NewDockTile() *DockTile { return &DockTile{draw: drawDockMeter} }

// Set shows m, or the plain icon for the zero value.
func (d *DockTile) Set(m DockMeter) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if m == d.shown {
		return
	}
	d.shown = m
	d.draw(m)
}

func drawDockMeter(m DockMeter) {
	if !m.Shown {
		C.CCPMDockRestore()
		return
	}
	C.CCPMDockShowMeter(C.double(m.Remaining)/100, C.uint(m.RGB))
}
