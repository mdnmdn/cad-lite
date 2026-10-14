package store_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"agentmanager"
	"agentmanager/store"
	"agentmanager/transcript"
)

func str(s string) *string { return &s }

func allEvents() []agentmanager.Event {
	done := agentmanager.ToolCompleted
	kind := agentmanager.KindEdit
	return []agentmanager.Event{
		agentmanager.SessionStarted{SessionID: "sess-1", Model: "m"},
		agentmanager.ModelsChanged{Current: "m", Available: []agentmanager.ModelInfo{{ID: "m", Name: "Em"}, {ID: "n"}}},
		agentmanager.ModelsChanged{Current: "n"},
		agentmanager.ThoughtDelta{MessageID: "t1", Text: "think"},
		agentmanager.TextDelta{MessageID: "a1", Text: "hello "},
		agentmanager.TextDelta{MessageID: "a1", Text: "world"},
		agentmanager.ToolCallStart{Call: agentmanager.ToolCall{ID: "c1", Title: "cad add_line", Kind: agentmanager.KindEdit,
			Status: agentmanager.ToolInProgress, RawInput: json.RawMessage(`{"x":1}`), MCPServer: "cad", MCPTool: "add_line"}},
		agentmanager.ToolCallUpdate{Update: agentmanager.ToolCallPatch{ID: "c1", Title: str("t"), Kind: &kind, Status: &done,
			Output: str("ok"), RawOutput: json.RawMessage(`{"h":"2A"}`)}},
		agentmanager.PermissionRequest{RequestID: "p1", Call: agentmanager.ToolCallPatch{ID: "c2", Title: str("run ls")},
			Options: []agentmanager.PermissionOption{{ID: "a", Name: "Allow", Kind: agentmanager.AllowOnce}, {ID: "r", Name: "No", Kind: agentmanager.RejectOnce}}},
		agentmanager.PlanUpdate{Entries: []agentmanager.PlanEntry{{Content: "a", Status: "pending"}, {Content: "b", Status: "completed"}}},
		agentmanager.Log{Level: "warn", Message: "careful"},
		agentmanager.TurnEnd{Reason: agentmanager.StopFailed, Err: "boom"},
		agentmanager.TurnEnd{Reason: agentmanager.StopEndTurn},
		agentmanager.Exited{Err: "gone"},
	}
}

func TestEventRoundTrip(t *testing.T) {
	for _, ev := range allEvents() {
		line, err := store.MarshalEvent(ev)
		if err != nil {
			t.Fatalf("%T: %v", ev, err)
		}
		got, err := store.UnmarshalEvent(line)
		if err != nil {
			t.Fatalf("%T: %v", ev, err)
		}
		if !reflect.DeepEqual(got, ev) {
			t.Errorf("%T round trip:\n got %#v\nwant %#v", ev, got, ev)
		}
	}
}

func TestNilRawMessageStaysNil(t *testing.T) {
	ev := agentmanager.ToolCallStart{Call: agentmanager.ToolCall{ID: "x"}}
	line, _ := store.MarshalEvent(ev)
	got, err := store.UnmarshalEvent(line)
	if err != nil || !reflect.DeepEqual(got, ev) {
		t.Fatalf("got %#v, %v", got, err)
	}
}

func open(t *testing.T) *store.Store {
	t.Helper()
	s, err := store.Open(filepath.Join(t.TempDir(), "agent"))
	if err != nil {
		t.Fatal(err)
	}
	return s
}

// script builds a transcript the way a host does and records the same inputs.
func fill(t *testing.T, r *store.Record, tr *transcript.T) {
	t.Helper()
	tr.AddUser("draw a line")
	if err := r.AddUser("draw a line"); err != nil {
		t.Fatal(err)
	}
	for _, ev := range allEvents() {
		tr.Apply(ev)
		if err := r.Append(ev); err != nil {
			t.Fatal(err)
		}
	}
	tr.ResolvePermission("p1")
	if err := r.ResolvePermission("p1"); err != nil {
		t.Fatal(err)
	}
}

func TestReplayRebuildsTranscript(t *testing.T) {
	s := open(t)
	r, err := s.Create(agentmanager.Claude)
	if err != nil {
		t.Fatal(err)
	}
	var want transcript.T
	fill(t, r, &want)
	if err := r.Close(); err != nil {
		t.Fatal(err)
	}
	loaded, err := s.Load(r.Meta().ID)
	if err != nil {
		t.Fatal(err)
	}
	var got transcript.T
	loaded.Replay(&got)
	if !reflect.DeepEqual(got.Blocks(), want.Blocks()) {
		t.Fatalf("blocks differ:\n got %#v\nwant %#v", got.Blocks(), want.Blocks())
	}
	if got.Running() {
		t.Fatal("replayed transcript is running")
	}
	m := loaded.Meta()
	if m.Harness != "claude" || m.AgentSessionID != "sess-1" || m.Title != "draw a line" {
		t.Fatalf("meta = %+v", m)
	}
}

func TestReplayClosesUnfinishedTurn(t *testing.T) {
	s := open(t)
	r, _ := s.Create(agentmanager.Codex)
	r.AddUser("hi")
	r.Append(agentmanager.TextDelta{Text: "par"})
	r.Close()
	l, _ := s.Load(r.Meta().ID)
	var tr transcript.T
	l.Replay(&tr)
	if tr.Running() {
		t.Fatal("still running")
	}
}

func TestTruncatedLastLine(t *testing.T) {
	s := open(t)
	r, _ := s.Create(agentmanager.Claude)
	r.AddUser("one")
	r.Append(agentmanager.TextDelta{Text: "kept"})
	r.Close()
	p := filepath.Join(s.Dir(), r.Meta().ID+".jsonl")
	f, err := os.OpenFile(p, os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	f.WriteString(`{"k":"text","t":"2026-01-01T00:00:00Z","d":{"MessageID":"","Text":"cut o`)
	f.Close()

	l, err := s.Load(r.Meta().ID)
	if err != nil {
		t.Fatal(err)
	}
	if n := len(l.Items()); n != 2 {
		t.Fatalf("items = %d, want 2", n)
	}
	if err := l.AddUser("two"); err != nil {
		t.Fatal(err)
	}
	l.Close()
	l2, _ := s.Load(r.Meta().ID)
	items := l2.Items()
	if len(items) != 3 || items[2] != (store.User{Text: "two"}) {
		t.Fatalf("items after append = %#v", items)
	}
	ms, _ := s.List()
	if len(ms) != 1 || ms[0].Title != "one" {
		t.Fatalf("list = %+v", ms)
	}
}

func TestListOrderTitleAndLatest(t *testing.T) {
	s := open(t)
	if _, err := s.Latest(); err != store.ErrNotFound {
		t.Fatalf("empty Latest err = %v", err)
	}
	if ms, err := s.List(); err != nil || len(ms) != 0 {
		t.Fatalf("empty list = %v, %v", ms, err)
	}
	var ids []string
	for i, h := range []agentmanager.Harness{agentmanager.Claude, agentmanager.Codex, agentmanager.OpenCode} {
		r, err := s.Create(h)
		if err != nil {
			t.Fatal(err)
		}
		r.AddUser(strings.Repeat("word ", 40) + string(h))
		r.Close()
		ids = append(ids, r.Meta().ID)
		at := time.Now().Add(time.Duration(i) * time.Hour)
		os.Chtimes(filepath.Join(s.Dir(), r.Meta().ID+".jsonl"), at, at)
	}
	// Touch the first one last: it becomes the newest.
	at := time.Now().Add(10 * time.Hour)
	os.Chtimes(filepath.Join(s.Dir(), ids[0]+".jsonl"), at, at)
	ms, err := s.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(ms) != 3 || ms[0].ID != ids[0] || ms[1].ID != ids[2] || ms[2].ID != ids[1] {
		t.Fatalf("order = %v, ids %v", ms, ids)
	}
	if n := len([]rune(ms[0].Title)); n > 80 || !strings.HasSuffix(ms[0].Title, "...") {
		t.Fatalf("title %q", ms[0].Title)
	}
	l, err := s.Latest()
	if err != nil || l.Meta().ID != ids[0] {
		t.Fatalf("latest = %v, %v", l, err)
	}
	if err := s.Delete(ids[0]); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Load(ids[0]); err != store.ErrNotFound {
		t.Fatalf("load deleted: %v", err)
	}
	if _, err := s.Load("../x"); err != store.ErrNotFound {
		t.Fatalf("path id: %v", err)
	}
}

func TestPermissions(t *testing.T) {
	s := open(t)
	r, _ := s.Create(agentmanager.Claude)
	r.AddUser("x")
	r.Close()
	di, _ := os.Stat(s.Dir())
	fi, _ := os.Stat(filepath.Join(s.Dir(), r.Meta().ID+".jsonl"))
	if di.Mode().Perm() != 0o700 || fi.Mode().Perm() != 0o600 {
		t.Fatalf("modes %v %v", di.Mode().Perm(), fi.Mode().Perm())
	}
}

func TestSetAgentSessionID(t *testing.T) {
	s := open(t)
	r, _ := s.Create(agentmanager.Codex)
	if err := r.SetAgentSessionID("thread-9"); err != nil {
		t.Fatal(err)
	}
	r.Close()
	l, _ := s.Load(r.Meta().ID)
	if l.Meta().AgentSessionID != "thread-9" {
		t.Fatalf("meta = %+v", l.Meta())
	}
	ms, _ := s.List()
	if ms[0].AgentSessionID != "thread-9" {
		t.Fatalf("list = %+v", ms)
	}
}

func TestConcurrentAppend(t *testing.T) {
	s := open(t)
	r, _ := s.Create(agentmanager.Claude)
	var wg sync.WaitGroup
	for g := 0; g < 8; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 50; i++ {
				r.Append(agentmanager.TextDelta{Text: "x"})
			}
		}()
	}
	wg.Wait()
	r.Close()
	l, err := s.Load(r.Meta().ID)
	if err != nil {
		t.Fatal(err)
	}
	if n := len(l.Items()); n != 400 {
		t.Fatalf("items = %d, want 400", n)
	}
}

func TestOpenDoesNotTouchDisk(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "nope", "agent")
	if _, err := store.Open(dir); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Dir(dir)); !os.IsNotExist(err) {
		t.Fatalf("directory created: %v", err)
	}
}

func TestOpenSet(t *testing.T) {
	s := open(t)
	if _, err := s.LoadOpen(); err != store.ErrNotFound {
		t.Fatalf("empty: %v", err)
	}
	a, _ := s.Create(agentmanager.Claude)
	b, _ := s.Create(agentmanager.Codex)
	a.Close()
	b.Close()
	want := store.OpenSet{IDs: []string{a.Meta().ID, b.Meta().ID}, Active: b.Meta().ID}
	if err := s.SaveOpen(want); err != nil {
		t.Fatal(err)
	}
	got, err := s.LoadOpen()
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v, %v", got, err)
	}
	fi, _ := os.Stat(filepath.Join(s.Dir(), "open.json"))
	if fi.Mode().Perm() != 0o600 {
		t.Fatalf("mode %v", fi.Mode().Perm())
	}
	if ms, _ := s.List(); len(ms) != 2 {
		t.Fatalf("open.json listed as a session: %v", ms)
	}
	s.Delete(b.Meta().ID)
	got, _ = s.LoadOpen()
	if len(got.IDs) != 1 || got.Active != "" {
		t.Fatalf("after delete %+v", got)
	}
	if err := s.SaveOpen(store.OpenSet{}); err != nil {
		t.Fatal(err)
	}
	got, err = s.LoadOpen()
	if err != nil || len(got.IDs) != 0 {
		t.Fatalf("empty set: %+v %v", got, err)
	}
}
