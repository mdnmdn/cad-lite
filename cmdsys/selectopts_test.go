package cmdsys

import (
	"testing"

	"cad-lite/geom"
)

func v(x, y float64) geom.Vec2 { return geom.Vec2{X: x, Y: y} }

func TestPointInPolygon(t *testing.T) {
	sq := []geom.Vec2{v(0, 0), v(10, 0), v(10, 10), v(0, 10)}
	if !PointInPolygon(v(5, 5), sq) || PointInPolygon(v(15, 5), sq) {
		t.Fatal("square")
	}
	// A U shape: the notch is outside.
	u := []geom.Vec2{v(0, 0), v(30, 0), v(30, 30), v(20, 30), v(20, 10), v(10, 10), v(10, 30), v(0, 30)}
	if PointInPolygon(v(15, 20), u) || !PointInPolygon(v(5, 20), u) {
		t.Fatal("concave")
	}
}

func TestSegmentsIntersect(t *testing.T) {
	cases := []struct {
		a, b, c, d geom.Vec2
		want       bool
	}{
		{v(0, 0), v(10, 10), v(0, 10), v(10, 0), true},
		{v(0, 0), v(10, 0), v(0, 1), v(10, 1), false},
		{v(0, 0), v(10, 0), v(10, 0), v(10, 5), true}, // touching end
		{v(0, 0), v(10, 0), v(5, 0), v(15, 0), true},  // collinear overlap
		{v(0, 0), v(4, 0), v(5, 0), v(15, 0), false},  // collinear apart
	}
	for i, c := range cases {
		if got := SegmentsIntersect(c.a, c.b, c.c, c.d); got != c.want {
			t.Errorf("case %d: got %v", i, got)
		}
	}
}

func TestPathPolygonFence(t *testing.T) {
	sq := []geom.Vec2{v(0, 0), v(10, 0), v(10, 10), v(0, 10)}
	inside := [][]geom.Vec2{{v(2, 2), v(8, 8)}}
	cross := [][]geom.Vec2{{v(5, 5), v(20, 5)}}
	outside := [][]geom.Vec2{{v(20, 20), v(30, 30)}}
	through := [][]geom.Vec2{{v(-5, 5), v(15, 5)}} // no vertex inside, crosses both sides
	if !PathsInsidePolygon(inside, sq) || PathsInsidePolygon(cross, sq) || PathsInsidePolygon(outside, sq) {
		t.Error("inside")
	}
	if !PathsCrossPolygon(inside, sq) || !PathsCrossPolygon(cross, sq) || PathsCrossPolygon(outside, sq) || !PathsCrossPolygon(through, sq) {
		t.Error("crossing")
	}
	// A concave notch: both ends inside, the middle leaves the polygon.
	u := []geom.Vec2{v(0, 0), v(30, 0), v(30, 30), v(20, 30), v(20, 10), v(10, 10), v(10, 30), v(0, 30)}
	if PathsInsidePolygon([][]geom.Vec2{{v(5, 20), v(25, 20)}}, u) {
		t.Error("segment across the notch is not inside")
	}
	fence := []geom.Vec2{v(0, 5), v(10, 5), v(10, 15)}
	if !PathsCrossFence([][]geom.Vec2{{v(5, 0), v(5, 10)}}, fence) || PathsCrossFence([][]geom.Vec2{{v(20, 0), v(20, 10)}}, fence) {
		t.Error("fence")
	}
	if !PathsCrossFence([][]geom.Vec2{{v(5, 12), v(15, 12)}}, fence) {
		t.Error("second fence segment")
	}
	// A fence is open: it does not close back to its start.
	if PathsCrossFence([][]geom.Vec2{{v(5, 8), v(5, 12)}}, []geom.Vec2{v(0, 0), v(10, 0), v(10, 10)}) {
		t.Error("fence must not close")
	}
}
