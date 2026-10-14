// Command uniform_list scrolls through 10,000 rows with a virtualized list.
// Each row has the same height, and the list requests only the row range
// needed for the viewport.
//
// Ported from gpui-ce crates/gpui/examples/learn/uniform_list.rs
//
// Not ported: Colors::for_appearance (the light palette is hard-coded) and
// the example prelude. The -scroll flag seeds a scrolled state for PNG
// rendering and ShowScrollbar is enabled as an addition.
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

var (
	textColor  = gpui.MustParseHex("#252525")
	background = gpui.MustParseHex("#ffffff")
	container  = gpui.MustParseHex("#f4f5f5")
)

// UniformListExample is the root view.
type UniformListExample struct {
	scroll *gpui.ScrollHandle
}

// Render builds the element tree.
func (u *UniformListExample) Render(w *gpui.Window, cx *gpui.App) gpui.IntoElement {
	return gpui.NewDiv().FlexCol().Gap(12).SizeFull().P(16).OverflowHidden().
		Bg(background).TextColor(textColor).Child(
		gpui.NewText("10,000 rows. Scroll to see more.").TextSize(14).TextColor(textColor),
		gpui.NewUniformList("items", 10_000, func(start, end int, w *gpui.Window, cx *gpui.App) []gpui.IntoElement {
			items := make([]gpui.IntoElement, 0, end-start)
			for i := start; i < end; i++ {
				items = append(items, gpui.NewDiv().ID(fmt.Sprintf("item-%d", i)).SizeH(24).Px_(8).Child(
					gpui.NewText(fmt.Sprintf("Item %d", i)).TextSize(14).TextColor(textColor),
				))
			}
			return items
		}).TrackScroll(u.scroll).ShowScrollbar().SizeH(300).Bg(container),
	)
}

func main() {
	png := flag.String("png", "", "render one frame to this PNG path and exit")
	width := flag.Int("width", 400, "window width in logical pixels")
	height := flag.Int("height", 400, "window height in logical pixels")
	smoke := flag.Int("smoke", 0, "quit after this many milliseconds (interactive mode)")
	scroll := flag.Float64("scroll", 0, "seed the list scroll offset (PNG mode)")
	flag.Parse()

	root := &UniformListExample{scroll: gpui.NewScrollHandle()}

	if *png != "" {
		root.scroll.SetOffset(gpui.Point{Y: gpui.Pixels(*scroll)})
		if err := gpui.RenderToPNG(*png, *width, *height, 1, root); err != nil {
			log.Fatal(err)
		}
		return
	}

	err := ebitenplatform.New().Run(func(cx *gpui.App) {
		opts := gpui.WindowOptions{Width: float32(*width), Height: float32(*height), Title: "Virtualized lists", Resizable: true}
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
