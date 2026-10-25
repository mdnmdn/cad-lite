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
	ws.lay.propsOff = hud
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

// BenchmarkFramePropsPanel measures whole frames with the side panel's editable Properties grid
// showing a dimension and a line (the HUD's place): the cached sheet is not rebuilt per frame.
func BenchmarkFramePropsPanel(b *testing.B) {
	b.Cleanup(func() { ApplyTheme(themeDark) })
	p := gpui.NewHeadlessPlatform()
	p.SetScaleFactor(2)
	app := gpui.NewAppWithPlatform(p)
	ws := NewWorkspace(doc.New(), nil)
	if err := app.OpenWindow(gpui.WindowOptions{Width: 1280, Height: 800},
		func(*gpui.Window, *gpui.App) gpui.EntityRoot { return settingsRoot{ws} }); err != nil {
		b.Fatal(err)
	}
	hw := p.Windows()[0]
	ws.Session().Script("LINE 0,0 100,0  DIMLINEAR 0,0 100,0 50,-20 ZOOM E")
	ws.session.SelectBox(geom.Vec2{X: -1000, Y: -1000}, geom.Vec2{X: 1000, Y: 1000}, false)
	win := hw.Window()
	win.Refresh()
	hw.Pixels()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		win.Refresh()
		win.Draw(app)
	}
}

// BenchmarkPropsSheetCached is the panel's per-frame sheet lookup on a 10,000-line selection.
func BenchmarkPropsSheetCached(b *testing.B) {
	ws := NewWorkspace(doc.New(), nil)
	d := ws.session.Drawing()
	tx, _ := d.Begin()
	for i := range 10000 {
		tx.Add(&doc.Line{EntityCommon: doc.EntityCommon{Layer: "0"}, End: geom.V(float64(i), 1)})
	}
	tx.Commit()
	ws.session.Selection().Add(d.Handles()...)
	ws.propsSheet()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ws.propsSheet()
	}
}

// BenchmarkPropsSheetInFrame is BenchmarkPropsSheetCached's lookup made again within the frame
// being built (the menus, the tab order): the selection is compared once per frame, not per call.
func BenchmarkPropsSheetInFrame(b *testing.B) {
	ws := NewWorkspace(doc.New(), nil)
	d := ws.session.Drawing()
	tx, _ := d.Begin()
	for i := range 10000 {
		tx.Add(&doc.Line{EntityCommon: doc.EntityCommon{Layer: "0"}, End: geom.V(float64(i), 1)})
	}
	tx.Commit()
	ws.session.Selection().Add(d.Handles()...)
	ws.frameN, ws.building = 1, true
	ws.propsSheet()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ws.propsSheet()
	}
}
