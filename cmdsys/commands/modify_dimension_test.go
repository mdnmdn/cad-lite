package commands_test

import (
	"math"
	"strings"
	"testing"

	"cad-lite/cmdsys"
	"cad-lite/doc"
	"cad-lite/geom"
)

// testDim returns a horizontal dimension of the points (0,0) and (1000,0) with its line at
// y = -200 and the ISO-25 style at DIMSCALE 10: text 25 high centred on (500, -181.25), extension
// lines from 6.25 below the origins to 12.5 past the line.
func testDim() *doc.Dimension {
	st := doc.DefaultDimStyle()
	st.Scale = 10
	d := &doc.Dimension{Kind: doc.DimRotated, Ext1: geom.V(0, 0), Ext2: geom.V(1000, 0), DimLine: geom.V(1000, -200)}
	d.SetStyle(nil, st)
	d.Recompute()
	return d
}

// addDim adds d to the session's drawing and returns its handle.
func addDim(t *testing.T, s *cmdsys.Session, d *doc.Dimension) doc.Handle {
	t.Helper()
	addEntities(t, s, d)
	return d.Handle()
}

func dimOf(t *testing.T, s *cmdsys.Session, h doc.Handle) *doc.Dimension {
	t.Helper()
	d, ok := s.Drawing().Entity(h).(*doc.Dimension)
	if !ok {
		t.Fatalf("entity %v is %T", h, s.Drawing().Entity(h))
	}
	return d
}

func TestDimensionSnaps(t *testing.T) {
	s := newSession(t)
	addDim(t, s, testDim())
	d, ix := s.Drawing(), s.Index()
	all := cmdsys.DefaultSnaps | cmdsys.SnapNode | cmdsys.SnapInsertion
	// The text offers nothing: no glyph stroke endpoints, no insertion point.
	if hit, ok := cmdsys.FindSnap(d, ix, geom.V(500, -181), 5, all); ok {
		t.Fatalf("the text snapped: %+v", hit)
	}
	cases := []struct {
		at    geom.Vec2
		modes cmdsys.SnapMode
		want  geom.Vec2
		mode  cmdsys.SnapMode
	}{
		{geom.V(2, 2), cmdsys.SnapNode, geom.V(0, 0), cmdsys.SnapNode},
		{geom.V(998, -198), cmdsys.SnapNode, geom.V(1000, -200), cmdsys.SnapNode},
		{geom.V(1, -5), cmdsys.DefaultSnaps, geom.V(0, -6.25), cmdsys.SnapEndpoint},
		{geom.V(501, -201), cmdsys.DefaultSnaps, geom.V(500, -200), cmdsys.SnapMidpoint},
		{geom.V(1, -199), cmdsys.DefaultSnaps, geom.V(0, -200), cmdsys.SnapEndpoint},
		{geom.V(1, -199), cmdsys.SnapIntersection, geom.V(0, -200), cmdsys.SnapIntersection},
		{geom.V(300, -202), cmdsys.SnapNearest, geom.V(300, -200), cmdsys.SnapNearest},
	}
	for _, c := range cases {
		hit, ok := cmdsys.FindSnap(d, ix, c.at, 5, c.modes)
		if !ok || hit.Mode != c.mode || !hit.Point.Near(c.want, 1e-9) {
			t.Errorf("snap at %v = %+v %v, want %v %v", c.at, hit, ok, c.mode, c.want)
		}
	}
	// The arrowheads are not snapped to: their base corners are no endpoints.
	if hit, ok := cmdsys.FindSnap(d, ix, geom.V(25, -195.8), 2, cmdsys.SnapEndpoint); ok {
		t.Fatalf("an arrowhead corner snapped: %+v", hit)
	}
	// A typed NOD override snaps to the definition point of the dimension the pick touches.
	s.Script("LINE\nNOD\n")
	s.Click(geom.V(1, -100), false)
	s.Script("50,50\n\n")
	if l := entities[*doc.Line](s); len(l) != 1 || l[0].Start != geom.V(0, 0) {
		t.Fatalf("lines = %+v\n%s", l, history(s))
	}
}

func TestDimensionPickAndBoxSelection(t *testing.T) {
	s := newSession(t)
	h := addDim(t, s, testDim())
	d, ix := s.Drawing(), s.Index()
	// Inside the text box, between strokes, and on a line both pick it.
	for _, p := range []geom.Vec2{geom.V(500, -181.25), geom.V(0.5, -100), geom.V(700, -200.5)} {
		if got, ok := cmdsys.Pick(d, ix, p, 1); !ok || got != h {
			t.Errorf("pick at %v = %v %v", p, got, ok)
		}
	}
	if _, ok := cmdsys.Pick(d, ix, geom.V(500, -100), 1); ok {
		t.Error("a click in the empty middle picked the dimension")
	}
	// A window must hold the whole dimension, its definition points included.
	if got := cmdsys.SelectBox(d, ix, geom.V(-20, -250), geom.V(1020, -3), 0.1); len(got) != 0 {
		t.Errorf("a window leaving out the origins selected %v", got)
	}
	if got := cmdsys.SelectBox(d, ix, geom.V(-20, -250), geom.V(1020, 20), 0.1); len(got) != 1 {
		t.Errorf("a window around everything selected %v", got)
	}
	// A crossing window around a definition point alone selects it, as for STRETCH.
	if got := cmdsys.SelectBox(d, ix, geom.V(3, 3), geom.V(-3, -3), 0.1); len(got) != 1 {
		t.Errorf("a crossing around the origin selected %v", got)
	}
	// A crossing window inside the text box selects it.
	if got := cmdsys.SelectBox(d, ix, geom.V(502, -180), geom.V(498, -182), 0.1); len(got) != 1 {
		t.Errorf("a crossing inside the text box selected %v", got)
	}
	// A fence inside the text box meets it.
	s.Script("ERASE F 490,-181 510,-181  ")
	if len(entities[*doc.Dimension](s)) != 0 {
		t.Fatalf("the fence did not select the dimension\n%s", history(s))
	}
}

func TestDimensionMoveRotateScaleMirror(t *testing.T) {
	s := newSession(t)
	h := addDim(t, s, testDim())
	sel := func() { s.Selection().Clear(); s.Selection().Add(h) }
	sel()
	xfeed(s, "MOVE", "0,0", "10,5")
	if d := dimOf(t, s, h); d.Ext1 != geom.V(10, 5) || d.Measurement() != 1000 {
		t.Fatalf("after MOVE: %+v", d)
	}
	sel()
	xfeed(s, "ROTATE", "10,5", "90")
	if d := dimOf(t, s, h); math.Abs(d.Rotation-math.Pi/2) > 1e-9 || math.Abs(d.Measurement()-1000) > 1e-9 || !d.Ext2.Near(geom.V(10, 1005), 1e-9) {
		t.Fatalf("after ROTATE: rot %v measurement %v ext2 %v", d.Rotation, d.Measurement(), d.Ext2)
	}
	sel()
	xfeed(s, "SCALE", "10,5", "0.5")
	if d := dimOf(t, s, h); math.Abs(d.Measurement()-500) > 1e-9 || d.Style().Scale != 10 || d.MeasurementText() != "500" {
		t.Fatalf("after SCALE: %v %q style %+v", d.Measurement(), d.MeasurementText(), d.Style())
	}

	// MIRROR keeps the text readable whatever MIRRTEXT says, as AutoCAD dimensions do.
	for _, mt := range []string{"0", "1"} {
		s2 := newSession(t)
		h2 := addDim(t, s2, testDim())
		xfeed(s2, "MIRRTEXT", mt)
		s2.Selection().Add(h2)
		xfeed(s2, "MIRROR", "0,0", "10,0", "N")
		ds := entities[*doc.Dimension](s2)
		if len(ds) != 2 {
			t.Fatalf("MIRRTEXT %s: %d dimensions\n%s", mt, len(ds), history(s2))
		}
		m := ds[1]
		lay := m.Layout()
		if math.Abs(m.Measurement()-1000) > 1e-9 || lay.Text.Generation != 0 || math.Abs(lay.Text.Rotation) > 1e-9 {
			t.Fatalf("MIRRTEXT %s: mirrored text %+v", mt, lay.Text)
		}
		if !m.DimLine.Near(geom.V(1000, 200), 1e-9) || !(m.TextPos.Y > 200) {
			t.Fatalf("MIRRTEXT %s: line %v text %v (text must sit above its line)", mt, m.DimLine, m.TextPos)
		}
	}
}

func TestStretchDimension(t *testing.T) {
	s := newSession(t)
	h := addDim(t, s, testDim())
	// The crossing window holds the second origin and the dimension line point.
	xfeed(s, "STRETCH", "C", "990,10", "1010,-210", "", "0,0", "200,0")
	d := dimOf(t, s, h)
	if d.Ext1 != geom.V(0, 0) || d.Ext2 != geom.V(1200, 0) || d.Measurement() != 1200 || d.TextMoved || !d.TextPos.Near(geom.V(600, -181.25), 1e-9) {
		t.Fatalf("after stretching one end: %+v\n%s", d, history(s))
	}
	// Only the text inside: it moves and becomes user-positioned.
	xfeed(s, "STRETCH", "C", "590,-170", "610,-190", "", "0,0", "0,-100")
	d = dimOf(t, s, h)
	if !d.TextMoved || !d.TextPos.Near(geom.V(600, -281.25), 1e-9) || d.Ext2 != geom.V(1200, 0) {
		t.Fatalf("after stretching the text: %+v", d)
	}
	// Everything inside moves it whole.
	xfeed(s, "STRETCH", "C", "-50,50", "1300,-400", "", "0,0", "10,10")
	d = dimOf(t, s, h)
	if d.Ext1 != geom.V(10, 10) || d.Ext2 != geom.V(1210, 10) || !d.TextPos.Near(geom.V(610, -271.25), 1e-9) {
		t.Fatalf("after stretching all of it: %+v", d)
	}
}

func TestDimensionTrimExtend(t *testing.T) {
	s := newSession(t)
	h := addDim(t, s, testDim())
	before := *dimOf(t, s, h)
	// The extension line is a cutting edge.
	addEntities(t, s, line(geom.V(-100, -100), geom.V(100, -100)))
	s.Script("TRIM -50,-100 ")
	if ls := sortedLines(s); len(ls) != 1 || !hasLine(ls, geom.V(0, -100), geom.V(100, -100)) {
		t.Fatalf("lines = %v\n%s", dumpLines(ls), history(s))
	}
	// The dimension itself is never trimmed: a pick says so, a crossing passes over it.
	s.Script("TRIM 500,-200 ")
	wantHistory(t, s, "Cannot trim this object.")
	s.Script("TRIM C 310,-190 290,-210 ")
	s.Script("EXTEND 700,-200 ")
	wantHistory(t, s, "Cannot extend")
	after := dimOf(t, s, h)
	if after.Ext1 != before.Ext1 || after.Ext2 != before.Ext2 || after.DimLine != before.DimLine {
		t.Fatalf("the dimension changed: %+v", after)
	}
}

func TestDimensionRefusedByCurveEdits(t *testing.T) {
	s := newSession(t)
	addDim(t, s, testDim())
	addEntities(t, s, line(geom.V(2000, 0), geom.V(2000, 100)))
	s.Script("OFFSET 10 500,-200 500,-300 ")
	wantHistory(t, s, "Cannot offset this object.")
	s.Escape()
	s.Script("FILLET 500,-200 2000,50")
	wantHistory(t, s, "supports two lines.")
	s.Escape()
	s.Script("BREAK 500,-200 600,-200")
	wantHistory(t, s, "Cannot break this object.")
	s.Escape()
	s.Script("JOIN 500,-200  ")
	wantHistory(t, s, "Cannot join the source object.")
	s.Escape()
	if ds := entities[*doc.Dimension](s); len(ds) != 1 || len(entities[*doc.Line](s)) != 1 {
		t.Fatalf("the drawing changed\n%s", history(s))
	}
}

func TestExplodeDimension(t *testing.T) {
	s := newSession(t)
	d := testDim()
	d.Color = 3
	h := addDim(t, s, d)
	s.Selection().Add(h)
	xfeed(s, "EXPLODE")
	if len(entities[*doc.Dimension](s)) != 0 {
		t.Fatalf("the dimension is still there\n%s", history(s))
	}
	ls, pls, ts := entities[*doc.Line](s), entities[*doc.LWPolyline](s), entities[*doc.Text](s)
	if len(ls) != 3 || len(pls) != 2 || len(ts) != 1 {
		t.Fatalf("parts: %d lines, %d polylines, %d texts", len(ls), len(pls), len(ts))
	}
	if !hasLine(ls, geom.V(0, -200), geom.V(1000, -200)) || !hasLine(ls, geom.V(0, -6.25), geom.V(0, -212.5)) {
		t.Fatalf("lines = %v", dumpLines(ls))
	}
	if ts[0].Value != "1000" || !ts[0].Anchor().Near(geom.V(500, -181.25), 1e-9) || ts[0].Color != 3 || ts[0].Height != 25 {
		t.Fatalf("text = %+v", ts[0])
	}
	if !pls[0].Closed || len(pls[0].Points) != 3 || pls[0].Points[0] != geom.V(0, -200) {
		t.Fatalf("arrowhead = %+v", pls[0])
	}
	wantHistory(t, s, "1 object exploded.")
}

func TestListDimension(t *testing.T) {
	s := newSession(t)
	d := testDim()
	d.Text = "<> mm"
	addDim(t, s, d)
	s.Script("LIST ALL ")
	wantHistory(t, s, `DIMENSION  Layer: "0"`, "type: horizontal",
		"1st extension defpoint: X=  0.0000  Y=  0.0000  Z=  0.0000",
		"2nd extension defpoint: X=1000.0000  Y=  0.0000  Z=  0.0000",
		"dimension line defpoint: X=1000.0000  Y=-200.0000  Z=  0.0000",
		"default text position: X=500.0000  Y=-181.2500  Z=  0.0000",
		"measurement 1000.0000", `dimension text "<> mm"`, `dimension style: "ISO-25"`)

	s2 := newSession(t)
	a := &doc.Dimension{Kind: doc.DimArcLength, Center: geom.V(0, 0), Radius: 10, StartAngle: 0, EndAngle: math.Pi / 2,
		DimLine: geom.V(0, 15)}
	a.Recompute()
	addDim(t, s2, a)
	s2.Script("LIST ALL ")
	wantHistory(t, s2, "type: arc length", "center point, X=  0.0000", "radius 10.0000", "measurement 15.7080", "default text")
}

func TestMatchpropOntoDimension(t *testing.T) {
	s := newSession(t)
	s.Drawing().AddLayer("Dims", 2)
	src := line(geom.V(2000, 0), geom.V(2000, 100))
	src.Layer, src.Color = "Dims", 5
	addEntities(t, s, src)
	h := addDim(t, s, testDim())
	s.Script("MATCHPROP\n2000,50\n500,-200\n\n")
	if d := dimOf(t, s, h); d.Layer != "Dims" || d.Color != 5 {
		t.Fatalf("dimension = layer %q colour %d\n%s", d.Layer, d.Color, history(s))
	}
}

// TestExplodeDimensionKeepsPartColours checks the pieces take the style's DIMCLRE (extension
// lines), DIMCLRD (dimension line and arrowheads) and DIMCLRT (text), a ByBlock one being the
// dimension's own colour.
func TestExplodeDimensionKeepsPartColours(t *testing.T) {
	s := newSession(t)
	d := testDim()
	st := d.Style()
	st.ExtColor, st.LineColor, st.TextColor = 1, doc.ByBlock, 5
	d.SetStyle(nil, st)
	d.Recompute()
	d.Color = 3
	h := addDim(t, s, d)
	s.Selection().Add(h)
	xfeed(s, "EXPLODE")
	ls, pls, ts := entities[*doc.Line](s), entities[*doc.LWPolyline](s), entities[*doc.Text](s)
	if len(ls) != 3 || len(pls) != 2 || len(ts) != 1 {
		t.Fatalf("parts: %d lines, %d polylines, %d texts", len(ls), len(pls), len(ts))
	}
	for _, l := range ls {
		want := 1
		if l.Start.Y == l.End.Y {
			want = 3
		}
		if l.Color != want {
			t.Errorf("line %v-%v colour %d, want %d", l.Start, l.End, l.Color, want)
		}
	}
	for _, p := range pls {
		if p.Color != 3 {
			t.Errorf("arrowhead colour %d, want 3", p.Color)
		}
	}
	if ts[0].Color != 5 {
		t.Errorf("text colour %d, want 5", ts[0].Color)
	}
}

// TestMatchpropCopiesDimensionStyle checks MATCHPROP's Dimension setting, on by default as in
// AutoCAD, copies a dimension's style and overrides onto a dimension, and that switching it off
// leaves them alone.
func TestMatchpropCopiesDimensionStyle(t *testing.T) {
	s := newSession(t)
	d := s.Drawing()
	arch := doc.DefaultDimStyle()
	arch.Name, arch.TextHeight = "ARCH", 5
	tx, _ := d.Begin()
	if err := tx.SetDimStyle(arch); err != nil {
		t.Fatal(err)
	}
	tx.Commit()
	src := testDim()
	st := arch
	st.ArrowSize = 7
	src.SetStyle(d, st)
	src.Recompute()
	addDim(t, s, src)
	h := addDim(t, s, &doc.Dimension{Kind: doc.DimRotated, Ext1: geom.V(0, 1000), Ext2: geom.V(1000, 1000), DimLine: geom.V(1000, 1200)})
	s.Script("MATCHPROP\n500,-200\n500,1200\n\n")
	if got := dimOf(t, s, h); got.StyleName != "ARCH" || got.Overrides != src.Overrides || got.Style().ArrowSize != 7 {
		t.Fatalf("target style %q overrides %+v\n%s", got.StyleName, got.Overrides.Set, history(s))
	}
	if !strings.Contains(history(s), "Dimension") {
		t.Fatalf("settings line does not name Dimension\n%s", history(s))
	}
	d.Undo()
	if got := dimOf(t, s, h); got.StyleName != "" || got.Overrides.Set != 0 {
		t.Fatalf("undo: %q", got.StyleName)
	}
	s.Script("MATCHPROP\n500,-200\nS\nD\n\n500,1200\n\n")
	if got := dimOf(t, s, h); got.StyleName != "" || got.Overrides.Set != 0 {
		t.Fatalf("Dimension off still copied %q\n%s", got.StyleName, history(s))
	}
}
