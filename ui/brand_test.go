package ui_test

import (
	"testing"

	"go-draw/doc"
	"go-draw/internal/gpui"
	"go-draw/ui"
)

// brandFrame renders a workspace in the given brand at scale and returns the NRGBA frame.
func brandFrame(t *testing.T, b ui.Brand, scale float32) (*ui.Workspace, func(x, y int) [4]uint8) {
	t.Helper()
	ws := ui.NewWorkspace(doc.New(), nil)
	ws.Brand = b
	p := gpui.NewHeadlessPlatform()
	p.SetScaleFactor(scale)
	var hw *gpui.HeadlessWindow
	if err := p.Run(func(cx *gpui.App) {
		if err := cx.OpenWindow(gpui.WindowOptions{Width: 900, Height: 560}, func(*gpui.Window, *gpui.App) gpui.EntityRoot { return ws }); err != nil {
			t.Fatal(err)
		}
		hw = p.Windows()[0]
	}); err != nil {
		t.Fatal(err)
	}
	hw.Pixels()
	img := hw.Image()
	return ws, func(x, y int) [4]uint8 {
		c := img.NRGBAAt(x, y)
		return [4]uint8{c.R, c.G, c.B, c.A}
	}
}

// brandInk counts pixels in the toolbar's left 48x40 logical box that differ from the
// panel background at its corner, so absence and presence of a brand can be told apart.
func brandInk(at func(x, y int) [4]uint8, scale int) int {
	bg := at(2, 2)
	n := 0
	for y := 4 * scale; y < 44*scale; y++ {
		for x := 8 * scale; x < 44*scale; x++ {
			if at(x, y) != bg {
				n++
			}
		}
	}
	return n
}

func TestBrandModes(t *testing.T) {
	for _, scale := range []int{1, 2} {
		_, icon := brandFrame(t, ui.BrandIcon, float32(scale))
		_, none := brandFrame(t, ui.BrandNone, float32(scale))
		_, text := brandFrame(t, ui.BrandText, float32(scale))
		// The icon plate fills most of its 32px square; text covers far less.
		if n := brandInk(icon, scale); n < 500*scale*scale {
			t.Errorf("scale %d: icon ink %d, want a filled plate", scale, n)
		}
		if icon(24*scale, 24*scale) == icon(2, 2) {
			t.Errorf("scale %d: icon centre equals the panel background", scale)
		}
		if text(24*scale, 24*scale) == text(2, 2) && brandInk(text, scale) == 0 {
			t.Errorf("scale %d: text mode drew nothing", scale)
		}
		if n := brandInk(icon, scale); n <= brandInk(text, scale) {
			t.Errorf("scale %d: icon ink %d not above text ink %d", scale, n, brandInk(text, scale))
		}
		// Without a brand the first toolbar button starts at the row's left padding.
		if n := brandInk(none, scale); n == 0 {
			t.Errorf("scale %d: first button missing", scale)
		}
	}
}

func TestBrandNoneLeavesNoGap(t *testing.T) {
	_, none := brandFrame(t, ui.BrandNone, 1)
	// Find the x of the first non-background pixel on the icon row (y 14, above the label) at the
	// toolbar's left: with no brand it is within the 8px padding plus one button's 6px padding.
	first := func(at func(x, y int) [4]uint8) int {
		bg := at(2, 14)
		for x := 0; x < 400; x++ {
			for y := 6; y < 30; y++ {
				if at(x, y) != bg {
					return x
				}
			}
		}
		return -1
	}
	if f := first(none); f < 0 || f > 24 {
		t.Errorf("no-brand first ink at x=%d, want the first button at the left edge", f)
	}
}

func TestBrandForOS(t *testing.T) {
	for goos, want := range map[string]ui.Brand{"darwin": ui.BrandIcon, "windows": ui.BrandNone, "linux": ui.BrandText, "js": ui.BrandText} {
		if got := ui.BrandForOS(goos); got != want {
			t.Errorf("BrandForOS(%q) = %v, want %v", goos, got, want)
		}
	}
	if b, ok := ui.ParseBrand("mac"); !ok || b != ui.BrandIcon {
		t.Error("ParseBrand mac")
	}
	if _, ok := ui.ParseBrand("x"); ok {
		t.Error("ParseBrand accepted junk")
	}
}
