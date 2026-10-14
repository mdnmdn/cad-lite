package gpui

import (
	"testing"
)

func TestBundledFontWhenSystemFontsDisabled(t *testing.T) {
	t.Setenv(NoSystemFontsEnv, "1")
	if p := DefaultFontPath(); p != "" {
		t.Fatalf("DefaultFontPath = %q with system fonts disabled", p)
	}
	m := NewFontManager()
	reg, err := m.DefaultFace(16, 400)
	if err != nil {
		t.Fatal(err)
	}
	if reg.path != bundledRegularPath || !reg.HasGlyph('A') || reg.Ascent(16) <= 0 {
		t.Fatalf("default face = %q", reg.path)
	}
	bold, err := m.DefaultFace(16, 700)
	if err != nil {
		t.Fatal(err)
	}
	if bold.path != bundledBoldPath || bold.synthBold {
		t.Fatalf("bold face = %q synth=%v, want real bundled bold", bold.path, bold.synthBold)
	}
	named, err := m.Face("DejaVu Sans", 16, 400)
	if err != nil || named.path != bundledRegularPath {
		t.Fatalf("named family = %v, %v; want bundled default", named, err)
	}
	if f := m.Fallback('A', 16, 400); f != nil {
		t.Fatalf("Fallback used a system font: %q", f.path)
	}
}

func TestMonospaceFallsBackToBundledGoMono(t *testing.T) {
	t.Setenv(NoSystemFontsEnv, "1")
	m := NewFontManager()
	reg, err := m.Face("monospace", 14, 400)
	if err != nil || reg.path != bundledMonoPath {
		t.Fatalf("monospace = %v, %v; want bundled Go Mono", reg, err)
	}
	bold, err := m.Face("monospace", 14, 700)
	if err != nil || bold.path != bundledMonoBoldPath || bold.synthBold {
		t.Fatalf("bold monospace = %v, %v; want real bundled Go Mono Bold", bold, err)
	}
	if reg.Family() == "" || !reg.HasGlyph('0') {
		t.Fatalf("bundled mono face unusable: family %q", reg.Family())
	}
}
