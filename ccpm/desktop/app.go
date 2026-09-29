//go:build darwin

package main

import (
	"time"

	"context"

	"github.com/fsnotify/fsnotify"
	"github.com/nitin-1926/claude-code-profile-manager/ccpm/desktop/rail"
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
	rail    *rail.Controller
}

// NewApp creates a new App application struct
func NewApp(updater *services.Updater) *App {
	return &App{updater: updater, rail: rail.New()}
}

// startup saves the runtime context, hands it to the updater, and starts the
// freshness watcher.
func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	a.updater.SetContext(ctx)
	a.startWatcher()
	a.startRail()
}

// startRail brings up the floating usage panel according to the stored
// preferences.
//
// Note this runs on a background goroutine — Wails dispatches OnStartup off the
// main thread while [NSApp run] holds it — so nothing here may touch AppKit
// directly. The rail package hops to the main queue internally for exactly
// this reason.
func (a *App) startRail() {
	if a.rail == nil {
		return
	}
	a.rail.Start()
	a.ApplyRailPrefs()

	// Once a minute too: reset countdowns and staleness move with the clock
	// even when no file changes, and a window that has reset should stop
	// showing its old usage without waiting for the next status line write.
	go func() {
		t := time.NewTicker(time.Minute)
		defer t.Stop()
		for {
			select {
			case <-a.ctx.Done():
				return
			case <-t.C:
				a.ApplyRailPrefs()
			}
		}
	}()
}

// ApplyRailPrefs re-reads the preferences and reshapes the rail. Exported so
// the frontend can call it the moment a setting changes, rather than waiting
// for the next watcher tick.
func (a *App) ApplyRailPrefs() {
	if a.rail == nil {
		return
	}
	prefs := services.LoadPrefs()

	limits, err := services.NewLimits().All()
	if err != nil {
		limits = nil
	}
	shown := make([]services.ProfileLimits, 0, len(limits))
	for _, l := range limits {
		if prefs.RailEnabled(l.Profile) {
			shown = append(shown, l)
		}
	}

	a.rail.SetLayout(rail.ParseEdge(prefs.RailEdge), len(shown), prefs.RailPercent)
	a.rail.SetModel(rail.BuildModel(shown, prefs.Theme, prefs.RailMain, time.Now()))
	a.rail.SetVisibility(notchVisibility(prefs))
	// Hover mode still shows the panel; the reveal is what hover drives. The
	// master switch is the only thing that takes it off screen entirely.
	a.rail.SetVisible(prefs.RailOn && len(shown) > 0)
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
	if a.rail != nil {
		a.rail.Stop()
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

// notchVisibility maps the stored preferences onto the notch's tri-state. The
// notch owns the whole reveal decision now, so it needs to know about "always"
// as its own mode rather than inferring it from a hover flag being off. Off is
// the master switch, whatever reveal it is keeping for later.
func notchVisibility(p services.DesktopPrefs) rail.Visibility {
	switch {
	case !p.RailOn:
		return rail.VisibilityHidden
	case p.RailMode == services.RailModeAlways:
		return rail.VisibilityAlways
	default:
		return rail.VisibilityHover
	}
}
