// Command text demonstrates text capabilities: font sizes and weights,
// alignment, decoration, line height, inline styled runs, overflow handling
// (ellipsis, line clamp, truncate, no-wrap) and a unicode/emoji character grid.
//
// Ported from gpui-ce crates/gpui/examples/learn/text.rs
//
// Not ported: italic (no italic faces; the Italic example and styled run are
// drawn upright), underline colour (drawn as a 1px accent bar under the text),
// strikethrough colour (uses the text colour), distinct weights (only regular
// and bold faces exist; THIN..MEDIUM render regular, SEMIBOLD..BLACK bold),
// text_ellipsis/line_clamp/truncate/whitespace_nowrap (emulated by pre-cutting
// the string to the container width measured at render time), CSS grid (the
// character grid is built from flex rows), scrolling beyond the window.
// Glyphs missing from the system font (CJK, emoji) render as tofu boxes.
package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"cad-lite/internal/gpui"
	"cad-lite/internal/gpui/ebitenplatform"
)

// colors mirrors gpui::colors::Colors::light().
type colors struct {
	text, background, disabled, selected, border, container gpui.Rgba
}

func lightColors() colors {
	return colors{
		text:       gpui.MustParseHex("#252525"),
		background: gpui.MustParseHex("#ffffff"),
		disabled:   gpui.MustParseHex("#b0b0b0"),
		selected:   gpui.MustParseHex("#2a63d9"),
		border:     gpui.MustParseHex("#d9d9d9"),
		container:  gpui.MustParseHex("#f4f5f5"),
	}
}

var errorRed = gpui.MustParseHex("#d32f2f")

func label(s string, size float32, c gpui.Rgba) *gpui.Text {
	return gpui.NewText(s).TextSize(size).TextColor(c)
}

func caption(c colors, s string) *gpui.Text { return label(s, 12, c.disabled) }

// lines stacks one Text per line of s: the Text element has no newline support,
// and Text measurement ignores TextStyle.LineHeight, so each line sits in a
// fixed-height row when a line height is requested.
func lines(s string, size, lh float32, c gpui.Rgba) *gpui.Div {
	d := gpui.NewDiv().FlexCol()
	for _, l := range strings.Split(s, "\n") {
		t := label(l, size, c)
		if lh > 0 {
			t.TextLineHeight(lh)
			d.Child(gpui.NewDiv().SizeH(lh).FlexRow().ItemsCenter().Child(t))
			continue
		}
		d.Child(t)
	}
	return d
}

func group(gap float32, children ...gpui.IntoElement) *gpui.Div {
	return gpui.NewDiv().FlexCol().Gap(gap).Child(children...)
}

func wrapRow(gap float32, children ...gpui.IntoElement) *gpui.Div {
	return gpui.NewDiv().FlexRow().Gap(gap).Child(children...)
}

func textSizes(c colors) gpui.IntoElement {
	return group(4,
		caption(c, "Font sizes: text_xs, text_sm, text_base, text_lg, text_xl"),
		gpui.NewDiv().FlexRow().ItemsEnd().Gap(12).Child(
			label("Extra Small", 12, c.text),
			label("Small", 14, c.text),
			label("Base", 16, c.text),
			label("Large", 18, c.text),
			label("Extra Large", 20, c.text),
		),
	)
}

func textWeights(c colors) gpui.IntoElement {
	weights := []struct {
		name string
		w    int
	}{{"Thin", 100}, {"Light", 300}, {"Normal", 400}, {"Medium", 500}, {"Semibold", 600}, {"Bold", 700}, {"Black", 900}}
	row := wrapRow(12)
	for _, w := range weights {
		row.Child(label(w.name, 16, c.text).FontWeight(w.w))
	}
	return group(4, caption(c, "Font weights: THIN through BLACK"), row)
}

func textAlignment(c colors) gpui.IntoElement {
	cell := func(s string, align func(*gpui.Div) *gpui.Div) *gpui.Div {
		d := gpui.NewDiv().FlexCol().P(8).Bg(c.container).Rounded(2).Child(label(s, 16, c.text))
		return align(d)
	}
	return group(8,
		caption(c, "Alignment: default (left), text_center, text_right"),
		group(4,
			cell("Left aligned (default)", func(d *gpui.Div) *gpui.Div { return d }),
			cell("Center aligned", func(d *gpui.Div) *gpui.Div { return d.ItemsCenter() }),
			cell("Right aligned", func(d *gpui.Div) *gpui.Div { return d.ItemsEnd() }),
		),
	)
}

func textDecoration(c colors) gpui.IntoElement {
	underlined := gpui.NewDiv().FlexCol().Child(
		label("Underlined text", 16, c.text),
		gpui.NewDiv().SizeH(1).Bg(c.selected),
	)
	return group(8,
		caption(c, "Decorations: underline, strikethrough, italic"),
		wrapRow(16,
			underlined,
			label("Strikethrough text", 16, c.text).TextStrikethrough(),
			label("Italic text", 16, c.text),
		),
	)
}

func lineHeight(c colors) gpui.IntoElement {
	col := func(s string, lh float32) *gpui.Div {
		return gpui.NewDiv().Flex1().P(8).Bg(c.container).Rounded(2).
			Child(lines(s, 14, lh, c.text))
	}
	return group(8,
		caption(c, "Line height: relative(1.0), relative(1.5), relative(2.0)"),
		gpui.NewDiv().FlexRow().Gap(12).Child(
			col("Tight\nline\nheight", 14),
			col("Normal\nline\nheight", 21),
			col("Loose\nline\nheight", 28),
		),
	)
}

func styledText(c colors) gpui.IntoElement {
	run := func(s string, weight int) gpui.TextRun {
		ts := gpui.DefaultTextStyle()
		ts.FontSize = 18
		ts.Color = c.text
		ts.FontWeight = weight
		return gpui.TextRun{Text: s, Style: ts}
	}
	return group(8,
		caption(c, "StyledText with inline highlights"),
		gpui.NewStyledText(
			run("Bold", 700), run(" ", 400), run("Italic", 400), run(" Normal ", 400), run("Semibold", 600),
		),
	)
}

// fitPrefix returns the longest rune prefix of s whose single-line width does
// not exceed width.
func fitPrefix(w *gpui.Window, s string, ts gpui.TextStyle, width float32) string {
	runes := []rune(s)
	lo, hi := 0, len(runes)
	for lo < hi {
		mid := (lo + hi + 1) / 2
		tw, _ := w.TextMeasurer().MeasureWrapped(string(runes[:mid]), ts, 1e6)
		if float32(tw) <= width {
			lo = mid
		} else {
			hi = mid - 1
		}
	}
	return string(runes[:lo])
}

// clampText emulates line clamping: it breaks s into at most lines lines of the
// given width (at word boundaries) and, when text remains, cuts the last line
// and appends ellipsis.
func clampText(w *gpui.Window, s string, ts gpui.TextStyle, width float32, lines int, ellipsis string) string {
	var out []string
	rest := s
	for i := 0; i < lines && rest != ""; i++ {
		last := i == lines-1
		budget := width
		if last && ellipsis != "" {
			tw, _ := w.TextMeasurer().MeasureWrapped(ellipsis, ts, 1e6)
			budget -= float32(tw)
		}
		if tw, _ := w.TextMeasurer().MeasureWrapped(rest, ts, 1e6); float32(tw) <= width {
			out = append(out, rest)
			rest = ""
			break
		}
		fit := fitPrefix(w, rest, ts, budget)
		if !last {
			if cut := strings.LastIndex(fit, " "); cut > 0 {
				fit = fit[:cut]
			}
		}
		rest = strings.TrimLeft(rest[len(fit):], " ")
		if last {
			fit = strings.TrimRight(fit, " ") + ellipsis
		}
		out = append(out, fit)
	}
	return strings.Join(out, "\n")
}

func textOverflow(w *gpui.Window, c colors) gpui.IntoElement {
	const long = "The quick brown fox jumps over the lazy dog. This is a long sentence that will overflow its container."
	avail := float32(w.Bounds().Size.W)
	if avail > 600+32 {
		avail = 600 + 32
	}
	avail -= 32 + 24 + 16 + 2
	ts := gpui.DefaultTextStyle()
	ts.FontSize = 16
	box := func(s string) *gpui.Div {
		return gpui.NewDiv().P(8).Bg(c.container).Border(1, c.border).Rounded(2).OverflowHidden().
			Child(lines(s, 16, 0, c.text))
	}
	item := func(title, s string) *gpui.Div {
		return group(4, caption(c, title), box(s))
	}
	return group(8,
		caption(c, "Overflow handling: ellipsis, truncate, line_clamp"),
		group(8,
			item("text_ellipsis (single line):", clampText(w, long, ts, avail, 1, "…")),
			item("line_clamp(2):", clampText(w, long, ts, avail, 2, "…")),
			item("truncate (hard cut):", clampText(w, long, ts, avail, 1, "")),
			item("whitespace_nowrap:", clampText(w, long, ts, avail, 1, "")),
		),
	)
}

func characterGrid(c colors) gpui.IntoElement {
	chars := []string{
		"A", "B", "C", "D", "E", "a", "b", "c", "d", "e",
		"0", "1", "2", "3", "4", "5", "6", "7", "8", "9",
		"α", "β", "γ", "δ", "ε", "θ", "λ", "π", "σ", "ω",
		"Д", "Ж", "И", "Л", "Ф", "Ц", "Ш", "Щ", "Ы", "Я",
		"你", "好", "世", "界", "日", "本", "語", "中", "文", "字",
		"→", "←", "↑", "↓", "•", "★", "♠", "♥", "♦", "♣",
		"😀", "🎉", "🚀", "💡", "🔥", "✨", "🎨", "📚", "🎵", "❤️",
	}
	grid := gpui.NewDiv().FlexCol().Gap(4)
	for i := 0; i < len(chars); i += 10 {
		row := gpui.NewDiv().FlexRow().Gap(4)
		for _, ch := range chars[i : i+10] {
			cell := gpui.NewDiv().SizeWH(32, 32).FlexRow().ItemsCenter().
				Child(label(ch, 18, c.text).TextLineHeight(18))
			row.Child(cell.JustifyCenter())
		}
		grid.Child(row)
	}
	return group(8,
		caption(c, "Unicode and emoji support"),
		gpui.NewDiv().P(8).Bg(c.container).Border(1, c.border).Rounded(6).Child(grid),
	)
}

func section(c colors, title string, content gpui.IntoElement) *gpui.Div {
	return gpui.NewDiv().FlexCol().Gap(8).P(12).Bg(c.container.Opacity(0.5)).Rounded(8).Child(
		label(title, 14, c.text).FontWeight(600),
		content,
	)
}

// TextExample is the root view; it holds no state.
type TextExample struct{}

// Render builds the element tree.
func (TextExample) Render(w *gpui.Window, cx *gpui.App) gpui.IntoElement {
	c := lightColors()
	return gpui.NewDiv().ID("main").SizeFull().P(16).Bg(c.background).OverflowScroll().Child(
		gpui.NewDiv().FlexCol().Gap(16).MaxW(600).Child(
			group(4,
				label("Text & Typography", 20, c.text).FontWeight(700),
				label("Font styling, alignment, overflow, and unicode support", 14, c.disabled),
			),
			section(c, "Font Sizes", textSizes(c)),
			section(c, "Font Weights", textWeights(c)),
			section(c, "Text Alignment", textAlignment(c)),
			section(c, "Text Decoration", textDecoration(c)),
			section(c, "Line Height", lineHeight(c)),
			section(c, "Styled Text", styledText(c)),
			section(c, "Text Overflow", textOverflow(w, c)),
			section(c, "Character Grid", characterGrid(c)),
		),
	)
}

func main() {
	png := flag.String("png", "", "render one frame to this PNG path and exit")
	width := flag.Int("width", 650, "window width in logical pixels")
	height := flag.Int("height", 900, "window height in logical pixels")
	smoke := flag.Int("smoke", 0, "quit after this many milliseconds (interactive mode)")
	flag.Parse()

	if *png != "" {
		if err := gpui.RenderToPNG(*png, *width, *height, 1, TextExample{}); err != nil {
			log.Fatal(err)
		}
		return
	}

	err := ebitenplatform.New().Run(func(cx *gpui.App) {
		opts := gpui.WindowOptions{Width: float32(*width), Height: float32(*height), Title: "Text", Resizable: true}
		if err := cx.OpenWindow(opts, func(*gpui.Window, *gpui.App) gpui.EntityRoot { return TextExample{} }); err != nil {
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
