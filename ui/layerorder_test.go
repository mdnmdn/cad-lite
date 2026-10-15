package ui

import (
	"cad-lite/doc"
	"cad-lite/geom"
	"os"
	"slices"
	"strings"
	"testing"
)

func uiLayerNames(ws *Workspace) []string {
	var out []string
	for _, l := range ws.session.Drawing().Layers() {
		out = append(out, l.Name)
	}
	return out
}
func wantUILayers(t *testing.T, ws *Workspace, names ...string) {
	t.Helper()
	if got := uiLayerNames(ws); !slices.Equal(got, names) {
		t.Fatalf("order=%v want%v", got, names)
	}
}
func TestLayerOrderSidePanelDirtyUndoAndCurrent(t *testing.T) {
	h := layerRig(t)
	d := h.ws.session.Drawing()
	h.clickID("layer-up-Walls")
	h.hw.Pixels()
	wantUILayers(t, h.ws, "Walls", "0", "Doors", "Windows", "wall-hatch")
	if d.CurrentLayer != "0" || !h.ws.session.Dirty() || d.UndoDepth() != 1 || h.ws.session.Modified() != 1 {
		t.Fatal("reorder state", d.CurrentLayer, h.ws.session.Modified(), d.UndoDepth())
	}
	if h.find("layer-Walls").Y >= h.find("layer-0").Y {
		t.Fatal("panel did not follow canonical order")
	}
	h.ws.session.Run("U")
	h.hw.Window().Refresh()
	h.hw.Pixels()
	wantUILayers(t, h.ws, "0", "Walls", "Doors", "Windows", "wall-hatch")
	h.ws.session.Run("REDO")
	h.hw.Window().Refresh()
	h.hw.Pixels()
	wantUILayers(t, h.ws, "Walls", "0", "Doors", "Windows", "wall-hatch")
	h.ws.session.MarkSaved()
	before := h.ws.session.Modified()
	h.clickID("layer-up-Walls")
	h.hw.Pixels()
	if h.ws.session.Modified() != before || h.ws.session.Dirty() {
		t.Fatal("boundary no-op became dirty")
	}
	h.clickID("layer-up-0")
	h.hw.Pixels()
	if d.CurrentLayer != "0" || !h.ws.session.Dirty() {
		t.Fatal("zero layer could not reorder")
	}
	h.ws.session.SetFiles(layerOrderFiles{})
	h.ws.session.Run("QUIT")
	if !strings.Contains(h.ws.session.Prompt(), "Save changes") {
		t.Fatal("reorder did not reach save prompt", h.ws.session.Prompt())
	}
	h.ws.session.Escape()
}
func TestLayerOrderFilteredPopoverUsesFullNeighborsAndKeepsHighlight(t *testing.T) {
	h := layerRig(t)
	h.openLayerPop()
	h.typeText("wall")
	h.key("down")
	h.hw.Pixels()
	if h.ws.status().hi != 1 {
		t.Fatal("highlight setup")
	}
	h.clickID("layer-pop-up-wall-hatch")
	h.hw.Pixels()
	wantUILayers(t, h.ws, "0", "Walls", "Doors", "wall-hatch", "Windows")
	if !h.ws.LayerPopoverShown() || h.ws.status().hi != 1 || h.ws.session.Drawing().CurrentLayer != "0" {
		t.Fatal("reorder picked/closed/lost highlight")
	}
	h.clickID("layer-pop-up-wall-hatch")
	h.hw.Pixels()
	h.clickID("layer-pop-up-wall-hatch")
	h.hw.Pixels()
	wantUILayers(t, h.ws, "0", "wall-hatch", "Walls", "Doors", "Windows")
	if h.ws.status().hi != 0 {
		t.Fatal("highlight did not follow identity", h.ws.status().hi)
	}
}
func TestLayerOrderBusyGuardsCommandsAndOpenTransactions(t *testing.T) {
	h := layerRig(t)
	before := uiLayerNames(h.ws)
	h.ws.session.Run("LINE")
	h.hw.Window().Refresh()
	h.hw.Pixels()
	h.clickID("layer-up-Doors")
	h.hw.Pixels()
	if !slices.Equal(before, uiLayerNames(h.ws)) || h.ws.session.Active() != "LINE" || h.ws.session.Dirty() || !strings.Contains(strings.Join(h.ws.session.History(), "\n"), "before reordering layers") {
		t.Fatal("busy command not refused", h.ws.session.History())
	}
	h.ws.session.Escape()
	tx, _ := h.ws.session.Drawing().Begin()
	h.hw.Window().Refresh()
	h.hw.Pixels()
	h.clickID("layer-up-Doors")
	h.hw.Pixels()
	if !slices.Equal(before, uiLayerNames(h.ws)) || h.ws.session.Dirty() {
		t.Fatal("open transaction mutated")
	}
	tx.Abort()
}
func TestLayerOrderVisualArtifacts(t *testing.T) {
	if err := os.MkdirAll("../_tmp/n23", 0755); err != nil {
		t.Fatal(err)
	}
	d := doc.New()
	d.AddLayer("Red", 1).Lineweight = 150
	d.AddLayer("Blue", 5).Lineweight = 150
	tx, _ := d.Begin()
	tx.Add(&doc.Line{EntityCommon: doc.EntityCommon{Layer: "Red"}, Start: geom.V(-20, -15), End: geom.V(20, 15)})
	tx.Add(&doc.Line{EntityCommon: doc.EntityCommon{Layer: "Blue"}, Start: geom.V(-20, 15), End: geom.V(20, -15)})
	tx.Commit()
	h := openSettingsWS(t, NewWorkspace(d, nil), 1280, 800, 1)
	h.ws.session.Script("LWDISPLAY 1 ZOOM E")
	h.hw.Window().Refresh()
	h.hw.Pixels()
	h.clickID("layer-up-Red")
	h.hw.Pixels()
	if err := h.hw.SavePNG("../_tmp/n23/panel-after.png"); err != nil {
		t.Fatal(err)
	}
	h.openLayerPop()
	h.clickID("layer-pop-up-Blue")
	h.hw.Pixels()
	if err := h.hw.SavePNG("../_tmp/n23/popover.png"); err != nil {
		t.Fatal(err)
	}
	n := openSettingsWS(t, NewWorkspace(d, nil), 800, 640, 1)
	n.ws.lay.sideW = 180
	n.hw.Window().Refresh()
	n.hw.Pixels()
	n.openLayerPop()
	n.hw.Pixels()
	if err := n.hw.SavePNG("../_tmp/n23/narrow.png"); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"layer-pop-up-Red", "layer-pop-down-Red", "layer-pop-lock-Red", "layer-pop-vis-Red"} {
		if !n.inFrame(id) {
			t.Fatal("narrow control missing", id)
		}
	}
}

type layerOrderFiles struct{}

func (layerOrderFiles) Open(string) (*doc.Drawing, []string, error) { return doc.New(), nil, nil }
func (layerOrderFiles) Save(string, *doc.Drawing) ([]string, error) { return nil, nil }
