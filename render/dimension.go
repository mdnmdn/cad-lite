package render

import (
	"cad-lite/doc"
	"cad-lite/geom"
)

// dimensionItems draws a dimension from its layout (doc.Dimension.Layout) in the colours of its
// style: the extension lines in DIMCLRE, the dimension line, its arc and the arrowheads in
// DIMCLRD, the text and the arc length symbol in DIMCLRT, each resolved against the dimension's
// own appearance (ByBlock is its colour). The dimension and extension lines take its linetype, as
// AutoCAD's DIMLTYPE and DIMLTEX1/2 default to ByBlock; arrowheads and text draw solid.
func (b *Builder) dimensionItems(h doc.Handle, e *doc.Dimension, band int) []DisplayItem {
	app := b.d.ResolveEntity(e)
	st := e.Style()
	lay := e.Layout()
	base := b.style(app, 1)
	in := func(aci int) strokeStyle {
		s := base
		s.color = b.color(b.d.ResolveColorIn(aci, app))
		return s
	}
	ext, line, text := in(st.ExtColor), in(st.LineColor), in(st.TextColor)
	var items []DisplayItem
	for _, l := range lay.ExtLines {
		items = b.appendCurve(items, h, l, ext, band)
	}
	if e.Kind == doc.DimArcLength {
		items = b.appendCurve(items, h, lay.DimArc, line, band)
	}
	for _, l := range lay.DimLine {
		items = b.appendCurve(items, h, l, line, band)
	}
	arrow := line
	arrow.pattern = nil
	for _, a := range lay.Arrows {
		items = arrowItems(items, h, a, arrow, band)
	}
	text.pattern = nil
	if lay.Symbol.R > 0 {
		items = b.appendCurve(items, h, lay.Symbol, text, band)
	}
	if lay.Text != nil {
		items = b.textItems(items, h, lay.Text, text, band)
	}
	return items
}

// arrowItems draws one arrowhead as doc.DimArrow describes it: its outline (filled when Filled),
// its lines (as Width-wide bars when Width is set), circles (filled dots) and arcs. Every filled
// shape also gets its outline, so an arrow under a pixel still shows.
func arrowItems(items []DisplayItem, h doc.Handle, a doc.DimArrow, s strokeStyle, band int) []DisplayItem {
	tol := BandTolerance(band)
	fill := func(pts []geom.Vec2) {
		items = append(items, DisplayItem{Owner: h, Kind: ItemFill, Points: pts, Color: s.color})
	}
	stroke := func(pts []geom.Vec2, closed bool) {
		items = append(items, DisplayItem{Owner: h, Kind: ItemPolyline, Points: pts, Color: s.color, Weight: s.weight, Closed: closed})
	}
	if len(a.Outline) > 0 {
		if a.Filled {
			fill(a.Outline)
		}
		stroke(a.Outline, true)
	}
	for _, l := range a.Lines {
		if a.Width > 0 {
			off := l.B.Sub(l.A).Norm().Perp().Scale(a.Width / 2)
			bar := []geom.Vec2{l.A.Add(off), l.B.Add(off), l.B.Sub(off), l.A.Sub(off)}
			fill(bar)
			stroke(bar, true)
			continue
		}
		stroke([]geom.Vec2{l.A, l.B}, false)
	}
	for _, c := range a.Circles {
		pts := c.Tessellate(tol, nil)
		if a.Filled {
			fill(pts)
		}
		stroke(pts, false)
	}
	for _, c := range a.Arcs {
		stroke(c.Tessellate(tol, nil), false)
	}
	return items
}
