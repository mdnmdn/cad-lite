package render_test

import (
	"testing"

	"cad-lite/doc"
	"cad-lite/geom"
	"cad-lite/render"
)

func TestByBlockInsideInsertsAndNestedInserts(t *testing.T) {
	d, b := dashDrawing(t, "HIDDEN")
	b.Lineweights = true
	tx, _ := d.Begin()
	inner := &doc.Line{Start: geom.V(0, 0), End: geom.V(100, 0)}
	inner.Color, inner.Linetype, inner.Lineweight = doc.ByBlock, doc.LinetypeByBlock, doc.LineweightByBlock
	_ = tx.DefineBlock(&doc.Block{Name: "inner", Entities: []doc.Entity{inner}})
	// The nested reference passes its parent's properties on (ByBlock itself).
	nested := doc.NewInsert("inner", geom.V(0, 0))
	nested.Color, nested.Linetype, nested.Lineweight = doc.ByBlock, doc.LinetypeByBlock, doc.LineweightByBlock
	fixed := &doc.Line{Start: geom.V(0, 10), End: geom.V(100, 10)}
	fixed.Color = 3
	_ = tx.DefineBlock(&doc.Block{Name: "outer", Entities: []doc.Entity{nested, fixed}})
	in := doc.NewInsert("outer", geom.V(0, 0))
	in.Color, in.Linetype, in.Lineweight = 1, "HIDDEN", 100
	h := tx.Add(in)
	tx.Commit()
	v := render.DefaultView(800, 600)
	var red, green int
	for _, it := range runs(b.Items(h, v)) {
		switch it.Color {
		case render.ACI(1):
			red++
			if it.Weight < 3.7 || it.Weight > 3.9 {
				t.Fatalf("ByBlock lineweight: %v px, want the reference's 1.00 mm", it.Weight)
			}
		case render.ACI(3):
			green++
		default:
			t.Fatalf("unexpected colour %v", it.Color)
		}
	}
	// HIDDEN (6.35, -3.175) over 100 units: dashed, from the reference two levels up.
	if red < 5 || green != 1 {
		t.Fatalf("red dashes %d (want the reference's HIDDEN), green items %d (want 1 solid)", red, green)
	}

	// A reference scaled 2 scales the pattern with it.
	tx, _ = d.Begin()
	big := doc.NewInsert("inner", geom.V(0, 100))
	big.ScaleX, big.ScaleY, big.Linetype = 2, 2, "HIDDEN"
	hb := tx.Add(big)
	tx.Commit()
	// The reference's own linetype scale does not reach its children (each has its own, as in
	// AutoCAD); its geometric scale does.
	if n := len(runs(b.Items(hb, v))); n != red {
		t.Fatalf("scaled reference: %d dashes, want %d (the unscaled count over twice the length)", n, red)
	}
}

func TestPerEntityLineweightAndLWDEFAULT(t *testing.T) {
	d, b := dashDrawing(t)
	l := d.AddLayer("thick", 7)
	l.Lineweight = 100
	b.SyncLayers()
	own := &doc.Line{End: geom.V(10, 0)}
	own.Layer, own.Lineweight = "thick", 50
	zero := &doc.Line{End: geom.V(10, 0)}
	zero.Layer, zero.Lineweight = "thick", doc.LineweightZero
	byLayer := &doc.Line{End: geom.V(10, 0)}
	byLayer.Layer = "thick"
	def := &doc.Line{End: geom.V(10, 0)}
	def.Lineweight = doc.LineweightDefault
	ho, hz, hl, hd := add(t, d, own), add(t, d, zero), add(t, d, byLayer), add(t, d, def)
	v := render.DefaultView(800, 600)
	w := func(h doc.Handle) float32 { return runs(b.Items(h, v))[0].Weight }
	if w(ho) != 1 {
		t.Fatal("lineweights drawn with LWDISPLAY off")
	}
	b.Lineweights = true
	b.InvalidateAll()
	near := func(got float32, mm float64) bool {
		want := mm * 96 / 25.4
		return float64(got) > want-0.01 && float64(got) < want+0.01
	}
	if !near(w(ho), 0.5) || w(hz) != 1 || !near(w(hl), 1) || w(hd) != 1 {
		t.Fatalf("own %v, zero %v, by layer %v, default %v", w(ho), w(hz), w(hl), w(hd))
	}
	tx, _ := d.Begin()
	cur := d.Current()
	cur.LWDefault = 80
	if err := tx.SetCurrent(cur); err != nil {
		t.Fatal(err)
	}
	tx.Commit()
	if !near(w(hd), 0.8) {
		t.Fatalf("LWDEFAULT 0.80 mm: default draws %v px", w(hd))
	}
}

func TestSupportEditsInvalidateTheCache(t *testing.T) {
	d, b := dashDrawing(t, "HIDDEN", "ACAD_ISO02W100")
	h := add(t, d, &doc.Line{End: geom.V(100, 0)})
	v := render.DefaultView(800, 600)
	if n := len(runs(b.Items(h, v))); n != 1 {
		t.Fatalf("continuous: %d items", n)
	}
	// A journalled layer edit.
	tx, _ := d.Begin()
	_ = tx.ModifyLayer("0", func(l *doc.Layer) { l.Linetype = "HIDDEN" })
	tx.Commit()
	hidden := len(runs(b.Items(h, v)))
	if hidden < 5 {
		t.Fatalf("layer linetype edit: %d items, want dashes", hidden)
	}
	// A redefined linetype record.
	tx, _ = d.Begin()
	_ = tx.SetLinetype(doc.Linetype{Name: "HIDDEN", Pattern: []float64{20, -5}})
	tx.Commit()
	if n := len(runs(b.Items(h, v))); n == hidden || n < 2 {
		t.Fatalf("linetype redefinition: %d items, was %d", n, hidden)
	}
	// A direct layer edit (the layers panel), seen by SyncLayers.
	d.Layer("0").Linetype = ""
	if !b.SyncLayers() || len(runs(b.Items(h, v))) != 1 {
		t.Fatal("direct layer linetype edit kept the cache")
	}
	d.LTScale = 3
	if !b.SyncLayers() {
		t.Fatal("direct LTSCALE edit kept the cache")
	}
	// Changing the current layer alone changes no appearance and keeps the cache.
	runs(b.Items(h, v))
	tx, _ = d.Begin()
	cur := d.Current()
	cur.Layer = "0"
	cur.Color = 3
	_ = tx.SetCurrent(cur)
	tx.Commit()
	if b.Cached() == 0 {
		t.Fatal("a current-colour edit dropped the cache")
	}
}

func TestDimensionStyleEditRedrawsDimension(t *testing.T) {
	d, b := dashDrawing(t)
	tx, _ := d.Begin()
	dm := &doc.Dimension{Kind: doc.DimRotated, Ext1: geom.V(0, 0), Ext2: geom.V(100, 0), DimLine: geom.V(0, 20)}
	dm.SetStyle(d, doc.DefaultDimStyle())
	dm.Recompute()
	h := tx.Add(dm)
	tx.Commit()
	v := render.DefaultView(800, 600)
	if c := runs(b.Items(h, v))[0].Color; c != render.ACI(7).WithAlpha(255) && c != (render.Color{R: 255, G: 255, B: 255, A: 255}) {
		t.Fatalf("ByBlock extension line: %v", c)
	}
	st, _ := d.DimStyle(dm.StyleName)
	st.ExtColor = 1
	tx, _ = d.Begin()
	if err := tx.SetDimStyle(st); err != nil {
		t.Fatal(err)
	}
	tx.Commit()
	if c := runs(b.Items(h, v))[0].Color; c != render.ACI(1) {
		t.Fatalf("DIMCLRE 1 after a style edit: %v", c)
	}
}

func TestDimensionArrowKindsAndColours(t *testing.T) {
	d, b := dashDrawing(t, "HIDDEN")
	v := render.DefaultView(800, 600)
	v.Scale = 8
	type want struct{ fills, strokes int }
	cases := map[string]want{
		"":              {1, 1},
		"_ClosedBlank":  {0, 1},
		"_Closed":       {0, 1},
		"_Dot":          {1, 1},
		"_DotSmall":     {1, 1},
		"_DotBlank":     {0, 1},
		"_ArchTick":     {1, 1},
		"_Oblique":      {0, 1},
		"_Open":         {0, 2},
		"_Open90":       {0, 2},
		"_Open30":       {0, 2},
		"_Origin":       {0, 1},
		"_Origin2":      {0, 2},
		"_Small":        {1, 1},
		"_BoxBlank":     {0, 1},
		"_BoxFilled":    {1, 1},
		"_DatumBlank":   {0, 1},
		"_DatumFilled":  {1, 1},
		"_Integral":     {0, 2},
		"_None":         {0, 0},
		"SomeUserBlock": {1, 1},
	}
	for block, w := range cases {
		tx, _ := d.Begin()
		dm := &doc.Dimension{Kind: doc.DimRotated, Ext1: geom.V(0, 0), Ext2: geom.V(100, 0), DimLine: geom.V(0, 20)}
		dm.Color, dm.Linetype = 4, "HIDDEN"
		st := doc.DefaultDimStyle()
		st.ArrowBlock = block
		st.LineColor, st.ExtColor, st.TextColor = 1, doc.ByLayer, 2
		dm.SetStyle(d, st)
		dm.Recompute()
		h := tx.Add(dm)
		tx.Commit()
		var fills, arrowStrokes, ext, text int
		for _, it := range runs(b.Items(h, v)) {
			switch it.Color {
			case render.ACI(1): // DIMCLRD: dimension line pieces and arrows
				if it.Kind == render.ItemFill {
					fills++
				} else if !(len(it.Points) == 2 && it.Points[0].Y == it.Points[1].Y && it.Points[0].Y == 20 && !it.Closed) || block == "_Oblique" || block == "_ArchTick" {
					arrowStrokes++
				}
			case render.Color{R: 255, G: 255, B: 255, A: 255}: // DIMCLRE ByLayer: layer 0 is 7
				ext++
			case render.ACI(2):
				text++
			default:
				t.Fatalf("%q: unexpected colour %v", block, it.Color)
			}
		}
		if fills != 2*w.fills {
			t.Errorf("%q: %d fills, want %d", block, fills, 2*w.fills)
		}
		if w.strokes == 0 && arrowStrokes != 0 || w.strokes > 0 && arrowStrokes < 2*w.strokes {
			t.Errorf("%q: %d arrow strokes, want %d", block, arrowStrokes, 2*w.strokes)
		}
		// Extension lines take the dimension's HIDDEN linetype; text draws solid.
		if ext <= 2 || text == 0 {
			t.Errorf("%q: %d extension line items (want dashes), %d text items", block, ext, text)
		}
	}
}

func TestBuildSeesDirectLayerEdits(t *testing.T) {
	d, b := dashDrawing(t, "HIDDEN")
	tx, _ := d.Begin()
	if err := tx.DefineBlock(&doc.Block{Name: "sym", Entities: []doc.Entity{&doc.Line{End: geom.V(100, 0)}}}); err != nil {
		t.Fatal(err)
	}
	h := tx.Add(doc.NewInsert("sym", geom.V(0, 0)))
	tx.Commit()
	v := render.DefaultView(800, 600)
	var dl render.DisplayList
	b.Build([]doc.Handle{h}, v, &dl)
	if len(runs(dl.Items)) != 1 {
		t.Fatalf("continuous: %d runs", len(runs(dl.Items)))
	}
	// The layers panel writes the layer in place, without a transaction or a sync call.
	d.Layer("0").Linetype, d.Layer("0").Color = "HIDDEN", 1
	dl.Reset()
	b.Build([]doc.Handle{h}, v, &dl)
	if n := len(runs(dl.Items)); n < 5 || dl.Items[0].Color != render.ACI(1) {
		t.Fatalf("direct edit: %d runs in %v, want red dashes", n, dl.Items[0].Color)
	}
}
