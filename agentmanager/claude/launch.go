// Package claude is the Claude Code stream-json backend.
package claude

import (
	"context"
	"fmt"

	"agentmanager"
	"agentmanager/internal/proc"
)

type opener struct{}

// New returns the Opener for `claude -p` in stream-json mode.
func New() agentmanager.Opener { return opener{} }

// envHygiene lists variables removed from the child so a launch from inside Claude Code does not
// inherit the parent session's identity.
var envHygiene = []string{
	"CLAUDECODE",
	"CLAUDE_CODE_ENTRYPOINT",
	"CLAUDE_CODE_EXECPATH",
	"CLAUDE_CODE_SESSION_ID",
	"CLAUDE_CODE_SSE_PORT",
}

func buildArgs(cfg agentmanager.Config, mcpPath string) []string {
	args := []string{
		"-p",
		"--input-format", "stream-json",
		"--output-format", "stream-json",
		"--verbose",
		"--include-partial-messages",
		"--permission-prompt-tool", "stdio",
		"--disallowedTools", "AskUserQuestion",
		"--mcp-config", mcpPath,
		"--strict-mcp-config",
	}
	if cfg.Model != "" {
		args = append(args, "--model", cfg.Model)
	}
	if cfg.ResumeID != "" {
		args = append(args, "--resume", cfg.ResumeID)
	}
	if cfg.SystemPrompt != "" {
		args = append(args, "--append-system-prompt", cfg.SystemPrompt)
	}
	return append(args, cfg.Args...)
}

// Open implements agentmanager.Opener. There is no handshake: it returns right after the spawn
// and SessionStarted is emitted when Claude's system/init line arrives (after the first prompt).
func (opener) Open(ctx context.Context, cfg agentmanager.Config) (agentmanager.Session, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	bin := cfg.Binary
	if bin == "" {
		bin = "claude"
	}
	mcpPath, err := writeMCPConfig(cfg.MCPServers)
	if err != nil {
		return nil, fmt.Errorf("claude: write mcp config: %w", err)
	}
	p, err := proc.Start(proc.Options{
		Path:      bin,
		Args:      buildArgs(cfg, mcpPath),
		Dir:       cfg.Cwd,
		Env:       cfg.Env,
		RemoveEnv: envHygiene,
	})
	if err != nil {
		removeFile(mcpPath)
		return nil, fmt.Errorf("claude: start %s: %w", bin, err)
	}
	return newSession(cfg, p, mcpPath), nil
}
