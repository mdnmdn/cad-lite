package geom

import (
	"math"
	"testing"
)

func TestCutPolylineSkipsArcSegmentsAndOverlaps(t *testing.T) {
	// A corner next to a bulged segment is left alone.
	p := Polyline{Pts: []Vec2{{0, 0}, {10, 0}, {10, 10}}, Bulges: []float64{0.5, 0}}
	if _, _, n, _ := CutPolyline(p, nil, FilletCut(1)); n != 0 {
		t.Fatalf("cut %d corners beside an arc segment", n)
	}
	// Two corners on a short segment: the second does not fit.
	q := Polyline{Pts: []Vec2{{0, 0}, {10, 0}, {10, 2}, {0, 2}}}
	out, _, n, skipped := CutPolyline(q, nil, FilletCut(1.5))
	if n != 1 || skipped != 1 || len(out.Pts) != 5 {
		t.Fatalf("n=%d skipped=%d pts=%d", n, skipped, len(out.Pts))
	}
}

func TestChamferAngleDistance(t *testing.T) {
	d, ok := ChamferAngleDistance(math.Pi/2, 10, math.Pi/4)
	if !ok || math.Abs(d-10) > 1e-9 {
		t.Fatalf("d = %v ok = %v", d, ok)
	}
	if _, ok := ChamferAngleDistance(math.Pi/2, 10, math.Pi/2); ok {
		t.Fatal("a chamfer at 90 degrees never meets the second line")
	}
}
