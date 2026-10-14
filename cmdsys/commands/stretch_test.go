package commands_test

import (
	"math"
	"strings"
	"testing"

	"go-draw/doc"
	"go-draw/geom"
)

func TestStretchLineEndpointAndInside(t *testing.T) {
	s := newSession(t)
	xfeed(s, "LINE", "0,0", "10,0", "", "LINE", "6,5", "8,5", "")
	// The window covers x 5..12: the first line's end and all of the second.
	xfeed(s, "STRETCH", "C", "5,-5", "12,10", "", "0,0", "0,3")
	got := strings.Join(lineSet(s), ";")
	if got != "0,0 10,3;6,8 8,8" {
		t.Fatalf("lines = %s", got)
	}
	if s.Active() != "" {
		t.Fatalf("still running %s", s.Active())
	}
}

func TestStretchPolylineVerticesAndDisplacement(t *testing.T) {
	s := newSession(t)
	xfeed(s, "RECTANG", "0,0", "10,10")
	// A click is the first corner of a crossing window; Displacement is typed.
	xfeed(s, "STRETCH", "5,-5", "15,15", "", "D", "5,0")
	p := entities[*doc.LWPolyline](s)[0]
	want := []geom.Vec2{{X: 0, Y: 0}, {X: 15, Y: 0}, {X: 15, Y: 10}, {X: 0, Y: 10}}
	for i, w := range want {
		if p.Points[i] != w {
			t.Fatalf("points = %v", p.Points)
		}
	}
}

func TestStretchEnterUsesBaseAsDisplacement(t *testing.T) {
	s := newSession(t)
	xfeed(s, "LINE", "0,0", "4,0", "")
	xfeed(s, "STRETCH", "C", "-1,-1", "5,1", "", "2,3", "")
	if got := strings.Join(lineSet(s), ";"); got != "2,3 6,3" {
		t.Fatalf("lines = %s", got)
	}
}

func TestStretchCircleMovesOnlyWithCentreInside(t *testing.T) {
	s := newSession(t)
	xfeed(s, "CIRCLE", "0,0", "5", "CIRCLE", "20,0", "3")
	// The window crosses the first circle without holding its centre and holds the second's.
	xfeed(s, "STRETCH", "C", "3,-10", "30,10", "", "0,0", "0,4")
	cs := entities[*doc.Circle](s)
	if cs[0].Center != (geom.Vec2{}) || cs[1].Center != (geom.Vec2{X: 20, Y: 4}) {
		t.Fatalf("centres = %v %v", cs[0].Center, cs[1].Center)
	}
}

func TestStretchArcKeepsRadius(t *testing.T) {
	s := newSession(t)
	xfeed(s, "ARC", "C", "0,0", "10,0", "A", "90")
	if len(entities[*doc.Arc](s)) != 1 {
		t.Fatal("no arc")
	}
	// Only the end (0,10) is inside the window; it moves to (-6,8), still on the circle.
	xfeed(s, "STRETCH", "C", "-3,5", "3,15", "", "0,10", "-6,8")
	got := entities[*doc.Arc](s)[0]
	if math.Abs(got.Radius-10) > 1e-9 {
		t.Fatalf("radius = %v", got.Radius)
	}
	end := func(a float64) geom.Vec2 {
		return got.Center.Add(geom.Vec2{X: math.Cos(a), Y: math.Sin(a)}.Scale(got.Radius))
	}
	if end(got.EndAngle).Sub(geom.Vec2{X: -6, Y: 8}).Len() > 1e-9 {
		t.Fatalf("end = %v", end(got.EndAngle))
	}
	if end(got.StartAngle).Sub(geom.Vec2{X: 10, Y: 0}).Len() > 1e-9 {
		t.Fatalf("start moved: %v", end(got.StartAngle))
	}
}

func TestStretchCrossingPolygon(t *testing.T) {
	s := newSession(t)
	xfeed(s, "LINE", "0,0", "10,0", "")
	xfeed(s, "STRETCH", "CP", "8,-2", "12,-2", "12,2", "8,2", "", "", "0,0", "0,5")
	if got := strings.Join(lineSet(s), ";"); got != "0,0 10,5" {
		t.Fatalf("lines = %s", got)
	}
}

func TestStretchSelectsNothingEnds(t *testing.T) {
	s := newSession(t)
	xfeed(s, "STRETCH", "")
	if s.Active() != "" {
		t.Fatalf("active %s", s.Active())
	}
}

func TestStretchPrompts(t *testing.T) {
	s := newSession(t)
	s.Submit("STRETCH")
	if got := s.Prompt(); got != "Select objects or [Crossing/CPolygon]:" {
		t.Fatalf("prompt = %q", got)
	}
	if !strings.Contains(lastLines(s, 4), "Select objects to stretch by crossing-window or crossing-polygon...") {
		t.Fatal("no hint")
	}
	feed(s, "C", "0,0", "5,5", "")
	if got := s.Prompt(); got != "Specify base point or [Displacement] <Displacement>:" {
		t.Fatalf("prompt = %q", got)
	}
}

func TestMultipleRepeatsUntilEscape(t *testing.T) {
	s := newSession(t)
	xfeed(s, "MULTIPLE", "CIRCLE", "0,0", "5", "10,0", "5")
	if n := len(entities[*doc.Circle](s)); n != 2 {
		t.Fatalf("circles = %d", n)
	}
	if s.Active() != "CIRCLE" {
		t.Fatalf("active = %q", s.Active())
	}
	s.Escape()
	if s.Active() != "" || len(entities[*doc.Circle](s)) != 2 {
		t.Fatalf("after Escape active %q, circles %d", s.Active(), len(entities[*doc.Circle](s)))
	}
	// Each repetition is its own undo step.
	s.Drawing().Undo()
	if n := len(entities[*doc.Circle](s)); n != 1 {
		t.Fatalf("after one undo circles = %d", n)
	}
}

func TestMultipleUnknownCommandEnds(t *testing.T) {
	s := newSession(t)
	xfeed(s, "MULTIPLE", "NOSUCH")
	if s.Active() != "" || !strings.Contains(lastLines(s, 2), "NOSUCH") {
		t.Fatalf("active %q\n%s", s.Active(), lastLines(s, 3))
	}
}
