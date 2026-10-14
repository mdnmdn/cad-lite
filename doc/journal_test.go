package doc

import (
	"testing"

	"go-draw/geom"
)

func addLine(t *testing.T, d *Drawing, x float64) Handle {
	t.Helper()
	tx, err := d.Begin()
	if err != nil {
		t.Fatal(err)
	}
	h := tx.Add(&Line{End: geom.V(x, 0)})
	tx.Commit()
	return h
}

func TestUndoToMark(t *testing.T) {
	d := New()
	addLine(t, d, 1)
	d.Mark()
	addLine(t, d, 2)
	addLine(t, d, 3)
	if n := d.UndoToMark(); n != 2 || d.Len() != 1 {
		t.Fatalf("UndoToMark = %d, %d entities left; want 2, 1", n, d.Len())
	}
	if d.HasMark() || d.UndoToMark() != 0 {
		t.Fatal("the marker should be consumed")
	}
}

func TestMarkDroppedByUndoPast(t *testing.T) {
	d := New()
	addLine(t, d, 1)
	addLine(t, d, 2)
	d.Mark()
	d.UndoN(2)
	if d.HasMark() {
		t.Fatal("a marker above the undo depth must be dropped")
	}
}

func TestGroupIsOneStep(t *testing.T) {
	d := New()
	addLine(t, d, 1)
	d.BeginGroup()
	a := addLine(t, d, 2)
	d.BeginGroup()
	addLine(t, d, 3)
	tx, _ := d.Begin()
	tx.Modify(a, func(e Entity) { e.(*Line).End = geom.V(9, 9) })
	tx.Commit()
	d.EndGroup()
	if d.UndoDepth() != 4 {
		t.Fatalf("inner EndGroup must not merge: depth %d", d.UndoDepth())
	}
	d.EndGroup()
	if d.UndoDepth() != 2 {
		t.Fatalf("depth after merge = %d, want 2", d.UndoDepth())
	}
	if !d.Undo() || d.Len() != 1 {
		t.Fatalf("one Undo must remove the whole group, %d entities left", d.Len())
	}
	if !d.Redo() || d.Len() != 3 || d.Entity(a).(*Line).End != geom.V(9, 9) {
		t.Fatal("Redo must restore the group's final state")
	}
}

func TestUndoNRedoN(t *testing.T) {
	d := New()
	for i := 0; i < 3; i++ {
		addLine(t, d, float64(i))
	}
	if n := d.UndoN(5); n != 3 || d.Len() != 0 {
		t.Fatalf("UndoN(5) = %d, %d left", n, d.Len())
	}
	if n := d.RedoN(2); n != 2 || d.Len() != 2 {
		t.Fatalf("RedoN(2) = %d, %d entities", n, d.Len())
	}
}
