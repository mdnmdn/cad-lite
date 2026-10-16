package harnessspike

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"agentmanager"
	"agentmanager/mcp"
)

func TestProfiles(t *testing.T) {
	c := Default()
	c.Ollama.APIKey, c.OpenCode.APIKey = "ollama-secret", "go-secret"
	path := filepath.Join(t.TempDir(), "harness.json")
	for _, provider := range []string{"ollama", "opencode-go"} {
		c.Provider = provider
		if err := Save(path, c); err != nil {
			t.Fatal(err)
		}
		loaded, err := Load(path)
		if err != nil || loaded != c {
			t.Fatalf("round trip: %v", err)
		}
		p, url, err := loaded.Selected()
		if err != nil || p.APIKey == "" || !strings.HasPrefix(url, "https://") {
			t.Fatal("invalid selected profile")
		}
	}
	info, _ := os.Stat(path)
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("credential permissions: %v", info.Mode())
	}
	c.Provider = "unknown"
	if _, _, err := c.Selected(); err == nil {
		t.Fatal("unknown provider accepted")
	}
	c = Default()
	if _, _, err := validate(c); err == nil {
		t.Fatal("missing key accepted")
	}
}

func drainTurn(t *testing.T, s agentmanager.Session) []agentmanager.Event {
	t.Helper()
	var events []agentmanager.Event
	timer := time.NewTimer(10 * time.Second)
	defer timer.Stop()
	for {
		select {
		case e, ok := <-s.Events():
			if !ok {
				t.Fatal("session closed before turn ended")
			}
			events = append(events, e)
			if _, ok := e.(agentmanager.TurnEnd); ok {
				return events
			}
		case <-timer.C:
			t.Fatal("turn did not end")
		}
	}
}

func TestStreamingToolAndResume(t *testing.T) {
	var calls atomic.Int32
	srv := mcp.NewServer(mcp.Options{Name: "cad", Version: "test"})
	srv.Register(mcp.Tool{Name: "test_draw", Description: "Draw a test line", InputSchema: json.RawMessage(`{"type":"object","properties":{}}`), Handler: func(ctx context.Context, args json.RawMessage) (mcp.Result, error) {
		calls.Add(1)
		return mcp.Text("line drawn"), nil
	}})
	ep, err := srv.Start()
	if err != nil {
		t.Fatal(err)
	}
	defer srv.Close()
	var requests atomic.Int32
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" || r.Header.Get("Authorization") != "Bearer test-secret" {
			t.Errorf("bad API route/auth")
		}
		var body struct {
			Model    string           `json:"model"`
			Messages []map[string]any `json:"messages"`
			Tools    []map[string]any `json:"tools"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		if body.Model != "test-model" {
			t.Errorf("model = %s", body.Model)
		}
		for _, d := range body.Tools {
			fn, _ := d["function"].(map[string]any)
			if fn["name"] != "mcp__cad__test_draw" {
				t.Errorf("unexpected tool: %v", fn["name"])
			}
		}
		w.Header().Set("Content-Type", "text/event-stream")
		if requests.Add(1) == 1 {
			fmt.Fprint(w, "data: {\"id\":\"first\",\"choices\":[{\"index\":0,\"delta\":{\"tool_calls\":[{\"index\":0,\"id\":\"call1\",\"type\":\"function\",\"function\":{\"name\":\"mcp__cad__test_draw\",\"arguments\":\"{}\"}}]},\"finish_reason\":null}]}\n\ndata: {\"id\":\"first\",\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":\"tool_calls\"}]}\n\ndata: [DONE]\n\n")
		} else {
			if len(body.Messages) < 4 {
				t.Errorf("tool history lost: %d messages", len(body.Messages))
			}
			fmt.Fprint(w, "data: {\"id\":\"reply\",\"choices\":[{\"index\":0,\"delta\":{\"content\":\"Done drawing\"},\"finish_reason\":null}]}\n\ndata: {\"id\":\"reply\",\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n")
		}
	}))
	defer api.Close()
	cfg := agentmanager.Config{Cwd: t.TempDir(), MCPServers: []agentmanager.MCPServer{{Name: "cad", URL: ep.URL, Token: ep.Token}}}
	p := Profile{APIKey: "test-secret", Model: "test-model"}
	s, err := open(context.Background(), cfg, "ollama", p, api.URL+"/v1")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if err := s.Prompt(context.Background(), "draw a line"); err != nil {
		t.Fatal(err)
	}
	events := drainTurn(t, s)
	var text, started, completed bool
	for _, e := range events {
		switch e := e.(type) {
		case agentmanager.TextDelta:
			text = text || e.Text == "Done drawing"
		case agentmanager.ToolCallStart:
			started = e.Call.ID == "call1"
		case agentmanager.ToolCallUpdate:
			completed = e.Update.Status != nil && *e.Update.Status == agentmanager.ToolCompleted
		case agentmanager.TurnEnd:
			if e.Reason != agentmanager.StopEndTurn {
				t.Errorf("turn failed: %+v", e)
			}
		}
	}
	if !text || !started || !completed || calls.Load() != 1 {
		t.Fatalf("text %v, start %v, completed %v, calls %d", text, started, completed, calls.Load())
	}
	cfg.ResumeID = s.ID()
	s.Close()
	resumed, err := open(context.Background(), cfg, "ollama", p, api.URL+"/v1")
	if err != nil {
		t.Fatal(err)
	}
	defer resumed.Close()
	if err := resumed.Prompt(context.Background(), "what did you draw?"); err != nil {
		t.Fatal(err)
	}
	drainTurn(t, resumed)
	if resumed.ID() != cfg.ResumeID {
		t.Fatal("resume key changed")
	}
}

func TestCancelAndClose(t *testing.T) {
	entered := make(chan struct{}, 1)
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: {\"id\":\"wait\",\"choices\":[{\"index\":0,\"delta\":{\"content\":\"waiting\"}}]}\n\n")
		w.(http.Flusher).Flush()
		entered <- struct{}{}
		<-r.Context().Done()
	}))
	defer api.Close()
	s, err := open(context.Background(), agentmanager.Config{Cwd: t.TempDir()}, "ollama", Profile{APIKey: "secret", Model: "test"}, api.URL+"/v1")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if err := s.Prompt(context.Background(), "wait"); err != nil {
		t.Fatal(err)
	}
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("no request")
	}
	if err := s.Prompt(context.Background(), "overlap"); err == nil {
		t.Fatal("overlap accepted")
	}
	s.Cancel()
	events := drainTurn(t, s)
	if end := events[len(events)-1].(agentmanager.TurnEnd); end.Reason != agentmanager.StopCancelled {
		t.Fatalf("cancel: %+v", end)
	}
	s.Close()
	if err := s.Prompt(context.Background(), "closed"); err != agentmanager.ErrClosed {
		t.Fatalf("closed prompt: %v", err)
	}
}
