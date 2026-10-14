package render_test

import (
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
