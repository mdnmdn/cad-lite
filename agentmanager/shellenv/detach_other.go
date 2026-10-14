//go:build !unix

package shellenv

import "os/exec"

func detach(*exec.Cmd) {}
