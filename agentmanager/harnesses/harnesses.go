// Package harnesses opens a session on any supported backend by Harness id.
package harnesses

import (
	"context"
	"fmt"
	"os/exec"

	"agentmanager"
	"agentmanager/acp"
	"agentmanager/claude"
	"agentmanager/codex"
)

// Open starts a session on the backend selected by cfg.Harness.
func Open(ctx context.Context, cfg agentmanager.Config) (agentmanager.Session, error) {
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

// Available reports where the harness's default executable is on PATH.
func Available(h agentmanager.Harness) (path string, ok bool) {
	p, err := exec.LookPath(string(h))
	if err != nil {
		return "", false
	}
	return p, true
}
