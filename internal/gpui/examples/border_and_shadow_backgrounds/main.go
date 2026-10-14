// Command border_and_shadow_backgrounds shows cards with coloured borders,
// drop shadows and inset shadows on rounded elements.
//
// Ported from gpui-ce crates/gpui/examples/border_and_shadow_backgrounds.rs
//
// Not ported: gradient borders and gradient shadows (each uses the solid
// colour of its gradient's first stop, so the ocean/sunset blend is lost),
// rounded_smoothing (corners are circular). The -scroll flag seeds a scrolled state for
// PNG rendering.
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

// ocean and sunset are the first stops of the source's gradients, with the
// source's alpha applied by the caller.
func ocean(alpha float32) gpui.Rgba {
	return gpui.Hsla{H: 0.52, S: 0.95, L: 0.62, A: alpha}.ToRgba()
}

func sunset(alpha float32) gpui.Rgba {
	return gpui.Hsla{H: 0.98, S: 0.92, L: 0.64, A: alpha}.ToRgba()
}

func hex(s string) gpui.Rgba { return gpui.MustParseHex(s) }

func panel(title, description string, preview gpui.IntoElement) *gpui.Div {
	return gpui.NewDiv().SizeWH(420, 250).P(24).FlexCol().Gap(16).Rounded(24).
		Bg(hex("#12182a")).Border(1, hex("#28324a")).Child(
		gpui.NewDiv().SizeH(140).Flex().ItemsCenter().JustifyCenter().Child(preview),
		gpui.NewDiv().FlexCol().Gap(4).Child(
			gpui.NewText(title).TextSize(16).TextColor(hex("#f1f5f9")).FontWeight(600),
			gpui.NewText(description).TextSize(14).TextColor(hex("#96a3bd")),
		),
	)
}

func previewCard(label string) *gpui.Div {
	return gpui.NewDiv().SizeWH(230, 96).Rounded(22).Bg(hex("#f8fafc")).
		Flex().ItemsCenter().JustifyCenter().Child(
		gpui.NewText(label).TextSize(16).TextColor(hex("#172033")).FontWeight(600),
	)
}

// Showcase is the root view.
type Showcase struct {
	scroll *gpui.ScrollHandle
}

// Render builds the element tree.
func (s *Showcase) Render(w *gpui.Window, cx *gpui.App) gpui.IntoElement {
	border := previewCard("Gradient border").Border(4, ocean(1))

	dropShadow := previewCard("Gradient drop shadow").Shadow(
		gpui.NewBoxShadow(-12, 12, ocean(0.58)).WithBlur(22).WithSpread(2),
		gpui.NewBoxShadow(12, 12, sunset(0.46)).WithBlur(22).WithSpread(2),
	)

	insetShadow := previewCard("Gradient inset shadow").Shadow(
		gpui.NewBoxShadow(0, 0, sunset(0.9)).WithBlur(12).WithSpread(7).AsInset(),
	)

	combined := previewCard("Combined").Border(4, sunset(1)).Shadow(
		gpui.NewBoxShadow(0, 14, ocean(0.58)).WithBlur(24).WithSpread(3),
		gpui.NewBoxShadow(0, 0, ocean(0.7)).WithBlur(8).WithSpread(4).AsInset(),
	)

	return gpui.NewDiv().ID("border-and-shadow-backgrounds").TrackScroll(s.scroll).
		SizeFull().OverflowYScroll().Bg(hex("#080c18")).P(32).FlexCol().Gap(24).Child(
		gpui.NewDiv().FlexCol().Gap(8).Child(
			gpui.NewText("Background borders & shadows").TextSize(24).TextColor(hex("#f1f5f9")).FontWeight(700),
			gpui.NewText("Coloured borders and box-shadow paint on rounded cards.").TextSize(16).TextColor(hex("#a7b2c9")),
		),
		gpui.NewDiv().FlexRow().FlexWrap().JustifyCenter().Gap(24).Child(
			panel("Border background", "A border follows the rounded quad.", border),
			panel("Drop-shadow backgrounds", "Two translucent shadows overlap outside the card.", dropShadow),
			panel("Inset-shadow background", "A warm shadow is blurred and clipped inside the card.", insetShadow),
			panel("Border + drop + inset", "Border, drop and inset paths render on one rounded element.", combined),
		),
	)
}

func main() {
	png := flag.String("png", "", "render one frame to this PNG path and exit")
	width := flag.Int("width", 1000, "window width in logical pixels")
	height := flag.Int("height", 760, "window height in logical pixels")
	smoke := flag.Int("smoke", 0, "quit after this many milliseconds (interactive mode)")
	scroll := flag.Float64("scroll", 0, "seed the vertical scroll offset (PNG mode)")
	flag.Parse()

	root := &Showcase{scroll: gpui.NewScrollHandle()}

	if *png != "" {
		root.scroll.SetOffset(gpui.Point{Y: gpui.Pixels(*scroll)})
		if err := gpui.RenderToPNG(*png, *width, *height, 1, root); err != nil {
			log.Fatal(err)
		}
		return
	}

	err := ebitenplatform.New().Run(func(cx *gpui.App) {
		opts := gpui.WindowOptions{Width: float32(*width), Height: float32(*height), Title: "Border and shadow backgrounds", Resizable: true}
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
