package commands_test

import (
	"math"
	"testing"

	"cad-lite/cmdsys"
	"cad-lite/geom"
)

func TestNonFiniteInputRejected(t *testing.T) {
	p := &cmdsys.Prompt{Accepts: cmdsys.InputPoint | cmdsys.InputDistance}
	for _, in := range []string{"nan,nan", "1,inf", "-inf,0", "1e999,0", "@nan,1", "nan<0", "5<nan"} {
		if got, err := cmdsys.ParseInput(p, in, nil, geom.V(0, 0)); err == nil {
			t.Errorf("%q parsed: %+v", in, got)
		}
	}
}

func TestScriptNaNKeepsViewFinite(t *testing.T) {
	for _, script := range []string{
		"LINE nan,nan 1,1  ZOOM E",
		"CIRCLE 0,0 inf  ZOOM E",
		"CIRCLE 0,0 1e308  ZOOM E",
		"LINE 0,0 1e308,1e308  ZOOM E",
	} {
		s := newSession(t)
		s.Script(script)
		v := s.View()
		for _, f := range []float64{v.Center.X, v.Center.Y, v.Scale} {
			if math.IsNaN(f) || math.IsInf(f, 0) {
				t.Errorf("%q: view %+v", script, *v)
				break
			}
		}
		for _, h := range s.Drawing().Handles() {
			b := s.Drawing().Entity(h).Bounds()
			if math.IsNaN(b.Min.X) || math.IsInf(b.Max.X, 0) {
				t.Errorf("%q: non-finite entity %v", script, b)
			}
		}
	}
}
