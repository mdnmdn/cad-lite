package geom_test

import (
	"math"
	"testing"

	"cad-lite/geom"
)

func closeTo(t *testing.T, name string, got, want float64) {
	t.Helper()
	if math.Abs(got-want) > 1e-9 {
		t.Fatalf("%s = %v, want %v", name, got, want)
	}
}

func TestPolygonAreaSign(t *testing.T) {
	sq := []geom.Vec2{geom.V(0, 0), geom.V(4, 0), geom.V(4, 3), geom.V(0, 3)}
	closeTo(t, "ccw", geom.PolygonArea(sq), 12)
	closeTo(t, "cw", geom.PolygonArea([]geom.Vec2{sq[3], sq[2], sq[1], sq[0]}), -12)
}

func TestPolylineAreaCircleFromBulges(t *testing.T) {
	p := geom.Polyline{Pts: []geom.Vec2{geom.V(5, 0), geom.V(-5, 0)}, Bulges: []float64{1, 1}, Closed: true}
	closeTo(t, "circle", geom.PolylineArea(p), math.Pi*25)
	p.Bulges = []float64{-1, -1}
	closeTo(t, "cw circle", geom.PolylineArea(p), -math.Pi*25)
}

func TestPolylineAreaStadium(t *testing.T) {
	p := geom.Polyline{
		Pts:    []geom.Vec2{geom.V(0, 0), geom.V(10, 0), geom.V(10, 4), geom.V(0, 4)},
		Bulges: []float64{0, 1, 0, 1},
		Closed: true,
	}
	closeTo(t, "stadium", geom.PolylineArea(p), 40+4*math.Pi)
}

func TestPolylineAreaInwardBulge(t *testing.T) {
	p := geom.Polyline{
		Pts:    []geom.Vec2{geom.V(0, 0), geom.V(10, 0), geom.V(10, 4), geom.V(0, 4)},
		Bulges: []float64{0, -1, 0, 0},
		Closed: true,
	}
	closeTo(t, "dent", geom.PolylineArea(p), 40-2*2*math.Pi/2*1)
}

func TestPolylineAreaOpenClosedByChord(t *testing.T) {
	p := geom.Polyline{Pts: []geom.Vec2{geom.V(0, 0), geom.V(4, 0), geom.V(4, 3)}, Bulges: []float64{0, 0, 5}}
	closeTo(t, "triangle", geom.PolylineArea(p), 6)
	closeTo(t, "half disc", geom.PolylineArea(geom.Polyline{Pts: []geom.Vec2{geom.V(0, 0), geom.V(2, 0)}, Bulges: []float64{1}}), math.Pi/2)
	closeTo(t, "single point", geom.PolylineArea(geom.Polyline{Pts: []geom.Vec2{geom.V(1, 1)}}), 0)
}

func TestArcSegmentArea(t *testing.T) {
	closeTo(t, "semicircle", geom.ArcSegmentArea(2, math.Pi), math.Pi*2)
	closeTo(t, "quarter", geom.ArcSegmentArea(1, math.Pi/2), math.Pi/4-0.5)
}
