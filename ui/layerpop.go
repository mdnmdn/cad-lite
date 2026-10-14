package ui

import (
	"runtime"
	"strings"
	"sync"
	"weak"

	"cad-lite/cmdsys"
	"cad-lite/doc"
	"cad-lite/i18n"
	"cad-lite/internal/gpui"
)

// Layer popover metrics, in logical pixels.
const (
	layerPopWidth = 300
	// layerPopRows is how many rows the list shows before it scrolls.
	layerPopRows = 10
	// layerPopGap is the space between the popover and the status bar item it hangs from.
	layerPopGap = 6
)

// statusState is the state of the status bar's overlays: the layer popover and the properties
// HUD. It lives beside the Workspace (see status) and never references it.
type statusState struct {
	// pop is set while the layer popover is open; popFocus asks the next frame to focus its
	// filter; hi is the highlighted row among the matches.
	pop      bool
	popFocus bool
	hi       int
	filter   *gpui.TextInput
	list     *gpui.ScrollHandle
	// anchor is the status bar layer item's bounds in the last frame.
	anchor gpui.Bounds
	// barTop is the status bar's top edge in the last frame, which places the HUD.
	barTop float32
	// app is the App the popover was opened with; the status bar is built without one and the
	// filter field needs it.
	app weak.Pointer[gpui.App]
	// hud shows the properties HUD over the canvas; it is not saved.
	hud bool
	// popSession is the session the popover was opened for; another one (a tab switch) closes it.
	// Weak, like every reference from here to what the Workspace owns: see layerFilter.
	popSession weak.Pointer[cmdsys.Session]
	// props caches the Properties lines' summary of the selection (selectionSummary).
	props propsSummary
}

var statusTable = struct {
	sync.Mutex
	m map[weak.Pointer[Workspace]]*statusState
}{m: map[weak.Pointer[Workspace]]*statusState{}}

// status returns ws's status bar overlay state, creating it on first use.
func (ws *Workspace) status() *statusState {
	key := weak.Make(ws)
	statusTable.Lock()
	defer statusTable.Unlock()
	if s, ok := statusTable.m[key]; ok {
		return s
	}
	s := &statusState{list: gpui.NewScrollHandle()}
	statusTable.m[key] = s
	runtime.AddCleanup(ws, func(k weak.Pointer[Workspace]) {
		statusTable.Lock()
		delete(statusTable.m, k)
		statusTable.Unlock()
	}, key)
	return s
}

// LayerPopoverShown reports whether the status bar's layer popover is open.
func (ws *Workspace) LayerPopoverShown() bool { return ws.status().pop }

// layerMatches returns the layers whose name contains filter, ignoring case, in table order.
func layerMatches(d *doc.Drawing, filter string) []*doc.Layer {
	f := strings.ToLower(strings.TrimSpace(filter))
	var out []*doc.Layer
	for _, l := range d.Layers() {
		if f == "" || strings.Contains(strings.ToLower(l.Name), f) {
			out = append(out, l)
		}
	}
	return out
}

// openLayerPopover opens the popover with an empty filter, the current layer highlighted and the
// keyboard in the filter.
func (ws *Workspace) openLayerPopover(w *gpui.Window, cx *gpui.App) {
	s := ws.status()
	s.pop, s.popFocus, s.app, s.popSession = true, true, weak.Make(cx), weak.Make(ws.session)
	ws.layerFilter().SetText("")
	s.hi = 0
	d := ws.session.Drawing()
	for i, l := range d.Layers() {
		if l.Name == d.CurrentLayer {
			s.hi = i
		}
	}
	s.list.ScrollToItem(s.hi)
	w.Refresh()
}

// closeLayerPopover closes the popover and gives the keyboard back to the command line.
func (ws *Workspace) closeLayerPopover(w *gpui.Window) {
	s := ws.status()
	if !s.pop {
		return
	}
	s.pop = false
	if fh := ws.input.FocusHandle(); fh != nil {
		w.RequestFocus(fh)
	}
	w.Refresh()
}

// toggleLayerPopover opens the popover, or closes it when it is open.
func (ws *Workspace) toggleLayerPopover(w *gpui.Window, cx *gpui.App) {
	if ws.status().pop {
		ws.closeLayerPopover(w)
	} else {
		ws.openLayerPopover(w, cx)
	}
}

// layerFilter is the popover's filter field, made on first use: typing filters, Enter picks,
// Escape closes. The field lives in statusTable, so its callbacks hold the Workspace weakly:
// a strong reference from there would keep it from ever being collected.
func (ws *Workspace) layerFilter() *gpui.TextInput {
	s := ws.status()
	if s.filter != nil {
		return s.filter
	}
	wp := weak.Make(ws)
	with := func(f func(*Workspace)) {
		if ws := wp.Value(); ws != nil {
			f(ws)
		}
	}
	s.filter = themeBareField(gpui.NewTextInput(i18n.T("ui.layers.filter_placeholder")), fontUI, false)
	s.filter.OnChange(func(_ string, w *gpui.Window, _ *gpui.App) {
		s.hi = 0
		s.list.ScrollToTop()
		w.Refresh()
	})
	s.filter.OnSubmit(func(_ string, w *gpui.Window, _ *gpui.App) { with(func(ws *Workspace) { ws.pickLayerMatch(w) }) })
	s.filter.OnEscape(func(w *gpui.Window, _ *gpui.App) { with(func(ws *Workspace) { ws.closeLayerPopover(w) }) })
	return s.filter
}

// pickLayerMatch answers Enter: a single match, else the highlighted one, becomes current and the
// popover closes. With no match, or on a frozen layer, it does nothing.
func (ws *Workspace) pickLayerMatch(w *gpui.Window) {
	s := ws.status()
	m := layerMatches(ws.session.Drawing(), ws.layerFilter().Text())
	switch {
	case len(m) == 1:
		ws.setCurrentLayer(m[0], w)
	case s.hi >= 0 && s.hi < len(m):
		ws.setCurrentLayer(m[s.hi], w)
	}
}

// setCurrentLayer makes l current, as a click on its row in the side panel does, and closes the
// popover. A frozen layer cannot be current (-LAYER Set refuses it too): nothing happens.
func (ws *Workspace) setCurrentLayer(l *doc.Layer, w *gpui.Window) {
	if l.Frozen {
		return
	}
	ws.makeLayerCurrent(l)
	ws.closeLayerPopover(w)
}

func (ws *Workspace) makeLayerCurrent(l *doc.Layer) {
	d := ws.session.Drawing()
	if !l.Frozen && d.CurrentLayer != l.Name {
		d.CurrentLayer = l.Name
		ws.session.MarkModified()
	}
}

// layerFold returns the layer whose name is name ignoring case, or nil: layer names compare
// without case in AutoCAD, so "wall" must not become a second layer beside "Wall".
func layerFold(d *doc.Drawing, name string) *doc.Layer {
	for _, l := range d.Layers() {
		if strings.EqualFold(l.Name, name) {
			return l
		}
	}
	return nil
}

// moveLayerHighlight moves the highlight by delta rows, stopping at either end.
func (ws *Workspace) moveLayerHighlight(delta int, w *gpui.Window) {
	s := ws.status()
	n := len(layerMatches(ws.session.Drawing(), ws.layerFilter().Text()))
	if n == 0 {
		return
	}
	s.hi = min(max(s.hi+delta, 0), n-1)
	s.list.ScrollToItem(s.hi)
	w.Refresh()
}

// newLayerFromPopover creates a layer: the filter text, when it names no layer even ignoring
// case, becomes a new current layer through LAYER; otherwise LAYER starts on the command line for
// a name.
func (ws *Workspace) newLayerFromPopover(w *gpui.Window) {
	name := strings.TrimSpace(ws.layerFilter().Text())
	ws.closeLayerPopover(w)
	ws.run("LAYER", w)
	if name != "" && layerFold(ws.session.Drawing(), name) == nil {
		d := ws.session.Drawing()
		n, current := len(d.Layers()), d.CurrentLayer
		ws.submit(name, w)
		if len(d.Layers()) != n || d.CurrentLayer != current {
			ws.session.MarkModified()
		}
	}
}

// layerItem is the status bar's current-layer label: a press opens the layer popover over it.
// A zero-width marker at its left edge records where it is while painting, so the popover can
// hang above it; being zero wide it never takes the pointer from the label.
func (ws *Workspace) layerItem() *gpui.Div {
	s := ws.status()
	d := ws.session.Drawing()
	mark := gpui.NewDiv().SizeWH(0, 18).FlexShrink(0).Child(gpui.NewCanvas(func(b gpui.Bounds, _ *gpui.Window, _ *gpui.App) { s.anchor = b }))
	item := gpui.NewDiv().ID("status-layer").FlexRow().ItemsCenter().Px_(6).Py_(1).Rounded(5).FlexShrink(0).
		Tooltip(i18n.T("ui.status.layer_tip")).
		Hover(func(m *gpui.StyledMixin) *gpui.StyledMixin { return m.Bg(uiPanelAlt) }).
		// A press, not a click, opens it: while it is open the backdrop takes the press that closes
		// it, so a click on the label never reopens it.
		OnMouseDown(func(e *gpui.MouseDownEvent, w *gpui.Window, cx *gpui.App) {
			if e.Button == gpui.MouseLeft {
				ws.toggleLayerPopover(w, cx)
			}
		}).
		Child(mark, gpui.NewText(i18n.T("ui.status.layer", d.CurrentLayer)).TextSize(fontUI).TextColor(uiText))
	if s.pop {
		item.Bg(activeFill())
	}
	return item
}

// layerPopover is the popover itself: the filter, the matching layers (rows shared with the side
// panel) and the New layer button. winW and winH place it above the status bar item.
func (ws *Workspace) layerPopover(w *gpui.Window, winW, winH float32) *gpui.Div {
	s := ws.status()
	d := ws.session.Drawing()
	f := ws.layerFilter()
	field := f.Render(w, s.app.Value())
	if s.popFocus {
		if fh := f.FocusHandle(); fh != nil {
			s.popFocus = false
			w.RequestFocus(fh)
		}
	}
	matches := layerMatches(d, f.Text())
	s.hi = min(s.hi, max(len(matches)-1, 0))
	// The list's height is set, not left to its rows: hung from the status bar, the popover would
	// otherwise be squeezed to the bar's height.
	rows := min(max(len(matches), 1), layerPopRows)
	list := gpui.NewDiv().ID("layer-pop-list").FlexCol().Gap(layerRowGap).OverflowYScroll().TrackScroll(s.list).
		SizeH(float32(rows)*layerRowHeight + float32(rows-1)*layerRowGap).FlexShrink(0)
	for i, l := range matches {
		list.Child(ws.layerRow("layer-pop-", d, l, i == s.hi, func(w *gpui.Window) { ws.setCurrentLayer(l, w) }))
	}
	if len(matches) == 0 {
		list.Child(gpui.NewText(i18n.T("ui.layers.no_match", f.Text())).TextColor(uiMuted).Px_(6).Py_(4))
	}
	label := i18n.T("ui.layers.new")
	if name := strings.TrimSpace(f.Text()); name != "" && layerFold(d, name) == nil {
		label = i18n.T("ui.layers.new_named", name)
	}
	add := menuRow("layer-pop-new", i18n.T("ui.layers.new_tip"), ws.newLayerFromPopover).
		Child(gpui.NewText(label).TextSize(fontUI).TextColor(uiText))

	a := s.anchor
	left := min(float32(a.Origin.X), winW-layerPopWidth-8)
	bottom := winH - float32(a.Origin.Y) + layerPopGap
	stop := func(_ *gpui.MouseDownEvent, w *gpui.Window, _ *gpui.App) { w.StopPropagation() }
	return gpui.NewDiv().ID("layer-pop").Absolute().Left(max(left, 8)).Bottom(bottom).SizeW(layerPopWidth).
		FlexCol().Gap(4).P(6).Rounded(8).Bg(uiPanelAlt).Border(1, uiBorder).TextSize(fontUI).
		OnMouseDown(stop).
		OnClick(func(_ *gpui.ClickEvent, w *gpui.Window, _ *gpui.App) { w.StopPropagation() }).
		OnScrollWheel(func(_ *gpui.ScrollWheelEvent, w *gpui.Window, _ *gpui.App) { w.StopPropagation() }).
		OnKeyDown(func(e *gpui.KeyDownEvent, w *gpui.Window, _ *gpui.App) {
			switch e.Keystroke.Key {
			case "up":
				ws.moveLayerHighlight(-1, w)
			case "down":
				ws.moveLayerHighlight(1, w)
			default:
				return
			}
			w.StopPropagation()
		}).
		Child(
			gpui.NewDiv().ID("layer-pop-filter").FlexCol().SizeH(28).FlexShrink(0).Px_(8).Rounded(5).Bg(uiField).Border(1, uiBorder).Child(field),
			list,
			gpui.NewDiv().SizeH(1).FlexShrink(0).Bg(uiBorder),
			add.FlexShrink(0),
		)
}

// statusOverlays are what the status bar draws over the window, as absolute children of the bar
// (whose left and bottom edges are the window's): a zero-size marker that records where the bar's
// top is, the properties HUD over the canvas, and the layer popover with the transparent
// backdrop that closes it on a press outside. Every div takes the pointer over its whole box, so
// none of these is bigger than what it shows, except the popover's backdrop, whose job is to.
func (ws *Workspace) statusOverlays(w *gpui.Window) []gpui.IntoElement {
	s := ws.status()
	if s.pop && s.popSession != weak.Make(ws.session) {
		ws.closeLayerPopover(w) // another drawing tab became active
	}
	winW, winH := float32(w.Bounds().Size.W), float32(w.Bounds().Size.H)
	out := []gpui.IntoElement{gpui.NewDiv().Absolute().Left(0).Top(0).SizeWH(0, 0).
		Child(gpui.NewCanvas(func(b gpui.Bounds, _ *gpui.Window, _ *gpui.App) { s.barTop = float32(b.Origin.Y) }))}
	if s.hud {
		out = append(out, ws.propsHUD(winW))
	}
	if s.pop {
		// The popover is the backdrop's child: placed from its bottom, it needs a parent as tall as
		// the window, and its own handlers keep its presses from reaching the backdrop's.
		out = append(out,
			gpui.NewDiv().ID("layer-pop-backdrop").Absolute().Left(0).Bottom(0).SizeWH(winW, winH).
				OnMouseDown(func(_ *gpui.MouseDownEvent, w *gpui.Window, _ *gpui.App) {
					w.StopPropagation()
					ws.closeLayerPopover(w)
				}).
				OnScrollWheel(func(_ *gpui.ScrollWheelEvent, w *gpui.Window, _ *gpui.App) { w.StopPropagation() }).
				Child(ws.layerPopover(w, winW, winH)))
	}
	return out
}
