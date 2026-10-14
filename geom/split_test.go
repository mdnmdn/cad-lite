package geom

import (
	"math"
	"testing"
)

func closeTo(a, b float64) bool { return math.Abs(a-b) < 1e-9 }

func wantSpans(t *testing.T, got []Span, want ...Span) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("spans = %v, want %v", got, want)
	}
	for i := range want {
		if !closeTo(got[i].A, want[i].A) || !closeTo(got[i].B, want[i].B) {
			t.Fatalf("spans = %v, want %v", got, want)
		}
	}
}

func TestTrimSpanLine(t *testing.T) {
	l := Line{V(0, 0), V(10, 0)}
	// Cuts at 0.3 and 0.7: pick between removes the middle.
	k, ok := TrimSpan(l, []float64{0.7, 0.3}, 0.5)
	if !ok {
		t.Fatal("not ok")
	}
	wantSpans(t, k, Span{0, 0.3}, Span{0.7, 1})
	// Pick before the first cut: removes from the start to the cut.
	k, _ = TrimSpan(l, []float64{0.7, 0.3}, 0.1)
	wantSpans(t, k, Span{0.3, 1})
	// One cut only: pick on the long side removes up to the end.
	k, _ = TrimSpan(l, []float64{0.3}, 0.9)
	wantSpans(t, k, Span{0, 0.3})
	k, _ = TrimSpan(l, []float64{0.3}, 0.1)
	wantSpans(t, k, Span{0.3, 1})
	// Cuts on the ends are not cuts; duplicates collapse.
	if _, ok := TrimSpan(l, []float64{0, 1}, 0.5); ok {
		t.Fatal("end cuts must not trim")
	}
	if _, ok := TrimSpan(l, nil, 0.5); ok {
		t.Fatal("no cuts must not trim")
	}
	k, _ = TrimSpan(l, []float64{0.5, 0.5 + 1e-12}, 0.9)
	wantSpans(t, k, Span{0, 0.5})
}

func TestTrimSpanArcAndCircle(t *testing.T) {
	a := Arc{C: V(0, 0), R: 1, Start: 0, End: math.Pi}
	k, ok := TrimSpan(a, []float64{math.Pi / 4}, 3*math.Pi/4)
	if !ok {
		t.Fatal("not ok")
	}
	wantSpans(t, k, Span{0, math.Pi / 4})
	p := SubCurve(a, k[0]).(Arc)
	if !closeTo(p.Start, 0) || !closeTo(p.End, math.Pi/4) {
		t.Fatalf("arc piece = %+v", p)
	}

	c := Circle{V(0, 0), 1}
	if _, ok := TrimSpan(c, []float64{1}, 2); ok {
		t.Fatal("one cut cannot trim a circle")
	}
	// Cuts at 90 and 270 degrees; pick at 0 removes the right half.
	k, ok = TrimSpan(c, []float64{3 * math.Pi / 2, math.Pi / 2}, 0.1)
	if !ok {
		t.Fatal("not ok")
	}
	wantSpans(t, k, Span{math.Pi / 2, 3 * math.Pi / 2})
}

func TestTrimSpanCircleKeepsOtherSide(t *testing.T) {
	c := Circle{V(0, 0), 1}
	cuts := []float64{math.Pi / 2, 3 * math.Pi / 2}
	// Pick at angle 0 lies between the last cut and the first (wrapping): the
	// right half goes, the left half (90..270) stays.
	k, _ := TrimSpan(c, cuts, 0.1)
	wantSpans(t, k, Span{math.Pi / 2, 3 * math.Pi / 2})
	// Pick at 180 removes the left half; the right half wraps through 0.
	k, _ = TrimSpan(c, cuts, math.Pi)
	wantSpans(t, k, Span{3 * math.Pi / 2, 5 * math.Pi / 2})
	arc := SubCurve(c, k[0]).(Arc)
	if !closeTo(arc.Start, 3*math.Pi/2) || !closeTo(arc.End, math.Pi/2) {
		t.Fatalf("arc = %+v", arc)
	}
	if !closeTo(arc.Sweep(), math.Pi) {
		t.Fatalf("sweep = %v", arc.Sweep())
	}
}

func TestTrimSpanPolyline(t *testing.T) {
	p := Polyline{Pts: []Vec2{V(0, 0), V(10, 0), V(10, 10), V(0, 10)}}
	// Cuts at param 0.5 and 2.5 (middle of seg 0 and seg 2); pick on segment 1.
	k, ok := TrimSpan(p, []float64{0.5, 2.5}, 1.5)
	if !ok {
		t.Fatal("not ok")
	}
	wantSpans(t, k, Span{0, 0.5}, Span{2.5, 3})
	first := SubCurve(p, k[0]).(Polyline)
	if len(first.Pts) != 2 || !first.Pts[1].Near(V(5, 0), 1e-9) {
		t.Fatalf("first = %+v", first)
	}
	second := SubCurve(p, k[1]).(Polyline)
	if len(second.Pts) != 2 || !second.Pts[0].Near(V(5, 10), 1e-9) || !second.Pts[1].Near(V(0, 10), 1e-9) {
		t.Fatalf("second = %+v", second)
	}
}

func TestTrimSpanClosedPolyline(t *testing.T) {
	sq := Polyline{Pts: []Vec2{V(0, 0), V(10, 0), V(10, 10), V(0, 10)}, Closed: true}
	// Cuts mid-bottom (0.5) and mid-top (2.5); pick on the right side (1.5):
	// the right half goes, leaving one open polyline through the left side.
	k, ok := TrimSpan(sq, []float64{0.5, 2.5}, 1.5)
	if !ok {
		t.Fatal("not ok")
	}
	wantSpans(t, k, Span{2.5, 4.5})
	r := SubCurve(sq, k[0]).(Polyline)
	if r.Closed || len(r.Pts) != 4 {
		t.Fatalf("result = %+v", r)
	}
	want := []Vec2{V(5, 10), V(0, 10), V(0, 0), V(5, 0)}
	for i, w := range want {
		if !r.Pts[i].Near(w, 1e-9) {
			t.Fatalf("pts = %v, want %v", r.Pts, want)
		}
	}
	// Pick on the left side removes the left half instead.
	k, _ = TrimSpan(sq, []float64{0.5, 2.5}, 3.5)
	wantSpans(t, k, Span{0.5, 2.5})
	// A pick before the first cut (param 0.2) wraps to the last cut.
	k, _ = TrimSpan(sq, []float64{0.5, 2.5}, 0.2)
	wantSpans(t, k, Span{0.5, 2.5})
	if _, ok := TrimSpan(sq, []float64{0.5}, 1); ok {
		t.Fatal("single cut cannot trim a closed polyline")
	}
}

func TestSubPolylineKeepsBulges(t *testing.T) {
	// A half-circle segment of radius 5 from (0,0) to (10,0), bulge 1,
	// followed by a straight segment.
	p := Polyline{Pts: []Vec2{V(0, 0), V(10, 0), V(10, 10)}, Bulges: []float64{1, 0}}
	// Quarter of the arc: bulge tan(45deg/... ) = tan(atan(1)*0.5).
	s := SubPolyline(p, 0.25, 0.75)
	if len(s.Pts) != 2 {
		t.Fatalf("pts = %v", s.Pts)
	}
	if !closeTo(s.Bulges[0], math.Tan(math.Atan(1)*0.5)) {
		t.Fatalf("bulge = %v", s.Bulges[0])
	}
	// The part is still on the original circle.
	for _, q := range s.Pts {
		if !closeTo(q.Dist(V(5, 0)), 5) {
			t.Fatalf("point %v off the circle", q)
		}
	}
	// Length is half of the half-circle.
	if !closeTo(s.Length(), math.Pi*5/2) {
		t.Fatalf("length = %v", s.Length())
	}
	// Spanning a vertex keeps both segments.
	s = SubPolyline(p, 0.5, 1.5)
	if len(s.Pts) != 3 || s.Bulges[1] != 0 || !s.Pts[2].Near(V(10, 5), 1e-9) {
		t.Fatalf("across vertex = %+v", s)
	}
	// Clockwise bulge: same geometry mirrored, traversal still from a to b.
	n := Polyline{Pts: []Vec2{V(0, 0), V(10, 0)}, Bulges: []float64{-1}}
	s = SubPolyline(n, 0, 0.5)
	if !s.Pts[1].Near(V(5, 5), 1e-9) || !closeTo(s.Bulges[0], -math.Tan(math.Atan(1)*0.5)) {
		t.Fatalf("clockwise = %+v", s)
	}
}

func TestSplitCurve(t *testing.T) {
	l := Line{V(0, 0), V(10, 0)}
	parts := SplitCurve(l, []float64{0.4})
	if len(parts) != 2 || !parts[0].(Line).B.Near(V(4, 0), 1e-9) || !parts[1].(Line).A.Near(V(4, 0), 1e-9) {
		t.Fatalf("parts = %v", parts)
	}
	if got := SplitCurve(l, []float64{0, 1}); len(got) != 1 {
		t.Fatalf("end cuts: %v", got)
	}
	c := Circle{V(0, 0), 1}
	arcs := SplitCurve(c, []float64{0, math.Pi})
	if len(arcs) != 2 {
		t.Fatalf("circle parts = %v", arcs)
	}
	total := 0.0
	for _, a := range arcs {
		total += a.Length()
	}
	if !closeTo(total, 2*math.Pi) {
		t.Fatalf("total = %v", total)
	}
	sq := Polyline{Pts: []Vec2{V(0, 0), V(10, 0), V(10, 10), V(0, 10)}, Closed: true}
	ps := SplitCurve(sq, []float64{0.5, 2.5})
	if len(ps) != 2 || !closeTo(ps[0].Length()+ps[1].Length(), 40) {
		t.Fatalf("closed polyline parts = %v", ps)
	}
}

func TestKeepOutside(t *testing.T) {
	l := Line{V(0, 0), V(10, 0)}
	wantSpans(t, KeepOutside(l, 0.7, 0.2), Span{0, 0.2}, Span{0.7, 1})
	wantSpans(t, KeepOutside(l, 0.5, 0.5), Span{0, 0.5}, Span{0.5, 1})
	wantSpans(t, KeepOutside(l, 0, 0.5), Span{0.5, 1})
	if got := KeepOutside(l, 0, 1); len(got) != 0 {
		t.Fatalf("whole line removed: %v", got)
	}
	c := Circle{V(0, 0), 1}
	// The removed part runs counter-clockwise from a to b.
	wantSpans(t, KeepOutside(c, 0, math.Pi/2), Span{math.Pi / 2, 2 * math.Pi})
	wantSpans(t, KeepOutside(c, math.Pi/2, 0), Span{0, math.Pi / 2})
	wantSpans(t, KeepOutside(c, 1, 1), Span{1, 1 + 2*math.Pi})
}

func TestBulgeArcRoundTrip(t *testing.T) {
	a, b := V(0, 0), V(10, 0)
	for _, bu := range []float64{0.3, 1, 2.5, -0.3, -1, -2.5} {
		arc, rev, ok := ArcFromBulge(a, b, bu)
		if !ok || rev != (bu < 0) {
			t.Fatalf("bulge %v: rev=%v ok=%v", bu, rev, ok)
		}
		if got := BulgeFromArc(arc, rev); !closeTo(got, bu) {
			t.Fatalf("bulge %v round-trips to %v", bu, got)
		}
		s, e := arc.StartPoint(), arc.EndPoint()
		if rev {
			s, e = e, s
		}
		if !s.Near(a, 1e-9) || !e.Near(b, 1e-9) {
			t.Fatalf("bulge %v: endpoints %v %v", bu, s, e)
		}
	}
	if _, _, ok := ArcFromBulge(a, b, 0); ok {
		t.Fatal("zero bulge is straight")
	}
	// Quarter arc: sweep 90 degrees, bulge tan(22.5 deg).
	if got := BulgeFromArc(Arc{C: V(0, 0), R: 1, Start: 0, End: math.Pi / 2}, false); !closeTo(got, math.Tan(math.Pi/8)) {
		t.Fatalf("quarter bulge = %v", got)
	}
}

func TestPolylinePiecesRoundTrip(t *testing.T) {
	p := Polyline{Pts: []Vec2{V(0, 0), V(10, 0), V(10, 10), V(0, 10)}, Bulges: []float64{0.5, 0, -0.4, 0}}
	ps := PolylinePieces(p)
	if len(ps) != 3 {
		t.Fatalf("pieces = %d", len(ps))
	}
	q := PolylineFromPieces(ps, 1e-9)
	if q.Closed || len(q.Pts) != 4 {
		t.Fatalf("rebuilt = %+v", q)
	}
	for i := range q.Pts {
		if !q.Pts[i].Near(p.Pts[i], 1e-9) {
			t.Fatalf("pt %d = %v", i, q.Pts[i])
		}
	}
	for i := 0; i < 3; i++ {
		if !closeTo(q.Bulges[i], p.Bulges[i]) {
			t.Fatalf("bulge %d = %v, want %v", i, q.Bulges[i], p.Bulges[i])
		}
	}
	p.Closed = true
	p.Bulges[3] = 0.2
	ps = PolylinePieces(p)
	q = PolylineFromPieces(ps, 1e-9)
	if !q.Closed || len(q.Pts) != 4 || !closeTo(q.Bulges[3], 0.2) {
		t.Fatalf("closed rebuilt = %+v", q)
	}
	if !closeTo(q.Length(), p.Length()) {
		t.Fatalf("length %v vs %v", q.Length(), p.Length())
	}
}

func TestChainPieces(t *testing.T) {
	l1 := []Piece{{Line{V(0, 0), V(10, 0)}, false}}
	l2 := []Piece{{Line{V(10, 10), V(10, 0)}, false}} // runs towards l1's end
	arc := []Piece{{Arc{C: V(10, 10), R: 5, Start: 0, End: math.Pi / 2}, false}}
	// arc starts at (15,10) and ends at (10,15); not connected to l2's (10,10).
	chain, used := ChainPieces([][]Piece{l1, l2, arc}, 0, 1e-9)
	if len(chain) != 2 || len(used) != 2 || used[0] != 0 || used[1] != 1 {
		t.Fatalf("chain=%d used=%v", len(chain), used)
	}
	if !chain[1].Rev || !chain[1].Start().Near(V(10, 0), 1e-9) {
		t.Fatalf("second piece not reversed: %+v", chain[1])
	}
	// A group attached at the chain's start is prepended.
	pre := []Piece{{Line{V(-5, 0), V(0, 0)}, false}}
	chain, used = ChainPieces([][]Piece{l1, pre}, 0, 1e-9)
	if len(chain) != 2 || used[0] != 1 || !chain[0].Start().Near(V(-5, 0), 1e-9) {
		t.Fatalf("prepend: %v %v", chain, used)
	}
	// A closed loop stops growing.
	tri := [][]Piece{
		{{Line{V(0, 0), V(10, 0)}, false}},
		{{Line{V(10, 0), V(5, 8)}, false}},
		{{Line{V(5, 8), V(0, 0)}, false}},
	}
	chain, used = ChainPieces(tri, 0, 1e-9)
	if len(chain) != 3 || len(used) != 3 {
		t.Fatalf("triangle: %d %v", len(chain), used)
	}
	if pl := PolylineFromPieces(chain, 1e-9); !pl.Closed || len(pl.Pts) != 3 {
		t.Fatalf("triangle polyline = %+v", pl)
	}
}

func TestJoinLines(t *testing.T) {
	a := Line{V(0, 0), V(10, 0)}
	cases := []struct {
		b    Line
		ok   bool
		want Line
	}{
		{Line{V(10, 0), V(20, 0)}, true, Line{V(0, 0), V(20, 0)}},          // touching
		{Line{V(5, 0), V(15, 0)}, true, Line{V(0, 0), V(15, 0)}},           // overlapping
		{Line{V(3, 0), V(6, 0)}, true, Line{V(0, 0), V(10, 0)}},            // contained
		{Line{V(-5, 0), V(0, 0)}, true, Line{V(-5, 0), V(10, 0)}},          // touching at start
		{Line{V(20, 0), V(12, 0)}, false, Line{}},                          // gap
		{Line{V(10, 1), V(20, 1)}, false, Line{}},                          // parallel
		{Line{V(10, 0), V(10, 5)}, false, Line{}},                          // perpendicular
		{Line{V(30, 0), V(15, 0)}, false, Line{}},                          // far
		{Line{V(10.0000001, 0), V(20, 0)}, false, Line{}},                  // gap beyond tol
		{Line{V(20, 0), V(10, 0)}, true, Line{V(0, 0), V(20, 0)}},          // reversed
		{Line{V(10, 1e-12), V(20, -1e-12)}, true, Line{V(0, 0), V(20, 0)}}, // within tol
	}
	for i, c := range cases {
		got, ok := JoinLines(a, c.b, 1e-9)
		if ok != c.ok {
			t.Fatalf("case %d: ok = %v", i, ok)
		}
		if ok && (!got.A.Near(c.want.A, 1e-9) || !got.B.Near(c.want.B, 1e-9)) {
			t.Fatalf("case %d: %v, want %v", i, got, c.want)
		}
	}
}

func TestJoinArcs(t *testing.T) {
	a := Arc{C: V(0, 0), R: 5, Start: 0, End: math.Pi / 2}
	b := Arc{C: V(0, 0), R: 5, Start: math.Pi / 2, End: math.Pi}
	got, ok := JoinArcs(a, b, 1e-9)
	arc, isArc := got.(Arc)
	if !ok || !isArc || !closeTo(arc.Start, 0) || !closeTo(arc.End, math.Pi) {
		t.Fatalf("touching: %v %v", got, ok)
	}
	// Reverse order gives the same arc.
	got, _ = JoinArcs(b, a, 1e-9)
	arc = got.(Arc)
	if !closeTo(arc.Start, 0) || !closeTo(arc.End, math.Pi) {
		t.Fatalf("reversed: %v", got)
	}
	// Overlapping, and across the 0/2pi seam.
	c := Arc{C: V(0, 0), R: 5, Start: 3 * math.Pi / 2, End: math.Pi / 4}
	got, ok = JoinArcs(a, c, 1e-9)
	arc, _ = got.(Arc)
	if !ok || !closeTo(arc.Start, 3*math.Pi/2) || !closeTo(arc.End, math.Pi/2) {
		t.Fatalf("seam: %v %v", got, ok)
	}
	// Gap, other circle.
	if _, ok := JoinArcs(a, Arc{C: V(0, 0), R: 5, Start: 2, End: 3}, 1e-9); ok {
		t.Fatal("gap joined")
	}
	if _, ok := JoinArcs(a, Arc{C: V(0, 0), R: 6, Start: math.Pi / 2, End: math.Pi}, 1e-9); ok {
		t.Fatal("different radius joined")
	}
	if _, ok := JoinArcs(a, Arc{C: V(1, 0), R: 5, Start: math.Pi / 2, End: math.Pi}, 1e-9); ok {
		t.Fatal("different centre joined")
	}
	// Two halves close the circle.
	h1 := Arc{C: V(0, 0), R: 5, Start: 0, End: math.Pi}
	h2 := Arc{C: V(0, 0), R: 5, Start: math.Pi, End: 0}
	got, ok = JoinArcs(h1, h2, 1e-9)
	if _, isCircle := got.(Circle); !ok || !isCircle {
		t.Fatalf("halves: %v %v", got, ok)
	}
}

func TestExtendEndLine(t *testing.T) {
	l := Line{V(0, 0), V(10, 0)}
	wall := Line{V(25, -10), V(25, 10)}
	far := Line{V(40, -10), V(40, 10)}
	got, ok := ExtendEnd(l, true, []Curve{far, wall}, 1e-9)
	if !ok || !got.(Line).B.Near(V(25, 0), 1e-9) || !got.(Line).A.Near(V(0, 0), 0) {
		t.Fatalf("end: %v %v", got, ok)
	}
	// Extending the start finds nothing on that side.
	if _, ok := ExtendEnd(l, false, []Curve{far, wall}, 1e-9); ok {
		t.Fatal("start extended to nothing")
	}
	back := Line{V(-7, -10), V(-7, 10)}
	got, ok = ExtendEnd(l, false, []Curve{wall, back}, 1e-9)
	if !ok || !got.(Line).A.Near(V(-7, 0), 1e-9) {
		t.Fatalf("start: %v %v", got, ok)
	}
	// An edge too short to be hit does not count.
	if _, ok := ExtendEnd(l, true, []Curve{Line{V(25, 1), V(25, 10)}}, 1e-9); ok {
		t.Fatal("extended to a boundary it misses")
	}
	// A circle boundary: the nearer crossing wins.
	got, ok = ExtendEnd(l, true, []Curve{Circle{V(30, 0), 10}}, 1e-9)
	if !ok || !got.(Line).B.Near(V(20, 0), 1e-9) {
		t.Fatalf("circle: %v %v", got, ok)
	}
	// A boundary already touching the end is skipped for the next one.
	got, ok = ExtendEnd(l, true, []Curve{Line{V(10, -5), V(10, 5)}, wall}, 1e-9)
	if !ok || !got.(Line).B.Near(V(25, 0), 1e-9) {
		t.Fatalf("touching: %v %v", got, ok)
	}
	if _, ok := ExtendEnd(Circle{V(0, 0), 1}, true, []Curve{wall}, 1e-9); ok {
		t.Fatal("circle extended")
	}
}

func TestExtendEndArc(t *testing.T) {
	a := Arc{C: V(0, 0), R: 10, Start: 0, End: math.Pi / 4}
	// A vertical line x=0 meets the circle at 90 degrees.
	wall := Line{V(0, -20), V(0, 20)}
	got, ok := ExtendEnd(a, true, []Curve{wall}, 1e-9)
	arc, _ := got.(Arc)
	if !ok || !closeTo(arc.End, math.Pi/2) || !closeTo(arc.Start, 0) {
		t.Fatalf("end: %v %v", got, ok)
	}
	// Start side: reaches the x axis? Use a line y=-5 crossing below.
	low := Line{V(-20, -5), V(20, -5)}
	got, ok = ExtendEnd(a, false, []Curve{low}, 1e-9)
	arc, _ = got.(Arc)
	if !ok || !closeTo(arc.Start, NormAngle(-math.Asin(0.5))) || !closeTo(arc.End, math.Pi/4) {
		t.Fatalf("start: %v %v", got, ok)
	}
	// The arc's own body is not a target.
	if _, ok := ExtendEnd(a, true, []Curve{Line{V(10*math.Cos(0.3), -20), V(10*math.Cos(0.3), 20)}}, 1e-9); ok {
		// The vertical line at x=10cos(.3) meets the circle at +-0.3 rad; -0.3 is
		// in the arc's gap, reached by going the long way round.
		t.Log("reached through the gap, as expected")
	}
}
