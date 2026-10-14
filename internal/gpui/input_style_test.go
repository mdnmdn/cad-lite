package gpui_test

import (
	"testing"

	"go-draw/internal/gpui"
)

func TestTextInputStyleAppliesBackgroundAndPlaceholder(t *testing.T) {
	t.Setenv(gpui.NoSystemFontsEnv, "1")
	dark := gpui.MustParseHex("#202020")
	pink := gpui.MustParseHex("#ff00ff")
	ti := gpui.NewTextInput("WWWWWWWW").
		Style(func(d *gpui.Div) *gpui.Div { return d.Bg(dark).Border(1, dark) }).
		PlaceholderColor(pink)
	p := gpui.NewHeadlessPlatform()
	app := gpui.NewAppWithPlatform(p)
	if err := app.OpenWindow(gpui.WindowOptions{Width: 400, Height: 300}, func(*gpui.Window, *gpui.App) gpui.EntityRoot { return inputTestRoot(ti) }); err != nil {
		t.Fatal(err)
	}
	img := p.Windows()[0].Image()
	if c := img.NRGBAAt(300, 10); c.R != 0x20 || c.G != 0x20 || c.B != 0x20 {
		t.Fatalf("field background = %v, want #202020", c)
	}
	magenta := 0
	for y := 0; y < 40; y++ {
		for x := 0; x < 200; x++ {
			if c := img.NRGBAAt(x, y); c.R > 200 && c.G < 60 && c.B > 200 {
				magenta++
			}
		}
	}
	if magenta == 0 {
		t.Fatal("placeholder not drawn in the PlaceholderColor")
	}
}

func TestTextInputSelectionColor(t *testing.T) {
	t.Setenv(gpui.NoSystemFontsEnv, "1")
	green := gpui.MustParseHex("#00ff00")
	ti := gpui.NewTextInput("").SelectionColor(green)
	ti.SetText("WWWW")
	p := gpui.NewHeadlessPlatform()
	app := gpui.NewAppWithPlatform(p)
	if err := app.OpenWindow(gpui.WindowOptions{Width: 400, Height: 300}, func(*gpui.Window, *gpui.App) gpui.EntityRoot { return inputTestRoot(ti) }); err != nil {
		t.Fatal(err)
	}
	hw := p.Windows()[0]
	field := inputSelectionField(t, hw)
	inputSelectionPress(hw, inputSelectionPoint(ti, field, 0), 1, 0)
	ti.SetSelectedTextRange(gpui.IntRange{Start: 0, End: 4})
	img := hw.Image()
	found := false
	for y := 0; y < 40 && !found; y++ {
		for x := 0; x < 200; x++ {
			if c := img.NRGBAAt(x, y); c.G > 200 && c.R < 80 && c.B < 80 {
				found = true
				break
			}
		}
	}
	if !found {
		t.Fatal("selection not drawn in SelectionColor")
	}
}
