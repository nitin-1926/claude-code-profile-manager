//go:build darwin

package main

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/fsnotify/fsnotify"

	"github.com/nitin-1926/claude-code-profile-manager/ccpm/internal/config"
	"github.com/nitin-1926/claude-code-profile-manager/ccpm/internal/transcript"
	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// changeEvent is the frontend signal to refetch. Emitted (debounced) whenever
// anything under ~/.ccpm or ~/.claude changes — so a CLI edit in another
// terminal reflects in the GUI without a manual refresh.
const changeEvent = "ccpm:changed"

func (a *App) startWatcher() {
	w, err := fsnotify.NewWatcher()
	if err != nil {
		return
	}
	a.watcher = w

	home, _ := os.UserHomeDir()
	base, _ := config.BaseDir()
	a.watchRoots = []string{base, filepath.Join(home, ".claude")}
	for _, root := range a.watchRoots {
		addTree(w, a.watchRoots, root)
	}
	go a.watchLoop()
}

// watchDepth is how many levels below a root are watched.
const watchDepth = 3

// skipDirs are noisy / heavy directories never watched, at any depth.
var skipDirs = map[string]bool{
	".git":         true,
	"node_modules": true,
	"projects":     true, // transcripts, huge + irrelevant to config
}

// shouldWatch reports whether directory p belongs in the watch set: under one
// of roots, within watchDepth of it, and not inside a skipped directory.
func shouldWatch(roots []string, p string) bool {
	for _, root := range roots {
		rel, err := filepath.Rel(root, p)
		if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
			continue
		}
		if rel == "." {
			return true
		}
		parts := strings.Split(rel, string(os.PathSeparator))
		if len(parts) > watchDepth {
			return false
		}
		for _, part := range parts {
			if skipDirs[part] {
				return false
			}
		}
		return true
	}
	return false
}

// addTree watches start and every directory beneath it that shouldWatch
// admits. fsnotify isn't recursive, so this runs over each root up front and
// again, lazily, for every directory created while the app runs.
func addTree(w *fsnotify.Watcher, roots []string, start string) {
	_ = filepath.WalkDir(start, func(p string, d fs.DirEntry, err error) error {
		if err != nil || !d.IsDir() {
			return nil
		}
		if !shouldWatch(roots, p) {
			return filepath.SkipDir
		}
		_ = w.Add(p)
		return nil
	})
}

func (a *App) watchLoop() {
	var timer *time.Timer
	for {
		select {
		case ev, ok := <-a.watcher.Events:
			if !ok {
				return
			}
			// The history sidecar is written BY a History fetch, inside the
			// watched tree. Without this exclusion an append to any transcript
			// produced a write here, a ccpm:changed event, a refetch, and a
			// second full scan. IsIndexFile also covers atomicwrite's staged
			// sibling, which is the name the write actually arrives under.
			if transcript.IsIndexFile(ev.Name) {
				continue
			}
			// pick up newly created directories so future changes inside them fire too
			if ev.Op&fsnotify.Create != 0 {
				// through the same filter as the initial walk, or a profile created
				// while the app runs gets its projects/ watched.
				addTree(a.watcher, a.watchRoots, ev.Name)
			}
			if timer != nil {
				timer.Stop()
			}
			timer = time.AfterFunc(300*time.Millisecond, func() {
				// The notch is native, so the frontend event does not reach it.
				// Without this its rings kept the usage they launched with:
				// the status line rewrites limits.json and nothing redrew.
				a.ApplyRailPrefs()
				if a.ctx != nil {
					runtime.EventsEmit(a.ctx, changeEvent)
				}
			})
		case _, ok := <-a.watcher.Errors:
			if !ok {
				return
			}
		}
	}
}
