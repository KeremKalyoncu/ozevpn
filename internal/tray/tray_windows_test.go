//go:build windows

package tray

import (
	"testing"
	"unsafe"
)

func TestNotifyIconDataABISizeAndIconLoad(t *testing.T) {
	// Verify Windows x64 NOTIFYICONDATAW struct alignment/size matches Win32 ABI (976 bytes)
	if sz := unsafe.Sizeof(notifyIconData{}); sz != 976 {
		t.Fatalf("unexpected NOTIFYICONDATAW size: got %d, want 976", sz)
	}

	hIdle := createIconFromICOBytes(idleICO, 16, 16)
	if hIdle == 0 {
		t.Fatal("failed to create Win32 HICON from embedded ozevpn.ico")
	}
	procDestroyIcon.Call(hIdle)

	hConn := createIconFromICOBytes(connectedICO, 16, 16)
	if hConn == 0 {
		t.Fatal("failed to create Win32 HICON from embedded ozevpn-connected.ico")
	}
	procDestroyIcon.Call(hConn)
}
