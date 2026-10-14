package ui

import (
	"math"
	"os"
	"path/filepath"
	"testing"

	"cad-lite/doc"
	"cad-lite/internal/gpui"
	"cad-lite/settings"
)

func TestResizeSpanClampsAndHighWins(t *testing.T) {
	for _, c := range []struct{ from, delta, lo, hi, want float32 }{
		{300, 50, 180, 640, 350},
		{300, -500, 180, 640, 180},
		{300, 900, 180, 640, 640},
		{300, 0, 180, 640, 300},
		{300, 0, 180, 100, 100}, // an empty range: the neighbour's need (hi) wins
	} {
		if got := resizeSpan(c.from, c.delta, c.lo, c.hi); got != c.want {
			t.Errorf("resizeSpan(%v, %v, %v, %v) = %v, want %v", c.from, c.delta, c.lo, c.hi, got, c.want)
		}
	}
}

func TestColumnWidthsAlwaysLeaveTheCanvasItsMinimum(t *testing.T) {
	for _, c := range []struct {
		name                  string
		window, side, agent   float32
		sideOff, agentOn      bool
		wantSide, wantAgent   float32 // 0 means "only check the budget"
		wantAtLeastSide       float32
		wantAtLeastAgentWidth float32
	}{
		{name: "defaults", window: 2000, wantSide: sidePanelWidth(2000)},
		{name: "saved width", window: 2000, side: 400, wantSide: 400},
		{name: "saved width too small", window: 2000, side: 50, wantSide: minSideWidth},
		{name: "saved width too large for its range", window: 2000, side: 1800, wantSide: maxSideWidth},
		{name: "saved width larger than the window", window: 700, side: 600, wantAtLeastSide: minSideWidth},
		{name: "agent shrinks before the side column", window: 900, side: 300, agent: 500, agentOn: true, wantSide: 300},
		{name: "both saved wider than the window", window: 800, side: 640, agent: 800, agentOn: true, wantAtLeastSide: minSideWidth, wantAtLeastAgentWidth: minAgentWidth},
		{name: "collapsed column is a strip", window: 1000, side: 400, sideOff: true, wantSide: stripWidth},
		{name: "tiny window", window: 400, side: 300, agent: 300, agentOn: true},
		{name: "tiny window, strip and agent", window: 400, agent: 700, agentOn: true, sideOff: true},
	} {
		t.Run(c.name, func(t *testing.T) {
			side, agent := columnWidths(c.window, c.side, c.agent, c.sideOff, c.agentOn)
			if c.wantSide > 0 && side != c.wantSide {
				t.Errorf("side %v, want %v", side, c.wantSide)
			}
			if side < c.wantAtLeastSide || agent < c.wantAtLeastAgentWidth {
				t.Errorf("side %v agent %v below their minimums", side, agent)
			}
			if !c.agentOn && agent != 0 {
				t.Errorf("hidden agent column has width %v", agent)
			}
			gaps := float32(1)
			if !c.sideOff {
				gaps = splitterSize
			}
			if c.agentOn {
				gaps += splitterSize
			}
			if canvas := c.window - side - agent - gaps; canvas < minCanvasWidth-0.5 {
				t.Errorf("canvas %v px wide with side %v agent %v in a %v window, want at least %v", canvas, side, agent, c.window, float32(minCanvasWidth))
			}
		})
	}
}

func TestColumnRangesNeverCrowdOutTheCanvas(t *testing.T) {
	_, hi := sideRange(800, 0)
	if want := float32(800 - minCanvasWidth - splitterSize); hi != want {
		t.Errorf("side range tops at %v, want %v", hi, want)
	}
	_, hi = sideRange(1000, 300)
	if want := float32(1000 - minCanvasWidth - 2*splitterSize - 300); hi != want {
		t.Errorf("side range with an agent column tops at %v, want %v", hi, want)
	}
	if lo, hi := sideRange(300, 300); lo != minSideWidth || hi != minSideWidth {
		t.Errorf("side range in a tiny window is %v..%v, want it pinned at the minimum", lo, hi)
	}
	_, hi = agentRange(5000, 200, false)
	if hi != maxAgentWidth {
		t.Errorf("agent range tops at %v, want %v", hi, float32(maxAgentWidth))
	}
}

func TestLayersSectionUsesOnlyTheSpaceItsRowsNeed(t *testing.T) {
	const avail = 700
	if got, want := layersHeight(avail, 4, 0, false, false), layersContentHeight(4); got != want {
		t.Errorf("4 rows: height %v, want the content height %v", got, want)
	}
	if got := layersHeight(avail, 0, 0, false, false); got != sectionHeaderHeight {
		t.Errorf("no layers: height %v, want just the header", got)
	}
	// Many rows scroll: Properties keeps its header and a minimal body.
	if got, want := layersHeight(avail, 100, 0, false, false), float32(avail-sectionHeaderHeight-minPropsBody); got != want {
		t.Errorf("100 rows: height %v, want %v", got, want)
	}
	// With Properties folded the list may take everything but its header.
	if got, want := layersHeight(avail, 100, 0, false, true), float32(avail-sectionHeaderHeight); got != want {
		t.Errorf("100 rows, Properties folded: height %v, want %v", got, want)
	}
	// A dragged height caps the section, never enlarges it past its rows.
	if got := layersHeight(avail, 10, 120, false, false); got != 120 {
		t.Errorf("dragged to 120: height %v", got)
	}
	if got, want := layersHeight(avail, 2, 500, false, false), layersContentHeight(2); got != want {
		t.Errorf("dragged past the rows: height %v, want %v", got, want)
	}
	if got := layersHeight(avail, 10, 120, true, false); got != sectionHeaderHeight {
		t.Errorf("folded: height %v, want the header", got)
	}
	lo, hi := layersLimits(avail, 10, false)
	if lo != layersContentHeight(1) || hi != layersContentHeight(10) {
		t.Errorf("limits %v..%v, want one row to all rows", lo, hi)
	}
	if lo, hi := layersLimits(20, 10, false); lo != sectionHeaderHeight || hi != sectionHeaderHeight {
		t.Errorf("limits %v..%v in a column with no room, want the header only", lo, hi)
	}
}

// spanAt returns the vertical extent of the element with the id along the column x, from the
// last frame's hit test; ok is false when nothing hit.
func (h *sHarness) spanAt(id string, x float32) (y0, y1 float32, ok bool) {
	w := h.hw.Window()
	y0, y1 = 1e9, -1
	for y := float32(0); y < float32(w.Bounds().Size.H); y++ {
		gid, hit := w.HitTest(gpui.Point{X: gpui.Pixels(x), Y: gpui.Pixels(y)})
		if !hit {
			continue
		}
		for _, k := range gid.Path {
			if k.Key == id {
				y0, y1 = min(y0, y), max(y1, y+1)
				break
			}
		}
	}
	return y0, y1, y1 >= 0
}

// drag presses at from, moves to to in steps and releases there.
func (h *sHarness) drag(from, to gpui.Point) {
	h.hw.Dispatch(gpui.MouseMoveEvent{Position: from})
	h.hw.Dispatch(gpui.MouseDownEvent{Position: from, Button: gpui.MouseLeft, ClickCount: 1})
	for i := 1; i <= 4; i++ {
		f := gpui.Pixels(i) / 4
		h.hw.Dispatch(gpui.MouseMoveEvent{Position: gpui.Point{X: from.X + (to.X-from.X)*f, Y: from.Y + (to.Y-from.Y)*f}})
	}
	h.hw.Dispatch(gpui.MouseUpEvent{Position: to, Button: gpui.MouseLeft, ClickCount: 1})
	h.frame()
}

func nearPx(a, b float32) bool { return math.Abs(float64(a-b)) < 1.5 }

func TestSideSplitterResizesTheColumnWithinLimits(t *testing.T) {
	ws := NewWorkspace(doc.New(), nil)
	h := openSettingsWS(t, ws, 1400, 800, 1)
	w0, _ := ws.widths(1400)
	p := h.find("split-side")
	h.drag(p, gpui.Point{X: p.X - 80, Y: p.Y})
	if got, _ := ws.widths(1400); !nearPx(got, w0+80) {
		t.Fatalf("dragging 80 px left: column %v -> %v, want %v", w0, got, w0+80)
	}
	p = h.find("split-side")
	h.drag(p, gpui.Point{X: p.X - 2000, Y: p.Y})
	if got, _ := ws.widths(1400); got != float32(1400-minCanvasWidth-splitterSize) && got != maxSideWidth {
		t.Fatalf("a huge drag left: column %v, want the cap", got)
	}
	if got, _ := ws.widths(1400); 1400-got-splitterSize < minCanvasWidth {
		t.Fatalf("column %v leaves the canvas under its minimum", got)
	}
	p = h.find("split-side")
	h.drag(p, gpui.Point{X: 1399, Y: p.Y})
	if got, _ := ws.widths(1400); got != minSideWidth {
		t.Fatalf("a huge drag right: column %v, want the minimum %v", got, float32(minSideWidth))
	}
	// A double click gives the column back its default width.
	p = h.find("split-side")
	h.hw.Dispatch(gpui.MouseMoveEvent{Position: p})
	h.hw.Dispatch(gpui.MouseDownEvent{Position: p, Button: gpui.MouseLeft, ClickCount: 2})
	h.hw.Dispatch(gpui.MouseUpEvent{Position: p, Button: gpui.MouseLeft, ClickCount: 2})
	if ws.lay.sideW != 0 {
		t.Fatalf("double click left the saved width at %v, want automatic", ws.lay.sideW)
	}
}

func TestAgentSplitterResizesTheAgentColumn(t *testing.T) {
	ws := NewWorkspace(doc.New(), nil)
	p := ws.AgentPanel()
	p.probed, p.choices = true, nil
	p.hist.loaded, p.hist.off = true, true
	p.Show()
	h := openSettingsWS(t, ws, 1600, 800, 1)
	_, a0 := ws.widths(1600)
	sp := h.find("split-agent")
	h.drag(sp, gpui.Point{X: sp.X - 60, Y: sp.Y})
	if _, got := ws.widths(1600); !nearPx(got, a0+60) {
		t.Fatalf("agent column %v -> %v, want %v", a0, got, a0+60)
	}
	sp = h.find("split-agent")
	h.drag(sp, gpui.Point{X: 1599, Y: sp.Y})
	if _, got := ws.widths(1600); got != minAgentWidth {
		t.Fatalf("agent column %v, want the minimum %v", got, float32(minAgentWidth))
	}
}

func TestSavedWidthsLargerThanTheWindowStillLeaveTheCanvas(t *testing.T) {
	ws := NewWorkspace(doc.New(), nil)
	ws.lay.sideW, ws.lay.agentW = 640, 800
	ws.AgentPanel().Show()
	h := openSettingsWS(t, ws, 900, 700, 1)
	side, agent := ws.widths(900)
	if canvas := 900 - side - agent - 2*splitterSize; canvas < minCanvasWidth-0.5 {
		t.Fatalf("canvas %v px wide (side %v, agent %v), want at least %v", canvas, side, agent, float32(minCanvasWidth))
	}
	if !h.has("split-agent") || !h.has("canvas") {
		t.Fatal("agent column or canvas missing")
	}
	// The saved widths are kept: a larger window gets them back.
	if side, _ := ws.widths(3000); side != 640 {
		t.Fatalf("saved side width lost: %v", side)
	}
}

func TestLayersListFitsItsRowsAndPropertiesTakesTheRest(t *testing.T) {
	ws := NewWorkspace(doc.New(), nil)
	for _, n := range []string{"a", "b", "c"} {
		ws.Session().Drawing().AddLayer(n, 3)
	}
	h := openSettingsWS(t, ws, 1400, 900, 1)
	h.frame()
	side, _ := ws.widths(1400)
	x := 1400 - side + 30
	ly0, ly1, ok := h.spanAt("layers-toggle", x)
	if !ok {
		t.Fatal("no Layers header")
	}
	n := len(ws.Session().Drawing().Layers())
	py0, _, ok := h.spanAt("props-toggle", x)
	if !ok {
		t.Fatal("no Properties header")
	}
	want := layersContentHeight(n) + splitterSize
	if got := py0 - (ly0 - (sectionHeaderHeight-ly1+ly0)/2); math.Abs(float64(got-want)) > 6 {
		t.Fatalf("Properties starts %v px below the Layers header, want about %v (its %d rows)", got, want, n)
	}
	_, pe, _ := h.spanAt("props-scroll", x)
	if _, se, _ := h.spanAt("side-panel", x); se-pe > 1 {
		t.Fatalf("Properties ends at %v, the column at %v: it must take the rest", pe, se)
	}
}

func TestLayersSplitterCapsTheListAndScrolls(t *testing.T) {
	ws := NewWorkspace(doc.New(), nil)
	for _, n := range []string{"a", "b", "c", "d", "e", "f", "g"} {
		ws.Session().Drawing().AddLayer(n, 3)
	}
	h := openSettingsWS(t, ws, 1400, 900, 1)
	h.frame()
	n := len(ws.Session().Drawing().Layers())
	full := layersContentHeight(n)
	sp := h.find("split-layers")
	h.drag(sp, gpui.Point{X: sp.X, Y: sp.Y - 70})
	got := layersHeight(ws.sectionAvail(), n, ws.lay.layersH, false, false)
	if !nearPx(got, full-70) {
		t.Fatalf("dragging 70 px up: Layers %v, want %v", got, full-70)
	}
	sp = h.find("split-layers")
	h.drag(sp, gpui.Point{X: sp.X, Y: sp.Y + 500})
	if got := layersHeight(ws.sectionAvail(), n, ws.lay.layersH, false, false); got != full {
		t.Fatalf("dragging past the rows: Layers %v, want the content height %v", got, full)
	}
	sp = h.find("split-layers")
	h.drag(sp, gpui.Point{X: sp.X, Y: sp.Y - 900})
	if got := layersHeight(ws.sectionAvail(), n, ws.lay.layersH, false, false); got != layersContentHeight(1) {
		t.Fatalf("dragging to the top: Layers %v, want one row (%v)", got, layersContentHeight(1))
	}
}

func TestSectionsAndColumnFoldAndRestoreTheirSize(t *testing.T) {
	ws := NewWorkspace(doc.New(), nil)
	h := openSettingsWS(t, ws, 1400, 900, 1)
	side, _ := ws.widths(1400)
	x := 1400 - side + 30
	_, pe0, _ := h.spanAt("props-scroll", x)
	top, _, _ := h.spanAt("side-panel", x)

	h.clickID("layers-toggle")
	h.frame()
	if !ws.lay.layersOff || h.has("layers-scroll") {
		t.Fatal("Layers did not fold to its header")
	}
	_, pe, _ := h.spanAt("props-scroll", x)
	py0, _, _ := h.spanAt("props-scroll", x)
	if py0 > top+2*sectionHeaderHeight+sideTopPad+splitterSize+2 {
		t.Fatalf("with Layers folded Properties starts %v px below the column top: it should take the freed space", py0-top)
	}
	if pe != pe0 {
		t.Fatalf("Properties end moved from %v to %v", pe0, pe)
	}
	h.clickID("layers-toggle")
	h.frame()
	if ws.lay.layersOff || !h.has("layers-scroll") {
		t.Fatal("Layers did not unfold")
	}

	h.clickID("props-toggle")
	h.frame()
	if !ws.lay.propsOff || h.has("props-scroll") {
		t.Fatal("Properties did not fold")
	}
	h.clickID("props-toggle")
	h.frame()

	// The column folds to a strip and comes back at the width the user gave it.
	ws.lay.sideW = 333
	h.frame()
	h.clickID("side-collapse")
	h.frame()
	if !ws.lay.sideOff || !h.has("side-strip") || h.has("layers-toggle") {
		t.Fatal("column did not fold to a strip")
	}
	if w, _ := ws.widths(1400); w != stripWidth {
		t.Fatalf("strip is %v wide, want %v", w, float32(stripWidth))
	}
	h.clickID("side-expand")
	h.frame()
	if w, _ := ws.widths(1400); ws.lay.sideOff || w != 333 || !h.has("layers-toggle") {
		t.Fatalf("column %v wide after expanding (off=%v), want its 333 px back", w, ws.lay.sideOff)
	}
}

func TestHistoryFoldsToTheInputRowAndRestoresItsHeight(t *testing.T) {
	ws := NewWorkspace(doc.New(), nil)
	h := chromeRig(t, ws)
	ws.cmd.histH = 150
	h.frame()
	if !h.has("cmd-history") {
		t.Fatal("no history region")
	}
	h.clickID("cmd-collapse")
	h.frame()
	if !ws.lay.histOff || h.has("cmd-history") || !h.has("cmd-box") {
		t.Fatal("history did not fold away leaving the input row")
	}
	// Dragging the folded handle does nothing.
	p := h.find("cmd-handle")
	h.drag(p, gpui.Point{X: p.X, Y: p.Y - 100})
	if ws.cmd.histH != 150 {
		t.Fatalf("a drag on the folded handle changed the height to %v", ws.cmd.histH)
	}
	h.clickID("cmd-collapse")
	h.frame()
	if ws.lay.histOff || !h.has("cmd-history") || ws.historyHeight() != 150 {
		t.Fatalf("history height %v after expanding, want 150", ws.historyHeight())
	}
}

func TestRibbonFoldsFromTheTitleBar(t *testing.T) {
	ws := NewWorkspace(doc.New(), nil)
	h := chromeRig(t, ws)
	if !h.has("tool-line") {
		t.Fatal("no ribbon at start")
	}
	h.clickID("tool-ribbon")
	h.frame()
	if h.has("tool-line") || !ws.lay.ribbonOff || !h.has("tool-ribbon") {
		t.Fatal("ribbon did not fold, or its toggle went with it")
	}
	h.clickID("tool-ribbon")
	h.frame()
	if !h.has("tool-line") {
		t.Fatal("ribbon did not come back")
	}
}

func TestLayoutPersistsInTheSettingsFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.toml")
	ws := NewWorkspace(doc.New(), nil)
	if err := ws.LoadSettingsFrom(path); err != nil {
		t.Fatal(err)
	}
	h := openSettingsWS(t, ws, 1400, 900, 1)
	p := h.find("split-side")
	h.drag(p, gpui.Point{X: p.X - 50, Y: p.Y})
	h.clickID("props-toggle")
	h.clickID("tool-ribbon")
	h.frame()
	h.clickID("cmd-collapse")
	if err := ws.SaveSettings(); err != nil {
		t.Fatal(err)
	}
	got, _, err := settings.LoadWarn(path)
	if err != nil {
		t.Fatal(err)
	}
	l := got.Layout
	if want := int(sidePanelWidth(1400) + 50 + 0.5); l.SideWidth < want-1 || l.SideWidth > want+1 {
		t.Errorf("saved side width %d, want about %d", l.SideWidth, want)
	}
	if !l.PropsCollapsed || !l.RibbonCollapsed || !l.HistoryCollapsed || l.LayersCollapsed || l.SideCollapsed {
		t.Errorf("saved collapsed flags %+v", l)
	}

	// A new workspace reading the file starts with the same arrangement.
	ws2 := NewWorkspace(doc.New(), nil)
	if err := ws2.LoadSettingsFrom(path); err != nil {
		t.Fatal(err)
	}
	if ws2.lay.sideW != float32(l.SideWidth) || !ws2.lay.propsOff || !ws2.lay.ribbonOff || !ws2.lay.histOff || ws2.lay.layersOff {
		t.Errorf("restored layout %+v", ws2.lay)
	}
	h2 := openSettingsWS(t, ws2, 1400, 900, 1)
	if h2.has("tool-line") || h2.has("cmd-history") || h2.has("props-scroll") {
		t.Error("restored workspace does not show the folded panels folded")
	}
}

func TestHistoryHeightPersists(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.toml")
	if err := os.WriteFile(path, []byte("[layout]\nhistory_height = 200\nlayers_height = 90\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	ws := NewWorkspace(doc.New(), nil)
	if err := ws.LoadSettingsFrom(path); err != nil {
		t.Fatal(err)
	}
	if ws.historyHeight() != 200 || ws.lay.layersH != 90 {
		t.Fatalf("history %v layers cap %v, want 200 and 90", ws.historyHeight(), ws.lay.layersH)
	}
	if err := ws.SaveSettings(); err != nil {
		t.Fatal(err)
	}
	got, _, _ := settings.LoadWarn(path)
	if got.Layout.HistoryHeight != 200 || got.Layout.LayersHeight != 90 {
		t.Fatalf("saved layout %+v", got.Layout)
	}
}
