package ui

import (
	"testing"

	"cad-lite/doc"
	"cad-lite/internal/gpui"
)

// openSettingsBench opens a workspace in a 1280x800 headless window at device
// scale 2 (a Retina laptop), with the settings panel open on tab when open is
// set, for the frame benchmarks below.
func openSettingsBench(b *testing.B, open bool, tab settingsTab) (*gpui.HeadlessWindow, *gpui.App, *Workspace) {
	b.Helper()
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
	hw.Pixels()
	if open {
		ws.openSettingsAt(hw.Window(), tab)
		hw.Pixels()
	}
	return hw, app, ws
}

// benchSettingsDraw measures whole frames: build, layout, paint and rasterize.
func benchSettingsDraw(b *testing.B, open bool, tab settingsTab) {
	hw, app, _ := openSettingsBench(b, open, tab)
	win := hw.Window()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		win.Draw(app)
	}
}

// benchSettingsBuild measures building the panel's element tree only.
func benchSettingsBuild(b *testing.B, tab settingsTab) {
	_, app, ws := openSettingsBench(b, true, tab)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ws.settingsOverlay(app)
	}
}

func BenchmarkSettingsFrameClosed(b *testing.B)  { benchSettingsDraw(b, false, settingsGeneral) }
func BenchmarkSettingsFrameOpen(b *testing.B)    { benchSettingsDraw(b, true, settingsGeneral) }
func BenchmarkSettingsFrameAI(b *testing.B)      { benchSettingsDraw(b, true, settingsAI) }
func BenchmarkSettingsFrameUpdates(b *testing.B) { benchSettingsDraw(b, true, settingsUpdates) }
func BenchmarkSettingsBuild(b *testing.B)        { benchSettingsBuild(b, settingsGeneral) }
func BenchmarkSettingsBuildAI(b *testing.B)      { benchSettingsBuild(b, settingsAI) }
func BenchmarkSettingsBuildUpdates(b *testing.B) { benchSettingsBuild(b, settingsUpdates) }
