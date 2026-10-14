//go:build windows

package shellenv

import (
	"os/exec"
	"syscall"
)

// hideWindow keeps reg.exe from flashing a console.
func hideWindow(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000}
}
