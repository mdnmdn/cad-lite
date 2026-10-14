//go:build !unix && !windows

package agentinstall

import "os/exec"

// prepare has nothing to add where programs cannot run (the browser).
func prepare(*exec.Cmd) {}
