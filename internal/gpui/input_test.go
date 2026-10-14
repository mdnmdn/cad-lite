package gpui_test

import (
	"testing"

	"go-draw/internal/gpui"
)

func keyChar(s string) *string { return &s }

func inputTestRoot(ti *gpui.TextInput) gpui.EntityRoot {
	return gpui.RenderFunc(func(w *gpui.Window, cx *gpui.App) gpui.IntoElement {
		return gpui.NewDiv().SizeWH(400, 300).Child(ti.Render(w, cx))
	})
}

func openInputApp(t *testing.T, ti *gpui.TextInput) (*gpui.App, *gpui.Window) {
	t.Helper()
	p := &textFakePlatform{}
	app := gpui.NewAppWithPlatform(p)
	if err := app.OpenWindow(gpui.WindowOptions{Width: 400, Height: 300}, func(w *gpui.Window, cx *gpui.App) gpui.EntityRoot {
		return inputTestRoot(ti)
	}); err != nil {
		t.Fatalf("OpenWindow: %v", err)
	}
	win := app.Windows()[0]
	win.Draw(app)
	return app, win
}

func focusInput(t *testing.T, app *gpui.App, win *gpui.Window, ti *gpui.TextInput) {
	t.Helper()
	win.DispatchEvent(gpui.MouseDownEvent{Position: gpui.Point{X: 10, Y: 10}, Button: gpui.MouseLeft}, app)
	win.Draw(app)
	if !ti.FocusHandle().IsFocused(win) {
		t.Fatal("mousedown did not focus the input")
	}
}

func TestTextInputTypingThroughDispatch(t *testing.T) {
	ti := gpui.NewTextInput("placeholder")
	app, win := openInputApp(t, ti)
	focusInput(t, app, win, ti)

	for _, c := range []string{"a", "b", "c"} {
		win.DispatchEvent(gpui.KeyDownEvent{
			Keystroke:            gpui.Keystroke{Key: c, KeyChar: keyChar(c)},
			PreferCharacterInput: true,
		}, app)
	}
	if got := ti.Text(); got != "abc" {
		t.Errorf("Text() = %q, want %q", got, "abc")
	}
}

func TestTextInputSubmitOnEnter(t *testing.T) {
	ti := gpui.NewTextInput("")
	ti.SetText("hello")
	submitted := ""
	ti.OnSubmit(func(s string, w *gpui.Window, cx *gpui.App) { submitted = s })

	app, win := openInputApp(t, ti)
	focusInput(t, app, win, ti)

	win.DispatchEvent(gpui.KeyDownEvent{Keystroke: gpui.Keystroke{Key: "enter"}}, app)
	if submitted != "hello" {
		t.Errorf("submitted = %q, want %q", submitted, "hello")
	}
}

func TestTextInputEscapeBlurs(t *testing.T) {
	ti := gpui.NewTextInput("")
	app, win := openInputApp(t, ti)
	focusInput(t, app, win, ti)

	win.DispatchEvent(gpui.KeyDownEvent{Keystroke: gpui.Keystroke{Key: "escape"}}, app)
	if ti.FocusHandle().IsFocused(win) {
		t.Error("escape did not blur the input")
	}
}

func TestTextInputOnEscapeKeepsFocus(t *testing.T) {
	ti := gpui.NewTextInput("")
	escapes := 0
	ti.OnEscape(func(*gpui.Window, *gpui.App) { escapes++ })
	app, win := openInputApp(t, ti)
	focusInput(t, app, win, ti)

	win.DispatchEvent(gpui.KeyDownEvent{Keystroke: gpui.Keystroke{Key: "escape"}}, app)
	if escapes != 1 {
		t.Errorf("OnEscape calls = %d, want 1", escapes)
	}
	if !ti.FocusHandle().IsFocused(win) {
		t.Error("escape blurred an input that has an OnEscape handler")
	}
}

func TestTextInputBackspaceThroughDispatch(t *testing.T) {
	ti := gpui.NewTextInput("")
	ti.SetText("abc")
	app, win := openInputApp(t, ti)
	focusInput(t, app, win, ti)
	inputSelectionKey(win, app, "end", 0)

	win.DispatchEvent(gpui.KeyDownEvent{Keystroke: gpui.Keystroke{Key: "backspace"}}, app)
	if got := ti.Text(); got != "ab" {
		t.Errorf("Text() = %q after backspace, want %q", got, "ab")
	}
}

func TestTextInputArrowKeysMoveCaret(t *testing.T) {
	ti := gpui.NewTextInput("")
	ti.SetText("abc")
	app, win := openInputApp(t, ti)
	focusInput(t, app, win, ti)
	inputSelectionKey(win, app, "end", 0)

	win.DispatchEvent(gpui.KeyDownEvent{Keystroke: gpui.Keystroke{Key: "left"}}, app)
	sel, _ := ti.SelectedTextRange()
	if sel.Range.Start != 2 || sel.Range.End != 2 {
		t.Fatalf("after left: selection = %+v, want caret at 2", sel.Range)
	}

	win.DispatchEvent(gpui.KeyDownEvent{Keystroke: gpui.Keystroke{Key: "backspace"}}, app)
	if got := ti.Text(); got != "ac" {
		t.Errorf("Text() = %q after left+backspace, want %q", got, "ac")
	}
}

func TestTextInputHandlerContract(t *testing.T) {
	ti := gpui.NewTextInput("")
	ti.ReplaceTextInRange(nil, "héllo")
	if got := ti.Text(); got != "héllo" {
		t.Fatalf("Text() = %q, want %q", got, "héllo")
	}
	if n, ok := ti.TextLengthUTF16(); !ok || n != 5 {
		t.Errorf("TextLengthUTF16() = %d, %v; want 5, true", n, ok)
	}
	if s, ok := ti.TextForRange(gpui.IntRange{Start: 0, End: 2}); !ok || s != "hé" {
		t.Errorf("TextForRange(0,2) = %q, %v; want \"hé\", true", s, ok)
	}
	if !ti.AcceptsTextInput() {
		t.Error("AcceptsTextInput() = false, want true")
	}
	if _, ok := ti.MarkedTextRange(); ok {
		t.Error("MarkedTextRange() reported a marked range, want none")
	}
	ti.SetText("h\U0001F600llo")
	if n, ok := ti.TextLengthUTF16(); !ok || n != 6 {
		t.Errorf("TextLengthUTF16() with astral char = %d, %v; want 6, true", n, ok)
	}
}

func openHeadlessInput(t *testing.T, ti *gpui.TextInput, scale float32) (*gpui.App, *gpui.Window) {
	t.Helper()
	p := gpui.NewHeadlessPlatform()
	p.SetScaleFactor(scale)
	app := gpui.NewAppWithPlatform(p)
	if err := app.OpenWindow(gpui.WindowOptions{Width: 400, Height: 300}, func(w *gpui.Window, cx *gpui.App) gpui.EntityRoot {
		return inputTestRoot(ti)
	}); err != nil {
		t.Fatalf("OpenWindow: %v", err)
	}
	win := app.Windows()[0]
	win.Draw(app)
	return app, win
}

func inputFieldAndCaret(t *testing.T, win *gpui.Window) (field, caret gpui.Bounds) {
	t.Helper()
	var haveField, haveCaret bool
	for _, q := range win.Scene().Quads {
		if q.Bounds.Size.W == 2 {
			caret, haveCaret = q.Bounds, true
		} else if q.BorderWidths != (gpui.Edges{}) && q.Bounds.Size.W > 100 {
			field, haveField = q.Bounds, true
		}
	}
	if !haveField || !haveCaret {
		t.Fatalf("field=%v caret=%v not found in scene", haveField, haveCaret)
	}
	return field, caret
}

func TestTextInputFocusedHeightAndCaret(t *testing.T) {
	for _, scale := range []float32{1, 2} {
		ti := gpui.NewTextInput("What needs doing?")
		app, win := openHeadlessInput(t, ti, scale)
		focusInput(t, app, win, ti)
		emptyField, emptyCaret := inputFieldAndCaret(t, win)

		if emptyField.Size.H < 30 {
			t.Errorf("scale %v: empty focused field height %v is collapsed", scale, emptyField.Size.H)
		}
		if emptyCaret.Origin.Y < emptyField.Origin.Y || emptyCaret.Origin.Y+emptyCaret.Size.H > emptyField.Origin.Y+emptyField.Size.H {
			t.Errorf("scale %v: caret %v outside field %v", scale, emptyCaret, emptyField)
		}
		if len(win.Scene().Sprites) == 0 {
			t.Errorf("scale %v: placeholder glyphs not painted when empty and focused", scale)
		}

		ti.SetText("hello")
		win.Refresh()
		win.Draw(app)
		field, caret := inputFieldAndCaret(t, win)
		if field.Size.H != emptyField.Size.H {
			t.Errorf("scale %v: field height empty=%v non-empty=%v", scale, emptyField.Size.H, field.Size.H)
		}
		if caret.Origin.Y != emptyCaret.Origin.Y {
			t.Errorf("scale %v: caret y changed %v -> %v", scale, emptyCaret.Origin.Y, caret.Origin.Y)
		}
		if caret.Origin.Y < field.Origin.Y || caret.Origin.Y+caret.Size.H > field.Origin.Y+field.Size.H {
			t.Errorf("scale %v: caret %v outside field %v", scale, caret, field)
		}
		if caret.Origin.X <= emptyCaret.Origin.X {
			t.Errorf("scale %v: caret x %v did not advance past %v", scale, caret.Origin.X, emptyCaret.Origin.X)
		}
	}
}

func TestTextInputSetPlaceholderRerenders(t *testing.T) {
	p := gpui.NewHeadlessPlatform()
	app := gpui.NewAppWithPlatform(p)
	in := gpui.NewTextInput("a")
	if err := app.OpenWindow(gpui.WindowOptions{Width: 300, Height: 80}, func(*gpui.Window, *gpui.App) gpui.EntityRoot {
		return gpui.RenderFunc(func(w *gpui.Window, cx *gpui.App) gpui.IntoElement { return in.Render(w, cx) })
	}); err != nil {
		t.Fatal(err)
	}
	w := p.Windows()[0].Window()
	p.Windows()[0].Pixels()
	n := len(w.Scene().Sprites)
	in.SetPlaceholder("a much longer placeholder")
	p.Windows()[0].Pixels()
	if got := len(w.Scene().Sprites); got <= n {
		t.Fatalf("placeholder not re-rendered: %d sprites before, %d after", n, got)
	}
}
