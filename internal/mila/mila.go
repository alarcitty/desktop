package mila

import (
	"context"
	"fmt"
	"runtime"
	"sync/atomic"
	"time"

	wailsRuntime "github.com/wailsapp/wails/v2/pkg/runtime"

	"mila/internal/logger"
	"mila/internal/screen"
)

type Mila struct {
	ctx          context.Context
	log          *logger.Logger
	isFullscreen atomic.Bool
	isPip        atomic.Bool
}

// make_unique mila instance
func New() *Mila {
	return &Mila{
		log: logger.Make("mila"),
	}
}

// wails startup hook
func (m *Mila) Startup(ctx context.Context) {
	m.ctx = ctx
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
