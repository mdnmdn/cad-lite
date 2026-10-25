package ui

import (
	"math"
	"slices"

	"cad-lite/cmdsys/props"
	"cad-lite/doc"
	"cad-lite/geom"
	"cad-lite/internal/gpui"
	"cad-lite/render"
)

// Sample metrics of the Properties panel, in logical pixels: a colour swatch is a square, the
// linetype, lineweight and arrow samples a strip sampleFieldW wide in a row's field, where the
// value's text needs the room, and sampleListW wide in a drop-down's items, where a dash preview
// needs it (two repeats of an ISO pattern at a few pixels a dash).
const (
	sampleSwatch = 11
	sampleFieldW = 18
	sampleListW  = 48
	sampleStripH = 12
)

// sampleColor is the colour a swatch of ACI index aci shows: as the canvas draws it, so ACI 7 is
// black on a light canvas and white on a dark one.
func sampleColor(aci int) gpui.Rgba {
	if aci == 7 {
		c := colCanvas
		if 299*int(c.R)+587*int(c.G)+114*int(c.B) > 127500 {
			return gpuiColor(render.Color{A: 255})
		}
		return gpuiColor(render.Color{R: 255, G: 255, B: 255, A: 255})
	}
	return gpuiColor(render.ACI(aci))
}

// colorSwatch is the square swatch of ACI index aci, outlined so white and black read on either
// theme. id names it for tests ("" for none).
func colorSwatch(id string, aci int) *gpui.Div {
	d := gpui.NewDiv().SizeWH(sampleSwatch, sampleSwatch).Rounded(2).FlexShrink(0).Bg(sampleColor(aci)).Border(1, uiBorder)
	if id != "" {
		d.ID(id)
	}
	return d
}

// sampleElement returns the swatch s asks for, or nil when it asks for none: a colour square, a
// dash preview of the linetype, a bar as thick as the lineweight or the arrowhead, the strips
// stripW wide. id prefixes the element's id ("<id>-color"...), which tests look for.
func sampleElement(id string, s props.Sample, stripW float32) gpui.IntoElement {
	switch {
	case s.HasArrow:
		return sampleStrip(id+"-arrow", stripW, func(b gpui.Bounds, w *gpui.Window) { paintArrowSample(w, b, s.Arrow) })
	case s.Linetype != "":
		pat := s.Pattern
		return sampleStrip(id+"-linetype", stripW, func(b gpui.Bounds, w *gpui.Window) { paintDashSample(w, b, pat) })
	case s.Lineweight != 0:
		return lineweightBar(id+"-lineweight", s.Lineweight, stripW)
	case s.ACI > 0:
		return colorSwatch(id+"-color", s.ACI)
	}
	return nil
}

// sampleStrip is a stripW-wide box that paint draws into.
func sampleStrip(id string, stripW float32, paint func(gpui.Bounds, *gpui.Window)) *gpui.Div {
	c := gpui.NewCanvas(func(b gpui.Bounds, w *gpui.Window, _ *gpui.App) { paint(b, w) })
	c.Absolute().Inset(0)
	return gpui.NewDiv().ID(id).Relative().SizeWH(stripW, sampleStripH).FlexShrink(0).Child(c)
}

// lineweightThickness is the bar thickness that stands for lw: proportional to the width, from a
// 1 px hairline for 0.00 mm to 8 px for 2.11 mm.
func lineweightThickness(lw doc.Lineweight) float32 {
	if lw == doc.LineweightZero || lw <= 0 {
		return 1
	}
	return min(8, max(1, 1+float32(lw)/100*3.3))
}

// lineweightBar is a stripW-wide horizontal bar whose thickness stands for lw.
func lineweightBar(id string, lw doc.Lineweight, stripW float32) *gpui.Div {
	return gpui.NewDiv().ID(id).FlexCol().JustifyCenter().SizeWH(stripW, sampleStripH).FlexShrink(0).
		Child(gpui.NewDiv().SizeH(lineweightThickness(lw)).Rounded(0.5).Bg(uiText))
}

// pt is a window point.
func pt(x, y float32) gpui.Point { return gpui.Point{X: gpui.Pixels(x), Y: gpui.Pixels(y)} }

// strokeLine paints one segment.
func strokeLine(w *gpui.Window, a, b gpui.Point, width float32, c gpui.Rgba) {
	w.PaintStroke(gpui.Stroke{Points: []gpui.Point{a, b}, Width: gpui.Pixels(width), Color: c})
}

// paintDashSample draws a linetype's pattern across b: a positive length is a dash, a negative
// one a gap and 0 a dot, scaled so the pattern repeats twice; an empty pattern is a solid line.
func paintDashSample(w *gpui.Window, b gpui.Bounds, pattern []float64) {
	x0, x1 := float32(b.Origin.X)+1, float32(b.Origin.X+b.Size.W)-1
	y := float32(b.Origin.Y) + float32(b.Size.H)/2
	total := 0.0
	for _, p := range pattern {
		total += math.Abs(p)
	}
	if total <= 0 {
		strokeLine(w, pt(x0, y), pt(x1, y), 1.4, uiText)
		return
	}
	scale := float64(x1-x0) / (2 * total)
	x := float64(x0)
	for i := 0; x < float64(x1) && i < 400; i++ {
		el := pattern[i%len(pattern)] * scale
		switch {
		case el > 0:
			strokeLine(w, pt(float32(x), y), pt(float32(min(x+el, float64(x1))), y), 1.4, uiText)
			x += el
		case el < 0:
			x -= el
		default:
			strokeLine(w, pt(float32(x), y), pt(float32(x)+0.01, y), 1.6, uiText)
		}
	}
}

// arrowSampleSize is the arrowhead length, in logical pixels, of an arrow sample.
const arrowSampleSize = 8

// paintArrowSample draws the arrowhead block (a DIMBLK value) at the right end of b, pointing
// right, with a stub of dimension line: the shape doc.LayoutArrow gives the dimension.
func paintArrowSample(w *gpui.Window, b gpui.Bounds, block string) {
	a := doc.LayoutArrow(block, geom.V(0, 0), geom.V(1, 0), 1)
	s := float32(arrowSampleSize)
	tipX := float32(b.Origin.X+b.Size.W) - s/2 - 1.5
	midY := float32(b.Origin.Y) + float32(b.Size.H)/2
	at := func(v geom.Vec2) gpui.Point { return pt(tipX+float32(v.X)*s, midY-float32(v.Y)*s) }
	col := uiText
	strokeLine(w, pt(float32(b.Origin.X)+1, midY), at(a.LineEnd), 1, col)
	if len(a.Outline) > 0 {
		pts := make([]gpui.Point, len(a.Outline))
		for i, v := range a.Outline {
			pts[i] = at(v)
		}
		if a.Filled {
			fillPolygon(w, pts, col)
		}
		w.PaintStroke(gpui.Stroke{Points: pts, Closed: true, Width: 1, Color: col})
	}
	for _, l := range a.Lines {
		strokeLine(w, at(l.A), at(l.B), max(1, float32(a.Width)*s), col)
	}
	for _, c := range a.Circles {
		r := float32(c.R) * s
		if a.Filled {
			p := at(c.C)
			w.PaintStroke(gpui.Stroke{Points: []gpui.Point{p, {X: p.X + 0.01, Y: p.Y}}, Width: gpui.Pixels(max(2*r, 1.5)), Color: col})
			continue
		}
		pts := make([]gpui.Point, 24)
		for i := range pts {
			t := 2 * math.Pi * float64(i) / float64(len(pts))
			pts[i] = at(c.C.Add(geom.V(math.Cos(t), math.Sin(t)).Scale(c.R)))
		}
		w.PaintStroke(gpui.Stroke{Points: pts, Closed: true, Width: 1, Color: col})
	}
	for _, arc := range a.Arcs {
		pts := make([]gpui.Point, 13)
		sw := arc.Sweep()
		for i := range pts {
			pts[i] = at(arc.PointAt(arc.Start + sw*float64(i)/float64(len(pts)-1)))
		}
		w.PaintStroke(gpui.Stroke{Points: pts, Width: 1, Color: col})
	}
}

// fillPolygon fills pts (even-odd) with horizontal strokes one logical pixel apart, as
// render/gpuirender fills: gpui has no polygon primitive.
func fillPolygon(w *gpui.Window, pts []gpui.Point, col gpui.Rgba) {
	top, bottom := float32(math.MaxFloat32), float32(-math.MaxFloat32)
	for _, p := range pts {
		top, bottom = min(top, float32(p.Y)), max(bottom, float32(p.Y))
	}
	var xs []float32
	for y := float32(math.Floor(float64(top))) + 0.5; y < bottom; y++ {
		xs = xs[:0]
		for i := range pts {
			a, b := pts[i], pts[(i+1)%len(pts)]
			ay, by := float32(a.Y), float32(b.Y)
			if (ay <= y) != (by <= y) {
				xs = append(xs, float32(a.X)+(y-ay)*float32(b.X-a.X)/(by-ay))
			}
		}
		slices.Sort(xs)
		for k := 0; k+1 < len(xs); k += 2 {
			strokeLine(w, pt(xs[k], y), pt(xs[k+1], y), 1.25, col)
		}
	}
}
