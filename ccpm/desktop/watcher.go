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
	for _, root := range []string{base, filepath.Join(home, ".claude")} {
		addTree(w, root, 3)
	}
	go a.watchLoop()
}

// addTree watches root and its subdirectories up to maxDepth, skipping noisy /
// heavy directories. fsnotify isn't recursive, so we enumerate up front and
// add new directories lazily in the loop.
func addTree(w *fsnotify.Watcher, root string, maxDepth int) {
	base := strings.Count(root, string(os.PathSeparator))
	_ = filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil || !d.IsDir() {
			return nil
		}
		if strings.Count(p, string(os.PathSeparator))-base > maxDepth {
			return filepath.SkipDir
		}
		switch d.Name() {
		case ".git", "node_modules", "projects": // projects = transcripts, huge + irrelevant to config
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
			// pick up newly created directories so future changes inside them fire too
			if ev.Op&fsnotify.Create != 0 {
				if fi, err := os.Stat(ev.Name); err == nil && fi.IsDir() {
					_ = a.watcher.Add(ev.Name)
				}
			}
			if timer != nil {
				timer.Stop()
			}
			timer = time.AfterFunc(300*time.Millisecond, func() {
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
