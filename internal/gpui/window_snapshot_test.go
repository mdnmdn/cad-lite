package gpui_test

import (
	"bytes"
	"testing"

	"cad-lite/internal/gpui"
)

func TestWindowSnapshotOwnsPresentedPixels(t *testing.T) {
	var w *gpui.Window
	p := gpui.NewHeadlessPlatform()
	var color = gpui.RgbaOf(23, 47, 91, 255)
	if err := p.Run(func(app *gpui.App) {
		if err := app.OpenWindow(gpui.WindowOptions{Width: 64, Height: 48}, func(win *gpui.Window, _ *gpui.App) gpui.EntityRoot {
			w = win
			return snapshotRoot{color: &color}
		}); err != nil {
			t.Fatal(err)
		}
	}); err != nil {
		t.Fatal(err)
	}
	hw := p.Windows()[0]
	hw.Pixels()
	snapshot := w.Snapshot()
	if snapshot == nil || snapshot.Bounds().Dx() != 64 {
		t.Fatal("no snapshot")
	}
	pixels := append([]byte(nil), snapshot.Pix...)
	color = gpui.RgbaOf(200, 100, 50, 255)
	w.Refresh()
	hw.Pixels()
	if !bytes.Equal(pixels, snapshot.Pix) {
		t.Fatal("snapshot changed on redraw")
	}
	if bytes.Equal(snapshot.Pix, w.Snapshot().Pix) {
		t.Fatal("new snapshot did not see redraw")
	}
}

type snapshotRoot struct{ color *gpui.Rgba }

func (r snapshotRoot) Render(_ *gpui.Window, _ *gpui.App) gpui.IntoElement {
	return gpui.NewDiv().SizeFull().Bg(*r.color)
}
