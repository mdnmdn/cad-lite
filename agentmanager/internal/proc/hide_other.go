//go:build !windows

package proc

import "os/exec"

// HideWindow leaves commands unchanged on platforms without Windows consoles.
func HideWindow(*exec.Cmd) {}
