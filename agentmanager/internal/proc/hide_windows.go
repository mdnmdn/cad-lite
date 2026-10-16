//go:build windows

package proc

import (
	"os/exec"
	"syscall"
)

// HideWindow suppresses a Windows console for a background command, preserving
// its other process attributes and piped standard streams.
func HideWindow(cmd *exec.Cmd) {
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.HideWindow = true
	cmd.SysProcAttr.CreationFlags |= 0x08000000 // CREATE_NO_WINDOW
}
