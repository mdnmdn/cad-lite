package geom_test

import (
	"math"
	"testing"

	"cad-lite/geom"
)

const eps = 1e-9

func near(t *testing.T, name string, got, want geom.Vec2) {
	t.Helper()
	if !got.Near(want, 1e-7) {
		t.Fatalf("%s = %v, want %v", name, got, want)
	}
}

func TestMat3ComposesRightToLeft(t *testing.T) {
	m := geom.Translate(geom.V(10, 0)).Mul(geom.Rotate(geom.V(0, 0), math.Pi/2))
	near(t, "rotate then translate", m.Apply(geom.V(1, 0)), geom.V(10, 1))
	r := geom.Rotate(geom.V(5, 5), math.Pi)
	near(t, "rotate about point", r.Apply(geom.V(6, 5)), geom.V(4, 5))
	s := geom.ScaleAbout(geom.V(1, 1), 2)
	near(t, "scale about point", s.Apply(geom.V(2, 2)), geom.V(3, 3))
}

func TestArcBoundsIncludeQuadrants(t *testing.T) {
	a := geom.Arc{C: geom.V(0, 0), R: 2, Start: 0, End: math.Pi}
	b := a.Bounds()
	if math.Abs(b.Max.Y-2) > eps || math.Abs(b.Min.Y) > eps || math.Abs(b.Min.X+2) > eps {
		t.Fatalf("half-circle bounds = %+v", b)
	}
	full := geom.Arc{C: geom.V(0, 0), R: 1, Start: 1, End: 1}
	if math.Abs(full.Sweep()-2*math.Pi) > eps {
		t.Fatalf("equal angles sweep = %v, want 2π", full.Sweep())
	}
}

func TestArcClosestParamSnapsToNearerEnd(t *testing.T) {
	a := geom.Arc{C: geom.V(0, 0), R: 1, Start: 0, End: math.Pi / 2}
	near(t, "inside", geom.Closest(a, geom.V(2, 2)), geom.V(math.Sqrt2/2, math.Sqrt2/2))
	near(t, "past start", geom.Closest(a, geom.V(1, -0.1)), geom.V(1, 0))
	near(t, "past end", geom.Closest(a, geom.V(-0.1, 1)), geom.V(0, 1))
}

func TestBulgeSemicircle(t *testing.T) {
	c, r, _, sweep := geom.BulgeArc(geom.V(0, 0), geom.V(2, 0), 1)
	near(t, "center", c, geom.V(1, 0))
	if math.Abs(r-1) > eps || math.Abs(sweep-math.Pi) > eps {
		t.Fatalf("r=%v sweep=%v", r, sweep)
	}
	p := geom.Polyline{Pts: []geom.Vec2{{X: 0, Y: 0}, {X: 2, Y: 0}}, Bulges: []float64{1}}
	// A positive bulge turns counter-clockwise: from (0,0) to (2,0) the arc
	// passes below the chord.
	near(t, "arc midpoint", p.PointAt(0.5), geom.V(1, -1))
	if l := p.Length(); math.Abs(l-math.Pi) > 1e-9 {
		t.Fatalf("length = %v, want π", l)
	}
}

func TestTessellationRespectsChordTolerance(t *testing.T) {
	c := geom.Circle{C: geom.V(0, 0), R: 100}
	for _, tol := range []float64{1, 0.1, 0.01} {
		pts := c.Tessellate(tol, nil)
		for i := 1; i < len(pts); i++ {
			mid := pts[i-1].Lerp(pts[i], 0.5)
			if sag := 100 - mid.Len(); sag > tol*1.0001 {
				t.Fatalf("tol %v: sagitta %v at segment %d", tol, sag, i)
			}
		}
	}
}

func TestIntersections(t *testing.T) {
	x := geom.Intersect(geom.Line{A: geom.V(0, 0), B: geom.V(2, 2)}, geom.Line{A: geom.V(0, 2), B: geom.V(2, 0)}, 1e-9)
	if len(x) != 1 {
		t.Fatalf("line/line = %v", x)
	}
	near(t, "line/line", x[0], geom.V(1, 1))
	if x := geom.Intersect(geom.Line{A: geom.V(0, 0), B: geom.V(1, 0)}, geom.Line{A: geom.V(0, 1), B: geom.V(1, 1)}, 1e-9); len(x) != 0 {
		t.Fatalf("parallel lines = %v", x)
	}
	lc := geom.Intersect(geom.Line{A: geom.V(-5, 0), B: geom.V(5, 0)}, geom.Circle{C: geom.V(0, 0), R: 2}, 1e-9)
	if len(lc) != 2 {
		t.Fatalf("line/circle = %v", lc)
	}
	la := geom.Intersect(geom.Line{A: geom.V(-5, 0.5), B: geom.V(5, 0.5)}, geom.Arc{C: geom.V(0, 0), R: 1, Start: 0, End: math.Pi / 2}, 1e-9)
	if len(la) != 1 || la[0].X <= 0 {
		t.Fatalf("line/arc = %v, want the one point on the first quadrant", la)
	}
	cc := geom.Intersect(geom.Circle{C: geom.V(0, 0), R: 1}, geom.Circle{C: geom.V(1, 0), R: 1}, 1e-9)
	if len(cc) != 2 {
		t.Fatalf("circle/circle = %v", cc)
	}
	near(t, "circle/circle", cc[0], geom.V(0.5, math.Sqrt(3)/2))
	poly := geom.Polyline{Pts: []geom.Vec2{{X: 0, Y: 0}, {X: 4, Y: 0}, {X: 4, Y: 4}}}
	if x := geom.Intersect(poly, geom.Line{A: geom.V(2, -1), B: geom.V(2, 1)}, 1e-9); len(x) != 1 {
		t.Fatalf("polyline/line = %v", x)
	}
}

func TestArcTransformMirrorKeepsCounterClockwise(t *testing.T) {
	a := geom.Arc{C: geom.V(0, 0), R: 1, Start: 0, End: math.Pi / 2}
	mirror := geom.Mat3{-1, 0, 0, 1, 0, 0}
	m := a.Transform(mirror).(geom.Arc)
	near(t, "mirrored start", m.StartPoint(), geom.V(0, 1))
	near(t, "mirrored end", m.EndPoint(), geom.V(-1, 0))
}

func TestSegmentIntersectsBox(t *testing.T) {
	box := geom.Box2{Min: geom.V(0, 0), Max: geom.V(1, 1)}
	if !geom.SegmentIntersectsBox(geom.V(-1, 0.5), geom.V(2, 0.5), box) {
		t.Fatal("segment crossing the box missed")
	}
	if geom.SegmentIntersectsBox(geom.V(-1, 2), geom.V(2, 2), box) {
		t.Fatal("segment above the box hit")
	}
}
