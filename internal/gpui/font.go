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
	"golang.org/x/image/font/gofont/gobold"
	"golang.org/x/image/font/gofont/gomono"
	"golang.org/x/image/font/gofont/gomonobold"
	"golang.org/x/image/font/gofont/goregular"
	"golang.org/x/image/font/sfnt"
	"golang.org/x/image/math/fixed"
)

// ErrNoFont is returned when no usable system font could be located on this
// machine. Text APIs that depend on a font degrade gracefully (zero metrics,
// empty slices) instead of panicking when they encounter it.
var ErrNoFont = errors.New("gpui: no usable system font found")

// Pseudo-paths naming the bundled Go fonts in the FontManager caches.
const (
	bundledRegularPath  = "gpui:bundled/goregular"
	bundledBoldPath     = "gpui:bundled/gobold"
	bundledMonoPath     = "gpui:bundled/gomono"
	bundledMonoBoldPath = "gpui:bundled/gomonobold"
)

// NoSystemFontsEnv is the environment variable that, when non-empty, makes
// the library ignore every system font and use only the bundled fallback
// face. It exists for deterministic renders and for testing the fallback.
const NoSystemFontsEnv = "GPUI_NO_SYSTEM_FONTS"

// systemFontsDisabled reports whether system font discovery is switched off:
// always on js, where no font directories exist, and on request through
// NoSystemFontsEnv.
func systemFontsDisabled() bool {
	return runtime.GOOS == "js" || os.Getenv(NoSystemFontsEnv) != ""
}

// bundledFontData returns the embedded font bytes for a bundled pseudo-path.
func bundledFontData(path string) []byte {
	switch path {
	case bundledRegularPath:
		return goregular.TTF
	case bundledBoldPath:
		return gobold.TTF
	case bundledMonoPath:
		return gomono.TTF
	case bundledMonoBoldPath:
		return gomonobold.TTF
	}
	return nil
}

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
	synthBold bool         // weight >= 600 but no bold file found: glyphs are emboldened at raster time
}

// FontManager loads, parses and caches fonts, and resolves the platform
// default family. It is safe for concurrent use.
type FontManager struct {
	mu    sync.Mutex
	base  map[string]*Font  // parsed fonts keyed by path
	faces map[faceKey]*Font // sized faces keyed by path, size and weight
	bold  map[string]*Font  // bold file per base path; nil value = none found
	def   *Font             // memoized result of DefaultFont

	index    map[string]string      // normalized font file stem -> path, built on first family lookup
	families map[string]familyEntry // resolved families by normalized name (misses included)
	fallback []string               // existing fallback font paths, in priority order
	fbLoaded bool                   // fallback list has been probed
}

// familyEntry is a family's resolved regular and bold fonts; nil fields mean
// "not found" (a nil regular sends the lookup to the default font).
type familyEntry struct{ regular, bold *Font }

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
// path is returned. It returns "" when no candidate exists or system fonts are
// disabled; the FontManager then uses the bundled Go Regular face.
func DefaultFontPath() string {
	if systemFontsDisabled() {
		return ""
	}
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
	data := bundledFontData(path)
	if data == nil {
		var err error
		if data, err = os.ReadFile(path); err != nil {
			return nil, fmt.Errorf("gpui: read font %q: %w", path, err)
		}
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
// first use. When no system font can be found, or system fonts are disabled,
// it is the bundled Go Regular face; ErrNoFont is returned only if even that
// fails to load.
func (m *FontManager) DefaultFont() (*Font, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.def != nil {
		return m.def, nil
	}
	path := DefaultFontPath()
	if path == "" {
		path = bundledRegularPath
	}
	f, err := m.loadLocked(path)
	if err != nil {
		return nil, err
	}
	m.def = f
	return f, nil
}

// Face resolves a face at the given size and weight. family is looked up by
// name in the system font directories (see familyLocked); an empty, generic
// ("sans-serif", "system-ui") or unknown family resolves to the platform
// default font. A weight of 600 or above uses a real bold file when one is
// found; otherwise the regular outlines are emboldened synthetically at raster
// time (a one-pixel horizontal smear). Repeated calls with the same arguments
// return the identical pointer.
func (m *FontManager) Face(family string, size float32, weight int) (*Font, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	base, err := m.defaultLocked()
	if err != nil {
		return nil, err
	}
	var bold *Font
	haveBold := false
	if family != "" {
		if e := m.familyLocked(family); e.regular != nil {
			base, bold, haveBold = e.regular, e.bold, true
		}
	}
	synth := false
	if weight >= 600 {
		if !haveBold {
			bold = m.boldLocked(base)
		}
		if bold != nil {
			base = bold
		} else {
			synth = true
		}
	}
	return m.sizedLocked(base, size, weight, synth), nil
}

// sizedLocked returns the cached face of base at size and weight. Callers must
// hold m.mu.
func (m *FontManager) sizedLocked(base *Font, size float32, weight int, synth bool) *Font {
	key := faceKey{path: base.path, size: size, weight: weight}
	if f, ok := m.faces[key]; ok {
		return f
	}
	face := &Font{
		sfntFont:  base.sfntFont,
		shapeFace: base.shapeFace,
		path:      base.path,
		family:    base.family,
		size:      size,
		weight:    weight,
		synthBold: synth,
	}
	m.faces[key] = face
	return face
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
		path = bundledRegularPath
	}
	f, err := m.loadLocked(path)
	if err != nil {
		return nil, err
	}
	m.def = f
	return f, nil
}

// boldLocked returns a loadable bold font file for base, or nil, memoizing
// the (filesystem-probing) lookup per base path. Callers must hold m.mu.
func (m *FontManager) boldLocked(base *Font) *Font {
	if f, ok := m.bold[base.path]; ok {
		return f
	}
	f := m.findBoldLocked(base)
	if m.bold == nil {
		m.bold = make(map[string]*Font)
	}
	m.bold[base.path] = f
	return f
}

// findBoldLocked probes boldCandidates for base. Callers must hold m.mu.
func (m *FontManager) findBoldLocked(base *Font) *Font {
	if base.path == bundledRegularPath {
		f, _ := m.loadLocked(bundledBoldPath)
		return f
	}
	for _, p := range boldCandidates(base.path) {
		if p == "" || p == base.path {
			continue
		}
		if _, err := os.Stat(p); err != nil {
			continue
		}
		if f, err := m.loadLocked(p); err == nil {
			return f
		}
	}
	return nil
}

// boldCandidates lists bold font files to try for a default font at path: the
// "Bold" sibling, then well-known per-platform bold faces.
func boldCandidates(path string) []string {
	c := []string{boldSiblingPath(path)}
	switch runtime.GOOS {
	case "darwin":
		c = append(c,
			"/System/Library/Fonts/Supplemental/Arial Bold.ttf",
			"/Library/Fonts/Arial Bold.ttf")
	case "windows":
		dir := filepath.Join(os.Getenv("WINDIR"), "Fonts")
		c = append(c, filepath.Join(dir, "segoeuib.ttf"), filepath.Join(dir, "arialbd.ttf"))
	default:
		c = append(c,
			"/usr/share/fonts/truetype/dejavu/DejaVuSans-Bold.ttf",
			"/usr/share/fonts/truetype/liberation/LiberationSans-Bold.ttf",
			"/usr/share/fonts/TTF/DejaVuSans-Bold.ttf")
	}
	return c
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

// fontDirs lists the directories searched for named families.
func fontDirs() []string {
	home, _ := os.UserHomeDir()
	switch runtime.GOOS {
	case "darwin":
		return []string{"/System/Library/Fonts", "/System/Library/Fonts/Supplemental", "/Library/Fonts", filepath.Join(home, "Library/Fonts")}
	case "windows":
		return []string{filepath.Join(os.Getenv("WINDIR"), "Fonts"), filepath.Join(os.Getenv("LOCALAPPDATA"), "Microsoft", "Windows", "Fonts")}
	default:
		return []string{"/usr/share/fonts", "/usr/local/share/fonts", filepath.Join(home, ".fonts"), filepath.Join(home, ".local/share/fonts")}
	}
}

// normalizeFontName lowercases s and drops spaces, dashes, underscores and
// quotes, so "DejaVu Sans", "dejavu-sans" and "DejaVuSans" compare equal.
func normalizeFontName(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		switch r {
		case ' ', '-', '_', '"', '\'', ',':
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

// fontIndexLocked lists the font directories once, mapping each font file's
// normalized stem to its path (first directory wins). Files are not opened.
func (m *FontManager) fontIndexLocked() map[string]string {
	if m.index != nil {
		return m.index
	}
	m.index = make(map[string]string)
	if systemFontsDisabled() {
		return m.index
	}
	for _, dir := range fontDirs() {
		_ = filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return nil
			}
			ext := strings.ToLower(filepath.Ext(path))
			if ext != ".ttf" && ext != ".otf" && ext != ".ttc" {
				return nil
			}
			key := normalizeFontName(strings.TrimSuffix(filepath.Base(path), filepath.Ext(path)))
			if _, dup := m.index[key]; !dup {
				m.index[key] = path
			}
			return nil
		})
	}
	return m.index
}

// genericFamilies maps CSS generic family names to ordered candidate families;
// a nil list means "use the default font". Keys are normalizeFontName forms.
var genericFamilies = map[string][]string{
	"sansserif":   nil,
	"systemui":    nil,
	"default":     nil,
	"applesystem": nil,
	"monospace":   {"menlo", "sfnsmono", "consolas", "dejavusansmono", "liberationmonoregular", "couriernew", "courier"},
	"serif":       {"times", "timesnewroman", "georgia", "dejavuserif", "liberationserifregular"},
	"cursive":     {"markerfelt", "comicsansms"},
	"fantasy":     nil,
	"uimonospace": {"menlo", "sfnsmono", "consolas", "dejavusansmono"},
}

// familyLocked resolves a family name to its regular and bold fonts through
// the file-name index, memoizing hits and misses. Callers must hold m.mu.
func (m *FontManager) familyLocked(name string) familyEntry {
	key := normalizeFontName(name)
	if e, ok := m.families[key]; ok {
		return e
	}
	if m.families == nil {
		m.families = make(map[string]familyEntry)
	}
	cands := []string{key}
	if g, ok := genericFamilies[key]; ok {
		cands = g
	}
	idx := m.fontIndexLocked()
	var e familyEntry
	for _, c := range cands {
		path := idx[c]
		if path == "" {
			path = idx[c+"regular"]
		}
		if path == "" {
			continue
		}
		reg, err := m.loadLocked(path)
		if err != nil {
			continue
		}
		e.regular = reg
		for _, suffix := range []string{"bold", "bd"} {
			if bp := idx[c+suffix]; bp != "" && bp != path {
				if bf, err := m.loadLocked(bp); err == nil {
					e.bold = bf
					break
				}
			}
		}
		if e.bold == nil {
			if sp := boldSiblingPath(path); sp != "" && sp != path {
				if _, err := os.Stat(sp); err == nil {
					e.bold, _ = m.loadLocked(sp)
				}
			}
		}
		break
	}
	if e.regular == nil && (key == "monospace" || key == "uimonospace") {
		e.regular, _ = m.loadLocked(bundledMonoPath)
		if e.regular != nil {
			e.bold, _ = m.loadLocked(bundledMonoBoldPath)
		}
	}
	m.families[key] = e
	return e
}

// fallbackCandidates lists system fonts tried, in order, for runes the
// requested face lacks.
func fallbackCandidates() []string {
	switch runtime.GOOS {
	case "darwin":
		return []string{
			"/System/Library/Fonts/Supplemental/Arial Unicode.ttf",
			"/System/Library/Fonts/Apple Symbols.ttf",
			"/System/Library/Fonts/Hiragino Sans GB.ttc",
			"/System/Library/Fonts/AppleSDGothicNeo.ttc",
		}
	case "windows":
		dir := filepath.Join(os.Getenv("WINDIR"), "Fonts")
		return []string{
			filepath.Join(dir, "seguisym.ttf"), filepath.Join(dir, "msyh.ttc"),
			filepath.Join(dir, "YuGothR.ttc"), filepath.Join(dir, "malgun.ttf"),
		}
	default:
		return []string{
			"/usr/share/fonts/truetype/dejavu/DejaVuSans.ttf",
			"/usr/share/fonts/opentype/noto/NotoSansCJK-Regular.ttc",
			"/usr/share/fonts/truetype/noto/NotoSans-Regular.ttf",
			"/usr/share/fonts/truetype/wqy/wqy-zenhei.ttc",
			"/usr/share/fonts/truetype/unifont/unifont.ttf",
		}
	}
}

// Fallback returns a face at size and weight from the first system fallback
// font that has a glyph for r, or nil when none does. Fallback fonts are
// loaded lazily on the first call; colour emoji fonts are not supported.
func (m *FontManager) Fallback(r rune, size float32, weight int) *Font {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.fbLoaded {
		m.fbLoaded = true
		var cands []string
		if !systemFontsDisabled() {
			cands = fallbackCandidates()
		}
		for _, p := range cands {
			if _, err := os.Stat(p); err == nil {
				m.fallback = append(m.fallback, p)
			}
		}
	}
	for i := 0; i < len(m.fallback); i++ {
		f, err := m.loadLocked(m.fallback[i])
		if err != nil {
			m.fallback = append(m.fallback[:i], m.fallback[i+1:]...)
			i--
			continue
		}
		if f.HasGlyph(r) {
			return m.sizedLocked(f, size, weight, weight >= 600)
		}
	}
	return nil
}
