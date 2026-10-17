package ui

import (
	"testing"

	"cad-lite/doc"
	"cad-lite/i18n"
	"cad-lite/internal/gpui"
	"cad-lite/updater"
)

// settingsTabContent lists, per tab, element ids that tab shows and the
// other tabs must not.
var settingsTabContent = [settingsTabCount][]string{
	settingsGeneral: {"settings-sec-appearance", "theme-light", "ui-scale-up", "lang-it", "settings-sec-drawing", "set-grid", "wheel-pan"},
	settingsAI:      {"ai-mode-basic", "ai-mode-advanced"},
	settingsUpdates: {"settings-sec-updates"},
}

// inFrame reports whether id hit-tests anywhere in the last frame, without
// switching tabs.
func (h *sHarness) inFrame(id string) bool { _, ok := h.locate(id); return ok }

// frameIDs is every element id on the hit-test path of some point of the
// last frame, ancestors included: one scan instead of one per id.
func (h *sHarness) frameIDs() map[string]bool {
	w := h.hw.Window()
	b := w.Bounds()
	ids := map[string]bool{}
	for y := float32(0); y < float32(b.Size.H); y += 3 {
		for x := float32(0); x < float32(b.Size.W); x += 3 {
			if gid, ok := w.HitTest(gpui.Point{X: gpui.Pixels(x), Y: gpui.Pixels(y)}); ok {
				for _, e := range gid.Path {
					ids[e.Key] = true
				}
			}
		}
	}
	return ids
}

// checkTab fails unless the panel shows tab t: its content, and none of the
// other tabs' content.
func (h *sHarness) checkTab(t settingsTab) {
	h.t.Helper()
	if got := h.ws.prefs().tab; got != t {
		h.t.Fatalf("tab %d, want %d", got, t)
	}
	frame := h.frameIDs()
	for tab, ids := range settingsTabContent {
		for _, id := range ids {
			if shown := frame[id]; shown != (settingsTab(tab) == t) {
				h.t.Errorf("tab %d: %s shown %v", t, id, shown)
			}
		}
	}
	for _, id := range []string{"settings-close", "settings-reset", "settings-tab-general", "settings-tab-ai", "settings-tab-updates"} {
		if !frame[id] {
			h.t.Errorf("tab %d: %s missing", t, id)
		}
	}
}

func TestSettingsOpensOnGeneralAndTabsSwitchByClick(t *testing.T) {
	h := openSettingsWS(t, NewWorkspace(doc.New(), nil), 900, 2000, 1)
	h.openPanel()
	h.checkTab(settingsGeneral)
	h.click(h.mustLocate("settings-tab-ai"))
	h.hw.Pixels()
	h.checkTab(settingsAI)
	h.click(h.mustLocate("settings-tab-updates"))
	h.hw.Pixels()
	h.checkTab(settingsUpdates)
	h.click(h.mustLocate("settings-tab-general"))
	h.hw.Pixels()
	h.checkTab(settingsGeneral)
	if !h.ws.SettingsOpen() {
		t.Fatal("clicking a tab closed the panel")
	}
}

// mustLocate is locate that fails the test when id is not in the frame.
func (h *sHarness) mustLocate(id string) gpui.Point {
	h.t.Helper()
	p, ok := h.locate(id)
	if !ok {
		h.t.Fatalf("%s not in the frame", id)
	}
	return p
}

func TestSettingsTabKeysCycleTabsNotDrawings(t *testing.T) {
	h := openSettingsWS(t, NewWorkspace(doc.New(), nil), 900, 2000, 1)
	h.ws.run("NEW", h.hw.Window())
	h.hw.Pixels()
	active := h.ws.docTab
	h.openPanel()
	shiftTab := func() {
		h.hw.Dispatch(gpui.KeyDownEvent{Keystroke: gpui.Keystroke{Key: "tab", Modifiers: gpui.Control | gpui.Shift}})
		h.hw.Pixels()
	}
	for _, want := range []settingsTab{settingsAI, settingsUpdates, settingsGeneral} {
		h.ctrl("tab")
		h.hw.Pixels()
		h.checkTab(want)
	}
	shiftTab()
	h.checkTab(settingsUpdates)
	shiftTab()
	h.checkTab(settingsAI)
	if h.ws.docTab != active {
		t.Error("Ctrl+Tab in the settings panel switched the drawing")
	}
	h.key("escape")
	h.hw.Pixels()
	if h.ws.SettingsOpen() {
		t.Fatal("Escape did not close the panel")
	}
	h.ctrl("tab")
	h.hw.Pixels()
	if h.ws.docTab == active {
		t.Error("with the panel closed, Ctrl+Tab no longer switches drawings")
	}
}

func TestSettingsTabRememberedForTheSession(t *testing.T) {
	h := openSettingsWS(t, NewWorkspace(doc.New(), nil), 900, 2000, 1)
	h.openPanel()
	h.clickID("settings-tab-updates")
	h.hw.Pixels()
	h.ws.CloseSettings(h.hw.Window())
	h.hw.Pixels()
	h.ctrl(",")
	h.hw.Pixels()
	if !h.ws.SettingsOpen() {
		t.Fatal("Ctrl+, did not open the panel")
	}
	h.checkTab(settingsUpdates)
	// Not persisted: a new workspace starts on General.
	h2 := openSettingsWS(t, NewWorkspace(doc.New(), nil), 900, 2000, 1)
	h2.openPanel()
	h2.checkTab(settingsGeneral)
}

func TestSettingsTabSwitchScrollsBodyToTop(t *testing.T) {
	h := openSettingsWS(t, NewWorkspace(doc.New(), nil), 900, 500, 1)
	h.openPanelAt(settingsAI)
	sc := h.ws.prefs().bodyScroll
	if sc.MaxOffset().H <= 0 {
		t.Fatalf("the AI tab must overflow a 500 px window, max offset %v", sc.MaxOffset())
	}
	body := h.find("settings-body")
	for i := 0; i < 5; i++ {
		h.hw.Dispatch(gpui.ScrollWheelEvent{Position: body, DeltaY: 60})
		h.hw.Pixels()
	}
	if sc.Offset().Y <= 0 {
		t.Fatal("the body did not scroll")
	}
	h.clickID("settings-tab-general")
	h.hw.Pixels()
	if sc.Offset().Y != 0 {
		t.Errorf("General opened scrolled by %v", sc.Offset().Y)
	}
}

// fakeUpdate makes a release wait, as a finished check would.
func fakeUpdate(ws *Workspace, version string) {
	u := ws.upd()
	u.phase, u.latest = updAvailable, updater.Release{Version: version, Notes: "## What's new\n- **Faster** zoom\n- Fixed `LINE`\n"}
}

func TestGearWithUpdateBadgeOpensUpdatesTab(t *testing.T) {
	h := openSettingsWS(t, NewWorkspace(doc.New(), nil), 900, 2000, 1)
	h.clickID("tool-settings")
	h.hw.Pixels()
	h.checkTab(settingsGeneral)
	h.clickID("settings-close")
	h.hw.Pixels()

	fakeUpdate(h.ws, "9.9.9")
	h.hw.Window().Refresh()
	h.hw.Pixels()
	if !h.inFrame("update-badge") {
		t.Fatal("no badge on the gear while a release waits")
	}
	h.clickID("tool-settings")
	h.hw.Pixels()
	if !h.ws.SettingsOpen() {
		t.Fatal("the gear did not open the panel")
	}
	h.checkTab(settingsUpdates)
	if !h.inFrame("settings-tab-updates") {
		t.Fatal("no Updates tab")
	}
}

func TestSettingsTabLabelsFollowLanguage(t *testing.T) {
	t.Cleanup(func() { i18n.Set(i18n.English) })
	for _, c := range []struct {
		lang i18n.Lang
		want [settingsTabCount]string
	}{
		{i18n.English, [settingsTabCount]string{"General", "AI", "Updates"}},
		{i18n.Italian, [settingsTabCount]string{"Generale", "AI", "Aggiornamenti"}},
	} {
		i18n.Set(c.lang)
		for tab, d := range settingsTabs {
			if got := i18n.T(d.label); got != c.want[tab] {
				t.Errorf("%v tab %d: %q, want %q", c.lang, tab, got, c.want[tab])
			}
			if tip := i18n.T(d.tip); tip == d.tip || tip == "" {
				t.Errorf("%v tab %d: no tooltip text for %s", c.lang, tab, d.tip)
			}
		}
		if k := i18n.T("ui.settings.tab_keys_tip"); k == "ui.settings.tab_keys_tip" {
			t.Errorf("%v: no text for the tab keys", c.lang)
		}
	}
}

// Renders every tab in the dark and the light theme into _tmp/settings (a
// look-at-it test), plus the Updates tab with a release waiting.
func TestRenderSettingsTabs(t *testing.T) {
	for _, th := range []string{"dark", "light"} {
		ws := NewWorkspace(doc.New(), nil)
		ws.SetVersion("1.0.0")
		ws.SetUpdateClient(newUpdRig(t, "v9.9.9").client)
		h := openSettingsWS(t, ws, 1280, 860, 1)
		s := h.ws.Settings()
		s.Theme, s.AutoUpdate = th, false
		h.ws.SetSettings(s, h.hw.Window())
		h.openPanel()
		for tab, d := range settingsTabs {
			h.openPanelAt(settingsTab(tab))
			h.png("tab-" + d.id + "-" + th + ".png")
		}
		fakeUpdate(h.ws, "9.9.9")
		h.hw.Window().Refresh()
		h.hw.Pixels()
		h.png("tab-updates-waiting-" + th + ".png")
	}
}
