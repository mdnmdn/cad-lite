package gpui

import "math"

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

// textLayout is Text's RequestLayout state: the node plus the wrap width the
// layout engine last measured with. Paint reuses that width so shaping matches
// the layout exactly.
type textLayout struct {
	node  *LayoutNode
	wrapW Pixels
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
		st.wrapW = Pixels(maxWidth)
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
	wrapW := float32(math.MaxFloat32)
	if st, ok := layoutState.(*textLayout); ok && st.wrapW > 0 {
		wrapW = float32(st.wrapW)
	}
	ts := t.resolvedStyle().TextStyle
	lines := w.text.ShapeWrapped(t.text, ts, wrapW)
	paintShapedLines(w, bounds.Origin.X, bounds.Origin.Y, ts, lines)
}

// measureText sizes text through the window's measurer, falling back to a
// crude estimator when no shaper is installed (e.g. the stub platform).
func measureText(w *Window, text string, ts TextStyle, maxWidth float32) (float32, float32) {
	if w != nil && w.text != nil {
		sw, sh := w.text.MeasureWrapped(text, ts, maxWidth)
		return float32(sw), float32(sh)
	}
	fs := float32(ts.FontSize)
	if fs <= 0 {
		fs = float32(DefaultTextStyle().FontSize)
	}
	return float32(len(text)) * fs * 0.55, float32(ts.ResolvedLineHeight())
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
			w.PaintGlyph(GlyphSprite{
				Bounds: Bounds{
					Origin: Point{
						X: x + g.X + Pixels(math.Floor(float64(m.MinX))) - 1,
						Y: baseY - Pixels(math.Ceil(float64(m.MaxY))) - 1,
					},
					Size: Size{W: Pixels(m.W), H: Pixels(m.H)},
				},
				Mask:  m,
				Color: ts.Color,
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
		st.wrapW = Pixels(maxWidth)
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
	wrapW := float32(math.MaxFloat32)
	if st, ok := layoutState.(*textLayout); ok && st.wrapW > 0 {
		wrapW = float32(st.wrapW)
	}
	x := bounds.Origin.X
	for _, r := range s.runs {
		if r.Text == "" {
			continue
		}
		ts := r.Style
		if ts.FontSize <= 0 {
			ts = DefaultTextStyle()
		}
		lines := w.text.ShapeWrapped(r.Text, ts, wrapW)
		paintShapedLines(w, x, bounds.Origin.Y, ts, lines)
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
