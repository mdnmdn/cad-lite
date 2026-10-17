package ui

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"agentmanager"
	"cad-lite/settings"
)

func TestFetchOllamaModels(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		status     int
		fail       bool
	}{
		{"models", `{"data":[{"id":"qwen3-coder"},{"id":"gpt-oss:120b"},{"id":"qwen3-coder"},{"id":""}]}`, 200, false},
		{"unauthorized", `secret-key`, 401, true},
		{"invalid", `secret-key`, 200, true},
		{"empty", `{"data":[]}`, 200, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Header.Get("Authorization") != "Bearer secret-key" {
					t.Error("missing saved key")
				}
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(tc.body))
			}))
			defer server.Close()
			models, err := fetchOllamaModels(context.Background(), server.Client(), server.URL, "secret-key")
			if (err != nil) != tc.fail {
				t.Fatalf("error = %v", err)
			}
			if err != nil && strings.Contains(err.Error(), "secret-key") {
				t.Fatal("response body leaked")
			}
			if !tc.fail {
				if len(models) != 2 || models[0].ID != "ollama/qwen3-coder" {
					t.Fatalf("models: %v", models)
				}
				c := settings.Default()
				c.OllamaKey = "secret-key"
				env := basicOpenCodeEnv(c, models...)
				var config struct {
					Model    string
					Provider map[string]struct{ Models map[string]any }
				}
				for _, v := range env {
					if s, ok := strings.CutPrefix(v, "OPENCODE_CONFIG_CONTENT="); ok {
						if err := json.Unmarshal([]byte(s), &config); err != nil {
							t.Fatal(err)
						}
					}
				}
				if len(config.Provider["ollama"].Models) != 2 {
					t.Fatal("live model catalog not passed to OpenCode")
				}
			}
		})
	}
}

type basicCheckSession struct {
	events         chan agentmanager.Event
	closed, denied bool
}

func (s *basicCheckSession) ID() string                           { return "test" }
func (s *basicCheckSession) Events() <-chan agentmanager.Event    { return s.events }
func (s *basicCheckSession) Prompt(context.Context, string) error { return nil }
func (s *basicCheckSession) Cancel() error                        { return nil }
func (s *basicCheckSession) Close() error                         { s.closed = true; return nil }
func (s *basicCheckSession) RespondPermission(_ string, a agentmanager.PermissionAnswer) error {
	s.denied = a.Cancelled
	return nil
}

func TestBasicConnectionRunsEveryConfiguredProviderWithSavedKeys(t *testing.T) {
	c := settings.Default()
	c.OpenCodeKey = "opencode-secret"
	c.OllamaKey = "ollama-secret"
	var sessions []*basicCheckSession
	var dirs []string
	open := func(ctx context.Context, cfg agentmanager.Config) (agentmanager.Session, error) {
		if cfg.Binary != "/test/opencode" || cfg.Harness != agentmanager.OpenCode {
			t.Fatal("wrong executable")
		}
		if _, err := os.Stat(cfg.Cwd); err != nil {
			t.Fatal("missing isolated directory")
		}
		dirs = append(dirs, cfg.Cwd)
		if !strings.Contains(strings.Join(cfg.Env, "\n"), "OLLAMA_KEY=ollama-secret") || !strings.Contains(strings.Join(cfg.Env, "\n"), "OPENCODE_KEY=opencode-secret") {
			t.Fatal("keys not passed")
		}
		if len(sessions) == 0 && cfg.Model != "opencode/gpt-5-nano" {
			t.Fatal("key test must use authenticated model")
		}
		if len(sessions) == 1 && cfg.Model != "ollama/qwen3-coder" {
			t.Fatal("live Ollama model missing")
		}
		if len(cfg.MCPServers) != 0 {
			t.Fatal("connection test exposes drawing tools")
		}
		s := &basicCheckSession{events: make(chan agentmanager.Event, 4)}
		s.events <- agentmanager.PermissionRequest{RequestID: "deny"}
		s.events <- agentmanager.TextDelta{Text: "OK"}
		s.events <- agentmanager.TurnEnd{Reason: agentmanager.StopEndTurn}
		sessions = append(sessions, s)
		return s, nil
	}
	if err := checkBasicAI(context.Background(), "/test/opencode", c, []agentmanager.ModelInfo{{ID: "ollama/qwen3-coder"}}, open); err != nil {
		t.Fatal(err)
	}
	if len(sessions) != 2 {
		t.Fatal("not all keys tested")
	}
	for _, s := range sessions {
		if !s.closed || !s.denied {
			t.Fatal("session leaked or permission allowed")
		}
	}
	for _, dir := range dirs {
		if _, err := os.Stat(dir); !os.IsNotExist(err) {
			t.Fatal("test directory leaked")
		}
	}
}

func TestBasicConnectionRedactsFailuresAndHandlesCancellation(t *testing.T) {
	c := settings.Default()
	c.OllamaKey = "private-key"
	open := func(context.Context, agentmanager.Config) (agentmanager.Session, error) {
		return nil, errors.New("private-key invalid")
	}
	err := checkBasicAI(context.Background(), "test", c, nil, open)
	if err == nil || strings.Contains(err.Error(), "private-key") {
		t.Fatalf("unsafe failure: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Millisecond)
	defer cancel()
	s := &basicCheckSession{events: make(chan agentmanager.Event)}
	err = checkBasicSession(ctx, agentmanager.Config{}, func(context.Context, agentmanager.Config) (agentmanager.Session, error) { return s, nil })
	if !errors.Is(err, context.DeadlineExceeded) || !s.closed {
		t.Fatal("test did not stop on deadline")
	}
}

func TestOllamaRefreshRejectsStaleCredentials(t *testing.T) {
	h := openSettingsApp(t)
	a := &h.ws.prefs().aiBasic
	h.ws.prefs().cur.OllamaKey = "new-key"
	a.modelKey = "old-key"
	a.modelResult = make(chan basicModelsResult, 1)
	a.modelResult <- basicModelsResult{key: "old-key", models: []agentmanager.ModelInfo{{ID: "ollama/stale"}}}
	h.ws.pollBasicAI(h.hw.Window())
	if len(a.models) != 0 {
		t.Fatal("stale result accepted")
	}
}

func TestBasicConnectionDiscardsResultForRemovedKey(t *testing.T) {
	h := openSettingsApp(t)
	a := &h.ws.prefs().aiBasic
	a.checkResult = make(chan basicCheckResult, 1)
	a.checkResult <- basicCheckResult{ollamaKey: "removed-key"}
	h.ws.pollBasicAI(h.hw.Window())
	if a.checkMessage != "" || a.checkResult != nil {
		t.Fatal("stale connection success shown")
	}
}

func TestCloseAgentCancelsBasicChecks(t *testing.T) {
	h := openSettingsApp(t)
	a := &h.ws.prefs().aiBasic
	modelsCancelled, checkCancelled := false, false
	a.modelCancel = func() { modelsCancelled = true }
	a.checkCancel = func() { checkCancelled = true }
	a.checkResult = make(chan basicCheckResult, 1)
	a.checkResult <- basicCheckResult{}
	h.ws.CloseAgent()
	if !modelsCancelled || !checkCancelled {
		t.Fatal("Basic checks survived shutdown")
	}
}
