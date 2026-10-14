package ui

import (
	"cad-lite/assets/icons"
	"cad-lite/internal/gpui"
	"cad-lite/settings"
)

// Workspace layout metrics, in logical pixels.
const (
	// splitterSize is the hit area of a splitter; a 1 px line sits in its middle.
	splitterSize = 5
	// minSideWidth and maxSideWidth bound the Layers and Properties column.
	minSideWidth = 180
	maxSideWidth = 640
	// minAgentWidth and maxAgentWidth bound the agent column.
	minAgentWidth = 280
	maxAgentWidth = 800
	// minCanvasWidth is the least width the canvas column keeps, whatever the columns want.
	minCanvasWidth = 240
	// stripWidth is the width of a collapsed column.
	stripWidth = 30
	// sectionHeaderHeight is the height of a Layers or Properties header, which is all a
	// collapsed section shows.
	sectionHeaderHeight = 28
	// layerRowHeight and layerRowGap size one row of the layer list; layerListPad is the space
	// under the last row.
	layerRowHeight = 26
	layerRowGap    = 2
	layerListPad   = 6
	// minPropsBody is the least height the Properties body keeps below its header.
	minPropsBody = 56
	// sideTopPad is the space above the column's first section.
	sideTopPad = 4
	// collapseButtonSize is the edge of a panel's collapse button.
	collapseButtonSize = 22
)

// splitKind says which splitter a drag moves.
type splitKind int

const (
	splitNone splitKind = iota
	splitSide
	splitAgent
	splitLayers
	splitHistory
)

// layoutState is the arrangement of the workspace panels. A size of 0 means automatic. Collapsing
// a panel never touches its size, so expanding restores it.
type layoutState struct {
	// sideW and agentW are the widths of the Layers and Properties column and of the agent column.
	sideW, agentW float32
	// layersH caps the Layers section's height; the section never grows past its rows.
	layersH float32
	// sideOff folds the column to a strip; layersOff and propsOff fold one section to its header;
	// histOff hides the command history; ribbonOff hides the ribbon.
	sideOff, layersOff, propsOff, histOff, ribbonOff bool
	// drag is the splitter being held: the pointer coordinate and the size when it went down.
	drag struct {
		kind      splitKind
		pos, from float32
	}
	// col is the column's last-frame bounds, which say how much height the sections share.
	col *gpui.ScrollHandle
}

// resizeSpan returns from moved by delta and clamped to [lo, hi]; hi wins over lo when the range
// is empty, so a panel is never pushed past what its neighbours need.
func resizeSpan(from, delta, lo, hi float32) float32 {
	return max(min(from+delta, hi), min(lo, hi))
}

// columnWidths returns the widths of the Layers and Properties column and of the agent column
// (0 while it is hidden) for a window of the given width. A saved width of 0 means the default;
// each width is clamped to its own range; when the two do not leave minCanvasWidth for the
// canvas, the agent column gives way first, then the Layers column, and below their minimums
// both share what is left. A collapsed column is a strip of stripWidth.
func columnWidths(window, sideSaved, agentSaved float32, sideOff, agentOn bool) (side, agent float32) {
	side = stripWidth
	if !sideOff {
		side = sideSaved
		if side <= 0 {
			side = sidePanelWidth(window)
		}
		side = min(max(side, minSideWidth), maxSideWidth)
	}
	if agentOn {
		agent = agentSaved
		if agent <= 0 {
			agent = agentPanelWidth(window)
		}
		agent = min(max(agent, minAgentWidth), maxAgentWidth)
	}
	gaps := float32(0)
	if !sideOff {
		gaps += splitterSize
	} else {
		gaps++
	}
	if agentOn {
		gaps += splitterSize
	}
	budget := window - minCanvasWidth - gaps
	if over := side + agent - budget; over > 0 {
		cut := min(over, max(0, agent-minAgentWidth))
		agent -= cut
		over -= cut
		if !sideOff {
			cut = min(over, max(0, side-minSideWidth))
			side -= cut
			over -= cut
		}
		if over > 0 && agentOn && !sideOff {
			side, agent = max(budget/2, 60), max(budget/2, 60)
		} else if over > 0 && agentOn {
			agent = max(agent-over, 60)
		} else if over > 0 && !sideOff {
			side = max(side-over, 60)
		}
	}
	return side, agent
}

// sideRange is the width range the side splitter drags within, given the agent column's width
// (0 while hidden).
func sideRange(window, agent float32) (lo, hi float32) {
	gaps := float32(splitterSize)
	if agent > 0 {
		gaps += splitterSize + agent
	}
	return minSideWidth, max(minSideWidth, min(maxSideWidth, window-minCanvasWidth-gaps))
}

// agentRange is the width range the agent splitter drags within, given the side column's width.
func agentRange(window, side float32, sideOff bool) (lo, hi float32) {
	gaps := float32(splitterSize*2) + side
	if sideOff {
		gaps = splitterSize + 1 + side
	}
	return minAgentWidth, max(minAgentWidth, min(maxAgentWidth, window-minCanvasWidth-gaps))
}

// layersContentHeight is the Layers section's height with all its rows showing.
func layersContentHeight(rows int) float32 {
	if rows <= 0 {
		return sectionHeaderHeight
	}
	return sectionHeaderHeight + float32(rows)*layerRowHeight + float32(rows-1)*layerRowGap + layerListPad
}

// layersLimits is the range the Layers section's height can take: from the header and one row to
// what is left once Properties has its header (and, unless folded, a minimal body), but never past
// the rows themselves.
func layersLimits(avail float32, rows int, propsOff bool) (lo, hi float32) {
	room := avail - sectionHeaderHeight
	if !propsOff {
		room -= minPropsBody
	}
	hi = min(layersContentHeight(rows), room)
	lo = min(layersContentHeight(1), hi)
	return max(lo, sectionHeaderHeight), max(hi, sectionHeaderHeight)
}

// layersHeight is the Layers section's height when avail is shared by both sections: only what
// its rows need, capped by the user's dragged height (0 for none) and by what Properties leaves.
// A folded section is its header.
func layersHeight(avail float32, rows int, explicit float32, layersOff, propsOff bool) float32 {
	if layersOff {
		return sectionHeaderHeight
	}
	_, hi := layersLimits(avail, rows, propsOff)
	if explicit > 0 {
		hi = min(hi, max(explicit, sectionHeaderHeight))
	}
	return hi
}

// agentShown reports whether the agent column is on screen.
func (ws *Workspace) agentShown() bool { return ws.agent != nil && ws.agent.visible }

// widths returns the current column widths for a window of the given width.
func (ws *Workspace) widths(window float32) (side, agent float32) {
	return columnWidths(window, ws.lay.sideW, ws.lay.agentW, ws.lay.sideOff, ws.agentShown())
}

// columnHeight is the Layers and Properties column's height: the last frame's, or an estimate
// from the window before there is one.
func (ws *Workspace) columnHeight() float32 {
	if h := float32(ws.lay.col.Bounds().Size.H); h > 0 {
		return h
	}
	return max(0, ws.winH-200)
}

// sectionAvail is the height the Layers and Properties sections share.
func (ws *Workspace) sectionAvail() float32 {
	return ws.columnHeight() - sideTopPad - splitterSize
}

// historyLimit is the tallest the command history may be dragged: it leaves the canvas room.
func (ws *Workspace) historyLimit() float32 {
	return max(minHistoryHeight, min(maxHistoryHeight, ws.winH-420))
}

// beginDrag starts moving a splitter; pos is the pointer coordinate along its axis and from the
// size it resizes.
func (ws *Workspace) beginDrag(kind splitKind, pos, from float32) {
	d := &ws.lay.drag
	d.kind, d.pos, d.from = kind, pos, from
}

// dragMove continues the drag in progress with the pointer at p (window coordinates).
func (ws *Workspace) dragMove(p gpui.Point, w *gpui.Window) {
	d := ws.lay.drag
	if d.kind == splitNone {
		return
	}
	x, y := float32(p.X), float32(p.Y)
	switch d.kind {
	case splitSide:
		_, agent := ws.widths(ws.winW)
		lo, hi := sideRange(ws.winW, agent)
		ws.lay.sideW = resizeSpan(d.from, d.pos-x, lo, hi)
	case splitAgent:
		side, _ := ws.widths(ws.winW)
		lo, hi := agentRange(ws.winW, side, ws.lay.sideOff)
		ws.lay.agentW = resizeSpan(d.from, d.pos-x, lo, hi)
	case splitLayers:
		lo, hi := layersLimits(ws.sectionAvail(), len(ws.session.Drawing().Layers()), ws.lay.propsOff)
		ws.lay.layersH = resizeSpan(d.from, y-d.pos, lo, hi)
	case splitHistory:
		ws.cmd.histH = resizeSpan(d.from, d.pos-y, minHistoryHeight, ws.historyLimit())
	}
	w.Refresh()
}

// endDrag releases the splitter and saves the layout when one was held.
func (ws *Workspace) endDrag(w *gpui.Window) {
	if ws.lay.drag.kind == splitNone {
		return
	}
	ws.lay.drag.kind = splitNone
	ws.persistLayout(w)
}

// layoutSettings returns the layout as the settings file keeps it.
func (ws *Workspace) layoutSettings() settings.Layout {
	r := func(v float32) int { return max(0, int(v+0.5)) }
	l := &ws.lay
	return settings.Layout{
		SideWidth: r(l.sideW), AgentWidth: r(l.agentW), LayersHeight: r(l.layersH), HistoryHeight: r(ws.cmd.histH),
		SideCollapsed: l.sideOff, LayersCollapsed: l.layersOff, PropsCollapsed: l.propsOff,
		HistoryCollapsed: l.histOff, RibbonCollapsed: l.ribbonOff,
	}
}

// applyLayout makes l the live layout.
func (ws *Workspace) applyLayout(l settings.Layout) {
	ws.lay.sideW, ws.lay.agentW, ws.lay.layersH = float32(l.SideWidth), float32(l.AgentWidth), float32(l.LayersHeight)
	ws.cmd.histH = float32(l.HistoryHeight)
	ws.lay.sideOff, ws.lay.layersOff, ws.lay.propsOff = l.SideCollapsed, l.LayersCollapsed, l.PropsCollapsed
	ws.lay.histOff, ws.lay.ribbonOff = l.HistoryCollapsed, l.RibbonCollapsed
}

// persistLayout records the live layout in the settings and saves them shortly after.
func (ws *Workspace) persistLayout(w *gpui.Window) {
	ws.prefs().cur.Layout = ws.layoutSettings()
	ws.scheduleSave(w)
}

// toggleFold flips one collapsed flag, saves the layout and redraws.
func (ws *Workspace) toggleFold(flag *bool, w *gpui.Window) {
	*flag = !*flag
	ws.persistLayout(w)
	w.Refresh()
}

// splitter is a draggable divider: a splitterSize hit area with the 1 px line in its middle that
// takes the accent while hovered or dragged. horizontal means a line across (it moves up and
// down). start returns the size the drag begins from; reset runs on a double click.
func (ws *Workspace) splitter(id string, kind splitKind, horizontal bool, tip string, start func() float32, reset func()) *gpui.Div {
	hot := mix(uiPanel, uiAccent, 0.5)
	s := gpui.NewDiv().ID(id).FlexShrink(0).Tooltip(tip).
		Hover(func(m *gpui.StyledMixin) *gpui.StyledMixin { return m.Bg(hot) }).
		OnMouseDown(func(e *gpui.MouseDownEvent, w *gpui.Window, _ *gpui.App) {
			if e.Button != gpui.MouseLeft {
				return
			}
			w.StopPropagation()
			if e.ClickCount >= 2 {
				reset()
				ws.persistLayout(w)
				w.Refresh()
				return
			}
			pos := float32(e.Position.X)
			if horizontal {
				pos = float32(e.Position.Y)
			}
			ws.beginDrag(kind, pos, start())
		})
	if ws.lay.drag.kind == kind {
		s.Bg(hot)
	}
	if horizontal {
		return s.FlexCol().JustifyCenter().SizeH(splitterSize).Child(gpui.NewDiv().SizeH(1).Bg(uiBorder))
	}
	return s.FlexRow().JustifyCenter().SizeW(splitterSize).Child(gpui.NewDiv().SizeW(1).Bg(uiBorder))
}

// collapseButton is a small icon button that folds or unfolds a panel. Its press does not start
// the drag of a handle it sits on.
func collapseButton(id, tip string, icon icons.Name, act func(*gpui.Window)) *gpui.Div {
	return gpui.NewDiv().ID(id).FlexRow().ItemsCenter().JustifyCenter().SizeWH(collapseButtonSize, collapseButtonSize).Rounded(5).FlexShrink(0).
		Tooltip(tip).
		Hover(func(m *gpui.StyledMixin) *gpui.StyledMixin { return m.Bg(uiPanelAlt) }).
		OnMouseDown(func(_ *gpui.MouseDownEvent, w *gpui.Window, _ *gpui.App) { w.StopPropagation() }).
		OnClick(func(_ *gpui.ClickEvent, w *gpui.Window, _ *gpui.App) { act(w) }).
		Child(NewIcon(icon, 14, uiMuted))
}
