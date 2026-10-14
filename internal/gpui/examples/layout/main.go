// Command layout demonstrates layout approaches: flexbox rows and columns with
// alignment, justification and grow/shrink, grid-like arrangements, and common
// patterns (app shell, centering, stacking).
//
// Ported from gpui-ce crates/gpui/examples/learn/layout.rs
//
// Not ported: CSS grid (grid_cols/col_span/row_span) is approximated with
// flex rows.
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

// colors mirrors gpui::colors::Colors::light().
type colors struct {
	text, selectedText, background, disabled, selected, border, container gpui.Rgba
}

func lightColors() colors {
	return colors{
		text:         gpui.MustParseHex("#252525"),
		selectedText: gpui.MustParseHex("#ffffff"),
		background:   gpui.MustParseHex("#ffffff"),
		disabled:     gpui.MustParseHex("#b0b0b0"),
		selected:     gpui.MustParseHex("#2a63d9"),
		border:       gpui.MustParseHex("#d9d9d9"),
		container:    gpui.MustParseHex("#f4f5f5"),
	}
}

var (
	red    = gpui.MustParseHex("#ff0000")
	green  = gpui.MustParseHex("#008000")
	blue   = gpui.MustParseHex("#0000ff")
	yellow = gpui.MustParseHex("#ffff00")
)

func label(s string, size float32, c gpui.Rgba) *gpui.Text {
	return gpui.NewText(s).TextSize(size).TextColor(c)
}

func block(text string, color, textColor gpui.Rgba) *gpui.Div {
	d := gpui.NewDiv().FlexRow().ItemsCenter().Bg(color).
		Border(1, gpui.White.Opacity(0.3)).Rounded(6).
		Child(label(text, 12, textColor))
	return d.JustifyCenter()
}

func caption(c colors, s string) *gpui.Text { return label(s, 12, c.disabled) }

func titled(c colors, caption_ string, body gpui.IntoElement, gap float32) *gpui.Div {
	return gpui.NewDiv().FlexCol().Gap(gap).Child(caption(c, caption_), body)
}

func flexboxRow(c colors) *gpui.Div {
	return titled(c, "flex().flex_row().gap_2()", gpui.NewDiv().FlexRow().Gap(8).Child(
		block("A", red, c.selectedText).SizeWH(32, 32),
		block("B", green, c.selectedText).SizeWH(32, 32),
		block("C", blue, c.selectedText).SizeWH(32, 32),
	), 8)
}

func flexboxColumn(c colors) *gpui.Div {
	return titled(c, "flex().flex_col().gap_2()", gpui.NewDiv().SizeH(96).FlexCol().Gap(8).Child(
		block("A", red, c.selectedText).SizeH(24),
		block("B", green, c.selectedText).SizeH(24),
		block("C", blue, c.selectedText).SizeH(24),
	), 8)
}

func flexboxJustify(c colors) *gpui.Div {
	tag := func(s string, col gpui.Rgba) *gpui.Div {
		return block(s, col, c.selectedText).Px_(8).Py_(4)
	}
	strip := func() *gpui.Div {
		return gpui.NewDiv().FlexRow().P(4).Bg(c.container).Rounded(2)
	}
	return titled(c, "justify_between / justify_center / justify_end", gpui.NewDiv().FlexCol().Gap(4).Child(
		strip().JustifyBetween().Child(tag("Start", red), tag("End", blue)),
		strip().JustifyCenter().Child(tag("Center", green)),
		strip().JustifyEnd().Child(tag("End", yellow)),
	), 8)
}

func flexboxGrow(c colors) *gpui.Div {
	fixed := func(col gpui.Rgba) *gpui.Div {
		return block("fixed", col, c.selectedText).FlexGrow(0).FlexShrink(0).SizeWH(64, 32)
	}
	return titled(c, "flex_1 (grow) vs flex_none (fixed)", gpui.NewDiv().FlexRow().Gap(8).Child(
		fixed(red),
		block("flex_1 (grows)", green, c.selectedText).Flex1().SizeH(32),
		fixed(blue),
	), 8)
}

// gridRow lays out one row of an n-column grid with equal-width cells.
func gridRow(cells ...gpui.IntoElement) *gpui.Div {
	return gpui.NewDiv().FlexRow().Gap(4).Child(cells...)
}

func gridBasic(c colors) *gpui.Div {
	cell := func(s string, col gpui.Rgba) *gpui.Div {
		return block(s, col, c.selectedText).Flex1().MinW(0).SizeH(32)
	}
	return titled(c, "grid().grid_cols(3).gap_1()", gpui.NewDiv().FlexCol().Gap(4).Child(
		gridRow(cell("1", red), cell("2", green), cell("3", blue)),
		gridRow(cell("4", yellow), cell("5", red), cell("6", green)),
	), 8)
}

func gridSpan(c colors) *gpui.Div {
	side := block("Side", green, c.selectedText).MinW(0).SizeH(52)
	content := block("Content (col_span 3)", blue, c.selectedText).MinW(0).SizeH(52)
	for i, d := range []*gpui.Div{side, content} {
		l := gpui.Flex(float32(1 + 2*i))
		d.StyleRef().Width = &l
	}
	return titled(c, "col_span / row_span", gpui.NewDiv().FlexCol().Gap(4).Child(
		block("Header (col_span_full)", red, c.selectedText).SizeH(24),
		gridRow(side, content),
	), 8)
}

func appShell(c colors) *gpui.Div {
	header := gpui.NewDiv().SizeH(24).FlexRow().ItemsCenter().Px_(8).Bg(c.selected).
		Child(label("Header", 12, c.selectedText))
	centered := func() *gpui.Div { return gpui.NewDiv().FlexRow().ItemsCenter().JustifyCenter() }
	side := centered().SizeW(64).Bg(c.container).Child(label("Side", 12, c.disabled))
	content := centered().Flex1().Bg(c.background).Child(label("Content", 12, c.disabled))
	return titled(c, "App Shell: Header + Sidebar + Content",
		gpui.NewDiv().SizeH(128).FlexCol().Border(1, c.border).Rounded(6).OverflowHidden().Child(
			header,
			gpui.NewDiv().Flex1().FlexRow().Child(side, content),
		), 8)
}

func centering(c colors) *gpui.Div {
	chip := gpui.NewDiv().Px_(16).Py_(8).Bg(c.selected).Rounded(6).
		Child(label("Perfectly Centered", 12, c.text))
	return titled(c, "Centering: items_center + justify_center",
		gpui.NewDiv().SizeH(80).FlexRow().ItemsCenter().JustifyCenter().Bg(c.container).Rounded(6).Child(chip), 8)
}

func stack(c colors) *gpui.Div {
	box := func(col gpui.Rgba, top, left float32) *gpui.Div {
		return gpui.NewDiv().Absolute().Top(top).Left(left).SizeWH(40, 40).Bg(col.Opacity(0.7)).Rounded(6)
	}
	return titled(c, "Stack: Overlapping with absolute positioning",
		gpui.NewDiv().SizeH(80).FlexRow().ItemsStart().Bg(c.container).Rounded(6).Child(
			box(red, 8, 8), box(green, 16, 24), box(blue, 24, 40),
		), 8)
}

func section(c colors, title string, content gpui.IntoElement) *gpui.Div {
	return gpui.NewDiv().FlexCol().Gap(8).P(12).Bg(c.container.Opacity(0.5)).Rounded(8).Child(
		label(title, 14, c.text).FontWeight(600),
		content,
	)
}

// LayoutExample is the root view; it holds no state.
type LayoutExample struct{}

// Render builds the element tree.
func (LayoutExample) Render(w *gpui.Window, cx *gpui.App) gpui.IntoElement {
	c := lightColors()
	return gpui.NewDiv().ID("main").SizeFull().P(16).Bg(c.background).OverflowScroll().Child(
		gpui.NewDiv().FlexCol().Gap(16).MaxW(600).Child(
			gpui.NewDiv().FlexCol().Gap(4).Child(
				label("Layout Patterns", 20, c.text).FontWeight(700),
				label("Flexbox, Grid, and common layout patterns in GPUI", 14, c.disabled),
			),
			section(c, "Flexbox: Row", flexboxRow(c)),
			section(c, "Flexbox: Column", flexboxColumn(c)),
			section(c, "Flexbox: Justify Content", flexboxJustify(c)),
			section(c, "Flexbox: Grow/Shrink", flexboxGrow(c)),
			section(c, "Grid: Basic", gridBasic(c)),
			section(c, "Grid: Spans", gridSpan(c)),
			section(c, "Pattern: App Shell", appShell(c)),
			section(c, "Pattern: Centering", centering(c)),
			section(c, "Pattern: Stack", stack(c)),
		),
	)
}

func main() {
	png := flag.String("png", "", "render one frame to this PNG path and exit")
	width := flag.Int("width", 650, "window width in logical pixels")
	height := flag.Int("height", 700, "window height in logical pixels")
	smoke := flag.Int("smoke", 0, "quit after this many milliseconds (interactive mode)")
	flag.Parse()

	if *png != "" {
		if err := gpui.RenderToPNG(*png, *width, *height, 1, LayoutExample{}); err != nil {
			log.Fatal(err)
		}
		return
	}

	err := ebitenplatform.New().Run(func(cx *gpui.App) {
		opts := gpui.WindowOptions{Width: float32(*width), Height: float32(*height), Title: "Layout", Resizable: true}
		if err := cx.OpenWindow(opts, func(*gpui.Window, *gpui.App) gpui.EntityRoot { return LayoutExample{} }); err != nil {
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
