//go:build windows

package tray

import (
	"encoding/binary"
	"runtime"
	"syscall"
	"unsafe"
)

const (
	wmDestroy     = 0x0002
	wmClose       = 0x0010
	wmCommand     = 0x0111
	wmLButtonUp   = 0x0202
	wmLButtonDbl  = 0x0203
	wmRButtonUp   = 0x0205
	wmContextMenu = 0x007B
	wmUser        = 0x0400
	wmTrayIcon    = wmUser + 42

	nimAdd        = 0x00000000
	nimModify     = 0x00000001
	nimDelete     = 0x00000002
	nifMessage    = 0x00000001
	nifIcon       = 0x00000002
	nifTip        = 0x00000004
	mfString      = 0x00000000
	mfSeparator   = 0x00000800
	tpmBottomAlign = 0x0020
	tpmRightAlign  = 0x0008
	tpmReturnCmd   = 0x0100
	idiApplication = 32512
	hwndMessage    = ^uintptr(2) // (HWND)-3
)

type point struct {
	x int32
	y int32
}

type msg struct {
	hwnd    uintptr
	message uint32
	wParam  uintptr
	lParam  uintptr
	time    uint32
	pt      point
}

type wndClassExW struct {
	cbSize        uint32
	style         uint32
	lpfnWndProc   uintptr
	cbClsExtra    int32
	cbWndExtra    int32
	hInstance     uintptr
	hIcon         uintptr
	hCursor       uintptr
	hbrBackground uintptr
	lpszMenuName  *uint16
	lpszClassName *uint16
	hIconSm       uintptr
}

type notifyIconDataW struct {
	cbSize           uint32
	hwnd             uintptr
	uID              uint32
	uFlags           uint32
	uCallbackMessage uint32
	hIcon            uintptr
	szTip            [128]uint16
	dwState          uint32
	dwStateMask      uint32
	szInfo           [256]uint16
	uVersion         uint32
	szInfoTitle      [64]uint16
	dwInfoFlags      uint32
	guidItem         [16]byte
	hBalloonIcon     uintptr
}

var (
	kernel32                 = syscall.NewLazyDLL("kernel32.dll")
	user32                   = syscall.NewLazyDLL("user32.dll")
	shell32                  = syscall.NewLazyDLL("shell32.dll")
	getModuleHandleW         = kernel32.NewProc("GetModuleHandleW")
	registerClassExW         = user32.NewProc("RegisterClassExW")
	createWindowExW          = user32.NewProc("CreateWindowExW")
	defWindowProcW           = user32.NewProc("DefWindowProcW")
	destroyWindow            = user32.NewProc("DestroyWindow")
	postQuitMessage          = user32.NewProc("PostQuitMessage")
	postMessageW             = user32.NewProc("PostMessageW")
	getMessageW              = user32.NewProc("GetMessageW")
	translateMessage         = user32.NewProc("TranslateMessage")
	dispatchMessageW         = user32.NewProc("DispatchMessageW")
	loadIconW                = user32.NewProc("LoadIconW")
	createIconFromResourceEx = user32.NewProc("CreateIconFromResourceEx")
	destroyIcon              = user32.NewProc("DestroyIcon")
	createPopupMenu          = user32.NewProc("CreatePopupMenu")
	destroyMenu              = user32.NewProc("DestroyMenu")
	appendMenuW              = user32.NewProc("AppendMenuW")
	getCursorPos             = user32.NewProc("GetCursorPos")
	setForegroundWindow      = user32.NewProc("SetForegroundWindow")
	trackPopupMenuEx         = user32.NewProc("TrackPopupMenuEx")
	shellNotifyIconW         = shell32.NewProc("Shell_NotifyIconW")
)

func loadBestICOHandle(ico []byte) (hIcon uintptr, owned bool) {
	if len(ico) >= 6 && binary.LittleEndian.Uint16(ico[0:2]) == 0 && binary.LittleEndian.Uint16(ico[2:4]) == 1 {
		count := int(binary.LittleEndian.Uint16(ico[4:6]))
		if 6+count*16 <= len(ico) {
			bestIdx := -1
			bestScore := -1
			for i := 0; i < count; i++ {
				entry := ico[6+i*16 : 6+(i+1)*16]
				w := int(entry[0])
				if w == 0 {
					w = 256
				}
				size := int(binary.LittleEndian.Uint32(entry[8:12]))
				offset := int(binary.LittleEndian.Uint32(entry[12:16]))
				if size <= 0 || offset <= 0 || offset+size > len(ico) {
					continue
				}
				score := 1000 - absInt(w-32)
				if w == 32 {
					score += 500
				}
				if score > bestScore {
					bestScore = score
					bestIdx = i
				}
			}
			if bestIdx >= 0 {
				entry := ico[6+bestIdx*16 : 6+(bestIdx+1)*16]
				size := binary.LittleEndian.Uint32(entry[8:12])
				offset := binary.LittleEndian.Uint32(entry[12:16])
				h, _, _ := createIconFromResourceEx.Call(
					uintptr(unsafe.Pointer(&ico[offset])),
					uintptr(size),
					1,
					0x00030000,
					32,
					32,
					0,
				)
				if h != 0 {
					return h, true
				}
			}
		}
	}
	def, _, _ := loadIconW.Call(0, uintptr(idiApplication))
	return def, false
}

func absInt(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

func (m *Manager) start(cfg Config) {
	ready := make(chan uintptr, 1)

	go func() {
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()

		hInst, _, _ := getModuleHandleW.Call(0)
		className, _ := syscall.UTF16PtrFromString("MilaTrayMessageWindow")

		var hwnd uintptr
		wndProc := syscall.NewCallback(func(h uintptr, uMsg uint32, wParam, lParam uintptr) uintptr {
			switch uMsg {
			case wmTrayIcon:
				ev := uint32(lParam & 0xffff)
				switch ev {
				case wmLButtonUp, wmLButtonDbl:
					m.Dispatch(MenuToggleShow)
				case wmRButtonUp, wmContextMenu:
					m.showContextMenu(h)
				}
				return 0
			case wmCommand:
				cmdID := MenuID(wParam & 0xffff)
				m.Dispatch(cmdID)
				return 0
			case wmClose:
				_, _, _ = destroyWindow.Call(h)
				return 0
			case wmDestroy:
				_, _, _ = postQuitMessage.Call(0)
				return 0
			}
			r, _, _ := defWindowProcW.Call(h, uintptr(uMsg), wParam, lParam)
			return r
		})

		wc := wndClassExW{
			cbSize:        uint32(unsafe.Sizeof(wndClassExW{})),
			lpfnWndProc:   wndProc,
			hInstance:     hInst,
			lpszClassName: className,
		}
		_, _, _ = registerClassExW.Call(uintptr(unsafe.Pointer(&wc)))

		titlePtr, _ := syscall.UTF16PtrFromString("MilaTray")
		hwnd, _, _ = createWindowExW.Call(
			0,
			uintptr(unsafe.Pointer(className)),
			uintptr(unsafe.Pointer(titlePtr)),
			0,
			0, 0, 0, 0,
			hwndMessage,
			0,
			hInst,
			0,
		)
		ready <- hwnd
		if hwnd == 0 {
			return
		}

		hIcon, ownedIcon := loadBestICOHandle(cfg.IconICO)
		defer func() {
			if ownedIcon && hIcon != 0 {
				_, _, _ = destroyIcon.Call(hIcon)
			}
		}()

		nid := notifyIconDataW{
			cbSize:           uint32(unsafe.Sizeof(notifyIconDataW{})),
			hwnd:             hwnd,
			uID:              1,
			uFlags:           nifMessage | nifIcon | nifTip,
			uCallbackMessage: wmTrayIcon,
			hIcon:            hIcon,
		}
		tip := cfg.Tooltip
		if tip == "" {
			tip = "Mila"
		}
		if utf16Tip, err := syscall.UTF16FromString(tip); err == nil {
			copy(nid.szTip[:len(nid.szTip)-1], utf16Tip)
		}

		_, _, _ = shellNotifyIconW.Call(nimAdd, uintptr(unsafe.Pointer(&nid)))
		defer func() {
			_, _, _ = shellNotifyIconW.Call(nimDelete, uintptr(unsafe.Pointer(&nid)))
		}()

		var mMsg msg
		for {
			ret, _, _ := getMessageW.Call(uintptr(unsafe.Pointer(&mMsg)), 0, 0, 0)
			if int32(ret) <= 0 {
				break
			}
			_, _, _ = translateMessage.Call(uintptr(unsafe.Pointer(&mMsg)))
			_, _, _ = dispatchMessageW.Call(uintptr(unsafe.Pointer(&mMsg)))
		}
	}()

	hwnd := <-ready
	if hwnd != 0 {
		m.mu.Lock()
		if m.closed {
			m.mu.Unlock()
			_, _, _ = postMessageW.Call(hwnd, wmClose, 0, 0)
			return
		}
		m.stopFn = func() {
			_, _, _ = postMessageW.Call(hwnd, wmClose, 0, 0)
		}
		m.mu.Unlock()
	}
}

func (m *Manager) showContextMenu(hwnd uintptr) {
	menu, _, _ := createPopupMenu.Call()
	if menu == 0 {
		return
	}
	defer destroyMenu.Call(menu)

	playLabel, visLabel := m.MenuLabels()
	appendItem := func(id MenuID, text string) {
		ptr, _ := syscall.UTF16PtrFromString(text)
		_, _, _ = appendMenuW.Call(menu, mfString, uintptr(id), uintptr(unsafe.Pointer(ptr)))
	}

	appendItem(MenuTogglePlay, playLabel)
	appendItem(MenuToggleShow, visLabel)
	appendItem(MenuCheckUpd, "Check for Updates")
	_, _, _ = appendMenuW.Call(menu, mfSeparator, 0, 0)
	appendItem(MenuQuit, "Quit")

	var pt point
	_, _, _ = getCursorPos.Call(uintptr(unsafe.Pointer(&pt)))
	_, _, _ = setForegroundWindow.Call(hwnd)
	cmd, _, _ := trackPopupMenuEx.Call(
		menu,
		tpmBottomAlign|tpmRightAlign|tpmReturnCmd,
		uintptr(pt.x),
		uintptr(pt.y),
		hwnd,
		0,
	)
	if cmd != 0 {
		m.Dispatch(MenuID(cmd))
	}
}
