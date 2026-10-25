package render_test

import (
	"cad-lite/doc"
	"cad-lite/geom"
	"cad-lite/render"
	"cad-lite/spatial"
	"math"
	"testing"
)

func TestBlockLayerVisibilityBoundsAndByBlock(t *testing.T) {
	d := doc.New()
	d.AddLayer("root", 1)
	child := d.AddLayer("child", 5)
	child.Off = true
	tx, _ := d.Begin()
	if err := tx.DefineBlock(&doc.Block{Name: "symbol", Entities: []doc.Entity{&doc.Line{EntityCommon: doc.EntityCommon{Layer: "child", Color: doc.ByBlock}, Start: geom.V(1000, 0), End: geom.V(1010, 0)}}}); err != nil {
		t.Fatal(err)
	}
	in := doc.NewInsert("symbol", geom.V(0, 0))
	in.Layer = "root"
	h := tx.Add(in)
	tx.Commit()
	ix := spatial.New()
	ix.Build(d)
	b := render.NewBuilder(d, render.Color{A: 255})
	d.OnChange(b.Invalidate)
	d.OnChange(func(cs doc.ChangeSet) { ix.Apply(d, cs) })
	v := render.DefaultView(800, 600)
	if len(b.Items(h, v)) != 0 {
		t.Fatal("hidden block child drawn")
	}
	if len(ix.Query(geom.BoxOf(geom.V(1000, -1), geom.V(1010, 1)))) != 1 {
		t.Fatal("hidden child missing from spatial bounds")
	}
	child.Off = false
	if !b.SyncLayers() {
		t.Fatal("visibility failed to invalidate")
	}
	items := b.Items(h, v)
	if len(items) != 1 || items[0].Color != render.ACI(1) {
		t.Fatal("ByBlock failed to inherit root color", items)
	}
	if d.RemoveLayer("child") {
		t.Fatal("purged block child layer")
	}
	tx, _ = d.Begin()
	_ = tx.DefineBlock(&doc.Block{Name: "symbol", Entities: []doc.Entity{&doc.Line{Start: geom.V(2000, 0), End: geom.V(2010, 0)}}})
	tx.Commit()
	if b.Cached() != 0 || len(ix.Query(geom.BoxOf(geom.V(2000, -1), geom.V(2010, 1)))) != 1 {
		t.Fatal("definition replacement stale")
	}
	d.Undo()
	if b.Cached() != 0 || len(ix.Query(geom.BoxOf(geom.V(1000, -1), geom.V(1010, 1)))) != 1 {
		t.Fatal("definition undo stale")
	}
}

// firstDash returns the length of the first dash the insert h draws.
func firstDash(t *testing.T, b *render.Builder, h doc.Handle) float64 {
	t.Helper()
	r := runs(b.Items(h, render.DefaultView(800, 600)))
	if len(r) == 0 {
		t.Fatal("no dashes drawn")
	}
	return r[0].Points[0].Dist(r[0].Points[len(r[0].Points)-1])
}

// Regression: a nested reference's pattern scaled with the outermost reference only.
func TestNestedInsertScalesPatternByAccumulatedScale(t *testing.T) {
	d, b := dashDrawing(t, "ACAD_ISO02W100")
	tx, _ := d.Begin()
	line := dashLine("ACAD_ISO02W100", 100)
	if err := tx.DefineBlock(&doc.Block{Name: "inner", Entities: []doc.Entity{line}}); err != nil {
		t.Fatal(err)
	}
	nest := doc.NewInsert("inner", geom.V(0, 0))
	nest.ScaleX, nest.ScaleY = 2, 2
	if err := tx.DefineBlock(&doc.Block{Name: "outer", Entities: []doc.Entity{nest}}); err != nil {
		t.Fatal(err)
	}
	flat := func(s float64) *doc.Insert {
		in := doc.NewInsert("inner", geom.V(0, 0))
		in.ScaleX, in.ScaleY = s, s
		return in
	}
	nested := doc.NewInsert("outer", geom.V(0, 0))
	nested.ScaleX, nested.ScaleY = 2, 2
	hNested, h2, h4 := tx.Add(nested), tx.Add(flat(2)), tx.Add(flat(4))
	tx.Commit()
	// The pattern is in model units, so the dash length shows the scale it was drawn with.
	d2, d4, dn := firstDash(t, b, h2), firstDash(t, b, h4), firstDash(t, b, hNested)
	if math.Abs(d4-2*d2) > 1e-9 {
		t.Fatalf("dashes of scale 2 and 4: %g, %g", d2, d4)
	}
	if math.Abs(dn-d4) > 1e-9 {
		t.Fatalf("nested 2x2 dash %g, want %g of scale 4 (scale 2 draws %g)", dn, d4, d2)
	}
}

// Regression: text inside a block took the part's dashed linetype.
func TestDashedTextInsideBlockDrawsSolid(t *testing.T) {
	d, b := dashDrawing(t, "ACAD_ISO02W100")
	tx, _ := d.Begin()
	txt := &doc.Text{Insert: geom.V(0, 0), Height: 100, Value: "HIL", WidthFactor: 1}
	txt.Linetype = "ACAD_ISO02W100"
	if err := tx.DefineBlock(&doc.Block{Name: "label", Entities: []doc.Entity{txt}}); err != nil {
		t.Fatal(err)
	}
	h := tx.Add(doc.NewInsert("label", geom.V(0, 0)))
	tx.Commit()
	items := b.Items(h, render.DefaultView(800, 600))
	if len(items) == 0 {
		t.Fatal("no text drawn")
	}
	for _, it := range items {
		if it.Starts != nil {
			t.Fatalf("block text drawn dashed: %+v", it)
		}
	}
}

// A dimension inside a block follows its linetype on the dimension and extension lines only,
// as a top-level one does; arrowheads and text stay solid.
func TestDimensionInsideBlockDashesOnlyItsLines(t *testing.T) {
	d, b := dashDrawing(t, "HIDDEN")
	tx, _ := d.Begin()
	dm := &doc.Dimension{Kind: doc.DimRotated, Ext1: geom.V(0, 0), Ext2: geom.V(100, 0), DimLine: geom.V(0, 20)}
	dm.Linetype = "HIDDEN"
	st := doc.DefaultDimStyle()
	st.ExtColor, st.LineColor, st.TextColor = 1, 2, 3
	dm.SetStyle(d, st)
	dm.Recompute()
	if err := tx.DefineBlock(&doc.Block{Name: "dim", Entities: []doc.Entity{dm}}); err != nil {
		t.Fatal(err)
	}
	h := tx.Add(doc.NewInsert("dim", geom.V(0, 0)))
	tx.Commit()
	v := render.DefaultView(800, 600)
	v.Scale = 8
	var dashed, text, arrows int
	for _, it := range b.Items(h, v) {
		switch {
		case it.Starts != nil:
			dashed++
			if it.Color == render.ACI(3) {
				t.Fatalf("text dashed: %+v", it)
			}
		case it.Color == render.ACI(3):
			text++
		case it.Color == render.ACI(2):
			arrows++
		}
	}
	if dashed == 0 || text == 0 || arrows == 0 {
		t.Fatalf("dashed %d, solid text %d, solid arrow strokes %d", dashed, text, arrows)
	}
}
