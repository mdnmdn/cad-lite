// Package acp is a generic Agent Client Protocol (v1) client backend; OpenCode is a preset.
package acp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"agentmanager"
	"agentmanager/internal/proc"
	"agentmanager/jsonrpc"
)

// Command is the agent process to launch.
type Command struct {
	Path string
	Args []string
	Env  []string
}

type opener struct {
	cmd Command
	// authHint is appended to "authentication required" errors.
	authHint string
	// envMCP, when set, builds extra env that injects MCP servers for agents that do not accept
	// them over the wire; the process is restarted with it when http MCP is not advertised.
	envMCP func([]agentmanager.MCPServer) ([]string, error)
}

// New returns an Opener that launches cmd and speaks ACP over its stdio.
func New(cmd Command) agentmanager.Opener { return opener{cmd: cmd} }

var (
	handshakeTimeout = 30 * time.Second
	requestTimeout   = 10 * time.Second
	closeGrace       = 10 * time.Second
)

type capabilities struct {
	loadSession bool
	mcpHTTP     bool
	mcpSSE      bool
	// image is promptCapabilities.image: the agent takes image content blocks.
	image bool
}

type initResult struct {
	ProtocolVersion   int `json:"protocolVersion"`
	AgentCapabilities struct {
		LoadSession        bool `json:"loadSession"`
		PromptCapabilities struct {
			Image bool `json:"image"`
		} `json:"promptCapabilities"`
		McpCapabilities struct {
			HTTP bool `json:"http"`
			SSE  bool `json:"sse"`
		} `json:"mcpCapabilities"`
	} `json:"agentCapabilities"`
	AuthMethods []struct {
		ID string `json:"id"`
	} `json:"authMethods"`
	Meta struct {
		DefaultAuthMethodID string `json:"defaultAuthMethodId"`
	} `json:"_meta"`
}

type sessionResult struct {
	SessionID string `json:"sessionId"`
	Models    *struct {
		CurrentModelID  string `json:"currentModelId"`
		AvailableModels []struct {
			ModelID string `json:"modelId"`
			Name    string `json:"name"`
		} `json:"availableModels"`
	} `json:"models"`
	ConfigOptions []struct {
		ID           string         `json:"id"`
		Category     string         `json:"category"`
		CurrentValue any            `json:"currentValue"`
		Options      []configChoice `json:"options"`
	} `json:"configOptions"`
}

// configChoice is one value of a select config option, or a group of them.
type configChoice struct {
	Value   any            `json:"value"`
	Name    string         `json:"name"`
	Options []configChoice `json:"options"`
}

// flatten appends the values of cs (groups expanded) to out as models.
func flatten(cs []configChoice, out []agentmanager.ModelInfo) []agentmanager.ModelInfo {
	for _, c := range cs {
		if v, ok := c.Value.(string); ok && v != "" {
			out = append(out, agentmanager.ModelInfo{ID: v, Name: c.Name})
		}
		out = flatten(c.Options, out)
	}
	return out
}

// Open implements agentmanager.Opener.
func (o opener) Open(ctx context.Context, cfg agentmanager.Config) (agentmanager.Session, error) {
	s, authIDs, err := o.start(ctx, cfg, nil)
	if err != nil {
		return nil, err
	}
	servers := cfg.MCPServers
	var wire []map[string]any
	if len(servers) > 0 {
		switch {
		case s.caps.mcpHTTP:
			wire = httpMCPEntries(servers)
		case o.envMCP != nil:
			env, err := o.envMCP(servers)
			if err != nil {
				s.abort()
				return nil, err
			}
			s.abort()
			if s, authIDs, err = o.start(ctx, cfg, env); err != nil {
				return nil, err
			}
		default:
			s.logEvent("warn", "agent does not advertise http MCP support; MCP servers not injected")
		}
	}
	if wire == nil {
		wire = []map[string]any{}
	}
	if err := s.openSession(ctx, wire, authIDs, o.authHint); err != nil {
		s.abort()
		return nil, err
	}
	go s.watch()
	return s, nil
}

func (o opener) start(ctx context.Context, cfg agentmanager.Config, extraEnv []string) (*session, []string, error) {
	path := o.cmd.Path
	if cfg.Binary != "" {
		path = cfg.Binary
	}
	args := append(append([]string(nil), o.cmd.Args...), cfg.Args...)
	env := append(append(append([]string(nil), o.cmd.Env...), cfg.Env...), extraEnv...)
	p, err := proc.Start(proc.Options{Path: path, Args: args, Dir: cfg.Cwd, Env: env})
	if err != nil {
		return nil, nil, fmt.Errorf("acp: start %s: %w", path, err)
	}
	s := newSession(cfg, p)
	hctx, cancel := context.WithTimeout(ctx, handshakeTimeout)
	defer cancel()
	var ir initResult
	err = s.conn.Call(hctx, "initialize", map[string]any{
		"protocolVersion": 1,
		"clientInfo":      map[string]any{"name": "cad-lite", "version": "0"},
		"clientCapabilities": map[string]any{
			"fs":       map[string]any{"readTextFile": false, "writeTextFile": false},
			"terminal": false,
		},
	}, &ir)
	if err != nil {
		return nil, nil, s.fail("initialize", err)
	}
	if ir.ProtocolVersion != 1 {
		s.abort()
		return nil, nil, fmt.Errorf("acp: agent speaks protocol version %d, want 1", ir.ProtocolVersion)
	}
	s.caps = capabilities{ir.AgentCapabilities.LoadSession, ir.AgentCapabilities.McpCapabilities.HTTP, ir.AgentCapabilities.McpCapabilities.SSE,
		ir.AgentCapabilities.PromptCapabilities.Image}
	var ids []string
	for _, m := range ir.AuthMethods {
		ids = append(ids, m.ID)
	}
	if id := ir.Meta.DefaultAuthMethodID; id != "" {
		if err := s.conn.Call(hctx, "authenticate", map[string]any{"methodId": id}, nil); err != nil {
			return nil, nil, s.fail("authenticate", err)
		}
	}
	return s, ids, nil
}

// fail tears the session down and builds a handshake error carrying the stderr tail.
func (s *session) fail(step string, err error) error {
	s.abort()
	var je *jsonrpc.Error
	switch {
	case errors.Is(err, context.DeadlineExceeded):
		err = fmt.Errorf("timed out")
	case errors.Is(err, jsonrpc.ErrClosed):
		err = errors.New("agent exited")
	case errors.As(err, &je):
		err = errors.New(je.Message)
	}
	msg := fmt.Sprintf("acp: %s: %v", step, err)
	if tail := s.p.StderrTail(); tail != "" {
		msg += " (stderr: " + tail + ")"
	}
	return errors.New(msg)
}

func (s *session) openSession(ctx context.Context, wire []map[string]any, authIDs []string, hint string) error {
	cwd := s.cfg.Cwd
	if cwd != "" && !filepath.IsAbs(cwd) {
		if abs, err := filepath.Abs(cwd); err == nil {
			cwd = abs
		}
	}
	hctx, cancel := context.WithTimeout(ctx, handshakeTimeout)
	defer cancel()
	params := map[string]any{"cwd": cwd, "mcpServers": wire}
	var res sessionResult
	if s.cfg.ResumeID != "" {
		if !s.caps.loadSession {
			s.abort()
			return errors.New("acp: agent does not support session/load; cannot resume")
		}
		params["sessionId"] = s.cfg.ResumeID
		s.mu.Lock()
		s.loading = true
		s.mu.Unlock()
		err := s.conn.Call(hctx, "session/load", params, &res)
		s.mu.Lock()
		s.loading = false
		s.mu.Unlock()
		if err != nil {
			return s.fail("session/load", err)
		}
		res.SessionID = s.cfg.ResumeID
	} else if err := s.conn.Call(hctx, "session/new", params, &res); err != nil {
		var je *jsonrpc.Error
		if errors.As(err, &je) && je.Code == -32000 {
			s.abort()
			msg := "acp: authentication required"
			if hint != "" {
				msg += ": " + hint
			}
			if len(authIDs) > 0 {
				msg += " (auth methods: " + strings.Join(authIDs, ", ") + ")"
			}
			return errors.New(msg)
		}
		return s.fail("session/new", err)
	}
	s.mu.Lock()
	s.id = res.SessionID
	s.mu.Unlock()
	model, avail := s.selectModel(ctx, &res)
	s.q.Push(agentmanager.SessionStarted{SessionID: res.SessionID, Model: model})
	if len(avail) > 0 {
		s.q.Push(agentmanager.ModelsChanged{Current: model, Available: avail})
	}
	return nil
}

// selectModel records how the agent switches models, applies Config.Model through the wire (never
// argv) and returns the model in effect and the models the agent offers.
func (s *session) selectModel(ctx context.Context, res *sessionResult) (string, []agentmanager.ModelInfo) {
	cur := ""
	var avail []agentmanager.ModelInfo
	if res.Models != nil {
		cur = res.Models.CurrentModelID
		for _, m := range res.Models.AvailableModels {
			if m.ModelID != "" {
				avail = append(avail, agentmanager.ModelInfo{ID: m.ModelID, Name: m.Name})
			}
		}
	}
	configID := ""
	for _, o := range res.ConfigOptions {
		if o.Category == "model" || o.ID == "model" {
			configID = o.ID
			if v, ok := o.CurrentValue.(string); ok {
				cur = v
			}
			if l := flatten(o.Options, nil); len(l) > 0 {
				avail = l
			}
			break
		}
	}
	s.mu.Lock()
	s.modelConfig, s.modelWire = configID, res.Models != nil
	s.mu.Unlock()
	want := s.cfg.Model
	if want == "" || want == cur {
		return cur, avail
	}
	if err := s.applyModel(ctx, res.SessionID, want); err != nil {
		s.logEvent("warn", err.Error())
		return cur, avail
	}
	return want, avail
}

// applyModel switches the session's model with session/set_config_option (preferred) or
// session/set_model, whichever the agent advertised.
func (s *session) applyModel(ctx context.Context, sid, want string) error {
	s.mu.Lock()
	configID, wire := s.modelConfig, s.modelWire
	s.mu.Unlock()
	rctx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()
	var err error
	switch {
	case configID != "":
		err = s.conn.Call(rctx, "session/set_config_option", map[string]any{"sessionId": sid, "configId": configID, "value": want}, nil)
	case wire:
		err = s.conn.Call(rctx, "session/set_model", map[string]any{"sessionId": sid, "modelId": want}, nil)
	default:
		return errors.New("agent offers no model selection; ignoring model " + want)
	}
	if err != nil {
		var je *jsonrpc.Error
		if errors.As(err, &je) {
			err = errors.New(je.Message)
		}
		return fmt.Errorf("set model %s: %w", want, err)
	}
	return nil
}

// SetModel implements agentmanager.ModelSetter.
func (s *session) SetModel(ctx context.Context, model string) error {
	if model == "" {
		return agentmanager.ErrNoModel
	}
	s.mu.Lock()
	closed, sid := s.closed, s.id
	s.mu.Unlock()
	if closed {
		return agentmanager.ErrClosed
	}
	if err := s.applyModel(ctx, sid, model); err != nil {
		if errors.Is(err, jsonrpc.ErrClosed) {
			return agentmanager.ErrClosed
		}
		return fmt.Errorf("acp: %w", err)
	}
	s.q.Push(agentmanager.ModelsChanged{Current: model})
	return nil
}

func httpMCPEntries(servers []agentmanager.MCPServer) []map[string]any {
	out := make([]map[string]any, 0, len(servers))
	for _, m := range servers {
		e := map[string]any{"type": "http", "name": m.Name, "url": m.URL, "headers": []map[string]string{}}
		if m.Token != "" {
			e["headers"] = []map[string]string{{"name": "Authorization", "value": "Bearer " + m.Token}}
		}
		out = append(out, e)
	}
	return out
}

func rawOrNil(b json.RawMessage) json.RawMessage {
	if len(b) == 0 || string(b) == "null" {
		return nil
	}
	return b
}
