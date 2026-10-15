package doc_test

import (
	"cad-lite/doc"
	"slices"
	"testing"
)

func layerNames(d *doc.Drawing) []string {
	var out []string
	for _, l := range d.Layers() {
		out = append(out, l.Name)
	}
	return out
}
func wantLayers(t *testing.T, d *doc.Drawing, want ...string) {
	t.Helper()
	if got := layerNames(d); !slices.Equal(got, want) {
		t.Fatalf("layers=%v want%v", got, want)
	}
}
func TestLayerOrderValidationAndNetNoop(t *testing.T) {
	d := doc.New()
	d.AddLayer("A", 1)
	d.AddLayer("B", 3)
	notifications := 0
	d.OnChange(func(doc.ChangeSet) { notifications++ })
	for _, names := range [][]string{{"0", "A"}, {"0", "A", "A"}, {"0", "A", "missing"}} {
		tx, _ := d.Begin()
		if err := tx.SetLayerOrder(names); err == nil {
			t.Fatal("invalid permutation accepted", names)
		}
		if !tx.Commit().Empty() {
			t.Fatal("invalid order published")
		}
		wantLayers(t, d, "0", "A", "B")
	}
	tx, _ := d.Begin()
	if err := tx.SetLayerOrder([]string{"0", "A", "B"}); err != nil {
		t.Fatal(err)
	}
	if tx.Touched() || !tx.Commit().Empty() {
		t.Fatal("no-op touched")
	}
	tx, _ = d.Begin()
	_ = tx.SetLayerOrder([]string{"B", "0", "A"})
	_ = tx.SetLayerOrder([]string{"0", "A", "B"})
	if tx.Touched() || !tx.Commit().Empty() {
		t.Fatal("net no-op touched")
	}
	if d.UndoDepth() != 0 || notifications != 0 {
		t.Fatal("no-op history/notification", d.UndoDepth(), notifications)
	}
}
func TestLayerOrderUndoPreservesMetadataAndLaterLayers(t *testing.T) {
	d := doc.New()
	a := d.AddLayer("A", 1)
	a.Locked = true
	a.Lineweight = 100
	d.AddLayer("B", 3)
	d.CurrentLayer = "A"
	events := 0
	d.OnChange(func(cs doc.ChangeSet) {
		if !cs.LayerOrder {
			t.Fatal("missing order signal", cs)
		}
		events++
	})
	tx, _ := d.Begin()
	if err := tx.SetLayerOrder([]string{"B", "0", "A"}); err != nil {
		t.Fatal(err)
	}
	if cs := tx.Commit(); !cs.LayerOrder || len(cs.Modified) != 0 {
		t.Fatal(cs)
	}
	if d.UndoDepth() != 1 || d.CurrentLayer != "A" || !a.Locked || a.Lineweight != 100 {
		t.Fatal("reorder altered metadata")
	}
	d.AddLayer("Later", 5)
	a.Off = true
	d.CurrentLayer = "B"
	d.Undo()
	wantLayers(t, d, "0", "A", "B", "Later")
	if !d.Layer("A").Off || d.CurrentLayer != "B" {
		t.Fatal("undo altered direct edits")
	}
	d.Redo()
	wantLayers(t, d, "B", "0", "A", "Later")
	if events != 3 {
		t.Fatal(events)
	}
	for i, l := range d.Layers() {
		if d.LayerRank(l.Name) != i {
			t.Fatal("rank cache stale")
		}
	}
}
func TestLayerOrderAbortGroupedUndoAndSupport(t *testing.T) {
	d := doc.New()
	d.AddLayer("A", 1)
	tx, _ := d.Begin()
	_ = tx.SetLayerOrder([]string{"A", "0"})
	tx.Abort()
	wantLayers(t, d, "0", "A")
	if d.UndoDepth() != 0 {
		t.Fatal("abort journaled")
	}
	src := doc.New()
	src.AddLayer("B", 3)
	d.BeginGroup()
	tx, _ = d.Begin()
	tx.ImportBlockSupport(src)
	tx.Commit()
	tx, _ = d.Begin()
	_ = tx.SetLayerOrder([]string{"B", "A", "0"})
	tx.Commit()
	tx, _ = d.Begin()
	_ = tx.SetLayerOrder([]string{"A", "B", "0"})
	tx.Commit()
	d.EndGroup()
	if d.UndoDepth() != 1 {
		t.Fatal(d.UndoDepth())
	}
	d.Layer("A").Frozen = true
	d.AddLayer("Later", 5)
	d.Undo()
	wantLayers(t, d, "0", "A", "Later")
	if !d.Layer("A").Frozen {
		t.Fatal("property restored by order")
	}
	d.Redo()
	wantLayers(t, d, "A", "B", "Later", "0")
	if !d.Layer("A").Frozen {
		t.Fatal("property changed on redo")
	}
	tx, _ = d.Begin()
	if err := d.LoadLayerOrder([]string{"0", "A", "B", "Later"}); err != doc.ErrTxnOpen {
		t.Fatal(err)
	}
	tx.Abort()
}
