package installer

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInstallerPathsAndEscapePS(t *testing.T) {
	p := DefaultInstallPath()
	if !strings.HasSuffix(strings.ToLower(p), filepath.Join("ozevpn", "ozevpn.exe")) {
		t.Fatalf("unexpected default install path: %q", p)
	}
	if got := escapePS("C:\\User's\\App"); got != "C:\\User''s\\App" {
		t.Fatalf("escapePS failed: %q", got)
	}
	st := GetStatus()
	if st.InstallPath == "" {
		t.Fatal("expected non-empty InstallPath in GetStatus")
	}
}

func TestCopyFileRoundTrip(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src.bin")
	dst := filepath.Join(dir, "dst.bin")
	if err := os.WriteFile(src, []byte("ozevpn-payload"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := copyFile(src, dst); err != nil {
		t.Fatalf("copyFile failed: %v", err)
	}
	got, err := os.ReadFile(dst)
	if err != nil || string(got) != "ozevpn-payload" {
		t.Fatalf("copyFile content mismatch: %q err=%v", string(got), err)
	}
}
