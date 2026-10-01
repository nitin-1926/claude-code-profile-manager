//go:build darwin

package main

import (
	"embed"

	"github.com/nitin-1926/claude-code-profile-manager/ccpm/desktop/services"
	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/mac"
)

//go:embed all:frontend/dist
var assets embed.FS

func main() {
	updater := services.NewUpdater()
	app := NewApp(updater)
	profiles := services.NewProfiles()
	cascade := services.NewCascade()
	usage := services.NewUsage()
	health := services.NewHealth()
	mutate := services.NewMutate()
	details := services.NewDetails()
	settings := services.NewSettings()
	limits := services.NewLimits()
	prefs := services.NewPrefs()
	// Every preference write reshapes the rail. Wiring it here rather than
	// making the frontend ask for a reshape after each write removes the only
	// way the two can drift apart.
	prefs.OnChange = app.applyRailPrefs
	history := services.NewHistory()
	statusline := services.NewStatusLine()

	err := wails.Run(&options.App{
		Title:     "CCPM",
		Width:     1180,
		Height:    760,
		MinWidth:  920,
		MinHeight: 600,
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		// darkmatter dark background (oklch(0.1797 0.0043 308) ≈ #161519)
		BackgroundColour: &options.RGBA{R: 22, G: 21, B: 25, A: 1},
		// The rail outlives the main window, so closing that window must stop
		// quitting the app. Wails' WindowDelegate sends "Q" to Go on close
		// unless this is set. Its partner is canHide=NO on the panel: the
		// hide-on-close path calls [NSApp hide:], which would otherwise take
		// the rail down with the window.
		HideWindowOnClose: true,
		OnStartup:         app.startup,
		OnShutdown:        app.shutdown,
		// A second launch focuses the running window instead of starting another
		// app. Belt-and-braces after the findCCPM fork bomb: if anything ever
		// execs this binary again, the OS gets one extra process that exits
		// immediately, not a tree of them.
		SingleInstanceLock: &options.SingleInstanceLock{
			UniqueId:               "dev.ccpm.desktop",
			OnSecondInstanceLaunch: app.onSecondInstanceLaunch,
		},
		Mac: &mac.Options{
			TitleBar:             mac.TitleBarHiddenInset(),
			WebviewIsTransparent: false,
			WindowIsTranslucent:  false,
		},
		Bind: []interface{}{
			app,
			profiles,
			cascade,
			usage,
			health,
			mutate,
			details,
			settings,
			limits,
			prefs,
			history,
			statusline,
			updater,
		},
	})

	if err != nil {
		println("Error:", err.Error())
	}
}
