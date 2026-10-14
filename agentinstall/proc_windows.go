//go:build windows

package agentinstall

import (
	"os/exec"
	"strconv"
	"syscall"
)

// prepare keeps mise and opencode from flashing a console window (CREATE_NO_WINDOW) and makes
// cancelling cmd end its whole process tree (taskkill /T), not only mise.
func prepare(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000}
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return nil
		}
		kill := exec.Command("taskkill", "/T", "/F", "/PID", strconv.Itoa(cmd.Process.Pid))
		kill.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000}
		_ = kill.Run()
		return cmd.Process.Kill()
	}
}
