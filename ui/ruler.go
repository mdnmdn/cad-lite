package ui

import (
	"math"
	"strconv"
	"strings"

	"cad-lite/cmdsys"
	"cad-lite/geom"
	"cad-lite/internal/gpui"
	"cad-lite/render"
)

// Rulers along two edges of a viewport, in model units: the canvas draws them along the top and
// the left (rulerStyle canvasRulers), render_png along the bottom and the left (snapshotRulers).
// Both are this one implementation. The rulers overlay the drawing: the view keeps the whole
// viewport, so model coordinates, picking, snapping and zooming do not depend on them.

const (
	rulerBand  = 20 // height of the horizontal ruler, px
	rulerTick  = 6  // length of a major tick, px
	rulerMinor = 3  // length of a minor tick, px
	rulerLabel = 11 // label text size, px
	// rulerMinorGap is the closest two minor ticks may sit, px.
	rulerMinorGap = 6
)

// render_png's ruler colours: fixed, whatever the theme, so the MCP image is stable.
var (
	colRulerBand = render.Color{R: 18, G: 22, B: 27, A: 215}
	colRulerTick = render.Color{R: 200, G: 206, B: 214, A: 255}
	uiRulerText  = gpui.RgbaOf(215, 220, 226, 255)
)

// rulerStyle says where the horizontal ruler sits and how the rulers look.
type rulerStyle struct {
	// top puts the horizontal ruler along the top edge, else along the bottom.
	top bool
	// minor adds unlabelled ticks between the labelled ones.
	minor bool
	band  render.Color
	tick  render.Color
	text  gpui.Rgba
	// edge, when set, outlines the side of each ruler that faces the drawing.
	edge render.Color
	// marker, when set, is the colour of the cursor's position on each ruler.
	marker render.Color
	// exact gives labels the decimals their step needs and the left ruler a width that holds
	// while panning (the canvas). Without it labels have at most four decimals and the left
	// ruler fits the longest label shown: render_png's pinned look.
	exact bool
}

// snapshotRulers is render_png's style (cadmcp `rulers: true`).
func snapshotRulers() rulerStyle {
	return rulerStyle{band: colRulerBand, tick: colRulerTick, text: uiRulerText}
}

// canvasRulers is the canvas's style, from the current theme.
func canvasRulers() rulerStyle {
	return rulerStyle{top: true, minor: true, band: renderColor(uiPanel), tick: renderColor(uiMuted),
		text: uiMuted, edge: renderColor(uiBorder), marker: renderColor(uiAccent), exact: true}
}

func renderColor(c gpui.Rgba) render.Color { return render.Color{R: c.R, G: c.G, B: c.B, A: c.A} }

// niceStep returns the smallest 1, 2 or 5 times a power of ten that is at
// least minPx pixels at scale.
func niceStep(scale, minPx float64) float64 {
	raw := minPx / scale
	p := math.Pow(10, math.Floor(math.Log10(raw)))
	for _, m := range []float64{1, 2, 5, 10} {
		if m*p >= raw {
			return m * p
		}
	}
	return 10 * p
}

// minorStep divides a 1-2-5 step into 5, 4 or 5 parts (0.2, 0.5 or 1 of its power of ten).
func minorStep(step float64) float64 {
	p := math.Pow(10, math.Floor(math.Log10(step)+1e-9))
	if m := math.Round(step / p); m == 2 {
		return step / 4
	}
	return step / 5
}

type tick struct {
	px    float64
	value float64
	// dec is how many decimals the label has (see stepDecimals); -1 formats it as a command-line
	// number, with at most four.
	dec int
}

func (t tick) label() string {
	if t.dec < 0 {
		return cmdsys.FormatNumber(t.value)
	}
	return rulerNumber(t.value, t.dec)
}

// stepDecimals is how many decimals tell apart the multiples of a 1-2-5 step: none from 1 up,
// one for 0.5 and 0.2, five for 5e-5.
func stepDecimals(step float64) int {
	return max(0, int(-math.Floor(math.Log10(step)+1e-9)))
}

// rulerNumber formats a ruler value with dec decimals, without trailing zeros or a "-0".
func rulerNumber(v float64, dec int) string {
	s := strconv.FormatFloat(v, 'f', dec, 64)
	if strings.Contains(s, ".") {
		s = strings.TrimRight(strings.TrimRight(s, "0"), ".")
	}
	if s == "-0" {
		s = "0"
	}
	return s
}

// rulerSteps returns the labelled tick steps of the horizontal and vertical rulers, in model units.
func rulerSteps(v render.View) (sx, sy float64) { return niceStep(v.Scale, 80), niceStep(v.Scale, 48) }

// ticksAlong returns the ticks every step within [lo, hi] (model units), at most 200, with their
// pixel positions from px; exact labels them with the decimals the step needs.
func ticksAlong(lo, hi, step float64, px func(float64) float64, exact bool) []tick {
	var ts []tick
	dec := -1
	if exact {
		dec = stepDecimals(step)
	}
	for i := math.Ceil(lo / step); i*step <= hi && len(ts) < 200; i++ {
		ts = append(ts, tick{px(i * step), i * step, dec})
	}
	return ts
}

// rulerTicks returns the labelled ticks of the horizontal (xs) and vertical (ys) rulers; exact as
// in rulerStyle.
func rulerTicks(v render.View, exact bool) (xs, ys []tick) {
	vis := v.Visible()
	sx, sy := rulerSteps(v)
	xs = ticksAlong(vis.Min.X, vis.Max.X, sx, func(x float64) float64 { px, _ := v.ToScreen(geom.V(x, 0)); return px }, exact)
	ys = ticksAlong(vis.Min.Y, vis.Max.Y, sy, func(y float64) float64 { _, py := v.ToScreen(geom.V(0, y)); return py }, exact)
	return
}

// fitBand is the width of a left ruler that fits its longest label.
func fitBand(ys []tick) float64 {
	n := 0
	for _, t := range ys {
		n = max(n, len(t.label()))
	}
	return float64(n)*6.4 + 12
}

// leftBand is the width of the left ruler for the visible range lo to hi and the labelled step:
// wide enough for the label of either end of the range, which is the longest any label in it can
// be, rounded up to a multiple of three characters, so the band (and the overlays inside it) does
// not jump as the labels pass by while panning, only when the range gains or loses digits.
func leftBand(lo, hi, step float64) float64 {
	dec := stepDecimals(step)
	n := max(len(rulerNumber(math.Floor(lo/step)*step, dec)), len(rulerNumber(math.Ceil(hi/step)*step, dec)))
	n = (n + 2) / 3 * 3
	return float64(n)*6.4 + 12
}

// rulerFrame is where the two rulers sit in a viewport, in pixels.
type rulerFrame struct {
	xs, ys []tick
	// lb is the left ruler's width; hy0 and hy1 the top and bottom of the horizontal ruler;
	// vy0 and vy1 the vertical extent of the left ruler.
	lb, hy0, hy1, vy0, vy1 float64
	// edgeY is the horizontal ruler's edge that faces the drawing, where its ticks start.
	edgeY float64
}

func (s rulerStyle) frame(v render.View) rulerFrame {
	xs, ys := rulerTicks(v, s.exact)
	f := rulerFrame{xs: xs, ys: ys, lb: fitBand(ys)}
	if s.exact {
		vis := v.Visible()
		_, sy := rulerSteps(v)
		f.lb = leftBand(vis.Min.Y, vis.Max.Y, sy)
	}
	if s.top {
		f.hy0, f.hy1, f.vy0, f.vy1, f.edgeY = 0, rulerBand, rulerBand, v.Height, rulerBand
	} else {
		f.hy0, f.hy1, f.vy0, f.vy1, f.edgeY = v.Height-rulerBand, v.Height, 0, v.Height-rulerBand, v.Height
	}
	return f
}

// inV reports whether a vertical-ruler pixel row is on the left ruler (not under the other one).
func (f rulerFrame) inV(py float64) bool { return py >= f.vy0 && py <= f.vy1 }

// lines appends the ruler bands, ticks and, when the style has a marker and cursor is non-nil,
// the cursor's position on each ruler to dl.
func (s rulerStyle) lines(dl *render.DisplayList, v render.View, cursor *geom.Vec2) {
	s.linesIn(dl, v, s.frame(v), cursor)
}

// linesIn is lines with the frame f of v already worked out.
func (s rulerStyle) linesIn(dl *render.DisplayList, v render.View, f rulerFrame, cursor *geom.Vec2) {
	at := func(x, y float64) geom.Vec2 { return v.ToModel(x, y) }
	dl.Rect(at(0, f.hy0), at(v.Width, f.hy1), s.band, render.Color{}, 0)
	dl.Rect(at(0, f.vy0), at(f.lb, f.vy1), s.band, render.Color{}, 0)
	if s.edge != (render.Color{}) {
		dl.Polyline([]geom.Vec2{at(f.lb, f.edgeY), at(v.Width, f.edgeY)}, s.edge, 0, false)
		dl.Polyline([]geom.Vec2{at(f.lb, f.vy0), at(f.lb, f.vy1)}, s.edge, 0, false)
	}
	hTick := func(px, n float64) {
		dl.Polyline([]geom.Vec2{at(px, f.edgeY-n), at(px, f.edgeY)}, s.tick, 1, false)
	}
	vTick := func(py, n float64) {
		dl.Polyline([]geom.Vec2{at(f.lb-n, py), at(f.lb, py)}, s.tick, 1, false)
	}
	if s.minor {
		vis := v.Visible()
		sx, sy := rulerSteps(v)
		if mx := minorStep(sx); mx*v.Scale >= rulerMinorGap {
			for _, t := range ticksAlong(vis.Min.X, vis.Max.X, mx, func(x float64) float64 { px, _ := v.ToScreen(geom.V(x, 0)); return px }, false) {
				if t.px >= f.lb {
					hTick(t.px, rulerMinor)
				}
			}
		}
		if my := minorStep(sy); my*v.Scale >= rulerMinorGap {
			for _, t := range ticksAlong(vis.Min.Y, vis.Max.Y, my, func(y float64) float64 { _, py := v.ToScreen(geom.V(0, y)); return py }, false) {
				if f.inV(t.px) {
					vTick(t.px, rulerMinor)
				}
			}
		}
	}
	for _, t := range f.xs {
		if t.px < f.lb {
			continue
		}
		hTick(t.px, rulerTick)
	}
	for _, t := range f.ys {
		if s.top && !f.inV(t.px) || !s.top && t.px > f.vy1 {
			continue
		}
		vTick(t.px, rulerTick)
	}
	if s.marker != (render.Color{}) && cursor != nil {
		x, y := v.ToScreen(*cursor)
		if x >= f.lb && x <= v.Width {
			dl.Polyline([]geom.Vec2{at(x, f.hy0), at(x, f.hy1)}, s.marker, 1, false)
		}
		if f.inV(y) {
			dl.Polyline([]geom.Vec2{at(0, y), at(f.lb, y)}, s.marker, 1, false)
		}
	}
}

// labels adds the labels of the labelled ticks to root, a relatively positioned element with the
// viewport's origin.
func (s rulerStyle) labels(root *gpui.Div, v render.View) { s.labelsIn(root, s.frame(v)) }

// labelsIn is labels with the frame already worked out.
func (s rulerStyle) labelsIn(root *gpui.Div, f rulerFrame) {
	for _, t := range f.xs {
		if t.px < f.lb {
			continue
		}
		root.Child(gpui.NewDiv().Absolute().Left(float32(t.px) + 3).Top(float32(f.hy0) + 3).
			TextSize(rulerLabel).TextColor(s.text).Child(gpui.NewText(t.label())))
	}
	for _, t := range f.ys {
		if s.top && (t.px-rulerLabel-4 < f.vy0 || t.px > f.vy1) || !s.top && t.px > f.vy1 {
			continue
		}
		root.Child(gpui.NewDiv().Absolute().Left(4).Top(float32(t.px) - rulerLabel - 4).
			TextSize(rulerLabel).TextColor(s.text).Child(gpui.NewText(t.label())))
	}
}
