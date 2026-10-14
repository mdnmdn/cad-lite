package geom

import (
	"math"
	"testing"
)

func TestCutPiecesOpenAndPeriodic(t *testing.T) {
	l := Line{V(0, 0), V(10, 0)}
	ps := CutPieces(l, []float64{0.3, 0.7, 0.3, 0, 1})
	if len(ps) != 3 || ps[0] != (Span{0, 0.3}) || ps[2] != (Span{0.7, 1}) {
		t.Fatalf("line pieces = %v", ps)
	}
	if CutPieces(l, []float64{0, 1}) != nil {
		t.Fatal("cuts at the ends are not cuts")
	}
	c := Circle{V(0, 0), 5}
	if CutPieces(c, []float64{1}) != nil {
		t.Fatal("a circle needs two cuts")
	}
	cp := CutPieces(c, []float64{4, 1})
	if len(cp) != 2 || cp[0] != (Span{1, 4}) || math.Abs(cp[1].B-(1+2*math.Pi)) > 1e-12 {
		t.Fatalf("circle pieces = %v", cp)
	}
}

func TestKeepRunsMergesUntouchedNeighbours(t *testing.T) {
	l := Line{V(0, 0), V(10, 0)}
	ps := CutPieces(l, []float64{0.25, 0.5, 0.75})
	keep, ok := KeepRuns(ps, []bool{false, false, true, false}, 0)
	if !ok || len(keep) != 2 || keep[0] != (Span{0, 0.5}) || keep[1] != (Span{0.75, 1}) {
		t.Fatalf("keep = %v", keep)
	}
	if _, ok := KeepRuns(ps, make([]bool, 4), 0); ok {
		t.Fatal("nothing flagged must report false")
	}
}

func TestKeepRunsPeriodic(t *testing.T) {
	c := Circle{V(0, 0), 5}
	ps := CutPieces(c, []float64{1, 2, 4})
	// Remove the piece 2..4: the other two join across the wrap into one arc from 4 to 2+2pi.
	keep, ok := KeepRuns(ps, []bool{false, true, false}, 2*math.Pi)
	if !ok || len(keep) != 1 || math.Abs(keep[0].A-4) > 1e-12 || math.Abs(keep[0].B-(2+2*math.Pi)) > 1e-12 {
		t.Fatalf("keep = %v", keep)
	}
	a, _ := SubCurve(c, keep[0]).(Arc)
	if math.Abs(a.Sweep()-(2*math.Pi-2)) > 1e-9 {
		t.Fatalf("arc sweep = %v", a.Sweep())
	}
}

func TestImpliedEdge(t *testing.T) {
	got := ImpliedEdge(Line{V(0, 0), V(10, 0)}, 100)
	if l, ok := got[0].(Line); !ok || !l.A.Near(V(-100, 0), 1e-9) || !l.B.Near(V(110, 0), 1e-9) {
		t.Fatalf("line = %v", got)
	}
	if c, ok := ImpliedEdge(Arc{C: V(1, 2), R: 3, Start: 0, End: 1}, 100)[0].(Circle); !ok || c.R != 3 {
		t.Fatal("an arc stands for its circle")
	}
	pl := Polyline{Pts: []Vec2{V(0, 0), V(10, 0), V(10, 10)}}
	got = ImpliedEdge(pl, 100)
	if len(got) != 4 {
		t.Fatalf("open polyline: %d curves, want 2 segments + 2 projections", len(got))
	}
	pl.Closed = true
	if got = ImpliedEdge(pl, 100); len(got) != 3 {
		t.Fatalf("closed polyline: %d curves", len(got))
	}
}
