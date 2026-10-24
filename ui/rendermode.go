package ui

import (
	"runtime"
	"sync"
	"weak"

	"cad-lite/i18n"
	"cad-lite/internal/gpui"
	"cad-lite/settings"
)

// RenderInfo is how this session presents its frames, as the host decided at
// startup. Frames are rasterized on the CPU in every mode; the renderer only
// picks the graphics device that shows them.
type RenderInfo struct {
	// Active is the renderer in use: settings.RendererGPU or
	// settings.RendererSoftware. Empty when the host has no choice (macOS,
	// the browser, tests), which hides the restart offer.
	Active string
	// Forced is set when the -render flag or CADLITE_RENDER chose the
	// renderer, so the setting does not apply to this session.
	Forced bool
	// Fallback is set when software rendering follows a failed start on the
	// graphics card; the workspace then shows a one-time notice.
	Fallback bool
}

// rendererRowShown says whether the settings panel offers the renderer:
// the browser has no software switch. Tests set it.
var rendererRowShown = runtime.GOOS != "js"

// rendererMac selects the macOS wording of the software hint (OpenGL instead
// of Metal). Tests set it.
var rendererMac = runtime.GOOS == "darwin"

// renderState is a workspace's render information and the notice's state.
type renderState struct {
	info RenderInfo
	// notice is set while the fallback notice shows.
	notice bool
	// err is the last failed restart request, shown under the renderer row.
	err error
}

var renderTable = struct {
	sync.Mutex
	m map[weak.Pointer[Workspace]]*renderState
}{m: map[weak.Pointer[Workspace]]*renderState{}}

// rnd returns ws's render state, creating it on first use. Like prefs, it
// lives beside the Workspace so the Workspace stays collectable.
func (ws *Workspace) rnd() *renderState {
	key := weak.Make(ws)
	renderTable.Lock()
	defer renderTable.Unlock()
	if r, ok := renderTable.m[key]; ok {
		return r
	}
	r := &renderState{}
	renderTable.m[key] = r
	runtime.AddCleanup(ws, func(k weak.Pointer[Workspace]) {
		renderTable.Lock()
		delete(renderTable.m, k)
		renderTable.Unlock()
	}, key)
	return r
}

// SetRenderInfo tells the workspace how the host presents its frames; with
// Fallback set it shows the one-time software rendering notice.
func (ws *Workspace) SetRenderInfo(r RenderInfo) {
	s := ws.rnd()
	s.info = r
	s.notice = r.Fallback
}

// RenderInfo returns what SetRenderInfo set.
func (ws *Workspace) RenderInfo() RenderInfo { return ws.rnd().info }

// RenderNoticeShown reports whether the software rendering notice is showing.
func (ws *Workspace) RenderNoticeShown() bool { return ws.rnd().notice }

// rendererWanted is the renderer setting s asks for in this session: auto is
// satisfied by software after a fallback and asks for the graphics card
// otherwise.
func rendererWanted(s string, info RenderInfo) string {
	if s == settings.RendererAuto {
		if info.Fallback {
			return settings.RendererSoftware
		}
		return settings.RendererGPU
	}
	return s
}

// rendererRestartNeeded reports whether the renderer setting differs from the
// renderer in use, so a restart would change it.
func (ws *Workspace) rendererRestartNeeded() bool {
	info := ws.rnd().info
	return !info.Forced && info.Active != "" && rendererWanted(ws.prefs().cur.Renderer, info) != info.Active
}

// restartForRenderer asks the host to restart the application (see RequestRestart).
func (ws *Workspace) restartForRenderer(w *gpui.Window, cx *gpui.App) {
	s := ws.rnd()
	s.err = ws.RequestRestart(cx)
	w.Refresh()
}

// rendererHint is the line under the renderer choice r; on macOS software means OpenGL.
func rendererHint(r string, mac bool) string {
	if mac && r == settings.RendererSoftware {
		return i18n.T("ui.settings.renderer_hint_software_mac")
	}
	return map[string]string{
		settings.RendererAuto:     i18n.T("ui.settings.renderer_hint_auto"),
		settings.RendererGPU:      i18n.T("ui.settings.renderer_hint_gpu"),
		settings.RendererSoftware: i18n.T("ui.settings.renderer_hint_software"),
	}[r]
}

// displaySection is the settings panel's renderer row, nil where there is no
// choice to make.
func (ws *Workspace) displaySection() *gpui.Div {
	if !rendererRowShown {
		return nil
	}
	c := ws.prefs().cur
	r := ws.rnd()
	tip := i18n.T("ui.settings.renderer_tip")
	seg := segmented("renderer-", [][2]string{
		{settings.RendererAuto, i18n.T("ui.settings.renderer_auto")},
		{settings.RendererGPU, i18n.T("ui.settings.renderer_gpu")},
		{settings.RendererSoftware, i18n.T("ui.settings.renderer_software")},
	}, tip, c.Renderer, func(v string, w *gpui.Window) { ws.change(w, func(s *settings.Settings) { s.Renderer = v }) })
	hint := rendererHint(c.Renderer, rendererMac)
	col := gpui.NewDiv().FlexCol().Gap(4).Flex1().Child(seg, gpui.NewText(hint).TextSize(11).TextColor(uiMuted))
	switch {
	case r.info.Forced:
		col.Child(gpui.NewText(i18n.T("ui.settings.renderer_forced")).TextSize(11).TextColor(uiWarn))
	default:
		col.Child(gpui.NewText(i18n.T("ui.settings.renderer_restart")).TextSize(11).TextColor(uiMuted))
	}
	if r.info.Active != "" {
		col.Child(gpui.NewText(i18n.T("ui.settings.renderer_active", i18n.T("ui.settings.renderer_"+r.info.Active))).TextSize(11).TextColor(uiMuted))
	}
	if ws.rendererRestartNeeded() {
		col.Child(gpui.NewDiv().FlexRow().Child(
			actionButton("renderer-restart", i18n.T("ui.settings.renderer_restart_now"), i18n.T("ui.settings.renderer_restart_now_tip"), true, true,
				ws.restartForRenderer)))
	}
	if r.err != nil {
		col.Child(gpui.NewText(r.err.Error()).TextSize(11).TextColor(uiErr))
	}
	return gpui.NewDiv().ID("settings-sec-display").FlexCol().Gap(settingsRowGap).Child(
		sectionTitle(i18n.T("ui.settings.section_display")),
		settingRow(i18n.T("ui.settings.renderer"), tip, col),
	)
}

// renderNotice is the one-time card that says software rendering follows a
// failed graphics start, with a button that keeps software rendering for
// good. It does not block the window.
func (ws *Workspace) renderNotice() gpui.IntoElement {
	r := ws.rnd()
	if !r.notice {
		return gpui.NewDiv().Absolute().SizeWH(0, 0)
	}
	dismiss := func(w *gpui.Window) {
		r.notice = false
		w.Refresh()
	}
	return gpui.NewDiv().ID("render-notice").Absolute().Top(48).Right(16).SizeW(380).FlexCol().Gap(10).P(14).
		Bg(uiPanel).Border(1, uiWarn).Rounded(8).TextColor(uiText).TextSize(13).
		OnClick(func(_ *gpui.ClickEvent, w *gpui.Window, _ *gpui.App) { w.StopPropagation() }).
		Child(
			gpui.NewText(i18n.T("ui.render.fallback_title")).FontWeight(700),
			gpui.NewText(i18n.T("ui.render.fallback_body")),
			gpui.NewDiv().FlexRow().Gap(8).JustifyEnd().Child(
				actionButton("render-notice-ok", i18n.T("ui.render.fallback_dismiss"), i18n.T("ui.render.fallback_dismiss_tip"), false, true,
					func(w *gpui.Window, _ *gpui.App) { dismiss(w) }),
				actionButton("render-notice-keep", i18n.T("ui.render.fallback_keep"), i18n.T("ui.render.fallback_keep_tip"), true, true,
					func(w *gpui.Window, _ *gpui.App) {
						ws.change(w, func(s *settings.Settings) { s.Renderer = settings.RendererSoftware })
						dismiss(w)
					}),
			),
		)
}
