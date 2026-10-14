package commands_test

import (
	"math"
	"strings"
	"testing"

	"cad-lite/cmdsys"
	"cad-lite/doc"
	"cad-lite/geom"
	"cad-lite/render"
)

// previewItems returns how many display items the active command's preview
// draws with the cursor at p.
func previewItems(s *cmdsys.Session, p geom.Vec2) int {
	s.Hover(p)
	var dl render.DisplayList
	s.Overlay(&dl)
	return len(dl.Items)
}

func history(s *cmdsys.Session) string { return strings.Join(s.History(), "\n") }

func selectAll(s *cmdsys.Session) {
	s.Selection().Add(s.Drawing().Handles()...)
}

func snapshotGeometry(s *cmdsys.Session) string {
	var b strings.Builder
	for _, h := range s.Drawing().Handles() {
		e := s.Drawing().Entity(h)
		b.WriteString(e.Class())
		for _, c := range e.Geometry() {
			b.WriteString(" " + cmdsys.FormatPoint(c.Bounds().Min))
		}
		b.WriteString("\n")
	}
	return b.String()
}

func TestMirrorCopiesAndKeepsSource(t *testing.T) {
	s := newSession(t)
	addEntities(t, s,
		&doc.Line{EntityCommon: doc.EntityCommon{Layer: "0"}, Start: geom.V(1, 1), End: geom.V(3, 2)},
		&doc.Arc{Center: geom.V(5, 5), Radius: 2, StartAngle: 0, EndAngle: math.Pi / 2},
	)
	selectAll(s)
	s.Script("MI 0,0 10,0 ")
	if s.Active() != "" {
		t.Fatalf("still active: %s", s.Active())
	}
	if s.Drawing().Len() != 4 {
		t.Fatalf("entities = %d, want 4\n%s", s.Drawing().Len(), history(s))
	}
	lines := entities[*doc.Line](s)
	near(t, "mirrored start", lines[1].Start, geom.V(1, -1))
	near(t, "mirrored end", lines[1].End, geom.V(3, -2))
	arcs := entities[*doc.Arc](s)
	a := arcs[1]
	near(t, "arc centre", a.Center, geom.V(5, -5))
	// A mirrored counter-clockwise arc stays counter-clockwise: 0..90 becomes 270..360.
	if e := math.Mod(a.EndAngle, 2*math.Pi); math.Abs(a.StartAngle-3*math.Pi/2) > 1e-9 || (math.Abs(e) > 1e-9 && math.Abs(e-2*math.Pi) > 1e-9) {
		t.Fatalf("mirrored arc angles = %v..%v", a.StartAngle, a.EndAngle)
	}
	s.Submit("U")
	if s.Drawing().Len() != 2 {
		t.Fatalf("after undo: %d entities, want 2", s.Drawing().Len())
	}
}

func TestMirrorEraseSourceAndDiagonalLine(t *testing.T) {
	s := newSession(t)
	addEntities(t, s, &doc.Line{Start: geom.V(4, 0), End: geom.V(6, 0)})
	selectAll(s)
	s.Script("MIRROR 0,0 1,1 Y")
	if s.Drawing().Len() != 1 {
		t.Fatalf("entities = %d, want 1 after erasing source", s.Drawing().Len())
	}
	l := entities[*doc.Line](s)[0]
	near(t, "start", l.Start, geom.V(0, 4))
	near(t, "end", l.End, geom.V(0, 6))
	s.Submit("U")
	if l := entities[*doc.Line](s); len(l) != 1 || !l[0].Start.Near(geom.V(4, 0), 1e-9) {
		t.Fatalf("undo did not restore the source: %+v", l)
	}
}

func TestMirrorPolylineBulgesAndCircle(t *testing.T) {
	s := newSession(t)
	addEntities(t, s,
		&doc.LWPolyline{Points: []geom.Vec2{geom.V(0, 1), geom.V(4, 1)}, Bulges: []float64{0.5, 0}},
		&doc.Circle{Center: geom.V(2, 3), Radius: 1},
	)
	selectAll(s)
	s.Script("MIRROR 0,0 1,0 N")
	pl := entities[*doc.LWPolyline](s)
	if len(pl) != 2 || pl[1].Bulges[0] != -0.5 {
		t.Fatalf("mirrored bulge = %+v", pl)
	}
	near(t, "poly pt", pl[1].Points[0], geom.V(0, -1))
	near(t, "circle", entities[*doc.Circle](s)[1].Center, geom.V(2, -3))
}

func TestMirrorPromptsAndErrors(t *testing.T) {
	s := newSession(t)
	addEntities(t, s, &doc.Line{Start: geom.V(1, 1), End: geom.V(2, 1)})
	s.Submit("MIRROR")
	if got := s.Prompt(); !strings.HasPrefix(got, "Select objects") {
		t.Fatalf("prompt = %q", got)
	}
	selectAll(s)
	s.Submit("")
	s.Submit("0,0")
	if got := s.Prompt(); !strings.HasPrefix(got, "Specify second point of mirror line") {
		t.Fatalf("prompt = %q", got)
	}
	s.Submit("0,0")
	if !strings.Contains(history(s), "must differ") || !strings.HasPrefix(s.Prompt(), "Specify second point") {
		t.Fatalf("coincident points not rejected: %q\n%s", s.Prompt(), history(s))
	}
	s.Submit("5,0")
	if got := s.Prompt(); !strings.Contains(got, "Erase source objects? [Yes/No] <N>") {
		t.Fatalf("prompt = %q", got)
	}
	s.Escape()
	if s.Drawing().Len() != 1 {
		t.Fatalf("a cancelled MIRROR changed the drawing: %d entities", s.Drawing().Len())
	}
	// Empty selection ends the command.
	s.Script("MIRROR ")
	if s.Active() != "" {
		t.Fatalf("empty selection left %s active", s.Active())
	}
}

func TestMirrorPreview(t *testing.T) {
	s := newSession(t)
	addEntities(t, s, &doc.Line{Start: geom.V(1, 1), End: geom.V(2, 1)})
	selectAll(s)
	s.Script("MIRROR 0,0")
	if n := previewItems(s, geom.V(5, 0)); n < 2 {
		t.Fatalf("mirror preview items = %d, want the line and the mirrored copy", n)
	}
	s.Escape()
}

func TestOffsetLineBothSidesAndLoop(t *testing.T) {
	s := newSession(t)
	addEntities(t, s, &doc.Line{EntityCommon: doc.EntityCommon{Layer: "walls"}, Start: geom.V(0, 0), End: geom.V(100, 0)})
	s.Script("O 10 50,0 50,40 50,0 50,-40 ")
	if s.Active() != "" {
		t.Fatalf("OFFSET still active: %s", s.Active())
	}
	ls := entities[*doc.Line](s)
	if len(ls) != 3 {
		t.Fatalf("lines = %d, want 3\n%s", len(ls), history(s))
	}
	near(t, "above", ls[1].Start, geom.V(0, 10))
	near(t, "above end", ls[1].End, geom.V(100, 10))
	near(t, "below", ls[2].Start, geom.V(0, -10))
	if ls[1].Layer != "walls" {
		t.Fatalf("offset layer = %q, want the source layer", ls[1].Layer)
	}
	s.Submit("U")
	if s.Drawing().Len() != 1 {
		t.Fatalf("OFFSET with two results is not one undo step: %d entities", s.Drawing().Len())
	}
}

func TestOffsetRemembersLastDistanceAndPrompts(t *testing.T) {
	s := newSession(t)
	addEntities(t, s, &doc.Line{Start: geom.V(0, 0), End: geom.V(100, 0)})
	s.Script("OFFSET 7 50,0 50,5 ")
	s.Submit("OFFSET")
	if got := s.Prompt(); got != "Specify offset distance or [Through/Erase/Layer] <7>:" {
		t.Fatalf("prompt = %q", got)
	}
	s.Submit("")
	if got := s.Prompt(); got != "Select object to offset or [Exit/Undo] <Exit>:" {
		t.Fatalf("prompt = %q", got)
	}
	s.Submit("50,0")
	if got := s.Prompt(); got != "Specify point on side to offset or [Exit/Multiple/Undo] <Exit>:" {
		t.Fatalf("prompt = %q", got)
	}
	s.Submit("50,9")
	ls := entities[*doc.Line](s)
	near(t, "default distance used", ls[len(ls)-1].Start, geom.V(0, 7))
	s.Submit("")
	if s.Active() != "" {
		t.Fatalf("Enter at the select prompt did not exit: %s", s.Active())
	}
}

func TestOffsetThrough(t *testing.T) {
	s := newSession(t)
	addEntities(t, s, &doc.Line{Start: geom.V(0, 0), End: geom.V(100, 0)}, &doc.Circle{Center: geom.V(200, 0), Radius: 10})
	s.Script("OFFSET T 50,0 30,23 200,10 200,-35 ")
	ls := entities[*doc.Line](s)
	near(t, "through line", ls[1].Start, geom.V(0, 23))
	cs := entities[*doc.Circle](s)
	if len(cs) != 2 || math.Abs(cs[1].Radius-35) > 1e-9 {
		t.Fatalf("circles = %+v", cs)
	}
}

func TestOffsetTwoPointDistance(t *testing.T) {
	s := newSession(t)
	addEntities(t, s, &doc.Line{Start: geom.V(0, 0), End: geom.V(100, 0)})
	s.Script("OFFSET 0,0 0,4 50,0 50,9 ")
	near(t, "two-point distance", entities[*doc.Line](s)[1].Start, geom.V(0, 4))
}

func TestOffsetCircleAndArc(t *testing.T) {
	s := newSession(t)
	addEntities(t, s,
		&doc.Circle{Center: geom.V(0, 0), Radius: 10},
		&doc.Arc{Center: geom.V(100, 0), Radius: 10, StartAngle: 0, EndAngle: math.Pi},
	)
	s.Script("OFFSET 3 10,0 20,0 -10,0 1,0 100,10 100,20 110,0 100,1 ")
	cs := entities[*doc.Circle](s)
	if len(cs) != 3 || cs[1].Radius != 13 || cs[2].Radius != 7 {
		t.Fatalf("circles = %+v\n%s", cs, history(s))
	}
	as := entities[*doc.Arc](s)
	if len(as) != 3 || as[1].Radius != 13 || as[2].Radius != 7 || as[1].StartAngle != 0 || math.Abs(as[1].EndAngle-math.Pi) > 1e-12 {
		t.Fatalf("arcs = %+v\n%s", as, history(s))
	}
}

func TestOffsetPolylineClosedAndOpen(t *testing.T) {
	s := newSession(t)
	addEntities(t, s,
		&doc.LWPolyline{Points: []geom.Vec2{geom.V(0, 0), geom.V(10, 0), geom.V(10, 10), geom.V(0, 10)}, Closed: true},
		&doc.LWPolyline{Points: []geom.Vec2{geom.V(50, 0), geom.V(60, 0), geom.V(60, 10)}},
	)
	// Inside the square, then outside the open L (the outer side of its corner).
	s.Script("OFFSET 2 5,0 5,5 60,5 70,-5 ")
	ps := entities[*doc.LWPolyline](s)
	if len(ps) != 4 {
		t.Fatalf("polylines = %d\n%s", len(ps), history(s))
	}
	if !ps[2].Closed {
		t.Fatal("offset of a closed polyline is not closed")
	}
	for i, w := range []geom.Vec2{geom.V(2, 2), geom.V(8, 2), geom.V(8, 8), geom.V(2, 8)} {
		near(t, "inner", ps[2].Points[i], w)
	}
	for i, w := range []geom.Vec2{geom.V(50, -2), geom.V(62, -2), geom.V(62, 10)} {
		near(t, "open", ps[3].Points[i], w)
	}
}

func TestOffsetPolylineWithBulge(t *testing.T) {
	s := newSession(t)
	addEntities(t, s, &doc.LWPolyline{Points: []geom.Vec2{geom.V(0, 0), geom.V(10, 0)}, Bulges: []float64{1, 0}})
	// The semicircle bulges below the chord (centre (5,0), radius 5).
	s.Script("OFFSET 1 5,-5 5,-3 ")
	ps := entities[*doc.LWPolyline](s)
	if len(ps) != 2 {
		t.Fatalf("polylines = %d\n%s", len(ps), history(s))
	}
	near(t, "start", ps[1].Points[0], geom.V(1, 0))
	near(t, "end", ps[1].Points[1], geom.V(9, 0))
	if math.Abs(ps[1].Bulges[0]-1) > 1e-9 {
		t.Fatalf("bulge = %v", ps[1].Bulges)
	}
}

func TestOffsetErrors(t *testing.T) {
	s := newSession(t)
	addEntities(t, s,
		&doc.Circle{Center: geom.V(0, 0), Radius: 5},
		&doc.Line{Start: geom.V(100, 0), End: geom.V(200, 0)},
	)
	s.Script("OFFSET 0")
	if !strings.Contains(history(s), "positive") || !strings.HasPrefix(s.Prompt(), "Specify offset distance") {
		t.Fatalf("zero distance accepted: %q\n%s", s.Prompt(), history(s))
	}
	s.Submit("5")
	s.Submit("500,500")
	if !strings.Contains(history(s), "Nothing found") {
		t.Fatalf("missing nothing-found message\n%s", history(s))
	}
	// Inward by the full radius: degenerate, no entity, command keeps asking.
	s.Submit("5,0")
	s.Submit("1,0")
	if !strings.Contains(history(s), "zero or negative radius") {
		t.Fatalf("missing degenerate message\n%s", history(s))
	}
	if s.Drawing().Len() != 2 || !strings.HasPrefix(s.Prompt(), "Select object to offset") {
		t.Fatalf("after degenerate offset: %d entities, prompt %q", s.Drawing().Len(), s.Prompt())
	}
	s.Submit("")
	if s.Active() != "" || s.Drawing().Len() != 2 {
		t.Fatalf("active %q, entities %d", s.Active(), s.Drawing().Len())
	}
}

func TestOffsetPreview(t *testing.T) {
	s := newSession(t)
	addEntities(t, s, &doc.Line{Start: geom.V(0, 0), End: geom.V(100, 0)})
	s.Script("OFFSET 5 50,0")
	if n := previewItems(s, geom.V(50, 30)); n == 0 {
		t.Fatal("no offset preview")
	}
	s.Escape()
}

func lineEnds(l *doc.Line) [2]geom.Vec2 { return [2]geom.Vec2{l.Start, l.End} }

func TestFilletTwoLinesRadius(t *testing.T) {
	s := newSession(t)
	addEntities(t, s,
		&doc.Line{EntityCommon: doc.EntityCommon{Layer: "a"}, Start: geom.V(0, 0), End: geom.V(100, 0)},
		&doc.Line{Start: geom.V(0, 0), End: geom.V(0, 100)},
	)
	s.Script("FILLET R 10 50,0 0,50")
	if s.Active() != "" {
		t.Fatalf("still active %s\n%s", s.Active(), history(s))
	}
	ls := entities[*doc.Line](s)
	near(t, "l1 start", ls[0].Start, geom.V(10, 0))
	near(t, "l1 end", ls[0].End, geom.V(100, 0))
	near(t, "l2 start", ls[1].Start, geom.V(0, 10))
	near(t, "l2 end", ls[1].End, geom.V(0, 100))
	as := entities[*doc.Arc](s)
	if len(as) != 1 {
		t.Fatalf("arcs = %d\n%s", len(as), history(s))
	}
	near(t, "arc centre", as[0].Center, geom.V(10, 10))
	if as[0].Radius != 10 || as[0].Layer != "a" {
		t.Fatalf("arc = %+v", as[0])
	}
	if sw := geom.NormAngle(as[0].EndAngle - as[0].StartAngle); math.Abs(sw-math.Pi/2) > 1e-9 {
		t.Fatalf("sweep = %v", sw)
	}
	s.Submit("U")
	ls = entities[*doc.Line](s)
	if len(entities[*doc.Arc](s)) != 0 || lineEnds(ls[0]) != [2]geom.Vec2{geom.V(0, 0), geom.V(100, 0)} {
		t.Fatalf("undo did not restore the lines: %+v", ls)
	}
}

func TestFilletRemembersRadiusAndZeroTrims(t *testing.T) {
	s := newSession(t)
	addEntities(t, s,
		&doc.Line{Start: geom.V(0, 0), End: geom.V(40, 0)},
		&doc.Line{Start: geom.V(50, -10), End: geom.V(50, 40)},
	)
	s.Script("F R 0")
	if s.Active() != "FILLET" {
		t.Fatalf("FILLET ended after setting the radius: %q", s.Active())
	}
	s.Script("30,0 50,30")
	ls := entities[*doc.Line](s)
	near(t, "extended", ls[0].End, geom.V(50, 0))
	near(t, "trimmed", ls[1].Start, geom.V(50, 0))
	near(t, "kept end", ls[1].End, geom.V(50, 40))
	if len(entities[*doc.Arc](s)) != 0 {
		t.Fatal("radius 0 must not draw an arc")
	}
	// The radius is remembered by the next FILLET.
	s.Submit("FILLET")
	if !strings.Contains(history(s), "Radius = 0") {
		t.Fatalf("settings line missing\n%s", history(s))
	}
	s.Escape()
}

func TestFilletPickSideSelectsKeptPart(t *testing.T) {
	s := newSession(t)
	addEntities(t, s,
		&doc.Line{Start: geom.V(-100, 0), End: geom.V(100, 0)},
		&doc.Line{Start: geom.V(0, -100), End: geom.V(0, 100)},
	)
	s.Script("FILLET R 5 -50,0 0,-50")
	ls := entities[*doc.Line](s)
	near(t, "h kept", ls[0].Start, geom.V(-100, 0))
	near(t, "h cut", ls[0].End, geom.V(-5, 0))
	near(t, "v kept", ls[1].Start, geom.V(0, -100))
	near(t, "v cut", ls[1].End, geom.V(0, -5))
	near(t, "arc centre", entities[*doc.Arc](s)[0].Center, geom.V(-5, -5))
}

func TestFilletErrors(t *testing.T) {
	s := newSession(t)
	addEntities(t, s,
		&doc.Line{Start: geom.V(0, 0), End: geom.V(100, 0)},
		&doc.Line{Start: geom.V(0, 10), End: geom.V(100, 10)},
		&doc.Line{Start: geom.V(0, 0), End: geom.V(0, 10)},
		&doc.Circle{Center: geom.V(300, 300), Radius: 5},
	)
	before := snapshotGeometry(s)
	s.Script("FILLET R 1 50,0 50,10")
	if !strings.Contains(history(s), "parallel") || s.Active() != "" {
		t.Fatalf("parallel not reported / still active\n%s", history(s))
	}
	s.Script("FILLET R 50 50,0 0,5")
	if !strings.Contains(history(s), "too large") || s.Active() != "" {
		t.Fatalf("radius too large not reported\n%s", history(s))
	}
	s.Script("FILLET R 1 305,300")
	if !strings.Contains(history(s), "Fillet supports two lines.") || s.Active() != "" {
		t.Fatalf("non-line first object\n%s", history(s))
	}
	s.Script("FILLET 50,0 305,300")
	if !strings.Contains(history(s), "Fillet supports two lines.") || s.Active() != "" {
		t.Fatalf("non-line second object\n%s", history(s))
	}
	s.Script("FILLET 50,0 50,0")
	if !strings.Contains(history(s), "different") || s.Active() != "FILLET" {
		t.Fatalf("same object twice; active %q\n%s", s.Active(), history(s))
	}
	s.Escape()
	s.Script("FILLET 900,900")
	if !strings.Contains(history(s), "Nothing found") || s.Active() != "FILLET" {
		t.Fatalf("empty pick; active %q", s.Active())
	}
	s.Submit("R")
	s.Submit("-3")
	if !strings.Contains(history(s), "non-negative") {
		t.Fatalf("negative radius accepted\n%s", history(s))
	}
	s.Escape()
	if got := snapshotGeometry(s); got != before {
		t.Fatalf("an error changed the drawing:\n%s\nwas\n%s", got, before)
	}
}

func TestFilletPreview(t *testing.T) {
	s := newSession(t)
	addEntities(t, s,
		&doc.Line{Start: geom.V(0, 0), End: geom.V(100, 0)},
		&doc.Line{Start: geom.V(0, 0), End: geom.V(0, 100)},
	)
	s.Script("FILLET R 10 50,0")
	if n := previewItems(s, geom.V(0, 50)); n < 3 {
		t.Fatalf("fillet preview items = %d, want two lines and an arc", n)
	}
	s.Escape()
}

func TestChamferTwoLines(t *testing.T) {
	s := newSession(t)
	addEntities(t, s,
		&doc.Line{EntityCommon: doc.EntityCommon{Layer: "a"}, Start: geom.V(0, 0), End: geom.V(100, 0)},
		&doc.Line{Start: geom.V(0, 0), End: geom.V(0, 100)},
	)
	s.Script("CHA D 10 20")
	if s.Active() != "CHAMFER" {
		t.Fatalf("CHAMFER ended after setting distances: %q\n%s", s.Active(), history(s))
	}
	s.Script("50,0 0,50")
	if s.Active() != "" {
		t.Fatalf("still active: %s", s.Active())
	}
	ls := entities[*doc.Line](s)
	if len(ls) != 3 {
		t.Fatalf("lines = %d\n%s", len(ls), history(s))
	}
	near(t, "l1", ls[0].Start, geom.V(10, 0))
	near(t, "l2", ls[1].Start, geom.V(0, 20))
	near(t, "bevel a", ls[2].Start, geom.V(10, 0))
	near(t, "bevel b", ls[2].End, geom.V(0, 20))
	if ls[2].Layer != "a" {
		t.Fatalf("bevel layer = %q", ls[2].Layer)
	}
	s.Submit("U")
	if s.Drawing().Len() != 2 {
		t.Fatalf("undo: %d entities", s.Drawing().Len())
	}
	near(t, "restored", entities[*doc.Line](s)[0].Start, geom.V(0, 0))
}

func TestChamferDefaultsAndSecondDistanceDefault(t *testing.T) {
	s := newSession(t)
	addEntities(t, s,
		&doc.Line{Start: geom.V(0, 0), End: geom.V(100, 0)},
		&doc.Line{Start: geom.V(0, 0), End: geom.V(0, 100)},
	)
	s.Script("CHAMFER D 15")
	if got := s.Prompt(); got != "Specify second chamfer distance <15>:" {
		t.Fatalf("prompt = %q", got)
	}
	s.Submit("")
	if got := s.Prompt(); got != "Select first line or [Undo/Polyline/Distance/Angle/Trim/mEthod/Multiple]:" {
		t.Fatalf("prompt = %q", got)
	}
	s.Script("50,0 0,50")
	ls := entities[*doc.Line](s)
	near(t, "bevel a", ls[2].Start, geom.V(15, 0))
	near(t, "bevel b", ls[2].End, geom.V(0, 15))
}

func TestChamferSharpAndErrors(t *testing.T) {
	s := newSession(t)
	addEntities(t, s,
		&doc.Line{Start: geom.V(0, 0), End: geom.V(40, 0)},
		&doc.Line{Start: geom.V(50, -10), End: geom.V(50, 40)},
		&doc.Circle{Center: geom.V(300, 300), Radius: 5},
	)
	s.Script("CHAMFER D 0 0 30,0 50,30")
	ls := entities[*doc.Line](s)
	near(t, "extended", ls[0].End, geom.V(50, 0))
	near(t, "trimmed", ls[1].Start, geom.V(50, 0))
	if len(ls) != 2 {
		t.Fatalf("zero distances added a bevel: %d lines", len(ls))
	}
	s.Script("CHAMFER D 500 500 30,0 50,30")
	if !strings.Contains(history(s), "too large") {
		t.Fatalf("missing too-large message\n%s", history(s))
	}
	s.Script("CHAMFER 30,0 305,300")
	if !strings.Contains(history(s), "Chamfer supports two lines.") || s.Active() != "" {
		t.Fatalf("non-line second object\n%s", history(s))
	}
	s.Script("CHAMFER D -1")
	if !strings.Contains(history(s), "non-negative") {
		t.Fatalf("negative distance accepted\n%s", history(s))
	}
	s.Escape()
}

func TestChamferPreview(t *testing.T) {
	s := newSession(t)
	addEntities(t, s,
		&doc.Line{Start: geom.V(0, 0), End: geom.V(100, 0)},
		&doc.Line{Start: geom.V(0, 0), End: geom.V(0, 100)},
	)
	s.Script("CHAMFER D 10 10 50,0")
	if n := previewItems(s, geom.V(0, 50)); n < 3 {
		t.Fatalf("chamfer preview items = %d, want two lines and a bevel", n)
	}
	s.Escape()
}
