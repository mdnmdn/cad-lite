package dxf_test

import (
	"bytes"
	"math"
	"strings"
	"testing"

	"cad-lite/doc"
	"cad-lite/fileio/dxf"
	"cad-lite/geom"
)

// dimDrawing holds one dimension of every kind, with the fields a file must keep: style
// overrides and a style the file lacks, a text override, moved text, a text angle, a partial arc.
func dimDrawing(t *testing.T) *doc.Drawing {
	t.Helper()
	d := doc.New()
	d.AddLayer("dims", 2)
	custom := doc.DefaultDimStyle()
	custom.Scale, custom.ArrowSize, custom.Decimals, custom.DecSep, custom.TextVertical, custom.ZeroSupp = 2, 3.5, 1, ".", 0, 0
	custom.ExtOffset, custom.ExtExtend, custom.Gap, custom.TextHeight = 1, 2, 0.8, 3
	other := doc.DefaultDimStyle()
	other.Name = "Arch"
	dims := []*doc.Dimension{
		{Kind: doc.DimRotated, Ext1: geom.V(0, 0), Ext2: geom.V(120, 30), DimLine: geom.V(0, 50)},
		{EntityCommon: doc.EntityCommon{Layer: "dims", Color: 3}, Kind: doc.DimRotated, Ext1: geom.V(0, 0), Ext2: geom.V(120, 30.25),
			DimLine: geom.V(140, 0), Rotation: math.Pi / 2, Text: "L=<> mm"},
		{Kind: doc.DimAligned, Ext1: geom.V(10, 10), Ext2: geom.V(40, 50), DimLine: geom.V(0, 20), TextMoved: true,
			TextPos: geom.V(-30, 70), TextAngle: 0.3, Text: "fixed"},
		{Kind: doc.DimAligned, Ext1: geom.V(0, 0), Ext2: geom.V(-30, -40), DimLine: geom.V(5, 5), Text: " "},
		{Kind: doc.DimArcLength, Center: geom.V(200, 0), Radius: 40, StartAngle: 0.2, EndAngle: 2.5,
			DimLine: geom.V(200, 55)},
		{Kind: doc.DimArcLength, Center: geom.V(300, 0), Radius: 30, StartAngle: 0, EndAngle: math.Pi, Partial: true,
			Ext1: geom.V(300+30*math.Cos(0.5), 30*math.Sin(0.5)), Ext2: geom.V(300+30*math.Cos(2), 30*math.Sin(2)),
			DimLine: geom.V(300, 20), ArcSymbol: doc.DimArcSymAbove, Text: "<>*", TextMoved: true, TextPos: geom.V(290, 50),
			TextAngle: math.Pi},
	}
	styles := []doc.DimStyle{doc.DefaultDimStyle(), custom, other, doc.DefaultDimStyle(), doc.DefaultDimStyle(), custom}
	tx, _ := d.Begin()
	for i, e := range dims {
		e.SetStyle(d, styles[i])
		e.Recompute()
		tx.Add(e)
	}
	tx.Commit()
	return d
}

func near(a, b float64) bool { return math.Abs(a-b) <= 1e-9*math.Max(1, math.Abs(a)) }

// sameDim reports how two dimensions differ, "" when they are equal up to rounding.
func sameDim(a, b *doc.Dimension) string {
	switch {
	case a.Kind != b.Kind:
		return "kind"
	case a.Layer != b.Layer || a.Color != b.Color:
		return "layer or colour"
	case !a.Ext1.Near(b.Ext1, 1e-9) || !a.Ext2.Near(b.Ext2, 1e-9):
		return "extension origins"
	case !a.DimLine.Near(b.DimLine, 1e-9):
		return "dimension line point"
	case !a.TextPos.Near(b.TextPos, 1e-9):
		return "text position"
	case a.TextMoved != b.TextMoved || a.Partial != b.Partial || a.ArcSymbol != b.ArcSymbol:
		return "flags"
	case !near(math.Cos(a.Rotation), math.Cos(b.Rotation)) || !near(math.Sin(a.Rotation), math.Sin(b.Rotation)):
		return "rotation"
	case !near(a.TextAngle, b.TextAngle):
		return "text angle"
	case a.Text != b.Text:
		return "text " + a.Text + " / " + b.Text
	case a.Style() != b.Style():
		return "style"
	case a.Kind == doc.DimArcLength && (!a.Center.Near(b.Center, 1e-9) || !near(a.Radius, b.Radius) ||
		!near(a.StartAngle, b.StartAngle) || !near(a.EndAngle, b.EndAngle)):
		return "arc"
	case a.MeasurementText() != b.MeasurementText():
		return "measurement text"
	}
	return ""
}

func readDXF(t *testing.T, s string) (*doc.Drawing, []string) {
	t.Helper()
	d, warns, err := dxf.Codec{}.Read(strings.NewReader(s))
	if err != nil {
		t.Fatal(err)
	}
	return d, warns
}

// blockNames lists the BLOCK records of a DXF text by name.
func blockNames(s string) []string {
	lines := strings.Split(s, "\n")
	var out []string
	for i := 0; i+1 < len(lines); i += 2 {
		if strings.TrimSpace(lines[i]) == "0" && strings.TrimSpace(lines[i+1]) == "BLOCK" {
			for j := i + 2; j+1 < len(lines) && strings.TrimSpace(lines[j]) != "0"; j += 2 {
				if strings.TrimSpace(lines[j]) == "2" {
					out = append(out, strings.TrimSpace(lines[j+1]))
				}
			}
		}
	}
	return out
}

func countPrefix(ss []string, p string) int {
	n := 0
	for _, s := range ss {
		if strings.HasPrefix(s, p) {
			n++
		}
	}
	return n
}

func TestDimensionRoundTripBothVersions(t *testing.T) {
	for _, v := range []string{dxf.VersionR12, dxf.Version2000} {
		src := dimDrawing(t)
		if got := (dxf.Codec{}).AutoVersion(src); got != dxf.VersionR12 {
			t.Fatalf("auto version %s", got)
		}
		out, warns := writeVersion(t, src, v)
		if len(warns) > 0 {
			t.Fatalf("%s: write warnings %v", v, warns)
		}
		if strings.Contains(out, "AcDb") != (v == dxf.Version2000) {
			t.Fatalf("%s: subclass markers present = %v", v, strings.Contains(out, "AcDb"))
		}
		for _, want := range []string{"DSTYLE", "CADLITE_DIM", "DEFPOINTS", "\nSOLID\n", "\nPOINT\n", "\nISO-25\n", "\nArch\n"} {
			if !strings.Contains(out, want) {
				t.Fatalf("%s: output lacks %q", v, want)
			}
		}
		if n := countPrefix(blockNames(out), "*D"); n != 6 {
			t.Fatalf("%s: %d *D blocks written", v, n)
		}
		back, warns := readDXF(t, out)
		if len(warns) > 0 {
			t.Fatalf("%s: read warnings %v", v, warns)
		}
		for _, h := range src.Handles() {
			a := src.Entity(h).(*doc.Dimension)
			b, ok := back.Entity(h).(*doc.Dimension)
			if !ok {
				t.Fatalf("%s: %s read back as %T", v, h, back.Entity(h))
			}
			if diff := sameDim(a, b); diff != "" {
				t.Fatalf("%s: dimension %s differs in %s:\n%+v\n%+v", v, h, diff, *a, *b)
			}
		}
		if n := countPrefix(blockNamesOf(back), "*D"); n != 0 {
			t.Fatalf("%s: %d *D blocks kept after reading", v, n)
		}
		if l := back.Layer("DEFPOINTS"); l == nil || !l.NoPlot {
			t.Fatalf("%s: DEFPOINTS layer %+v", v, l)
		}
		// A second save regenerates the blocks instead of adding to them.
		again, _ := writeVersion(t, back, v)
		if n := countPrefix(blockNames(again), "*D"); n != 6 {
			t.Fatalf("%s: %d *D blocks after a second save", v, n)
		}
		if strings.Count(again, "\nDIMSTYLE\n") != strings.Count(out, "\nDIMSTYLE\n") {
			t.Fatalf("%s: DIMSTYLE records duplicated on the second save", v)
		}
		third, _ := readDXF(t, again)
		for _, h := range src.Handles() {
			if diff := sameDim(src.Entity(h).(*doc.Dimension), third.Entity(h).(*doc.Dimension)); diff != "" {
				t.Fatalf("%s: second round trip differs in %s", v, diff)
			}
		}
	}
}

func blockNamesOf(d *doc.Drawing) []string {
	var out []string
	for _, b := range d.Blocks() {
		out = append(out, b.Name)
	}
	for _, b := range d.Preserved.Blocks {
		out = append(out, b.Name())
	}
	return out
}

// A dimension inside a block definition gets its own *D block too.
func TestDimensionInsideBlockRoundTrip(t *testing.T) {
	d := doc.New()
	dim := &doc.Dimension{EntityCommon: doc.EntityCommon{Layer: "0", Color: doc.ByLayer}, Kind: doc.DimAligned, Ext1: geom.V(0, 0), Ext2: geom.V(10, 0), DimLine: geom.V(0, 5)}
	dim.Recompute()
	d.LoadBlock(&doc.Block{Name: "SYM", Entities: []doc.Entity{dim, &doc.Line{End: geom.V(10, 0)}}})
	tx, _ := d.Begin()
	tx.Add(doc.NewInsert("SYM", geom.V(5, 5)))
	tx.Commit()
	for _, v := range []string{dxf.VersionR12, dxf.Version2000} {
		out, warns := writeVersion(t, d, v)
		if len(warns) > 0 {
			t.Fatalf("%s: %v", v, warns)
		}
		back, warns := readDXF(t, out)
		if len(warns) > 0 {
			t.Fatalf("%s: %v", v, warns)
		}
		b := back.Block("SYM")
		if b == nil || len(b.Entities) != 2 {
			t.Fatalf("%s: block %+v", v, b)
		}
		got, ok := b.Entities[0].(*doc.Dimension)
		if !ok || sameDim(dim, got) != "" || countPrefix(blockNamesOf(back), "*D") != 0 {
			t.Fatalf("%s: dimension in block %+v, blocks %v", v, b.Entities[0], blockNamesOf(back))
		}
	}
}

// autocadDims is an AutoCAD 2000 style file: a DIMSTYLE table whose ISO-25 record leaves out
// what equals AutoCAD's built-in defaults, a rotated dimension with DSTYLE overrides and a text
// rotation, an angular dimension (kept unknown with its block), and an ARC_DIMENSION.
var autocadDims = dgroup("0", "SECTION", "2", "HEADER", "9", "$ACADVER", "1", "AC1018", "9", "$HANDSEED", "5", "200", "0", "ENDSEC") +
	dgroup("0", "SECTION", "2", "TABLES") +
	dgroup("0", "TABLE", "2", "LAYER", "5", "2", "330", "0", "100", "AcDbSymbolTable", "70", "1") +
	dgroup("0", "LAYER", "5", "10", "330", "2", "100", "AcDbSymbolTableRecord", "100", "AcDbLayerTableRecord", "2", "0", "70", "0", "62", "7", "6", "Continuous") +
	dgroup("0", "ENDTAB") +
	dgroup("0", "TABLE", "2", "APPID", "5", "9", "330", "0", "100", "AcDbSymbolTable", "70", "1") +
	dgroup("0", "APPID", "5", "12", "330", "9", "100", "AcDbSymbolTableRecord", "100", "AcDbRegAppTableRecord", "2", "ACAD", "70", "0") +
	dgroup("0", "ENDTAB") +
	dgroup("0", "TABLE", "2", "DIMSTYLE", "5", "A", "330", "0", "100", "AcDbSymbolTable", "70", "2", "100", "AcDbDimStyleTable", "71", "0") +
	dgroup("0", "DIMSTYLE", "105", "27", "330", "A", "100", "AcDbSymbolTableRecord", "100", "AcDbDimStyleTableRecord",
		"2", "ISO-25", "70", "0", "41", "2.5", "42", "0.625", "43", "3.75", "44", "1.25", "73", "0", "74", "0", "77", "1", "78", "8",
		"140", "2.5", "141", "2.5", "143", "0.03937007874016", "147", "0.625", "171", "3", "172", "1", "271", "2", "272", "2",
		"274", "3", "278", "44", "283", "0", "284", "8", "340", "11") +
	dgroup("0", "DIMSTYLE", "105", "28", "330", "A", "100", "AcDbSymbolTableRecord", "100", "AcDbDimStyleTableRecord",
		"2", "Standard", "70", "0", "340", "11") +
	dgroup("0", "ENDTAB") +
	dgroup("0", "ENDSEC") +
	dgroup("0", "SECTION", "2", "BLOCKS") +
	dgroup("0", "BLOCK", "5", "30", "330", "31", "100", "AcDbEntity", "8", "0", "100", "AcDbBlockBegin", "2", "*D1", "70", "1", "10", "0", "20", "0", "30", "0", "3", "*D1", "1", "") +
	dgroup("0", "LINE", "5", "32", "330", "31", "100", "AcDbEntity", "8", "0", "100", "AcDbLine", "10", "0", "20", "0", "30", "0", "11", "100", "21", "0", "31", "0") +
	dgroup("0", "ENDBLK", "5", "33", "330", "31", "100", "AcDbEntity", "8", "0", "100", "AcDbBlockEnd") +
	dgroup("0", "BLOCK", "5", "34", "330", "35", "100", "AcDbEntity", "8", "0", "100", "AcDbBlockBegin", "2", "*D2", "70", "1", "10", "0", "20", "0", "30", "0", "3", "*D2", "1", "") +
	dgroup("0", "LINE", "5", "36", "330", "35", "100", "AcDbEntity", "8", "0", "100", "AcDbLine", "10", "0", "20", "0", "30", "0", "11", "5", "21", "5", "31", "0") +
	dgroup("0", "ENDBLK", "5", "37", "330", "35", "100", "AcDbEntity", "8", "0", "100", "AcDbBlockEnd") +
	dgroup("0", "BLOCK", "5", "38", "330", "39", "100", "AcDbEntity", "8", "0", "100", "AcDbBlockBegin", "2", "*D3", "70", "1", "10", "0", "20", "0", "30", "0", "3", "*D3", "1", "") +
	dgroup("0", "ENDBLK", "5", "3A", "330", "39", "100", "AcDbEntity", "8", "0", "100", "AcDbBlockEnd") +
	dgroup("0", "ENDSEC") +
	dgroup("0", "SECTION", "2", "ENTITIES") +
	dgroup("0", "DIMENSION", "5", "50", "330", "1F", "100", "AcDbEntity", "8", "0", "100", "AcDbDimension", "2", "*D1",
		"10", "100", "20", "20", "30", "0", "11", "50", "21", "23.75", "31", "0", "70", "32", "71", "5", "42", "100",
		"53", "30", "3", "ISO-25", "100", "AcDbAlignedDimension", "13", "0", "23", "0", "33", "0", "14", "100", "24", "0", "34", "0",
		"50", "0", "100", "AcDbRotatedDimension",
		"1001", "ACAD", "1000", "DSTYLE", "1002", "{", "1070", "41", "1040", "5", "1070", "271", "1070", "3", "1070", "278", "1070", "46", "1002", "}") +
	dgroup("0", "DIMENSION", "5", "51", "330", "1F", "100", "AcDbEntity", "8", "0", "100", "AcDbDimension", "2", "*D2",
		"10", "0", "20", "0", "30", "0", "11", "5", "21", "5", "31", "0", "70", "34", "3", "ISO-25", "100", "AcDb3PointAngularDimension",
		"13", "1", "23", "0", "33", "0", "14", "0", "24", "1", "34", "0", "15", "0", "25", "0", "35", "0") +
	dgroup("0", "ARC_DIMENSION", "5", "52", "330", "1F", "100", "AcDbEntity", "8", "0", "100", "AcDbDimension", "2", "*D3",
		"10", "0", "20", "60", "30", "0", "11", "0", "21", "63", "31", "0", "70", "37", "3", "Standard",
		"100", "AcDbArcDimension", "13", "50", "23", "0", "33", "0", "14", "-50", "24", "0", "34", "0", "15", "0", "25", "0", "35", "0",
		"40", "0", "41", "3.141592653589793", "70", "0", "71", "0", "16", "0", "26", "0", "36", "0", "17", "0", "27", "0", "37", "0") +
	dgroup("0", "ENDSEC", "0", "EOF")

func TestReadAutoCADDimensions(t *testing.T) {
	d, warns := readDXF(t, autocadDims)
	if len(warns) != 1 || !strings.Contains(warns[0], "DIMENSION") {
		t.Fatalf("warnings %v", warns)
	}
	lin, ok := d.Entity(0x50).(*doc.Dimension)
	if !ok {
		t.Fatalf("rotated dimension read as %T", d.Entity(0x50))
	}
	want := doc.DefaultDimStyle()
	want.ArrowSize, want.Decimals, want.DecSep = 5, 3, "."
	if lin.Kind != doc.DimRotated || lin.Style() != want || lin.Ext2 != geom.V(100, 0) || lin.DimLine != geom.V(100, 20) {
		t.Fatalf("rotated dimension %+v", *lin)
	}
	if !near(lin.TextAngle, math.Pi/6) || lin.MeasurementText() != "100" {
		t.Fatalf("text angle %v, text %q", lin.TextAngle, lin.MeasurementText())
	}
	if _, ok := d.Entity(0x51).(*doc.Unknown); !ok {
		t.Fatalf("angular dimension read as %T", d.Entity(0x51))
	}
	arc, ok := d.Entity(0x52).(*doc.Dimension)
	if !ok || arc.Kind != doc.DimArcLength {
		t.Fatalf("arc dimension read as %T", d.Entity(0x52))
	}
	// Standard leaves every variable out: AutoCAD's built-in defaults.
	std := doc.DimStyle{Name: "Standard", Scale: 1, ArrowSize: 0.18, TextHeight: 0.18, ExtOffset: 0.0625, ExtExtend: 0.18, Gap: 0.09, Decimals: 4, DecSep: ".",
		TextInsideHorizontal: true, TextOutsideHorizontal: true, TextStyle: "Standard", Fit: doc.DimFitBest}
	if arc.Center != geom.V(0, 0) || arc.Radius != 50 || !near(arc.StartAngle, 0) || !near(arc.EndAngle, math.Pi) || arc.Style() != std {
		t.Fatalf("arc dimension %+v", *arc)
	}
	if !near(arc.Measurement(), 50*math.Pi) || arc.DimLine.Dist(arc.Center) != 60 {
		t.Fatalf("arc measurement %v, radius %v", arc.Measurement(), arc.DimLine.Dist(arc.Center))
	}
	if names := blockNamesOf(d); len(names) != 1 || names[0] != "*D2" {
		t.Fatalf("blocks after reading %v", names)
	}
	for _, v := range []string{dxf.VersionR12, dxf.Version2000} {
		out, _ := writeVersion(t, d, v)
		names := blockNames(out)
		if countPrefix(names, "*D2") != 1 || countPrefix(names, "*D") != 3 {
			t.Fatalf("%s: blocks written %v", v, names)
		}
		back, _ := readDXF(t, out)
		if _, ok := back.Entity(0x51).(*doc.Unknown); !ok || back.Block("*D2") == nil {
			t.Fatalf("%s: angular dimension or its block lost", v)
		}
		for _, h := range []doc.Handle{0x50, 0x52} {
			if diff := sameDim(d.Entity(h).(*doc.Dimension), back.Entity(h).(*doc.Dimension)); diff != "" {
				t.Fatalf("%s: %s differs in %s", v, h, diff)
			}
		}
		if strings.Count(out, "\n  2\nACAD\n") != 1 || strings.Count(out, "\nISO-25\n") < 1 {
			t.Fatalf("%s: APPID ACAD written %d times", v, strings.Count(out, "\n  2\nACAD\n"))
		}
	}
}

// An R12 DIMENSION, without subclass markers, with an R12 DIMSTYLE record (block names in
// groups 5 to 7, no DIMDEC or DIMDSEP) and text rotated to 0 explicitly.
func TestReadR12Dimension(t *testing.T) {
	s := dgroup("0", "SECTION", "2", "TABLES", "0", "TABLE", "2", "DIMSTYLE", "70", "1") +
		dgroup("0", "DIMSTYLE", "2", "STANDARD", "70", "0", "3", "", "4", "", "5", "", "6", "", "7", "",
			"40", "10", "41", "0.2", "42", "0.1", "44", "0.3", "140", "0.25", "147", "0.05", "77", "1", "78", "0") +
		dgroup("0", "ENDTAB", "0", "ENDSEC", "0", "SECTION", "2", "ENTITIES") +
		dgroup("0", "DIMENSION", "5", "20", "8", "0", "2", "*D0", "10", "40", "20", "0", "30", "0", "11", "20", "21", "5", "31", "0",
			"70", "1", "1", "<>", "53", "360", "3", "STANDARD", "13", "0", "23", "0", "33", "0", "14", "40", "24", "0", "34", "0") +
		dgroup("0", "ENDSEC", "0", "EOF")
	d, warns := readDXF(t, s)
	if len(warns) > 0 {
		t.Fatal(warns)
	}
	e, ok := d.Entity(0x20).(*doc.Dimension)
	if !ok {
		t.Fatalf("read as %T", d.Entity(0x20))
	}
	want := doc.DimStyle{Name: "STANDARD", Scale: 10, ArrowSize: 0.2, TextHeight: 0.25, ExtOffset: 0.1, ExtExtend: 0.3, Gap: 0.05, Decimals: 4, DecSep: ".", TextVertical: 1,
		TextInsideHorizontal: true, TextOutsideHorizontal: true, TextStyle: "Standard", Fit: doc.DimFitBest}
	if e.Kind != doc.DimAligned || e.Style() != want || e.MeasurementText() != "40.0000" {
		t.Fatalf("dimension %+v, text %q", *e, e.MeasurementText())
	}
	// An explicit 0° text angle is horizontal text, kept as 180° (readable: horizontal).
	if e.TextAngle != math.Pi || e.Layout().Text.Rotation != 0 {
		t.Fatalf("text angle %v, drawn %v", e.TextAngle, e.Layout().Text.Rotation)
	}
	// Saved as AutoCAD 2000, the R12 record loses the DIMBLK names, which are handles there.
	out, _ := writeVersion(t, d, dxf.Version2000)
	back, _ := readDXF(t, out)
	if diff := sameDim(e, back.Entity(0x20).(*doc.Dimension)); diff != "" {
		t.Fatalf("differs in %s", diff)
	}
	if strings.Contains(out, "AcDbDimStyleTableRecord\n  2\nSTANDARD\n 70\n0\n  3\n\n  4\n\n  5") {
		t.Fatal("DIMBLK name written as a handle group")
	}
}

// A dimension type the document does not model keeps its raw groups and its block, and is not
// renamed or duplicated by a save.
func TestUnknownDimensionTypesPreserved(t *testing.T) {
	d, _ := readDXF(t, autocadDims)
	before := d.Entity(0x51).(*doc.Unknown).Raw
	out, _ := writeVersion(t, d, dxf.Version2000)
	back, _ := readDXF(t, out)
	after := back.Entity(0x51).(*doc.Unknown).Raw
	if len(after) != len(before) {
		t.Fatalf("raw groups %d, want %d", len(after), len(before))
	}
	var buf bytes.Buffer
	if _, err := (dxf.Codec{}).Write(&buf, back); err != nil || countPrefix(blockNames(buf.String()), "*D2") != 1 {
		t.Fatalf("second save: %v, blocks %v", err, blockNames(buf.String()))
	}
}

// dgroup is group with extended data codes (four digits) allowed.
func dgroup(kv ...string) string {
	var b strings.Builder
	for i := 0; i < len(kv); i += 2 {
		b.WriteString(strings.Repeat(" ", max(0, 3-len(kv[i]))) + kv[i] + "\n" + kv[i+1] + "\n")
	}
	return b.String()
}
