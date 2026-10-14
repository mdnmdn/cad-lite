// Command focus_visible compares focus() and focus_visible() styling: click
// a button to see the focus() border only, press Tab to see the
// focus_visible() border (keyboard focus).
//
// Ported from gpui-ce crates/gpui/examples/legacy/focus_visible.rs
//
// Not ported: cursor_pointer. The headless PNG renderer cannot simulate a key
// press, so -focus seeds the focused button and uses the
// keyboard (focus-visible) border colour for focus(); interactive mode uses Div.FocusVisible.
package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"time"

	"go-draw/internal/gpui"
	"go-draw/internal/gpui/ebitenplatform"
)

type tabAction struct{}

func (tabAction) Name() string              { return "focus_visible::Tab" }
func (tabAction) Clone() gpui.Action        { return tabAction{} }
func (tabAction) Equals(o gpui.Action) bool { _, ok := o.(tabAction); return ok }

type tabPrevAction struct{}

func (tabPrevAction) Name() string              { return "focus_visible::TabPrev" }
func (tabPrevAction) Clone() gpui.Action        { return tabPrevAction{} }
func (tabPrevAction) Equals(o gpui.Action) bool { _, ok := o.(tabPrevAction); return ok }

type quitAction struct{}

func (quitAction) Name() string              { return "focus_visible::Quit" }
func (quitAction) Clone() gpui.Action        { return quitAction{} }
func (quitAction) Equals(o gpui.Action) bool { _, ok := o.(quitAction); return ok }

func registerKeybinds(km *gpui.Keymap) {
	km.AddBindings(
		gpui.NewKeyBinding("tab", tabAction{}, ""),
		gpui.NewKeyBinding("shift-tab", tabPrevAction{}, ""),
		gpui.NewKeyBinding("cmd-q", quitAction{}, ""),
	)
}

var (
	amber = gpui.MustParseHex("#fbbf24")
	green = gpui.MustParseHex("#10b981")
)

func hex(s string) gpui.Rgba { return gpui.MustParseHex(s) }

// Example is the root view.
type Example struct {
	root    *gpui.FocusHandle
	items   []*gpui.FocusHandle
	message string
	// seed focuses that button on the first render (-1: none).
	seed int
	// seedKeyboard paints the focus-visible border on the seeded button.
	seedKeyboard bool
}

func (e *Example) ensure(w *gpui.Window, cx *gpui.App) {
	if e.root != nil {
		return
	}
	for i := 1; i <= 3; i++ {
		e.items = append(e.items, cx.NewFocusHandle().SetTabIndex(i).SetTabStop(true))
	}
	e.root = cx.NewFocusHandle()
	if e.seed >= 0 && e.seed < len(e.items) {
		w.RequestFocus(e.items[e.seed])
	} else {
		w.RequestFocus(e.root)
	}
}

func (e *Example) button(w *gpui.Window, id string, h *gpui.FocusHandle, label, clicked string) *gpui.Div {
	d := gpui.NewDiv().ID(id).TrackFocus(h).SizeH(64).Flex().JustifyCenter().ItemsCenter().
		Bg(hex("#2563eb")).Rounded(6).
		Hover(func(m *gpui.StyledMixin) *gpui.StyledMixin { return m.Bg(hex("#1d4ed8")) }).
		OnClick(func(_ *gpui.ClickEvent, w *gpui.Window, _ *gpui.App) {
			e.message = clicked
			w.Refresh()
		}).
		Child(gpui.NewText(label).TextSize(16).TextColor(gpui.White))
	return d
}

func (e *Example) section(title string, button *gpui.Div) *gpui.Div {
	return gpui.NewDiv().FlexCol().Gap(8).Child(
		gpui.NewText(title).TextSize(14).FontWeight(700).TextColor(hex("#374151")),
		button,
	)
}

func (e *Example) step(w *gpui.Window, next bool) {
	if next {
		w.FocusNext()
		e.message = "Pressed Tab - focus-visible border should appear!"
	} else {
		w.FocusPrev()
		e.message = "Pressed Shift-Tab - focus-visible border should appear!"
	}
}

// Render builds the element tree.
func (e *Example) Render(w *gpui.Window, cx *gpui.App) gpui.IntoElement {
	e.ensure(w, cx)
	focusBorder := func(m *gpui.StyledMixin) *gpui.StyledMixin { return m.Border(4, amber) }
	seededVisible := func(m *gpui.StyledMixin) *gpui.StyledMixin {
		if e.seedKeyboard {
			return m.Border(4, green)
		}
		return m
	}
	visibleBorder := func(m *gpui.StyledMixin) *gpui.StyledMixin { return m.Border(4, green) }

	b1 := e.button(w, "button1", e.items[0], "Button with .focus() - always shows border when focused",
		"Clicked button 1 - focus border is visible!").Focus(focusBorder)
	b2 := e.button(w, "button2", e.items[1], "Button with .focus_visible() - only shows border with keyboard",
		"Clicked button 2 - no border! Try Tab instead.").FocusVisible(visibleBorder)
	b3 := e.button(w, "button3", e.items[2], "Button with both .focus() and .focus_visible()",
		"Clicked button 3 - yellow border. Tab shows green!").Focus(focusBorder).FocusVisible(visibleBorder)
	if e.seedKeyboard {
		b2.Focus(seededVisible)
		b3.Focus(seededVisible)
	}

	return gpui.NewDiv().ID("app").TrackFocus(e.root).
		OnAction(tabAction{}, func(_ gpui.Action, w *gpui.Window, _ *gpui.App) { e.step(w, true) }).
		OnAction(tabPrevAction{}, func(_ gpui.Action, w *gpui.Window, _ *gpui.App) { e.step(w, false) }).
		OnAction(quitAction{}, func(_ gpui.Action, _ *gpui.Window, cx *gpui.App) { cx.Quit() }).
		SizeFull().FlexCol().P(32).Gap(24).Bg(hex("#f3f4f6")).Child(
		gpui.NewText("CSS focus-visible Demo").TextSize(24).FontWeight(700).TextColor(hex("#111827")),
		gpui.NewDiv().P(16).Rounded(6).Bg(hex("#dbeafe")).Child(
			gpui.NewText(e.message).TextSize(14).TextColor(hex("#1e3a8a")),
		),
		gpui.NewDiv().FlexCol().Gap(16).Child(
			e.section("1. Regular .focus() - always visible:", b1),
			e.section("2. New .focus_visible() - only keyboard:", b2),
			e.section("3. Both .focus() (yellow) and .focus_visible() (green):", b3),
		),
	)
}

func main() {
	png := flag.String("png", "", "render one frame to this PNG path and exit")
	width := flag.Int("width", 800, "window width in logical pixels")
	height := flag.Int("height", 600, "window height in logical pixels")
	smoke := flag.Int("smoke", 0, "quit after this many milliseconds (interactive mode)")
	focus := flag.Int("focus", 2, "seed focus on button N (0-2, PNG mode; -1 for none)")
	keyboard := flag.Bool("keyboard", true, "seed the focus as keyboard focus (PNG mode)")
	flag.Parse()

	const hint = "Try clicking vs tabbing! Click shows no border, Tab shows border."

	if *png != "" {
		km := gpui.NewKeymap()
		registerKeybinds(km)
		root := &Example{message: hint, seed: *focus, seedKeyboard: *keyboard}
		if err := gpui.RenderToPNG(*png, *width, *height, 1, root, gpui.WithKeymap(km)); err != nil {
			log.Fatal(err)
		}
		return
	}

	err := ebitenplatform.New().Run(func(cx *gpui.App) {
		registerKeybinds(cx.Keymap())
		opts := gpui.WindowOptions{Width: float32(*width), Height: float32(*height), Title: "Focus visible", Resizable: true}
		root := &Example{message: hint, seed: -1}
		if err := cx.OpenWindow(opts, func(*gpui.Window, *gpui.App) gpui.EntityRoot { return root }); err != nil {
			log.Fatal(err)
		}
		if *smoke > 0 {
			time.AfterFunc(time.Duration(*smoke)*time.Millisecond, cx.Quit)
		}
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
