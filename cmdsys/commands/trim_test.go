package commands_test

import (
	"math"
	"testing"

	"go-draw/cmdsys"
	"go-draw/doc"
	"go-draw/geom"
)

func line(a, b geom.Vec2) *doc.Line { return &doc.Line{Start: a, End: b} }

func sortedLines(s *cmdsys.Session) []*doc.Line { return entities[*doc.Line](s) }

func hasLine(ls []*doc.Line, a, b geom.Vec2) bool {
	for _, l := range ls {
		if (l.Start.Near(a, 1e-9) && l.End.Near(b, 1e-9)) || (l.Start.Near(b, 1e-9) && l.End.Near(a, 1e-9)) {
			return true
		}
	}
	return false
}

func TestTrimAliasesAndPrompt(t *testing.T) {
	s := newSession(t)
	for alias, name := range map[string]string{"TR": "TRIM", "EX": "EXTEND", "BR": "BREAK", "J": "JOIN", "X": "EXPLODE"} {
		s.Escape()
		if !s.Run(alias) {
			t.Fatalf("alias %s unknown", alias)
		}
		if s.Active() != name && s.Active() != "" {
			t.Fatalf("alias %s runs %s, want %s", alias, s.Active(), name)
		}
	}
	s.Escape()
	s.Run("TRIM")
	if got := s.Prompt(); got != "Select object to trim or shift-select to extend or [cuTting edges/Crossing/mOde/Project/eRase/Undo]:" {
		t.Fatalf("prompt = %q", got)
	}
}

func TestTrimMiddleOfLineBetweenTwoEdges(t *testing.T) {
	s := newSession(t)
	addEntities(t, s, line(geom.V(0, 0), geom.V(100, 0)), line(geom.V(30, -10), geom.V(30, 10)), line(geom.V(70, -10), geom.V(70, 10)))
	s.Script("TRIM 50,0 ")
	if s.Active() != "" {
		t.Fatalf("still active: %s", s.Active())
	}
	ls := sortedLines(s)
	if len(ls) != 4 || !hasLine(ls, geom.V(0, 0), geom.V(30, 0)) || !hasLine(ls, geom.V(70, 0), geom.V(100, 0)) {
		t.Fatalf("lines = %v", dumpLines(ls))
	}
	if !s.Drawing().Undo() {
		t.Fatal("nothing to undo")
	}
	ls = sortedLines(s)
	if len(ls) != 3 || !hasLine(ls, geom.V(0, 0), geom.V(100, 0)) {
		t.Fatalf("after undo: %v", dumpLines(ls))
	}
}

func dumpLines(ls []*doc.Line) []string {
	var out []string
	for _, l := range ls {
		out = append(out, cmdsys.FormatPoint(l.Start)+"-"+cmdsys.FormatPoint(l.End))
	}
	return out
}

func TestTrimIsOneUndoStepAndRepeats(t *testing.T) {
	s := newSession(t)
	addEntities(t, s, line(geom.V(0, 0), geom.V(100, 0)), line(geom.V(30, -10), geom.V(30, 10)), line(geom.V(70, -10), geom.V(70, 10)))
	// Trim both overhangs of the verticals: (30,5) and (70,-5) lie above/below.
	s.Script("TRIM 30,5 70,-5 ")
	ls := sortedLines(s)
	if !hasLine(ls, geom.V(30, 0), geom.V(30, -10)) || !hasLine(ls, geom.V(70, 0), geom.V(70, 10)) {
		t.Fatalf("lines = %v", dumpLines(ls))
	}
	s.Drawing().Undo()
	if len(sortedLines(s)) != 3 || !hasLine(sortedLines(s), geom.V(30, -10), geom.V(30, 10)) {
		t.Fatalf("two trims are not one undo step: %v", dumpLines(sortedLines(s)))
	}
}

func TestTrimToEndWhenOneSide(t *testing.T) {
	s := newSession(t)
	addEntities(t, s, line(geom.V(0, 0), geom.V(100, 0)), line(geom.V(30, -10), geom.V(30, 10)))
	s.Script("TRIM 90,0 ")
	ls := sortedLines(s)
	if !hasLine(ls, geom.V(0, 0), geom.V(30, 0)) || len(ls) != 2 {
		t.Fatalf("lines = %v", dumpLines(ls))
	}
}

func TestTrimNothingToTrim(t *testing.T) {
	s := newSession(t)
	addEntities(t, s, line(geom.V(0, 0), geom.V(100, 0)), line(geom.V(0, 50), geom.V(100, 50)))
	s.Script("TRIM 50,0")
	if got := lastLine(s); got == "" || len(sortedLines(s)) != 2 {
		t.Fatalf("lines changed: %v", dumpLines(sortedLines(s)))
	}
	h := s.History()
	if h[len(h)-1] != "Object does not intersect an edge." {
		t.Fatalf("history tail = %q", h[len(h)-1])
	}
	if s.Active() != "TRIM" {
		t.Fatal("TRIM must keep prompting")
	}
	s.Submit("")
	// Nothing changed: no undo step was recorded, a miss prints a message.
	s.Script("TRIM 500,500")
	if got := lastLine(s); got != "No object found at that point." {
		t.Fatalf("miss message = %q", got)
	}
	s.Escape()
	// The only undo step is the setup: a command that changed nothing left none.
	s.Drawing().Undo()
	if s.Drawing().Len() != 0 {
		t.Fatal("a command that changed nothing left an undo step")
	}
}

func TestTrimCircleToArc(t *testing.T) {
	s := newSession(t)
	addEntities(t, s, &doc.Circle{Center: geom.V(0, 0), Radius: 10}, line(geom.V(0, -20), geom.V(0, 20)))
	s.Script("TRIM 10,0 ")
	arcs := entities[*doc.Arc](s)
	if len(arcs) != 1 || len(entities[*doc.Circle](s)) != 0 {
		t.Fatalf("arcs=%d circles=%d", len(arcs), len(entities[*doc.Circle](s)))
	}
	a := arcs[0]
	if math.Abs(a.StartAngle-math.Pi/2) > 1e-9 || math.Abs(a.EndAngle-3*math.Pi/2) > 1e-9 || a.Radius != 10 {
		t.Fatalf("arc = %+v", a)
	}
	s.Drawing().Undo()
	if len(entities[*doc.Circle](s)) != 1 || len(entities[*doc.Arc](s)) != 0 {
		t.Fatal("undo did not restore the circle")
	}
}

func TestTrimArcBothSides(t *testing.T) {
	s := newSession(t)
	// Half circle on top; two vertical lines cross it at x=-6 and x=6.
	addEntities(t, s, &doc.Arc{Center: geom.V(0, 0), Radius: 10, StartAngle: 0, EndAngle: math.Pi},
		line(geom.V(-6, -5), geom.V(-6, 15)), line(geom.V(6, -5), geom.V(6, 15)))
	s.Script("TRIM 0,10 ")
	arcs := entities[*doc.Arc](s)
	if len(arcs) != 2 {
		t.Fatalf("arcs = %d", len(arcs))
	}
	want := math.Acos(0.6)
	var right, left *doc.Arc
	for _, a := range arcs {
		if a.StartAngle < 1 {
			right = a
		} else {
			left = a
		}
	}
	if right == nil || left == nil || math.Abs(right.EndAngle-want) > 1e-9 || math.Abs(left.StartAngle-(math.Pi-want)) > 1e-9 {
		t.Fatalf("arcs = %+v %+v", right, left)
	}
}

func TestTrimPolylineWithBulge(t *testing.T) {
	s := newSession(t)
	pl := &doc.LWPolyline{Points: []geom.Vec2{geom.V(0, 0), geom.V(10, 0), geom.V(10, 10)}, Bulges: []float64{1, 0}}
	pl.Layer = "WALLS"
	pl.Color = 3
	addEntities(t, s, pl, line(geom.V(5, -10), geom.V(5, 10)))
	s.Script("TRIM 0,0 ")
	pls := entities[*doc.LWPolyline](s)
	if len(pls) != 1 {
		t.Fatalf("polylines = %d", len(pls))
	}
	p := pls[0]
	if len(p.Points) != 3 || p.Closed {
		t.Fatalf("polyline = %+v", p)
	}
	near(t, "p0", p.Points[0], geom.V(5, -5))
	near(t, "p1", p.Points[1], geom.V(10, 0))
	near(t, "p2", p.Points[2], geom.V(10, 10))
	if math.Abs(p.Bulges[0]-math.Tan(math.Pi/8)) > 1e-9 || p.Bulges[1] != 0 {
		t.Fatalf("bulges = %v", p.Bulges)
	}
	if p.Layer != "WALLS" || p.Color != 3 {
		t.Fatalf("lost properties: %q %d", p.Layer, p.Color)
	}
}

func TestTrimPolylineIntoTwoPieces(t *testing.T) {
	s := newSession(t)
	pl := &doc.LWPolyline{Points: []geom.Vec2{geom.V(0, 0), geom.V(100, 0), geom.V(100, 100), geom.V(0, 100)}}
	pl.Layer = "A"
	addEntities(t, s, pl, line(geom.V(50, -10), geom.V(50, 10)), line(geom.V(50, 90), geom.V(50, 110)))
	// Cuts on the first and third segments; pick the middle one (x=100,y=50).
	s.Script("TRIM 100,50 ")
	pls := entities[*doc.LWPolyline](s)
	if len(pls) != 2 {
		t.Fatalf("polylines = %d", len(pls))
	}
	for _, p := range pls {
		if p.Layer != "A" || len(p.Points) != 2 {
			t.Fatalf("piece = %+v", p)
		}
	}
	near(t, "first end", pls[0].Points[1], geom.V(50, 0))
	near(t, "second start", pls[1].Points[0], geom.V(50, 100))
}

func TestTrimClosedPolylineBecomesOpen(t *testing.T) {
	s := newSession(t)
	sq := &doc.LWPolyline{Points: []geom.Vec2{geom.V(0, 0), geom.V(100, 0), geom.V(100, 100), geom.V(0, 100)}, Closed: true}
	addEntities(t, s, sq, line(geom.V(-10, 50), geom.V(110, 50)))
	s.Script("TRIM 50,100 ")
	pls := entities[*doc.LWPolyline](s)
	if len(pls) != 1 || pls[0].Closed || len(pls[0].Points) != 4 {
		t.Fatalf("polylines = %+v", pls)
	}
	want := []geom.Vec2{geom.V(0, 50), geom.V(0, 0), geom.V(100, 0), geom.V(100, 50)}
	for i, w := range want {
		near(t, "pt", pls[0].Points[i], w)
	}
	// The class and handle survive (the polyline is modified, not replaced).
	if pls[0].Handle() != sq.Handle() {
		t.Fatal("handle changed")
	}
}

func TestTrimCuttingEdges(t *testing.T) {
	s := newSession(t)
	addEntities(t, s, line(geom.V(0, 0), geom.V(100, 0)),
		line(geom.V(30, -10), geom.V(30, 10)), line(geom.V(70, -10), geom.V(70, 10)),
		line(geom.V(90, -10), geom.V(90, 10)))
	s.Run("TRIM")
	s.Submit("T")
	if got := s.Prompt(); got != "Select objects or <select all>:" {
		t.Fatalf("prompt = %q", got)
	}
	s.Click(geom.V(30, 8), false)
	s.Click(geom.V(70, 8), false)
	s.Submit("")
	if got := s.Prompt(); got != "Select object to trim or shift-select to extend or [cuTting edges/Crossing/mOde/Project/eRase/Undo]:" {
		t.Fatalf("prompt after edges = %q", got)
	}
	// Only x=30 and x=70 cut; picking at 80 removes 70..100 (the x=90 line is
	// not an edge).
	s.Submit("80,0")
	s.Submit("")
	ls := sortedLines(s)
	if !hasLine(ls, geom.V(0, 0), geom.V(70, 0)) {
		t.Fatalf("lines = %v", dumpLines(ls))
	}
}

func TestTrimEmptyEdgeSelectionMeansAll(t *testing.T) {
	s := newSession(t)
	addEntities(t, s, line(geom.V(0, 0), geom.V(100, 0)), line(geom.V(30, -10), geom.V(30, 10)))
	s.Script("TRIM T  90,0 ")
	if !hasLine(sortedLines(s), geom.V(0, 0), geom.V(30, 0)) {
		t.Fatalf("lines = %v", dumpLines(sortedLines(s)))
	}
}

func TestTrimIgnoresHiddenLayerEdges(t *testing.T) {
	s := newSession(t)
	s.Drawing().AddLayer("HIDE", 1).Off = true
	cut := line(geom.V(30, -10), geom.V(30, 10))
	cut.Layer = "HIDE"
	addEntities(t, s, line(geom.V(0, 0), geom.V(100, 0)), cut)
	s.Script("TRIM 90,0")
	if lastLine(s) != "Object does not intersect an edge." {
		t.Fatalf("last = %q", lastLine(s))
	}
}

func TestExtendLineToBoundary(t *testing.T) {
	s := newSession(t)
	addEntities(t, s, line(geom.V(0, 0), geom.V(10, 0)), line(geom.V(25, -10), geom.V(25, 10)), line(geom.V(40, -10), geom.V(40, 10)))
	s.Script("EXTEND 9,0 ")
	ls := sortedLines(s)
	if !hasLine(ls, geom.V(0, 0), geom.V(25, 0)) {
		t.Fatalf("lines = %v", dumpLines(ls))
	}
	s.Drawing().Undo()
	if !hasLine(sortedLines(s), geom.V(0, 0), geom.V(10, 0)) {
		t.Fatal("undo did not restore the line")
	}
}

func TestExtendStartAndMiss(t *testing.T) {
	s := newSession(t)
	addEntities(t, s, line(geom.V(0, 0), geom.V(10, 0)), line(geom.V(-15, -10), geom.V(-15, 10)))
	s.Script("EXTEND 1,0")
	if !hasLine(sortedLines(s), geom.V(-15, 0), geom.V(10, 0)) {
		t.Fatalf("lines = %v", dumpLines(sortedLines(s)))
	}
	// Now extend the far end: nothing lies beyond it.
	s.Submit("9,0")
	if got := lastLine(s); got != "No boundary edge in the extension of that end." {
		t.Fatalf("last = %q", got)
	}
}

func TestExtendArc(t *testing.T) {
	s := newSession(t)
	addEntities(t, s, &doc.Arc{Center: geom.V(0, 0), Radius: 10, StartAngle: 0, EndAngle: math.Pi / 4}, line(geom.V(0, -20), geom.V(0, 20)))
	// Pick near the end of the arc (45 degrees).
	s.Script("EXTEND 7,7 ")
	a := entities[*doc.Arc](s)[0]
	if math.Abs(a.EndAngle-math.Pi/2) > 1e-9 || a.StartAngle != 0 {
		t.Fatalf("arc = %+v", a)
	}
}

func TestExtendPolylineEndSegment(t *testing.T) {
	s := newSession(t)
	pl := &doc.LWPolyline{Points: []geom.Vec2{geom.V(0, 10), geom.V(0, 0), geom.V(10, 0)}}
	addEntities(t, s, pl, line(geom.V(30, -10), geom.V(30, 10)))
	s.Script("EXTEND 9,0 ")
	p := entities[*doc.LWPolyline](s)[0]
	near(t, "last vertex", p.Points[2], geom.V(30, 0))
	near(t, "others kept", p.Points[1], geom.V(0, 0))
}

func TestExtendClosedPolylineAndCircleRefused(t *testing.T) {
	s := newSession(t)
	addEntities(t, s, &doc.LWPolyline{Points: []geom.Vec2{geom.V(0, 0), geom.V(10, 0), geom.V(10, 10)}, Closed: true},
		&doc.Circle{Center: geom.V(100, 100), Radius: 5})
	s.Script("EXTEND 5,0")
	if got := lastLine(s); got != "Cannot extend a closed polyline." {
		t.Fatalf("last = %q", got)
	}
	s.Submit("105,100")
	if got := lastLine(s); got != "Cannot extend this object." {
		t.Fatalf("last = %q", got)
	}
}

func TestBreakLineBetweenPoints(t *testing.T) {
	s := newSession(t)
	l := line(geom.V(0, 0), geom.V(100, 0))
	l.Layer = "X"
	addEntities(t, s, l)
	s.Script("BREAK 20,0 60,0")
	if s.Active() != "" {
		t.Fatalf("active = %s", s.Active())
	}
	ls := sortedLines(s)
	if len(ls) != 2 || !hasLine(ls, geom.V(0, 0), geom.V(20, 0)) || !hasLine(ls, geom.V(60, 0), geom.V(100, 0)) {
		t.Fatalf("lines = %v", dumpLines(ls))
	}
	for _, x := range ls {
		if x.Layer != "X" {
			t.Fatalf("layer = %q", x.Layer)
		}
	}
	s.Drawing().Undo()
	if len(sortedLines(s)) != 1 {
		t.Fatal("undo")
	}
}

func TestBreakAtPointWithAt(t *testing.T) {
	s := newSession(t)
	addEntities(t, s, line(geom.V(0, 0), geom.V(100, 0)))
	s.Script("BREAK 40,0 @")
	ls := sortedLines(s)
	if len(ls) != 2 || !hasLine(ls, geom.V(0, 0), geom.V(40, 0)) || !hasLine(ls, geom.V(40, 0), geom.V(100, 0)) {
		t.Fatalf("lines = %v", dumpLines(ls))
	}
}

func TestBreakFirstPoint(t *testing.T) {
	s := newSession(t)
	addEntities(t, s, line(geom.V(0, 0), geom.V(100, 0)))
	s.Run("BREAK")
	s.Submit("10,0")
	if got := s.Prompt(); got != "Specify second break point or [First point]:" {
		t.Fatalf("prompt = %q", got)
	}
	s.Script("F 20,0 60,0")
	ls := sortedLines(s)
	if len(ls) != 2 || !hasLine(ls, geom.V(0, 0), geom.V(20, 0)) || !hasLine(ls, geom.V(60, 0), geom.V(100, 0)) {
		t.Fatalf("lines = %v", dumpLines(ls))
	}
}

func TestBreakPastTheEndAndWholeLine(t *testing.T) {
	s := newSession(t)
	addEntities(t, s, line(geom.V(0, 0), geom.V(100, 0)))
	s.Script("BREAK 80,0 150,0")
	ls := sortedLines(s)
	if len(ls) != 1 || !hasLine(ls, geom.V(0, 0), geom.V(80, 0)) {
		t.Fatalf("lines = %v", dumpLines(ls))
	}
	s.Script("BREAK 0,0 80,0")
	if len(sortedLines(s)) != 0 {
		t.Fatalf("breaking the whole line must erase it: %v", dumpLines(sortedLines(s)))
	}
}

func TestBreakCircleRemovesCounterClockwise(t *testing.T) {
	s := newSession(t)
	addEntities(t, s, &doc.Circle{Center: geom.V(0, 0), Radius: 10})
	s.Script("BREAK 10,0 0,10")
	arcs := entities[*doc.Arc](s)
	if len(arcs) != 1 || len(entities[*doc.Circle](s)) != 0 {
		t.Fatalf("arcs=%d", len(arcs))
	}
	if math.Abs(arcs[0].StartAngle-math.Pi/2) > 1e-9 || math.Abs(geom.NormAngle(arcs[0].EndAngle)) > 1e-9 {
		t.Fatalf("arc = %+v", arcs[0])
	}
	// Breaking a circle at a single point is refused.
	s2 := newSession(t)
	addEntities(t, s2, &doc.Circle{Center: geom.V(0, 0), Radius: 10})
	s2.Script("BREAK 10,0 @")
	if got := lastLine(s2); got != "Cannot break a circle at a single point." {
		t.Fatalf("last = %q", got)
	}
	if len(entities[*doc.Circle](s2)) != 1 {
		t.Fatal("circle changed")
	}
}

func TestBreakArcAndPolyline(t *testing.T) {
	s := newSession(t)
	addEntities(t, s, &doc.Arc{Center: geom.V(0, 0), Radius: 10, StartAngle: 0, EndAngle: math.Pi})
	s.Script("BREAK 7.0710678118654755,7.0710678118654755 0,10")
	if len(entities[*doc.Arc](s)) != 2 {
		t.Fatalf("arcs = %d", len(entities[*doc.Arc](s)))
	}
	p := newSession(t)
	pl := &doc.LWPolyline{Points: []geom.Vec2{geom.V(0, 0), geom.V(100, 0), geom.V(100, 100)}}
	addEntities(t, p, pl)
	p.Script("BREAK 50,0 100,50")
	pls := entities[*doc.LWPolyline](p)
	if len(pls) != 2 {
		t.Fatalf("polylines = %d", len(pls))
	}
	near(t, "first end", pls[0].Points[len(pls[0].Points)-1], geom.V(50, 0))
	near(t, "second start", pls[1].Points[0], geom.V(100, 50))
}

func TestBreakClosedPolylineAtPointOpensIt(t *testing.T) {
	s := newSession(t)
	addEntities(t, s, &doc.LWPolyline{Points: []geom.Vec2{geom.V(0, 0), geom.V(100, 0), geom.V(100, 100), geom.V(0, 100)}, Closed: true})
	s.Script("BREAK 50,0 @")
	pls := entities[*doc.LWPolyline](s)
	if len(pls) != 1 || pls[0].Closed {
		t.Fatalf("polylines = %+v", pls)
	}
	near(t, "start", pls[0].Points[0], geom.V(50, 0))
	near(t, "end", pls[0].Points[len(pls[0].Points)-1], geom.V(50, 0))
}

func selectEverything(s *cmdsys.Session) {
	for _, h := range s.Drawing().Handles() {
		s.Selection().Add(h)
	}
}

func TestJoinCollinearLines(t *testing.T) {
	s := newSession(t)
	addEntities(t, s, line(geom.V(0, 0), geom.V(10, 0)), line(geom.V(10, 0), geom.V(20, 0)), line(geom.V(15, 0), geom.V(30, 0)))
	selectEverything(s)
	s.Run("JOIN")
	if s.Active() != "" {
		t.Fatalf("active = %s", s.Active())
	}
	ls := sortedLines(s)
	if len(ls) != 1 || !hasLine(ls, geom.V(0, 0), geom.V(30, 0)) {
		t.Fatalf("lines = %v", dumpLines(ls))
	}
	if got := lastLine(s); got != "2 objects joined." {
		t.Fatalf("last = %q", got)
	}
	s.Drawing().Undo()
	if len(sortedLines(s)) != 3 {
		t.Fatal("undo did not restore the lines")
	}
}

func TestJoinPromptsSourceThenObjects(t *testing.T) {
	s := newSession(t)
	addEntities(t, s, line(geom.V(0, 0), geom.V(10, 0)), line(geom.V(10, 0), geom.V(20, 0)))
	s.Run("J")
	if got := s.Prompt(); got != "Select source object or multiple objects to join at once:" {
		t.Fatalf("prompt = %q", got)
	}
	s.Click(geom.V(2, 0), false)
	s.Submit("")
	if got := s.Prompt(); got != "Select objects to join to source:" {
		t.Fatalf("prompt = %q", got)
	}
	s.Click(geom.V(18, 0), false)
	s.Submit("")
	if s.Active() != "" || len(sortedLines(s)) != 1 {
		t.Fatalf("lines = %v active=%q", dumpLines(sortedLines(s)), s.Active())
	}
	if got := lastLine(s); got != "1 line joined to source" {
		t.Fatalf("last = %q", got)
	}
}

func TestJoinLeavesGappedAndParallelLines(t *testing.T) {
	s := newSession(t)
	addEntities(t, s, line(geom.V(0, 0), geom.V(10, 0)), line(geom.V(11, 0), geom.V(20, 0)), line(geom.V(0, 1), geom.V(10, 1)))
	selectEverything(s)
	s.Run("JOIN")
	if len(sortedLines(s)) != 3 {
		t.Fatalf("lines = %v", dumpLines(sortedLines(s)))
	}
	if got := lastLine(s); got != "0 objects joined." {
		t.Fatalf("last = %q", got)
	}
}

func TestJoinMixedChainToPolylineWithBulges(t *testing.T) {
	s := newSession(t)
	pl := &doc.LWPolyline{Points: []geom.Vec2{geom.V(0, 0), geom.V(10, 0)}}
	pl.Layer = "P"
	// Quarter arc from (10,0): centre (10,10), from 270 to 0 degrees ends at (20,10).
	arc := &doc.Arc{Center: geom.V(10, 10), Radius: 10, StartAngle: 3 * math.Pi / 2, EndAngle: 0}
	// A line from (20,10) up to (20,30), given backwards.
	ln := line(geom.V(20, 30), geom.V(20, 10))
	addEntities(t, s, pl, arc, ln)
	selectEverything(s)
	s.Run("JOIN")
	pls := entities[*doc.LWPolyline](s)
	if len(pls) != 1 || len(entities[*doc.Arc](s)) != 0 || len(sortedLines(s)) != 0 {
		t.Fatalf("pls=%d arcs=%d lines=%d", len(pls), len(entities[*doc.Arc](s)), len(sortedLines(s)))
	}
	p := pls[0]
	if p.Layer != "P" || p.Closed || len(p.Points) != 4 {
		t.Fatalf("polyline = %+v", p)
	}
	want := []geom.Vec2{geom.V(0, 0), geom.V(10, 0), geom.V(20, 10), geom.V(20, 30)}
	for i, w := range want {
		near(t, "pt", p.Points[i], w)
	}
	if p.Bulges[0] != 0 || math.Abs(p.Bulges[1]-math.Tan(math.Pi/8)) > 1e-9 || p.Bulges[2] != 0 {
		t.Fatalf("bulges = %v", p.Bulges)
	}
	if got := lastLine(s); got != "2 objects joined." {
		t.Fatalf("last = %q", got)
	}
}

func TestJoinLinesIntoClosedPolyline(t *testing.T) {
	s := newSession(t)
	addEntities(t, s, line(geom.V(0, 0), geom.V(10, 0)), line(geom.V(10, 0), geom.V(5, 8)), line(geom.V(0, 0), geom.V(5, 8)))
	selectEverything(s)
	s.Run("JOIN")
	pls := entities[*doc.LWPolyline](s)
	if len(pls) != 1 || !pls[0].Closed || len(pls[0].Points) != 3 || len(sortedLines(s)) != 0 {
		t.Fatalf("pls = %+v lines=%d", pls, len(sortedLines(s)))
	}
}

func TestJoinPolylinesEndToEnd(t *testing.T) {
	s := newSession(t)
	a := &doc.LWPolyline{Points: []geom.Vec2{geom.V(0, 0), geom.V(10, 0), geom.V(10, 10)}, Bulges: []float64{0.5, 0}}
	b := &doc.LWPolyline{Points: []geom.Vec2{geom.V(30, 10), geom.V(10, 10)}}
	addEntities(t, s, a, b)
	selectEverything(s)
	s.Run("JOIN")
	pls := entities[*doc.LWPolyline](s)
	if len(pls) != 1 || len(pls[0].Points) != 4 {
		t.Fatalf("pls = %+v", pls)
	}
	if pls[0].Handle() != a.Handle() {
		t.Fatal("source polyline must survive")
	}
	near(t, "last", pls[0].Points[3], geom.V(30, 10))
	if pls[0].Bulges[0] != 0.5 {
		t.Fatalf("bulges = %v", pls[0].Bulges)
	}
}

func TestJoinCocircularArcs(t *testing.T) {
	s := newSession(t)
	addEntities(t, s,
		&doc.Arc{Center: geom.V(0, 0), Radius: 5, StartAngle: 0, EndAngle: math.Pi / 2},
		&doc.Arc{Center: geom.V(0, 0), Radius: 5, StartAngle: math.Pi / 2, EndAngle: math.Pi},
		&doc.Arc{Center: geom.V(0, 0), Radius: 5, StartAngle: 3 * math.Pi / 4, EndAngle: 5 * math.Pi / 4})
	selectEverything(s)
	s.Run("JOIN")
	arcs := entities[*doc.Arc](s)
	if len(arcs) != 1 || math.Abs(arcs[0].StartAngle) > 1e-9 || math.Abs(arcs[0].EndAngle-5*math.Pi/4) > 1e-9 {
		t.Fatalf("arcs = %+v", arcs)
	}
}

func TestJoinArcsIntoCircle(t *testing.T) {
	s := newSession(t)
	addEntities(t, s,
		&doc.Arc{Center: geom.V(1, 2), Radius: 5, StartAngle: 0, EndAngle: math.Pi},
		&doc.Arc{Center: geom.V(1, 2), Radius: 5, StartAngle: math.Pi, EndAngle: 0})
	selectEverything(s)
	s.Run("JOIN")
	cs := entities[*doc.Circle](s)
	if len(cs) != 1 || len(entities[*doc.Arc](s)) != 0 || cs[0].Radius != 5 {
		t.Fatalf("circles = %+v", cs)
	}
}

func TestJoinSourceCannotBeCircle(t *testing.T) {
	s := newSession(t)
	addEntities(t, s, &doc.Circle{Center: geom.V(0, 0), Radius: 5}, line(geom.V(0, 0), geom.V(1, 1)))
	selectEverything(s)
	s.Run("JOIN")
	if got := lastLine(s); got != "Cannot join the source object." {
		t.Fatalf("last = %q", got)
	}
}

func TestExplodePolylineKeepsLayerAndColour(t *testing.T) {
	s := newSession(t)
	pl := &doc.LWPolyline{Points: []geom.Vec2{geom.V(0, 0), geom.V(10, 0), geom.V(10, 10), geom.V(0, 10)}, Bulges: []float64{0, 1, 0, 0}}
	pl.Layer = "WALLS"
	pl.Color = 5
	addEntities(t, s, pl)
	s.Selection().Add(pl.Handle())
	s.Run("EXPLODE")
	if s.Active() != "" {
		t.Fatalf("active = %s", s.Active())
	}
	ls, arcs := sortedLines(s), entities[*doc.Arc](s)
	if len(ls) != 2 || len(arcs) != 1 || len(entities[*doc.LWPolyline](s)) != 0 {
		t.Fatalf("lines=%d arcs=%d", len(ls), len(arcs))
	}
	for _, e := range []doc.Entity{ls[0], ls[1], arcs[0]} {
		if e.Common().Layer != "WALLS" || e.Common().Color != 5 {
			t.Fatalf("lost properties: %+v", e.Common())
		}
	}
	a := arcs[0]
	// Bulge 1 from (10,0) to (10,10): a half circle on the right, centre (10,5).
	near(t, "centre", a.Center, geom.V(10, 5))
	if math.Abs(a.Radius-5) > 1e-9 || math.Abs(geom.NormAngle(a.StartAngle)-3*math.Pi/2) > 1e-9 || math.Abs(a.EndAngle-math.Pi/2) > 1e-9 {
		t.Fatalf("arc = %+v", a)
	}
	if got := lastLine(s); got != "1 object exploded." {
		t.Fatalf("last = %q", got)
	}
	s.Drawing().Undo()
	if len(entities[*doc.LWPolyline](s)) != 1 || len(sortedLines(s)) != 0 || len(entities[*doc.Arc](s)) != 0 {
		t.Fatal("undo did not restore the polyline")
	}
}

func TestExplodeClosedPolylineAndNegativeBulge(t *testing.T) {
	s2 := newSession(t)
	addEntities(t, s2, &doc.LWPolyline{Points: []geom.Vec2{geom.V(0, 0), geom.V(10, 0), geom.V(10, 10)}, Bulges: []float64{0, 0, -0.5}, Closed: true})
	// Selecting by click at the selection prompt, then Enter.
	s2.Script("X")
	s2.Click(geom.V(5, 0), false)
	s2.Submit("")
	if len(sortedLines(s2)) != 2 || len(entities[*doc.Arc](s2)) != 1 {
		t.Fatalf("lines=%d arcs=%d", len(sortedLines(s2)), len(entities[*doc.Arc](s2)))
	}
	a := entities[*doc.Arc](s2)[0]
	// The closing segment runs (10,10)->(0,0) clockwise, so it is stored
	// counter-clockwise from (0,0) to (10,10).
	sp, ep := geom.V(a.Center.X+a.Radius*math.Cos(a.StartAngle), a.Center.Y+a.Radius*math.Sin(a.StartAngle)), geom.V(a.Center.X+a.Radius*math.Cos(a.EndAngle), a.Center.Y+a.Radius*math.Sin(a.EndAngle))
	near(t, "arc start", sp, geom.V(0, 0))
	near(t, "arc end", ep, geom.V(10, 10))
}

func TestExplodeRefusesOtherObjects(t *testing.T) {
	s := newSession(t)
	addEntities(t, s, line(geom.V(0, 0), geom.V(1, 1)), &doc.Circle{Center: geom.V(5, 5), Radius: 1})
	selectEverything(s)
	s.Run("EXPLODE")
	h := s.History()
	if h[len(h)-2] != "2 objects cannot be exploded." || h[len(h)-1] != "0 objects exploded." {
		t.Fatalf("history tail = %q", h[len(h)-2:])
	}
	s.Drawing().Undo()
	if s.Drawing().Len() != 0 {
		t.Fatal("a refused EXPLODE left an undo step")
	}
}

func TestTrimGridLeavesCells(t *testing.T) {
	// A 3x3 grid of lines trimmed down to the inner square.
	s := newSession(t)
	var es []doc.Entity
	for i := 0; i < 4; i++ {
		v := float64(i) * 10
		es = append(es, line(geom.V(-10, v), geom.V(40, v)), line(geom.V(v, -10), geom.V(v, 40)))
	}
	addEntities(t, s, es...)
	for _, y := range []float64{0, 30} {
		s.Script("TRIM -5," + cmdsys.FormatNumber(y) + " 35," + cmdsys.FormatNumber(y) + " ")
	}
	for _, x := range []float64{0, 30} {
		s.Script("TRIM " + cmdsys.FormatNumber(x) + ",-5 " + cmdsys.FormatNumber(x) + ",35 ")
	}
	ls := sortedLines(s)
	if len(ls) != 8 {
		t.Fatalf("lines = %v", dumpLines(ls))
	}
	if !hasLine(ls, geom.V(0, 0), geom.V(30, 0)) || !hasLine(ls, geom.V(0, 0), geom.V(0, 30)) {
		t.Fatalf("lines = %v", dumpLines(ls))
	}
}
