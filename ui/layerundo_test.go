package ui

import "testing"

// undo answers U on the command line.
func (h *sHarness) undo() {
	h.ws.submit("U", h.hw.Window())
	h.frame()
}

// The Layers section's switches and its row click each make one undo step named LAYER.
func TestLayerPanelEditsAreUndoable(t *testing.T) {
	h := layerRig(t)
	d := h.ws.session.Drawing()
	steps := []struct {
		id    string
		check func() bool
	}{
		{"layer-vis-Walls", func() bool { return d.Layer("Walls").Off }},
		{"layer-lock-Walls", func() bool { return d.Layer("Walls").Locked }},
		{"layer-Doors", func() bool { return d.CurrentLayer == "Doors" }},
	}
	for _, st := range steps {
		rev := h.ws.pp.rev
		h.clickID(st.id)
		h.frame()
		if !st.check() {
			t.Fatalf("%s: no change", st.id)
		}
		if got := d.UndoName(); got != "LAYER" {
			t.Fatalf("%s: undo step %q, want LAYER", st.id, got)
		}
		if h.ws.pp.rev == rev {
			t.Fatalf("%s: the Properties sheet was not told", st.id)
		}
		h.undo()
		if st.check() {
			t.Fatalf("%s: U did not undo it", st.id)
		}
		h.ws.submit("REDO", h.hw.Window())
		h.frame()
		if !st.check() {
			t.Fatalf("%s: REDO did not redo it", st.id)
		}
		h.undo()
	}
}

// The popover's switches and its pick are undoable too.
func TestLayerPopoverEditsAreUndoable(t *testing.T) {
	h := layerRig(t)
	d := h.ws.session.Drawing()
	h.openLayerPop()
	if h.ws.session.Dirty() {
		t.Fatal("the rig starts dirty")
	}
	h.clickID("layer-pop-vis-Doors")
	h.frame()
	if !h.ws.session.Dirty() {
		t.Fatal("a layer switch left the drawing clean")
	}
	h.clickID("layer-pop-lock-Doors")
	h.frame()
	if !d.Layer("Doors").Off || !d.Layer("Doors").Locked {
		t.Fatal("the popover switches did nothing")
	}
	h.clickID("layer-pop-Windows")
	h.frame()
	if d.CurrentLayer != "Windows" {
		t.Fatal("the pick did nothing")
	}
	h.undo()
	if d.CurrentLayer != "0" {
		t.Fatalf("U: current %q, want 0", d.CurrentLayer)
	}
	h.undo()
	if !d.Layer("Doors").Off || d.Layer("Doors").Locked {
		t.Fatal("U did not undo the lock alone")
	}
	h.undo()
	if d.Layer("Doors").Off {
		t.Fatal("U did not undo the visibility switch")
	}
}

// While a command runs the switches and the current-layer click are refused, as the Properties
// panel's edits are.
func TestLayerPanelEditsRefusedWhileCommandRuns(t *testing.T) {
	h := layerRig(t)
	d := h.ws.session.Drawing()
	h.ws.run("LINE", h.hw.Window())
	h.frame()
	h.clickID("layer-vis-Walls")
	h.clickID("layer-lock-Walls")
	h.clickID("layer-Doors")
	h.frame()
	if l := d.Layer("Walls"); l.Off || l.Locked || d.CurrentLayer != "0" {
		t.Fatalf("edited while LINE runs: off %v locked %v current %q", l.Off, l.Locked, d.CurrentLayer)
	}
	if h.ws.session.Active() != "LINE" {
		t.Fatal("the click ended LINE")
	}
}
