package gpui_test

import (
	"testing"
	"time"

	"go-gpui/gpui"
)

func TestHoverStartEnd(t *testing.T) {
	var log []string
	p := gpui.NewHeadlessPlatform()
	app := gpui.NewAppWithPlatform(p)
	err := app.OpenWindow(gpui.WindowOptions{Width: 200, Height: 150}, func(*gpui.Window, *gpui.App) gpui.EntityRoot {
		return gpui.RenderFunc(func(*gpui.Window, *gpui.App) gpui.IntoElement {
			inner := gpui.NewDiv().SizeWH(40, 40).Bg(gpui.Red500).
				OnHoverStart(func(*gpui.Window, *gpui.App) { log = append(log, "in+") }).
				OnHoverEnd(func(*gpui.Window, *gpui.App) { log = append(log, "in-") })
			return gpui.NewDiv().SizeWH(100, 100).Bg(gpui.Blue500).
				OnHoverStart(func(*gpui.Window, *gpui.App) { log = append(log, "out+") }).
				OnHoverEnd(func(*gpui.Window, *gpui.App) { log = append(log, "out-") }).
				Child(inner)
		})
	})
	if err != nil {
		t.Fatal(err)
	}
	hw := p.Windows()[0]
	move := func(x, y gpui.Pixels) { hw.Dispatch(gpui.MouseMoveEvent{Position: gpui.Point{X: x, Y: y}}) }
	check := func(want ...string) {
		t.Helper()
		if len(log) != len(want) {
			t.Fatalf("log = %v, want %v", log, want)
		}
		for i := range want {
			if log[i] != want[i] {
				t.Fatalf("log = %v, want %v", log, want)
			}
		}
		log = nil
	}
	move(150, 120)
	check()
	move(80, 80)
	check("out+")
	move(81, 81)
	check()
	move(10, 10)
	check("in+")
	move(80, 80)
	check("in-")
	move(150, 120)
	check("out-")
}

func TestQuitFromOtherGoroutine(t *testing.T) {
	app := gpui.NewApp()
	done := make(chan struct{})
	time.AfterFunc(time.Millisecond, func() { app.Quit(); close(done) })
	for !app.IsQuitting() {
		time.Sleep(time.Millisecond)
	}
	<-done
}
