// Command gradient shows linear gradients at many angles and colour-stop
// positions on rounded boxes. Click the button in the header to switch
// between Oklab and sRGB interpolation.
//
// Ported from gpui-ce crates/gpui/examples/legacy/gradient.rs
//
// Not ported: the canvas/paint_path trapezoid with a gradient fill (a plain
// gradient box stands in). The -oklab flag
// seeds the Oklab colour space for PNG rendering.
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
	red    = gpui.MustParseHex("#ff0000")
	green  = gpui.MustParseHex("#008000")
	blue   = gpui.MustParseHex("#0000ff")
	yellow = gpui.MustParseHex("#ffff00")
)

// GradientViewer is the root view.
type GradientViewer struct {
	colorSpace gpui.ColorSpace
}

func solid(c gpui.Rgba) *gpui.Div {
	return gpui.NewDiv().SizeFull().Rounded(12).Flex().ItemsCenter().JustifyCenter().Bg(c).Child(
		gpui.NewText("Solid Color").TextSize(14).TextColor(gpui.White),
	)
}

func (g *GradientViewer) box(angle float32, from gpui.Rgba, fromPct float32, to gpui.Rgba, toPct float32) *gpui.Div {
	return gpui.NewDiv().Flex1().Rounded(12).Bg(
		gpui.LinearGradient(angle, gpui.LinearColorStop(from, fromPct), gpui.LinearColorStop(to, toPct)).
			ColorSpace(g.colorSpace))
}

func row(children ...gpui.IntoElement) *gpui.Div {
	return gpui.NewDiv().FlexRow().Flex1().Gap(12).Child(children...)
}

// Render builds the element tree.
func (g *GradientViewer) Render(w *gpui.Window, cx *gpui.App) gpui.IntoElement {
	white := gpui.White
	return gpui.NewDiv().Bg(gpui.White).SizeFull().P(16).FlexCol().Gap(12).Child(
		gpui.NewDiv().FlexRow().Gap(8).JustifyBetween().ItemsCenter().Child(
			gpui.NewText("Gradient Examples").TextSize(14).TextColor(gpui.Black),
			gpui.NewDiv().ID("method").Px_(12).Py_(4).Bg(gpui.Black).
				Active(func(s *gpui.StyledMixin) *gpui.StyledMixin { return s.Bg(gpui.Gray700) }).
				OnClick(func(_ *gpui.ClickEvent, w *gpui.Window, _ *gpui.App) {
					if g.colorSpace == gpui.ColorSpaceOklab {
						g.colorSpace = gpui.ColorSpaceSrgb
					} else {
						g.colorSpace = gpui.ColorSpaceOklab
					}
					w.Refresh()
				}).
				Child(gpui.NewText(g.colorSpace.String()).TextSize(12).TextColor(gpui.White)),
		),
		row(solid(red), solid(blue)),
		row(
			g.box(45, red, 0, blue, 1),
			g.box(135, red, 0, green, 1),
			g.box(225, green, 0, blue, 1),
			g.box(315, green, 0, yellow, 1),
		),
		row(
			g.box(0, red, 0, white, 1),
			g.box(90, blue, 0, white, 1),
			g.box(180, green, 0, white, 1),
			g.box(360, yellow, 0, white, 1),
		),
		g.box(0, green, 0.05, yellow, 0.95),
		g.box(90, blue, 0.05, red, 0.95),
		row(
			g.box(90, blue, 0.5, red, 0.5),
			g.box(180, green, 0, blue, 0.5),
		),
		gpui.NewDiv().SizeH(96).FlexRow().JustifyCenter().Child(
			gpui.NewDiv().SizeWH(480, 80).Bg(
				gpui.LinearGradient(180, gpui.LinearColorStop(red, 0), gpui.LinearColorStop(blue, 1)).
					ColorSpace(g.colorSpace)),
		),
	)
}

func main() {
	png := flag.String("png", "", "render one frame to this PNG path and exit")
	width := flag.Int("width", 800, "window width in logical pixels")
	height := flag.Int("height", 800, "window height in logical pixels")
	smoke := flag.Int("smoke", 0, "quit after this many milliseconds (interactive mode)")
	oklab := flag.Bool("oklab", false, "seed the Oklab colour space (PNG mode)")
	flag.Parse()

	root := &GradientViewer{}

	if *png != "" {
		if *oklab {
			root.colorSpace = gpui.ColorSpaceOklab
		}
		if err := gpui.RenderToPNG(*png, *width, *height, 1, root); err != nil {
			log.Fatal(err)
		}
		return
	}

	err := ebitenplatform.New().Run(func(cx *gpui.App) {
		opts := gpui.WindowOptions{Width: float32(*width), Height: float32(*height), Title: "Gradient", Resizable: true}
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
