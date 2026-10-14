package mcp_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"agentmanager/mcp"
)

func counting(t *testing.T) (mcp.Endpoint, *atomic.Int32) {
	t.Helper()
	var n atomic.Int32
	s := mcp.NewServer(mcp.Options{})
	s.Register(mcp.Tool{Name: "count", Handler: func(context.Context, json.RawMessage) (mcp.Result, error) {
		n.Add(1)
		return mcp.Text("ok"), nil
	}})
	ep, err := s.Start()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return ep, &n
}

func countBatch(n int) string {
	frames := make([]string, n)
	for i := range frames {
		frames[i] = fmt.Sprintf(`{"jsonrpc":"2.0","id":%d,"method":"tools/call","params":{"name":"count"}}`, i)
	}
	return "[" + strings.Join(frames, ",") + "]"
}

// G49: a JSON-RPC batch array had no length cap.
func TestBatchLengthCap(t *testing.T) {
	ep, n := counting(t)
	resp, b := do(t, ep, "POST", countBatch(100), nil)
	var out []map[string]any
	if err := json.Unmarshal(b, &out); err != nil || resp.StatusCode != 200 || len(out) != 100 || n.Load() != 100 {
		t.Fatalf("100 frames: %d, %d replies, %d calls", resp.StatusCode, len(out), n.Load())
	}
	resp, b = do(t, ep, "POST", countBatch(101), nil)
	var e map[string]any
	if err := json.Unmarshal(b, &e); err != nil || !bytes.Contains(b, []byte("-32600")) || !bytes.Contains(b, []byte("100")) {
		t.Fatalf("101 frames: %d %.200s", resp.StatusCode, b)
	}
	if n.Load() != 100 {
		t.Fatalf("an over-long batch ran %d calls", n.Load()-100)
	}
}

// G49: the HTTP server had no write timeout, so a stuck handler held its connection forever.
func TestWriteTimeoutEndsOverlongResponse(t *testing.T) {
	defer mcp.SetWriteTimeout(300 * time.Millisecond)()
	s := mcp.NewServer(mcp.Options{})
	s.Register(mcp.Tool{Name: "slow", Handler: func(context.Context, json.RawMessage) (mcp.Result, error) {
		time.Sleep(700 * time.Millisecond)
		return mcp.Text("late"), nil
	}})
	ep, _ := s.Start()
	defer s.Close()
	req, _ := http.NewRequest("POST", ep.URL, strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"slow"}}`))
	req.Header.Set("Authorization", "Bearer "+ep.Token)
	resp, err := http.DefaultClient.Do(req)
	if err == nil {
		resp.Body.Close()
		t.Fatalf("a response written past the write timeout arrived: %d", resp.StatusCode)
	}
}

// A call that cannot start well inside the write timeout gets an error reply and never runs, rather
// than running and losing its reply to a closed connection.
func TestCallThatCannotStartInTimeIsRefused(t *testing.T) {
	defer mcp.SetWriteTimeout(1500 * time.Millisecond)()
	s := mcp.NewServer(mcp.Options{})
	started := make(chan struct{})
	var fastRan atomic.Int32
	s.Register(mcp.Tool{Name: "hold", Handler: func(context.Context, json.RawMessage) (mcp.Result, error) {
		close(started)
		time.Sleep(1200 * time.Millisecond)
		return mcp.Text("ok"), nil
	}})
	s.Register(mcp.Tool{Name: "fast", Handler: func(context.Context, json.RawMessage) (mcp.Result, error) {
		fastRan.Add(1)
		return mcp.Text("ok"), nil
	}})
	ep, _ := s.Start()
	defer s.Close()
	held := make(chan map[string]any)
	go func() { held <- call(t, ep, "hold", `{}`) }()
	<-started
	r := call(t, ep, "fast", `{}`)
	if r["isError"] != true || !strings.Contains(firstText(r), "not started") {
		t.Fatalf("queued call: %v", r)
	}
	if h := <-held; h["isError"] != false {
		t.Fatalf("holding call: %v", h)
	}
	if fastRan.Load() != 0 {
		t.Fatal("the refused call ran")
	}
}
