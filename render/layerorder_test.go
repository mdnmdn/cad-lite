package render_test

import (
	"cad-lite/doc"
	"cad-lite/geom"
	"cad-lite/render"
	"slices"
	"testing"
)

func TestBuilderLayerOrderKeepsCandidateStorageAndInsertionAtomic(t *testing.T) {
	d := doc.New()
	d.AddLayer("Front", 1)
	d.AddLayer("Back", 5)
	tx, _ := d.Begin()
	if err := tx.DefineBlock(&doc.Block{Name: "symbol", Entities: []doc.Entity{&doc.Line{EntityCommon: doc.EntityCommon{Layer: "Back", Color: 2}, End: geom.V(1, 0)}, &doc.Line{EntityCommon: doc.EntityCommon{Layer: "Front", Color: 3}, End: geom.V(0, 1)}}}); err != nil {
		t.Fatal(err)
	}
	frontOld := tx.Add(&doc.Line{EntityCommon: doc.EntityCommon{Layer: "Front"}, End: geom.V(2, 0)})
	back := tx.Add(&doc.Line{EntityCommon: doc.EntityCommon{Layer: "Back"}, End: geom.V(3, 0)})
	frontNew := tx.Add(&doc.Line{EntityCommon: doc.EntityCommon{Layer: "Front"}, End: geom.V(4, 0)})
	in := doc.NewInsert("symbol", geom.V(0, 0))
	in.Layer = "Front"
	reference := tx.Add(in)
	tx.Commit()
	candidates := slices.Clone(d.Handles())
	original := slices.Clone(candidates)
	b := render.NewBuilder(d, render.Color{A: 255})
	v := render.DefaultView(800, 600)
	var dl render.DisplayList
	b.Build(candidates, v, &dl)
	got := []doc.Handle{}
	for _, item := range dl.Items {
		got = append(got, item.Owner)
	}
	if !slices.Equal(got, []doc.Handle{back, frontOld, frontNew, reference, reference}) {
		t.Fatal("draw order", got)
	}
	if !slices.Equal(candidates, original) || !slices.Equal(d.Handles(), original) {
		t.Fatal("render mutated storage order")
	}
	if dl.Items[len(dl.Items)-2].Color != render.ACI(2) || dl.Items[len(dl.Items)-1].Color != render.ACI(3) {
		t.Fatal("definition internal order changed")
	}
	tx, _ = d.Begin()
	_ = tx.SetLayerOrder([]string{"Back", "0", "Front"})
	tx.Commit()
	dl.Items = nil
	b.Build(candidates, v, &dl)
	if dl.Items[len(dl.Items)-1].Owner != back {
		t.Fatal("reorder not applied")
	}
}
