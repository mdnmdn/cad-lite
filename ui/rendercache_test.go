package ui

import "testing"

func TestSubmitKeepsRenderCacheUnlessLayersChange(t *testing.T) {
	h := dialogRig(t, nil)
	s := h.ws.Session()
	s.Script("LINE 0,0 40,0 ")
	s.Escape()
	hd := s.Drawing().Handles()[0]
	before := h.ws.RenderItems(hd)
	if len(before) == 0 {
		t.Fatal("no cached items for the line")
	}
	h.ws.submit("CIRCLE", h.hw.Window())
	h.ws.submit("0,20", h.hw.Window())
	h.ws.submit("5", h.hw.Window())
	if after := h.ws.RenderItems(hd); &after[0] != &before[0] {
		t.Fatal("a command-line CIRCLE dropped the cached items of an untouched line")
	}
	h.ws.submit("LAYER", h.hw.Window())
	h.ws.submit("walls=4", h.hw.Window())
	h.ws.submit("", h.hw.Window())
	if s.Drawing().Layer("walls") == nil {
		t.Fatal("the LAYER answer did not add layer walls")
	}
	if after := h.ws.RenderItems(hd); &after[0] == &before[0] {
		t.Fatal("a layer colour edit on the command line kept stale cached items")
	}
}

// A TRIM pick redraws the trimmed line at once: the canvas drops the cached items of what the
// running command changed, not only what it committed.
func TestRenderCacheFollowsEditsOfTheRunningCommand(t *testing.T) {
	h := dialogRig(t, nil)
	s := h.ws.Session()
	s.Script("LINE 0,0 100,0 ")
	s.Script("LINE 50,-10 50,10 ")
	hd := s.Drawing().Handles()[0]
	h.hw.Pixels()
	if items := h.ws.RenderItems(hd); len(items) == 0 {
		t.Fatal("no cached items for the line")
	}
	h.ws.submit("TRIM", h.hw.Window())
	h.ws.submit("80,0", h.hw.Window())
	if s.Active() != "TRIM" {
		t.Fatalf("TRIM ended: %q", s.Active())
	}
	h.hw.Pixels()
	for _, it := range h.ws.RenderItems(hd) {
		for _, p := range it.Points {
			if p.X > 50+1e-9 {
				t.Fatalf("the trimmed line still draws to x=%v", p.X)
			}
		}
	}
}
