//go:build darwin

package rail

/*
#include <stdlib.h>
#include "tray.h"
*/
import "C"

import (
	"encoding/json"
	"sync/atomic"
	"unsafe"
)

// Tray is the menu bar item. Wails v2 has no tray API, so this is an
// NSStatusItem built the way the notch is: Go owns the content, the C layer
// draws it and reports clicks, and every call is safe from any goroutine.
type Tray struct {
	run  func(profile string)
	show func()
}

// liveTray is the tray clicks are delivered to.
var liveTray atomic.Pointer[Tray]

// NewTray wires the menu's clicks. Nothing appears until SetVisible.
func NewTray(run func(profile string), show func()) *Tray {
	t := &Tray{run: run, show: show}
	liveTray.Store(t)
	return t
}

// SetVisible adds or removes the item. Idempotent.
func (t *Tray) SetVisible(on bool) {
	if on {
		C.CCPMTrayShow()
		return
	}
	C.CCPMTrayHide()
}

// SetRows replaces the rows the next menu opening is built from. The C layer
// only caches them: opening the menu reads that cache and never calls back into
// Go, so a slow disk can never freeze the main thread on a click.
func (t *Tray) SetRows(rows []MenuRow) {
	b, err := json.Marshal(rows)
	if err != nil {
		return // same call as pushModelLocked: a bug, but not worth a panic in the UI
	}
	cs := C.CString(string(b))
	defer C.free(unsafe.Pointer(cs))
	C.CCPMTraySetRows(cs)
}

// ccpmTrayRun is called on the main thread when a profile row is clicked. The
// handler runs off it: starting a session can wait on a consent sheet for 30s.
//
//export ccpmTrayRun
func ccpmTrayRun(profile *C.char) {
	name := C.GoString(profile) // a copy; the C side still owns profile
	if t := liveTray.Load(); t != nil {
		go t.run(name)
	}
}

// ccpmTrayShowWindow is called on the main thread by the Show Window row.
//
//export ccpmTrayShowWindow
func ccpmTrayShowWindow() {
	if t := liveTray.Load(); t != nil {
		go t.show()
	}
}
