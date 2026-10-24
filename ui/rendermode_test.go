package ui

import (
	"testing"

	"cad-lite/doc"
	"cad-lite/internal/gpui"
	"cad-lite/settings"
)

// showRendererRow makes the renderer row visible whatever the test's OS.
func showRendererRow(t *testing.T) {
	old := rendererRowShown
	rendererRowShown = true
	t.Cleanup(func() { rendererRowShown = old })
}

func TestRendererRowChangesSettingAndOffersRestart(t *testing.T) {
	showRendererRow(t)
	h := openSettingsApp(t)
	h.ws.SetRenderInfo(RenderInfo{Active: settings.RendererGPU})
	h.openPanelAt(settingsGeneral)
	if !h.has("renderer-auto") || !h.has("renderer-software") {
		t.Fatal("renderer row missing")
	}
	if h.has("renderer-restart") {
		t.Fatal("restart offered while the setting matches the renderer in use")
	}
	h.clickID("renderer-software")
	h.hw.Pixels()
	if got := h.ws.Settings().Renderer; got != settings.RendererSoftware {
		t.Fatalf("renderer setting %q, want software", got)
	}
	if !h.has("renderer-restart") {
		t.Fatal("no Restart now after changing the renderer")
	}
	p := h.ws.prefs()
	p.bodyScroll.SetOffset(gpui.Point{Y: p.bodyScroll.MaxOffset().H})
	h.hw.Window().Refresh()
	h.hw.Pixels()
	h.png("renderer-row.png")
	h.clickID("renderer-auto")
	h.hw.Pixels()
	if h.has("renderer-restart") {
		t.Fatal("restart still offered after going back to auto on the GPU")
	}
}

func TestRendererRowHiddenWithoutChoice(t *testing.T) {
	old := rendererRowShown
	rendererRowShown = false
	t.Cleanup(func() { rendererRowShown = old })
	h := openSettingsApp(t)
	h.openPanelAt(settingsGeneral)
	if h.has("settings-sec-display") {
		t.Fatal("renderer row shown where there is no software renderer")
	}
}

func TestRendererRestartNeeded(t *testing.T) {
	cases := []struct {
		setting string
		info    RenderInfo
		want    bool
	}{
		{settings.RendererAuto, RenderInfo{Active: "gpu"}, false},
		{settings.RendererSoftware, RenderInfo{Active: "gpu"}, true},
		{settings.RendererAuto, RenderInfo{Active: "software", Fallback: true}, false},
		{settings.RendererSoftware, RenderInfo{Active: "software", Fallback: true}, false},
		{settings.RendererGPU, RenderInfo{Active: "software", Fallback: true}, true},
		{settings.RendererAuto, RenderInfo{Active: "software"}, true},
		{settings.RendererSoftware, RenderInfo{Active: "gpu", Forced: true}, false},
		{settings.RendererSoftware, RenderInfo{}, false},
	}
	for _, c := range cases {
		ws := NewWorkspace(doc.New(), nil)
		ws.prefs().cur.Renderer = c.setting
		ws.SetRenderInfo(c.info)
		if got := ws.rendererRestartNeeded(); got != c.want {
			t.Errorf("setting %s, info %+v: restart %v, want %v", c.setting, c.info, got, c.want)
		}
	}
}

func TestRenderFallbackNotice(t *testing.T) {
	showRendererRow(t)
	h := openSettingsApp(t)
	h.ws.SetRenderInfo(RenderInfo{Active: settings.RendererSoftware, Fallback: true})
	h.hw.Window().Refresh()
	h.hw.Pixels()
	if !h.ws.RenderNoticeShown() || !h.has("render-notice-keep") {
		t.Fatal("fallback notice not shown")
	}
	h.png("render-notice.png")
	h.clickID("render-notice-keep")
	h.hw.Pixels()
	if h.ws.RenderNoticeShown() || h.has("render-notice") {
		t.Fatal("notice still shown after Always use software rendering")
	}
	if got := h.ws.Settings().Renderer; got != settings.RendererSoftware {
		t.Fatalf("renderer %q after keeping software", got)
	}
	if h.ws.rendererRestartNeeded() {
		t.Fatal("keeping the renderer in use must not ask for a restart")
	}
}

func TestRenderFallbackNoticeDismiss(t *testing.T) {
	h := openSettingsApp(t)
	h.ws.SetRenderInfo(RenderInfo{Active: settings.RendererSoftware, Fallback: true})
	h.hw.Window().Refresh()
	h.hw.Pixels()
	h.clickID("render-notice-ok")
	h.hw.Pixels()
	if h.ws.RenderNoticeShown() {
		t.Fatal("notice still shown after OK")
	}
	if got := h.ws.Settings().Renderer; got != settings.RendererAuto {
		t.Fatalf("OK changed the renderer to %q", got)
	}
}

func TestRequestRestartRelaunchesAfterQuit(t *testing.T) {
	h := openSettingsApp(t)
	quits := 0
	h.ws.SetQuitHandler(func() { quits++ })
	if err := h.ws.RequestRestart(nil); err != nil {
		t.Fatal(err)
	}
	u := h.ws.upd()
	if quits != 1 || !u.restart || u.plan.Exe == "" {
		t.Fatalf("clean drawing: quits %d restart %v plan %+v", quits, u.restart, u.plan)
	}
}
