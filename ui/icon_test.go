package ui_test

import (
	"testing"

	"go-draw/assets/icons"
	"go-draw/internal/gpui"
	"go-draw/ui"
)

func TestIconMaskCoversInk(t *testing.T) {
	for _, n := range icons.All {
		m := ui.IconMask(n, 32)
		if m == nil || m.W != 32 || m.H != 32 || len(m.Pix) != 32*32 {
			t.Fatalf("%s: bad mask %+v", n, m)
		}
		ink := 0
		for _, v := range m.Pix {
			if v > 0 {
				ink++
			}
		}
		if ink < 40 {
			t.Errorf("%s: only %d inked pixels", n, ink)
		}
	}
}

func TestIconMaskCached(t *testing.T) {
	a, b := ui.IconMask(icons.DrawLine, 20), ui.IconMask(icons.DrawLine, 20)
	if a == nil || a != b {
		t.Fatal("same name and size must return the cached mask")
	}
	if c := ui.IconMask(icons.DrawLine, 40); c == a || c.W != 40 {
		t.Fatal("a different size must rasterise a new mask")
	}
	if ui.IconMask("nope", 16) != nil {
		t.Fatal("unknown icon must yield nil")
	}
}

type iconRoot struct{ color gpui.Rgba }

func (r iconRoot) Render(*gpui.Window, *gpui.App) gpui.IntoElement {
	return gpui.NewDiv().SizeFull().P(10).Child(ui.NewIcon(icons.DrawCircle, 16, r.color))
}

func TestIconPaintsTintedSprite(t *testing.T) {
	tint := gpui.RgbaOf(12, 200, 90, 255)
	for _, scale := range []float32{1, 2} {
		p := gpui.NewHeadlessPlatform()
		p.SetScaleFactor(scale)
		app := gpui.NewAppWithPlatform(p)
		if err := app.OpenWindow(gpui.WindowOptions{Width: 100, Height: 60}, func(*gpui.Window, *gpui.App) gpui.EntityRoot { return iconRoot{tint} }); err != nil {
			t.Fatal(err)
		}
		hw := p.Windows()[0]
		hw.Pixels()
		sprites := hw.Window().Scene().Sprites
		if len(sprites) != 1 {
			t.Fatalf("scale %v: %d sprites, want 1", scale, len(sprites))
		}
		s := sprites[0]
		if s.Color != tint {
			t.Errorf("scale %v: colour %v, want %v", scale, s.Color, tint)
		}
		if s.Mask.W != int(16*scale) {
			t.Errorf("scale %v: mask %d px, want %d", scale, s.Mask.W, int(16*scale))
		}
		if s.Bounds.Origin.X != 10 || s.Bounds.Origin.Y != 10 || s.Bounds.Size.W != 16 {
			t.Errorf("scale %v: bounds %v, want 16px square at 10,10", scale, s.Bounds)
		}
	}
}
