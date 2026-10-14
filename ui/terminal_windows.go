//go:build windows

package ui

import (
	"os/exec"
	"syscall"
)

// newConsole makes cmd open its own console window (CREATE_NEW_CONSOLE) and
// hands Windows the command line exactly as written.
func newConsole(cmd *exec.Cmd, cmdLine string) {
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: 0x00000010, CmdLine: cmdLine}
}
