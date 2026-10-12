package gpui_test

import (
	"testing"

	"go-gpui/gpui"
)

func TestNamedActionsAreDistinctInOnAction(t *testing.T) {
	var log []string
	add, blur := gpui.NewAction("add-todo"), gpui.NewAction("blur")
	root := gpui.RenderFunc(func(*gpui.Window, *gpui.App) gpui.IntoElement {
		return gpui.NewDiv().SizeFull().
			OnAction(add, func(a gpui.Action, _ *gpui.Window, _ *gpui.App) { log = append(log, "add:"+a.Name()) }).
			OnAction(blur, func(a gpui.Action, _ *gpui.Window, _ *gpui.App) { log = append(log, "blur:"+a.Name()) })
	})
	hw, app := openHeadless(t, 100, 100, root)
	app.Keymap().AddBindings(
		gpui.NewKeyBinding("enter", add, ""),
		gpui.NewKeyBinding("escape", blur, ""),
	)
	hw.Dispatch(gpui.KeyDownEvent{Keystroke: gpui.NewKeystroke("enter", 0)})
	hw.Dispatch(gpui.KeyDownEvent{Keystroke: gpui.NewKeystroke("escape", 0)})
	checkLog(t, log, "add:add-todo", "blur:blur")

	if gpui.ActionKeyOf(add) == gpui.ActionKeyOf(blur) {
		t.Error("ActionKeyOf: named actions share a key")
	}
	if gpui.ActionKeyOf(add) != gpui.ActionKeyOf(gpui.NewAction("add-todo")) {
		t.Error("ActionKeyOf: equal named actions differ")
	}
}

func TestBindingMatchesKeyRegardlessOfKeyChar(t *testing.T) {
	a := "a"
	ks, _ := gpui.ParseKeystroke("a")
	typed := gpui.Keystroke{Key: "a", KeyChar: &a}
	if !ks.Matches(typed) {
		t.Error(`binding "a" does not match a typed "a" with KeyChar`)
	}
	q := "?"
	qs, _ := gpui.ParseKeystroke("?")
	if !qs.Matches(gpui.Keystroke{Key: "?", KeyChar: &q}) {
		t.Error(`binding "?" does not match typed "?"`)
	}
	if ks.Matches(gpui.Keystroke{Key: "a", Modifiers: gpui.Control}) {
		t.Error(`binding "a" matches ctrl-a`)
	}
	if !ks.Equals(gpui.Keystroke{Key: "a"}) || ks.Equals(typed) {
		t.Error("Equals must stay exact (KeyChar compared)")
	}
}

func TestBarePrintableBindingFiresFromCharEvent(t *testing.T) {
	var log []string
	toggle := gpui.NewAction("toggle")
	root := gpui.RenderFunc(func(*gpui.Window, *gpui.App) gpui.IntoElement {
		return gpui.NewDiv().SizeFull().
			OnAction(toggle, func(gpui.Action, *gpui.Window, *gpui.App) { log = append(log, "toggle") })
	})
	hw, app := openHeadless(t, 100, 100, root)
	app.Keymap().AddBindings(gpui.NewKeyBinding("space", toggle, ""), gpui.NewKeyBinding("t", toggle, ""))
	sp, tc := " ", "t"
	hw.Dispatch(gpui.KeyDownEvent{Keystroke: gpui.Keystroke{Key: "space", KeyChar: &sp}, PreferCharacterInput: true})
	hw.Dispatch(gpui.KeyDownEvent{Keystroke: gpui.Keystroke{Key: "t", KeyChar: &tc}, PreferCharacterInput: true})
	checkLog(t, log, "toggle", "toggle")
}

func TestCharInputGoesToInputWhenNoBindingConsumes(t *testing.T) {
	ti := gpui.NewTextInput("")
	app, win := openInputApp(t, ti)
	// "x" and "y" are bound, but no OnAction handler on the focus path
	// handles their actions, so the chars still reach the text field.
	unhandled, handled := gpui.NewAction("unhandled"), gpui.NewAction("handled")
	app.Keymap().AddBindings(gpui.NewKeyBinding("x", unhandled, ""), gpui.NewKeyBinding("y", handled, "textbox"))
	focusInput(t, app, win, ti)
	for _, c := range []string{"x", "y", "z"} {
		win.DispatchEvent(gpui.KeyDownEvent{Keystroke: gpui.Keystroke{Key: c, KeyChar: keyChar(c)}, PreferCharacterInput: true}, app)
	}
	if got := ti.Text(); got != "xyz" {
		t.Errorf("Text() = %q, want %q (no OnAction handler consumed y)", got, "xyz")
	}
}

func TestCharInputConsumedByHandledBinding(t *testing.T) {
	ti := gpui.NewTextInput("")
	var log []string
	p := gpui.NewHeadlessPlatform()
	app := gpui.NewAppWithPlatform(p)
	handled := gpui.NewAction("handled")
	app.Keymap().AddBindings(gpui.NewKeyBinding("y", handled, "textbox"))
	_ = app.OpenWindow(gpui.WindowOptions{Width: 400, Height: 300}, func(*gpui.Window, *gpui.App) gpui.EntityRoot {
		return gpui.RenderFunc(func(w *gpui.Window, cx *gpui.App) gpui.IntoElement {
			return gpui.NewDiv().SizeWH(400, 300).
				OnAction(handled, func(gpui.Action, *gpui.Window, *gpui.App) { log = append(log, "handled") }).
				Child(ti.Render(w, cx))
		})
	})
	hw := p.Windows()[0]
	hw.Pixels()
	hw.Dispatch(gpui.MouseDownEvent{Position: gpui.Point{X: 10, Y: 10}, Button: gpui.MouseLeft})
	hw.Pixels()
	for _, c := range []string{"x", "y", "z"} {
		hw.Dispatch(gpui.KeyDownEvent{Keystroke: gpui.Keystroke{Key: c, KeyChar: keyChar(c)}, PreferCharacterInput: true})
	}
	if got := ti.Text(); got != "xz" {
		t.Errorf("Text() = %q, want %q", got, "xz")
	}
	checkLog(t, log, "handled")
}
