package procutil

import (
	"os/exec"
	"runtime"
	"testing"
)

func TestHideWindowSetsAttrOnWindows(t *testing.T) {
	cmd := exec.Command("cmd", "/c", "echo", "ok")
	HideWindow(cmd)
	if runtime.GOOS == "windows" {
		if cmd.SysProcAttr == nil || !cmd.SysProcAttr.HideWindow {
			t.Fatal("expected SysProcAttr.HideWindow=true on Windows")
		}
	}
}
