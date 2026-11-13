//go:build !windows

package logger

func init() {
	// posix terminals have ansi color support
	colorsSupported = true
}
