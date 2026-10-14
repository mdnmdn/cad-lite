package mcp_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"agentmanager/mcp"
)

func start(t *testing.T) (*mcp.Server, mcp.Endpoint) {
	t.Helper()
	s := mcp.NewServer(mcp.Options{Name: "test", Version: "1.2.3", Instructions: "use me"})
	s.Register(mcp.Tool{
		Name: "echo", Description: "echo", InputSchema: json.RawMessage(`{"type":"object"}`),
		Handler: func(ctx context.Context, args json.RawMessage) (mcp.Result, error) {
			return mcp.JSON(map[string]any{"args": json.RawMessage(args)}), nil
		},
	})
	s.Register(mcp.Tool{Name: "fail", Handler: func(context.Context, json.RawMessage) (mcp.Result, error) {
		return mcp.Result{}, errors.New("boom")
	}})
	s.Register(mcp.Tool{Name: "soft", Handler: func(context.Context, json.RawMessage) (mcp.Result, error) {
		return mcp.Errorf("bad %d", 7), nil
	}})
	s.Register(mcp.Tool{Name: "img", Handler: func(context.Context, json.RawMessage) (mcp.Result, error) {
		return mcp.Result{Content: []mcp.Content{mcp.Image([]byte{1, 2, 3})}, Structured: map[string]int{"n": 1}}, nil
	}})
	s.Register(mcp.Tool{Name: "panic", Handler: func(context.Context, json.RawMessage) (mcp.Result, error) {
		panic("oops")
	}})
	ep, err := s.Start()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s, ep
}

func do(t *testing.T, ep mcp.Endpoint, method, body string, hdr map[string]string) (*http.Response, []byte) {
	t.Helper()
	req, _ := http.NewRequest(method, ep.URL, strings.NewReader(body))
	if _, ok := hdr["Authorization"]; !ok {
		req.Header.Set("Authorization", "Bearer "+ep.Token)
	}
	for k, v := range hdr {
		if k == "Host" {
			req.Host = v
		} else if v == "" {
			req.Header.Del(k)
		} else {
			req.Header.Set(k, v)
		}
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return resp, b
}

func rpc(t *testing.T, ep mcp.Endpoint, body string) map[string]any {
	t.Helper()
	resp, b := do(t, ep, "POST", body, nil)
	if resp.StatusCode != 200 {
		t.Fatalf("status %d: %s", resp.StatusCode, b)
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatalf("%v: %s", err, b)
	}
	return m
}

func TestAuth(t *testing.T) {
	_, ep := start(t)
	body := `{"jsonrpc":"2.0","id":1,"method":"ping"}`
	for name, h := range map[string]map[string]string{
		"missing": {"Authorization": ""},
		"wrong":   {"Authorization": "Bearer nope"},
		"scheme":  {"Authorization": "Basic " + ep.Token},
		"empty":   {"Authorization": "Bearer "},
	} {
		resp, _ := do(t, ep, "POST", body, h)
		if resp.StatusCode != 401 {
			t.Errorf("%s: status %d, want 401", name, resp.StatusCode)
		}
	}
	if resp, _ := do(t, ep, "POST", body, nil); resp.StatusCode != 200 {
		t.Errorf("valid: %d", resp.StatusCode)
	}
	if len(ep.Token) != 64 {
		t.Errorf("token length %d", len(ep.Token))
	}
}

func TestHostOrigin(t *testing.T) {
	_, ep := start(t)
	body := `{"jsonrpc":"2.0","id":1,"method":"ping"}`
	cases := []struct {
		hdr  map[string]string
		want int
	}{
		{map[string]string{"Host": "evil.example:80"}, 403},
		{map[string]string{"Host": "127.0.0.1:1"}, 403},
		{map[string]string{"Origin": "http://evil.example"}, 403},
		{map[string]string{"Origin": "null"}, 403},
		{map[string]string{"Origin": "http://localhost:3000"}, 200},
		{map[string]string{"Origin": "http://127.0.0.1:9"}, 200},
	}
	for _, c := range cases {
		if resp, _ := do(t, ep, "POST", body, c.hdr); resp.StatusCode != c.want {
			t.Errorf("%v: %d, want %d", c.hdr, resp.StatusCode, c.want)
		}
	}
	port := ep.URL[strings.LastIndex(ep.URL, ":")+1 : strings.Index(ep.URL, "/mcp")]
	if resp, _ := do(t, ep, "POST", body, map[string]string{"Host": "localhost:" + port}); resp.StatusCode != 200 {
		t.Errorf("localhost host: %d", resp.StatusCode)
	}
}

func TestMethods(t *testing.T) {
	_, ep := start(t)
	for _, m := range []string{"GET", "DELETE", "PUT"} {
		if resp, _ := do(t, ep, m, "", nil); resp.StatusCode != 405 {
			t.Errorf("%s: %d", m, resp.StatusCode)
		}
	}
}

func TestInitialize(t *testing.T) {
	_, ep := start(t)
	for _, c := range []struct{ ask, want string }{
		{"2024-11-05", "2024-11-05"}, {"2025-03-26", "2025-03-26"}, {"2025-06-18", "2025-06-18"}, {"1999-01-01", "2025-06-18"}, {"", "2025-06-18"},
	} {
		m := rpc(t, ep, fmt.Sprintf(`{"jsonrpc":"2.0","id":"a","method":"initialize","params":{"protocolVersion":%q}}`, c.ask))
		res := m["result"].(map[string]any)
		if res["protocolVersion"] != c.want {
			t.Errorf("ask %q: got %v", c.ask, res["protocolVersion"])
		}
		if m["id"] != "a" {
			t.Errorf("id not echoed: %v", m["id"])
		}
		si := res["serverInfo"].(map[string]any)
		if si["name"] != "test" || si["version"] != "1.2.3" || res["instructions"] != "use me" {
			t.Errorf("serverInfo/instructions: %v", res)
		}
		if _, ok := res["capabilities"].(map[string]any)["tools"]; !ok {
			t.Errorf("no tools capability")
		}
	}
}

func TestNotificationAndPing(t *testing.T) {
	_, ep := start(t)
	resp, b := do(t, ep, "POST", `{"jsonrpc":"2.0","method":"notifications/initialized"}`, nil)
	if resp.StatusCode != 202 || len(b) != 0 {
		t.Errorf("notification: %d %q", resp.StatusCode, b)
	}
	m := rpc(t, ep, `{"jsonrpc":"2.0","id":5,"method":"ping"}`)
	if r, ok := m["result"].(map[string]any); !ok || len(r) != 0 {
		t.Errorf("ping: %v", m)
	}
	m = rpc(t, ep, `{"jsonrpc":"2.0","id":6,"method":"nope"}`)
	if m["error"].(map[string]any)["code"].(float64) != -32601 {
		t.Errorf("unknown method: %v", m)
	}
}

func TestParseErrors(t *testing.T) {
	_, ep := start(t)
	m := rpc(t, ep, `not json`)
	if m["error"].(map[string]any)["code"].(float64) != -32700 || m["id"] != nil {
		t.Errorf("%v", m)
	}
	m = rpc(t, ep, `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{}}`)
	if m["error"].(map[string]any)["code"].(float64) != -32602 {
		t.Errorf("%v", m)
	}
	m = rpc(t, ep, `{"jsonrpc":"2.0","id":1}`)
	if m["error"].(map[string]any)["code"].(float64) != -32600 {
		t.Errorf("%v", m)
	}
}

func TestToolsList(t *testing.T) {
	s, ep := start(t)
	list := func() []any {
		return rpc(t, ep, `{"jsonrpc":"2.0","id":1,"method":"tools/list"}`)["result"].(map[string]any)["tools"].([]any)
	}
	tools := list()
	if len(tools) != 5 || tools[0].(map[string]any)["name"] != "echo" {
		t.Fatalf("%v", tools)
	}
	if tools[1].(map[string]any)["inputSchema"].(map[string]any)["type"] != "object" {
		t.Errorf("default schema missing")
	}
	s.Register(mcp.Tool{Name: "late", Handler: func(context.Context, json.RawMessage) (mcp.Result, error) { return mcp.Text("x"), nil }})
	s.Register(mcp.Tool{Name: "echo", Description: "replaced"})
	tools = list()
	if len(tools) != 6 || tools[0].(map[string]any)["description"] != "replaced" {
		t.Errorf("%v", tools)
	}
}

func call(t *testing.T, ep mcp.Endpoint, name, args string) map[string]any {
	t.Helper()
	m := rpc(t, ep, fmt.Sprintf(`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":%q,"arguments":%s}}`, name, args))
	if m["error"] != nil {
		t.Fatalf("rpc error: %v", m)
	}
	return m["result"].(map[string]any)
}

func firstText(r map[string]any) string {
	return r["content"].([]any)[0].(map[string]any)["text"].(string)
}

func TestToolsCall(t *testing.T) {
	_, ep := start(t)
	r := call(t, ep, "echo", `{"x":1}`)
	if r["isError"] != false || firstText(r) != `{"args":{"x":1}}` {
		t.Errorf("echo: %v", r)
	}
	r = call(t, ep, "fail", `{}`)
	if r["isError"] != true || firstText(r) != "boom" {
		t.Errorf("fail: %v", r)
	}
	r = call(t, ep, "soft", `{}`)
	if r["isError"] != true || firstText(r) != "bad 7" {
		t.Errorf("soft: %v", r)
	}
	r = call(t, ep, "nope", `{}`)
	if r["isError"] != true || !strings.Contains(firstText(r), "unknown tool") {
		t.Errorf("unknown: %v", r)
	}
	r = call(t, ep, "panic", `{}`)
	if r["isError"] != true || !strings.Contains(firstText(r), "oops") {
		t.Errorf("panic: %v", r)
	}
	r = call(t, ep, "img", `{}`)
	c := r["content"].([]any)[0].(map[string]any)
	if c["type"] != "image" || c["data"] != "AQID" || c["mimeType"] != "image/png" || r["structuredContent"].(map[string]any)["n"] != 1.0 {
		t.Errorf("img: %v", r)
	}
	m := rpc(t, ep, `{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"echo"}}`)
	if firstText(m["result"].(map[string]any)) != `{"args":{}}` {
		t.Errorf("missing args: %v", m)
	}
}

func TestBatch(t *testing.T) {
	_, ep := start(t)
	resp, b := do(t, ep, "POST", `[{"jsonrpc":"2.0","id":1,"method":"ping"},{"jsonrpc":"2.0","method":"notifications/initialized"},{"jsonrpc":"2.0","id":"x","method":"tools/list"}]`, nil)
	var out []map[string]any
	if err := json.Unmarshal(b, &out); err != nil || resp.StatusCode != 200 || len(out) != 2 {
		t.Fatalf("%d %s", resp.StatusCode, b)
	}
	if out[0]["id"] != 1.0 || out[1]["id"] != "x" {
		t.Errorf("%v", out)
	}
	resp, b = do(t, ep, "POST", `[{"jsonrpc":"2.0","method":"notifications/initialized"}]`, nil)
	if resp.StatusCode != 202 || len(b) != 0 {
		t.Errorf("all-notification batch: %d %q", resp.StatusCode, b)
	}
	_, b = do(t, ep, "POST", `[]`, nil)
	if !bytes.Contains(b, []byte("-32600")) {
		t.Errorf("empty batch: %s", b)
	}
}

func TestConcurrentSerialized(t *testing.T) {
	s := mcp.NewServer(mcp.Options{})
	var cur, max, total int32
	s.Register(mcp.Tool{Name: "slow", Handler: func(context.Context, json.RawMessage) (mcp.Result, error) {
		n := atomic.AddInt32(&cur, 1)
		if n > atomic.LoadInt32(&max) {
			atomic.StoreInt32(&max, n)
		}
		time.Sleep(5 * time.Millisecond)
		atomic.AddInt32(&cur, -1)
		atomic.AddInt32(&total, 1)
		return mcp.Text("ok"), nil
	}})
	ep, _ := s.Start()
	defer s.Close()
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if r := call(t, ep, "slow", `{}`); r["isError"] != false {
				t.Errorf("%v", r)
			}
		}()
	}
	wg.Wait()
	if total != 20 || max != 1 {
		t.Errorf("total=%d max concurrency=%d", total, max)
	}
}

func TestConcurrentParallel(t *testing.T) {
	f := false
	s := mcp.NewServer(mcp.Options{Serialize: &f})
	var cur, max int32
	s.Register(mcp.Tool{Name: "slow", Handler: func(context.Context, json.RawMessage) (mcp.Result, error) {
		n := atomic.AddInt32(&cur, 1)
		for {
			m := atomic.LoadInt32(&max)
			if n <= m || atomic.CompareAndSwapInt32(&max, m, n) {
				break
			}
		}
		time.Sleep(50 * time.Millisecond)
		atomic.AddInt32(&cur, -1)
		return mcp.Text("ok"), nil
	}})
	ep, _ := s.Start()
	defer s.Close()
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); call(t, ep, "slow", `{}`) }()
	}
	wg.Wait()
	if atomic.LoadInt32(&max) < 2 {
		t.Errorf("expected overlap, max=%d", max)
	}
}

func TestClose(t *testing.T) {
	s := mcp.NewServer(mcp.Options{})
	started := make(chan struct{})
	cancelled := make(chan struct{})
	s.Register(mcp.Tool{Name: "hang", Handler: func(ctx context.Context, _ json.RawMessage) (mcp.Result, error) {
		close(started)
		<-ctx.Done()
		close(cancelled)
		return mcp.Text("late"), nil
	}})
	ep, _ := s.Start()
	go func() {
		req, _ := http.NewRequest("POST", ep.URL, strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"hang"}}`))
		req.Header.Set("Authorization", "Bearer "+ep.Token)
		if resp, err := http.DefaultClient.Do(req); err == nil {
			resp.Body.Close()
		}
	}()
	<-started
	t0 := time.Now()
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	if d := time.Since(t0); d > 4*time.Second {
		t.Errorf("Close took %v", d)
	}
	// Close does not wait for handlers, so the handler may still be unwinding.
	select {
	case <-cancelled:
	case <-time.After(5 * time.Second):
		t.Error("handler context not cancelled")
	}
	if err := s.Close(); err != nil {
		t.Errorf("second close: %v", err)
	}
	c := &http.Client{Timeout: time.Second}
	req, _ := http.NewRequest("POST", ep.URL, strings.NewReader(`{}`))
	if resp, err := c.Do(req); err == nil {
		resp.Body.Close()
		t.Error("server still accepting")
	}
	if _, err := s.Start(); err == nil {
		t.Error("Start after Close should fail")
	}
}

func TestCloseUnstarted(t *testing.T) {
	if err := mcp.NewServer(mcp.Options{}).Close(); err != nil {
		t.Fatal(err)
	}
}

func TestBodyLimit(t *testing.T) {
	_, ep := start(t)
	big := `{"jsonrpc":"2.0","id":1,"method":"ping","params":"` + strings.Repeat("a", 5<<20) + `"}`
	if resp, _ := do(t, ep, "POST", big, nil); resp.StatusCode != 413 {
		t.Errorf("status %d", resp.StatusCode)
	}
}

func TestCancelledCallDoesNotWaitForOrRunAfterBusyHandler(t *testing.T) {
	noticed := make(chan struct{}, 1)
	s := mcp.NewServer(mcp.Options{Logger: func(m string) {
		if strings.Contains(m, "cancelled before the handler started") {
			select {
			case noticed <- struct{}{}:
			default:
			}
		}
	}})
	release := make(chan struct{})
	started := make(chan struct{})
	var fastRan atomic.Int32
	s.Register(mcp.Tool{Name: "block", Handler: func(context.Context, json.RawMessage) (mcp.Result, error) {
		close(started)
		<-release
		return mcp.Text("ok"), nil
	}})
	s.Register(mcp.Tool{Name: "fast", Handler: func(context.Context, json.RawMessage) (mcp.Result, error) {
		fastRan.Add(1)
		return mcp.Text("ok"), nil
	}})
	ep, _ := s.Start()
	defer s.Close()
	go func() { _, _ = http.DefaultClient.Do(blockReq(ep)) }()
	<-started

	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, "POST", ep.URL, strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"fast","arguments":{}}}`))
	req.Header.Set("Authorization", "Bearer "+ep.Token)
	if resp, err := http.DefaultClient.Do(req); err == nil {
		resp.Body.Close()
		t.Fatal("expected the client to time out while the handler is busy")
	}
	// Wait until the server has seen the cancellation, then free the slot.
	select {
	case <-noticed:
	case <-time.After(5 * time.Second):
		t.Fatal("server never noticed the cancelled call")
	}
	close(release)
	if n := fastRan.Load(); n != 0 {
		t.Fatalf("abandoned call ran %d times after the busy handler finished", n)
	}
	if r := call(t, ep, "fast", `{}`); r["isError"] != false {
		t.Fatalf("server stuck: %v", r)
	}
}

func blockReq(ep mcp.Endpoint) *http.Request {
	req, _ := http.NewRequest("POST", ep.URL, strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"block","arguments":{}}}`))
	req.Header.Set("Authorization", "Bearer "+ep.Token)
	return req
}
