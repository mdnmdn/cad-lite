package dwg

import (
	"math"
	"os"
	"path/filepath"
	"testing"

	"cad-lite/doc"
	"cad-lite/fileio/dxf"
)

// TestParityWithDXF compares each sample DWG with the ASCII DXF export of the same drawing in
// ACadSharp's samples folder: every DWG entity must equal the DXF reader's entity with the same
// handle, and the layer tables must match. The DXF exports are 2.6 MB each, so they are not
// kept here; point DWG_PARITY_DIR at a checkout of ACadSharp's samples directory to run it.
func TestParityWithDXF(t *testing.T) {
	dir := os.Getenv("DWG_PARITY_DIR")
	if dir == "" {
		t.Skip("DWG_PARITY_DIR is not set")
	}
	for _, ver := range []string{"AC1015", "AC1018", "AC1021", "AC1024", "AC1027", "AC1032"} {
		t.Run(ver, func(t *testing.T) {
			dd := mustRead(t, filepath.Join("testdata", "sample_"+ver+".dwg"), Codec{})
			xf, err := os.Open(filepath.Join(dir, "sample_"+ver+"_ascii.dxf"))
			if err != nil {
				t.Skip(err)
			}
			defer xf.Close()
			dx, _, err := dxf.Codec{}.Read(xf)
			if err != nil {
				t.Fatal(err)
			}
			compareDrawings(t, dd, dx)
		})
	}
}

func compareDrawings(t *testing.T, dw, dx *doc.Drawing) {
	t.Helper()
	if len(dw.Layers()) != len(dx.Layers()) {
		t.Errorf("layers: dwg %d, dxf %d", len(dw.Layers()), len(dx.Layers()))
	}
	for i, l := range dx.Layers() {
		if i >= len(dw.Layers()) {
			break
		}
		g := dw.Layers()[i]
		if *g != *l {
			t.Errorf("layer %d: dwg %+v, dxf %+v", i, *g, *l)
		}
	}
	n := 0
	for _, h := range dw.Handles() {
		e := dw.Entity(h)
		x := dx.Entity(h)
		if x == nil {
			t.Errorf("%s %s: no such handle in the DXF", e.Class(), h)
			continue
		}
		n++
		if e.Class() != x.Class() {
			t.Errorf("%s: class dwg %s, dxf %s", h, e.Class(), x.Class())
			continue
		}
		if e.Common().Layer != x.Common().Layer || e.Common().Color != x.Common().Color {
			t.Errorf("%s %s: layer/colour dwg %q/%d, dxf %q/%d", e.Class(), h, e.Common().Layer, e.Common().Color, x.Common().Layer, x.Common().Color)
		}
		if !sameGeometry(e, x) {
			t.Errorf("%s %s: geometry differs\n dwg %+v\n dxf %+v", e.Class(), h, e, x)
		}
	}
	// Every supported entity the DXF holds in model space must be in the DWG too.
	for _, h := range dx.Handles() {
		x := dx.Entity(h)
		switch x.Class() {
		case "LINE", "CIRCLE", "ARC", "LWPOLYLINE":
			if dw.Entity(h) == nil {
				t.Errorf("%s %s: in the DXF but not read from the DWG", x.Class(), h)
			}
		}
	}
	t.Logf("%d entities compared", n)
}

func near(a, b float64) bool { return math.Abs(a-b) <= 1e-9*(1+math.Abs(a)+math.Abs(b)) }

func sameGeometry(a, b doc.Entity) bool {
	switch e := a.(type) {
	case *doc.Line:
		x := b.(*doc.Line)
		return near(e.Start.X, x.Start.X) && near(e.Start.Y, x.Start.Y) && near(e.End.X, x.End.X) && near(e.End.Y, x.End.Y)
	case *doc.Circle:
		x := b.(*doc.Circle)
		return near(e.Center.X, x.Center.X) && near(e.Center.Y, x.Center.Y) && near(e.Radius, x.Radius)
	case *doc.Arc:
		x := b.(*doc.Arc)
		return near(e.Center.X, x.Center.X) && near(e.Center.Y, x.Center.Y) && near(e.Radius, x.Radius) &&
			near(e.StartAngle, x.StartAngle) && near(e.EndAngle, x.EndAngle)
	case *doc.LWPolyline:
		x := b.(*doc.LWPolyline)
		if e.Closed != x.Closed || len(e.Points) != len(x.Points) {
			return false
		}
		for i := range e.Points {
			if !near(e.Points[i].X, x.Points[i].X) || !near(e.Points[i].Y, x.Points[i].Y) || !near(e.Bulges[i], x.Bulges[i]) {
				return false
			}
		}
		return true
	}
	return true
}
