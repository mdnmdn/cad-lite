package ui

import (
	"fmt"
	"strings"

	"go-draw/cmdsys"
	"go-draw/cmdsys/commands"
	"go-draw/doc"
	"go-draw/internal/gpui"
	"go-draw/render"
)

// Workspace is the root view: one drawing, its session and the window
// chrome around the canvas.
type Workspace struct {
	session *cmdsys.Session
	view    *render.View
	builder *render.Builder
	input   *gpui.TextInput

	canvas canvasState
	// Grid toggles the background grid (F7).
	Grid bool
	// fitPending zooms to extents once the canvas size is known.
	fitPending bool
}

// historyLines is how many history lines show above the command line.
const historyLines = 4

// NewWorkspace returns a workspace editing d. files may be nil, which
// disables OPEN and SAVE (as in a browser build).
func NewWorkspace(d *doc.Drawing, files cmdsys.FileHandler) *Workspace {
	v := render.DefaultView(800, 600)
	ws := &Workspace{view: &v, Grid: true, fitPending: d.Len() > 0}
	ws.session = cmdsys.NewSession(d, ws.view, commands.NewRegistry())
	ws.session.SetFiles(files)
	ws.attach(d)
	ws.session.OnDrawing(ws.attach)
	ws.input = gpui.NewTextInput("Type a command")
	ws.input.OnSubmit(func(text string, w *gpui.Window, _ *gpui.App) { ws.submit(text, w) })
	ws.input.OnEscape(func(w *gpui.Window, _ *gpui.App) { ws.escape(w) })
	ws.input.OnChange(func(text string, w *gpui.Window, _ *gpui.App) {
		// Space is Enter, as in AutoCAD, except where the prompt takes text.
		if strings.HasSuffix(text, " ") && !ws.acceptsText() {
			ws.submit(strings.TrimSuffix(text, " "), w)
		}
	})
	return ws
}

// attach builds the display cache for a (new) drawing.
func (ws *Workspace) attach(d *doc.Drawing) {
	ws.builder = render.NewBuilder(d, colCanvas)
	d.OnChange(ws.builder.Invalidate)
}

// Session returns the command session, for scripting and tests.
func (ws *Workspace) Session() *cmdsys.Session { return ws.session }

// View returns the canvas view.
func (ws *Workspace) View() *render.View { return ws.view }

func (ws *Workspace) acceptsText() bool {
	p := ws.session.CurrentPrompt()
	return p != nil && p.Accepts&cmdsys.InputText != 0
}

func (ws *Workspace) submit(text string, w *gpui.Window) {
	ws.input.SetText("")
	ws.session.Submit(text)
	ws.builder.InvalidateAll()
	w.Refresh()
}

func (ws *Workspace) escape(w *gpui.Window) {
	ws.input.SetText("")
	ws.session.Escape()
	w.Refresh()
}

func (ws *Workspace) run(name string, w *gpui.Window) {
	ws.input.SetText("")
	ws.session.Run(name)
	w.Refresh()
}

// Actions bound by Bindings.
var (
	actionUndo      = gpui.NewAction("cad-undo")
	actionRedo      = gpui.NewAction("cad-redo")
	actionSnap      = gpui.NewAction("cad-toggle-snap")
	actionOrtho     = gpui.NewAction("cad-toggle-ortho")
	actionGrid      = gpui.NewAction("cad-toggle-grid")
	actionZoomExtnt = gpui.NewAction("cad-zoom-extents")
)

// Bindings returns the workspace key bindings, to add to the app keymap.
func Bindings() []gpui.KeyBinding {
	return []gpui.KeyBinding{
		gpui.NewKeyBinding("ctrl-z", actionUndo, "cad"),
		gpui.NewKeyBinding("cmd-z", actionUndo, "cad"),
		gpui.NewKeyBinding("ctrl-y", actionRedo, "cad"),
		gpui.NewKeyBinding("cmd-shift-z", actionRedo, "cad"),
		gpui.NewKeyBinding("f3", actionSnap, "cad"),
		gpui.NewKeyBinding("f7", actionGrid, "cad"),
		gpui.NewKeyBinding("f8", actionOrtho, "cad"),
		gpui.NewKeyBinding("ctrl-e", actionZoomExtnt, "cad"),
	}
}

// Render builds the window for the current state.
func (ws *Workspace) Render(w *gpui.Window, cx *gpui.App) gpui.IntoElement {
	cmdline := ws.input.Render(w, cx)
	if fh := ws.input.FocusHandle(); fh != nil && w.Focus() == nil {
		w.RequestFocus(fh)
	}
	toggle := func(f func()) func(gpui.Action, *gpui.Window, *gpui.App) {
		return func(_ gpui.Action, w *gpui.Window, _ *gpui.App) { f(); w.Refresh() }
	}
	s := ws.session
	return gpui.NewDiv().KeyContext("cad").
		FlexCol().SizeFull().Bg(uiBg).TextColor(uiText).TextSize(13).
		OnAction(actionUndo, func(_ gpui.Action, w *gpui.Window, _ *gpui.App) { ws.run("UNDO", w) }).
		OnAction(actionRedo, func(_ gpui.Action, w *gpui.Window, _ *gpui.App) { ws.run("REDO", w) }).
		OnAction(actionSnap, toggle(func() { s.SnapOn = !s.SnapOn })).
		OnAction(actionOrtho, toggle(func() { s.Ortho = !s.Ortho })).
		OnAction(actionGrid, toggle(func() { ws.Grid = !ws.Grid })).
		OnAction(actionZoomExtnt, func(_ gpui.Action, w *gpui.Window, _ *gpui.App) { ws.run("ZOOM", w); ws.submit("e", w) }).
		Child(
			ws.toolbar(),
			gpui.NewDiv().FlexRow().Flex1().MinH(0).Child(
				ws.canvasElement(),
				ws.sidePanel(),
			),
			ws.commandArea(cmdline),
			ws.statusBar(),
		)
}

func (ws *Workspace) button(id, label, command string) *gpui.Div {
	active := ws.session.Active() == command
	b := gpui.NewDiv().ID("tool-" + id).Px_(10).Py_(5).Rounded(4).
		Hover(func(m *gpui.StyledMixin) *gpui.StyledMixin { return m.Bg(uiPanelAlt) }).
		OnClick(func(_ *gpui.ClickEvent, w *gpui.Window, _ *gpui.App) { ws.run(command, w) }).
		Child(gpui.NewText(label))
	if active {
		b.Bg(uiAccent)
	}
	return b
}

func separator() *gpui.Div { return gpui.NewDiv().SizeWH(1, 18).Bg(uiBorder).Mx(4) }

func (ws *Workspace) toolbar() *gpui.Div {
	return gpui.NewDiv().FlexRow().ItemsCenter().Gap(2).Px_(8).Py_(4).Bg(uiPanel).Child(
		gpui.NewText("go-draw").FontWeight(700).TextColor(uiOn).Pr(10),
		ws.button("line", "Line", "LINE"),
		ws.button("pline", "Polyline", "PLINE"),
		ws.button("circle", "Circle", "CIRCLE"),
		ws.button("arc", "Arc", "ARC"),
		ws.button("rect", "Rectangle", "RECTANG"),
		separator(),
		ws.button("move", "Move", "MOVE"),
		ws.button("copy", "Copy", "COPY"),
		ws.button("rotate", "Rotate", "ROTATE"),
		ws.button("scale", "Scale", "SCALE"),
		ws.button("erase", "Erase", "ERASE"),
		separator(),
		ws.button("undo", "Undo", "UNDO"),
		ws.button("redo", "Redo", "REDO"),
		ws.button("zoom", "Zoom", "ZOOM"),
	)
}

func (ws *Workspace) sidePanel() *gpui.Div {
	d := ws.session.Drawing()
	layers := gpui.NewDiv().FlexCol().Gap(2)
	for _, l := range d.Layers() {
		l := l
		swatch := gpui.NewDiv().SizeWH(12, 12).Rounded(2).Bg(gpuiColor(render.ACI(l.Color)))
		name := gpui.NewText(l.Name).Flex1()
		if l.Name == d.CurrentLayer {
			name.FontWeight(700).TextColor(uiOn)
		}
		state := "on"
		stateCol := uiText
		if l.Off {
			state, stateCol = "off", uiMuted
		}
		vis := gpui.NewDiv().ID("layer-vis-" + l.Name).Px_(6).Rounded(3).Bg(uiPanelAlt).
			OnClick(func(_ *gpui.ClickEvent, w *gpui.Window, _ *gpui.App) {
				l.Off = !l.Off
				w.Refresh()
			}).
			Child(gpui.NewText(state).TextColor(stateCol))
		layers.Child(gpui.NewDiv().ID("layer-"+l.Name).FlexRow().ItemsCenter().Gap(8).Px_(6).Py_(4).Rounded(4).
			Hover(func(m *gpui.StyledMixin) *gpui.StyledMixin { return m.Bg(uiPanelAlt) }).
			OnClick(func(_ *gpui.ClickEvent, w *gpui.Window, _ *gpui.App) {
				d.CurrentLayer = l.Name
				w.Refresh()
			}).
			Child(swatch, name, vis))
	}
	props := gpui.NewDiv().FlexCol().Gap(4)
	sel := ws.session.Selection()
	if sel.Len() == 0 {
		props.Child(gpui.NewText("No selection").TextColor(uiMuted))
	} else {
		counts := map[string]int{}
		var order []string
		for _, h := range sel.Handles() {
			if e := d.Entity(h); e != nil {
				if counts[e.Class()] == 0 {
					order = append(order, e.Class())
				}
				counts[e.Class()]++
			}
		}
		props.Child(gpui.NewText(fmt.Sprintf("%d selected", sel.Len())).FontWeight(700))
		for _, c := range order {
			props.Child(gpui.NewText(fmt.Sprintf("%s × %d", c, counts[c])).TextColor(uiMuted))
		}
		if sel.Len() == 1 {
			e := d.Entity(sel.Handles()[0])
			props.Child(gpui.NewText("Layer: "+e.Common().Layer).TextColor(uiMuted),
				gpui.NewText("Handle: "+e.Handle().String()).TextColor(uiMuted))
		}
	}
	heading := func(s string) *gpui.Text {
		return gpui.NewText(s).FontWeight(700).TextSize(11).TextColor(uiMuted)
	}
	return gpui.NewDiv().FlexCol().SizeW(210).Gap(8).P(10).Bg(uiPanel).
		Child(heading("LAYERS"), layers, heading("PROPERTIES"), props)
}

func (ws *Workspace) commandArea(cmdline gpui.IntoElement) *gpui.Div {
	hist := ws.session.History()
	lines := gpui.NewDiv().FlexCol().Px_(10).Pt(4)
	for i := max(0, len(hist)-historyLines); i < len(hist); i++ {
		lines.Child(gpui.NewText(hist[i]).TextColor(uiMuted))
	}
	for i := len(hist); i < historyLines; i++ {
		lines.Child(gpui.NewText(" "))
	}
	return gpui.NewDiv().FlexCol().Bg(uiPanel).Child(
		lines,
		gpui.NewDiv().FlexRow().ItemsCenter().Gap(8).Px_(10).Py_(6).Child(
			gpui.NewText(ws.session.Prompt()).FontWeight(700).FlexShrink(0),
			gpui.NewDiv().Flex1().FlexCol().TextColor(gpui.Gray900).Child(cmdline),
		),
	)
}

func (ws *Workspace) statusBar() *gpui.Div {
	s := ws.session
	cur := s.Cursor()
	flag := func(id, label string, on bool, f func()) *gpui.Div {
		c := uiMuted
		if on {
			c = uiOn
		}
		return gpui.NewDiv().ID("status-" + id).Px_(8).Py_(2).Rounded(3).
			Hover(func(m *gpui.StyledMixin) *gpui.StyledMixin { return m.Bg(uiPanelAlt) }).
			OnClick(func(_ *gpui.ClickEvent, w *gpui.Window, _ *gpui.App) { f(); w.Refresh() }).
			Child(gpui.NewText(label).FontWeight(700).TextColor(c))
	}
	return gpui.NewDiv().FlexRow().ItemsCenter().Gap(6).Px_(10).Py_(4).Bg(uiBg).TextSize(12).Child(
		gpui.NewText(fmt.Sprintf("%s, %s", cmdsys.FormatNumber(cur.X), cmdsys.FormatNumber(cur.Y))).SizeW(170),
		flag("grid", "GRID", ws.Grid, func() { ws.Grid = !ws.Grid }),
		flag("snap", "OSNAP", s.SnapOn, func() { s.SnapOn = !s.SnapOn }),
		flag("ortho", "ORTHO", s.Ortho, func() { s.Ortho = !s.Ortho }),
		gpui.NewDiv().Flex1(),
		gpui.NewText(fmt.Sprintf("Layer %s   ·   %d entities   ·   1 px = %s units", s.Drawing().CurrentLayer, s.Drawing().Len(), cmdsys.FormatNumber(ws.view.PixelSize()))).TextColor(uiMuted),
	)
}

func gpuiColor(c render.Color) gpui.Rgba { return gpui.RgbaOf(c.R, c.G, c.B, c.A) }

// FitOnFirstFrame zooms to the drawing's extents once the canvas has its
// size (after opening a file or running a startup script).
func (ws *Workspace) FitOnFirstFrame() { ws.fitPending = true }
