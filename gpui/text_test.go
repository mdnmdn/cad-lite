package gpui

import (
	"math"
	"testing"
)

// textFixture skips t when the machine has no system font and otherwise
// returns a manager, shaper, a default-size face and the default text style.
func textFixture(t *testing.T) (*FontManager, *Shaper, *Font, TextStyle) {
	t.Helper()
	requireFont(t)
	m := NewFontManager()
	ts := DefaultTextStyle()
	face, err := m.DefaultFace(float32(ts.FontSize), ts.FontWeight)
	if err != nil {
		t.Fatalf("DefaultFace(%v, %d): %v", ts.FontSize, ts.FontWeight, err)
	}
	return m, NewShaper(m), face, ts
}

func TestShapeLineSingleLine(t *testing.T) {
	_, s, face, _ := textFixture(t)

	line := s.ShapeLine("Hello", face)
	if line.Width <= 0 {
		t.Errorf("ShapeLine width = %v, want > 0", line.Width)
	}
	if len(line.Glyphs) == 0 {
		t.Fatal("ShapeLine produced no glyphs")
	}
	for _, g := range line.Glyphs {
		if g.Advance < 0 {
			t.Errorf("glyph %v advance = %v, want >= 0", g.GlyphID, g.Advance)
		}
	}
	prev := -1
	for _, g := range line.Glyphs {
		if g.Cluster < prev {
			t.Errorf("clusters not non-decreasing: %d after %d", g.Cluster, prev)
		}
		prev = g.Cluster
	}

	lines := s.ShapeWrap("Hello", face, math.MaxFloat32)
	if len(lines) != 1 {
		t.Fatalf("ShapeWrap without wrap produced %d lines, want 1", len(lines))
	}
	if lines[0].Width <= 0 {
		t.Errorf("ShapeWrap width = %v, want > 0", lines[0].Width)
	}
	if len(lines[0].Glyphs) == 0 {
		t.Fatal("ShapeWrap produced no glyphs")
	}
}

func TestMeasureSingleLine(t *testing.T) {
	_, s, face, ts := textFixture(t)
	w, h := s.Measure("Hello world", face, math.MaxFloat32)
	if w <= 0 {
		t.Errorf("Measure width = %v, want > 0", w)
	}
	if h != ts.ResolvedLineHeight() {
		t.Errorf("Measure height = %v, want single line height %v", h, ts.ResolvedLineHeight())
	}
}

func TestMeasureNarrowWrapProducesMoreLines(t *testing.T) {
	_, s, face, _ := textFixture(t)
	const text = "Hello world this is a wrapping test"
	const wrap = float32(60)

	wideLines := s.ShapeWrap(text, face, math.MaxFloat32)
	narrowLines := s.ShapeWrap(text, face, wrap)
	if len(wideLines) == 0 {
		t.Fatal("wide shape produced no lines")
	}
	if len(narrowLines) <= len(wideLines) {
		t.Fatalf("narrow wrap produced %d lines, want more than the %d unwrapped lines",
			len(narrowLines), len(wideLines))
	}

	maxAdv := Pixels(0)
	for _, l := range wideLines {
		for _, g := range l.Glyphs {
			if g.Advance > maxAdv {
				maxAdv = g.Advance
			}
		}
	}
	w, _ := s.Measure(text, face, wrap)
	if w > Pixels(wrap)+maxAdv {
		t.Errorf("wrapped width %v exceeds wrapWidth %v + max advance %v", w, wrap, maxAdv)
	}
}

func TestShapeWrapNoWrapMatchesShapeLine(t *testing.T) {
	_, s, face, _ := textFixture(t)
	const text = "Hello world"
	line := s.ShapeLine(text, face)
	lines := s.ShapeWrap(text, face, math.MaxFloat32)
	if len(lines) != 1 {
		t.Fatalf("ShapeWrap with MaxFloat32 produced %d lines, want 1", len(lines))
	}
	d := lines[0].Width - line.Width
	if d < -0.5 || d > 0.5 {
		t.Errorf("unwrapped width %v differs from ShapeLine width %v by %v (> 0.5)",
			lines[0].Width, line.Width, d)
	}
}

func TestGlyphMaskRasterizesAndCaches(t *testing.T) {
	_, s, face, _ := textFixture(t)
	lines := s.ShapeWrap("H", face, math.MaxFloat32)
	if len(lines) == 0 || len(lines[0].Glyphs) == 0 {
		t.Fatal("could not obtain a glyph id for \"H\"")
	}
	gid := lines[0].Glyphs[0].GlyphID

	m1 := s.GlyphMask(face, gid, 16)
	if m1 == nil {
		t.Fatal("GlyphMask returned nil")
	}
	if m1.W <= 0 || m1.H <= 0 {
		t.Fatalf("GlyphMask size = %dx%d, want > 0", m1.W, m1.H)
	}
	inked := false
	for _, a := range m1.Pix {
		if a > 0 {
			inked = true
			break
		}
	}
	if !inked {
		t.Error("GlyphMask has no pixel with alpha > 0")
	}

	m2 := s.GlyphMask(face, gid, 16)
	if m1 != m2 {
		t.Error("second GlyphMask call returned a different pointer; want a cache hit")
	}
}

func TestShapeWrappedAttachesMasks(t *testing.T) {
	_, s, _, _ := textFixture(t)
	lines := s.ShapeWrapped("Hello", TextStyle{FontSize: 16}, math.MaxFloat32)
	if len(lines) == 0 {
		t.Fatal("ShapeWrapped produced no lines")
	}
	for _, l := range lines {
		if l.Width <= 0 {
			t.Errorf("line width = %v, want > 0", l.Width)
		}
		if len(l.Glyphs) == 0 {
			t.Error("line has no glyphs")
		}
		for _, g := range l.Glyphs {
			if g.Mask == nil {
				t.Errorf("glyph %d has nil Mask", g.GlyphID)
				continue
			}
			if g.Mask.W <= 0 {
				t.Errorf("glyph %d mask width = %d, want > 0", g.GlyphID, g.Mask.W)
			}
		}
	}
}

func TestShapeWrappedDefaultFontSize(t *testing.T) {
	_, s, _, _ := textFixture(t)
	lines := s.ShapeWrapped("Hello", TextStyle{FontSize: 0}, math.MaxFloat32)
	if len(lines) == 0 {
		t.Fatal("ShapeWrapped with FontSize 0 produced no lines")
	}
	if lines[0].Width <= 0 {
		t.Errorf("line width = %v, want > 0 at the default size", lines[0].Width)
	}
}

func TestWrappedDegradesWithoutFont(t *testing.T) {
	s := NewShaper(nil)
	w, h := s.MeasureWrapped("Hello", DefaultTextStyle(), math.MaxFloat32)
	if w != 0 || h != 0 {
		t.Errorf("MeasureWrapped without a manager = (%v, %v), want (0, 0)", w, h)
	}
	if got := s.ShapeWrapped("Hello", DefaultTextStyle(), math.MaxFloat32); len(got) != 0 {
		t.Errorf("ShapeWrapped without a manager returned %d lines, want 0", len(got))
	}
	if line := s.ShapeLine("Hello", nil); len(line.Glyphs) != 0 {
		t.Errorf("ShapeLine with nil face returned %d glyphs, want 0", len(line.Glyphs))
	}
}
