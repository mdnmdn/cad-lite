package ui

import (
	"strings"

	"cad-lite/cmdsys"
	"cad-lite/cmdsys/commands"
	"cad-lite/doc"
	"cad-lite/i18n"
	"cad-lite/internal/gpui"
	"cad-lite/render"
)

// Workspace is the root view: one drawing, its session and the window
// chrome around the canvas.
type Workspace struct {
	session *cmdsys.Session
	// files is the handler OPEN and SAVE use; it also lists the extensions the file dialogs show.
	files cmdsys.FileHandler
	// dlg is the native file dialog state.
	dlg     dialogState
	view    *render.View
	builder *render.Builder
	input   *gpui.TextInput
	// agent is created the first time the panel or the AI command is used.
	agent *AgentPanel
	// projectPath is the drawing's file, which decides where agent history lives.
	projectPath string
	win         *gpui.Window

	canvas canvasState
	// WheelZoom makes a precise (trackpad) vertical scroll zoom instead of
	// pan, so the plain wheel always zooms as in AutoCAD. Ctrl+wheel zooms
	// and Shift+wheel pans either way.
	WheelZoom bool
	// Brand is what leads the title bar: the icon with the wordmark, the icon alone or nothing.
	Brand Brand
	// tipTheme is the theme whose tooltip style the App currently has.
	tipTheme string
	// fitPending zooms to extents once the canvas size is known.
	fitPending bool
	// zoom is the status bar slider's state; zoomSaved is true once the
	// drag in progress has put the pre-drag view on the ZOOM Previous stack.
	zoom      SliderState
	zoomSaved bool
	// winH is the window height of the last frame, which sizes the default history region.
	winH float32
	// winW is the window width of the last frame, which sizes the columns.
	winW float32
	// cmd is the command area's state: history height and recent commands.
	cmd cmdAreaState
	// lay is the panel layout: sizes, collapsed panels and the splitter drag in progress.
	lay layoutState
}

// NewWorkspace returns a workspace editing d. files may be nil, which
// disables OPEN and SAVE (as in a browser build).
func NewWorkspace(d *doc.Drawing, files cmdsys.FileHandler) *Workspace {
	v := render.DefaultView(800, 600)
	ws := &Workspace{view: &v, files: files, fitPending: d.Len() > 0}
	ws.lay.col = gpui.NewScrollHandle()
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
	ws.input = themeBareField(gpui.NewTextInput(i18n.T("ui.cmdline.placeholder")), fontUI+1, true)
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
	n := ws.session.RunCount()
	ws.session.Submit(text)
	ws.noteIfRan(n)
	ws.builder.InvalidateAll()
	w.Refresh()
}

func (ws *Workspace) escape(w *gpui.Window) {
	ws.cmd.open = false
	ws.input.SetText("")
	ws.session.Escape()
	w.Refresh()
}

func (ws *Workspace) run(name string, w *gpui.Window) {
	ws.input.SetText("")
	n := ws.session.RunCount()
	ws.session.Run(name)
	ws.noteIfRan(n)
	w.Refresh()
}

// noteIfRan records the last started command as recent when the session started one since the
// run count was n.
func (ws *Workspace) noteIfRan(n int) {
	if ws.session.RunCount() > n {
		ws.noteCommand(ws.session.LastCommand())
	}
}

// Actions bound by Bindings.
var (
	actionUndo      = gpui.NewAction("cad-undo")
	actionRedo      = gpui.NewAction("cad-redo")
	actionOpen      = gpui.NewAction("cad-open")
	actionSave      = gpui.NewAction("cad-save")
	actionSaveAs    = gpui.NewAction("cad-save-as")
	actionSnap      = gpui.NewAction("cad-toggle-snap")
	actionOrtho     = gpui.NewAction("cad-toggle-ortho")
	actionGrid      = gpui.NewAction("cad-toggle-grid")
	actionSnapGrid  = gpui.NewAction("cad-toggle-snap-grid")
	actionPolar     = gpui.NewAction("cad-toggle-polar")
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
		gpui.NewKeyBinding("ctrl-o", actionOpen, "cad"),
		gpui.NewKeyBinding("cmd-o", actionOpen, "cad"),
		gpui.NewKeyBinding("ctrl-s", actionSave, "cad"),
		gpui.NewKeyBinding("cmd-s", actionSave, "cad"),
		gpui.NewKeyBinding("ctrl-shift-s", actionSaveAs, "cad"),
		gpui.NewKeyBinding("cmd-shift-s", actionSaveAs, "cad"),
		gpui.NewKeyBinding("f3", actionSnap, "cad"),
		gpui.NewKeyBinding("f7", actionGrid, "cad"),
		gpui.NewKeyBinding("f8", actionOrtho, "cad"),
		gpui.NewKeyBinding("f9", actionSnapGrid, "cad"),
		gpui.NewKeyBinding("f10", actionPolar, "cad"),
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
	win := float32(w.Bounds().Size.W)
	ws.winH = float32(w.Bounds().Size.H)
	ws.winW = win
	sideW, agentW := ws.widths(win)
	cmdline := ws.input.Render(w, cx)
	if fh := ws.input.FocusHandle(); fh != nil && w.Focus() == nil {
		w.RequestFocus(fh)
	}
	toggle := func(f func()) func(gpui.Action, *gpui.Window, *gpui.App) {
		return func(_ gpui.Action, w *gpui.Window, _ *gpui.App) { f(); w.Refresh() }
	}
	s := ws.session
	root := gpui.NewDiv().KeyContext("cad").FlexCol().SizeFull().Bg(uiBg).TextColor(uiText).TextSize(13)
	for command, action := range commandKeys {
		root.OnAction(action, func(_ gpui.Action, w *gpui.Window, _ *gpui.App) { ws.run(command, w) })
	}
	return ws.withSettings(cx, root.
		OnAction(actionOpen, func(_ gpui.Action, w *gpui.Window, _ *gpui.App) { ws.openFile(w) }).
		OnAction(actionSave, func(_ gpui.Action, w *gpui.Window, _ *gpui.App) { ws.saveFile(w) }).
		OnAction(actionSaveAs, func(_ gpui.Action, w *gpui.Window, _ *gpui.App) { ws.saveFileAs(w) }).
		OnAction(actionSnap, toggle(func() { s.SnapOn = !s.SnapOn })).
		OnAction(actionOrtho, toggle(func() { s.SetOrtho(!s.Ortho) })).
		OnAction(actionPolar, toggle(func() { s.SetPolar(!s.Polar()) })).
		OnAction(actionAgent, func(_ gpui.Action, w *gpui.Window, _ *gpui.App) { ws.AgentPanel().Toggle(w) }).
		OnAction(actionGrid, toggle(func() { s.SetGrid(!s.Grid()) })).
		OnAction(actionSnapGrid, toggle(func() { s.SetSnapGrid(!s.SnapGrid()) })).
		OnAction(actionZoomExtnt, func(_ gpui.Action, w *gpui.Window, _ *gpui.App) { ws.run("ZOOM", w); ws.submit("e", w) }).
		OnMouseMove(func(e *gpui.MouseMoveEvent, w *gpui.Window, _ *gpui.App) {
			ws.zoom.Move(e.Position, w)
			ws.dragMove(e.Position, w)
		}).
		OnMouseUp(func(_ *gpui.MouseUpEvent, w *gpui.Window, _ *gpui.App) {
			ws.zoom.End(w)
			ws.endDrag(w)
		}).
		Child(ws.chrome(win, sideW, agentW, cmdline, w, cx)...).
		Child(ws.statusBar()))
}

// chrome returns the window's rows above the status bar: the title bar, the ribbon unless it is
// collapsed, and the body, each followed by a divider.
func (ws *Workspace) chrome(win, sideW, agentW float32, cmdline gpui.IntoElement, w *gpui.Window, cx *gpui.App) []gpui.IntoElement {
	rows := []gpui.IntoElement{ws.titleBar(), hDivider()}
	if !ws.lay.ribbonOff {
		rows = append(rows, ws.ribbon(win), hDivider())
	}
	body := gpui.NewDiv().FlexRow().Flex1().MinH(0).Child(
		gpui.NewDiv().FlexCol().Flex1().MinW(0).Child(
			ws.canvasElement(),
			hDivider(),
			ws.commandArea(cmdline, w),
		))
	l := &ws.lay
	if l.sideOff {
		body.Child(gpui.NewDiv().SizeW(1).Bg(uiBorder).FlexShrink(0))
	} else {
		body.Child(ws.splitter("split-side", splitSide, false, i18n.T("ui.panel.side_splitter_tip"),
			func() float32 { s, _ := ws.widths(ws.winW); return s }, func() { l.sideW = 0 }))
	}
	body.Child(ws.sidePanel(sideW))
	if ws.agentShown() {
		body.Child(ws.splitter("split-agent", splitAgent, false, i18n.T("ui.panel.agent_splitter_tip"),
			func() float32 { _, a := ws.widths(ws.winW); return a }, func() { l.agentW = 0 }),
			ws.agent.render(w, cx, agentW))
	}
	return append(rows, body, hDivider())
}

// tb returns the translated toolbar caption for key.
func tb(key string) string { return i18n.T("ui.toolbar." + key) }

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
