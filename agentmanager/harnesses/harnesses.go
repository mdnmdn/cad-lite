// Package harnesses opens a session on any supported backend by Harness id.
package harnesses

import (
	"context"
	"fmt"
	"os/exec"
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
// cfg.Binary is empty the harness's executable is looked up on the effective
// PATH (the process PATH plus the one shellenv.Set stored), and the child
// gets that PATH in its environment so the agent's own subprocesses (node,
// bun) are found too.
func Open(ctx context.Context, cfg agentmanager.Config) (agentmanager.Session, error) {
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

// Available reports where the harness's default executable is on the
// effective PATH.
func Available(h agentmanager.Harness) (path string, ok bool) {
	return shellenv.Resolve(string(h))
}

// Found is one harness as Discover saw it.
type Found struct {
	Harness agentmanager.Harness
	// Path is the executable's full path; empty when it is not on the PATH.
	Path string
	// Version is the first line `--version` printed, or empty.
	Version string
}

// Discover looks every harness up on path and asks each one found for its
// version, all at once, each answer bounded to a few seconds. The result has
// one entry per harness, in the order of All.
func Discover(ctx context.Context, path string) []Found {
	out := make([]Found, len(All))
	var wg sync.WaitGroup
	for i, h := range All {
		out[i].Harness = h
		p, ok := shellenv.Lookup(string(h), path)
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
