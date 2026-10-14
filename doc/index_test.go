package doc_test

import (
	"fmt"
	"slices"
	"testing"

	"cad-lite/doc"
	"cad-lite/geom"
)

// fill adds n lines in one transaction and returns their handles.
func fill(tb testing.TB, d *doc.Drawing, n int) []doc.Handle {
	tb.Helper()
	tx, err := d.Begin()
	if err != nil {
		tb.Fatal(err)
	}
	hs := make([]doc.Handle, n)
	for i := range hs {
		hs[i] = tx.Add(&doc.Line{End: geom.V(float64(i), 1)})
	}
	tx.Commit()
	return hs
}

// checkOrder fails unless Handles is exactly the ascending set of live handles.
func checkOrder(t *testing.T, d *doc.Drawing, want []doc.Handle) {
	t.Helper()
	want = slices.Clone(want)
	slices.Sort(want)
	got := d.Handles()
	if !slices.Equal(got, want) || d.Len() != len(want) {
		t.Fatalf("Handles = %v (Len %d), want %v", got, d.Len(), want)
	}
	for _, h := range got {
		if d.Entity(h) == nil {
			t.Fatalf("handle %v listed but absent", h)
		}
	}
}

func TestEraseKeepsDrawOrderThroughUndoRedo(t *testing.T) {
	d := doc.New()
	hs := fill(t, d, 10)
	all := slices.Clone(hs)

	tx, _ := d.Begin()
	for _, i := range []int{0, 3, 4, 9} {
		tx.Erase(hs[i])
	}
	tx.Erase(hs[3]) // erasing twice is harmless
	tx.Commit()
	left := []doc.Handle{hs[1], hs[2], hs[5], hs[6], hs[7], hs[8]}
	checkOrder(t, d, left)

	for range 3 {
		d.Undo()
		checkOrder(t, d, all)
		d.Redo()
		checkOrder(t, d, left)
	}
	// Undo and redo without reading Handles in between, so erased slots pile up.
	for range 50 {
		d.Undo()
		d.Redo()
	}
	checkOrder(t, d, left)
	d.Undo()
	checkOrder(t, d, all)
}

func TestEraseAbortAndReAdd(t *testing.T) {
	d := doc.New()
	hs := fill(t, d, 5)
	tx, _ := d.Begin()
	tx.Erase(hs[2])
	tx.Erase(hs[0])
	n := tx.Add(&doc.Circle{Radius: 1})
	tx.Abort()
	if d.Entity(n) != nil {
		t.Fatal("aborted add survived")
	}
	checkOrder(t, d, hs)

	tx, _ = d.Begin()
	tx.Erase(hs[1])
	n = tx.Add(&doc.Circle{Radius: 1})
	tx.Commit()
	checkOrder(t, d, []doc.Handle{hs[0], hs[2], hs[3], hs[4], n})
	d.Undo()
	checkOrder(t, d, hs)
	d.Redo()
	checkOrder(t, d, []doc.Handle{hs[0], hs[2], hs[3], hs[4], n})
}

func TestEraseThenGroupUndo(t *testing.T) {
	d := doc.New()
	hs := fill(t, d, 6)
	d.BeginGroup()
	for _, h := range hs[:3] {
		tx, _ := d.Begin()
		tx.Erase(h)
		tx.Commit()
	}
	d.EndGroup()
	checkOrder(t, d, hs[3:])
	d.Undo()
	checkOrder(t, d, hs)
	d.Redo()
	checkOrder(t, d, hs[3:])
}

func TestLayerLookupFollowsTableEdits(t *testing.T) {
	d := doc.New()
	if l := d.Layer("0"); l == nil || l.Name != "0" {
		t.Fatal("layer 0 missing")
	}
	a := d.AddLayer("Walls", 1)
	if d.AddLayer("Walls", 5) != a || a.Color != 1 {
		t.Fatal("AddLayer duplicated an existing layer")
	}
	if d.Layer("Walls") != a {
		t.Fatal("added layer not found")
	}
	if d.Layer("WALLS") != nil {
		t.Fatal("layer names are case-sensitive")
	}
	if !d.RemoveLayer("Walls") || d.Layer("Walls") != nil {
		t.Fatal("removed layer still found")
	}
	if b := d.AddLayer("Walls", 2); d.Layer("Walls") != b || b == a {
		t.Fatal("re-added layer not found")
	}
	// Entity edits, undo and redo do not touch the layer table.
	d.CurrentLayer = "Walls"
	fill(t, d, 3)
	d.Undo()
	d.Redo()
	if d.Layer("Walls") == nil || len(d.Layers()) != 2 {
		t.Fatal("layer table changed by undo/redo")
	}
}

func TestLayerLookupIgnoresStaleName(t *testing.T) {
	d := doc.New()
	l := d.AddLayer("old", 1)
	l.Name = "new" // unsupported, but must not return a layer under a name it no longer has
	if d.Layer("old") != nil {
		t.Fatal("stale name still resolves")
	}
	if d.Layer("new") != l {
		t.Fatal("renamed layer not found after the index rebuilt")
	}
}

func BenchmarkLayerLookup(b *testing.B) {
	d := doc.New()
	names := make([]string, 200)
	for i := range names {
		names[i] = fmt.Sprintf("LAYER-%03d", i)
		d.AddLayer(names[i], 7)
	}
	b.ResetTimer()
	for i := range b.N {
		if d.Layer(names[i%len(names)]) == nil {
			b.Fatal("missing")
		}
	}
}

func BenchmarkErase10k(b *testing.B) {
	for range b.N {
		b.StopTimer()
		d := doc.New()
		hs := fill(b, d, 10000)
		d.Handles()
		b.StartTimer()
		tx, _ := d.Begin()
		for _, h := range hs {
			tx.Erase(h)
		}
		tx.Commit()
		d.Handles()
	}
}
