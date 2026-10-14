package transcript_test

import (
	"encoding/json"
	"reflect"
	"testing"

	am "agentmanager"
	"agentmanager/transcript"
)

func sp[T any](v T) *T { return &v }

func TestUserAndRunning(t *testing.T) {
	var tr transcript.T
	if tr.Running() || tr.Version() != 0 || len(tr.Blocks()) != 0 {
		t.Fatal("zero value not empty")
	}
	tr.AddUser("hi")
	if !tr.Running() || tr.Version() == 0 {
		t.Fatal("AddUser should start a turn")
	}
	if !reflect.DeepEqual(tr.Blocks(), []transcript.Block{transcript.UserMsg{Text: "hi"}}) {
		t.Fatalf("%v", tr.Blocks())
	}
	tr.Apply(am.TurnEnd{Reason: am.StopEndTurn})
	if tr.Running() || len(tr.Blocks()) != 1 {
		t.Fatalf("end_turn: running=%v blocks=%v", tr.Running(), tr.Blocks())
	}
}

func TestDeltaMerging(t *testing.T) {
	var tr transcript.T
	tr.Apply(am.TextDelta{MessageID: "m1", Text: "Hel"})
	tr.Apply(am.TextDelta{MessageID: "m1", Text: "lo"})
	tr.Apply(am.TextDelta{Text: " there"})
	tr.Apply(am.ThoughtDelta{MessageID: "t1", Text: "hmm"})
	tr.Apply(am.ThoughtDelta{MessageID: "t1", Text: "!"})
	tr.Apply(am.TextDelta{MessageID: "m2", Text: "second"})
	tr.Apply(am.TextDelta{MessageID: "m1", Text: "?"})
	want := []transcript.Block{
		transcript.AgentMsg{ID: "m1", Text: "Hello there?"},
		transcript.Thought{ID: "t1", Text: "hmm!"},
		transcript.AgentMsg{ID: "m2", Text: "second"},
	}
	if !reflect.DeepEqual(tr.Blocks(), want) {
		t.Fatalf("got %#v", tr.Blocks())
	}
}

func TestEmptyIDStartsNewAfterOtherBlock(t *testing.T) {
	var tr transcript.T
	tr.Apply(am.TextDelta{Text: "a"})
	tr.Apply(am.ToolCallStart{Call: am.ToolCall{ID: "c"}})
	tr.Apply(am.TextDelta{Text: "b"})
	tr.Apply(am.ThoughtDelta{Text: "t"})
	tr.Apply(am.ThoughtDelta{Text: "u"})
	bl := tr.Blocks()
	if len(bl) != 4 || bl[0] != (transcript.AgentMsg{Text: "a"}) || bl[2] != (transcript.AgentMsg{Text: "b"}) || bl[3] != (transcript.Thought{Text: "tu"}) {
		t.Fatalf("%#v", bl)
	}
}

func TestEmptyDeltaNoChange(t *testing.T) {
	var tr transcript.T
	tr.Apply(am.TextDelta{MessageID: "m", Text: ""})
	if tr.Version() != 0 || len(tr.Blocks()) != 0 {
		t.Fatal("empty delta changed transcript")
	}
}

func TestToolDedupeAndStatus(t *testing.T) {
	var tr transcript.T
	tr.Apply(am.ToolCallStart{Call: am.ToolCall{ID: "c1", Title: "Read a.go", Kind: am.KindRead, RawInput: json.RawMessage(`{"p":1}`), MCPServer: "cad", MCPTool: "x"}})
	tr.Apply(am.ToolCallStart{Call: am.ToolCall{ID: "c1", Status: am.ToolInProgress}})
	if len(tr.Blocks()) != 1 {
		t.Fatalf("duplicate start made %d blocks", len(tr.Blocks()))
	}
	b := tr.Blocks()[0].(transcript.ToolBlock)
	if b.Call.Title != "Read a.go" || b.Call.Kind != am.KindRead || b.Status != am.ToolInProgress || b.Call.MCPServer != "cad" || string(b.Call.RawInput) != `{"p":1}` {
		t.Fatalf("start merge lost fields: %#v", b)
	}
	tr.Apply(am.ToolCallUpdate{Update: am.ToolCallPatch{ID: "c1", Output: sp("partial")}})
	tr.Apply(am.ToolCallUpdate{Update: am.ToolCallPatch{ID: "c1", Status: sp(am.ToolCompleted), Output: sp("done"), Title: sp("Read b.go")}})
	b = tr.Blocks()[0].(transcript.ToolBlock)
	if b.Status != am.ToolCompleted || b.Call.Status != am.ToolCompleted || b.Output != "done" || b.Call.Title != "Read b.go" {
		t.Fatalf("%#v", b)
	}
	v := tr.Version()
	tr.Apply(am.ToolCallUpdate{Update: am.ToolCallPatch{ID: "c1", Status: sp(am.ToolCompleted), Output: sp("done")}})
	if tr.Version() != v {
		t.Fatal("identical update bumped version")
	}
	tr.Apply(am.ToolCallUpdate{Update: am.ToolCallPatch{ID: "c1"}})
	if tr.Version() != v {
		t.Fatal("empty patch bumped version")
	}
}

func TestToolUpdateBeforeStartCreatesBlock(t *testing.T) {
	var tr transcript.T
	tr.Apply(am.ToolCallUpdate{Update: am.ToolCallPatch{ID: "z", Status: sp(am.ToolFailed), Output: sp("err")}})
	b, ok := tr.Blocks()[0].(transcript.ToolBlock)
	if !ok || b.Call.ID != "z" || b.Status != am.ToolFailed || b.Output != "err" {
		t.Fatalf("%#v", tr.Blocks())
	}
	tr.Apply(am.ToolCallStart{Call: am.ToolCall{ID: "z", Title: "T", Status: am.ToolPending}})
	if len(tr.Blocks()) != 1 {
		t.Fatal("late start duplicated")
	}
}

func TestPermissionFlow(t *testing.T) {
	var tr transcript.T
	req := am.PermissionRequest{RequestID: "r1", Call: am.ToolCallPatch{ID: "c1", Title: sp("Run ls")},
		Options: []am.PermissionOption{{ID: "a", Name: "Allow", Kind: am.AllowOnce}, {ID: "r", Name: "Reject", Kind: am.RejectOnce}}}
	tr.Apply(req)
	tr.Apply(req)
	if len(tr.Blocks()) != 1 {
		t.Fatalf("duplicate request made %d blocks", len(tr.Blocks()))
	}
	p := tr.Blocks()[0].(transcript.PermissionBlock)
	if p.Answered || len(p.Options) != 2 || *p.Call.Title != "Run ls" {
		t.Fatalf("%#v", p)
	}
	v := tr.Version()
	tr.ResolvePermission("nope")
	if tr.Version() != v {
		t.Fatal("unknown id bumped version")
	}
	tr.ResolvePermission("r1")
	if !tr.Blocks()[0].(transcript.PermissionBlock).Answered || tr.Version() == v {
		t.Fatal("not resolved")
	}
	v = tr.Version()
	tr.ResolvePermission("r1")
	if tr.Version() != v {
		t.Fatal("second resolve bumped version")
	}
}

func TestTurnEndClosesPermissions(t *testing.T) {
	var tr transcript.T
	tr.AddUser("x")
	tr.Apply(am.PermissionRequest{RequestID: "r"})
	tr.Apply(am.TurnEnd{Reason: am.StopCancelled})
	bl := tr.Blocks()
	if !bl[1].(transcript.PermissionBlock).Answered || tr.Running() {
		t.Fatalf("%#v", bl)
	}
	if n := bl[2].(transcript.Note); n.Level != "info" {
		t.Fatalf("%#v", n)
	}
}

func TestPlanReplacedInPlace(t *testing.T) {
	var tr transcript.T
	entries := []am.PlanEntry{{Content: "a", Status: "pending"}}
	tr.Apply(am.PlanUpdate{Entries: entries})
	tr.Apply(am.TextDelta{Text: "x"})
	entries[0].Status = "mutated"
	tr.Apply(am.PlanUpdate{Entries: []am.PlanEntry{{Content: "a", Status: "completed"}, {Content: "b", Status: "in_progress"}}})
	bl := tr.Blocks()
	if len(bl) != 2 {
		t.Fatalf("%#v", bl)
	}
	p := bl[0].(transcript.Plan)
	if len(p.Entries) != 2 || p.Entries[0].Status != "completed" {
		t.Fatalf("%#v", p)
	}
}

func TestNotesAndFailure(t *testing.T) {
	var tr transcript.T
	tr.AddUser("go")
	tr.Apply(am.Log{Level: "warn", Message: "careful"})
	tr.Apply(am.TurnEnd{Reason: am.StopFailed, Err: "rate limited"})
	if tr.Running() {
		t.Fatal("still running")
	}
	bl := tr.Blocks()
	if bl[1] != (transcript.Note{Level: "warn", Text: "careful"}) || bl[2] != (transcript.Note{Level: "error", Text: "rate limited"}) {
		t.Fatalf("%#v", bl)
	}
	tr.AddUser("again")
	if !tr.Running() {
		t.Fatal("new turn not running")
	}
	tr.Apply(am.Exited{Err: "crash\nstderr"})
	if tr.Running() {
		t.Fatal("exit left running")
	}
	last := tr.Blocks()[len(tr.Blocks())-1].(transcript.Note)
	if last.Level != "error" || last.Text != "agent exited: crash\nstderr" {
		t.Fatalf("%#v", last)
	}
}

func TestSessionStartedNoChange(t *testing.T) {
	var tr transcript.T
	tr.Apply(am.SessionStarted{SessionID: "s"})
	if tr.Version() != 0 || len(tr.Blocks()) != 0 {
		t.Fatal("SessionStarted should not change transcript")
	}
}

func TestFullTurn(t *testing.T) {
	var tr transcript.T
	tr.AddUser("draw a circle")
	for _, ev := range []am.Event{
		am.SessionStarted{SessionID: "s1"},
		am.ThoughtDelta{Text: "think"},
		am.TextDelta{MessageID: "m", Text: "Sure."},
		am.ToolCallStart{Call: am.ToolCall{ID: "t", Title: "cad draw", Status: am.ToolPending}},
		am.PermissionRequest{RequestID: "p", Call: am.ToolCallPatch{ID: "t"}},
		am.ToolCallUpdate{Update: am.ToolCallPatch{ID: "t", Status: sp(am.ToolInProgress)}},
		am.ToolCallUpdate{Update: am.ToolCallPatch{ID: "t", Status: sp(am.ToolCompleted), Output: sp("ok")}},
		am.TextDelta{MessageID: "m2", Text: "Done."},
		am.TurnEnd{Reason: am.StopEndTurn},
	} {
		tr.Apply(ev)
	}
	kinds := ""
	for _, b := range tr.Blocks() {
		switch b.(type) {
		case transcript.UserMsg:
			kinds += "U"
		case transcript.Thought:
			kinds += "H"
		case transcript.AgentMsg:
			kinds += "A"
		case transcript.ToolBlock:
			kinds += "T"
		case transcript.PermissionBlock:
			kinds += "P"
		}
	}
	if kinds != "UHATPA" || tr.Running() {
		t.Fatalf("kinds=%s running=%v", kinds, tr.Running())
	}
}

func TestRevisionTracksChangedBlocks(t *testing.T) {
	var tr transcript.T
	tr.AddUser("hi")
	tr.Apply(am.ToolCallStart{Call: am.ToolCall{ID: "t1", Title: "ls"}})
	tr.Apply(am.TextDelta{MessageID: "m", Text: "a"})
	if r := tr.Revision(0); r != 1 {
		t.Fatalf("user block revision = %d, want 1", r)
	}
	tool, msg := tr.Revision(1), tr.Revision(2)
	if tool == 0 || msg <= tool || msg != tr.Version() {
		t.Fatalf("revisions tool=%d msg=%d version=%d", tool, msg, tr.Version())
	}
	// Patching the tool card in place moves only its revision.
	tr.Apply(am.ToolCallUpdate{Update: am.ToolCallPatch{ID: "t1", Status: sp(am.ToolCompleted)}})
	if tr.Revision(1) != tr.Version() || tr.Revision(2) != msg || tr.Revision(0) != 1 {
		t.Fatalf("after tool update revisions = %d %d %d, version %d", tr.Revision(0), tr.Revision(1), tr.Revision(2), tr.Version())
	}
	// Streaming extends the message block.
	tr.Apply(am.TextDelta{MessageID: "m", Text: "b"})
	if tr.Revision(2) != tr.Version() {
		t.Fatalf("message revision = %d, version %d", tr.Revision(2), tr.Version())
	}
	if tr.Revision(-1) != 0 || tr.Revision(3) != 0 {
		t.Fatal("out-of-range revision not 0")
	}
}
