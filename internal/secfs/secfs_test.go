package secfs

import (
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestDPAPIRoundTripAndLegacyPlaintext(t *testing.T) {
	secret := "vless://11111111-2222-3333-4444-555555555555@eu.example.com:443?security=tls&sni=eu.example.com"
	enc, err := ProtectSecret(secret)
	if err != nil {
		t.Fatalf("ProtectSecret failed: %v", err)
	}
	if runtime.GOOS == "windows" {
		if !IsProtected(enc) || strings.Contains(enc, "11111111") {
			t.Fatalf("expected DPAPI-encrypted output without plaintext, got: %q", enc)
		}
	}
	dec, err := UnprotectSecret(enc)
	if err != nil {
		t.Fatalf("UnprotectSecret failed: %v", err)
	}
	if dec != secret {
		t.Fatalf("round-trip mismatch: got %q want %q", dec, secret)
	}

	// Legacy plaintext must pass through transparently for automatic migration
	legacy, err := UnprotectSecret(secret)
	if err != nil || legacy != secret {
		t.Fatalf("legacy plaintext passthrough failed: %q err=%v", legacy, err)
	}
}

func TestWritePrivateFileRestrictsNTFSACL(t *testing.T) {
	p := filepath.Join(t.TempDir(), "secret.json")
	if err := WritePrivateFile(p, []byte(`{"secret":"test"}`)); err != nil {
		t.Fatalf("WritePrivateFile failed: %v", err)
	}
	if runtime.GOOS == "windows" {
		out, err := exec.Command("icacls", p).CombinedOutput()
		if err != nil {
			t.Fatalf("icacls read failed: %v", err)
		}
		acl := string(out)
		if strings.Contains(acl, "(I)") {
			t.Fatalf("expected inherited ACL entries to be stripped, got:\n%s", acl)
		}
		if strings.Contains(strings.ToLower(acl), "everyone") || strings.Contains(strings.ToLower(acl), "codexsandboxusers") {
			t.Fatalf("unexpected non-owner principal retained in ACL:\n%s", acl)
		}
	}
}
