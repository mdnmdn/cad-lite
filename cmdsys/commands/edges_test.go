package commands_test

import (
	"math"
	"strings"
	"testing"

	"go-draw/cmdsys"
	"go-draw/doc"
	"go-draw/geom"
)

// standard puts the session in Standard TRIM and EXTEND mode (TRIMEXTENDMODE 0).
func standard(t *testing.T, s *cmdsys.Session) {
	t.Helper()
	if err := s.Vars().Set("TRIMEXTENDMODE", 0); err != nil {
		t.Fatal(err)
	}
}

func TestTrimExtendModeVariable(t *testing.T) {
	s := newSession(t)
	if got := s.Vars().Int("TRIMEXTENDMODE"); got != 1 {
		t.Fatalf("default TRIMEXTENDMODE = %d, want 1 (Quick)", got)
	}
	s.Run("TRIM")
	h := history(s)
	if !strings.Contains(h, "Current settings: Projection=UCS, Edge=None, Mode=Quick") {
		t.Fatalf("history = %q", h)
	}
}

func TestTrimStandardAsksEdgesFirst(t *testing.T) {
	s := newSession(t)
	standard(t, s)
	s.Run("TRIM")
	h := history(s)
	if !strings.Contains(h, "Current settings: Projection=UCS, Edge=None\n") || strings.Contains(h, "Mode=Quick") ||
		!strings.Contains(h, "Select cutting edges ...") {
		t.Fatalf("history = %q", h)
	}
	if got := s.Prompt(); got != "Select objects or <select all>:" {
		t.Fatalf("prompt = %q", got)
	}
	s.Submit("")
	if got := s.Prompt(); got != "Select object to trim or shift-select to extend or [Fence/Crossing/Project/Edge/eRase/Undo]:" {
		t.Fatalf("prompt = %q", got)
	}
	s.Submit("")
	if s.Active() != "" {
		t.Fatalf("Enter did not end TRIM: %s", s.Active())
	}
}

func TestTrimStandardEnterMeansAllEdges(t *testing.T) {
	s := newSession(t)
	standard(t, s)
	addEntities(t, s, line(geom.V(0, 0), geom.V(100, 0)), line(geom.V(30, -10), geom.V(30, 10)))
	s.Script("TRIM  90,0 ")
	if !hasLine(sortedLines(s), geom.V(0, 0), geom.V(30, 0)) {
		t.Fatalf("lines = %v", dumpLines(sortedLines(s)))
	}
}

func TestTrimStandardExplicitEdgesByTypedWindow(t *testing.T) {
	s := newSession(t)
	standard(t, s)
	addEntities(t, s, line(geom.V(0, 0), geom.V(100, 0)),
		line(geom.V(30, -10), geom.V(30, 10)), line(geom.V(70, -10), geom.V(70, 10)))
	// W selects only the x=30 line as the edge; the x=70 line does not cut.
	s.Script("TRIM W 25,-20 35,20  90,0 ")
	if s.Active() != "" {
		t.Fatalf("active = %s (%s)", s.Active(), s.Prompt())
	}
	if !hasLine(sortedLines(s), geom.V(0, 0), geom.V(30, 0)) {
		t.Fatalf("lines = %v", dumpLines(sortedLines(s)))
	}
}

func TestTrimQuickCuttingEdgesOption(t *testing.T) {
	s := newSession(t)
	addEntities(t, s, line(geom.V(0, 0), geom.V(100, 0)),
		line(geom.V(30, -10), geom.V(30, 10)), line(geom.V(70, -10), geom.V(70, 10)))
	s.Run("TRIM")
	s.Submit("T")
	if got := s.Prompt(); got != "Select objects or <select all>:" {
		t.Fatalf("prompt = %q", got)
	}
	s.Click(geom.V(70, 8), false)
	s.Submit("")
	s.Submit("90,0")
	s.Submit("")
	if !hasLine(sortedLines(s), geom.V(0, 0), geom.V(70, 0)) {
		t.Fatalf("lines = %v", dumpLines(sortedLines(s)))
	}
}

func TestTrimModeOptionSwitchesToStandard(t *testing.T) {
	s := newSession(t)
	s.Run("TRIM")
	s.Submit("O")
	if got := s.Prompt(); got != "Enter a trimming mode option [Quick/Standard] <Quick>:" {
		t.Fatalf("prompt = %q", got)
	}
	s.Submit("S")
	if got := s.Vars().Int("TRIMEXTENDMODE"); got != 0 {
		t.Fatalf("TRIMEXTENDMODE = %d", got)
	}
	if got := s.Prompt(); got != "Select objects or <select all>:" {
		t.Fatalf("prompt = %q", got)
	}
}

func fenceGrid(t *testing.T) *cmdsys.Session {
	s := newSession(t)
	standard(t, s)
	addEntities(t, s, line(geom.V(50, 90), geom.V(50, 130)),
		line(geom.V(0, 100), geom.V(100, 100)), line(geom.V(0, 110), geom.V(100, 110)), line(geom.V(0, 120), geom.V(100, 120)))
	return s
}

func TestTrimFence(t *testing.T) {
	s := fenceGrid(t)
	s.Script("TRIM  F 75,95 75,125  ")
	if s.Active() != "" {
		t.Fatalf("active = %s (%s)", s.Active(), s.Prompt())
	}
	ls := sortedLines(s)
	for _, y := range []float64{100, 110, 120} {
		if !hasLine(ls, geom.V(0, y), geom.V(50, y)) {
			t.Fatalf("lines = %v", dumpLines(ls))
		}
	}
	s.Drawing().Undo()
	if !hasLine(sortedLines(s), geom.V(0, 100), geom.V(100, 100)) {
		t.Fatal("a fence trim is not one undo step")
	}
}

func TestTrimFenceKeepsPiecesItDoesNotTouch(t *testing.T) {
	// Two edges cut the line in three; a fence over the middle piece only removes it.
	s := newSession(t)
	standard(t, s)
	addEntities(t, s, line(geom.V(0, 0), geom.V(100, 0)),
		line(geom.V(30, -10), geom.V(30, 10)), line(geom.V(70, -10), geom.V(70, 10)))
	s.Script("TRIM  F 50,-5 50,5  ")
	ls := sortedLines(s)
	if len(ls) != 4 || !hasLine(ls, geom.V(0, 0), geom.V(30, 0)) || !hasLine(ls, geom.V(70, 0), geom.V(100, 0)) {
		t.Fatalf("lines = %v", dumpLines(ls))
	}
}

func TestTrimCrossingWindow(t *testing.T) {
	s := fenceGrid(t)
	// Right-to-left or not, Crossing trims what the window touches: the pieces right of x=50.
	s.Script("TRIM  C 60,95 80,125  ")
	ls := sortedLines(s)
	if !hasLine(ls, geom.V(0, 110), geom.V(50, 110)) || !hasLine(ls, geom.V(50, 90), geom.V(50, 130)) || len(ls) != 4 {
		t.Fatalf("lines = %v", dumpLines(ls))
	}
}

func TestTrimUndoOption(t *testing.T) {
	s := newSession(t)
	standard(t, s)
	addEntities(t, s, &doc.Circle{Center: geom.V(0, 0), Radius: 10}, line(geom.V(0, -20), geom.V(0, 20)))
	s.Script("TRIM  10,0")
	if len(entities[*doc.Arc](s)) != 1 || len(entities[*doc.Circle](s)) != 0 {
		t.Fatal("circle was not trimmed")
	}
	s.Submit("U")
	if len(entities[*doc.Arc](s)) != 0 || len(entities[*doc.Circle](s)) != 1 {
		t.Fatalf("arcs=%d circles=%d after Undo", len(entities[*doc.Arc](s)), len(entities[*doc.Circle](s)))
	}
	if c := entities[*doc.Circle](s)[0]; c.Radius != 10 {
		t.Fatalf("circle = %+v", c)
	}
	s.Submit("U")
	if got := lastLine(s); got != "Nothing to undo." {
		t.Fatalf("last = %q", got)
	}
	s.Submit("")
	if s.Active() != "" || len(entities[*doc.Circle](s)) != 1 || len(entities[*doc.Arc](s)) != 0 {
		t.Fatalf("active=%s circles=%d arcs=%d", s.Active(), len(entities[*doc.Circle](s)), len(entities[*doc.Arc](s)))
	}
}

func TestTrimUndoSeveralSteps(t *testing.T) {
	s := newSession(t)
	addEntities(t, s, line(geom.V(0, 0), geom.V(100, 0)), line(geom.V(30, -10), geom.V(30, 10)), line(geom.V(70, -10), geom.V(70, 10)))
	s.Script("TRIM 30,5 70,-5")
	s.Submit("U")
	ls := sortedLines(s)
	if !hasLine(ls, geom.V(30, 0), geom.V(30, -10)) || !hasLine(ls, geom.V(70, -10), geom.V(70, 10)) {
		t.Fatalf("lines = %v", dumpLines(ls))
	}
	s.Submit("U")
	if !hasLine(sortedLines(s), geom.V(30, -10), geom.V(30, 10)) {
		t.Fatalf("lines = %v", dumpLines(sortedLines(s)))
	}
	s.Submit("")
}

func TestTrimEraseOption(t *testing.T) {
	s := newSession(t)
	addEntities(t, s, line(geom.V(0, 0), geom.V(100, 0)), line(geom.V(0, 50), geom.V(100, 50)))
	s.Run("TRIM")
	s.Submit("R")
	if got := s.Prompt(); got != "Select objects to erase or <exit>:" {
		t.Fatalf("prompt = %q", got)
	}
	s.Click(geom.V(50, 50), false)
	s.Submit("")
	if ls := sortedLines(s); len(ls) != 1 || !hasLine(ls, geom.V(0, 0), geom.V(100, 0)) {
		t.Fatalf("lines = %v", dumpLines(ls))
	}
	s.Submit("U")
	if len(sortedLines(s)) != 2 {
		t.Fatalf("Undo did not restore the erased line: %v", dumpLines(sortedLines(s)))
	}
	s.Submit("")
}

func TestTrimEdgeExtendUsesImpliedEdge(t *testing.T) {
	s := newSession(t)
	standard(t, s)
	// The cutting line stops short of the object; only its projection cuts it.
	addEntities(t, s, line(geom.V(0, 0), geom.V(100, 0)), line(geom.V(50, 10), geom.V(50, 30)))
	s.Script("TRIM  90,0")
	if lastLine(s) != "Object does not intersect an edge." {
		t.Fatalf("last = %q", lastLine(s))
	}
	s.Submit("E")
	if got := s.Prompt(); got != "Enter an implied edge extension mode [Extend/No extend] <No extend>:" {
		t.Fatalf("prompt = %q", got)
	}
	s.Submit("E")
	if got := s.Vars().Int("EDGEMODE"); got != 1 {
		t.Fatalf("EDGEMODE = %d", got)
	}
	s.Submit("90,0")
	s.Submit("")
	if !hasLine(sortedLines(s), geom.V(0, 0), geom.V(50, 0)) {
		t.Fatalf("lines = %v", dumpLines(sortedLines(s)))
	}
}

func TestExtendEdgeExtendReachesProjectedBoundary(t *testing.T) {
	s := newSession(t)
	_ = s.Vars().Set("EDGEMODE", 1)
	addEntities(t, s, line(geom.V(0, 0), geom.V(10, 0)), line(geom.V(40, 5), geom.V(40, 20)))
	s.Script("EXTEND 9,0 ")
	if !hasLine(sortedLines(s), geom.V(0, 0), geom.V(40, 0)) {
		t.Fatalf("lines = %v", dumpLines(sortedLines(s)))
	}
}

func TestTrimProjectOption(t *testing.T) {
	s := newSession(t)
	s.Run("TRIM")
	s.Submit("P")
	if got := s.Prompt(); got != "Enter a projection option [None/Ucs/View] <Ucs>:" {
		t.Fatalf("prompt = %q", got)
	}
	s.Submit("N")
	if got := s.Vars().Int("PROJMODE"); got != 0 {
		t.Fatalf("PROJMODE = %d", got)
	}
	s.Escape()
	s.Run("TRIM")
	if !strings.Contains(history(s), "Projection=None") {
		t.Fatalf("history = %q", history(s))
	}
}

func TestExtendStandardPromptsAndKeywords(t *testing.T) {
	s := newSession(t)
	standard(t, s)
	addEntities(t, s, line(geom.V(0, 0), geom.V(50, 0)), line(geom.V(100, -50), geom.V(100, 50)))
	s.Run("EX")
	if !strings.Contains(history(s), "Select boundary edges ...") || s.Prompt() != "Select objects or <select all>:" {
		t.Fatalf("history = %q prompt = %q", history(s), s.Prompt())
	}
	s.Submit("")
	if got := s.Prompt(); got != "Select object to extend or shift-select to trim or [Fence/Crossing/Project/Edge/Undo]:" {
		t.Fatalf("prompt = %q", got)
	}
	s.Submit("40,0")
	s.Submit("")
	if !hasLine(sortedLines(s), geom.V(0, 0), geom.V(100, 0)) {
		t.Fatalf("lines = %v", dumpLines(sortedLines(s)))
	}
}

func TestExtendQuickPrompt(t *testing.T) {
	s := newSession(t)
	s.Run("EXTEND")
	if got := s.Prompt(); got != "Select object to extend or shift-select to trim or [Boundary edges/Crossing/mOde/Project/eRase/Undo]:" {
		t.Fatalf("prompt = %q", got)
	}
}

func TestExtendFence(t *testing.T) {
	s := newSession(t)
	standard(t, s)
	addEntities(t, s, line(geom.V(0, 0), geom.V(10, 0)), line(geom.V(0, 20), geom.V(10, 20)), line(geom.V(40, -10), geom.V(40, 40)))
	s.Script("EXTEND  F 8,-5 8,25  ")
	ls := sortedLines(s)
	if !hasLine(ls, geom.V(0, 0), geom.V(40, 0)) || !hasLine(ls, geom.V(0, 20), geom.V(40, 20)) {
		t.Fatalf("lines = %v", dumpLines(ls))
	}
}

func TestTrimKeywordsItalian(t *testing.T) {
	italian(t)
	s := newSession(t)
	standard(t, s)
	s.Run("TAGLIA")
	s.Submit("")
	if got := s.Prompt(); !strings.Contains(got, "[Recinto/Incrocio/Proietta/Lato/Cancella/Annulla]") {
		t.Fatalf("prompt = %q", got)
	}
	s.Submit("r")
	s.Submit("0,0")
	s.Submit("10,0")
	s.Submit("")
	s.Submit("")
	if s.Active() != "" {
		t.Fatalf("active = %s (%s)", s.Active(), s.Prompt())
	}
}

func TestBreakAtPoint(t *testing.T) {
	s := newSession(t)
	addEntities(t, s, line(geom.V(0, 0), geom.V(100, 0)))
	s.Run("BREAKATPOINT")
	if got := s.Prompt(); got != "Select object:" {
		t.Fatalf("prompt = %q", got)
	}
	s.Submit("10,0")
	if got := s.Prompt(); got != "Specify break point:" {
		t.Fatalf("prompt = %q", got)
	}
	s.Submit("40,0")
	ls := sortedLines(s)
	if s.Active() != "" || len(ls) != 2 || !hasLine(ls, geom.V(0, 0), geom.V(40, 0)) || !hasLine(ls, geom.V(40, 0), geom.V(100, 0)) {
		t.Fatalf("lines = %v", dumpLines(ls))
	}
	s.Drawing().Undo()
	if len(sortedLines(s)) != 1 {
		t.Fatal("BREAKATPOINT is not one undo step")
	}
}

func TestBreakAtPointRefusesClosedObjects(t *testing.T) {
	s := newSession(t)
	addEntities(t, s, &doc.Circle{Center: geom.V(0, 0), Radius: 10})
	s.Script("BREAKATPOINT 10,0")
	if s.Active() != "" || lastLine(s) != "Cannot break a closed object at a single point." {
		t.Fatalf("active = %s last = %q", s.Active(), lastLine(s))
	}
	if len(entities[*doc.Circle](s)) != 1 {
		t.Fatal("circle changed")
	}
}

func TestBreakAtPointArc(t *testing.T) {
	s := newSession(t)
	addEntities(t, s, &doc.Arc{Center: geom.V(0, 0), Radius: 10, StartAngle: 0, EndAngle: math.Pi})
	s.Script("BREAKATPOINT 10,0 0,10")
	arcs := entities[*doc.Arc](s)
	if len(arcs) != 2 {
		t.Fatalf("arcs = %d", len(arcs))
	}
}

func TestBreakAtSignIsFirstPoint(t *testing.T) {
	s := newSession(t)
	addEntities(t, s, line(geom.V(0, 0), geom.V(100, 0)))
	s.Script("BREAK 25,0 @")
	ls := sortedLines(s)
	if len(ls) != 2 || !hasLine(ls, geom.V(0, 0), geom.V(25, 0)) || !hasLine(ls, geom.V(25, 0), geom.V(100, 0)) {
		t.Fatalf("lines = %v", dumpLines(ls))
	}
	// After First point, @ is that first point.
	s = newSession(t)
	addEntities(t, s, line(geom.V(0, 0), geom.V(100, 0)))
	s.Script("BREAK 25,0 F 60,0 @")
	ls = sortedLines(s)
	if len(ls) != 2 || !hasLine(ls, geom.V(0, 0), geom.V(60, 0)) {
		t.Fatalf("lines = %v", dumpLines(ls))
	}
}

func TestJoinSourceMessagesAndArcClose(t *testing.T) {
	s := newSession(t)
	addEntities(t, s, &doc.Arc{Center: geom.V(0, 0), Radius: 5, StartAngle: 0, EndAngle: math.Pi})
	s.Run("JOIN")
	s.Submit("5,0")
	s.Submit("")
	if got := s.Prompt(); got != "Select arcs to join to source or [Close]:" {
		t.Fatalf("prompt = %q", got)
	}
	s.Submit("C")
	cs := entities[*doc.Circle](s)
	if s.Active() != "" || len(cs) != 1 || len(entities[*doc.Arc](s)) != 0 || cs[0].Radius != 5 {
		t.Fatalf("circles = %+v active = %s", cs, s.Active())
	}
}

func TestJoinTypedWindow(t *testing.T) {
	s := newSession(t)
	addEntities(t, s, line(geom.V(0, 0), geom.V(50, 0)), line(geom.V(50, 0), geom.V(100, 0)))
	s.Script("JOIN W -5,-5 105,5 ")
	if ls := sortedLines(s); s.Active() != "" || len(ls) != 1 || !hasLine(ls, geom.V(0, 0), geom.V(100, 0)) {
		t.Fatalf("lines = %v active = %s", dumpLines(sortedLines(s)), s.Active())
	}
	if got := lastLine(s); got != "1 object joined." {
		t.Fatalf("last = %q", got)
	}
}

func TestExplodePrompt(t *testing.T) {
	s := newSession(t)
	s.Run("EXPLODE")
	if got := s.Prompt(); got != "Select objects:" {
		t.Fatalf("prompt = %q", got)
	}
}
