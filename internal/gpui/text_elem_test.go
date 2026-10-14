package gpui_test

import (
	"testing"

	"go-draw/internal/gpui"
)

type textFakeWindow struct {
	w, h    gpui.Pixels
	scale   float32
	handler gpui.PlatformInputHandler
	frames  int
}

func (f *textFakeWindow) Bounds() (gpui.Pixels, gpui.Pixels) { return f.w, f.h }
func (f *textFakeWindow) ScaleFactor() float32 {
	if f.scale == 0 {
		return 1
	}
	return f.scale
}
func (f *textFakeWindow) RequestFrame()                               { f.frames++ }
func (f *textFakeWindow) Present(pix []byte, w, h int)                {}
func (f *textFakeWindow) SetInputHandler(h gpui.PlatformInputHandler) { f.handler = h }
func (f *textFakeWindow) TakeInputHandler() gpui.PlatformInputHandler {
	h := f.handler
	f.handler = nil
	return h
}
func (f *textFakeWindow) Quit() {}

type textFakePlatform struct{ last *textFakeWindow }

func (p *textFakePlatform) Run(onLaunch func(*gpui.App)) error {
	if onLaunch != nil {
		onLaunch(gpui.NewApp())
	}
	return nil
}
func (p *textFakePlatform) OpenWindow(o gpui.WindowOptions) (gpui.PlatformWindow, error) {
	f := &textFakeWindow{w: gpui.Pixels(o.Width), h: gpui.Pixels(o.Height)}
	p.last = f
	return f, nil
}
func (p *textFakePlatform) Quit()        {}
func (p *textFakePlatform) Name() string { return "textfake" }

func drawTextRoot(t *testing.T, w, h float32, root gpui.EntityRoot, withShaper bool) (*gpui.App, *gpui.Window) {
	t.Helper()
	p := &textFakePlatform{}
	app := gpui.NewAppWithPlatform(p)
	if err := app.OpenWindow(gpui.WindowOptions{Width: w, Height: h, Title: "test"}, func(w *gpui.Window, cx *gpui.App) gpui.EntityRoot {
		if withShaper {
			m := gpui.NewFontManager()
			if path := gpui.DefaultFontPath(); path != "" {
				w.SetTextMeasurer(gpui.NewShaper(m))
			}
		}
		if !withShaper {
			w.SetTextMeasurer(nil)
		}
		return root
	}); err != nil {
		t.Fatalf("OpenWindow: %v", err)
	}
	win := app.Windows()[0]
	win.Draw(app)
	return app, win
}

func requireFontForShaper(t *testing.T) bool {
	t.Helper()
	return gpui.DefaultFontPath() != ""
}

func TestTextElementPaintsGlyphs(t *testing.T) {
	if !requireFontForShaper(t) {
		t.Skip("no system font found on this machine")
	}
	root := gpui.RenderFunc(func(w *gpui.Window, cx *gpui.App) gpui.IntoElement {
		return gpui.NewDiv().SizeWH(400, 300).Child(
			gpui.NewText("Hello, GPUI!").TextSize(24),
		)
	})
	_, win := drawTextRoot(t, 400, 300, root, true)

	sc := win.Scene()
	if len(sc.Sprites) == 0 {
		t.Fatal("text element emitted no glyph sprites")
	}
	for i, s := range sc.Sprites {
		if s.Mask == nil || s.Mask.W <= 0 || s.Mask.H <= 0 {
			t.Errorf("sprite %d has an empty mask", i)
		}
		if s.Bounds.Origin.X < 0 || s.Bounds.Origin.Y < 0 ||
			s.Bounds.Max().X > 400 || s.Bounds.Max().Y > 300 {
			t.Errorf("sprite %d bounds %+v outside the window", i, s.Bounds)
		}
		if s.Color != gpui.Black {
			t.Errorf("sprite %d color = %+v, want Black", i, s.Color)
		}
	}
}

func TestTextStrikethroughPaintsUnderline(t *testing.T) {
	if !requireFontForShaper(t) {
		t.Skip("no system font found on this machine")
	}
	root := gpui.RenderFunc(func(w *gpui.Window, cx *gpui.App) gpui.IntoElement {
		return gpui.NewDiv().SizeWH(400, 300).Child(
			gpui.NewText("struck").TextSize(24).TextStrikethrough(),
		)
	})
	_, win := drawTextRoot(t, 400, 300, root, true)

	sc := win.Scene()
	if len(sc.Underlines) == 0 {
		t.Fatal("strikethrough text emitted no underline")
	}
	u := sc.Underlines[0]
	if u.Color != gpui.Black {
		t.Errorf("underline color = %+v, want Black", u.Color)
	}
	if u.Thickness <= 0 {
		t.Errorf("underline thickness = %v, want > 0", u.Thickness)
	}
}

func TestTextWithoutMeasurerStillLaysOut(t *testing.T) {
	root := gpui.RenderFunc(func(w *gpui.Window, cx *gpui.App) gpui.IntoElement {
		return gpui.NewDiv().SizeWH(400, 300).Child(
			gpui.NewText("crude estimate"),
		)
	})
	_, win := drawTextRoot(t, 400, 300, root, false)

	sc := win.Scene()
	if len(sc.Sprites) != 0 {
		t.Fatalf("got %d sprites without a measurer, want 0", len(sc.Sprites))
	}
}

func TestStrChildRenders(t *testing.T) {
	if !requireFontForShaper(t) {
		t.Skip("no system font found on this machine")
	}
	root := gpui.RenderFunc(func(w *gpui.Window, cx *gpui.App) gpui.IntoElement {
		return gpui.NewDiv().SizeWH(400, 300).Child(gpui.Str("via Str"))
	})
	_, win := drawTextRoot(t, 400, 300, root, true)

	if sc := win.Scene(); len(sc.Sprites) == 0 {
		t.Fatal("Child(Str(...)) emitted no glyph sprites")
	}
}

func TestStyledTextPaintsAllRuns(t *testing.T) {
	if !requireFontForShaper(t) {
		t.Skip("no system font found on this machine")
	}
	root := gpui.RenderFunc(func(w *gpui.Window, cx *gpui.App) gpui.IntoElement {
		return gpui.NewDiv().SizeWH(400, 300).Child(
			gpui.NewStyledText(
				gpui.TextRun{Text: "first ", Style: gpui.TextStyle{FontSize: 20, Color: gpui.Black}},
				gpui.TextRun{Text: "second", Style: gpui.TextStyle{FontSize: 24, Color: gpui.Blue500}},
			),
		)
	})
	_, win := drawTextRoot(t, 400, 300, root, true)

	sc := win.Scene()
	if len(sc.Sprites) < 2 {
		t.Fatalf("got %d sprites, want at least 2 (one per run)", len(sc.Sprites))
	}
	seenBlue := false
	for _, s := range sc.Sprites {
		if s.Color == gpui.Blue500 {
			seenBlue = true
		}
	}
	if !seenBlue {
		t.Error("no sprite painted with the second run's Blue500 color")
	}
}

func TestNonHeadlessPlatformGetsDefaultShaper(t *testing.T) {
	if !requireFontForShaper(t) {
		t.Skip("no system font found on this machine")
	}
	p := &textFakePlatform{}
	app := gpui.NewAppWithPlatform(p)
	root := gpui.RenderFunc(func(w *gpui.Window, cx *gpui.App) gpui.IntoElement {
		return gpui.NewDiv().SizeWH(400, 300).Child(gpui.NewText("Hello").TextSize(24))
	})
	open := func() *gpui.Window {
		if err := app.OpenWindow(gpui.WindowOptions{Width: 400, Height: 300}, func(w *gpui.Window, cx *gpui.App) gpui.EntityRoot {
			if w.TextMeasurer() == nil {
				t.Fatal("window on a non-headless platform has no text measurer")
			}
			return root
		}); err != nil {
			t.Fatalf("OpenWindow: %v", err)
		}
		return app.Windows()[len(app.Windows())-1]
	}
	w1 := open()
	w1.Draw(app)
	if len(w1.Scene().Sprites) == 0 {
		t.Fatal("Text element painted no glyph sprites")
	}
	if w2 := open(); w2.TextMeasurer() != w1.TextMeasurer() {
		t.Error("windows of one App should share the shaper")
	}
}
