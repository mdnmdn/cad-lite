package codex

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"agentmanager"
	"agentmanager/internal/proc"
	"agentmanager/jsonrpc"
)

type session struct {
	p      *proc.Process
	conn   *jsonrpc.Conn
	q      *agentmanager.Queue
	policy agentmanager.PermissionPolicy
	log    agentmanager.Logger

	mu         sync.Mutex
	threadID   string
	closed     bool // Close called or process gone: methods return ErrClosed
	closing    bool // Close initiated by the caller (exit is expected)
	turnActive bool
	// model overrides the thread's model from the next turn/start on; "" keeps it.
	model         string
	turnID        string // "" until turn/start is acknowledged
	cancelPending bool   // Cancel arrived before the turn id was known
	asks          map[string]*ask
	st            streamState

	exited   chan struct{}
	closeOne sync.Once
}

func newSession(p *proc.Process, cfg agentmanager.Config) *session {
	s := &session{
		p:      p,
		q:      agentmanager.NewQueue(),
		policy: cfg.AutoApprove,
		log:    cfg.Logger,
		asks:   map[string]*ask{},
		st:     newStreamState(),
		exited: make(chan struct{}),
	}
	var opts []jsonrpc.Option
	if cfg.Logger != nil {
		opts = append(opts, jsonrpc.WithFrameLogger(func(dir string, line []byte) {
			cfg.Logger.Debugf("codex %s %s", dir, line)
		}))
	}
	s.conn = jsonrpc.NewConn(p.Stdout(), p.Stdin(), s, opts...)
	go s.watch()
	return s
}

// watch waits for the process to go away and emits the final Exited event.
func (s *session) watch() {
	<-s.conn.Done()
	select {
	case <-s.p.Done():
	case <-time.After(2 * time.Second):
		_ = s.p.Close(0)
	}
	s.mu.Lock()
	expected := s.closing
	s.closed = true
	s.turnActive = false
	s.asks = map[string]*ask{}
	s.mu.Unlock()
	ev := agentmanager.Exited{}
	if !expected {
		ev.Err = "codex exited unexpectedly"
		if err := s.p.ExitErr(); err != nil {
			ev.Err = fmt.Sprintf("codex exited: %v", err)
		}
		if tail := s.p.StderrTail(); tail != "" {
			ev.Err += ": " + tail
		}
	}
	s.q.Push(ev)
	s.q.Close()
	close(s.exited)
}

func (s *session) debugf(format string, a ...any) {
	if s.log != nil {
		s.log.Debugf(format, a...)
	}
}

func (s *session) logEvent(level, msg string) { s.q.Push(agentmanager.Log{Level: level, Message: msg}) }

// ID implements agentmanager.Session.
func (s *session) ID() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.threadID
}

// Events implements agentmanager.Session.
func (s *session) Events() <-chan agentmanager.Event { return s.q.Events() }

// Prompt implements agentmanager.Session. The turn runs asynchronously; a failed turn/start is
// reported as TurnEnd{Failed}.
func (s *session) Prompt(ctx context.Context, text string) error {
	return s.PromptImages(ctx, text, nil)
}

// AcceptsImages implements agentmanager.ImagePrompter; turn/start takes image input items.
func (s *session) AcceptsImages() bool { return true }

// PromptImages implements agentmanager.ImagePrompter; each image is an input item
// {"type":"image","url":"data:<mime>;base64,..."} after the text item.
func (s *session) PromptImages(_ context.Context, text string, images []agentmanager.Image) error {
	s.mu.Lock()
	switch {
	case s.closed:
		s.mu.Unlock()
		return agentmanager.ErrClosed
	case s.turnActive:
		s.mu.Unlock()
		return errors.New("codex: a turn is already running")
	}
	s.turnActive, s.turnID, s.cancelPending = true, "", false
	thread, model := s.threadID, s.model
	s.mu.Unlock()

	input := []any{map[string]any{"type": "text", "text": text, "text_elements": []any{}}}
	for _, im := range images {
		input = append(input, map[string]any{"type": "image", "url": "data:" + im.MIME + ";base64," + base64.StdEncoding.EncodeToString(im.Data)})
	}
	params := map[string]any{
		"threadId": thread,
		"input":    input,
	}
	if model != "" {
		params["model"] = model
	}
	_, err := s.conn.CallAsync("turn/start", params, s.turnAcked)
	if err != nil {
		s.mu.Lock()
		s.turnActive = false
		s.mu.Unlock()
		return agentmanager.ErrClosed
	}
	return nil
}

// turnAcked runs on the reader goroutine with turn/start's response.
func (s *session) turnAcked(res json.RawMessage, rerr *jsonrpc.Error) {
	if rerr != nil {
		s.mu.Lock()
		was := s.turnActive
		s.turnActive = false
		s.cancelPending = false
		s.mu.Unlock()
		if was && rerr.Message != jsonrpc.ErrClosed.Error() {
			s.q.Push(agentmanager.TurnEnd{Reason: agentmanager.StopFailed, Err: rerr.Message})
		}
		return
	}
	var r struct {
		Turn struct {
			ID string `json:"id"`
		} `json:"turn"`
	}
	_ = json.Unmarshal(res, &r)
	s.mu.Lock()
	if !s.turnActive {
		s.mu.Unlock()
		return
	}
	s.turnID = r.Turn.ID
	interrupt := s.cancelPending
	s.cancelPending = false
	thread := s.threadID
	s.mu.Unlock()
	if interrupt {
		s.interrupt(thread, r.Turn.ID)
	}
}

func (s *session) interrupt(thread, turn string) {
	_, err := s.conn.CallAsync("turn/interrupt", map[string]any{"threadId": thread, "turnId": turn},
		func(_ json.RawMessage, e *jsonrpc.Error) {
			if e != nil && e.Message != jsonrpc.ErrClosed.Error() {
				s.logEvent("warn", "turn/interrupt: "+e.Message)
			}
		})
	if err != nil {
		s.debugf("codex interrupt: %v", err)
	}
}

// Cancel implements agentmanager.Session: parked asks are answered cancel first, then the turn is
// interrupted (deferred until turn/start is acknowledged if its ack has not arrived).
func (s *session) Cancel() error {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return agentmanager.ErrClosed
	}
	if !s.turnActive {
		s.mu.Unlock()
		return nil
	}
	asks := s.asks
	s.asks = map[string]*ask{}
	turn, thread := s.turnID, s.threadID
	if turn == "" {
		s.cancelPending = true
	}
	s.mu.Unlock()
	for _, a := range asks {
		_ = s.conn.Reply(a.id, a.result(""))
	}
	if turn != "" {
		s.interrupt(thread, turn)
	}
	return nil
}

// SetModel implements agentmanager.ModelSetter. Codex takes the model as a turn/start override,
// so nothing goes over the wire until the next prompt, and the override sticks for later turns.
func (s *session) SetModel(_ context.Context, model string) error {
	if model == "" {
		return agentmanager.ErrNoModel
	}
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return agentmanager.ErrClosed
	}
	s.model = model
	s.mu.Unlock()
	s.q.Push(agentmanager.ModelsChanged{Current: model})
	return nil
}

// modalityVision maps a model's inputModalities to Vision: nil is unknown.
func modalityVision(mods []string) agentmanager.Vision {
	if mods == nil {
		return agentmanager.VisionUnknown
	}
	for _, m := range mods {
		if m == "image" {
			return agentmanager.VisionYes
		}
	}
	return agentmanager.VisionNo
}

// listModels asks the app-server for its models (model/list, first page) and reports them; an
// error, such as an older app-server without the method, is ignored.
func (s *session) listModels(current string) {
	_, err := s.conn.CallAsync("model/list", map[string]any{}, func(res json.RawMessage, e *jsonrpc.Error) {
		if e != nil {
			s.debugf("codex model/list: %s", e.Message)
			return
		}
		var r struct {
			Data []struct {
				ID          string `json:"id"`
				Model       string `json:"model"`
				DisplayName string `json:"displayName"`
				// InputModalities is absent on app-servers that predate it.
				InputModalities []string `json:"inputModalities"`
			} `json:"data"`
		}
		if json.Unmarshal(res, &r) != nil {
			return
		}
		var avail []agentmanager.ModelInfo
		for _, m := range r.Data {
			id := m.Model
			if id == "" {
				id = m.ID
			}
			if id != "" {
				avail = append(avail, agentmanager.ModelInfo{ID: id, Name: m.DisplayName, Vision: modalityVision(m.InputModalities)})
			}
		}
		if len(avail) > 0 {
			s.mu.Lock()
			if s.model != "" {
				current = s.model
			}
			s.mu.Unlock()
			s.q.Push(agentmanager.ModelsChanged{Current: current, Available: avail})
		}
	})
	if err != nil {
		s.debugf("codex model/list: %v", err)
	}
}

// Close implements agentmanager.Session.
func (s *session) Close() error {
	s.shutdown()
	return nil
}

func (s *session) shutdown() {
	s.closeOne.Do(func() {
		s.mu.Lock()
		s.closed = true
		s.closing = true
		s.mu.Unlock()
		_ = s.conn.CloseWrite()
		_ = s.p.Close(closeGrace)
	})
	<-s.exited
}
