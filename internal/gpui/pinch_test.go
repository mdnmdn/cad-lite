package gpui_test

import (
	"testing"

	"cad-lite/internal/gpui"
)

var _ gpui.PlatformInput = gpui.PinchEvent{}

func TestPinchReachesOnPinchWithPosition(t *testing.T) {
	var got []gpui.PinchEvent
	var outer int
	root := gpui.RenderFunc(func(*gpui.Window, *gpui.App) gpui.IntoElement {
		return gpui.NewDiv().SizeFull().
			OnPinch(func(*gpui.PinchEvent, *gpui.Window, *gpui.App) { outer++ }).
			Child(gpui.NewDiv().ID("inner").SizeWH(100, 100).
				OnPinch(func(e *gpui.PinchEvent, _ *gpui.Window, _ *gpui.App) { got = append(got, *e) }))
	})
	hw, _ := openHeadless(t, 200, 200, root)
	hw.Dispatch(gpui.PinchEvent{Position: gpui.Point{X: 30, Y: 40}, Delta: 0.25, Modifiers: gpui.Shift})
	if len(got) != 1 || got[0].Position != (gpui.Point{X: 30, Y: 40}) || got[0].Delta != 0.25 || got[0].Modifiers != gpui.Shift {
		t.Fatalf("inner got %+v", got)
	}
	if outer != 1 {
		t.Fatalf("outer handler ran %d times, want 1 (bubbling)", outer)
	}
	hw.Dispatch(gpui.PinchEvent{Position: gpui.Point{X: 150, Y: 150}, Delta: -0.1})
	if len(got) != 1 || outer != 2 {
		t.Fatalf("pinch outside inner: inner=%d outer=%d", len(got), outer)
	}
}

func TestPinchStopPropagation(t *testing.T) {
	outer := 0
	root := gpui.RenderFunc(func(*gpui.Window, *gpui.App) gpui.IntoElement {
		return gpui.NewDiv().SizeFull().
			OnPinch(func(*gpui.PinchEvent, *gpui.Window, *gpui.App) { outer++ }).
			Child(gpui.NewDiv().ID("inner").SizeWH(100, 100).
				OnPinch(func(_ *gpui.PinchEvent, w *gpui.Window, _ *gpui.App) { w.StopPropagation() }))
	})
	hw, _ := openHeadless(t, 200, 200, root)
	hw.Dispatch(gpui.PinchEvent{Position: gpui.Point{X: 10, Y: 10}, Delta: 0.1})
	if outer != 0 {
		t.Fatalf("outer ran %d times after StopPropagation", outer)
	}
}

func TestPinchPositionDividedByUIScale(t *testing.T) {
	var got gpui.Point
	root := gpui.RenderFunc(func(*gpui.Window, *gpui.App) gpui.IntoElement {
		return gpui.NewDiv().SizeFull().
			OnPinch(func(e *gpui.PinchEvent, _ *gpui.Window, _ *gpui.App) { got = e.Position })
	})
	hw, _ := openHeadless(t, 200, 200, root)
	hw.Window().SetUIScale(2)
	hw.Dispatch(gpui.PinchEvent{Position: gpui.Point{X: 100, Y: 60}, Delta: 0.1})
	if got != (gpui.Point{X: 50, Y: 30}) {
		t.Fatalf("position = %v, want 50,30", got)
	}
}
