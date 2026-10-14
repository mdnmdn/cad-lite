package commands_test

import (
	"math"
	"strings"
	"testing"

	"go-draw/cmdsys"
	"go-draw/doc"
	"go-draw/geom"
	"go-draw/render"
)

func addEntities(t *testing.T, s *cmdsys.Session, es ...doc.Entity) {
	t.Helper()
	tx, err := s.Drawing().Begin()
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range es {
		tx.Add(e)
	}
	tx.Commit()
}

func lastLine(s *cmdsys.Session) string {
	h := s.History()
	return h[len(h)-1]
}

func wantLast(t *testing.T, s *cmdsys.Session, want string) {
	t.Helper()
	if got := lastLine(s); got != want {
		t.Fatalf("last history line = %q, want %q", got, want)
	}
	if s.Active() != "" {
		t.Fatalf("command %s still active", s.Active())
	}
}

func TestAreaPoints(t *testing.T) {
	s := newSession(t)
	s.Script("AREA 0,0 10,0 10,5 ")
	wantLast(t, s, "Area = 25, Perimeter = 26.1803")
	s.Script("AA 0,0 4,0 4,3 0,3 ")
	if got := lastLine(s); got != "Area = 12, Perimeter = 14" {
		t.Fatalf("alias AA: %q", got)
	}
	if len(entities[*doc.Line](s)) != 0 {
		t.Fatal("AREA must not draw")
	}
}

func TestAreaPointsClockwiseAndPrompt(t *testing.T) {
	s := newSession(t)
	s.Run("AREA")
	if got := s.Prompt(); got != "Specify first corner point or [Object]:" {
		t.Fatalf("prompt = %q", got)
	}
	s.Script("0,0 0,3 4,3")
	if got := s.Prompt(); got != "Specify next corner point or press ENTER for total:" {
		t.Fatalf("prompt = %q", got)
	}
	b := s.CurrentPrompt().Base
	if b == nil || *b != geom.V(4, 3) {
		t.Fatalf("base = %v", b)
	}
	s.Submit("")
	wantLast(t, s, "Area = 6, Perimeter = 12")
}

func TestAreaPreview(t *testing.T) {
	s := newSession(t)
	s.Script("AREA 0,0 10,0")
	s.Hover(geom.V(10, 10))
	var dl render.DisplayList
	s.Overlay(&dl)
	if len(dl.Items) != 1 || !dl.Items[0].Closed || len(dl.Items[0].Points) != 3 {
		t.Fatalf("preview = %+v", dl.Items)
	}
}

func TestAreaObjectCircle(t *testing.T) {
	s := newSession(t)
	addEntities(t, s, &doc.Circle{Center: geom.V(0, 0), Radius: 10})
	s.Run("AREA")
	s.Submit("O")
	if got := s.Prompt(); got != "Select object:" {
		t.Fatalf("prompt = %q", got)
	}
	s.Click(geom.V(10, 0), false)
	wantLast(t, s, "Area = 314.1593, Circumference = 62.8319")
}

func TestAreaObjectClickWithHover(t *testing.T) {
	s := newSession(t)
	addEntities(t, s, &doc.Circle{Center: geom.V(0, 0), Radius: 10})
	s.Script("AREA Object")
	s.Hover(geom.V(0, 10))
	var dl render.DisplayList
	s.Overlay(&dl)
	if len(dl.Items) == 0 {
		t.Fatal("no hover highlight")
	}
}

func TestAreaObjectPolylineBulges(t *testing.T) {
	s := newSession(t)
	addEntities(t, s,
		&doc.LWPolyline{Points: []geom.Vec2{geom.V(0, 0), geom.V(10, 0), geom.V(10, 4), geom.V(0, 4)}, Bulges: []float64{0, 1, 0, 1}, Closed: true},
		&doc.LWPolyline{Points: []geom.Vec2{geom.V(100, 0), geom.V(110, 0)}, Bulges: []float64{1, 1}, Closed: true},
	)
	s.Script("AREA O")
	s.Click(geom.V(5, 0), false)
	area := 40 + 4*math.Pi
	perim := 20 + 4*math.Pi
	wantLast(t, s, "Area = "+cmdsys.FormatNumber(area)+", Perimeter = "+cmdsys.FormatNumber(perim))
	s.Script("AREA O")
	s.Click(geom.V(105, -5), false)
	wantLast(t, s, "Area = "+cmdsys.FormatNumber(math.Pi*25)+", Perimeter = "+cmdsys.FormatNumber(10*math.Pi))
}

func TestAreaObjectOpenShapesCloseByChord(t *testing.T) {
	s := newSession(t)
	addEntities(t, s,
		&doc.LWPolyline{Points: []geom.Vec2{geom.V(0, 0), geom.V(4, 0), geom.V(4, 3)}},
		&doc.Arc{Center: geom.V(100, 0), Radius: 2, StartAngle: 0, EndAngle: math.Pi},
	)
	s.Script("AREA O")
	s.Click(geom.V(2, 0), false)
	wantLast(t, s, "Area = 6, Perimeter = 7")
	s.Script("AREA O")
	s.Click(geom.V(100, 2), false)
	wantLast(t, s, "Area = "+cmdsys.FormatNumber(2*math.Pi)+", Perimeter = "+cmdsys.FormatNumber(2*math.Pi))
}

func TestAreaObjectLineAndMiss(t *testing.T) {
	s := newSession(t)
	addEntities(t, s, &doc.Line{Start: geom.V(0, 0), End: geom.V(10, 0)})
	s.Script("AREA O")
	s.Click(geom.V(50, 50), false)
	if got := lastLine(s); got != "No object found." || s.Active() != "AREA" {
		t.Fatalf("miss: %q active=%q", got, s.Active())
	}
	s.Click(geom.V(5, 0), false)
	wantLast(t, s, "Object has no area.")
}

func TestMeasureGeomDistance(t *testing.T) {
	s := newSession(t)
	s.Run("MEA")
	if got := s.Prompt(); got != "Enter an option or [Distance/Radius/Angle/ARea] <Distance>:" {
		t.Fatalf("prompt = %q", got)
	}
	s.Script(" 0,0 3,4")
	wantLast(t, s, "Distance = 5,  Angle = 53.1301°,  Delta X = 3,  Delta Y = 4")
}

func TestMeasureGeomRadius(t *testing.T) {
	s := newSession(t)
	addEntities(t, s, &doc.Circle{Center: geom.V(0, 0), Radius: 7}, &doc.Line{Start: geom.V(50, 0), End: geom.V(60, 0)})
	s.Script("MEASUREGEOM R")
	s.Click(geom.V(55, 0), false)
	if got := lastLine(s); got != "Select an arc or a circle." || s.Active() == "" {
		t.Fatalf("line: %q", got)
	}
	s.Click(geom.V(0, 7), false)
	wantLast(t, s, "Radius = 7, Diameter = 14")
}

func TestMeasureGeomAngleTwoLines(t *testing.T) {
	s := newSession(t)
	addEntities(t, s,
		&doc.Line{Start: geom.V(0, 0), End: geom.V(20, 0)},
		&doc.Line{Start: geom.V(0, 0), End: geom.V(0, 20)},
		&doc.Line{Start: geom.V(0, 0), End: geom.V(20, 20)},
	)
	s.Script("MEA A")
	s.Click(geom.V(10, 0), false)
	s.Click(geom.V(0, 10), false)
	wantLast(t, s, "Angle = 90°")
	s.Script("MEA A")
	s.Click(geom.V(10, 0), false)
	s.Click(geom.V(10, 10), false)
	wantLast(t, s, "Angle = 45°")
}

func TestMeasureGeomAngleOppositeSide(t *testing.T) {
	s := newSession(t)
	addEntities(t, s,
		&doc.Line{Start: geom.V(-20, 0), End: geom.V(20, 0)},
		&doc.Line{Start: geom.V(-20, -20), End: geom.V(20, 20)},
	)
	s.Script("MEA A")
	s.Click(geom.V(-10, 0), false)
	s.Click(geom.V(10, 10), false)
	wantLast(t, s, "Angle = 135°")
}

func TestMeasureGeomAngleArcAndVertex(t *testing.T) {
	s := newSession(t)
	addEntities(t, s, &doc.Arc{Center: geom.V(0, 0), Radius: 10, StartAngle: math.Pi / 6, EndAngle: math.Pi / 2})
	s.Script("MEA A")
	s.Click(geom.V(0, 10), false)
	wantLast(t, s, "Angle = 60°")
	s.Script("MEA A  100,100 110,100 100,110")
	wantLast(t, s, "Angle = 90°")
	s.Script("MEA A 200,200 210,200 @10<60")
	wantLast(t, s, "Angle = 60°")
}

func TestMeasureGeomAngleCircle(t *testing.T) {
	s := newSession(t)
	addEntities(t, s, &doc.Circle{Center: geom.V(0, 0), Radius: 10})
	s.Script("MEA A")
	s.Click(geom.V(10, 0), false)
	s.Script("0,10")
	wantLast(t, s, "Angle = 90°")
}

func TestMeasureGeomArea(t *testing.T) {
	s := newSession(t)
	s.Script("MEA AR 0,0 6,0 6,2 ")
	wantLast(t, s, "Area = 6, Perimeter = 14.3246")
	addEntities(t, s, &doc.Circle{Center: geom.V(0, 0), Radius: 1})
	s.Script("MEA AR O")
	s.Click(geom.V(1, 0), false)
	wantLast(t, s, "Area = 3.1416, Circumference = 6.2832")
}

func TestDistMultiple(t *testing.T) {
	s := newSession(t)
	s.Run("DIST")
	if got := s.Prompt(); got != "Specify first point or [Multiple points]:" {
		t.Fatalf("prompt = %q", got)
	}
	s.Script("M 0,0 3,4 3,10 ")
	wantLast(t, s, "Distance = 11")
	s.Script("DI 0,0 3,4")
	if !strings.HasPrefix(lastLine(s), "Distance = 5,") {
		t.Fatalf("default DIST changed: %q", lastLine(s))
	}
}
