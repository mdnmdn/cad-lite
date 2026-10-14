//go:build windows

package agentinstall

import (
	"os/exec"
	"syscall"
)

// hideWindow keeps mise and opencode from flashing a console window (CREATE_NO_WINDOW).
func hideWindow(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000}
}
