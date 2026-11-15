package mila

import (
	"context"
	"encoding/json"
	"fmt"
	"runtime"
	"strings"
	"sync/atomic"
	"time"

	wailsRuntime "github.com/wailsapp/wails/v2/pkg/runtime"

	"mila/internal/logger"
	"mila/internal/screen"
)

type Mila struct {
	ctx             context.Context
	log             *logger.Logger
	isFullscreen    atomic.Bool
	isPip           atomic.Bool
	runtimeStarted  atomic.Bool
	windowStatePath string
	emitCloseEvent  func(ctx context.Context, eventName string)
	captureGeometry func(ctx context.Context) (screen.WindowGeometry, bool)
}

// make_unique mila instance
func New() *Mila {
	statePath, _ := screen.DefaultWindowStatePath()
	m := &Mila{
		log:             logger.Make("mila"),
		windowStatePath: statePath,
	}
	m.emitCloseEvent = func(ctx context.Context, eventName string) {
		if ctx == nil || !m.runtimeStarted.Load() {
			return
		}
		wailsRuntime.EventsEmit(ctx, eventName)
	}
	m.captureGeometry = func(ctx context.Context) (screen.WindowGeometry, bool) {
		if geom, ok := screen.CurrentWindowGeometry("milaWindow", "mila"); ok {
			return geom, true
		}
		if ctx == nil || !m.runtimeStarted.Load() {
			return screen.WindowGeometry{}, false
		}
		w, h := wailsRuntime.WindowGetSize(ctx)
		x, y := wailsRuntime.WindowGetPosition(ctx)
		maximized := wailsRuntime.WindowIsMaximised(ctx)
		return screen.NormalizeWindowGeometry(screen.WindowGeometry{
			X:         x,
			Y:         y,
			Width:     w,
			Height:    h,
			Maximized: maximized,
		})
	}
	return m
}

// wails startup hook
func (m *Mila) Startup(ctx context.Context) {
	m.ctx = ctx
	m.runtimeStarted.Store(true)
	m.log.Info("runtime.init", "go_os", runtime.GOOS, "arch", runtime.GOARCH, "cpus", runtime.NumCPU())

	screens, err := wailsRuntime.ScreenGetAll(ctx)
	if err == nil && len(screens) > 0 {
		for i, s := range screens {
			if s.IsCurrent || s.IsPrimary {
				sw := s.Size.Width
				sh := s.Size.Height
				if sw == 0 || sh == 0 {
					sw = s.Width
					sh = s.Height
				}
				if sw > 0 && sh > 0 {
					// 3/4 of screen size
					w := (sw * 3) / 4
					h := (sh * 3) / 4
					wailsRuntime.WindowSetSize(ctx, w, h)
					wailsRuntime.WindowSetMinSize(ctx, 858, 651)
					wailsRuntime.WindowCenter(ctx)

					m.log.Ok("display.ready",
						"index", i,
						"screen", fmt.Sprintf("%dx%d", sw, sh),
						"window", fmt.Sprintf("%dx%d", w, h),
						"radius", 15,
					)
				}
				break
			}
		}
	} else {
		m.log.Warn("display.fallback", "screens", len(screens), "err", err)
	}

	// clear dwm blur to achieve pure transparent corners
	go func() {
		for i := 0; i < 15; i++ {
			if screen.ClearWindowBlur("milaWindow", "mila") {
				m.log.Ok("window.blur_cleared", "attempt", i+1)
				return
			}
			time.Sleep(25 * time.Millisecond)
		}
	}()
}

// BeforeClose notifies the frontend to flush unsaved progress, persists window geometry, and flushes logs.
func (m *Mila) BeforeClose(ctx context.Context) (prevent bool) {
	if ctx == nil {
		ctx = m.ctx
	}
	if m.emitCloseEvent != nil {
		m.emitCloseEvent(ctx, "mila:before-close")
	}
	if m.captureGeometry != nil {
		if geom, ok := m.captureGeometry(ctx); ok {
			if err := screen.SaveWindowGeometry(m.windowStatePath, geom); err != nil {
				m.log.Warn("window.state_save_failed", "err", err.Error())
			} else {
				m.log.Info("window.state_saved", "width", geom.Width, "height", geom.Height, "maximized", geom.Maximized)
			}
		}
	}
	_ = logger.FlushDefaultSink()
	return false
}

// Shutdown finalizes background logging when the desktop host terminates.
func (m *Mila) Shutdown(ctx context.Context) {
	m.log.Info("runtime.shutdown")
	_ = logger.CloseDefaultSink()
}

// SaveWindowState persists explicit window placement metrics into local JSON config.
func (m *Mila) SaveWindowState(x, y, width, height int, maximized bool) error {
	return screen.SaveWindowGeometry(m.windowStatePath, screen.WindowGeometry{
		X:         x,
		Y:         y,
		Width:     width,
		Height:    height,
		Maximized: maximized,
	})
}

// ExportDiagnosticsLogs exports sanitized rotating log archives to a ZIP bundle and returns a `{data}` envelope.
func (m *Mila) ExportDiagnosticsLogs(destDir string) (string, error) {
	_ = logger.FlushDefaultSink()
	report, err := logger.ExportDiagnosticsZip(strings.TrimSpace(destDir))
	if err != nil {
		return `{"error":"failed to export diagnostic logs"}`, err
	}
	raw, err := json.Marshal(map[string]any{"data": report})
	if err != nil {
		return `{"error":"failed to serialize report"}`, err
	}
	return string(raw), nil
}

// GetContractInfo exposes the unified Go<->JS contract descriptor for parity with HTTP `/health`.
func (m *Mila) GetContractInfo() (string, error) {
	raw, err := json.Marshal(map[string]any{
		"data": map[string]any{
			"status":       "ok",
			"mode":         "wails",
			"apiBase":      "/api/v1",
			"extBase":      "/ext",
			"capabilities": []string{"anime", "shikimori", "image", "providers", "torrent", "diagnostics"},
		},
	})
	if err != nil {
		return `{"error":"marshal failed"}`, err
	}
	return string(raw), nil
}

// SetFullscreen toggles true OS monitor fullscreen (covering the taskbar).
func (m *Mila) SetFullscreen(fullscreen bool) {
	if m.ctx == nil {
		return
	}
	m.isFullscreen.Store(fullscreen)
	if fullscreen {
		wailsRuntime.WindowFullscreen(m.ctx)
		screen.SetWindowFullscreen("milaWindow", "mila", true)
	} else {
		screen.SetWindowFullscreen("milaWindow", "mila", false)
		wailsRuntime.WindowUnfullscreen(m.ctx)
		screen.ClearWindowBlur("milaWindow", "mila")
	}
}

// SetFullscreenFocus adjusts topmost state on Alt+Tab while in fullscreen so the taskbar stays hidden only when active.
func (m *Mila) SetFullscreenFocus(focused bool) {
	if m.ctx == nil || !m.isFullscreen.Load() || m.isPip.Load() {
		return
	}
	screen.SetWindowTopmost("milaWindow", "mila", focused)
}

// SetPipMode transforms the native desktop window into an Always-On-Top rounded 16:9 Picture-in-Picture window over all OS apps.
func (m *Mila) SetPipMode(pip bool) {
	if m.ctx == nil {
		return
	}
	if pip {
		if m.isFullscreen.Load() {
			m.SetFullscreen(false)
		}
		m.isPip.Store(true)
		if wailsRuntime.WindowIsMaximised(m.ctx) {
			wailsRuntime.WindowUnmaximise(m.ctx)
		}
		wailsRuntime.WindowSetMinSize(m.ctx, 320, 180)
		wailsRuntime.WindowSetAlwaysOnTop(m.ctx, true)
		screen.SetWindowPip("milaWindow", "mila", true)
		return
	}

	m.isPip.Store(false)
	wailsRuntime.WindowSetAlwaysOnTop(m.ctx, false)
	screen.SetWindowPip("milaWindow", "mila", false)
	wailsRuntime.WindowSetMinSize(m.ctx, 858, 651)
}

// StartPipDrag initiates a native OS drag on the Picture-in-Picture window.
func (m *Mila) StartPipDrag() {
	if m.ctx == nil || !m.isPip.Load() {
		return
	}
	screen.StartWindowDrag("milaWindow", "mila")
}
