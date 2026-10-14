package gpui_test

import (
	"fmt"
	"testing"

	"cad-lite/internal/gpui"
)

// benchTodo mirrors examples/todo's view with a populated list.
type benchTodo struct {
	titles []string
	done   []bool
	input  *gpui.TextInput
}

func newBenchTodo() *benchTodo {
	return &benchTodo{
		titles: []string{"Port GPUI to Go", "Write the flexbox engine", "Render text with strikethrough", "Ship the todo example", "Profile the rasterizer", "Make idle cost zero CPU"},
		done:   []bool{false, true, true, false, false, true},
		input:  gpui.NewTextInput("What needs doing?"),
	}
}

func (t *benchTodo) Render(w *gpui.Window, cx *gpui.App) gpui.IntoElement {
	list := gpui.NewDiv().ID("list").Flex1().OverflowScroll().FlexCol().Gap(4)
	for i, title := range t.titles {
		box := gpui.NewDiv().SizeWH(16, 16).Rounded(4).Border(2, gpui.Blue500)
		label := gpui.NewText(title).TextColor(gpui.Gray900)
		if t.done[i] {
			box.Bg(gpui.Blue500)
			label.TextStrikethrough().TextColor(gpui.Gray500)
		}
		list.Child(gpui.NewDiv().ID(fmt.Sprintf("todo-%d", i)).
			FlexRow().ItemsCenter().Gap(8).P(8).Bg(gpui.White).Rounded(6).
			OnClick(func(*gpui.ClickEvent, *gpui.Window, *gpui.App) {}).
			Child(box, label))
	}
	add := gpui.NewDiv().ID("add").P(8).Bg(gpui.Blue500).Rounded(6).
		OnClick(func(*gpui.ClickEvent, *gpui.Window, *gpui.App) {}).
		Child(gpui.NewText("Add").TextColor(gpui.White))
	return gpui.NewDiv().FlexCol().SizeFull().Gap(12).P(16).Bg(gpui.Gray50).Child(
		gpui.NewText("Todos").TextSize(24).FontWeight(700).TextColor(gpui.Gray900),
		gpui.NewDiv().FlexRow().Gap(8).ItemsCenter().Child(
			gpui.NewDiv().Flex1().FlexCol().Child(t.input.Render(w, cx)),
			add,
		),
		list,
	)
}

func benchRun(text string, mutate func(*gpui.TextStyle)) gpui.TextRun {
	ts := gpui.DefaultTextStyle()
	ts.Color = gpui.Gray800
	if mutate != nil {
		mutate(&ts)
	}
	return gpui.TextRun{Text: text, Style: ts}
}

func benchSection(title string, body gpui.IntoElement) *gpui.Div {
	return gpui.NewDiv().FlexCol().Gap(6).Child(
		gpui.NewText(title).TextSize(12).FontWeight(700).TextColor(gpui.Gray500), body)
}

func benchSwatch(c gpui.Rgba, label string) *gpui.Div {
	d := gpui.NewDiv().SizeWH(72, 48).Bg(c).Rounded(8).ItemsCenter().
		Child(gpui.NewText(label).TextSize(12).TextColor(gpui.White))
	d.JustifyCenter()
	return d
}

func benchCard(title, body string, accent gpui.Rgba) *gpui.Div {
	return gpui.NewDiv().Flex1().MinW(0).FlexCol().Gap(4).P(12).Bg(gpui.White).Rounded(10).
		Border(1, gpui.Gray300).Child(
		gpui.NewDiv().FlexRow().Gap(6).ItemsCenter().Child(
			gpui.NewDiv().SizeWH(10, 10).Bg(accent).Rounded(5),
			gpui.NewText(title).FontWeight(700).TextColor(gpui.Gray900)),
		gpui.NewText(body).TextSize(13).TextColor(gpui.Gray700))
}

// benchGallery mirrors examples/gallery's view.
func benchGallery() gpui.EntityRoot {
	return gpui.RenderFunc(func(w *gpui.Window, cx *gpui.App) gpui.IntoElement {
		colors := gpui.NewDiv().FlexRow().Gap(8).Child(
			benchSwatch(gpui.Blue500, "blue"), benchSwatch(gpui.Red500, "red"),
			benchSwatch(gpui.Green600, "green"), benchSwatch(gpui.Yellow400, "yellow"),
			benchSwatch(gpui.Gray800, "gray"))
		corners := gpui.NewDiv().FlexRow().Gap(8).ItemsCenter().Child(
			gpui.NewDiv().SizeWH(48, 48).Bg(gpui.Blue500),
			gpui.NewDiv().SizeWH(48, 48).Bg(gpui.Blue500).Rounded(10),
			gpui.NewDiv().SizeWH(48, 48).Bg(gpui.Blue500).Rounded(24),
			gpui.NewDiv().SizeWH(48, 48).Border(3, gpui.Red500).Rounded(10),
			gpui.NewDiv().SizeWH(48, 48).Bg(gpui.Yellow400).Border(2, gpui.Gray800).Rounded(24))
		between := gpui.NewDiv().FlexRow().ItemsCenter().JustifyBetween().P(8).Bg(gpui.Gray200).Rounded(8).Child(
			gpui.NewDiv().SizeWH(40, 24).Bg(gpui.Blue500).Rounded(4),
			gpui.NewDiv().SizeWH(40, 40).Bg(gpui.Red500).Rounded(4),
			gpui.NewDiv().SizeWH(40, 24).Bg(gpui.Green600).Rounded(4))
		grow := gpui.NewDiv().FlexRow().Gap(8).Child(
			gpui.NewDiv().SizeW(80).P(8).Bg(gpui.Gray300).Rounded(6).Child(gpui.NewText("fixed")),
			gpui.NewDiv().Flex1().P(8).Bg(gpui.Blue500).Rounded(6).Child(gpui.NewText("flex 1").TextColor(gpui.White)),
			gpui.NewDiv().Flex1().P(8).Bg(gpui.Green600).Rounded(6).Child(gpui.NewText("flex 1").TextColor(gpui.White)))
		rich := gpui.NewStyledText(
			benchRun("Styled text mixes ", nil),
			benchRun("bold", func(t *gpui.TextStyle) { t.FontWeight = 700 }),
			benchRun(", ", nil),
			benchRun("colour", func(t *gpui.TextStyle) { t.Color = gpui.Red500 }),
			benchRun(", ", nil),
			benchRun("strikethrough", func(t *gpui.TextStyle) { t.Strikethrough = true; t.Color = gpui.Gray500 }),
			benchRun(" and ", nil),
			benchRun("big blue bold", func(t *gpui.TextStyle) { t.FontSize = 20; t.FontWeight = 700; t.Color = gpui.Blue600 }),
			benchRun(" runs.", nil))
		wrapped := gpui.NewDiv().SizeW(260).P(10).Bg(gpui.White).Rounded(8).Border(1, gpui.Gray300).Child(
			gpui.NewText("Text wraps to the width its container gives it, breaking at word boundaries onto as many lines as it needs.").
				TextSize(13).TextColor(gpui.Gray800))
		cards := gpui.NewDiv().FlexRow().Gap(10).Child(
			benchCard("Layout", "Flexbox rows, columns, gap and grow.", gpui.Blue500),
			benchCard("Style", "Borders, radii, colour and padding.", gpui.Green600),
			benchCard("Text", "Runs, weights and wrapping.", gpui.Red500))
		return gpui.NewDiv().FlexCol().SizeFull().Gap(14).P(20).Bg(gpui.Gray100).OverflowHidden().Child(
			gpui.NewText("gpui gallery").TextSize(26).FontWeight(700).TextColor(gpui.Gray900),
			benchSection("COLOURS", colors),
			benchSection("CORNERS AND BORDERS", corners),
			benchSection("JUSTIFY BETWEEN, ALIGN CENTER", between),
			benchSection("FIXED AND FLEX-GROW", grow),
			benchSection("STYLED TEXT RUNS", rich),
			benchSection("WRAPPING", wrapped),
			benchSection("NESTED CARDS", cards))
	})
}

// openBench opens root in a headless window of w x h logical px at scale.
func openBench(tb testing.TB, w, h int, scale float32, root gpui.EntityRoot) (*gpui.HeadlessWindow, *gpui.App) {
	tb.Helper()
	p := gpui.NewHeadlessPlatform()
	p.SetScaleFactor(scale)
	app := gpui.NewAppWithPlatform(p)
	err := app.OpenWindow(gpui.WindowOptions{Width: float32(w), Height: float32(h)},
		func(*gpui.Window, *gpui.App) gpui.EntityRoot { return root })
	if err != nil {
		tb.Fatal(err)
	}
	return p.Windows()[0], app
}

// benchDraw measures the full pipeline: render, layout, prepaint, paint,
// rasterize at device scale and Present.
func benchDraw(b *testing.B, w, h int, scale float32, root gpui.EntityRoot) {
	hw, app := openBench(b, w, h, scale, root)
	win := hw.Window()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		win.Draw(app)
	}
}

func BenchmarkDrawTodoScale2(b *testing.B)    { benchDraw(b, 480, 640, 2, newBenchTodo()) }
func BenchmarkDrawTodoScale1(b *testing.B)    { benchDraw(b, 480, 640, 1, newBenchTodo()) }
func BenchmarkDrawGalleryScale2(b *testing.B) { benchDraw(b, 520, 760, 2, benchGallery()) }

// BenchmarkIdleTodoScale2 measures fetching the frame when nothing changed:
// no input, no Refresh, so no redraw should happen.
func BenchmarkIdleTodoScale2(b *testing.B) {
	hw, _ := openBench(b, 480, 640, 2, newBenchTodo())
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = hw.Pixels()
	}
}

// BenchmarkHoverMoveTodoScale2 measures a mouse move over the list with no
// hover styling: it must not trigger a redraw.
func BenchmarkHoverMoveTodoScale2(b *testing.B) {
	hw, _ := openBench(b, 480, 640, 2, newBenchTodo())
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		hw.Dispatch(gpui.MouseMoveEvent{Position: gpui.Point{X: gpui.Pixels(100 + i%50), Y: 150}})
	}
}
