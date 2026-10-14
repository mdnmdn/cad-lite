package ui

import (
	"runtime"
	"strings"
	"testing"
	"time"
	"weak"

	"cad-lite/doc"
	"cad-lite/internal/gpui"
)

// layerRig opens a workspace whose drawing has a few layers besides 0.
func layerRig(t *testing.T) *sHarness {
	t.Helper()
	d := doc.New()
	d.AddLayer("Walls", 1)
	d.AddLayer("Doors", 3)
	d.AddLayer("Windows", 5)
	d.AddLayer("wall-hatch", 8)
	return openSettingsWS(t, NewWorkspace(d, nil), 1280, 800, 1)
}

// typeText types s into the focused field, one character at a time.
func (h *sHarness) typeText(s string) {
	for _, r := range s {
		c := string(r)
		h.hw.Dispatch(gpui.KeyDownEvent{Keystroke: gpui.Keystroke{Key: c, KeyChar: &c}, PreferCharacterInput: true})
	}
	h.hw.Pixels()
}

// popRows returns which layers the popover lists, in the last frame.
func (h *sHarness) popRows() map[string]bool {
	ids := h.frameIDs()
	out := map[string]bool{}
	for _, l := range h.ws.session.Drawing().Layers() {
		if ids["layer-pop-"+l.Name] {
			out[l.Name] = true
		}
	}
	return out
}

func (h *sHarness) openLayerPop() {
	h.t.Helper()
	h.clickID("status-layer")
	h.hw.Pixels()
	if !h.ws.LayerPopoverShown() {
		h.t.Fatal("a click on the status bar layer did not open the popover")
	}
}

func TestLayerPopoverOpensFiltersAndPicks(t *testing.T) {
	h := layerRig(t)
	h.openLayerPop()
	if got := h.popRows(); len(got) != 5 {
		t.Fatalf("popover lists %v, want all 5 layers", got)
	}
	if fh := h.ws.layerFilter().FocusHandle(); fh == nil || !fh.IsFocused(h.hw.Window()) {
		t.Fatal("the filter does not have the keyboard on open")
	}
	h.typeText("WAL")
	if got := h.popRows(); len(got) != 2 || !got["Walls"] || !got["wall-hatch"] {
		t.Fatalf("filter WAL lists %v, want Walls and wall-hatch (case-insensitive)", got)
	}
	h.typeText("L")
	h.key("down") // the highlight moves within the matches
	h.hw.Pixels()
	if h.ws.status().hi != 1 {
		t.Fatalf("Down moved the highlight to %d", h.ws.status().hi)
	}
	h.key("up")
	h.hw.Pixels()
	h.key("up") // stops at the first row
	h.hw.Pixels()
	if h.ws.status().hi != 0 {
		t.Fatalf("Up moved the highlight to %d", h.ws.status().hi)
	}
	h.typeText("s")
	if got := h.popRows(); len(got) != 1 || !got["Walls"] {
		t.Fatalf("filter WALLs lists %v", got)
	}
	h.key("enter")
	h.hw.Pixels()
	if cur := h.ws.session.Drawing().CurrentLayer; cur != "Walls" {
		t.Fatalf("Enter on the single match: current layer %q", cur)
	}
	if h.ws.LayerPopoverShown() {
		t.Error("the popover stayed open after Enter")
	}
	if fh := h.ws.input.FocusHandle(); fh == nil || !fh.IsFocused(h.hw.Window()) {
		t.Error("closing the popover did not give the command line the keyboard back")
	}
	if !h.inFrame("status-layer") || h.inFrame("layer-pop") {
		t.Error("the popover is still drawn")
	}
}

func TestLayerPopoverClickPicksAndSwitchesToggle(t *testing.T) {
	h := layerRig(t)
	d := h.ws.session.Drawing()
	h.openLayerPop()
	h.clickID("layer-pop-vis-Doors")
	h.hw.Pixels()
	if !d.Layer("Doors").Off || d.CurrentLayer == "Doors" || !h.ws.LayerPopoverShown() {
		t.Fatalf("visibility switch: off %v current %q open %v", d.Layer("Doors").Off, d.CurrentLayer, h.ws.LayerPopoverShown())
	}
	h.clickID("layer-pop-lock-Windows")
	h.hw.Pixels()
	if !d.Layer("Windows").Locked || d.CurrentLayer == "Windows" {
		t.Fatal("lock switch did not lock, or picked the layer")
	}
	h.clickID("layer-pop-Doors")
	h.hw.Pixels()
	if d.CurrentLayer != "Doors" || h.ws.LayerPopoverShown() {
		t.Fatalf("a click on a row: current %q, open %v", d.CurrentLayer, h.ws.LayerPopoverShown())
	}
	// The side panel has the same switches, on the same flags.
	if !h.inFrame("layer-lock-Windows") || !h.inFrame("layer-vis-Doors") {
		t.Fatal("the side panel lacks the same switches")
	}
	h.clickID("layer-lock-Windows")
	h.hw.Pixels()
	if d.Layer("Windows").Locked || d.CurrentLayer != "Doors" {
		t.Fatal("the side panel's lock switch did not unlock without picking")
	}
}

func TestLayerPopoverEscapeAndOutsideClickClose(t *testing.T) {
	h := layerRig(t)
	h.openLayerPop()
	h.key("escape")
	h.hw.Pixels()
	if h.ws.LayerPopoverShown() {
		t.Fatal("Escape did not close the popover")
	}
	h.openLayerPop()
	cur := h.ws.session.Drawing().CurrentLayer
	sel := h.ws.session.Selection().Len()
	h.click(gpui.Point{X: 500, Y: 400}) // on the canvas
	h.hw.Pixels()
	if h.ws.LayerPopoverShown() {
		t.Fatal("a click outside did not close the popover")
	}
	if h.ws.session.Drawing().CurrentLayer != cur || h.ws.session.Selection().Len() != sel {
		t.Error("the closing click reached the drawing")
	}
	// A click on the item toggles it closed too (the backdrop takes it).
	at := h.mustLocate("status-layer")
	h.openLayerPop()
	h.click(at)
	h.hw.Pixels()
	if h.ws.LayerPopoverShown() {
		t.Error("a click on the layer item did not close the popover")
	}
}

func TestLayerPopoverNewLayer(t *testing.T) {
	h := layerRig(t)
	h.openLayerPop()
	h.typeText("Furniture")
	if len(h.popRows()) != 0 || !h.inFrame("layer-pop-new") {
		t.Fatal("an unmatched filter should list nothing and offer a new layer")
	}
	h.clickID("layer-pop-new")
	h.hw.Pixels()
	d := h.ws.session.Drawing()
	if d.Layer("Furniture") == nil || d.CurrentLayer != "Furniture" || h.ws.LayerPopoverShown() {
		t.Fatalf("New layer: layer %v current %q open %v", d.Layer("Furniture"), d.CurrentLayer, h.ws.LayerPopoverShown())
	}
}

func TestStatusLayerHasTooltip(t *testing.T) {
	h := layerRig(t)
	for _, id := range []string{"status-layer", "status-props"} {
		if !h.hoverTip(h.find(id)) {
			t.Errorf("%s: no tooltip after 600 ms", id)
		}
	}
	h.openLayerPop()
	for _, id := range []string{"layer-pop-lock-Walls", "layer-pop-vis-Walls", "layer-pop-new"} {
		if !h.hoverTip(h.find(id)) {
			t.Errorf("%s: no tooltip after 600 ms", id)
		}
	}
}

// A frozen layer cannot be current, from the popover (click or Enter) or the side panel, as
// -LAYER Set refuses it.
func TestFrozenLayerCannotBeMadeCurrent(t *testing.T) {
	h := layerRig(t)
	d := h.ws.session.Drawing()
	d.Layer("Doors").Frozen = true
	h.hw.Window().Refresh()
	h.hw.Pixels()
	h.clickID("layer-Doors")
	h.hw.Pixels()
	if d.CurrentLayer == "Doors" {
		t.Fatal("the side panel made a frozen layer current")
	}
	h.openLayerPop()
	h.clickID("layer-pop-Doors")
	h.hw.Pixels()
	if d.CurrentLayer == "Doors" || !h.ws.LayerPopoverShown() {
		t.Fatalf("a click on a frozen row: current %q, open %v", d.CurrentLayer, h.ws.LayerPopoverShown())
	}
	h.typeText("door")
	h.key("enter")
	h.hw.Pixels()
	if d.CurrentLayer == "Doors" || !h.ws.LayerPopoverShown() {
		t.Fatalf("Enter on a frozen match: current %q, open %v", d.CurrentLayer, h.ws.LayerPopoverShown())
	}
	if !h.hoverTip(h.find("layer-pop-Doors")) {
		t.Error("the frozen row has no tooltip")
	}
}

// "wall" is not offered as a new layer beside "Walls"'s twin "WALL": names compare without case.
func TestLayerPopoverNewIgnoresCase(t *testing.T) {
	h := layerRig(t)
	d := h.ws.session.Drawing()
	d.AddLayer("WALL", 2)
	n := len(d.Layers())
	h.openLayerPop()
	h.typeText("wall")
	if lbl := h.ws.layerFilter().Text(); layerFold(d, lbl) == nil {
		t.Fatal("layerFold missed WALL")
	}
	h.clickID("layer-pop-new")
	h.hw.Pixels()
	if len(d.Layers()) != n || d.Layer("wall") != nil {
		t.Fatalf("New made a case twin: %d layers", len(d.Layers()))
	}
	h.key("escape") // LAYER waits for a name on the command line
	h.hw.Pixels()
}

// Switching drawings (Ctrl+Tab) or opening the settings closes the popover.
func TestLayerPopoverClosesOnTabSwitchAndSettings(t *testing.T) {
	h := layerRig(t)
	h.ws.run("NEW", h.hw.Window())
	h.hw.Pixels()
	h.openLayerPop()
	h.ctrl("tab")
	h.hw.Pixels()
	h.hw.Pixels()
	if h.ws.LayerPopoverShown() {
		t.Fatal("the popover stayed open over another drawing")
	}
	h.openLayerPop()
	h.ws.OpenSettings(h.hw.Window())
	h.hw.Pixels()
	if h.ws.LayerPopoverShown() {
		t.Fatal("the popover stayed open under the settings")
	}
	h.key("escape")
	h.hw.Pixels()
	if h.ws.SettingsOpen() || h.ws.LayerPopoverShown() {
		t.Fatal("Escape left a panel open")
	}
}

// The popover's filter does not keep its Workspace alive.
func TestLayerPopoverDoesNotLeakTheWorkspace(t *testing.T) {
	gone := make(chan struct{})
	func() {
		ws := NewWorkspace(doc.New(), nil)
		ws.layerFilter()
		ws.selectionSummary()
		ws.status().popSession = weak.Make(ws.session)
		runtime.AddCleanup(ws, func(c chan struct{}) { close(c) }, gone)
	}()
	for i := 0; i < 20; i++ {
		runtime.GC()
		select {
		case <-gone:
			return
		case <-time.After(20 * time.Millisecond):
		}
	}
	t.Fatal("the Workspace was never collected")
}

func TestOpenedLayerPopoverDoesNotRetainTheApp(t *testing.T) {
	gone := make(chan struct{})
	func() {
		ws := NewWorkspace(doc.New(), nil)
		p := gpui.NewHeadlessPlatform()
		app := gpui.NewAppWithPlatform(p)
		if err := app.OpenWindow(gpui.WindowOptions{Width: 1280, Height: 800},
			func(*gpui.Window, *gpui.App) gpui.EntityRoot { return ws }); err != nil {
			t.Fatal(err)
		}
		ws.openLayerPopover(p.Windows()[0].Window(), app)
		runtime.AddCleanup(ws, func(c chan struct{}) { close(c) }, gone)
	}()
	for i := 0; i < 20; i++ {
		runtime.GC()
		select {
		case <-gone:
			return
		case <-time.After(20 * time.Millisecond):
		}
	}
	t.Fatal("the opened popover kept the App and its Workspace alive")
}

func TestLayerControlsMarkDrawingModified(t *testing.T) {
	for _, prefix := range []string{"layer-", "layer-pop-"} {
		for _, suffix := range []string{"lock-Walls", "vis-Walls", "Walls"} {
			t.Run(prefix+suffix, func(t *testing.T) {
				h := layerRig(t)
				h.ws.session.MarkSaved()
				if prefix == "layer-pop-" {
					h.openLayerPop()
				}
				h.clickID(prefix + suffix)
				h.hw.Pixels()
				if !h.ws.session.Dirty() || h.ws.session.Modified() != 1 {
					t.Fatalf("persistent layer edit is not counted: dirty %v modified %d", h.ws.session.Dirty(), h.ws.session.Modified())
				}
			})
		}
	}
}

func TestLayerCurrentSelectionNoOpStaysClean(t *testing.T) {
	h := layerRig(t)
	d := h.ws.session.Drawing()
	d.Layer("Walls").Frozen = true
	for _, id := range []string{"layer-0", "layer-Walls", "layer-pop-0", "layer-pop-Walls"} {
		h.ws.session.MarkSaved()
		if strings.HasPrefix(id, "layer-pop-") {
			h.openLayerPop()
		}
		h.clickID(id)
		h.hw.Pixels()
		if h.ws.session.Dirty() {
			t.Fatalf("%s dirtied an unchanged current layer", id)
		}
		if h.ws.LayerPopoverShown() {
			h.key("escape")
			h.hw.Pixels()
		}
	}
}

func TestLayerPopoverNewLayerMarksDrawingModified(t *testing.T) {
	h := layerRig(t)
	h.ws.session.MarkSaved()
	h.openLayerPop()
	h.typeText("Furniture")
	h.clickID("layer-pop-new")
	h.hw.Pixels()
	if !h.ws.session.Dirty() || h.ws.session.Drawing().Layer("Furniture") == nil {
		t.Fatal("creating a persistent layer left the drawing clean")
	}
}
