// Command tab_stop demonstrates keyboard tab order: items with explicit tab
// indices, one item that is not a tab stop, and two buttons. Tab and
// Shift-Tab move focus.
//
// Ported from gpui-ce crates/gpui/examples/legacy/tab_stop.rs
//
// Not ported: the example font loader. The -focus flag seeds focus on the
// n-th focusable (0-4 items, 5-6 buttons) for PNG rendering.
package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"time"

	"go-gpui/gpui"
	"go-gpui/gpui/ebitenplatform"
)

type tabAction struct{}

func (tabAction) Name() string              { return "tab_stop::Tab" }
func (tabAction) Clone() gpui.Action        { return tabAction{} }
func (tabAction) Equals(o gpui.Action) bool { _, ok := o.(tabAction); return ok }

type tabPrevAction struct{}

func (tabPrevAction) Name() string              { return "tab_stop::TabPrev" }
func (tabPrevAction) Clone() gpui.Action        { return tabPrevAction{} }
func (tabPrevAction) Equals(o gpui.Action) bool { _, ok := o.(tabPrevAction); return ok }

func registerKeybinds(km *gpui.Keymap) {
	km.AddBindings(
		gpui.NewKeyBinding("tab", tabAction{}, ""),
		gpui.NewKeyBinding("shift-tab", tabPrevAction{}, ""),
	)
}

var blue = gpui.MustParseHex("#0000ff")

// Example is the root view.
type Example struct {
	root    *gpui.FocusHandle
	items   []*gpui.FocusHandle
	buttons []*gpui.FocusHandle
	message string
	seed    int
}

func (e *Example) ensure(w *gpui.Window, cx *gpui.App) {
	if e.root != nil {
		return
	}
	stop := func(i int) *gpui.FocusHandle { return cx.NewFocusHandle().SetTabIndex(i).SetTabStop(true) }
	e.items = []*gpui.FocusHandle{stop(1), stop(2), stop(3), cx.NewFocusHandle(), stop(2)}
	e.buttons = []*gpui.FocusHandle{stop(4), stop(5)}
	e.root = cx.NewFocusHandle()
	all := append(append([]*gpui.FocusHandle{}, e.items...), e.buttons...)
	if e.seed >= 0 && e.seed < len(all) {
		w.RequestFocus(all[e.seed])
	} else {
		w.RequestFocus(e.root)
	}
}

func focusStyle(m *gpui.StyledMixin) *gpui.StyledMixin { return m.Border(3, blue) }

func (e *Example) item(w *gpui.Window, ix int, h *gpui.FocusHandle) gpui.IntoElement {
	d := gpui.NewDiv().ID(fmt.Sprintf("item-%d", ix)).TrackFocus(h).SizeH(40).
		Flex().JustifyCenter().ItemsCenter().Border(1, gpui.Black)
	if h.TabStop() && h.IsFocused(w) {
		d.Border(3, blue)
	}
	if h.TabStop() {
		return d.Hover(func(m *gpui.StyledMixin) *gpui.StyledMixin { return m.Bg(gpui.Black.Opacity(0.1)) }).
			Child(gpui.NewText(fmt.Sprintf("tab_index: %d", h.TabIndex())).TextSize(14).TextColor(gpui.Black))
	}
	return d.Opacity(0.4).Child(gpui.NewText("tab_stop: false").TextSize(14).TextColor(gpui.Black))
}

func (e *Example) button(id string, h *gpui.FocusHandle, label, msg string) *gpui.Div {
	return gpui.NewDiv().ID(id).TrackFocus(h).SizeH(40).Flex1().
		Flex().JustifyCenter().ItemsCenter().Border(1, gpui.Black).Bg(gpui.Black).
		Focus(focusStyle).ShadowSm().
		OnClick(func(_ *gpui.ClickEvent, w *gpui.Window, _ *gpui.App) {
			e.message = msg
			w.Refresh()
		}).
		Child(gpui.NewText(label).TextSize(14).TextColor(gpui.White))
}

func (e *Example) step(w *gpui.Window, next bool) {
	if next {
		w.FocusNext()
		e.message = "You have pressed `Tab`."
	} else {
		w.FocusPrev()
		e.message = "You have pressed `Shift-Tab`."
	}
}

// Render builds the element tree.
func (e *Example) Render(w *gpui.Window, cx *gpui.App) gpui.IntoElement {
	e.ensure(w, cx)
	app := gpui.NewDiv().ID("app").TrackFocus(e.root).
		OnAction(tabAction{}, func(_ gpui.Action, w *gpui.Window, _ *gpui.App) { e.step(w, true) }).
		OnAction(tabPrevAction{}, func(_ gpui.Action, w *gpui.Window, _ *gpui.App) { e.step(w, false) }).
		SizeFull().FlexCol().P(16).Gap(12).Bg(gpui.White).TextColor(gpui.Black).
		Child(gpui.NewText(e.message).TextSize(14).TextColor(gpui.Black))
	for i, h := range e.items {
		app.Child(e.item(w, i, h))
	}
	return app.Child(gpui.NewDiv().FlexRow().Gap(12).ItemsCenter().Child(
		e.button("el1", e.buttons[0], "Button 1", "You have clicked Button 1."),
		e.button("el2", e.buttons[1], "Button 2", "You have clicked Button 2."),
	))
}

func main() {
	png := flag.String("png", "", "render one frame to this PNG path and exit")
	width := flag.Int("width", 800, "window width in logical pixels")
	height := flag.Int("height", 600, "window height in logical pixels")
	smoke := flag.Int("smoke", 0, "quit after this many milliseconds (interactive mode)")
	focus := flag.Int("focus", 1, "seed focus on this focusable (PNG mode; -1 for none)")
	flag.Parse()

	const hint = "Press `Tab`, `Shift-Tab` to switch focus."

	if *png != "" {
		km := gpui.NewKeymap()
		registerKeybinds(km)
		root := &Example{message: hint, seed: *focus}
		if err := gpui.RenderToPNG(*png, *width, *height, 1, root, gpui.WithKeymap(km)); err != nil {
			log.Fatal(err)
		}
		return
	}

	err := ebitenplatform.New().Run(func(cx *gpui.App) {
		registerKeybinds(cx.Keymap())
		opts := gpui.WindowOptions{Width: float32(*width), Height: float32(*height), Title: "Tab stop", Resizable: true}
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
