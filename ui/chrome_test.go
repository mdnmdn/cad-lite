package ui

import (
	"math"
	"testing"

	"cad-lite/doc"
	"cad-lite/geom"
	"cad-lite/i18n"
	"cad-lite/internal/gpui"
)

// chromeRig is a 1280x800 window; the ribbon fits on one row and the canvas is large.
func chromeRig(t *testing.T, ws *Workspace) *sHarness {
	t.Helper()
	return openSettingsWS(t, ws, 1280, 800, 1)
}

func (h *sHarness) frame() {
	h.hw.Window().Refresh()
	h.hw.Pixels()
}

func (h *sHarness) width(id string) float32 {
	h.t.Helper()
	w := h.hw.Window()
	var lo, hi float32 = 1e9, -1
	for x := float32(0); x < 1280; x++ {
		gid, ok := w.HitTest(gpui.Point{X: gpui.Pixels(x), Y: 26})
		if ok && len(gid.Path) > 0 && gid.Path[len(gid.Path)-1].Key == id {
			lo, hi = min(lo, x), max(hi, x)
		}
	}
	if hi < 0 {
		h.t.Fatalf("%s not on the title bar row", id)
	}
	return hi - lo + 1
}

func TestDocChipShowsTheFileNameAndADotOnlyWhileDirty(t *testing.T) {
	h := dialogRig(t, nil)
	if got, want := h.ws.docName(), i18n.T("ui.title.untitled"); got != want {
		t.Fatalf("a drawing without a file is called %q, want %q", got, want)
	}
	h.frame()
	clean := h.width("doc-chip")
	h.ws.Session().Script("LINE 0,0 10,10  ")
	h.frame()
	dirty := h.width("doc-chip")
	if !h.ws.Session().Dirty() || dirty < clean+8 {
		t.Fatalf("dirty=%v: chip %v px clean, %v px with unsaved changes; want the dot to widen it", h.ws.Session().Dirty(), clean, dirty)
	}
	// A freshly opened drawing is clean and carries its file's name.
	h = dialogRig(t, nil)
	h.ws.Session().Script("OPEN " + savedDrawing(t))
	h.frame()
	if h.ws.Session().Dirty() {
		t.Fatal("a freshly opened drawing must be clean")
	}
	if got := h.ws.docName(); got != "plan.dxf" {
		t.Fatalf("chip names %q, want the file name plan.dxf", got)
	}
}

func TestZoomPillButtonsZoomAndFit(t *testing.T) {
	ws := NewWorkspace(doc.New(), nil)
	h := chromeRig(t, ws)
	v := ws.View()
	s0 := v.Scale
	h.clickID("canvas-zoom-in")
	if math.Abs(v.Scale/s0-zoomButtonFactor) > 1e-9 {
		t.Fatalf("zoom in: scale %v -> %v, want a step of %v", s0, v.Scale, zoomButtonFactor)
	}
	h.clickID("canvas-zoom-out")
	if math.Abs(v.Scale/s0-1) > 1e-9 {
		t.Fatalf("zoom out did not undo zoom in: %v", v.Scale)
	}
	ws.Session().Script("LINE 500,500 900,700  ")
	h.frame()
	box := ws.Session().Drawing().Extents()
	vis := v.Visible()
	if vis.Min.X <= box.Min.X && vis.Max.X >= box.Max.X && vis.Min.Y <= box.Min.Y && vis.Max.Y >= box.Max.Y {
		t.Fatal("setup: the line should be out of view")
	}
	n := v.History().Len()
	h.clickID("canvas-zoom-fit")
	vis = v.Visible()
	if !(vis.Min.X <= box.Min.X && vis.Max.X >= box.Max.X && vis.Min.Y <= box.Min.Y && vis.Max.Y >= box.Max.Y) {
		t.Fatalf("zoom extents did not frame %v: visible %v", box, vis)
	}
	if v.History().Len() != n+1 {
		t.Fatalf("zoom extents must be one ZOOM Previous step: %d -> %d", n, v.History().Len())
	}
}

func TestPressesOnTheZoomPillNeverReachTheCanvas(t *testing.T) {
	ws := NewWorkspace(doc.New(), nil)
	h := chromeRig(t, ws)
	ws.Session().Run("LINE")
	lines := len(ws.Session().History())
	h.clickID("canvas-zoom-in")
	h.clickID("canvas-zoom-fit")
	if got := len(ws.Session().History()); got != lines || ws.Session().Active() != "LINE" {
		t.Fatalf("a click on the pill answered the LINE prompt: %d -> %d history lines, active %q", lines, got, ws.Session().Active())
	}
}

func TestStatusChipsToggleSessionState(t *testing.T) {
	ws := NewWorkspace(doc.New(), nil)
	h := chromeRig(t, ws)
	s := ws.Session()
	for _, c := range []struct {
		id  string
		get func() bool
	}{
		{"status-grid", s.Grid},
		{"status-snapgrid", s.SnapGrid},
		{"status-polar", s.Polar},
		{"status-snap", func() bool { return s.SnapOn }},
		{"status-lwt", s.LineweightDisplay},
	} {
		before := c.get()
		h.clickID(c.id)
		if c.get() == before {
			t.Errorf("%s did not toggle", c.id)
		}
		h.clickID(c.id)
		if c.get() != before {
			t.Errorf("%s did not toggle back", c.id)
		}
	}
	h.clickID("status-polar")
	h.clickID("status-ortho")
	if !s.Ortho || s.Polar() {
		t.Fatalf("ortho on must turn polar off: ortho %v polar %v", s.Ortho, s.Polar())
	}
	h.clickID("status-polar")
	if s.Ortho || !s.Polar() {
		t.Fatalf("polar on must turn ortho off: ortho %v polar %v", s.Ortho, s.Polar())
	}
	h.key("f10")
	if s.Polar() {
		t.Fatal("F10 must toggle polar tracking")
	}
	h.key("f10")
	if !s.Polar() {
		t.Fatal("F10 must toggle polar tracking back on")
	}
}

func TestLineweightChipDrawsLayerLineweights(t *testing.T) {
	ws := NewWorkspace(doc.New(), nil)
	h := chromeRig(t, ws)
	ws.Session().Script("LAYER thick=1 LINE 0,0 10,0  ")
	d := ws.Session().Drawing()
	d.Layer("thick").Lineweight = 100
	h.frame()
	weight := func() float32 {
		for _, hd := range d.Handles() {
			if it := ws.builder.Items(hd, *ws.view); len(it) > 0 {
				return it[0].Weight
			}
		}
		return 0
	}
	if w := weight(); w != 1 {
		t.Fatalf("weight %v with lineweights off, want 1", w)
	}
	h.clickID("status-lwt")
	h.frame()
	if w := weight(); w < 3 {
		t.Fatalf("weight %v with lineweights on, want about 3.8", w)
	}
}

func TestPolarTrackingLocksTheRubberBandThroughTheWorkspace(t *testing.T) {
	ws := NewWorkspace(doc.New(), nil)
	h := chromeRig(t, ws)
	s := ws.Session()
	s.SnapOn = false
	s.Script("LINE 0,0")
	h.clickID("status-polar")
	a := 47 * math.Pi / 180
	h.hw.Dispatch(gpui.MouseMoveEvent{Position: ws.ScreenPoint(geom.V(100*math.Cos(a), 100*math.Sin(a)))})
	if c := s.Cursor(); math.Abs(c.X-c.Y) > 1e-6 {
		t.Fatalf("cursor %v is not on the 45 degree polar ray", c)
	}
}

func TestLayerPlusStartsTheLayerCommand(t *testing.T) {
	ws := NewWorkspace(doc.New(), nil)
	h := chromeRig(t, ws)
	h.clickID("layer-add")
	if ws.Session().Active() != "LAYER" {
		t.Fatalf("active command %q, want LAYER", ws.Session().Active())
	}
}

func TestCommandHistoryHandleResizesTheHistory(t *testing.T) {
	ws := NewWorkspace(doc.New(), nil)
	h := chromeRig(t, ws)
	h0 := ws.historyHeight()
	p := h.find("cmd-handle")
	h.hw.Dispatch(gpui.MouseMoveEvent{Position: p})
	h.hw.Dispatch(gpui.MouseDownEvent{Position: p, Button: gpui.MouseLeft, ClickCount: 1})
	h.hw.Dispatch(gpui.MouseMoveEvent{Position: gpui.Point{X: p.X, Y: p.Y - 60}})
	if got := ws.historyHeight(); math.Abs(float64(got-(h0+60))) > 0.5 {
		t.Fatalf("dragging the handle up 60 px: history %v -> %v, want %v", h0, got, h0+60)
	}
	h.hw.Dispatch(gpui.MouseUpEvent{Position: gpui.Point{X: p.X, Y: p.Y - 60}, Button: gpui.MouseLeft, ClickCount: 1})
	h.hw.Dispatch(gpui.MouseMoveEvent{Position: gpui.Point{X: p.X, Y: p.Y + 200}})
	if got := ws.historyHeight(); math.Abs(float64(got-(h0+60))) > 0.5 {
		t.Fatalf("history kept following the pointer after the release: %v", got)
	}
	// A drag far below stops at the minimum.
	h.frame()
	p = h.find("cmd-handle")
	h.hw.Dispatch(gpui.MouseMoveEvent{Position: p})
	h.hw.Dispatch(gpui.MouseDownEvent{Position: p, Button: gpui.MouseLeft, ClickCount: 1})
	h.hw.Dispatch(gpui.MouseMoveEvent{Position: gpui.Point{X: p.X, Y: p.Y + 200}})
	if got := ws.historyHeight(); got != minHistoryHeight {
		t.Fatalf("history %v, want the minimum %v", got, float32(minHistoryHeight))
	}
}

func TestRecentCommandsMenuRunsACommandAgain(t *testing.T) {
	ws := NewWorkspace(doc.New(), nil)
	h := chromeRig(t, ws)
	h.clickID("tool-circle")
	ws.Session().Escape()
	h.clickID("tool-line")
	ws.Session().Escape()
	h.clickID("cmd-recent")
	if !h.has("cmd-recent-LINE") || !h.has("cmd-recent-CIRCLE") {
		t.Fatalf("recent menu lacks the commands just run: %v", ws.cmd.recent)
	}
	h.clickID("cmd-recent-CIRCLE")
	if ws.Session().Active() != "CIRCLE" || ws.cmd.open {
		t.Fatalf("active %q, menu open %v: want CIRCLE running and the menu closed", ws.Session().Active(), ws.cmd.open)
	}
	if ws.cmd.recent[0] != "CIRCLE" || len(ws.cmd.recent) != 2 {
		t.Fatalf("recent %v, want CIRCLE first and no duplicates", ws.cmd.recent)
	}
}

func TestAgentPanelCollapseAndNoCLICard(t *testing.T) {
	ws := NewWorkspace(doc.New(), nil)
	p := ws.AgentPanel()
	p.probed, p.choices = true, nil // no agent CLI installed, whatever this machine has
	p.hist.loaded, p.hist.off = true, true
	p.Show()
	h := chromeRig(t, ws)
	if !h.has("agent-no-cli") {
		t.Fatal("with no agent CLI the panel must show the warning card")
	}
	h.clickID("agent-start")
	if len(p.Transcript().Blocks()) == 0 {
		t.Fatal("Start without an agent CLI must say why nothing starts")
	}
	if p.Running() {
		t.Fatal("no session can start without a CLI")
	}
	h.clickID("agent-collapse")
	if p.Visible() {
		t.Fatal("the chevron must collapse the panel")
	}
	if h.has("agent-panel") {
		t.Fatal("a collapsed panel leaves the layout")
	}
}
