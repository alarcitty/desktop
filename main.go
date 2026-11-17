package main

import (
	"embed"
	"fmt"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/windows"

	"mila/internal/logger"
	"mila/internal/mila"
	"mila/internal/screen"
)

//go:embed all:frontend/dist
var assets embed.FS

func main() {
	// make_unique logger and app instance
	_ = logger.EnableDefaultFileLogging()
	defer func() {
		_ = logger.CloseDefaultSink()
	}()

	_ = screen.SetAppUserModelID("com.mila.app")

	log := logger.Make("boot")
	m := mila.New()

	// 3/4 of work area
	width, height := screen.ThreeQuarterSize()
	log.Info("window.geometry", "size", fmt.Sprintf("%dx%d", width, height), "radius", 15)

	err := wails.Run(&options.App{
		Title:     "Mila",
		Width:     width,
		Height:    height,
		MinWidth:  858,
		MinHeight: 651,
		Frameless: true,
		AssetServer: &assetserver.Options{
			Assets:  assets,
			Handler: mila.NewProviderHandler(),
		},
		BackgroundColour: &options.RGBA{R: 0, G: 0, B: 0, A: 0}, // transparent for rounded corners
		OnStartup:        m.Startup,
		OnBeforeClose:    m.BeforeClose,
		OnShutdown:       m.Shutdown,
		Bind: []interface{}{
			m,
		},
		Windows: &windows.Options{
			WindowClassName:                   "MilaWindow",
			WebviewIsTransparent:              true,
			WindowIsTranslucent:               true, // enables WS_EX_NOREDIRECTIONBITMAP and true alpha
			DisableFramelessWindowDecorations: true, // removes DWM black rectangular frame
		},
	})

	if err != nil {
		log.Error("app.crashed", "err", err.Error())
	}
}
