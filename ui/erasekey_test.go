package ui

import (
	"slices"
	"testing"

	"cad-lite/doc"
)

// eraseRig is a window over two lines on layer 0 and one on the locked layer Fixed.
func eraseRig(t *testing.T) *sHarness {
	t.Helper()
	d := doc.New()
	d.AddLayer("Fixed", 3)
	h := openSettingsWS(t, NewWorkspace(d, nil), 1280, 800, 1)
	h.ws.Session().Script("LINE 0,0 100,0  LINE 0,50 100,50  -LAYER S Fixed  LINE 0,80 100,80  -LAYER S 0  ZOOM E")
	d.Layer("Fixed").Locked = true
	h.ws.session.Selection().Clear()
	h.frame()
	return h
}

func TestDeleteErasesTheSelection(t *testing.T) {
	for _, key := range []string{"delete", "backspace"} {
		t.Run(key, func(t *testing.T) {
			h := eraseRig(t)
			s := h.ws.Session()
			h.selectAll()
			if s.Selection().Len() != 2 {
				t.Fatalf("selection %d, want the two unlocked lines", s.Selection().Len())
			}
			h.key(key)
			if n := s.Drawing().Len(); n != 1 {
				t.Fatalf("%s left %d entities, want the locked line only", key, n)
			}
			if s.Selection().Len() != 0 || s.Active() != "" {
				t.Fatalf("selection %d, active %q", s.Selection().Len(), s.Active())
			}
			if !slices.ContainsFunc(s.History(), func(l string) bool { return l == "Command: ERASE" }) {
				t.Fatalf("ERASE not echoed: %q", s.History())
			}
			h.ws.submit("U", h.hw.Window())
			if n := s.Drawing().Len(); n != 3 {
				t.Fatalf("one undo should bring both lines back, have %d", n)
			}
		})
	}
}

func TestDeleteKeepsEditingTypedText(t *testing.T) {
	h := eraseRig(t)
	h.selectAll()
	h.typeCmd("LI")
	h.key("backspace")
	if h.ws.input.Text() != "L" || h.ws.Session().Drawing().Len() != 3 {
		t.Fatalf("Backspace with text typed should edit it: %q, %d entities", h.ws.input.Text(), h.ws.Session().Drawing().Len())
	}
	h.key("left")
	h.key("delete")
	if h.ws.input.Text() != "" || h.ws.Session().Drawing().Len() != 3 {
		t.Fatalf("Delete with text typed should edit it: %q", h.ws.input.Text())
	}
}

func TestDeleteDoesNothingMidCommandOrWithoutSelection(t *testing.T) {
	h := eraseRig(t)
	h.key("delete")
	if h.ws.Session().Drawing().Len() != 3 || h.ws.Session().Active() != "" {
		t.Fatal("Delete without a selection should do nothing")
	}
	h.selectAll()
	h.ws.run("MOVE", h.hw.Window())
	h.ws.Session().Escape()
	h.selectAll()
	h.ws.run("LINE", h.hw.Window())
	h.key("delete")
	h.key("backspace")
	if h.ws.Session().Drawing().Len() != 3 || h.ws.Session().Active() != "LINE" {
		t.Fatalf("Delete mid-command should not erase: %d entities, active %q", h.ws.Session().Drawing().Len(), h.ws.Session().Active())
	}
}
