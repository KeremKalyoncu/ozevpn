package installer

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

type Status struct {
	ResidentEnabled bool   `json:"resident_enabled"`
	InstallPath     string `json:"install_path"`
	CurrentExe      string `json:"current_exe"`
	StartupCommand  string `json:"startup_command"`
}

func DefaultInstallPath() string {
	localApp := os.Getenv("LOCALAPPDATA")
	if localApp == "" {
		home, _ := os.UserHomeDir()
		localApp = filepath.Join(home, "AppData", "Local")
	}
	return filepath.Join(localApp, "OzeVPN", "ozevpn.exe")
}

func GetStatus() Status {
	exe, _ := os.Executable()
	st := Status{
		InstallPath: DefaultInstallPath(),
		CurrentExe:  exe,
	}
	if runtime.GOOS != "windows" {
		return st
	}
	out, err := runPS(`(Get-ItemProperty -Path 'HKCU:\Software\Microsoft\Windows\CurrentVersion\Run' -Name 'OzeVPN' -ErrorAction SilentlyContinue).OzeVPN`)
	if err == nil && strings.TrimSpace(out) != "" {
		st.ResidentEnabled = true
		st.StartupCommand = strings.TrimSpace(out)
	}
	return st
}

// EnableResident installs ozevpn.exe to %LOCALAPPDATA%\OzeVPN, registers
// Windows auto-start (-silent -autoconnect), and creates a Desktop shortcut.
func EnableResident() (Status, error) {
	if runtime.GOOS != "windows" {
		return GetStatus(), nil
	}
	srcExe, err := os.Executable()
	if err != nil {
		return GetStatus(), err
	}
	dstExe := DefaultInstallPath()
	if err := os.MkdirAll(filepath.Dir(dstExe), 0755); err != nil {
		return GetStatus(), fmt.Errorf("kurulum klasörü oluşturulamadı: %w", err)
	}

	// Copy executable if not already running from target path
	if !strings.EqualFold(filepath.Clean(srcExe), filepath.Clean(dstExe)) {
		if err := copyFile(srcExe, dstExe); err != nil {
			// If destination is locked/running, still register current path
			dstExe = srcExe
		}
	}

	startupCmd := fmt.Sprintf(`"%s" -silent -autoconnect`, dstExe)
	regKey := `HKCU\Software\Microsoft\Windows\CurrentVersion\Run`
	if err := exec.Command("reg", "add", regKey, "/v", "OzeVPN", "/t", "REG_SZ", "/d", startupCmd, "/f").Run(); err != nil {
		return GetStatus(), fmt.Errorf("başlangıç kaydı eklenemedi: %w", err)
	}

	ps := fmt.Sprintf(`
$desktop = [Environment]::GetFolderPath('Desktop')
$lnk = Join-Path $desktop 'OzeVPN.lnk'
$ws = New-Object -ComObject WScript.Shell
$sc = $ws.CreateShortcut($lnk)
$sc.TargetPath = '%s'
$sc.WorkingDirectory = '%s'
$sc.Description = 'OzeVPN - Engelsiz ve Hızlı İnternet'
$sc.Save()
`, escapePS(dstExe), escapePS(filepath.Dir(dstExe)))
	_, _ = runPS(ps)
	return GetStatus(), nil
}

// DisableResident removes the Windows auto-start entry so OzeVPN only runs when opened manually.
func DisableResident() (Status, error) {
	if runtime.GOOS != "windows" {
		return GetStatus(), nil
	}
	regKey := `HKCU\Software\Microsoft\Windows\CurrentVersion\Run`
	_ = exec.Command("reg", "delete", regKey, "/v", "OzeVPN", "/f").Run()
	return GetStatus(), nil
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()

	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return out.Close()
}

func escapePS(s string) string {
	return strings.ReplaceAll(s, "'", "''")
}

func runPS(script string) (string, error) {
	cmd := exec.Command("powershell", "-NoProfile", "-NonInteractive", "-Command", script)
	var out, stderr bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return out.String(), fmt.Errorf("%w: %s", err, strings.TrimSpace(stderr.String()))
	}
	return out.String(), nil
}
