package geom_test

import (
	"cad-lite/geom"
	"testing"
)

type protocolCurve struct {
	geom.Circle
	handled bool
}

func (c protocolCurve) IntersectCurve(geom.Curve, float64) ([]geom.Vec2, bool) {
	return []geom.Vec2{geom.V(7, 8)}, c.handled
}
func TestOptionalIntersectionProtocol(t *testing.T) {
	line := geom.Line{A: geom.V(-2, 0), B: geom.V(2, 0)}
	for _, reverse := range []bool{false, true} {
		c := protocolCurve{Circle: geom.Circle{R: 1}, handled: true}
		var ps []geom.Vec2
		if reverse {
			ps = geom.Intersect(line, c, 1e-9)
		} else {
			ps = geom.Intersect(c, line, 1e-9)
		}
		if len(ps) != 1 || ps[0] != geom.V(7, 8) {
			t.Fatal(ps)
		}
	}
	if ps := geom.Intersect(geom.Circle{R: 1}, line, 1e-9); len(ps) != 2 {
		t.Fatal("native dispatcher changed", ps)
	}
}
