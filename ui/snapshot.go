package ui

import (
	"bytes"
	"errors"
	"image/png"
	"math"

	"cad-lite/cadmcp"
	"cad-lite/cmdsys"
	"cad-lite/geom"
	"cad-lite/internal/gpui"
	"cad-lite/render"
	"cad-lite/render/gpuirender"
)

const (
	rulerBand  = 20
	rulerTick  = 6
	rulerLabel = 11
)

var (
	colRulerBand = render.Color{R: 18, G: 22, B: 27, A: 215}
	colRulerTick = render.Color{R: 200, G: 206, B: 214, A: 255}
	uiRulerText  = gpui.RgbaOf(215, 220, 226, 255)
)

// Snapshot renders only the drawing of s as a PNG: background, optional grid,
// entities, highlights and optional coordinate rulers, at req.View. It needs
// no window and must run on the goroutine that owns s.
func Snapshot(s *cmdsys.Session, req cadmcp.SnapshotRequest) ([]byte, error) {
	if s == nil {
		return nil, errors.New("no drawing is open")
	}
	if req.Width <= 0 || req.Height <= 0 {
		return nil, errors.New("invalid image size")
	}
	v := req.View
	v.Width, v.Height = float64(req.Width), float64(req.Height)
	if !(v.Scale > 0) {
		return nil, errors.New("invalid view scale")
	}

	builder := render.NewBuilder(s.Drawing(), colCanvas)
	paint := func(b gpui.Bounds, w *gpui.Window, _ *gpui.App) {
		w.PushClip(b)
		defer w.PopClip()
		w.PaintQuad(gpui.Quad{Bounds: b, Background: gpui.BackgroundSolid, Fill: gpuiColor(colCanvas)})
		var dl render.DisplayList
		if req.Grid {
			gridLines(&dl, v)
		}
		builder.Build(s.Index().Query(v.Visible()), v, &dl)
		for _, h := range req.Highlight {
			for _, it := range builder.Items(h, v) {
				it.Color, it.Weight = colSelected, 2
				dl.Add(it)
			}
		}
		if req.Rulers {
			rulerLines(&dl, v)
		}
		gpuirender.Painter{Origin: b.Origin, View: v}.Paint(w, &dl)
	}

	root := gpui.RenderFunc(func(*gpui.Window, *gpui.App) gpui.IntoElement {
		r := gpui.NewDiv().SizeFull().Relative().Child(gpui.NewCanvas(paint))
		if req.Rulers {
			rulerLabels(r, v)
		}
		return r
	})
	img, err := gpui.RenderToImage(req.Width, req.Height, 1, root, gpui.WithBackgroundColor(gpuiColor(colCanvas)))
	if err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// gridLines mirrors the interactive grid: every power of ten at least 16 px
// apart, every tenth line stronger.
func gridLines(dl *render.DisplayList, v render.View) {
	step := math.Pow(10, math.Ceil(math.Log10(16*v.PixelSize())))
	vis := v.Visible()
	x0, x1 := math.Floor(vis.Min.X/step), math.Ceil(vis.Max.X/step)
	y0, y1 := math.Floor(vis.Min.Y/step), math.Ceil(vis.Max.Y/step)
	if x1-x0 > 400 || y1-y0 > 400 {
		return
	}
	col := func(i float64) render.Color {
		if math.Mod(i, 10) == 0 {
			return colGridMajor
		}
		return colGrid
	}
	for i := x0; i <= x1; i++ {
		dl.Polyline([]geom.Vec2{{X: i * step, Y: vis.Min.Y}, {X: i * step, Y: vis.Max.Y}}, col(i), 0, false)
	}
	for j := y0; j <= y1; j++ {
		dl.Polyline([]geom.Vec2{{X: vis.Min.X, Y: j * step}, {X: vis.Max.X, Y: j * step}}, col(j), 0, false)
	}
}

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

type tick struct {
	px    float64
	value float64
}

// rulerTicks returns the tick positions along the bottom (x) and left (y)
// rulers.
func rulerTicks(v render.View) (xs, ys []tick) {
	vis := v.Visible()
	sx, sy := niceStep(v.Scale, 80), niceStep(v.Scale, 48)
	for i := math.Ceil(vis.Min.X / sx); i*sx <= vis.Max.X && len(xs) < 200; i++ {
		px, _ := v.ToScreen(geom.V(i*sx, 0))
		xs = append(xs, tick{px, i * sx})
	}
	for j := math.Ceil(vis.Min.Y / sy); j*sy <= vis.Max.Y && len(ys) < 200; j++ {
		_, py := v.ToScreen(geom.V(0, j*sy))
		ys = append(ys, tick{py, j * sy})
	}
	return
}

func (t tick) label() string { return cmdsys.FormatNumber(t.value) }

// leftBand is the width of the left ruler, wide enough for its longest label.
func leftBand(ys []tick) float64 {
	n := 0
	for _, t := range ys {
		n = max(n, len(t.label()))
	}
	return float64(n)*6.4 + 12
}

func rulerLines(dl *render.DisplayList, v render.View) {
	xs, ys := rulerTicks(v)
	lb := leftBand(ys)
	at := func(x, y float64) geom.Vec2 { return v.ToModel(x, y) }
	dl.Rect(at(0, v.Height-rulerBand), at(v.Width, v.Height), colRulerBand, render.Color{}, 0)
	dl.Rect(at(0, 0), at(lb, v.Height-rulerBand), colRulerBand, render.Color{}, 0)
	for _, t := range xs {
		if t.px < lb {
			continue
		}
		dl.Polyline([]geom.Vec2{at(t.px, v.Height-rulerTick), at(t.px, v.Height)}, colRulerTick, 1, false)
	}
	for _, t := range ys {
		if t.px > v.Height-rulerBand {
			continue
		}
		dl.Polyline([]geom.Vec2{at(lb-rulerTick, t.px), at(lb, t.px)}, colRulerTick, 1, false)
	}
}

func rulerLabels(root *gpui.Div, v render.View) {
	xs, ys := rulerTicks(v)
	lb := leftBand(ys)
	for _, t := range xs {
		if t.px < lb {
			continue
		}
		root.Child(gpui.NewDiv().Absolute().Left(float32(t.px) + 3).Top(float32(v.Height) - rulerBand + 3).
			TextSize(rulerLabel).TextColor(uiRulerText).Child(gpui.NewText(t.label())))
	}
	for _, t := range ys {
		if t.px > v.Height-rulerBand {
			continue
		}
		root.Child(gpui.NewDiv().Absolute().Left(4).Top(float32(t.px) - rulerLabel - 4).
			TextSize(rulerLabel).TextColor(uiRulerText).Child(gpui.NewText(t.label())))
	}
}
