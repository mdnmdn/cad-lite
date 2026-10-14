// Command gallery is a static showcase of gpui's layout and styling engine.
// With -png it renders one frame headlessly; otherwise it opens a window.
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

// Gallery is the root view; it holds no state.
type Gallery struct{}

func run(text string, mutate func(*gpui.TextStyle)) gpui.TextRun {
	ts := gpui.DefaultTextStyle()
	ts.Color = gpui.Gray800
	if mutate != nil {
		mutate(&ts)
	}
	return gpui.TextRun{Text: text, Style: ts}
}

func section(title string, body gpui.IntoElement) *gpui.Div {
	return gpui.NewDiv().FlexCol().Gap(6).Child(
		gpui.NewText(title).TextSize(12).FontWeight(700).TextColor(gpui.Gray500),
		body,
	)
}

func swatch(c gpui.Rgba, label string) *gpui.Div {
	d := gpui.NewDiv().SizeWH(72, 48).Bg(c).Rounded(8).ItemsCenter().
		Child(gpui.NewText(label).TextSize(12).TextColor(gpui.White))
	d.JustifyCenter()
	return d
}

func card(title, body string, accent gpui.Rgba) *gpui.Div {
	return gpui.NewDiv().Flex1().MinW(0).FlexCol().Gap(4).P(12).Bg(gpui.White).Rounded(10).
		Border(1, gpui.Gray300).
		Child(
			gpui.NewDiv().FlexRow().Gap(6).ItemsCenter().Child(
				gpui.NewDiv().SizeWH(10, 10).Bg(accent).Rounded(5),
				gpui.NewText(title).FontWeight(700).TextColor(gpui.Gray900),
			),
			gpui.NewText(body).TextSize(13).TextColor(gpui.Gray700),
		)
}

// Render builds the element tree.
func (Gallery) Render(w *gpui.Window, cx *gpui.App) gpui.IntoElement {
	colors := gpui.NewDiv().FlexRow().Gap(8).Child(
		swatch(gpui.Blue500, "blue"),
		swatch(gpui.Red500, "red"),
		swatch(gpui.Green600, "green"),
		swatch(gpui.Yellow400, "yellow"),
		swatch(gpui.Gray800, "gray"),
	)

	corners := gpui.NewDiv().FlexRow().Gap(8).ItemsCenter().Child(
		gpui.NewDiv().SizeWH(48, 48).Bg(gpui.Blue500),
		gpui.NewDiv().SizeWH(48, 48).Bg(gpui.Blue500).Rounded(10),
		gpui.NewDiv().SizeWH(48, 48).Bg(gpui.Blue500).Rounded(24),
		gpui.NewDiv().SizeWH(48, 48).Border(3, gpui.Red500).Rounded(10),
		gpui.NewDiv().SizeWH(48, 48).Bg(gpui.Yellow400).Border(2, gpui.Gray800).Rounded(24),
	)

	between := gpui.NewDiv().FlexRow().ItemsCenter().JustifyBetween().P(8).Bg(gpui.Gray200).Rounded(8).Child(
		gpui.NewDiv().SizeWH(40, 24).Bg(gpui.Blue500).Rounded(4),
		gpui.NewDiv().SizeWH(40, 40).Bg(gpui.Red500).Rounded(4),
		gpui.NewDiv().SizeWH(40, 24).Bg(gpui.Green600).Rounded(4),
	)

	grow := gpui.NewDiv().FlexRow().Gap(8).Child(
		gpui.NewDiv().SizeW(80).P(8).Bg(gpui.Gray300).Rounded(6).Child(gpui.NewText("fixed")),
		gpui.NewDiv().Flex1().P(8).Bg(gpui.Blue500).Rounded(6).Child(gpui.NewText("flex 1").TextColor(gpui.White)),
		gpui.NewDiv().Flex1().P(8).Bg(gpui.Green600).Rounded(6).Child(gpui.NewText("flex 1").TextColor(gpui.White)),
	)

	rich := gpui.NewStyledText(
		run("Styled text mixes ", nil),
		run("bold", func(t *gpui.TextStyle) { t.FontWeight = 700 }),
		run(", ", nil),
		run("colour", func(t *gpui.TextStyle) { t.Color = gpui.Red500 }),
		run(", ", nil),
		run("strikethrough", func(t *gpui.TextStyle) { t.Strikethrough = true; t.Color = gpui.Gray500 }),
		run(" and ", nil),
		run("big blue bold", func(t *gpui.TextStyle) { t.FontSize = 20; t.FontWeight = 700; t.Color = gpui.Blue600 }),
		run(" runs.", nil),
	)

	wrapped := gpui.NewDiv().SizeW(260).P(10).Bg(gpui.White).Rounded(8).Border(1, gpui.Gray300).Child(
		gpui.NewText("Text wraps to the width its container gives it, breaking at word boundaries onto as many lines as it needs.").
			TextSize(13).TextColor(gpui.Gray800),
	)

	cards := gpui.NewDiv().FlexRow().Gap(10).Child(
		card("Layout", "Flexbox rows, columns, gap and grow.", gpui.Blue500),
		card("Style", "Borders, radii, colour and padding.", gpui.Green600),
		card("Text", "Runs, weights and wrapping.", gpui.Red500),
	)

	return gpui.NewDiv().FlexCol().SizeFull().Gap(14).P(20).Bg(gpui.Gray100).OverflowHidden().
		Child(
			gpui.NewText("gpui gallery").TextSize(26).FontWeight(700).TextColor(gpui.Gray900),
			section("COLOURS", colors),
			section("CORNERS AND BORDERS", corners),
			section("JUSTIFY BETWEEN, ALIGN CENTER", between),
			section("FIXED AND FLEX-GROW", grow),
			section("STYLED TEXT RUNS", rich),
			section("WRAPPING", wrapped),
			section("NESTED CARDS", cards),
		)
}

func main() {
	png := flag.String("png", "", "render one frame to this PNG path and exit")
	width := flag.Int("width", 520, "window width in logical pixels")
	height := flag.Int("height", 760, "window height in logical pixels")
	smoke := flag.Int("smoke", 0, "quit after this many milliseconds (interactive mode)")
	flag.Parse()

	if *png != "" {
		if err := gpui.RenderToPNG(*png, *width, *height, 1, Gallery{}); err != nil {
			log.Fatal(err)
		}
		return
	}

	err := ebitenplatform.New().Run(func(cx *gpui.App) {
		opts := gpui.WindowOptions{Width: float32(*width), Height: float32(*height), Title: "Gallery", Resizable: true}
		if err := cx.OpenWindow(opts, func(*gpui.Window, *gpui.App) gpui.EntityRoot { return Gallery{} }); err != nil {
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
