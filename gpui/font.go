package gpui

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"

	tsfont "github.com/go-text/typesetting/font"
	"golang.org/x/image/font"
	"golang.org/x/image/font/sfnt"
	"golang.org/x/image/math/fixed"
)

// ErrNoFont is returned when no usable system font could be located on this
// machine. Text APIs that depend on a font degrade gracefully (zero metrics,
// empty slices) instead of panicking when they encounter it.
var ErrNoFont = errors.New("gpui: no usable system font found")

// Font is an opaque loaded font face: a parsed, immutable font file bound to
// the size and weight it will be rendered at. Fonts are produced by a
// FontManager and are safe for concurrent use.
type Font struct {
	sfntFont  *sfnt.Font   // used for metrics and glyph outline loading
	shapeFace *tsfont.Face // used for OpenType shaping; nil if the file could not be parsed by go-text
	path      string       // file the font was loaded from
	family    string       // family name from the name table, may be ""
	size      float32      // ppem this face instance is bound to; 0 on raw loaded fonts
	weight    int          // requested CSS-style weight (400 regular, 700 bold)
}

// FontManager loads, parses and caches fonts, and resolves the platform
// default family. It is safe for concurrent use.
type FontManager struct {
	mu    sync.Mutex
	base  map[string]*Font  // parsed fonts keyed by path
	faces map[faceKey]*Font // sized faces keyed by path, size and weight
	def   *Font             // memoized result of DefaultFont
}

// faceKey identifies a sized face within a FontManager cache.
type faceKey struct {
	path   string
	size   float32
	weight int
}

// NewFontManager returns a FontManager with empty caches.
func NewFontManager() *FontManager {
	return &FontManager{
		base:  make(map[string]*Font),
		faces: make(map[faceKey]*Font),
	}
}

// DefaultFontPath resolves the platform's default sans-serif font file. The
// GPUI_FONT environment variable takes precedence when it names an existing
// file. Otherwise a per-GOOS candidate list is walked and the first existing
// path is returned. It returns "" when no candidate exists.
func DefaultFontPath() string {
	if p := os.Getenv("GPUI_FONT"); p != "" {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	for _, p := range defaultFontCandidates() {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return ""
}

// defaultFontCandidates returns the per-platform ordered font search list.
func defaultFontCandidates() []string {
	switch runtime.GOOS {
	case "darwin":
		return []string{
			"/System/Library/Fonts/SFNS.ttf",
			"/System/Library/Fonts/Helvetica.ttc",
			"/Library/Fonts/Arial.ttf",
		}
	case "windows":
		dir := filepath.Join(os.Getenv("WINDIR"), "Fonts")
		return []string{
			filepath.Join(dir, "segoeui.ttf"),
			filepath.Join(dir, "arial.ttf"),
			filepath.Join(dir, "seguisb.ttf"),
		}
	default: // linux and other unixes
		home, _ := os.UserHomeDir()
		candidates := []string{
			"/usr/share/fonts/truetype/dejavu/DejaVuSans.ttf",
			"/usr/share/fonts/truetype/liberation/LiberationSans-Regular.ttf",
			"/usr/share/fonts/TTF/DejaVuSans.ttf",
		}
		if home != "" {
			candidates = append(candidates, filepath.Join(home, ".fonts", "DejaVuSans.ttf"))
		}
		return candidates
	}
}

// Load parses the font file at path and returns a size-agnostic Font for it.
// TrueType collections (.ttc) yield their first face. Results are cached by
// path, so repeated calls return the identical pointer.
func (m *FontManager) Load(path string) (*Font, error) {
	if path == "" {
		return nil, errors.New("gpui: Load: empty font path")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.loadLocked(path)
}

// loadLocked is Load without acquiring the mutex; callers must hold m.mu.
func (m *FontManager) loadLocked(path string) (*Font, error) {
	if f, ok := m.base[path]; ok {
		if f == nil {
			return nil, fmt.Errorf("gpui: load font %q: previously failed", path)
		}
		return f, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("gpui: read font %q: %w", path, err)
	}
	f, err := parseFontData(data, path)
	if err != nil {
		return nil, fmt.Errorf("gpui: parse font %q: %w", path, err)
	}
	m.base[path] = f
	return f, nil
}

// parseFontData parses raw font bytes. Collections (.ttc, detected by the
// leading "ttcf" tag) contribute their face index 0; single files are parsed
// directly. Both an sfnt font (for metrics/outlines) and a go-text face (for
// shaping) are required; a file only one of them understands is rejected.
func parseFontData(data []byte, path string) (*Font, error) {
	var (
		sf *sfnt.Font
		tf *tsfont.Face
	)
	if len(data) >= 4 && string(data[:4]) == "ttcf" {
		col, err := sfnt.ParseCollection(data)
		if err != nil {
			return nil, err
		}
		if col.NumFonts() == 0 {
			return nil, errors.New("empty TrueType collection")
		}
		if sf, err = col.Font(0); err != nil {
			return nil, err
		}
		faces, err := tsfont.ParseTTC(bytes.NewReader(data))
		if err != nil {
			return nil, err
		}
		if len(faces) == 0 {
			return nil, errors.New("empty TrueType collection")
		}
		tf = faces[0]
	} else {
		var err error
		if sf, err = sfnt.Parse(data); err != nil {
			return nil, err
		}
		if tf, err = tsfont.ParseTTF(bytes.NewReader(data)); err != nil {
			return nil, err
		}
	}
	var buf sfnt.Buffer
	family, _ := sf.Name(&buf, sfnt.NameIDFamily)
	return &Font{
		sfntFont:  sf,
		shapeFace: tf,
		path:      path,
		family:    family,
	}, nil
}

// DefaultFont returns the platform default font, loading and memoizing it on
// first use. It returns ErrNoFont when no system font could be found.
func (m *FontManager) DefaultFont() (*Font, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.def != nil {
		return m.def, nil
	}
	path := DefaultFontPath()
	if path == "" {
		return nil, ErrNoFont
	}
	f, err := m.loadLocked(path)
	if err != nil {
		return nil, err
	}
	m.def = f
	return f, nil
}

// Face resolves a face at the given size and weight. For this POC, family
// selection is not implemented: any family name (including "") resolves to the
// platform default family. A weight of 600 or above prefers a bold sibling
// file (the same directory with "Bold" inserted before the extension) when one
// exists, falling back to the regular face. Repeated calls with the same
// arguments return the identical pointer.
func (m *FontManager) Face(family string, size float32, weight int) (*Font, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	base, err := m.defaultLocked()
	if err != nil {
		return nil, err
	}
	if weight >= 600 {
		if bold, err := m.loadLocked(boldSiblingPath(base.path)); err == nil {
			base = bold
		}
	}
	key := faceKey{path: base.path, size: size, weight: weight}
	if f, ok := m.faces[key]; ok {
		return f, nil
	}
	face := &Font{
		sfntFont:  base.sfntFont,
		shapeFace: base.shapeFace,
		path:      base.path,
		family:    base.family,
		size:      size,
		weight:    weight,
	}
	m.faces[key] = face
	return face, nil
}

// DefaultFace is Face with the empty (default) family.
func (m *FontManager) DefaultFace(size float32, weight int) (*Font, error) {
	return m.Face("", size, weight)
}

// defaultLocked is DefaultFont without acquiring the mutex.
func (m *FontManager) defaultLocked() (*Font, error) {
	if m.def != nil {
		return m.def, nil
	}
	path := DefaultFontPath()
	if path == "" {
		return nil, ErrNoFont
	}
	f, err := m.loadLocked(path)
	if err != nil {
		return nil, err
	}
	m.def = f
	return f, nil
}

// boldSiblingPath rewrites /dir/Name.ttf to /dir/NameBold.ttf (and .otf the
// same way); for any other extension it returns "" so callers fall back.
func boldSiblingPath(path string) string {
	ext := filepath.Ext(path)
	if ext != ".ttf" && ext != ".otf" {
		return ""
	}
	return strings.TrimSuffix(path, ext) + "Bold" + ext
}

// Family returns the family name from the font's name table ("" if absent).
func (f *Font) Family() string { return f.family }

// UnitsPerEm returns the font design units per em (typically 1000 or 2048).
func (f *Font) UnitsPerEm() float32 { return float32(f.sfntFont.UnitsPerEm()) }

// Ascent returns the distance in pixels from the baseline to the top of the
// tallest glyphs at the given size.
func (f *Font) Ascent(size float32) float32 {
	var buf sfnt.Buffer
	m, err := f.sfntFont.Metrics(&buf, fixed.Int26_6(size*64), font.HintingNone)
	if err != nil {
		return 0
	}
	return float32(m.Ascent) / 64
}

// Descent returns the distance in pixels from the baseline down to the bottom
// of the deepest glyphs at the given size (positive).
func (f *Font) Descent(size float32) float32 {
	var buf sfnt.Buffer
	m, err := f.sfntFont.Metrics(&buf, fixed.Int26_6(size*64), font.HintingNone)
	if err != nil {
		return 0
	}
	return float32(m.Descent) / 64
}

// HasGlyph reports whether the font provides a glyph for r.
func (f *Font) HasGlyph(r rune) bool {
	var buf sfnt.Buffer
	gi, err := f.sfntFont.GlyphIndex(&buf, r)
	return err == nil && gi != 0
}
