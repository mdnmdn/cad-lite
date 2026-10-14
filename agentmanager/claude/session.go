package claude

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"agentmanager"
	"agentmanager/internal/proc"
	"agentmanager/jsonrpc"
)

const (
	closeGrace = 10 * time.Second
	drainGrace = 2 * time.Second
)

type pending struct{ ask permissionAsk }

type session struct {
	cfg     agentmanager.Config
	p       *proc.Process
	conn    *jsonrpc.Conn
	q       *agentmanager.Queue
	mcpPath string

	finished chan struct{}
	rmOnce   sync.Once

	mu          sync.Mutex
	id          string
	started     bool
	turn        bool
	interrupted bool
	closing     bool
	dead        bool
	intSeq      int
	curMsg      string
	streamed    map[string]bool
	tools       map[string]agentmanager.ToolCall
	perms       map[string]*pending
	// ctl holds the callbacks of our control requests that await a control_response.
	ctl    map[string]func(controlResult)
	ctlSeq int
}

// controlResult is Claude's answer to one of our control requests.
type controlResult struct {
	ok       bool
	errMsg   string
	response json.RawMessage
}

func newSession(cfg agentmanager.Config, p *proc.Process, mcpPath string) *session {
	s := &session{
		cfg: cfg, p: p, q: agentmanager.NewQueue(), mcpPath: mcpPath,
		finished: make(chan struct{}),
		id:       cfg.ResumeID,
		streamed: map[string]bool{},
		tools:    map[string]agentmanager.ToolCall{},
		perms:    map[string]*pending{},
		ctl:      map[string]func(controlResult){},
	}
	opts := []jsonrpc.Option{jsonrpc.WithRawHandler(s.handle)}
	if cfg.Logger != nil {
		opts = append(opts, jsonrpc.WithFrameLogger(func(dir string, b []byte) {
			cfg.Logger.Debugf("claude %s %s", dir, b)
		}))
	}
	s.conn = jsonrpc.NewConn(p.Stdout(), p.Stdin(), nil, opts...)
	go s.monitor()
	s.requestModels()
	return s
}

// control sends a control request; done, when set, runs on the reader goroutine with the answer.
func (s *session) control(subtype string, fields map[string]any, done func(controlResult)) (string, error) {
	s.mu.Lock()
	s.ctlSeq++
	id := fmt.Sprintf("gd-ctl-%d", s.ctlSeq)
	if done != nil {
		s.ctl[id] = done
	}
	s.mu.Unlock()
	req := map[string]any{"subtype": subtype}
	for k, v := range fields {
		req[k] = v
	}
	if err := s.conn.SendRaw(map[string]any{"type": "control_request", "request_id": id, "request": req}); err != nil {
		s.mu.Lock()
		delete(s.ctl, id)
		s.mu.Unlock()
		return id, agentmanager.ErrClosed
	}
	return id, nil
}

// requestModels sends the initialize control request, whose answer lists the models Claude offers;
// they are reported as ModelsChanged. A CLI that does not answer it reports nothing.
func (s *session) requestModels() {
	_, _ = s.control("initialize", nil, func(r controlResult) {
		var res struct {
			Models []struct {
				Value       string `json:"value"`
				DisplayName string `json:"displayName"`
			} `json:"models"`
		}
		if !r.ok || json.Unmarshal(r.response, &res) != nil {
			return
		}
		var avail []agentmanager.ModelInfo
		for _, m := range res.Models {
			if m.Value != "" {
				avail = append(avail, agentmanager.ModelInfo{ID: m.Value, Name: m.DisplayName})
			}
		}
		if len(avail) > 0 {
			s.q.Push(agentmanager.ModelsChanged{Current: s.cfg.Model, Available: avail})
		}
	})
}

// SetModel implements agentmanager.ModelSetter with the set_model control request.
func (s *session) SetModel(ctx context.Context, model string) error {
	if model == "" {
		return agentmanager.ErrNoModel
	}
	s.mu.Lock()
	gone := s.gone()
	s.mu.Unlock()
	if gone {
		return agentmanager.ErrClosed
	}
	ch := make(chan controlResult, 1)
	id, err := s.control("set_model", map[string]any{"model": model}, func(r controlResult) { ch <- r })
	if err != nil {
		return err
	}
	select {
	case r := <-ch:
		if !r.ok {
			return fmt.Errorf("claude: set model %s: %s", model, r.errMsg)
		}
		s.q.Push(agentmanager.ModelsChanged{Current: model})
		return nil
	case <-ctx.Done():
		s.mu.Lock()
		delete(s.ctl, id)
		s.mu.Unlock()
		return ctx.Err()
	case <-s.conn.Done():
		return agentmanager.ErrClosed
	}
}

func (s *session) log(format string, a ...any) {
	if s.cfg.Logger != nil {
		s.cfg.Logger.Debugf("claude: "+format, a...)
	}
}

func removeFile(path string) {
	if path != "" {
		_ = os.Remove(path)
	}
}

func (s *session) removeConfig() { s.rmOnce.Do(func() { removeFile(s.mcpPath) }) }

// monitor waits for the process and its stdout to end, then emits the final Exited event.
func (s *session) monitor() {
	defer close(s.finished)
	select {
	case <-s.conn.Done():
		select {
		case <-s.p.Done():
		case <-time.After(drainGrace):
			_ = s.p.Close(0)
		}
	case <-s.p.Done():
		select {
		case <-s.conn.Done():
		case <-time.After(drainGrace):
		}
	}
	<-s.p.Done()
	s.mu.Lock()
	closing := s.closing
	s.dead = true
	s.mu.Unlock()
	exit := agentmanager.Exited{}
	if !closing {
		exit.Err = "claude exited unexpectedly"
		if err := s.p.ExitErr(); err != nil {
			exit.Err += ": " + err.Error()
		}
		if tail := s.p.StderrTail(); tail != "" {
			exit.Err += "\nstderr: " + tail
		}
	}
	s.q.Push(exit)
	s.q.Close()
	s.removeConfig()
}

func (s *session) ID() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.id
}

func (s *session) Events() <-chan agentmanager.Event { return s.q.Events() }

func (s *session) gone() bool { return s.closing || s.dead }

func (s *session) Prompt(ctx context.Context, text string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	s.mu.Lock()
	if s.gone() {
		s.mu.Unlock()
		return agentmanager.ErrClosed
	}
	if s.turn {
		s.mu.Unlock()
		return errors.New("claude: a turn is already running")
	}
	s.turn = true
	s.interrupted = false
	s.mu.Unlock()
	err := s.conn.SendRaw(map[string]any{
		"type": "user",
		"message": map[string]any{
			"role":    "user",
			"content": []any{map[string]any{"type": "text", "text": text}},
		},
	})
	if err != nil {
		s.mu.Lock()
		s.turn = false
		s.mu.Unlock()
		return agentmanager.ErrClosed
	}
	return nil
}

// denyAll answers every outstanding permission request with a cancelled denial.
func (s *session) denyAll() {
	s.mu.Lock()
	ps := s.perms
	s.perms = map[string]*pending{}
	s.mu.Unlock()
	for id, p := range ps {
		_ = s.send(id, p, agentmanager.PermissionAnswer{Cancelled: true})
	}
}

func (s *session) Cancel() error {
	s.mu.Lock()
	if s.gone() {
		s.mu.Unlock()
		return agentmanager.ErrClosed
	}
	if !s.turn {
		s.mu.Unlock()
		return nil
	}
	s.interrupted = true
	s.intSeq++
	id := fmt.Sprintf("gd-int-%d", s.intSeq)
	s.mu.Unlock()
	s.denyAll()
	err := s.conn.SendRaw(map[string]any{
		"type":       "control_request",
		"request_id": id,
		"request":    map[string]any{"subtype": "interrupt", "reason": "interrupt"},
	})
	if err != nil {
		return agentmanager.ErrClosed
	}
	return nil
}

func (s *session) RespondPermission(requestID string, ans agentmanager.PermissionAnswer) error {
	s.mu.Lock()
	if s.gone() {
		s.mu.Unlock()
		return agentmanager.ErrClosed
	}
	p, ok := s.perms[requestID]
	s.mu.Unlock()
	if !ok {
		return fmt.Errorf("claude: no pending permission request %q", requestID)
	}
	if err := s.respond(requestID, p, ans); err != nil {
		return err
	}
	return nil
}

// respond sends the answer and forgets the request; an unknown option id is an error and leaves the
// request pending.
func (s *session) respond(requestID string, p *pending, ans agentmanager.PermissionAnswer) error {
	if err := s.send(requestID, p, ans); err != nil {
		return err
	}
	s.mu.Lock()
	delete(s.perms, requestID)
	s.mu.Unlock()
	return nil
}

func (s *session) send(requestID string, p *pending, ans agentmanager.PermissionAnswer) error {
	resp := map[string]any{}
	switch {
	case ans.Cancelled:
		resp["behavior"] = "deny"
		resp["message"] = "Cancelled"
	case ans.OptionID == "deny":
		resp["behavior"] = "deny"
		resp["message"] = "Denied by user"
	case ans.OptionID == "allow" || strings.HasPrefix(ans.OptionID, "allow_always:"):
		input := p.ask.Input
		if len(input) == 0 {
			input = json.RawMessage("{}")
		}
		resp["behavior"] = "allow"
		resp["updatedInput"] = input
		if ans.OptionID != "allow" {
			var n int
			if _, err := fmt.Sscanf(ans.OptionID, "allow_always:%d", &n); err != nil || n < 0 || n >= len(p.ask.Suggestions) {
				return fmt.Errorf("claude: unknown permission option %q", ans.OptionID)
			}
			resp["updatedPermissions"] = []json.RawMessage{p.ask.Suggestions[n]}
		}
	default:
		return fmt.Errorf("claude: unknown permission option %q", ans.OptionID)
	}
	err := s.conn.SendRaw(map[string]any{
		"type": "control_response",
		"response": map[string]any{
			"subtype": "success", "request_id": requestID, "response": resp,
		},
	})
	if err != nil {
		return agentmanager.ErrClosed
	}
	return nil
}

func (s *session) Close() error {
	s.mu.Lock()
	if s.closing {
		s.mu.Unlock()
		<-s.finished
		return nil
	}
	s.closing = true
	running := s.turn
	s.intSeq++
	id := fmt.Sprintf("gd-int-%d", s.intSeq)
	s.mu.Unlock()
	s.denyAll()
	if running {
		_ = s.conn.SendRaw(map[string]any{
			"type":       "control_request",
			"request_id": id,
			"request":    map[string]any{"subtype": "interrupt", "reason": "interrupt"},
		})
	}
	_ = s.conn.CloseWrite()
	select {
	case <-s.p.Done():
	case <-time.After(closeGrace):
	}
	_ = s.p.Close(0)
	<-s.finished
	s.removeConfig()
	return nil
}
