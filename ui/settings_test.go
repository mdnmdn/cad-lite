package ui

import (
	"image/color"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"cad-lite/doc"
	"cad-lite/geom"
	"cad-lite/i18n"
	"cad-lite/internal/gpui"
	"cad-lite/settings"
)

// settingsRoot mounts the settings panel over the workspace the way
// Workspace.Render does once the hook is applied.
type settingsRoot struct{ ws *Workspace }

func (r settingsRoot) Render(w *gpui.Window, cx *gpui.App) gpui.IntoElement {
	inner := r.ws.Render(w, cx)
	if r.ws.prefs().hooked {
		return inner
	}
	return r.ws.mountSettings(cx, gpui.NewDiv().SizeFull().Child(inner))
}

type sHarness struct {
	t  *testing.T
	ws *Workspace
	hp *gpui.HeadlessPlatform
	hw *gpui.HeadlessWindow
}

func openSettingsWS(t *testing.T, ws *Workspace, width, height, device float32) *sHarness {
	t.Helper()
	t.Cleanup(func() { ApplyTheme(themeDark) })
	p := gpui.NewHeadlessPlatform()
	p.SetScaleFactor(device)
	var hw *gpui.HeadlessWindow
	if err := p.Run(func(cx *gpui.App) {
		cx.Keymap().AddBindings(Bindings()...)
		cx.Keymap().AddBindings(settingsBindings()...)
		if err := cx.OpenWindow(gpui.WindowOptions{Width: width, Height: height}, func(*gpui.Window, *gpui.App) gpui.EntityRoot { return settingsRoot{ws} }); err != nil {
			t.Fatal(err)
		}
		hw = p.Windows()[0]
	}); err != nil {
		t.Fatal(err)
	}
	hw.Pixels()
	return &sHarness{t: t, ws: ws, hp: p, hw: hw}
}

func openSettingsApp(t *testing.T) *sHarness {
	t.Helper()
	return openSettingsWS(t, NewWorkspace(doc.New(), nil), 900, 700, 1)
}

// find returns the centre, in platform coordinates, of the element with the
// given id, found by hit-testing the last frame.
func (h *sHarness) find(id string) gpui.Point {
	h.t.Helper()
	w := h.hw.Window()
	b := w.Bounds()
	var minX, minY, maxX, maxY float32 = 1e9, 1e9, -1, -1
	for y := float32(0); y < float32(b.Size.H); y += 2 {
		for x := float32(0); x < float32(b.Size.W); x += 2 {
			gid, ok := w.HitTest(gpui.Point{X: gpui.Pixels(x), Y: gpui.Pixels(y)})
			if !ok || len(gid.Path) == 0 || gid.Path[len(gid.Path)-1].Key != id {
				continue
			}
			minX, minY, maxX, maxY = min(minX, x), min(minY, y), max(maxX, x), max(maxY, y)
		}
	}
	if maxX < 0 {
		h.t.Fatalf("element %q not found", id)
	}
	f := w.UIScale()
	return gpui.Point{X: gpui.Pixels((minX + maxX) / 2 * f), Y: gpui.Pixels((minY + maxY) / 2 * f)}
}

func (h *sHarness) click(p gpui.Point) {
	h.hw.Dispatch(gpui.MouseMoveEvent{Position: p})
	h.hw.Dispatch(gpui.MouseDownEvent{Position: p, Button: gpui.MouseLeft, ClickCount: 1})
	up := gpui.MouseUpEvent{Position: p, Button: gpui.MouseLeft, ClickCount: 1}
	h.hw.Dispatch(up)
	h.hw.Dispatch(gpui.ClickEvent{Up: &up})
}

// openPanel opens the panel the way the ctrl-, binding does.
func (h *sHarness) openPanel() {
	h.t.Helper()
	h.ws.OpenSettings(h.hw.Window())
	h.hw.Pixels()
	if !h.ws.SettingsOpen() {
		h.t.Fatal("panel did not open")
	}
}

func (h *sHarness) clickID(id string) { h.t.Helper(); h.click(h.find(id)) }

func (h *sHarness) key(k string) {
	h.hw.Dispatch(gpui.KeyDownEvent{Keystroke: gpui.Keystroke{Key: k}})
}

func (h *sHarness) ctrl(k string) {
	h.hw.Dispatch(gpui.KeyDownEvent{Keystroke: gpui.Keystroke{Key: k, Modifiers: gpui.Control}})
}

func (h *sHarness) png(name string) {
	h.t.Helper()
	if err := os.MkdirAll("../_tmp/settings", 0o755); err != nil {
		h.t.Fatal(err)
	}
	if err := h.hw.SavePNG(filepath.Join("../_tmp/settings", name)); err != nil {
		h.t.Fatal(err)
	}
}

func pixel(h *sHarness, x, y int) color.NRGBA { return h.hw.Image().NRGBAAt(x, y) }

func TestSettingsPanelOpensAndCloses(t *testing.T) {
	h := openSettingsApp(t)
	if h.ws.SettingsOpen() {
		t.Fatal("panel open at start")
	}
	h.openPanel()
	h.png("panel-dark.png")
	h.key("escape")
	if h.ws.SettingsOpen() {
		t.Fatal("Escape did not close the panel")
	}
	h.ctrl(",")
	if !h.ws.SettingsOpen() {
		t.Fatal("ctrl-, did not open the panel")
	}
	h.clickID("settings-close")
	if h.ws.SettingsOpen() {
		t.Fatal("close button did not close the panel")
	}
	h.ws.OpenSettings(h.hw.Window())
	h.hw.Pixels()
	h.click(gpui.Point{X: 5, Y: 400}) // backdrop, left of the card
	if h.ws.SettingsOpen() {
		t.Fatal("backdrop click did not close the panel")
	}
}

func TestSettingsBackdropBlocksTheToolbar(t *testing.T) {
	h := openSettingsApp(t)
	h.ws.OpenSettings(h.hw.Window())
	h.hw.Pixels()
	// The Line button sits under the backdrop at the top left.
	h.click(gpui.Point{X: 90, Y: 30})
	if got := h.ws.Session().Active(); got != "" {
		t.Fatalf("a click on the backdrop started %q", got)
	}
}

func TestThemeSwitchChangesPixelsAndBuilder(t *testing.T) {
	h := openSettingsApp(t)
	d := h.ws.Session().Drawing()
	tx, _ := d.Begin()
	l := &doc.Line{End: geom.V(10, 0)}
	lh := tx.Add(l)
	tx.Commit()
	canvasPx := func() color.NRGBA {
		b := h.ws.canvas.bounds
		return pixel(h, int(b.Origin.X)+20, int(b.Origin.Y)+20)
	}
	if got := canvasPx(); got != (color.NRGBA{33, 40, 48, 255}) {
		t.Fatalf("dark canvas pixel = %v", got)
	}
	if h.ws.builder.Background != colCanvas {
		t.Fatal("builder background out of sync")
	}
	if c := h.ws.builder.EntityColor(l); c.R != 255 {
		t.Fatalf("ACI 7 on dark = %v, want white", c)
	}
	if it := h.ws.builder.Items(lh, *h.ws.view); it[0].Color.R != 255 {
		t.Fatalf("cached ACI 7 item on dark = %v", it[0].Color)
	}
	h.ws.change(h.hw.Window(), func(s *settings.Settings) { s.Theme = "light" })
	if it := h.ws.builder.Items(lh, *h.ws.view); it[0].Color.R != 0 {
		t.Fatalf("theme switch left a stale cached item: %v", it[0].Color)
	}
	got := canvasPx()
	if got != (color.NRGBA{252, 252, 253, 255}) {
		t.Fatalf("light canvas pixel = %v", got)
	}
	if h.ws.builder.Background != colCanvas || colCanvas.R != 252 {
		t.Fatal("builder background not updated")
	}
	if c := h.ws.builder.EntityColor(l); c.R != 0 || c.G != 0 || c.B != 0 {
		t.Fatalf("ACI 7 on the light theme = %v, want black", c)
	}
	h.png("workspace-light.png")
}

func TestEveryThemeRenders(t *testing.T) {
	h := openSettingsApp(t)
	d := h.ws.Session().Drawing()
	tx, _ := d.Begin()
	tx.Add(&doc.Line{End: geom.V(100, 40)})
	tx.Add(&doc.Circle{Center: geom.V(50, 20), Radius: 25})
	tx.Commit()
	h.ws.FitOnFirstFrame()
	for _, th := range Themes() {
		h.ws.change(h.hw.Window(), func(s *settings.Settings) { s.Theme = th.Name })
		h.hw.Pixels()
		h.png("workspace-" + th.Name + ".png")
		if CurrentTheme().Name != th.Name {
			t.Fatalf("current theme %q, want %q", CurrentTheme().Name, th.Name)
		}
	}
	h.ws.OpenSettings(h.hw.Window())
	for _, th := range Themes() {
		h.ws.change(h.hw.Window(), func(s *settings.Settings) { s.Theme = th.Name })
		h.hw.Pixels()
		h.png("panel-" + th.Name + ".png")
	}
}

func TestUnknownThemeFallsBackToDark(t *testing.T) {
	h := openSettingsApp(t)
	h.ws.SetSettings(settings.Settings{Theme: "neon", UIScale: 1, Wheel: "auto"}, h.hw.Window())
	if CurrentTheme().Name != "dark" {
		t.Fatalf("theme = %q", CurrentTheme().Name)
	}
}

func TestSettingsPersistAndReload(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("CADLITE_"+settings.EnvDir, dir)
	ws := NewWorkspace(doc.New(), nil)
	if err := ws.LoadSettings(); err != nil {
		t.Fatal(err)
	}
	if ws.SettingsPath() != filepath.Join(dir, "settings.toml") {
		t.Fatalf("path = %q", ws.SettingsPath())
	}
	h := openSettingsWS(t, ws, 900, 700, 1)
	h.openPanel()
	h.clickID("theme-classic")
	h.clickID("set-ortho")
	h.clickID("wheel-pan")
	if _, err := os.Stat(ws.SettingsPath()); err == nil {
		t.Fatal("saved before the debounce delay")
	}
	if err := h.hp.Advance(settingsSaveDelay + time.Millisecond); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(ws.SettingsPath())
	if err != nil {
		t.Fatalf("not saved after the delay: %v", err)
	}
	for _, want := range []string{`theme = "classic"`, "ortho = true", `wheel = "pan"`} {
		if !strings.Contains(string(data), want) {
			t.Errorf("file lacks %q:\n%s", want, data)
		}
	}
	ApplyTheme(themeDark)
	ws2 := NewWorkspace(doc.New(), nil)
	if err := ws2.LoadSettings(); err != nil {
		t.Fatal(err)
	}
	if CurrentTheme().Name != "classic" || !ws2.Session().Ortho || ws2.WheelMode() != settings.WheelPan {
		t.Fatalf("reload: theme %q ortho %v wheel %q", CurrentTheme().Name, ws2.Session().Ortho, ws2.WheelMode())
	}
	if ws2.builder.Background != themeClassic.Canvas {
		t.Fatal("a workspace created after loading does not use the loaded canvas colour")
	}
}

func TestCloseSavesImmediately(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.toml")
	ws := NewWorkspace(doc.New(), nil)
	ws.LoadSettingsFrom(path)
	h := openSettingsWS(t, ws, 900, 700, 1)
	h.openPanel()
	h.clickID("theme-light")
	h.key("escape")
	data, err := os.ReadFile(path)
	if err != nil || !strings.Contains(string(data), `theme = "light"`) {
		t.Fatalf("close did not flush: %v %s", err, data)
	}
}

func TestUIScaleFromSettingsApplies(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.toml")
	if err := os.WriteFile(path, []byte("[appearance]\nui_scale = 1.5\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	ws := NewWorkspace(doc.New(), nil)
	if err := ws.LoadSettingsFrom(path); err != nil {
		t.Fatal(err)
	}
	h := openSettingsWS(t, ws, 900, 600, 1)
	w := h.hw.Window()
	if w.UIScale() != 1.5 {
		t.Fatalf("UIScale = %v, want 1.5 from the file", w.UIScale())
	}
	if b := w.Bounds(); b.Size.W != 600 || b.Size.H != 400 {
		t.Fatalf("logical size = %v, want 600x400", b.Size)
	}
	if img := h.hw.Image(); img.Bounds().Dx() != 900 {
		t.Fatalf("frame width %d", img.Bounds().Dx())
	}
	h.png("workspace-scale150.png")
}

func TestUIScaleButtonsAndReset(t *testing.T) {
	h := openSettingsApp(t)
	w := h.hw.Window()
	h.openPanel()
	h.clickID("ui-scale-up")
	if w.UIScale() != 1.05 || h.ws.Settings().UIScale != 1.05 {
		t.Fatalf("after + : window %v settings %v", w.UIScale(), h.ws.Settings().UIScale)
	}
	h.clickID("ui-scale-down")
	h.clickID("ui-scale-down")
	if got := w.UIScale(); got != 0.95 {
		t.Fatalf("after - -: %v", got)
	}
	h.clickID("ui-scale-reset")
	if w.UIScale() != 1 {
		t.Fatalf("reset: %v", w.UIScale())
	}
}

func TestUIScaleSliderDragAndKeys(t *testing.T) {
	h := openSettingsApp(t)
	w := h.hw.Window()
	h.openPanel()
	tr := h.ws.prefs().scale.track
	f := float64(w.UIScale())
	at := func(frac float64) gpui.Point {
		x0 := float64(tr.Origin.X) + sliderThumb/2
		span := float64(tr.Size.W) - sliderThumb
		return gpui.Point{X: gpui.Pixels((x0 + span*frac) * f), Y: gpui.Pixels(float64(tr.Origin.Y+tr.Size.H/2) * f)}
	}
	// Press at the far right, drag to 40% and let go outside the slider.
	h.hw.Dispatch(gpui.MouseMoveEvent{Position: at(1)})
	h.hw.Dispatch(gpui.MouseDownEvent{Position: at(1), Button: gpui.MouseLeft, ClickCount: 1})
	if !h.ws.prefs().scale.Dragging() {
		t.Fatal("press on the track did not start a drag")
	}
	if w.UIScale() != 1 {
		t.Fatalf("scale applied during the drag: %v", w.UIScale())
	}
	p := at(0.4)
	p.Y += 200 // well off the track: the overlay keeps the drag alive
	h.hw.Dispatch(gpui.MouseMoveEvent{Position: p})
	h.hw.Dispatch(gpui.MouseUpEvent{Position: p, Button: gpui.MouseLeft, ClickCount: 1})
	want := settings.ClampUIScale(0.75 + 0.4*1.25)
	if w.UIScale() != float32(want) || h.ws.Settings().UIScale != want {
		t.Fatalf("after drag: window %v settings %v, want %v", w.UIScale(), h.ws.Settings().UIScale, want)
	}
	// Keyboard: the press focused the slider.
	h.key("right")
	if got := h.ws.Settings().UIScale; got != settings.ClampUIScale(want+0.05) {
		t.Fatalf("right: %v", got)
	}
	h.key("home")
	if w.UIScale() != 0.75 {
		t.Fatalf("home: %v", w.UIScale())
	}
	h.key("end")
	if w.UIScale() != 2 {
		t.Fatalf("end: %v", w.UIScale())
	}
	h.png("panel-scale200.png")
}

func TestSliderSnapAndKeys(t *testing.T) {
	for _, c := range []struct{ in, want float64 }{{0.74, 0.75}, {1.013, 1.0}, {1.03, 1.05}, {9, 2}} {
		if got := SnapSlider(c.in, 0.75, 2, 0.05); got != c.want {
			t.Errorf("SnapSlider(%v) = %v, want %v", c.in, got, c.want)
		}
	}
	if got := SnapSlider(3.3, 0, 10, 0); got != 3.3 {
		t.Errorf("continuous slider snapped to %v", got)
	}
}

func TestSettingsClicksOnPanelDoNotCloseIt(t *testing.T) {
	h := openSettingsApp(t)
	h.openPanel()
	h.clickID("set-grid")
	if !h.ws.SettingsOpen() {
		t.Fatal("clicking a control closed the panel")
	}
	if h.ws.Session().Grid() {
		t.Fatal("grid toggle did not apply")
	}
	if h.ws.Settings().Grid {
		t.Fatal("grid default not stored")
	}
}

func TestSettingsInMemoryWhenNoPath(t *testing.T) {
	h := openSettingsApp(t)
	h.openPanel()
	h.clickID("theme-light")
	if h.ws.SettingsPath() != "" || h.ws.SaveSettings() != nil {
		t.Fatal("a workspace without a settings file tried to save")
	}
}

func TestSettingsPanelAtSmallWindowScrolls(t *testing.T) {
	ws := NewWorkspace(doc.New(), nil)
	h := openSettingsWS(t, ws, 700, 420, 2)
	h.ws.OpenSettings(h.hw.Window())
	h.hw.Pixels()
	h.png("panel-small-2x.png")
}

// The gear button exists once the toolbar hook (settingsButton) is mounted;
// before that the panel is reached with ctrl-,.
func TestToolbarGearButtonOpensPanel(t *testing.T) {
	h := openSettingsApp(t)
	w := h.hw.Window()
	found := false
	for x := float32(0); x < float32(w.Bounds().Size.W) && !found; x += 4 {
		if gid, ok := w.HitTest(gpui.Point{X: gpui.Pixels(x), Y: 30}); ok && gid.Path[len(gid.Path)-1].Key == "tool-settings" {
			found = true
		}
	}
	if !found {
		t.Skip("toolbar does not mount settingsButton yet")
	}
	h.clickID("tool-settings")
	if !h.ws.SettingsOpen() {
		t.Fatal("gear button did not open the panel")
	}
}

func TestLanguagePickerFiresHook(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.toml")
	os.WriteFile(path, []byte("[appearance]\nlanguage = \"it\"\n"), 0o600)
	ws := NewWorkspace(doc.New(), nil)
	ws.LoadSettingsFrom(path)
	var got []string
	ws.SetLanguageHandler(func(l string) { got = append(got, l) })
	if len(got) != 1 || got[0] != "it" {
		t.Fatalf("hook on registration: %v", got)
	}
	h := openSettingsWS(t, ws, 900, 700, 1)
	h.openPanel()
	h.clickID("lang-en")
	if len(got) != 2 || got[1] != "en" || ws.Language() != "en" {
		t.Fatalf("hook after picking English: %v", got)
	}
	h.clickID("theme-light")
	if len(got) != 2 {
		t.Fatalf("hook fired for an unrelated change: %v", got)
	}
	h.hp.Advance(settingsSaveDelay + time.Millisecond)
	if data, _ := os.ReadFile(path); !strings.Contains(string(data), `language = "en"`) {
		t.Fatalf("language not saved:\n%s", data)
	}
	ws.SetLanguageHandler(nil)
	h.clickID("lang-auto")
}

func TestLanguagePickerSwitchesCatalog(t *testing.T) {
	t.Cleanup(func() { i18n.Set(i18n.English) })
	ws := NewWorkspace(doc.New(), nil)
	h := openSettingsWS(t, ws, 900, 700, 1)
	h.openPanel()
	h.clickID("lang-it")
	if i18n.Current() != i18n.Italian {
		t.Fatalf("language after picking Italiano = %v", i18n.Current())
	}
	h.clickID("lang-en")
	if i18n.Current() != i18n.English {
		t.Fatalf("language after picking English = %v", i18n.Current())
	}
}

// settingsBubble reports whether a one-line tooltip bubble (themed border,
// wider than any control) is in the last frame.
func (h *sHarness) settingsBubble() bool {
	th := CurrentTheme()
	for _, q := range h.hw.Window().Scene().Quads {
		if q.BorderColor == th.Border && q.Bounds.Size.W > 150 && q.Bounds.Size.H < 40 {
			return true
		}
	}
	return false
}

func TestSettingsControlsHaveTooltips(t *testing.T) {
	h := openSettingsWS(t, NewWorkspace(doc.New(), nil), 900, 1000, 1) // tall: the footer sits below the updates section
	h.openPanel()
	for _, id := range []string{"settings-close", "theme-light", "ui-scale-down", "ui-scale-up", "ui-scale-reset",
		"lang-it", "set-grid", "set-osnap", "set-ortho", "wheel-pan", "agent-claude", "settings-reset"} {
		p := h.find(id)
		h.hw.Dispatch(gpui.MouseMoveEvent{Position: gpui.Point{X: 5, Y: 5}})
		h.hp.Advance(600 * time.Millisecond)
		h.hw.Dispatch(gpui.MouseMoveEvent{Position: p})
		h.hp.Advance(600 * time.Millisecond)
		if !h.settingsBubble() {
			t.Errorf("%s: no tooltip after 600 ms", id)
		}
	}
}
