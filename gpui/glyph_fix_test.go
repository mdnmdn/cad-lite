package gpui

import (
	"math"
	"testing"
)

func maskRowSum(m *GlyphMask, y0, y1 int) (sum int) {
	for y := y0; y < y1; y++ {
		for x := 0; x < m.W; x++ {
			sum += int(m.At(x, y))
		}
	}
	return sum
}

func glyphMaskFor(t *testing.T, s *Shaper, face *Font, text string) *GlyphMask {
	t.Helper()
	line := s.ShapeLine(text, face)
	if len(line.Glyphs) == 0 {
		t.Fatalf("no glyphs for %q", text)
	}
	m := s.GlyphMask(face, line.Glyphs[0].GlyphID, 48)
	if m == nil || m.W == 0 || m.H == 0 {
		t.Fatalf("empty mask for %q", text)
	}
	return m
}

func TestGlyphMaskUpright(t *testing.T) {
	_, s, _, _ := textFixture(t)
	face, err := s.fonts.DefaultFace(48, 400)
	if err != nil {
		t.Fatal(err)
	}
	m := glyphMaskFor(t, s, face, "T")
	third := m.H / 3
	top := maskRowSum(m, 0, third)
	bottom := maskRowSum(m, m.H-third, m.H)
	if top <= bottom*3/2 {
		t.Errorf("T crossbar not at top: top-third coverage %d, bottom-third %d", top, bottom)
	}
	if m.MaxY <= 0 || m.MaxY <= m.MinY {
		t.Errorf("bad y bounds MinY=%v MaxY=%v", m.MinY, m.MaxY)
	}
	if math.Abs(float64(m.MaxY-m.MinY)-float64(m.H-2)) > 1.01 {
		t.Errorf("ink height %v inconsistent with mask height %d", m.MaxY-m.MinY, m.H)
	}
}

func TestBoldIsHeavier(t *testing.T) {
	_, s, _, _ := textFixture(t)
	reg, _ := s.fonts.DefaultFace(48, 400)
	bold, _ := s.fonts.DefaultFace(48, 700)
	r := glyphMaskFor(t, s, reg, "l")
	b := glyphMaskFor(t, s, bold, "l")
	if rs, bs := maskRowSum(r, 0, r.H), maskRowSum(b, 0, b.H); bs <= rs {
		t.Errorf("bold l coverage %d not above regular %d", bs, rs)
	}

	synth := *reg
	synth.synthBold = true
	sm := glyphMaskFor(t, s, &synth, "l")
	if rs, ss := maskRowSum(r, 0, r.H), maskRowSum(sm, 0, sm.H); ss <= rs {
		t.Errorf("synthetic bold l coverage %d not above regular %d", ss, rs)
	}
}
