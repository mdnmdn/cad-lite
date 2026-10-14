package dwg

import (
	"math"
	"strings"
	"testing"

	"cad-lite/doc"
	"cad-lite/geom"
)

// The sample drawings are ACadSharp's sample_AC10xx.dwg: one drawing saved by AutoCAD in each
// version. The expected values below are those of its ASCII DXF export (sample_AC10xx_ascii.dxf),
// read by the DXF reader: entity counts of model space, layer names, and exact coordinates.

// allLayers is the layer table of the R2000 to R2007 samples; later versions lack the last two
// layers (the " @ 1" layers of a purged xref).
var allLayers = []string{
	"0", "Layer1", "Layer_Off", "Layer_Freeze", "Layer_Lock", "Layer_NoPlot", "Layer_color_80", "Layer_Lt_dash",
	"Layer_lw_035", "Layer_transp_70", "Layer_vp_freeze", "Layer_desc", "Layer_true_color", "Layer_color_book",
	"Defpoints", "Layer2", "Layer_lw_050", "app_layer", "color_125_33_79", "0 @ 1", "Layer_color_80 @ 1",
}

type fixture struct {
	file   string
	layers []string
	// insUnits is $INSUNITS of the file; 4, the document default, when the version has none.
	insUnits int
}

var fixtures = []fixture{
	{"sample_AC1015.dwg", allLayers, 1},
	{"sample_AC1018.dwg", allLayers, 1},
	{"sample_AC1021.dwg", allLayers, 1},
	{"sample_AC1024.dwg", allLayers[:19], 1},
	{"sample_AC1027.dwg", allLayers[:19], 1},
	{"sample_AC1032.dwg", allLayers[:19], 1},
}

func classCounts(d *doc.Drawing) map[string]int {
	n := map[string]int{}
	for _, h := range d.Handles() {
		n[d.Entity(h).Class()]++
	}
	return n
}

func TestFixtures(t *testing.T) {
	for _, fx := range fixtures {
		t.Run(fx.file, func(t *testing.T) {
			d, warns, err := readFile("testdata/"+fx.file, Codec{})
			if err != nil {
				t.Fatal(err)
			}
			var names []string
			for _, l := range d.Layers() {
				names = append(names, l.Name)
			}
			if strings.Join(names, "|") != strings.Join(fx.layers, "|") {
				t.Errorf("layers = %q, want %q", names, fx.layers)
			}
			want := map[string]int{"LINE": 26, "CIRCLE": 8, "ARC": 1, "LWPOLYLINE": 21, "TEXT": 29}
			got := classCounts(d)
			for c, n := range want {
				if got[c] != n {
					t.Errorf("%s: %d entities, want %d", c, got[c], n)
				}
			}
			if d.Len() != 85 {
				t.Errorf("%d entities, want 85", d.Len())
			}
			if d.Header.InsUnits != fx.insUnits {
				t.Errorf("InsUnits = %d, want %d", d.Header.InsUnits, fx.insUnits)
			}
			if d.Header.HandSeed <= 0x99F {
				t.Errorf("HandSeed %s does not clear the loaded handles", d.Header.HandSeed)
			}
			// The warning summary names what the DXF reader keeps as unknown objects.
			all := strings.Join(warns, "\n")
			for _, s := range []string{"15 MULTILEADER", "11 DIMENSION", "8 HATCH", "6 INSERT", "5 MTEXT", "2 SPLINE", "1 ELLIPSE"} {
				if !strings.Contains(all, s) {
					t.Errorf("warnings lack %q:\n%s", s, all)
				}
			}
			if len(warns) == 0 || !strings.HasPrefix(warns[0], "skipped ") {
				t.Fatalf("no skipped summary first in %q", warns)
			}
			list := strings.TrimPrefix(warns[0][:strings.Index(warns[0], " (")], "skipped ")
			for _, part := range strings.Split(list, ", ") {
				name := part[strings.Index(part, " ")+1:]
				switch name {
				case "LINE", "CIRCLE", "ARC", "LWPOLYLINE", "TEXT":
					t.Errorf("an imported class is listed as skipped: %s", part)
				}
			}
			if strings.Contains(all, "CRC") || strings.Contains(all, "could not be read") {
				t.Errorf("a clean file reported damage:\n%s", all)
			}
		})
	}
}

func TestFixtureLayerProperties(t *testing.T) {
	for _, fx := range fixtures {
		t.Run(fx.file, func(t *testing.T) {
			d := mustRead(t, "testdata/"+fx.file, Codec{})
			want := map[string]doc.Layer{
				"0":                {Name: "0", Color: 7},
				"Layer_Off":        {Name: "Layer_Off", Color: 7, Off: true},
				"Layer_Freeze":     {Name: "Layer_Freeze", Color: 7, Frozen: true},
				"Layer_Lock":       {Name: "Layer_Lock", Color: 7, Locked: true},
				"Layer_NoPlot":     {Name: "Layer_NoPlot", Color: 7, NoPlot: true},
				"Layer_color_80":   {Name: "Layer_color_80", Color: 80},
				"Layer_Lt_dash":    {Name: "Layer_Lt_dash", Color: 10, Linetype: "ACAD_ISO02W100"},
				"Layer_lw_035":     {Name: "Layer_lw_035", Color: 7, Lineweight: 35},
				"Layer_lw_050":     {Name: "Layer_lw_050", Color: 7, Lineweight: 50},
				"Layer2":           {Name: "Layer2", Color: 65},
				"Defpoints":        {Name: "Defpoints", Color: 7, NoPlot: true},
				"Layer_true_color": {Name: "Layer_true_color", Color: 150}, // RGB 17,146,238: nearest index
				"Layer_color_book": {Name: "Layer_color_book", Color: 31},
			}
			for name, w := range want {
				l := d.Layer(name)
				if l == nil {
					t.Errorf("no layer %q", name)
					continue
				}
				if fx.file == "sample_AC1015.dwg" && name == "Layer_lw_035" {
					// R2000 stores the lineweight too.
					w.Lineweight = 35
				}
				if *l != w {
					t.Errorf("layer %q = %+v, want %+v", name, *l, w)
				}
			}
		})
	}
}

func TestFixtureGeometry(t *testing.T) {
	deg := func(v float64) float64 { return geom.NormAngle(v * math.Pi / 180) }
	for _, fx := range fixtures {
		t.Run(fx.file, func(t *testing.T) {
			d := mustRead(t, "testdata/"+fx.file, Codec{})
			line := func(h doc.Handle) *doc.Line {
				l, _ := d.Entity(h).(*doc.Line)
				if l == nil {
					t.Fatalf("handle %s is not a LINE", h)
				}
				return l
			}
			// The DXF export prints 16 significant digits, one fewer than a double needs.
			checkVec := func(what string, got geom.Vec2, x, y float64) {
				t.Helper()
				if !near(got.X, x) || !near(got.Y, y) {
					t.Errorf("%s = (%v, %v), want (%v, %v)", what, got.X, got.Y, x, y)
				}
			}
			l := line(0x2C7)
			checkVec("LINE 2C7 start", l.Start, 3.592533998909388, 1.477241896180196)
			checkVec("LINE 2C7 end", l.End, 6.863547033979557, 1.477241896180196)
			if l.Layer != "0" || l.Color != doc.ByLayer || l.Owner != doc.ModelSpace {
				t.Errorf("LINE 2C7 common = %+v", l.EntityCommon)
			}
			l = line(0x2CF)
			if l.Color != 3 {
				t.Errorf("LINE 2CF colour = %d, want 3", l.Color)
			}
			l = line(0x517)
			checkVec("LINE 517 start", l.Start, 330.2890594765795, 2.941179455067986)
			checkVec("LINE 517 end", l.End, 364.4872644138525, 37.13938439234094)
			if l.Layer != "Layer2" {
				t.Errorf("LINE 517 layer = %q, want Layer2", l.Layer)
			}

			c, _ := d.Entity(0x31F).(*doc.Circle)
			if c == nil {
				t.Fatal("handle 31F is not a CIRCLE")
			}
			checkVec("CIRCLE 31F centre", c.Center, 46.87247836145255, 6.672963963946202)
			if !near(c.Radius, 3.682480951587929) {
				t.Errorf("CIRCLE 31F radius = %v", c.Radius)
			}

			a, _ := d.Entity(0x320).(*doc.Arc)
			if a == nil {
				t.Fatal("handle 320 is not an ARC")
			}
			checkVec("ARC 320 centre", a.Center, 56.35179242595231, 4.697601732518876)
			if !near(a.Radius, 3.044494390598888) {
				t.Errorf("ARC 320 radius = %v", a.Radius)
			}
			// The file stores radians; the DXF stores degrees (341.0435453511963 and 161.0435453511958).
			if math.Abs(a.StartAngle-deg(341.0435453511963)) > 1e-12 || math.Abs(a.EndAngle-deg(161.0435453511958)) > 1e-12 {
				t.Errorf("ARC 320 angles = %v, %v", a.StartAngle, a.EndAngle)
			}

			p, _ := d.Entity(0x2E4).(*doc.LWPolyline)
			if p == nil {
				t.Fatal("handle 2E4 is not an LWPOLYLINE")
			}
			if !p.Closed || len(p.Points) != 7 || len(p.Bulges) != 7 {
				t.Fatalf("LWPOLYLINE 2E4: closed %v, %d points, %d bulges", p.Closed, len(p.Points), len(p.Bulges))
			}
			checkVec("LWPOLYLINE 2E4 vertex 0", p.Points[0], 28.00563684412818, 0.3613048628239071)
			checkVec("LWPOLYLINE 2E4 vertex 6", p.Points[6], 28.66642855916602, 1.333494561262174)
			if !near(p.Bulges[1], -0.7995594967893557) || !near(p.Bulges[4], 0.7187509838525309) || !near(p.Bulges[6], -0.0476105397037214) || p.Bulges[0] != 0 {
				t.Errorf("LWPOLYLINE 2E4 bulges = %v", p.Bulges)
			}

			w, _ := d.Entity(0x35B).(*doc.LWPolyline)
			if w == nil {
				t.Fatal("handle 35B is not an LWPOLYLINE")
			}
			if w.Closed || len(w.Points) != 5 || len(w.Widths) != 5 {
				t.Fatalf("LWPOLYLINE 35B: closed %v, %d points, %d widths", w.Closed, len(w.Points), len(w.Widths))
			}
			if !near(w.Widths[0][0], 0.2) || !near(w.Widths[0][1], 0.1) || w.Widths[1] != [2]float64{0, 0} || !near(w.Widths[4][0], 0.1) || !near(w.Widths[4][1], 0.5) {
				t.Errorf("LWPOLYLINE 35B widths = %v", w.Widths)
			}
		})
	}
}

// TestFixtureExtents checks the drawing against the header's model space extents ($EXTMIN,
// $EXTMAX of the DXF), which bound everything the file draws.
func TestFixtureExtents(t *testing.T) {
	for _, fx := range fixtures {
		t.Run(fx.file, func(t *testing.T) {
			data := mustBytes(t, "testdata/"+fx.file)
			f, err := openFile(data)
			if err != nil {
				t.Fatal(err)
			}
			h, ok := f.readHeader()
			if !ok {
				t.Fatal("the header did not parse")
			}
			if h.extMin != [3]float64{0, -566.1685822008872, -5.091688756873282} || h.extMax != [3]float64{1852.1856335495922, 111.23549508976582, 5.47860020423747} {
				t.Errorf("extents = %v .. %v", h.extMin, h.extMax)
			}
			if h.limMax != [2]float64{12, 9} || h.clayer != 0x10 {
				t.Errorf("limits %v, current layer handle %#x", h.limMax, h.clayer)
			}
			d := mustRead(t, "testdata/"+fx.file, Codec{})
			b := d.Extents()
			if b.Min.X < h.extMin[0]-1e-9 || b.Min.Y < h.extMin[1]-1e-9 || b.Max.X > h.extMax[0]+1e-9 || b.Max.Y > h.extMax[1]+1e-9 {
				t.Errorf("drawing extents %v exceed the header's", b)
			}
			if d.CurrentLayer != "0" {
				t.Errorf("current layer = %q", d.CurrentLayer)
			}
		})
	}
}

func TestEmptyDrawing(t *testing.T) {
	d, warns, err := readFile("testdata/empty_AC1032.dwg", Codec{})
	if err != nil {
		t.Fatal(err)
	}
	if d.Len() != 0 || len(warns) != 0 {
		t.Errorf("empty.dwg: %d entities, warnings %v", d.Len(), warns)
	}
	if d.Layer("0") == nil {
		t.Error("layer 0 is missing")
	}
}

// TestR14MatchesR2000 compares the R14 sample with the R2000 one, which holds the same drawing:
// handles differ between the files, so the geometry is compared as a multiset.
func TestR14MatchesR2000(t *testing.T) {
	a := mustRead(t, "testdata/sample_AC1014.dwg", Codec{})
	b := mustRead(t, "testdata/sample_AC1015.dwg", Codec{})
	sig := func(d *doc.Drawing) map[string]int {
		m := map[string]int{}
		for _, h := range d.Handles() {
			e := d.Entity(h)
			m[strings.ToUpper(strings.ReplaceAll(e.Common().Layer, " @ ", "___"))+"|"+geometrySignature(e)]++
		}
		return m
	}
	sa, sb := sig(a), sig(b)
	if len(sa) != len(sb) {
		t.Errorf("%d distinct entities in R14, %d in R2000", len(sa), len(sb))
	}
	for k, n := range sa {
		if sb[k] != n {
			t.Errorf("R14 entity %s occurs %d times, R2000 %d", k, n, sb[k])
		}
	}
	if len(a.Layers()) != len(b.Layers()) {
		t.Errorf("%d layers in R14, %d in R2000", len(a.Layers()), len(b.Layers()))
	}
	for i, l := range a.Layers() {
		o := b.Layers()[i]
		// R14 stores " @ " in a layer name as "___".
		if !strings.EqualFold(strings.ReplaceAll(o.Name, " @ ", "___"), l.Name) || l.Color != o.Color || l.Off != o.Off || l.Frozen != o.Frozen || l.Locked != o.Locked || l.Linetype != o.Linetype {
			t.Errorf("layer %d: R14 %+v, R2000 %+v", i, *l, *o)
		}
	}
}

func TestR14(t *testing.T) {
	d, warns, err := readFile("testdata/sample_AC1014.dwg", Codec{})
	if err != nil {
		t.Fatal(err)
	}
	got := classCounts(d)
	for c, n := range map[string]int{"LINE": 26, "CIRCLE": 8, "ARC": 1, "LWPOLYLINE": 21, "TEXT": 29} {
		if got[c] != n {
			t.Errorf("%s: %d entities, want %d", c, got[c], n)
		}
	}
	if len(d.Layers()) != 21 || d.Layers()[2].Name != "LAYER_OFF" || !d.Layers()[2].Off || !d.Layers()[3].Frozen || !d.Layers()[4].Locked {
		t.Errorf("layers = %v", d.Layers())
	}
	// R14 has no $INSUNITS and no lineweights or plot flags: the document defaults stay.
	if d.Header.InsUnits != 4 || d.Layers()[8].Lineweight != 0 || d.Layers()[5].NoPlot {
		t.Errorf("units %d, lineweight %d, no-plot %v", d.Header.InsUnits, d.Layers()[8].Lineweight, d.Layers()[5].NoPlot)
	}
	if all := strings.Join(warns, "\n"); strings.Contains(all, " TEXT") || !strings.Contains(all, "5 MTEXT") {
		t.Errorf("warnings = %q", warns)
	}
}
