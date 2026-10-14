// Package render turns document entities into a backend-neutral display
// list and owns the view (camera). It never imports a graphics package:
// render/gpuirender draws a display list with the gpui library.
package render

import (
	"math"

	"go-draw/geom"
)

// View is the camera: which part of model space the viewport shows. Model
// space is y-up, the viewport y-down; screen coordinates are logical pixels
// relative to the viewport's top-left corner.
type View struct {
	// Center is the model point shown at the middle of the viewport.
	Center geom.Vec2
	// Scale is pixels per model unit.
	Scale float64
	// Width and Height are the viewport size in logical pixels.
	Width, Height float64
}

// MinScale and MaxScale bound zooming.
const (
	MinScale = 1e-6
	MaxScale = 1e6
)

// DefaultView returns a view of the model origin at one pixel per unit.
func DefaultView(w, h float64) View {
	return View{Center: geom.V(0, 0), Scale: 1, Width: w, Height: h}
}

// ToScreen maps a model point to viewport pixels. The subtraction of Center
// happens in float64 before any narrowing, so large coordinates do not
// jitter (camera-relative rendering).
func (v View) ToScreen(p geom.Vec2) (x, y float64) {
	return (p.X-v.Center.X)*v.Scale + v.Width/2, -(p.Y-v.Center.Y)*v.Scale + v.Height/2
}

// ToModel maps viewport pixels to a model point.
func (v View) ToModel(x, y float64) geom.Vec2 {
	return geom.V((x-v.Width/2)/v.Scale+v.Center.X, -(y-v.Height/2)/v.Scale+v.Center.Y)
}

// PixelSize returns the model length of one logical pixel.
func (v View) PixelSize() float64 { return 1 / v.Scale }

// Visible returns the model-space box the viewport shows.
func (v View) Visible() geom.Box2 {
	return geom.BoxOf(v.ToModel(0, 0), v.ToModel(v.Width, v.Height))
}

// ZoomAt scales by factor, keeping the model point under the viewport
// pixel (x, y) fixed.
func (v *View) ZoomAt(x, y, factor float64) {
	anchor := v.ToModel(x, y)
	v.Scale = math.Max(MinScale, math.Min(MaxScale, v.Scale*factor))
	after := v.ToModel(x, y)
	v.Center = v.Center.Add(anchor.Sub(after))
}

// PanBy moves the drawing by (dx, dy) viewport pixels.
func (v *View) PanBy(dx, dy float64) {
	v.Center = v.Center.Add(geom.V(-dx/v.Scale, dy/v.Scale))
}

// Fit centres box in the viewport, leaving margin pixels on each side. An
// empty box leaves the view unchanged.
func (v *View) Fit(box geom.Box2, margin float64) {
	if box.IsEmpty() || v.Width <= 2*margin || v.Height <= 2*margin {
		return
	}
	w, h := box.Size()
	w, h = math.Max(w, 1e-9), math.Max(h, 1e-9)
	v.Scale = math.Max(MinScale, math.Min(MaxScale, math.Min((v.Width-2*margin)/w, (v.Height-2*margin)/h)))
	v.Center = box.Center()
}

// Band is the zoom bucket for scale: tessellations are cached per band, so
// zooming within a band reuses them.
func Band(scale float64) int { return int(math.Floor(math.Log2(scale))) }

// BandTolerance returns the chord tolerance (model units) for band: a
// quarter pixel at the band's lowest scale, so a curve is within half a
// pixel at every scale in the band.
func BandTolerance(band int) float64 { return 0.25 / math.Exp2(float64(band)) }
