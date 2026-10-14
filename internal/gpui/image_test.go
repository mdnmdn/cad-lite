package gpui_test

import (
	"image"
	"image/color"
	"testing"

	"cad-lite/internal/gpui"
)

var _ gpui.PlatformInput = gpui.ImagePasteEvent{}

func TestImagePasteFollowsFocusPath(t *testing.T) {
	var log []string
	var fh *gpui.FocusHandle
	rec := func(name string, stop bool) func(*gpui.ImagePasteEvent, *gpui.Window, *gpui.App) {
		return func(e *gpui.ImagePasteEvent, w *gpui.Window, _ *gpui.App) {
			log = append(log, name+":"+e.MIME)
			if stop {
				w.StopPropagation()
			}
		}
	}
	stopInner := false
	root := gpui.RenderFunc(func(w *gpui.Window, cx *gpui.App) gpui.IntoElement {
		if fh == nil {
			fh = cx.NewFocusHandle()
		}
		return gpui.NewDiv().SizeWH(200, 100).OnImagePaste(rec("root", false)).
			Child(
				gpui.NewDiv().SizeWH(50, 50).OnImagePaste(rec("A", false)),
				gpui.NewDiv().SizeWH(50, 50).OnImagePaste(rec("B", stopInner)).Child(
					gpui.NewDiv().SizeWH(10, 10).TrackFocus(fh)),
			)
	})
	hw, _ := openHeadless(t, 200, 100, root)
	ev := gpui.ImagePasteEvent{Data: []byte{1}, MIME: "image/png"}

	hw.Dispatch(ev)
	checkLog(t, log, "root:image/png")
	log = nil

	fh.Focus(hw.Window())
	hw.Pixels()
	hw.Dispatch(ev)
	checkLog(t, log, "B:image/png", "root:image/png")
	log = nil

	stopInner = true
	hw.Window().Refresh()
	hw.Pixels()
	hw.Dispatch(ev)
	checkLog(t, log, "B:image/png")
}

func TestPaintImageScalesIntoBounds(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 2, 1))
	img.Set(0, 0, color.NRGBA{R: 255, A: 255})
	img.Set(1, 0, color.NRGBA{B: 255, A: 255})
	root := gpui.RenderFunc(func(*gpui.Window, *gpui.App) gpui.IntoElement {
		return gpui.NewDiv().SizeFull().Bg(gpui.RgbaOf(255, 255, 255, 255)).Child(
			sizedCanvas(func(b gpui.Bounds, w *gpui.Window, _ *gpui.App) {
				w.PaintImage(gpui.NewBounds(10, 10, 40, 20), img)
				w.PaintImage(gpui.NewBounds(0, 0, 5, 5), nil)
			}))
	})
	hw, _ := openHeadless(t, 60, 40, root)
	out := hw.Image()
	at := func(x, y int) color.NRGBA { return out.NRGBAAt(x, y) }
	if c := at(12, 20); c.R < 240 || c.B > 15 {
		t.Errorf("left half %v, want red", c)
	}
	if c := at(47, 20); c.B < 240 || c.R > 15 {
		t.Errorf("right half %v, want blue", c)
	}
	if c := at(30, 20); c.R < 100 || c.B < 100 {
		t.Errorf("middle %v, want a red-blue blend", c)
	}
	for _, p := range [][2]int{{9, 20}, {50, 20}, {30, 9}, {30, 30}} {
		if c := at(p[0], p[1]); c != (color.NRGBA{255, 255, 255, 255}) {
			t.Errorf("outside the bounds at %v: %v", p, c)
		}
	}
}

func sizedCanvas(f func(gpui.Bounds, *gpui.Window, *gpui.App)) *gpui.Canvas {
	c := gpui.NewCanvas(f)
	c.SizeFull()
	return c
}
