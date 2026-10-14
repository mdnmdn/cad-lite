package ui_test

import (
	"os"
	"path/filepath"
	"testing"

	"agentmanager"
	"agentmanager/store"
)

func (r *agentRig) open(path string) {
	r.t.Helper()
	s := r.ws.Session()
	s.Run("OPEN")
	s.Submit(path)
}

func (r *agentRig) saveAs(path string) {
	r.t.Helper()
	s := r.ws.Session()
	s.Run("SAVEAS")
	s.Submit(path)
}

// chat sends text on the active tab as session i and plays one finished turn answering reply.
func (r *agentRig) chat(i int, text, sid, reply string) {
	r.t.Helper()
	r.say(text, i)
	r.nextCfg()
	r.turn(r.sess(i), sid, reply)
	r.until(reply, func() bool { return r.hasText(0, reply) && !r.ws.AgentPanel().Transcript().Running() })
}

func TestEachDrawingTabShowsItsOwnConversations(t *testing.T) {
	dir := t.TempDir()
	r := newRig(t, dir, memFiles{}, true)
	p := r.ws.AgentPanel()
	r.chat(0, "about the plate", "SP", "plate answer")

	// bracket.dxf opens in a tab of its own; plate's session keeps running in its tab.
	r.open(filepath.Join(dir, "bracket.dxf"))
	if len(r.ws.Documents()) != 2 {
		t.Fatalf("bracket did not open in a tab: %+v", r.ws.Documents())
	}
	if tabs := p.Tabs(); len(tabs) != 1 || tabs[0].SessionID != "" || len(p.Transcript().Blocks()) != 0 {
		t.Fatalf("bracket starts with %+v", tabs)
	}
	if live, _ := p.Live(); !live || r.fs.isClosed() {
		t.Fatal("opening another drawing stopped plate's session")
	}
	r.chat(1, "about the bracket", "SB", "bracket answer")

	// OPEN of the open plate.dxf shows its tab, with its conversation still live.
	r.open(filepath.Join(dir, "plate.dxf"))
	if !r.ws.Documents()[0].Active || !r.hasText(0, "plate answer") || r.hasText(0, "bracket answer") || len(p.Tabs()) != 1 || !p.Tabs()[0].Live {
		t.Fatalf("plate tabs %+v", p.Tabs())
	}
	r.ws.SwitchDocument(1)
	if !r.hasText(0, "bracket answer") || r.hasText(0, "plate answer") {
		t.Fatalf("bracket tabs %+v", p.Tabs())
	}
	// Both conversations are in the project's history, each tagged with its drawing.
	ms := p.Sessions()
	drawings := map[string]bool{}
	for _, m := range ms {
		drawings[m.Drawing] = true
	}
	if len(ms) != 2 || !drawings["plate.dxf"] || !drawings["bracket.dxf"] {
		t.Fatalf("history %+v", ms)
	}
	// Closing the bracket tab ends its session only.
	r.ws.CloseDocument(1, r.hw.Window())
	r.until("bracket closed", r.sess(1).isClosed)
	if r.fs.isClosed() || !r.hasText(0, "plate answer") {
		t.Fatal("closing bracket's tab touched plate's conversation")
	}
}

func TestSaveAsInProjectKeepsSessionAndMovesTabs(t *testing.T) {
	dir := t.TempDir()
	r := newRig(t, dir, memFiles{}, true)
	p := r.ws.AgentPanel()
	r.chat(0, "draw a plate", "SP", "plate drawn")

	r.saveAs(filepath.Join(dir, "plate-v2.dxf"))
	for range 5 {
		r.tick()
	}
	if r.fs.isClosed() || !p.Tabs()[0].Live || !r.hasText(0, "plate drawn") {
		t.Fatalf("save as dropped the session: %+v", p.Tabs())
	}
	if m := p.Tabs()[0]; m.SessionID == "" {
		t.Fatal("tab lost its record")
	}
	r.ws.CloseAgent()

	r2 := newRig(t, dir, memFiles{}, true)
	if len(r2.ws.AgentPanel().Tabs()) != 1 || r2.hasText(0, "plate drawn") {
		t.Fatalf("plate.dxf still has the moved tabs: %+v", r2.ws.AgentPanel().Tabs())
	}
	r2.open(filepath.Join(dir, "plate-v2.dxf"))
	if !r2.hasText(0, "plate drawn") {
		t.Fatalf("plate-v2.dxf did not get the tabs: %+v", r2.ws.AgentPanel().Tabs())
	}
	if m := r2.ws.AgentPanel().Sessions()[0]; m.Drawing != "plate-v2.dxf" {
		t.Fatalf("record drawing %q", m.Drawing)
	}
}

func TestSaveAsToAnotherProjectCopiesConversations(t *testing.T) {
	a, b := t.TempDir(), t.TempDir()
	r := newRig(t, a, memFiles{}, true)
	p := r.ws.AgentPanel()
	r.chat(0, "draw a plate", "SP", "plate drawn")
	id := p.Tabs()[0].SessionID

	r.saveAs(filepath.Join(b, "copy.dxf"))
	r.until("closed", r.fs.isClosed)
	if tabs := p.Tabs(); len(tabs) != 1 || tabs[0].SessionID != id || tabs[0].Live || !r.hasText(0, "plate drawn") {
		t.Fatalf("tabs after save as %+v", tabs)
	}
	if _, err := os.Stat(filepath.Join(b, ".cad-lite", "agent", id+".jsonl")); err != nil {
		t.Fatalf("conversation not copied: %v", err)
	}
	if _, err := os.Stat(filepath.Join(a, ".cad-lite", "agent", id+".jsonl")); err != nil {
		t.Fatalf("original history lost: %v", err)
	}
	r.say("continue", 1)
	if cfg := r.nextCfg(); cfg.ResumeID != "SP" || cfg.Cwd != b {
		t.Fatalf("resume in b: %q in %q", cfg.ResumeID, cfg.Cwd)
	}
	r.open(filepath.Join(a, "plate.dxf"))
	if len(p.Tabs()) != 1 || r.hasText(0, "plate drawn") {
		t.Fatalf("a/plate.dxf kept the moved tabs: %+v", p.Tabs())
	}
}

func TestUntitledDrawingTabsFollowFirstSave(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	r := newRig(t, dir, memFiles{}, true)
	p := r.ws.AgentPanel()
	r.ws.Session().Run("NEW") // an untitled tab beside plate.dxf
	r.chat(0, "sketch", "SU", "sketched")

	r.saveAs(filepath.Join(dir, "sketch.dxf"))
	for range 5 {
		r.tick()
	}
	if r.fs.isClosed() || !r.hasText(0, "sketched") {
		t.Fatal("first save dropped the untitled conversation")
	}
	st, _ := store.Open(filepath.Join(dir, ".cad-lite", "agent"))
	if _, err := st.LoadOpen(); err == nil {
		t.Fatal("untitled open set still lists the saved drawing's tabs")
	}
	if set, err := st.LoadOpenFor("sketch.dxf"); err != nil || len(set.IDs) != 1 {
		t.Fatalf("sketch.dxf set %+v %v", set, err)
	}
	r.ws.Session().Run("NEW")
	if len(p.Tabs()) != 1 || r.hasText(0, "sketched") {
		t.Fatalf("the new drawing shows sketch.dxf's tabs: %+v", p.Tabs())
	}
	r.ws.SwitchDocument(1)
	if r.fs.isClosed() || !r.hasText(0, "sketched") {
		t.Fatal("sketch.dxf's tab lost its conversation")
	}
}

func TestLegacyOpenSetGoesToFirstDrawing(t *testing.T) {
	dir := t.TempDir()
	st, _ := store.Open(filepath.Join(dir, ".cad-lite", "agent"))
	rec, err := st.Create(agentmanager.Claude)
	if err != nil {
		t.Fatal(err)
	}
	rec.AddUser("from an older version")
	rec.Close()
	id := rec.Meta().ID
	legacy := `{"IDs":["` + id + `"],"Active":"` + id + `"}`
	if err := os.WriteFile(filepath.Join(st.Dir(), "open.json"), []byte(legacy), 0o600); err != nil {
		t.Fatal(err)
	}
	r := newRig(t, dir, memFiles{}, true)
	if tabs := r.ws.AgentPanel().Tabs(); len(tabs) != 1 || tabs[0].SessionID != id {
		t.Fatalf("legacy tabs not restored: %+v", tabs)
	}
	if _, err := os.Stat(filepath.Join(st.Dir(), "open.json")); !os.IsNotExist(err) {
		t.Fatalf("legacy open set left behind: %v", err)
	}
	if set, err := st.LoadOpenFor("plate.dxf"); err != nil || len(set.IDs) != 1 || set.IDs[0] != id {
		t.Fatalf("plate.dxf set %+v %v", set, err)
	}
	r.open(filepath.Join(dir, "other.dxf"))
	if tabs := r.ws.AgentPanel().Tabs(); len(tabs) != 1 || tabs[0].SessionID != "" {
		t.Fatalf("a second drawing adopted the legacy tabs too: %+v", tabs)
	}
}
