// Package harnesses opens a session on any supported backend by Harness id.
package harnesses

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"agentmanager"
	"agentmanager/acp"
	"agentmanager/claude"
	"agentmanager/codex"
	"agentmanager/shellenv"
)

// All lists the supported harnesses in the order Auto prefers them.
var All = []agentmanager.Harness{agentmanager.OpenCode, agentmanager.Claude, agentmanager.Codex}

// versionTimeout bounds one `--version` call.
const versionTimeout = 3 * time.Second

// Open starts a session on the backend selected by cfg.Harness. When
// cfg.Binary is empty the executable SetBinary stored for the harness is used,
// else it is looked up on the effective PATH (the process PATH plus the one
// shellenv.Set stored); the child gets that PATH in its environment so the
// agent's own subprocesses (node, bun) are found too.
func Open(ctx context.Context, cfg agentmanager.Config) (agentmanager.Session, error) {
	if cfg.Binary == "" {
		cfg.Binary = Binary(cfg.Harness)
	}
	if env := shellenv.Env(); env != nil {
		cfg.Env = append(append([]string(nil), env...), cfg.Env...)
		if cfg.Binary == "" {
			if p, ok := shellenv.Lookup(string(cfg.Harness), shellenv.Effective()); ok {
				cfg.Binary = p
			}
		}
	}
	switch cfg.Harness {
	case agentmanager.OpenCode:
		return acp.OpenCode().Open(ctx, cfg)
	case agentmanager.Claude:
		return claude.New().Open(ctx, cfg)
	case agentmanager.Codex:
		return codex.New().Open(ctx, cfg)
	}
	return nil, fmt.Errorf("agentmanager: unknown harness %q", cfg.Harness)
}

// Available reports where the harness's executable is: the one SetBinary
// stored (ok only while that file exists), else the default name on the
// effective PATH.
func Available(h agentmanager.Harness) (path string, ok bool) {
	if b := Binary(h); b != "" {
		_, err := os.Stat(b)
		return b, err == nil
	}
	return shellenv.Resolve(string(h))
}

var overrides = struct {
	sync.Mutex
	m map[agentmanager.Harness]string
}{m: map[agentmanager.Harness]string{}}

// SetBinary makes path the executable Open, Available and Discover use for h
// instead of looking h up on the PATH; an empty path goes back to the lookup.
// Like shellenv.Set it holds for the whole process.
func SetBinary(h agentmanager.Harness, path string) {
	overrides.Lock()
	defer overrides.Unlock()
	if path == "" {
		delete(overrides.m, h)
	} else {
		overrides.m[h] = path
	}
}

// Binary returns the executable SetBinary stored for h, or "".
func Binary(h agentmanager.Harness) string {
	overrides.Lock()
	defer overrides.Unlock()
	return overrides.m[h]
}

// Errors Check returns for a file that cannot be an agent's executable.
var (
	// ErrNotExist is returned for a path where there is no file.
	ErrNotExist = errors.New("harnesses: no such file")
	// ErrIsDir is returned for a path that is a directory.
	ErrIsDir = errors.New("harnesses: is a directory")
	// ErrNotExecutable is returned for a file this system will not run: no
	// execute permission, or on Windows no .exe, .cmd, .bat or .com extension.
	ErrNotExecutable = errors.New("harnesses: not executable")
)

// Check reports whether bin is a file the system can run: it exists, is not a
// directory and is executable. When it is, Check runs `bin --version` with the
// effective PATH and returns the first line printed, or "" when nothing
// answered within a few seconds. The error wraps ErrNotExist, ErrIsDir or
// ErrNotExecutable.
func Check(ctx context.Context, bin string) (ver string, err error) {
	fi, err := os.Stat(bin)
	switch {
	case err != nil:
		return "", fmt.Errorf("%w: %s", ErrNotExist, bin)
	case fi.IsDir():
		return "", fmt.Errorf("%w: %s", ErrIsDir, bin)
	case !executable(runtime.GOOS, bin, fi.Mode()):
		return "", fmt.Errorf("%w: %s", ErrNotExecutable, bin)
	}
	return version(ctx, bin, shellenv.Effective()), nil
}

// executable reports whether a file named name with mode m runs on goos.
func executable(goos, name string, m os.FileMode) bool {
	if goos == "windows" {
		switch strings.ToLower(filepath.Ext(name)) {
		case ".exe", ".cmd", ".bat", ".com":
			return true
		}
		return false
	}
	return m.Perm()&0o111 != 0
}

// Found is one harness as Discover saw it.
type Found struct {
	Harness agentmanager.Harness
	// Path is the executable's full path; empty when it is not on the PATH.
	Path string
	// Version is the first line `--version` printed, or empty.
	Version string
	// Override is set when Path is the executable SetBinary stored rather
	// than one found on the PATH.
	Override bool
}

// Discover looks every harness up on path, or takes the executable SetBinary
// stored for it, and asks each one found for its version, all at once, each
// answer bounded to a few seconds. The result has one entry per harness, in
// the order of All.
func Discover(ctx context.Context, path string) []Found {
	out := make([]Found, len(All))
	var wg sync.WaitGroup
	for i, h := range All {
		out[i].Harness = h
		p, ok := shellenv.Lookup(string(h), path)
		if b := Binary(h); b != "" {
			p, ok, out[i].Override = b, true, true
		}
		if !ok {
			continue
		}
		out[i].Path = p
		wg.Add(1)
		go func() {
			defer wg.Done()
			out[i].Version = version(ctx, p, path)
		}()
	}
	wg.Wait()
	return out
}

// version runs `bin --version` with path as PATH and returns the first
// non-empty output line, or "".
func version(ctx context.Context, bin, path string) string {
	ctx, cancel := context.WithTimeout(ctx, versionTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, bin, "--version")
	cmd.Env = append(cmd.Environ(), "PATH="+path)
	cmd.WaitDelay = 200 * time.Millisecond
	b, err := cmd.Output()
	if err != nil && len(b) == 0 {
		return ""
	}
	for _, l := range strings.Split(string(b), "\n") {
		if l = strings.TrimSpace(l); l != "" {
			return l
		}
	}
	return ""
}
