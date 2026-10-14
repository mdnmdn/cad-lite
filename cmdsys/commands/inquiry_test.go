package commands_test

import (
	"math"
	"strings"
	"testing"

	"cad-lite/cmdsys"
	"cad-lite/doc"
	"cad-lite/geom"
)

func historyHas(s *cmdsys.Session, want string) bool {
	for _, h := range s.History() {
		if strings.Contains(h, want) {
			return true
		}
	}
	return false
}

func wantHistory(t *testing.T, s *cmdsys.Session, want ...string) {
	t.Helper()
	for _, w := range want {
		if !historyHas(s, w) {
			t.Errorf("history lacks %q:\n%s", w, strings.Join(s.History(), "\n"))
		}
	}
}

func TestDistOutputAndVariable(t *testing.T) {
	s := newSession(t)
	s.Script("DIST 0,0 3,4")
	wantHistory(t, s,
		"Distance = 5.0000,  Angle in XY Plane = 53,  Angle from XY Plane = 0",
		"Delta X = 3.0000,  Delta Y = 4.0000,  Delta Z = 0.0000")
	if got := s.Vars().Real("DISTANCE"); math.Abs(got-5) > 1e-9 {
		t.Fatalf("DISTANCE = %v", got)
	}
}

func TestDistMultipleStoresTotal(t *testing.T) {
	s := newSession(t)
	s.Script("DIST M 0,0 3,4 3,10 ")
	wantHistory(t, s, "Distance = 11.0000")
	if got := s.Vars().Real("DISTANCE"); math.Abs(got-11) > 1e-9 {
		t.Fatalf("DISTANCE = %v", got)
	}
}

func TestIDSetsLastPoint(t *testing.T) {
	s := newSession(t)
	s.Script("ID 7,8")
	wantHistory(t, s, "X = 7.0000  Y = 8.0000  Z = 0.0000")
	if got := s.Vars().Point("LASTPOINT"); got != geom.V(7, 8) {
		t.Fatalf("LASTPOINT = %v", got)
	}
	s.Script("LINE @ 20,20 ")
	ls := entities[*doc.Line](s)
	if len(ls) != 1 || ls[0].Start != geom.V(7, 8) {
		t.Fatalf("@ after ID did not start at the ID point: %+v", ls)
	}
}

func TestAreaVariables(t *testing.T) {
	s := newSession(t)
	s.Script("AREA 0,0 4,0 4,3 ")
	wantHistory(t, s, "Area = 6.0000, Perimeter = 12.0000")
	if s.Vars().Real("AREA") != 6 || s.Vars().Real("PERIMETER") != 12 {
		t.Fatalf("AREA %v PERIMETER %v", s.Vars().Real("AREA"), s.Vars().Real("PERIMETER"))
	}
}

func TestAreaAddSubtract(t *testing.T) {
	s := newSession(t)
	s.Run("AREA")
	s.Submit("A")
	if got := s.Prompt(); got != "Specify first corner point or [Object/Subtract area/Exit]:" {
		t.Fatalf("prompt after Add = %q", got)
	}
	s.Script("0,0 10,0 10,10 0,10 ")
	wantHistory(t, s, "Total area = 100.0000")
	s.Submit("S")
	s.Script("0,0 2,0 2,2 0,2 ")
	wantHistory(t, s, "Total area = 96.0000")
	if s.Vars().Real("AREA") != 96 {
		t.Fatalf("AREA = %v", s.Vars().Real("AREA"))
	}
	if s.Active() == "" {
		t.Fatal("AREA ended before eXit")
	}
	s.Submit("E")
	if s.Active() != "" {
		t.Fatalf("still in %s after Exit", s.Active())
	}
}

func TestAreaAddObject(t *testing.T) {
	s := newSession(t)
	addEntities(t, s, &doc.Circle{Center: geom.V(0, 0), Radius: 1})
	s.Script("AREA A O")
	s.Click(geom.V(1, 0), false)
	wantHistory(t, s, "Area = 3.1416, Circumference = 6.2832", "Total area = 3.1416")
}

func TestMeasureGeomLoopsUntilExit(t *testing.T) {
	s := newSession(t)
	s.Script("MEASUREGEOM D 0,0 3,4")
	if s.Active() != "MEASUREGEOM" {
		t.Fatalf("MEASUREGEOM ended after one measurement")
	}
	if got := s.Prompt(); !strings.Contains(got, "eXit") && !strings.Contains(got, "Exit") {
		t.Fatalf("prompt = %q", got)
	}
	s.Submit("D")
	s.Script("0,0 6,8")
	wantHistory(t, s, "Distance = 10.0000")
	s.Submit("E")
	if s.Active() != "" {
		t.Fatalf("still in %s after Exit", s.Active())
	}
}

func TestListLineFormat(t *testing.T) {
	s := newSession(t)
	addEntities(t, s, &doc.Line{Start: geom.V(0, 0), End: geom.V(10, 0)})
	s.Script("LS ALL ")
	wantHistory(t, s, `LINE  Layer: "0"`, "Space: Model space", "Handle = ",
		"from point, X=  0.0000  Y=  0.0000  Z=  0.0000",
		"  to point, X= 10.0000  Y=  0.0000  Z=  0.0000",
		"Length = 10.0000,  Angle in XY Plane = 0",
		"Delta X = 10.0000,  Delta Y = 0.0000,  Delta Z = 0.0000")
}

func TestListClosedPolylineAreaPerimeter(t *testing.T) {
	s := newSession(t)
	addEntities(t, s, &doc.LWPolyline{Points: []geom.Vec2{geom.V(0, 0), geom.V(4, 0), geom.V(4, 3)}, Closed: true})
	s.Script("LIST ALL ")
	wantHistory(t, s, "Closed", "area 6.0000", "perimeter 12.0000", "at point  X=  4.0000  Y=  3.0000")
}
