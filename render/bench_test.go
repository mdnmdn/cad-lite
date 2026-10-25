package render_test

import (
	"math/rand"
	"testing"

	"cad-lite/doc"
	"cad-lite/geom"
	"cad-lite/render"
)

// dashedDrawing returns n entities (lines, circles and bulged polylines) spread over a 10000-unit
// square, every one in an ISO linetype when dashed is set, and a view that shows them all at
// about one pixel per 8 units, where a 15-unit ISO dash period is about 2 pixels.
func dashedDrawing(b *testing.B, n int, dashed bool) (*doc.Drawing, []doc.Handle, render.View) {
	b.Helper()
	d := doc.New()
	tx, _ := d.Begin()
	types := []string{"ACAD_ISO02W100", "ACAD_ISO04W100", "ACAD_ISO07W100", "ACAD_ISO10W100", "CENTER", "HIDDEN", "PHANTOM", "DASHDOT"}
	for _, lt := range types {
		_ = tx.LoadLibraryLinetype(lt)
	}
	r := rand.New(rand.NewSource(1))
	for i := range n {
		p := geom.V(r.Float64()*10000, r.Float64()*10000)
		var e doc.Entity
		switch i % 4 {
		case 0, 1:
			e = &doc.Line{Start: p, End: p.Add(geom.V(r.Float64()*400-200, r.Float64()*400-200))}
		case 2:
			e = &doc.Circle{Center: p, Radius: 10 + r.Float64()*100}
		default:
			e = &doc.LWPolyline{Points: []geom.Vec2{p, p.Add(geom.V(150, 0)), p.Add(geom.V(150, 150)), p.Add(geom.V(0, 150))}, Bulges: []float64{0, 0.4, 0, 0}, Closed: true}
		}
		if dashed {
			e.Common().Linetype = types[i%len(types)]
		}
		tx.Add(e)
	}
	tx.Commit()
	v := render.DefaultView(1600, 1000)
	v.Fit(geom.BoxOf(geom.V(0, 0), geom.V(10000, 10000)), 0)
	v.Scale *= 4 // zoomed in: the dashes are a few pixels long
	return d, d.Handles(), v
}

func benchBuild(b *testing.B, dashed bool) {
	d, hs, v := dashedDrawing(b, 20000, dashed)
	bl := render.NewBuilder(d, render.Color{A: 255})
	var dl render.DisplayList
	b.ResetTimer()
	for range b.N {
		bl.InvalidateAll()
		dl.Reset()
		bl.Build(hs, v, &dl)
	}
	b.ReportMetric(float64(len(dl.Items)), "items")
}

// BenchmarkBuildDashed measures a full rebuild (a zoom into a new band) of 20k entities in ISO
// linetypes: tessellation, dashing and draw-order sort.
func BenchmarkBuildDashed(b *testing.B) { benchBuild(b, true) }

// BenchmarkBuildSolid is BenchmarkBuildDashed's drawing drawn continuous, for comparison.
func BenchmarkBuildSolid(b *testing.B) { benchBuild(b, false) }

// BenchmarkBuildDashedCached measures a frame within the same band: every entity's items come
// from the cache.
func BenchmarkBuildDashedCached(b *testing.B) {
	d, hs, v := dashedDrawing(b, 20000, true)
	bl := render.NewBuilder(d, render.Color{A: 255})
	var dl render.DisplayList
	bl.Build(hs, v, &dl)
	b.ResetTimer()
	for range b.N {
		dl.Reset()
		bl.Build(hs, v, &dl)
	}
}

// BenchmarkSyncUnchanged measures the per-frame support check of Build on a drawing with every
// library linetype and 50 layers, when nothing changed.
func BenchmarkSyncUnchanged(b *testing.B) {
	d := doc.New()
	tx, _ := d.Begin()
	for _, l := range doc.LibraryLinetypes() {
		_ = tx.LoadLibraryLinetype(l.Name)
	}
	for i := range 50 {
		_, _ = tx.AddLayer(string(rune('A'+i%26))+string(rune('a'+i/26)), 1+i%7)
	}
	tx.Commit()
	bl := render.NewBuilder(d, render.Color{A: 255})
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		if bl.SyncLayers() {
			b.Fatal("synced without a change")
		}
	}
}
