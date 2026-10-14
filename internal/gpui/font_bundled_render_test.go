package gpui_test

import (
	"testing"

	"go-draw/internal/gpui"
)

func TestBundledFontRendersGlyphs(t *testing.T) {
	t.Setenv(gpui.NoSystemFontsEnv, "1")
	p := gpui.NewHeadlessPlatform()
	app := gpui.NewAppWithPlatform(p)
	err := app.OpenWindow(gpui.WindowOptions{Width: 200, Height: 60}, func(*gpui.Window, *gpui.App) gpui.EntityRoot {
		return inputTestRoot(func() *gpui.TextInput { ti := gpui.NewTextInput(""); ti.SetText("Hello"); return ti }())
	})
	if err != nil {
		t.Fatal(err)
	}
	hw := p.Windows()[0]
	m := hw.Window().TextMeasurer()
	if m == nil {
		t.Fatal("no text measurer without system fonts")
	}
	lines := m.ShapeWrapped("Hello", gpui.DefaultTextStyle(), 1000)
	if len(lines) == 0 || len(lines[0].Glyphs) != 5 {
		t.Fatalf("shaped lines = %v", lines)
	}
	img := hw.Image()
	dark := 0
	for y := 0; y < img.Bounds().Dy(); y++ {
		for x := 0; x < img.Bounds().Dx(); x++ {
			if c := img.NRGBAAt(x, y); c.R < 128 && c.G < 128 && c.B < 128 {
				dark++
			}
		}
	}
	if dark < 20 {
		t.Fatalf("only %d dark pixels: text did not render", dark)
	}
}
