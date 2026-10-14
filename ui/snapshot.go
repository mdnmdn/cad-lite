package ui

import (
	"bytes"
	"errors"
	"image"
	"image/png"
	"math"

	"cad-lite/cadmcp"
	"cad-lite/cmdsys"
	"cad-lite/geom"
	"cad-lite/internal/gpui"
	"cad-lite/render"
	"cad-lite/render/gpuirender"
)

// Snapshot renders only the drawing of s as a PNG; see SnapshotImage.
func Snapshot(s *cmdsys.Session, req cadmcp.SnapshotRequest) ([]byte, error) {
	img, err := SnapshotImage(s, req)
	if err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// SnapshotImage renders only the drawing of s: background, optional grid,
// entities, highlights and optional coordinate rulers, at req.View. It needs
// no window and must run on the goroutine that owns s; the image it returns
// is its own, so it may be encoded on any goroutine.
func SnapshotImage(s *cmdsys.Session, req cadmcp.SnapshotRequest) (image.Image, error) {
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
			snapshotRulers().lines(&dl, v, nil)
		}
		gpuirender.Painter{Origin: b.Origin, View: v}.Paint(w, &dl)
	}

	root := gpui.RenderFunc(func(*gpui.Window, *gpui.App) gpui.IntoElement {
		r := gpui.NewDiv().SizeFull().Relative().Child(gpui.NewCanvas(paint))
		if req.Rulers {
			snapshotRulers().labels(r, v)
		}
		return r
	})
	img, err := gpui.RenderToImage(req.Width, req.Height, 1, root, gpui.WithBackgroundColor(gpuiColor(colCanvas)))
	if err != nil {
		return nil, err
	}
	return img, nil
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
