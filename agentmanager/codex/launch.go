// Package codex is the Codex app-server (JSON-RPC over stdio) backend.
package codex

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"agentmanager"
	"agentmanager/internal/proc"
	"agentmanager/jsonrpc"
)

const (
	defaultBinary = "codex"
	tokenEnvBase  = "GODRAW_MCP_TOKEN"
	closeGrace    = 10 * time.Second
)

var handshakeTimeout = 10 * time.Second

type opener struct{}

// New returns the Opener for `codex app-server --listen stdio://`.
func New() agentmanager.Opener { return opener{} }

// Open implements agentmanager.Opener. It spawns the app-server, runs the initialize handshake
// and starts (or resumes) a thread; the returned session has already queued SessionStarted.
// The sandbox is read-only and the approval policy asks, with no permissive mode.
func (opener) Open(ctx context.Context, cfg agentmanager.Config) (agentmanager.Session, error) {
	bin := cfg.Binary
	if bin == "" {
		bin = defaultBinary
	}
	args, env := launchArgs(cfg)
	p, err := proc.Start(proc.Options{Path: bin, Args: args, Dir: cfg.Cwd, Env: env})
	if err != nil {
		return nil, fmt.Errorf("codex: start %s: %w", bin, err)
	}
	s := newSession(p, cfg)
	if err := s.handshake(ctx, cfg); err != nil {
		s.shutdown()
		msg := err.Error()
		if tail := p.StderrTail(); tail != "" {
			msg += ": " + tail
		}
		return nil, errors.New("codex: " + msg)
	}
	return s, nil
}

// launchArgs returns the argv (the -c overrides go before the subcommand) and the extra env.
func launchArgs(cfg agentmanager.Config) (args, env []string) {
	add := func(kv string) { args = append(args, "-c", kv) }
	add(`sandbox_mode="read-only"`)
	add(`approval_policy="on-request"`)
	if cfg.Model != "" {
		add("model=" + tomlString(cfg.Model))
	}
	if cfg.SystemPrompt != "" {
		add("developer_instructions=" + tomlString(cfg.SystemPrompt))
	}
	for i, m := range cfg.MCPServers {
		tokenEnv := tokenEnvBase
		if i > 0 {
			tokenEnv += "_" + strconv.Itoa(i)
		}
		add(fmt.Sprintf("mcp_servers.%s={url=%s, bearer_token_env_var=%s, default_tools_approval_mode=\"approve\", tool_timeout_sec=120}",
			tomlKey(m.Name), tomlString(m.URL), tomlString(tokenEnv)))
		env = append(env, tokenEnv+"="+m.Token)
	}
	args = append(args, "app-server", "--listen", "stdio://")
	args = append(args, cfg.Args...)
	env = append(env, cfg.Env...)
	return args, env
}

func tomlKey(k string) string {
	ok := k != ""
	for _, r := range k {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '_' || r == '-') {
			ok = false
		}
	}
	if ok {
		return k
	}
	return tomlString(k)
}

func tomlString(s string) string {
	var b strings.Builder
	b.WriteByte('"')
	for _, r := range s {
		switch {
		case r == '"':
			b.WriteString(`\"`)
		case r == '\\':
			b.WriteString(`\\`)
		case r == '\n':
			b.WriteString(`\n`)
		case r == '\r':
			b.WriteString(`\r`)
		case r == '\t':
			b.WriteString(`\t`)
		case r < 0x20 || r == 0x7f:
			fmt.Fprintf(&b, `\u%04X`, r)
		default:
			b.WriteRune(r)
		}
	}
	b.WriteByte('"')
	return b.String()
}

func (s *session) handshake(ctx context.Context, cfg agentmanager.Config) error {
	call := func(method string, params, result any) error {
		cctx, cancel := context.WithTimeout(ctx, handshakeTimeout)
		defer cancel()
		err := s.conn.Call(cctx, method, params, result)
		switch {
		case errors.Is(err, context.DeadlineExceeded) && ctx.Err() == nil:
			return fmt.Errorf("%s: timed out after %s", method, handshakeTimeout)
		case err != nil:
			return fmt.Errorf("%s: %w", method, err)
		}
		return nil
	}
	if err := call("initialize", map[string]any{
		"clientInfo":   map[string]any{"name": "go-draw", "version": "0"},
		"capabilities": map[string]any{"experimentalApi": true},
	}, nil); err != nil {
		return err
	}
	if err := s.conn.Notify("initialized", map[string]any{}); err != nil {
		return err
	}
	var res struct {
		Thread struct {
			ID string `json:"id"`
		} `json:"thread"`
		Model string `json:"model"`
	}
	if cfg.ResumeID != "" {
		err := call("thread/resume", map[string]any{"threadId": cfg.ResumeID, "cwd": cfg.Cwd, "excludeTurns": true}, &res)
		if err != nil {
			return err
		}
		if res.Thread.ID == "" {
			res.Thread.ID = cfg.ResumeID
		}
	} else if err := call("thread/start", map[string]any{"cwd": cfg.Cwd}, &res); err != nil {
		return err
	}
	if res.Thread.ID == "" {
		return errors.New("thread response without thread id")
	}
	s.mu.Lock()
	s.threadID = res.Thread.ID
	s.mu.Unlock()
	s.q.Push(agentmanager.SessionStarted{SessionID: res.Thread.ID, Model: res.Model})
	return nil
}

var _ jsonrpc.Handler = (*session)(nil)
