# Text

Source: `gpui/font.go` (fonts), `gpui/text.go` (shaping, wrapping, glyph
masks), `gpui/text_elem.go` (elements). These are the only files allowed to
import `x/image` and `go-text/typesetting`.

## Font discovery

`FontManager` loads and caches fonts. `App` creates one shared `Shaper` (and
`FontManager`) lazily on first use; if no system font is found the shaper is
`nil`, `Window.TextMeasurer()` returns `nil`, and text elements fall back to a
crude estimator for layout (0.55 em per rune, greedy wrap at spaces) and paint
nothing.

`DefaultFontPath()` resolves the default sans-serif file:

1. the `GPUI_FONT` environment variable, if it names an existing file;
2. the first existing file of a per-OS list:
   - macOS: `/System/Library/Fonts/SFNS.ttf`, `Helvetica.ttc`,
     `/Library/Fonts/Arial.ttf`
   - Windows: `%WINDIR%\Fonts\segoeui.ttf`, `arial.ttf`, `seguisb.ttf`
   - Linux: DejaVu Sans, Liberation Sans, `~/.fonts/DejaVuSans.ttf`;
3. otherwise `""` (`ErrNoFont` from `FontManager.DefaultFont`).

`.ttc` collections load their first face. Fonts are parsed twice, once by
`x/image/font/sfnt` (metrics, outlines) and once by go-text (shaping); a file
either library rejects is rejected.

`FontManager.Face(family, size, weight)` returns a sized face (cached by
path, size and weight). **Family lookup** is by file name: on first use of a
named family the font directories (macOS `/System/Library/Fonts`, its
`Supplemental`, `/Library/Fonts`, `~/Library/Fonts`; Windows `%WINDIR%\Fonts`;
Linux `/usr/share/fonts`, `~/.fonts`, ...) are listed once (files are not
opened), then names are matched with case/space/dash ignored (`Arial` ->
`Arial.ttf`, bold -> `Arial Bold.ttf`). Results, including misses, are cached.
Unknown or generic families use the default font. Weight `>= 600` selects the
family's real bold file when found (else the default-font bold search) and
otherwise marks the face for synthetic bold.

**Fallback.** For each rune the face lacks (not whitespace/controls/joiners),
`FontManager.Fallback(r, size, weight)` searches system fallback fonts (macOS:
Arial Unicode, Apple Symbols, Hiragino Sans GB, Apple SD Gothic Neo; similar
lists elsewhere), loaded lazily. `ShapedGlyph.Face` is set for glyphs owned by
a fallback face. Colour emoji (bitmap fonts) are not supported and still draw
as tofu.

## Shaper

`Shaper` (`gpui/text.go`) is the `TextMeasurer` the window uses:

```go
type TextMeasurer interface {
    MeasureWrapped(text string, ts TextStyle, maxWidth float32) (w, h Pixels)
    ShapeWrapped(text string, ts TextStyle, maxWidth float32) []ShapedLineOut
}
```

`*Shaper` also provides `ShapeWrappedScaled(text, ts, maxWidth, scale)`, which
`Text` uses when present, plus lower-level `ShapeLine`, `ShapeWrap`,
`Measure`, `GlyphMask` and `GlyphMaskScaled`.

- **Shaping** is HarfBuzz via `go-text/typesetting`, fixed to
  left-to-right direction and the Latin script. Bidirectional and complex
  scripts are not handled. Ligatures and kerning from the font's OpenType
  tables apply. Kerning of a glyph against a following space is dropped
  (SF's `.`+space pair otherwise collapses the gap after a period).
- **Caches.** Shaped runs are cached by `(face, text)` (2048 entries; the map
  is dropped when full). Everything downstream of shaping (wrapping, line
  building) runs per call over the cached run.
- **Not goroutine-safe**; it shares scratch buffers and maps, like the rest of
  the UI.

## Wrapping

`ShapeWrap(text, face, wrapWidth)` breaks lines using Unicode UAX #14 line
break opportunities (`go-text/typesetting/segmenter`) over shaped clusters:

- `"\n"` (and `"\r\n"`) are hard line breaks: the text is split into
  paragraphs, each shaped and wrapped separately; empty paragraphs are blank
  lines and no glyph is drawn for the newline;
- text breaks at the last allowed opportunity that fits;
- if no opportunity precedes the overflowing cluster, it falls back to the last
  whitespace, and if there is none, the line breaks between clusters
  (mid-word) so a single long word never exceeds the width except when a
  single cluster is itself wider than `wrapWidth`;
- `maxWidth = math.MaxFloat32` means "no wrapping".

`MeasureWrapped` returns the widest line and `lines * ts.ResolvedLineHeight()`,
the same line height `Text` paints with (`Shaper.Measure(text, face, w)`, which
has no style, uses `1.2 * size`).

Layout asks for three widths: max-content (`MaxFloat32`), a bounded wrap width,
and min-content (`<= 0`, implemented in `measureText` as the longest word).
`Text` paints with the wrap width equal to the laid-out content width, so it
wraps the same way it was measured.

## Scale-aware glyph masks

Glyphs are rasterized, not drawn as outlines at paint time. For each
`(face, glyph id, size, scale)` the shaper rasterizes the outline with
`x/image/vector` at **ppem = size x window scale** into an 8-bit coverage
`GlyphMask` (1 px padding, no hinting, greyscale anti-aliasing) and caches it
(4096 entries; dropped when full). The mask records its device-pixel offset
(`X0`, `Y0`), size, and the `Scale` it was rendered at.

When `Text` paints it passes `Window.ScaleFactor()` to `ShapeWrappedScaled`
and converts the mask's device-pixel box back to logical bounds
(`X0/scale`, `W/scale`, ...). `scaleScene` then multiplies by the scale, so
on a 2x display the sprite lands exactly on a 1:1 texel grid and is blitted
without resampling: text is sharp rather than an upscaled 1x bitmap. Glyph
origins are positioned at fractional logical pixels but the blit rounds to
whole device pixels, so there is no sub-pixel glyph positioning.

`Underline` primitives (used for strikethrough) are plain quads with a
thickness of `max(1, FontSize/14)`.

## Known limits

- Family lookup is by file name only; weights are regular and "bold".
- Fallback covers glyphs present in the listed system fonts; no bidi, no
  complex-script shaping, no colour emoji.
- `StyledText` runs wrap independently and never share a line.
- `TextInput` measures at scale 1.
