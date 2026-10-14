package ui_test

import (
	"context"
	"testing"
	"time"

	"agentmanager"
)

// slowClose is a session whose Close takes a while, like an agent process shutting down.
type slowClose struct{ *fakeSession }

func (s slowClose) Close() error {
	time.Sleep(150 * time.Millisecond)
	return s.fakeSession.Close()
}

func TestQuitDuringStartClosesTheLateSession(t *testing.T) {
	r := openAgent(t)
	late := slowClose{newFakeSession()}
	entered := make(chan struct{})
	r.ws.AgentPanel().SetOpener(func(ctx context.Context, cfg agentmanager.Config) (agentmanager.Session, error) {
		close(entered)
		<-ctx.Done() // the start completes after quit cancelled it
		return late, nil
	})
	r.click(r.pos("agent-start"))
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("opener never ran")
	}
	r.ws.CloseAgent()
	if !late.isClosed() {
		t.Fatal("CloseAgent returned while the session from the in-flight start was still open (orphaned agent)")
	}
}
