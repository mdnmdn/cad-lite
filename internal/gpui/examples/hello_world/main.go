// Command hello_world renders a greeting and a row of colour swatches in a
// bordered, centred column.
//
// Ported from gpui-ce crates/gpui/examples/legacy/hello_world.rs
//
// Not ported: shadow_lg on the root, dashed swatch borders (drawn solid).
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

// HelloWorld is the root view.
type HelloWorld struct {
	text string
}

func swatch(bg, border gpui.Rgba) *gpui.Div {
	return gpui.NewDiv().SizeWH(32, 32).Bg(bg).Border(1, border).Rounded(6)
}

// Render builds the element tree.
func (h *HelloWorld) Render(w *gpui.Window, cx *gpui.App) gpui.IntoElement {
	white := gpui.White
	return gpui.NewDiv().FlexCol().Gap(12).SizeFull().
		Bg(gpui.MustParseHex("#505050")).
		ItemsCenter().
		Border(1, gpui.MustParseHex("#0000ff")).
		Child(
			gpui.NewText(fmt.Sprintf("Hello, %s!", h.text)).TextSize(20).TextColor(white),
			gpui.NewDiv().FlexRow().Gap(8).Child(
				swatch(gpui.MustParseHex("#ff0000"), white),
				swatch(gpui.MustParseHex("#008000"), white),
				swatch(gpui.MustParseHex("#0000ff"), white),
				swatch(gpui.MustParseHex("#ffff00"), white),
				swatch(gpui.Black, white),
				swatch(white, gpui.Black),
			),
		).JustifyCenter()
}

func main() {
	png := flag.String("png", "", "render one frame to this PNG path and exit")
	width := flag.Int("width", 500, "window width in logical pixels")
	height := flag.Int("height", 500, "window height in logical pixels")
	smoke := flag.Int("smoke", 0, "quit after this many milliseconds (interactive mode)")
	flag.Parse()

	root := &HelloWorld{text: "World"}

	if *png != "" {
		if err := gpui.RenderToPNG(*png, *width, *height, 1, root); err != nil {
			log.Fatal(err)
		}
		return
	}

	err := ebitenplatform.New().Run(func(cx *gpui.App) {
		opts := gpui.WindowOptions{Width: float32(*width), Height: float32(*height), Title: "Hello World", Resizable: true}
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
