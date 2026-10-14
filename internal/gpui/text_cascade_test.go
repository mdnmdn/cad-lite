package gpui_test

import (
	"testing"

	"cad-lite/internal/gpui"
)

func spritesOf(t *testing.T, root gpui.EntityRoot) []gpui.GlyphSprite {
	t.Helper()
	_, win := drawTextRoot(t, 400, 300, root, true)
	return append([]gpui.GlyphSprite(nil), win.Scene().Sprites...)
}

func sameSprites(t *testing.T, got, want []gpui.GlyphSprite) {
	t.Helper()
	if len(got) == 0 || len(got) != len(want) {
		t.Fatalf("got %d sprites, want %d (non-zero)", len(got), len(want))
	}
	for i := range got {
		if got[i].Color != want[i].Color || got[i].Bounds != want[i].Bounds {
			t.Fatalf("sprite %d: got color %v bounds %v, want color %v bounds %v",
				i, got[i].Color, got[i].Bounds, want[i].Color, want[i].Bounds)
		}
	}
}

func TestTextInheritsFromDiv(t *testing.T) {
	if !requireFontForShaper(t) {
		t.Skip("no system font found on this machine")
	}
	inherited := spritesOf(t, gpui.RenderFunc(func(w *gpui.Window, cx *gpui.App) gpui.IntoElement {
		return gpui.NewDiv().SizeWH(400, 300).TextColor(gpui.White).TextSize(20).
			Child(gpui.NewDiv().Child(gpui.NewText("xy")))
	}))
	explicit := spritesOf(t, gpui.RenderFunc(func(w *gpui.Window, cx *gpui.App) gpui.IntoElement {
		return gpui.NewDiv().SizeWH(400, 300).
			Child(gpui.NewDiv().Child(gpui.NewText("xy").TextColor(gpui.White).TextSize(20)))
	}))
	sameSprites(t, inherited, explicit)
	if inherited[0].Color != gpui.White {
		t.Errorf("color = %v, want White", inherited[0].Color)
	}
}

func TestTextOwnStyleBeatsInherited(t *testing.T) {
	if !requireFontForShaper(t) {
		t.Skip("no system font found on this machine")
	}
	sp := spritesOf(t, gpui.RenderFunc(func(w *gpui.Window, cx *gpui.App) gpui.IntoElement {
		return gpui.NewDiv().SizeWH(400, 300).TextColor(gpui.White).TextSize(20).
			Child(gpui.NewText("x").TextColor(gpui.Red500))
	}))
	want := spritesOf(t, gpui.RenderFunc(func(w *gpui.Window, cx *gpui.App) gpui.IntoElement {
		return gpui.NewDiv().SizeWH(400, 300).
			Child(gpui.NewText("x").TextColor(gpui.Red500).TextSize(20))
	}))
	sameSprites(t, sp, want)
}

func TestStrAndStyledTextInherit(t *testing.T) {
	if !requireFontForShaper(t) {
		t.Skip("no system font found on this machine")
	}
	sp := spritesOf(t, gpui.RenderFunc(func(w *gpui.Window, cx *gpui.App) gpui.IntoElement {
		return gpui.NewDiv().SizeWH(400, 300).TextColor(gpui.Blue500).TextSize(22).
			Child(gpui.Str("a"), gpui.NewStyledText(gpui.TextRun{Text: "b"}))
	}))
	want := spritesOf(t, gpui.RenderFunc(func(w *gpui.Window, cx *gpui.App) gpui.IntoElement {
		return gpui.NewDiv().SizeWH(400, 300).
			Child(gpui.NewText("a").TextColor(gpui.Blue500).TextSize(22),
				gpui.NewStyledText(gpui.TextRun{Text: "b", Style: gpui.TextStyle{FontSize: 22, FontWeight: 400, Color: gpui.Blue500}}))
	}))
	sameSprites(t, sp, want)
}

func TestTextInputInheritsTextStyle(t *testing.T) {
	if !requireFontForShaper(t) {
		t.Skip("no system font found on this machine")
	}
	in := gpui.NewTextInput("")
	in.SetText("ab")
	sp := spritesOf(t, gpui.RenderFunc(func(w *gpui.Window, cx *gpui.App) gpui.IntoElement {
		return gpui.NewDiv().SizeWH(400, 300).TextColor(gpui.Red500).TextSize(24).Child(in.Render(w, cx))
	}))
	found := false
	for _, s := range sp {
		if s.Color == gpui.Red500 {
			found = true
		}
	}
	if !found {
		t.Error("TextInput text did not inherit the ancestor's text color")
	}
}

func TestTextLineHeightAffectsLayout(t *testing.T) {
	if !requireFontForShaper(t) {
		t.Skip("no system font found on this machine")
	}
	_, win := drawTextRoot(t, 400, 300, gpui.RenderFunc(func(w *gpui.Window, cx *gpui.App) gpui.IntoElement {
		return gpui.NewDiv().SizeWH(400, 300)
	}), true)
	m := win.TextMeasurer()
	_, h1 := m.MeasureWrapped("a\nb", gpui.TextStyle{FontSize: 14}, 1e9)
	_, h2 := m.MeasureWrapped("a\nb", gpui.TextStyle{FontSize: 14, LineHeight: 30}, 1e9)
	if h2 != 60 || h1 >= h2 {
		t.Errorf("heights default=%v custom=%v, want custom 60", h1, h2)
	}
}
