package doc_test

import (
	"math"
	"math/rand/v2"
	"reflect"
	"testing"

	"cad-lite/doc"
	"cad-lite/geom"
)

func snapshot(d *doc.Drawing) map[doc.Handle]doc.Entity {
	out := map[doc.Handle]doc.Entity{}
	for _, h := range d.Handles() {
		out[h] = d.Entity(h).Clone().(doc.Entity)
	}
	return out
}

func TestTxnAddUsesCurrentLayerAndHandSeed(t *testing.T) {
	d := doc.New()
	d.AddLayer("walls", 1)
	d.CurrentLayer = "walls"
	tx, _ := d.Begin()
	h := tx.Add(&doc.Line{End: geom.V(1, 0)})
	cs := tx.Commit()
	if h != 0x100 || d.Header.HandSeed != 0x101 {
		t.Fatalf("handle %v seed %v", h, d.Header.HandSeed)
	}
	c := d.Entity(h).Common()
	if c.Layer != "walls" || c.Color != doc.ByLayer || c.Owner != doc.ModelSpace {
		t.Fatalf("common = %+v", *c)
	}
	if !reflect.DeepEqual(cs.Added, []doc.Handle{h}) {
		t.Fatalf("change set = %+v", cs)
	}
}

func TestOnlyOneTxnAtATime(t *testing.T) {
	d := doc.New()
	tx, _ := d.Begin()
	if _, err := d.Begin(); err != doc.ErrTxnOpen {
		t.Fatalf("second Begin err = %v", err)
	}
	tx.Abort()
	if _, err := d.Begin(); err != nil {
		t.Fatalf("Begin after Abort: %v", err)
	}
}

func TestAbortRestores(t *testing.T) {
	d := doc.New()
	tx, _ := d.Begin()
	h := tx.Add(&doc.Circle{Radius: 1})
	tx.Commit()
	tx, _ = d.Begin()
	tx.Modify(h, func(e doc.Entity) { e.(*doc.Circle).Radius = 5 })
	tx.Add(&doc.Line{})
	tx.Abort()
	if d.Len() != 1 || d.Entity(h).(*doc.Circle).Radius != 1 {
		t.Fatalf("abort left %d entities, radius %v", d.Len(), d.Entity(h).(*doc.Circle).Radius)
	}
}

func TestChangeListenersSeeCommitUndoRedo(t *testing.T) {
	d := doc.New()
	var got []doc.ChangeSet
	d.OnChange(func(cs doc.ChangeSet) { got = append(got, cs) })
	tx, _ := d.Begin()
	h := tx.Add(&doc.Line{})
	tx.Commit()
	d.Undo()
	d.Redo()
	if len(got) != 3 || got[1].Erased[0] != h || got[2].Added[0] != h {
		t.Fatalf("change sets = %+v", got)
	}
}

// TestRandomEditsUndoToOriginal applies random transactions, undoes them all
// and compares with the starting drawing, then redoes them all and compares
// with the final one.
func TestRandomEditsUndoToOriginal(t *testing.T) {
	r := rand.New(rand.NewPCG(1, 2))
	d := doc.New()
	tx, _ := d.Begin()
	for i := 0; i < 20; i++ {
		tx.Add(&doc.Line{Start: geom.V(r.Float64(), r.Float64()), End: geom.V(r.Float64(), r.Float64())})
	}
	tx.Commit()
	start := snapshot(d)
	steps := 0
	for i := 0; i < 200; i++ {
		tx, _ := d.Begin()
		hs := append([]doc.Handle(nil), d.Handles()...)
		for k := 0; k < 1+r.IntN(4); k++ {
			switch op := r.IntN(3); {
			case op == 0 || len(hs) == 0:
				tx.Add(&doc.Circle{Center: geom.V(r.Float64(), r.Float64()), Radius: r.Float64()})
			case op == 1:
				h := hs[r.IntN(len(hs))]
				tx.Modify(h, func(e doc.Entity) { e.Transform(geom.Rotate(geom.V(0, 0), r.Float64()*math.Pi)) })
			default:
				tx.Erase(hs[r.IntN(len(hs))])
			}
		}
		if !tx.Commit().Empty() {
			steps++
		}
	}
	end := snapshot(d)
	for ; steps > 0; steps-- {
		if !d.Undo() {
			t.Fatalf("undo ran out with %d steps left", steps)
		}
	}
	if !reflect.DeepEqual(snapshot(d), start) {
		t.Fatal("undoing everything did not restore the original drawing")
	}
	for d.Redo() {
	}
	if !reflect.DeepEqual(snapshot(d), end) {
		t.Fatal("redoing everything did not restore the final drawing")
	}
}

func TestArcFrom3Points(t *testing.T) {
	a, ok := doc.ArcFrom3Points(geom.V(1, 0), geom.V(0, 1), geom.V(-1, 0))
	if !ok || !a.C.Near(geom.V(0, 0), 1e-9) || math.Abs(a.R-1) > 1e-9 || math.Abs(a.Sweep()-math.Pi) > 1e-9 {
		t.Fatalf("ccw arc = %+v", a)
	}
	cw, ok := doc.ArcFrom3Points(geom.V(-1, 0), geom.V(0, 1), geom.V(1, 0))
	if !ok || !cw.MidPoint().Near(geom.V(0, 1), 1e-9) {
		t.Fatalf("cw arc = %+v, midpoint %v", cw, cw.MidPoint())
	}
	if _, ok := doc.ArcFrom3Points(geom.V(0, 0), geom.V(1, 1), geom.V(2, 2)); ok {
		t.Fatal("collinear points made an arc")
	}
}
