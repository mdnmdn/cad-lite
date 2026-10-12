package gpui

import (
	"image"
	"image/color"
	"math"

	"github.com/go-text/typesetting/di"
	"github.com/go-text/typesetting/language"
	"github.com/go-text/typesetting/segmenter"
	"github.com/go-text/typesetting/shaping"
	"golang.org/x/image/font/sfnt"
	"golang.org/x/image/math/fixed"
	"golang.org/x/image/vector"
)

// glyphMaskCacheLimit bounds the Shaper's glyph mask cache; exceeding it
// clears the whole cache (a deliberately simple bound for this POC).
const glyphMaskCacheLimit = 4096

// shapeCacheLimit bounds the Shaper's shaped-run cache the same way.
const shapeCacheLimit = 2048

// Shaper shapes text into positioned glyphs and rasterizes glyphs to coverage
// masks. A Shaper is not safe for concurrent use.
type Shaper struct {
	fonts *FontManager
	hb    shaping.HarfbuzzShaper
	buf   sfnt.Buffer                 // scratch buffer for outline loading
	masks map[glyphMaskKey]*GlyphMask // rasterized glyph cache
	runs  map[shapeKey]*shapedRun     // shaped text cache
}

// shapeKey identifies one cached shaping result.
type shapeKey struct {
	face *Font
	text string
}

// shapedRun is the wrap-independent result of shaping a string with one
// face: HarfBuzz glyphs (Cluster still a rune index), the final pen, and the
// line-break data. It is immutable once cached.
type shapedRun struct {
	runes     []rune
	glyphs    []ShapedGlyph
	pen       fixed.Int26_6
	byteOff   []int
	clusters  []clusterSpan
	allowed   map[int]bool
	mandatory map[int]bool
}

// shaped returns the cached shaping of text with face, shaping it on a miss.
// Layout measures the same strings many times per frame and every frame, so
// this keeps HarfBuzz and the line segmenter off the steady-state path.
func (s *Shaper) shaped(text string, face *Font) *shapedRun {
	key := shapeKey{face: face, text: text}
	if r := s.runs[key]; r != nil {
		return r
	}
	runes := []rune(text)
	out := s.shapeRunes(runes, face)
	glyphs, pen := toShapedGlyphs(out.Glyphs)
	allowed, mandatory := lineBreaks(runes)
	r := &shapedRun{
		runes:     runes,
		glyphs:    glyphs,
		pen:       pen,
		byteOff:   runeByteOffsets(runes),
		clusters:  groupClusters(glyphs),
		allowed:   allowed,
		mandatory: mandatory,
	}
	if s.runs == nil || len(s.runs) >= shapeCacheLimit {
		s.runs = make(map[shapeKey]*shapedRun)
	}
	s.runs[key] = r
	return r
}

// glyphMaskKey identifies one cached glyph mask: the face pointer selects the
// font file, size the ppem.
type glyphMaskKey struct {
	face    *Font
	glyphID uint32
	size    float32
	scale   float32
}

// NewShaper returns a Shaper that resolves faces through m. A nil manager is
// allowed: shape and measure calls then degrade to empty results.
func NewShaper(m *FontManager) *Shaper {
	return &Shaper{fonts: m}
}

// ShapedLine is one visual line of shaped text.
type ShapedLine struct {
	Width, Ascent, Descent Pixels
	Glyphs                 []ShapedGlyph
}

// ShapedGlyph is one positioned glyph of a shaped line.
type ShapedGlyph struct {
	GlyphID uint32
	X, Y    Pixels // pen position, relative to the line's origin on the baseline
	Advance Pixels
	Cluster int // byte offset into the source string
}

// ShapeLine shapes text into a single line with no wrapping. Empty text
// yields one empty line carrying the face's metrics.
func (s *Shaper) ShapeLine(text string, face *Font) ShapedLine {
	ascent, descent := lineMetrics(face)
	if face == nil || face.shapeFace == nil {
		return ShapedLine{Ascent: ascent, Descent: descent}
	}
	run := s.shaped(text, face)
	glyphs := append([]ShapedGlyph(nil), run.glyphs...)
	for i := range glyphs {
		glyphs[i].Cluster = run.byteOff[glyphs[i].Cluster]
	}
	return ShapedLine{
		Width:   px26(run.pen),
		Ascent:  ascent,
		Descent: descent,
		Glyphs:  glyphs,
	}
}

// ShapeWrap shapes text, greedily breaking it into lines no wider than
// wrapWidth (math.MaxFloat32 means no wrap). Breaks are placed at Unicode
// line-break opportunities, then at the last whitespace before an overflow,
// and only then mid-cluster as a last resort, so a shaping cluster (ligature,
// combining marks) never straddles two lines.
func (s *Shaper) ShapeWrap(text string, face *Font, wrapWidth float32) []ShapedLine {
	ascent, descent := lineMetrics(face)
	if face == nil || face.shapeFace == nil {
		return []ShapedLine{{Ascent: ascent, Descent: descent}}
	}
	run := s.shaped(text, face)
	runes, glyphs, byteOff := run.runes, run.glyphs, run.byteOff
	clusters, allowed, mandatory := run.clusters, run.allowed, run.mandatory

	var lines []ShapedLine
	for k := 0; k < len(clusters); {
		pen := Pixels(0)
		brk := -1 // glyph index at which the current line should end
		j := k
		for j < len(clusters) {
			cr := glyphs[clusters[j].first].Cluster
			if j > k && mandatory[cr] {
				break
			}
			if pen+clusters[j].width > Pixels(wrapWidth) && j > k {
				if brk > k {
					j = brk
				} else if ws := lastWhitespace(clusters, glyphs, runes, k, j); ws >= k {
					j = ws + 1
				}
				break
			}
			pen += clusters[j].width
			j++
			if j < len(clusters) {
				if allowed[glyphs[clusters[j].first].Cluster] {
					brk = j
				}
			}
		}
		lines = append(lines, buildLine(clusters[k:j], glyphs, byteOff, ascent, descent))
		k = j
	}
	if len(lines) == 0 {
		lines = append(lines, ShapedLine{Ascent: ascent, Descent: descent})
	}
	return lines
}

// Measure returns the wrapped text's natural size: the widest line's width by
// the number of lines times the resolved line height.
func (s *Shaper) Measure(text string, face *Font, wrapWidth float32) (w, h Pixels) {
	lines := s.ShapeWrap(text, face, wrapWidth)
	for _, l := range lines {
		w = maxPixels(w, l.Width)
	}
	ts := TextStyle{FontSize: Pixels(faceSize(face))}
	h = Pixels(len(lines)) * ts.ResolvedLineHeight()
	return w, h
}

// GlyphMask rasterizes one glyph of face at size into an alpha coverage mask,
// caching the result by (face, glyphID, size).
func (s *Shaper) GlyphMask(face *Font, glyphID uint32, size float32) *GlyphMask {
	return s.GlyphMaskScaled(face, glyphID, size, 1)
}

// GlyphMaskScaled rasterizes one glyph at size*scale device pixels, caching by
// (face, glyphID, size, scale). The mask's ink bounds stay in logical pixels.
func (s *Shaper) GlyphMaskScaled(face *Font, glyphID uint32, size, scale float32) *GlyphMask {
	if scale <= 0 {
		scale = 1
	}
	if face == nil || face.sfntFont == nil {
		return nil
	}
	if size <= 0 {
		size = faceSize(face)
	}
	if s.masks == nil {
		s.masks = make(map[glyphMaskKey]*GlyphMask)
	}
	key := glyphMaskKey{face: face, glyphID: glyphID, size: size, scale: scale}
	if m := s.masks[key]; m != nil {
		return m
	}
	m := s.rasterizeGlyph(face, glyphID, size, scale)
	if len(s.masks) >= glyphMaskCacheLimit {
		s.masks = make(map[glyphMaskKey]*GlyphMask)
	}
	s.masks[key] = m
	return m
}

// ShapedLineOut is the window-facing view of a shaped line.
type ShapedLineOut struct {
	Width, Ascent, Descent Pixels
	Glyphs                 []ShapedGlyphOut
}

// ShapedGlyphOut is one glyph of a ShapedLineOut with its mask attached.
type ShapedGlyphOut struct {
	GlyphID uint32
	X, Y    Pixels
	Advance Pixels
	Cluster int
	Mask    *GlyphMask
}

// MeasureWrapped resolves ts against the shaper's FontManager and measures the
// text wrapped to maxWidth. A FontSize of 0 uses the default text size; when
// no system font is available it reports zero metrics rather than panicking.
func (s *Shaper) MeasureWrapped(text string, ts TextStyle, maxWidth float32) (w, h Pixels) {
	face := s.resolveFace(ts)
	if face == nil {
		return 0, 0
	}
	return s.Measure(text, face, maxWidth)
}

// ShapeWrapped resolves ts against the shaper's FontManager, wraps the text to
// maxWidth and attaches a rasterized mask to every glyph. A FontSize of 0 uses
// the default text size; when no system font is available it returns an empty
// slice rather than panicking.
func (s *Shaper) ShapeWrapped(text string, ts TextStyle, maxWidth float32) []ShapedLineOut {
	return s.ShapeWrappedScaled(text, ts, maxWidth, 1)
}

// ShapeWrappedScaled is ShapeWrapped with glyph masks rasterized at size*scale
// device pixels so they can be drawn 1:1 on a HiDPI target.
func (s *Shaper) ShapeWrappedScaled(text string, ts TextStyle, maxWidth, scale float32) []ShapedLineOut {
	face := s.resolveFace(ts)
	if face == nil {
		return []ShapedLineOut{}
	}
	lines := s.ShapeWrap(text, face, maxWidth)
	out := make([]ShapedLineOut, len(lines))
	for i, l := range lines {
		glyphs := make([]ShapedGlyphOut, len(l.Glyphs))
		for j, g := range l.Glyphs {
			glyphs[j] = ShapedGlyphOut{
				GlyphID: g.GlyphID,
				X:       g.X,
				Y:       g.Y,
				Advance: g.Advance,
				Cluster: g.Cluster,
				Mask:    s.GlyphMaskScaled(face, g.GlyphID, faceSize(face), scale),
			}
		}
		out[i] = ShapedLineOut{Width: l.Width, Ascent: l.Ascent, Descent: l.Descent, Glyphs: glyphs}
	}
	return out
}

// resolveFace maps a TextStyle to a face through the FontManager, substituting
// the default font size when ts.FontSize is 0. It returns nil when no face can
// be resolved (no FontManager, or ErrNoFont from the manager).
func (s *Shaper) resolveFace(ts TextStyle) *Font {
	if s == nil || s.fonts == nil {
		return nil
	}
	size := ts.FontSize
	if size <= 0 {
		size = DefaultTextStyle().FontSize
	}
	face, err := s.fonts.Face(ts.FontFamily, float32(size), ts.FontWeight)
	if err != nil {
		return nil
	}
	return face
}

// shapeRunes runs the HarfBuzz shaper over runes at the face's size. Script is
// fixed to Latin: per-run script detection is not implemented in this POC.
func (s *Shaper) shapeRunes(runes []rune, face *Font) shaping.Output {
	in := shaping.Input{
		Text:      runes,
		RunStart:  0,
		RunEnd:    len(runes),
		Direction: di.DirectionLTR,
		Face:      face.shapeFace,
		Size:      fixed.Int26_6(math.Round(float64(faceSize(face)) * 64)),
		Script:    language.Latin,
	}
	return s.hb.Shape(in)
}

// toShapedGlyphs converts HarfBuzz output into ShapedGlyphs whose X is the
// pen position (offsets applied) and whose Cluster is still a rune index; it
// also returns the final pen position in 26.6 fixed point.
func toShapedGlyphs(gs []shaping.Glyph) ([]ShapedGlyph, fixed.Int26_6) {
	out := make([]ShapedGlyph, len(gs))
	pen := fixed.Int26_6(0)
	for i, g := range gs {
		out[i] = ShapedGlyph{
			GlyphID: uint32(g.GlyphID),
			X:       px26(pen + g.XOffset),
			Y:       px26(g.YOffset),
			Advance: px26(g.Advance),
			Cluster: g.ClusterIndex,
		}
		pen += g.Advance
	}
	return out, pen
}

// px26 converts a 26.6 fixed-point value to Pixels.
func px26(v fixed.Int26_6) Pixels { return Pixels(v) / 64 }

// runeByteOffsets maps rune indices to UTF-8 byte offsets, with an extra
// entry at len(runes) holding the total byte length.
func runeByteOffsets(runes []rune) []int {
	off := make([]int, len(runes)+1)
	n := 0
	for i, r := range runes {
		off[i] = n
		n += len(string(r))
	}
	off[len(runes)] = n
	return off
}

// clusterSpan is a run of consecutive glyphs sharing one shaping cluster.
type clusterSpan struct {
	first, end int // glyph index range [first, end)
	width      Pixels
}

// groupClusters merges consecutive glyphs with equal cluster indices so that
// line breaks can never split a shaping cluster.
func groupClusters(gs []ShapedGlyph) []clusterSpan {
	if len(gs) == 0 {
		return nil
	}
	out := make([]clusterSpan, 0, len(gs))
	for i := 0; i < len(gs); {
		j := i + 1
		w := gs[i].Advance
		for j < len(gs) && gs[j].Cluster == gs[i].Cluster {
			w += gs[j].Advance
			j++
		}
		out = append(out, clusterSpan{first: i, end: j, width: w})
		i = j
	}
	return out
}

// lineBreaks reports the rune offsets at which a line break is allowed by the
// Unicode line-breaking algorithm, and which of those are mandatory (newline).
func lineBreaks(runes []rune) (allowed, mandatory map[int]bool) {
	allowed = make(map[int]bool)
	mandatory = make(map[int]bool)
	var seg segmenter.Segmenter
	seg.Init(runes)
	it := seg.LineIterator()
	prevMandatory := false
	for it.Next() {
		l := it.Line()
		if l.Offset > 0 {
			allowed[l.Offset] = true
			if prevMandatory {
				mandatory[l.Offset] = true
			}
		}
		prevMandatory = l.IsMandatoryBreak
	}
	return allowed, mandatory
}

// lastWhitespace returns the index of the last cluster span in [k, j) that
// starts with a space-like rune, or -1 when there is none.
func lastWhitespace(clusters []clusterSpan, gs []ShapedGlyph, runes []rune, k, j int) int {
	for c := j - 1; c >= k; c-- {
		r := runes[gs[clusters[c].first].Cluster]
		if r == ' ' || r == '\t' {
			return c
		}
	}
	return -1
}

// buildLine emits one ShapedLine from clusters [spans first..last), rebasing
// glyph X positions so the line starts at 0 and converting rune-index clusters
// to byte offsets.
func buildLine(spans []clusterSpan, gs []ShapedGlyph, byteOff []int, ascent, descent Pixels) ShapedLine {
	if len(spans) == 0 {
		return ShapedLine{Ascent: ascent, Descent: descent}
	}
	base := gs[spans[0].first].X
	line := ShapedLine{Ascent: ascent, Descent: descent}
	var width Pixels
	for _, sp := range spans {
		for i := sp.first; i < sp.end; i++ {
			g := gs[i]
			g.X -= base
			g.Cluster = byteOff[g.Cluster]
			line.Glyphs = append(line.Glyphs, g)
			width += g.Advance
		}
	}
	line.Width = width
	return line
}

// faceSize returns the ppem a face is bound to, falling back to the default
// text size for size-agnostic (raw loaded) fonts and for a nil face.
func faceSize(f *Font) float32 {
	if f != nil && f.size > 0 {
		return f.size
	}
	return float32(DefaultTextStyle().FontSize)
}

// lineMetrics returns the face's ascent and descent in pixels at its bound size.
func lineMetrics(f *Font) (ascent, descent Pixels) {
	if f == nil || f.sfntFont == nil {
		return 0, 0
	}
	size := faceSize(f)
	return Pixels(f.Ascent(size)), Pixels(f.Descent(size))
}

// rasterizeGlyph loads a glyph outline and converts the sfnt segment stream
// into vector.Rasterizer path calls, then reads back the anti-aliased alpha
// coverage. The path is translated so the mask's (0,0) is the top-left of the
// glyph ink's bounds minus a one-pixel pad in screen orientation (row 0 is
// the top), keeping anti-aliased edges unclipped.
func (s *Shaper) rasterizeGlyph(face *Font, glyphID uint32, size, scale float32) *GlyphMask {
	ppem := fixed.Int26_6(math.Round(float64(size) * float64(scale) * 64))
	segs, err := face.sfntFont.LoadGlyph(&s.buf, sfnt.GlyphIndex(glyphID), ppem, nil)
	if err != nil || len(segs) == 0 {
		return &GlyphMask{}
	}
	b := segs.Bounds()
	x0 := int(math.Floor(float64(b.Min.X)/64)) - 1
	y0 := int(math.Floor(float64(b.Min.Y)/64)) - 1
	x1 := int(math.Ceil(float64(b.Max.X)/64)) + 1
	y1 := int(math.Ceil(float64(b.Max.Y)/64)) + 1
	w, h := x1-x0, y1-y0
	if w <= 0 || h <= 0 {
		return &GlyphMask{}
	}
	r := vector.NewRasterizer(w, h)
	// sfnt.LoadGlyph already returns y-down coordinates (like
	// vector.Rasterizer), so no flip is needed: row 0 is the top of the ink.
	dx, dy := float32(-x0), float32(-y0)
	p := func(pt fixed.Point26_6) (float32, float32) {
		return float32(pt.X)/64 + dx, float32(pt.Y)/64 + dy
	}
	// sfnt emits no close operator: each MoveTo starts a new contour, so the
	// previous one must be closed explicitly for the fill rule to work.
	inContour := false
	for _, seg := range segs {
		switch seg.Op {
		case sfnt.SegmentOpMoveTo:
			if inContour {
				r.ClosePath()
			}
			ax, ay := p(seg.Args[0])
			r.MoveTo(ax, ay)
			inContour = true
		case sfnt.SegmentOpLineTo:
			bx, by := p(seg.Args[0])
			r.LineTo(bx, by)
		case sfnt.SegmentOpQuadTo:
			bx, by := p(seg.Args[0])
			cx, cy := p(seg.Args[1])
			r.QuadTo(bx, by, cx, cy)
		case sfnt.SegmentOpCubeTo:
			bx, by := p(seg.Args[0])
			cx, cy := p(seg.Args[1])
			ex, ey := p(seg.Args[2])
			r.CubeTo(bx, by, cx, cy, ex, ey)
		}
	}
	if inContour {
		r.ClosePath()
	}
	dst := image.NewAlpha(image.Rect(0, 0, w, h))
	r.Draw(dst, dst.Bounds(), image.NewUniform(color.Alpha{A: 0xff}), image.Point{})
	pix := dst.Pix
	if face.synthBold {
		pix = embolden(pix, w, h, max(1, int(math.Round(float64(scale)))))
	}
	return &GlyphMask{
		Pix:  pix,
		W:    w,
		H:    h,
		MinX: float32(b.Min.X) / 64 / scale,
		MinY: -float32(b.Max.Y) / 64 / scale,
		MaxX: float32(b.Max.X) / 64 / scale,
		MaxY: -float32(b.Min.Y) / 64 / scale,

		Scale: scale,
		X0:    x0,
		Y0:    y0,
	}
}

// embolden synthesizes a bold weight by smearing coverage one pixel to the
// right (the mask's one-pixel pad absorbs the growth).
func embolden(pix []uint8, w, h, n int) []uint8 {
	out := make([]uint8, len(pix))
	for y := 0; y < h; y++ {
		row := pix[y*w : (y+1)*w]
		for x := 0; x < w; x++ {
			v := row[x]
			for k := 1; k <= n && k <= x; k++ {
				if row[x-k] > v {
					v = row[x-k]
				}
			}
			out[y*w+x] = v
		}
	}
	return out
}
