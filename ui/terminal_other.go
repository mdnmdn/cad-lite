//go:build !windows

package ui

import "os/exec"

// newConsole has no effect outside Windows, where the terminal program makes
// its own window.
func newConsole(*exec.Cmd, string) {}

// startTerminal starts a terminal and reaps the launcher after it exits.
func startTerminal(cmd *exec.Cmd) error {
	if err := cmd.Start(); err != nil {
		return err
	}
	go func() { _ = cmd.Wait() }()
	return nil
}
