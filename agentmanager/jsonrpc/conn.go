package jsonrpc

import (
	"context"
	"encoding/json"
	"io"
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
type Conn struct{ _ struct{} }

// NewConn starts the reader and writer goroutines. h may be nil with WithRawHandler.
func NewConn(r io.Reader, w io.Writer, h Handler, opts ...Option) *Conn { panic("todo: WP1") }

// Call sends a request with a fresh int64 id (registered before the write) and waits for the
// response or ctx. result may be nil.
func (c *Conn) Call(ctx context.Context, method string, params, result any) error {
	panic("todo: WP1")
}

// Notify sends a notification.
func (c *Conn) Notify(method string, params any) error { panic("todo: WP1") }

// CallAsync sends a request and returns at once; done runs on the reader goroutine with the response
// (or ErrClosed-style *Error when the connection dies). Used for session/prompt.
func (c *Conn) CallAsync(method string, params any, done func(result json.RawMessage, err *Error)) (id int64, err error) {
	panic("todo: WP1")
}

// Reply answers an inbound request, echoing id verbatim.
func (c *Conn) Reply(id json.RawMessage, result any) error { panic("todo: WP1") }

// ReplyError answers an inbound request with an error.
func (c *Conn) ReplyError(id json.RawMessage, code int, msg string) error { panic("todo: WP1") }

// SendRaw writes an arbitrary JSON line (claude control_response etc.). It never blocks.
func (c *Conn) SendRaw(v any) error { panic("todo: WP1") }

// CloseWrite flushes the queue then closes the writer (stdin EOF).
func (c *Conn) CloseWrite() error { panic("todo: WP1") }

// Done is closed when the reader exited (EOF); pending calls fail with ErrClosed.
func (c *Conn) Done() <-chan struct{} { panic("todo: WP1") }
