package ui

import (
	"runtime"
	"sync"
	"time"
	"weak"

	"agentmanager"
	"cad-lite/i18n"
	"cad-lite/internal/gpui"
	"cad-lite/settings"
)

// settingsSaveDelay debounces writes while a slider or button is being
// worked.
const settingsSaveDelay = 500 * time.Millisecond

// actionSettings opens and closes the settings panel.
var actionSettings = gpui.NewAction("cad-settings")

// settingsBindings are the keys that open the panel (ctrl-, and cmd-,).
func settingsBindings() []gpui.KeyBinding {
	return []gpui.KeyBinding{
		gpui.NewKeyBinding("ctrl-,", actionSettings, "cad"),
		gpui.NewKeyBinding("cmd-,", actionSettings, "cad"),
	}
}

// prefsState is a workspace's settings state: the current values, where they
// are saved and the panel's own state. It lives beside the Workspace (see
// prefs) and never references it, so the Workspace stays collectable.
type prefsState struct {
	cur  settings.Settings
	path string
	// warnings are what Load clamped; err is the last load or save failure.
	warnings []string
	err      error

	// pendingScale is set until the first frame applies cur.UIScale to the
	// window, which settings loaded before the window exists need.
	pendingScale bool
	// hooked is set once Render itself mounts the panel via withSettings.
	hooked bool
	// onLanguage is the host's language hook; see SetLanguageHandler.
	onLanguage func(lang string)

	open        bool
	focus       *gpui.FocusHandle
	focusWanted bool
	scale       SliderState
	saveTimer   *gpui.Timer
	// bodyScroll is the scroll state of the panel's body, between the fixed
	// title bar and footer.
	bodyScroll *gpui.ScrollHandle
}

var prefsTable = struct {
	sync.Mutex
	m map[weak.Pointer[Workspace]]*prefsState
}{m: map[weak.Pointer[Workspace]]*prefsState{}}

// prefs returns ws's settings state, creating it (with default settings and
// no file) on first use.
func (ws *Workspace) prefs() *prefsState {
	key := weak.Make(ws)
	prefsTable.Lock()
	defer prefsTable.Unlock()
	if p, ok := prefsTable.m[key]; ok {
		return p
	}
	p := &prefsState{cur: settings.Default(), pendingScale: false, bodyScroll: gpui.NewScrollHandle()}
	prefsTable.m[key] = p
	runtime.AddCleanup(ws, func(k weak.Pointer[Workspace]) {
		prefsTable.Lock()
		delete(prefsTable.m, k)
		prefsTable.Unlock()
	}, key)
	return p
}

// Settings returns the current settings.
func (ws *Workspace) Settings() settings.Settings { return ws.prefs().cur.Clone() }

// SettingsPath returns the file settings are saved to; empty means they are
// kept in memory only.
func (ws *Workspace) SettingsPath() string { return ws.prefs().path }

// SettingsError returns the last settings load or save failure, or nil.
func (ws *Workspace) SettingsError() error { return ws.prefs().err }

// SettingsWarnings returns what loading the file clamped or ignored.
func (ws *Workspace) SettingsWarnings() []string {
	return append([]string(nil), ws.prefs().warnings...)
}

// LoadSettings loads the settings file in the OS user config folder (see
// settings.Path) and applies it: theme, drawing aids, default agent, and the
// UI scale on the first frame. A missing file gives the defaults. When there
// is no config folder, or the file cannot be read, the defaults apply, the
// settings stay in memory and the error is returned; callers may ignore it.
func (ws *Workspace) LoadSettings() error {
	path, err := settings.Path()
	if err != nil {
		ws.prefs().err = err
		ws.applySettings(settings.Default(), nil)
		return err
	}
	return ws.LoadSettingsFrom(path)
}

// LoadSettingsFrom is LoadSettings for an explicit file; later changes are
// saved there.
func (ws *Workspace) LoadSettingsFrom(path string) error {
	p := ws.prefs()
	p.path = path
	s, warn, err := settings.LoadWarn(path)
	p.warnings, p.err = warn, err
	ws.applySettings(s, nil)
	return err
}

// SetSettings applies s without saving it. w may be nil before the window
// exists; the UI scale then applies on the first frame.
func (ws *Workspace) SetSettings(s settings.Settings, w *gpui.Window) { ws.applySettings(s, w) }

// applySettings makes s current: appearance (theme and UI scale), the drawing
// aids and the default agent.
func (ws *Workspace) applySettings(s settings.Settings, w *gpui.Window) {
	p := ws.prefs()
	s.Normalize()
	p.cur = s.Clone()
	ws.applyAppearance(w)
	ws.applyDrawingDefaults()
	p.fireLanguage()
	ws.refreshPlaceholders()
	ws.applyLayout(s.Layout)
	if h := s.DefaultAgent; h != "" {
		ws.AgentPanel().Configure(agentmanager.Harness(h), "")
	}
	if w != nil {
		w.Refresh()
	}
}

// applyAppearance applies the current theme and UI scale. A theme name that is
// not registered falls back to the default theme.
func (ws *Workspace) applyAppearance(w *gpui.Window) {
	p := ws.prefs()
	t, ok := ThemeByName(p.cur.Theme)
	if !ok {
		t, _ = ThemeByName(DefaultThemeName)
	}
	ApplyTheme(t)
	if ws.builder != nil {
		ws.builder.Background = colCanvas
		ws.builder.InvalidateAll()
	}
	if w == nil {
		w = ws.win
	}
	if w != nil {
		w.SetUIScale(float32(p.cur.UIScale))
		p.pendingScale = false
		w.Refresh()
	} else {
		p.pendingScale = true
	}
}

// applyDrawingDefaults sets the grid, object snap and ortho flags from the
// settings.
func (ws *Workspace) applyDrawingDefaults() {
	c := ws.prefs().cur
	ws.session.SetGrid(c.Grid)
	ws.session.SnapOn = c.OSnap
	ws.session.Ortho = c.Ortho
}

// SetLanguageHandler installs the hook that applies the interface language:
// f is called with settings.LangAuto, LangEnglish or LangItalian when the
// settings are loaded or set, when the user picks another language in the
// panel, and once immediately with the current value. The workspace itself
// translates nothing. A nil f removes the hook.
func (ws *Workspace) SetLanguageHandler(f func(lang string)) {
	p := ws.prefs()
	p.onLanguage = f
	if f != nil {
		f(p.cur.Language)
		ws.refreshPlaceholders()
	}
}

// Language returns the configured interface language.
func (ws *Workspace) Language() string { return ws.prefs().cur.Language }

func (p *prefsState) fireLanguage() {
	if p.onLanguage != nil {
		p.onLanguage(p.cur.Language)
	}
}

// WheelMode is the configured mouse wheel behaviour: settings.WheelAuto,
// WheelZoom or WheelPan.
func (ws *Workspace) WheelMode() string { return ws.prefs().cur.Wheel }

// change edits the settings from the panel: f mutates a copy, the result is
// applied by what changed and saved after a short delay.
func (ws *Workspace) change(w *gpui.Window, f func(*settings.Settings)) {
	p := ws.prefs()
	old := p.cur
	next := old.Clone()
	f(&next)
	next.Normalize()
	p.cur = next
	if next.Theme != old.Theme || next.UIScale != old.UIScale {
		ws.applyAppearance(w)
	}
	if next.Language != old.Language {
		i18n.Set(i18n.FromSetting(next.Language))
		p.fireLanguage()
		ws.refreshPlaceholders()
	}
	if next.AutoUpdate != old.AutoUpdate {
		ws.armAutoCheck(w, updateFirstDelay)
	}
	if next.Grid != old.Grid || next.OSnap != old.OSnap || next.Ortho != old.Ortho {
		ws.applyDrawingDefaults()
	}
	if next.DefaultAgent != old.DefaultAgent && next.DefaultAgent != "" {
		ws.AgentPanel().Configure(agentmanager.Harness(next.DefaultAgent), "")
	}
	ws.scheduleSave(w)
	w.Refresh()
}

// scheduleSave saves the settings once settingsSaveDelay passes without
// another change.
func (ws *Workspace) scheduleSave(w *gpui.Window) {
	p := ws.prefs()
	if p.path == "" {
		return
	}
	p.saveTimer.Cancel()
	p.saveTimer = w.After(settingsSaveDelay, func(w *gpui.Window, _ *gpui.App) {
		ws.SaveSettings()
		w.Refresh()
	})
}

// SaveSettings writes the current settings to the settings file now, along
// with the window's size. It does nothing when the settings are in memory
// only.
func (ws *Workspace) SaveSettings() error {
	p := ws.prefs()
	p.saveTimer.Cancel()
	p.saveTimer = nil
	if p.path == "" {
		return nil
	}
	p.cur.Layout = ws.layoutSettings()
	s := p.cur.Clone()
	if w := ws.win; w != nil {
		b, f := w.Bounds(), w.UIScale()
		s.Window = settings.Window{Width: int(float32(b.Size.W)*f + 0.5), Height: int(float32(b.Size.H)*f + 0.5)}
		p.cur.Window = s.Window
	}
	p.err = settings.Save(p.path, s)
	return p.err
}

// SettingsOpen reports whether the settings panel is showing.
func (ws *Workspace) SettingsOpen() bool { return ws.prefs().open }

// OpenSettings shows the settings panel.
func (ws *Workspace) OpenSettings(w *gpui.Window) {
	p := ws.prefs()
	p.open, p.focusWanted = true, true
	p.bodyScroll.ScrollToTop()
	w.Refresh()
}

// CloseSettings hides the panel, saves any pending change and hands the
// keyboard back to the command line.
func (ws *Workspace) CloseSettings(w *gpui.Window) {
	p := ws.prefs()
	if !p.open {
		return
	}
	p.open = false
	p.scale.End(w)
	ws.SaveSettings()
	if fh := ws.input.FocusHandle(); fh != nil {
		w.RequestFocus(fh)
	}
	w.Refresh()
}

// ToggleSettings opens the panel, or closes it when it is open.
func (ws *Workspace) ToggleSettings(w *gpui.Window) {
	if ws.prefs().open {
		ws.CloseSettings(w)
	} else {
		ws.OpenSettings(w)
	}
}

// withSettings finishes the root element built by Render: it handles the
// settings action, mounts the panel over everything and applies a UI scale
// that was loaded before the window existed.
func (ws *Workspace) withSettings(cx *gpui.App, root *gpui.Div) *gpui.Div {
	ws.prefs().hooked = true
	return ws.mountSettings(cx, root)
}

// mountSettings is withSettings without recording that Render calls it, for
// hosts (tests) that wrap a Render that does not.
func (ws *Workspace) mountSettings(cx *gpui.App, root *gpui.Div) *gpui.Div {
	if p := ws.prefs(); p.pendingScale && ws.win != nil {
		ws.win.SetUIScale(float32(p.cur.UIScale))
		p.pendingScale = false
	}
	ws.startUpdates(ws.win)
	ws.startDiscovery(ws.win)
	return root.Relative().
		OnAction(actionSettings, func(_ gpui.Action, w *gpui.Window, _ *gpui.App) { ws.ToggleSettings(w) }).
		Child(ws.settingsOverlay(cx), ws.updateOverlay())
}
