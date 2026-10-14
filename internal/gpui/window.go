package gpui

import (
	"math"
	"reflect"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"
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
	// deviceScale is the platform's pixel ratio; scale is deviceScale*uiScale.
	deviceScale    float32
	uiScale        float32
	invalid        bool
	focus          FocusID
	mousePosition  Point
	hasMouse       bool
	propagateEvent bool
	// hovered maps the global-id key of each currently hovered element to its
	// hitbox depth; callbacks are looked up in the rendered frame when fired.
	hovered map[string]int
	tooltip tooltipState
	// active captures the primary press path, pruned on leave and before paint.
	active         map[string]struct{}
	scrollbarDrag  *scrollbarDrag
	scrollbarPress bool
	scrollbarClick *MouseUpEvent
	inputSelection *inputSelectionDrag

	// hitboxStack and keyStack hold the indices of the innermost enclosing
	// hitbox / key node while prepainting, giving each new one its parent.
	hitboxStack []int
	keyStack    []int

	renderedFrame Frame
	nextFrame     Frame

	layoutNodes  map[LayoutID]*LayoutNode
	nextLayoutID LayoutID

	// elementStates retains per-element state (scroll offsets, ...) across
	// frames, keyed by the string encoding of the element's global id (a
	// GlobalElementID contains a slice and so cannot be a Go map key).
	elementStates map[string]any
	// stateSeen holds the elementStates keys accessed since the last Draw
	// finished; Draw drops the others (GPUI drops element state not
	// accessed during a frame).
	stateSeen map[string]struct{}

	clipStack []ContentMask

	text        TextMeasurer
	textVersion uint64
	// textStyles is the inherited text-style stack of the RequestLayout walk.
	textStyles []TextStyle
	// paintTexts is the paint-time text-override stack (interaction text
	// styles of the ancestors being painted); see paintTextStyle.
	paintTexts []textOverride
	app        *App

	background Rgba
	raster     *Rasterizer
	// deviceScene is scratch space for the scene scaled to device pixels.
	deviceScene Scene

	// currentElementID is the global id of the element currently being
	// prepainted; the walker sets it before each Prepaint call.
	currentElementID GlobalElementID

	// opacity is the product of the Opacity of the elements enclosing the
	// one being painted; Paint* multiply primitive alphas by it.
	opacity float32
	// focusVisible records whether the last key/mouse press came from the
	// keyboard (CSS :focus-visible modality).
	focusVisible bool
	// paintHover caches, per Draw, the hitboxes of the frame being built
	// that lie on the hit path at the pointer (for Hover styles).
	paintHover      []int
	paintHoverValid bool
	// hoverAfterDraw is set while Draw re-runs hover; the frame already
	// painted hover styles at the pointer, so no restyle redraw is needed.
	hoverAfterDraw bool

	// renderedEntities are the entities read during the last draw (Notify
	// on one invalidates the window); nextEntities collects the current one.
	renderedEntities map[EntityID]struct{}
	nextEntities     map[EntityID]struct{}
	// closed is set by Close.
	closed                                  bool
	drawing, frameValid                     bool
	frameTime                               time.Time
	frameSerial                             uint64
	nextFrameRequested, nextFrameWakeIssued bool
	nextFrameAt                             time.Time
}

// Frame is the result of one Draw: a scene plus the hitboxes, listeners and
// input handlers registered during that frame's prepaint. The rendered frame
// is what DispatchEvent reads.
type Frame struct {
	scene          Scene
	hitboxes       BoundsTree
	mouseListeners []MouseListener
	keyNodes       []keyNode
	// rootKey is the index of the root element's key node, or -1.
	rootKey       int
	inputHandlers []PlatformInputHandler
	elementStates map[string]any
	order         uint32
}

// reset empties the frame for a new draw.
func (f *Frame) reset() {
	f.scene.Clear()
	f.hitboxes.Clear()
	f.mouseListeners = f.mouseListeners[:0]
	f.keyNodes = f.keyNodes[:0]
	f.rootKey = -1
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
	OnPinch       func(*PinchEvent, *Window, *App)
	OnHoverStart  func(*Window, *App)
	OnHoverEnd    func(*Window, *App)
	tooltip       string
	KeyContext    string

	// elementID identifies the owning element for retained state lookups
	// (scroll offsets) during scroll dispatch.
	elementID GlobalElementID
	// scrollable marks a scroll container; scrollWheel dispatch clamps and
	// stores an offset for the innermost one under the cursor.
	scrollable bool
	// scrollMax is the container's scrollable extent, captured at prepaint.
	scrollMax Size
	// scrollAxes are the axes the container scrolls with the wheel.
	scrollAxes Axes
	// focusID is the focus id a mouse-down on this element focuses (0 =
	// not focusable).
	focusID FocusID
	// hoverStyle marks an element whose paint depends on hover, so hover
	// changes must redraw.
	hoverStyle  bool
	activeStyle bool
	scrollbar   *scrollbarInput
	textInput   *TextInput
	// parent is the index of the nearest ancestor hitbox, or -1; depth is
	// the number of ancestor hitboxes.
	parent, depth int
}

// keyNode is the per-element keyboard dispatch table recorded during
// prepaint, in tree order (root first).
type keyNode struct {
	KeyContext string
	FocusID    FocusID
	OnKeyDown  func(*KeyDownEvent, *Window, *App)
	OnKeyUp    func(*KeyUpEvent, *Window, *App)
	OnAction   map[ActionKey]func(Action, *Window, *App)
	// tabIndex and tabStop place the node in Tab traversal.
	tabIndex int
	tabStop  bool
	// parent is the index of the nearest ancestor key node, or -1.
	parent int
}

// scrollState is the retained scroll offset of one scroll container.
type scrollState struct {
	offset     Point
	maxW, maxH Pixels
	// handle is the ScrollHandle tracking the container, if any; it holds
	// the authoritative offset.
	handle *ScrollHandle
}

// windowBinder is implemented by platform windows that need the owning
// *Window (the headless window uses it to draw, rasterize and dispatch).
type windowBinder interface{ bindWindow(*Window) }

// newWindow wraps a PlatformWindow. It hooks the platform's frame callback
// when the window implements FrameRequester.
func newWindow(pw PlatformWindow, o WindowOptions, app *App) *Window {
	w := &Window{
		platformWindow: pw,
		options:        o,
		app:            app,
		scale:          1,
		deviceScale:    1,
		uiScale:        1,
		background:     White,
		invalid:        true,
		layoutNodes:    make(map[LayoutID]*LayoutNode),
		nextLayoutID:   1,
		elementStates:  make(map[string]any),
	}
	if app != nil {
		if sh := app.defaultShaper(); sh != nil {
			w.text = sh
		}
	}
	if pw != nil {
		if s := pw.ScaleFactor(); s > 0 {
			w.deviceScale, w.scale = s, s
		}
		if wb, ok := pw.(windowBinder); ok {
			wb.bindWindow(w)
		}
		if fr, ok := pw.(FrameRequester); ok {
			fr.OnRequestFrame(func() {
				if w.app != nil {
					w.app.Tick()
				}
			})
		}
	}
	return w
}

// Bounds returns the window size in logical pixels (the platform size divided
// by UIScale), falling back to the requested options and then to 800x600 when
// the platform reports nothing.
func (w *Window) Bounds() Bounds {
	b := w.platformBounds()
	f := Pixels(w.uiScale)
	return Bounds{Size: Size{W: b.Size.W / f, H: b.Size.H / f}}
}

// platformBounds is the window size in the platform's logical pixels.
func (w *Window) platformBounds() Bounds {
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

// ScaleFactor returns the effective raster scale: the device pixel ratio
// times UIScale. Elements that rasterize at paint time (icons) use it.
func (w *Window) ScaleFactor() float32 { return w.scale }

// UIScale returns the window-level UI zoom set by SetUIScale (default 1).
func (w *Window) UIScale() float32 { return w.uiScale }

// SetUIScale zooms the whole UI by f, clamped to [0.25, 4]: layout runs at the
// platform size divided by f, the scene rasterizes at the device scale times
// f, and DispatchEvent divides pointer positions by f. The window is
// invalidated; a non-finite f is ignored.
func (w *Window) SetUIScale(f float32) {
	if f != f || f > 1e6 || f < -1e6 {
		return
	}
	f = min(max(f, 0.25), 4)
	if f == w.uiScale {
		return
	}
	if w.hasMouse {
		k := Pixels(w.uiScale / f)
		w.mousePosition = Point{X: w.mousePosition.X * k, Y: w.mousePosition.Y * k}
	}
	w.uiScale = f
	w.scale = w.deviceScale * f
	w.Refresh()
}

// Refresh marks the window dirty and asks the platform for a frame. The
// dirty flag is cleared by the next Draw.
func (w *Window) Refresh() {
	if w.closed {
		return
	}
	w.invalid = true
	if !w.drawing {
		w.scheduleFrame()
	}
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
// The hit-testable region is b clipped to the enclosing clip containers.
func (w *Window) InsertHitbox(b Bounds) int {
	hb := b
	if len(w.clipStack) > 0 {
		hb = b.Intersection(w.CurrentClip())
	}
	idx := w.nextFrame.hitboxes.Insert(Hitbox{Bounds: hb, Order: w.nextFrame.order})
	parent := -1
	if n := len(w.hitboxStack); n > 0 {
		parent = w.hitboxStack[n-1]
	}
	w.nextFrame.mouseListeners = append(w.nextFrame.mouseListeners, MouseListener{
		Bounds:    b,
		elementID: w.currentElementID,
		parent:    parent,
		depth:     len(w.hitboxStack),
	})
	w.nextFrame.order++
	return idx
}

// insertKeyNode records an element's key dispatch table for the frame being
// built, linking it to the enclosing key node.
func (w *Window) insertKeyNode(kn keyNode) {
	kn.parent = -1
	if n := len(w.keyStack); n > 0 {
		kn.parent = w.keyStack[n-1]
	}
	w.nextFrame.keyNodes = append(w.nextFrame.keyNodes, kn)
}

// PaintQuad inserts a quad into the frame being built, filling an unset
// content mask with the current clip and applying the element opacity.
func (w *Window) PaintQuad(q Quad) {
	if q.ContentMask.Bounds.Size.W <= 0 || q.ContentMask.Bounds.Size.H <= 0 {
		q.ContentMask = w.CurrentContentMask()
	}
	if w.opacity < 1 {
		q.Fill = q.Fill.Opacity(w.opacity)
		q.BorderColor = q.BorderColor.Opacity(w.opacity)
		q.Gradient.Stops[0].Color = q.Gradient.Stops[0].Color.Opacity(w.opacity)
		q.Gradient.Stops[1].Color = q.Gradient.Stops[1].Color.Opacity(w.opacity)
	}
	w.nextFrame.scene.InsertQuad(q)
}

// PaintShadow inserts a box shadow into the frame being built, filling an
// unset content mask with the current clip and applying the element
// opacity.
func (w *Window) PaintShadow(s Shadow) {
	if s.ContentMask.Bounds.Size.W <= 0 || s.ContentMask.Bounds.Size.H <= 0 {
		s.ContentMask = w.CurrentContentMask()
	}
	if w.opacity < 1 {
		s.Color = s.Color.Opacity(w.opacity)
	}
	w.nextFrame.scene.InsertShadow(s)
}

// PaintUnderline inserts an underline into the frame being built.
func (w *Window) PaintUnderline(u Underline) {
	if u.ContentMask.Bounds.Size.W <= 0 || u.ContentMask.Bounds.Size.H <= 0 {
		u.ContentMask = w.CurrentContentMask()
	}
	if w.opacity < 1 {
		u.Color = u.Color.Opacity(w.opacity)
	}
	w.nextFrame.scene.InsertUnderline(u)
}

// PaintGlyph inserts a glyph sprite into the frame being built.
func (w *Window) PaintGlyph(g GlyphSprite) {
	if g.ContentMask.Bounds.Size.W <= 0 || g.ContentMask.Bounds.Size.H <= 0 {
		g.ContentMask = w.CurrentContentMask()
	}
	if w.opacity < 1 {
		g.Color = g.Color.Opacity(w.opacity)
	}
	w.nextFrame.scene.InsertSprite(g)
}

// PushLayer records a z-order boundary in the scene being built.
func (w *Window) PushLayer() { w.nextFrame.scene.PushLayer() }

// PushClip intersects b with the current clip and pushes the result.
func (w *Window) PushClip(b Bounds) {
	w.PushContentMask(ContentMask{Bounds: b})
}

// PushContentMask intersects m (a rect, optionally rounded) with the current
// content mask and pushes the result; pop it with PopClip.
func (w *Window) PushContentMask(m ContentMask) {
	if len(w.clipStack) == 0 {
		w.clipStack = append(w.clipStack, m.intersect(InfiniteContentMask()))
		return
	}
	w.clipStack = append(w.clipStack, m.intersect(w.clipStack[len(w.clipStack)-1]))
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
		return w.clipStack[n-1].Bounds
	}
	return InfiniteContentMask().Bounds
}

// CurrentContentMask returns the innermost content mask (rect plus corner
// radii), or InfiniteContentMask when nothing has been pushed.
func (w *Window) CurrentContentMask() ContentMask {
	if n := len(w.clipStack); n > 0 {
		return w.clipStack[n-1]
	}
	return InfiniteContentMask()
}

// globalIDKey encodes a GlobalElementID as a comparable string. It is
// required because GlobalElementID contains a slice and cannot be a map key.
func globalIDKey(id GlobalElementID) string {
	var buf [64]byte
	b := buf[:0]
	for _, k := range id.Path {
		b = append(b, '/')
		b = strconv.AppendInt(b, int64(k.Index), 10)
		b = append(b, ':')
		b = append(b, k.Key...)
	}
	return string(b)
}

// ElementState copies the retained state stored for id into out, which must
// be a non-nil pointer. It reports whether state was found and assignable.
func (w *Window) ElementState(id GlobalElementID, out any) bool {
	if out == nil || w.elementStates == nil {
		return false
	}
	key := globalIDKey(id)
	v, ok := w.elementStates[key]
	if !ok {
		return false
	}
	w.touchState(key)
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
	key := globalIDKey(id)
	w.elementStates[key] = v
	w.touchState(key)
}

// touchState marks an elementStates key as accessed, keeping it past the
// next Draw.
func (w *Window) touchState(key string) {
	if w.stateSeen == nil {
		w.stateSeen = make(map[string]struct{})
	}
	w.stateSeen[key] = struct{}{}
}

// pruneElementStates drops the element states not accessed since the last
// prune, releasing the focus id of a dropped implicit focus handle.
func (w *Window) pruneElementStates() {
	for k, v := range w.elementStates {
		if _, ok := w.stateSeen[k]; ok {
			continue
		}
		if h, ok := v.(*FocusHandle); ok && strings.HasSuffix(k, implicitFocusSuffix) && w.app != nil {
			w.app.focusMap.release(h.id)
		}
		delete(w.elementStates, k)
	}
	clear(w.stateSeen)
}

// ScrollOffset returns the scroll offset previously stored for id, or the
// zero point.
func (w *Window) ScrollOffset(id GlobalElementID) Point {
	key := globalIDKey(id)
	st, _ := w.elementStates[key].(scrollState)
	if _, ok := w.elementStates[key]; ok {
		w.touchState(key)
	}
	return st.offset
}

// SetScrollOffset stores a scroll offset for id, clamped to the extent
// captured at the last prepaint of that element. A ScrollHandle tracking
// the container is updated too.
func (w *Window) SetScrollOffset(id GlobalElementID, p Point) {
	key := globalIDKey(id)
	st, _ := w.elementStates[key].(scrollState)
	w.touchState(key)
	st.offset = Point{X: clampf(p.X, 0, st.maxW), Y: clampf(p.Y, 0, st.maxH)}
	if st.handle != nil {
		st.handle.SetOffset(st.offset)
	}
	w.elementStates[key] = st
}

// SetTextMeasurer installs the window's text backend. A nil measurer makes
// text elements fall back to a crude estimator.
func (w *Window) SetTextMeasurer(t TextMeasurer) { w.text = t; w.textVersion++ }

// TextMeasurer returns the window's text backend, or nil when none is
// installed.
func (w *Window) TextMeasurer() TextMeasurer { return w.text }

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

// RegisterLayoutNode registers a caller-built LayoutNode for the frame being
// built and returns its id; custom elements return it from RequestLayout.
// Children must already be linked into n.Children.
func (w *Window) RegisterLayoutNode(n *LayoutNode) LayoutID { return w.registerLayoutNode(n) }

// LayoutNode returns the node registered under id in the frame being built,
// or nil.
func (w *Window) LayoutNode(id LayoutID) *LayoutNode { return w.layoutNodes[id] }

// RequestLayoutWithChildren runs RequestLayout on each non-nil child, builds
// a node with style whose children are the children's nodes, registers it
// and returns its id plus the state to return from RequestLayout (pair it
// with ParentElementChildren so the window walks the children). The
// children inherit the enclosing text style overridden by the fields of
// style.TextStyle that are non-zero and differ from DefaultTextStyle (so
// DefaultStyle() inherits everything); the node records the result.
func (w *Window) RequestLayoutWithChildren(style Style, children []AnyElement, cx *App) (LayoutID, *ChildLayout) {
	style.TextStyle = inheritTextStyle(w.currentTextStyle(), style.TextStyle)
	node := &LayoutNode{Style: style}
	st := &ChildLayout{Node: node}
	w.pushTextStyle(style.TextStyle)
	defer w.popTextStyle()
	for _, c := range children {
		cel := c.Element()
		if cel == nil {
			continue
		}
		cid, cstate := cel.RequestLayout(w, cx)
		st.childIDs = append(st.childIDs, cid)
		st.childStates = append(st.childStates, cstate)
		if cn := w.layoutNodes[cid]; cn != nil {
			node.Children = append(node.Children, cn)
		}
	}
	return w.registerLayoutNode(node), st
}

// inheritTextStyle overlays the fields of ts that are set (non-zero and
// different from DefaultTextStyle) onto inherited.
func inheritTextStyle(inherited, ts TextStyle) TextStyle {
	def := DefaultTextStyle()
	if ts.FontFamily != "" && ts.FontFamily != def.FontFamily {
		inherited.FontFamily = ts.FontFamily
	}
	if ts.FontSize > 0 && ts.FontSize != def.FontSize {
		inherited.FontSize = ts.FontSize
	}
	if ts.FontWeight > 0 && ts.FontWeight != def.FontWeight {
		inherited.FontWeight = ts.FontWeight
	}
	if ts.Color != (Rgba{}) && ts.Color != def.Color {
		inherited.Color = ts.Color
	}
	if ts.LineHeight > 0 && ts.LineHeight != def.LineHeight {
		inherited.LineHeight = ts.LineHeight
	}
	if ts.Strikethrough {
		inherited.Strikethrough = true
	}
	return inherited
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
	if w.closed {
		return
	}
	if w.drawing {
		w.Refresh()
		return
	}
	if cx != nil && cx.ticking {
		if cx.tickDrawn[w] {
			w.Refresh()
			return
		}
		cx.tickDrawn[w] = true
	}
	w.drawing = true
	w.invalid = false
	w.frameSerial++
	if cx != nil && cx.ticking {
		w.frameTime = cx.tickTime
	} else if w.app != nil {
		w.frameTime = w.app.clock.Now()
	}
	w.frameValid = true
	if w.nextFrameRequested && w.frameTime.After(w.nextFrameAt) {
		w.nextFrameRequested = false
		w.nextFrameWakeIssued = false
	}
	defer func() {
		w.drawing = false
		w.scheduleTimedWake()
		if w.invalid && !w.closed {
			w.scheduleFrame()
		}
	}()
	w.nextFrame.reset()
	w.layoutNodes = make(map[LayoutID]*LayoutNode)
	w.nextLayoutID = 1
	w.clipStack = w.clipStack[:0]
	w.hitboxStack = w.hitboxStack[:0]
	w.keyStack = w.keyStack[:0]
	w.opacity = 1
	w.textStyles = w.textStyles[:0]
	w.paintTexts = w.paintTexts[:0]
	w.paintHoverValid = false

	var prevDrawing *Window
	if cx != nil {
		prevDrawing, cx.drawing = cx.drawing, w
		clear(w.nextEntities)
		defer func() { cx.drawing = prevDrawing }()
	}
	defer func() {
		if w.closed {
			w.nextFrame.reset()
			w.clipStack = w.clipStack[:0]
			w.hitboxStack = w.hitboxStack[:0]
			w.keyStack = w.keyStack[:0]
			w.paintTexts = w.paintTexts[:0]
			w.textStyles = w.textStyles[:0]
			w.opacity = 1
		}
	}()
	var el Element
	if w.root != nil {
		el = IntoElementOf(w.root.Render(w, cx))
	}
	if w.closed {
		return
	}
	var rn *renderNode
	if el != nil {
		rootPath := elementIDOf(el)
		gid := GlobalElementID{Path: append([]ElementPathKey(nil), rootPath.Path...)}
		lid, state := el.RequestLayout(w, cx)
		if w.closed {
			return
		}
		rootNode := w.layoutNodes[lid]
		if rootNode != nil {
			b := w.Bounds()
			Layout(rootNode, Definite(float32(b.Width()), float32(b.Height())))
		}
		if w.closed {
			return
		}
		rn = w.buildRenderNode(el, gid, rootNode, state, Point{}, cx)
		w.prepaintTree(rn, cx, true)
	}
	if w.closed {
		return
	}
	w.reconcileInputSelection(&w.nextFrame)
	w.reconcileScrollbarDrag(&w.nextFrame)
	w.retainActive(&w.nextFrame, false)
	w.paintTree(rn, cx)
	w.paintTooltip(cx)
	if w.closed {
		return
	}
	if cx != nil {
		cx.drawing = prevDrawing
		w.renderedEntities, w.nextEntities = w.nextEntities, w.renderedEntities
	}
	w.pruneElementStates()

	w.renderedFrame, w.nextFrame = w.nextFrame, w.renderedFrame
	w.nextFrame.reset()

	if w.platformWindow != nil {
		w.present()
		w.platformWindow.SetInputHandler(w.focusedInputHandler())
	}

	// The tree under a stationary pointer may have changed: re-run hover.
	if w.hasMouse {
		w.hoverAfterDraw = true
		if cx != nil {
			cx.Update(func(cx *App) { w.updateHover(w.mousePosition, cx) })
		} else {
			w.updateHover(w.mousePosition, nil)
		}
		w.updateTooltip(w.mousePosition, false)
		w.hoverAfterDraw = false
	}
}

// present rasterizes the rendered frame at device scale and hands it to the
// platform window.
func (w *Window) present() {
	if s := w.platformWindow.ScaleFactor(); s > 0 {
		w.deviceScale = s
	}
	w.scale = w.deviceScale * w.uiScale
	b := w.platformBounds()
	pw := int(math.Round(float64(b.Width()) * float64(w.deviceScale)))
	ph := int(math.Round(float64(b.Height()) * float64(w.deviceScale)))
	if w.raster == nil || w.raster.w != pw || w.raster.h != ph {
		w.raster = NewRasterizer(pw, ph)
	}
	w.raster.Reset(w.background)
	w.raster.Render(scaleScene(&w.renderedFrame.scene, w.scale, &w.deviceScene))
	w.platformWindow.Present(w.raster.Pixels(), pw, ph)
}

// Close removes the window from its App, releases the entity reference held
// by an EntityView root and asks the platform window to close. It is
// idempotent.
func (w *Window) Close() {
	if w.closed {
		return
	}
	w.closed = true
	w.nextFrameRequested = false
	w.invalid = false
	if w.app != nil {
		remaining := w.app.timers[:0]
		for _, timer := range w.app.timers {
			if timer.owner == w {
				timer.done = true
				timer.callback = nil
			} else {
				remaining = append(remaining, timer)
			}
		}
		w.app.timers = remaining
	}
	w.scheduleTimedWake()
	app := w.app
	if app != nil {
		for i, o := range app.windows {
			if o == w {
				app.windows = append(app.windows[:i:i], app.windows[i+1:]...)
				break
			}
		}
	}
	root := w.root
	w.root = nil
	w.renderedEntities = nil
	if r, ok := root.(viewReleaser); ok && app != nil {
		r.releaseView(app)
	}
	if w.platformWindow != nil {
		w.platformWindow.Quit()
	}
}

// SetBackground sets the color the frame is cleared to before rasterizing
// (default opaque white).
func (w *Window) SetBackground(c Rgba) { w.background = c }

// HitTest hit-tests the last drawn frame at p (logical px) and returns the
// global id of the element owning the topmost hitbox. The id is stable across
// frames while the element keeps its path; compare ids with Equal.
func (w *Window) HitTest(p Point) (GlobalElementID, bool) {
	i := w.renderedFrame.hitboxes.HitTest(p)
	if i < 0 {
		return GlobalElementID{}, false
	}
	id := w.renderedFrame.mouseListeners[i].elementID
	return GlobalElementID{Path: append([]ElementPathKey(nil), id.Path...)}, true
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
	kids, ok := childrenOf(el)
	if !ok || len(kids) == 0 {
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
		var h *ScrollHandle
		if de, ok := el.(divElement); ok {
			h = de.d.scrollHandle
		}
		mw, mh := node.ScrollMax()
		var axes Axes
		if node.Style.Overflow == OverflowScroll {
			axes = node.Style.ScrollAxes
		}
		off := w.syncScroll(gid, h, rn.bounds, node.Style.Padding, Size{W: mw, H: mh}, axes, func(ix int) (Bounds, bool) {
			if ix < 0 || ix >= len(node.Children) || node.Children[ix] == nil {
				return Bounds{}, false
			}
			return node.Children[ix].Rect, true
		})
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
// The first hitbox and key node an element registers become the parents of
// those its descendants register, which is what dispatch paths follow.
func (w *Window) prepaintTree(n *renderNode, cx *App, isRoot bool) {
	if w.closed || n == nil || n.elem == nil {
		return
	}
	if n.layout != nil && !n.layout.Style.Display {
		return
	}
	hb, kb := len(w.nextFrame.mouseListeners), len(w.nextFrame.keyNodes)
	w.currentElementID = n.id
	n.prepaint = n.elem.Prepaint(w, cx, ElementID{Path: n.id.Path}, n.bounds, n.state)
	if w.closed {
		return
	}
	if lc, ok := n.prepaint.(lateChildren); ok {
		n.children = append(n.children, lc.lateChildren()...)
	}
	clipped := false
	if sc, ok := n.prepaint.(paintScoper); ok {
		clipped = sc.clipsChildren()
	}
	if clipped {
		w.PushClip(n.bounds)
	}
	pushedHitbox := len(w.nextFrame.mouseListeners) > hb
	pushedKey := len(w.nextFrame.keyNodes) > kb
	if pushedHitbox {
		w.hitboxStack = append(w.hitboxStack, hb)
	}
	if pushedKey {
		w.keyStack = append(w.keyStack, kb)
		if isRoot {
			w.nextFrame.rootKey = kb
		}
	}
	for _, c := range n.children {
		w.prepaintTree(c, cx, false)
	}
	if sp, ok := n.prepaint.(scrollbarRegistrar); ok && !w.closed {
		sp.registerScrollbar(w)
	}
	if pushedHitbox {
		w.hitboxStack = w.hitboxStack[:len(w.hitboxStack)-1]
	}
	if pushedKey {
		w.keyStack = w.keyStack[:len(w.keyStack)-1]
	}
	if clipped {
		w.PopClip()
	}
}

// paintScoper is implemented by prepaint states of elements that clip their
// children or resolve their paint style (hover/focus) at paint time.
type paintScoper interface {
	// clipsChildren reports whether children are clipped to the bounds.
	clipsChildren() bool
	// paintOpacity resolves the element's paint style and returns its
	// opacity; called once per frame, before Paint.
	paintOpacity(w *Window) float32
}

// scrollbarPainter is implemented by prepaint states of scroll containers;
// paintScrollbar runs after the children, inside the container's clip.
type scrollbarPainter interface {
	paintScrollbar(w *Window)
}

type scrollbarRegistrar interface {
	registerScrollbar(w *Window)
}

// lateChildren is implemented by prepaint states of elements that create
// their children during prepaint (UniformList); they are appended to the
// render node and walked like ordinary children.
type lateChildren interface {
	lateChildren() []*renderNode
}

// isHoveredHitbox reports whether hitbox i of the frame being built is on
// the hit path at the pointer. Valid from paint on (all hitboxes exist).
func (w *Window) isHoveredHitbox(i int) bool {
	if !w.hasMouse || i < 0 {
		return false
	}
	if !w.paintHoverValid {
		w.paintHover = w.paintHover[:0]
		for j := w.nextFrame.hitboxes.HitTest(w.mousePosition); j >= 0; j = w.nextFrame.mouseListeners[j].parent {
			w.paintHover = append(w.paintHover, j)
		}
		w.paintHoverValid = true
	}
	for _, j := range w.paintHover {
		if j == i {
			return true
		}
	}
	return false
}

// paintTree walks the render tree in order: paint the element, then its
// children inside its clip (rounded by its corner radii), then pop the
// clip. The element's own box is not clipped by its overflow, as in GPUI. Clips are pushed here (not in
// prepaint) so the clip stack is balanced per subtree during painting — a
// prepaint-pushed stack would leave every container's clip active while early
// siblings paint, masking the wrong region.
func (w *Window) paintTree(n *renderNode, cx *App) {
	if w.closed || n == nil || n.elem == nil {
		return
	}
	if n.layout != nil && !n.layout.Style.Display {
		return
	}
	opacity := float32(1)
	pushed := false
	if sc, ok := n.prepaint.(paintScoper); ok {
		opacity = sc.paintOpacity(w)
		pushed = sc.clipsChildren()
	}
	prevOpacity := w.opacity
	if opacity < 1 {
		w.opacity *= max(opacity, 0)
	}
	pushedText := false
	if ts, ok := n.prepaint.(paintTextStyler); ok {
		if o, active := ts.paintTextOverride(); o.set != 0 && (active || len(w.paintTexts) > 0) {
			w.paintTexts = append(w.paintTexts, o)
			pushedText = true
		}
	}
	n.elem.Paint(w, cx, ElementID{Path: n.id.Path}, n.bounds, n.state, n.prepaint)
	if pushed {
		m := ContentMask{Bounds: n.bounds}
		if n.layout != nil {
			m.CornerRadii = n.layout.Style.CornerRadii
		}
		w.PushContentMask(m)
	}
	for _, c := range n.children {
		w.paintTree(c, cx)
	}
	if sp, ok := n.prepaint.(scrollbarPainter); ok && !w.closed {
		sp.paintScrollbar(w)
	}
	if pushed {
		w.PopClip()
	}
	if pushedText {
		w.paintTexts = w.paintTexts[:len(w.paintTexts)-1]
	}
	w.opacity = prevOpacity
}

// DispatchEvent routes one platform input event through the rendered frame.
// It runs inside App.Update, so effects queued by handlers (including entity
// releases) flush once, after dispatch.
func (w *Window) DispatchEvent(input PlatformInput, cx *App) {
	if cx == nil {
		cx = w.app
	}
	input = w.toUICoords(input)
	if cx == nil {
		w.dispatchEvent(input, nil)
		return
	}
	cx.Update(func(cx *App) { w.dispatchEvent(input, cx) })
}

// toUICoords divides pointer positions by UIScale: the platform reports
// positions in its own logical pixels, the element tree lives in zoomed ones.
func (w *Window) toUICoords(input PlatformInput) PlatformInput {
	if w.uiScale == 1 {
		return input
	}
	k := Pixels(1 / w.uiScale)
	div := func(p Point) Point { return Point{X: p.X * k, Y: p.Y * k} }
	switch e := input.(type) {
	case MouseDownEvent:
		e.Position = div(e.Position)
		return e
	case MouseUpEvent:
		e.Position = div(e.Position)
		return e
	case MouseMoveEvent:
		e.Position = div(e.Position)
		return e
	case ScrollWheelEvent:
		e.Position = div(e.Position)
		return e
	case PinchEvent:
		e.Position = div(e.Position)
		return e
	case ClickEvent:
		if e.Up != nil {
			up := *e.Up
			up.Position = div(up.Position)
			e.Up = &up
		}
		return e
	}
	return input
}

// dispatchEvent is DispatchEvent's body.
func (w *Window) dispatchEvent(input PlatformInput, cx *App) {
	if _, click := input.(ClickEvent); !click {
		w.scrollbarClick = nil
	}
	switch e := input.(type) {
	case MouseDownEvent:
		w.mousePosition, w.hasMouse = e.Position, true
		w.dismissTooltip()
		if e.Button == MouseLeft {
			w.inputSelection = nil
			w.scrollbarDrag, w.scrollbarPress = nil, false
			if w.beginScrollbar(e.Position) {
				return
			}
			w.beginActive(e.Position)
		} else {
			w.retainActive(&w.renderedFrame, true)
		}
		w.setFocusVisible(false)
		ev := e
		// Auto-focus is skipped once a handler moved focus itself (GPUI's
		// focus-on-mouse-down defers to an explicit focus change).
		focused := false
		start := w.focus
		w.dispatchMouse(e.Position, cx, func(l *MouseListener) {
			if l.OnMouseDown != nil {
				l.OnMouseDown(&ev, w, cx)
			}
			if w.focus != start {
				focused = true
			}
			if !focused && l.focusID != 0 {
				focused = true
				w.focusID(l.focusID)
			}
		})
	case MouseUpEvent:
		w.mousePosition, w.hasMouse = e.Position, true
		if e.Button == MouseLeft && w.scrollbarPress {
			w.moveScrollbar(e.Position)
			w.scrollbarDrag, w.scrollbarPress = nil, false
			up := e
			w.scrollbarClick = &up
			return
		}
		if e.Button == MouseLeft && w.inputSelection != nil {
			w.moveInputSelection(e.Position)
			w.inputSelection = nil
			if len(w.active) > 0 {
				clear(w.active)
				w.Refresh()
			}
			return
		}
		if e.Button == MouseLeft {
			if len(w.active) > 0 {
				clear(w.active)
				w.Refresh()
			}
		} else {
			w.retainActive(&w.renderedFrame, true)
		}
		ev := e
		w.dispatchMouse(e.Position, cx, func(l *MouseListener) {
			if l.OnMouseUp != nil {
				l.OnMouseUp(&ev, w, cx)
			}
		})
	case MouseMoveEvent:
		w.mousePosition, w.hasMouse = e.Position, true
		if w.scrollbarPress {
			w.moveScrollbar(e.Position)
			return
		}
		if w.inputSelection != nil {
			w.moveInputSelection(e.Position)
			w.retainActive(&w.renderedFrame, true)
			return
		}
		w.retainActive(&w.renderedFrame, true)
		w.updateHover(e.Position, cx)
		w.updateTooltip(e.Position, true)
		ev := e
		w.dispatchMouse(e.Position, cx, func(l *MouseListener) {
			if l.OnMouseMove != nil {
				l.OnMouseMove(&ev, w, cx)
			}
		})
	case ClickEvent:
		consumed := e.Up != nil && w.scrollbarClick != nil && *e.Up == *w.scrollbarClick
		w.scrollbarClick = nil
		if consumed {
			return
		}
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
		w.dismissTooltip()
		w.dispatchScroll(e, cx)
	case PinchEvent:
		w.dismissTooltip()
		w.dispatchPinch(e, cx)
	case KeyDownEvent:
		w.dismissTooltip()
		w.setFocusVisible(true)
		w.dispatchKeyDown(e, cx)
	case KeyUpEvent:
		w.dispatchKeyUp(e, cx)
	case ModifiersChangedEvent:
		// The POC has no modifier-state listeners.
	}
}

// hitPath returns the rendered frame's hitbox indices under pos, innermost
// first: the topmost hitbox followed by its ancestor chain.
func (w *Window) hitPath(pos Point) []int {
	idx := w.renderedFrame.hitboxes.HitTest(pos)
	var path []int
	for i := idx; i >= 0; i = w.renderedFrame.mouseListeners[i].parent {
		path = append(path, i)
	}
	return path
}

func (w *Window) beginActive(pos Point) {
	changed := len(w.active) > 0
	clear(w.active)
	for _, i := range w.hitPath(pos) {
		l := &w.renderedFrame.mouseListeners[i]
		if !l.activeStyle {
			continue
		}
		if w.active == nil {
			w.active = make(map[string]struct{})
		}
		w.active[globalIDKey(l.elementID)] = struct{}{}
		changed = true
	}
	if changed {
		w.Refresh()
	}
}

func (w *Window) isActiveElement(id GlobalElementID) bool {
	if len(w.active) == 0 {
		return false
	}
	_, ok := w.active[globalIDKey(id)]
	return ok
}

func (w *Window) retainActive(frame *Frame, refresh bool) {
	if len(w.active) == 0 {
		return
	}
	changed := false
	for key := range w.active {
		found := false
		for i := frame.hitboxes.HitTest(w.mousePosition); i >= 0; i = frame.mouseListeners[i].parent {
			l := &frame.mouseListeners[i]
			if l.activeStyle && globalIDKey(l.elementID) == key {
				found = true
				break
			}
		}
		if !found {
			delete(w.active, key)
			changed = true
		}
	}
	if changed && refresh {
		w.Refresh()
	}
}

// dispatchMouse runs call over the hit path at pos, innermost first, each
// listener once, until StopPropagation. All element mouse listeners are
// bubble-phase (GPUI's capture_* listeners are not ported).
func (w *Window) dispatchMouse(pos Point, cx *App, call func(*MouseListener)) {
	w.propagateEvent = true
	for _, i := range w.hitPath(pos) {
		if !w.propagateEvent {
			return
		}
		call(&w.renderedFrame.mouseListeners[i])
	}
}

// updateHover diffs the hover-aware elements on the hit path at pos against
// the previously hovered set. Ends fire first, deepest first; then starts,
// root first. Callbacks come from the current rendered frame, so an element
// that is no longer drawn gets no OnHoverEnd.
func (w *Window) updateHover(pos Point, cx *App) {
	ls := w.renderedFrame.mouseListeners
	path := w.hitPath(pos)
	now := make(map[string]int, len(path))
	for _, i := range path {
		if ls[i].OnHoverStart != nil || ls[i].OnHoverEnd != nil || ls[i].hoverStyle {
			now[globalIDKey(ls[i].elementID)] = i
		}
	}
	restyle := false

	type gone struct {
		key   string
		depth int
	}
	var ended []gone
	for k, d := range w.hovered {
		if _, ok := now[k]; !ok {
			ended = append(ended, gone{k, d})
		}
	}
	sort.Slice(ended, func(a, b int) bool {
		if ended[a].depth != ended[b].depth {
			return ended[a].depth > ended[b].depth
		}
		return ended[a].key < ended[b].key
	})
	for _, g := range ended {
		delete(w.hovered, g.key)
		for i := range ls {
			if (ls[i].OnHoverEnd != nil || ls[i].hoverStyle) && globalIDKey(ls[i].elementID) == g.key {
				restyle = restyle || ls[i].hoverStyle
				if ls[i].OnHoverEnd != nil {
					ls[i].OnHoverEnd(w, cx)
				}
				break
			}
		}
	}

	for j := len(path) - 1; j >= 0; j-- {
		l := &ls[path[j]]
		k := globalIDKey(l.elementID)
		if _, ok := now[k]; !ok {
			continue
		}
		if _, ok := w.hovered[k]; ok {
			continue
		}
		if w.hovered == nil {
			w.hovered = make(map[string]int)
		}
		w.hovered[k] = l.depth
		restyle = restyle || l.hoverStyle
		if l.OnHoverStart != nil {
			l.OnHoverStart(w, cx)
		}
	}
	if restyle && !w.hoverAfterDraw {
		w.Refresh()
	}
}

// dispatchScroll runs OnScrollWheel over the hit path, then, unless a
// handler stopped propagation, offers the delta to the scroll containers
// under the cursor, innermost first: the first whose offset changes takes it
// and requests a redraw.
func (w *Window) dispatchScroll(e ScrollWheelEvent, cx *App) {
	ev := e
	w.mousePosition, w.hasMouse = e.Position, true
	w.retainActive(&w.renderedFrame, true)
	w.dispatchMouse(e.Position, cx, func(l *MouseListener) {
		if l.OnScrollWheel != nil {
			l.OnScrollWheel(&ev, w, cx)
		}
	})
	if !w.propagateEvent {
		return
	}
	for _, i := range w.hitPath(e.Position) {
		l := &w.renderedFrame.mouseListeners[i]
		if !l.scrollable {
			continue
		}
		if w.applyWheel(l, e) {
			w.Refresh()
			return
		}
	}
}

// dispatchPinch runs OnPinch over the hit path at the event position,
// innermost first, until a handler calls StopPropagation.
func (w *Window) dispatchPinch(e PinchEvent, cx *App) {
	ev := e
	w.mousePosition, w.hasMouse = e.Position, true
	w.dispatchMouse(e.Position, cx, func(l *MouseListener) {
		if l.OnPinch != nil {
			l.OnPinch(&ev, w, cx)
		}
	})
}

// focusID moves focus to id if it is still alive.
func (w *Window) focusID(id FocusID) {
	if id == 0 || w.focus == id {
		return
	}
	if w.app != nil && !w.app.focusMap.alive(id) {
		return
	}
	w.focus = id
	w.Refresh()
}

// setFocusVisible records the input modality, redrawing when it flips while
// something is focused (FocusVisible styles depend on it).
func (w *Window) setFocusVisible(v bool) {
	if w.focusVisible == v {
		return
	}
	w.focusVisible = v
	if w.focus != 0 {
		w.Refresh()
	}
}

// FocusVisible reports whether the last key or mouse press came from the
// keyboard; FocusVisible styles apply only then (CSS :focus-visible).
func (w *Window) FocusVisible() bool { return w.focusVisible }

// tabStops returns the rendered frame's tab stops in traversal order: by
// tab index, ties in tree order, each focus id once.
func (w *Window) tabStops() []FocusID {
	type stop struct {
		id  FocusID
		idx int
	}
	var stops []stop
	seen := map[FocusID]bool{}
	for _, n := range w.renderedFrame.keyNodes {
		if n.FocusID == 0 || !n.tabStop || seen[n.FocusID] {
			continue
		}
		seen[n.FocusID] = true
		stops = append(stops, stop{n.FocusID, n.tabIndex})
	}
	sort.SliceStable(stops, func(a, b int) bool { return stops[a].idx < stops[b].idx })
	ids := make([]FocusID, len(stops))
	for i, s := range stops {
		ids[i] = s.id
	}
	return ids
}

// FocusNext moves focus to the next tab stop of the rendered frame (by tab
// index, ties in tree order), wrapping; with no focused stop it focuses the
// first. Tab is not bound by default: bind it to an action that calls this.
func (w *Window) FocusNext() { w.focusStep(1) }

// FocusPrev moves focus to the previous tab stop, wrapping; with no focused
// stop it focuses the last.
func (w *Window) FocusPrev() { w.focusStep(-1) }

func (w *Window) focusStep(d int) {
	ids := w.tabStops()
	n := len(ids)
	if n == 0 {
		return
	}
	cur := slices.Index(ids, w.focus)
	var next int
	switch {
	case cur < 0 && d > 0:
		next = 0
	case cur < 0:
		next = n - 1
	default:
		next = ((cur+d)%n + n) % n
	}
	w.focusID(ids[next])
}

// keyPath returns the key nodes a key event traverses, root first; the last
// entry is the target. It is the ancestor chain of the focused key node, or,
// when nothing focused is in the frame, the root element's key node alone
// (GPUI dispatches from the root).
func (w *Window) keyPath() []keyNode {
	nodes := w.renderedFrame.keyNodes
	target := -1
	if w.focus != 0 {
		for i := range nodes {
			if nodes[i].FocusID == w.focus {
				target = i
				break
			}
		}
	}
	if target < 0 {
		if r := w.renderedFrame.rootKey; r >= 0 && r < len(nodes) {
			return nodes[r : r+1]
		}
		return nil
	}
	var rev []keyNode
	for i := target; i >= 0; i = nodes[i].parent {
		rev = append(rev, nodes[i])
	}
	slices.Reverse(rev)
	return rev
}

// dispatchKeyDown routes a key press. Character input for a focused text
// field first offers the keystroke to the keymap (GPUI: bindings win) and
// goes to the field's input handler only if no OnAction handler consumed
// the matched action. Any other press runs OnKeyDown over the focus path
// (target first, bubble-phase) and then consults the keymap, dispatching
// any matched action to OnAction handlers along the same path.
func (w *Window) dispatchKeyDown(e KeyDownEvent, cx *App) {
	ev := e
	if h := w.focusedInputHandler(); h != nil && h.AcceptsTextInput() &&
		e.PreferCharacterInput && e.Keystroke.KeyChar != nil {
		if w.dispatchBinding(e.Keystroke, w.keyPath(), cx) {
			return
		}
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
	for i := len(path) - 1; i >= 0 && w.propagateEvent; i-- {
		if path[i].OnKeyDown != nil {
			path[i].OnKeyDown(&ev, w, cx)
		}
	}
	if !w.propagateEvent {
		return
	}
	w.dispatchBinding(e.Keystroke, path, cx)
}

// dispatchBinding matches ks against the keymap with the path's key
// contexts and delivers the action to OnAction handlers along the path,
// target first, until StopPropagation. It reports whether a handler ran.
func (w *Window) dispatchBinding(ks Keystroke, path []keyNode, cx *App) bool {
	if cx == nil || len(path) == 0 {
		return false
	}
	ctxStack := make([]string, 0, len(path))
	for _, n := range path {
		if n.KeyContext != "" {
			ctxStack = append(ctxStack, n.KeyContext)
		}
	}
	action := cx.Keymap().MatchKeystroke(ks, ctxStack)
	if action == nil {
		return false
	}
	key := ActionKeyOf(action)
	handled := false
	w.propagateEvent = true
	for i := len(path) - 1; i >= 0 && w.propagateEvent; i-- {
		if f, ok := path[i].OnAction[key]; ok {
			handled = true
			f(action, w, cx)
		}
	}
	return handled
}

// dispatchKeyUp runs OnKeyUp over the focus path, target first.
func (w *Window) dispatchKeyUp(e KeyUpEvent, cx *App) {
	ev := e
	path := w.keyPath()
	w.propagateEvent = true
	for i := len(path) - 1; i >= 0 && w.propagateEvent; i-- {
		if path[i].OnKeyUp != nil {
			path[i].OnKeyUp(&ev, w, cx)
		}
	}
}
