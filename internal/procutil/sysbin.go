package procutil

import (
	"os"
	"path/filepath"
)

func systemRoot() string {
	if root := os.Getenv("SystemRoot"); root != "" {
		return root
	}
	return `C:\Windows`
}

// System32 returns an absolute path under %SystemRoot%\System32 for a Windows
// built-in tool (e.g., "netsh.exe", "reg.exe", "icacls.exe") to avoid PATH hijacking.
func System32(exe string) string {
	return filepath.Join(systemRoot(), "System32", exe)
}

// PowerShell returns the absolute path to Windows PowerShell 5.1 under System32.
func PowerShell() string {
	return filepath.Join(systemRoot(), "System32", "WindowsPowerShell", "v1.0", "powershell.exe")
}
