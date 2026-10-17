package appenv

import (
	"context"
	"os/exec"
	"syscall"
)

func systemVersionOutput(ctx context.Context, args []string) ([]byte, error) {
	command := exec.CommandContext(ctx, args[0], args[1:]...)
	command.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	return command.Output()
}
