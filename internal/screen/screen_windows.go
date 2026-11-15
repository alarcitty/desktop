//go:build windows

package screen

import (
	"runtime"
	"syscall"
	"unsafe"
)

type rect struct {
	left   int32
	top    int32
	right  int32
	bottom int32
}

type accentPolicy struct {
	accentState   uint32
	accentFlags   uint32
	gradientColor uint32
	animationId   uint32
}

type windowCompositionAttribData struct {
	attrib uint32
	pvData unsafe.Pointer
	cbData uintptr
}

const (
	spiGetWorkArea                  = 0x0030
	wcaAccentPolicy                 = 19
	accentEnableTransparentGradient = 2

	gwlStyle           = -16
	wsPopup            = 0x80000000
	wsVisible          = 0x10000000
	wsOverlappedWindow = 0x00CF0000
	wsMaximize         = 0x01000000

	monitorDefaultToNearest = 2

	swpNoSize        = 0x0001
	swpNoMove        = 0x0002
	swpNoActivate    = 0x0010
	swpNoOwnerZOrder = 0x0200
	swpFrameChanged  = 0x0020
	swpShowWindow    = 0x0040

	dwmwaWindowCornerPreference = 33
	dwmwcpDefault               = 0
	dwmwcpDoNotRound            = 1
	dwmwcpRound                 = 2

	wmNclButtonDown = 0x00A1
	htCaption       = 2

	coinitApartmentThreaded = 0x2
	clsctxInprocServer      = 0x1
)

var (
	hwndTopmost   = ^uintptr(0) // (HWND)-1
	hwndNotopmost = ^uintptr(1) // (HWND)-2

	savedPipRect    rect
	hasSavedPipRect bool
)

type monitorInfo struct {
	cbSize    uint32
	rcMonitor rect
	rcWork    rect
	dwFlags   uint32
}

type guid struct {
	data1 uint32
	data2 uint16
	data3 uint16
	data4 [8]byte
}

type iTaskbarList2Vtbl struct {
	QueryInterface       uintptr
	AddRef               uintptr
	Release              uintptr
	HrInit               uintptr
	AddTab               uintptr
	DeleteTab            uintptr
	ActivateTab          uintptr
	SetActiveAlt         uintptr
	MarkFullscreenWindow uintptr
}

type iTaskbarList2 struct {
	lpVtbl *iTaskbarList2Vtbl
}

var (
	user32                        = syscall.NewLazyDLL("user32.dll")
	ole32                         = syscall.NewLazyDLL("ole32.dll")
	dwmapi                        = syscall.NewLazyDLL("dwmapi.dll")
	getSystemMetrics              = user32.NewProc("GetSystemMetrics")
	systemParametersInfo          = user32.NewProc("SystemParametersInfoW")
	findWindowW                   = user32.NewProc("FindWindowW")
	setWindowCompositionAttribute = user32.NewProc("SetWindowCompositionAttribute")
	getWindowLongW                = user32.NewProc("GetWindowLongW")
	setWindowLongW                = user32.NewProc("SetWindowLongW")
	monitorFromWindow             = user32.NewProc("MonitorFromWindow")
	getMonitorInfoW               = user32.NewProc("GetMonitorInfoW")
	getWindowRect                 = user32.NewProc("GetWindowRect")
	setWindowPos                  = user32.NewProc("SetWindowPos")
	setForegroundWindow           = user32.NewProc("SetForegroundWindow")
	releaseCapture                = user32.NewProc("ReleaseCapture")
	postMessageW                  = user32.NewProc("PostMessageW")
	coInitializeEx                = ole32.NewProc("CoInitializeEx")
	coUninitialize                = ole32.NewProc("CoUninitialize")
	coCreateInstance              = ole32.NewProc("CoCreateInstance")
	dwmSetWindowAttribute         = dwmapi.NewProc("DwmSetWindowAttribute")

	clsidTaskbarList = guid{
		data1: 0x56FDF344,
		data2: 0xFD6D,
		data3: 0x11D0,
		data4: [8]byte{0x95, 0x8A, 0x00, 0x60, 0x97, 0xC9, 0xA0, 0x90},
	}
	iidITaskbarList2 = guid{
		data1: 0x602D4995,
		data2: 0xB13A,
		data3: 0x429B,
		data4: [8]byte{0xA6, 0x6E, 0x19, 0x35, 0xE4, 0x4F, 0x43, 0x17},
	}
)

// get work area size minus taskbar
func Dimensions() (int, int) {
	var r rect
	ret, _, _ := systemParametersInfo.Call(
		uintptr(spiGetWorkArea),
		0,
		uintptr(unsafe.Pointer(&r)),
		0,
	)
	if ret != 0 {
		w := int(r.right - r.left)
		h := int(r.bottom - r.top)
		if w > 0 && h > 0 {
			return w, h
		}
	}

	w, _, _ := getSystemMetrics.Call(0) // SM_CXSCREEN
	h, _, _ := getSystemMetrics.Call(1) // SM_CYSCREEN
	if w > 0 && h > 0 {
		return int(w), int(h)
	}

	return 1280, 720
}

// 3/4 of work area
func ThreeQuarterSize() (int, int) {
	w, h := Dimensions()
	return (w * 3) / 4, (h * 3) / 4
}

func findAppWindow(className, title string) uintptr {
	var classPtr, titlePtr *uint16
	if className != "" {
		classPtr, _ = syscall.UTF16PtrFromString(className)
	}
	if title != "" {
		titlePtr, _ = syscall.UTF16PtrFromString(title)
	}

	hwnd, _, _ := findWindowW.Call(
		uintptr(unsafe.Pointer(classPtr)),
		uintptr(unsafe.Pointer(titlePtr)),
	)
	if hwnd == 0 && titlePtr != nil {
		hwnd, _, _ = findWindowW.Call(0, uintptr(unsafe.Pointer(titlePtr)))
	}
	return hwnd
}

func setWindowCornerPreference(hwnd uintptr, preference uint32) {
	if hwnd == 0 {
		return
	}
	_, _, _ = dwmSetWindowAttribute.Call(
		hwnd,
		uintptr(dwmwaWindowCornerPreference),
		uintptr(unsafe.Pointer(&preference)),
		unsafe.Sizeof(preference),
	)
}

func markTaskbarFullscreen(hwnd uintptr, fullscreen bool) {
	if hwnd == 0 {
		return
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()

		hrCom, _, _ := coInitializeEx.Call(0, uintptr(coinitApartmentThreaded))
		if hrCom == 0 || hrCom == 1 {
			defer coUninitialize.Call()
		}

		var obj *iTaskbarList2
		hr, _, _ := coCreateInstance.Call(
			uintptr(unsafe.Pointer(&clsidTaskbarList)),
			0,
			uintptr(clsctxInprocServer),
			uintptr(unsafe.Pointer(&iidITaskbarList2)),
			uintptr(unsafe.Pointer(&obj)),
		)
		if hr != 0 || obj == nil || obj.lpVtbl == nil {
			return
		}
		defer syscall.SyscallN(obj.lpVtbl.Release, uintptr(unsafe.Pointer(obj)))

		hrInit, _, _ := syscall.SyscallN(obj.lpVtbl.HrInit, uintptr(unsafe.Pointer(obj)))
		if hrInit != 0 {
			return
		}

		var flag uintptr
		if fullscreen {
			flag = 1
		}
		_, _, _ = syscall.SyscallN(obj.lpVtbl.MarkFullscreenWindow, uintptr(unsafe.Pointer(obj)), hwnd, flag)
	}()
	<-done
}

// SetWindowFullscreen ensures the native HWND covers the entire physical monitor (including the taskbar).
func SetWindowFullscreen(className, title string, fullscreen bool) bool {
	hwnd := findAppWindow(className, title)
	if hwnd == 0 {
		return false
	}

	if fullscreen {
		styleIdx := int32(gwlStyle)
		style, _, _ := getWindowLongW.Call(hwnd, uintptr(styleIdx))
		cleanStyle := (uint32(style) & ^uint32(wsOverlappedWindow|wsMaximize)) | wsPopup | wsVisible
		_, _, _ = setWindowLongW.Call(hwnd, uintptr(styleIdx), uintptr(cleanStyle))
		setWindowCornerPreference(hwnd, dwmwcpDoNotRound)

		monitor, _, _ := monitorFromWindow.Call(hwnd, uintptr(monitorDefaultToNearest))
		if monitor != 0 {
			mi := monitorInfo{cbSize: uint32(unsafe.Sizeof(monitorInfo{}))}
			ok, _, _ := getMonitorInfoW.Call(monitor, uintptr(unsafe.Pointer(&mi)))
			if ok != 0 {
				w := mi.rcMonitor.right - mi.rcMonitor.left
				h := mi.rcMonitor.bottom - mi.rcMonitor.top
				_, _, _ = setWindowPos.Call(
					hwnd,
					hwndTopmost,
					uintptr(mi.rcMonitor.left),
					uintptr(mi.rcMonitor.top),
					uintptr(w),
					uintptr(h),
					uintptr(swpNoOwnerZOrder|swpFrameChanged|swpShowWindow),
				)
			}
		}
		_, _, _ = setForegroundWindow.Call(hwnd)
		markTaskbarFullscreen(hwnd, true)
		return true
	}

	_, _, _ = setWindowPos.Call(
		hwnd,
		hwndNotopmost,
		0,
		0,
		0,
		0,
		uintptr(swpNoMove|swpNoSize|swpNoOwnerZOrder|swpNoActivate),
	)
	setWindowCornerPreference(hwnd, dwmwcpDefault)
	markTaskbarFullscreen(hwnd, false)
	return true
}

// SetWindowTopmost toggles TOPMOST vs NOTOPMOST without moving or activating, so Alt+Tab works cleanly in fullscreen.
func SetWindowTopmost(className, title string, topmost bool) bool {
	hwnd := findAppWindow(className, title)
	if hwnd == 0 {
		return false
	}
	zOrder := hwndNotopmost
	if topmost {
		zOrder = hwndTopmost
	}
	_, _, _ = setWindowPos.Call(
		hwnd,
		zOrder,
		0,
		0,
		0,
		0,
		uintptr(swpNoMove|swpNoSize|swpNoOwnerZOrder|swpNoActivate),
	)
	return true
}

// SetWindowPip switches the native HWND into an Always-On-Top rounded 16:9 Picture-in-Picture window over all OS apps, or restores its previous bounds.
func SetWindowPip(className, title string, pip bool) bool {
	hwnd := findAppWindow(className, title)
	if hwnd == 0 {
		return false
	}

	if pip {
		var cur rect
		if ok, _, _ := getWindowRect.Call(hwnd, uintptr(unsafe.Pointer(&cur))); ok != 0 {
			if cur.right-cur.left > 600 && cur.bottom-cur.top > 400 {
				savedPipRect = cur
				hasSavedPipRect = true
			}
		}

		setWindowCornerPreference(hwnd, dwmwcpRound)
		ClearWindowBlur(className, title)

		pipW := int32(480)
		pipH := int32(270)
		posX := int32(64)
		posY := int32(64)

		monitor, _, _ := monitorFromWindow.Call(hwnd, uintptr(monitorDefaultToNearest))
		if monitor != 0 {
			mi := monitorInfo{cbSize: uint32(unsafe.Sizeof(monitorInfo{}))}
			if ok, _, _ := getMonitorInfoW.Call(monitor, uintptr(unsafe.Pointer(&mi))); ok != 0 {
				workW := mi.rcWork.right - mi.rcWork.left
				scaledW := (workW * 25) / 100
				if scaledW < 420 {
					scaledW = 420
				} else if scaledW > 560 {
					scaledW = 560
				}
				pipW = scaledW
				pipH = (pipW * 9) / 16
				posX = mi.rcWork.right - pipW - 24
				posY = mi.rcWork.bottom - pipH - 24
			}
		}

		_, _, _ = setWindowPos.Call(
			hwnd,
			hwndTopmost,
			uintptr(posX),
			uintptr(posY),
			uintptr(pipW),
			uintptr(pipH),
			uintptr(swpNoOwnerZOrder|swpFrameChanged|swpShowWindow),
		)
		return true
	}

	setWindowCornerPreference(hwnd, dwmwcpRound)
	ClearWindowBlur(className, title)

	if hasSavedPipRect {
		w := savedPipRect.right - savedPipRect.left
		h := savedPipRect.bottom - savedPipRect.top
		_, _, _ = setWindowPos.Call(
			hwnd,
			hwndNotopmost,
			uintptr(savedPipRect.left),
			uintptr(savedPipRect.top),
			uintptr(w),
			uintptr(h),
			uintptr(swpNoOwnerZOrder|swpFrameChanged|swpShowWindow),
		)
		_, _, _ = setForegroundWindow.Call(hwnd)
		return true
	}

	_, _, _ = setWindowPos.Call(
		hwnd,
		hwndNotopmost,
		0,
		0,
		0,
		0,
		uintptr(swpNoMove|swpNoSize|swpNoOwnerZOrder|swpNoActivate),
	)
	return true
}

// StartWindowDrag initiates a native OS window drag on the HWND while the left mouse button is held down.
func StartWindowDrag(className, title string) bool {
	hwnd := findAppWindow(className, title)
	if hwnd == 0 {
		return false
	}
	_, _, _ = releaseCapture.Call()
	_, _, _ = postMessageW.Call(hwnd, uintptr(wmNclButtonDown), uintptr(htCaption), 0)
	return true
}

// CurrentWindowGeometry reads current HWND coordinates, dimensions, and maximize flag.
func CurrentWindowGeometry(className, title string) (WindowGeometry, bool) {
	hwnd := findAppWindow(className, title)
	if hwnd == 0 {
		return WindowGeometry{}, false
	}
	var wr rect
	ok, _, _ := getWindowRect.Call(hwnd, uintptr(unsafe.Pointer(&wr)))
	if ok == 0 {
		return WindowGeometry{}, false
	}
	style, _, _ := getWindowLongW.Call(hwnd, ^uintptr(15)) // GWL_STYLE = -16
	maximized := (style & uintptr(wsMaximize)) != 0

	return NormalizeWindowGeometry(WindowGeometry{
		X:         int(wr.left),
		Y:         int(wr.top),
		Width:     int(wr.right - wr.left),
		Height:    int(wr.bottom - wr.top),
		Maximized: maximized,
	})
}

// remove blur in corners by replacing ACCENT_ENABLE_BLURBEHIND with pure transparency
func ClearWindowBlur(className, title string) bool {
	hwnd := findAppWindow(className, title)
	if hwnd == 0 {
		return false
	}

	accent := accentPolicy{
		accentState:   accentEnableTransparentGradient,
		accentFlags:   2,
		gradientColor: 0x00000000,
	}
	data := windowCompositionAttribData{
		attrib: wcaAccentPolicy,
		pvData: unsafe.Pointer(&accent),
		cbData: unsafe.Sizeof(accent),
	}
	ret, _, _ := setWindowCompositionAttribute.Call(hwnd, uintptr(unsafe.Pointer(&data)))
	return ret != 0
}
