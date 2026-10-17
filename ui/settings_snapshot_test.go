package ui

import (
	"bytes"
	"testing"

	"cad-lite/doc"
	"cad-lite/internal/gpui"
)

func TestSettingsFreezesAndReleasesWorkspaceBackground(t *testing.T) {
	h := openSettingsApp(t)
	h.openPanel()
	p := h.ws.prefs()
	if p.snapshot == nil {
		t.Fatal("settings did not capture workspace")
	}
	snapshot := p.snapshot
	original := append([]byte(nil), snapshot.Pix...)
	h.ws.setSettingsTab(h.hw.Window(), settingsAI)
	h.hw.Pixels()
	for i := 0; i < 3; i++ {
		h.hw.Dispatch(gpui.ScrollWheelEvent{Position: gpui.Point{X: 450, Y: 350}, DeltaY: 70})
	}
	if p.snapshot != snapshot || !bytes.Equal(original, snapshot.Pix) {
		t.Fatal("scrolling changed fixed background")
	}
	// The workspace is one image while modal content remains interactive.
	if len(h.hw.Window().Scene().Strokes) != 0 {
		t.Fatal("workspace geometry still rendered under settings")
	}
	h.ws.CloseSettings(h.hw.Window())
	h.hw.Pixels()
	if p.snapshot != nil {
		t.Fatal("settings retained background after closing")
	}
	h.openPanel()
	if p.snapshot == snapshot || p.snapshot == nil {
		t.Fatal("reopening reused stale snapshot")
	}
}

func BenchmarkSettingsScrollFrozen(b *testing.B) {
	hw, app, ws := openSettingsBench(b, true, settingsAI)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		off := ws.prefs().bodyScroll.Offset()
		off.Y = gpui.Pixels(i % 100)
		ws.prefs().bodyScroll.SetOffset(off)
		hw.Window().Draw(app)
	}
}

func TestSettingsSnapshotResizeAndAppearance(t *testing.T) {
	h := openSettingsWS(t, NewWorkspace(doc.New(), nil), 900, 700, 1)
	h.openPanel()
	snapshot := h.ws.prefs().snapshot
	c := h.ws.Settings()
	c.Theme = "light"
	c.UIScale = 1.25
	h.ws.SetSettings(c, h.hw.Window())
	h.hw.Pixels()
	if h.ws.prefs().snapshot != snapshot || !h.has("settings-close") {
		t.Fatal("appearance change broke frozen dialog")
	}
}
