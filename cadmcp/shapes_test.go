package cadmcp_test

import (
	"math"
	"testing"

	"cad-lite/doc"
	"cad-lite/geom"
)

func entityOf[T doc.Entity](t *testing.T, r *rig, m map[string]any) T {
	t.Helper()
	e, ok := r.s.Drawing().Entity(mustHandle(t, str(m, "handle"))).(T)
	if !ok {
		t.Fatalf("entity %v has the wrong type", m)
	}
	return e
}

func TestAddArcByCentreAndAngles(t *testing.T) {
	r := newRig(t)
	a := entityOf[*doc.Arc](t, r, r.must("add_arc", `{"center":[10,20],"radius":5,"start_angle":-90,"end_angle":450}`))
	if a.Center != geom.V(10, 20) || a.Radius != 5 || math.Abs(a.StartAngle-3*math.Pi/2) > 1e-12 || math.Abs(a.EndAngle-math.Pi/2) > 1e-12 {
		t.Fatalf("arc = %+v (want 270° to 90° counter-clockwise)", a)
	}
	// Counter-clockwise from 270° to 90° passes through 0°, so the arc's right end is at x=15.
	if b := a.Bounds(); math.Abs(b.Max.X-15) > 1e-9 || math.Abs(b.Min.X-10) > 1e-9 {
		t.Fatalf("bounds = %+v", b)
	}
	r.must("add_arc", `{"p1":[0,0],"p2":[1,1],"p3":[2,0]}`) // the three-point form still works
	r.fails("add_arc", `{"center":[0,0],"radius":5,"start_angle":0}`, "needs center, radius, start_angle and end_angle")
	r.fails("add_arc", `{"center":[0,0],"radius":5,"start_angle":0,"end_angle":360}`, "must differ")
	r.fails("add_arc", `{"center":[0,0],"radius":0,"start_angle":0,"end_angle":90}`, "positive")
	r.fails("add_arc", `{"p1":[0,0],"p2":[1,1],"p3":[2,0],"radius":1}`, "not both")
	r.fails("add_arc", `{"p1":[0,0],"p2":[1,1]}`, "needs p1, p2 and p3")
	r.fails("add_arc", `{}`, "give p1, p2, p3")
}

func TestAddPolylineWithBulges(t *testing.T) {
	r := newRig(t)
	// A 10x10 slot: straight bottom, a half circle on the right (bulge 1), straight top, half
	// circle back on the left.
	p := entityOf[*doc.LWPolyline](t, r, r.must("add_polyline", `{"points":[[0,0],[10,0],[10,10],[0,10]],"bulges":[0,1,0,1],"closed":true}`))
	if len(p.Bulges) != 4 || p.Bulges[1] != 1 {
		t.Fatalf("bulges = %v", p.Bulges)
	}
	if b := p.Bounds(); math.Abs(b.Max.X-15) > 1e-9 || math.Abs(b.Min.X+5) > 1e-9 {
		t.Fatalf("bounds = %+v", b)
	}
	if p := entityOf[*doc.LWPolyline](t, r, r.must("add_polyline", `{"points":[[0,0],[1,0]],"bulges":[0,0]}`)); p.Bulges != nil {
		t.Fatalf("all-zero bulges stored: %v", p.Bulges)
	}
	r.fails("add_polyline", `{"points":[[0,0],[1,0],[2,0]],"bulges":[0,1]}`, "one value per point (3)")
}

func TestAddRectangleForms(t *testing.T) {
	r := newRig(t)
	p := entityOf[*doc.LWPolyline](t, r, r.must("add_rectangle", `{"origin":[10,10],"width":-4,"height":3}`))
	if b := p.Bounds(); b.Min != geom.V(6, 10) || b.Max != geom.V(10, 13) {
		t.Fatalf("size form bounds = %+v", b)
	}
	f := entityOf[*doc.LWPolyline](t, r, r.must("add_rectangle", `{"x1":0,"y1":0,"x2":10,"y2":6,"fillet":1}`))
	if len(f.Points) != 8 || f.Bounds().Max != geom.V(10, 6) {
		t.Fatalf("fillet = %+v", f)
	}
	arcs := 0
	for _, b := range f.Bulges {
		if math.Abs(math.Abs(b)-math.Tan(math.Pi/8)) < 1e-12 {
			arcs++
		}
	}
	if arcs != 4 {
		t.Fatalf("fillet bulges = %v", f.Bulges)
	}
	// Area of a 10x6 rectangle less four corners of (4-π)r².
	if got, want := geom.PolylineArea(f.Geometry()[0].(geom.Polyline)), 60-(4-math.Pi); math.Abs(math.Abs(got)-want) > 1e-9 {
		t.Fatalf("fillet area = %v, want %v", got, want)
	}
	c := entityOf[*doc.LWPolyline](t, r, r.must("add_rectangle", `{"origin":[0,0],"width":10,"height":6,"chamfer":[2,1]}`))
	if len(c.Points) != 8 || c.Bulges != nil && hasNonZero(c.Bulges) {
		t.Fatalf("chamfer = %+v", c)
	}
	// d1 comes off the edges along X, d2 off those along Y: a vertex at (2,0) and one at (0,1).
	if !hasPoint(c.Points, geom.V(2, 0)) || !hasPoint(c.Points, geom.V(0, 1)) || hasPoint(c.Points, geom.V(1, 0)) {
		t.Fatalf("chamfer points = %v", c.Points)
	}
	if got, want := geom.PolylineArea(c.Geometry()[0].(geom.Polyline)), 60-4*(2*1/2.0); math.Abs(math.Abs(got)-want) > 1e-9 {
		t.Fatalf("chamfer area = %v, want %v", got, want)
	}
	r.fails("add_rectangle", `{"x1":0,"y1":0,"x2":10,"y2":6,"fillet":4}`, "does not fit")
	r.fails("add_rectangle", `{"x1":0,"y1":0,"x2":10,"y2":6,"fillet":1,"chamfer":[1,1]}`, "not both")
	r.fails("add_rectangle", `{"x1":0,"y1":0,"x2":10,"y2":6,"origin":[0,0]}`, "not both")
	r.fails("add_rectangle", `{"origin":[0,0],"width":5}`, "needs origin, width and height")
	r.fails("add_rectangle", `{"origin":[0,0],"width":5,"height":0}`, "non-zero")
	r.fails("add_rectangle", `{"x1":0,"y1":0,"x2":10,"y2":6,"chamfer":[1]}`, "[d1, d2]")
	r.fails("add_rectangle", `{"x1":0,"y1":0,"x2":10,"y2":6,"fillet":-1}`, ">= 0")
}

func TestNewShapeFormsInBatch(t *testing.T) {
	r := newRig(t)
	m := r.must("batch", `{"ops":[
		{"op":"add_arc","id":"a","args":{"center":[0,0],"radius":2,"start_angle":0,"end_angle":90}},
		{"op":"add_polyline","id":"p","args":{"points":[[0,0],[4,0]],"bulges":[1,0]}},
		{"op":"add_rectangle","id":"r","args":{"origin":[0,0],"width":8,"height":4,"fillet":1}},
		{"op":"move","args":{"handles":["$a","$p","$r"],"dx":1,"dy":0}}
	]}`)
	if len(m["created"].([]any)) != 3 || r.s.Drawing().UndoDepth() != 1 {
		t.Fatalf("batch = %v", m)
	}
	r.fails("batch", `{"ops":[{"op":"add_line","args":{"x1":0,"y1":0,"x2":1,"y2":0}},{"op":"add_rectangle","args":{"origin":[0,0],"width":2,"height":2,"fillet":5}}]}`, "ops[1] (add_rectangle): the fillet or chamfer does not fit")
	if r.s.Drawing().Len() != 3 {
		t.Fatal("a failed batch changed the drawing")
	}
}

func hasNonZero(bs []float64) bool {
	for _, b := range bs {
		if b != 0 {
			return true
		}
	}
	return false
}

func hasPoint(ps []geom.Vec2, q geom.Vec2) bool {
	for _, p := range ps {
		if p.Dist(q) < 1e-9 {
			return true
		}
	}
	return false
}
