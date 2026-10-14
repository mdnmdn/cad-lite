package cmdsys_test

import (
	"math"
	"testing"

	"cad-lite/cmdsys"
	"cad-lite/cmdsys/commands"
	"cad-lite/doc"
	"cad-lite/geom"
	"cad-lite/render"
)

func polarSession() *cmdsys.Session {
	v := render.DefaultView(800, 600)
	s := cmdsys.NewSession(doc.New(), &v, commands.NewRegistry())
	s.SnapOn = false
	return s
}

func TestPolarLocksTheRubberBandAngleWithinTolerance(t *testing.T) {
	s := polarSession()
	s.Script("LINE\n0,0\n")
	s.SetPolar(true)
	// 47 degrees at radius 100 is within 5 degrees of 45: the cursor lands on the 45 degree ray.
	a := 47 * math.Pi / 180
	s.Hover(geom.V(100*math.Cos(a), 100*math.Sin(a)))
	got := s.Cursor()
	if math.Abs(got.X-got.Y) > 1e-9 || got.X <= 0 {
		t.Fatalf("cursor %v is not on the 45 degree ray", got)
	}
	// 70 degrees is 20 from 45 and 20 from 90 (outside tolerance of both): unchanged.
	b := 70 * math.Pi / 180
	p := geom.V(100*math.Cos(b), 100*math.Sin(b))
	s.Hover(p)
	if s.Cursor() != p {
		t.Fatalf("cursor %v moved although 70 degrees is not near a polar angle", s.Cursor())
	}
	// Relative to the base point of the prompt, not the origin.
	s.Escape()
	s.Script("LINE\n10,10\n")
	s.Hover(geom.V(10+100, 10+3))
	if c := s.Cursor(); math.Abs(c.Y-10) > 1e-9 {
		t.Fatalf("cursor %v not locked to the horizontal ray through the base", c)
	}
}

func TestPolarAngleVariableAndExclusionWithOrtho(t *testing.T) {
	s := polarSession()
	if err := s.Vars().Set("POLARANG", 30.0); err != nil || s.PolarAngle() != 30 {
		t.Fatalf("POLARANG: %v %v", err, s.PolarAngle())
	}
	s.Script("LINE\n0,0\n")
	s.SetPolar(true)
	a := 61 * math.Pi / 180
	s.Hover(geom.V(50*math.Cos(a), 50*math.Sin(a)))
	if c := s.Cursor(); math.Abs(math.Atan2(c.Y, c.X)*180/math.Pi-60) > 1e-9 {
		t.Fatalf("cursor %v not on the 60 degree ray", c)
	}
	s.SetOrtho(true)
	if s.Polar() {
		t.Fatal("turning ortho on must turn polar off")
	}
	s.SetPolar(true)
	if s.Ortho {
		t.Fatal("turning polar on must turn ortho off")
	}
	if s.Vars().Int("AUTOSNAP")&8 == 0 {
		t.Fatal("AUTOSNAP bit 8 must follow polar tracking")
	}
	if err := s.Vars().Set("AUTOSNAP", 55); err != nil || s.Polar() {
		t.Fatalf("clearing AUTOSNAP bit 8 must turn polar off: %v", err)
	}
}

func TestLineweightDisplayVariable(t *testing.T) {
	s := polarSession()
	if s.LineweightDisplay() || s.Vars().Int("LWDISPLAY") != 0 {
		t.Fatal("lineweights start hidden")
	}
	if err := s.Vars().Set("LWDISPLAY", 1); err != nil || !s.LineweightDisplay() {
		t.Fatalf("LWDISPLAY did not turn lineweights on: %v", err)
	}
	s.SetLineweightDisplay(false)
	if s.Vars().Int("LWDISPLAY") != 0 {
		t.Fatal("LWDISPLAY must follow the session")
	}
}
