//go:build windows

package logger

import (
	"os"
	"syscall"
	"testing"
	"unsafe"
)

var (
	kernel32       = syscall.NewLazyDLL("kernel32.dll")
	attachConsole  = kernel32.NewProc("AttachConsole")
	getConsoleMode = kernel32.NewProc("GetConsoleMode")
	setConsoleMode = kernel32.NewProc("SetConsoleMode")
	getStdHandle   = kernel32.NewProc("GetStdHandle")
)

const (
	attachParentProcess             = ^uintptr(0) // (DWORD)-1
	stdOutputHandle                 = ^uintptr(10) // STD_OUTPUT_HANDLE = ((DWORD)-11)
	enableVirtualTerminalProcessing = 0x0004
)

func init() {
	if testing.Testing() {
		return
	}
	// attach to parent console if run from terminal
	attachConsole.Call(attachParentProcess)

	var targetHandle syscall.Handle

	hOut, err := syscall.Open("CONOUT$", syscall.O_RDWR, 0)
	if err == nil {
		targetHandle = hOut
		fOut := os.NewFile(uintptr(hOut), "/dev/stdout")
		os.Stdout = fOut
		os.Stderr = fOut
	} else {
		hStd, _, _ := getStdHandle.Call(stdOutputHandle)
		if hStd != 0 && hStd != ^uintptr(0) {
			targetHandle = syscall.Handle(hStd)
		}
	}

	// enable vt100 colors in windows terminal
	if targetHandle != 0 {
		var mode uint32
		r, _, _ := getConsoleMode.Call(uintptr(targetHandle), uintptr(unsafe.Pointer(&mode)))
		if r != 0 {
			r2, _, _ := setConsoleMode.Call(uintptr(targetHandle), uintptr(mode|enableVirtualTerminalProcessing))
			if r2 != 0 {
				colorsSupported = true
			}
		}
	}
}
