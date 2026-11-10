package commands_test

import (
	"strings"
	"testing"

	"cad-lite/cmdsys"
	"cad-lite/cmdsys/commands"
	"cad-lite/doc"
	"cad-lite/geom"
	"cad-lite/render"
)

// plotHost records what PLOT shows; picking is what it answers PlotPicking with.
type plotHost struct {
	picking bool
	shown   []*geom.Box2
}

func (h *plotHost) PlotPicking() bool { return h.picking }
func (h *plotHost) ShowPlot(w *geom.Box2) {
	h.picking = false
	h.shown = append(h.shown, w)
}

func TestPlotOpensTheHostDialog(t *testing.T) {
	v := render.DefaultView(800, 600)
	s := cmdsys.NewSession(doc.New(), &v, commands.NewRegistry())
	s.Submit("PLOT")
	if got := strings.Join(s.History(), "\n"); !strings.Contains(got, "Plotting is not available.") || s.Active() != "" {
		t.Fatalf("without a host: active %q log %q", s.Active(), got)
	}
	h := &plotHost{}
	s.SetPlotHost(h)
	s.Submit("PRINT")
	if len(h.shown) != 1 || h.shown[0] != nil || s.Active() != "" {
		t.Fatalf("PRINT should open the dialog and end: %v, active %q", h.shown, s.Active())
	}
}

func TestPlotPicksAWindowForTheDialog(t *testing.T) {
	v := render.DefaultView(800, 600)
	s := cmdsys.NewSession(doc.New(), &v, commands.NewRegistry())
	h := &plotHost{picking: true}
	s.SetPlotHost(h)
	s.Script("PLOT 10,20 -5,40")
	if len(h.shown) != 1 || h.shown[0] == nil || *h.shown[0] != geom.BoxOf(geom.V(-5, 20), geom.V(10, 40)) {
		t.Fatalf("picked %v", h.shown)
	}
	h.picking = true
	s.Submit("PLOT")
	s.Submit("1,1")
	s.Escape()
	if len(h.shown) != 2 || h.shown[1] != nil {
		t.Fatalf("a cancelled pick should bring the dialog back with no window: %v", h.shown)
	}
}

func TestPlotIsRefusedToAgents(t *testing.T) {
	v := render.DefaultView(800, 600)
	s := cmdsys.NewSession(doc.New(), &v, commands.NewRegistry())
	h := &plotHost{}
	s.SetPlotHost(h)
	s.SetAgentCaller(true)
	s.Submit("PLOT")
	if len(h.shown) != 0 {
		t.Fatal("an agent must not open the plot dialog")
	}
}
