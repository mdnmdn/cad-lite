package doc_test

import (
	"slices"
	"testing"

	"cad-lite/doc"
	"cad-lite/geom"
)

func TestLinetypeLibrary(t *testing.T) {
	lib := doc.LibraryLinetypes()
	names := map[string]bool{}
	for _, l := range lib {
		names[l.Name] = true
		if len(l.Pattern) == 0 || l.Length() <= 0 || l.Description == "" {
			t.Errorf("%s: %+v", l.Name, l)
		}
	}
	for _, n := range []string{"ACAD_ISO02W100", "ACAD_ISO15W100", "DASHED", "DASHED2", "DASHEDX2", "HIDDEN", "CENTER",
		"PHANTOM", "DOT", "DASHDOT", "DIVIDE", "BORDER", "BORDERX2"} {
		if !names[n] {
			t.Errorf("library lacks %s", n)
		}
	}
	if len(lib) != 38 || names["CONTINUOUS"] {
		t.Fatalf("%d linetypes", len(lib))
	}
	if l, ok := doc.LibraryLinetype("acad_iso02w100"); !ok || !slices.Equal(l.Pattern, []float64{12, -3}) {
		t.Fatalf("ISO02 %+v", l)
	}
	if l, _ := doc.LibraryLinetype("ACAD_ISO07W100"); !slices.Equal(l.Pattern, []float64{0, -3}) || l.Continuous() {
		t.Fatalf("ISO dot %+v", l)
	}
	// Copies: changing one does not change the library.
	l, _ := doc.LibraryLinetype("DASHED")
	l.Pattern[0] = 99
	if again, _ := doc.LibraryLinetype("DASHED"); again.Pattern[0] != 12.7 {
		t.Fatal("library shared its slice")
	}
}

func TestLinetypeTableUndo(t *testing.T) {
	d := doc.New()
	if lts := d.Linetypes(); len(lts) != 1 || lts[0].Name != "CONTINUOUS" {
		t.Fatalf("new drawing %+v", lts)
	}
	var cs doc.ChangeSet
	d.OnChange(func(c doc.ChangeSet) { cs = c })
	tx, _ := d.Begin()
	if err := tx.LoadLibraryLinetype("hidden"); err != nil {
		t.Fatal(err)
	}
	if err := tx.LoadLibraryLinetype("NOPE"); err == nil {
		t.Fatal("unknown library name accepted")
	}
	if err := tx.SetLinetype(doc.Linetype{Name: "BYLAYER"}); err == nil {
		t.Fatal("reserved name accepted")
	}
	tx.Commit()
	if _, ok := d.Linetype("Hidden"); !ok || !cs.Support || d.UndoDepth() != 1 {
		t.Fatalf("load: %v %+v depth %d", ok, cs, d.UndoDepth())
	}
	d.Undo()
	if _, ok := d.Linetype("HIDDEN"); ok {
		t.Fatal("undo kept the linetype")
	}
	d.Redo()
	if _, ok := d.Linetype("HIDDEN"); !ok {
		t.Fatal("redo lost the linetype")
	}
	// In use: refused.
	d.Layer("0").Linetype = "HIDDEN"
	tx, _ = d.Begin()
	if err := tx.RemoveLinetype("HIDDEN"); err == nil {
		t.Fatal("removed a linetype in use")
	}
	if err := tx.RemoveLinetype("CONTINUOUS"); err == nil {
		t.Fatal("removed CONTINUOUS")
	}
	tx.Commit()
	if d.UndoDepth() != 1 {
		t.Fatal("a refused edit recorded a step")
	}
}

func TestLayerAndCurrentPropertyEditsUndo(t *testing.T) {
	d := doc.New()
	d.AddLayer("walls", 1)
	tx, _ := d.Begin()
	if err := tx.ModifyLayer("walls", func(l *doc.Layer) { l.Color, l.Lineweight, l.Linetype = 5, 50, "DASHED" }); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.AddLayer("doors", 3); err != nil {
		t.Fatal(err)
	}
	if err := tx.SetCurrent(doc.CurrentProps{Layer: "doors", Color: doc.ByBlock, Linetype: doc.LinetypeByBlock, LinetypeScale: 2, Lineweight: doc.LineweightByBlock, LTScale: 10, DimStyle: "ISO-25", LWDefault: 30}); err != nil {
		t.Fatal(err)
	}
	if err := tx.SetCurrent(doc.CurrentProps{Layer: "nowhere"}); err == nil {
		t.Fatal("missing layer accepted")
	}
	if err := tx.ModifyLayer("walls", func(l *doc.Layer) { l.Name = "x" }); err == nil || d.Layer("walls") == nil {
		t.Fatal("rename accepted")
	}
	tx.Commit()
	// A direct edit made after the step survives its undo.
	d.Layer("0").Color = 6
	d.Undo()
	l := d.Layer("walls")
	if l.Color != 1 || l.Lineweight != 0 || l.Linetype != "" || d.Layer("doors") != nil {
		t.Fatalf("undo layers %+v %v", *l, d.Layer("doors"))
	}
	if cur := d.Current(); cur.Layer != "0" || cur.Color != doc.ByLayer || cur.LTScale != 0 || cur.LWDefault != 0 || cur.Linetype != "" {
		t.Fatalf("undo current %+v", cur)
	}
	if d.Layer("0").Color != 6 {
		t.Fatal("undo reverted a direct edit")
	}
	d.Redo()
	if d.Layer("walls").Color != 5 || d.CurrentLayer != "doors" || d.LTScaleValue() != 10 || d.LWDefaultValue() != 30 {
		t.Fatalf("redo %+v %+v", *d.Layer("walls"), d.Current())
	}
	// Abort restores, and a transaction that changed nothing records nothing.
	tx, _ = d.Begin()
	_ = tx.ModifyLayer("walls", func(l *doc.Layer) { l.Color = 9 })
	tx.Abort()
	if d.Layer("walls").Color != 5 {
		t.Fatal("abort kept the edit")
	}
	depth := d.UndoDepth()
	tx, _ = d.Begin()
	_ = tx.ModifyLayer("walls", func(*doc.Layer) {})
	_ = tx.SetCurrent(d.Current())
	tx.Commit()
	if d.UndoDepth() != depth {
		t.Fatal("a no-op recorded a step")
	}
}

func TestDimStyleEditPropagatesAndUndoes(t *testing.T) {
	d := doc.New()
	tx, _ := d.Begin()
	plain := &doc.Dimension{Kind: doc.DimRotated, Ext1: geom.V(0, 0), Ext2: geom.V(100, 0), DimLine: geom.V(0, 20)}
	plain.SetStyle(d, doc.DefaultDimStyle())
	own := &doc.Dimension{Kind: doc.DimRotated, Ext1: geom.V(0, 50), Ext2: geom.V(100, 50), DimLine: geom.V(0, 70)}
	st := doc.DefaultDimStyle()
	st.TextColor = 1
	own.SetStyle(d, st)
	hp, ho := tx.Add(plain), tx.Add(own)
	tx.Commit()
	if plain.Overrides.Set != 0 || own.Overrides.Set != doc.DimVarTextColor {
		t.Fatalf("overrides %b %b", plain.Overrides.Set, own.Overrides.Set)
	}
	before := d.Entity(hp).(*doc.Dimension).Layout().Text.Height

	var cs doc.ChangeSet
	d.OnChange(func(c doc.ChangeSet) { cs = c })
	iso := d.ActiveDimStyle()
	iso.TextHeight, iso.TextColor, iso.ArrowBlock = 5, 3, "_ArchTick"
	tx, _ = d.Begin()
	if err := tx.SetDimStyle(iso); err != nil {
		t.Fatal(err)
	}
	tx.Commit()
	p, o := d.Entity(hp).(*doc.Dimension), d.Entity(ho).(*doc.Dimension)
	if p.Style().TextHeight != 5 || p.Layout().Text.Height != 5 || p.Layout().Arrows[0].Kind != doc.ArrowArchTick || before != 2.5 {
		t.Fatalf("record edit not inherited: %+v", p.Style())
	}
	if o.Style().TextColor != 1 || o.Style().TextHeight != 5 {
		t.Fatalf("override lost: %+v", o.Style())
	}
	if !cs.Support || !slices.Contains(cs.Modified, hp) || !slices.Contains(cs.Modified, ho) {
		t.Fatalf("change set %+v", cs)
	}
	d.Undo()
	if p := d.Entity(hp).(*doc.Dimension); p.Style().TextHeight != 2.5 || p.Layout().Text.Height != 2.5 {
		t.Fatalf("undo: %+v", p.Style())
	}
	// Clearing the override makes the dimension follow the record again.
	tx, _ = d.Begin()
	tx.Modify(ho, func(e doc.Entity) {
		dm := e.(*doc.Dimension)
		dm.Overrides = dm.Overrides.Without(doc.DimVarTextColor)
	})
	tx.Commit()
	if c := d.Entity(ho).(*doc.Dimension).Style().TextColor; c != doc.ByBlock {
		t.Fatalf("text colour %d", c)
	}
	// A style in use cannot be removed; the current one neither.
	tx, _ = d.Begin()
	if err := tx.RemoveDimStyle("ISO-25"); err == nil {
		t.Fatal("removed the current style")
	}
	tx.Commit()
}

func TestSetCurrentInsUnitsUndo(t *testing.T) {
	d := doc.New()
	if d.Current().InsUnits != 4 {
		t.Fatalf("default INSUNITS %d", d.Current().InsUnits)
	}
	tx, _ := d.Begin()
	p := d.Current()
	p.InsUnits = 6
	if err := tx.SetCurrent(p); err != nil {
		t.Fatal(err)
	}
	p.InsUnits = 25
	if err := tx.SetCurrent(p); err == nil {
		t.Fatal("INSUNITS 25 accepted")
	}
	tx.Commit()
	if d.Header.InsUnits != 6 {
		t.Fatalf("INSUNITS %d", d.Header.InsUnits)
	}
	d.Undo()
	if d.Header.InsUnits != 4 {
		t.Fatalf("undo INSUNITS %d", d.Header.InsUnits)
	}
	d.Redo()
	if d.Header.InsUnits != 6 {
		t.Fatalf("redo INSUNITS %d", d.Header.InsUnits)
	}
}

// dimDrawing returns a drawing holding n aligned dimensions and one line, and their handles.
func dimDrawing(tb testing.TB, n int) (*doc.Drawing, []doc.Handle) {
	d := doc.New()
	tx, _ := d.Begin()
	var hs []doc.Handle
	for i := 0; i < n; i++ {
		y := float64(i) * 20
		dm := &doc.Dimension{Kind: doc.DimAligned, Ext1: geom.V(0, y), Ext2: geom.V(100, y), DimLine: geom.V(0, y+10)}
		dm.SetStyle(d, d.ActiveDimStyle())
		hs = append(hs, tx.Add(dm))
	}
	tx.Add(&doc.Line{End: geom.V(1, 0)})
	tx.Commit()
	return d, hs
}

// TestSupportDependentsOnlyOnLookChange checks that a change of a current property that does not
// change how existing entities draw (CECOLOR, CLAYER, ...) reports Support without marking every
// dimension modified, while a layer, LTSCALE or dimension style change does, through commit, undo,
// redo, abort and TakeEdits, each handle once.
func TestSupportDependentsOnlyOnLookChange(t *testing.T) {
	d, hs := dimDrawing(t, 3)
	var got []doc.ChangeSet
	d.OnChange(func(cs doc.ChangeSet) { got = append(got, cs) })
	check := func(what string, look bool) {
		t.Helper()
		for i, cs := range got {
			if !cs.Support {
				t.Errorf("%s: change set %d does not report Support", what, i)
			}
			if look {
				if len(cs.Modified) != len(hs) {
					t.Errorf("%s: change set %d modified %v, want each of %v once", what, i, cs.Modified, hs)
				}
			} else if len(cs.Modified) != 0 {
				t.Errorf("%s: change set %d modified %v, want none", what, i, cs.Modified)
			}
		}
		got = nil
	}
	edit := func(what string, look bool, fn func(tx *doc.Txn)) {
		t.Helper()
		tx, _ := d.Begin()
		fn(tx)
		fn(tx)
		if n := len(tx.TakeEdits()); look && n != len(hs) || !look && n != 0 {
			t.Errorf("%s: TakeEdits returned %d handles", what, n)
		}
		tx.Commit()
		d.Undo()
		d.Redo()
		check(what, look)
		tx, _ = d.Begin()
		fn(tx)
		tx.Abort()
		check(what+" abort", look)
	}
	color := 1
	edit("CECOLOR", false, func(tx *doc.Txn) {
		p := d.Current()
		p.Color = color
		color++
		if err := tx.SetCurrent(p); err != nil {
			t.Fatal(err)
		}
	})
	scale := 2.0
	edit("LTSCALE", true, func(tx *doc.Txn) {
		p := d.Current()
		p.LTScale = scale
		scale++
		if err := tx.SetCurrent(p); err != nil {
			t.Fatal(err)
		}
	})
	edit("layer colour", true, func(tx *doc.Txn) {
		if err := tx.ModifyLayer("0", func(l *doc.Layer) { l.Color = color }); err != nil {
			t.Fatal(err)
		}
		color++
	})
}

// BenchmarkCurrentColorChange commits and undoes a CECOLOR change in a drawing of 20,000
// dimensions: it does not depend on the dimensions.
func BenchmarkCurrentColorChange(b *testing.B) {
	d, _ := dimDrawing(b, 20000)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		tx, _ := d.Begin()
		p := d.Current()
		p.Color = 1 + i%200
		_ = tx.SetCurrent(p)
		tx.Commit()
		d.Undo()
	}
}

// BenchmarkLayerColorChange commits and undoes a layer colour change in a drawing of 20,000
// dimensions: linear in the dependents, each reported modified once.
func BenchmarkLayerColorChange(b *testing.B) {
	d, _ := dimDrawing(b, 20000)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		tx, _ := d.Begin()
		_ = tx.ModifyLayer("0", func(l *doc.Layer) { l.Color = 1 + i%200 })
		tx.Commit()
		d.Undo()
	}
}
