package gpui_test

import (
	"testing"

	"go-draw/internal/gpui"
)

func TestScrollWheelStopPropagationPreventsScrolling(t *testing.T) {
	h := gpui.NewScrollHandle()
	stop := true
	root := gpui.RenderFunc(func(*gpui.Window, *gpui.App) gpui.IntoElement {
		return gpui.NewDiv().SizeFull().Child(
			gpui.NewDiv().ID("list").SizeWH(100, 100).OverflowYScroll().TrackScroll(h).Child(
				gpui.NewDiv().ID("tall").SizeWH(100, 400).
					OnScrollWheel(func(_ *gpui.ScrollWheelEvent, w *gpui.Window, _ *gpui.App) {
						if stop {
							w.StopPropagation()
						}
					}),
			),
		)
	})
	hw, _ := openHeadless(t, 200, 200, root)
	hw.Dispatch(wheel(50, 50, 0, 40))
	if o := h.Offset(); o.Y != 0 {
		t.Fatalf("offset = %v, want 0: the handler stopped propagation", o)
	}
	stop = false
	hw.Dispatch(wheel(50, 50, 0, 40))
	if o := h.Offset(); o.Y != 40 {
		t.Fatalf("offset = %v, want 40", o)
	}
}
