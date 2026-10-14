# Reference: text, fonts and shaping

Source: `style.go` (`TextStyle`), `font.go`, `text.go`.
Only `font.go` and `text.go` may import `golang.org/x/image` and
`github.com/go-text/typesetting`.

Back to the [reference index](README.md). Concept page:
[text](../concepts/text.md). Troubleshooting:
[no text in the window](../guides/troubleshooting.md#no-text-in-the-window).

## TextStyle

```text
type TextStyle struct {
    FontFamily    string  // "" means the platform default family
    FontSize      Pixels  // default 14
    FontWeight    int     // 400 normal, 700 bold
    Color         Rgba    // default black
    LineHeight    Pixels  // 0 means 1.2 * FontSize
    Strikethrough bool
}
func DefaultTextStyle() TextStyle
func (t TextStyle) ResolvedLineHeight() Pixels     // effective line height in pixels

type TextRun struct { Text string; Style TextStyle }   // see StyledText in elements.md
```

`TextFields` is a bit set (`TextFieldFamily`, `TextFieldSize`,
`TextFieldWeight`, `TextFieldColor`, `TextFieldLineHeight`,
`TextFieldStrikethrough`) naming which `TextStyle` fields a `StyleRefinement`
sets explicitly; `StyleRefinement.RefineText(base)` overlays exactly those.
The element-level builders (`TextColor`, `TextSize`, ...) are listed in
[style-builders.md](style-builders.md#text-style-inherited-by-descendants-on-a-div).

Text style set on an ancestor `Div` cascades to descendant text during layout;
hover/focus colour and strikethrough cascade at paint time.

## Font discovery

```text
func DefaultFontPath() string      // GPUI_FONT (if it names an existing file), else a per-OS candidate list; "" when none exists or system fonts are disabled
const NoSystemFontsEnv = "GPUI_NO_SYSTEM_FONTS"
var ErrNoFont error                // "gpui: no usable system font found"
```

`GPUI_FONT=/path/to/font.ttf` overrides the default.

When no system font is found, text still renders: the library bundles Go
Regular and Go Bold (`golang.org/x/image/font/gofont`) and uses them as the
default and bold faces. On `js/wasm` there are no system fonts, so the bundled
faces are always used. Setting `GPUI_NO_SYSTEM_FONTS` to any non-empty value
does the same on any platform (default, named families and fallback lookups
skip the system), which gives identical glyphs everywhere and is what the font
tests use. Go fonts cover Latin, Greek and Cyrillic; other scripts show
missing-glyph boxes in the bundled face. System fonts remain preferred
elsewhere. `ErrNoFont` is returned only if the bundled font fails to load.

## FontManager and Font

```text
func NewFontManager() *FontManager       // safe for concurrent use

func (m *FontManager) Load(path string) (*Font, error)                    // parse a font file; .ttc yields the first face; cached by path
func (m *FontManager) DefaultFont() (*Font, error)                        // platform default, else the bundled Go Regular
func (m *FontManager) DefaultFace(size float32, weight int) (*Font, error)  // Face with the empty family
func (m *FontManager) Face(family string, size float32, weight int) (*Font, error)
func (m *FontManager) Fallback(r rune, size float32, weight int) *Font    // first system fallback font with a glyph for r, or nil
```

- `Face` looks `family` up by file name in the system font directories; an
  empty, generic (`sans-serif`, `system-ui`) or unknown family resolves to the
  default font; `monospace`, `ui-monospace`, `serif` and `cursive` map to
  ordered candidate lists. Weight 600 and above uses a real bold file when one
  is found, otherwise the regular outlines are emboldened synthetically.
  Repeated calls with the same arguments return the identical pointer.
- `Fallback` loads fallback fonts lazily; colour emoji fonts are not
  supported.

```text
type Font struct{ /* opaque: a parsed face bound to size and weight; safe for concurrent use */ }
func (f *Font) Family() string                 // name-table family, "" if absent
func (f *Font) UnitsPerEm() float32
func (f *Font) Ascent(size float32) float32    // baseline to top of tallest glyphs, in pixels
func (f *Font) Descent(size float32) float32   // baseline to bottom of deepest glyphs (positive)
func (f *Font) HasGlyph(r rune) bool
```

## Shaper and TextMeasurer

```text
type TextMeasurer interface {
    MeasureWrapped(text string, ts TextStyle, maxWidth float32) (w, h Pixels)
    ShapeWrapped(text string, ts TextStyle, maxWidth float32) []ShapedLineOut
}

func NewShaper(m *FontManager) *Shaper         // nil manager: calls degrade to empty results; not goroutine-safe

func (s *Shaper) ShapeLine(text string, face *Font) ShapedLine                       // one unwrapped line
func (s *Shaper) ShapeWrap(text string, face *Font, wrapWidth float32) []ShapedLine  // math.MaxFloat32 = no wrap
func (s *Shaper) Measure(text string, face *Font, wrapWidth float32) (w, h Pixels)   // widest line x lines * 1.2*size
func (s *Shaper) MeasureWrapped(text string, ts TextStyle, maxWidth float32) (w, h Pixels)
func (s *Shaper) ShapeWrapped(text string, ts TextStyle, maxWidth float32) []ShapedLineOut
func (s *Shaper) ShapeWrappedScaled(text string, ts TextStyle, maxWidth, scale float32) []ShapedLineOut
func (s *Shaper) GlyphMask(face *Font, glyphID uint32, size float32) *GlyphMask
func (s *Shaper) GlyphMaskScaled(face *Font, glyphID uint32, size, scale float32) *GlyphMask
```

`*Shaper` satisfies `TextMeasurer`; the window's measurer is available as
`Window.TextMeasurer()` (nil when no system font was found). The `ts`-taking
variants resolve the style against the shaper's `FontManager`; a `FontSize` of
0 uses the default size. `MeasureWrapped` uses `ts.ResolvedLineHeight()` per
line, the height `Text` paints with, whereas `Measure` (no style) uses
`1.2 * size`.

Wrapping: hard breaks at `"\n"`; otherwise Unicode line-break opportunities,
then the last whitespace, then mid-cluster as a last resort. Shaping is
HarfBuzz via go-text, left-to-right and Latin only.

### Shaped output types

```text
type ShapedGlyph struct {
    GlyphID uint32
    X, Y    Pixels   // pen position relative to the line origin on the baseline
    Advance Pixels
    Cluster int      // byte offset into the source string
    Face    *Font    // fallback face that owns GlyphID; nil = the line's own face
}
type ShapedLine struct { Width, Ascent, Descent Pixels; Glyphs []ShapedGlyph }

type ShapedGlyphOut struct { GlyphID uint32; X, Y, Advance Pixels; Cluster int; Mask *GlyphMask }
type ShapedLineOut struct { Width, Ascent, Descent Pixels; Glyphs []ShapedGlyphOut }   // window-facing: masks attached
```

### Glyph masks

```text
type GlyphMask struct {
    Pix  []uint8                 // coverage, row-major, stride W
    W, H int
    MinX, MinY, MaxX, MaxY float32   // ink bounds in font pixels (y-up) relative to the pen position
    Scale float32                // device scale it was rasterized at; 0 means 1
    X0, Y0 int                   // top-left in device pixels relative to the pen/baseline (y-down); valid when Scale > 0
}
func (m *GlyphMask) At(x, y int) uint8     // out-of-bounds lookups return 0
```

Masks are rasterised at `size x scale` device pixels, so on a HiDPI target the
sprite is blitted 1:1 and text stays sharp. Glyphs are positioned at
fractional logical pixels but blitted at whole device pixels (no sub-pixel
positioning).

## Caches (performance)

Shaped runs are cached by `(face, text)` (2048 entries, dropped wholesale when
full) and glyph masks by `(face, glyph, size, scale)` (4096 entries). Text
that does not change between frames costs a map lookup.
