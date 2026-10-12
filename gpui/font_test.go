package gpui

import (
	"os"
	"testing"
)

// requireFont skips t when the machine exposes no usable system font, so CI
// images without fonts still pass.
func requireFont(t *testing.T) string {
	t.Helper()
	path := DefaultFontPath()
	if path == "" {
		t.Skip("no system font found on this machine")
	}
	if _, err := os.Stat(path); err != nil {
		t.Skipf("DefaultFontPath() = %q which does not exist: %v", path, err)
	}
	return path
}

func TestDefaultFontPath(t *testing.T) {
	path := DefaultFontPath()
	if path == "" {
		t.Skip("no system font found on this machine")
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("DefaultFontPath() = %q which does not exist: %v", path, err)
	}
}

func TestDefaultFontPathEnvOverride(t *testing.T) {
	t.Setenv("GPUI_FONT", "")
	base := DefaultFontPath()
	if base == "" {
		t.Skip("no system font found on this machine")
	}
	t.Setenv("GPUI_FONT", base)
	if got := DefaultFontPath(); got != base {
		t.Errorf("DefaultFontPath() = %q, want %q from GPUI_FONT", got, base)
	}
	t.Setenv("GPUI_FONT", "/definitely/not/a/font.ttf")
	if got := DefaultFontPath(); got != base {
		t.Errorf("DefaultFontPath() = %q, want candidate fallback %q", got, base)
	}
}

func TestFontLoadMissingPath(t *testing.T) {
	m := NewFontManager()
	if _, err := m.Load("/definitely/not/a/font.ttf"); err == nil {
		t.Error("Load of a non-existent path succeeded, want error")
	}
	if _, err := m.Load(""); err == nil {
		t.Error("Load of an empty path succeeded, want error")
	}
}

func TestDefaultFontMetrics(t *testing.T) {
	requireFont(t)
	m := NewFontManager()
	f, err := m.DefaultFont()
	if err != nil {
		t.Fatalf("DefaultFont(): %v", err)
	}
	if f == nil {
		t.Fatal("DefaultFont() = nil")
	}
	if f.UnitsPerEm() <= 0 {
		t.Errorf("UnitsPerEm() = %v, want > 0", f.UnitsPerEm())
	}
	if f.Ascent(16) <= 0 {
		t.Errorf("Ascent(16) = %v, want > 0", f.Ascent(16))
	}
	if f.Descent(16) <= 0 {
		t.Errorf("Descent(16) = %v, want > 0", f.Descent(16))
	}
}

func TestFontManagerFaceDefaultFamily(t *testing.T) {
	requireFont(t)
	m := NewFontManager()
	face, err := m.Face("", 16, 400)
	if err != nil {
		t.Fatalf("Face(\"\", 16, 400): %v", err)
	}
	def, err := m.DefaultFace(16, 400)
	if err != nil {
		t.Fatalf("DefaultFace(16, 400): %v", err)
	}
	if face != def {
		t.Error("Face(\"\", 16, 400) is not the identical face as DefaultFace(16, 400)")
	}
	base, err := m.DefaultFont()
	if err != nil {
		t.Fatalf("DefaultFont(): %v", err)
	}
	if face.Family() != base.Family() {
		t.Errorf("Face family = %q, want default family %q", face.Family(), base.Family())
	}
	again, err := m.Face("", 16, 400)
	if err != nil || again != face {
		t.Error("repeated Face(\"\", 16, 400) should return the identical cached pointer")
	}
}

func TestFontManagerFaceBoldWeight(t *testing.T) {
	requireFont(t)
	m := NewFontManager()
	regular, err := m.Face("", 16, 400)
	if err != nil {
		t.Fatalf("Face weight 400: %v", err)
	}
	bold, err := m.Face("", 16, 700)
	if err != nil {
		t.Fatalf("Face weight 700: %v", err)
	}
	if bold == nil {
		t.Fatal("Face weight 700 = nil")
	}
	again, err := m.Face("", 16, 700)
	if err != nil || again != bold {
		t.Error("repeated Face weight 700 should return the identical cached pointer")
	}
	if bold == regular {
		t.Error("bold weight returned the regular face pointer; want at least a distinct sized face")
	}
}

func TestFontErrNoFontSentinel(t *testing.T) {
	if ErrNoFont == nil {
		t.Fatal("ErrNoFont is nil")
	}
	if ErrNoFont.Error() == "" {
		t.Error("ErrNoFont has an empty message")
	}
}
