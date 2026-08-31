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
		OnStartup:        app.startup,
		OnShutdown:       app.shutdown,
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
			updater,
		},
	})

	if err != nil {
		println("Error:", err.Error())
	}
}
