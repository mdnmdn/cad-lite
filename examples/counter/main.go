// Command counter is the minimal gpui app: a count, +/- buttons with hover
// styles and up/down key bindings. With -png it renders one frame headlessly.
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

type incAction struct{}

func (incAction) Name() string              { return "increment" }
func (incAction) Clone() gpui.Action        { return incAction{} }
func (incAction) Equals(o gpui.Action) bool { _, ok := o.(incAction); return ok }

type decAction struct{}

func (decAction) Name() string              { return "decrement" }
func (decAction) Clone() gpui.Action        { return decAction{} }
func (decAction) Equals(o gpui.Action) bool { _, ok := o.(decAction); return ok }

// Counter is the root view: a count and the id of the hovered button.
type Counter struct {
	count   int
	hovered string
}

func (c *Counter) add(delta int, w *gpui.Window) {
	c.count += delta
	w.Refresh()
}

func (c *Counter) button(id, label string, base, hover gpui.Rgba, delta int) *gpui.Div {
	bg := base
	if c.hovered == id {
		bg = hover
	}
	b := gpui.NewDiv().ID(id).SizeWH(56, 56).Bg(bg).Rounded(12).ItemsCenter().
		OnMouseMove(func(_ *gpui.MouseMoveEvent, w *gpui.Window, _ *gpui.App) {
			if c.hovered != id {
				c.hovered = id
				w.Refresh()
			}
		}).
		OnClick(func(_ *gpui.ClickEvent, w *gpui.Window, _ *gpui.App) { c.add(delta, w) }).
		Child(gpui.NewText(label).TextSize(28).FontWeight(700).TextColor(gpui.White))
	b.JustifyCenter()
	return b
}

// Render builds the element tree for the current state.
func (c *Counter) Render(w *gpui.Window, cx *gpui.App) gpui.IntoElement {
	root := gpui.NewDiv().KeyContext("counter").
		FlexCol().SizeFull().Gap(20).Bg(gpui.Gray50).ItemsCenter().
		OnMouseMove(func(_ *gpui.MouseMoveEvent, w *gpui.Window, _ *gpui.App) {
			if c.hovered != "" {
				c.hovered = ""
				w.Refresh()
			}
		}).
		OnAction(incAction{}, func(_ gpui.Action, w *gpui.Window, _ *gpui.App) { c.add(1, w) }).
		OnAction(decAction{}, func(_ gpui.Action, w *gpui.Window, _ *gpui.App) { c.add(-1, w) }).
		Child(
			gpui.NewText("Counter").TextSize(20).TextColor(gpui.Gray600),
			gpui.NewText(fmt.Sprint(c.count)).TextSize(72).FontWeight(700).TextColor(gpui.Gray900),
			gpui.NewDiv().FlexRow().Gap(16).
				Child(
					c.button("dec", "-", gpui.Red500, gpui.Red600, -1),
					c.button("inc", "+", gpui.Blue500, gpui.Blue600, 1),
				),
			gpui.NewText("Click the buttons or press up / down").TextSize(13).TextColor(gpui.Gray500),
		)
	root.JustifyCenter()
	return root
}

func main() {
	png := flag.String("png", "", "render one frame to this PNG path and exit")
	width := flag.Int("width", 360, "window width in logical pixels")
	height := flag.Int("height", 360, "window height in logical pixels")
	smoke := flag.Int("smoke", 0, "quit after this many milliseconds (interactive mode)")
	flag.Parse()

	c := &Counter{}

	if *png != "" {
		c.count = 7
		c.hovered = "inc"
		if err := gpui.RenderToPNG(*png, *width, *height, 1, c); err != nil {
			log.Fatal(err)
		}
		return
	}

	err := ebitenplatform.New().Run(func(cx *gpui.App) {
		cx.Keymap().AddBindings(
			gpui.NewKeyBinding("up", incAction{}, "counter"),
			gpui.NewKeyBinding("down", decAction{}, "counter"),
		)
		opts := gpui.WindowOptions{Width: float32(*width), Height: float32(*height), Title: "Counter", Resizable: true}
		if err := cx.OpenWindow(opts, func(*gpui.Window, *gpui.App) gpui.EntityRoot { return c }); err != nil {
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
