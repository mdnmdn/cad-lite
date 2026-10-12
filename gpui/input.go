package gpui

import (
	"math"
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

	focus    *FocusHandle
	measurer TextMeasurer
	win      *Window
	app      *App

	onSubmit []func(string, *Window, *App)
	onChange []func(string, *Window, *App)
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
}

// FocusHandle returns the handle the field renders with, allocating it from
// the App on first use. It is only valid after Render has run at least once.
func (t *TextInput) FocusHandle() *FocusHandle { return t.focus }

// OnSubmit registers a callback invoked with the current text when the user
// presses Enter while the field is focused.
func (t *TextInput) OnSubmit(f func(string, *Window, *App)) *TextInput {
	t.onSubmit = append(t.onSubmit, f)
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
		t.win = w
	}
	if cx != nil {
		t.app = cx
	}
	var label IntoElement
	if t.text == "" {
		label = NewText(t.placeholder).TextColor(Gray500)
	} else {
		label = NewText(t.text)
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
		OnMouseDown(func(e *MouseDownEvent, w *Window, cx *App) {
			t.focus.Focus(w)
		}).
		OnKeyDown(t.onKeyDown).
		Child(label)
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
	if e.ti.focus != nil && e.ti.focus.IsFocused(w) {
		w.HandleInput(e.ti)
	}
	return st
}

// Paint delegates to the div, then draws the caret when focused.
func (e inputElement) Paint(w *Window, cx *App, id ElementID, bounds Bounds, layoutState, prepaintState any) {
	e.div.paint(w, bounds, prepaintState)
	if e.ti.focus != nil && e.ti.focus.IsFocused(w) {
		e.paintCaret(w, bounds, layoutState)
	}
}

// paintCaret draws a 2px rule at the caret position, centred vertically in the
// content box.
func (e inputElement) paintCaret(w *Window, bounds Bounds, layoutState any) {
	style := e.div.resolvedStyle()
	ts := style.TextStyle
	if dl, ok := layoutState.(*divLayout); ok && dl.node != nil {
		ts = dl.node.Style.TextStyle
	}
	if ts.FontSize <= 0 {
		ts = DefaultTextStyle()
	}
	lh := ts.ResolvedLineHeight()
	bw, pad := style.BorderWidths, style.Padding
	x := bounds.Origin.X + bw.Left + pad.Left + e.caretAdvance(w, ts)
	top := bounds.Origin.Y + bw.Top + pad.Top
	inner := bounds.Size.H - bw.Top - bw.Bottom - pad.Top - pad.Bottom
	w.PaintQuad(Quad{
		Bounds: Bounds{
			Origin: Point{X: x, Y: top + (inner-lh)/2},
			Size:   Size{W: 2, H: lh},
		},
		Background: BackgroundSolid,
		Fill:       ts.Color,
	})
}

// caretAdvance measures the text before the caret using the shaper captured
// at Render time, falling back to the crude per-glyph estimate.
func (e inputElement) caretAdvance(w *Window, ts TextStyle) Pixels {
	m := e.ti.measurer
	if m == nil {
		return Pixels(float32(utf16Length(e.ti.text[:e.ti.byteOffset(tiCaret(e.ti))])) * float32(ts.FontSize) * 0.55)
	}
	head := e.ti.text[:e.ti.byteOffset(tiCaret(e.ti))]
	lines := m.ShapeWrapped(head, ts, float32(math.MaxFloat32))
	if len(lines) == 0 {
		return 0
	}
	return lines[0].Width
}

// onKeyDown handles the editing keys. Printable characters never reach here:
// the window routes them to ReplaceTextInRange before the key path runs.
func (t *TextInput) onKeyDown(e *KeyDownEvent, w *Window, cx *App) {
	if t.focus == nil || !t.focus.IsFocused(w) {
		return
	}
	caret := tiCaret(t)
	switch e.Keystroke.Key {
	case "enter":
		for _, f := range t.onSubmit {
			f(t.text, w, cx)
		}
		w.StopPropagation()
	case "escape":
		t.focus.Blur(w)
		w.StopPropagation()
	case "backspace":
		if t.sel.Range.Start != t.sel.Range.End {
			t.replaceSelection("", w)
		} else if caret > 0 {
			end := t.byteOffset(caret)
			start := t.byteOffset(caret - 1)
			t.replaceRange(start, end, "", w)
		}
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
	case "left":
		if t.sel.Range.Start != t.sel.Range.End {
			t.sel = UTF16Selection{Range: IntRange{Start: t.sel.Range.Start, End: t.sel.Range.Start}}
		} else if caret > 0 {
			t.sel = UTF16Selection{Range: IntRange{Start: caret - 1, End: caret - 1}}
		}
		w.Refresh()
	case "right":
		if t.sel.Range.Start != t.sel.Range.End {
			t.sel = UTF16Selection{Range: IntRange{Start: t.sel.Range.End, End: t.sel.Range.End}}
		} else if caret < utf16Length(t.text) {
			t.sel = UTF16Selection{Range: IntRange{Start: caret + 1, End: caret + 1}}
		}
		w.Refresh()
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
	t.text = t.text[:start] + text + t.text[end:]
	caret := utf16Length(t.text[:start]) + utf16Length(text)
	t.sel = UTF16Selection{Range: IntRange{Start: caret, End: caret}}
	t.hasMarked = false
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

// byteOffset converts a UTF-16 offset to a byte offset, reporting false when
// the offset is out of range or lands inside a surrogate pair.
func (t *TextInput) byteOffset(u int) int {
	if u < 0 {
		return 0
	}
	i := 0
	for pos := 0; pos < len(t.text); {
		if i == u {
			return pos
		}
		r, size := utf8.DecodeRuneInString(t.text[pos:])
		if r > 0xFFFF {
			i += 2
		} else {
			i++
		}
		pos += size
	}
	if i == u {
		return len(t.text)
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
	s := t.byteOffset(rng.Start)
	e := t.byteOffset(rng.End)
	if rng.Start < 0 || rng.End < rng.Start || e > len(t.text) {
		return "", false
	}
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

// BoundsForRange returns the caret rect at the range start. POC limitation:
// the shaper is not available without a window, so positions use the crude
// per-glyph estimate.
func (t *TextInput) BoundsForRange(rng IntRange, elementBounds Bounds) (Bounds, bool) {
	ts := DefaultTextStyle()
	lh := ts.ResolvedLineHeight()
	used := t.text[:t.byteOffset(rng.Start)]
	x := Pixels(float32(utf16Length(used)) * float32(ts.FontSize) * 0.55)
	return Bounds{
		Origin: Point{X: elementBounds.Origin.X + 8 + x, Y: elementBounds.Origin.Y + 8},
		Size:   Size{W: 2, H: lh},
	}, true
}

// CharacterIndexForPoint maps a point within the element's bounds to a
// UTF-16 code unit offset, using the crude per-glyph estimate.
func (t *TextInput) CharacterIndexForPoint(p Point, elementBounds Bounds) (int, bool) {
	ts := DefaultTextStyle()
	gw := float32(ts.FontSize) * 0.55
	if gw <= 0 {
		return 0, false
	}
	rel := float32(p.X - elementBounds.Origin.X - 8)
	idx := int(rel / gw)
	if idx < 0 {
		idx = 0
	}
	if idx > utf16Length(t.text) {
		idx = utf16Length(t.text)
	}
	return idx, true
}

// SetSelectedTextRange moves the selection to the given range.
func (t *TextInput) SetSelectedTextRange(rng IntRange) {
	t.sel = UTF16Selection{Range: rng}
}

// TextLengthUTF16 returns the document length in UTF-16 code units.
func (t *TextInput) TextLengthUTF16() (int, bool) { return utf16Length(t.text), true }

// AcceptsTextInput reports that this handler wants printable key characters
// routed to it.
func (t *TextInput) AcceptsTextInput() bool { return true }
