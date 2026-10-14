//go:build !windows

package agentinstall

import "os/exec"

func hideWindow(*exec.Cmd) {}
