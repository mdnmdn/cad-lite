package ui

import (
	"testing"

	"cad-lite/doc"
	"cad-lite/geom"
	"cad-lite/internal/gpui"
)

// BenchmarkCrosshairMove1080p measures one pointer move over the canvas of a 1920x1080 window at
// device scale 1: the crosshair follows the pointer, so each frame redraws only the tiles it crosses
// (plus the coordinate readout) once damage tracking is in place.
func BenchmarkCrosshairMove1080p(b *testing.B) {
	h := openSettingsWS(&testing.T{}, NewWorkspace(doc.New(), nil), 1920, 1080, 1)
	h.ws.Session().Script("RECTANG 0,0 220,130 CIRCLE 110,50 20 LINE 0,0 220,130 ")
	h.ws.FitOnFirstFrame()
	h.hw.Pixels()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		p := h.ws.ScreenPoint(geom.V(float64(50+i%100), 60))
		h.hw.Dispatch(gpui.MouseMoveEvent{Position: p})
		h.hw.Pixels()
	}
}
