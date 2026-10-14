// Command actions_and_keybinds binds typed actions to keyboard shortcuts in a
// focused view: Space increments the counter and Backspace resets it.
//
// Ported from gpui-ce crates/gpui/examples/learn/actions_and_keybinds.rs
package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"time"

	"cad-lite/internal/gpui"
	"cad-lite/internal/gpui/ebitenplatform"
)

type incrementAction struct{}

func (incrementAction) Name() string              { return "keybinds_example::Increment" }
func (incrementAction) Clone() gpui.Action        { return incrementAction{} }
func (incrementAction) Equals(o gpui.Action) bool { _, ok := o.(incrementAction); return ok }

type resetAction struct{}

func (resetAction) Name() string              { return "keybinds_example::Reset" }
func (resetAction) Clone() gpui.Action        { return resetAction{} }
func (resetAction) Equals(o gpui.Action) bool { _, ok := o.(resetAction); return ok }

var (
	background = gpui.MustParseHex("#ffffff")
	textColor  = gpui.MustParseHex("#252525")
)

func registerKeybinds(km *gpui.Keymap) {
	km.AddBindings(
		gpui.NewKeyBinding("space", incrementAction{}, "Counter"),
		gpui.NewKeyBinding("backspace", resetAction{}, "Counter"),
	)
}

// ActionsExample is the root view.
type ActionsExample struct {
	count int
	focus *gpui.FocusHandle
}

// Render builds the element tree.
func (a *ActionsExample) Render(w *gpui.Window, cx *gpui.App) gpui.IntoElement {
	return gpui.NewDiv().ID("counter").KeyContext("Counter").TrackFocus(a.focus).
		OnAction(incrementAction{}, func(_ gpui.Action, w *gpui.Window, _ *gpui.App) {
			a.count++
			w.Refresh()
		}).
		OnAction(resetAction{}, func(_ gpui.Action, w *gpui.Window, _ *gpui.App) {
			a.count = 0
			w.Refresh()
		}).
		FlexCol().ItemsCenter().JustifyCenter().Gap(16).SizeFull().Bg(background).
		Child(
			gpui.NewText(fmt.Sprintf("Count: %d", a.count)).TextSize(30).TextColor(textColor),
			gpui.NewText("Space to increment. Backspace to reset.").TextSize(16).TextColor(textColor),
		)
}

func main() {
	png := flag.String("png", "", "render one frame to this PNG path and exit")
	width := flag.Int("width", 480, "window width in logical pixels")
	height := flag.Int("height", 240, "window height in logical pixels")
	smoke := flag.Int("smoke", 0, "quit after this many milliseconds (interactive mode)")
	flag.Parse()

	if *png != "" {
		km := gpui.NewKeymap()
		registerKeybinds(km)
		root := &ActionsExample{count: 3}
		if err := gpui.RenderToPNG(*png, *width, *height, 1, root, gpui.WithKeymap(km)); err != nil {
			log.Fatal(err)
		}
		return
	}

	err := ebitenplatform.New().Run(func(cx *gpui.App) {
		registerKeybinds(cx.Keymap())
		opts := gpui.WindowOptions{Width: float32(*width), Height: float32(*height), Title: "Actions and keybinds", Resizable: true}
		err := cx.OpenWindow(opts, func(w *gpui.Window, cx *gpui.App) gpui.EntityRoot {
			root := &ActionsExample{focus: cx.NewFocusHandle()}
			w.RequestFocus(root.focus)
			return root
		})
		if err != nil {
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
