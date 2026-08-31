//go:build darwin

package main

import (
	"context"

	"github.com/fsnotify/fsnotify"
	"github.com/nitin-1926/claude-code-profile-manager/ccpm/desktop/services"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// App holds the Wails runtime context and the filesystem watcher that keeps the
// UI fresh when the CLI / Claude Code mutate profile state underneath it.
type App struct {
	ctx     context.Context
	watcher *fsnotify.Watcher
	updater *services.Updater
}

// NewApp creates a new App application struct
func NewApp(updater *services.Updater) *App {
	return &App{updater: updater}
}

// startup saves the runtime context, hands it to the updater, and starts the
// freshness watcher.
func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	a.updater.SetContext(ctx)
	a.startWatcher()
}

// onSecondInstanceLaunch runs when the single-instance lock turns away another
// launch. Bring the window that is already running to the front so the user
// sees a response rather than a click that seemingly did nothing.
func (a *App) onSecondInstanceLaunch(_ options.SecondInstanceData) {
	if a.ctx == nil {
		return
	}
	runtime.WindowUnminimise(a.ctx)
	runtime.Show(a.ctx)
}

// shutdown tears the watcher down cleanly.
func (a *App) shutdown(_ context.Context) {
	if a.watcher != nil {
		_ = a.watcher.Close()
	}
}

// PickDirectory opens a native directory picker and returns the chosen path
// (empty if cancelled). Used by the Assets tab to add an asset from disk.
func (a *App) PickDirectory() string {
	dir, err := runtime.OpenDirectoryDialog(a.ctx, runtime.OpenDialogOptions{
		Title: "Select an asset directory (skill, agent, command, rule, or hook)",
	})
	if err != nil {
		return ""
	}
	return dir
}
