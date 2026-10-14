//go:build !windows

package shellenv

import "os/exec"

func hideWindow(*exec.Cmd) {}
