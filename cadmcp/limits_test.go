package cadmcp_test

import (
	"context"
	"errors"
	"math"
	"testing"
	"time"

	"cad-lite/cadmcp"
	"cad-lite/doc"
	"cad-lite/geom"
)

// Finite but huge inputs used to produce NaN or infinite geometry, after which get_drawing could
// not encode the drawing until an undo.
func TestHugeInputsCannotMakeNonFiniteGeometry(t *testing.T) {
	r := newRig(t)
	big := r.must("add_circle", `{"cx":1e12,"cy":0,"r":1}`) // at the cap: accepted
	r.fails("add_polyline", `{"points":[[0,0],[1,0]],"bulges":[1e300,0]}`, "bulge")
	r.fails("add_circle", `{"cx":1e308,"cy":1e308,"r":1e308}`, "1e+12")
	r.fails("add_line", `{"x1":0,"y1":0,"x2":1e13,"y2":0}`, "1e+12")
	r.fails("add_rectangle", `{"origin":[0,0],"width":1e300,"height":1}`, "1e+12")
	r.fails("move", `{"handles":["`+str(big, "handle")+`"],"dx":1e300,"dy":0}`, "1e+12")
	r.fails("scale", `{"handles":["`+str(big, "handle")+`"],"cx":0,"cy":0,"factor":1e300}`, "1e+12")
	// Each input within the caps, the result still out of range: refused by the bounds check.
	r.fails("scale", `{"handles":["`+str(big, "handle")+`"],"cx":-1e12,"cy":0,"factor":1e12}`, "out of range")
	r.fails("batch", `{"ops":[{"op":"add_circle","id":"c","args":{"cx":1e12,"cy":0,"r":1}},{"op":"scale","args":{"handles":["$c"],"cx":-1e12,"cy":0,"factor":1e12}}]}`, "ops[1] (scale): ")
	if r.s.Drawing().Len() != 1 || r.s.Drawing().UndoDepth() != 1 {
		t.Fatalf("refused calls changed the drawing: len=%d depth=%d", r.s.Drawing().Len(), r.s.Drawing().UndoDepth())
	}
	r.must("get_drawing", `{}`)
	r.must("list_entities", `{}`)
}

// A batch that fails validation must not cancel the user's running command.
func TestFailedBatchKeepsRunningCommand(t *testing.T) {
	r := newRig(t)
	r.s.Run("LINE")
	r.s.Submit("0,0")
	if r.s.Active() != "LINE" {
		t.Fatalf("setup: active %q", r.s.Active())
	}
	r.fails("batch", `{"ops":[{"op":"move","args":{"handles":["FFFF"],"dx":1,"dy":1}}]}`, "no entity")
	r.fails("batch", `{"ops":[{"op":"add_line","args":{"x1":0,"y1":0,"x2":1,"y2":1,"layer":"nope"}}]}`, "does not exist")
	if r.s.Active() != "LINE" {
		t.Fatalf("a refused batch cancelled the running command (active %q)", r.s.Active())
	}
}

// An entity moved onto a locked layer earlier in the batch is refused while checking, before the
// running command is cancelled.
func TestBatchLockedLayerCheckedBeforeCancelling(t *testing.T) {
	r := newRig(t)
	r.must("create_layer", `{"name":"locked","locked":true}`)
	r.s.Run("LINE")
	r.fails("batch", `{"ops":[{"op":"add_line","id":"a","args":{"x1":0,"y1":0,"x2":1,"y2":1,"layer":"locked"}},{"op":"move","args":{"handles":["$a"],"dx":1,"dy":1}}]}`, "ops[1] (move): entity $a is on locked layer")
	if r.s.Active() != "LINE" {
		t.Fatalf("active %q", r.s.Active())
	}
}

func TestRectangleFilletOfHalfASideHasNoDegenerateSegment(t *testing.T) {
	r := newRig(t)
	p := entityOf[*doc.LWPolyline](t, r, r.must("add_rectangle", `{"origin":[0,0],"width":10,"height":4,"fillet":2}`))
	n := len(p.Points)
	if n != 6 {
		t.Fatalf("points = %v, want 6 (the vertical sides vanish)", p.Points)
	}
	for i := range n {
		if d := p.Points[i].Dist(p.Points[(i+1)%n]); d < 1e-9 {
			t.Fatalf("segment %d has length %g: %v", i, d, p.Points)
		}
	}
	if got, want := math.Abs(geom.PolylineArea(p.Geometry()[0].(geom.Polyline))), 40-(4-math.Pi)*4; math.Abs(got-want) > 1e-9 {
		t.Fatalf("area = %v, want %v", got, want)
	}
}

func TestArcAnglesNormalisedWithTolerance(t *testing.T) {
	r := newRig(t)
	r.fails("add_arc", `{"center":[0,0],"radius":1,"start_angle":-1e-20,"end_angle":0}`, "must differ")
	r.fails("add_arc", `{"center":[0,0],"radius":1,"start_angle":10,"end_angle":10.0000000000001}`, "must differ")
	a := entityOf[*doc.Arc](t, r, r.must("add_arc", `{"center":[0,0],"radius":1,"start_angle":-1e-20,"end_angle":90}`))
	if a.StartAngle != 0 || math.Abs(a.EndAngle-math.Pi/2) > 1e-15 {
		t.Fatalf("arc = %+v", a)
	}
}

// Do must leave the run budget of a deadline free: work that cannot start with RunBudget to spare
// is refused at once rather than run where its reply would be lost.
func TestDoKeepsRunBudgetBeforeDeadline(t *testing.T) {
	d := cadmcp.NewDispatcher(nil) // never drained
	ctx, cancel := context.WithTimeout(context.Background(), cadmcp.RunBudget-time.Second)
	defer cancel()
	start := time.Now()
	err := d.Do(ctx, func() { t.Error("ran") })
	if !errors.Is(err, cadmcp.ErrUIBusy) || time.Since(start) > 500*time.Millisecond {
		t.Fatalf("err %v after %v", err, time.Since(start))
	}
}
