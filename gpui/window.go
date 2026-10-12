package gpui

import (
	"fmt"
	"reflect"
	"strings"
)

// TextMeasurer is implemented by the window's text backend. It is satisfied
// by *Shaper once font.go/text.go land; window.go only needs this seam, so
// the package compiles and tests before text support exists.
type TextMeasurer interface {
	MeasureWrapped(text string, ts TextStyle, maxWidth float32) (w, h Pixels)
	ShapeWrapped(text string, ts TextStyle, maxWidth float32) []ShapedLineOut
}

// Window is one OS window plus all of its per-frame drawing state. It is
// created by App.OpenWindow and driven by the platform: the platform calls
// DispatchEvent for input and Draw when a frame is requested.
type Window struct {
	platformWindow PlatformWindow
	root           EntityRoot
	options        WindowOptions
	scale          float32
	invalid        bool
	focus          FocusID
	mousePosition  Point
	propagateEvent bool
	capturedHitbox int

	renderedFrame Frame
	nextFrame     Frame

	layoutNodes  map[LayoutID]*LayoutNode
	nextLayoutID LayoutID

	// elementStates retains per-element state (scroll offsets, ...) across
	// frames, keyed by the string encoding of the element's global id (a
	// GlobalElementID contains a slice and so cannot be a Go map key).
	elementStates map[string]any

	clipStack []Bounds

	text TextMeasurer
	app  *App

	// currentElementID is the global id of the element currently being
	// prepainted; the walker sets it before each Prepaint call.
	currentElementID GlobalElementID
}

// Frame is the result of one Draw: a scene plus the hitboxes, listeners and
// input handlers registered during that frame's prepaint. The rendered frame
// is what DispatchEvent reads.
type Frame struct {
	scene          Scene
	hitboxes       BoundsTree
	mouseListeners []MouseListener
	keyNodes       []keyNode
	inputHandlers  []PlatformInputHandler
	elementStates  map[string]any
	order          uint32
}

// reset empties the frame for a new draw.
func (f *Frame) reset() {
	f.scene.Clear()
	f.hitboxes.Clear()
	f.mouseListeners = f.mouseListeners[:0]
	f.keyNodes = f.keyNodes[:0]
	f.inputHandlers = f.inputHandlers[:0]
	f.elementStates = nil
	f.order = 0
}

// SceneRef returns the frame's scene for inspection or rasterization.
func (f *Frame) SceneRef() *Scene { return &f.scene }

// HitboxCount returns the number of hitboxes registered this frame.
func (f *Frame) HitboxCount() int { return f.hitboxes.Len() }

// HitTest hit-tests the frame's bounds tree.
func (f *Frame) HitTest(p Point) int { return f.hitboxes.HitTest(p) }

// MouseListener is the per-hitbox mouse dispatch table recorded during
// prepaint. mouseListeners[i] pairs with hitboxes[i].
type MouseListener struct {
	Bounds        Bounds
	OnMouseDown   func(*MouseDownEvent, *Window, *App)
	OnMouseUp     func(*MouseUpEvent, *Window, *App)
	OnMouseMove   func(*MouseMoveEvent, *Window, *App)
	OnClick       func(*ClickEvent, *Window, *App)
	OnScrollWheel func(*ScrollWheelEvent, *Window, *App)
	KeyContext    string

	// elementID identifies the owning element for retained state lookups
	// (scroll offsets) during scroll dispatch.
	elementID GlobalElementID
	// scrollable marks a scroll container; scrollWheel dispatch clamps and
	// stores an offset for the innermost one under the cursor.
	scrollable bool
	// scrollMax is the container's scrollable extent, captured at prepaint.
	scrollMax Size
}

// keyNode is the per-element keyboard dispatch table recorded during
// prepaint, in tree order (root first).
type keyNode struct {
	KeyContext string
	FocusID    FocusID
	OnKeyDown  func(*KeyDownEvent, *Window, *App)
	OnKeyUp    func(*KeyUpEvent, *Window, *App)
	OnAction   map[reflect.Type]func(Action, *Window, *App)
}

// scrollState is the retained scroll offset of one scroll container.
type scrollState struct {
	offset     Point
	maxW, maxH Pixels
}

// newWindow wraps a PlatformWindow. It hooks the platform's frame callback
// when the window implements FrameRequester.
func newWindow(pw PlatformWindow, o WindowOptions, app *App) *Window {
	w := &Window{
		platformWindow: pw,
		options:        o,
		app:            app,
		scale:          1,
		invalid:        true,
		layoutNodes:    make(map[LayoutID]*LayoutNode),
		nextLayoutID:   1,
		elementStates:  make(map[string]any),
	}
	if pw != nil {
		if s := pw.ScaleFactor(); s > 0 {
			w.scale = s
		}
		if fr, ok := pw.(FrameRequester); ok {
			fr.OnRequestFrame(func() {
				if w.app != nil {
					w.Draw(w.app)
				}
			})
		}
	}
	return w
}

// Bounds returns the window size in logical pixels, falling back to the
// requested options and then to 800x600 when the platform reports nothing.
func (w *Window) Bounds() Bounds {
	var bw, bh Pixels
	if w.platformWindow != nil {
		bw, bh = w.platformWindow.Bounds()
	}
	if bw <= 0 {
		bw = Pixels(w.options.Width)
	}
	if bh <= 0 {
		bh = Pixels(w.options.Height)
	}
	if bw <= 0 {
		bw = 800
	}
	if bh <= 0 {
		bh = 600
	}
	return Bounds{Size: Size{W: bw, H: bh}}
}

// ScaleFactor returns the device pixel ratio of the window.
func (w *Window) ScaleFactor() float32 { return w.scale }

// Refresh marks the window dirty and asks the platform for a frame. The
// dirty flag is cleared by the next Draw.
func (w *Window) Refresh() {
	w.invalid = true
	w.scheduleFrame()
}

// scheduleFrame asks the platform to invoke the registered frame callback.
func (w *Window) scheduleFrame() {
	if w.platformWindow != nil {
		w.platformWindow.RequestFrame()
	}
}

// RequestFocus moves keyboard focus to h.
func (w *Window) RequestFocus(h *FocusHandle) {
	if h == nil || !h.IsValid() {
		return
	}
	w.focus = h.ID()
	w.Refresh()
}

// Focus returns a handle to the currently focused element, or nil when
// nothing (or a released id) is focused.
func (w *Window) Focus() *FocusHandle {
	if w.focus == 0 || w.app == nil || !w.app.focusMap.alive(w.focus) {
		return nil
	}
	return &FocusHandle{id: w.focus, app: w.app}
}

// Blur clears the window's focus.
func (w *Window) Blur() {
	if w.focus == 0 {
		return
	}
	w.focus = 0
	w.Refresh()
}

// StopPropagation halts the current DispatchEvent traversal: remaining
// capture listeners, all bubble listeners, and keymap dispatch are skipped.
func (w *Window) StopPropagation() { w.propagateEvent = false }

// HandleInput registers a text input handler for this frame's input
// dispatch. Called during prepaint by focused text elements.
func (w *Window) HandleInput(h PlatformInputHandler) {
	if h == nil {
		return
	}
	w.nextFrame.inputHandlers = append(w.nextFrame.inputHandlers, h)
}

// focusedInputHandler returns the last handler registered this frame, which
// the POC treats as the focused text field.
func (w *Window) focusedInputHandler() PlatformInputHandler {
	n := len(w.renderedFrame.inputHandlers)
	if n == 0 {
		return nil
	}
	return w.renderedFrame.inputHandlers[n-1]
}

// InsertHitbox registers a hit-testable region for the frame being built and
// returns its index, which pairs with the frame's MouseListener slice.
func (w *Window) InsertHitbox(b Bounds) int {
	idx := w.nextFrame.hitboxes.Insert(Hitbox{Bounds: b, Order: w.nextFrame.order})
	w.nextFrame.mouseListeners = append(w.nextFrame.mouseListeners, MouseListener{Bounds: b})
	w.nextFrame.order++
	return idx
}

// PaintQuad inserts a quad into the frame being built, filling an unset
// content mask with the current clip.
func (w *Window) PaintQuad(q Quad) {
	if q.ContentMask.Bounds.Size.W <= 0 || q.ContentMask.Bounds.Size.H <= 0 {
		q.ContentMask = ContentMask{Bounds: w.CurrentClip()}
	}
	w.nextFrame.scene.InsertQuad(q)
}

// PaintUnderline inserts an underline into the frame being built.
func (w *Window) PaintUnderline(u Underline) {
	if u.ContentMask.Bounds.Size.W <= 0 || u.ContentMask.Bounds.Size.H <= 0 {
		u.ContentMask = ContentMask{Bounds: w.CurrentClip()}
	}
	w.nextFrame.scene.InsertUnderline(u)
}

// PaintGlyph inserts a glyph sprite into the frame being built.
func (w *Window) PaintGlyph(g GlyphSprite) {
	if g.ContentMask.Bounds.Size.W <= 0 || g.ContentMask.Bounds.Size.H <= 0 {
		g.ContentMask = ContentMask{Bounds: w.CurrentClip()}
	}
	w.nextFrame.scene.InsertSprite(g)
}

// PushLayer records a z-order boundary in the scene being built.
func (w *Window) PushLayer() { w.nextFrame.scene.PushLayer() }

// PushClip intersects b with the current clip and pushes the result.
func (w *Window) PushClip(b Bounds) {
	w.clipStack = append(w.clipStack, b.Intersection(w.CurrentClip()))
}

// PopClip removes the innermost clip.
func (w *Window) PopClip() {
	if n := len(w.clipStack); n > 0 {
		w.clipStack = w.clipStack[:n-1]
	}
}

// CurrentClip returns the innermost clip rect, or a rect large enough to
// contain any practical primitive when nothing has been pushed.
func (w *Window) CurrentClip() Bounds {
	if n := len(w.clipStack); n > 0 {
		return w.clipStack[n-1]
	}
	return InfiniteContentMask().Bounds
}

// globalIDKey encodes a GlobalElementID as a comparable string. It is
// required because GlobalElementID contains a slice and cannot be a map key.
func globalIDKey(id GlobalElementID) string {
	var b strings.Builder
	for _, k := range id.Path {
		fmt.Fprintf(&b, "/%d:%s", k.Index, k.Key)
	}
	return b.String()
}

// ElementState copies the retained state stored for id into out, which must
// be a non-nil pointer. It reports whether state was found and assignable.
func (w *Window) ElementState(id GlobalElementID, out any) bool {
	if out == nil || w.elementStates == nil {
		return false
	}
	v, ok := w.elementStates[globalIDKey(id)]
	if !ok {
		return false
	}
	ov := reflect.ValueOf(out)
	if ov.Kind() != reflect.Ptr || ov.IsNil() {
		return false
	}
	vv := reflect.ValueOf(v)
	if !vv.Type().AssignableTo(ov.Type().Elem()) {
		return false
	}
	ov.Elem().Set(vv)
	return true
}

// SetElementState stores retained state for id, replacing any previous
// value.
func (w *Window) SetElementState(id GlobalElementID, v any) {
	if w.elementStates == nil {
		w.elementStates = make(map[string]any)
	}
	w.elementStates[globalIDKey(id)] = v
}

// ScrollOffset returns the scroll offset previously stored for id, or the
// zero point.
func (w *Window) ScrollOffset(id GlobalElementID) Point {
	st, _ := w.elementStates[globalIDKey(id)].(scrollState)
	return st.offset
}

// SetScrollOffset stores a scroll offset for id, clamped to the extent
// captured at the last prepaint of that element.
func (w *Window) SetScrollOffset(id GlobalElementID, p Point) {
	key := globalIDKey(id)
	st, _ := w.elementStates[key].(scrollState)
	st.offset = Point{X: clampf(p.X, 0, st.maxW), Y: clampf(p.Y, 0, st.maxH)}
	w.elementStates[key] = st
}

// initScrollState records a scroll container's extent, keeping any existing
// offset clamped to it.
func (w *Window) initScrollState(id GlobalElementID, max Size) {
	key := globalIDKey(id)
	st, _ := w.elementStates[key].(scrollState)
	st.maxW, st.maxH = max.W, max.H
	st.offset.X = clampf(st.offset.X, 0, st.maxW)
	st.offset.Y = clampf(st.offset.Y, 0, st.maxH)
	w.elementStates[key] = st
}

// SetTextMeasurer installs the window's text backend. A nil measurer makes
// text elements fall back to a crude estimator.
func (w *Window) SetTextMeasurer(t TextMeasurer) { w.text = t }

// Scene returns the last drawn frame's scene.
func (w *Window) Scene() *Scene { return &w.renderedFrame.scene }

// registerLayoutNode stores a node for the frame being built and returns its
// id.
func (w *Window) registerLayoutNode(n *LayoutNode) LayoutID {
	if w.layoutNodes == nil {
		w.layoutNodes = make(map[LayoutID]*LayoutNode)
	}
	id := w.nextLayoutID
	w.nextLayoutID++
	w.layoutNodes[id] = n
	return id
}

// renderNode is one element's slot in the resolved render tree the draw
// pipeline walks: its final bounds, layout node, and per-phase states.
type renderNode struct {
	elem     Element
	id       GlobalElementID
	bounds   Bounds
	layout   *LayoutNode
	state    any
	prepaint any
	children []*renderNode
}

// Draw runs the full pipeline: render the root element, lay out, prepaint
// (hitboxes, listeners, scroll state), paint (scene primitives), then swap
// the new frame into renderedFrame and clear the dirty flag.
func (w *Window) Draw(cx *App) {
	if cx == nil {
		cx = w.app
	}
	w.nextFrame.reset()
	w.layoutNodes = make(map[LayoutID]*LayoutNode)
	w.nextLayoutID = 1
	w.clipStack = w.clipStack[:0]

	var el Element
	if w.root != nil {
		el = IntoElementOf(w.root.Render(w, cx))
	}
	if el != nil {
		rootPath := elementIDOf(el)
		gid := GlobalElementID{Path: append([]ElementPathKey(nil), rootPath.Path...)}
		lid, state := el.RequestLayout(w, cx)
		rootNode := w.layoutNodes[lid]
		if rootNode != nil {
			b := w.Bounds()
			Layout(rootNode, Definite(float32(b.Width()), float32(b.Height())))
		}
		rn := w.buildRenderNode(el, gid, rootNode, state, Point{}, cx)
		w.prepaintTree(rn, cx)
		w.paintTree(rn, cx)
	}

	w.renderedFrame, w.nextFrame = w.nextFrame, w.renderedFrame
	w.nextFrame.reset()
	w.invalid = false
}

// buildRenderNode correlates the element tree with the layout tree,
// assigning each node its final absolute bounds. Scroll containers translate
// their children by the retained scroll offset.
func (w *Window) buildRenderNode(el Element, gid GlobalElementID, node *LayoutNode, state any, origin Point, cx *App) *renderNode {
	var rect Bounds
	if node != nil {
		rect = node.Rect
	}
	rn := &renderNode{
		elem:   el,
		id:     gid,
		bounds: rect.Translate(origin),
		layout: node,
		state:  state,
	}
	pe, ok := el.(hasChildren)
	if !ok {
		return rn
	}
	kids := pe.elementChildren()
	if len(kids) == 0 {
		return rn
	}
	var childIDs []LayoutID
	var childStates []any
	if lc, ok := state.(layoutChildIDs); ok {
		childIDs = lc.childLayoutIDs()
	}
	if cs, ok := state.(layoutChildStates); ok {
		childStates = cs.childLayoutStates()
	}
	childOrigin := Point{X: origin.X + rect.Origin.X, Y: origin.Y + rect.Origin.Y}
	if node != nil && node.Style.Overflow != OverflowVisible {
		off := w.ScrollOffset(gid)
		childOrigin.X -= off.X
		childOrigin.Y -= off.Y
	}
	ki := 0
	for _, c := range kids {
		cel := c.Element()
		if cel == nil {
			continue
		}
		var lid LayoutID
		if ki < len(childIDs) {
			lid = childIDs[ki]
		}
		var cstate any
		if ki < len(childStates) {
			cstate = childStates[ki]
		}
		ki++
		local, hasID := cel.ID()
		key := ElementPathKey{Index: ki - 1}
		if hasID && len(local.Path) > 0 {
			key.Key = local.Path[len(local.Path)-1].Key
		}
		cpath := make([]ElementPathKey, 0, len(gid.Path)+1)
		cpath = append(cpath, gid.Path...)
		cpath = append(cpath, key)
		rn.children = append(rn.children,
			w.buildRenderNode(cel, GlobalElementID{Path: cpath}, w.layoutNodes[lid], cstate, childOrigin, cx))
	}
	return rn
}

// prepaintTree walks the render tree in order, letting each element register
// hitboxes, listeners and input handlers for the frame being built.
func (w *Window) prepaintTree(n *renderNode, cx *App) {
	if n == nil || n.elem == nil {
		return
	}
	if n.layout != nil && !n.layout.Style.Display {
		return
	}
	w.currentElementID = n.id
	n.prepaint = n.elem.Prepaint(w, cx, ElementID{Path: n.id.Path}, n.bounds, n.state)
	for _, c := range n.children {
		w.prepaintTree(c, cx)
	}
}

// paintTree walks the render tree in order: paint the element, then its
// children inside its clip, then pop the clip. Clips are pushed here (not in
// prepaint) so the clip stack is balanced per subtree during painting — a
// prepaint-pushed stack would leave every container's clip active while early
// siblings paint, masking the wrong region.
func (w *Window) paintTree(n *renderNode, cx *App) {
	if n == nil || n.elem == nil {
		return
	}
	if n.layout != nil && !n.layout.Style.Display {
		return
	}
	st, isDiv := n.prepaint.(*divPrepaint)
	pushed := isDiv && st.clipped
	if pushed {
		w.PushClip(n.bounds)
	}
	n.elem.Paint(w, cx, ElementID{Path: n.id.Path}, n.bounds, n.state, n.prepaint)
	for _, c := range n.children {
		w.paintTree(c, cx)
	}
	if pushed {
		w.PopClip()
	}
}

// DispatchEvent routes one platform input event through the rendered frame.
func (w *Window) DispatchEvent(input PlatformInput, cx *App) {
	if cx == nil {
		cx = w.app
	}
	switch e := input.(type) {
	case MouseDownEvent:
		w.mousePosition = e.Position
		ev := e
		w.dispatchMouse(e.Position, cx, func(l *MouseListener) {
			if l.OnMouseDown != nil {
				l.OnMouseDown(&ev, w, cx)
			}
		})
	case MouseUpEvent:
		w.mousePosition = e.Position
		ev := e
		w.dispatchMouse(e.Position, cx, func(l *MouseListener) {
			if l.OnMouseUp != nil {
				l.OnMouseUp(&ev, w, cx)
			}
		})
	case MouseMoveEvent:
		w.mousePosition = e.Position
		ev := e
		w.dispatchMouse(e.Position, cx, func(l *MouseListener) {
			if l.OnMouseMove != nil {
				l.OnMouseMove(&ev, w, cx)
			}
		})
	case ClickEvent:
		pos := w.mousePosition
		if e.Up != nil {
			pos = e.Up.Position
		}
		ev := e
		w.dispatchMouse(pos, cx, func(l *MouseListener) {
			if l.OnClick != nil {
				l.OnClick(&ev, w, cx)
			}
		})
	case ScrollWheelEvent:
		w.dispatchScroll(e, cx)
	case KeyDownEvent:
		w.dispatchKeyDown(e, cx)
	case KeyUpEvent:
		w.dispatchKeyUp(e, cx)
	case ModifiersChangedEvent:
		// The POC has no modifier-state listeners.
	}
}

// dispatchMouse hit-tests the rendered frame and runs call over the hit
// path: capture front-to-back (ancestors only), then bubble back-to-front
// (innermost first), honoring StopPropagation. The innermost target runs
// once, in the bubble phase, matching DOM semantics.
func (w *Window) dispatchMouse(pos Point, cx *App, call func(*MouseListener)) {
	idx := w.renderedFrame.hitboxes.HitTest(pos)
	w.capturedHitbox = idx
	if idx < 0 {
		return
	}
	w.propagateEvent = true
	for i := 0; i < idx && w.propagateEvent; i++ {
		call(&w.renderedFrame.mouseListeners[i])
	}
	for i := idx; i >= 0 && w.propagateEvent; i-- {
		call(&w.renderedFrame.mouseListeners[i])
	}
}

// dispatchScroll runs OnScrollWheel over the hit path, then clamps and
// stores a scroll offset for the innermost scroll container under the
// cursor and requests a redraw.
func (w *Window) dispatchScroll(e ScrollWheelEvent, cx *App) {
	ev := e
	w.mousePosition = e.Position
	w.dispatchMouse(e.Position, cx, func(l *MouseListener) {
		if l.OnScrollWheel != nil {
			l.OnScrollWheel(&ev, w, cx)
		}
	})
	idx := w.renderedFrame.hitboxes.HitTest(e.Position)
	for i := idx; i >= 0; i-- {
		l := &w.renderedFrame.mouseListeners[i]
		if !l.scrollable {
			continue
		}
		key := globalIDKey(l.elementID)
		st, _ := w.elementStates[key].(scrollState)
		st.offset.X = clampf(st.offset.X+e.DeltaX, 0, st.maxW)
		st.offset.Y = clampf(st.offset.Y+e.DeltaY, 0, st.maxH)
		st.maxW, st.maxH = l.scrollMax.W, l.scrollMax.H
		st.offset.X = clampf(st.offset.X, 0, st.maxW)
		st.offset.Y = clampf(st.offset.Y, 0, st.maxH)
		w.elementStates[key] = st
		w.Refresh()
		return
	}
}

// keyPath returns the key nodes a key event should traverse: the focus path
// (root to the focused node) when something is focused, otherwise every key
// node. The last entry is the event target.
func (w *Window) keyPath() []keyNode {
	nodes := w.renderedFrame.keyNodes
	if w.focus == 0 {
		return nodes
	}
	for i := range nodes {
		if nodes[i].FocusID != 0 && nodes[i].FocusID == w.focus {
			return nodes[:i+1]
		}
	}
	return nodes
}

// dispatchKeyDown delivers a key press to a focused text input when it wants
// character input; otherwise it runs OnKeyDown over the focus path
// (capture then bubble) and finally consults the keymap, dispatching any
// matched action to OnAction handlers along the same path.
func (w *Window) dispatchKeyDown(e KeyDownEvent, cx *App) {
	ev := e
	if h := w.focusedInputHandler(); h != nil && h.AcceptsTextInput() &&
		e.PreferCharacterInput && e.Keystroke.KeyChar != nil {
		var rng *IntRange
		if sel, ok := h.SelectedTextRange(); ok && !sel.IsNil {
			r := sel.Range
			rng = &r
		}
		h.ReplaceTextInRange(rng, *e.Keystroke.KeyChar)
		return
	}

	path := w.keyPath()
	if len(path) == 0 {
		return
	}
	w.propagateEvent = true
	for i := 0; i < len(path)-1 && w.propagateEvent; i++ {
		if path[i].OnKeyDown != nil {
			path[i].OnKeyDown(&ev, w, cx)
		}
	}
	for i := len(path) - 1; i >= 0 && w.propagateEvent; i-- {
		if path[i].OnKeyDown != nil {
			path[i].OnKeyDown(&ev, w, cx)
		}
	}
	if !w.propagateEvent || cx == nil {
		return
	}

	ctxStack := make([]string, 0, len(path))
	for _, n := range path {
		if n.KeyContext != "" {
			ctxStack = append(ctxStack, n.KeyContext)
		}
	}
	action := cx.Keymap().MatchKeystroke(e.Keystroke, ctxStack)
	if action == nil {
		return
	}
	at := reflect.TypeOf(action)
	w.propagateEvent = true
	for i := len(path) - 1; i >= 0 && w.propagateEvent; i-- {
		if f, ok := path[i].OnAction[at]; ok {
			f(action, w, cx)
		}
	}
}

// dispatchKeyUp runs OnKeyUp over the focus path, capture then bubble.
func (w *Window) dispatchKeyUp(e KeyUpEvent, cx *App) {
	ev := e
	path := w.keyPath()
	if len(path) == 0 {
		return
	}
	w.propagateEvent = true
	for i := 0; i < len(path)-1 && w.propagateEvent; i++ {
		if path[i].OnKeyUp != nil {
			path[i].OnKeyUp(&ev, w, cx)
		}
	}
	for i := len(path) - 1; i >= 0 && w.propagateEvent; i-- {
		if path[i].OnKeyUp != nil {
			path[i].OnKeyUp(&ev, w, cx)
		}
	}
}
