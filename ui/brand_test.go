package ui_test

import (
	"testing"

	"cad-lite/doc"
	"cad-lite/internal/gpui"
	"cad-lite/ui"
)

// brandFrame renders a workspace in the given brand at scale and returns the NRGBA frame and the
// hit-test rig of its window.
func brandFrame(t *testing.T, b ui.Brand, scale float32) (*tbRig, func(x, y int) [4]uint8) {
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
	return &tbRig{t: t, ws: ws, hw: hw}, func(x, y int) [4]uint8 {
		c := img.NRGBAAt(x, y)
		return [4]uint8{c.R, c.G, c.B, c.A}
	}
}

// brandInk counts pixels in the title bar's logo box (the 32 px square after its 16 px padding)
// that differ from the title bar background at its corner, so the icon's presence can be told.
func brandInk(at func(x, y int) [4]uint8, scale int) int {
	bg := at(2, 2)
	n := 0
	for y := 10 * scale; y < 42*scale; y++ {
		for x := 16 * scale; x < 48*scale; x++ {
			if at(x, y) != bg {
				n++
			}
		}
	}
	return n
}

// chipX is where the title bar's document chip starts: what follows the brand moves with it.
func chipX(t *testing.T, r *tbRig) float32 {
	t.Helper()
	b, ok := r.bounds("doc-chip")
	if !ok {
		t.Fatal("document chip not found")
	}
	return b.x0
}

func TestBrandModes(t *testing.T) {
	for _, scale := range []int{1, 2} {
		ri, icon := brandFrame(t, ui.BrandIcon, float32(scale))
		rn, _ := brandFrame(t, ui.BrandNone, float32(scale))
		rt, text := brandFrame(t, ui.BrandText, float32(scale))
		// The icon plate fills most of its 32px square, in both modes that show it.
		for name, at := range map[string]func(x, y int) [4]uint8{"icon": icon, "text": text} {
			if n := brandInk(at, scale); n < 500*scale*scale {
				t.Errorf("scale %d: %s mode: icon ink %d, want a filled plate", scale, name, n)
			}
			if at(32*scale, 26*scale) == at(2, 2) {
				t.Errorf("scale %d: %s mode: icon centre equals the title bar background", scale, name)
			}
		}
		// The wordmark follows the icon in text mode only, which pushes the chip right.
		xn, xi, xt := chipX(t, rn), chipX(t, ri), chipX(t, rt)
		if xt < xi+40 {
			t.Errorf("scale %d: text mode chip at %v, icon mode at %v: the wordmark is missing", scale, xt, xi)
		}
		if xi <= xn+32 || xn > 20 {
			t.Errorf("scale %d: chip at %v (none) and %v (icon): want the first at the left padding, the second after the 32 px icon", scale, xn, xi)
		}
	}
}

func TestBrandNoneLeavesNoGap(t *testing.T) {
	r, _ := brandFrame(t, ui.BrandNone, 1)
	if x := chipX(t, r); x > 18 {
		t.Errorf("no-brand chip starts at x=%v, want the title bar's 16 px padding", x)
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
