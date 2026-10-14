package geom

import (
	"math"
	"testing"
)

// tangentTo reports the distance from t's circle to curve c at full extension: zero when tangent.
func tangentGap(c Curve, t Circle) float64 {
	switch v := c.(type) {
	case Line:
		d := v.B.Sub(v.A).Norm()
		return math.Abs(math.Abs(t.C.Sub(v.A).Cross(d)) - t.R)
	default:
		o, _ := round(c)
		d := t.C.Dist(o.C)
		return math.Min(math.Abs(d-(o.R+t.R)), math.Abs(d-math.Abs(o.R-t.R)))
	}
}

func TestTangentCirclesTwoLines(t *testing.T) {
	a, b := Line{V(0, 0), V(100, 0)}, Line{V(0, 0), V(0, 100)}
	got := TangentCircles(a, b, 10)
	if len(got) != 4 {
		t.Fatalf("got %d circles, want 4: %+v", len(got), got)
	}
	for _, c := range got {
		if tangentGap(a, c) > 1e-9 || tangentGap(b, c) > 1e-9 {
			t.Errorf("%+v is not tangent to both lines", c)
		}
	}
	c, ok := NearestTangentCircle(a, b, V(50, 0), V(0, 50), 10)
	if !ok {
		t.Fatal("no solution")
	}
	vNear(t, "centre", c.C, V(10, 10))
	c, _ = NearestTangentCircle(a, b, V(50, 0), V(0, -50), 10)
	vNear(t, "centre below", c.C, V(10, -10))
	c, _ = NearestTangentCircle(a, b, V(-50, 0), V(0, 50), 10)
	vNear(t, "centre left", c.C, V(-10, 10))
}

func TestTangentCirclesLineAndCircle(t *testing.T) {
	l := Line{V(-100, 0), V(100, 0)}
	circ := Circle{C: V(0, 12), R: 10}
	got := TangentCircles(l, circ, 5)
	if len(got) == 0 {
		t.Fatal("no solutions")
	}
	for _, c := range got {
		if tangentGap(l, c) > 1e-9 || tangentGap(circ, c) > 1e-9 {
			t.Errorf("%+v is not tangent to the line and the circle", c)
		}
	}
	// The same through an arc, which is taken at full extension.
	arc := Arc{C: V(0, 12), R: 10, Start: 0, End: math.Pi / 2}
	if n := len(TangentCircles(l, arc, 5)); n != len(got) {
		t.Errorf("arc gave %d solutions, circle %d", n, len(got))
	}
	// Nearest to a pick on the right of the circle and a pick on the line.
	c, ok := NearestTangentCircle(l, circ, V(30, 0), V(10, 12), 5)
	if !ok || c.C.X <= 0 || c.C.Y != 5 {
		t.Errorf("nearest = %+v, %v", c, ok)
	}
}

func TestTangentCirclesTwoCircles(t *testing.T) {
	a, b := Circle{C: V(0, 0), R: 10}, Circle{C: V(30, 0), R: 10}
	got := TangentCircles(a, b, 10)
	if len(got) == 0 {
		t.Fatal("no solutions")
	}
	for _, c := range got {
		if tangentGap(a, c) > 1e-9 || tangentGap(b, c) > 1e-9 {
			t.Errorf("%+v is not tangent to both circles", c)
		}
	}
	// Externally tangent to both, above the axis: the centre is 20 from each centre.
	c, ok := NearestTangentCircle(a, b, V(7, 7), V(23, 7), 10)
	if !ok {
		t.Fatal("no solution")
	}
	vNear(t, "centre", c.C, V(15, math.Sqrt(175)))
}

func TestTangentCirclesEnclosing(t *testing.T) {
	// A circle bigger than the curve that touches it from outside: the tangent point is on the
	// far side of the small circle.
	o := Circle{C: V(0, 0), R: 5}
	c := Circle{C: V(10, 0), R: 15}
	vNear(t, "enclosing tangent point", TangentPoint(o, c), V(-5, 0))
	vNear(t, "external tangent point", TangentPoint(o, Circle{C: V(15, 0), R: 10}), V(5, 0))
}

func TestTangentCirclesNone(t *testing.T) {
	a, b := Circle{C: V(0, 0), R: 1}, Circle{C: V(100, 0), R: 1}
	if got := TangentCircles(a, b, 1); len(got) != 0 {
		t.Errorf("distant circles with a small radius gave %+v", got)
	}
	if _, ok := NearestTangentCircle(a, b, V(1, 0), V(99, 0), 1); ok {
		t.Error("expected no solution")
	}
	if got := TangentCircles(Line{V(0, 0), V(1, 0)}, Line{V(0, 5), V(1, 5)}, 1); len(got) != 0 {
		t.Errorf("parallel lines 5 apart, r=1: %+v", got)
	}
	if got := TangentCircles(a, b, 0); got != nil {
		t.Errorf("r=0 gave %+v", got)
	}
	if got := TangentCircles(Polyline{Pts: []Vec2{V(0, 0), V(1, 1)}}, a, 1); got != nil {
		t.Errorf("polyline gave %+v", got)
	}
}

func TestTangentCircleBetweenParallelLines(t *testing.T) {
	a, b := Line{V(0, 0), V(100, 0)}, Line{V(0, 10), V(100, 10)}
	if got := TangentCircles(a, b, 5); len(got) != 0 {
		t.Fatalf("parallel family returned %+v", got)
	}
	c, ok := NearestTangentCircle(a, b, V(30, 0), V(50, 10), 5)
	if !ok {
		t.Fatal("no solution")
	}
	vNear(t, "centre", c.C, V(40, 5))
	if _, ok := NearestTangentCircle(a, b, V(30, 0), V(50, 10), 4); ok {
		t.Error("r=4 between lines 10 apart must have no solution")
	}
}
