package gpui

import (
	"math"
	"strings"
)

// Text is a leaf element that renders a single run of styled text. It sizes
// itself through the window's TextMeasurer and paints one GlyphSprite per
// shaped glyph.
type Text struct {
	StyledMixin
	text string
}

// NewText returns a Text element showing s. It is named NewText rather than
// Text because Go does not allow a type and a package-level function to
// share a name.
func NewText(s string) *Text {
	return &Text{StyledMixin: NewStyledMixin(), text: s}
}

// Text sets the rendered string.
func (t *Text) Text(s string) *Text {
	t.text = s
	return t
}

// IntoElement satisfies IntoElement via the textElement adapter.
func (t *Text) IntoElement() Element { return textElement{t: t} }

// Str adapts a string for use as a child element, so Child(Str("hi")) works.
func Str(s string) IntoElement { return NewText(s) }

// resolvedStyle merges the refinement onto DefaultStyle.
func (t *Text) resolvedStyle() Style { return t.ResolvedStyle(DefaultStyle()) }

// textLayout is Text's RequestLayout state.
type textLayout struct {
	node *LayoutNode
}

// textElement adapts *Text to Element. It exists because *Text carries the
// fluent builder Text(string) *Text, which collides with Element's
// ID() (ElementID, bool) only in spirit — Text declares no local id — but the
// adapter keeps the Element implementation off the builder type, mirroring
// divElement.
type textElement struct{ t *Text }

// ID reports that the element declares no local id.
func (e textElement) ID() (ElementID, bool) { return ElementID{}, false }

// RequestLayout registers a leaf LayoutNode sized by the window's text
// measurer.
func (e textElement) RequestLayout(w *Window, cx *App) (LayoutID, any) {
	t := e.t
	node := &LayoutNode{Style: t.resolvedStyle(), Text: t.text}
	st := &textLayout{node: node}
	node.Measure = func(text string, ts TextStyle, maxWidth float32) (float32, float32) {
		return measureText(w, text, ts, maxWidth)
	}
	return w.registerLayoutNode(node), st
}

// Prepaint is a no-op: text leaves register no hitboxes or listeners.
func (e textElement) Prepaint(w *Window, cx *App, id ElementID, bounds Bounds, layoutState any) any {
	return nil
}

// Paint delegates to the Text.
func (e textElement) Paint(w *Window, cx *App, id ElementID, bounds Bounds, layoutState, prepaintState any) {
	e.t.paint(w, bounds, layoutState)
}

// paint emits one GlyphSprite per shaped glyph, plus a strikethrough rule
// when the style asks for one.
func (t *Text) paint(w *Window, bounds Bounds, layoutState any) {
	if w == nil || w.text == nil || t.text == "" {
		return
	}
	style := t.resolvedStyle()
	ts := style.TextStyle
	lines := shapeWrapped(w, t.text, ts, paintWrapWidth(bounds, style))
	paintShapedLines(w, bounds.Origin.X+style.Padding.Left, bounds.Origin.Y+style.Padding.Top, ts, lines)
}

const wrapEpsilon = 0.01

// paintWrapWidth is the wrap width text paints with: the laid-out content
// width, which is the width the layout engine last measured the leaf at. A
// zero-width box (never laid out) paints unwrapped. A small epsilon absorbs
// float32 rounding in bounds - padding so the last word never wraps at paint.
func paintWrapWidth(bounds Bounds, style Style) float32 {
	if bounds.Size.W <= 0 {
		return math.MaxFloat32
	}
	return float32(maxPixels(0, bounds.Size.W-style.Padding.Horizontal())) + wrapEpsilon
}

// measureText sizes text through the window's measurer, falling back to a
// crude estimator when no shaper is installed (e.g. the stub platform).
// maxWidth <= 0 requests the min-content size: the longest word's width and
// the height of wrapping at that width.
func measureText(w *Window, text string, ts TextStyle, maxWidth float32) (float32, float32) {
	if maxWidth <= 0 {
		var longest float32
		for _, word := range strings.Fields(text) {
			ww, _ := measureText(w, word, ts, math.MaxFloat32)
			longest = maxf32(longest, ww)
		}
		if longest <= 0 {
			return measureText(w, text, ts, math.MaxFloat32)
		}
		_, h := measureText(w, text, ts, longest)
		return longest, h
	}
	if w != nil && w.text != nil {
		sw, sh := w.text.MeasureWrapped(text, ts, maxWidth)
		return float32(sw), float32(sh)
	}
	return estimateText(text, ts, maxWidth)
}

// estimateText is the shaper-less fallback: a fixed advance per rune and
// greedy wrapping at spaces.
func estimateText(text string, ts TextStyle, maxWidth float32) (float32, float32) {
	fs := float32(ts.FontSize)
	if fs <= 0 {
		fs = float32(DefaultTextStyle().FontSize)
	}
	cw := fs * 0.55
	lh := float32(ts.ResolvedLineHeight())
	var widest, cur float32
	lines := 1
	for i, word := range strings.Fields(text) {
		ww := float32(len([]rune(word))) * cw
		switch {
		case i == 0:
			cur = ww
		case cur+cw+ww <= maxWidth:
			cur += cw + ww
		default:
			widest = maxf32(widest, cur)
			cur = ww
			lines++
		}
	}
	return maxf32(widest, cur), float32(lines) * lh
}

// shapeWrapped shapes through the window's text backend, rasterizing masks at
// the window scale when the backend supports it.
func shapeWrapped(w *Window, text string, ts TextStyle, maxWidth float32) []ShapedLineOut {
	if sc, ok := w.text.(interface {
		ShapeWrappedScaled(text string, ts TextStyle, maxWidth, scale float32) []ShapedLineOut
	}); ok {
		return sc.ShapeWrappedScaled(text, ts, maxWidth, w.scale)
	}
	return w.text.ShapeWrapped(text, ts, maxWidth)
}

// paintShapedLines draws shaped lines starting at (x, y), stacking lines
// vertically by the style's line height.
func paintShapedLines(w *Window, x, y Pixels, ts TextStyle, lines []ShapedLineOut) {
	lh := ts.ResolvedLineHeight()
	for _, line := range lines {
		baseY := y + line.Ascent
		for _, g := range line.Glyphs {
			m := g.Mask
			if m == nil || m.W <= 0 || m.H <= 0 || len(m.Pix) == 0 {
				continue
			}
			bounds := Bounds{
				Origin: Point{
					X: x + g.X + Pixels(math.Floor(float64(m.MinX))) - 1,
					Y: baseY - Pixels(math.Ceil(float64(m.MaxY))) - 1,
				},
				Size: Size{W: Pixels(m.W), H: Pixels(m.H)},
			}
			if k := Pixels(m.Scale); k > 0 {
				bounds = Bounds{
					Origin: Point{X: x + g.X + Pixels(m.X0)/k, Y: baseY + Pixels(m.Y0)/k},
					Size:   Size{W: Pixels(m.W) / k, H: Pixels(m.H) / k},
				}
			}
			w.PaintGlyph(GlyphSprite{
				Bounds: bounds,
				Mask:   m,
				Color:  ts.Color,
			})
		}
		if ts.Strikethrough && line.Width > 0 {
			th := Pixels(math.Max(1, float64(ts.FontSize)/14))
			w.PaintUnderline(Underline{
				Bounds: Bounds{
					Origin: Point{X: x, Y: baseY - line.Ascent*0.35 - th},
					Size:   Size{W: line.Width, H: th},
				},
				Color:     ts.Color,
				Thickness: th,
			})
		}
		y += lh
	}
}

// StyledText renders multiple runs of differing styles within one paragraph.
// POC limitation: each run wraps independently at the same width and runs
// never share line breaks; they are laid out left to right.
type StyledText struct {
	StyledMixin
	runs []TextRun
}

// TextRun is one styled run of text.
type TextRun struct {
	Text  string
	Style TextStyle
}

// NewStyledText returns a StyledText element from runs.
func NewStyledText(runs ...TextRun) *StyledText {
	return &StyledText{StyledMixin: NewStyledMixin(), runs: runs}
}

// resolvedStyle merges the refinement onto DefaultStyle.
func (s *StyledText) resolvedStyle() Style { return s.ResolvedStyle(DefaultStyle()) }

// IntoElement satisfies IntoElement via the styledTextElement adapter.
func (s *StyledText) IntoElement() Element { return styledTextElement{s: s} }

// styledTextElement adapts *StyledText to Element, mirroring textElement.
type styledTextElement struct{ s *StyledText }

// ID reports that the element declares no local id.
func (e styledTextElement) ID() (ElementID, bool) { return ElementID{}, false }

// RequestLayout registers a leaf LayoutNode whose measure concatenates the
// runs. The node's Text is set to the joined run texts so the layout engine
// treats it as a measurable leaf.
func (e styledTextElement) RequestLayout(w *Window, cx *App) (LayoutID, any) {
	s := e.s
	node := &LayoutNode{Style: s.resolvedStyle(), Text: s.joinedText()}
	st := &textLayout{node: node}
	node.Measure = func(text string, ts TextStyle, maxWidth float32) (float32, float32) {
		return measureRuns(w, s.runs, maxWidth)
	}
	return w.registerLayoutNode(node), st
}

// Prepaint is a no-op: text leaves register no hitboxes or listeners.
func (e styledTextElement) Prepaint(w *Window, cx *App, id ElementID, bounds Bounds, layoutState any) any {
	return nil
}

// Paint delegates to the StyledText.
func (e styledTextElement) Paint(w *Window, cx *App, id ElementID, bounds Bounds, layoutState, prepaintState any) {
	e.s.paint(w, bounds, layoutState)
}

// joinedText concatenates the run texts so the layout engine measures the
// node as a text leaf.
func (s *StyledText) joinedText() string {
	var n int
	for _, r := range s.runs {
		n += len(r.Text)
	}
	buf := make([]byte, 0, n)
	for _, r := range s.runs {
		buf = append(buf, r.Text...)
	}
	return string(buf)
}

// paint shapes each run independently at the layout wrap width and lays the
// runs out left to right.
func (s *StyledText) paint(w *Window, bounds Bounds, layoutState any) {
	if w == nil || w.text == nil {
		return
	}
	style := s.resolvedStyle()
	wrapW := paintWrapWidth(bounds, style)
	x := bounds.Origin.X + style.Padding.Left
	y := bounds.Origin.Y + style.Padding.Top
	for _, r := range s.runs {
		if r.Text == "" {
			continue
		}
		ts := r.Style
		if ts.FontSize <= 0 {
			ts = DefaultTextStyle()
		}
		lines := shapeWrapped(w, r.Text, ts, wrapW)
		paintShapedLines(w, x, y, ts, lines)
		rw := Pixels(0)
		for _, l := range lines {
			rw = maxPixels(rw, l.Width)
		}
		x += rw
	}
}

// measureRuns sizes each run independently at maxWidth: the runs flow left to
// right, so the width is the sum of the runs' widest lines and the height the
// tallest run.
func measureRuns(w *Window, runs []TextRun, maxWidth float32) (float32, float32) {
	var wsum, hmax float32
	for _, r := range runs {
		ts := r.Style
		if ts.FontSize <= 0 {
			ts = DefaultTextStyle()
		}
		rw, rh := measureText(w, r.Text, ts, maxWidth)
		wsum += rw
		hmax = maxf32(hmax, rh)
	}
	return wsum, hmax
}

// --- style-forwarding helpers, so a *Text-typed chain stays *Text -----------

// TextColor sets the text color.
func (t *Text) TextColor(c Rgba) *Text { t.StyledMixin.TextColor(c); return t }

// TextSize sets the font size.
func (t *Text) TextSize(px float32) *Text { t.StyledMixin.TextSize(px); return t }

// FontWeight sets the font weight.
func (t *Text) FontWeight(w int) *Text { t.StyledMixin.FontWeight(w); return t }

// TextStrikethrough strikes through the text.
func (t *Text) TextStrikethrough() *Text { t.StyledMixin.TextStrikethrough(); return t }

// FontFamily sets the font family.
func (t *Text) FontFamily(name string) *Text { t.StyledMixin.FontFamily(name); return t }

// TextLineHeight sets the line height.
func (t *Text) TextLineHeight(px float32) *Text { t.StyledMixin.TextLineHeight(px); return t }

// SizeW sets the width to a definite pixel length.
func (t *Text) SizeW(w float32) *Text { t.StyledMixin.SizeW(w); return t }

// SizeH sets the height to a definite pixel length.
func (t *Text) SizeH(h float32) *Text { t.StyledMixin.SizeH(h); return t }

// SizeFull fills both axes (see StyledMixin.SizeFull for the POC's 100%
// stand-in).
func (t *Text) SizeFull() *Text { t.StyledMixin.SizeFull(); return t }

// Flex1 grows and shrinks to consume the available main-axis space.
func (t *Text) Flex1() *Text { t.StyledMixin.Flex1(); return t }

// FlexGrow sets the flex-grow factor.
func (t *Text) FlexGrow(v float32) *Text { t.StyledMixin.FlexGrow(v); return t }

// MinW sets a minimum width.
func (t *Text) MinW(v float32) *Text { t.StyledMixin.MinW(v); return t }

// MinH sets a minimum height.
func (t *Text) MinH(v float32) *Text { t.StyledMixin.MinH(v); return t }

// MaxW sets a maximum width.
func (t *Text) MaxW(v float32) *Text { t.StyledMixin.MaxW(v); return t }

// MaxH sets a maximum height.
func (t *Text) MaxH(v float32) *Text { t.StyledMixin.MaxH(v); return t }

// Hidden removes the text from layout and paint (Display:none).
func (t *Text) Hidden() *Text { t.StyledMixin.Hidden(); return t }

// Visible undoes Hidden.
func (t *Text) Visible() *Text { t.StyledMixin.Visible(); return t }

// SizeWH is StyledMixin.SizeWH returning *Text to keep the chain.
func (t *Text) SizeWH(w, h float32) *Text { t.StyledMixin.SizeWH(w, h); return t }

// FlexShrink is StyledMixin.FlexShrink returning *Text to keep the chain.
func (t *Text) FlexShrink(v float32) *Text { t.StyledMixin.FlexShrink(v); return t }

// Flex is StyledMixin.Flex returning *Text to keep the chain.
func (t *Text) Flex() *Text { t.StyledMixin.Flex(); return t }

// FlexRow is StyledMixin.FlexRow returning *Text to keep the chain.
func (t *Text) FlexRow() *Text { t.StyledMixin.FlexRow(); return t }

// FlexCol is StyledMixin.FlexCol returning *Text to keep the chain.
func (t *Text) FlexCol() *Text { t.StyledMixin.FlexCol(); return t }

// Gap is StyledMixin.Gap returning *Text to keep the chain.
func (t *Text) Gap(v float32) *Text { t.StyledMixin.Gap(v); return t }

// P is StyledMixin.P returning *Text to keep the chain.
func (t *Text) P(v float32) *Text { t.StyledMixin.P(v); return t }

// Px_ is StyledMixin.Px_ returning *Text to keep the chain.
func (t *Text) Px_(v float32) *Text { t.StyledMixin.Px_(v); return t }

// Py_ is StyledMixin.Py_ returning *Text to keep the chain.
func (t *Text) Py_(v float32) *Text { t.StyledMixin.Py_(v); return t }

// Pt is StyledMixin.Pt returning *Text to keep the chain.
func (t *Text) Pt(v float32) *Text { t.StyledMixin.Pt(v); return t }

// Pr is StyledMixin.Pr returning *Text to keep the chain.
func (t *Text) Pr(v float32) *Text { t.StyledMixin.Pr(v); return t }

// Pb is StyledMixin.Pb returning *Text to keep the chain.
func (t *Text) Pb(v float32) *Text { t.StyledMixin.Pb(v); return t }

// Pl is StyledMixin.Pl returning *Text to keep the chain.
func (t *Text) Pl(v float32) *Text { t.StyledMixin.Pl(v); return t }

// M is StyledMixin.M returning *Text to keep the chain.
func (t *Text) M(v float32) *Text { t.StyledMixin.M(v); return t }

// Bg sets the background fill (Rgba, Hsla or a Background).
func (t *Text) Bg(f Fill) *Text { t.StyledMixin.Bg(f); return t }

// Rounded is StyledMixin.Rounded returning *Text to keep the chain.
func (t *Text) Rounded(px float32) *Text { t.StyledMixin.Rounded(px); return t }

// RoundedAll is StyledMixin.RoundedAll returning *Text to keep the chain.
func (t *Text) RoundedAll(tl, tr, br, bl float32) *Text {
	t.StyledMixin.RoundedAll(tl, tr, br, bl)
	return t
}

// Border is StyledMixin.Border returning *Text to keep the chain.
func (t *Text) Border(w float32, c Rgba) *Text { t.StyledMixin.Border(w, c); return t }

// ItemsStart is StyledMixin.ItemsStart returning *Text to keep the chain.
func (t *Text) ItemsStart() *Text { t.StyledMixin.ItemsStart(); return t }

// ItemsCenter is StyledMixin.ItemsCenter returning *Text to keep the chain.
func (t *Text) ItemsCenter() *Text { t.StyledMixin.ItemsCenter(); return t }

// ItemsEnd is StyledMixin.ItemsEnd returning *Text to keep the chain.
func (t *Text) ItemsEnd() *Text { t.StyledMixin.ItemsEnd(); return t }

// ItemsStretch is StyledMixin.ItemsStretch returning *Text to keep the chain.
func (t *Text) ItemsStretch() *Text { t.StyledMixin.ItemsStretch(); return t }

// JustifyStart is StyledMixin.JustifyStart returning *Text to keep the chain.
func (t *Text) JustifyStart() *Text { t.StyledMixin.JustifyStart(); return t }

// JustifyCenter is StyledMixin.JustifyCenter returning *Text to keep the chain.
func (t *Text) JustifyCenter() *Text { t.StyledMixin.JustifyCenter(); return t }

// JustifyEnd is StyledMixin.JustifyEnd returning *Text to keep the chain.
func (t *Text) JustifyEnd() *Text { t.StyledMixin.JustifyEnd(); return t }

// JustifyBetween is StyledMixin.JustifyBetween returning *Text to keep the chain.
func (t *Text) JustifyBetween() *Text { t.StyledMixin.JustifyBetween(); return t }

// JustifyAround is StyledMixin.JustifyAround returning *Text to keep the chain.
func (t *Text) JustifyAround() *Text { t.StyledMixin.JustifyAround(); return t }

// OverflowHidden is StyledMixin.OverflowHidden returning *Text to keep the chain.
func (t *Text) OverflowHidden() *Text { t.StyledMixin.OverflowHidden(); return t }

// OverflowScroll is StyledMixin.OverflowScroll returning *Text to keep the chain.
func (t *Text) OverflowScroll() *Text { t.StyledMixin.OverflowScroll(); return t }
