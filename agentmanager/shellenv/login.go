package shellenv

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"
)

// Timeout bounds one LoginPATH probe.
const Timeout = 5 * time.Second

// LoginPATH returns the PATH of the user's login environment. On Unix it runs
// $SHELL as a login, interactive shell (so ~/.zprofile, ~/.zshrc,
// ~/.bash_profile and ~/.bashrc are read) and prints $PATH between unique
// markers, ignoring any banner or rc-file output; stdin is closed and no
// window opens. On Windows it reads the Machine and User Path from the
// registry. The probe gives up after Timeout.
func LoginPATH(ctx context.Context) (string, error) {
	if runtime.GOOS == "windows" {
		return windowsPATH(ctx)
	}
	return shellPATH(ctx, userShell(), Timeout)
}

// userShell is $SHELL, else the platform's default shell.
func userShell() string {
	if s := strings.TrimSpace(os.Getenv("SHELL")); s != "" {
		return s
	}
	if runtime.GOOS == "darwin" {
		return "/bin/zsh"
	}
	if _, err := os.Stat("/bin/bash"); err == nil {
		return "/bin/bash"
	}
	return "/bin/sh"
}

func marker() string {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(b[:])
}

// shellPATH runs shell -l -i -c and extracts $PATH from its output.
func shellPATH(ctx context.Context, shell string, timeout time.Duration) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	mark := "__CADLITE_PATH_" + marker() + "__"
	script := "printf '%s' '" + mark + "'\"$PATH\"'" + mark + "'"
	cmd := exec.CommandContext(ctx, shell, "-l", "-i", "-c", script)
	cmd.Stdin = nil // the null device: the shell sees a closed input
	cmd.Env = os.Environ()
	if os.Getenv("TERM") == "" {
		cmd.Env = append(cmd.Env, "TERM=dumb")
	}
	detach(cmd)
	cmd.WaitDelay = 500 * time.Millisecond
	out, err := cmd.Output()
	if p, ok := between(string(out), mark); ok {
		return p, nil
	}
	if ctx.Err() != nil {
		return "", fmt.Errorf("shellenv: %s did not answer within %s", shell, timeout)
	}
	if err != nil {
		return "", fmt.Errorf("shellenv: %s: %w", shell, err)
	}
	return "", errors.New("shellenv: " + shell + " printed no PATH")
}

// between returns the text between the first two occurrences of mark.
func between(out, mark string) (string, bool) {
	i := strings.Index(out, mark)
	if i < 0 {
		return "", false
	}
	rest := out[i+len(mark):]
	j := strings.Index(rest, mark)
	if j <= 0 {
		return "", false
	}
	return strings.TrimSpace(rest[:j]), true
}
