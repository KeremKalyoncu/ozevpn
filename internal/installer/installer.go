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

const ElevatedTaskName = "OzeVPN"

type Status struct {
	ResidentEnabled     bool   `json:"resident_enabled"`
	ElevatedTaskEnabled bool   `json:"elevated_task_enabled"`
	InstallPath         string `json:"install_path"`
	ElevatedInstallPath string `json:"elevated_install_path"`
	CurrentExe          string `json:"current_exe"`
	StartupCommand      string `json:"startup_command"`
}

func DefaultInstallPath() string {
	localApp := os.Getenv("LOCALAPPDATA")
	if localApp == "" {
		home, _ := os.UserHomeDir()
		localApp = filepath.Join(home, "AppData", "Local")
	}
	return filepath.Join(localApp, "OzeVPN", "ozevpn.exe")
}

// ElevatedInstallPath returns the Administrator-protected install target under %ProgramFiles%\OzeVPN
// so a /RL HIGHEST logon scheduled task cannot be tampered with by unprivileged user processes.
func ElevatedInstallPath() string {
	pf := strings.TrimSpace(os.Getenv("ProgramFiles"))
	if pf == "" {
		pf = `C:\Program Files`
	}
	return filepath.Join(pf, "OzeVPN", "OzeVPN.exe")
}

// IsUnderProgramFiles reports whether dir resides inside %ProgramFiles% or %ProgramFiles(x86)%.
func IsUnderProgramFiles(dir string) bool {
	if runtime.GOOS != "windows" || strings.TrimSpace(dir) == "" {
		return false
	}
	cleanDir := strings.ToLower(filepath.Clean(dir))
	for _, envName := range []string{"ProgramFiles", "ProgramFiles(x86)"} {
		root := strings.TrimSpace(os.Getenv(envName))
		if root == "" {
			continue
		}
		cleanRoot := strings.ToLower(filepath.Clean(root))
		if cleanDir == cleanRoot || strings.HasPrefix(cleanDir, cleanRoot+string(os.PathSeparator)) {
			return true
		}
	}
	return false
}

func isProcessAdmin() bool {
	if runtime.GOOS != "windows" {
		return false
	}
	cmd := exec.Command(procutil.System32("net.exe"), "session")
	procutil.HideWindow(cmd)
	return cmd.Run() == nil
}

func hasElevatedScheduledTask() bool {
	if runtime.GOOS != "windows" {
		return false
	}
	cmd := exec.Command(procutil.System32("schtasks.exe"), "/Query", "/TN", ElevatedTaskName, "/FO", "LIST")
	procutil.HideWindow(cmd)
	out, err := cmd.Output()
	if err != nil {
		return false
	}
	return parseTaskQueryOutput(string(out))
}

func parseTaskQueryOutput(out string) bool {
	lower := strings.ToLower(out)
	return strings.Contains(lower, `\ozevpn`) || strings.Contains(lower, "ozevpn")
}

func GetStatus() Status {
	exe, _ := os.Executable()
	st := Status{
		InstallPath:         DefaultInstallPath(),
		ElevatedInstallPath: ElevatedInstallPath(),
		CurrentExe:          exe,
	}
	if runtime.GOOS != "windows" {
		return st
	}
	if hasElevatedScheduledTask() {
		st.ElevatedTaskEnabled = true
		st.ResidentEnabled = true
		st.StartupCommand = fmt.Sprintf(`Task Scheduler (\%s · Yönetici / Oyun TUN)`, ElevatedTaskName)
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

// EnableElevatedResident installs OzeVPN.exe into the Admin-protected %ProgramFiles%\OzeVPN directory
// and registers an official Windows Task Scheduler logon task (/RL HIGHEST) so future boots and
// launches run with full Oyun TUN + Kill Switch privileges without repeated UAC prompts.
func EnableElevatedResident() (Status, error) {
	if runtime.GOOS != "windows" {
		return GetStatus(), nil
	}
	if !isProcessAdmin() {
		return GetStatus(), fmt.Errorf("otomatik yönetici görevi için Yönetici (UAC) onayı gereklidir")
	}
	srcExe, err := os.Executable()
	if err != nil {
		return GetStatus(), err
	}
	dstExe := ElevatedInstallPath()
	if err := os.MkdirAll(filepath.Dir(dstExe), 0755); err != nil {
		return GetStatus(), fmt.Errorf("Program Files kurulum klasörü oluşturulamadı: %w", err)
	}
	if !strings.EqualFold(filepath.Clean(srcExe), filepath.Clean(dstExe)) {
		if err := copyFile(srcExe, dstExe); err != nil {
			// If destination binary is currently locked by an older copy, keep running binary if already protected
			return GetStatus(), fmt.Errorf("OzeVPN.exe Program Files içine kopyalanamadı: %w", err)
		}
	}

	// Migrate portable ozevpn.json to %LOCALAPPDATA%\OzeVPN if it does not exist there yet
	migrateConfigToLocalAppData(filepath.Dir(srcExe))

	taskCmd := fmt.Sprintf(`"%s" -silent -autoconnect`, dstExe)
	createArgs := []string{
		"/Create",
		"/TN", ElevatedTaskName,
		"/TR", taskCmd,
		"/SC", "ONLOGON",
		"/RL", "HIGHEST",
		"/F",
	}
	cmd := exec.Command(procutil.System32("schtasks.exe"), createArgs...)
	procutil.HideWindow(cmd)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return GetStatus(), fmt.Errorf("Windows Zamanlanmış Görev oluşturulamadı: %w (%s)", err, strings.TrimSpace(stderr.String()))
	}

	// Ensure laptops on battery still auto-start OzeVPN and Windows never stops it after 72 hours
	tunePS := fmt.Sprintf(`
try {
  $s = New-ScheduledTaskSettingsSet -AllowStartIfOnBatteries -DontStopIfGoingOnBatteries -ExecutionTimeLimit (New-TimeSpan -Seconds 0) -StartWhenAvailable
  Set-ScheduledTask -TaskName '%s' -Settings $s | Out-Null
} catch {}
`, escapePS(ElevatedTaskName))
	_, _ = runPS(tunePS)

	// Remove legacy HKCU Run entry so Windows does not spawn a duplicate non-admin instance
	delReg := exec.Command(procutil.System32("reg.exe"), "delete", `HKCU\Software\Microsoft\Windows\CurrentVersion\Run`, "/v", "OzeVPN", "/f")
	procutil.HideWindow(delReg)
	_ = delReg.Run()

	createDesktopShortcut(dstExe)
	return GetStatus(), nil
}

// EnableResident installs OzeVPN for Windows startup. When elevated, it automatically registers
// the Admin-protected Task Scheduler logon task (/RL HIGHEST); otherwise uses standard HKCU Run.
func EnableResident() (Status, error) {
	if runtime.GOOS != "windows" {
		return GetStatus(), nil
	}
	if isProcessAdmin() {
		return EnableElevatedResident()
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

	createDesktopShortcut(dstExe)
	return GetStatus(), nil
}

// DisableResident removes both the HKCU auto-start entry and the elevated Windows Scheduled Task
// so OzeVPN only runs when opened manually.
func DisableResident() (Status, error) {
	if runtime.GOOS != "windows" {
		return GetStatus(), nil
	}
	regKey := `HKCU\Software\Microsoft\Windows\CurrentVersion\Run`
	regDel := exec.Command(procutil.System32("reg.exe"), "delete", regKey, "/v", "OzeVPN", "/f")
	procutil.HideWindow(regDel)
	_ = regDel.Run()

	taskDel := exec.Command(procutil.System32("schtasks.exe"), "/Delete", "/TN", ElevatedTaskName, "/F")
	procutil.HideWindow(taskDel)
	_ = taskDel.Run()

	return GetStatus(), nil
}

// Uninstall cleanly removes the Windows auto-start registry entry, elevated Scheduled Task,
// Desktop shortcut, and user-local runtime/profile data directory (%LOCALAPPDATA%\OzeVPN).
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
		// Best-effort cleanup of Program Files binary when uninstalling as Admin
		_ = os.Remove(ElevatedInstallPath())
		_ = os.Remove(filepath.Dir(ElevatedInstallPath()))
	}
	for _, p := range extraPaths {
		if strings.TrimSpace(p) != "" {
			_ = os.Remove(p)
		}
	}
	return nil
}

func migrateConfigToLocalAppData(srcDir string) {
	localDir := filepath.Dir(DefaultInstallPath())
	dstCfg := filepath.Join(localDir, "ozevpn.json")
	if _, err := os.Stat(dstCfg); err == nil {
		return
	}
	srcCfg := filepath.Join(srcDir, "ozevpn.json")
	if strings.EqualFold(filepath.Clean(srcCfg), filepath.Clean(dstCfg)) {
		return
	}
	if _, err := os.Stat(srcCfg); err == nil {
		_ = os.MkdirAll(localDir, 0700)
		_ = copyFile(srcCfg, dstCfg)
	}
}

func createDesktopShortcut(targetExe string) {
	ps := fmt.Sprintf(`
$desktop = [Environment]::GetFolderPath('Desktop')
$lnk = Join-Path $desktop 'OzeVPN.lnk'
$ws = New-Object -ComObject WScript.Shell
$sc = $ws.CreateShortcut($lnk)
$sc.TargetPath = '%s'
$sc.WorkingDirectory = '%s'
$sc.Description = 'OzeVPN - Anadolu Vaşağı | Engelsiz ve Hızlı İnternet'
$sc.Save()
`, escapePS(targetExe), escapePS(filepath.Dir(targetExe)))
	_, _ = runPS(ps)
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
