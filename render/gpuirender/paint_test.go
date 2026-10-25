package gpuirender_test

import (
	"image"
	"math"
	"math/rand"
	"testing"

	"cad-lite/doc"
	"cad-lite/geom"
	"cad-lite/internal/gpui"
	"cad-lite/render"
	"cad-lite/render/gpuirender"
)

// paint renders dl in a w×h headless window over a black background, view v filling it.
func paint(t testing.TB, w, h int, v render.View, dl *render.DisplayList) *image.NRGBA {
	t.Helper()
	root := gpui.RenderFunc(func(*gpui.Window, *gpui.App) gpui.IntoElement {
		return gpui.NewDiv().SizeFull().Child(gpui.NewCanvas(func(b gpui.Bounds, win *gpui.Window, _ *gpui.App) {
			win.PushClip(b)
			defer win.PopClip()
			gpuirender.Painter{Origin: b.Origin, View: v}.Paint(win, dl)
		}))
	})
	img, err := gpui.RenderToImage(w, h, 1, root, gpui.WithBackgroundColor(gpui.Rgba{A: 255}))
	if err != nil {
		t.Fatal(err)
	}
	return img
}

func TestFillPaintsTheInteriorAndDashRunsPaintApart(t *testing.T) {
	v := render.DefaultView(200, 100)
	var dl render.DisplayList
	red := render.Color{R: 255, A: 255}
	// A triangle left of centre, model y up.
	dl.Fill([]geom.Vec2{geom.V(-80, -30), geom.V(-20, -30), geom.V(-50, 30)}, red)
	// Two dashes along y = 0 right of centre, with a gap from x 40 to 60.
	dl.Add(render.DisplayItem{Kind: render.ItemPolyline, Points: []geom.Vec2{geom.V(20, 0), geom.V(40, 0), geom.V(60, 0), geom.V(80, 0)}, Starts: []int32{0, 2}, Color: red, Weight: 2})
	img := paint(t, 200, 100, v, &dl)
	at := func(x, y float64) uint8 {
		sx, sy := v.ToScreen(geom.V(x, y))
		return img.NRGBAAt(int(sx), int(sy)).R
	}
	for _, p := range [][2]float64{{-50, 0}, {-50, -25}, {-30, -20}, {30, 0}, {70, 0}} {
		if at(p[0], p[1]) < 200 {
			t.Errorf("(%v, %v) not painted", p[0], p[1])
		}
	}
	for _, p := range [][2]float64{{-75, 20}, {-25, 20}, {50, 0}, {-50, 40}} {
		if at(p[0], p[1]) > 50 {
			t.Errorf("(%v, %v) painted", p[0], p[1])
		}
	}
}

// BenchmarkPaintDashed measures a full frame (scene build and CPU raster) of 20k entities in ISO
// linetypes, zoomed so the dashes are a few pixels long, against the same drawing solid.
func BenchmarkPaintDashed(b *testing.B) { benchPaint(b, true) }

// BenchmarkPaintSolid is BenchmarkPaintDashed's drawing drawn continuous.
func BenchmarkPaintSolid(b *testing.B) { benchPaint(b, false) }

func benchPaint(b *testing.B, dashed bool) {
	d := doc.New()
	tx, _ := d.Begin()
	types := []string{"ACAD_ISO02W100", "ACAD_ISO04W100", "ACAD_ISO07W100", "ACAD_ISO10W100", "CENTER", "HIDDEN", "PHANTOM", "DASHDOT"}
	for _, lt := range types {
		_ = tx.LoadLibraryLinetype(lt)
	}
	r := rand.New(rand.NewSource(1))
	for i := range 20000 {
		p := geom.V(r.Float64()*10000, r.Float64()*10000)
		var e doc.Entity = &doc.Line{Start: p, End: p.Add(geom.V(r.Float64()*400-200, r.Float64()*400-200))}
		if i%2 == 1 {
			e = &doc.Circle{Center: p, Radius: 10 + r.Float64()*100}
		}
		if dashed {
			e.Common().Linetype = types[i%len(types)]
		}
		tx.Add(e)
	}
	tx.Commit()
	v := render.DefaultView(1600, 1000)
	v.Fit(geom.BoxOf(geom.V(0, 0), geom.V(10000, 10000)), 0)
	v.Scale *= 4
	bl := render.NewBuilder(d, render.Color{A: 255})
	var dl render.DisplayList
	bl.Build(d.Handles(), v, &dl)
	b.ResetTimer()
	for range b.N {
		paint(b, 1600, 1000, v, &dl)
	}
}

// BenchmarkPaintFill measures a full frame of 5,000 filled 16-gons (dots, filled arrowheads).
func BenchmarkPaintFill(b *testing.B) {
	r := rand.New(rand.NewSource(1))
	var dl render.DisplayList
	for range 5000 {
		c := geom.V(r.Float64()*10000, r.Float64()*10000)
		pts := make([]geom.Vec2, 16)
		for k := range pts {
			a := float64(k) * math.Pi / 8
			pts[k] = c.Add(geom.V(40*math.Cos(a), 40*math.Sin(a)))
		}
		dl.Fill(pts, render.Color{R: 255, A: 255})
	}
	v := render.DefaultView(1600, 1000)
	v.Fit(geom.BoxOf(geom.V(0, 0), geom.V(10000, 10000)), 0)
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		paint(b, 1600, 1000, v, &dl)
	}
}
