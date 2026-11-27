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

	"ozevpn/internal/procutil"
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
	cmd := exec.Command(procutil.System32("reg.exe"), "query", `HKCU\Software\Microsoft\Windows\CurrentVersion\Run`, "/v", "OzeVPN")
	procutil.HideWindow(cmd)
	out, err := cmd.Output()
	if err == nil {
		for _, line := range strings.Split(string(out), "\n") {
			if strings.Contains(line, "REG_SZ") {
				parts := strings.SplitN(line, "REG_SZ", 2)
				if len(parts) == 2 && strings.TrimSpace(parts[1]) != "" {
					st.ResidentEnabled = true
					st.StartupCommand = strings.TrimSpace(parts[1])
				}
			}
		}
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
	regAdd := exec.Command(procutil.System32("reg.exe"), "add", regKey, "/v", "OzeVPN", "/t", "REG_SZ", "/d", startupCmd, "/f")
	procutil.HideWindow(regAdd)
	if err := regAdd.Run(); err != nil {
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
	regDel := exec.Command(procutil.System32("reg.exe"), "delete", regKey, "/v", "OzeVPN", "/f")
	procutil.HideWindow(regDel)
	_ = regDel.Run()
	return GetStatus(), nil
}

// Uninstall cleanly removes the Windows auto-start registry entry, Desktop shortcut,
// and user-local runtime/profile data directory (%LOCALAPPDATA%\OzeVPN) without leaving orphans.
func Uninstall(extraPaths ...string) error {
	_, _ = DisableResident()
	if runtime.GOOS == "windows" {
		ps := `
$desktop = [Environment]::GetFolderPath('Desktop')
$lnk = Join-Path $desktop 'OzeVPN.lnk'
if (Test-Path $lnk) { Remove-Item $lnk -Force -ErrorAction SilentlyContinue }
`
		_, _ = runPS(ps)
		appDir := filepath.Dir(DefaultInstallPath())
		_ = os.RemoveAll(filepath.Join(appDir, "runtime"))
		_ = os.RemoveAll(filepath.Join(appDir, "ui-profile"))
		_ = os.Remove(filepath.Join(appDir, ".ozevpn_token"))
		_ = os.Remove(filepath.Join(appDir, "ozevpn.json"))
	}
	for _, p := range extraPaths {
		if strings.TrimSpace(p) != "" {
			_ = os.Remove(p)
		}
	}
	return nil
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
	cmd := exec.Command(procutil.PowerShell(), "-NoProfile", "-NonInteractive", "-Command", script)
	procutil.HideWindow(cmd)
	var out, stderr bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return out.String(), fmt.Errorf("%w: %s", err, strings.TrimSpace(stderr.String()))
	}
	return out.String(), nil
}
