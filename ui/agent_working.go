package ui

import (
	"time"

	"cad-lite/internal/gpui"
)

// The working animation is a CAD rubber band: from a fixed base point a dashed line follows a
// crosshair that sweeps right and back, as when a LINE waits for its second point. It is a few
// 1px strokes in a 30x12 box next to the status label, drawn only while a turn runs. The window
// already redraws at the pump's period while a turn runs, so the animation adds no frames: it
// picks its pose from the frame time, stepping agentWorkingFPS times a second.

const (
	agentWorkingFPS   = 10
	agentWorkingSteps = 16
	agentWorkingW     = 30
	agentWorkingH     = 12
)

// agentWorkingAnim turns the animation off (frame-cost measurements compare both).
var agentWorkingAnim = true

// workingPhase is the animation pose at time t: 0..agentWorkingSteps-1, a step per 1/fps.
func workingPhase(t time.Time) int {
	return int(t.UnixMilli()/int64(1000/agentWorkingFPS)) % agentWorkingSteps
}

// workingX is the crosshair's offset from the base point, in 0..1, for a phase: out and back.
func workingX(phase int) float32 {
	half := agentWorkingSteps / 2
	if phase > half {
		phase = agentWorkingSteps - phase
	}
	return float32(phase) / float32(half)
}

// workingIndicator is the animation element.
func workingIndicator() *gpui.Div {
	c := gpui.NewCanvas(func(b gpui.Bounds, w *gpui.Window, _ *gpui.App) {
		paintWorking(w, b, workingPhase(w.FrameTime()))
	})
	return gpui.NewDiv().ID("agent-working").SizeWH(agentWorkingW, agentWorkingH).FlexShrink(0).Child(c)
}

// paintWorking draws pose phase into b.
func paintWorking(w *gpui.Window, b gpui.Bounds, phase int) {
	x0, y := b.Origin.X+2, b.Origin.Y+b.Size.H/2
	span := b.Size.W - 8
	x1 := x0 + 4 + gpui.Pixels(workingX(phase))*(span-4)
	line := func(c gpui.Rgba, pts ...gpui.Point) {
		w.PaintStroke(gpui.Stroke{Points: pts, Width: 1, Color: c})
	}
	// Base point: a small square marker.
	line(uiMuted, gpui.Point{X: x0 - 1.5, Y: y - 1.5}, gpui.Point{X: x0 + 1.5, Y: y - 1.5},
		gpui.Point{X: x0 + 1.5, Y: y + 1.5}, gpui.Point{X: x0 - 1.5, Y: y + 1.5}, gpui.Point{X: x0 - 1.5, Y: y - 1.5})
	// Rubber band: 2px dashes with 2px gaps up to the crosshair.
	for x := x0 + 3; x < x1-1; x += 4 {
		line(uiMuted, gpui.Point{X: x, Y: y}, gpui.Point{X: min(x+2, x1-1), Y: y})
	}
	// Crosshair.
	line(uiAccent, gpui.Point{X: x1 - 4, Y: y}, gpui.Point{X: x1 + 4, Y: y})
	line(uiAccent, gpui.Point{X: x1, Y: y - 4}, gpui.Point{X: x1, Y: y + 4})
}
