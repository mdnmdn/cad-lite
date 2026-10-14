package jsonrpc_test

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	"agentmanager/jsonrpc"
)

type handler struct {
	mu    sync.Mutex
	notes []string
	reqs  func(c *jsonrpc.Conn, id json.RawMessage, method string, params json.RawMessage) bool
}

func (h *handler) HandleRequest(c *jsonrpc.Conn, id json.RawMessage, method string, params json.RawMessage) bool {
	if h.reqs == nil {
		return false
	}
	return h.reqs(c, id, method, params)
}

func (h *handler) HandleNotification(c *jsonrpc.Conn, method string, params json.RawMessage) {
	h.mu.Lock()
	h.notes = append(h.notes, method+" "+string(params))
	h.mu.Unlock()
}

func (h *handler) notesLen() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return len(h.notes)
}

// pair returns two connected Conns.
func pair(t *testing.T, ha, hb jsonrpc.Handler, oa, ob []jsonrpc.Option) (a, b *jsonrpc.Conn) {
	t.Helper()
	ar, bw := io.Pipe()
	br, aw := io.Pipe()
	a = jsonrpc.NewConn(ar, aw, ha, oa...)
	b = jsonrpc.NewConn(br, bw, hb, ob...)
	t.Cleanup(func() { a.CloseWrite(); b.CloseWrite() })
	return
}

func echoHandler() *handler {
	return &handler{reqs: func(c *jsonrpc.Conn, id json.RawMessage, method string, params json.RawMessage) bool {
		if method == "echo" {
			c.Reply(id, json.RawMessage(params))
			return true
		}
		if method == "fail" {
			c.ReplyError(id, -1, "nope")
			return true
		}
		return false
	}}
}

func ctx(t *testing.T) context.Context {
	c, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	t.Cleanup(cancel)
	return c
}

func TestCallResponse(t *testing.T) {
	a, _ := pair(t, nil, echoHandler(), nil, nil)
	var out struct{ X int }
	if err := a.Call(ctx(t), "echo", map[string]int{"X": 7}, &out); err != nil || out.X != 7 {
		t.Fatalf("out=%+v err=%v", out, err)
	}
	err := a.Call(ctx(t), "fail", nil, nil)
	var je *jsonrpc.Error
	if !errors.As(err, &je) || je.Code != -1 || je.Message != "nope" {
		t.Fatalf("err=%v", err)
	}
	err = a.Call(ctx(t), "missing", nil, nil)
	if !errors.As(err, &je) || je.Code != jsonrpc.CodeMethodNotFound {
		t.Fatalf("err=%v", err)
	}
}

func TestNotifications(t *testing.T) {
	hb := echoHandler()
	a, _ := pair(t, nil, hb, nil, nil)
	a.Notify("ping", map[string]int{"n": 1})
	a.Notify("pong", nil)
	deadline := time.Now().Add(5 * time.Second)
	for hb.notesLen() < 2 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	hb.mu.Lock()
	defer hb.mu.Unlock()
	if len(hb.notes) != 2 || hb.notes[0] != `ping {"n":1}` || hb.notes[1] != "pong " {
		t.Fatalf("notes=%q", hb.notes)
	}
}

func TestServerInitiatedRequest(t *testing.T) {
	// b asks a for "who" while a is waiting on a call to b; a's handler answers it.
	ha := &handler{reqs: func(c *jsonrpc.Conn, id json.RawMessage, method string, params json.RawMessage) bool {
		if method == "who" {
			c.Reply(id, "alice")
			return true
		}
		return false
	}}
	hb := &handler{reqs: func(c *jsonrpc.Conn, id json.RawMessage, method string, params json.RawMessage) bool {
		go func() {
			var who string
			if err := c.Call(context.Background(), "who", nil, &who); err != nil {
				c.ReplyError(id, 1, err.Error())
				return
			}
			c.Reply(id, "hello "+who)
		}()
		return true
	}}
	a, _ := pair(t, ha, hb, nil, nil)
	var out string
	if err := a.Call(ctx(t), "greet", nil, &out); err != nil || out != "hello alice" {
		t.Fatalf("out=%q err=%v", out, err)
	}
}

func TestIDsEchoedVerbatim(t *testing.T) {
	ar, bw := io.Pipe()
	br, aw := io.Pipe()
	h := &handler{reqs: func(c *jsonrpc.Conn, id json.RawMessage, method string, params json.RawMessage) bool {
		c.Reply(id, "ok")
		return true
	}}
	c := jsonrpc.NewConn(ar, aw, h)
	defer c.CloseWrite()
	go fmt.Fprint(bw, `{"jsonrpc":"2.0","id":"abc-1","method":"x"}`+"\n")
	line, _ := bufio.NewReader(br).ReadString('\n')
	if !strings.Contains(line, `"id":"abc-1"`) || !strings.Contains(line, `"result":"ok"`) {
		t.Fatalf("line=%s", line)
	}
}

func TestReplyNilResultIsNull(t *testing.T) {
	ar, bw := io.Pipe()
	br, aw := io.Pipe()
	h := &handler{reqs: func(c *jsonrpc.Conn, id json.RawMessage, method string, params json.RawMessage) bool {
		c.Reply(id, nil)
		return true
	}}
	c := jsonrpc.NewConn(ar, aw, h)
	defer c.CloseWrite()
	go fmt.Fprint(bw, `{"jsonrpc":"2.0","id":1,"method":"x"}`+"\n")
	line, _ := bufio.NewReader(br).ReadString('\n')
	if !strings.Contains(line, `"result":null`) {
		t.Fatalf("line=%s", line)
	}
}

func TestConcurrentCalls(t *testing.T) {
	a, _ := pair(t, nil, echoHandler(), nil, nil)
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			var out int
			if err := a.Call(ctx(t), "echo", i, &out); err != nil || out != i {
				t.Errorf("i=%d out=%d err=%v", i, out, err)
			}
		}()
	}
	wg.Wait()
}

func TestCallAsync(t *testing.T) {
	a, _ := pair(t, nil, echoHandler(), nil, nil)
	ch := make(chan string, 1)
	id, err := a.CallAsync("echo", "hi", func(res json.RawMessage, e *jsonrpc.Error) { ch <- string(res) })
	if err != nil || id == 0 {
		t.Fatal(id, err)
	}
	select {
	case s := <-ch:
		if s != `"hi"` {
			t.Fatal(s)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timeout")
	}
}

func TestEOFFailsPending(t *testing.T) {
	ar, bw := io.Pipe()
	_, aw := io.Pipe()
	a := jsonrpc.NewConn(ar, aw, nil)
	defer a.CloseWrite()
	errc := make(chan error, 1)
	go func() { errc <- a.Call(context.Background(), "never", nil, nil) }()
	asyncErr := make(chan *jsonrpc.Error, 1)
	time.Sleep(20 * time.Millisecond)
	a.CallAsync("never2", nil, func(_ json.RawMessage, e *jsonrpc.Error) { asyncErr <- e })
	bw.Close()
	select {
	case err := <-errc:
		if !errors.Is(err, jsonrpc.ErrClosed) {
			t.Fatalf("err=%v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timeout")
	}
	if e := <-asyncErr; e == nil {
		t.Fatal("async not failed")
	}
	select {
	case <-a.Done():
	case <-time.After(time.Second):
		t.Fatal("Done not closed")
	}
	if err := a.Call(context.Background(), "x", nil, nil); !errors.Is(err, jsonrpc.ErrClosed) {
		t.Fatalf("after EOF: %v", err)
	}
	if err := a.Notify("x", nil); err == nil {
		// writer may still be alive; only Call is required to fail
		_ = err
	}
}

func TestCallContextCancel(t *testing.T) {
	ar, _ := io.Pipe()
	_, aw := io.Pipe()
	a := jsonrpc.NewConn(ar, aw, nil)
	c, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if err := a.Call(c, "x", nil, nil); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("err=%v", err)
	}
}

func TestRawLines(t *testing.T) {
	ar, bw := io.Pipe()
	br, aw := io.Pipe()
	lines := make(chan string, 4)
	c := jsonrpc.NewConn(ar, aw, nil, jsonrpc.WithRawHandler(func(l []byte) { lines <- string(l) }))
	go fmt.Fprint(bw, "{\"type\":\"system\"}\n\nnot json at all\n")
	for _, want := range []string{`{"type":"system"}`, "not json at all"} {
		select {
		case got := <-lines:
			if got != want {
				t.Fatalf("got %q want %q", got, want)
			}
		case <-time.After(5 * time.Second):
			t.Fatal("timeout")
		}
	}
	go c.SendRaw(map[string]string{"type": "user"})
	line, _ := bufio.NewReader(br).ReadString('\n')
	if line != "{\"type\":\"user\"}\n" {
		t.Fatalf("line=%q", line)
	}
	c.CloseWrite()
}

func TestMalformedIgnoredAndLargeLine(t *testing.T) {
	a, _ := pair(t, nil, echoHandler(), nil, nil)
	big := strings.Repeat("x", 1<<20)
	var out string
	if err := a.Call(ctx(t), "echo", big, &out); err != nil || len(out) != len(big) {
		t.Fatalf("len=%d err=%v", len(out), err)
	}
	ar, bw := io.Pipe()
	_, aw := io.Pipe()
	c := jsonrpc.NewConn(ar, aw, nil)
	go fmt.Fprint(bw, "garbage\n{broken\n")
	time.Sleep(20 * time.Millisecond)
	select {
	case <-c.Done():
		t.Fatal("reader died on malformed input")
	default:
	}
}

func TestWriterNeverBlocks(t *testing.T) {
	_, aw := io.Pipe() // nobody reads
	ar, _ := io.Pipe()
	a := jsonrpc.NewConn(ar, aw, nil)
	done := make(chan struct{})
	go func() {
		for i := 0; i < 5000; i++ {
			a.Notify("n", i)
		}
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Notify blocked")
	}
}

func TestCloseWriteFlushesThenEOF(t *testing.T) {
	ar, _ := io.Pipe()
	br, aw := io.Pipe()
	a := jsonrpc.NewConn(ar, aw, nil)
	for i := 0; i < 3; i++ {
		a.Notify("n", i)
	}
	a.CloseWrite()
	data, err := io.ReadAll(br)
	if err != nil || strings.Count(string(data), "\n") != 3 {
		t.Fatalf("data=%q err=%v", data, err)
	}
	if err := a.Notify("late", nil); !errors.Is(err, jsonrpc.ErrClosed) {
		t.Fatalf("err=%v", err)
	}
}

func TestFrameLogger(t *testing.T) {
	var mu sync.Mutex
	var got []string
	log := func(dir string, line []byte) {
		mu.Lock()
		got = append(got, dir+" "+string(line))
		mu.Unlock()
	}
	a, _ := pair(t, nil, echoHandler(), []jsonrpc.Option{jsonrpc.WithFrameLogger(log)}, nil)
	if err := a.Call(ctx(t), "echo", 1, nil); err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(got) != 2 || !strings.HasPrefix(got[0], "out ") || !strings.HasPrefix(got[1], "in ") {
		t.Fatalf("got=%q", got)
	}
}

func TestErrorString(t *testing.T) {
	e := &jsonrpc.Error{Code: -32601, Message: "x"}
	if !strings.Contains(e.Error(), "-32601") || !strings.Contains(e.Error(), "x") {
		t.Fatal(e.Error())
	}
}
