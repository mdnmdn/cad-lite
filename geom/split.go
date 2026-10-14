package geom

import (
	"math"
	"slices"
)

// ParamEps is the parameter distance below which two cut parameters are the
// same cut and a span is empty. Parameters are angles (Circle, Arc), segment
// fractions (Line) or segment indices plus a fraction (Polyline).
const ParamEps = 1e-9

// Span is a closed parameter interval [A, B] of a curve with A < B. On a
// periodic curve B may exceed the end of the parameter range: the span then
// wraps through the start.
type Span struct{ A, B float64 }

// IsPeriodic reports whether c is a closed curve whose parameter wraps: a
// Circle or a closed Polyline.
func IsPeriodic(c Curve) bool {
	switch c := c.(type) {
	case Circle:
		return true
	case Polyline:
		return c.Closed && c.NumSegments() > 0
	}
	return false
}

// wrapParam reduces t into [t0, t0+period).
func wrapParam(t, t0, period float64) float64 {
	t = t0 + math.Mod(t-t0, period)
	if t < t0 {
		t += period
	}
	if t >= t0+period-ParamEps {
		t = t0
	}
	return t
}

// cutParams returns ts sorted and de-duplicated. A periodic curve wraps
// them into [t0, t1); an open curve keeps only those strictly inside it.
func cutParams(ts []float64, t0, t1 float64, periodic bool) []float64 {
	out := make([]float64, 0, len(ts))
	for _, t := range ts {
		switch {
		case math.IsNaN(t):
		case periodic:
			out = append(out, wrapParam(t, t0, t1-t0))
		case t > t0+ParamEps && t < t1-ParamEps:
			out = append(out, t)
		}
	}
	slices.Sort(out)
	return slices.CompactFunc(out, func(a, b float64) bool { return math.Abs(b-a) < ParamEps })
}

// TrimSpan computes the pieces of c that survive trimming at the point of
// parameter pick, given the parameters ts where cutting edges cross c. The
// removed piece runs between the two cuts around pick, or from the one cut
// to the end of an open curve when pick has a cut on one side only. A cut
// at an end of an open curve is not a cut. A periodic curve needs two
// distinct cuts. ok is false when there is nothing to trim. A pick on a cut
// removes the piece after it.
func TrimSpan(c Curve, ts []float64, pick float64) (keep []Span, ok bool) {
	t0, t1 := c.ParamRange()
	per := IsPeriodic(c)
	cuts := cutParams(ts, t0, t1, per)
	if len(cuts) == 0 || (per && len(cuts) < 2) {
		return nil, false
	}
	if per {
		pick = wrapParam(pick, t0, t1-t0)
	}
	idx := 0
	for _, k := range cuts {
		if k < pick-ParamEps {
			idx++
		}
	}
	var lo, hi float64
	switch {
	case per && (idx == 0 || idx == len(cuts)):
		lo, hi = cuts[len(cuts)-1], cuts[0]
	case per:
		lo, hi = cuts[idx-1], cuts[idx]
	default:
		lo, hi = t0, t1
		if idx > 0 {
			lo = cuts[idx-1]
		}
		if idx < len(cuts) {
			hi = cuts[idx]
		}
	}
	return KeepOutside(c, lo, hi), true
}

// KeepOutside returns the spans of c that remain when the part between the
// parameters a and b is removed. On an open curve a and b may come in either
// order and the result is the piece before the first and the piece after the
// second (empty pieces dropped); a == b splits the curve there. On a
// periodic curve the removed part runs forward from a to b (counter-
// clockwise for a Circle) and a single wrapping span remains; a == b removes
// nothing and returns the whole curve opened at a.
func KeepOutside(c Curve, a, b float64) []Span {
	t0, t1 := c.ParamRange()
	if IsPeriodic(c) {
		p := t1 - t0
		a, b = wrapParam(a, t0, p), wrapParam(b, t0, p)
		switch {
		case math.Abs(b-a) < ParamEps:
			return []Span{{a, a + p}}
		case b < a:
			return []Span{{b, a}}
		}
		return []Span{{b, a + p}}
	}
	if a > b {
		a, b = b, a
	}
	var out []Span
	if a-t0 > ParamEps {
		out = append(out, Span{t0, a})
	}
	if t1-b > ParamEps {
		out = append(out, Span{b, t1})
	}
	return out
}

// SubCurve returns the part of c over s: a Line from a Line, an Arc from an
// Arc or a Circle, an open Polyline from a Polyline (bulges kept, a closed
// polyline's span may wrap). It returns nil for a degenerate span.
func SubCurve(c Curve, s Span) Curve {
	if s.B-s.A < ParamEps {
		return nil
	}
	switch c := c.(type) {
	case Line:
		return Line{c.PointAt(s.A), c.PointAt(s.B)}
	case Arc:
		return Arc{C: c.C, R: c.R, Start: NormAngle(s.A), End: NormAngle(s.B)}
	case Circle:
		return Arc{C: c.C, R: c.R, Start: NormAngle(s.A), End: NormAngle(s.B)}
	case Polyline:
		if p := SubPolyline(c, s.A, s.B); len(p.Pts) >= 2 {
			return p
		}
	}
	return nil
}

// SplitCurve splits c at the parameters ts: an open curve into len(ts)+1
// pieces (fewer when cuts sit on its ends), a periodic one into len(ts)
// arcs or open polylines. Cuts are sorted and de-duplicated first.
func SplitCurve(c Curve, ts []float64) []Curve {
	t0, t1 := c.ParamRange()
	per := IsPeriodic(c)
	cuts := cutParams(ts, t0, t1, per)
	var spans []Span
	switch {
	case len(cuts) == 0:
		return []Curve{c}
	case per:
		for i, k := range cuts {
			next := cuts[(i+1)%len(cuts)]
			if i == len(cuts)-1 {
				next += t1 - t0
			}
			spans = append(spans, Span{k, next})
		}
	default:
		prev := t0
		for _, k := range cuts {
			spans = append(spans, Span{prev, k})
			prev = k
		}
		spans = append(spans, Span{prev, t1})
	}
	var out []Curve
	for _, s := range spans {
		if sc := SubCurve(c, s); sc != nil {
			out = append(out, sc)
		}
	}
	return out
}

// pointOnSegment returns the point a fraction f along segment i of p, in the
// direction of travel (so a clockwise bulge runs backwards round its arc).
func (p Polyline) pointOnSegment(i int, f float64) Vec2 {
	s := p.Segment(i)
	if arc, ok := s.(Arc); ok && p.bulge(i) < 0 {
		return segPoint(arc, 1-f)
	}
	return segPoint(s, f)
}

// subBulge returns the bulge of the part of a bulged segment covering the
// fraction df of its angle.
func subBulge(bulge, df float64) float64 {
	if bulge == 0 {
		return 0
	}
	return math.Tan(math.Atan(bulge) * df)
}

// SubPolyline returns the part of p between parameters a < b as an open
// polyline. On a closed polyline b may exceed NumSegments(), wrapping past
// the start.
func SubPolyline(p Polyline, a, b float64) Polyline {
	n := p.NumSegments()
	if n == 0 || b-a < ParamEps {
		return Polyline{}
	}
	out := Polyline{}
	cur := a
	var lastSeg int
	var lastF float64
	for cur < b-ParamEps {
		i := int(math.Floor(cur + ParamEps))
		end := math.Min(b, float64(i+1))
		f0, f1 := cur-float64(i), end-float64(i)
		seg := i % n
		out.Pts = append(out.Pts, p.pointOnSegment(seg, f0))
		out.Bulges = append(out.Bulges, subBulge(p.bulge(seg), f1-f0))
		lastSeg, lastF = seg, f1
		cur = end
	}
	if len(out.Pts) == 0 {
		return Polyline{}
	}
	out.Pts = append(out.Pts, p.pointOnSegment(lastSeg, lastF))
	out.Bulges = append(out.Bulges, 0)
	return out
}

// ArcFromBulge returns the counter-clockwise arc of the bulged segment from
// a to b. rev is true when the segment runs clockwise (negative bulge), that
// is from the arc's end to its start. ok is false for a straight or
// degenerate segment.
func ArcFromBulge(a, b Vec2, bulge float64) (arc Arc, rev, ok bool) {
	if bulge == 0 || a.Near(b, 0) {
		return Arc{}, false, false
	}
	c, r, a0, sweep := BulgeArc(a, b, bulge)
	if sweep > 0 {
		return Arc{C: c, R: r, Start: NormAngle(a0), End: NormAngle(a0 + sweep)}, false, true
	}
	return Arc{C: c, R: r, Start: NormAngle(a0 + sweep), End: NormAngle(a0)}, true, true
}

// BulgeFromArc returns the bulge of the polyline segment that traverses arc
// from Start to End, or from End to Start when rev is set.
func BulgeFromArc(arc Arc, rev bool) float64 {
	b := math.Tan(arc.Sweep() / 4)
	if rev {
		return -b
	}
	return b
}

// Piece is a Line or an Arc in a chain, traversed forwards or, when Rev is
// set, from its end to its start.
type Piece struct {
	Curve Curve
	Rev   bool
}

func curveEnds(c Curve) (a, b Vec2) {
	switch c := c.(type) {
	case Line:
		return c.A, c.B
	case Arc:
		return c.StartPoint(), c.EndPoint()
	}
	t0, t1 := c.ParamRange()
	return c.PointAt(t0), c.PointAt(t1)
}

// Start returns where the piece begins in the direction of travel.
func (p Piece) Start() Vec2 {
	a, b := curveEnds(p.Curve)
	if p.Rev {
		return b
	}
	return a
}

// End returns where the piece ends in the direction of travel.
func (p Piece) End() Vec2 {
	a, b := curveEnds(p.Curve)
	if p.Rev {
		return a
	}
	return b
}

// Bulge returns the bulge of the piece as a polyline segment: 0 for a Line.
func (p Piece) Bulge() float64 {
	if a, ok := p.Curve.(Arc); ok {
		return BulgeFromArc(a, p.Rev)
	}
	return 0
}

// ReversePieces returns ps traversed in the opposite direction.
func ReversePieces(ps []Piece) []Piece {
	out := make([]Piece, len(ps))
	for i, p := range ps {
		out[len(ps)-1-i] = Piece{p.Curve, !p.Rev}
	}
	return out
}

// PolylinePieces returns the segments of p as pieces, in order, skipping
// zero-length ones.
func PolylinePieces(p Polyline) []Piece {
	var out []Piece
	for i := 0; i < p.NumSegments(); i++ {
		a, b := p.Pts[i], p.Pts[(i+1)%len(p.Pts)]
		if a.Near(b, 0) {
			continue
		}
		if arc, rev, ok := ArcFromBulge(a, b, p.bulge(i)); ok {
			out = append(out, Piece{arc, rev})
			continue
		}
		out = append(out, Piece{Line{a, b}, false})
	}
	return out
}

// PolylineFromPieces builds the polyline that follows ps, which must be
// connected end to start within tol. It is closed when the last piece ends
// where the first starts.
func PolylineFromPieces(ps []Piece, tol float64) Polyline {
	if len(ps) == 0 {
		return Polyline{}
	}
	out := Polyline{}
	for _, p := range ps {
		out.Pts = append(out.Pts, p.Start())
		out.Bulges = append(out.Bulges, p.Bulge())
	}
	last := ps[len(ps)-1].End()
	if len(ps) > 1 && last.Near(out.Pts[0], tol) {
		out.Closed = true
		return out
	}
	out.Pts = append(out.Pts, last)
	out.Bulges = append(out.Bulges, 0)
	return out
}

// ChainPieces links groups of pieces end to end, starting from groups[first]
// and growing at both ends, reversing groups as needed; ends within tol
// meet. It returns the chain and the indices of the groups used, in chain
// order. Growth stops when the chain closes on itself. Empty groups are
// never used.
func ChainPieces(groups [][]Piece, first int, tol float64) (chain []Piece, used []int) {
	chain = slices.Clone(groups[first])
	used = []int{first}
	taken := map[int]bool{first: true}
	for len(chain) > 0 {
		if len(chain) > 1 && chain[0].Start().Near(chain[len(chain)-1].End(), tol) {
			break
		}
		grew := false
		end, start := chain[len(chain)-1].End(), chain[0].Start()
		for j, g := range groups {
			if taken[j] || len(g) == 0 {
				continue
			}
			gs, ge := g[0].Start(), g[len(g)-1].End()
			switch {
			case gs.Near(end, tol):
				chain = append(chain, g...)
				used = append(used, j)
			case ge.Near(end, tol):
				chain = append(chain, ReversePieces(g)...)
				used = append(used, j)
			case ge.Near(start, tol):
				chain = append(slices.Clone(g), chain...)
				used = append([]int{j}, used...)
			case gs.Near(start, tol):
				chain = append(ReversePieces(g), chain...)
				used = append([]int{j}, used...)
			default:
				continue
			}
			taken[j], grew = true, true
			break
		}
		if !grew {
			break
		}
	}
	return chain, used
}

// JoinLines merges two collinear lines that overlap or touch within tol into
// the one line spanning both, running the way a does.
func JoinLines(a, b Line, tol float64) (Line, bool) {
	d := a.B.Sub(a.A)
	l := d.Len()
	if l <= tol {
		return Line{}, false
	}
	u := d.Scale(1 / l)
	if math.Abs(u.Cross(b.A.Sub(a.A))) > tol || math.Abs(u.Cross(b.B.Sub(a.A))) > tol {
		return Line{}, false
	}
	tb0, tb1 := u.Dot(b.A.Sub(a.A)), u.Dot(b.B.Sub(a.A))
	if tb0 > tb1 {
		tb0, tb1 = tb1, tb0
	}
	if tb0 > l+tol || tb1 < -tol {
		return Line{}, false
	}
	lo, hi := math.Min(0, tb0), math.Max(l, tb1)
	return Line{a.A.Add(u.Scale(lo)), a.A.Add(u.Scale(hi))}, true
}

// JoinArcs merges two arcs of the same circle that overlap or touch within
// tol into one arc, or into the Circle when together they close it.
func JoinArcs(a, b Arc, tol float64) (Curve, bool) {
	if !a.C.Near(b.C, tol) || math.Abs(a.R-b.R) > tol || a.R <= 0 {
		return nil, false
	}
	at := tol / a.R
	sa, sb := a.Sweep(), b.Sweep()
	d := NormAngle(b.Start - a.Start)
	e := NormAngle(a.Start - b.Start)
	aHasB := d <= sa+at || d >= 2*math.Pi-at
	bHasA := e <= sb+at || e >= 2*math.Pi-at
	var start, sweep float64
	switch {
	case aHasB && bHasA:
		// Each reaches the other's start: together they go all the way round.
		return Circle{a.C, a.R}, true
	case aHasB:
		start, sweep = a.Start, math.Max(sa, d+sb)
	case bHasA:
		start, sweep = b.Start, math.Max(sb, e+sa)
	default:
		return nil, false
	}
	if sweep >= 2*math.Pi-at {
		return Circle{a.C, a.R}, true
	}
	return Arc{C: a.C, R: a.R, Start: NormAngle(start), End: NormAngle(start + sweep)}, true
}

// ExtendEnd extends one end of a Line or an Arc (its end when atEnd is set,
// else its start) to the nearest point at which the extension meets one of
// boundaries. An Arc extends along its own circle, never past its other
// end. It returns false when c is neither, or nothing lies ahead.
func ExtendEnd(c Curve, atEnd bool, boundaries []Curve, tol float64) (Curve, bool) {
	switch c := c.(type) {
	case Line:
		return extendLine(c, atEnd, boundaries, tol)
	case Arc:
		return extendArc(c, atEnd, boundaries, tol)
	}
	return nil, false
}

func extendLine(l Line, atEnd bool, boundaries []Curve, tol float64) (Curve, bool) {
	from, dir := l.B, l.B.Sub(l.A).Norm()
	if !atEnd {
		from, dir = l.A, l.A.Sub(l.B).Norm()
	}
	if dir == (Vec2{}) || len(boundaries) == 0 {
		return nil, false
	}
	box := BoxOf(from)
	for _, b := range boundaries {
		box = box.Union(b.Bounds())
	}
	w, h := box.Size()
	ray := Line{from, from.Add(dir.Scale(math.Hypot(w, h)*1.01 + 1))}
	best, found := math.Inf(1), false
	var hit Vec2
	for _, b := range boundaries {
		for _, p := range Intersect(ray, b, tol) {
			if d := p.Sub(from).Dot(dir); d > tol && d < best {
				best, hit, found = d, p, true
			}
		}
	}
	if !found {
		return nil, false
	}
	if atEnd {
		return Line{l.A, hit}, true
	}
	return Line{hit, l.B}, true
}

func extendArc(a Arc, atEnd bool, boundaries []Curve, tol float64) (Curve, bool) {
	if a.R <= tol {
		return nil, false
	}
	circ := Circle{a.C, a.R}
	gap := 2*math.Pi - a.Sweep()
	at := tol / a.R
	best, found := math.Inf(1), false
	for _, b := range boundaries {
		for _, p := range Intersect(circ, b, tol) {
			ang := p.Sub(a.C).Angle()
			d := NormAngle(ang - a.End)
			if !atEnd {
				d = NormAngle(a.Start - ang)
			}
			if d > 2*math.Pi-at {
				d = 0
			}
			if d <= at || d > gap+at {
				continue
			}
			if d < best {
				best, found = d, true
			}
		}
	}
	if !found {
		return nil, false
	}
	if atEnd {
		return Arc{C: a.C, R: a.R, Start: a.Start, End: NormAngle(a.End + best)}, true
	}
	return Arc{C: a.C, R: a.R, Start: NormAngle(a.Start - best), End: a.End}, true
}
