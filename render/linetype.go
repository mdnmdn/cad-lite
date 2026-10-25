package render

import (
	"math"
	"slices"
	"strings"

	"cad-lite/doc"
	"cad-lite/geom"
)

// This file applies linetype dash patterns to tessellated curves, following AutoCAD's display
// rules: the pattern runs in model units (its lengths times the entity's linetype scale times
// LTSCALE, times the scale of an enclosing block reference), starts afresh on every entity and on
// every polyline segment (PLINEGEN 0), and an open run is laid out so that it begins and ends
// with a dash; a run shorter than one repetition, and a pattern too dense to see at the current
// zoom band, draw solid.

// minPeriodPx is the pattern repetition, in pixels at the lowest scale of a zoom band, below
// which a linetype draws solid: its dashes and gaps would blur into a grey, dotted smear that
// costs many strokes to draw (AutoCAD likewise regenerates such lines continuous).
const minPeriodPx = 2

// dotPx is the length, in pixels at the lowest scale of a zoom band, a dot (a 0 pattern element)
// draws with, so that dots stay visible at any zoom.
const dotPx = 1

// maxDashes caps the dash and dot elements one run may draw. A run that would need more (a line
// many screens long under a fine pattern) draws solid, which keeps the per-band cache bounded.
const maxDashes = 1 << 15

// pattern returns the dash pattern linetype name draws with, or nil for a solid line: CONTINUOUS,
// a name missing from the table, a pattern without gaps, and a complex linetype (doc.Linetype.Raw,
// shapes or text the document does not model) are solid. The result is cached until the next
// sync and must not be modified.
func (b *Builder) pattern(name string) []float64 {
	key := strings.ToUpper(name)
	if p, ok := b.patterns[key]; ok {
		return p
	}
	var p []float64
	if l, ok := b.d.Linetype(name); ok && l.Raw == nil && !l.Continuous() && l.Length() > 0 {
		p = l.Pattern
	}
	b.patterns[key] = p
	return p
}

// dasher lays a dash pattern along polylines. A builder keeps one and reuses its scratch
// storage across curves.
type dasher struct {
	// pat is the pattern scaled to model units; period its repetition length.
	pat    []float64
	period float64
	// dot is the model length a dot draws with.
	dot float64
	// buf holds the points of every run laid since the last flush; starts the index in buf where
	// each run begins.
	buf    []geom.Vec2
	starts []int32
	// cum and spans are scratch: cumulative lengths, and the dashes of one run.
	cum   []float64
	spans [][2]float64
}

// reset scales pattern by scale for a zoom band whose lowest scale is pxPerUnit pixels per model
// unit, and reports false when the line draws solid: no pattern, a non-positive or non-finite
// scale, or a period under minPeriodPx.
func (d *dasher) reset(pattern []float64, scale, pxPerUnit float64) bool {
	if len(pattern) == 0 || !(scale > 0) || math.IsInf(scale, 0) {
		return false
	}
	d.pat, d.period, d.dot = d.pat[:0], 0, dotPx/pxPerUnit
	for _, v := range pattern {
		d.pat = append(d.pat, v*scale)
		d.period += math.Abs(v * scale)
	}
	return d.period*pxPerUnit >= minPeriodPx && !math.IsInf(d.period, 0)
}

// solid adds pts as one undashed run.
func (d *dasher) solid(pts []geom.Vec2) {
	d.starts = append(d.starts, int32(len(d.buf)))
	d.buf = append(d.buf, pts...)
}

// run lays the pattern along the polyline pts, closed when it is a loop (a circle, a closed
// curve), and reports false, adding nothing, when the run draws solid: shorter than one
// repetition, or needing more than maxDashes elements.
func (d *dasher) run(pts []geom.Vec2, closed bool) bool {
	if len(pts) < 2 {
		return false
	}
	d.cum = append(d.cum[:0], 0)
	for i := 1; i < len(pts); i++ {
		d.cum = append(d.cum, d.cum[i-1]+pts[i].Dist(pts[i-1]))
	}
	total := d.cum[len(d.cum)-1]
	if !(total >= d.period) || total/d.period*float64(len(d.pat)) > maxDashes {
		return false
	}
	spans := d.spans[:0]
	// Open runs with a leading dash are centred, as AutoCAD lays them: n repetitions and a
	// closing first dash, the slack shared by the first and last dash so both ends are dashes.
	// A run with room for one repetition but not for the closing dash shortens both end dashes
	// equally instead.
	d0 := d.pat[0]
	if !closed && d0 > 0 {
		n := max(1, math.Floor((total-d0)/d.period))
		off := (total - n*d.period - d0) / 2
		for k := 0.0; k < n; k++ {
			spans = d.appendRep(spans, k*d.period+off, total)
		}
		spans = append(spans, [2]float64{n*d.period + off, total})
		spans[0][0] = 0
		spans = mergeSpans(spans)
	} else {
		for k := 0.0; k*d.period < total; k++ {
			spans = d.appendRep(spans, k*d.period, total)
		}
	}
	d.extract(pts, spans)
	d.spans = spans[:0]
	return true
}

// appendRep appends the dashes and dots of one repetition starting at arc length u, clipped to
// [0, total].
func (d *dasher) appendRep(spans [][2]float64, u, total float64) [][2]float64 {
	for _, v := range d.pat {
		switch {
		case v > 0:
			spans = appendSpan(spans, u, u+v, total)
		case v == 0:
			spans = appendSpan(spans, u-d.dot/2, u+d.dot/2, total)
		}
		u += math.Abs(v)
	}
	return spans
}

func appendSpan(spans [][2]float64, a, b, total float64) [][2]float64 {
	a, b = max(a, 0), min(b, total)
	if b < a || a >= total {
		return spans
	}
	return append(spans, [2]float64{a, b})
}

// mergeSpans joins spans that touch or overlap (a pattern ending in a dash meets the next
// repetition's first dash), in place.
func mergeSpans(spans [][2]float64) [][2]float64 {
	out := spans[:0]
	for _, s := range spans {
		if n := len(out); n > 0 && s[0] <= out[n-1][1] {
			out[n-1][1] = max(out[n-1][1], s[1])
			continue
		}
		out = append(out, s)
	}
	return out
}

// extract appends the part of pts between the arc lengths of each span (ascending) to buf.
func (d *dasher) extract(pts []geom.Vec2, spans [][2]float64) {
	at := func(i int, s float64) geom.Vec2 {
		seg := d.cum[i+1] - d.cum[i]
		if seg <= 0 {
			return pts[i]
		}
		return pts[i].Lerp(pts[i+1], (s-d.cum[i])/seg)
	}
	i := 0
	last := len(pts) - 2
	for _, sp := range spans {
		for i < last && d.cum[i+1] <= sp[0] {
			i++
		}
		d.starts = append(d.starts, int32(len(d.buf)))
		d.buf = append(d.buf, at(i, sp[0]))
		for i < last && d.cum[i+1] < sp[1] {
			i++
			d.buf = append(d.buf, pts[i])
		}
		end := at(i, sp[1])
		if sp[1] == sp[0] {
			// A zero-length span (a dot at a run's end) still draws: a point-sized stroke.
			end = end.Add(geom.V(d.dot*1e-3, 0))
		}
		d.buf = append(d.buf, end)
	}
}

// isClosedCurve reports whether c is a loop: a circle, or a curve that ends where it starts.
func isClosedCurve(c geom.Curve, pts []geom.Vec2, tol float64) bool {
	switch c := c.(type) {
	case geom.Circle:
		return true
	case geom.Polyline:
		return c.Closed
	}
	return len(pts) > 2 && pts[0].Dist(pts[len(pts)-1]) <= tol
}

// strokeStyle is how one curve draws: its colour, width and dash pattern.
type strokeStyle struct {
	color  Color
	weight float32
	// pattern is the unscaled dash pattern (nil solid) and ltScale the scale applied to it.
	pattern []float64
	ltScale float64
}

// appendCurve appends c as display items owned by h: one polyline, or one item whose runs
// (DisplayItem.Starts) are the dashes when the style's pattern applies. A polyline restarts the
// pattern on every segment (PLINEGEN 0).
func (b *Builder) appendCurve(items []DisplayItem, h doc.Handle, c geom.Curve, s strokeStyle, band int) []DisplayItem {
	tol := BandTolerance(band)
	d := &b.dash
	if !d.reset(s.pattern, s.ltScale, math.Exp2(float64(band))) {
		return append(items, DisplayItem{Owner: h, Kind: ItemPolyline, Points: c.Tessellate(tol, nil), Color: s.color, Weight: s.weight})
	}
	d.buf, d.starts = d.buf[:0], d.starts[:0]
	if p, isPoly := c.(geom.Polyline); isPoly && p.NumSegments() > 1 {
		var pts []geom.Vec2
		for i := range p.NumSegments() {
			bu := 0.0
			if i < len(p.Bulges) {
				bu = p.Bulges[i]
			}
			seg := geom.Polyline{Pts: []geom.Vec2{p.Pts[i], p.Pts[(i+1)%len(p.Pts)]}, Bulges: []float64{bu}}
			pts = seg.Tessellate(tol, pts[:0])
			if !d.run(pts, false) {
				d.solid(pts)
			}
		}
	} else {
		pts := c.Tessellate(tol, nil)
		if !d.run(pts, isClosedCurve(c, pts, tol*1e-3)) {
			return append(items, DisplayItem{Owner: h, Kind: ItemPolyline, Points: pts, Color: s.color, Weight: s.weight})
		}
	}
	it := DisplayItem{Owner: h, Kind: ItemPolyline, Points: slices.Clone(d.buf), Color: s.color, Weight: s.weight}
	if len(d.starts) > 1 {
		it.Starts = slices.Clone(d.starts)
	}
	return append(items, it)
}
