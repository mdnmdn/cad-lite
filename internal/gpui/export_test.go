package gpui

import "image"

// RenderDamageScaled exposes renderDamage (logical scene drawn at scale) to
// the external tests.
func (r *Rasterizer) RenderDamageScaled(s *Scene, scale float32, clear Rgba) []image.Rectangle {
	return r.renderDamage(s, scale, clear)
}

// ScaleSceneRef exposes the stored-copy scaling reference to the external
// tests.
func ScaleSceneRef(s *Scene, scale float32) *Scene { return scaleScene(s, scale, &Scene{}) }
