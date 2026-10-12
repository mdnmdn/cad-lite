package gpui

import "reflect"

// Div is the generic box element: a styled, optionally-flex container with
// optional mouse/key listeners, focus tracking and clipping.
//
// Chaining note: *Div embeds StyledMixin, whose methods return *StyledMixin.
// To keep a *Div-typed chain, Div forwards the most common style calls
// (SizeWH, Flex, Bg, ...) back to *Div; prefer those, or keep the result in a
// *Div variable and interleave carefully.
//
// Element-interface note: the contract requires both the fluent builder
// Div.ID(string) *Div and Element.ID() (ElementID, bool). Go has no
// overloading, so *Div exposes the builder and satisfies Element through a
// thin adapter (IntoElement) that supplies ID() and delegates the rest.
type Div struct {
	StyledMixin

	id       string
	children []AnyElement

	onMouseDown   func(*MouseDownEvent, *Window, *App)
	onMouseUp     func(*MouseUpEvent, *Window, *App)
	onMouseMove   func(*MouseMoveEvent, *Window, *App)
	onClick       func(*ClickEvent, *Window, *App)
	onScrollWheel func(*ScrollWheelEvent, *Window, *App)
	onKeyDown     func(*KeyDownEvent, *Window, *App)
	onKeyUp       func(*KeyUpEvent, *Window, *App)
	onHoverStart  func(*Window, *App)
	onHoverEnd    func(*Window, *App)
	onAction      map[reflect.Type]func(Action, *Window, *App)

	focusHandle *FocusHandle
	tracksFocus bool
	keyContext  string
	tabIndex    int
	hasTabIndex bool
	tabStop     *bool

	hoverStyle        *StyleRefinement
	focusStyle        *StyleRefinement
	focusVisibleStyle *StyleRefinement

	scrollHandle  *ScrollHandle
	showScrollbar bool
}

// NewDiv returns a new Div seeded with DefaultStyle, so a bare NewDiv() is a
// visible, auto-sized block.
// NewDiv returns a new Div. It is named NewDiv rather than Div because Go
// does not allow a type and a function to share a name.
func NewDiv() *Div {
	base := DefaultStyle()
	d := &Div{StyledMixin: NewStyledMixin()}
	d.style = StyleRefinement{
		Display:      &base.Display,
		Direction:    &base.Direction,
		Width:        &base.Width,
		Height:       &base.Height,
		MinWidth:     &base.MinWidth,
		MinHeight:    &base.MinHeight,
		MaxWidth:     &base.MaxWidth,
		MaxHeight:    &base.MaxHeight,
		Margin:       &base.Margin,
		Padding:      &base.Padding,
		Gap:          &base.Gap,
		FlexGrow:     &base.FlexGrow,
		FlexShrink:   &base.FlexShrink,
		Justify:      &base.Justify,
		AlignItems:   &base.AlignItems,
		AlignSelf:    &base.AlignSelf,
		Overflow:     &base.Overflow,
		BorderWidths: &base.BorderWidths,
		BorderColor:  &base.BorderColor,
		CornerRadii:  &base.CornerRadii,
		Background:   &base.Background,
		Opacity:      &base.Opacity,
		ScrollAxes:   &base.ScrollAxes,
		TextStyle:    &base.TextStyle,
	}
	return d
}

// ID sets the element's local id, used to key retained state across frames.
func (d *Div) ID(id string) *Div {
	d.id = id
	return d
}

// Child appends child elements.
func (d *Div) Child(children ...IntoElement) *Div {
	for _, c := range children {
		d.children = append(d.children, NewAnyElement(c))
	}
	return d
}

// Children appends child elements; it is an alias of Child kept for parity
// with the contract's ParentElement shape.
func (d *Div) Children(children ...IntoElement) *Div { return d.Child(children...) }

// OnMouseDown registers a button-press listener.
func (d *Div) OnMouseDown(f func(*MouseDownEvent, *Window, *App)) *Div {
	d.onMouseDown = f
	return d
}

// OnMouseUp registers a button-release listener.
func (d *Div) OnMouseUp(f func(*MouseUpEvent, *Window, *App)) *Div {
	d.onMouseUp = f
	return d
}

// OnMouseMove registers a pointer-motion listener.
func (d *Div) OnMouseMove(f func(*MouseMoveEvent, *Window, *App)) *Div {
	d.onMouseMove = f
	return d
}

// OnClick registers a completed-click (press+release) listener.
func (d *Div) OnClick(f func(*ClickEvent, *Window, *App)) *Div {
	d.onClick = f
	return d
}

// OnScrollWheel registers a scroll listener.
func (d *Div) OnScrollWheel(f func(*ScrollWheelEvent, *Window, *App)) *Div {
	d.onScrollWheel = f
	return d
}

// OnKeyDown registers a key-press listener.
func (d *Div) OnKeyDown(f func(*KeyDownEvent, *Window, *App)) *Div {
	d.onKeyDown = f
	return d
}

// OnKeyUp registers a key-release listener.
func (d *Div) OnKeyUp(f func(*KeyUpEvent, *Window, *App)) *Div {
	d.onKeyUp = f
	return d
}

// OnHoverStart registers a pointer-enter listener, fired from mouse-move
// dispatch when the pointer enters the element (or one of its descendants).
func (d *Div) OnHoverStart(f func(*Window, *App)) *Div {
	d.onHoverStart = f
	return d
}

// OnHoverEnd registers a pointer-leave listener; see OnHoverStart.
func (d *Div) OnHoverEnd(f func(*Window, *App)) *Div {
	d.onHoverEnd = f
	return d
}

// OnAction registers a handler for an action dispatched through the keymap.
// Handlers are keyed by the action's concrete reflect.Type, so two actions
// built with NewAction share one type and cannot be told apart; define a
// distinct type per action when they must differ.
func (d *Div) OnAction(action Action, f func(Action, *Window, *App)) *Div {
	if action == nil {
		return d
	}
	if d.onAction == nil {
		d.onAction = make(map[reflect.Type]func(Action, *Window, *App))
	}
	d.onAction[reflect.TypeOf(action)] = f
	return d
}

// TrackFocus makes the div the keyboard-focus target for h; when the div is
// prepainted, its key node records h's FocusID for focus-path dispatch.
func (d *Div) TrackFocus(h *FocusHandle) *Div {
	d.focusHandle = h
	d.tracksFocus = h != nil
	return d
}

// KeyContext names the div's key context, visible to keymap context
// predicates when the div is on the focus path.
func (d *Div) KeyContext(ctx string) *Div {
	d.keyContext = ctx
	return d
}

// TabIndex sets the div's position in tab-order traversal and makes it a
// tab stop. Without TrackFocus the div gets an implicit focus handle,
// retained per element id.
func (d *Div) TabIndex(i int) *Div {
	d.tabIndex = i
	d.hasTabIndex = true
	return d
}

// TabStop sets whether Tab traversal may stop at the div, overriding the
// tracked handle's own setting.
func (d *Div) TabStop(b bool) *Div {
	d.tabStop = &b
	return d
}

// Hover sets a style refinement applied while the pointer is over the div
// (GPUI's .hover). f receives an empty StyledMixin and returns it. Like
// every interaction style it is applied at paint, so layout fields set in
// it have no effect.
func (d *Div) Hover(f func(*StyledMixin) *StyledMixin) *Div {
	d.hoverStyle = d.interactionStyle(f)
	return d
}

// Focus sets a style refinement applied while the div's focus handle is
// focused (GPUI's .focus).
func (d *Div) Focus(f func(*StyledMixin) *StyledMixin) *Div {
	d.focusStyle = d.interactionStyle(f)
	return d
}

// FocusVisible sets a style refinement applied while the div is focused
// and the last input was the keyboard (GPUI's .focus_visible, CSS
// :focus-visible). It applies after Focus and before Hover.
func (d *Div) FocusVisible(f func(*StyledMixin) *StyledMixin) *Div {
	d.focusVisibleStyle = d.interactionStyle(f)
	return d
}

// interactionStyle runs f on a fresh mixin whose text style starts from
// the div's, so text tweaks keep the other text properties.
func (d *Div) interactionStyle(f func(*StyledMixin) *StyledMixin) *StyleRefinement {
	if f == nil {
		return nil
	}
	m := &StyledMixin{}
	if d.style.TextStyle != nil {
		ts := *d.style.TextStyle
		m.style.TextStyle = &ts
	}
	if r := f(m); r != nil {
		m = r
	}
	return &m.style
}

// TrackScroll binds h to this scroll container: h reports and controls the
// container's offset.
func (d *Div) TrackScroll(h *ScrollHandle) *Div {
	d.scrollHandle = h
	return d
}

// ShowScrollbar opts a scroll container into a thin overlay scrollbar,
// painted when its content overflows on a scrolling axis. GPUI divs paint
// none by default; neither does this port.
func (d *Div) ShowScrollbar() *Div {
	d.showScrollbar = true
	return d
}

// resolvedStyle merges the div's refinement onto DefaultStyle.
func (d *Div) resolvedStyle() Style { return d.ResolvedStyle(DefaultStyle()) }

// elementChildren exposes the child elements to the window's tree walk.
func (d *Div) elementChildren() []AnyElement { return d.children }

// IntoElement satisfies IntoElement via the divElement adapter.
func (d *Div) IntoElement() Element { return divElement{d: d} }

// --- style-forwarding helpers, so a *Div-typed chain stays *Div -----------

// SizeFull fills both axes (see StyledMixin.SizeFull for the POC's 100%
// stand-in).
func (d *Div) SizeFull() *Div { d.StyledMixin.SizeFull(); return d }

// SizeWH sets both axes to definite pixel lengths.
func (d *Div) SizeWH(w, h float32) *Div { d.StyledMixin.SizeWH(w, h); return d }

// SizeW sets the width to a definite pixel length.
func (d *Div) SizeW(w float32) *Div { d.StyledMixin.SizeW(w); return d }

// SizeH sets the height to a definite pixel length.
func (d *Div) SizeH(h float32) *Div { d.StyledMixin.SizeH(h); return d }

// Flex turns the div into a vertical flex container.
func (d *Div) Flex() *Div { d.StyledMixin.Flex(); return d }

// FlexRow makes the div a horizontal flex container.
func (d *Div) FlexRow() *Div { d.StyledMixin.FlexRow(); return d }

// FlexCol makes the div a vertical flex container.
func (d *Div) FlexCol() *Div { d.StyledMixin.FlexCol(); return d }

// Flex1 grows and shrinks to consume the available main-axis space.
func (d *Div) Flex1() *Div { d.StyledMixin.Flex1(); return d }

// Gap sets the main-axis gutter between adjacent children.
func (d *Div) Gap(v float32) *Div { d.StyledMixin.Gap(v); return d }

// P sets padding on all four edges.
func (d *Div) P(v float32) *Div { d.StyledMixin.P(v); return d }

// Bg sets the background fill: an Rgba, an Hsla or a Background such as
// LinearGradient(...).
func (d *Div) Bg(f Fill) *Div { d.StyledMixin.Bg(f); return d }

// Opacity sets the opacity of the div and its subtree.
func (d *Div) Opacity(f float32) *Div { d.StyledMixin.Opacity(f); return d }

// Shadow replaces the div's box shadows.
func (d *Div) Shadow(shadows ...BoxShadow) *Div { d.StyledMixin.Shadow(shadows...); return d }

// ShadowNone removes every box shadow.
func (d *Div) ShadowNone() *Div { d.StyledMixin.ShadowNone(); return d }

// Shadow2xs applies GPUI's shadow_2xs preset.
func (d *Div) Shadow2xs() *Div { d.StyledMixin.Shadow2xs(); return d }

// ShadowXs applies GPUI's shadow_xs preset.
func (d *Div) ShadowXs() *Div { d.StyledMixin.ShadowXs(); return d }

// ShadowSm applies GPUI's shadow_sm preset.
func (d *Div) ShadowSm() *Div { d.StyledMixin.ShadowSm(); return d }

// ShadowMd applies GPUI's shadow_md preset.
func (d *Div) ShadowMd() *Div { d.StyledMixin.ShadowMd(); return d }

// ShadowLg applies GPUI's shadow_lg preset.
func (d *Div) ShadowLg() *Div { d.StyledMixin.ShadowLg(); return d }

// ShadowXl applies GPUI's shadow_xl preset.
func (d *Div) ShadowXl() *Div { d.StyledMixin.ShadowXl(); return d }

// Shadow2xl applies GPUI's shadow_2xl preset.
func (d *Div) Shadow2xl() *Div { d.StyledMixin.Shadow2xl(); return d }

// OverflowXScroll makes the div scroll horizontally (the other axis clips).
func (d *Div) OverflowXScroll() *Div { d.StyledMixin.OverflowXScroll(); return d }

// OverflowYScroll makes the div scroll vertically (the other axis clips).
func (d *Div) OverflowYScroll() *Div { d.StyledMixin.OverflowYScroll(); return d }

// TextColor sets the text color inherited by descendants.
func (d *Div) TextColor(c Rgba) *Div { d.StyledMixin.TextColor(c); return d }

// TextSize sets the font size inherited by descendants.
func (d *Div) TextSize(px float32) *Div { d.StyledMixin.TextSize(px); return d }

// Rounded sets all four corner radii.
func (d *Div) Rounded(px float32) *Div { d.StyledMixin.Rounded(px); return d }

// Border sets a uniform border width and color.
func (d *Div) Border(w float32, c Rgba) *Div { d.StyledMixin.Border(w, c); return d }

// ItemsCenter centers children on the cross axis.
func (d *Div) ItemsCenter() *Div { d.StyledMixin.ItemsCenter(); return d }

// ItemsStart aligns children to the start of the cross axis.
func (d *Div) ItemsStart() *Div { d.StyledMixin.ItemsStart(); return d }

// JustifyBetween spreads children with equal space between them.
func (d *Div) JustifyBetween() *Div { d.StyledMixin.JustifyBetween(); return d }

// OverflowScroll clips painting and makes the div scrollable by input.
func (d *Div) OverflowScroll() *Div { d.StyledMixin.OverflowScroll(); return d }

// OverflowHidden clips painting without input-driven scrolling.
func (d *Div) OverflowHidden() *Div { d.StyledMixin.OverflowHidden(); return d }

// MinW sets a minimum width.
func (d *Div) MinW(v float32) *Div { d.StyledMixin.MinW(v); return d }

// MinH sets a minimum height.
func (d *Div) MinH(v float32) *Div { d.StyledMixin.MinH(v); return d }

// MaxW sets a maximum width.
func (d *Div) MaxW(v float32) *Div { d.StyledMixin.MaxW(v); return d }

// MaxH sets a maximum height.
func (d *Div) MaxH(v float32) *Div { d.StyledMixin.MaxH(v); return d }

// Hidden removes the div from layout and paint (Display:none).
func (d *Div) Hidden() *Div { d.StyledMixin.Hidden(); return d }

// Visible undoes Hidden.
func (d *Div) Visible() *Div { d.StyledMixin.Visible(); return d }

// --- Element implementation ----------------------------------------------

// divElement adapts *Div to Element. It exists because *Div carries the
// fluent builder ID(string) *Div, which collides with Element's
// ID() (ElementID, bool); Go cannot declare both on one type. Everything
// else is delegated straight to the Div.
type divElement struct{ d *Div }

// ID returns the div's local element id (empty when the div has none).
func (e divElement) ID() (ElementID, bool) {
	if e.d.id == "" {
		return ElementID{}, false
	}
	return ElementID{Path: []ElementPathKey{{Key: e.d.id}}}, true
}

// RequestLayout delegates to the Div.
func (e divElement) RequestLayout(w *Window, cx *App) (LayoutID, any) {
	return e.d.requestLayout(w, cx)
}

// Prepaint delegates to the Div.
func (e divElement) Prepaint(w *Window, cx *App, id ElementID, bounds Bounds, layoutState any) any {
	return e.d.prepaint(w, cx, bounds, layoutState)
}

// Paint delegates to the Div.
func (e divElement) Paint(w *Window, cx *App, id ElementID, bounds Bounds, layoutState, prepaintState any) {
	e.d.paint(w, bounds, prepaintState)
}

// elementChildren exposes the children to the window's tree walk; without it
// hasChildren is not satisfied and the walk would stop at every div.
func (e divElement) elementChildren() []AnyElement { return e.d.elementChildren() }

// divLayout is Div's RequestLayout state: the node plus one LayoutID and one
// RequestLayout state per child, so the window's walk can pair element
// children with layout nodes without re-running RequestLayout.
type divLayout struct {
	node        *LayoutNode
	childIDs    []LayoutID
	childStates []any
}

// childLayoutIDs satisfies layoutChildIDs.
func (s *divLayout) childLayoutIDs() []LayoutID { return s.childIDs }

// childLayoutStates satisfies layoutChildStates.
func (s *divLayout) childLayoutStates() []any { return s.childStates }

// requestLayout builds the div's LayoutNode, recursing into children and
// registering every node with the window.
func (d *Div) requestLayout(w *Window, cx *App) (LayoutID, any) {
	node := &LayoutNode{Style: d.resolvedStyle()}
	st := &divLayout{node: node}
	for _, c := range d.children {
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

// divPrepaint is Div's per-frame prepaint state.
type divPrepaint struct {
	d        *Div
	bounds   Bounds
	globalID GlobalElementID
	hitbox   int
	clipped  bool
	focusID  FocusID
	// scroll is set for OverflowScroll containers (overlay scrollbar).
	scroll *scrollbarInfo
	// style is the paint style: base plus the active interaction styles,
	// resolved by paintOpacity (resolved reports that it ran).
	style    Style
	resolved bool
}

func (st *divPrepaint) clipsChildren() bool { return st.clipped }

// paintOpacity resolves the paint style (focus, focus-visible and hover
// refinements over the base style) and returns its opacity.
func (st *divPrepaint) paintOpacity(w *Window) float32 {
	d := st.d
	focused := st.focusID != 0 && w.focus == st.focusID
	hovered := d.hoverStyle != nil && w.isHoveredHitbox(st.hitbox)
	if (focused && (d.focusStyle != nil || d.focusVisibleStyle != nil)) || hovered {
		r := d.style
		if focused {
			r.merge(d.focusStyle)
			if w.focusVisible {
				r.merge(d.focusVisibleStyle)
			}
		}
		if hovered {
			r.merge(d.hoverStyle)
		}
		st.style = r.Refine(DefaultStyle())
	} else {
		st.style = d.resolvedStyle()
	}
	st.resolved = true
	return st.style.Opacity
}

func (st *divPrepaint) paintScrollbar(w *Window) {
	if st.scroll != nil {
		st.scroll.paint(w)
	}
}

// implicitFocusHandle returns the focus handle a TabIndex div without
// TrackFocus owns, retained per element id.
func (d *Div) implicitFocusHandle(w *Window, cx *App, gid GlobalElementID) *FocusHandle {
	if cx == nil {
		return nil
	}
	key := globalIDKey(gid) + "#focus"
	if h, ok := w.elementStates[key].(*FocusHandle); ok && h.IsValid() && cx.focusMap.alive(h.id) {
		return h
	}
	h := cx.NewFocusHandle()
	w.elementStates[key] = h
	return h
}

// prepaint registers the div's hitbox, mouse/key listeners, scroll state and
// clip flag for the frame being built.
func (d *Div) prepaint(w *Window, cx *App, bounds Bounds, layoutState any) any {
	style := d.resolvedStyle()
	gid := w.currentElementID

	st := &divPrepaint{d: d, bounds: bounds, globalID: gid}
	st.hitbox = w.InsertHitbox(bounds)

	fh := d.focusHandle
	if fh == nil && d.hasTabIndex {
		fh = d.implicitFocusHandle(w, cx, gid)
	}
	var focusID FocusID
	if fh != nil && fh.IsValid() {
		focusID = fh.ID()
	}
	st.focusID = focusID

	ml := &w.nextFrame.mouseListeners[st.hitbox]
	ml.KeyContext = d.keyContext
	ml.elementID = gid
	ml.OnMouseDown = d.onMouseDown
	ml.OnMouseUp = d.onMouseUp
	ml.OnMouseMove = d.onMouseMove
	ml.OnClick = d.onClick
	ml.OnScrollWheel = d.onScrollWheel
	ml.OnHoverStart = d.onHoverStart
	ml.OnHoverEnd = d.onHoverEnd
	ml.focusID = focusID
	ml.hoverStyle = d.hoverStyle != nil

	if style.Overflow != OverflowVisible {
		st.clipped = true
		ml.scrollable = style.Overflow == OverflowScroll
		ml.scrollAxes = style.ScrollAxes
		var node *LayoutNode
		if ls, ok := layoutState.(*divLayout); ok {
			node = ls.node
		}
		if node != nil {
			mw, mh := node.ScrollMax()
			ml.scrollMax = Size{W: mw, H: mh}
		}
		if ml.scrollable && d.showScrollbar {
			sst, _ := w.elementStates[globalIDKey(gid)].(scrollState)
			st.scroll = &scrollbarInfo{bounds: bounds, offset: sst.offset, max: ml.scrollMax, axes: style.ScrollAxes}
		}
	}

	if d.onKeyDown != nil || d.onKeyUp != nil || len(d.onAction) > 0 || focusID != 0 || d.keyContext != "" {
		kn := keyNode{
			KeyContext: d.keyContext,
			FocusID:    focusID,
			OnKeyDown:  d.onKeyDown,
			OnKeyUp:    d.onKeyUp,
			OnAction:   d.onAction,
		}
		if focusID != 0 {
			kn.tabIndex, kn.tabStop = fh.TabIndex(), fh.TabStop()
			if d.hasTabIndex {
				kn.tabIndex, kn.tabStop = d.tabIndex, true
			}
			if d.tabStop != nil {
				kn.tabStop = *d.tabStop
			}
		}
		w.insertKeyNode(kn)
	}
	return st
}

// paint emits the div's shadows, background and border with the current
// content mask, using the paint style resolved by paintOpacity. Children
// are painted by the window's walk, after this returns.
func (d *Div) paint(w *Window, bounds Bounds, prepaintState any) {
	if st, ok := prepaintState.(*divPrepaint); ok && st.resolved {
		paintBox(w, bounds, st.style)
		return
	}
	paintBox(w, bounds, d.resolvedStyle())
}

// FlexGrow is StyledMixin.FlexGrow returning *Div to keep the chain.
func (d *Div) FlexGrow(v float32) *Div { d.StyledMixin.FlexGrow(v); return d }

// FlexShrink is StyledMixin.FlexShrink returning *Div to keep the chain.
func (d *Div) FlexShrink(v float32) *Div { d.StyledMixin.FlexShrink(v); return d }

// Px_ is StyledMixin.Px_ returning *Div to keep the chain.
func (d *Div) Px_(v float32) *Div { d.StyledMixin.Px_(v); return d }

// Py_ is StyledMixin.Py_ returning *Div to keep the chain.
func (d *Div) Py_(v float32) *Div { d.StyledMixin.Py_(v); return d }

// Pt is StyledMixin.Pt returning *Div to keep the chain.
func (d *Div) Pt(v float32) *Div { d.StyledMixin.Pt(v); return d }

// Pr is StyledMixin.Pr returning *Div to keep the chain.
func (d *Div) Pr(v float32) *Div { d.StyledMixin.Pr(v); return d }

// Pb is StyledMixin.Pb returning *Div to keep the chain.
func (d *Div) Pb(v float32) *Div { d.StyledMixin.Pb(v); return d }

// Pl is StyledMixin.Pl returning *Div to keep the chain.
func (d *Div) Pl(v float32) *Div { d.StyledMixin.Pl(v); return d }

// M is StyledMixin.M returning *Div to keep the chain.
func (d *Div) M(v float32) *Div { d.StyledMixin.M(v); return d }

// FontWeight is StyledMixin.FontWeight returning *Div to keep the chain.
func (d *Div) FontWeight(w int) *Div { d.StyledMixin.FontWeight(w); return d }

// FontFamily is StyledMixin.FontFamily returning *Div to keep the chain.
func (d *Div) FontFamily(name string) *Div { d.StyledMixin.FontFamily(name); return d }

// TextStrikethrough is StyledMixin.TextStrikethrough returning *Div to keep the chain.
func (d *Div) TextStrikethrough() *Div { d.StyledMixin.TextStrikethrough(); return d }

// TextLineHeight is StyledMixin.TextLineHeight returning *Div to keep the chain.
func (d *Div) TextLineHeight(px float32) *Div { d.StyledMixin.TextLineHeight(px); return d }

// RoundedAll is StyledMixin.RoundedAll returning *Div to keep the chain.
func (d *Div) RoundedAll(tl, tr, br, bl float32) *Div {
	d.StyledMixin.RoundedAll(tl, tr, br, bl)
	return d
}

// ItemsEnd is StyledMixin.ItemsEnd returning *Div to keep the chain.
func (d *Div) ItemsEnd() *Div { d.StyledMixin.ItemsEnd(); return d }

// ItemsStretch is StyledMixin.ItemsStretch returning *Div to keep the chain.
func (d *Div) ItemsStretch() *Div { d.StyledMixin.ItemsStretch(); return d }

// JustifyStart is StyledMixin.JustifyStart returning *Div to keep the chain.
func (d *Div) JustifyStart() *Div { d.StyledMixin.JustifyStart(); return d }

// JustifyCenter is StyledMixin.JustifyCenter returning *Div to keep the chain.
func (d *Div) JustifyCenter() *Div { d.StyledMixin.JustifyCenter(); return d }

// JustifyEnd is StyledMixin.JustifyEnd returning *Div to keep the chain.
func (d *Div) JustifyEnd() *Div { d.StyledMixin.JustifyEnd(); return d }

// JustifyAround is StyledMixin.JustifyAround returning *Div to keep the chain.
func (d *Div) JustifyAround() *Div { d.StyledMixin.JustifyAround(); return d }
