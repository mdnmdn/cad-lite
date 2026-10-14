package gpui_test

import (
	"math"
	"testing"

	"cad-lite/internal/gpui"
)

const mlLineHeight = gpui.Pixels(14 * 1.2)

func mlRoot(ti *gpui.TextInput, width float32) gpui.EntityRoot {
	return gpui.RenderFunc(func(w *gpui.Window, cx *gpui.App) gpui.IntoElement {
		return gpui.NewDiv().SizeWH(width, 300).Child(ti.Render(w, cx))
	})
}

// openMultiline renders ti (bundled font, so wrapping is deterministic) in
// a column of the given width and focuses it.
func openMultiline(t *testing.T, ti *gpui.TextInput, width float32) (*gpui.HeadlessWindow, gpui.Bounds) {
	t.Helper()
	t.Setenv(gpui.NoSystemFontsEnv, "1")
	hw, _ := openHeadless(t, 400, 300, mlRoot(ti, width))
	field := inputSelectionField(t, hw)
	hw.Dispatch(gpui.MouseDownEvent{Position: gpui.Point{X: field.Origin.X + 4, Y: field.Origin.Y + 4}, Button: gpui.MouseLeft, ClickCount: 1})
	hw.Dispatch(gpui.MouseUpEvent{Position: gpui.Point{X: field.Origin.X + 4, Y: field.Origin.Y + 4}, Button: gpui.MouseLeft, ClickCount: 1})
	if !ti.FocusHandle().IsFocused(hw.Window()) {
		t.Fatal("click did not focus the field")
	}
	return hw, inputSelectionField(t, hw)
}

func mlKey(hw *gpui.HeadlessWindow, key string, mods gpui.Modifiers) {
	hw.Dispatch(gpui.KeyDownEvent{Keystroke: gpui.Keystroke{Key: key, Modifiers: mods}})
}

func mlType(hw *gpui.HeadlessWindow, s string) {
	hw.Dispatch(gpui.KeyDownEvent{Keystroke: gpui.Keystroke{Key: s, KeyChar: &s}, PreferCharacterInput: true})
}

func mlNear(a, b gpui.Pixels) bool { return math.Abs(float64(a-b)) < 0.01 }

func TestMultilineFieldGrowsUpToMaxLinesThenScrolls(t *testing.T) {
	ti := gpui.NewTextInput("Ask").Multiline(4)
	hw, empty := openMultiline(t, ti, 300)
	for _, tc := range []struct {
		text string
		rows int
	}{{"a\nb\nc", 3}, {"a\nb\nc\nd\ne\nf", 4}, {"", 1}} {
		ti.SetText(tc.text)
		field := inputSelectionField(t, hw)
		if want := empty.Size.H + gpui.Pixels(tc.rows-1)*mlLineHeight; !mlNear(field.Size.H, want) {
			t.Errorf("%q: field height %v, want %v", tc.text, field.Size.H, want)
		}
	}
	ti.SetText("a\nb\nc\nd\ne\nf")
	field := inputSelectionField(t, hw)
	end, _ := ti.BoundsForRange(gpui.IntRange{Start: 11, End: 11}, field)
	if end.Origin.Y < field.Origin.Y || end.Origin.Y+end.Size.H > field.Origin.Y+field.Size.H+0.01 {
		t.Fatalf("caret at end %v not scrolled into field %v", end, field)
	}
	top, _ := ti.BoundsForRange(gpui.IntRange{}, field)
	if top.Origin.Y >= field.Origin.Y {
		t.Fatalf("first line %v should be scrolled above field %v", top, field)
	}
	hw.Dispatch(gpui.ScrollWheelEvent{Position: gpui.Point{X: field.Origin.X + 10, Y: field.Origin.Y + 10}, DeltaY: -100})
	hw.Image()
	top2, _ := ti.BoundsForRange(gpui.IntRange{}, field)
	if top2.Origin.Y < field.Origin.Y || top2.Origin.Y > field.Origin.Y+mlLineHeight {
		t.Fatalf("wheel up did not scroll to the top: %v (field %v)", top2, field)
	}
}

func TestMultilineEnterShiftEnterAndSubmit(t *testing.T) {
	ti := gpui.NewTextInput("").Multiline(5)
	hw, _ := openMultiline(t, ti, 300)
	mlType(hw, "ab")
	mlKey(hw, "enter", 0)
	mlType(hw, "c")
	if ti.Text() != "ab\nc" {
		t.Fatalf("enter without OnSubmit: %q", ti.Text())
	}
	var sent []string
	ti.OnSubmit(func(s string, _ *gpui.Window, _ *gpui.App) { sent = append(sent, s) })
	mlKey(hw, "enter", gpui.Shift)
	mlType(hw, "d")
	if ti.Text() != "ab\nc\nd" || len(sent) != 0 {
		t.Fatalf("shift-enter: text %q sent %v", ti.Text(), sent)
	}
	mlKey(hw, "enter", 0)
	if len(sent) != 1 || sent[0] != "ab\nc\nd" || ti.Text() != "ab\nc\nd" {
		t.Fatalf("enter with OnSubmit: sent %v text %q", sent, ti.Text())
	}
}

func TestMultilineUpDownHomeEnd(t *testing.T) {
	ti := gpui.NewTextInput("").Multiline(5)
	ti.SetText("abcd\nef\nabcd")
	hw, _ := openMultiline(t, ti, 300)
	ti.SetSelectedTextRange(gpui.IntRange{Start: 3, End: 3})
	for _, tc := range []struct {
		key        string
		mods       gpui.Modifiers
		start, end int
		reversed   bool
	}{
		{"down", 0, 7, 7, false},   // "ef" is shorter: its end
		{"down", 0, 11, 11, false}, // the goal column survives the short line
		{"up", 0, 7, 7, false},
		{"up", 0, 3, 3, false},
		{"up", 0, 0, 0, false}, // first line: start of text
		{"down", gpui.Shift, 0, 5, false},
		{"end", 0, 7, 7, false},
		{"home", gpui.Shift, 5, 7, true},
		{"down", 0, 8, 8, false},
		{"down", 0, 12, 12, false}, // last line: end of text
		{"home", 0, 8, 8, false},
		{"end", 0, 12, 12, false},
	} {
		mlKey(hw, tc.key, tc.mods)
		inputSelectionWant(t, ti, tc.start, tc.end, tc.reversed)
	}
}

func TestMultilineSoftWrapRowsAndEnd(t *testing.T) {
	ti := gpui.NewTextInput("").Multiline(10)
	ti.SetText("alpha beta gamma delta epsilon zeta")
	hw, field := openMultiline(t, ti, 120)
	if field.Size.H < 3*mlLineHeight {
		t.Fatalf("long text did not wrap: field %v", field)
	}
	ti.SetSelectedTextRange(gpui.IntRange{})
	mlKey(hw, "end", 0)
	end, _ := ti.SelectedTextRange()
	home, _ := ti.BoundsForRange(gpui.IntRange{}, field)
	caret, _ := ti.BoundsForRange(end.Range, field)
	if end.Range.End == 0 || end.Range.End >= 35 || !mlNear(caret.Origin.Y, home.Origin.Y) {
		t.Fatalf("End on a soft-wrapped row: %+v at %v (row start %v)", end, caret, home)
	}
	next, _ := ti.BoundsForRange(gpui.IntRange{Start: end.Range.End + 1, End: end.Range.End + 1}, field)
	if !mlNear(next.Origin.Y, home.Origin.Y+mlLineHeight) || !mlNear(next.Origin.X, home.Origin.X) {
		t.Fatalf("offset after the wrap %v should start row 2 (row 1 starts at %v)", next, home)
	}
	mlKey(hw, "down", 0)
	sel, _ := ti.SelectedTextRange()
	if b, _ := ti.BoundsForRange(sel.Range, field); !mlNear(b.Origin.Y, home.Origin.Y+mlLineHeight) {
		t.Fatalf("down from row 1 landed at %v", b)
	}
}

func TestMultilineMouseMapsRowsAndTripleClickSelectsLine(t *testing.T) {
	ti := gpui.NewTextInput("").Multiline(5)
	ti.SetText("one two\nthree four\nfive")
	hw, field := openMultiline(t, ti, 300)
	inputSelectionPress(hw, inputSelectionPoint(ti, field, 10), 1, 0)
	inputSelectionWant(t, ti, 10, 10, false)
	hw.Dispatch(gpui.MouseMoveEvent{Position: inputSelectionPoint(ti, field, 21)})
	inputSelectionWant(t, ti, 10, 21, false)
	hw.Dispatch(gpui.MouseUpEvent{Position: inputSelectionPoint(ti, field, 21), Button: gpui.MouseLeft})
	inputSelectionPress(hw, inputSelectionPoint(ti, field, 12), 3, 0)
	inputSelectionWant(t, ti, 8, 18, false)
	hw.Dispatch(gpui.MouseUpEvent{Position: inputSelectionPoint(ti, field, 12), Button: gpui.MouseLeft})
	p, q := inputSelectionPoint(ti, field, 9), inputSelectionPoint(ti, field, 10)
	p.X = (p.X + q.X) / 2
	inputSelectionPress(hw, p, 2, 0)
	inputSelectionWant(t, ti, 8, 13, false)
}

func TestMultilineSelectionPaintsEveryRow(t *testing.T) {
	sel := gpui.RgbaOf(255, 0, 0, 255)
	ti := gpui.NewTextInput("").Multiline(5).SelectionColor(sel)
	ti.SetText("abc\ndef\nghi")
	hw, _ := openMultiline(t, ti, 300)
	ti.SetSelectedTextRange(gpui.IntRange{Start: 1, End: 9})
	hw.Image()
	var ys []gpui.Pixels
	for _, q := range hw.Window().Scene().Quads {
		if q.Fill == sel {
			ys = append(ys, q.Bounds.Origin.Y)
		}
	}
	if len(ys) != 3 || !mlNear(ys[1]-ys[0], mlLineHeight) || !mlNear(ys[2]-ys[1], mlLineHeight) {
		t.Fatalf("selection quads at %v, want three rows", ys)
	}
}

func TestTextInputPasteNewlines(t *testing.T) {
	ml := gpui.NewTextInput("").Multiline(3)
	ml.ReplaceTextInRange(nil, "a\r\nb\rc\nd")
	if ml.Text() != "a\nb\nc\nd" {
		t.Errorf("multi-line paste = %q", ml.Text())
	}
	one := gpui.NewTextInput("")
	one.ReplaceTextInRange(nil, "a\r\nb\rc\nd")
	if one.Text() != "a b c d" {
		t.Errorf("single-line paste = %q", one.Text())
	}
	if got, _ := ml.TextForRange(gpui.IntRange{Start: 0, End: 5}); got != "a\nb\nc" {
		t.Errorf("copy across lines = %q", got)
	}
}
