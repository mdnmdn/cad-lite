package gpui

// Element is the per-frame UI node. Everything is rebuilt each frame. The id
// passed to Prepaint and Paint is the fully-resolved GlobalElementID path
// assigned by the window's tree walk, so elements do not need to carry it.
type Element interface {
	// ID returns the element's self-declared local id, if any.
	ID() (ElementID, bool)
	// RequestLayout registers the element's LayoutNode tree with the window
	// and returns the root node's LayoutID plus opaque per-element state.
	RequestLayout(w *Window, cx *App) (LayoutID, any)
	// Prepaint runs after layout and before paint: it registers hitboxes,
	// listeners, scroll state and input handlers, and returns per-element
	// state consumed by Paint.
	Prepaint(w *Window, cx *App, id ElementID, bounds Bounds, layoutState any) any
	// Paint emits draw primitives for this element (not its children; the
	// window walks children itself, in tree order).
	Paint(w *Window, cx *App, id ElementID, bounds Bounds, layoutState, prepaintState any)
}

// IntoElement is anything that can be converted into an Element, so that
// Child/Children accept *Div, *Text, AnyElement and friends.
type IntoElement interface {
	IntoElement() Element
}

// ElementID identifies an element across frames. It is the element's
// self-declared key (or none); the window prefixes it with the ancestor path
// to form a GlobalElementID.
type ElementID struct {
	Path []ElementPathKey
}

// ElementPathKey is one segment of an element path: a child index plus an
// optional string key.
type ElementPathKey struct {
	Index int
	Key   string
}

// GlobalElementID is the fully-resolved id used as a map key for retained
// element state (scroll offsets, hover, ...).
type GlobalElementID struct {
	Path []ElementPathKey
}

// Equal reports whether two global ids have the same path.
func (a GlobalElementID) Equal(b GlobalElementID) bool {
	if len(a.Path) != len(b.Path) {
		return false
	}
	for i := range a.Path {
		if a.Path[i] != b.Path[i] {
			return false
		}
	}
	return true
}

// LayoutID indexes into the window's per-frame layout engine.
type LayoutID int

// AnyElement erases an Element for a children slice.
type AnyElement struct{ elem Element }

// NewAnyElement converts e into an AnyElement. A nil IntoElement yields an
// empty AnyElement whose Element() is nil.
func NewAnyElement(e IntoElement) AnyElement {
	return AnyElement{elem: IntoElementOf(e)}
}

// Element returns the wrapped Element, or nil when empty.
func (a AnyElement) Element() Element { return a.elem }

// IntoElementOf normalizes anything IntoElement into an Element. It is
// nil-safe: a nil argument yields a nil Element.
func IntoElementOf(e IntoElement) Element {
	if e == nil {
		return nil
	}
	return e.IntoElement()
}

// elementIDOf returns an element's local id, or the empty id when the
// element is nil or declares none.
func elementIDOf(e Element) ElementID {
	if e == nil {
		return ElementID{}
	}
	if id, ok := e.ID(); ok {
		return id
	}
	return ElementID{}
}

// hasChildren lets the window's tree walk recurse into an element's child
// elements without knowing the concrete type. *Div implements it; leaf
// elements such as Text do not.
type hasChildren interface {
	elementChildren() []AnyElement
}

// layoutChildIDs is implemented by element layout states that carry one
// LayoutID per child, letting the draw pipeline correlate the element tree
// with the layout tree without re-running RequestLayout.
type layoutChildIDs interface {
	childLayoutIDs() []LayoutID
}

// layoutChildStates is implemented alongside layoutChildIDs when the state
// also carries each child's RequestLayout return value.
type layoutChildStates interface {
	childLayoutStates() []any
}

// ParentElementChildren is implemented by Elements that have child elements,
// letting custom containers outside the package participate in the window's
// tree walk like a Div: the window prepaints and paints each child after the
// element itself, with the child's laid-out bounds. RequestLayout must
// return the *ChildLayout from Window.RequestLayoutWithChildren (or a value
// embedding it) so the walk can pair children with their layout nodes.
type ParentElementChildren interface {
	Element
	// ElementChildren returns the children passed to
	// RequestLayoutWithChildren, in the same order.
	ElementChildren() []AnyElement
}

// ChildLayout is the RequestLayout state built by
// Window.RequestLayoutWithChildren: the container's node plus each child's
// LayoutID and RequestLayout state.
type ChildLayout struct {
	// Node is the container's layout node; its Rect and ContentSize are
	// valid from Prepaint on.
	Node        *LayoutNode
	childIDs    []LayoutID
	childStates []any
}

// childLayoutIDs satisfies layoutChildIDs.
func (c *ChildLayout) childLayoutIDs() []LayoutID { return c.childIDs }

// childLayoutStates satisfies layoutChildStates.
func (c *ChildLayout) childLayoutStates() []any { return c.childStates }

// childrenOf returns an element's children for the window's tree walk.
func childrenOf(el Element) ([]AnyElement, bool) {
	if pe, ok := el.(hasChildren); ok {
		return pe.elementChildren(), true
	}
	if pe, ok := el.(ParentElementChildren); ok {
		return pe.ElementChildren(), true
	}
	return nil, false
}
