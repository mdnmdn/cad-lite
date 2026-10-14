package gpui_test

import (
	"bytes"
	"testing"

	"cad-lite/internal/gpui"
)

func uiScaleRoot(onClick func()) gpui.EntityRoot {
	return gpui.RenderFunc(func(w *gpui.Window, cx *gpui.App) gpui.IntoElement {
		box := gpui.NewDiv().ID("box").SizeWH(100, 40).Bg(gpui.Red500).Child(gpui.NewText("Hello, UI scale").TextColor(gpui.White))
		if onClick != nil {
			box.OnClick(func(*gpui.ClickEvent, *gpui.Window, *gpui.App) { onClick() })
		}
		return gpui.NewDiv().SizeFull().Bg(gpui.Blue500).Child(box)
	})
}

func openScaled(t *testing.T, w, h, device, ui float32, root gpui.EntityRoot) (*gpui.HeadlessWindow, *gpui.Window) {
	t.Helper()
	p := gpui.NewHeadlessPlatform()
	p.SetScaleFactor(device)
	app := gpui.NewAppWithPlatform(p)
	if err := app.OpenWindow(gpui.WindowOptions{Width: w, Height: h}, func(*gpui.Window, *gpui.App) gpui.EntityRoot { return root }); err != nil {
		t.Fatal(err)
	}
	hw := p.Windows()[0]
	hw.Window().SetUIScale(ui)
	return hw, hw.Window()
}

func TestUIScaleDefaultIsOne(t *testing.T) {
	_, w := openScaled(t, 300, 200, 2, 1, uiScaleRoot(nil))
	if w.UIScale() != 1 || w.ScaleFactor() != 2 {
		t.Fatalf("UIScale %v ScaleFactor %v", w.UIScale(), w.ScaleFactor())
	}
}

func TestUIScaleShrinksLogicalSize(t *testing.T) {
	hw, w := openScaled(t, 300, 150, 1, 1.5, uiScaleRoot(nil))
	hw.Pixels()
	if b := w.Bounds(); b.Size.W != 200 || b.Size.H != 100 {
		t.Fatalf("logical bounds = %v, want 200x100", b.Size)
	}
	if w.ScaleFactor() != 1.5 {
		t.Fatalf("effective scale = %v", w.ScaleFactor())
	}
	if img := hw.Image(); img.Bounds().Dx() != 300 || img.Bounds().Dy() != 150 {
		t.Fatalf("frame is %v, want 300x150 device px", img.Bounds())
	}
}

// A window zoomed by f must rasterize exactly like a window whose platform
// reports device scale f: sharp text at the effective scale.
func TestUIScaleMatchesDeviceScaleRender(t *testing.T) {
	zoomed, _ := openScaled(t, 300, 150, 1, 1.5, uiScaleRoot(nil))
	native, _ := openScaled(t, 200, 100, 1.5, 1, uiScaleRoot(nil))
	a, b := zoomed.Image(), native.Image()
	if a.Bounds() != b.Bounds() || !bytes.Equal(a.Pix, b.Pix) {
		t.Fatal("UIScale 1.5 at device 1 differs from device scale 1.5")
	}
}

func TestUIScaleClicksHitScaledElements(t *testing.T) {
	clicks := 0
	hw, _ := openScaled(t, 300, 150, 1, 1.5, uiScaleRoot(func() { clicks++ }))
	hw.Pixels()
	click := func(x, y gpui.Pixels) {
		p := gpui.Point{X: x, Y: y}
		hw.Dispatch(gpui.MouseMoveEvent{Position: p})
		hw.Dispatch(gpui.MouseDownEvent{Position: p, Button: gpui.MouseLeft, ClickCount: 1})
		up := gpui.MouseUpEvent{Position: p, Button: gpui.MouseLeft, ClickCount: 1}
		hw.Dispatch(up)
		hw.Dispatch(gpui.ClickEvent{Up: &up})
	}
	click(140, 50) // box spans 150x60 device px
	if clicks != 1 {
		t.Fatalf("click inside the scaled box: clicks = %d", clicks)
	}
	click(160, 50) // right of the 150 px wide box
	click(120, 70) // below the 60 px tall box
	if clicks != 1 {
		t.Fatalf("clicks outside the box registered: %d", clicks)
	}
}

func TestSetUIScaleClampsAndInvalidates(t *testing.T) {
	hw, w := openScaled(t, 300, 150, 1, 1, uiScaleRoot(nil))
	hw.Pixels()
	w.SetUIScale(100)
	if w.UIScale() != 4 {
		t.Fatalf("UIScale = %v, want clamp to 4", w.UIScale())
	}
	w.SetUIScale(0.01)
	if w.UIScale() != 0.25 {
		t.Fatalf("UIScale = %v, want clamp to 0.25", w.UIScale())
	}
	w.SetUIScale(float32(nan()))
	if w.UIScale() != 0.25 {
		t.Fatalf("NaN changed UIScale to %v", w.UIScale())
	}
}

func nan() float64 { var z float64; return z / z }
