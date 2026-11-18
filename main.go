package main

import (
	"embed"
	"fmt"
	"os"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/windows"

	"mila/internal/logger"
	"mila/internal/mila"
	"mila/internal/protocol"
	"mila/internal/screen"
)

//go:embed all:frontend/dist
var assets embed.FS

//go:embed build/windows/icon.ico
var trayIconICO []byte

func main() {
	// make_unique logger and app instance
	_ = logger.EnableDefaultFileLogging()
	defer func() {
		_ = logger.CloseDefaultSink()
	}()

	_ = screen.SetAppUserModelID("com.mila.app")

	log := logger.Make("boot")
	m := mila.New()
	m.SetTrayIcon(trayIconICO)

	if exePath, exeErr := os.Executable(); exeErr == nil {
		if regErr := protocol.Register(exePath); regErr != nil {
			log.Warn("protocol.register_failed", "err", regErr.Error())
		} else {
			log.Info("protocol.registered", "scheme", "mila")
		}
	}
	m.SetPendingDeepLink(protocol.FindLink(os.Args[1:]))

	// Restore persisted multi-monitor-clamped geometry if available; otherwise use 3/4 work area.
	width, height := screen.ThreeQuarterSize()
	startState := options.Normal
	if restored, ok := screen.LoadClampedWindowGeometry(""); ok {
		width = restored.Width
		height = restored.Height
		if restored.Maximized {
			startState = options.Maximised
		}
		log.Info("window.geometry_restored", "x", restored.X, "y", restored.Y, "size", fmt.Sprintf("%dx%d", width, height), "maximized", restored.Maximized)
	} else {
		log.Info("window.geometry", "size", fmt.Sprintf("%dx%d", width, height), "radius", 15)
	}

	err := wails.Run(&options.App{
		Title:            "Mila",
		Width:            width,
		Height:           height,
		MinWidth:         858,
		MinHeight:        651,
		WindowStartState: startState,
		Frameless:        true,
		AssetServer: &assetserver.Options{
			Assets:  assets,
			Handler: mila.NewProviderHandler(),
		},
		BackgroundColour: &options.RGBA{R: 0, G: 0, B: 0, A: 0}, // transparent for rounded corners
		SingleInstanceLock: &options.SingleInstanceLock{
			UniqueId: "com.mila.app",
			OnSecondInstanceLaunch: func(data options.SecondInstanceData) {
				m.HandleDeepLink(protocol.FindLink(data.Args))
			},
		},
		OnStartup:     m.Startup,
		OnBeforeClose: m.BeforeClose,
		OnShutdown:    m.Shutdown,
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
