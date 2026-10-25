package doc_test

import (
	"math"
	"testing"

	"cad-lite/doc"
	"cad-lite/geom"
)

func TestLineweightDXFEncoding(t *testing.T) {
	cases := []struct {
		dxf  int
		want doc.Lineweight
	}{{-1, doc.LineweightByLayer}, {-2, doc.LineweightByBlock}, {-3, doc.LineweightDefault}, {0, doc.LineweightZero}, {35, 35}, {211, 211}}
	for _, c := range cases {
		if got := doc.LineweightFromDXF(c.dxf); got != c.want || got.DXF() != c.dxf {
			t.Errorf("DXF %d: %v, back %d", c.dxf, got, got.DXF())
		}
	}
	if doc.LineweightFromDXF(-9) != doc.LineweightByLayer || doc.LineweightFromDXF(500) != doc.LineweightByLayer {
		t.Fatal("out of range not ByLayer")
	}
	if h, ok := doc.LineweightDefault.Hundredths(); !ok || h != 25 {
		t.Fatalf("default %d %v", h, ok)
	}
	if _, ok := doc.LineweightByBlock.Hundredths(); ok {
		t.Fatal("ByBlock has a width")
	}
	var c doc.EntityCommon
	if c.LtScale() != 1 || c.Lineweight != doc.LineweightByLayer || !doc.IsByLayerLinetype(c.Linetype) {
		t.Fatal("zero EntityCommon is not ByLayer")
	}
}

func TestResolveAppearance(t *testing.T) {
	d := doc.New()
	l := d.AddLayer("walls", 3)
	l.Linetype, l.Lineweight = "DASHED", 50
	d.LTScale = 2
	byLayer := doc.EntityCommon{Layer: "walls", Color: doc.ByLayer}
	a := d.Resolve(&byLayer, nil)
	if a.Color != 3 || a.Linetype != "DASHED" || a.Lineweight != 50 || a.LinetypeScale != 2 || a.Layer != "walls" {
		t.Fatalf("ByLayer %+v", a)
	}
	// ByBlock outside a block: colour 7, CONTINUOUS, Default (LWDEFAULT, 0.25 mm unless set).
	byBlock := doc.EntityCommon{Layer: "walls", Color: doc.ByBlock, Linetype: "byblock", Lineweight: doc.LineweightByBlock, LinetypeScale: 0.5}
	a = d.Resolve(&byBlock, nil)
	if a.Color != 7 || a.Linetype != doc.LinetypeContinuous || a.Lineweight != 25 || a.LinetypeScale != 1 {
		t.Fatalf("ByBlock in model space %+v", a)
	}
	d.LWDefault = 18
	if a := d.Resolve(&doc.EntityCommon{Layer: "0", Lineweight: doc.LineweightDefault}, nil); a.Lineweight != 18 {
		t.Fatalf("Default with LWDEFAULT 0.18: %v", a.Lineweight)
	}
	d.LWDefault = 0
	// Inside a block reference: ByBlock takes the parent's, layer 0 the parent's layer.
	parent := doc.Appearance{Layer: "walls", Color: 1, Linetype: "HIDDEN", LinetypeScale: 1, Lineweight: 70}
	a = d.Resolve(&byBlock, &parent)
	if a.Color != 1 || a.Linetype != "HIDDEN" || a.Lineweight != 70 {
		t.Fatalf("ByBlock in a block %+v", a)
	}
	onZero := doc.EntityCommon{Layer: "0", Color: doc.ByLayer}
	if a := d.Resolve(&onZero, &parent); a.Layer != "walls" || a.Color != 3 || a.Linetype != "DASHED" {
		t.Fatalf("layer 0 in a block %+v", a)
	}
	// A missing layer and explicit values.
	odd := doc.EntityCommon{Layer: "nowhere", Color: doc.ByLayer, Linetype: "CENTER", Lineweight: doc.LineweightZero}
	if a := d.Resolve(&odd, nil); a.Color != 7 || a.Linetype != "CENTER" || a.Lineweight != doc.LineweightZero {
		t.Fatalf("explicit %+v", a)
	}
	if got := d.ResolveColorIn(doc.ByBlock, doc.Appearance{Layer: "walls", Color: 5}); got != 5 {
		t.Fatalf("part ByBlock %d", got)
	}
	if got := d.ResolveColorIn(doc.ByLayer, doc.Appearance{Layer: "walls", Color: 5}); got != 3 {
		t.Fatalf("part ByLayer %d", got)
	}
}

func TestBlockPartsResolveLinetypeAndLineweight(t *testing.T) {
	d := doc.New()
	tx, _ := d.Begin()
	child := &doc.Line{EntityCommon: doc.EntityCommon{Layer: "0", Color: doc.ByBlock, Linetype: doc.LinetypeByBlock, Lineweight: doc.LineweightByBlock}, End: geom.V(1, 0)}
	if err := tx.DefineBlock(&doc.Block{Name: "b", Entities: []doc.Entity{child}}); err != nil {
		t.Fatal(err)
	}
	in := doc.NewInsert("b", geom.V(0, 0))
	in.Color, in.Linetype, in.Lineweight = 2, "PHANTOM", 30
	h := tx.Add(in)
	tx.Commit()
	parts := d.Entity(h).(*doc.Insert).Parts()
	if len(parts) != 1 || parts[0].Color != 2 || parts[0].Linetype != "PHANTOM" || parts[0].Lineweight != 30 || parts[0].Layer != "0" {
		t.Fatalf("parts %+v", parts)
	}
}

// Regression: Txn.Add turned an unset colour into ByLayer even with CECOLOR BYBLOCK, so ByBlock
// was lost.
func TestAddAppliesCurrentByBlockColour(t *testing.T) {
	d := doc.New()
	if d.CurrentColor != doc.ByLayer {
		t.Fatalf("new drawing CECOLOR %d", d.CurrentColor)
	}
	d.CurrentColor = doc.ByBlock
	tx, _ := d.Begin()
	h := tx.Add(&doc.Line{End: geom.V(1, 0)})
	tx.Commit()
	if c := d.Entity(h).Common().Color; c != doc.ByBlock {
		t.Fatalf("colour %d, want ByBlock", c)
	}
}

func TestAddAppliesCurrentProperties(t *testing.T) {
	d := doc.New()
	tx, _ := d.Begin()
	_ = tx.LoadLibraryLinetype("DASHED")
	if err := tx.SetCurrent(doc.CurrentProps{Layer: "0", Color: 4, Linetype: "DASHED", LinetypeScale: 0.5, Lineweight: 35, LTScale: 1, DimStyle: "ISO-25"}); err != nil {
		t.Fatal(err)
	}
	fresh := tx.Add(&doc.Line{End: geom.V(1, 0)})
	explicit := tx.Add(&doc.Line{EntityCommon: doc.EntityCommon{Color: doc.ByLayer, Linetype: doc.LinetypeByLayer, LinetypeScale: 1, Lineweight: doc.LineweightDefault}, End: geom.V(1, 0)})
	copied := tx.AddCopy(&doc.Line{EntityCommon: doc.EntityCommon{Color: doc.ByBlock}, End: geom.V(1, 0)})
	tx.Commit()
	if c := d.Entity(fresh).Common(); c.Color != 4 || c.Linetype != "DASHED" || c.LinetypeScale != 0.5 || c.Lineweight != 35 {
		t.Fatalf("fresh %+v", *c)
	}
	if c := d.Entity(explicit).Common(); c.Color != doc.ByLayer || c.Linetype != doc.LinetypeByLayer || c.LtScale() != 1 || c.Lineweight != doc.LineweightDefault {
		t.Fatalf("explicit %+v", *c)
	}
	if c := d.Entity(copied).Common(); c.Color != doc.ByBlock || c.Linetype != "" || c.Lineweight != doc.LineweightByLayer || c.Layer != "0" {
		t.Fatalf("copy %+v", *c)
	}
}

func TestNewEntityPropertiesUndo(t *testing.T) {
	d := doc.New()
	tx, _ := d.Begin()
	h := tx.Add(&doc.Line{End: geom.V(1, 0)})
	tx.Commit()
	tx, _ = d.Begin()
	tx.Modify(h, func(e doc.Entity) {
		c := e.Common()
		c.Linetype, c.LinetypeScale, c.Lineweight = "HIDDEN", 3, 50
	})
	tx.Commit()
	d.Undo()
	if c := d.Entity(h).Common(); c.Linetype != "" || c.LinetypeScale != 0 || c.Lineweight != 0 {
		t.Fatalf("undo %+v", *c)
	}
	d.Redo()
	if c := d.Entity(h).Common(); c.Linetype != "HIDDEN" || c.LinetypeScale != 3 || c.Lineweight != 50 {
		t.Fatalf("redo %+v", *c)
	}
}

func TestBlockPartsCarryAccumulatedScaleAndKind(t *testing.T) {
	d := doc.New()
	tx, _ := d.Begin()
	txt := &doc.Text{Height: 1, Value: "A", WidthFactor: 1}
	if err := tx.DefineBlock(&doc.Block{Name: "inner", Entities: []doc.Entity{&doc.Line{End: geom.V(1, 0)}, txt}}); err != nil {
		t.Fatal(err)
	}
	nest := doc.NewInsert("inner", geom.V(0, 0))
	nest.ScaleX, nest.ScaleY = 3, 3
	if err := tx.DefineBlock(&doc.Block{Name: "outer", Entities: []doc.Entity{nest}}); err != nil {
		t.Fatal(err)
	}
	in := doc.NewInsert("outer", geom.V(0, 0))
	in.ScaleX, in.ScaleY = 2, 2
	h := tx.Add(in)
	tx.Commit()
	parts := d.Entity(h).(*doc.Insert).Parts()
	if len(parts) != 2 || parts[0].Kind != doc.PartCurve || parts[1].Kind != doc.PartText {
		t.Fatalf("parts %+v", parts)
	}
	for _, p := range parts {
		if math.Abs(p.Scale-6) > 1e-9 {
			t.Fatalf("scale %g, want 6", p.Scale)
		}
	}
}
