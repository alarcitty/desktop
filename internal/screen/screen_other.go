//go:build !windows

package screen

// fallback display size
func Dimensions() (int, int) {
	return 1280, 720
}

// 3/4 fallback size
func ThreeQuarterSize() (int, int) {
	return (1280 * 3) / 4, (720 * 3) / 4
}

// no-op on non-windows
func ClearWindowBlur(className, title string) bool {
	return true
}

// SetWindowFullscreen ensures the native HWND covers the entire physical monitor (no-op on non-windows).
func SetWindowFullscreen(className, title string, fullscreen bool) bool {
	return true
}

// SetWindowTopmost toggles TOPMOST vs NOTOPMOST (no-op on non-windows).
func SetWindowTopmost(className, title string, topmost bool) bool {
	return true
}

// SetWindowPip toggles OS-level Picture-in-Picture window mode (no-op on non-windows).
func SetWindowPip(className, title string, pip bool) bool {
	return true
}

// StartWindowDrag initiates a native OS window drag (no-op on non-windows).
func StartWindowDrag(className, title string) bool {
	return true
}
