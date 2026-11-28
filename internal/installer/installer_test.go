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
	if st.InstallPath == "" || st.ElevatedInstallPath == "" {
		t.Fatal("expected non-empty InstallPath and ElevatedInstallPath in GetStatus")
	}
	ep := ElevatedInstallPath()
	if !strings.HasSuffix(strings.ToLower(ep), filepath.Join("ozevpn", "ozevpn.exe")) {
		t.Fatalf("unexpected elevated install path: %q", ep)
	}
	if !parseTaskQueryOutput("TaskName: \\OzeVPN\r\nStatus: Ready\r\n") {
		t.Fatal("expected parseTaskQueryOutput to match \\OzeVPN")
	}
	if parseTaskQueryOutput("ERROR: The system cannot find the file specified.") {
		t.Fatal("expected parseTaskQueryOutput to reject error output")
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
