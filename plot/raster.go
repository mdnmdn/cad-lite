package plot

import (
	"image"
	"image/color"
	"image/draw"
	"math"

	"github.com/srwiley/rasterx"
	"golang.org/x/image/math/fixed"

	"cad-lite/geom"
)

// Raster draws the sheet on white paper at pxPerMM pixels per millimetre, for a preview. Strokes
// are never thinner than one pixel, so the finest lines stay visible at preview size.
func (s *Sheet) Raster(pxPerMM float64) *image.NRGBA {
	w := max(1, int(math.Round(s.Width*pxPerMM)))
	h := max(1, int(math.Round(s.Height*pxPerMM)))
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	draw.Draw(img, img.Bounds(), image.NewUniform(color.White), image.Point{}, draw.Src)
	to := func(p geom.Vec2) fixed.Point26_6 {
		return fixed.Point26_6{X: fixed.Int26_6(p.X * pxPerMM * 64), Y: fixed.Int26_6((s.Height - p.Y) * pxPerMM * 64)}
	}
	clip := image.Rect(
		int(math.Floor(s.Clip.Min.X*pxPerMM)), int(math.Floor((s.Height-s.Clip.Max.Y)*pxPerMM)),
		int(math.Ceil(s.Clip.Max.X*pxPerMM)), int(math.Ceil((s.Height-s.Clip.Min.Y)*pxPerMM)))
	sc := rasterx.NewScannerGV(w, h, img, img.Bounds())
	sc.SetClip(clip)
	filler := rasterx.NewFiller(w, h, sc)
	stroker := rasterx.NewStroker(w, h, sc)
	for _, it := range s.Items {
		if len(it.Points) == 0 {
			continue
		}
		c := color.NRGBA{R: it.Color.R, G: it.Color.G, B: it.Color.B, A: it.Color.A}
		if it.Fill {
			filler.Clear()
			sc.SetWinding(false)
			filler.Start(to(it.Points[0]))
			for _, p := range it.Points[1:] {
				filler.Line(to(p))
			}
			filler.Stop(true)
			filler.SetColor(c)
			filler.Draw()
			continue
		}
		width := fixed.Int26_6(max(1, it.Width*pxPerMM) * 64)
		stroker.SetStroke(width, 4*64, rasterx.RoundCap, nil, rasterx.RoundGap, rasterx.Round)
		stroker.Clear()
		sc.SetWinding(true)
		it.runs(func(ps []geom.Vec2) {
			if len(ps) == 0 {
				return
			}
			stroker.Start(to(ps[0]))
			for _, p := range ps[1:] {
				stroker.Line(to(p))
			}
			if len(ps) == 1 {
				stroker.Line(to(ps[0]).Add(fixed.Point26_6{X: 1}))
			}
			stroker.Stop(it.Closed)
		})
		stroker.SetColor(c)
		stroker.Draw()
	}
	return img
}
