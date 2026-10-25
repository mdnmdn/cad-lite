package ui

import (
	"image"
	"image/png"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"cad-lite/cmdsys"
	"cad-lite/cmdsys/props"
	"cad-lite/doc"
	"cad-lite/geom"
	"cad-lite/i18n"
	"cad-lite/internal/gpui"
)

// propsRig opens a workspace with a line and a circle on layer Walls and a linear dimension on 0,
// nothing selected. It returns the harness and the handles: line, circle, dimension.
func propsRig(t *testing.T, device float32) (*sHarness, []doc.Handle) {
	t.Helper()
	d := doc.New()
	d.AddLayer("Walls", 1)
	d.AddLayer("Doors", 3)
	h := openSettingsWS(t, NewWorkspace(d, nil), 1280, 800, device)
	s := h.ws.Session()
	s.Script("-LAYER S Walls  LINE 0,0 100,0  CIRCLE 50,50 20 -LAYER S 0  DIMLINEAR 0,0 100,0 50,-20 ZOOM E")
	hs := d.Handles()
	if len(hs) != 3 {
		t.Fatalf("rig made %d objects", len(hs))
	}
	s.Selection().Clear()
	h.frame()
	return h, hs
}

// selectHandles makes hs the selection, as picking them would.
func (h *sHarness) selectHandles(hs ...doc.Handle) {
	sel := h.ws.session.Selection()
	sel.Clear()
	sel.Add(hs...)
	h.frame()
}

// savePanel writes the side panel (with what hangs from it) of the last frame to _tmp/props.
func (h *sHarness) savePanel(name string) {
	h.t.Helper()
	img := h.hw.Image()
	f := h.hw.Window().UIScale() * h.hw.ScaleFactor()
	side, _ := h.ws.widths(h.ws.winW)
	x0 := int((h.ws.winW - side - 330) * f)
	sub := img.SubImage(image.Rect(max(0, x0), 0, img.Bounds().Dx(), img.Bounds().Dy()))
	dir := filepath.Join("..", "_tmp", "props")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		h.t.Fatal(err)
	}
	out, err := os.Create(filepath.Join(dir, name))
	if err != nil {
		h.t.Fatal(err)
	}
	defer out.Close()
	if err := png.Encode(out, sub); err != nil {
		h.t.Fatal(err)
	}
}

// Renders the panel for each target and each drop-down, dark and light, into _tmp/props.
func TestRenderPropertiesPanel(t *testing.T) {
	for _, th := range []string{"dark", "light"} {
		h, hs := propsRig(t, 2)
		s := h.ws.Settings()
		s.Theme = th
		h.ws.SetSettings(s, h.hw.Window())
		h.frame()
		h.selectHandles(hs[0])
		h.savePanel("line-" + th + ".png")
		h.selectHandles(hs[2])
		h.savePanel("dim-" + th + ".png")
		h.selectHandles(hs...)
		h.savePanel("mixed-" + th + ".png")
		h.selectHandles()
		h.savePanel("defaults-" + th + ".png")
		h.clickID("layer-Walls")
		h.frame()
		h.savePanel("layer-" + th + ".png")
		h.ws.clearLayerTarget(h.hw.Window())
		h.selectHandles(hs[0])
		for _, id := range []string{"color", "linetype", "lineweight"} {
			h.clickID("props-ctl-" + id)
			h.frame()
			h.savePanel("pop-" + id + "-" + th + ".png")
			h.key("escape")
			h.frame()
		}
		h.clickID("props-ctl-color")
		h.frame()
		h.clickID("props-more-colors")
		h.frame()
		h.savePanel("pop-aci-" + th + ".png")
		h.key("escape")
		h.selectHandles(hs[2])
		h.clickID("props-ctl-arrow1")
		h.frame()
		h.savePanel("pop-arrow-" + th + ".png")
		h.key("escape")
		h.frame()
	}
}

// line returns the line of the rig.
func rigLine(h *sHarness, hs []doc.Handle) *doc.Line {
	return h.ws.session.Drawing().Entity(hs[0]).(*doc.Line)
}

// pickOption opens drop-down id by its control and clicks the item labelled label.
func (h *sHarness) pickOption(ctl, menu, label string) {
	h.t.Helper()
	h.clickID(ctl)
	h.frame()
	items, _, ok := h.ws.propMenuItems(menu)
	if !ok || h.ws.pe.menu.id != menu {
		h.t.Fatalf("drop-down %s did not open", menu)
	}
	for i, it := range items {
		if it.label == label {
			h.clickID("props-opt-" + strconv.Itoa(i))
			h.frame()
			return
		}
	}
	h.t.Fatalf("drop-down %s has no %q", menu, label)
}

// cmdFocused reports whether the command line has the keyboard.
func (h *sHarness) cmdFocused() bool {
	fh := h.ws.input.FocusHandle()
	return fh != nil && fh.IsFocused(h.hw.Window())
}

func TestPropsLineEditCommitUndoEscapeBlur(t *testing.T) {
	h, hs := propsRig(t, 1)
	h.selectHandles(hs[0])
	ids := h.frameIDs()
	if !ids["props-cat-general"] || !ids["props-cat-geometry"] {
		t.Fatal("a line shows General and Geometry")
	}
	l := rigLine(h, hs)
	h.clickID("props-ctl-start_x")
	h.frame()
	if h.ws.pe.id != "start_x" || !h.ws.pe.field.FocusHandle().IsFocused(h.hw.Window()) {
		t.Fatal("a click on Start X did not open its field with the keyboard")
	}
	h.typeText("5")
	h.key("enter")
	h.frame()
	if l = rigLine(h, hs); l.Start.X != 5 {
		t.Fatalf("Enter: start x %v, want 5", l.Start.X)
	}
	if !h.cmdFocused() || h.ws.pe.id != "" {
		t.Fatal("after Enter the keyboard goes back to the command line")
	}
	h.ws.submit("U", h.hw.Window())
	h.frame()
	if l = rigLine(h, hs); l.Start.X != 0 {
		t.Fatalf("U: start x %v, want 0", l.Start.X)
	}
	// Escape reverts.
	h.clickID("props-ctl-start_x")
	h.frame()
	h.typeText("7")
	h.key("escape")
	h.frame()
	if l = rigLine(h, hs); l.Start.X != 0 || !h.cmdFocused() || h.ws.pe.id != "" {
		t.Fatalf("Escape: start x %v, cmd focused %v", l.Start.X, h.cmdFocused())
	}
	// Blur commits: a press on the canvas takes the keyboard.
	h.clickID("props-ctl-start_x")
	h.frame()
	h.typeText("9")
	cb := h.ws.canvas.bounds
	h.click(gpui.Point{X: cb.Origin.X + 40, Y: cb.Origin.Y + cb.Size.H - 40})
	h.frame()
	if l = rigLine(h, hs); l.Start.X != 9 {
		t.Fatalf("blur: start x %v, want 9", l.Start.X)
	}
}

func TestPropsInvalidValueShowsErrorAndChangesNothing(t *testing.T) {
	h, hs := propsRig(t, 1)
	h.selectHandles(hs[0])
	n := h.ws.session.Modified()
	h.clickID("props-ctl-start_x")
	h.frame()
	h.typeText("abc")
	h.key("enter")
	h.frame()
	if h.ws.pe.errID != "start_x" || h.ws.pe.err == "" || h.ws.pe.id != "start_x" {
		t.Fatalf("no error on the row, or the field closed: %+v", h.ws.pe.errID)
	}
	if rigLine(h, hs).Start.X != 0 || h.ws.session.Modified() != n {
		t.Fatal("an invalid value changed the drawing")
	}
	h.key("escape")
	h.frame()
	if h.ws.pe.errID != "" {
		t.Fatal("Escape keeps the error")
	}
}

func TestPropsColorOnMixedSelectionAndTypeFilter(t *testing.T) {
	h, hs := propsRig(t, 1)
	d := h.ws.session.Drawing()
	h.selectHandles(hs[0], hs[1])
	h.pickOption("props-ctl-color", "color", "Red")
	for _, hd := range hs[:2] {
		if c := d.Entity(hd).Common().Color; c != 1 {
			t.Fatalf("%v colour %d, want 1", hd, c)
		}
	}
	h.ws.submit("U", h.hw.Window())
	for _, hd := range hs[:2] {
		if c := d.Entity(hd).Common().Color; c != doc.ByLayer {
			t.Fatalf("one undo: %v colour %d", hd, c)
		}
	}
	h.frame()
	h.pickOption("props-filter", propTypesMenu, "Circle (1)")
	if h.ws.propsSheet().Target.Class != "CIRCLE" {
		t.Fatal("the filter did not limit the sheet to the circle")
	}
	if !h.frameIDs()["props-row-radius"] {
		t.Fatal("the circle's own rows are missing")
	}
	h.pickOption("props-ctl-color", "color", "Green")
	if d.Entity(hs[1]).Common().Color != 3 || d.Entity(hs[0]).Common().Color != doc.ByLayer {
		t.Fatal("the filtered edit reached the line or missed the circle")
	}
}

func TestPropsReadOnlyWhileCommandRuns(t *testing.T) {
	h, _ := propsRig(t, 1)
	h.ws.run("LINE", h.hw.Window())
	h.frame()
	if !h.frameIDs()["props-busy"] {
		t.Fatal("no busy note while LINE runs")
	}
	h.clickID("props-ctl-color")
	h.frame()
	if h.ws.pe.menu.id != "" {
		t.Fatal("a drop-down opened while a command runs")
	}
	h.clickID("props-ctl-ltscale")
	h.frame()
	if h.ws.pe.id != "" {
		t.Fatal("a field opened while a command runs")
	}
}

func TestPropsLayerTarget(t *testing.T) {
	h, hs := propsRig(t, 1)
	d := h.ws.session.Drawing()
	h.clickID("layer-Doors")
	h.frame()
	if sh := h.ws.propsSheet(); d.CurrentLayer != "Doors" || sh.Target.Kind != props.TargetLayer || sh.Target.Layer != "Doors" {
		t.Fatalf("layer click: current %q, sheet %+v", d.CurrentLayer, sh.Target)
	}
	h.pickOption("props-ctl-color", "color", "Red")
	if d.Layer("Doors").Color != 1 {
		t.Fatalf("layer colour %d", d.Layer("Doors").Color)
	}
	h.ws.submit("U", h.hw.Window())
	if d.Layer("Doors").Color != 3 {
		t.Fatalf("undo: layer colour %d", d.Layer("Doors").Color)
	}
	h.frame()
	d.Layer("Walls").Frozen = true
	h.frame()
	h.clickID("layer-Walls")
	h.frame()
	if sh := h.ws.propsSheet(); d.CurrentLayer != "Doors" || sh.Target.Layer != "Walls" {
		t.Fatalf("frozen layer click: current %q, sheet %+v", d.CurrentLayer, sh.Target)
	}
	h.selectHandles(hs[2])
	if sh := h.ws.propsSheet(); sh.Target.Kind != props.TargetSelection {
		t.Fatal("a later selection did not switch the sheet back")
	}
	h.clickID("layer-Doors")
	h.frame()
	h.key("escape")
	h.frame()
	if sh := h.ws.propsSheet(); sh.Target.Kind != props.TargetSelection || h.ws.session.Selection().Len() != 1 {
		t.Fatal("Escape drops the layer target (and only that)")
	}
	h.clickID("layer-Doors")
	h.frame()
	h.clickID("props-layer-close")
	h.frame()
	if h.ws.propsSheet().Target.Kind != props.TargetSelection {
		t.Fatal("the × did not drop the layer target")
	}
}

func TestPropsDefaultsSheetSetsCECOLOR(t *testing.T) {
	h, _ := propsRig(t, 1)
	if h.ws.propsSheet().Target.Kind != props.TargetDefaults {
		t.Fatal("nothing selected shows the defaults")
	}
	h.pickOption("props-ctl-color", "color", "Blue")
	if got := h.ws.session.Vars().String("CECOLOR"); got != cmdsys.ColorText(5) {
		t.Fatalf("CECOLOR %q", got)
	}
}

func TestPropsDimensionOverrideAndClear(t *testing.T) {
	h, hs := propsRig(t, 1)
	h.selectHandles(hs[2])
	if h.frameIDs()["props-over-dimasz"] {
		t.Fatal("a fresh dimension shows an override")
	}
	h.clickID("props-ctl-dimasz")
	h.frame()
	h.typeText("5")
	h.key("enter")
	h.frame()
	p, _ := h.ws.propsSheet().Find("dimasz")
	ids := h.frameIDs()
	if !p.Overridden || p.Value.Num != 5 || !ids["props-over-dimasz"] || !ids["props-clear-dimasz"] {
		t.Fatalf("override not shown: %+v", p)
	}
	h.savePanel("dim-override.png")
	h.clickID("props-clear-dimasz")
	h.frame()
	p, _ = h.ws.propsSheet().Find("dimasz")
	if p.Overridden || p.Value.Num != 2.5 || h.frameIDs()["props-over-dimasz"] {
		t.Fatalf("clear override: %+v", p)
	}
}

func TestPropsHUDHiddenWhilePanelShown(t *testing.T) {
	h, _ := propsRig(t, 1)
	h.ws.togglePropsHUD()
	h.frame()
	if h.inFrame("props-hud") || !strings.Contains(h.ws.propsChipTip(), i18n.T("ui.props.hud_hidden_tip")) {
		t.Fatal("the HUD shows beside the expanded Properties section")
	}
	h.clickID("props-toggle")
	h.frame()
	h.frame()
	if !h.inFrame("props-hud") || !h.ws.PropsHUDShown() || strings.Contains(h.ws.propsChipTip(), i18n.T("ui.props.hud_hidden_tip")) {
		t.Fatal("folding Properties did not bring the HUD back")
	}
	h.clickID("props-toggle")
	h.frame()
	if h.inFrame("props-hud") || !h.ws.PropsHUDShown() {
		t.Fatal("PROPS lost its state")
	}
}

func TestPropsSamplesInFieldsAndItems(t *testing.T) {
	h, hs := propsRig(t, 1)
	h.selectHandles(hs[0])
	want := map[string]string{"color": "color", "linetype": "linetype", "lineweight": "lineweight"}
	ids := h.frameIDs()
	for id, kind := range want {
		if !ids["props-ctl-"+id+"-sample-"+kind] {
			t.Errorf("%s: no sample in the closed field", id)
		}
		h.clickID("props-ctl-" + id)
		h.frame()
		if !h.frameIDs()["props-opt-0-sample-"+kind] || !h.frameIDs()["props-opt-2-sample-"+kind] {
			t.Errorf("%s: no sample in the items", id)
		}
		h.key("escape")
		h.frame()
	}
	h.selectHandles(hs[2])
	if !h.frameIDs()["props-ctl-arrow1-sample-arrow"] {
		t.Error("arrow: no sample in the closed field")
	}
	h.clickID("props-ctl-arrow1")
	h.frame()
	if !h.frameIDs()["props-opt-1-sample-arrow"] {
		t.Error("arrow: no sample in the items")
	}
}

func TestPropsTypingDoesNotTriggerShortcuts(t *testing.T) {
	h, hs := propsRig(t, 1)
	d := h.ws.session.Drawing()
	h.selectHandles(hs[0])
	h.clickID("props-ctl-start_x")
	h.frame()
	h.typeText("1 u")
	h.key("delete")
	h.key("backspace")
	h.ctrl("z")
	h.frame()
	if d.Len() != 3 || h.ws.session.Active() != "" || h.ws.pe.field.Text() != "1 " {
		t.Fatalf("shortcuts fired while typing: %d objects, active %q, field %q", d.Len(), h.ws.session.Active(), h.ws.pe.field.Text())
	}
	// Shift+Tab moves to the Lineweight drop-down; Delete there erases nothing either.
	h.key("escape")
	h.clickID("props-ctl-start_x")
	h.frame()
	h.hw.Dispatch(gpui.KeyDownEvent{Keystroke: gpui.Keystroke{Key: "tab", Modifiers: gpui.Shift}})
	h.frame()
	if fh := h.ws.pe.handles["lineweight"]; fh == nil || !fh.IsFocused(h.hw.Window()) {
		t.Fatal("Shift+Tab did not focus the previous editable row")
	}
	h.key("delete")
	h.frame()
	if d.Len() != 3 {
		t.Fatal("Delete on a focused drop-down erased the selection")
	}
	h.key("tab")
	h.frame()
	if h.ws.pe.id != "start_x" {
		t.Fatalf("Tab from Lineweight opened %q", h.ws.pe.id)
	}
}

func TestPropsMultiTextOpensTextEdit(t *testing.T) {
	h, _ := propsRig(t, 1)
	d := h.ws.session.Drawing()
	tx, _ := d.Begin()
	m := tx.Add(doc.NewMText(geom.V(0, 0), 2.5, 0, `first\Psecond`))
	tx.Commit()
	h.selectHandles(m)
	p, ok := h.ws.propsSheet().Find("contents")
	if !ok || p.Kind != props.KindMultiText {
		t.Fatalf("no multi-line contents row: %+v", p)
	}
	h.clickID("props-more-contents")
	h.frame()
	if h.ws.textEditor == nil {
		t.Fatal("… did not open TEXTEDIT")
	}
}
