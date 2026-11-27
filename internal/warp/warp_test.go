package warp

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNewClientAndStatusReadOnly(t *testing.T) {
	c := NewClient()
	if c == nil {
		t.Fatal("NewClient returned nil")
	}
	st := c.Status()
	if st.State == "" {
		t.Fatal("expected non-empty State string from warp.Status()")
	}
}

func TestNativeWGProfileSaveLoadEncryptsPrivateKey(t *testing.T) {
	p := filepath.Join(t.TempDir(), "warp_wg.json")
	orig := &NativeWGProfile{
		PrivateKey: "TestSecretWireGuardKey1234567890123456789012=",
		AddressV4:  "172.16.0.2/32",
		AddressV6:  "2606:4700:110:8000::2/128",
		PeerPubKey: "bmXOC+F1FxEMF9dyiK2H5/1SUtzH0JuVo51h2wPfgyo=",
		Endpoint:   "162.159.192.1",
		Port:       2408,
		Reserved:   []int{1, 2, 3},
	}
	if err := SaveNativeWGProfile(p, orig); err != nil {
		t.Fatalf("SaveNativeWGProfile failed: %v", err)
	}
	raw, err := os.ReadFile(p)
	if err != nil {
		t.Fatalf("ReadFile failed: %v", err)
	}
	if strings.Contains(string(raw), orig.PrivateKey) {
		t.Fatalf("expected private_key to be DPAPI-encrypted on disk, found plaintext in %s", string(raw))
	}
	loaded, err := LoadNativeWGProfile(p)
	if err != nil {
		t.Fatalf("LoadNativeWGProfile failed: %v", err)
	}
	if loaded.PrivateKey != orig.PrivateKey || loaded.Endpoint != orig.Endpoint || loaded.Port != orig.Port || len(loaded.Reserved) != 3 {
		t.Fatalf("loaded profile mismatch: got %+v want %+v", loaded, orig)
	}
}
