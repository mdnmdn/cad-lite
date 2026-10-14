package gpui

import (
	"math"
	"sort"
	"strings"
	"unicode/utf8"
)

// Multiline makes the field a multi-line editor: text breaks at "\n" and
// wraps at the field's width, the text area grows from one line up to
// maxLines visible lines (values below 1 count as 1) and then scrolls.
// Shift+Enter inserts a newline; Enter submits when an OnSubmit callback is
// registered and inserts a newline otherwise. Up/Down move between visual
// lines and Home/End go to the ends of the visual line.
func (t *TextInput) Multiline(maxLines int) *TextInput {
	t.multiline = true
	t.maxLines = max(1, maxLines)
	return t
}

// inputRow is one visual line of a multi-line field: bytes [start, end) of
// the text (end excludes a hard newline), whether it wraps softly into the
// next row, and its caret stops with x relative to the row start.
type inputRow struct {
	start, end int
	soft       bool
	stops      []inputCaretStop
}

// inputRowsCache memoizes the rows for one text, style, width and measurer.
type inputRowsCache struct {
	text    string
	ts      TextStyle
	width   float32
	version uint64
	rows    []inputRow
	ok      bool
}

// layoutRows breaks the text into visual rows at hard newlines and at the
// shaper's soft wraps for width.
func (t *TextInput) layoutRows(ts TextStyle, width float32) []inputRow {
	if ts.FontSize <= 0 {
		ts = DefaultTextStyle()
	}
	c := &t.rowsCache
	if c.ok && c.text == t.text && c.ts == ts && c.width == width && c.version == t.measurerVersion {
		return c.rows
	}
	var rows []inputRow
	units, ps := 0, 0
	for {
		pe := strings.IndexByte(t.text[ps:], '\n')
		last := pe < 0
		if last {
			pe = len(t.text)
		} else {
			pe += ps
		}
		rows = t.paragraphRows(rows, ps, pe, &units, ts, width)
		if last {
			break
		}
		units++
		ps = pe + 1
	}
	*c = inputRowsCache{text: t.text, ts: ts, width: width, version: t.measurerVersion, rows: rows, ok: true}
	return rows
}

// paragraphRows appends the rows of the paragraph in bytes [ps, pe), whose
// first UTF-16 offset is *units; it advances *units to pe.
func (t *TextInput) paragraphRows(rows []inputRow, ps, pe int, units *int, ts TextStyle, width float32) []inputRow {
	para := t.text[ps:pe]
	starts := []int{ps}
	var lines []ShapedLineOut
	lineOf := []int{0}
	if t.measurer != nil && para != "" {
		lines = t.measurer.ShapeWrapped(para, ts, width)
		for i := 1; i < len(lines); i++ {
			if len(lines[i].Glyphs) == 0 {
				continue
			}
			s := lines[i].Glyphs[0].Cluster
			for _, g := range lines[i].Glyphs {
				s = min(s, g.Cluster)
			}
			if s += ps; s > starts[len(starts)-1] && s <= pe {
				starts = append(starts, s)
				lineOf = append(lineOf, i)
			}
		}
	}
	for k, start := range starts {
		end := pe
		if k+1 < len(starts) {
			end = starts[k+1]
		}
		stops := []inputCaretStop{{offset: *units, byteOffset: start}}
		for pos, r := range t.text[start:end] {
			*units++
			if r > 0xffff {
				*units++
			}
			_, size := utf8.DecodeRuneInString(t.text[start+pos:])
			stops = append(stops, inputCaretStop{offset: *units, byteOffset: start + pos + size})
		}
		for i := range stops {
			stops[i].x = Pixels(i) * ts.FontSize * 0.55
		}
		if k < len(lineOf) && lineOf[k] < len(lines) {
			l := lines[lineOf[k]]
			placeStops(stops, l.Glyphs, l.Width, ps, end-ps)
		}
		rows = append(rows, inputRow{start: start, end: end, soft: k+1 < len(starts), stops: stops})
	}
	return rows
}

// mlRows returns the rows at the laid-out text area's width.
func (t *TextInput) mlRows() []inputRow {
	w := float32(math.MaxFloat32)
	if t.view.Size.W > 0 {
		w = float32(t.view.Size.W) + wrapEpsilon
	}
	return t.layoutRows(t.textStyle, w)
}

// lineHeight is the field's resolved line height.
func (t *TextInput) lineHeight() Pixels {
	ts := t.textStyle
	if ts.FontSize <= 0 {
		ts = DefaultTextStyle()
	}
	return ts.ResolvedLineHeight()
}

// rowIndex returns the row holding byte offset b: the last row starting at
// or before it, so a soft-wrap boundary belongs to the next row.
func rowIndex(rows []inputRow, b int) int {
	return max(0, sort.Search(len(rows), func(i int) bool { return rows[i].start > b })-1)
}

// caretRowX returns the visual row and x (relative to the row start) of a
// UTF-16 offset.
func (t *TextInput) caretRowX(offset int) (int, Pixels) {
	offset = t.snapOffset(offset)
	rows := t.mlRows()
	r := rowIndex(rows, t.byteOffset(offset))
	for _, s := range rows[r].stops {
		if s.offset == offset {
			return r, s.x
		}
	}
	return r, 0
}

// nearest returns the stop offset on the row closest to x. A soft row's last
// stop is the next row's first and is left to that row.
func (r inputRow) nearest(x Pixels) int {
	stops := r.stops
	if r.soft && len(stops) > 1 {
		stops = stops[:len(stops)-1]
	}
	for i := 0; i+1 < len(stops); i++ {
		if x < (stops[i].x+stops[i+1].x)/2 {
			return stops[i].offset
		}
	}
	return stops[len(stops)-1].offset
}

// rowEnds returns the offsets Home and End move to on the caret's row.
func (t *TextInput) rowEnds(caret int) (home, end int) {
	rows := t.mlRows()
	r := rows[rowIndex(rows, t.byteOffset(caret))]
	home, end = r.stops[0].offset, r.stops[len(r.stops)-1].offset
	if r.soft && len(r.stops) > 1 {
		end = r.stops[len(r.stops)-2].offset
	}
	return home, end
}

// verticalMove returns the offset one visual row above (dir -1) or below
// (dir 1) the caret, at the goal x remembered across chained moves; past the
// first or last row it returns the text's start or end and forgets the goal.
func (t *TextInput) verticalMove(caret, dir int) int {
	row, x := t.caretRowX(caret)
	if !t.hasGoal {
		t.goalX, t.hasGoal = x, true
	}
	rows := t.mlRows()
	switch next := row + dir; {
	case next < 0:
		t.hasGoal = false
		return 0
	case next >= len(rows):
		t.hasGoal = false
		return utf16Length(t.text)
	default:
		return rows[next].nearest(t.goalX)
	}
}

// paragraphRange returns the UTF-16 range of the logical line (between
// newlines) holding offset.
func (t *TextInput) paragraphRange(offset int) IntRange {
	b := t.byteOffset(offset)
	start := strings.LastIndexByte(t.text[:b], '\n') + 1
	end := len(t.text)
	if i := strings.IndexByte(t.text[b:], '\n'); i >= 0 {
		end = b + i
	}
	return IntRange{Start: utf16Length(t.text[:start]), End: utf16Length(t.text[:end])}
}

// clampScroll keeps the vertical scroll within the rows' height.
func (t *TextInput) clampScroll(rows int) {
	limit := maxPixels(0, Pixels(rows)*t.lineHeight()-t.view.Size.H)
	t.scrollY = max(0, min(t.scrollY, limit))
}

// layoutMultiline records the text area and scrolls the caret row into view
// after an edit or caret move.
func (t *TextInput) layoutMultiline(view Bounds) {
	t.view = view
	rows := t.mlRows()
	if t.follow {
		t.follow = false
		row, _ := t.caretRowX(tiCaret(t))
		lh := t.lineHeight()
		top := Pixels(row) * lh
		if top < t.scrollY {
			t.scrollY = top
		}
		if top+lh > t.scrollY+view.Size.H {
			t.scrollY = top + lh - view.Size.H
		}
	}
	t.clampScroll(len(rows))
}

// onScrollWheel scrolls a multi-line field's text, consuming the event only
// when the text moved.
func (t *TextInput) onScrollWheel(e *ScrollWheelEvent, w *Window, _ *App) {
	prev := t.scrollY
	t.scrollY += e.DeltaY
	t.clampScroll(len(t.mlRows()))
	if t.scrollY != prev {
		w.Refresh()
		w.StopPropagation()
	}
}

// multilineIndexForPoint maps a point to the nearest stop on the visual row
// under it; origin is the text area's origin in the caller's coordinates.
func (t *TextInput) multilineIndexForPoint(p, origin Point) int {
	rows := t.mlRows()
	row := int(math.Floor(float64((p.Y - origin.Y + t.scrollY) / t.lineHeight())))
	row = max(0, min(row, len(rows)-1))
	return rows[row].nearest(p.X - origin.X)
}

// paintMultilineSelection paints the selection rectangles row by row and
// the caret, clipped to the text area.
func (e inputElement) paintMultilineSelection(w *Window) {
	t := e.ti
	lh := t.lineHeight()
	w.PushClip(t.view)
	defer w.PopClip()
	top := t.textOrigin.Y - t.scrollY
	sel := Blue500.Opacity(0.25)
	if t.selectionCol != nil {
		sel = *t.selectionCol
	}
	rows := t.mlRows()
	if s, en := t.sel.Range.Start, t.sel.Range.End; s != en {
		nl := t.textStyle.FontSize * 0.3
		if nl <= 0 {
			nl = DefaultTextStyle().FontSize * 0.3
		}
		for i, r := range rows {
			first, last := r.stops[0], r.stops[len(r.stops)-1]
			if en < first.offset || s > last.offset {
				continue
			}
			x0, x1 := first.x, last.x
			for _, st := range r.stops {
				if st.offset == s {
					x0 = st.x
				}
				if st.offset == en {
					x1 = st.x
				}
			}
			if !r.soft && i+1 < len(rows) && en > last.offset {
				x1 += nl
			}
			if x1 <= x0 {
				continue
			}
			w.PaintQuad(Quad{Bounds: Bounds{Origin: Point{X: t.textOrigin.X + x0, Y: top + Pixels(i)*lh}, Size: Size{W: x1 - x0, H: lh}}, Background: BackgroundSolid, Fill: sel})
		}
	}
	row, x := t.caretRowX(tiCaret(t))
	w.PaintQuad(Quad{Bounds: Bounds{Origin: Point{X: t.textOrigin.X + x, Y: top + Pixels(row)*lh}, Size: Size{W: 2, H: lh}}, Background: BackgroundSolid, Fill: t.textStyle.Color})
}

// inputLinesElement is a multi-line field's text area: a leaf whose height
// is its visual row count (capped at maxLines) times the line height, and
// which paints the rows scrolled by the field's offset.
type inputLinesElement struct{ t *TextInput }

// IntoElement returns the element itself.
func (e inputLinesElement) IntoElement() Element { return e }

// ID reports that the element declares no local id.
func (e inputLinesElement) ID() (ElementID, bool) { return ElementID{}, false }

// RequestLayout registers a leaf measured by the field's row layout.
func (e inputLinesElement) RequestLayout(w *Window, cx *App) (LayoutID, any) {
	t := e.t
	st := DefaultStyle()
	st.TextStyle = w.currentTextStyle()
	st.MinWidth = Px(0)
	node := &LayoutNode{Style: st, Text: " "}
	node.Measure = func(_ string, ts TextStyle, maxWidth float32) (float32, float32) {
		if ts.FontSize <= 0 {
			ts = DefaultTextStyle()
		}
		lh := float32(ts.ResolvedLineHeight())
		if maxWidth <= 0 {
			return 0, lh
		}
		width := maxWidth
		if width < float32(infPixels) {
			width += wrapEpsilon
		}
		rows := t.layoutRows(ts, width)
		var widest Pixels
		for _, r := range rows {
			widest = maxPixels(widest, r.stops[len(r.stops)-1].x)
		}
		return min(float32(widest), maxWidth), float32(min(len(rows), t.maxLines)) * lh
	}
	return w.registerLayoutNode(node), &textLayout{node: node}
}

// Prepaint registers nothing; the field's own Prepaint reads this leaf's
// layout.
func (e inputLinesElement) Prepaint(*Window, *App, ElementID, Bounds, any) any { return nil }

// Paint draws the text (or the placeholder) row by row, clipped to the
// leaf and shifted up by the field's scroll offset.
func (e inputLinesElement) Paint(w *Window, cx *App, id ElementID, bounds Bounds, layoutState, prepaintState any) {
	t := e.t
	if w == nil || w.text == nil {
		return
	}
	style := layoutStyle(layoutState, DefaultStyle)
	ts := w.paintTextStyle(style.TextStyle, 0)
	w.PushClip(bounds)
	defer w.PopClip()
	if t.text == "" {
		ts.Color = Gray500
		if t.placeholderCol != nil {
			ts.Color = *t.placeholderCol
		}
		paintShapedLines(w, bounds.Origin.X, bounds.Origin.Y, ts, shapeWrapped(w, t.placeholder, ts, math.MaxFloat32))
		return
	}
	lh := ts.ResolvedLineHeight()
	y := bounds.Origin.Y - t.scrollY
	width := float32(bounds.Size.W) + wrapEpsilon
	for _, para := range strings.Split(t.text, "\n") {
		lines := shapeWrapped(w, para, ts, width)
		if y+Pixels(len(lines))*lh > bounds.Origin.Y && y < bounds.Origin.Y+bounds.Size.H {
			paintShapedLines(w, bounds.Origin.X, y, ts, lines)
		}
		y += Pixels(max(1, len(lines))) * lh
	}
}

// normalizeNewlines turns "\r\n" and "\r" into "\n" for a multi-line field
// and every one of them into a space for a single-line field.
func (t *TextInput) normalizeNewlines(s string) string {
	if !strings.ContainsAny(s, "\r\n") {
		return s
	}
	s = strings.ReplaceAll(strings.ReplaceAll(s, "\r\n", "\n"), "\r", "\n")
	if !t.multiline {
		s = strings.ReplaceAll(s, "\n", " ")
	}
	return s
}
