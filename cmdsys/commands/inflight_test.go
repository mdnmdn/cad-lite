package commands_test

import (
	"testing"

	"cad-lite/cmdsys"
	"cad-lite/cmdsys/commands"
	"cad-lite/doc"
	"cad-lite/geom"
	"cad-lite/render"
)

// everywhere is a box larger than any test drawing.
var everywhere = geom.BoxOf(geom.V(-1e6, -1e6), geom.V(1e6, 1e6))

// shown returns the display items the canvas draws for the drawing now: the entities the spatial
// index finds in the view, built as the canvas builds them.
func shown(s *cmdsys.Session) []render.DisplayItem {
	var dl render.DisplayList
	b := render.NewBuilder(s.Drawing(), render.Color{})
	b.Build(s.Index().Query(everywhere), *s.View(), &dl)
	return dl.Items
}

// Each LINE segment is an entity the moment it is placed, as in AutoCAD: it draws, it snaps and
// Undo inside LINE takes it away, while the command stays one undo step.
func TestLineSegmentsShowWhileDrawing(t *testing.T) {
	s := newSession(t)
	s.Script("LINE 0,0 100,0 100,50")
	if s.Active() != "LINE" {
		t.Fatalf("LINE ended: %q", s.Active())
	}
	if n := len(s.Index().Query(everywhere)); n != 2 {
		t.Fatalf("index holds %d of the 2 placed segments", n)
	}
	if n := len(shown(s)); n != 2 {
		t.Fatalf("display list draws %d of the 2 placed segments", n)
	}
	s.Hover(geom.V(50.5, 0.5))
	if hit, ok := s.Snap(); !ok || hit.Point != geom.V(50, 0) || hit.Mode != cmdsys.SnapMidpoint {
		t.Fatalf("no midpoint snap on the first segment: %+v %v", hit, ok)
	}
	s.Submit("U")
	if n := len(shown(s)); n != 1 || s.Drawing().Len() != 1 {
		t.Fatalf("after Undo: %d drawn, %d in the drawing, want 1", n, s.Drawing().Len())
	}
	s.Hover(geom.V(100.5, 49.5))
	if hit, ok := s.Snap(); ok && hit.Point == geom.V(100, 50) {
		t.Fatalf("snapped to the end of the undone segment: %+v", hit)
	}
	s.Script("100,80 ")
	if s.Active() != "" || s.Drawing().Len() != 2 {
		t.Fatalf("active %q, %d lines", s.Active(), s.Drawing().Len())
	}
	if d := s.Drawing().UndoDepth(); d != 1 {
		t.Fatalf("LINE made %d undo steps, want 1", d)
	}
	s.Script("U")
	if s.Drawing().Len() != 0 || len(s.Index().Query(everywhere)) != 0 {
		t.Fatalf("U left %d entities (%d indexed)", s.Drawing().Len(), len(s.Index().Query(everywhere)))
	}
}

// Escape keeps the segments placed, and they stay indexed after the commit.
func TestLineCancelKeepsShownSegments(t *testing.T) {
	s := newSession(t)
	s.Script("LINE 0,0 10,0 10,10")
	s.Escape()
	if s.Drawing().Len() != 2 || len(s.Index().Query(everywhere)) != 2 || len(shown(s)) != 2 {
		t.Fatalf("after Escape: %d entities, %d indexed", s.Drawing().Len(), len(s.Index().Query(everywhere)))
	}
}

// Every command that adds entities before it ends shows them while it runs.
func TestCommandsShowWorkBeforeTheyEnd(t *testing.T) {
	cases := []struct {
		name   string
		setup  func(*cmdsys.Session)
		script string
		want   int // entities drawn mid-command
	}{
		{"LINE", nil, "LINE 0,0 10,0 10,10 0,10", 3},
		{"COPY multiple", func(s *cmdsys.Session) {
			addEntities(t, s, line(geom.V(0, 0), geom.V(10, 0)))
			s.Selection().Add(s.Drawing().Handles()[0])
		}, "COPY 0,0 0,5 0,10", 3},
		{"OFFSET multiple", func(s *cmdsys.Session) {
			addEntities(t, s, line(geom.V(0, 0), geom.V(100, 0)))
		}, "OFFSET 10 50,0 M 50,5 50,15", 3},
		{"OFFSET", func(s *cmdsys.Session) {
			addEntities(t, s, line(geom.V(0, 0), geom.V(100, 0)))
		}, "OFFSET 10 50,0 50,5", 2},
		{"TEXT", nil, "TEXT\n0,0\n2.5\n0\nfirst\nsecond", 2},
	}
	for _, c := range cases {
		s := newSession(t)
		if c.setup != nil {
			c.setup(s)
		}
		s.Script(c.script)
		if s.Active() == "" {
			t.Fatalf("%s: the command ended; history:\n%s", c.name, history(s))
		}
		if n := len(s.Index().Query(everywhere)); n != c.want {
			t.Fatalf("%s: %d entities indexed mid-command, want %d", c.name, n, c.want)
		}
		hs := map[doc.Handle]bool{}
		for _, it := range shown(s) {
			hs[it.Owner] = true
		}
		if len(hs) != c.want {
			t.Fatalf("%s: %d entities drawn mid-command, want %d", c.name, len(hs), c.want)
		}
		s.Escape()
		if d := s.Drawing().UndoDepth(); c.setup == nil && d != 1 {
			t.Fatalf("%s: %d undo steps, want 1", c.name, d)
		}
	}
}

// An entity a command changes before it ends (each TRIM pick) is re-indexed and queued for the
// display cache at once, not when the command ends.
func TestTrimPickShowsBeforeTrimEnds(t *testing.T) {
	s := newSession(t)
	addEntities(t, s, line(geom.V(0, 0), geom.V(100, 0)), line(geom.V(50, -10), geom.V(50, 10)))
	h := s.Drawing().Handles()[0]
	s.TakeLiveEdits()
	s.Script("TRIM 80,0")
	if s.Active() != "TRIM" {
		t.Fatalf("TRIM ended: %q", s.Active())
	}
	for _, g := range s.Index().Query(geom.BoxOf(geom.V(70, -1), geom.V(90, 1))) {
		if g == h {
			t.Fatal("the index still holds the trimmed line's old extent")
		}
	}
	live := s.TakeLiveEdits()
	found := false
	for _, g := range live.Modified {
		found = found || g == h
	}
	if !found {
		t.Fatalf("the trimmed line is not queued for the display cache: %+v", live)
	}
	if again := s.TakeLiveEdits(); len(again.Modified)+len(again.Erased) != 0 {
		t.Fatalf("TakeLiveEdits did not empty its queue: %+v", again)
	}
	s.Submit("")
	if s.Drawing().UndoDepth() != 2 {
		t.Fatalf("undo depth %d, want 2 (the setup and TRIM)", s.Drawing().UndoDepth())
	}
}

// OFFSET picks a copy it made earlier in the same command (backlog G37): the copy is indexed when
// it is made.
func TestOffsetPicksItsOwnEarlierCopy(t *testing.T) {
	s := newSession(t)
	addEntities(t, s, line(geom.V(0, 0), geom.V(100, 0)))
	s.Script("OFFSET 10 50,0 50,5 50,10 50,15 ")
	ls := entities[*doc.Line](s)
	if len(ls) != 3 || !hasLine(ls, geom.V(0, 20), geom.V(100, 20)) {
		t.Fatalf("lines = %v\n%s", dumpLines(ls), history(s))
	}
}

// stepCmd is a test command: its first answer adds a line; its second erases the newest entity
// and then ends, or panics when panics is set. Cancel erases the newest entity too.
type stepCmd struct {
	n      int
	panics bool
	// oneStep adds, erases and panics within the first answer.
	oneStep bool
}

func (*stepCmd) Name() string { return "STEPCMD" }
func (*stepCmd) Start(*cmdsys.Context) *cmdsys.Prompt {
	return &cmdsys.Prompt{Message: "p:", Accepts: cmdsys.InputPoint}
}
func (c *stepCmd) Input(ctx *cmdsys.Context, in cmdsys.Input) *cmdsys.Prompt {
	c.n++
	if c.oneStep {
		ctx.Tx().Erase(ctx.Tx().Add(line(in.Point, in.Point.Add(geom.V(10, 0)))))
		panic("stepCmd")
	}
	if c.n == 1 {
		ctx.Tx().Add(line(in.Point, in.Point.Add(geom.V(10, 0))))
		return &cmdsys.Prompt{Message: "p:", Accepts: cmdsys.InputPoint}
	}
	ctx.Tx().Erase(ctx.Doc.Handles()[len(ctx.Doc.Handles())-1])
	if c.panics {
		panic("stepCmd")
	}
	return nil
}
func (*stepCmd) Preview(*cmdsys.Context, geom.Vec2, *render.DisplayList) {}
func (c *stepCmd) Cancel(ctx *cmdsys.Context) {
	if hs := ctx.Doc.Handles(); c.n == 1 && len(hs) > 0 {
		ctx.Tx().Erase(hs[len(hs)-1])
	}
}

func stepSession() *cmdsys.Session {
	r := commands.NewRegistry()
	r.Register("STEPCMD", func() cmdsys.Command { return &stepCmd{} })
	r.Register("STEPPANIC", func() cmdsys.Command { return &stepCmd{panics: true} })
	r.Register("STEPONCE", func() cmdsys.Command { return &stepCmd{oneStep: true} })
	v := render.DefaultView(800, 600)
	return cmdsys.NewSession(doc.New(), &v, r)
}

// wantIndexed fails unless the spatial index holds exactly the drawing's entities.
func wantIndexed(t *testing.T, s *cmdsys.Session, when string) {
	t.Helper()
	if n, m := s.Drawing().Len(), len(s.Index().Query(everywhere)); n != m {
		t.Fatalf("%s: %d entities, %d indexed", when, n, m)
	}
}

// An entity indexed in one step and erased in the step that commits leaves no index entry: the
// commit's change set omits it (added and erased by one transaction), so the session syncs before
// every commit (finish, MULTIPLE's CommitStep, Escape) and before an abort.
func TestNoStaleIndexEntryAtCommit(t *testing.T) {
	s := stepSession()
	s.Script("MULTIPLE STEPCMD 0,0")
	wantIndexed(t, s, "MULTIPLE, line added")
	s.Submit("1,1")
	wantIndexed(t, s, "MULTIPLE, line erased at the run's commit")
	if live := s.TakeLiveEdits(); len(live.Erased) == 0 {
		t.Fatalf("the erased line is not queued for the display cache: %+v", live)
	}
	s.Escape()
	s.Script("STEPCMD 0,0")
	s.Escape() // Cancel erases the line
	wantIndexed(t, s, "Escape whose Cancel erases")
	s.Script("STEPCMD 0,0 1,1")
	wantIndexed(t, s, "a command that adds and erases")
}

// A panic aborts the transaction; an entity the command indexed in an earlier step leaves the
// index with it.
func TestNoStaleIndexEntryAfterPanic(t *testing.T) {
	s := stepSession()
	addEntities(t, s, line(geom.V(0, 50), geom.V(10, 50)))
	s.Script("STEPPANIC 0,0")
	wantIndexed(t, s, "added")
	s.Submit("1,1")
	if s.Active() != "" {
		t.Fatalf("the panic did not end the command: %q", s.Active())
	}
	wantIndexed(t, s, "after the panic")
	if s.Drawing().Len() != 1 {
		t.Fatalf("abort left %d entities, want the 1 from before", s.Drawing().Len())
	}
	// Added, erased and panicking within one answer: nothing was ever indexed.
	s.Script("STEPONCE 0,0")
	wantIndexed(t, s, "add, erase and panic in one step")
	if s.Drawing().Len() != 1 || s.Active() != "" {
		t.Fatalf("after the one-step panic: %d entities, active %q", s.Drawing().Len(), s.Active())
	}
}
