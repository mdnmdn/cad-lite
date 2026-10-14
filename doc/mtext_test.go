package doc_test

import (
	"math"
	"strings"
	"testing"
	"time"

	"cad-lite/doc"
	"cad-lite/geom"
)

func TestMTextPlain(t *testing.T) {
	got := doc.MTextPlain(`{\fArial|b1;Room}\PArea\~24 m\U+00B2 {\H0.7x;oak} a\\b \{x\} 1\S1/2; \LU\l`)
	if want := "Room\nArea 24 m\\U+00B2 oak a\\b {x} 11/2 U"; got != want {
		t.Fatalf("plain = %q, want %q", got, want)
	}
}

func TestMTextWrapsAndAttaches(t *testing.T) {
	m := doc.NewMText(geom.V(10, 20), 2, 30, "one two three four five six seven eight nine ten")
	lay := m.Layout()
	if len(lay.Lines) < 2 || strings.Join(lay.Lines, " ") != "one two three four five six seven eight nine ten" {
		t.Fatalf("lines = %q", lay.Lines)
	}
	for i, w := range lay.Widths {
		if w > 30+1e-9 {
			t.Fatalf("line %d is %v wide", i, w)
		}
	}
	// Top left: the box's top-left corner is Insert, the first baseline one height below.
	nearV(t, "top-left", lay.Corners[3], geom.V(10, 20), 1e-12)
	nearV(t, "first baseline", lay.Starts[0], geom.V(10, 18), 1e-12)
	nearV(t, "second baseline", lay.Starts[1], geom.V(10, 18-2*5.0/3), 1e-12)
	if w := lay.Corners[1].X - lay.Corners[0].X; math.Abs(w-30) > 1e-12 {
		t.Fatalf("box width = %v", w)
	}
	for a := 1; a <= 9; a++ {
		m.Attachment = a
		c := m.Layout().Corners
		x := []float64{c[0].X, (c[0].X + c[1].X) / 2, c[1].X}[(a-1)%3]
		y := []float64{c[3].Y, (c[0].Y + c[3].Y) / 2, c[0].Y}[(a-1)/3]
		nearV(t, "attachment", geom.V(x, y), geom.V(10, 20), 1e-9)
	}
	// Without a width nothing wraps; paragraphs still break.
	n := doc.NewMText(geom.V(0, 0), 1, 0, `a long line that does not wrap\Psecond`)
	if l := n.Layout().Lines; len(l) != 2 || l[0] != "a long line that does not wrap" {
		t.Fatalf("lines = %q", l)
	}
}

func TestMTextTransform(t *testing.T) {
	m := doc.NewMText(geom.V(5, 5), 2, 25, `Kitchen\Parea 12 m2 with island`)
	m.Attachment, m.Rotation = doc.MTextMiddleRight, 0.4
	for name, mat := range map[string]geom.Mat3{
		"move": geom.Translate(geom.V(3, -2)), "rotate": geom.Rotate(geom.V(1, 1), 1.1), "scale": geom.ScaleAbout(geom.V(0, 0), 0.5),
	} {
		want := strokes(m)
		for i := range want {
			want[i] = mat.Apply(want[i])
		}
		cp := m.Clone().(*doc.MText)
		cp.Transform(mat)
		got := strokes(cp)
		for i := range got {
			nearV(t, name, got[i], want[i], 1e-9)
		}
	}
	// A mirror keeps it readable and lands its box on the mirrored box.
	for _, mat := range []geom.Mat3{{-1, 0, 0, 1, 20, 0}, {1, 0, 0, -1, 0, 10}} {
		r := m.Clone().(*doc.MText)
		r.Rotation = 0
		box := r.Box()
		r.Transform(mat)
		if r.Layout().Lines[0] != "Kitchen" || math.Abs(math.Cos(r.Rotation)-1) > 1e-12 {
			t.Fatalf("mirrored: rotation %v", r.Rotation)
		}
		gb := r.Box()
		got := geom.BoxOf(gb[:]...)
		want := geom.BoxOf(mat.Apply(box[0]), mat.Apply(box[1]), mat.Apply(box[2]), mat.Apply(box[3]))
		nearV(t, "mirrored min", got.Min, want.Min, 1e-9)
		nearV(t, "mirrored max", got.Max, want.Max, 1e-9)
	}
}

func TestMTextEscape(t *testing.T) {
	for _, s := range []string{`C:\data\Pictures`, "{a} \\P }{", "plain", ""} {
		if got := doc.MTextPlain(doc.MTextEscape(s)); got != s {
			t.Fatalf("%q -> %q -> %q", s, doc.MTextEscape(s), got)
		}
	}
}

// Wrapping measures each word once, so a long paragraph lays out in time linear in its length
// (re-measuring the growing row made a 100 KB paragraph take seconds). The bound is generous:
// it only catches a return to quadratic cost.
func TestMTextWrapIsLinear(t *testing.T) {
	words := strings.Repeat("abcd ", 20000)
	start := time.Now()
	m := doc.NewMText(geom.V(0, 0), 1, 1e4, words)
	_ = m.Bounds()
	lines := len(m.Layout().Lines)
	if el := time.Since(start); el > 3*time.Second {
		t.Fatalf("wrapping 100 KB took %v", el)
	}
	if lines < 5 {
		t.Fatalf("%d rows, want the paragraph wrapped", lines)
	}
	for _, l := range m.Layout().Lines {
		if doc.TextAdvance(l) > 1e4+1e-6 && strings.Contains(l, " ") {
			t.Fatalf("row %q is wider than the box", l)
		}
	}
}

func BenchmarkMTextWrap100KB(b *testing.B) {
	words := strings.Repeat("abcd ", 20000)
	for i := 0; i < b.N; i++ {
		m := doc.NewMText(geom.V(0, 0), 1, 1e4, words)
		_ = m.Bounds()
	}
}
