package gpui_test

import (
	"image/color"
	"os"
	"path/filepath"
	"testing"

	"go-gpui/gpui"
)

func tree(onClick func()) gpui.EntityRoot {
	return gpui.RenderFunc(func(w *gpui.Window, cx *gpui.App) gpui.IntoElement {
		box := gpui.NewDiv().SizeWH(50, 50).Bg(gpui.Red500)
		if onClick != nil {
			box = box.OnClick(func(*gpui.ClickEvent, *gpui.Window, *gpui.App) { onClick() })
		}
		return gpui.NewDiv().SizeWH(100, 100).Bg(gpui.Blue500).Child(box)
	})
}

func want(c gpui.Rgba) color.RGBA { return color.RGBA{c.R, c.G, c.B, c.A} }

func TestRenderToImagePixels(t *testing.T) {
	img, err := gpui.RenderToImage(200, 150, 1, tree(nil))
	if err != nil {
		t.Fatal(err)
	}
	if b := img.Bounds(); b.Dx() != 200 || b.Dy() != 150 {
		t.Fatalf("size = %v", b)
	}
	for _, c := range []struct {
		x, y int
		c    gpui.Rgba
	}{{25, 25, gpui.Red500}, {75, 75, gpui.Blue500}, {150, 120, gpui.White}} {
		if got := img.RGBAAt(c.x, c.y); got != want(c.c) {
			t.Errorf("(%d,%d) = %v, want %v", c.x, c.y, got, want(c.c))
		}
	}
}

func TestRenderOptionsAndScale(t *testing.T) {
	bg := gpui.RgbaOf(10, 20, 30, 255)
	img, err := gpui.RenderToImage(150, 150, 2, tree(nil), gpui.WithBackgroundColor(bg), gpui.WithTitle("t"), gpui.WithKeymap(gpui.NewKeymap()))
	if err != nil {
		t.Fatal(err)
	}
	if b := img.Bounds(); b.Dx() != 300 || b.Dy() != 300 {
		t.Fatalf("size = %v, want 300x300", b)
	}
	if got := img.RGBAAt(80, 80); got != want(gpui.Red500) {
		t.Errorf("(80,80) = %v, want red", got)
	}
	if got := img.RGBAAt(150, 150); got != want(gpui.Blue500) {
		t.Errorf("(150,150) = %v, want blue", got)
	}
	if got := img.RGBAAt(290, 290); got != want(bg) {
		t.Errorf("(290,290) = %v, want bg", got)
	}
}

func TestRenderToPNG(t *testing.T) {
	path := filepath.Join(t.TempDir(), "out.png")
	if err := gpui.RenderToPNG(path, 64, 64, 1, tree(nil)); err != nil {
		t.Fatal(err)
	}
	if st, err := os.Stat(path); err != nil || st.Size() == 0 {
		t.Fatalf("png not written: %v", err)
	}
}

func TestHeadlessWindowDispatchClick(t *testing.T) {
	clicks := 0
	p := gpui.NewHeadlessPlatform()
	app := gpui.NewAppWithPlatform(p)
	err := app.OpenWindow(gpui.WindowOptions{Width: 200, Height: 150}, func(*gpui.Window, *gpui.App) gpui.EntityRoot {
		return tree(func() { clicks++ })
	})
	if err != nil {
		t.Fatal(err)
	}
	hw := p.Windows()[0]
	hw.Dispatch(gpui.ClickEvent{Up: &gpui.MouseUpEvent{Position: gpui.Point{X: 10, Y: 10}}})
	if clicks != 1 {
		t.Fatalf("clicks = %d, want 1", clicks)
	}
	hw.Dispatch(gpui.ClickEvent{Up: &gpui.MouseUpEvent{Position: gpui.Point{X: 150, Y: 120}}})
	if clicks != 1 {
		t.Fatalf("click outside fired handler")
	}
	if len(hw.Pixels()) != 4*200*150 {
		t.Fatalf("pixels len = %d", len(hw.Pixels()))
	}
	if err := hw.SavePNG(filepath.Join(t.TempDir(), "w.png")); err != nil {
		t.Fatal(err)
	}
}
