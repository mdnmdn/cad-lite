//go:build unix

package agentinstall

import (
	"os/exec"
	"syscall"
)

// prepare puts cmd in a process group of its own, so cancelling it kills mise together with the
// node, npm or download processes it started.
func prepare(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return nil
		}
		return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	}
}
