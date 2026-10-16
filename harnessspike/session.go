package harnessspike

import (
	"context"
	"crypto/rand"
	"errors"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"agentmanager"
	"github.com/sausheong/harness/providers/openai"
	hr "github.com/sausheong/harness/runtime"
	"github.com/sausheong/harness/session"
	"github.com/sausheong/harness/tool"
	hmcp "github.com/sausheong/harness/tools/mcp"
)

type conversation struct {
	mu        sync.Mutex
	rt        *hr.Runtime
	q         *agentmanager.Queue
	key       string
	cancel    context.CancelFunc
	done      chan struct{}
	closed    bool
	closeOnce sync.Once
	closeErr  error
}

// Open starts an in-process Harness session with only the trusted cad MCP tools.
// Provider settings are snapshotted per session; credentials never enter the transcript.
func Open(ctx context.Context, cfg agentmanager.Config, cloud Config) (agentmanager.Session, error) {
	p, url, err := validate(cloud)
	if err != nil {
		return nil, err
	}
	return open(ctx, cfg, cloud.Provider, p, url)
}

func open(ctx context.Context, cfg agentmanager.Config, provider string, p Profile, url string) (agentmanager.Session, error) {
	if cfg.Model != "" {
		p.Model = cfg.Model
	}
	key := cfg.ResumeID
	if key == "" {
		key = provider + "-" + rand.Text()
	}
	store := session.NewStore(filepath.Join(cfg.Cwd, ".cad-lite", "agent", "harness"))
	sess, err := store.Load(Harness, key)
	if err != nil {
		return nil, errors.New("cannot load Harness conversation")
	}
	var servers []hmcp.ServerConfig
	for _, srv := range cfg.MCPServers {
		if srv.Name != "cad" {
			sess.Close()
			return nil, errors.New("Harness spike only accepts the cad drawing server")
		}
		servers = append(servers, hmcp.ServerConfig{Name: srv.Name, URL: srv.URL, Headers: map[string]string{"Authorization": "Bearer " + srv.Token}, ConnectTimeout: 15 * time.Second})
	}
	rt, err := hr.BuildRuntimeContext(ctx, hr.RuntimeDeps{}, hr.RuntimeInputs{
		Provider: openai.NewOpenAIProviderWithKind(p.APIKey, url, "openai-compatible"), Tools: tool.NewRegistry(), Session: sess,
	}, hr.AgentSpec{ID: Harness, Name: "Cad-lite Harness spike", Model: "openai-compatible/" + p.Model,
		Workspace: cfg.Cwd, SystemPrompt: cfg.SystemPrompt, MaxTurns: 25, Loop: hr.LoopConfig{MaxToolConcurrency: 1}, MCPServers: servers})
	if err != nil {
		sess.Close()
		return nil, errors.New("cannot initialize Harness drawing tools")
	}
	s := &conversation{rt: rt, q: agentmanager.NewQueue(), key: key}
	s.q.Push(agentmanager.SessionStarted{SessionID: key, Model: p.Model})
	s.q.Push(agentmanager.ModelsChanged{Current: p.Model, Available: []agentmanager.ModelInfo{{ID: p.Model, Vision: agentmanager.VisionNo}}})
	return s, nil
}

func (s *conversation) ID() string                        { return s.key }
func (s *conversation) Events() <-chan agentmanager.Event { return s.q.Events() }
func (s *conversation) Prompt(ctx context.Context, text string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return agentmanager.ErrClosed
	}
	if s.done != nil {
		return errors.New("Harness turn already running")
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	events, err := s.rt.Run(ctx, text, nil)
	if err != nil {
		cancel()
		return errors.New("cannot start Harness turn")
	}
	s.cancel, s.done = cancel, make(chan struct{})
	go s.pump(ctx, events, s.done)
	return nil
}

func (s *conversation) pump(ctx context.Context, events <-chan hr.AgentEvent, done chan struct{}) {
	reason, message := agentmanager.StopEndTurn, ""
	for e := range events {
		switch e.Type {
		case hr.EventTextDelta:
			s.q.Push(agentmanager.TextDelta{Text: e.Text})
		case hr.EventToolCallReady:
			if e.ToolCall != nil {
				c := e.ToolCall
				s.q.Push(agentmanager.ToolCallStart{Call: agentmanager.ToolCall{ID: c.ID, Title: c.Name, RawInput: c.Input, Kind: agentmanager.KindOther, Status: agentmanager.ToolInProgress, MCPServer: "cad", MCPTool: strings.TrimPrefix(c.Name, "mcp__cad__")}})
			}
		case hr.EventToolResult:
			if e.ToolCall != nil && e.Result != nil {
				status, output := agentmanager.ToolCompleted, e.Result.Output
				if e.Result.Error != "" {
					status, output = agentmanager.ToolFailed, e.Result.Error
				}
				s.q.Push(agentmanager.ToolCallUpdate{Update: agentmanager.ToolCallPatch{ID: e.ToolCall.ID, Status: &status, Output: &output}})
			}
		case hr.EventError:
			reason, message = agentmanager.StopFailed, "Harness provider request failed; check the API key, model and service availability"
		case hr.EventAborted:
			reason = agentmanager.StopCancelled
		}
	}
	s.mu.Lock()
	if ctx.Err() != nil {
		reason, message = agentmanager.StopCancelled, ""
	}
	s.cancel()
	s.cancel, s.done = nil, nil
	s.q.Push(agentmanager.TurnEnd{Reason: reason, Err: message})
	close(done)
	s.mu.Unlock()
}

func (s *conversation) Cancel() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cancel != nil {
		s.cancel()
	}
	return nil
}
func (s *conversation) RespondPermission(string, agentmanager.PermissionAnswer) error {
	return errors.New("Harness spike exposes only automatically approved drawing tools")
}
func (s *conversation) Close() error {
	s.closeOnce.Do(func() {
		s.mu.Lock()
		s.closed = true
		if s.cancel != nil {
			s.cancel()
		}
		done := s.done
		s.mu.Unlock()
		cleanup := make(chan struct{})
		go func() {
			defer close(cleanup)
			if done != nil {
				<-done
			}
			s.rt.Close()
			s.rt.Session.Close()
			s.q.Push(agentmanager.Exited{})
			s.q.Close()
		}()
		select {
		case <-cleanup:
		case <-time.After(10 * time.Second):
			s.closeErr = errors.New("Harness cleanup timed out")
		}
	})
	return s.closeErr
}
