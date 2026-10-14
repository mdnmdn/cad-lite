package gpui

import (
	"math"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"
)

// TextInput is a ready-to-use single-line text field view. It implements
// PlatformInputHandler internally so the window's key dispatch routes
// printable characters straight into it.
//
// POC deviation: TextInput is not an Entity[T]. It is a plain struct the
// root view owns, so the view re-renders it by refreshing the window after
// mutations (every mutator calls Window.Refresh), replacing GPUI's
// entity-per-textbox design.
type TextInput struct {
	text        string
	sel         UTF16Selection
	marked      IntRange
	hasMarked   bool
	placeholder string

	focus                           *FocusHandle
	measurer                        TextMeasurer
	win                             *Window
	app                             *App
	bounds, content                 Bounds
	textOrigin                      Point
	textStyle                       TextStyle
	elementID                       GlobalElementID
	metricsText                     string
	metricsStyle                    TextStyle
	measurerVersion, metricsVersion uint64
	metrics                         []inputCaretStop

	style          func(*Div) *Div
	placeholderCol *Rgba
	selectionCol   *Rgba

	onSubmit []func(string, *Window, *App)
	onChange []func(string, *Window, *App)
	onEscape []func(*Window, *App)

	// Multi-line state: the row cap, the laid-out text area, the vertical
	// scroll, whether the next prepaint scrolls the caret into view, and
	// the x that chained Up/Down moves keep.
	multiline bool
	maxLines  int
	view      Bounds
	scrollY   Pixels
	follow    bool
	goalX     Pixels
	hasGoal   bool
	rowsCache inputRowsCache
}

// NewTextInput returns an empty single-line text field showing placeholder
// until text is entered.
func NewTextInput(placeholder string) *TextInput {
	return &TextInput{placeholder: placeholder}
}

// Text returns the current value.
func (t *TextInput) Text() string { return t.text }

// SetText replaces the value and collapses the selection to the end.
func (t *TextInput) SetText(s string) {
	t.text = s
	t.sel = UTF16Selection{Range: IntRange{Start: utf16Length(s), End: utf16Length(s)}}
	t.hasMarked = false
	t.follow, t.hasGoal = true, false
	t.cancelSelectionDrag()
	if t.win != nil {
		t.win.Refresh()
	}
}

// SetPlaceholder replaces the placeholder text and refreshes the window if the
// field has been rendered.
func (t *TextInput) SetPlaceholder(s string) {
	t.placeholder = s
	if t.win != nil {
		t.win.Refresh()
	}
}

// FocusHandle returns the handle the field renders with, allocating it from
// the App on first use. It is only valid after Render has run at least once.
func (t *TextInput) FocusHandle() *FocusHandle { return t.focus }

// Style registers a function applied to the field's box after the defaults
// (white background, 1px gray border, rounded 6, padding 8), so it can
// override any of them: background, border, padding, text colour and size,
// and a focused border through Div.Focus. The text colour set here also
// colours the caret. Calling Style again replaces the previous function.
func (t *TextInput) Style(f func(*Div) *Div) *TextInput {
	t.style = f
	return t
}

// PlaceholderColor sets the colour of the placeholder text (default Gray500).
func (t *TextInput) PlaceholderColor(c Rgba) *TextInput {
	t.placeholderCol = &c
	return t
}

// SelectionColor sets the fill of the selection highlight (default Blue500 at
// 25% opacity).
func (t *TextInput) SelectionColor(c Rgba) *TextInput {
	t.selectionCol = &c
	return t
}

// OnSubmit registers a callback invoked with the current text when the user
// presses Enter while the field is focused.
func (t *TextInput) OnSubmit(f func(string, *Window, *App)) *TextInput {
	t.onSubmit = append(t.onSubmit, f)
	return t
}

// OnEscape registers a callback invoked when the user presses Escape while
// the field is focused. While any is registered Escape keeps focus instead
// of blurring the field (a command line stays focused, for instance).
func (t *TextInput) OnEscape(f func(*Window, *App)) *TextInput {
	t.onEscape = append(t.onEscape, f)
	return t
}

// OnChange registers a callback invoked whenever the text changes through
// input dispatch.
func (t *TextInput) OnChange(f func(string, *Window, *App)) *TextInput {
	t.onChange = append(t.onChange, f)
	return t
}

// Render returns the element to draw: a focusable box in the "textbox" key
// context containing the text or the placeholder, plus a caret when focused.
func (t *TextInput) Render(w *Window, cx *App) IntoElement {
	if t.focus == nil && cx != nil {
		t.focus = cx.NewFocusHandle()
	}
	if w != nil {
		t.measurer = w.text
		t.measurerVersion = w.textVersion
		t.win = w
	}
	if cx != nil {
		t.app = cx
	}
	if t.multiline {
		return t.renderMultiline()
	}
	var label IntoElement
	if t.text == "" {
		ph := Gray500
		if t.placeholderCol != nil {
			ph = *t.placeholderCol
		}
		label = NewText(inputDisplayText(t.placeholder)).TextColor(ph).FlexShrink(0)
	} else {
		label = NewText(inputDisplayText(t.text)).FlexShrink(0)
	}
	div := NewDiv().
		FlexRow().
		ItemsCenter().
		P(8).
		Bg(White).
		Rounded(6).
		Border(1, Gray300).
		TrackFocus(t.focus).
		KeyContext("textbox").
		OverflowHidden().
		OnMouseDown(t.onMouseDown).
		OnKeyDown(t.onKeyDown).
		Child(label)
	if t.style != nil {
		if d := t.style(div); d != nil {
			div = d
		}
	}
	return inputElement{div: div, ti: t}
}

// renderMultiline builds a multi-line field: the same box laid out as a
// column around the text area leaf, with wheel scrolling.
func (t *TextInput) renderMultiline() IntoElement {
	div := NewDiv().
		FlexCol().
		P(8).
		Bg(White).
		Rounded(6).
		Border(1, Gray300).
		TrackFocus(t.focus).
		KeyContext("textbox").
		OverflowHidden().
		OnMouseDown(t.onMouseDown).
		OnKeyDown(t.onKeyDown).
		OnScrollWheel(t.onScrollWheel).
		Child(inputLinesElement{t: t})
	if t.style != nil {
		if d := t.style(div); d != nil {
			div = d
		}
	}
	return inputElement{div: div, ti: t}
}

// inputElement adapts TextInput to Element. The window sees a leaf whose
// children are the wrapped Div's children (the text label), while Prepaint
// registers the input handler and Paint adds the caret on top of the Div's
// own background and border.
type inputElement struct {
	div *Div
	ti  *TextInput
}

// IntoElement returns the element itself.
func (e inputElement) IntoElement() Element { return e }

// ID reports that the element declares no local id.
func (e inputElement) ID() (ElementID, bool) { return ElementID{}, false }

// RequestLayout delegates to the div, whose layout state carries the child
// layout ids the window's walk pairs with elementChildren.
func (e inputElement) RequestLayout(w *Window, cx *App) (LayoutID, any) {
	return e.div.requestLayout(w, cx)
}

// elementChildren exposes the div's children to the window's tree walk.
func (e inputElement) elementChildren() []AnyElement { return e.div.elementChildren() }

// Prepaint delegates to the div, then registers the input handler when the
// field holds window focus — the frame's last handler is the focused field.
func (e inputElement) Prepaint(w *Window, cx *App, id ElementID, bounds Bounds, layoutState any) any {
	st := e.div.prepaint(w, cx, bounds, layoutState)
	t := e.ti
	t.bounds, t.content = bounds, bounds.Inset(e.div.resolvedStyle().Padding)
	t.textOrigin, t.textStyle = t.content.Origin, DefaultTextStyle()
	t.elementID = w.currentElementID
	if dl, ok := layoutState.(*divLayout); ok && dl.node != nil {
		t.textStyle = dl.node.Style.TextStyle
		if len(dl.node.Children) > 0 {
			label := dl.node.Children[0]
			t.textOrigin = label.Rect.Translate(bounds.Origin).Origin
			if t.multiline {
				t.layoutMultiline(Bounds{Origin: t.textOrigin, Size: label.Rect.Size})
			}
		}
	}
	if ps, ok := st.(*divPrepaint); ok {
		w.nextFrame.mouseListeners[ps.hitbox].textInput = t
	}

	if e.ti.focus != nil && e.ti.focus.IsFocused(w) {
		w.HandleInput(e.ti)
	}
	return st
}

// Paint delegates to the div, then draws the caret when focused.
func (e inputElement) Paint(w *Window, cx *App, id ElementID, bounds Bounds, layoutState, prepaintState any) {
	e.div.paint(w, bounds, prepaintState)
	if e.ti.focus != nil && e.ti.focus.IsFocused(w) {
		if e.ti.multiline {
			e.paintMultilineSelection(w)
		} else {
			e.paintSelection(w)
		}
	}
}

func inputDisplayText(text string) string {
	return strings.ReplaceAll(strings.ReplaceAll(text, "\r", " "), "\n", " ")
}

type inputCaretStop struct {
	offset, byteOffset int
	x                  Pixels
}

func (t *TextInput) caretStops() []inputCaretStop {
	ts := t.textStyle
	if ts.FontSize <= 0 {
		ts = DefaultTextStyle()
	}
	if t.metrics != nil && t.metricsText == t.text && t.metricsStyle == ts && t.metricsVersion == t.measurerVersion {
		return t.metrics
	}
	stops := []inputCaretStop{{}}
	units := 0
	for pos, r := range t.text {
		units++
		if r > 0xffff {
			units++
		}
		_, bytes := utf8.DecodeRuneInString(t.text[pos:])
		stops = append(stops, inputCaretStop{offset: units, byteOffset: pos + bytes})
	}
	for i := range stops {
		stops[i].x = Pixels(i) * ts.FontSize * 0.55
	}
	if t.measurer != nil && t.text != "" {
		lines := t.measurer.ShapeWrapped(inputDisplayText(t.text), ts, float32(math.MaxFloat32))
		if len(lines) > 0 {
			placeStops(stops, lines[0].Glyphs, lines[0].Width, 0, len(t.text))
		}
	}
	t.metricsText, t.metricsStyle, t.metrics = t.text, ts, stops
	t.metricsVersion = t.measurerVersion
	return stops
}

// caretX is the x of offset relative to the start of its (visual) line.
func (t *TextInput) caretX(offset int) Pixels {
	if t.multiline {
		_, x := t.caretRowX(offset)
		return x
	}
	return t.advance(offset)
}

// placeStops sets the x of each stop from one shaped line's glyphs. Glyph
// clusters and end are byte offsets relative to base, the byte offset of the
// line's first character in the text; stops inside a multi-rune cluster are
// spread evenly across it, and end sits at width.
func placeStops(stops []inputCaretStop, glyphs []ShapedGlyphOut, width Pixels, base, end int) {
	if len(glyphs) == 0 {
		return
	}
	starts := make(map[int]Pixels)
	var pen Pixels
	for _, g := range glyphs {
		if _, ok := starts[g.Cluster]; !ok {
			starts[g.Cluster] = pen
		}
		pen += g.Advance
	}
	keys := make([]int, 0, len(starts)+1)
	for key := range starts {
		keys = append(keys, key)
	}
	starts[end] = width
	keys = append(keys, end)
	sort.Ints(keys)
	firstIndex := 0
	for k := 0; k+1 < len(keys); k++ {
		first, last := keys[k], keys[k+1]
		for firstIndex < len(stops) && stops[firstIndex].byteOffset-base < first {
			firstIndex++
		}
		lastIndex := firstIndex
		for lastIndex+1 < len(stops) && stops[lastIndex+1].byteOffset-base <= last {
			lastIndex++
		}
		for i := firstIndex; i <= lastIndex && i < len(stops); i++ {
			x := starts[first]
			if lastIndex > firstIndex {
				x += (starts[last] - x) * Pixels(i-firstIndex) / Pixels(lastIndex-firstIndex)
			}
			stops[i].x = x
		}
		firstIndex = lastIndex
	}
}

func (t *TextInput) advance(offset int) Pixels {
	offset = t.snapOffset(offset)
	for _, stop := range t.caretStops() {
		if stop.offset == offset {
			return stop.x
		}
	}
	return 0
}

func (e inputElement) paintSelection(w *Window) {
	t := e.ti
	lh := t.textStyle.ResolvedLineHeight()
	w.PushClip(t.content)
	defer w.PopClip()
	top := t.textOrigin.Y
	sel := Blue500.Opacity(0.25)
	if t.selectionCol != nil {
		sel = *t.selectionCol
	}
	if t.sel.Range.Start != t.sel.Range.End {
		start, end := t.advance(t.sel.Range.Start), t.advance(t.sel.Range.End)
		w.PaintQuad(Quad{Bounds: Bounds{Origin: Point{X: t.textOrigin.X + start, Y: top}, Size: Size{W: maxPixels(0, end-start), H: lh}}, Background: BackgroundSolid, Fill: sel})
	}
	w.PaintQuad(Quad{Bounds: Bounds{Origin: Point{X: t.textOrigin.X + t.advance(tiCaret(t)), Y: top}, Size: Size{W: 2, H: lh}}, Background: BackgroundSolid, Fill: t.textStyle.Color})
}

func (t *TextInput) anchor() int {
	if t.sel.Reversed {
		return t.sel.Range.End
	}
	return t.sel.Range.Start
}

func (t *TextInput) selectTo(anchor, caret int) {
	anchor, caret = t.snapOffset(anchor), t.snapOffset(caret)
	r := IntRange{Start: anchor, End: caret}
	if r.Start > r.End {
		r.Start, r.End = r.End, r.Start
	}
	sel := UTF16Selection{Range: r, Reversed: caret < anchor}
	if t.sel != sel {
		t.sel = sel
		t.follow = true
		if t.win != nil {
			t.win.Refresh()
		}
	}
}

func (t *TextInput) runeBefore(offset int) int {
	b := t.byteOffset(t.snapOffset(offset))
	if b == 0 {
		return 0
	}
	_, size := utf8.DecodeLastRuneInString(t.text[:b])
	return utf16Length(t.text[:b-size])
}

func (t *TextInput) runeAfter(offset int) int {
	b := t.byteOffset(t.snapOffset(offset))
	if b == len(t.text) {
		return utf16Length(t.text)
	}
	_, size := utf8.DecodeRuneInString(t.text[b:])
	return utf16Length(t.text[:b+size])
}

func (t *TextInput) onMouseDown(e *MouseDownEvent, w *Window, _ *App) {
	if e.Button != MouseLeft {
		return
	}
	t.focus.Focus(w)
	t.hasGoal = false
	index, _ := t.CharacterIndexForPoint(e.Position, t.bounds)
	mode := uint8(1)
	r := IntRange{Start: index, End: index}
	if e.ClickCount >= 3 {
		mode, r = 3, IntRange{End: utf16Length(t.text)}
		if t.multiline {
			r = t.paragraphRange(index)
		}
	} else if e.ClickCount == 2 {
		mode = 2
		if index > 0 && e.Position.X-t.textOrigin.X < t.caretX(index) {
			index = t.runeBefore(index)
		}
		r = t.wordRange(index)
	} else if e.Modifiers&Shift != 0 {
		t.selectTo(t.anchor(), index)
		r.Start = t.anchor()
	} else {
		t.selectTo(index, index)
	}
	if mode != 1 {
		t.selectTo(r.Start, r.End)
	}
	w.inputSelection = &inputSelectionDrag{input: t, elementKey: globalIDKey(t.elementID), bounds: t.bounds, anchor: r.Start, anchorRange: r, mode: mode}
}

func inputWordClass(r rune) uint8 {
	if unicode.IsLetter(r) || unicode.IsNumber(r) || unicode.IsMark(r) || r == '_' {
		return 1
	}
	if unicode.IsSpace(r) {
		return 2
	}
	return 3
}

func (t *TextInput) wordRange(offset int) IntRange {
	offset = t.snapOffset(offset)
	if offset == utf16Length(t.text) {
		offset = t.runeBefore(offset)
	}
	start := t.byteOffset(offset)
	if start == len(t.text) {
		return IntRange{}
	}
	r, size := utf8.DecodeRuneInString(t.text[start:])
	end := start + size
	class := inputWordClass(r)
	for start > 0 {
		r, size = utf8.DecodeLastRuneInString(t.text[:start])
		if inputWordClass(r) != class {
			break
		}
		start -= size
	}
	for end < len(t.text) {
		r, size = utf8.DecodeRuneInString(t.text[end:])
		if inputWordClass(r) != class {
			break
		}
		end += size
	}
	return IntRange{Start: utf16Length(t.text[:start]), End: utf16Length(t.text[:end])}
}

type inputSelectionDrag struct {
	input       *TextInput
	elementKey  string
	bounds      Bounds
	anchor      int
	anchorRange IntRange
	mode        uint8
}

func (t *TextInput) cancelSelectionDrag() {
	if t.win != nil && t.win.inputSelection != nil && t.win.inputSelection.input == t {
		t.win.inputSelection = nil
	}
}

func (w *Window) reconcileInputSelection(frame *Frame) {
	d := w.inputSelection
	if d == nil {
		return
	}
	if d.input.focus == nil || !d.input.focus.IsFocused(w) {
		w.inputSelection = nil
		return
	}
	for i := range frame.mouseListeners {
		l := &frame.mouseListeners[i]
		if l.textInput == d.input && globalIDKey(l.elementID) == d.elementKey && l.Bounds == d.bounds && !frame.hitboxes.boxes[i].Bounds.IsEmpty() {
			return
		}
	}
	w.inputSelection = nil
}

func (w *Window) moveInputSelection(p Point) {
	w.reconcileInputSelection(&w.renderedFrame)
	d := w.inputSelection
	if d == nil {
		return
	}
	t := d.input
	caret, _ := t.CharacterIndexForPoint(p, t.bounds)
	switch d.mode {
	case 2:
		r := t.wordRange(caret)
		if caret < d.anchorRange.Start {
			t.selectTo(d.anchorRange.End, r.Start)
		} else if caret >= d.anchorRange.End {
			t.selectTo(d.anchorRange.Start, r.End)
		} else {
			t.selectTo(d.anchorRange.Start, d.anchorRange.End)
		}
	case 3:
		t.selectTo(d.anchorRange.Start, d.anchorRange.End)
	default:
		t.selectTo(d.anchor, caret)
	}
}

// onKeyDown handles the editing keys. Printable characters never reach here:
// the window routes them to ReplaceTextInRange before the key path runs.
func (t *TextInput) onKeyDown(e *KeyDownEvent, w *Window, cx *App) {
	if t.focus == nil || !t.focus.IsFocused(w) {
		return
	}
	caret := tiCaret(t)
	key := e.Keystroke.Key
	vertical := t.multiline && (key == "up" || key == "down")
	if !vertical {
		t.hasGoal = false
	}
	if vertical || key == "left" || key == "right" || key == "home" || key == "end" || key == "backspace" || key == "delete" {
		t.cancelSelectionDrag()
	}
	switch key {
	case "enter":
		if t.multiline && (e.Keystroke.Modifiers&Shift != 0 || len(t.onSubmit) == 0) {
			t.cancelSelectionDrag()
			t.ReplaceTextInRange(&t.sel.Range, "\n")
			w.StopPropagation()
			return
		}
		for _, f := range t.onSubmit {
			f(t.text, w, cx)
		}
		w.StopPropagation()
	case "up", "down":
		if !vertical {
			return
		}
		anchor := t.anchor()
		dir := 1
		if key == "up" {
			dir = -1
		}
		next := t.verticalMove(caret, dir)
		if e.Keystroke.Modifiers&Shift == 0 {
			anchor = next
		}
		t.selectTo(anchor, next)
		w.StopPropagation()
	case "escape":
		if len(t.onEscape) == 0 {
			t.focus.Blur(w)
		}
		for _, f := range t.onEscape {
			f(w, cx)
		}
		w.StopPropagation()
	case "backspace":
		if t.sel.Range.Start != t.sel.Range.End {
			t.replaceSelection("", w)
		} else if caret > 0 {
			end := t.byteOffset(caret)
			start := t.byteOffset(t.runeBefore(caret))
			t.replaceRange(start, end, "", w)
		}
		w.StopPropagation()
	case "delete":
		if t.sel.Range.Start != t.sel.Range.End {
			t.replaceSelection("", w)
		} else {
			end := t.byteOffset(caret)
			if end < len(t.text) {
				_, size := utf8.DecodeRuneInString(t.text[end:])
				t.replaceRange(end, end+size, "", w)
			}
		}
		w.StopPropagation()
	case "left", "right", "home", "end":
		anchor, next := t.anchor(), caret
		extend := e.Keystroke.Modifiers&Shift != 0
		switch key {
		case "home":
			next = 0
			if t.multiline {
				next, _ = t.rowEnds(caret)
			}
		case "end":
			next = utf16Length(t.text)
			if t.multiline {
				_, next = t.rowEnds(caret)
			}
		case "left":
			if !extend && t.sel.Range.Start != t.sel.Range.End {
				next = t.sel.Range.Start
			} else {
				next = t.runeBefore(caret)
			}
		case "right":
			if !extend && t.sel.Range.Start != t.sel.Range.End {
				next = t.sel.Range.End
			} else {
				next = t.runeAfter(caret)
			}
		}
		if !extend {
			anchor = next
		}
		t.selectTo(anchor, next)
		w.StopPropagation()
	}
}

// replaceSelection deletes the current selection and inserts text.
func (t *TextInput) replaceSelection(text string, w *Window) {
	if t.sel.IsNil {
		return
	}
	s := t.byteOffset(t.sel.Range.Start)
	e := t.byteOffset(t.sel.Range.End)
	t.replaceRange(s, e, text, w)
}

// replaceRange replaces the byte range [start, end) with text and moves the
// caret to the end of the inserted text.
func (t *TextInput) replaceRange(start, end int, text string, w *Window) {
	if start < 0 {
		start = 0
	}
	if end > len(t.text) {
		end = len(t.text)
	}
	if start > end {
		start, end = end, start
	}
	t.cancelSelectionDrag()
	t.text = t.text[:start] + text + t.text[end:]
	caret := utf16Length(t.text[:start]) + utf16Length(text)
	t.sel = UTF16Selection{Range: IntRange{Start: caret, End: caret}}
	t.hasMarked = false
	t.follow, t.hasGoal = true, false
	t.changed(w)
}

// changed fires the change callbacks and asks for a redraw.
func (t *TextInput) changed(w *Window) {
	if w == nil {
		w = t.win
	}
	for _, f := range t.onChange {
		f(t.text, w, t.app)
	}
	if w != nil {
		w.Refresh()
	}
}

// tiCaret returns the caret's UTF-16 offset: the selection start when reversed
// (the caret end), otherwise the selection end.
func tiCaret(t *TextInput) int {
	if t.sel.IsNil {
		return 0
	}
	if t.sel.Reversed {
		return t.sel.Range.Start
	}
	return t.sel.Range.End
}

func (t *TextInput) snapOffset(offset int) int {
	offset = max(0, min(offset, utf16Length(t.text)))
	units := 0
	for _, r := range t.text {
		next := units + 1
		if r > 0xffff {
			next++
		}
		if next > offset {
			return units
		}
		units = next
	}
	return units
}

func (t *TextInput) byteOffset(offset int) int {
	offset = t.snapOffset(offset)
	units := 0
	for pos, r := range t.text {
		if units == offset {
			return pos
		}
		units++
		if r > 0xffff {
			units++
		}
	}
	return len(t.text)
}

// utf16Length returns s's length in UTF-16 code units.
func utf16Length(s string) int {
	n := 0
	for _, r := range s {
		if r > 0xFFFF {
			n += 2
		} else {
			n++
		}
	}
	return n
}

// --- PlatformInputHandler ---------------------------------------------------

// TextForRange returns the text in the given UTF-16 range.
func (t *TextInput) TextForRange(rng IntRange) (string, bool) {
	if rng.Start < 0 || rng.End < rng.Start || rng.End > utf16Length(t.text) || t.snapOffset(rng.Start) != rng.Start || t.snapOffset(rng.End) != rng.End {
		return "", false
	}
	s, e := t.byteOffset(rng.Start), t.byteOffset(rng.End)
	return t.text[s:e], true
}

// SelectedTextRange returns the current selection; IsNil marks "no
// selection" (caret only).
func (t *TextInput) SelectedTextRange() (UTF16Selection, bool) {
	if t.sel.IsNil {
		return UTF16Selection{}, false
	}
	return t.sel, true
}

// MarkedTextRange returns the in-progress composing range; the POC has no
// IME composition, so it always reports none.
func (t *TextInput) MarkedTextRange() (IntRange, bool) {
	if !t.hasMarked {
		return IntRange{}, false
	}
	return t.marked, true
}

// UnmarkText commits any marked text; a no-op in the POC.
func (t *TextInput) UnmarkText() { t.hasMarked = false }

// ReplaceTextInRange replaces the given range (nil range = insert at caret)
// with text and moves the caret to the end of the inserted text.
func (t *TextInput) ReplaceTextInRange(rng *IntRange, text string) {
	text = t.normalizeNewlines(text)
	if rng == nil {
		caret := tiCaret(t)
		s := t.byteOffset(caret)
		t.replaceRange(s, s, text, nil)
		return
	}
	s := t.byteOffset(rng.Start)
	e := t.byteOffset(rng.End)
	t.replaceRange(s, e, text, nil)
}

// BoundsForRange returns the shaped caret rect at the normalized range start.
func (t *TextInput) BoundsForRange(rng IntRange, elementBounds Bounds) (Bounds, bool) {
	ts := t.textStyle
	if ts.FontSize <= 0 {
		ts = DefaultTextStyle()
	}
	origin := t.textOrigin
	if t.bounds.Size == (Size{}) {
		origin = Point{X: elementBounds.Origin.X + 8, Y: elementBounds.Origin.Y + 8}
	} else {
		origin.X += elementBounds.Origin.X - t.bounds.Origin.X
		origin.Y += elementBounds.Origin.Y - t.bounds.Origin.Y
	}
	if t.multiline {
		row, x := t.caretRowX(rng.Start)
		return Bounds{Origin: Point{X: origin.X + x, Y: origin.Y + Pixels(row)*t.lineHeight() - t.scrollY}, Size: Size{W: 2, H: t.lineHeight()}}, true
	}
	return Bounds{Origin: Point{X: origin.X + t.advance(rng.Start), Y: origin.Y}, Size: Size{W: 2, H: ts.ResolvedLineHeight()}}, true
}

// CharacterIndexForPoint maps x to the nearest shaped rune boundary in UTF-16.
func (t *TextInput) CharacterIndexForPoint(p Point, elementBounds Bounds) (int, bool) {
	origin := t.textOrigin.X
	if t.bounds.Size == (Size{}) {
		origin = elementBounds.Origin.X + 8
	} else {
		origin += elementBounds.Origin.X - t.bounds.Origin.X
	}
	if t.multiline {
		oy := elementBounds.Origin.Y + 8
		if t.bounds.Size != (Size{}) {
			oy = t.textOrigin.Y + elementBounds.Origin.Y - t.bounds.Origin.Y
		}
		return t.multilineIndexForPoint(p, Point{X: origin, Y: oy}), true
	}
	x := p.X - origin
	stops := t.caretStops()
	for i := 0; i+1 < len(stops); i++ {
		if x < (stops[i].x+stops[i+1].x)/2 {
			return stops[i].offset, true
		}
	}
	return stops[len(stops)-1].offset, true
}

// SetSelectedTextRange clamps and snaps endpoints down to rune boundaries;
// reversed endpoints select backwards. It refreshes an already rendered field.
func (t *TextInput) SetSelectedTextRange(rng IntRange) {
	t.cancelSelectionDrag()
	t.selectTo(rng.Start, rng.End)
}

// TextLengthUTF16 returns the document length in UTF-16 code units.
func (t *TextInput) TextLengthUTF16() (int, bool) { return utf16Length(t.text), true }

// AcceptsTextInput reports that this handler wants printable key characters
// routed to it.
func (t *TextInput) AcceptsTextInput() bool { return true }
