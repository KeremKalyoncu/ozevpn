//go:build windows

package procutil

import (
	"os/exec"
	"syscall"
)

const createNoWindow = 0x08000000

// HideWindow prevents Windows from flashing a black console window when a
// -H=windowsgui application spawns child CLI processes (warp-cli, reg, net, powershell, sing-box, tor).
func HideWindow(cmd *exec.Cmd) {
	if cmd == nil {
		return
	}
	cmd.SysProcAttr = &syscall.SysProcAttr{
		HideWindow:    true,
		CreationFlags: createNoWindow,
	}
}
