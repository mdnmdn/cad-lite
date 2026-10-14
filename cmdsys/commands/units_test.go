package commands_test

import (
	"math"
	"testing"

	"go-draw/doc"
	"go-draw/geom"
)

func TestUnitsVariablesRegistered(t *testing.T) {
	s := newSession(t)
	for name, want := range map[string]string{"LUNITS": "2", "LUPREC": "4", "AUNITS": "0", "AUPREC": "0", "ANGBASE": "0", "ANGDIR": "0", "INSUNITS": "4", "UNITMODE": "0"} {
		if !s.Vars().Has(name) || s.Vars().String(name) != want {
			t.Errorf("%s = %q, want %q", name, s.Vars().String(name), want)
		}
	}
	if err := s.Vars().SetString("INSUNITS", "6"); err != nil || s.Drawing().Header.InsUnits != 6 {
		t.Fatalf("INSUNITS does not write the header: %v %d", err, s.Drawing().Header.InsUnits)
	}
}

func TestDashUnitsSetsVariables(t *testing.T) {
	s := newSession(t)
	s.Run("-UNITS")
	// length format 4 architectural, denominator 8, angles DMS, 2 places, base 90, clockwise
	for _, a := range []string{"4", "8", "2", "2", "90", "Y"} {
		if s.Active() == "" {
			t.Fatalf("command ended before %q", a)
		}
		s.Submit(a)
	}
	if s.Active() != "" {
		t.Fatalf("still in %s: %q", s.Active(), s.Prompt())
	}
	v := s.Vars()
	if v.Int("LUNITS") != 4 || v.Int("LUPREC") != 3 || v.Int("AUNITS") != 1 || v.Int("AUPREC") != 2 || v.Real("ANGBASE") != 90 || v.Int("ANGDIR") != 1 {
		t.Fatalf("LUNITS %d LUPREC %d AUNITS %d AUPREC %d ANGBASE %v ANGDIR %d", v.Int("LUNITS"), v.Int("LUPREC"), v.Int("AUNITS"), v.Int("AUPREC"), v.Real("ANGBASE"), v.Int("ANGDIR"))
	}
}

func TestDashUnitsDefaultsAndRange(t *testing.T) {
	s := newSession(t)
	s.Run("UNITS")
	if got := s.Prompt(); got != "Enter choice, 1 to 5 <2>:" {
		t.Fatalf("prompt = %q", got)
	}
	s.Submit("9")
	wantHistory(t, s, "Requires an integer value between 1 and 5.")
	for i := 0; i < 6; i++ {
		s.Submit("")
	}
	if s.Active() != "" || s.Vars().Int("LUNITS") != 2 || s.Vars().Int("LUPREC") != 4 {
		t.Fatalf("Enter did not keep the defaults: %q", s.Prompt())
	}
}

func TestUnitsChangeInquiryOutput(t *testing.T) {
	s := newSession(t)
	_ = s.Vars().Set("LUPREC", 2)
	_ = s.Vars().Set("AUNITS", 1)
	_ = s.Vars().Set("AUPREC", 3)
	s.Script("DIST 0,0 3,4")
	wantHistory(t, s, `Distance = 5.00,  Angle in XY Plane = 53d7'48",`, "Delta X = 3.00")
}

func TestAngbaseAngdirOutputAndInput(t *testing.T) {
	s := newSession(t)
	_ = s.Vars().Set("ANGBASE", 90)
	_ = s.Vars().Set("ANGDIR", 1)
	// The direction of +X is 0 degrees from the X axis: measured from 90 clockwise it reads 90.
	s.Script("DIST 0,0 5,0")
	wantHistory(t, s, "Angle in XY Plane = 90,")
	// Typed angle: 30 from ANGBASE 90, clockwise, is the direction 60.
	addEntities(t, s, &doc.Line{Start: geom.V(0, 0), End: geom.V(10, 0)})
	s.Script("ROTATE ALL  0,0 30")
	l := entities[*doc.Line](s)[0]
	if got := l.End.Angle(); math.Abs(got-math.Pi/3) > 1e-9 {
		t.Fatalf("rotated by direction %v, want 60 degrees", got*180/math.Pi)
	}
}

func TestRotateHeaderReadsVariables(t *testing.T) {
	s := newSession(t)
	addEntities(t, s, &doc.Line{Start: geom.V(0, 0), End: geom.V(10, 0)})
	_ = s.Vars().Set("ANGBASE", 90)
	_ = s.Vars().Set("ANGDIR", 1)
	s.Script("ROTATE ALL ")
	wantHistory(t, s, "Current positive angle in UCS:  ANGDIR=clockwise  ANGBASE=90")
}

func TestTypedAngleUnits(t *testing.T) {
	s := newSession(t)
	addEntities(t, s, &doc.Line{Start: geom.V(0, 0), End: geom.V(10, 0)})
	s.Script("ROTATE ALL  0,0 100g")
	l := entities[*doc.Line](s)[0]
	if got := l.End.Angle(); math.Abs(got-math.Pi/2) > 1e-9 {
		t.Fatalf("100g rotated to %v degrees", got*180/math.Pi)
	}
}
