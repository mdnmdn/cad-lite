package render_test

import (
	"cad-lite/doc"
	"cad-lite/geom"
	"cad-lite/render"
	"cad-lite/spatial"
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
