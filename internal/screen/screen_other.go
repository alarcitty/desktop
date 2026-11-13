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
