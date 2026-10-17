//go:build !windows

package appenv

import (
	"context"
	"os/exec"
)

func systemVersionOutput(ctx context.Context, args []string) ([]byte, error) {
	return exec.CommandContext(ctx, args[0], args[1:]...).Output()
}
