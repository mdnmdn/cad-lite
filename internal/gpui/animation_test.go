package gpui_test

import (
	"cad-lite/internal/gpui"
	"testing"
	"time"
)

func TestScheduledTickDrawBudgetRegression(t *testing.T) {
	p := gpui.NewHeadlessPlatform()
	app := gpui.NewAppWithPlatform(p)
	draws := 0
	if err := app.OpenWindow(gpui.WindowOptions{Width: 30, Height: 30}, func(*gpui.Window, *gpui.App) gpui.EntityRoot {
		return gpui.RenderFunc(func(*gpui.Window, *gpui.App) gpui.IntoElement { draws++; return gpui.NewDiv() })
	}); err != nil {
		t.Fatal(err)
	}
	w := app.Windows()[0]
	w.After(0, func(w *gpui.Window, a *gpui.App) { w.Draw(a); w.Draw(a) })
	w.After(0, func(w *gpui.Window, a *gpui.App) { w.Draw(a) })
	before := draws
	if err := p.Advance(0); err != nil {
		t.Fatal(err)
	}
	if draws != before+1 {
		t.Fatalf("renders per Tick=%d want1", draws-before)
	}
}

func TestRequestNextFrameAfterIdleNeedsLaterTimestampRegression(t *testing.T) {
	p := gpui.NewHeadlessPlatform()
	a := gpui.NewAppWithPlatform(p)
	draws := 0
	if err := a.OpenWindow(gpui.WindowOptions{Width: 30, Height: 30}, func(*gpui.Window, *gpui.App) gpui.EntityRoot {
		return gpui.RenderFunc(func(*gpui.Window, *gpui.App) gpui.IntoElement { draws++; return gpui.NewDiv() })
	}); err != nil {
		t.Fatal(err)
	}
	if err := p.Advance(time.Second); err != nil {
		t.Fatal(err)
	}
	w := a.Windows()[0]
	w.RequestNextFrame()
	before := draws
	a.Tick()
	if draws != before {
		t.Fatalf("same-time nextframe rendered after idle: %d", draws-before)
	}
	if err := p.Advance(time.Nanosecond); err != nil {
		t.Fatal(err)
	}
	if draws != before+1 {
		t.Fatalf("later timestamp renders=%d want1", draws-before)
	}
}

func TestCloseDuringRenderDoesNotPresentRegression(t *testing.T) {
	p := gpui.NewHeadlessPlatform()
	a := gpui.NewAppWithPlatform(p)
	if err := a.OpenWindow(gpui.WindowOptions{Width: 30, Height: 30}, func(*gpui.Window, *gpui.App) gpui.EntityRoot {
		return gpui.RenderFunc(func(w *gpui.Window, _ *gpui.App) gpui.IntoElement {
			w.Close()
			return gpui.NewDiv().SizeFull().Bg(gpui.Red500)
		})
	}); err != nil {
		t.Fatal(err)
	}
	if pixels := p.Windows()[0].Pixels(); len(pixels) != 0 {
		t.Fatalf("closed window presented %d bytes", len(pixels))
	}
}
