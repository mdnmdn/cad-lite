package geom

import (
	"math"
	"testing"
)

func vNear(t *testing.T, name string, got, want Vec2) {
	t.Helper()
	if !got.Near(want, 1e-9) {
		t.Fatalf("%s = %v, want %v", name, got, want)
	}
}

func TestOffsetLineSides(t *testing.T) {
	l := Line{V(0, 0), V(10, 0)}
	left := OffsetLine(l, 3)
	vNear(t, "left A", left.A, V(0, 3))
	vNear(t, "left B", left.B, V(10, 3))
	right := OffsetLine(l, -3)
	vNear(t, "right A", right.A, V(0, -3))
	d := Line{V(0, 0), V(3, 4)}
	o := OffsetLine(d, 5)
	if dist := o.A.Dist(d.A); math.Abs(dist-5) > 1e-12 {
		t.Fatalf("offset distance = %v, want 5", dist)
	}
	if cross := o.B.Sub(o.A).Cross(d.B.Sub(d.A)); math.Abs(cross) > 1e-12 {
		t.Fatalf("offset line not parallel: %v", cross)
	}
}

func TestOffsetCircleAndArc(t *testing.T) {
	c, ok := OffsetCircle(Circle{V(1, 2), 5}, 2)
	if !ok || c.R != 7 || c.C != V(1, 2) {
		t.Fatalf("grow = %+v %v", c, ok)
	}
	c, ok = OffsetCircle(Circle{V(1, 2), 5}, -2)
	if !ok || c.R != 3 {
		t.Fatalf("shrink = %+v %v", c, ok)
	}
	for _, d := range []float64{-5, -6} {
		if _, ok := OffsetCircle(Circle{V(0, 0), 5}, d); ok {
			t.Fatalf("circle offset %v should be degenerate", d)
		}
	}
	a, ok := OffsetArc(Arc{C: V(0, 0), R: 4, Start: 0.5, End: 2}, -1)
	if !ok || a.R != 3 || a.Start != 0.5 || a.End != 2 {
		t.Fatalf("arc = %+v %v", a, ok)
	}
	if _, ok := OffsetArc(Arc{R: 4, End: 1}, -4); ok {
		t.Fatal("arc offset to zero radius should be degenerate")
	}
}

func TestOffsetPolylineOpenMiter(t *testing.T) {
	p := Polyline{Pts: []Vec2{V(0, 0), V(10, 0), V(10, 10)}}
	l, ok := OffsetPolyline(p, 2)
	if !ok || len(l.Pts) != 3 {
		t.Fatalf("left = %+v %v", l, ok)
	}
	vNear(t, "l0", l.Pts[0], V(0, 2))
	vNear(t, "l1", l.Pts[1], V(8, 2))
	vNear(t, "l2", l.Pts[2], V(8, 10))
	r, ok := OffsetPolyline(p, -2)
	if !ok || len(r.Pts) != 3 {
		t.Fatalf("right = %+v %v", r, ok)
	}
	vNear(t, "r0", r.Pts[0], V(0, -2))
	vNear(t, "r1", r.Pts[1], V(12, -2))
	vNear(t, "r2", r.Pts[2], V(12, 10))
	if r.Bulges != nil || r.Closed {
		t.Fatalf("straight open result carries bulges/closed: %+v", r)
	}
}

func TestOffsetPolylineClosedSquare(t *testing.T) {
	sq := Polyline{Pts: []Vec2{V(0, 0), V(10, 0), V(10, 10), V(0, 10)}, Closed: true}
	in, ok := OffsetPolyline(sq, 1)
	if !ok || !in.Closed || len(in.Pts) != 4 {
		t.Fatalf("inward = %+v %v", in, ok)
	}
	for i, w := range []Vec2{V(1, 1), V(9, 1), V(9, 9), V(1, 9)} {
		vNear(t, "in", in.Pts[i], w)
	}
	out, ok := OffsetPolyline(sq, -1)
	if !ok {
		t.Fatal("outward failed")
	}
	for i, w := range []Vec2{V(-1, -1), V(11, -1), V(11, 11), V(-1, 11)} {
		vNear(t, "out", out.Pts[i], w)
	}
	if _, ok := OffsetPolyline(sq, 5); ok {
		t.Fatal("collapsing a square to a point should be degenerate")
	}
	if _, ok := OffsetPolyline(sq, 6); ok {
		t.Fatal("inverting a square should be degenerate")
	}
	// Clockwise winding flips which side is "left".
	cw := Polyline{Pts: []Vec2{V(0, 0), V(0, 10), V(10, 10), V(10, 0)}, Closed: true}
	r, ok := OffsetPolyline(cw, 1)
	if !ok {
		t.Fatal("cw failed")
	}
	vNear(t, "cw0", r.Pts[0], V(-1, -1))
}

func TestOffsetPolylineBulgeSemicircle(t *testing.T) {
	// Bulge 1 from (0,0) to (10,0) runs counter-clockwise below the chord,
	// centre (5,0) radius 5.
	p := Polyline{Pts: []Vec2{V(0, 0), V(10, 0)}, Bulges: []float64{1, 0}}
	in, ok := OffsetPolyline(p, 1)
	if !ok {
		t.Fatal("inward failed")
	}
	vNear(t, "in0", in.Pts[0], V(1, 0))
	vNear(t, "in1", in.Pts[1], V(9, 0))
	if math.Abs(in.Bulges[0]-1) > 1e-9 {
		t.Fatalf("bulge = %v, want 1", in.Bulges[0])
	}
	if c, r, _, _ := BulgeArc(in.Pts[0], in.Pts[1], in.Bulges[0]); !c.Near(V(5, 0), 1e-9) || math.Abs(r-4) > 1e-9 {
		t.Fatalf("offset arc centre %v radius %v, want (5,0) r 4", c, r)
	}
	out, ok := OffsetPolyline(p, -1)
	if !ok {
		t.Fatal("outward failed")
	}
	vNear(t, "out0", out.Pts[0], V(-1, 0))
	if _, r, _, _ := BulgeArc(out.Pts[0], out.Pts[1], out.Bulges[0]); math.Abs(r-6) > 1e-9 {
		t.Fatalf("outward radius = %v, want 6", r)
	}
	// Clockwise arc: negative bulge, left is outward.
	cw := Polyline{Pts: []Vec2{V(0, 0), V(10, 0)}, Bulges: []float64{-1, 0}}
	o, ok := OffsetPolyline(cw, 1)
	if !ok {
		t.Fatal("cw failed")
	}
	if _, r, _, _ := BulgeArc(o.Pts[0], o.Pts[1], o.Bulges[0]); math.Abs(r-6) > 1e-9 || o.Bulges[0] >= 0 {
		t.Fatalf("cw left offset radius %v bulge %v, want 6 and negative", r, o.Bulges[0])
	}
}

func TestOffsetPolylineDegenerateArc(t *testing.T) {
	p := Polyline{Pts: []Vec2{V(0, 0), V(10, 0)}, Bulges: []float64{1, 0}}
	if _, ok := OffsetPolyline(p, 5); ok {
		t.Fatal("radius reaching zero should be degenerate")
	}
	if _, ok := OffsetPolyline(p, 7); ok {
		t.Fatal("negative radius should be degenerate")
	}
	if _, ok := OffsetPolyline(Polyline{Pts: []Vec2{V(1, 1)}}, 1); ok {
		t.Fatal("single point should be degenerate")
	}
}

// distToPolyline is the distance from q to the nearest point of p.
func distToPolyline(p Polyline, q Vec2) float64 { return Closest(p, q).Dist(q) }

func TestOffsetPolylineStadiumTangentJoins(t *testing.T) {
	// A counter-clockwise stadium: two lines and two semicircles.
	st := Polyline{Pts: []Vec2{V(0, 0), V(10, 0), V(10, 4), V(0, 4)}, Bulges: []float64{0, 1, 0, 1}, Closed: true}
	in, ok := OffsetPolyline(st, 1)
	if !ok || len(in.Pts) != 4 {
		t.Fatalf("inward = %+v %v", in, ok)
	}
	for i, w := range []Vec2{V(0, 1), V(10, 1), V(10, 3), V(0, 3)} {
		vNear(t, "pt", in.Pts[i], w)
	}
	for i, w := range []float64{0, 1, 0, 1} {
		if math.Abs(in.Bulges[i]-w) > 1e-9 {
			t.Fatalf("bulges = %v", in.Bulges)
		}
	}
	if got, want := polyArea(in), 10*2+math.Pi; math.Abs(got-want) > 1e-4 {
		t.Fatalf("inner area = %v, want %v", got, want)
	}
	for _, d := range []float64{1, -1, -2.5} {
		o, ok := OffsetPolyline(st, d)
		if !ok {
			t.Fatalf("d=%v failed", d)
		}
		for _, q := range o.Tessellate(1e-4, nil) {
			if got := distToPolyline(st, q); math.Abs(got-math.Abs(d)) > 2e-4 {
				t.Fatalf("d=%v: point %v is %v from the source", d, q, got)
			}
		}
	}
}

func TestOffsetPolylineBulgeWithMiter(t *testing.T) {
	// Line then a quarter arc meeting at a corner: the offset lines meet the
	// offset arc at the intersection of their supports.
	p := Polyline{Pts: []Vec2{V(0, 0), V(10, 0), V(15, 5)}, Bulges: []float64{0, math.Tan(math.Pi / 8), 0}}
	// Quarter arc from (10,0) to (15,5), centre (10,5), radius 5, ccw.
	o, ok := OffsetPolyline(p, -1) // right of travel = outward
	if !ok || len(o.Pts) != 3 {
		t.Fatalf("offset = %+v %v", o, ok)
	}
	vNear(t, "o0", o.Pts[0], V(0, -1))
	vNear(t, "o1", o.Pts[1], V(10, -1))
	vNear(t, "o2", o.Pts[2], V(16, 5))
	c, r, _, _ := BulgeArc(o.Pts[1], o.Pts[2], o.Bulges[1])
	if !c.Near(V(10, 5), 1e-9) || math.Abs(r-6) > 1e-9 {
		t.Fatalf("arc centre %v radius %v", c, r)
	}
}

func TestOffsetPolylineDropsInvertedChamfer(t *testing.T) {
	p := Polyline{Pts: []Vec2{V(0, 0), V(10, 0), V(10, 8), V(8, 10), V(0, 10)}, Closed: true}
	o, ok := OffsetPolyline(p, 4)
	if !ok || len(o.Pts) != 4 {
		t.Fatalf("offset = %+v %v", o, ok)
	}
	for i, w := range []Vec2{V(4, 4), V(6, 4), V(6, 6), V(4, 6)} {
		vNear(t, "pt", o.Pts[i], w)
	}
}

func TestOffsetPolylineAntiParallelCollapse(t *testing.T) {
	u := Polyline{Pts: []Vec2{V(0, 0), V(10, 0), V(10, 1), V(0, 1)}}
	if _, ok := OffsetPolyline(u, 2); ok {
		t.Fatal("a channel narrower than twice the offset should be degenerate")
	}
}

func TestOffsetPolylineZero(t *testing.T) {
	p := Polyline{Pts: []Vec2{V(0, 0), V(1, 0)}}
	o, ok := OffsetPolyline(p, 0)
	if !ok || len(o.Pts) != 2 {
		t.Fatalf("zero offset = %+v %v", o, ok)
	}
}
