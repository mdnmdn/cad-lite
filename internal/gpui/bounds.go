package gpui

// Hitbox is a hit-testable region registered during prepaint.
type Hitbox struct {
	// Bounds is the hit-testable rect.
	Bounds Bounds
	// Order is the paint order; higher = later = on top.
	Order uint32
	// HasMouseListeners reports whether any mouse listener was registered
	// against this hitbox during prepaint.
	HasMouseListeners bool
}

// NewHitbox builds a hitbox with the given bounds and paint order.
func NewHitbox(b Bounds, order uint32) Hitbox {
	return Hitbox{Bounds: b, Order: order}
}

// BoundsTree is the flat list of hitboxes for one frame, hit-tested by
// paint order. The zero value is ready to use.
type BoundsTree struct {
	boxes []Hitbox
}

// Clear removes every hitbox.
func (t *BoundsTree) Clear() {
	t.boxes = t.boxes[:0]
}

// Insert appends a hitbox and returns its index.
func (t *BoundsTree) Insert(h Hitbox) int {
	t.boxes = append(t.boxes, h)
	return len(t.boxes) - 1
}

// HitTest returns the index of the box containing p with the highest Order;
// among equal orders the later insert (higher index) wins. It returns -1
// when no box contains p. Bounds.Contains is half-open, so a point exactly
// on a box's right or bottom edge is not contained by that box.
func (t *BoundsTree) HitTest(p Point) int {
	best := -1
	for i := range t.boxes {
		if !t.boxes[i].Bounds.Contains(p) {
			continue
		}
		if best == -1 || t.boxes[i].Order >= t.boxes[best].Order {
			best = i
		}
	}
	return best
}

func (t *BoundsTree) truncate(n int) { t.boxes = t.boxes[:n] }

// Len returns the number of hitboxes.
func (t *BoundsTree) Len() int { return len(t.boxes) }

// Box returns the hitbox at index i; it panics if i is out of range.
func (t *BoundsTree) Box(i int) Hitbox { return t.boxes[i] }

// Boxes returns a copy of all hitboxes, in insertion order.
func (t *BoundsTree) Boxes() []Hitbox {
	return append([]Hitbox(nil), t.boxes...)
}
