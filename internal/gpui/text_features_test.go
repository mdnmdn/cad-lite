package gpui

import (
	"math"
	"os"
	"runtime"
	"testing"
)

func TestShapeWrapHardLineBreaks(t *testing.T) {
	_, s, face, _ := textFixture(t)
	lines := s.ShapeWrap("ab\ncd\n\nef", face, math.MaxFloat32)
	if len(lines) != 4 {
		t.Fatalf("got %d lines, want 4", len(lines))
	}
	if len(lines[2].Glyphs) != 0 {
		t.Errorf("blank line has %d glyphs", len(lines[2].Glyphs))
	}
	for i, l := range lines {
		for _, g := range l.Glyphs {
			if g.GlyphID == 0 {
				t.Errorf("line %d draws .notdef", i)
			}
		}
	}
	if got := lines[3].Glyphs[0].Cluster; got != 7 {
		t.Errorf("last line first cluster = %d, want byte offset 7", got)
	}
	_, h := s.Measure("ab\ncd", face, math.MaxFloat32)
	_, h1 := s.Measure("ab", face, math.MaxFloat32)
	if h != 2*h1 {
		t.Errorf("measure height %v, want %v", h, 2*h1)
	}
}

func TestPeriodSpaceNotKerned(t *testing.T) {
	m := NewFontManager()
	requireFont(t)
	s := NewShaper(m)
	face, _ := m.Face("", 16, 400)
	alone := s.ShapeWrap("t.", face, math.MaxFloat32)[0].Glyphs
	spaced := s.ShapeWrap("t. B", face, math.MaxFloat32)[0].Glyphs
	if alone[1].Advance != spaced[1].Advance {
		t.Errorf("period advance %v before a space, %v alone: kerned against space", spaced[1].Advance, alone[1].Advance)
	}
}

func TestFamilyLookup(t *testing.T) {
	requireFont(t)
	m := NewFontManager()
	def, _ := m.Face("", 14, 400)
	unk, _ := m.Face("No Such Family XYZ", 14, 400)
	if unk.path != def.path {
		t.Errorf("unknown family path %q, want default %q", unk.path, def.path)
	}
	if runtime.GOOS != "darwin" {
		t.Skip("family fixtures are darwin paths")
	}
	if _, err := os.Stat("/System/Library/Fonts/Supplemental/Arial.ttf"); err != nil {
		t.Skip("Arial not installed")
	}
	a, _ := m.Face("Arial", 14, 400)
	if a.path == def.path {
		t.Errorf("Arial resolved to default %q", def.path)
	}
	ab, _ := m.Face("arial", 14, 700)
	if ab.path == a.path || ab.synthBold {
		t.Errorf("bold Arial = %q synth=%v, want real bold file", ab.path, ab.synthBold)
	}
	if a2, _ := m.Face("Arial", 14, 400); a2 != a {
		t.Error("face not cached")
	}
}

func TestGlyphFallback(t *testing.T) {
	requireFont(t)
	m := NewFontManager()
	s := NewShaper(m)
	face, _ := m.Face("", 16, 400)
	r := '漢'
	if face.HasGlyph(r) {
		t.Skip("default font already covers CJK")
	}
	if m.Fallback(r, 16, 400) == nil {
		t.Skip("no fallback font with CJK on this machine")
	}
	line := s.ShapeWrap("a漢b", face, math.MaxFloat32)[0]
	if len(line.Glyphs) != 3 {
		t.Fatalf("got %d glyphs", len(line.Glyphs))
	}
	if line.Glyphs[0].Face != nil || line.Glyphs[2].Face != nil {
		t.Error("covered glyphs should use the primary face")
	}
	fb := line.Glyphs[1].Face
	if fb == nil || !fb.HasGlyph(r) || line.Glyphs[1].GlyphID == 0 {
		t.Errorf("CJK glyph not rendered through fallback: face=%v id=%d", fb, line.Glyphs[1].GlyphID)
	}
	if line.Glyphs[2].X <= line.Glyphs[1].X || line.Glyphs[1].Advance <= 0 {
		t.Error("fallback glyph advance not applied")
	}
	out := s.ShapeWrapped("a漢b", TextStyle{FontSize: 16}, math.MaxFloat32)[0]
	if out.Glyphs[1].Mask == nil || out.Glyphs[1].Mask.W == 0 {
		t.Error("fallback glyph has no mask")
	}
}
