// Command scrollable tests two-way scrolling in a nested layout: a vertically
// scrolling page containing a horizontally scrolling strip.
//
// Ported from gpui-ce crates/gpui/examples/legacy/scrollable.rs
//
// Not ported: the example font loader. The -vscroll and -hscroll flags seed a
// scrolled state for PNG rendering.
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

var (
	blue  = gpui.MustParseHex("#0000ff")
	green = gpui.MustParseHex("#008000")
)

// Scrollable is the root view.
type Scrollable struct {
	vertical   *gpui.ScrollHandle
	horizontal *gpui.ScrollHandle
}

func newScrollable() *Scrollable {
	return &Scrollable{vertical: gpui.NewScrollHandle(), horizontal: gpui.NewScrollHandle()}
}

// Render builds the element tree.
func (s *Scrollable) Render(w *gpui.Window, cx *gpui.App) gpui.IntoElement {
	label := func(t string) *gpui.Text { return gpui.NewText(t).TextSize(14).TextColor(gpui.Black) }
	return gpui.NewDiv().ID("vertical").TrackScroll(s.vertical).
		SizeFull().FlexCol().P(16).OverflowScroll().Bg(gpui.White).Child(
		label("Example for test 2 way scroll in nested layout"),
		gpui.NewDiv().FlexCol().SizeH(5000).Border(1, blue).Bg(blue.Opacity(0.05)).P(16).Child(
			gpui.NewDiv().Pb(20).Child(
				gpui.NewDiv().ID("horizontal").TrackScroll(s.horizontal).
					OverflowScroll().Child(
					gpui.NewDiv().SizeWH(2000, 150).Bg(green.Opacity(0.1)).
						Hover(func(m *gpui.StyledMixin) *gpui.StyledMixin { return m.Bg(green.Opacity(0.2)) }).
						Border(1, green).P(16).Child(label("Scroll Horizontal")),
				),
			),
			label("Scroll Vertical"),
		),
	)
}

func main() {
	png := flag.String("png", "", "render one frame to this PNG path and exit")
	width := flag.Int("width", 500, "window width in logical pixels")
	height := flag.Int("height", 500, "window height in logical pixels")
	smoke := flag.Int("smoke", 0, "quit after this many milliseconds (interactive mode)")
	vscroll := flag.Float64("vscroll", 0, "seed the vertical scroll offset (PNG mode)")
	hscroll := flag.Float64("hscroll", 0, "seed the horizontal scroll offset (PNG mode)")
	flag.Parse()

	root := newScrollable()

	if *png != "" {
		root.vertical.SetOffset(gpui.Point{Y: gpui.Pixels(*vscroll)})
		root.horizontal.SetOffset(gpui.Point{X: gpui.Pixels(*hscroll)})
		if err := gpui.RenderToPNG(*png, *width, *height, 1, root); err != nil {
			log.Fatal(err)
		}
		return
	}

	err := ebitenplatform.New().Run(func(cx *gpui.App) {
		opts := gpui.WindowOptions{Width: float32(*width), Height: float32(*height), Title: "Scrollable", Resizable: true}
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
