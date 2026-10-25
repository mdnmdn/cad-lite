package dxf_test

import (
	"bytes"
	"slices"
	"strings"
	"testing"

	"cad-lite/doc"
	"cad-lite/fileio/dxf"
	"cad-lite/geom"
)

// propsDrawing has entities with every kind of linetype, scale, lineweight and colour, and
// non-default current properties.
func propsDrawing(t *testing.T) *doc.Drawing {
	t.Helper()
	d := doc.New()
	tx, _ := d.Begin()
	_ = tx.LoadLibraryLinetype("DASHED")
	_ = tx.LoadLibraryLinetype("ACAD_ISO04W100")
	_ = tx.SetLinetype(doc.Linetype{Name: "MINE", Description: "my own", Pattern: []float64{5, -1, 0, -1}})
	l, _ := tx.AddLayer("walls", 3)
	_ = tx.ModifyLayer(l.Name, func(l *doc.Layer) { l.Linetype = "CENTER" }) // from the library, not loaded
	tx.AddCopy(&doc.Line{EntityCommon: doc.EntityCommon{Layer: "walls", Color: doc.ByBlock, Linetype: "DASHED", LinetypeScale: 2, Lineweight: 35}, End: geom.V(10, 0)})
	tx.AddCopy(&doc.Line{EntityCommon: doc.EntityCommon{Layer: "0", Color: doc.ByLayer, Linetype: doc.LinetypeByBlock, Lineweight: doc.LineweightZero}, End: geom.V(0, 10)})
	tx.AddCopy(&doc.Circle{EntityCommon: doc.EntityCommon{Layer: "0", Color: 5, Linetype: "MINE", Lineweight: doc.LineweightDefault}, Radius: 3})
	tx.AddCopy(&doc.Line{EntityCommon: doc.EntityCommon{Layer: "0", Color: doc.ByLayer, Lineweight: doc.LineweightByBlock}, End: geom.V(-10, 0)})
	arch := doc.DefaultDimStyle()
	arch.Name = "Arch"
	if err := tx.SetDimStyle(arch); err != nil {
		t.Fatal(err)
	}
	if err := tx.SetCurrent(doc.CurrentProps{Layer: "walls", Color: doc.ByBlock, Linetype: "ACAD_ISO04W100", LinetypeScale: 0.5,
		Lineweight: 50, LTScale: 10, DimStyle: "Arch"}); err != nil {
		t.Fatal(err)
	}
	tx.Commit()
	return d
}

func TestEntityPropertiesRoundTrip(t *testing.T) {
	d := propsDrawing(t)
	if got := (dxf.Codec{}).AutoVersion(d); got != dxf.Version2000 {
		t.Fatalf("auto version %s: lineweights need AC1015", got)
	}
	out, warns := writeVersion(t, d, dxf.Version2000)
	if len(warns) > 0 {
		t.Fatal(warns)
	}
	back, _ := readDXF(t, out)
	for _, h := range d.Handles() {
		a, b := d.Entity(h).Common(), back.Entity(h).Common()
		if a.Color != b.Color || !strings.EqualFold(a.Linetype, b.Linetype) || a.LtScale() != b.LtScale() || a.Lineweight != b.Lineweight {
			t.Errorf("%s: %+v became %+v", h, *a, *b)
		}
	}
	if got, want := back.Current(), d.Current(); got != want {
		t.Fatalf("current %+v, want %+v", got, want)
	}
	for _, name := range []string{"DASHED", "ACAD_ISO04W100", "MINE", "CENTER"} {
		a, ok := back.Linetype(name)
		b, _ := doc.LibraryLinetype(name)
		if name == "MINE" {
			b, _ = d.Linetype(name)
		}
		if !ok || !slices.Equal(a.Pattern, b.Pattern) || a.Description != b.Description {
			t.Errorf("%s: %+v", name, a)
		}
	}
	if !strings.Contains(out, "\n  6\nBYBLOCK\n") || !strings.Contains(out, "\n370\n35\n") || !strings.Contains(out, "\n 48\n2\n") ||
		!strings.Contains(out, "\n  2\nByLayer\n") || !strings.Contains(out, "\n 49\n-1\n 74\n0\n") {
		t.Fatal("AC1015 groups missing")
	}

	// R12 keeps the linetypes, ByBlock and the header variables it has; it has no lineweight or
	// linetype scale.
	out, _ = writeVersion(t, d, dxf.VersionR12)
	back, _ = readDXF(t, out)
	for _, h := range d.Handles() {
		a, b := d.Entity(h).Common(), back.Entity(h).Common()
		if a.Color != b.Color || !strings.EqualFold(a.Linetype, b.Linetype) || b.Lineweight != doc.LineweightByLayer || b.LinetypeScale != 0 {
			t.Errorf("R12 %s: %+v became %+v", h, *a, *b)
		}
	}
	cur := back.Current()
	if cur.Color != doc.ByBlock || cur.Linetype != "ACAD_ISO04W100" || cur.LTScale != 10 || cur.DimStyle != "Arch" || cur.Lineweight != 0 {
		t.Fatalf("R12 current %+v", cur)
	}
	if strings.Contains(out, "\n370\n") || strings.Contains(out, "\n 48\n") {
		t.Fatal("R12 output has AC1015 groups")
	}
}

func TestComplexLinetypeKeptInAC1015(t *testing.T) {
	s := group("0", "SECTION", "2", "TABLES", "0", "TABLE", "2", "LTYPE", "70", "1") +
		group("0", "LTYPE", "5", "40", "2", "GAS_LINE", "70", "0", "3", "Gas line ----GAS----", "72", "65", "73", "3", "40", "20",
			"49", "12", "74", "0", "49", "-5", "74", "2", "75", "0", "340", "11", "46", "2.5", "50", "0", "44", "-2.5", "45", "-1", "9", "GAS",
			"49", "-3", "74", "0") +
		group("0", "ENDTAB", "0", "ENDSEC", "0", "EOF")
	d, _ := readDXF(t, s)
	l, ok := d.Linetype("GAS_LINE")
	if !ok || !slices.Equal(l.Pattern, []float64{12, -5, -3}) || l.Raw == nil {
		t.Fatalf("complex linetype %+v", l)
	}
	out, _ := writeVersion(t, d, dxf.Version2000)
	if !strings.Contains(out, "\n  9\nGAS\n") {
		t.Fatal("complex element lost in AC1015")
	}
	out, _ = writeVersion(t, d, dxf.VersionR12)
	if strings.Contains(out, "GAS\n") && strings.Contains(out, "\n  9\n") {
		t.Fatal("R12 has a complex element")
	}
}

func TestDimStylePropertiesRoundTrip(t *testing.T) {
	d := doc.New()
	tx, _ := d.Begin()
	st := doc.DefaultDimStyle()
	st.Name = "Arch"
	st.ArrowBlock, st.ArrowBlock1, st.ArrowBlock2, st.SeparateArrows = "_ArchTick", "_Dot", "_Open", false
	st.LineColor, st.ExtColor, st.TextColor = 1, doc.ByLayer, 3
	st.TextStyle, st.TextJust, st.Fit, st.TextVertical = "Titles", doc.DimJustExt2, doc.DimFitTextFirst, doc.DimTextBelow
	st.TextInsideHorizontal, st.TextOutsideHorizontal, st.TextInside, st.LineInside = true, true, true, false
	st.Decimals, st.DecSep = 3, "."
	if err := tx.SetDimStyle(st); err != nil {
		t.Fatal(err)
	}
	plain := &doc.Dimension{Kind: doc.DimRotated, Ext2: geom.V(100, 0), DimLine: geom.V(0, 20)}
	plain.SetStyle(d, st)
	own := &doc.Dimension{Kind: doc.DimAligned, Ext2: geom.V(30, 40), DimLine: geom.V(-8, 6)}
	ov := st
	ov.TextColor, ov.ArrowBlock, ov.TextStyle, ov.Fit = 6, "_Oblique", "Standard", doc.DimFitBest
	own.SetStyle(d, ov)
	arc := &doc.Dimension{Kind: doc.DimArcLength, Center: geom.V(200, 0), Radius: 40, StartAngle: 0.2, EndAngle: 2.5, DimLine: geom.V(200, 55)}
	arc.SetStyle(d, ov)
	hp, ho, ha := tx.Add(plain), tx.Add(own), tx.Add(arc)
	tx.Commit()
	if plain.Overrides.Set != 0 || own.Overrides.Set == 0 {
		t.Fatalf("overrides %b %b", plain.Overrides.Set, own.Overrides.Set)
	}
	for _, v := range []string{dxf.Version2000, dxf.VersionR12} {
		out, warns := writeVersion(t, d, v)
		if len(warns) > 0 {
			t.Fatalf("%s: %v", v, warns)
		}
		back, _ := readDXF(t, out)
		got, ok := back.DimStyle("Arch")
		if !ok || got != st {
			t.Fatalf("%s: record\n%+v\nwant\n%+v", v, got, st)
		}
		for _, h := range []doc.Handle{hp, ho, ha} {
			a, b := d.Entity(h).(*doc.Dimension), back.Entity(h).(*doc.Dimension)
			want := a.Style()
			if v == dxf.VersionR12 && h != ha {
				want.TextStyle = st.TextStyle // R12 cannot override DIMTXSTY
			}
			if b.StyleName != "Arch" || b.Style() != want {
				t.Fatalf("%s %s: style\n%+v\nwant\n%+v", v, h, b.Style(), want)
			}
		}
		if p := back.Entity(hp).(*doc.Dimension); p.Overrides.Set != 0 {
			t.Fatalf("%s: plain dimension overrides %b", v, p.Overrides.Set)
		}
		names := blockNames(out)
		for _, n := range []string{"_ArchTick", "_Dot", "_Open", "_Oblique"} {
			if !slices.Contains(names, n) {
				t.Fatalf("%s: arrow block %s not written: %v", v, n, names)
			}
		}
		if v == dxf.Version2000 && (!strings.Contains(out, "\n342\n") || !strings.Contains(out, "\n340\n")) {
			t.Fatalf("AC1015 DIMBLK and DIMTXSTY handles missing")
		}
		// The arrow blocks the file now has are reused, not generated again.
		var buf bytes.Buffer
		if _, err := (dxf.Codec{}).WriteVersion(&buf, back, v); err != nil {
			t.Fatal(err)
		}
		if n := strings.Count(buf.String(), "\n  2\n_ArchTick\n"); n > 2 {
			t.Fatalf("%s: _ArchTick %d times", v, n)
		}
	}
	// A record edit after reading reaches the dimension that does not override it.
	out, _ := writeVersion(t, d, dxf.Version2000)
	back, _ := readDXF(t, out)
	edited, _ := back.DimStyle("Arch")
	edited.TextHeight = 7
	tx, _ = back.Begin()
	_ = tx.SetDimStyle(edited)
	tx.Commit()
	if back.Entity(hp).(*doc.Dimension).Layout().Text.Height != 7 {
		t.Fatal("record edit not inherited after a read")
	}
}

// A dimension of an old file carrying its whole style as DSTYLE overrides still reads with that
// style.
func TestOldFullOverrideDimensionReads(t *testing.T) {
	s := group("0", "SECTION", "2", "ENTITIES") +
		dgroup("0", "DIMENSION", "5", "20", "8", "0", "2", "*D1", "10", "40", "20", "10", "30", "0", "11", "20", "21", "12", "31", "0",
			"70", "33", "3", "ISO-25", "13", "0", "23", "0", "33", "0", "14", "40", "24", "0", "34", "0", "50", "0",
			"1001", "ACAD", "1000", "DSTYLE", "1002", "{", "1070", "40", "1040", "1", "1070", "41", "1040", "4", "1070", "140", "1040", "2.5",
			"1070", "77", "1070", "1", "1070", "271", "1070", "2", "1002", "}") +
		group("0", "ENDSEC", "0", "EOF")
	d, _ := readDXF(t, s)
	e := d.Entity(0x20).(*doc.Dimension)
	if e.Style().ArrowSize != 4 || e.StyleName != "ISO-25" || e.Overrides.Set&doc.DimVarArrowSize == 0 {
		t.Fatalf("style %+v overrides %b", e.Style(), e.Overrides.Set)
	}
}
