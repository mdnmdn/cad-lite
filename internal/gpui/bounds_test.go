package gpui

import "testing"

func TestBoundsTreeHitTestTopmostOrderWins(t *testing.T) {
	var tree BoundsTree
	low := tree.Insert(NewHitbox(NewBounds(0, 0, 100, 100), 1))
	high := tree.Insert(NewHitbox(NewBounds(0, 0, 100, 100), 3))
	mid := tree.Insert(NewHitbox(NewBounds(0, 0, 100, 100), 2))

	got := tree.HitTest(Point{X: 50, Y: 50})
	if got != high {
		t.Fatalf("HitTest(50,50) = %d, want %d (highest order)", got, high)
	}
	if low == high || mid == high {
		t.Fatalf("indices must be distinct: low=%d mid=%d high=%d", low, mid, high)
	}
}

func TestBoundsTreeHitTestSameOrderLaterInsertWins(t *testing.T) {
	var tree BoundsTree
	first := tree.Insert(NewHitbox(NewBounds(0, 0, 10, 10), 5))
	second := tree.Insert(NewHitbox(NewBounds(0, 0, 10, 10), 5))

	if got := tree.HitTest(Point{X: 5, Y: 5}); got != second {
		t.Fatalf("HitTest(5,5) = %d, want %d (later insert)", got, second)
	}
	if first == second {
		t.Fatalf("indices must be distinct")
	}
}

func TestBoundsTreeHitTestOutsideAllBoxes(t *testing.T) {
	var tree BoundsTree
	tree.Insert(NewHitbox(NewBounds(0, 0, 10, 10), 1))
	tree.Insert(NewHitbox(NewBounds(20, 20, 10, 10), 2))

	if got := tree.HitTest(Point{X: 15, Y: 15}); got != -1 {
		t.Fatalf("HitTest(15,15) = %d, want -1", got)
	}
}

func TestBoundsTreeHitTestHalfOpenEdges(t *testing.T) {
	var tree BoundsTree
	a := tree.Insert(NewHitbox(NewBounds(0, 0, 10, 10), 1))  // right edge at x=10
	b := tree.Insert(NewHitbox(NewBounds(10, 0, 10, 10), 0)) // starts at x=10

	if !tree.Box(a).Bounds.Contains(Point{X: 9.5, Y: 5}) {
		t.Fatalf("point (9.5,5) should be inside box A")
	}
	if tree.Box(a).Bounds.Contains(Point{X: 10, Y: 5}) {
		t.Fatalf("Contains is half-open: (10,5) must not be inside box A")
	}
	// (10,5) is only inside B; B wins even though its order is lower.
	if got := tree.HitTest(Point{X: 10, Y: 5}); got != b {
		t.Fatalf("HitTest(10,5) = %d, want %d (only B contains the edge point)", got, b)
	}
}

func TestBoundsTreeClearResetsLen(t *testing.T) {
	var tree BoundsTree
	tree.Insert(NewHitbox(NewBounds(0, 0, 10, 10), 1))
	tree.Insert(NewHitbox(NewBounds(0, 0, 20, 20), 2))
	if tree.Len() != 2 {
		t.Fatalf("Len() = %d, want 2", tree.Len())
	}
	tree.Clear()
	if tree.Len() != 0 {
		t.Fatalf("Len() after Clear = %d, want 0", tree.Len())
	}
	if got := tree.HitTest(Point{X: 5, Y: 5}); got != -1 {
		t.Fatalf("HitTest after Clear = %d, want -1", got)
	}
	if boxes := tree.Boxes(); len(boxes) != 0 {
		t.Fatalf("Boxes() after Clear = %d entries, want 0", len(boxes))
	}
}

func TestBoundsTreeBoxAndBoxes(t *testing.T) {
	var tree BoundsTree
	h := NewHitbox(NewBounds(1, 2, 3, 4), 7)
	h.HasMouseListeners = true
	idx := tree.Insert(h)
	if got := tree.Box(idx); got.Bounds != h.Bounds || got.Order != 7 || !got.HasMouseListeners {
		t.Fatalf("Box(%d) = %+v, want %+v", idx, got, h)
	}
	copies := tree.Boxes()
	if len(copies) != 1 || copies[0] != h {
		t.Fatalf("Boxes() = %+v, want [%+v]", copies, h)
	}
}
