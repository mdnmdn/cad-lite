// Package plot lays a drawing out on a sheet of paper and prints it: it chooses what to plot (the
// whole drawing or the selection) and which part of model space (the extents, the current view or
// a window), fits that onto a paper size in either orientation, in colour or monochrome, and then
// writes the sheet as a vector PDF, rasterises it for a preview, or hands the PDF to the system's
// print spooler. It imports doc, geom and render and never a UI package.
package plot

import (
	"errors"
	"math"
	"slices"

	"cad-lite/doc"
	"cad-lite/geom"
	"cad-lite/render"
)

// What chooses the entities a plot includes.
type What int

const (
	// WhatAll plots every plottable entity of the drawing.
	WhatAll What = iota
	// WhatSelection plots only the entities of Options.Selection.
	WhatSelection
)

// Area chooses the part of model space a plot shows.
type Area int

const (
	// AreaExtents shows the extents of what is plotted.
	AreaExtents Area = iota
	// AreaDisplay shows Options.Window, the box the canvas shows.
	AreaDisplay
	// AreaWindow shows Options.Window, a box the user picked.
	AreaWindow
)

// Style is how colours plot, after AutoCAD's acad, grayscale and monochrome plot style tables.
type Style int

const (
	// StyleColor plots each entity in its resolved colour.
	StyleColor Style = iota
	// StyleGray plots each colour as the grey of its luminance.
	StyleGray
	// StyleMono plots every line and fill black.
	StyleMono
)

// DefaultMargin is the unprintable border kept on every side of the paper, in millimetres.
const DefaultMargin = 10.0

// ErrNothing is returned when nothing plottable falls in the plot area.
var ErrNothing = errors.New("plot: nothing to plot")

// Options is how a drawing is laid out on paper.
type Options struct {
	// Paper is the sheet size.
	Paper Paper
	// Landscape turns the paper so its long side is horizontal.
	Landscape bool
	// Style is colour, grayscale or monochrome.
	Style Style
	// Scale is the plot scale; the zero value fits the area on the paper. A fixed scale converts
	// drawing units to millimetres through the drawing's INSUNITS (UnitMM).
	Scale Scale
	// NoCenter places the area's lower-left corner Offset millimetres from the lower-left corner of
	// the printable area instead of centring it on the paper.
	NoCenter bool
	// Offset is where the area starts with NoCenter, in millimetres; it may be negative.
	Offset geom.Vec2
	// NoLineweights plots every line at ThinWidth instead of its lineweight.
	NoLineweights bool
	// What is the whole drawing or the selection.
	What What
	// Selection is the entities WhatSelection plots.
	Selection []doc.Handle
	// Area is the part of model space that is fitted onto the paper.
	Area Area
	// Window is the model-space box AreaDisplay and AreaWindow show.
	Window geom.Box2
	// Margin is the border kept free on each side in millimetres; 0 is DefaultMargin.
	Margin float64
}

// Sheet is a drawing laid out on paper: display items in millimetres from the paper's lower-left
// corner, y up.
type Sheet struct {
	// Width and Height are the paper size as oriented, in millimetres.
	Width, Height float64
	// Clip is the box outside which nothing prints, in millimetres.
	Clip geom.Box2
	// Scale is millimetres of paper per drawing unit.
	Scale float64
	// Overflow is set when the area does not fit inside the printable part of the paper, so part
	// of it is cut off (a fixed scale too large, or an offset).
	Overflow bool
	// Items are drawn first to last.
	Items []Item
}

// Item is one stroked polyline or filled polygon on the sheet.
type Item struct {
	// Points are paper coordinates in millimetres.
	Points []geom.Vec2
	// Starts splits a stroke into separate runs, as render.DisplayItem.Starts does.
	Starts []int32
	// Color is the ink.
	Color render.Color
	// Width is the stroke width in millimetres; a fill ignores it.
	Width float64
	// Closed joins a stroke's last point to its first.
	Closed bool
	// Fill fills the polygon (even-odd) instead of stroking it.
	Fill bool
}

// minWidth is the thinnest stroke a sheet asks for, in millimetres: a lineweight of 0 plots as the
// finest line rather than not at all.
const minWidth = 0.05

// ThinWidth is the width every line plots at with Options.NoLineweights, in millimetres.
const ThinWidth = 0.1

// printTolerance is the chord error a sheet's curves are tessellated to, in millimetres of paper.
const printTolerance = 0.02

// Plottable reports whether e is drawn on paper: its layer is on, thawed and set to plot, and it is
// an entity Cad-lite displays.
func Plottable(d *doc.Drawing, e doc.Entity) bool {
	if _, ok := e.(*doc.Unknown); ok {
		return false
	}
	return d.EntityVisible(e) && !d.Layer(e.Common().Layer).NoPlot
}

// entityBounds is the box e draws in: an insert's visible parts, otherwise its bounds.
func entityBounds(e doc.Entity) geom.Box2 {
	if in, ok := e.(*doc.Insert); ok {
		return in.VisibleBounds()
	}
	return e.Bounds()
}

// Extents returns the box of what opt plots (WhatAll or WhatSelection), whatever its Area, or an
// empty box when nothing is plottable.
func Extents(d *doc.Drawing, opt Options) geom.Box2 {
	b := geom.EmptyBox()
	for _, h := range candidates(d, opt) {
		b = b.Union(entityBounds(d.Entity(h)))
	}
	return b
}

// candidates returns the plottable handles opt.What chooses, ascending.
func candidates(d *doc.Drawing, opt Options) []doc.Handle {
	src := d.Handles()
	if opt.What == WhatSelection {
		src = slices.Clone(opt.Selection)
		slices.Sort(src)
		src = slices.Compact(src)
	}
	out := make([]doc.Handle, 0, len(src))
	for _, h := range src {
		if e := d.Entity(h); e != nil && Plottable(d, e) {
			out = append(out, h)
		}
	}
	return out
}

// Compose lays out d on paper as opt says. It returns ErrNothing when nothing plottable meets the
// area, or when the area has no size.
func Compose(d *doc.Drawing, opt Options) (*Sheet, error) {
	hs := candidates(d, opt)
	area := geom.EmptyBox()
	switch opt.Area {
	case AreaDisplay, AreaWindow:
		area = geom.BoxOf(opt.Window.Min, opt.Window.Max)
	default:
		for _, h := range hs {
			area = area.Union(entityBounds(d.Entity(h)))
		}
	}
	if area.IsEmpty() || !finite(area) {
		return nil, ErrNothing
	}
	in := hs[:0:0]
	for _, h := range hs {
		if entityBounds(d.Entity(h)).Intersects(area) {
			in = append(in, h)
		}
	}
	if len(in) == 0 {
		return nil, ErrNothing
	}
	w, h := opt.Paper.Size(opt.Landscape)
	m := opt.Margin
	if m <= 0 {
		m = DefaultMargin
	}
	m = min(m, w/4, h/4)
	pw, ph := w-2*m, h-2*m
	aw, ah := area.Max.X-area.Min.X, area.Max.Y-area.Min.Y
	if aw <= 0 && ah <= 0 {
		// One point (a lone dot, or a window of zero size): show half a drawing unit around it.
		area = area.Grow(0.5)
		aw, ah = 1, 1
	}
	k := math.Inf(1)
	if aw > 0 {
		k = pw / aw
	}
	if ah > 0 {
		k = min(k, ph/ah)
	}
	if opt.Scale.Fixed() {
		k = opt.Scale.Paper / opt.Scale.Drawing * UnitMM(d.Header.InsUnits)
	}
	// The area's lower-left corner lands at (ox, oy) on the paper.
	ox, oy := w/2-aw*k/2, h/2-ah*k/2
	if opt.NoCenter {
		ox, oy = m+opt.Offset.X, m+opt.Offset.Y
	}
	toPaper := func(p geom.Vec2) geom.Vec2 {
		return geom.V((p.X-area.Min.X)*k+ox, (p.Y-area.Min.Y)*k+oy)
	}
	printable := geom.BoxOf(geom.V(m, m), geom.V(w-m, h-m))
	s := &Sheet{Width: w, Height: h, Scale: k, Clip: printable}
	placed := geom.BoxOf(toPaper(area.Min), toPaper(area.Max))
	const slack = 1e-6
	s.Overflow = !printable.Grow(slack).ContainsBox(placed)
	if opt.Area != AreaExtents {
		s.Clip = intersect(printable, placed)
	}
	if s.Clip.Max.X-s.Clip.Min.X <= 0 || s.Clip.Max.Y-s.Clip.Min.Y <= 0 {
		// Nothing of the area is on the paper, or it is a window with no width or height.
		return nil, ErrNothing
	}

	b := render.NewBuilder(d, render.Color{R: 255, G: 255, B: 255, A: 255})
	b.Weight, b.Lineweights = 0, true
	// Band tolerance is at most 0.5/Scale model units, which this scale makes printTolerance mm.
	v := render.View{Scale: max(render.MinScale, min(render.MaxScale, 0.5*k/printTolerance)), Width: 1, Height: 1}
	var dl render.DisplayList
	b.Build(in, v, &dl)
	for _, it := range dl.Items {
		if it.Color.A == 0 && (it.Kind != render.ItemRect || it.Outline.A == 0) {
			continue
		}
		col, out := it.Color, it.Outline
		col, out = opt.Style.ink(col), opt.Style.ink(out)
		width := max(minWidth, float64(it.Weight)*25.4/96)
		if opt.NoLineweights {
			width = ThinWidth
		}
		switch it.Kind {
		case render.ItemRect:
			if len(it.Points) < 2 {
				continue
			}
			a, c := it.Points[0], it.Points[1]
			box := []geom.Vec2{toPaper(a), toPaper(geom.V(c.X, a.Y)), toPaper(c), toPaper(geom.V(a.X, c.Y))}
			if col.A > 0 {
				s.add(Item{Points: box, Color: col, Fill: true})
			}
			if out.A > 0 {
				s.add(Item{Points: box, Color: out, Width: width, Closed: true})
			}
		case render.ItemFill:
			s.add(Item{Points: mapPoints(it.Points, toPaper), Color: col, Fill: true})
		default:
			s.add(Item{Points: mapPoints(it.Points, toPaper), Starts: it.Starts, Color: col, Width: width, Closed: it.Closed})
		}
	}
	return s, nil
}

func mapPoints(ps []geom.Vec2, f func(geom.Vec2) geom.Vec2) []geom.Vec2 {
	out := make([]geom.Vec2, len(ps))
	for i, p := range ps {
		out[i] = f(p)
	}
	return out
}

// ink is the colour c plots in, keeping its alpha.
func (st Style) ink(c render.Color) render.Color {
	switch st {
	case StyleMono:
		return render.Color{A: c.A}
	case StyleGray:
		g := uint8(math.Round(255 * (0.2126*float64(c.R) + 0.7152*float64(c.G) + 0.0722*float64(c.B)) / 255))
		return render.Color{R: g, G: g, B: g, A: c.A}
	}
	return c
}

// intersect is the overlap of a and b, empty when they do not overlap.
func intersect(a, b geom.Box2) geom.Box2 {
	return geom.Box2{Min: geom.V(max(a.Min.X, b.Min.X), max(a.Min.Y, b.Min.Y)), Max: geom.V(min(a.Max.X, b.Max.X), min(a.Max.Y, b.Max.Y))}
}

func finite(b geom.Box2) bool {
	for _, v := range []float64{b.Min.X, b.Min.Y, b.Max.X, b.Max.Y} {
		if math.IsNaN(v) || math.IsInf(v, 0) {
			return false
		}
	}
	return true
}

// runs calls f with each run of it's points, as Starts splits them.
func (it Item) runs(f func(ps []geom.Vec2)) {
	if len(it.Starts) == 0 {
		f(it.Points)
		return
	}
	for i, s := range it.Starts {
		end := len(it.Points)
		if i+1 < len(it.Starts) {
			end = int(it.Starts[i+1])
		}
		if int(s) < end && end <= len(it.Points) {
			f(it.Points[s:end])
		}
	}
}
