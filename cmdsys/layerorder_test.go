package cmdsys_test

import (
	"cad-lite/cmdsys"
	"cad-lite/doc"
	"cad-lite/geom"
	"cad-lite/spatial"
	"testing"
)

func TestPickLayerTieKeepsNearestAndNewestWithinLayer(t *testing.T) {
	d := doc.New()
	d.AddLayer("Front", 1)
	d.AddLayer("Back", 5)
	tx, _ := d.Begin()
	old := tx.Add(&doc.Line{EntityCommon: doc.EntityCommon{Layer: "Front"}, Start: geom.V(-10, 0), End: geom.V(10, 0)})
	back := tx.Add(&doc.Line{EntityCommon: doc.EntityCommon{Layer: "Back"}, Start: geom.V(-10, 0), End: geom.V(10, 0)})
	tx.Commit()
	ix := spatial.New()
	ix.Build(d)
	if h, _ := cmdsys.Pick(d, ix, geom.V(0, 0), 1); h != old {
		t.Fatal("new back-layer object won tie", h)
	}
	tx, _ = d.Begin()
	newer := tx.Add(&doc.Line{EntityCommon: doc.EntityCommon{Layer: "Front"}, Start: geom.V(-10, 0), End: geom.V(10, 0)})
	tx.Commit()
	ix.Build(d)
	if h, _ := cmdsys.Pick(d, ix, geom.V(0, 0), 1); h != newer {
		t.Fatal("newest within layer lost", h)
	}
	tx, _ = d.Begin()
	_ = tx.SetLayerOrder([]string{"Back", "0", "Front"})
	tx.Commit()
	if h, _ := cmdsys.Pick(d, ix, geom.V(0, 0), 1); h != back {
		t.Fatal("front changed but pick stale", h)
	}
	tx, _ = d.Begin()
	tx.Modify(back, func(e doc.Entity) { e.(*doc.Line).Start.Y = .5; e.(*doc.Line).End.Y = .5 })
	tx.Commit()
	ix.Build(d)
	if h, _ := cmdsys.Pick(d, ix, geom.V(0, 0), 1); h != newer {
		t.Fatal("far front object defeated nearest", h)
	}
}
