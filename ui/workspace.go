package ui

import (
	"fmt"
	"strings"

	"go-draw/assets/icons"
	"go-draw/cmdsys"
	"go-draw/cmdsys/commands"
	"go-draw/doc"
	"go-draw/i18n"
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
	// Brand is what leads the toolbar: the wordmark, the app icon or nothing.
	Brand Brand
	// tipTheme is the theme whose tooltip style the App currently has.
	tipTheme string
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
	ws.session.SetSettingsHandler(func() {
		if ws.win != nil {
			ws.ToggleSettings(ws.win)
		}
	})
	ws.input = themeField(gpui.NewTextInput(i18n.T("ui.cmdline.placeholder")))
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
	return append([]gpui.KeyBinding{
		gpui.NewKeyBinding("ctrl-z", actionUndo, "cad"),
		gpui.NewKeyBinding("cmd-z", actionUndo, "cad"),
		gpui.NewKeyBinding("ctrl-y", actionRedo, "cad"),
		gpui.NewKeyBinding("cmd-shift-z", actionRedo, "cad"),
		gpui.NewKeyBinding("f3", actionSnap, "cad"),
		gpui.NewKeyBinding("f7", actionGrid, "cad"),
		gpui.NewKeyBinding("f8", actionOrtho, "cad"),
		gpui.NewKeyBinding("ctrl-e", actionZoomExtnt, "cad"),
		gpui.NewKeyBinding("f2", actionAgent, "cad"),
	}, settingsBindings()...)
}

// Render builds the window for the current state.
func (ws *Workspace) Render(w *gpui.Window, cx *gpui.App) gpui.IntoElement {
	ws.win = w
	if t := CurrentTheme(); ws.tipTheme != t.Name {
		ws.tipTheme = t.Name
		cx.SetTooltipStyle(tooltipStyle(t))
	}
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
	return ws.withSettings(cx, gpui.NewDiv().KeyContext("cad").
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
		))
}

// toolIconSize is the toolbar icon edge in logical pixels.
const toolIconSize = 18

// button is a toolbar command button: icon over label, with the command's
// tooltip. label is the already translated caption; an empty one uses the
// command's display name.
func (ws *Workspace) button(id, label, command string, icon icons.Name) *gpui.Div {
	active := ws.session.Active() == command
	info := i18n.Command(command)
	if label == "" {
		label = info.Name
	}
	b := gpui.NewDiv().ID("tool-"+id).FlexCol().ItemsCenter().Gap(2).Px_(6).Py_(4).Rounded(4).Tooltip(info.Tip).
		Hover(func(m *gpui.StyledMixin) *gpui.StyledMixin { return m.Bg(uiPanelAlt) }).
		OnClick(func(_ *gpui.ClickEvent, w *gpui.Window, _ *gpui.App) { ws.run(command, w) }).
		Child(NewIcon(icon, toolIconSize, uiText), gpui.NewText(label).TextSize(10))
	if active {
		b.Bg(uiAccent)
	}
	return b
}

func separator() *gpui.Div { return gpui.NewDiv().SizeWH(1, 28).Bg(uiBorder).Mx(4) }

// tb returns the translated toolbar caption for key.
func tb(key string) string { return i18n.T("ui.toolbar." + key) }

// toolbar is two rows: drawing, measuring, history and the window buttons on
// top, the modify commands below. Commands without a short caption of their
// own show their display name.
func (ws *Workspace) toolbar() *gpui.Div {
	top := gpui.NewDiv().FlexRow().ItemsCenter().Gap(2)
	if b := ws.brand(); b != nil {
		top.Child(b)
	}
	top.Child(
		ws.button("line", tb("line"), "LINE", icons.DrawLine),
		ws.button("pline", tb("pline"), "PLINE", icons.DrawPolyline),
		ws.button("circle", tb("circle"), "CIRCLE", icons.DrawCircle),
		ws.button("arc", tb("arc"), "ARC", icons.DrawArc),
		ws.button("rect", tb("rect"), "RECTANG", icons.DrawRectangle),
		separator(),
		ws.button("dist", tb("dist"), "DIST", icons.MeasureDistance),
		ws.button("area", tb("area"), "AREA", icons.MeasureArea),
		ws.button("id", "", "ID", icons.MeasureIdPoint),
		separator(),
		ws.button("undo", tb("undo"), "UNDO", icons.EditUndo),
		ws.button("redo", tb("redo"), "REDO", icons.EditRedo),
		ws.button("zoom", tb("zoom"), "ZOOM", icons.ViewZoomWindow),
		gpui.NewDiv().Flex1(),
		ws.agentToggle(),
		ws.settingsButton(),
	)
	modify := gpui.NewDiv().FlexRow().ItemsCenter().Gap(2).Child(
		ws.button("move", tb("move"), "MOVE", icons.ModifyMove),
		ws.button("copy", tb("copy"), "COPY", icons.ModifyCopy),
		ws.button("rotate", tb("rotate"), "ROTATE", icons.ModifyRotate),
		ws.button("scale", tb("scale"), "SCALE", icons.ModifyScale),
		ws.button("erase", tb("erase"), "ERASE", icons.ModifyErase),
		separator(),
		ws.button("mirror", "", "MIRROR", icons.ModifyMirror),
		ws.button("offset", "", "OFFSET", icons.ModifyOffset),
		ws.button("fillet", "", "FILLET", icons.ModifyFillet),
		ws.button("chamfer", "", "CHAMFER", icons.ModifyChamfer),
		separator(),
		ws.button("trim", "", "TRIM", icons.ModifyTrim),
		ws.button("extend", "", "EXTEND", icons.ModifyExtend),
		ws.button("break", "", "BREAK", icons.ModifyBreak),
		ws.button("join", "", "JOIN", icons.ModifyJoin),
		ws.button("explode", "", "EXPLODE", icons.ModifyExplode),
	)
	return gpui.NewDiv().FlexCol().Px_(8).Py_(4).Bg(uiPanel).Child(top, modify)
}

func (ws *Workspace) sidePanel() *gpui.Div {
	d := ws.session.Drawing()
	layers := gpui.NewDiv().FlexCol().Gap(2)
	for _, l := range d.Layers() {
		l := l
		swatch := gpui.NewDiv().SizeWH(12, 12).Rounded(2).Bg(gpuiColor(render.ACI(l.Color))).Tooltip(i18n.T("ui.layers.swatch_tip", l.Color))
		name := gpui.NewText(l.Name).Flex1()
		if l.Name == d.CurrentLayer {
			name.FontWeight(700).TextColor(uiOn)
		}
		state := i18n.T("ui.layers.on")
		stateCol := uiText
		if l.Off {
			state, stateCol = i18n.T("ui.layers.off"), uiMuted
		}
		vis := gpui.NewDiv().ID("layer-vis-" + l.Name).Px_(6).Rounded(3).Bg(uiPanelAlt).Tooltip(i18n.T("ui.layers.visibility_tip")).
			OnClick(func(_ *gpui.ClickEvent, w *gpui.Window, _ *gpui.App) {
				l.Off = !l.Off
				w.Refresh()
			}).
			Child(gpui.NewText(state).TextColor(stateCol))
		layers.Child(gpui.NewDiv().ID("layer-"+l.Name).FlexRow().ItemsCenter().Gap(8).Px_(6).Py_(4).Rounded(4).Tooltip(i18n.T("ui.layers.current_tip")).
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
		props.Child(gpui.NewText(i18n.T("ui.props.none")).TextColor(uiMuted))
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
		props.Child(gpui.NewText(i18n.Plural("ui.props.selected", sel.Len())).FontWeight(700))
		for _, c := range order {
			props.Child(gpui.NewText(fmt.Sprintf("%s × %d", c, counts[c])).TextColor(uiMuted))
		}
		if sel.Len() == 1 {
			e := d.Entity(sel.Handles()[0])
			props.Child(gpui.NewText(i18n.T("ui.props.layer", e.Common().Layer)).TextColor(uiMuted),
				gpui.NewText(i18n.T("ui.props.handle", e.Handle().String())).TextColor(uiMuted))
		}
	}
	heading := func(s string) *gpui.Text {
		return gpui.NewText(s).FontWeight(700).TextSize(11).TextColor(uiMuted)
	}
	return gpui.NewDiv().FlexCol().SizeW(210).Gap(8).P(10).Bg(uiPanel).
		Child(heading(i18n.T("ui.panel.layers")), layers, heading(i18n.T("ui.panel.properties")), props)
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
	flag := func(id, label, tip string, icon icons.Name, on bool, f func()) *gpui.Div {
		c := uiMuted
		if on {
			c = uiOn
		}
		return gpui.NewDiv().ID("status-"+id).FlexRow().ItemsCenter().Gap(4).Px_(8).Py_(2).Rounded(3).Tooltip(tip).
			Hover(func(m *gpui.StyledMixin) *gpui.StyledMixin { return m.Bg(uiPanelAlt) }).
			OnClick(func(_ *gpui.ClickEvent, w *gpui.Window, _ *gpui.App) { f(); w.Refresh() }).
			Child(NewIcon(icon, 14, c), gpui.NewText(label).FontWeight(700).TextColor(c))
	}
	return gpui.NewDiv().FlexRow().ItemsCenter().Gap(6).Px_(10).Py_(4).Bg(uiBg).TextSize(12).Child(
		gpui.NewDiv().SizeW(170).Tooltip(i18n.T("ui.status.coords_tip")).Child(gpui.NewText(coords)),
		flag("grid", i18n.T("ui.status.grid"), i18n.T("ui.status.grid_tip"), icons.ViewGrid, ws.Grid, func() { ws.Grid = !ws.Grid }),
		flag("snap", i18n.T("ui.status.osnap"), i18n.T("ui.status.osnap_tip"), icons.SnapOsnap, s.SnapOn, func() { s.SnapOn = !s.SnapOn }),
		flag("ortho", i18n.T("ui.status.ortho"), i18n.T("ui.status.ortho_tip"), icons.SnapOrtho, s.Ortho, func() { s.Ortho = !s.Ortho }),
		gpui.NewDiv().Flex1(),
		gpui.NewText(i18n.T("ui.status.summary", s.Drawing().CurrentLayer, s.Drawing().Len(), cmdsys.FormatNumber(ws.view.PixelSize()))).TextColor(uiMuted),
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
