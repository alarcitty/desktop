//go:build windows

package screen

import (
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
)

var (
	user32                        = syscall.NewLazyDLL("user32.dll")
	getSystemMetrics              = user32.NewProc("GetSystemMetrics")
	systemParametersInfo          = user32.NewProc("SystemParametersInfoW")
	findWindowW                   = user32.NewProc("FindWindowW")
	setWindowCompositionAttribute = user32.NewProc("SetWindowCompositionAttribute")
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

// remove blur in corners by replacing ACCENT_ENABLE_BLURBEHIND with pure transparency
func ClearWindowBlur(className, title string) bool {
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
