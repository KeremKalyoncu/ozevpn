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

	icons := []struct {
		name string
		raw  []byte
	}{
		{"ozevpn.ico", appICO},
		{"ozevpn-idle.ico", idleICO},
		{"ozevpn-connecting.ico", connectingICO},
		{"ozevpn-connected.ico", connectedICO},
	}
	for _, ic := range icons {
		h := createIconFromICOBytes(ic.raw, 16, 16)
		if h == 0 {
			t.Fatalf("failed to create Win32 HICON from embedded %s", ic.name)
		}
		procDestroyIcon.Call(h)
	}
}
