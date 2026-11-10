package ui

import (
	"strings"

	"cad-lite/analytics"
	"cad-lite/cmdsys"
	"cad-lite/doc"
	"cad-lite/i18n"
	"cad-lite/internal/gpui"
	"cad-lite/render"
)

// Workspace is the root view: the open drawings, one document tab each, and the window chrome
// around the canvas. The per-drawing state (session, view, display cache, canvas state, file) is
// the embedded active docTab; the rest (ribbon, panels, settings, agent panel, recent files, file
// dialog, command line field) is the shell every tab shares.
type Workspace struct {
	// docTab is the active tab; its fields (session, view, builder, canvas, ...) read as the
	// workspace's own.
	*docTab
	// tabs are the open drawings in strip order; docTab is one of them.
	tabs []*docTab
	// files is the handler OPEN and SAVE use; it also lists the extensions the file dialogs show.
	files cmdsys.FileHandler
	// dlg is the native file dialog state.
	dlg   dialogState
	input *gpui.TextInput
	// agent is created the first time the panel or the AI command is used.
	agent *AgentPanel
	win   *gpui.Window

	// WheelZoom makes a precise (trackpad) vertical scroll zoom instead of
	// pan, so the plain wheel always zooms as in AutoCAD. Ctrl+wheel zooms
	// and Shift+wheel pans either way.
	WheelZoom bool
	// Brand is what leads the title bar: the icon with the wordmark, the icon alone or nothing.
	Brand Brand
	// tipTheme is the theme whose tooltip style the App currently has.
	tipTheme string
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
	// recent is the Open menu and start panel state; the lists are in the settings.
	recent recentState
	// quit is the host's way of ending the application (SetQuitHandler); quitting asks about each
	// dirty tab first (doctabs.go).
	quit quitState
	// aidsSet is true once the settings' drawing aids were applied; a new tab takes them too.
	aidsSet    bool
	textEditor *textEditState
	// discard is the unsaved-changes dialog while it is open (discard.go).
	discard *discardDialog
	// closeHooked is set once the window's close button asks windowClosing.
	closeHooked bool
	fb          *feedbackState
	// plt is the plot dialog (plot.go), made on first use; printer, printerList and viewer replace
	// plot.Print, plot.Printers and plot.OpenViewer in tests.
	plt         *plotState
	printer     Printer
	printerList PrinterLister
	viewer      Viewer
	analytics   analytics.Recorder
	// pe is the Properties panel's editing state (propsgrid.go), made on first use.
	pe *propsEdit
	// frameN counts the frames built; building is set while Render builds one, when the
	// selection cannot change (syncSel compares it once per frame).
	frameN   int
	building bool
}

// NewWorkspace returns a workspace editing d in its one tab. files may be nil, which
// disables OPEN and SAVE (as in a browser build).
func NewWorkspace(d *doc.Drawing, files cmdsys.FileHandler) *Workspace {
	ws := &Workspace{files: files}
	ws.lay.col = gpui.NewScrollHandle()
	ws.docTab = ws.newDocTab(d)
	ws.input = themeBareField(gpui.NewTextInput(i18n.T("ui.cmdline.placeholder")), fontUI+1, true)
	ws.wireCommandLine()
	return ws
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
	// The answer may close this tab or open another (CLOSE, OPEN), so the tab is held here.
	t := ws.docTab
	n := t.session.RunCount()
	t.session.Submit(text)
	ws.noteIfRan(t, n)
	t.builder.SyncLayers()
	w.Refresh()
}

func (ws *Workspace) escape(w *gpui.Window) {
	// Escape first drops the Properties panel's layer target, as the × beside its title does.
	if ws.session.Active() == "" && ws.input.Text() == "" && ws.clearLayerTarget(w) {
		return
	}
	ws.cmd.open = false
	ws.recent.menu = false
	ws.dismissStart()
	ws.input.SetText("")
	ws.session.Escape()
	w.Refresh()
}

func (ws *Workspace) run(name string, w *gpui.Window) {
	// A command started from the UI (Save, New, Close, a ribbon button) sees the panel's open edit.
	ws.flushPropEdit(w)
	ws.input.SetText("")
	t := ws.docTab
	n := t.session.RunCount()
	t.session.Run(name)
	ws.noteIfRan(t, n)
	w.Refresh()
}

// noteIfRan records the last started command as recent when tab t's session started one since
// the run count was n.
func (ws *Workspace) noteIfRan(t *docTab, n int) {
	if t.session.RunCount() > n {
		ws.noteCommand(t.session.LastCommand())
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
	actionNextTab   = gpui.NewAction("cad-next-tab")
	actionPrevTab   = gpui.NewAction("cad-prev-tab")
	actionPlot      = gpui.NewAction("cad-plot")
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
		gpui.NewKeyBinding("ctrl-p", actionPlot, "cad"),
		gpui.NewKeyBinding("cmd-p", actionPlot, "cad"),
		gpui.NewKeyBinding("cmd-shift-s", actionSaveAs, "cad"),
		gpui.NewKeyBinding("f3", actionSnap, "cad"),
		gpui.NewKeyBinding("f7", actionGrid, "cad"),
		gpui.NewKeyBinding("f8", actionOrtho, "cad"),
		gpui.NewKeyBinding("f9", actionSnapGrid, "cad"),
		gpui.NewKeyBinding("f10", actionPolar, "cad"),
		gpui.NewKeyBinding("ctrl-e", actionZoomExtnt, "cad"),
		gpui.NewKeyBinding("f2", actionAgent, "cad"),
		gpui.NewKeyBinding("ctrl-tab", actionNextTab, "cad"),
		gpui.NewKeyBinding("ctrl-shift-tab", actionPrevTab, "cad"),
	}, settingsBindings()...)
}

// Render builds the window for the current state, with the unsaved-changes dialog over it while
// it is open.
func (ws *Workspace) Render(w *gpui.Window, cx *gpui.App) gpui.IntoElement {
	ws.win = w
	ws.frameN++
	ws.building = true
	defer func() { ws.building = false }()
	if !ws.closeHooked {
		ws.closeHooked = true
		w.OnShouldClose(ws.windowClosing)
	}
	return ws.withDiscard(w, cx, ws.withPlot(w, cx, ws.render(w, cx)))
}

// render is Render without the unsaved-changes dialog.
func (ws *Workspace) render(w *gpui.Window, cx *gpui.App) gpui.IntoElement {
	if t := CurrentTheme(); ws.tipTheme != t.Name {
		ws.tipTheme = t.Name
		cx.SetTooltipStyle(tooltipStyle(t))
	}
	win := float32(w.Bounds().Size.W)
	ws.winH = float32(w.Bounds().Size.H)
	ws.winW = win
	if ws.fb != nil && ws.fb.open {
		return ws.renderFeedback(w, cx)
	}
	if p := ws.prefs(); p.open && p.snapshot != nil {
		snapshot := p.snapshot
		frozen := gpui.NewCanvas(func(b gpui.Bounds, w *gpui.Window, _ *gpui.App) { w.PaintImageVersion(b, snapshot, 1) })
		return ws.withSettings(cx, gpui.NewDiv().KeyContext("cad").FlexCol().SizeFull().Bg(uiBg).TextColor(uiText).TextSize(13).Child(frozen))
	}
	ws.validateTextEditor(w)
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
	return ws.withTextEditor(w, cx, ws.withSettings(cx, root.
		OnAction(actionOpen, func(_ gpui.Action, w *gpui.Window, _ *gpui.App) { ws.openFile(w) }).
		OnAction(actionSave, func(_ gpui.Action, w *gpui.Window, _ *gpui.App) { ws.saveFile(w) }).
		OnAction(actionSaveAs, func(_ gpui.Action, w *gpui.Window, _ *gpui.App) { ws.saveFileAs(w) }).
		OnAction(actionSnap, toggle(func() { s.SnapOn = !s.SnapOn })).
		OnAction(actionOrtho, toggle(func() { s.SetOrtho(!s.Ortho) })).
		OnAction(actionPolar, toggle(func() { s.SetPolar(!s.Polar()) })).
		OnAction(actionAgent, func(_ gpui.Action, w *gpui.Window, _ *gpui.App) { ws.AgentPanel().Toggle(w) }).
		OnAction(actionNextTab, toggle(func() { ws.cycleTab(1) })).
		OnAction(actionPrevTab, toggle(func() { ws.cycleTab(-1) })).
		OnAction(actionGrid, toggle(func() { s.SetGrid(!s.Grid()) })).
		OnAction(actionSnapGrid, toggle(func() { s.SetSnapGrid(!s.SnapGrid()) })).
		OnAction(actionZoomExtnt, func(_ gpui.Action, w *gpui.Window, _ *gpui.App) { ws.run("ZOOM", w); ws.submit("e", w) }).
		OnMouseMove(func(e *gpui.MouseMoveEvent, w *gpui.Window, _ *gpui.App) {
			if ws.movePropsHUD(e.Position, w) {
				return
			}
			if ws.textEditor != nil {
				return
			}
			ws.zoom.Move(e.Position, w)
			ws.dragMove(e.Position, w)
		}).
		OnMouseUp(func(e *gpui.MouseUpEvent, w *gpui.Window, _ *gpui.App) {
			if e.Button == gpui.MouseLeft {
				ws.endPropsDrag(w)
			}
			ws.zoom.End(w)
			ws.endDrag(w)
		}).
		OnFileDrop(ws.onFileDrop).
		// Delete with the focus outside any text field (a text field keeps its editing keys).
		OnKeyDown(func(e *gpui.KeyDownEvent, w *gpui.Window, cx *gpui.App) {
			if ws.eraseKey(e, w, cx) {
				w.StopPropagation()
			}
		}).
		Child(ws.chrome(win, sideW, agentW, cmdline, w, cx)...).
		Child(ws.statusBar(), ws.openMenuOverlay(), ws.propsMenuOverlay(w, cx))))
}

// chrome returns the window's rows above the status bar: the title bar, the ribbon unless it is
// collapsed, and the body, each followed by a divider. The body's left column is the document
// tab strip, the canvas and the command area.
func (ws *Workspace) chrome(win, sideW, agentW float32, cmdline gpui.IntoElement, w *gpui.Window, cx *gpui.App) []gpui.IntoElement {
	rows := []gpui.IntoElement{ws.titleBar(), hDivider()}
	if !ws.lay.ribbonOff {
		rows = append(rows, ws.ribbon(win), hDivider())
	}
	body := gpui.NewDiv().FlexRow().Flex1().MinH(0).Child(
		gpui.NewDiv().FlexCol().Flex1().MinW(0).Child(
			ws.tabStrip(),
			ws.canvasElement(),
			hDivider(),
			ws.commandArea(cmdline, ws.mainWidth(win, sideW, agentW), w),
		))
	l := &ws.lay
	if l.sideOff {
		body.Child(gpui.NewDiv().SizeW(1).Bg(uiBorder).FlexShrink(0))
	} else {
		body.Child(ws.splitter("split-side", splitSide, false, i18n.T("ui.panel.side_splitter_tip"),
			func() float32 { s, _ := ws.widths(ws.winW); return s }, func() { l.sideW = 0 }))
	}
	body.Child(ws.sidePanel(sideW, w, cx))
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
