package jsonrpc

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"sync"
)

// Handler receives inbound traffic. Calls come from the single reader goroutine; HandleRequest MUST
// return promptly or hand the reply to another goroutine via Conn.Reply (permission asks do this).
// reply reports whether the handler already answered (or will answer) the request itself; when
// false the Conn replies method-not-found.
type Handler interface {
	HandleRequest(c *Conn, id json.RawMessage, method string, params json.RawMessage) (reply bool)
	HandleNotification(c *Conn, method string, params json.RawMessage)
}

// Option configures a Conn.
type Option func(*connConfig)

type connConfig struct {
	frameLogger func(dir string, line []byte)
	rawHandler  func(line []byte)
}

// WithFrameLogger logs every frame; dir is "in" or "out".
func WithFrameLogger(f func(dir string, line []byte)) Option {
	return func(c *connConfig) { c.frameLogger = f }
}

// WithRawHandler delivers every inbound line to f instead of JSON-RPC demultiplexing, so
// non-JSON-RPC protocols (Claude stream-json) reuse the framing and writer. The Handler may be nil.
func WithRawHandler(f func(line []byte)) Option {
	return func(c *connConfig) { c.rawHandler = f }
}

// Conn is a newline-delimited JSON-RPC 2.0 peer over a reader/writer pair.
type Conn struct {
	cfg connConfig
	h   Handler

	mu      sync.Mutex
	cond    *sync.Cond
	queue   [][]byte
	closing bool // CloseWrite requested; writer drains then closes
	wdead   bool // writer goroutine finished
	nextID  int64
	pending map[int64]func(json.RawMessage, *Error)
	rdead   bool

	done chan struct{}
}

type requestFrame struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      *int64          `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type responseFrame struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *Error          `json:"error,omitempty"`
}

// NewConn starts the reader and writer goroutines. h may be nil with WithRawHandler.
func NewConn(r io.Reader, w io.Writer, h Handler, opts ...Option) *Conn {
	c := &Conn{h: h, pending: map[int64]func(json.RawMessage, *Error){}, done: make(chan struct{})}
	for _, o := range opts {
		o(&c.cfg)
	}
	c.cond = sync.NewCond(&c.mu)
	go c.writeLoop(w)
	go c.readLoop(r)
	return c
}

func (c *Conn) writeLoop(w io.Writer) {
	failed := false
	for {
		c.mu.Lock()
		for len(c.queue) == 0 && !c.closing {
			c.cond.Wait()
		}
		if len(c.queue) == 0 {
			c.wdead = true
			c.mu.Unlock()
			break
		}
		line := c.queue[0]
		c.queue[0] = nil
		c.queue = c.queue[1:]
		c.mu.Unlock()
		if failed {
			continue
		}
		if c.cfg.frameLogger != nil {
			c.cfg.frameLogger("out", bytes.TrimRight(line, "\n"))
		}
		if _, err := w.Write(line); err != nil {
			failed = true
			c.mu.Lock()
			c.wdead = true
			c.queue = nil
			c.mu.Unlock()
			continue
		}
	}
	if cl, ok := w.(io.Closer); ok {
		_ = cl.Close()
	}
}

func (c *Conn) readLoop(r io.Reader) {
	br := bufio.NewReaderSize(r, 64<<10)
	for {
		line, err := br.ReadBytes('\n')
		if t := bytes.TrimSpace(line); len(t) > 0 {
			c.handleLine(t)
		}
		if err != nil {
			break
		}
	}
	c.mu.Lock()
	c.rdead = true
	pend := c.pending
	c.pending = map[int64]func(json.RawMessage, *Error){}
	c.mu.Unlock()
	for _, f := range pend {
		f(nil, &Error{Code: CodeInternal, Message: ErrClosed.Error()})
	}
	close(c.done)
}

func (c *Conn) handleLine(line []byte) {
	if c.cfg.frameLogger != nil {
		c.cfg.frameLogger("in", line)
	}
	if c.cfg.rawHandler != nil {
		c.cfg.rawHandler(line)
		return
	}
	var m Message
	if err := json.Unmarshal(line, &m); err != nil {
		return
	}
	hasID := len(m.ID) > 0 && string(m.ID) != "null"
	switch {
	case m.Method != "" && hasID:
		replied := false
		if c.h != nil {
			replied = c.h.HandleRequest(c, m.ID, m.Method, m.Params)
		}
		if !replied {
			_ = c.ReplyError(m.ID, CodeMethodNotFound, "method not found: "+m.Method)
		}
	case m.Method != "":
		if c.h != nil {
			c.h.HandleNotification(c, m.Method, m.Params)
		}
	case hasID:
		var id int64
		if json.Unmarshal(m.ID, &id) != nil {
			return
		}
		c.mu.Lock()
		f := c.pending[id]
		delete(c.pending, id)
		c.mu.Unlock()
		if f != nil {
			f(m.Result, m.Error)
		}
	}
}

func (c *Conn) enqueue(v any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	b = append(b, '\n')
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closing || c.wdead {
		return ErrClosed
	}
	c.queue = append(c.queue, b)
	c.cond.Signal()
	return nil
}

func marshalParams(params any) (json.RawMessage, error) {
	if params == nil {
		return nil, nil
	}
	if raw, ok := params.(json.RawMessage); ok {
		return raw, nil
	}
	b, err := json.Marshal(params)
	return b, err
}

// Call sends a request with a fresh int64 id (registered before the write) and waits for the
// response or ctx. result may be nil.
func (c *Conn) Call(ctx context.Context, method string, params, result any) error {
	type resp struct {
		res json.RawMessage
		err *Error
	}
	ch := make(chan resp, 1)
	id, err := c.CallAsync(method, params, func(res json.RawMessage, e *Error) { ch <- resp{res, e} })
	if err != nil {
		return err
	}
	select {
	case r := <-ch:
		if r.err != nil {
			if r.err.Code == CodeInternal && r.err.Message == ErrClosed.Error() {
				return ErrClosed
			}
			return r.err
		}
		if result != nil && len(r.res) > 0 {
			return json.Unmarshal(r.res, result)
		}
		return nil
	case <-ctx.Done():
		c.mu.Lock()
		delete(c.pending, id)
		c.mu.Unlock()
		return ctx.Err()
	}
}

// Notify sends a notification.
func (c *Conn) Notify(method string, params any) error {
	p, err := marshalParams(params)
	if err != nil {
		return err
	}
	return c.enqueue(requestFrame{JSONRPC: "2.0", Method: method, Params: p})
}

// CallAsync sends a request and returns at once; done runs on the reader goroutine with the response
// (or ErrClosed-style *Error when the connection dies). Used for session/prompt.
func (c *Conn) CallAsync(method string, params any, done func(result json.RawMessage, err *Error)) (id int64, err error) {
	p, err := marshalParams(params)
	if err != nil {
		return 0, err
	}
	c.mu.Lock()
	if c.rdead || c.closing || c.wdead {
		c.mu.Unlock()
		return 0, ErrClosed
	}
	c.nextID++
	id = c.nextID
	c.pending[id] = done
	c.mu.Unlock()
	if err := c.enqueue(requestFrame{JSONRPC: "2.0", ID: &id, Method: method, Params: p}); err != nil {
		c.mu.Lock()
		delete(c.pending, id)
		c.mu.Unlock()
		return 0, err
	}
	return id, nil
}

// Reply answers an inbound request, echoing id verbatim.
func (c *Conn) Reply(id json.RawMessage, result any) error {
	b, err := json.Marshal(result)
	if err != nil {
		return err
	}
	return c.enqueue(responseFrame{JSONRPC: "2.0", ID: id, Result: b})
}

// ReplyError answers an inbound request with an error.
func (c *Conn) ReplyError(id json.RawMessage, code int, msg string) error {
	return c.enqueue(responseFrame{JSONRPC: "2.0", ID: id, Error: &Error{Code: code, Message: msg}})
}

// SendRaw writes an arbitrary JSON line (claude control_response etc.). It never blocks.
func (c *Conn) SendRaw(v any) error { return c.enqueue(v) }

// CloseWrite flushes the queue then closes the writer (stdin EOF).
func (c *Conn) CloseWrite() error {
	c.mu.Lock()
	c.closing = true
	c.cond.Broadcast()
	c.mu.Unlock()
	return nil
}

// Done is closed when the reader exited (EOF); pending calls fail with ErrClosed.
func (c *Conn) Done() <-chan struct{} { return c.done }
