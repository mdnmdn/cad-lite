package doc_test

import (
	"math"
	"testing"

	"cad-lite/doc"
	"cad-lite/geom"
)

func nearV(t *testing.T, name string, got, want geom.Vec2, tol float64) {
	t.Helper()
	if !got.Near(want, tol) {
		t.Fatalf("%s = %v, want %v", name, got, want)
	}
}

func justified(h doc.TextHAlign, v doc.TextVAlign, p geom.Vec2) *doc.Text {
	t := doc.NewText(p, 2, 0, "Agy 12")
	t.HAlign, t.VAlign, t.Align = h, v, p
	if h == doc.TextAligned || h == doc.TextFit {
		t.Insert, t.Align = p, p.Add(geom.V(20, 0))
	}
	return t
}

func TestTextLeftBaseline(t *testing.T) {
	tx := doc.NewText(geom.V(10, 5), 2, 0, "HI")
	lay := tx.Layout()
	nearV(t, "origin", lay.Origin, geom.V(10, 5), 1e-12)
	if w := doc.TextAdvance("HI") * 2; math.Abs(lay.Width-w) > 1e-12 || w <= 0 {
		t.Fatalf("width = %v, want %v", lay.Width, w)
	}
	// The bounds are the text box, from the descenders to the capitals over the advance width.
	if b := tx.Bounds(); !b.Min.Near(geom.V(10, 5-2.0/3), 1e-9) || !b.Max.Near(geom.V(10+lay.Width, 7), 1e-9) {
		t.Fatalf("bounds = %+v", b)
	}
	if len(tx.Geometry()) != 4 { // H: 3 strokes, I: 1
		t.Fatalf("%d strokes", len(tx.Geometry()))
	}
	if tx.Anchor() != geom.V(10, 5) || tx.Class() != "TEXT" {
		t.Fatal("anchor or class")
	}
}

func TestTextJustifications(t *testing.T) {
	p := geom.V(100, 50)
	w := doc.TextAdvance("Agy 12") * 2
	desc := 2.0 / 3 // the descent is a third of the height
	cases := []struct {
		name string
		h    doc.TextHAlign
		v    doc.TextVAlign
		// at is the point of the layout that must land on p.
		at func(l doc.TextLayout) geom.Vec2
	}{
		{"Center", doc.TextCenter, doc.TextBaseline, func(l doc.TextLayout) geom.Vec2 { return l.Origin.Add(geom.V(w/2, 0)) }},
		{"Right", doc.TextRight, doc.TextBaseline, func(l doc.TextLayout) geom.Vec2 { return l.Origin.Add(geom.V(w, 0)) }},
		{"TL", doc.TextLeft, doc.TextTop, func(l doc.TextLayout) geom.Vec2 { return l.Corners[3] }},
		{"TR", doc.TextRight, doc.TextTop, func(l doc.TextLayout) geom.Vec2 { return l.Corners[2] }},
		{"BL", doc.TextLeft, doc.TextBottom, func(l doc.TextLayout) geom.Vec2 { return l.Corners[0] }},
		{"BR", doc.TextRight, doc.TextBottom, func(l doc.TextLayout) geom.Vec2 { return l.Corners[1] }},
		{"MC", doc.TextCenter, doc.TextVMiddle, func(l doc.TextLayout) geom.Vec2 { return l.Origin.Add(geom.V(w/2, 1)) }},
		{"ML", doc.TextLeft, doc.TextVMiddle, func(l doc.TextLayout) geom.Vec2 { return l.Origin.Add(geom.V(0, 1)) }},
	}
	for _, c := range cases {
		tx := justified(c.h, c.v, p)
		if tx.Justify() != c.name {
			t.Fatalf("Justify = %q, want %q", tx.Justify(), c.name)
		}
		nearV(t, c.name, c.at(tx.Layout()), p, 1e-9)
		if tx.Anchor() != p {
			t.Fatalf("%s anchor = %v", c.name, tx.Anchor())
		}
	}
	if l := justified(doc.TextLeft, doc.TextBottom, p).Layout(); math.Abs(l.Origin.Y-(p.Y+desc)) > 1e-9 {
		t.Fatalf("bottom origin = %v", l.Origin)
	}
	// Middle centres the ink.
	m := justified(doc.TextMiddle, doc.TextBaseline, p)
	nearV(t, "middle", m.Bounds().Center(), p, 0.15)
	if c := m.Bounds().Center(); math.Abs(c.Y-p.Y) > 1e-9 {
		t.Fatalf("middle ink centre y = %v", c.Y)
	}
	// Aligned scales the height to the baseline, Fit the width factor.
	a := justified(doc.TextAligned, doc.TextBaseline, p).Layout()
	if math.Abs(a.Width-20) > 1e-9 || math.Abs(a.Height-20/doc.TextAdvance("Agy 12")) > 1e-9 || a.WidthFactor != 1 {
		t.Fatalf("aligned = %+v", a)
	}
	f := justified(doc.TextFit, doc.TextBaseline, p).Layout()
	if math.Abs(f.Width-20) > 1e-9 || f.Height != 2 || math.Abs(f.WidthFactor-20/w) > 1e-9 {
		t.Fatalf("fit = %+v", f)
	}
}

func TestJustifyNamesRoundTrip(t *testing.T) {
	for _, n := range []string{"Left", "Center", "Right", "Align", "Middle", "Fit", "TL", "TC", "TR", "ML", "MC", "MR", "BL", "BC", "BR"} {
		h, v, ok := doc.ParseJustify(n)
		if !ok || doc.JustifyName(h, v) != n {
			t.Fatalf("%s: %v %v %v -> %q", n, h, v, ok, doc.JustifyName(h, v))
		}
	}
	if _, _, ok := doc.ParseJustify("XX"); ok {
		t.Fatal("XX parsed")
	}
}

// strokes returns every stroke point of e.
func strokes(e doc.Entity) []geom.Vec2 {
	var out []geom.Vec2
	for _, c := range e.Geometry() {
		out = append(out, c.(geom.Polyline).Pts...)
	}
	return out
}

// TestTextTransformMatchesGeometry checks that transforming a text moves its strokes exactly as
// transforming the strokes would, for every kind of transform the editing commands use and every
// justification, generation flags and an oblique angle included.
func TestTextTransformMatchesGeometry(t *testing.T) {
	mirror := geom.Translate(geom.V(3, 1)).Mul(geom.Mat3{math.Cos(1), math.Sin(1), math.Sin(1), -math.Cos(1), 0, 0})
	ms := map[string]geom.Mat3{
		"move":   geom.Translate(geom.V(7, -3)),
		"rotate": geom.Rotate(geom.V(1, 2), 0.7),
		"scale":  geom.ScaleAbout(geom.V(-4, 2), 2.5),
		"mirror": mirror,
	}
	for name, m := range ms {
		for _, j := range []string{"Left", "Center", "Right", "Align", "Middle", "Fit", "TL", "MC", "BR"} {
			h, v, _ := doc.ParseJustify(j)
			tx := justified(h, v, geom.V(5, 5))
			tx.Rotation, tx.Oblique, tx.WidthFactor, tx.Generation = 0.3, 0.2, 0.8, doc.TextBackward
			want := strokes(tx)
			for i := range want {
				want[i] = m.Apply(want[i])
			}
			cp := tx.Clone().(*doc.Text)
			cp.Transform(m)
			got := strokes(cp)
			if len(got) != len(want) {
				t.Fatalf("%s %s: %d points, want %d", name, j, len(got), len(want))
			}
			for i := range got {
				nearV(t, name+" "+j, got[i], want[i], 1e-9)
			}
			if len(strokes(tx)) != len(want) || tx.Rotation != 0.3 {
				t.Fatal("the clone shares state with the original")
			}
		}
	}
}

// TestMirrorReadable checks MIRRTEXT 0: the text keeps reading normally and its box lands on the
// mirrored box.
func TestMirrorReadable(t *testing.T) {
	across := geom.Mat3{-1, 0, 0, 1, 20, 0} // x -> 20-x: a vertical mirror line at x = 10
	along := geom.Mat3{1, 0, 0, -1, 0, 10}  // y -> 10-y: a horizontal mirror line at y = 5
	for _, j := range []string{"Left", "Center", "Right", "TL", "MC", "BR", "Middle", "Align", "Fit"} {
		h, v, _ := doc.ParseJustify(j)
		tx := justified(h, v, geom.V(2, 1))
		box := tx.Layout().Corners

		a := tx.Clone().(*doc.Text)
		a.MirrorReadable(across)
		if a.Generation != 0 {
			t.Fatalf("%s across: generation %d", j, a.Generation)
		}
		if d := a.Layout().Dir(); d.X < 0.999 {
			t.Fatalf("%s across: reads along %v", j, d)
		}
		ac := a.Layout().Corners
		got := geom.BoxOf(ac[:]...)
		want := geom.BoxOf(across.Apply(box[0]), across.Apply(box[1]), across.Apply(box[2]), across.Apply(box[3]))
		nearV(t, j+" across min", got.Min, want.Min, 1e-9)
		nearV(t, j+" across max", got.Max, want.Max, 1e-9)

		b := tx.Clone().(*doc.Text)
		b.MirrorReadable(along)
		if b.Generation != 0 || b.Layout().Dir().X < 0.999 {
			t.Fatalf("%s along: generation %d, dir %v", j, b.Generation, b.Layout().Dir())
		}
		bc := b.Layout().Corners
		got = geom.BoxOf(bc[:]...)
		want = geom.BoxOf(along.Apply(box[0]), along.Apply(box[1]), along.Apply(box[2]), along.Apply(box[3]))
		nearV(t, j+" along min", got.Min, want.Min, 1e-9)
		nearV(t, j+" along max", got.Max, want.Max, 1e-9)
	}
	// A plain Transform mirrors for real.
	tx := doc.NewText(geom.V(0, 0), 1, 0, "A")
	tx.Transform(across)
	if tx.Generation != doc.TextUpsideDown {
		t.Fatalf("generation = %d", tx.Generation)
	}
}

func TestTextCacheFollowsFields(t *testing.T) {
	tx := doc.NewText(geom.V(0, 0), 1, 0, "I")
	n := len(tx.Geometry())
	cp := tx.Clone().(*doc.Text)
	cp.Value = "HH"
	if len(cp.Geometry()) == n || len(tx.Geometry()) != n {
		t.Fatal("geometry did not follow the value")
	}
	tx.Height = 3
	if b := tx.Bounds(); math.Abs(b.Max.Y-3) > 1e-9 {
		t.Fatalf("bounds after height change = %+v", b)
	}
	empty := doc.NewText(geom.V(4, 4), 1, 0, "  ")
	if b := empty.Bounds(); !b.Contains(geom.V(4, 4)) || !b.Contains(geom.V(4+doc.TextAdvance("  "), 5)) || len(empty.Geometry()) != 0 {
		t.Fatalf("blank text bounds = %+v", b)
	}
	if !doc.NewText(geom.V(0, 0), 1, 0, "WWW").HitBox(geom.V(1, 0.5)) || doc.NewText(geom.V(0, 0), 1, 0, "W").HitBox(geom.V(5, 0.5)) {
		t.Fatal("HitBox")
	}
}

func TestTextControlCodes(t *testing.T) {
	adv := doc.TextAdvance
	if adv("%%%") != adv("%") || adv("%%d") <= 0 || adv("%%d") == adv("?") || adv("%%c") == adv("?") || adv("%%p") == adv("?") {
		t.Fatal("%% codes")
	}
	if adv("%%uAB%%u") != adv("AB") || adv("%%065") != adv("A") {
		t.Fatal("underline or %%nnn")
	}
	if len(doc.NewText(geom.V(0, 0), 1, 0, "%%uAB").Geometry()) != len(doc.NewText(geom.V(0, 0), 1, 0, "AB").Geometry())+1 {
		t.Fatal("no underline stroke")
	}
	// An accented letter is its base letter with a mark; \U+ escapes decode.
	e, plain := doc.NewText(geom.V(0, 0), 1, 0, "è"), doc.NewText(geom.V(0, 0), 1, 0, "e")
	if adv("è") != adv("e") || len(e.Geometry()) != len(plain.Geometry())+1 || adv(`\U+00E8`) != adv("e") {
		t.Fatal("accent")
	}
	if adv("中") != adv("?") {
		t.Fatal("a missing glyph is not drawn as ?")
	}
}

func TestTextIsJournalled(t *testing.T) {
	d := doc.New()
	tx, _ := d.Begin()
	h := tx.Add(doc.NewText(geom.V(0, 0), 1, 0, "A"))
	tx.Commit()
	tx, _ = d.Begin()
	tx.Modify(h, func(e doc.Entity) { e.(*doc.Text).Value = "B" })
	tx.Commit()
	if d.Entity(h).(*doc.Text).Value != "B" {
		t.Fatal("modify")
	}
	d.Undo()
	if d.Entity(h).(*doc.Text).Value != "A" {
		t.Fatal("undo")
	}
	if d.Entity(h).Common().Layer != "0" || d.Entity(h).Common().Color != doc.ByLayer {
		t.Fatal("defaults")
	}
}

// TestObliqueIgnoresWidthFactor: the obliquing angle is the slant of the drawn strokes, whatever
// the width factor.
func TestObliqueIgnoresWidthFactor(t *testing.T) {
	for _, wf := range []float64{0.5, 1, 2} {
		tx := doc.NewText(geom.V(0, 0), 10, 0, "I")
		tx.WidthFactor, tx.Oblique = wf, math.Pi/4
		pts := tx.Geometry()[0].(geom.Polyline).Pts
		d := pts[len(pts)-1].Sub(pts[0])
		if math.Abs(math.Abs(d.X)-math.Abs(d.Y)) > 1e-9 {
			t.Fatalf("wf %v: stroke %v is not at 45 degrees", wf, d)
		}
	}
}

func TestDeleteCharacterIsNotADegreeSign(t *testing.T) {
	if doc.TextAdvance("\x7f") != doc.TextAdvance("?") {
		t.Fatal("U+007F does not draw as ?")
	}
}

func TestTextStyles(t *testing.T) {
	d := doc.New()
	d.Preserved.Tables = map[string][]doc.RawRecord{"STYLE": {{Pairs: []doc.Pair{{Code: 0, Value: "STYLE"}, {Code: 2, Value: "NOTES"}}}}}
	tx, _ := d.Begin()
	m := doc.NewMText(geom.V(0, 0), 1, 0, "x")
	m.Style = "Titles"
	tx.Add(m)
	tx.Commit()
	if got := doc.TextStyles(d); len(got) != 3 || got[0] != "Standard" || got[1] != "NOTES" || got[2] != "Titles" {
		t.Fatalf("styles = %q", got)
	}
	if n, ok := doc.FindTextStyle(d, " notes "); !ok || n != "NOTES" {
		t.Fatalf("find = %q %v", n, ok)
	}
	if _, ok := doc.FindTextStyle(d, "Bogus"); ok {
		t.Fatal("found Bogus")
	}
}

// TestMirrorAlongKeepsBaselineJustification: MIRRTEXT 0 along the baseline keeps Left, Center
// and Right and moves the text so its box lands on the mirrored box.
func TestMirrorAlongKeepsBaselineJustification(t *testing.T) {
	along := geom.Mat3{1, 0, 0, -1, 0, 10}
	for _, j := range []string{"Left", "Center", "Right"} {
		h, v, _ := doc.ParseJustify(j)
		tx := justified(h, v, geom.V(2, 1))
		box := tx.Layout().Corners
		tx.MirrorReadable(along)
		if tx.Justify() != j {
			t.Fatalf("%s became %s", j, tx.Justify())
		}
		gc := tx.Layout().Corners
		got := geom.BoxOf(gc[:]...)
		want := geom.BoxOf(along.Apply(box[0]), along.Apply(box[1]), along.Apply(box[2]), along.Apply(box[3]))
		nearV(t, j+" min", got.Min, want.Min, 1e-9)
		nearV(t, j+" max", got.Max, want.Max, 1e-9)
	}
}

// Extreme finite values from a file give a finite box: the layout clamps the height, the width
// factor and the obliquing angle (L3), and Sanitize stores the clamped values.
func TestExtremeTextHasFiniteBounds(t *testing.T) {
	finite := func(b geom.Box2) bool {
		for _, v := range []float64{b.Min.X, b.Min.Y, b.Max.X, b.Max.Y} {
			if math.IsNaN(v) || math.IsInf(v, 0) {
				return false
			}
		}
		return true
	}
	n := 0
	for _, h := range []float64{1e300, 1e-300, 0, math.MaxFloat64} {
		for _, wf := range []float64{1e300, 1, 1e-300} {
			for _, ob := range []float64{math.Pi / 2, 89.9999999 * math.Pi / 180, 0, -1e300} {
				for _, fit := range []bool{false, true} {
					e := doc.NewText(geom.V(1e300, -1e300), h, 0, "Hello WORLD")
					e.WidthFactor, e.Oblique = wf, ob
					if fit {
						e.HAlign, e.Align = doc.TextFit, geom.V(-1e300, 1e300)
					}
					if !finite(e.Bounds()) {
						t.Errorf("h %g wf %g oblique %g fit %v: bounds %v", h, wf, ob, fit, e.Bounds())
					}
					e.Sanitize()
					if e.Height > doc.TextMaxSize || e.WidthFactor > doc.TextMaxWidthFactor || math.Abs(e.Oblique) > doc.TextMaxOblique {
						t.Errorf("Sanitize kept %g %g %g", e.Height, e.WidthFactor, e.Oblique)
					}
					n++
				}
			}
		}
	}
	for _, h := range []float64{1e300, math.MaxFloat64} {
		m := doc.NewMText(geom.V(1e300, -1e300), h, 1e300, "Hello WORLD\\Psecond")
		m.LineSpacing = 1e300
		if !finite(m.Bounds()) {
			t.Errorf("MTEXT h %g: bounds %v", h, m.Bounds())
		}
	}
}
