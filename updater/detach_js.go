//go:build js

package updater

import "os/exec"

func detach(*exec.Cmd) {}
