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

func TestReadModernLWPolylineAndSkipUnknown(t *testing.T) {
	d, warns, err := (dxf.Codec{}).Read(strings.NewReader(modern))
	if err != nil {
		t.Fatal(err)
	}
	if len(warns) != 1 || !strings.Contains(warns[0], "SPLINE") {
		t.Fatalf("warnings = %v", warns)
	}
	p, ok := d.Entity(0x2A).(*doc.LWPolyline)
	if !ok || len(p.Points) != 2 || p.Bulges[0] != 1 || p.Points[1] != geom.V(4.5, 2.5) {
		t.Fatalf("polyline = %+v", d.Entity(0x2A))
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
