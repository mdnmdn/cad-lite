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

func TestPaintWrapWidthNeverBelowMeasuredWidth(t *testing.T) {
	pads := []Pixels{0.1, 0.3, 7.7, 13.37}
	for _, pad := range pads {
		style := DefaultStyle()
		style.Padding = Edges{Left: pad, Right: pad / 3}
		for i := 1; i < 3000; i++ {
			measured := Pixels(float32(i) * 0.37)
			b := Bounds{Size: Size{W: measured + style.Padding.Horizontal()}}
			if got := paintWrapWidth(b, style); got < float32(measured) {
				t.Fatalf("pad %v measured %v: wrap width %v is below measured width", pad, measured, got)
			}
		}
	}
}

func TestShapeWrappedScaledRasterizesAtDeviceSize(t *testing.T) {
	_, s, _, _ := textFixture(t)
	ts := TextStyle{FontSize: 16}
	face := s.resolveFace(ts)
	l1 := s.ShapeWrappedScaled("Hg", ts, math.MaxFloat32, 1)
	l2 := s.ShapeWrappedScaled("Hg", ts, math.MaxFloat32, 2)
	if len(l1) == 0 || len(l2) == 0 {
		t.Fatal("no lines")
	}
	for i, g := range l2[0].Glyphs {
		m := g.Mask
		if m.Scale != 2 {
			t.Fatalf("mask scale = %v, want 2", m.Scale)
		}
		direct := s.rasterizeGlyph(face, g.GlyphID, faceSize(face)*2, 1)
		if m.W != direct.W || m.H != direct.H || string(m.Pix) != string(direct.Pix) {
			t.Fatalf("glyph %d: scale-2 mask differs from direct size*2 mask", i)
		}
		m1 := l1[0].Glyphs[i].Mask
		if m.W < 2*m1.W-4 || m.H < 2*m1.H-4 {
			t.Fatalf("glyph %d: mask %dx%d not ~2x of %dx%d", i, m.W, m.H, m1.W, m1.H)
		}
	}
}

func TestRasterizerBlitsMatchingMaskOneToOne(t *testing.T) {
	m := &GlyphMask{W: 4, H: 4, Pix: make([]uint8, 16)}
	for i := range m.Pix {
		m.Pix[i] = uint8(i*16 + 7)
	}
	r := NewRasterizer(10, 10)
	r.Reset(Rgba{0, 0, 0, 0})
	sc := &Scene{}
	sc.Sprites = append(sc.Sprites, GlyphSprite{
		Bounds:      Bounds{Origin: Point{X: 1.3, Y: 1.7}, Size: Size{W: 4, H: 4}},
		ContentMask: ContentMask{Bounds: Bounds{Size: Size{W: 10, H: 10}}},
		Mask:        m,
		Color:       Rgba{0, 0, 0, 255},
	})
	r.Render(sc)
	for y := 0; y < 4; y++ {
		for x := 0; x < 4; x++ {
			if got := r.Pixels()[4*((y+2)*10+x+1)+3]; got != m.Pix[y*4+x] {
				t.Fatalf("pixel (%d,%d) alpha = %d, want %d", x, y, got, m.Pix[y*4+x])
			}
		}
	}
}
