//go:build unix

package shellenv

import (
	"os/exec"
	"syscall"
)

// detach puts the shell in its own session, without a controlling terminal,
// and makes cancelling kill its whole process group.
func detach(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return nil
		}
		return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	}
}
