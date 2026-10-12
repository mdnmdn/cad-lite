package gpui_test

import (
	"testing"

	"go-gpui/gpui"
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

func TestTextInputBackspaceThroughDispatch(t *testing.T) {
	ti := gpui.NewTextInput("")
	ti.SetText("abc")
	app, win := openInputApp(t, ti)
	focusInput(t, app, win, ti)

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
