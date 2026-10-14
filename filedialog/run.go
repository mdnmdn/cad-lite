//go:build !js

package filedialog

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"runtime"
)

// run shows the dialog for r with the backend this system has.
func run(ctx context.Context, r request) (string, error) {
	display := os.Getenv("DISPLAY") != "" || os.Getenv("WAYLAND_DISPLAY") != ""
	have := func(name string) bool { _, err := exec.LookPath(name); return err == nil }
	b := chooseBackend(runtime.GOOS, have, display)
	c, err := buildCommand(b, r)
	if err != nil {
		return "", err
	}
	path, err := execute(ctx, c)
	if err != nil {
		return "", err
	}
	if r.save {
		path = withDefaultExt(path, defaultExtension(r.filters))
	}
	return path, nil
}

// execute runs c and interprets its output.
func execute(ctx context.Context, c command) (string, error) {
	cmd := exec.CommandContext(ctx, c.name, c.args...)
	var out, errOut bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errOut
	err := cmd.Run()
	if ctx.Err() != nil {
		return "", ctx.Err()
	}
	exit := 0
	var ee *exec.ExitError
	switch {
	case errors.As(err, &ee):
		exit = ee.ExitCode()
	case errors.Is(err, exec.ErrNotFound), errors.Is(err, os.ErrNotExist):
		return "", ErrUnavailable
	case err != nil:
		return "", fmt.Errorf("filedialog: %s: %w", c.name, err)
	}
	return interpret(c.backend, out.String(), errOut.String(), exit)
}
