package commands_test

import (
	"strings"
	"testing"

	"cad-lite/doc"
	"cad-lite/geom"
)

func TestTextEditEntryAndAgentGuard(t *testing.T) {
	for _, name := range []string{"TEXTEDIT", "DDEDIT", "ED", "MODTESTO"} {
		t.Run(name, func(t *testing.T) {
			s := newSession(t)
			d := s.Drawing()
			tx, _ := d.Begin()
			h := tx.Add(doc.NewText(geom.V(0, 0), 10, 0, "Original"))
			tx.Commit()
			s.MarkSaved()
			opened := doc.Handle(0)
			s.SetTextEditHandler(func(got doc.Handle) bool { opened = got; return true })
			s.Run(name)
			if p := s.CurrentPrompt(); p == nil || !p.Pick {
				t.Fatal("TEXTEDIT must ask for an unsnapped object pick")
			}
			s.Click(geom.V(1, 2), false)
			if opened != h || s.Active() != "" || s.Dirty() {
				t.Fatalf("open %v active %q dirty %v", opened, s.Active(), s.Dirty())
			}
			opened = 0
			s.Selection().Add(h)
			s.Run(name)
			if opened != h || s.Active() != "" {
				t.Fatal("single preselection did not open the editor")
			}
			opened = 0
			s.SetAgentCaller(true)
			s.Run(name)
			if opened != 0 || !strings.Contains(strings.Join(s.History(), "\n"), "not available to agents") {
				t.Fatal("an agent reached the human editor")
			}
		})
	}
}

func TestTextEditRefusesIneligibleTargetsAndAmbiguousSelection(t *testing.T) {
	s := newSession(t)
	d := s.Drawing()
	tx, _ := d.Begin()
	h := tx.Add(doc.NewText(geom.V(0, 0), 10, 0, "Original"))
	m := tx.Add(doc.NewMText(geom.V(100, 0), 10, 0, "Multi"))
	tx.Commit()
	opened := false
	s.SetTextEditHandler(func(doc.Handle) bool { opened = true; return true })
	s.Selection().Add(h, m)
	s.Run("TEXTEDIT")
	if opened || s.Active() != "TEXTEDIT" {
		t.Fatal("multiple selected objects must not choose an arbitrary text")
	}
	s.Escape()
	for _, flag := range []string{"locked", "off", "frozen"} {
		l := d.Layer("0")
		l.Locked, l.Off, l.Frozen = flag == "locked", flag == "off", flag == "frozen"
		s.Selection().Clear()
		s.Selection().Add(h)
		s.Run("TEXTEDIT")
		s.Click(geom.V(1, 2), false)
		if opened {
			t.Fatalf("opened on %s layer", flag)
		}
		s.Escape()
	}
}
