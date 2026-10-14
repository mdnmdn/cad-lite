package acp

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"time"

	"agentmanager"
	"agentmanager/internal/proc"
	"agentmanager/jsonrpc"
)

type nopLogger struct{}

func (nopLogger) Debugf(string, ...any) {}

type toolInfo struct{ server, tool string }

type session struct {
	cfg  agentmanager.Config
	log  agentmanager.Logger
	q    *agentmanager.Queue
	p    *proc.Process
	conn *jsonrpc.Conn
	caps capabilities

	mu      sync.Mutex
	id      string
	turn    bool
	closed  bool // no more calls: closed by us or the agent exited
	closing bool // we asked for the shutdown
	loading bool // session/load replay in progress: updates are dropped
	perms   map[string]json.RawMessage
	sysDone bool

	tools map[string]toolInfo // touched by the reader goroutine only

	finished  chan struct{}
	closeOnce sync.Once
}

func newSession(cfg agentmanager.Config, p *proc.Process) *session {
	s := &session{cfg: cfg, log: cfg.Logger, q: agentmanager.NewQueue(), p: p,
		perms: map[string]json.RawMessage{}, tools: map[string]toolInfo{}, finished: make(chan struct{})}
	if s.log == nil {
		s.log = nopLogger{}
	}
	var secrets []string
	for _, m := range cfg.MCPServers {
		if m.Token != "" {
			secrets = append(secrets, m.Token)
		}
	}
	s.conn = jsonrpc.NewConn(p.Stdout(), p.Stdin(), s, jsonrpc.WithFrameLogger(func(dir string, line []byte) {
		l := string(line)
		for _, t := range secrets {
			l = strings.ReplaceAll(l, t, "***")
		}
		s.log.Debugf("acp %s %s", dir, l)
	}))
	return s
}

func (s *session) logEvent(level, msg string) { s.q.Push(agentmanager.Log{Level: level, Message: msg}) }

// abort kills a session that never finished opening; no Exited event is delivered.
func (s *session) abort() {
	s.mu.Lock()
	s.closed, s.closing = true, true
	s.mu.Unlock()
	_ = s.conn.CloseWrite()
	_ = s.p.Close(0)
	s.q.Close()
}

// watch delivers the final Exited event once the agent is gone.
func (s *session) watch() {
	defer close(s.finished)
	select {
	case <-s.conn.Done():
	case <-s.p.Done():
		select {
		case <-s.conn.Done():
		case <-time.After(2 * time.Second):
		}
	}
	select {
	case <-s.p.Done():
	case <-time.After(2 * time.Second):
	}
	s.mu.Lock()
	s.closed = true
	intended := s.closing
	s.perms = map[string]json.RawMessage{}
	s.mu.Unlock()
	ex := agentmanager.Exited{}
	if !intended {
		ex.Err = "agent exited unexpectedly"
		select {
		case <-s.p.Done():
			if err := s.p.ExitErr(); err != nil {
				ex.Err += ": " + err.Error()
			}
		default:
		}
		if tail := s.p.StderrTail(); tail != "" {
			ex.Err += "; stderr: " + tail
		}
	}
	s.q.Push(ex)
	s.q.Close()
}

func (s *session) ID() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.id
}

func (s *session) Events() <-chan agentmanager.Event { return s.q.Events() }

// Prompt implements agentmanager.Session.
func (s *session) Prompt(ctx context.Context, text string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return agentmanager.ErrClosed
	}
	if s.turn {
		s.mu.Unlock()
		return errors.New("acp: a turn is already running")
	}
	s.turn = true
	id := s.id
	if !s.sysDone && s.cfg.SystemPrompt != "" {
		text = s.cfg.SystemPrompt + "\n\n" + text
	}
	s.sysDone = true
	s.mu.Unlock()
	params := map[string]any{"sessionId": id, "prompt": []map[string]any{{"type": "text", "text": text}}}
	if _, err := s.conn.CallAsync("session/prompt", params, s.turnDone); err != nil {
		s.mu.Lock()
		s.turn = false
		s.mu.Unlock()
		return agentmanager.ErrClosed
	}
	return nil
}

// turnDone runs on the reader goroutine when the session/prompt response arrives.
func (s *session) turnDone(res json.RawMessage, rerr *jsonrpc.Error) {
	if rerr != nil && rerr.Code == jsonrpc.CodeInternal && rerr.Message == jsonrpc.ErrClosed.Error() {
		return // connection died; Exited reports it
	}
	end := agentmanager.TurnEnd{Reason: agentmanager.StopEndTurn}
	if rerr != nil {
		end = agentmanager.TurnEnd{Reason: agentmanager.StopFailed, Err: rerr.Message}
	} else {
		var r struct {
			StopReason string `json:"stopReason"`
		}
		_ = json.Unmarshal(res, &r)
		switch agentmanager.StopReason(r.StopReason) {
		case agentmanager.StopCancelled, agentmanager.StopMaxTokens, agentmanager.StopMaxTurns, agentmanager.StopRefusal:
			end.Reason = agentmanager.StopReason(r.StopReason)
		case agentmanager.StopEndTurn:
		default:
			s.logEvent("warn", "unknown stopReason "+r.StopReason)
		}
	}
	s.mu.Lock()
	s.turn = false
	s.perms = map[string]json.RawMessage{} // the agent already retired these ids
	s.mu.Unlock()
	s.q.Push(end)
}

// Cancel implements agentmanager.Session: pending permissions are answered cancelled first.
func (s *session) Cancel() error {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return agentmanager.ErrClosed
	}
	if !s.turn {
		s.mu.Unlock()
		return nil
	}
	id := s.id
	s.mu.Unlock()
	s.cancelPermissions()
	if err := s.conn.Notify("session/cancel", map[string]any{"sessionId": id}); err != nil {
		return agentmanager.ErrClosed
	}
	return nil
}

// Close implements agentmanager.Session.
func (s *session) Close() error {
	s.closeOnce.Do(func() {
		s.mu.Lock()
		already := s.closed
		s.closing = true
		turn, id := s.turn, s.id
		s.mu.Unlock()
		if !already {
			s.cancelPermissions()
			if turn {
				_ = s.conn.Notify("session/cancel", map[string]any{"sessionId": id})
			}
		}
		_ = s.conn.CloseWrite()
		select {
		case <-s.p.Done():
		case <-time.After(closeGrace):
		}
		_ = s.p.Close(time.Millisecond)
	})
	<-s.finished
	return nil
}
