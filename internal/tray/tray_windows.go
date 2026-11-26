//go:build windows

package tray

import (
	_ "embed"
	"encoding/binary"
	"runtime"
	"sync"
	"syscall"
	"time"
	"unsafe"
)

//go:embed ozevpn.ico
var idleICO []byte

//go:embed ozevpn-connected.ico
var connectedICO []byte

type Options struct {
	GetState         func() (connected bool, tooltip string)
	OnOpenPanel      func()
	OnToggleConnect  func()
	OnEmergencyReset func()
	OnQuit           func()
}

type Tray struct {
	mu            sync.Mutex
	opts          Options
	hwnd          uintptr
	iconIdle      uintptr
	iconConnected uintptr
	lastConnected bool
	lastTooltip   string
	taskbarMsg    uint32
	stopCh        chan struct{}
	doneCh        chan struct{}
	closed        bool
}

const (
	wmDestroy       = 0x0002
	wmClose         = 0x0010
	wmNull          = 0x0000
	wmCommand       = 0x0111
	wmLButtonUp     = 0x0202
	wmLButtonDblClk = 0x0203
	wmRButtonUp     = 0x0205
	wmContextMenu   = 0x007B
	wmApp           = 0x8000
	wmTrayCallback  = wmApp + 1
	wmTrayRefresh   = wmApp + 2

	nimAdd    = 0x00000000
	nimModify = 0x00000001
	nimDelete = 0x00000002

	nifMessage = 0x00000001
	nifIcon    = 0x00000002
	nifTip     = 0x00000004

	imageIcon      = 1
	lrDefaultColor = 0x00000000
	smCxSmIcon     = 49
	smCySmIcon     = 50

	mfString      = 0x00000000
	mfSeparator   = 0x00000800
	tpmLeftAlign  = 0x0000
	tpmBottomAlign = 0x0020
	tpmRightButton = 0x0002

	cmdOpenPanel      = 1001
	cmdToggleConnect  = 1002
	cmdEmergencyReset = 1003
	cmdQuit           = 1004
)

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

type notifyIconData struct {
	cbSize           uint32
	hWnd             uintptr
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
	kernel32 = syscall.NewLazyDLL("kernel32.dll")
	user32   = syscall.NewLazyDLL("user32.dll")
	shell32  = syscall.NewLazyDLL("shell32.dll")

	procGetModuleHandleW        = kernel32.NewProc("GetModuleHandleW")
	procRegisterClassExW        = user32.NewProc("RegisterClassExW")
	procCreateWindowExW         = user32.NewProc("CreateWindowExW")
	procDefWindowProcW          = user32.NewProc("DefWindowProcW")
	procGetMessageW             = user32.NewProc("GetMessageW")
	procTranslateMessage        = user32.NewProc("TranslateMessage")
	procDispatchMessageW        = user32.NewProc("DispatchMessageW")
	procPostMessageW            = user32.NewProc("PostMessageW")
	procPostQuitMessage         = user32.NewProc("PostQuitMessage")
	procDestroyWindow           = user32.NewProc("DestroyWindow")
	procLoadImageW              = user32.NewProc("LoadImageW")
	procCreateIconFromResEx     = user32.NewProc("CreateIconFromResourceEx")
	procDestroyIcon             = user32.NewProc("DestroyIcon")
	procGetSystemMetrics        = user32.NewProc("GetSystemMetrics")
	procCreatePopupMenu         = user32.NewProc("CreatePopupMenu")
	procAppendMenuW             = user32.NewProc("AppendMenuW")
	procSetMenuDefaultItem      = user32.NewProc("SetMenuDefaultItem")
	procTrackPopupMenu          = user32.NewProc("TrackPopupMenu")
	procDestroyMenu             = user32.NewProc("DestroyMenu")
	procSetForegroundWindow     = user32.NewProc("SetForegroundWindow")
	procGetCursorPos            = user32.NewProc("GetCursorPos")
	procRegisterWindowMessageW  = user32.NewProc("RegisterWindowMessageW")
	procShellNotifyIconW        = shell32.NewProc("Shell_NotifyIconW")

	activeMu   sync.RWMutex
	activeTray *Tray
	wndProcCb  = syscall.NewCallback(trayWndProc)
)

// Start launches the Windows System Tray icon on a dedicated Win32 UI thread.
func Start(opts Options) *Tray {
	t := &Tray{
		opts:        opts,
		lastTooltip: "OzeVPN v6 — Bağlı Değil",
		stopCh:      make(chan struct{}),
		doneCh:      make(chan struct{}),
	}
	activeMu.Lock()
	activeTray = t
	activeMu.Unlock()

	go t.runLoop()
	return t
}

// Stop removes the tray icon and cleanly terminates the Win32 message loop.
func (t *Tray) Stop() {
	if t == nil {
		return
	}
	t.mu.Lock()
	if t.closed {
		t.mu.Unlock()
		return
	}
	t.closed = true
	close(t.stopCh)
	hwnd := t.hwnd
	t.mu.Unlock()

	if hwnd != 0 {
		procPostMessageW.Call(hwnd, wmClose, 0, 0)
	}
	select {
	case <-t.doneCh:
	case <-time.After(1 * time.Second):
	}
}

func (t *Tray) runLoop() {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	defer close(t.doneCh)

	hInst, _, _ := procGetModuleHandleW.Call(0)
	className, _ := syscall.UTF16PtrFromString("OzeVPNTrayWindowClass")
	winTitle, _ := syscall.UTF16PtrFromString("OzeVPN v6 Tray")

	wc := wndClassExW{
		cbSize:        uint32(unsafe.Sizeof(wndClassExW{})),
		lpfnWndProc:   wndProcCb,
		hInstance:     hInst,
		lpszClassName: className,
	}
	procRegisterClassExW.Call(uintptr(unsafe.Pointer(&wc)))

	tbMsgName, _ := syscall.UTF16PtrFromString("TaskbarCreated")
	tbMsg, _, _ := procRegisterWindowMessageW.Call(uintptr(unsafe.Pointer(tbMsgName)))
	t.taskbarMsg = uint32(tbMsg)

	hwnd, _, _ := procCreateWindowExW.Call(
		0,
		uintptr(unsafe.Pointer(className)),
		uintptr(unsafe.Pointer(winTitle)),
		0, 0, 0, 0, 0,
		0, 0, hInst, 0,
	)
	if hwnd == 0 {
		return
	}

	cx, _, _ := procGetSystemMetrics.Call(smCxSmIcon)
	cy, _, _ := procGetSystemMetrics.Call(smCySmIcon)
	if cx == 0 {
		cx = 16
	}
	if cy == 0 {
		cy = 16
	}

	idleIcon := loadTrayIcon(hInst, 1, idleICO, int32(cx), int32(cy))
	connIcon := loadTrayIcon(hInst, 2, connectedICO, int32(cx), int32(cy))
	if connIcon == 0 {
		connIcon = idleIcon
	}

	t.mu.Lock()
	t.hwnd = hwnd
	t.iconIdle = idleIcon
	t.iconConnected = connIcon
	t.mu.Unlock()

	t.addOrUpdateIcon(nimAdd)

	// Background state watcher refreshes tray icon & tooltip whenever VPN connects/disconnects
	go func() {
		ticker := time.NewTicker(2 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-t.stopCh:
				return
			case <-ticker.C:
				if t.opts.GetState == nil {
					continue
				}
				conn, tip := t.opts.GetState()
				t.mu.Lock()
				changed := conn != t.lastConnected || tip != t.lastTooltip
				if changed {
					t.lastConnected = conn
					t.lastTooltip = tip
				}
				h := t.hwnd
				t.mu.Unlock()
				if changed && h != 0 {
					procPostMessageW.Call(h, wmTrayRefresh, 0, 0)
				}
			}
		}
	}()

	var m msg
	for {
		ret, _, _ := procGetMessageW.Call(uintptr(unsafe.Pointer(&m)), 0, 0, 0)
		if int32(ret) <= 0 {
			break
		}
		procTranslateMessage.Call(uintptr(unsafe.Pointer(&m)))
		procDispatchMessageW.Call(uintptr(unsafe.Pointer(&m)))
	}

	t.deleteIcon()
	if idleIcon != 0 {
		procDestroyIcon.Call(idleIcon)
	}
	if connIcon != 0 && connIcon != idleIcon {
		procDestroyIcon.Call(connIcon)
	}
}

func (t *Tray) addOrUpdateIcon(action uintptr) {
	t.mu.Lock()
	hwnd := t.hwnd
	icon := t.iconIdle
	if t.lastConnected && t.iconConnected != 0 {
		icon = t.iconConnected
	}
	tip := t.lastTooltip
	t.mu.Unlock()
	if hwnd == 0 {
		return
	}
	nid := notifyIconData{
		cbSize:           uint32(unsafe.Sizeof(notifyIconData{})),
		hWnd:             hwnd,
		uID:              1,
		uFlags:           nifMessage | nifIcon | nifTip,
		uCallbackMessage: wmTrayCallback,
		hIcon:            icon,
	}
	copyUTF16(nid.szTip[:], tip)
	procShellNotifyIconW.Call(action, uintptr(unsafe.Pointer(&nid)))
}

func (t *Tray) deleteIcon() {
	t.mu.Lock()
	hwnd := t.hwnd
	t.mu.Unlock()
	if hwnd == 0 {
		return
	}
	nid := notifyIconData{
		cbSize: uint32(unsafe.Sizeof(notifyIconData{})),
		hWnd:   hwnd,
		uID:    1,
	}
	procShellNotifyIconW.Call(nimDelete, uintptr(unsafe.Pointer(&nid)))
}

func (t *Tray) showContextMenu(hwnd uintptr) {
	hMenu, _, _ := procCreatePopupMenu.Call()
	if hMenu == 0 {
		return
	}
	defer procDestroyMenu.Call(hMenu)

	connected := false
	if t.opts.GetState != nil {
		connected, _ = t.opts.GetState()
	}

	appendMenuItem(hMenu, mfString, cmdOpenPanel, "Paneli Aç (OzeVPN v6)")
	procSetMenuDefaultItem.Call(hMenu, cmdOpenPanel, 0)
	appendMenuItem(hMenu, mfSeparator, 0, "")
	if connected {
		appendMenuItem(hMenu, mfString, cmdToggleConnect, "Bağlantıyı Kes")
	} else {
		appendMenuItem(hMenu, mfString, cmdToggleConnect, "Bağlan (Engelsiz İnternet)")
	}
	appendMenuItem(hMenu, mfString, cmdEmergencyReset, "Proxy Sıfırla (Acil)")
	appendMenuItem(hMenu, mfSeparator, 0, "")
	appendMenuItem(hMenu, mfString, cmdQuit, "Çıkış")

	var pt point
	procGetCursorPos.Call(uintptr(unsafe.Pointer(&pt)))
	procSetForegroundWindow.Call(hwnd)
	procTrackPopupMenu.Call(
		hMenu,
		tpmRightButton|tpmBottomAlign|tpmLeftAlign,
		uintptr(pt.x),
		uintptr(pt.y),
		0,
		hwnd,
		0,
	)
	procPostMessageW.Call(hwnd, wmNull, 0, 0)
}

func appendMenuItem(hMenu uintptr, flags uintptr, id uintptr, label string) {
	var ptr uintptr
	if label != "" {
		u16, _ := syscall.UTF16PtrFromString(label)
		ptr = uintptr(unsafe.Pointer(u16))
	}
	procAppendMenuW.Call(hMenu, flags, id, ptr)
}

func trayWndProc(hwnd uintptr, uMsg uint32, wParam uintptr, lParam uintptr) uintptr {
	activeMu.RLock()
	t := activeTray
	activeMu.RUnlock()

	if t != nil {
		if t.taskbarMsg != 0 && uMsg == t.taskbarMsg {
			// Explorer restarted — re-register tray icon automatically!
			t.addOrUpdateIcon(nimAdd)
			return 0
		}
		switch uMsg {
		case wmTrayRefresh:
			t.addOrUpdateIcon(nimModify)
			return 0
		case wmTrayCallback:
			switch uint32(lParam & 0xFFFF) {
			case wmLButtonUp, wmLButtonDblClk:
				if t.opts.OnOpenPanel != nil {
					go t.opts.OnOpenPanel()
				}
			case wmRButtonUp, wmContextMenu:
				t.showContextMenu(hwnd)
			}
			return 0
		case wmCommand:
			switch uint32(wParam & 0xFFFF) {
			case cmdOpenPanel:
				if t.opts.OnOpenPanel != nil {
					go t.opts.OnOpenPanel()
				}
			case cmdToggleConnect:
				if t.opts.OnToggleConnect != nil {
					go t.opts.OnToggleConnect()
				}
			case cmdEmergencyReset:
				if t.opts.OnEmergencyReset != nil {
					go t.opts.OnEmergencyReset()
				}
			case cmdQuit:
				if t.opts.OnQuit != nil {
					go t.opts.OnQuit()
				}
			}
			return 0
		case wmClose:
			t.deleteIcon()
			procDestroyWindow.Call(hwnd)
			return 0
		case wmDestroy:
			procPostQuitMessage.Call(0)
			return 0
		}
	}
	ret, _, _ := procDefWindowProcW.Call(hwnd, uintptr(uMsg), wParam, lParam)
	return ret
}

func loadTrayIcon(hInst uintptr, resID uintptr, fallbackICO []byte, cx, cy int32) uintptr {
	if hInst != 0 {
		h, _, _ := procLoadImageW.Call(hInst, resID, imageIcon, uintptr(cx), uintptr(cy), lrDefaultColor)
		if h != 0 {
			return h
		}
	}
	return createIconFromICOBytes(fallbackICO, cx, cy)
}

func createIconFromICOBytes(ico []byte, cx, cy int32) uintptr {
	if len(ico) < 22 {
		return 0
	}
	count := int(binary.LittleEndian.Uint16(ico[4:6]))
	if count <= 0 || len(ico) < 6+count*16 {
		return 0
	}
	// Pick best matching PNG entry (prefer >= cx, smallest >= cx; else largest)
	bestIdx := 0
	bestScore := 1 << 30
	for i := 0; i < count; i++ {
		off := 6 + i*16
		w := int(ico[off])
		if w == 0 {
			w = 256
		}
		diff := w - int(cx)
		score := diff
		if diff < 0 {
			score = -diff + 1000
		}
		if score < bestScore {
			bestScore = score
			bestIdx = i
		}
	}
	entryOff := 6 + bestIdx*16
	imgSize := binary.LittleEndian.Uint32(ico[entryOff+8 : entryOff+12])
	imgOff := binary.LittleEndian.Uint32(ico[entryOff+12 : entryOff+16])
	if int(imgOff+imgSize) > len(ico) || imgSize == 0 {
		return 0
	}
	chunk := ico[imgOff : imgOff+imgSize]
	h, _, _ := procCreateIconFromResEx.Call(
		uintptr(unsafe.Pointer(&chunk[0])),
		uintptr(imgSize),
		1,
		0x00030000,
		uintptr(cx),
		uintptr(cy),
		lrDefaultColor,
	)
	return h
}

func copyUTF16(dst []uint16, s string) {
	u16, err := syscall.UTF16FromString(s)
	if err != nil {
		return
	}
	n := len(u16)
	if n > len(dst) {
		n = len(dst)
		u16[n-1] = 0
	}
	copy(dst, u16[:n])
}
