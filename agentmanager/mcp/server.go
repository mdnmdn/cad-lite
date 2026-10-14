package mcp

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

const (
	maxBody        = 4 << 20
	defaultVersion = "2025-06-18"
)

// writeTimeout bounds a whole response, from the end of the request headers, handler time included.
// Servers read it when they start; tests shorten it.
var writeTimeout atomic.Int64

func init() { writeTimeout.Store(int64(60 * time.Second)) }

// The budget of one request with write timeout wt (60 s): a tool call must start (take the call
// slot) within startWithin (30 s) of the request's arrival; its handler's context expires at
// wt - replyMargin (55 s); the last replyMargin (5 s) is for encoding and writing the reply.

func startWithin(wt time.Duration) time.Duration { return wt / 2 }

func replyMargin(wt time.Duration) time.Duration { return wt / 12 }

// maxBatch caps the frames of one JSON-RPC batch array.
const maxBatch = 100

var supportedVersions = map[string]bool{"2025-06-18": true, "2025-03-26": true, "2024-11-05": true}

// Options configures a Server.
type Options struct {
	Name, Version, Instructions string
	Serialize                   *bool // serialise handler calls; nil = true
	Logger                      func(string)
}

// Endpoint is where a started Server listens; URL is http://127.0.0.1:P/mcp.
type Endpoint struct{ URL, Token string }

// Server is an in-process streamable-HTTP MCP server (loopback, bearer token).
type Server struct {
	opts Options

	mu      sync.Mutex
	tools   map[string]Tool
	order   []string
	started bool
	closed  bool
	srv     *http.Server
	wt      time.Duration // write timeout, fixed at Start
	port    int
	token   string
	cancel  context.CancelFunc
	done    chan struct{}

	// callSem serialises handler calls (capacity 1); waiting honours the request context.
	callSem chan struct{}
}

// NewServer returns a stopped Server.
func NewServer(o Options) *Server {
	return &Server{opts: o, tools: map[string]Tool{}, callSem: make(chan struct{}, 1)}
}

// Register adds or replaces a tool, before or after Start; tools/list reflects the current set.
func (s *Server) Register(t Tool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.tools[t.Name]; !ok {
		s.order = append(s.order, t.Name)
	}
	s.tools[t.Name] = t
}

// Start binds 127.0.0.1:0, generates the token and returns the endpoint.
func (s *Server) Start() (Endpoint, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.started || s.closed {
		return Endpoint{}, errors.New("mcp: server already started or closed")
	}
	var tok [32]byte
	if _, err := rand.Read(tok[:]); err != nil {
		return Endpoint{}, err
	}
	ln, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		return Endpoint{}, fmt.Errorf("mcp: listen: %w", err)
	}
	s.token = hex.EncodeToString(tok[:])
	s.port = ln.Addr().(*net.TCPAddr).Port
	ctx, cancel := context.WithCancel(context.Background())
	s.cancel = cancel
	s.wt = time.Duration(writeTimeout.Load())
	mux := http.NewServeMux()
	mux.HandleFunc("/mcp", s.handle)
	s.srv = &http.Server{
		Handler:           mux,
		BaseContext:       func(net.Listener) context.Context { return ctx },
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       2 * time.Minute,
		WriteTimeout:      s.wt,
	}
	s.done = make(chan struct{})
	srv, done := s.srv, s.done
	go func() {
		defer close(done)
		_ = srv.Serve(ln)
	}()
	s.started = true
	return Endpoint{URL: fmt.Sprintf("http://127.0.0.1:%d/mcp", s.port), Token: s.token}, nil
}

// Close shuts the server down with a 2 s grace and waits.
func (s *Server) Close() error {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return nil
	}
	s.closed = true
	srv, cancel, done := s.srv, s.cancel, s.done
	s.mu.Unlock()
	if srv == nil {
		return nil
	}
	ctx, stop := context.WithTimeout(context.Background(), 2*time.Second)
	defer stop()
	err := srv.Shutdown(ctx)
	if err != nil {
		cancel()
		_ = srv.Close()
	}
	cancel()
	<-done
	return nil
}

func (s *Server) logf(format string, a ...any) {
	if s.opts.Logger != nil {
		s.opts.Logger(fmt.Sprintf(format, a...))
	}
}

func loopbackHost(h string) bool {
	return h == "127.0.0.1" || h == "localhost" || h == "::1"
}

func (s *Server) hostOK(host string) bool {
	return host == fmt.Sprintf("127.0.0.1:%d", s.port) || host == fmt.Sprintf("localhost:%d", s.port)
}

func (s *Server) handle(w http.ResponseWriter, r *http.Request) {
	arrival := time.Now() // the budget counts from here, not from the end of the body
	if !s.hostOK(r.Host) {
		http.Error(w, "forbidden host", http.StatusForbidden)
		return
	}
	if o := r.Header.Get("Origin"); o != "" {
		u, err := url.Parse(o)
		if err != nil || !loopbackHost(u.Hostname()) {
			http.Error(w, "forbidden origin", http.StatusForbidden)
			return
		}
	}
	got, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
	if !ok || subtle.ConstantTimeCompare([]byte(got), []byte(s.token)) != 1 {
		w.Header().Set("WWW-Authenticate", "Bearer")
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", "POST")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxBody))
	if err != nil {
		var mbe *http.MaxBytesError
		if errors.As(err, &mbe) {
			http.Error(w, "body too large", http.StatusRequestEntityTooLarge)
			return
		}
		http.Error(w, "read error", http.StatusBadRequest)
		return
	}
	// Handlers get a context that expires when their reply would no longer fit in the write
	// timeout. Calls that cannot start by startBy are refused, so they never run.
	ctx, cancel := context.WithDeadline(r.Context(), arrival.Add(s.wt-replyMargin(s.wt)))
	defer cancel()
	startBy, cancelStart := context.WithDeadline(ctx, arrival.Add(startWithin(s.wt)))
	defer cancelStart()
	trimmed := strings.TrimSpace(string(body))
	if strings.HasPrefix(trimmed, "[") {
		var raws []json.RawMessage
		if err := json.Unmarshal(body, &raws); err != nil {
			writeJSON(w, errResp(nil, codeParse, "parse error: "+err.Error()))
			return
		}
		if len(raws) == 0 {
			writeJSON(w, errResp(nil, codeInvalidRequest, "empty batch"))
			return
		}
		if len(raws) > maxBatch {
			writeJSON(w, errResp(nil, codeInvalidRequest, fmt.Sprintf("batch of %d frames; at most %d per request", len(raws), maxBatch)))
			return
		}
		var out []response
		for _, raw := range raws {
			if resp, ok := s.process(ctx, startBy, raw); ok {
				out = append(out, resp)
			}
		}
		if len(out) == 0 {
			w.WriteHeader(http.StatusAccepted)
			return
		}
		writeJSON(w, out)
		return
	}
	resp, ok := s.process(ctx, startBy, body)
	if !ok {
		w.WriteHeader(http.StatusAccepted)
		return
	}
	writeJSON(w, resp)
}

const (
	codeParse          = -32700
	codeInvalidRequest = -32600
	codeMethodNotFound = -32601
	codeInvalidParams  = -32602
)

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type response struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  any             `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

func errResp(id json.RawMessage, code int, msg string) response {
	if len(id) == 0 {
		id = json.RawMessage("null")
	}
	return response{JSONRPC: "2.0", ID: id, Error: &rpcError{Code: code, Message: msg}}
}

func writeJSON(w http.ResponseWriter, v any) {
	b, err := json.Marshal(v)
	if err != nil {
		http.Error(w, "encode error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(b)
}

type request struct {
	ID     json.RawMessage `json:"id"`
	Method string          `json:"method"`
	Params json.RawMessage `json:"params"`
	Result json.RawMessage `json:"result"`
	Error  json.RawMessage `json:"error"`
}

// process handles one frame; ok=false means no reply (notification or client response). A tool
// call starts only while startBy is live, and its handler gets ctx.
func (s *Server) process(ctx, startBy context.Context, raw []byte) (response, bool) {
	var req request
	if err := json.Unmarshal(raw, &req); err != nil {
		var probe any
		if json.Unmarshal(raw, &probe) != nil {
			return errResp(nil, codeParse, "parse error: "+err.Error()), true
		}
		return errResp(nil, codeInvalidRequest, "invalid request"), true
	}
	if req.Method == "" {
		if len(req.Result) > 0 || len(req.Error) > 0 {
			return response{}, false
		}
		return errResp(req.ID, codeInvalidRequest, "missing method"), true
	}
	if len(req.ID) == 0 {
		return response{}, false
	}
	res, rerr := s.dispatch(ctx, startBy, req.Method, req.Params)
	if rerr != nil {
		return errResp(req.ID, rerr.Code, rerr.Message), true
	}
	return response{JSONRPC: "2.0", ID: req.ID, Result: res}, true
}

func (s *Server) dispatch(ctx, startBy context.Context, method string, params json.RawMessage) (any, *rpcError) {
	switch method {
	case "initialize":
		var p struct {
			ProtocolVersion string `json:"protocolVersion"`
		}
		if len(params) > 0 {
			_ = json.Unmarshal(params, &p)
		}
		ver := defaultVersion
		if supportedVersions[p.ProtocolVersion] {
			ver = p.ProtocolVersion
		}
		name, version := s.opts.Name, s.opts.Version
		if name == "" {
			name = "agentmanager-mcp"
		}
		if version == "" {
			version = "0.1.0"
		}
		res := map[string]any{
			"protocolVersion": ver,
			"capabilities":    map[string]any{"tools": map[string]any{"listChanged": false}},
			"serverInfo":      map[string]any{"name": name, "version": version},
		}
		if s.opts.Instructions != "" {
			res["instructions"] = s.opts.Instructions
		}
		return res, nil
	case "ping":
		return map[string]any{}, nil
	case "tools/list":
		s.mu.Lock()
		list := make([]map[string]any, 0, len(s.order))
		for _, n := range s.order {
			t := s.tools[n]
			schema := t.InputSchema
			if len(schema) == 0 {
				schema = json.RawMessage(`{"type":"object"}`)
			}
			list = append(list, map[string]any{"name": t.Name, "description": t.Description, "inputSchema": schema})
		}
		s.mu.Unlock()
		return map[string]any{"tools": list}, nil
	case "tools/call":
		var p struct {
			Name      string          `json:"name"`
			Arguments json.RawMessage `json:"arguments"`
		}
		if err := json.Unmarshal(params, &p); err != nil || p.Name == "" {
			return nil, &rpcError{codeInvalidParams, "invalid params: tools/call needs a name"}
		}
		if len(p.Arguments) == 0 || string(p.Arguments) == "null" {
			p.Arguments = json.RawMessage("{}")
		}
		return s.call(ctx, startBy, p.Name, p.Arguments).wire(), nil
	}
	return nil, &rpcError{codeMethodNotFound, "method not found: " + method}
}

func (s *Server) call(ctx, startBy context.Context, name string, args json.RawMessage) (res Result) {
	s.mu.Lock()
	t, ok := s.tools[name]
	s.mu.Unlock()
	if !ok || t.Handler == nil {
		return Errorf("unknown tool: %s", name)
	}
	if s.opts.Serialize == nil || *s.opts.Serialize {
		select {
		case s.callSem <- struct{}{}:
			defer func() { <-s.callSem }()
		case <-startBy.Done():
			return s.abandoned(name, ctx)
		}
	}
	// select picks randomly when both cases are ready, so re-check: a call whose request is
	// already gone, or too old to finish before the write timeout, must never reach its handler.
	if startBy.Err() != nil {
		return s.abandoned(name, ctx)
	}
	defer func() {
		if r := recover(); r != nil {
			s.logf("tool %s panicked: %v", name, r)
			res = Errorf("tool %s panicked: %v", name, r)
		}
	}()
	res, err := t.Handler(ctx, args)
	if err != nil {
		return Errorf("%s", err.Error())
	}
	return res
}

// abandoned answers a call that did not start: its request ended (ctx) or it waited past
// startWithin.
func (s *Server) abandoned(name string, ctx context.Context) Result {
	if err := ctx.Err(); err != nil {
		s.logf("tool %s: request cancelled before the handler started: %v", name, err)
		return Errorf("%s: cancelled while waiting for another tool call: %v", name, err)
	}
	s.logf("tool %s: not started within %v of its request; refused", name, startWithin(s.wt))
	return Errorf("%s: not started within %v of the request because other tool calls were running; nothing was done, retry", name, startWithin(s.wt))
}
