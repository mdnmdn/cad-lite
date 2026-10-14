package ui

import (
	"fmt"
	"strings"

	"go-draw/assets/icons"
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
	// agent is created the first time the panel or the AI command is used.
	agent *AgentPanel
	// projectPath is the drawing's file, which decides where agent history lives.
	projectPath string
	win         *gpui.Window

	canvas canvasState
	// Grid toggles the background grid (F7).
	Grid bool
	// WheelZoom makes a precise (trackpad) vertical scroll zoom instead of
	// pan, so the plain wheel always zooms as in AutoCAD. Ctrl+wheel zooms
	// and Shift+wheel pans either way.
	WheelZoom bool
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
	ws.session.OnFilePath(ws.pathChanged)
	ws.session.SetAgentHandler(ws.agentCommand)
	ws.input = themeField(gpui.NewTextInput("Type a command"))
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
	actionAgent     = gpui.NewAction("cad-toggle-agent")
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
		gpui.NewKeyBinding("f2", actionAgent, "cad"),
	}
}

// Render builds the window for the current state.
func (ws *Workspace) Render(w *gpui.Window, cx *gpui.App) gpui.IntoElement {
	ws.win = w
	var agentPanel gpui.IntoElement = gpui.NewDiv()
	if ws.agent != nil && ws.agent.visible {
		agentPanel = ws.agent.render(w, cx)
	}
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
		OnAction(actionAgent, func(_ gpui.Action, w *gpui.Window, _ *gpui.App) { ws.AgentPanel().Toggle(w) }).
		OnAction(actionGrid, toggle(func() { ws.Grid = !ws.Grid })).
		OnAction(actionZoomExtnt, func(_ gpui.Action, w *gpui.Window, _ *gpui.App) { ws.run("ZOOM", w); ws.submit("e", w) }).
		Child(
			ws.toolbar(),
			gpui.NewDiv().FlexRow().Flex1().MinH(0).Child(
				ws.canvasElement(),
				ws.sidePanel(),
				agentPanel,
			),
			ws.commandArea(cmdline),
			ws.statusBar(),
		)
}

// toolIconSize is the toolbar icon edge in logical pixels.
const toolIconSize = 18

func (ws *Workspace) button(id, label, command string, icon icons.Name) *gpui.Div {
	active := ws.session.Active() == command
	b := gpui.NewDiv().ID("tool-"+id).FlexCol().ItemsCenter().Gap(2).Px_(6).Py_(4).Rounded(4).
		Hover(func(m *gpui.StyledMixin) *gpui.StyledMixin { return m.Bg(uiPanelAlt) }).
		OnClick(func(_ *gpui.ClickEvent, w *gpui.Window, _ *gpui.App) { ws.run(command, w) }).
		Child(NewIcon(icon, toolIconSize, uiText), gpui.NewText(label).TextSize(10))
	if active {
		b.Bg(uiAccent)
	}
	return b
}

func separator() *gpui.Div { return gpui.NewDiv().SizeWH(1, 28).Bg(uiBorder).Mx(4) }

func (ws *Workspace) toolbar() *gpui.Div {
	return gpui.NewDiv().FlexRow().ItemsCenter().Gap(2).Px_(8).Py_(4).Bg(uiPanel).Child(
		gpui.NewText("go-draw").FontWeight(700).TextColor(uiOn).Pr(10),
		ws.button("line", "Line", "LINE", icons.DrawLine),
		ws.button("pline", "Polyline", "PLINE", icons.DrawPolyline),
		ws.button("circle", "Circle", "CIRCLE", icons.DrawCircle),
		ws.button("arc", "Arc", "ARC", icons.DrawArc),
		ws.button("rect", "Rect", "RECTANG", icons.DrawRectangle),
		separator(),
		ws.button("move", "Move", "MOVE", icons.ModifyMove),
		ws.button("copy", "Copy", "COPY", icons.ModifyCopy),
		ws.button("rotate", "Rotate", "ROTATE", icons.ModifyRotate),
		ws.button("scale", "Scale", "SCALE", icons.ModifyScale),
		ws.button("erase", "Erase", "ERASE", icons.ModifyErase),
		separator(),
		ws.button("dist", "Dist", "DIST", icons.MeasureDistance),
		ws.button("area", "Area", "AREA", icons.MeasureArea),
		separator(),
		ws.button("undo", "Undo", "UNDO", icons.EditUndo),
		ws.button("redo", "Redo", "REDO", icons.EditRedo),
		ws.button("zoom", "Zoom", "ZOOM", icons.ViewZoomWindow),
		gpui.NewDiv().Flex1(),
		ws.agentToggle(),
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
			gpui.NewDiv().Flex1().FlexCol().Child(cmdline),
		),
	)
}

func (ws *Workspace) statusBar() *gpui.Div {
	s := ws.session
	cur := s.Cursor()
	coords := "—"
	if ws.pointerOnCanvas() {
		coords = fmt.Sprintf("%s, %s", cmdsys.FormatNumber(cur.X), cmdsys.FormatNumber(cur.Y))
	}
	flag := func(id, label string, icon icons.Name, on bool, f func()) *gpui.Div {
		c := uiMuted
		if on {
			c = uiOn
		}
		return gpui.NewDiv().ID("status-"+id).FlexRow().ItemsCenter().Gap(4).Px_(8).Py_(2).Rounded(3).
			Hover(func(m *gpui.StyledMixin) *gpui.StyledMixin { return m.Bg(uiPanelAlt) }).
			OnClick(func(_ *gpui.ClickEvent, w *gpui.Window, _ *gpui.App) { f(); w.Refresh() }).
			Child(NewIcon(icon, 14, c), gpui.NewText(label).FontWeight(700).TextColor(c))
	}
	return gpui.NewDiv().FlexRow().ItemsCenter().Gap(6).Px_(10).Py_(4).Bg(uiBg).TextSize(12).Child(
		gpui.NewText(coords).SizeW(170),
		flag("grid", "GRID", icons.ViewGrid, ws.Grid, func() { ws.Grid = !ws.Grid }),
		flag("snap", "OSNAP", icons.SnapOsnap, s.SnapOn, func() { s.SnapOn = !s.SnapOn }),
		flag("ortho", "ORTHO", icons.SnapOrtho, s.Ortho, func() { s.Ortho = !s.Ortho }),
		gpui.NewDiv().Flex1(),
		gpui.NewText(fmt.Sprintf("Layer %s   ·   %d entities   ·   1 px = %s units", s.Drawing().CurrentLayer, s.Drawing().Len(), cmdsys.FormatNumber(ws.view.PixelSize()))).TextColor(uiMuted),
	)
}

func gpuiColor(c render.Color) gpui.Rgba { return gpui.RgbaOf(c.R, c.G, c.B, c.A) }

// FitOnFirstFrame zooms to the drawing's extents once the canvas has its
// size (after opening a file or running a startup script).
func (ws *Workspace) FitOnFirstFrame() { ws.fitPending = true }

// agentCommand is the AI command's entry point: an empty prompt toggles the
// panel, text opens it and sends the text to the active conversation.
func (ws *Workspace) agentCommand(prompt string) {
	p := ws.AgentPanel()
	prompt = strings.TrimSpace(prompt)
	if prompt == "" {
		p.visible = !p.visible
		if p.visible {
			p.ensureLoaded()
		}
		if ws.win != nil {
			ws.win.Refresh()
		}
		return
	}
	p.Show()
	if ws.win == nil {
		p.deferred = prompt
		return
	}
	p.sendFromCommand(prompt, ws.win)
}
