package agentmanager_test

import (
	"fmt"
	"testing"
	"time"

	"agentmanager"
)

func TestQueueOrderingUnbounded(t *testing.T) {
	q := agentmanager.NewQueue()
	const n = 10000
	for i := 0; i < n; i++ {
		q.Push(agentmanager.TextDelta{Text: fmt.Sprint(i)})
	}
	q.Close()
	i := 0
	for ev := range q.Events() {
		if got := ev.(agentmanager.TextDelta).Text; got != fmt.Sprint(i) {
			t.Fatalf("event %d = %s", i, got)
		}
		i++
	}
	if i != n {
		t.Fatalf("got %d events", i)
	}
}

func TestQueueCloseIdempotentAndDropsLate(t *testing.T) {
	q := agentmanager.NewQueue()
	q.Push(agentmanager.Log{Message: "a"})
	q.Close()
	q.Close()
	q.Push(agentmanager.Log{Message: "late"})
	var got []string
	for ev := range q.Events() {
		got = append(got, ev.(agentmanager.Log).Message)
	}
	if len(got) != 1 || got[0] != "a" {
		t.Fatalf("got %v", got)
	}
}

func TestQueueConcurrentConsumer(t *testing.T) {
	q := agentmanager.NewQueue()
	done := make(chan int)
	go func() {
		n := 0
		for range q.Events() {
			n++
		}
		done <- n
	}()
	for i := 0; i < 100; i++ {
		q.Push(agentmanager.Exited{})
	}
	q.Close()
	select {
	case n := <-done:
		if n != 100 {
			t.Fatalf("n=%d", n)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timeout")
	}
}

func TestAllowMCP(t *testing.T) {
	p := agentmanager.AllowMCP("cad")
	opts := []agentmanager.PermissionOption{
		{ID: "always", Kind: agentmanager.AllowAlways},
		{ID: "once", Kind: agentmanager.AllowOnce},
		{ID: "no", Kind: agentmanager.RejectOnce},
	}
	req := func(title string) agentmanager.PermissionRequest {
		return agentmanager.PermissionRequest{Call: agentmanager.ToolCallPatch{Title: &title}, Options: opts}
	}
	for _, title := range []string{"mcp__cad__add_line", "cad_add_line", "cad.add_line"} {
		if id, ok := p(req(title)); !ok || id != "once" {
			t.Errorf("%s: %q %v", title, id, ok)
		}
	}
	for _, title := range []string{"Bash", "mcp__other__x", "cadence_x"} {
		if _, ok := p(req(title)); ok {
			t.Errorf("%s allowed", title)
		}
	}
	if _, ok := p(agentmanager.PermissionRequest{Options: opts}); ok {
		t.Error("nil title allowed")
	}
}
