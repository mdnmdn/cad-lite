package gpui

import "testing"

func TestSceneClearEmptiesAndResetsOrdering(t *testing.T) {
	var s Scene
	s.InsertQuad(Quad{Bounds: NewBounds(0, 0, 10, 10)})
	s.InsertSprite(GlyphSprite{Bounds: NewBounds(0, 0, 10, 10)})
	s.InsertUnderline(Underline{Bounds: NewBounds(0, 0, 10, 10), Thickness: 1})
	if s.PrimitiveCount() != 3 {
		t.Fatalf("PrimitiveCount() = %d, want 3", s.PrimitiveCount())
	}
	s.PushLayer()

	s.Clear()

	if len(s.Quads) != 0 || len(s.Sprites) != 0 || len(s.Underlines) != 0 {
		t.Fatalf("Clear left primitives: %d quads, %d sprites, %d underlines",
			len(s.Quads), len(s.Sprites), len(s.Underlines))
	}
	// Ordering is reset: the next auto order matches a fresh scene's.
	var fresh Scene
	s.InsertQuad(Quad{})
	fresh.InsertQuad(Quad{})
	if s.Quads[0].Order != fresh.Quads[0].Order {
		t.Fatalf("auto order after Clear = %d, want %d (fresh scene)", s.Quads[0].Order, fresh.Quads[0].Order)
	}
}

func TestSceneAutoOrderStrictlyIncreases(t *testing.T) {
	var s Scene
	for i := 0; i < 3; i++ {
		s.InsertQuad(Quad{Bounds: NewBounds(0, 0, 10, 10)})
	}
	if len(s.Quads) != 3 {
		t.Fatalf("len(Quads) = %d, want 3", len(s.Quads))
	}
	for i := 1; i < len(s.Quads); i++ {
		if s.Quads[i].Order <= s.Quads[i-1].Order {
			t.Fatalf("orders not strictly increasing: %d then %d", s.Quads[i-1].Order, s.Quads[i].Order)
		}
	}
}

func TestScenePushLayerBumpsOrder(t *testing.T) {
	var s Scene
	s.InsertQuad(Quad{Bounds: NewBounds(0, 0, 10, 10)})
	s.InsertSprite(GlyphSprite{Bounds: NewBounds(0, 0, 10, 10)})
	before := s.Sprites[0].Order

	s.PushLayer()

	s.InsertUnderline(Underline{Bounds: NewBounds(0, 0, 10, 10), Thickness: 1})
	after := s.Underlines[0].Order
	if after <= before {
		t.Fatalf("order after PushLayer = %d, want > %d", after, before)
	}
}

func TestSceneExplicitOrderIsPreserved(t *testing.T) {
	var s Scene
	s.InsertQuad(Quad{Order: 42})
	if s.Quads[0].Order != 42 {
		t.Fatalf("explicit order overwritten: got %d, want 42", s.Quads[0].Order)
	}
	// Auto orders stay above explicit ones.
	s.InsertQuad(Quad{})
	if s.Quads[1].Order <= 42 {
		t.Fatalf("auto order %d, want > explicit 42", s.Quads[1].Order)
	}
}

func TestScenePrimitiveCount(t *testing.T) {
	var s Scene
	if s.PrimitiveCount() != 0 {
		t.Fatalf("empty PrimitiveCount() = %d, want 0", s.PrimitiveCount())
	}
	s.InsertQuad(Quad{})
	s.InsertQuad(Quad{})
	s.InsertSprite(GlyphSprite{})
	s.InsertUnderline(Underline{Thickness: 1})
	if s.PrimitiveCount() != 4 {
		t.Fatalf("PrimitiveCount() = %d, want 4", s.PrimitiveCount())
	}
}
