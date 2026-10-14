package ui

import (
	"testing"

	"cad-lite/doc"
	"cad-lite/geom"
	"cad-lite/internal/gpui"
)

// benchStatusOverlay measures whole frames of a 1280x800 window at device scale 2 with two lines
// selected, with the properties HUD and the layer popover as asked.
func benchStatusOverlay(b *testing.B, hud, pop bool) {
	b.Cleanup(func() { ApplyTheme(themeDark) })
	p := gpui.NewHeadlessPlatform()
	p.SetScaleFactor(2)
	app := gpui.NewAppWithPlatform(p)
	d := doc.New()
	for _, n := range []string{"Walls", "Doors", "Windows"} {
		d.AddLayer(n, 3)
	}
	ws := NewWorkspace(d, nil)
	if err := app.OpenWindow(gpui.WindowOptions{Width: 1280, Height: 800},
		func(*gpui.Window, *gpui.App) gpui.EntityRoot { return settingsRoot{ws} }); err != nil {
		b.Fatal(err)
	}
	hw := p.Windows()[0]
	ws.Session().Script("LINE 0,0 100,0  LINE 0,50 100,50  ZOOM E")
	ws.session.SelectBox(geom.Vec2{X: -1000, Y: -1000}, geom.Vec2{X: 1000, Y: 1000}, false)
	hw.Pixels()
	win := hw.Window()
	if hud {
		ws.togglePropsHUD()
	}
	if pop {
		ws.openLayerPopover(win, app)
	}
	win.Refresh()
	hw.Pixels()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		win.Draw(app)
	}
}

func BenchmarkFrameHUDOff(b *testing.B)       { benchStatusOverlay(b, false, false) }
func BenchmarkFrameHUDOn(b *testing.B)        { benchStatusOverlay(b, true, false) }
func BenchmarkFrameLayerPopover(b *testing.B) { benchStatusOverlay(b, false, true) }
