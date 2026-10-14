package render_test

import (
	"math"
	"testing"

	"cad-lite/doc"
	"cad-lite/geom"
	"cad-lite/render"
)

func TestBuilderDrawsLayerLineweightsOnlyWhenAsked(t *testing.T) {
	d := doc.New()
	l := d.AddLayer("thick", 1)
	l.Lineweight = 100 // 1.00 mm
	tx, _ := d.Begin()
	h := tx.Add(&doc.Line{EntityCommon: doc.EntityCommon{Layer: "thick"}, End: geom.V(10, 0)})
	h0 := tx.Add(&doc.Line{End: geom.V(0, 10)})
	tx.Commit()
	b := render.NewBuilder(d, render.Color{A: 255})
	v := render.DefaultView(800, 600)
	if w := b.Items(h, v)[0].Weight; w != 1 {
		t.Fatalf("lineweights off: weight %v, want 1", w)
	}
	b.Lineweights = true
	b.InvalidateAll()
	if w := b.Items(h, v)[0].Weight; w < 3.7 || w > 3.9 {
		t.Fatalf("1.00 mm at 96 dpi: weight %v, want about 3.78", w)
	}
	if w := b.Items(h0, v)[0].Weight; w != 1 {
		t.Fatalf("default lineweight: weight %v, want 1", w)
	}
}

func TestBuilderSyncLayersDropsCacheOnlyOnLayerChange(t *testing.T) {
	d := doc.New()
	tx, _ := d.Begin()
	h := tx.Add(&doc.Line{End: geom.V(10, 0)})
	tx.Commit()
	b := render.NewBuilder(d, render.Color{A: 255})
	v := render.DefaultView(800, 600)
	b.Items(h, v)
	if b.SyncLayers() || b.Cached() != 1 {
		t.Fatalf("unchanged layers dropped the cache (cached %d)", b.Cached())
	}
	d.Layer("0").Color = 1
	if !b.SyncLayers() || b.Cached() != 0 {
		t.Fatal("a layer colour change kept the cache")
	}
	if c := b.Items(h, v)[0].Color; c != render.ACI(1) {
		t.Fatalf("colour after sync = %v, want red", c)
	}
	d.AddLayer("walls", 4)
	if !b.SyncLayers() {
		t.Fatal("a new layer kept the cache")
	}
	d.Layer("walls").Lineweight = 50
	if !b.SyncLayers() {
		t.Fatal("a lineweight change kept the cache")
	}
	if b.SyncLayers() {
		t.Fatal("a second sync without changes dropped the cache")
	}
}

func TestBuilderDrawsTextStrokesOrOneLineWhenTiny(t *testing.T) {
	d := doc.New()
	tx, _ := d.Begin()
	h := tx.Add(doc.NewText(geom.V(0, 0), 10, 0, "HI"))
	tx.Commit()
	b := render.NewBuilder(d, render.Color{A: 255})
	v := render.DefaultView(800, 600)
	v.Scale = 4 // 40 px capitals
	if n := len(b.Items(h, v)); n != 4 {
		t.Fatalf("legible text: %d items, want its 4 strokes", n)
	}
	v.Scale = 0.1 // 1 px capitals
	items := b.Items(h, v)
	if len(items) != 1 || len(items[0].Points) != 2 || math.Abs(items[0].Points[0].Y-(10-10.0/3)/2) > 1e-12 {
		t.Fatalf("tiny text: %+v", items)
	}
}
