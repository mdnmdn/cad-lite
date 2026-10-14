package commands_test

import (
	"math"
	"strings"
	"testing"

	"go-draw/cmdsys"
	"go-draw/cmdsys/commands"
	"go-draw/doc"
	"go-draw/geom"
	"go-draw/render"
)

func newSession(t *testing.T) *cmdsys.Session {
	t.Helper()
	v := render.DefaultView(800, 600)
	return cmdsys.NewSession(doc.New(), &v, commands.NewRegistry())
}

func entities[T doc.Entity](s *cmdsys.Session) []T {
	var out []T
	for _, h := range s.Drawing().Handles() {
		if e, ok := s.Drawing().Entity(h).(T); ok {
			out = append(out, e)
		}
	}
	return out
}

func near(t *testing.T, name string, got, want geom.Vec2) {
	t.Helper()
	if !got.Near(want, 1e-9) {
		t.Fatalf("%s = %v, want %v", name, got, want)
	}
}

func TestParseInputForms(t *testing.T) {
	base := geom.V(10, 10)
	p := &cmdsys.Prompt{Accepts: cmdsys.InputPoint | cmdsys.InputKeyword | cmdsys.InputEnter, Keywords: []string{"Close", "Undo"}}
	cases := []struct {
		in   string
		want geom.Vec2
	}{
		{"3,4", geom.V(3, 4)},
		{"@3,4", geom.V(13, 14)},
		{"@5<90", geom.V(10, 15)},
		{"2<0", geom.V(2, 0)},
		{"5", geom.V(15, 10)},
	}
	for _, c := range cases {
		in, err := cmdsys.ParseInput(p, c.in, &base, geom.V(20, 10))
		if err != nil || in.Kind != cmdsys.InputPoint {
			t.Fatalf("%q: %+v, %v", c.in, in, err)
		}
		near(t, c.in, in.Point, c.want)
	}
	if in, _ := cmdsys.ParseInput(p, "c", &base, base); in.Keyword != "Close" {
		t.Fatalf("shortcut c = %+v", in)
	}
	if in, _ := cmdsys.ParseInput(p, "", &base, base); in.Kind != cmdsys.InputEnter {
		t.Fatalf("empty = %+v", in)
	}
	if _, err := cmdsys.ParseInput(p, "@1,1", nil, base); err != cmdsys.ErrNoBase {
		t.Fatalf("relative without base err = %v", err)
	}
	if _, err := cmdsys.ParseInput(p, "1,x", &base, base); err != cmdsys.ErrInvalidPoint {
		t.Fatalf("bad point err = %v", err)
	}
	dist := &cmdsys.Prompt{Accepts: cmdsys.InputDistance | cmdsys.InputPoint, Base: &base}
	if in, _ := cmdsys.ParseInput(dist, "7", &base, base); in.Kind != cmdsys.InputDistance || in.Value != 7 {
		t.Fatalf("bare number at a distance prompt = %+v", in)
	}
	ang := &cmdsys.Prompt{Accepts: cmdsys.InputAngle}
	if in, _ := cmdsys.ParseInput(ang, "90", nil, base); math.Abs(in.Value-math.Pi/2) > 1e-12 {
		t.Fatalf("angle = %+v", in)
	}
}

func TestLineScriptIsOneUndoStep(t *testing.T) {
	s := newSession(t)
	s.Script("LINE 0,0 100,0 @0,50 c")
	lines := entities[*doc.Line](s)
	if len(lines) != 3 {
		t.Fatalf("lines = %d, want 3; history:\n%s", len(lines), strings.Join(s.History(), "\n"))
	}
	near(t, "relative point", lines[1].End, geom.V(100, 50))
	near(t, "close", lines[2].End, geom.V(0, 0))
	if s.Active() != "" {
		t.Fatalf("command still active: %s", s.Active())
	}
	s.Submit("U")
	if n := s.Drawing().Len(); n != 0 {
		t.Fatalf("after U: %d entities, want 0 (one command is one undo step)", n)
	}
	s.Submit("REDO")
	if n := s.Drawing().Len(); n != 3 {
		t.Fatalf("after REDO: %d entities, want 3", n)
	}
}

func TestLineUndoKeywordAndEnterRepeats(t *testing.T) {
	s := newSession(t)
	s.Script("L 0,0 10,0 20,0 u ")
	if n := len(entities[*doc.Line](s)); n != 1 {
		t.Fatalf("after Undo keyword: %d lines, want 1", n)
	}
	s.Submit("")
	if s.Active() != "LINE" {
		t.Fatalf("Enter at Command: did not repeat LINE (active %q)", s.Active())
	}
	s.Escape()
}

func TestCircleArcRectPline(t *testing.T) {
	s := newSession(t)
	s.Script("CIRCLE 0,0 5 C 10,0 d 4 ARC 1,0 0,1 -1,0 REC 0,0 4,3 PL 0,0 1,0 1,1 c")
	cs := entities[*doc.Circle](s)
	if len(cs) != 2 || cs[0].Radius != 5 || cs[1].Radius != 2 {
		t.Fatalf("circles = %+v", cs)
	}
	arcs := entities[*doc.Arc](s)
	if len(arcs) != 1 || math.Abs(arcs[0].Radius-1) > 1e-9 {
		t.Fatalf("arcs = %+v", arcs)
	}
	ps := entities[*doc.LWPolyline](s)
	if len(ps) != 2 || !ps[0].Closed || len(ps[0].Points) != 4 || !ps[1].Closed || len(ps[1].Points) != 3 {
		t.Fatalf("polylines = %+v", ps)
	}
}

func TestEscapeKeepsCommittedSegments(t *testing.T) {
	s := newSession(t)
	s.Script("LINE 0,0 1,0 2,0")
	s.Escape()
	if n := s.Drawing().Len(); n != 2 || s.Active() != "" {
		t.Fatalf("after Escape: %d entities, active %q", n, s.Active())
	}
	if h := s.History(); h[len(h)-1] != "*Cancel*" {
		t.Fatalf("last history line = %q", h[len(h)-1])
	}
}

func TestClickSnapsToEndpointAndOrthoConstrains(t *testing.T) {
	s := newSession(t)
	s.Script("LINE 0,0 100,0 ")
	s.Run("LINE")
	// The view is 1 px per unit; 3 units from the endpoint is inside the
	// 8 px aperture.
	s.Click(geom.V(98, 3), false)
	s.Ortho = true
	s.Click(geom.V(150, 40), false)
	s.Escape()
	lines := entities[*doc.Line](s)
	if len(lines) != 2 {
		t.Fatalf("lines = %d", len(lines))
	}
	near(t, "snapped start", lines[1].Start, geom.V(100, 0))
	near(t, "ortho end", lines[1].End, geom.V(150, 0))
}

func TestSnapModes(t *testing.T) {
	s := newSession(t)
	s.Script("LINE 0,0 80,0  LINE 50,-40 50,60  CIRCLE 200,0 20")
	d, ix := s.Drawing(), s.Index()
	cases := []struct {
		at   geom.Vec2
		mode cmdsys.SnapMode
		want geom.Vec2
	}{
		{geom.V(2, 1), cmdsys.SnapEndpoint, geom.V(0, 0)},
		{geom.V(49, 2), cmdsys.SnapIntersection, geom.V(50, 0)},
		{geom.V(52, 58), cmdsys.SnapEndpoint, geom.V(50, 60)},
		{geom.V(201, 1), cmdsys.SnapCenter, geom.V(200, 0)},
		{geom.V(219, 2), cmdsys.SnapQuadrant, geom.V(220, 0)},
		{geom.V(41, 2), cmdsys.SnapMidpoint, geom.V(40, 0)},
	}
	for _, c := range cases {
		hit, ok := cmdsys.FindSnap(d, ix, c.at, 8, cmdsys.DefaultSnaps)
		if !ok || hit.Mode != c.mode {
			t.Fatalf("at %v: %+v %v, want %v", c.at, hit, ok, c.mode)
		}
		near(t, c.mode.String(), hit.Point, c.want)
	}
	if _, ok := cmdsys.FindSnap(d, ix, geom.V(30, 30), 8, cmdsys.DefaultSnaps); ok {
		t.Fatal("snapped in empty space")
	}
}

func TestWindowAndCrossingSelection(t *testing.T) {
	s := newSession(t)
	s.Script("LINE 0,0 10,0  LINE 20,0 40,0 ")
	s.SelectBox(geom.V(-1, -1), geom.V(30, 1), false)
	if s.Selection().Len() != 1 {
		t.Fatalf("window selected %d, want 1 (only the fully enclosed line)", s.Selection().Len())
	}
	s.Selection().Clear()
	s.SelectBox(geom.V(30, 1), geom.V(-1, -1), false)
	if s.Selection().Len() != 2 {
		t.Fatalf("crossing selected %d, want 2", s.Selection().Len())
	}
}

func TestNounVerbMoveAndErase(t *testing.T) {
	s := newSession(t)
	s.Script("CIRCLE 0,0 5")
	s.Click(geom.V(5, 0), false)
	if s.Selection().Len() != 1 {
		t.Fatal("click on the circle did not select it")
	}
	s.Script("MOVE 0,0 10,20")
	c := entities[*doc.Circle](s)[0]
	near(t, "moved center", c.Center, geom.V(10, 20))
	if s.Selection().Len() != 0 {
		t.Fatal("MOVE left the selection set")
	}
	s.Script("ERASE")
	if s.Active() != "ERASE" {
		t.Fatal("ERASE without a selection did not prompt")
	}
	s.Click(geom.V(15, 20), false)
	s.Submit("")
	if s.Drawing().Len() != 0 {
		t.Fatalf("ERASE left %d entities", s.Drawing().Len())
	}
}

func TestCopyRotateScale(t *testing.T) {
	s := newSession(t)
	s.Script("LINE 0,0 10,0 ")
	s.SelectBox(geom.V(-1, -1), geom.V(11, 1), false)
	s.Script("COPY 0,0 0,5 0,10 ")
	if n := len(entities[*doc.Line](s)); n != 3 {
		t.Fatalf("after COPY: %d lines, want 3", n)
	}
	s.SelectBox(geom.V(-1, -1), geom.V(11, 1), false)
	s.Script("ROTATE 0,0 90")
	l := entities[*doc.Line](s)[0]
	near(t, "rotated end", l.End, geom.V(0, 10))
	s.Selection().Add(l.Handle())
	s.Script("SCALE 0,0 2")
	near(t, "scaled end", entities[*doc.Line](s)[0].End, geom.V(0, 20))
}

func TestLayerZoomUnknown(t *testing.T) {
	s := newSession(t)
	s.Script("LAYER walls=1 LINE 0,0 100,50  ZOOM e")
	if s.Drawing().CurrentLayer != "walls" || s.Drawing().Layer("walls").Color != 1 {
		t.Fatalf("layer = %q %+v", s.Drawing().CurrentLayer, s.Drawing().Layer("walls"))
	}
	if l := entities[*doc.Line](s)[0]; l.Layer != "walls" {
		t.Fatalf("line layer = %q", l.Layer)
	}
	v := s.View()
	near(t, "zoom center", v.Center, geom.V(50, 25))
	s.Submit("FROB")
	if h := s.History(); !strings.Contains(h[len(h)-1], "Unknown command") {
		t.Fatalf("history = %q", h[len(h)-1])
	}
}
