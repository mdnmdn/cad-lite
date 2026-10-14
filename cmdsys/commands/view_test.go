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

func viewSession(t *testing.T) (*cmdsys.Session, *render.View) {
	t.Helper()
	v := render.DefaultView(800, 600)
	s := cmdsys.NewSession(doc.New(), &v, commands.NewRegistry())
	tx, _ := s.Drawing().Begin()
	tx.Add(&doc.Line{Start: geom.V(0, 0), End: geom.V(100, 0)})
	tx.Add(&doc.Line{Start: geom.V(0, 0), End: geom.V(0, 50)})
	tx.Commit()
	return s, &v
}

func TestZoomWindowThenPrevious(t *testing.T) {
	s, v := viewSession(t)
	before := *v
	s.Script("ZOOM\nW\n0,0\n10,10\n")
	if v.Scale == before.Scale {
		t.Fatal("window did not zoom")
	}
	s.Script("ZOOM\nP\n")
	if v.Center != before.Center || v.Scale != before.Scale {
		t.Fatalf("previous view = %v %v, want %v %v", v.Center, v.Scale, before.Center, before.Scale)
	}
	s.Script("ZOOM\nP\n")
	if !strings.Contains(history(s), "No previous view") {
		t.Fatalf("no message when the stack is empty:\n%s", history(s))
	}
}

func TestZoomPreviousSeesWheelChanges(t *testing.T) {
	s, v := viewSession(t)
	s.Script("ZOOM\nE\n")
	extents := *v
	v.ZoomAt(100, 100, 3) // the mouse wheel
	wheel := *v
	s.Script("ZOOM\nW\n0,0\n5,5\n")
	s.Script("ZOOM\nP\n")
	if v.Center != wheel.Center || v.Scale != wheel.Scale {
		t.Fatalf("previous = %v %v, want the wheel view %v %v", v.Center, v.Scale, wheel.Center, wheel.Scale)
	}
	s.Script("ZOOM\nP\n")
	if v.Center != extents.Center || v.Scale != extents.Scale {
		t.Fatalf("second previous = %v %v, want extents %v %v", v.Center, v.Scale, extents.Center, extents.Scale)
	}
}

func TestZoomScale(t *testing.T) {
	s, v := viewSession(t)
	s.Script("ZOOM\nE\n")
	base := v.Scale
	s.Script("ZOOM\n2X\n")
	if math.Abs(v.Scale-2*base) > 1e-9 {
		t.Fatalf("2X: scale %v, want %v", v.Scale, 2*base)
	}
	s.Script("ZOOM\n2\n") // relative to the full view
	if math.Abs(v.Scale-2*base) > 1e-9 {
		t.Fatalf("2: scale %v, want %v", v.Scale, 2*base)
	}
	s.Script("ZOOM\nS\n.5X\n")
	if math.Abs(v.Scale-base) > 1e-9 {
		t.Fatalf("S .5X: scale %v, want %v", v.Scale, base)
	}
	s.Script("ZOOM\n2XP\n")
	if math.Abs(v.Scale-2*base) > 1e-9 {
		t.Fatalf("2XP: scale %v", v.Scale)
	}
}

func TestZoomCenterAndAll(t *testing.T) {
	s, v := viewSession(t)
	s.Script("ZOOM\nC\n40,30\n100\n")
	if v.Center != geom.V(40, 30) || math.Abs(v.Height/v.Scale-100) > 1e-9 {
		t.Fatalf("center %v height %v", v.Center, v.Height/v.Scale)
	}
	s.Script("ZOOM\nC\n0,0\n\n")
	if v.Center != geom.V(0, 0) || math.Abs(v.Height/v.Scale-100) > 1e-9 {
		t.Fatalf("Enter must keep the height: center %v height %v", v.Center, v.Height/v.Scale)
	}
	s.Script("ZOOM\nA\n")
	if !v.Visible().ContainsBox(s.Drawing().Extents()) {
		t.Fatalf("All must show the drawing: %v", v.Visible())
	}
}

func TestZoomObjectAndExtentsIgnoreFrozen(t *testing.T) {
	s, v := viewSession(t)
	s.Script("ZOOM\nO\n100,0\n\n")
	if !v.Visible().Contains(geom.V(100, 0)) {
		t.Fatalf("object not in view: %v", v.Visible())
	}
	d := s.Drawing()
	d.AddLayer("FAR", 1)
	tx, _ := d.Begin()
	tx.Add(&doc.Line{EntityCommon: doc.EntityCommon{Layer: "FAR"}, Start: geom.V(5000, 5000), End: geom.V(5100, 5100)})
	tx.Commit()
	d.Layer("FAR").Frozen = true
	s.Script("ZOOM\nE\n")
	if v.Visible().Contains(geom.V(5000, 5000)) {
		t.Fatalf("frozen geometry must not count for extents: %v", v.Visible())
	}
}

func TestPanMovesTheView(t *testing.T) {
	s, v := viewSession(t)
	c := v.Center
	s.Script("-PAN\n0,0\n10,20\n")
	if got := v.Center; !got.Near(c.Sub(geom.V(10, 20)), 1e-9) {
		t.Fatalf("centre %v", got)
	}
	s.Script("-PAN\n5,5\n\n") // a displacement
	if got := v.Center; !got.Near(c.Sub(geom.V(15, 25)), 1e-9) {
		t.Fatalf("centre %v after a displacement", got)
	}
	s.Script("ZOOM\nP\n")
	if got := v.Center; !got.Near(c.Sub(geom.V(10, 20)), 1e-9) {
		t.Fatalf("previous after pan: %v", got)
	}
}

func TestRegenAndRedrawAreSilentNoOps(t *testing.T) {
	s, _ := viewSession(t)
	for _, c := range []string{"REGEN", "REGENALL", "REDRAW", "REDRAWALL", "RE", "R"} {
		n := len(s.History())
		s.Script(c + "\n")
		if s.Active() != "" {
			t.Fatalf("%s left a command running", c)
		}
		if got := s.History()[n:]; len(got) != 1 {
			t.Fatalf("%s printed %v, want only its echo", c, got)
		}
	}
}
