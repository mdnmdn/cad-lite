package geom

import "math"

// CutPieces splits c at the cut parameters ts into consecutive pieces, in parameter order: an
// open curve gives len(cuts)+1 pieces, a periodic one (Circle, closed Polyline) gives one piece per
// cut, the last wrapping past the end of the parameter range (B may exceed it, as for SubCurve).
// It returns nil when there is nothing to cut: no cut strictly inside an open curve, fewer than
// two on a periodic one.
func CutPieces(c Curve, ts []float64) []Span {
	t0, t1 := c.ParamRange()
	per := IsPeriodic(c)
	cuts := cutParams(ts, t0, t1, per)
	if len(cuts) == 0 || (per && len(cuts) < 2) {
		return nil
	}
	out := make([]Span, 0, len(cuts)+1)
	if per {
		for i, k := range cuts {
			next := cuts[0] + (t1 - t0)
			if i+1 < len(cuts) {
				next = cuts[i+1]
			}
			out = append(out, Span{k, next})
		}
		return out
	}
	prev := t0
	for _, k := range cuts {
		out = append(out, Span{prev, k})
		prev = k
	}
	return append(out, Span{prev, t1})
}

// KeepRuns returns the spans that remain when the pieces flagged in remove are deleted, with
// neighbouring kept pieces merged into one span (a cut that removes nothing on either side does
// not split the curve). period is the parameter period of a periodic curve, 0 for an open one.
// pieces must come from CutPieces. It returns nil, false when nothing is flagged.
func KeepRuns(pieces []Span, remove []bool, period float64) (keep []Span, removed bool) {
	n := len(pieces)
	first := -1
	for i := 0; i < n && i < len(remove); i++ {
		if remove[i] {
			first = i
			break
		}
	}
	if first < 0 {
		return nil, false
	}
	order := make([]int, 0, n)
	if period > 0 {
		for k := 1; k < n; k++ {
			order = append(order, (first+k)%n)
		}
	} else {
		for i := 0; i < n; i++ {
			order = append(order, i)
		}
	}
	var run *Span
	flush := func() {
		if run != nil {
			keep = append(keep, *run)
			run = nil
		}
	}
	for _, j := range order {
		if remove[j] {
			flush()
			continue
		}
		s := pieces[j]
		if period > 0 && j < first {
			s.A += period
			s.B += period
		}
		if run != nil && math.Abs(s.A-run.B) < ParamEps {
			run.B = s.B
			continue
		}
		flush()
		r := s
		run = &r
	}
	flush()
	return keep, true
}

// ImpliedEdge returns the curves that stand for c when edges are taken at their projection (the
// Edge=Extend option of TRIM and EXTEND): a Line becomes a Line reaching reach beyond both ends, an
// Arc becomes its Circle, and a Polyline keeps its segments with each end segment projected the
// same way. Any other curve is returned as it is.
func ImpliedEdge(c Curve, reach float64) []Curve {
	switch c := c.(type) {
	case Line:
		return []Curve{projectLine(c, reach, true, true)}
	case Arc:
		return []Curve{Circle{c.C, c.R}}
	case Polyline:
		segs := c.Segments()
		out := make([]Curve, 0, len(segs)+2)
		for i, s := range segs {
			out = append(out, s)
			if c.Closed {
				continue
			}
			first, last := i == 0, i == len(segs)-1
			switch s := s.(type) {
			case Line:
				if first || last {
					out = append(out, projectLine(s, reach, first, last))
				}
			case Arc:
				if first || last {
					out = append(out, Circle{s.C, s.R})
				}
			}
		}
		return out
	}
	return []Curve{c}
}

// projectLine extends l by reach before its start and/or after its end.
func projectLine(l Line, reach float64, atStart, atEnd bool) Line {
	dir := l.B.Sub(l.A).Norm()
	if dir == (Vec2{}) {
		return l
	}
	if atStart {
		l.A = l.A.Sub(dir.Scale(reach))
	}
	if atEnd {
		l.B = l.B.Add(dir.Scale(reach))
	}
	return l
}
