//go:build !windows

package ui

import "os/exec"

// newConsole has no effect outside Windows, where the terminal program makes
// its own window.
func newConsole(*exec.Cmd, string) {}
