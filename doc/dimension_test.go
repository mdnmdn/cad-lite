package doc_test

import (
	"math"
	"testing"

	"cad-lite/doc"
	"cad-lite/geom"
)

func newDim(k doc.DimKind, a, b, dl geom.Vec2, rot float64) *doc.Dimension {
	d := &doc.Dimension{Kind: k, Ext1: a, Ext2: b, DimLine: dl, Rotation: rot, Style: doc.DefaultDimStyle()}
	d.Recompute()
	return d
}

func deg(d float64) float64 { return d * math.Pi / 180 }

func TestDimensionMeasurement(t *testing.T) {
	h := newDim(doc.DimRotated, geom.V(0, 0), geom.V(120, 50), geom.V(60, 70), 0)
	if m := h.Measurement(); m != 120 {
		t.Fatalf("horizontal = %v", m)
	}
	v := newDim(doc.DimRotated, geom.V(0, 0), geom.V(120, 50), geom.V(130, 0), math.Pi/2)
	if m := v.Measurement(); math.Abs(m-50) > 1e-9 {
		t.Fatalf("vertical = %v", m)
	}
	a := newDim(doc.DimAligned, geom.V(0, 0), geom.V(30, 40), geom.V(-8, 6), 0)
	if m := a.Measurement(); math.Abs(m-50) > 1e-12 {
		t.Fatalf("aligned = %v", m)
	}
	nearV(t, "aligned direction", a.Direction(), geom.V(0.6, 0.8), 1e-12)
	if d := (&doc.Dimension{Kind: doc.DimAligned}).Direction(); d != geom.V(1, 0) {
		t.Fatalf("degenerate direction = %v", d)
	}
}

func TestDimensionText(t *testing.T) {
	cases := []struct {
		length float64
		mod    func(*doc.DimStyle)
		over   string
		want   string
	}{
		{120, nil, "", "120"},
		{12.5, nil, "", "12,5"},
		{12.345, nil, "", "12,35"},
		{12.004, nil, "", "12"},
		{0.5, nil, "", "0,5"},
		{0.5, func(s *doc.DimStyle) { s.ZeroSupp = 12 }, "", ",5"},
		{12.5, func(s *doc.DimStyle) { s.ZeroSupp = 0 }, "", "12,50"},
		{12.5, func(s *doc.DimStyle) { s.Decimals = 3; s.DecSep = "."; s.ZeroSupp = 0 }, "", "12.500"},
		{12.5, func(s *doc.DimStyle) { s.Decimals = 0 }, "", "12"},
		{120, nil, "L=<> mm", "L=120 mm"},
		{120, nil, "fixed", "fixed"},
		{120, nil, " ", ""},
	}
	for _, c := range cases {
		d := newDim(doc.DimRotated, geom.V(0, 0), geom.V(c.length, 0), geom.V(0, 10), 0)
		if c.mod != nil {
			c.mod(&d.Style)
		}
		d.Text = c.over
		if got := d.MeasurementText(); got != c.want {
			t.Errorf("%v %q: got %q, want %q", c.length, c.over, got, c.want)
		}
	}
}

func TestDimensionLayoutHorizontal(t *testing.T) {
	d := newDim(doc.DimRotated, geom.V(0, 0), geom.V(120, 0), geom.V(40, 15), 0)
	nearV(t, "DimLine on the Ext2 end", d.DimLine, geom.V(120, 15), 1e-12)
	l := d.Layout()
	nearV(t, "ext1 start", l.ExtLines[0].A, geom.V(0, 0.625), 1e-12)
	nearV(t, "ext1 end", l.ExtLines[0].B, geom.V(0, 16.25), 1e-12)
	nearV(t, "ext2 start", l.ExtLines[1].A, geom.V(120, 0.625), 1e-12)
	nearV(t, "tip1", l.ArrowTips[0], geom.V(0, 15), 1e-12)
	nearV(t, "tip2", l.ArrowTips[1], geom.V(120, 15), 1e-12)
	if l.Flipped || len(l.DimLine) != 1 {
		t.Fatalf("flipped %v, %d dim lines", l.Flipped, len(l.DimLine))
	}
	// Arrows point outward: tip at the extension line, base 2.5 inside, 1/3 as wide as long.
	a := l.Arrows[0]
	nearV(t, "arrow tip", a[0], geom.V(0, 15), 1e-12)
	nearV(t, "arrow base", a[1].Lerp(a[2], 0.5), geom.V(2.5, 15), 1e-12)
	if w := a[1].Dist(a[2]); math.Abs(w-2.5/3) > 1e-12 {
		t.Fatalf("arrow width = %v", w)
	}
	nearV(t, "arrow2 base", l.Arrows[1][1].Lerp(l.Arrows[1][2], 0.5), geom.V(117.5, 15), 1e-12)
	// Text: middle at the line's middle, Gap + half the height above it.
	nearV(t, "text pos", d.TextPos, geom.V(60, 15+0.625+1.25), 1e-12)
	if l.Text.Value != "120" || l.Text.Rotation != 0 || l.Text.Height != 2.5 {
		t.Fatalf("text = %q rot %v h %v", l.Text.Value, l.Text.Rotation, l.Text.Height)
	}
	box := geom.BoxOf(l.TextBox[:]...)
	// The box runs from the descenders; the baseline sits Gap above the line.
	if base := box.Min.Y + 2.5*7/21; math.Abs(base-15.625) > 1e-9 || math.Abs(box.Center().X-60) > 1e-9 {
		t.Fatalf("text box %+v: baseline %v, want Gap above the line", box, base)
	}
	// Lines are ext lines, the dim line and two arrows; Geometry adds fills and text.
	if n := len(d.Lines()); n != 5 {
		t.Fatalf("%d lines", n)
	}
	if len(d.Geometry()) <= len(d.Lines())+10 {
		t.Fatalf("geometry has %d curves", len(d.Geometry()))
	}
	b := d.Bounds()
	if !b.Contains(geom.V(0, 0)) || !b.Contains(geom.V(60, 18)) {
		t.Fatalf("bounds %+v", b)
	}
	if got := d.DefPoints(); len(got) != 4 || got[0] != d.Ext1 || got[3] != d.TextPos {
		t.Fatalf("defpoints %v", got)
	}
}

func TestDimensionLayoutDimLineBelow(t *testing.T) {
	// A dimension line below its origins: extension lines run downward.
	d := newDim(doc.DimRotated, geom.V(0, 0), geom.V(50, 0), geom.V(0, -10), 0)
	l := d.Layout()
	nearV(t, "ext start", l.ExtLines[0].A, geom.V(0, -0.625), 1e-12)
	nearV(t, "ext end", l.ExtLines[0].B, geom.V(0, -11.25), 1e-12)
	// The text still sits above the line (reading sense).
	if d.TextPos.Y <= -10 {
		t.Fatalf("text at %v", d.TextPos)
	}
}

func TestDimensionReadableText(t *testing.T) {
	for _, a := range []float64{30, 90, 150, 210, 270, 330} {
		dir := geom.V(math.Cos(deg(a)), math.Sin(deg(a)))
		d := newDim(doc.DimAligned, geom.V(0, 0), dir.Scale(60), dir.Perp().Scale(10), 0)
		r := d.Layout().Text.Rotation // TextLayout keeps [0, 2π); Text keeps what was set
		if r <= -math.Pi/2-1e-9 || r > math.Pi/2+1e-9 {
			t.Errorf("%v°: text rotation %v not readable", a, r*180/math.Pi)
		}
		// Up for the readable text: the text is on its left.
		u := geom.V(math.Cos(r), math.Sin(r))
		mid := d.Layout().ArrowTips[0].Lerp(d.Layout().ArrowTips[1], 0.5)
		if off := d.TextPos.Sub(mid); math.Abs(off.Dot(u.Perp())-1.875) > 1e-9 {
			t.Errorf("%v°: text offset %v", a, off)
		}
	}
}

func TestDimensionTinyFlipsArrows(t *testing.T) {
	d := newDim(doc.DimRotated, geom.V(0, 0), geom.V(4, 0), geom.V(2, 10), 0)
	l := d.Layout()
	if !l.Flipped || len(l.DimLine) != 3 {
		t.Fatalf("flipped %v, %d lines", l.Flipped, len(l.DimLine))
	}
	nearV(t, "arrow1 base outside", l.Arrows[0][1].Lerp(l.Arrows[0][2], 0.5), geom.V(-2.5, 10), 1e-12)
	nearV(t, "arrow2 base outside", l.Arrows[1][1].Lerp(l.Arrows[1][2], 0.5), geom.V(6.5, 10), 1e-12)
	nearV(t, "stub1 end", l.DimLine[1].B, geom.V(-5, 10), 1e-12)
	nearV(t, "stub2 end", l.DimLine[2].B, geom.V(9, 10), 1e-12)
}

func TestDimensionScaleStyle(t *testing.T) {
	d := newDim(doc.DimRotated, geom.V(0, 0), geom.V(100, 0), geom.V(0, 20), 0)
	d.Style.Scale = 2
	d.Recompute()
	l := d.Layout()
	if l.Text.Height != 5 {
		t.Fatalf("height %v", l.Text.Height)
	}
	nearV(t, "ext start", l.ExtLines[0].A, geom.V(0, 1.25), 1e-12)
	nearV(t, "text pos", d.TextPos, geom.V(50, 20+1.25+2.5), 1e-12)
}

func TestDimensionTextMoved(t *testing.T) {
	d := newDim(doc.DimRotated, geom.V(0, 0), geom.V(100, 0), geom.V(0, 20), 0)
	d.TextPos, d.TextMoved = geom.V(130, 40), true
	d.Recompute()
	nearV(t, "kept", d.TextPos, geom.V(130, 40), 0)
	d.Transform(geom.Translate(geom.V(1, 1)))
	nearV(t, "moved with the dimension", d.TextPos, geom.V(131, 41), 1e-12)
}

func TestDimensionTransform(t *testing.T) {
	base := func() *doc.Dimension {
		return newDim(doc.DimRotated, geom.V(0, 0), geom.V(100, 0), geom.V(0, 20), 0)
	}
	d := base()
	d.Transform(geom.Translate(geom.V(10, 5)))
	nearV(t, "move ext1", d.Ext1, geom.V(10, 5), 1e-12)
	nearV(t, "move text", d.TextPos, geom.V(60, 26.875), 1e-12)

	d = base()
	d.Transform(geom.Rotate(geom.V(0, 0), math.Pi/2))
	if math.Abs(d.Rotation-math.Pi/2) > 1e-12 || math.Abs(d.Measurement()-100) > 1e-9 {
		t.Fatalf("rotated: rotation %v measurement %v", d.Rotation, d.Measurement())
	}
	nearV(t, "rotated text left of the line", d.TextPos, geom.V(-21.875, 50), 1e-9)

	d = base()
	d.Transform(geom.ScaleAbout(geom.V(0, 0), 2))
	if d.Measurement() != 200 || d.MeasurementText() != "200" || d.Layout().Text.Height != 2.5 {
		t.Fatalf("scaled: %v %q h %v", d.Measurement(), d.MeasurementText(), d.Layout().Text.Height)
	}

	// Mirror about the X axis: the dimension line goes below, the text stays readable and above
	// its line.
	d = base()
	d.Transform(geom.Mat3{1, 0, 0, -1, 0, 0})
	l := d.Layout()
	if l.Text.Rotation != 0 || d.TextPos.Y <= -20 || math.Abs(d.TextPos.Y-(-20+1.875)) > 1e-9 {
		t.Fatalf("mirrored: rotation %v text %v", l.Text.Rotation, d.TextPos)
	}
	// Mirror about the Y axis: the text reads left to right.
	d = base()
	d.Transform(geom.Mat3{-1, 0, 0, 1, 0, 0})
	if r := d.Layout().Text.Rotation; math.Abs(r) > 1e-9 {
		t.Fatalf("mirrored about Y: rotation %v", r)
	}
	if d.Measurement() != 100 {
		t.Fatalf("mirrored measurement %v", d.Measurement())
	}
}

func TestDimensionClone(t *testing.T) {
	d := newDim(doc.DimAligned, geom.V(0, 0), geom.V(10, 0), geom.V(0, 5), 0)
	d.H, d.Layer = 7, "DIM"
	_ = d.Layout()
	c := d.Clone().(*doc.Dimension)
	c.Ext2, c.Style.TextHeight, c.Text = geom.V(20, 0), 5, "x<>"
	c.Recompute()
	if d.Ext2 != geom.V(10, 0) || d.Style.TextHeight != 2.5 || d.Text != "" || d.MeasurementText() != "10" {
		t.Fatalf("original changed: %+v", d)
	}
	if c.MeasurementText() != "x20" || c.H != 7 || c.Layer != "DIM" {
		t.Fatalf("clone: %q %v %q", c.MeasurementText(), c.H, c.Layer)
	}
	if d.Layout().Text.Value != "10" || c.Layout().Text.Height != 5 {
		t.Fatal("layouts shared")
	}
}

func TestDimensionLayoutCache(t *testing.T) {
	d := newDim(doc.DimRotated, geom.V(0, 0), geom.V(10, 0), geom.V(0, 5), 0)
	g := d.Geometry()
	if &d.Geometry()[0] != &g[0] {
		t.Fatal("geometry not cached")
	}
	d.Ext2 = geom.V(20, 0)
	if d.Layout().Text.Value != "20" {
		t.Fatal("cache not invalidated")
	}
}

func newArcDim(r, s, e, dl float64) *doc.Dimension {
	mid := s + geom.NormAngle(e-s)/2
	d := &doc.Dimension{Kind: doc.DimArcLength, Radius: r, StartAngle: s, EndAngle: e,
		DimLine: geom.V(math.Cos(mid), math.Sin(mid)).Scale(dl), Style: doc.DefaultDimStyle()}
	d.Recompute()
	return d
}

func TestArcDimensionMeasurement(t *testing.T) {
	d := newArcDim(40, deg(30), deg(90), 50)
	if m := d.Measurement(); math.Abs(m-40*math.Pi/3) > 1e-12 {
		t.Fatalf("measurement %v", m)
	}
	if d.MeasurementText() != "41,89" {
		t.Fatalf("text %q", d.MeasurementText())
	}
	nearV(t, "ext1 on the arc start", d.Ext1, geom.V(40*math.Cos(deg(30)), 40*math.Sin(deg(30))), 1e-12)
	nearV(t, "ext2 on the arc end", d.Ext2, geom.V(0, 40), 1e-12)
	if len(d.DefPoints()) != 5 {
		t.Fatalf("defpoints %v", d.DefPoints())
	}
	// Across 0°.
	if m := newArcDim(10, deg(350), deg(10), 15).Measurement(); math.Abs(m-10*deg(20)) > 1e-12 {
		t.Fatalf("across zero %v", m)
	}
	// Partial, given in either order.
	p := newArcDim(40, 0, math.Pi, 50)
	p.Partial, p.Ext1, p.Ext2 = true, geom.V(-20, 20*math.Sqrt(3)), geom.V(40, 0)
	p.Recompute()
	if m := p.Measurement(); math.Abs(m-40*deg(120)) > 1e-9 {
		t.Fatalf("partial %v", m)
	}
	nearV(t, "partial keeps ext1", p.Ext1, geom.V(-20, 20*math.Sqrt(3)), 0)
}

func TestArcDimensionLayout(t *testing.T) {
	// Under 90°: extension lines parallel to the bisector (here the Y axis).
	d := newArcDim(40, deg(60), deg(120), 50)
	l := d.Layout()
	if math.Abs(l.DimArc.R-50) > 1e-12 {
		t.Fatalf("dim arc radius %v", l.DimArc.R)
	}
	for i, e := range l.ExtLines {
		if math.Abs(e.A.X-e.B.X) > 1e-9 || e.B.Y <= e.A.Y {
			t.Errorf("ext %d not parallel upward: %+v", i, e)
		}
		if math.Abs(l.ArrowTips[i].Len()-50) > 1e-9 {
			t.Errorf("tip %d off the dim arc", i)
		}
	}
	nearV(t, "ext1 start", l.ExtLines[0].A, geom.V(20, 20*math.Sqrt(3)+0.625), 1e-9)
	// Arrow 1 points clockwise along the arc's tangent at its tip.
	a := l.Arrows[0]
	tan := a[0].Sub(a[1].Lerp(a[2], 0.5)).Norm()
	if want := geom.V(math.Cos(l.DimArc.Start), math.Sin(l.DimArc.Start)).Perp().Scale(-1); !tan.Near(want, 1e-9) {
		t.Fatalf("arrow direction %v, want %v", tan, want)
	}
	// Text: horizontal at the top of the dim arc, above it, after the symbol.
	if l.Text.Rotation != 0 || d.TextPos.Y < 50 || l.Symbol.R == 0 {
		t.Fatalf("text rot %v pos %v symbol %+v", l.Text.Rotation, d.TextPos, l.Symbol)
	}
	if l.Symbol.C.X >= l.Text.Align.X {
		t.Fatal("symbol not before the text")
	}

	// 90° and more: radial extension lines.
	r := newArcDim(40, 0, deg(150), 50).Layout()
	for i, e := range r.ExtLines {
		if math.Abs(e.A.Cross(e.B)) > 1e-9 {
			t.Errorf("ext %d not radial: %+v", i, e)
		}
	}
	// A dimension arc inside the arc: extension lines run inward.
	in := newArcDim(40, deg(60), deg(120), 30).Layout()
	if in.ExtLines[0].B.Y >= in.ExtLines[0].A.Y {
		t.Fatalf("inner ext %+v", in.ExtLines[0])
	}
	// No symbol.
	n := newArcDim(40, deg(60), deg(120), 50)
	n.ArcSymbol = doc.DimArcSymNone
	if n.Layout().Symbol.R != 0 {
		t.Fatal("symbol drawn")
	}
	// A bottom arc keeps its text readable.
	if b := newArcDim(40, deg(200), deg(340), 50).Layout().Text.Rotation; math.Abs(b) > 1e-9 {
		t.Fatalf("bottom rotation %v", b)
	}
}

func TestArcDimensionTransform(t *testing.T) {
	d := newArcDim(40, deg(30), deg(90), 50)
	d.Transform(geom.Mat3{-1, 0, 0, 1, 0, 0})
	if math.Abs(d.Measurement()-40*math.Pi/3) > 1e-9 {
		t.Fatalf("mirrored measurement %v", d.Measurement())
	}
	if math.Abs(d.StartAngle-deg(90)) > 1e-9 || math.Abs(d.EndAngle-deg(150)) > 1e-9 {
		t.Fatalf("mirrored arc %v..%v", d.StartAngle*180/math.Pi, d.EndAngle*180/math.Pi)
	}
	nearV(t, "ext1", d.Ext1, geom.V(0, 40), 1e-9)
	r := d.Layout().Text.Rotation
	if r <= -math.Pi/2 || r > math.Pi/2 {
		t.Fatalf("mirrored text rotation %v", r)
	}
	d.Transform(geom.ScaleAbout(geom.V(0, 0), 2))
	if d.Radius != 80 || math.Abs(d.Measurement()-80*math.Pi/3) > 1e-9 {
		t.Fatalf("scaled radius %v", d.Radius)
	}
}
