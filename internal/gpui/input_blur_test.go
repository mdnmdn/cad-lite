package gpui_test

import (
	"testing"

	"cad-lite/internal/gpui"
)

func TestTextInputOnBlur(t *testing.T) {
	a := gpui.NewTextInput("")
	b := gpui.NewTextInput("")
	var got []string
	a.OnBlur(func(s string, _ *gpui.Window, _ *gpui.App) { got = append(got, s) })
	p := &textFakePlatform{}
	app := gpui.NewAppWithPlatform(p)
	if err := app.OpenWindow(gpui.WindowOptions{Width: 400, Height: 300}, func(w *gpui.Window, cx *gpui.App) gpui.EntityRoot {
		return gpui.RenderFunc(func(w *gpui.Window, cx *gpui.App) gpui.IntoElement {
			return gpui.NewDiv().SizeWH(400, 300).FlexCol().
				Child(a.Render(w, cx)).
				Child(gpui.NewDiv().SizeWH(400, 100)).
				Child(b.Render(w, cx))
		})
	}); err != nil {
		t.Fatal(err)
	}
	win := app.Windows()[0]
	win.Draw(app)
	press := func(y float32) {
		win.DispatchEvent(gpui.MouseDownEvent{Position: gpui.Point{X: 10, Y: gpui.Pixels(y)}, Button: gpui.MouseLeft}, app)
		win.Draw(app)
	}
	press(10)
	if !a.FocusHandle().IsFocused(win) {
		t.Fatal("a not focused")
	}
	a.SetText("one")
	win.Draw(app)
	if len(got) != 0 {
		t.Fatalf("fired while focused: %v", got)
	}
	win.RequestFocus(a.FocusHandle())
	win.Draw(app)
	if len(got) != 0 {
		t.Fatalf("fired on refocus of same handle: %v", got)
	}

	// Escape with OnEscape keeps focus and does not blur.
	a.OnEscape(func(*gpui.Window, *gpui.App) {})
	win.DispatchEvent(gpui.KeyDownEvent{Keystroke: gpui.Keystroke{Key: "escape"}}, app)
	if len(got) != 0 || !a.FocusHandle().IsFocused(win) {
		t.Fatalf("escape handled by OnEscape: got=%v focused=%v", got, a.FocusHandle().IsFocused(win))
	}

	// Another handle's RequestFocus.
	win.RequestFocus(b.FocusHandle())
	win.Draw(app)
	if len(got) != 1 || got[0] != "one" {
		t.Fatalf("after focusing b: %v", got)
	}
	// Further focus changes not involving a do not fire.
	win.Blur()
	win.Draw(app)
	if len(got) != 1 {
		t.Fatalf("fired again: %v", got)
	}

	// Window.Blur.
	win.RequestFocus(a.FocusHandle())
	win.Blur()
	win.Draw(app)
	if len(got) != 2 {
		t.Fatalf("after Window.Blur: %v", got)
	}
	win.Blur()
	if len(got) != 2 {
		t.Fatalf("Blur with nothing focused fired: %v", got)
	}
}

func TestTextInputEscapeWithoutHandlerBlurs(t *testing.T) {
	a := gpui.NewTextInput("")
	n := 0
	a.OnBlur(func(string, *gpui.Window, *gpui.App) { n++ })
	app, win := openInputApp(t, a)
	focusInput(t, app, win, a)
	win.DispatchEvent(gpui.KeyDownEvent{Keystroke: gpui.Keystroke{Key: "escape"}}, app)
	if n != 1 {
		t.Fatalf("blur count = %d, want 1", n)
	}
}

func TestTextInputOnBlurPressElsewhere(t *testing.T) {
	a := gpui.NewTextInput("")
	b := gpui.NewTextInput("")
	var got []string
	a.OnBlur(func(s string, _ *gpui.Window, _ *gpui.App) { got = append(got, s) })
	app := gpui.NewAppWithPlatform(&textFakePlatform{})
	if err := app.OpenWindow(gpui.WindowOptions{Width: 400, Height: 300}, func(w *gpui.Window, cx *gpui.App) gpui.EntityRoot {
		return gpui.RenderFunc(func(w *gpui.Window, cx *gpui.App) gpui.IntoElement {
			return gpui.NewDiv().SizeWH(400, 300).FlexCol().
				Child(a.Render(w, cx)).
				Child(gpui.NewDiv().SizeWH(400, 100)).
				Child(b.Render(w, cx))
		})
	}); err != nil {
		t.Fatal(err)
	}
	win := app.Windows()[0]
	win.Draw(app)
	press := func(y gpui.Pixels) {
		win.DispatchEvent(gpui.MouseDownEvent{Position: gpui.Point{X: 10, Y: y}, Button: gpui.MouseLeft}, app)
		win.Draw(app)
	}
	press(10)
	press(10)
	if len(got) != 0 {
		t.Fatalf("press inside the field fired: %v", got)
	}
	press(125)
	if !b.FocusHandle().IsFocused(win) {
		t.Fatal("press did not focus b")
	}
	if len(got) != 1 || got[0] != "" {
		t.Fatalf("after press elsewhere: %v", got)
	}
}
