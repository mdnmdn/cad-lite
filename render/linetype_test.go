package render_test

import (
	"math"
	"slices"
	"testing"

	"cad-lite/doc"
	"cad-lite/geom"
	"cad-lite/render"
)

// dashDrawing returns a drawing with the named library linetypes loaded and a builder fed by its
// change sets.
func dashDrawing(t *testing.T, linetypes ...string) (*doc.Drawing, *render.Builder) {
	t.Helper()
	d := doc.New()
	tx, _ := d.Begin()
	for _, n := range linetypes {
		if err := tx.LoadLibraryLinetype(n); err != nil {
			t.Fatal(err)
		}
	}
	tx.Commit()
	b := render.NewBuilder(d, render.Color{A: 255})
	d.OnChange(b.Invalidate)
	return d, b
}

func add(t *testing.T, d *doc.Drawing, e doc.Entity) doc.Handle {
	t.Helper()
	tx, err := d.Begin()
	if err != nil {
		t.Fatal(err)
	}
	h := tx.Add(e)
	tx.Commit()
	return h
}

func dashLine(lt string, length float64) *doc.Line {
	l := &doc.Line{End: geom.V(length, 0)}
	l.Linetype = lt
	return l
}

// runs splits dashed items into one item per run (DisplayItem.Starts).
func runs(items []render.DisplayItem) []render.DisplayItem {
	var out []render.DisplayItem
	for _, it := range items {
		if it.Starts == nil {
			out = append(out, it)
			continue
		}
		for k, s := range it.Starts {
			e := len(it.Points)
			if k+1 < len(it.Starts) {
				e = int(it.Starts[k+1])
			}
			r := it
			r.Points, r.Starts = it.Points[s:e], nil
			out = append(out, r)
		}
	}
	return out
}

func span(it render.DisplayItem) (a, b float64) { return it.Points[0].X, it.Points[len(it.Points)-1].X }

func TestDashedLineIsCentredAndStartsAndEndsWithADash(t *testing.T) {
	d, b := dashDrawing(t, "ACAD_ISO02W100") // 12 dash, 3 gap: period 15
	h := add(t, d, dashLine("ACAD_ISO02W100", 100))
	items := runs(b.Items(h, render.DefaultView(800, 600)))
	// Five repetitions and a closing dash (87) leave 13 of slack, 6.5 on each end dash.
	want := [][2]float64{{0, 18.5}, {21.5, 33.5}, {36.5, 48.5}, {51.5, 63.5}, {66.5, 78.5}, {81.5, 100}}
	if len(items) != len(want) {
		t.Fatalf("%d dashes, want %d: %+v", len(items), len(want), items)
	}
	for i, it := range items {
		a, z := span(it)
		if math.Abs(a-want[i][0]) > 1e-9 || math.Abs(z-want[i][1]) > 1e-9 {
			t.Fatalf("dash %d spans %v..%v, want %v", i, a, z, want[i])
		}
	}
}

func TestDashedLineShortOrDenseDrawsSolid(t *testing.T) {
	d, b := dashDrawing(t, "ACAD_ISO02W100")
	short := add(t, d, dashLine("ACAD_ISO02W100", 10)) // under one repetition
	long := add(t, d, dashLine("ACAD_ISO02W100", 1000))
	v := render.DefaultView(800, 600)
	if n := len(runs(b.Items(short, v))); n != 1 {
		t.Fatalf("short line: %d items, want 1 (solid)", n)
	}
	v.Scale = 0.1 // a 15-unit period is 1.5 px
	if n := len(runs(b.Items(long, v))); n != 1 {
		t.Fatalf("dense pattern: %d items, want 1 (solid)", n)
	}
	v.Scale = 0.25 // 3.75 px
	if n := len(runs(b.Items(long, v))); n < 60 {
		t.Fatalf("visible pattern: %d items, want dashes", n)
	}
}

func TestDashPhaseDoesNotMoveWhenPanning(t *testing.T) {
	d, _ := dashDrawing(t, "ACAD_ISO04W100")
	h := add(t, d, dashLine("ACAD_ISO04W100", 500))
	v := render.DefaultView(800, 600)
	a := render.NewBuilder(d, render.Color{A: 255}).Items(h, v)
	v.PanBy(123.4, -56.7)
	z := render.NewBuilder(d, render.Color{A: 255}).Items(h, v)
	if len(a) != len(z) {
		t.Fatalf("dash count changed with the pan: %d, %d", len(a), len(z))
	}
	for i := range a {
		if !slices.Equal(a[i].Points, z[i].Points) {
			t.Fatalf("dash %d moved with the pan", i)
		}
	}
}

func TestDotsDrawAsPixelDashes(t *testing.T) {
	d, b := dashDrawing(t, "ACAD_ISO07W100") // dot, 3 gap
	h := add(t, d, dashLine("ACAD_ISO07W100", 30))
	v := render.DefaultView(800, 600)
	v.Scale = 4 // band 2: a dot is a quarter unit
	items := runs(b.Items(h, v))
	if len(items) != 10 {
		t.Fatalf("%d dots, want 10", len(items))
	}
	for i, it := range items {
		a, z := span(it)
		if l := z - a; l <= 0 || l > 0.25+1e-9 {
			t.Fatalf("dot %d is %v long, want up to one pixel (0.25)", i, l)
		}
	}
}

func TestLinetypeScaleAndLTSCALEScaleThePattern(t *testing.T) {
	d, b := dashDrawing(t, "ACAD_ISO02W100")
	l := dashLine("ACAD_ISO02W100", 300)
	l.LinetypeScale = 2
	h := add(t, d, l)
	v := render.DefaultView(800, 600)
	n1 := len(runs(b.Items(h, v))) // period 30: floor(288/30)=9 repetitions and a closing dash
	if n1 != 10 {
		t.Fatalf("entity scale 2: %d dashes, want 10", n1)
	}
	tx, _ := d.Begin()
	cur := d.Current()
	cur.LTScale = 2
	if err := tx.SetCurrent(cur); err != nil {
		t.Fatal(err)
	}
	tx.Commit()
	if n := len(runs(b.Items(h, v))); n != 5 {
		t.Fatalf("LTSCALE 2 after entity scale 2: %d dashes, want 5", n)
	}
	d.Undo()
	if n := len(runs(b.Items(h, v))); n != 10 {
		t.Fatalf("undo of LTSCALE: %d dashes, want 10", n)
	}
}

func TestDashedCircleArcAndPolylineSegments(t *testing.T) {
	d, b := dashDrawing(t, "ACAD_ISO02W100")
	c := &doc.Circle{Center: geom.V(0, 0), Radius: 100}
	c.Linetype = "ACAD_ISO02W100"
	ch := add(t, d, c)
	v := render.DefaultView(800, 600)
	// A closed loop is not centred: the pattern runs from angle 0, ceil(628.3/15) repetitions.
	if n := len(runs(b.Items(ch, v))); n != 42 {
		t.Fatalf("circle: %d dashes, want 42", n)
	}
	p := &doc.LWPolyline{Points: []geom.Vec2{geom.V(0, 0), geom.V(100, 0), geom.V(100, 100)}, Bulges: []float64{0, 0.5}}
	p.Linetype = "ACAD_ISO02W100"
	ph := add(t, d, p)
	items := runs(b.Items(ph, v))
	// PLINEGEN 0: each segment restarts the pattern, so a dash ends on the middle vertex and the
	// next one starts there.
	var atVertex int
	for _, it := range items {
		for _, q := range []geom.Vec2{it.Points[0], it.Points[len(it.Points)-1]} {
			if q.Dist(geom.V(100, 0)) < 1e-9 {
				atVertex++
			}
		}
	}
	if atVertex != 2 || len(items) < 12 {
		t.Fatalf("polyline: %d dashes, %d dash ends on the vertex, want 2", len(items), atVertex)
	}
}

func TestComplexAndMissingLinetypesDrawSolid(t *testing.T) {
	d, b := dashDrawing(t)
	d.LoadLinetype(doc.Linetype{Name: "GAS_LINE", Pattern: []float64{12.7, -5.08, -5.08}, Raw: []doc.Pair{{Code: 0, Value: "LTYPE"}}})
	v := render.DefaultView(800, 600)
	for _, lt := range []string{"GAS_LINE", "NO_SUCH"} {
		if n := len(runs(b.Items(add(t, d, dashLine(lt, 200)), v))); n != 1 {
			t.Fatalf("%s: %d items, want 1 (solid)", lt, n)
		}
	}
}

func TestTextDrawsSolidOnADashedLayer(t *testing.T) {
	d, b := dashDrawing(t, "HIDDEN")
	d.Layer("0").Linetype = "HIDDEN"
	b.SyncLayers()
	h := add(t, d, doc.NewText(geom.V(0, 0), 100, 0, "I"))
	v := render.DefaultView(800, 600)
	if n := len(runs(b.Items(h, v))); n != 1 {
		t.Fatalf("text: %d items, want its one stroke", n)
	}
}
