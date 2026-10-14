package doc_test

import (
	"testing"

	"go-draw/doc"
	"go-draw/geom"
)

func spline() *doc.Unknown {
	return &doc.Unknown{
		EntityCommon: doc.EntityCommon{H: 0x40, Layer: "0", Color: doc.ByLayer},
		Type:         "SPLINE",
		Raw:          []doc.Pair{{Code: 5, Value: "40"}, {Code: 10, Value: "100"}, {Code: 20, Value: "200"}},
	}
}

func TestUnknownIsInertAndDeepCloned(t *testing.T) {
	u := spline()
	if u.Class() != "SPLINE" || len(u.Geometry()) != 0 {
		t.Fatalf("class %q geometry %v", u.Class(), u.Geometry())
	}
	if b := u.Bounds(); b.Min != geom.V(100, 200) || b.Max != geom.V(100, 200) {
		t.Fatalf("bounds %+v", b)
	}
	u.Transform(geom.Translate(geom.V(5, 5)))
	if b := u.Bounds(); b.Min != geom.V(100, 200) {
		t.Fatalf("Transform moved the entity: %+v", b)
	}
	c := u.Clone().(*doc.Unknown)
	c.Raw[1].Value = "0"
	if u.Raw[1].Value != "100" {
		t.Fatal("clone shares Raw")
	}
	if src := u.Source(); src[0] != (doc.Pair{Code: 0, Value: "SPLINE"}) || len(src) != 4 {
		t.Fatalf("source %v", src)
	}
	if !(&doc.Unknown{Type: "X"}).Bounds().IsEmpty() {
		t.Fatal("no points must give an empty box")
	}
}

func TestUnknownIsNeverEditedByTransactions(t *testing.T) {
	d := doc.New()
	d.Load(spline())
	d.Load(&doc.Line{EntityCommon: doc.EntityCommon{H: 0x41, Layer: "0"}, End: geom.V(1, 0)})
	tx, _ := d.Begin()
	if h := tx.Add(spline()); h != 0 {
		t.Fatalf("Add of an Unknown returned %v", h)
	}
	called := false
	tx.Modify(0x40, func(doc.Entity) { called = true })
	tx.Erase(0x40)
	if called || d.Entity(0x40) == nil || tx.Touched() {
		t.Fatal("transaction edited an Unknown")
	}
	tx.Modify(0x41, func(e doc.Entity) { e.Transform(geom.Translate(geom.V(1, 1))) })
	if cs := tx.Commit(); len(cs.Modified) != 1 || d.Len() != 2 {
		t.Fatalf("change set %+v, %d entities", cs, d.Len())
	}
}

func TestUnknownLoadKeepsHandleAndRaisesSeed(t *testing.T) {
	d := doc.New()
	d.Load(spline())
	if d.Entity(0x40) == nil || d.Header.HandSeed != 0x100 {
		t.Fatalf("handle or seed wrong: %v", d.Header.HandSeed)
	}
	u := spline()
	u.H = 0x500
	d.Load(u)
	if d.Header.HandSeed != 0x501 {
		t.Fatalf("seed %v", d.Header.HandSeed)
	}
}

func TestExtentsIgnoreUnknown(t *testing.T) {
	d := doc.New()
	d.Load(spline())
	if !d.Extents().IsEmpty() {
		t.Fatalf("extents %+v", d.Extents())
	}
	d.Load(&doc.Line{EntityCommon: doc.EntityCommon{H: 0x41, Layer: "0"}, End: geom.V(1, 0)})
	if b := d.Extents(); b.Max != geom.V(1, 0) {
		t.Fatalf("extents %+v", b)
	}
}
