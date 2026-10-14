package render_test

import (
	"math"
	"testing"

	"go-draw/geom"
	"go-draw/render"
)

func TestFitIgnoresNonFiniteBoxes(t *testing.T) {
	inf, nan := math.Inf(1), math.NaN()
	boxes := []geom.Box2{
		geom.BoxOf(geom.V(nan, nan), geom.V(1, 1)),
		geom.BoxOf(geom.V(0, 0), geom.V(inf, 1)),
		{Min: geom.V(-1e308, -1e308), Max: geom.V(1e308, 1e308)},
		{Min: geom.V(nan, 0), Max: geom.V(nan, 0)},
	}
	for _, b := range boxes {
		v := render.DefaultView(800, 600)
		want := v
		v.Fit(b, 20)
		if v != want {
			t.Errorf("Fit(%v) changed the view: %+v", b, v)
		}
	}
}

func TestZoomAtIgnoresNonFinite(t *testing.T) {
	v := render.DefaultView(800, 600)
	want := v
	v.ZoomAt(math.NaN(), 0, 2)
	v.ZoomAt(0, 0, math.Inf(1))
	v.ZoomAt(0, 0, math.NaN())
	if v != want {
		t.Errorf("view changed: %+v", v)
	}
}
