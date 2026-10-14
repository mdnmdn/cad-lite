package dxf_test

import (
	"bytes"
	"math"
	"reflect"
	"strings"
	"testing"

	"go-draw/doc"
	"go-draw/fileio/dxf"
	"go-draw/geom"
)

func sample() *doc.Drawing {
	d := doc.New()
	d.AddLayer("walls", 1)
	off := d.AddLayer("hidden", 5)
	off.Off = true
	d.CurrentLayer = "walls"
	tx, _ := d.Begin()
	tx.Add(&doc.Line{Start: geom.V(0, 0), End: geom.V(100, 50)})
	tx.Add(&doc.Circle{EntityCommon: doc.EntityCommon{Color: 3}, Center: geom.V(10, 10), Radius: 5})
	tx.Add(&doc.Arc{EntityCommon: doc.EntityCommon{Layer: "hidden"}, Center: geom.V(1, 2), Radius: 3, StartAngle: 0.5, EndAngle: 2})
	tx.Add(&doc.LWPolyline{Points: []geom.Vec2{{X: 0, Y: 0}, {X: 10, Y: 0}, {X: 10, Y: 10}}, Bulges: []float64{0, 0.5, 0}, Closed: true})
	tx.Commit()
	return d
}

func TestRoundTripKeepsEntitiesHandlesAndLayers(t *testing.T) {
	src := sample()
	var buf bytes.Buffer
	if warns, err := (dxf.Codec{}).Write(&buf, src); err != nil || len(warns) != 0 {
		t.Fatalf("write: %v %v", warns, err)
	}
	got, warns, err := (dxf.Codec{}).Read(&buf)
	if err != nil || len(warns) != 0 {
		t.Fatalf("read: %v %v", warns, err)
	}
	if !reflect.DeepEqual(got.Handles(), src.Handles()) {
		t.Fatalf("handles %v, want %v", got.Handles(), src.Handles())
	}
	for _, h := range src.Handles() {
		a, b := src.Entity(h), got.Entity(h)
		if a.Class() != b.Class() || *a.Common() != *b.Common() {
			t.Fatalf("%s: %s %+v vs %s %+v", h, a.Class(), *a.Common(), b.Class(), *b.Common())
		}
		ba, bb := a.Bounds(), b.Bounds()
		if !ba.Min.Near(bb.Min, 1e-9) || !ba.Max.Near(bb.Max, 1e-9) {
			t.Fatalf("%s %s bounds %+v vs %+v", h, a.Class(), ba, bb)
		}
	}
	arc := got.Entity(src.Handles()[2]).(*doc.Arc)
	if math.Abs(arc.StartAngle-0.5) > 1e-12 || math.Abs(arc.EndAngle-2) > 1e-12 {
		t.Fatalf("arc angles %v %v", arc.StartAngle, arc.EndAngle)
	}
	if got.CurrentLayer != "walls" || got.Layer("walls").Color != 1 || !got.Layer("hidden").Off {
		t.Fatalf("layers: current %q, %+v %+v", got.CurrentLayer, got.Layer("walls"), got.Layer("hidden"))
	}
	if got.Header.HandSeed <= src.Handles()[3] {
		t.Fatalf("handseed %v not above the last handle", got.Header.HandSeed)
	}
	tx, _ := got.Begin()
	if h := tx.Add(&doc.Line{}); src.Entity(h) != nil {
		t.Fatalf("new handle %v collides with a loaded one", h)
	}
	tx.Commit()
}

// A minimal R2000-style file with an LWPOLYLINE and an unsupported entity.
const modern = `  0
SECTION
  2
HEADER
  9
$ACADVER
  1
AC1015
  9
$HANDSEED
  5
2A0
  9
$INSUNITS
 70
6
  0
ENDSEC
  0
SECTION
  2
ENTITIES
  0
LWPOLYLINE
  5
2A
  8
0
 90
2
 70
0
 10
1.5
 20
2.5
 42
1.0
 10
4.5
 20
2.5
  0
SPLINE
  5
2B
  8
0
  0
ENDSEC
  0
EOF
`

func TestReadModernLWPolylineAndKeepUnknown(t *testing.T) {
	d, warns, err := (dxf.Codec{}).Read(strings.NewReader(modern))
	if err != nil {
		t.Fatal(err)
	}
	if len(warns) != 1 || !strings.Contains(warns[0], "SPLINE") || !strings.Contains(warns[0], "unknown") {
		t.Fatalf("warnings = %v", warns)
	}
	p, ok := d.Entity(0x2A).(*doc.LWPolyline)
	if !ok || len(p.Points) != 2 || p.Bulges[0] != 1 || p.Points[1] != geom.V(4.5, 2.5) {
		t.Fatalf("polyline = %+v", d.Entity(0x2A))
	}
	if u, ok := d.Entity(0x2B).(*doc.Unknown); !ok || u.Class() != "SPLINE" {
		t.Fatalf("spline = %+v", d.Entity(0x2B))
	}
	if d.Header.HandSeed != 0x2A0 || d.Header.InsUnits != 6 {
		t.Fatalf("header = %+v", d.Header)
	}
}

func TestReadRejectsTruncatedFile(t *testing.T) {
	if _, _, err := (dxf.Codec{}).Read(strings.NewReader("  0\nSECTION\n  2\n")); err == nil {
		t.Fatal("truncated file read without error")
	}
}

// group formats groups as a DXF fragment.
func group(kv ...string) string {
	var b strings.Builder
	for i := 0; i < len(kv); i += 2 {
		b.WriteString(strings.Repeat(" ", 3-len(kv[i])) + kv[i] + "\n" + kv[i+1] + "\n")
	}
	return b.String()
}

const (
	textEnt = "  0\nTEXT\n  5\n30\n330\n1F\n100\nAcDbEntity\n  8\nnotes\n  6\nDASHED\n100\nAcDbText\n 10\n5.5\n 20\n6.5\n 30\n0.0\n 40\n2.5\n  1\n  Hello  \n 72\n0\n100\nAcDbText\n"
	insEnt  = "  0\nINSERT\n  5\n31\n  8\n0\n 66\n1\n  2\nB1\n 10\n1\n 20\n2\n 30\n0\n  0\nATTRIB\n  5\n32\n  8\n0\n  1\nv\n  2\nTAG\n  0\nSEQEND\n  5\n33\n  8\n0\n"
	splEnt  = "  0\nSPLINE\n  5\n34\n  8\n0\n100\nAcDbEntity\n100\nAcDbSpline\n 70\n8\n 71\n3\n 40\n0.0\n 40\n1.0\n 10\n-3\n 20\n-4\n 30\n0\n 10\n3\n 20\n4\n 30\n0\n"
	custom  = "  0\nACME_WIDGET\n  5\n35\n  8\nacme\n 62\n  3\n  1\nstrange \u00e9\n 90\n17\n"
	pl3d    = "  0\nPOLYLINE\n  5\n36\n  8\n0\n 66\n1\n 70\n8\n  0\nVERTEX\n  5\n37\n  8\n0\n 10\n0\n 20\n0\n 30\n1\n 70\n32\n  0\nVERTEX\n  5\n38\n  8\n0\n 10\n1\n 20\n1\n 30\n2\n 70\n32\n  0\nSEQEND\n  5\n39\n  8\n0\n"
	lineEnt = "  0\nLINE\n  5\n40\n  8\n0\n 10\n0\n 20\n0\n 11\n9\n 21\n9\n"
)

func pr(code int, v string) doc.Pair { return doc.Pair{Code: code, Value: v} }

func richFile(entities string) string {
	return group("0", "SECTION", "2", "HEADER", "9", "$ACADVER", "1", "AC1015", "9", "$HANDSEED", "5", "50", "0", "ENDSEC") +
		group("0", "SECTION", "2", "TABLES") +
		group("0", "TABLE", "2", "LTYPE", "70", "3") +
		group("0", "LTYPE", "5", "A", "2", "CONTINUOUS", "70", "0") +
		group("0", "LTYPE", "5", "B", "2", "DASHED", "70", "0", "3", "Dashed", "72", "65", "73", "2", "40", "1.0", "49", "0.5", "49", "-0.5") +
		group("0", "ENDTAB") +
		group("0", "TABLE", "2", "LAYER", "70", "1") +
		group("0", "LAYER", "2", "0", "70", "0", "62", "7", "6", "CONTINUOUS", "0", "ENDTAB") +
		group("0", "ENDSEC") +
		group("0", "SECTION", "2", "BLOCKS") +
		group("0", "BLOCK", "5", "60", "8", "0", "2", "B1", "70", "2", "10", "0", "20", "0", "30", "0", "3", "B1") +
		group("0", "LINE", "5", "61", "8", "0", "10", "0", "20", "0", "11", "1", "21", "1") +
		group("0", "ENDBLK", "5", "62", "8", "0") +
		group("0", "ENDSEC") +
		group("0", "SECTION", "2", "ENTITIES") + entities + group("0", "ENDSEC", "0", "EOF")
}

// r12Groups drops the groups the writer leaves out of an R12 file.
func r12Groups(ps []doc.Pair) []doc.Pair {
	var out []doc.Pair
	for _, p := range ps {
		if p.Code != 100 && p.Code != 102 && p.Code != 330 && p.Code != 360 {
			out = append(out, p)
		}
	}
	return out
}

// stripR13 is r12Groups for a DXF fragment.
func stripR13(frag string) string {
	lines := strings.Split(strings.TrimSuffix(frag, "\n"), "\n")
	var b strings.Builder
	for i := 0; i+1 < len(lines); i += 2 {
		switch strings.TrimSpace(lines[i]) {
		case "100", "102", "330", "360":
		default:
			b.WriteString(lines[i] + "\n" + lines[i+1] + "\n")
		}
	}
	return b.String()
}

// unknownRaw returns the group list of every unknown entity, by handle.
func unknownRaw(d *doc.Drawing) map[doc.Handle][]doc.Pair {
	out := map[doc.Handle][]doc.Pair{}
	for _, h := range d.Handles() {
		if u, ok := d.Entity(h).(*doc.Unknown); ok {
			out[h] = u.Raw
		}
	}
	return out
}

func TestUnknownEntitiesKeepRawGroups(t *testing.T) {
	src := richFile(textEnt + insEnt + splEnt + custom + pl3d + lineEnt)
	d, warns, err := (dxf.Codec{}).Read(strings.NewReader(src))
	if err != nil {
		t.Fatal(err)
	}
	if len(warns) != 5 {
		t.Fatalf("warnings = %q", warns)
	}
	raw := unknownRaw(d)
	if len(raw) != 5 {
		t.Fatalf("%d unknown entities, want 5", len(raw))
	}
	text := d.Entity(0x30).(*doc.Unknown)
	if text.Class() != "TEXT" || text.Layer != "notes" || text.Owner != 0x1F || text.Handle() != 0x30 {
		t.Fatalf("text = %+v", text)
	}
	want := []doc.Pair{pr(5, "30"), pr(330, "1F"), pr(100, "AcDbEntity"), pr(8, "notes"), pr(6, "DASHED"), pr(100, "AcDbText"),
		pr(10, "5.5"), pr(20, "6.5"), pr(30, "0.0"), pr(40, "2.5"), pr(1, "  Hello  "), pr(72, "0"), pr(100, "AcDbText")}
	if !reflect.DeepEqual(text.Raw, want) {
		t.Fatalf("text raw = %v", text.Raw)
	}
	if b := text.Bounds(); b.Min != geom.V(5.5, 6.5) || b.Max != geom.V(5.5, 6.5) {
		t.Fatalf("text bounds = %+v", b)
	}
	ins := d.Entity(0x31).(*doc.Unknown)
	if n := len(ins.Raw); ins.Raw[n-1] != (pr(8, "0")) || ins.Raw[n-3] != (pr(0, "SEQEND")) || ins.Raw[n-8] != (pr(0, "ATTRIB")) {
		t.Fatalf("insert raw = %v", ins.Raw)
	}
	if d.Entity(0x32) != nil || d.Entity(0x33) != nil || d.Entity(0x37) != nil {
		t.Fatal("sub-records became entities")
	}
	spl := d.Entity(0x34).(*doc.Unknown)
	if b := spl.Bounds(); b.Min != geom.V(-3, -4) || b.Max != geom.V(3, 4) {
		t.Fatalf("spline bounds = %+v", b)
	}
	w := d.Entity(0x35).(*doc.Unknown)
	if w.Class() != "ACME_WIDGET" || w.Layer != "acme" || w.Color != 3 || !w.Bounds().IsEmpty() {
		t.Fatalf("widget = %+v", w)
	}
	if w.Raw[3] != (pr(1, "strange \u00e9")) {
		t.Fatalf("widget raw = %v", w.Raw)
	}
	if _, ok := d.Entity(0x36).(*doc.Unknown); !ok {
		t.Fatalf("3D polyline = %T", d.Entity(0x36))
	}
	if _, ok := d.Entity(0x40).(*doc.Line); !ok {
		t.Fatal("line lost")
	}
}

func TestWriteKeepsR12UnknownEntitiesVerbatimAndWarnsForTheRest(t *testing.T) {
	src := richFile(textEnt + insEnt + splEnt + custom + pl3d + lineEnt)
	d, _, err := (dxf.Codec{}).Read(strings.NewReader(src))
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	warns, err := (dxf.Codec{}).Write(&buf, d)
	if err != nil {
		t.Fatal(err)
	}
	if len(warns) != 2 || !strings.HasPrefix(warns[0], "SPLINE 34 not written") || !strings.HasPrefix(warns[1], "ACME_WIDGET 35 not written") {
		t.Fatalf("warnings = %q", warns)
	}
	out := buf.String()
	// The written file keeps every R12-representable record byte for byte
	// (group code padding aside, which group() and the writer share).
	for _, frag := range []string{stripR13(textEnt), insEnt, pl3d} {
		if !strings.Contains(out, frag) {
			t.Fatalf("output lacks %q", frag)
		}
	}
	for _, gone := range []string{"SPLINE", "ACME_WIDGET"} {
		if strings.Contains(out, gone) {
			t.Fatalf("output contains %s", gone)
		}
	}
	for _, kept := range []string{"DASHED", "CONTINUOUS", "BLOCK", "ENDBLK", "B1"} {
		if !strings.Contains(out, kept) {
			t.Fatalf("output lacks preserved %s", kept)
		}
	}
	if strings.Count(out, "CONTINUOUS") < 2 || strings.Count(out, "\nLTYPE\n") != 3 {
		t.Fatalf("linetype table wrong:\n%s", out)
	}

	// Reading it back gives the same unknown groups for what was written.
	back, warns, err := (dxf.Codec{}).Read(strings.NewReader(out))
	if err != nil {
		t.Fatal(err)
	}
	if len(warns) != 3 {
		t.Fatalf("reread warnings = %q", warns)
	}
	before, after := unknownRaw(d), unknownRaw(back)
	for _, h := range []doc.Handle{0x30, 0x31, 0x36} {
		if !reflect.DeepEqual(r12Groups(before[h]), after[h]) {
			t.Fatalf("%s: %v vs %v", h, before[h], after[h])
		}
	}
	if len(after) != 3 || len(back.Preserved.Blocks) != 1 || len(back.Preserved.Tables["LTYPE"]) != 1 {
		t.Fatalf("reread: %d unknown, %d blocks, %v", len(after), len(back.Preserved.Blocks), back.Preserved.Tables)
	}
}

func TestPreservedTablesAndBlocks(t *testing.T) {
	d, _, err := (dxf.Codec{}).Read(strings.NewReader(richFile(lineEnt)))
	if err != nil {
		t.Fatal(err)
	}
	lt := d.Preserved.Tables["LTYPE"]
	if len(lt) != 1 || lt[0].Name() != "DASHED" || lt[0].Type() != "LTYPE" {
		t.Fatalf("linetypes = %+v", lt)
	}
	if len(d.Preserved.Blocks) != 1 || d.Preserved.Blocks[0].Name() != "B1" || len(d.Preserved.Blocks[0].Pairs) != 19 {
		t.Fatalf("blocks = %+v", d.Preserved.Blocks)
	}
	if d.Header.HandSeed <= 0x62 {
		t.Fatalf("handseed %v not above the block handles", d.Header.HandSeed)
	}
}

func TestDuplicateHandleIsRenumbered(t *testing.T) {
	dup := strings.Replace(splEnt, "\n  5\n34\n", "\n  5\n40\n", 1) // collides with the LINE
	d, warns, err := (dxf.Codec{}).Read(strings.NewReader(richFile(lineEnt + dup + custom)))
	if err != nil {
		t.Fatal(err)
	}
	if d.Len() != 3 {
		t.Fatalf("%d entities, want 3", d.Len())
	}
	if _, ok := d.Entity(0x40).(*doc.Line); !ok {
		t.Fatalf("handle 40 = %T", d.Entity(0x40))
	}
	var spl *doc.Unknown
	for _, h := range d.Handles() {
		if u, ok := d.Entity(h).(*doc.Unknown); ok && u.Type == "SPLINE" {
			spl = u
		}
	}
	if spl == nil || spl.Handle() == 0x40 || spl.Handle() == 0x35 || spl.Raw[0] != (pr(5, spl.Handle().String())) {
		t.Fatalf("spline = %+v", spl)
	}
	found := false
	for _, w := range warns {
		found = found || strings.Contains(w, "already used")
	}
	if !found {
		t.Fatalf("no collision warning in %q", warns)
	}
}

func TestUnknownWithoutHandleGetsOne(t *testing.T) {
	src := group("0", "SECTION", "2", "ENTITIES", "0", "SOLID", "8", "0", "10", "0", "20", "0") + group("0", "ENDSEC", "0", "EOF")
	d, _, err := (dxf.Codec{}).Read(strings.NewReader(src))
	if err != nil {
		t.Fatal(err)
	}
	u := d.Entity(d.Handles()[0]).(*doc.Unknown)
	if u.Raw[0] != (pr(5, u.Handle().String())) || u.Handle() == 0 {
		t.Fatalf("raw = %v handle %v", u.Raw, u.Handle())
	}
}

func TestWriteDropsR13GroupsAndNonR12BlockEntities(t *testing.T) {
	src := strings.Replace(richFile(textEnt), group("0", "ENDBLK", "5", "62", "8", "0"),
		group("0", "LWPOLYLINE", "5", "63", "8", "0", "90", "0")+group("0", "ENDBLK", "5", "62", "8", "0"), 1)
	d, _, err := (dxf.Codec{}).Read(strings.NewReader(src))
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	warns, err := (dxf.Codec{}).Write(&buf, d)
	if err != nil {
		t.Fatal(err)
	}
	if len(warns) != 1 || !strings.HasPrefix(warns[0], "LWPOLYLINE in block B1 not written") {
		t.Fatalf("warnings = %q", warns)
	}
	out := buf.String()
	if strings.Contains(out, "AcDb") || strings.Contains(out, "LWPOLYLINE") || !strings.Contains(out, "ENDBLK") {
		t.Fatalf("output:\n%s", out)
	}
}

func TestReadTruncatedFileIsAnError(t *testing.T) {
	for _, in := range []string{
		"0\nSECTION\n",
		"0\nSECTION\n2\nENTITIES\n0\nLINE\n8\n0\n10\n0\n20\n0\n11\n1\n21\n1\n",
		"0\nSECTION\n2\nENTITIES\n0\nSECTION\n2\nTABLES\n0\nENDSEC\n0\nEOF\n",
	} {
		if d, _, err := (dxf.Codec{}).Read(strings.NewReader(in)); err == nil {
			t.Errorf("%q: opened with %d entities, want an error", in, d.Len())
		}
	}
}

func TestReadMissingEOFWarns(t *testing.T) {
	_, warns, err := (dxf.Codec{}).Read(strings.NewReader("0\nSECTION\n2\nENTITIES\n0\nENDSEC\n"))
	if err != nil || len(warns) != 1 || !strings.Contains(warns[0], "EOF") {
		t.Fatalf("warns %v err %v", warns, err)
	}
}

func TestReadNonFiniteNumbers(t *testing.T) {
	in := "0\nSECTION\n2\nENTITIES\n0\nLINE\n8\n0\n10\nnan\n20\n0\n11\ninf\n21\n1e999\n0\nLWPOLYLINE\n8\n0\n90\n2\n70\n0\n10\n0\n20\n0\n10\nnan\n20\n5\n0\nENDSEC\n0\nEOF\n"
	d, warns, err := (dxf.Codec{}).Read(strings.NewReader(in))
	if err != nil {
		t.Fatal(err)
	}
	if len(warns) != 1 || !strings.Contains(warns[0], "non-finite") {
		t.Errorf("warns %v", warns)
	}
	for _, h := range d.Handles() {
		b := d.Entity(h).Bounds()
		for _, f := range []float64{b.Min.X, b.Min.Y, b.Max.X, b.Max.Y} {
			if math.IsNaN(f) || math.IsInf(f, 0) {
				t.Fatalf("entity %s has non-finite bounds %v", h, b)
			}
		}
	}
	var buf bytes.Buffer
	(dxf.Codec{}).Write(&buf, d)
	if s := strings.ToLower(buf.String()); strings.Contains(s, "nan") || strings.Contains(s, "inf") {
		t.Error("writer emitted a non-finite number")
	}
}
