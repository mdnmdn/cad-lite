//go:build windows

package proc

import (
	"os/exec"
	"syscall"
	"testing"
)

func TestHideWindowSetsConsoleFlags(t *testing.T) {
	for _, existing := range []bool{false, true} {
		cmd := exec.Command("unused")
		if existing {
			cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: 0x00000200, NoInheritHandles: true}
		}
		HideWindow(cmd)
		if !cmd.SysProcAttr.HideWindow || cmd.SysProcAttr.CreationFlags&0x08000000 == 0 {
			t.Fatal("background command can allocate a console")
		}
		if existing && (cmd.SysProcAttr.CreationFlags&0x00000200 == 0 || !cmd.SysProcAttr.NoInheritHandles) {
			t.Fatal("existing launch settings lost")
		}
	}
}
