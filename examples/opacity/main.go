// Command opacity shows element opacity: a translucent panel with a shadow,
// nested content and per-item hover opacity. Click the panel to replay the
// fade-in animation.
//
// Ported from gpui-ce crates/gpui/examples/legacy/opacity.rs
//
// Not ported: img and svg children, the emoji row (letters stand in: the
// bundled fonts have no emoji glyphs), the wavy text decoration and the asset
// source.
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

var (
	blue   = gpui.MustParseHex("#0000ff")
	red    = gpui.MustParseHex("#ff0000")
	yellow = gpui.MustParseHex("#ffff00")
)

// Opacity is the root view.
type Opacity struct {
	opacity   float32
	animating bool
}

func (o *Opacity) startAnimation(_ *gpui.ClickEvent, w *gpui.Window, _ *gpui.App) {
	o.opacity = 0
	o.animating = true
	w.Refresh()
}

func emoji(s string) *gpui.Div {
	return gpui.NewDiv().ID("emoji-" + s).
		Hover(func(m *gpui.StyledMixin) *gpui.StyledMixin { return m.Opacity(0.5) }).
		Child(gpui.NewText(s).TextSize(20).TextColor(yellow))
}

// Render builds the element tree.
func (o *Opacity) Render(w *gpui.Window, cx *gpui.App) gpui.IntoElement {
	if o.animating {
		o.opacity += 0.005
		if o.opacity >= 1 {
			o.animating = false
			o.opacity = 1
		} else {
			w.Refresh()
		}
	}

	panel := gpui.NewDiv().ID("panel").OnClick(o.startAnimation).
		Absolute().Inset(32).Opacity(o.opacity).FlexCol().JustifyCenter().ItemsCenter().
		Bg(gpui.White).Border(3, red).TextColor(yellow).Child(
		gpui.NewDiv().FlexCol().Gap(8).JustifyCenter().ItemsCenter().SizeWH(300, 300).
			Bg(blue).Border(3, gpui.Black).
			Shadow(gpui.NewBoxShadow(10, 10, gpui.Black.Opacity(0.5)).WithBlur(1).WithSpread(5)).Child(
			gpui.NewText("Opacity Panel (Click to test)").TextSize(20).TextColor(yellow),
			gpui.NewDiv().ID("deep-level-text").P(16).Bg(gpui.Black).Child(
				gpui.NewText(fmt.Sprintf("opacity: %.1f", o.opacity)).TextSize(20).TextColor(gpui.White),
			),
			gpui.NewDiv().FlexRow().Child(
				emoji("A"), emoji("B"), emoji("C"), emoji("D"), emoji("E"), emoji("F"),
			),
		),
	)

	return gpui.NewDiv().FlexCol().SizeFull().Bg(gpui.MustParseHex("#e0e0e0")).Child(
		gpui.NewDiv().SizeFull().FlexCol().JustifyCenter().ItemsCenter().Border(1, gpui.Black).Child(
			gpui.NewText("This is background text.").TextSize(20).TextColor(blue),
		),
		panel,
	)
}

func main() {
	png := flag.String("png", "", "render one frame to this PNG path and exit")
	width := flag.Int("width", 500, "window width in logical pixels")
	height := flag.Int("height", 500, "window height in logical pixels")
	smoke := flag.Int("smoke", 0, "quit after this many milliseconds (interactive mode)")
	level := flag.Float64("opacity", 0.5, "initial panel opacity")
	flag.Parse()

	root := &Opacity{opacity: float32(*level)}

	if *png != "" {
		if err := gpui.RenderToPNG(*png, *width, *height, 1, root); err != nil {
			log.Fatal(err)
		}
		return
	}

	err := ebitenplatform.New().Run(func(cx *gpui.App) {
		opts := gpui.WindowOptions{Width: float32(*width), Height: float32(*height), Title: "Opacity", Resizable: true}
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
