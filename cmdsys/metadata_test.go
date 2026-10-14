package cmdsys_test

import (
	"reflect"
	"testing"

	"cad-lite/cmdsys"
	"cad-lite/cmdsys/commands"
	"cad-lite/doc"
	"cad-lite/i18n"
	"cad-lite/render"
)

func TestHostMetadataMarksSessionDirtyWithoutUndo(t *testing.T) {
	v := render.DefaultView(800, 600)
	s := cmdsys.NewSession(doc.New(), &v, commands.NewRegistry())
	other := cmdsys.NewSession(doc.New(), &v, commands.NewRegistry())
	s.Run("LINE")
	s.Submit("0,0")
	s.Submit("10,0")
	s.Submit("")
	s.Run("U")
	s.MarkSaved()
	d := s.Drawing()
	undo, redo := d.UndoDepth(), d.RedoDepth()
	d.Layer("0").Color = 3
	s.MarkModified()
	if !s.Dirty() || s.Modified() != 1 || s.Vars().Int("DBMOD") != 1 {
		t.Fatalf("metadata not unsaved: Dirty=%v Modified=%d DBMOD=%d", s.Dirty(), s.Modified(), s.Vars().Int("DBMOD"))
	}
	d.Layer("0").Locked = true
	s.MarkModified()
	if s.Modified() != 2 || d.UndoDepth() != undo || d.RedoDepth() != redo {
		t.Fatalf("metadata changed history: Modified=%d undo=%d redo=%d", s.Modified(), d.UndoDepth(), d.RedoDepth())
	}
	if other.Dirty() || other.Modified() != 0 || other.Vars().Int("DBMOD") != 0 {
		t.Fatal("metadata change leaked into another session")
	}
	s.MarkSaved()
	if s.Dirty() || s.Modified() != 0 || s.Vars().Int("DBMOD") != 0 {
		t.Fatal("save did not clear metadata changes")
	}
	s.MarkModified()
	s.SetDrawing(doc.New())
	if s.Dirty() || s.Modified() != 0 || s.Vars().Int("DBMOD") != 0 {
		t.Fatal("replacement drawing inherited metadata changes")
	}
}

func TestTextPromptsIncludedInKeywordRegistry(t *testing.T) {
	for _, tc := range []struct {
		group, command string
		answers        []string
	}{
		{"TEXT start", "TEXT", nil},
		{"TEXT justify", "TEXT", []string{"_Justify"}},
		{"TEXT/MTEXT style", "TEXT", []string{"_Style"}},
		{"MTEXT corner", "MTEXT", []string{"0,0"}},
		{"MTEXT justify", "MTEXT", []string{"0,0", "_Justify"}},
		{"TEXT/MTEXT style", "MTEXT", []string{"0,0", "_Style"}},
	} {
		t.Run(tc.command+"/"+tc.group, func(t *testing.T) {
			v := render.DefaultView(800, 600)
			s := cmdsys.NewSession(doc.New(), &v, commands.NewRegistry())
			s.Run(tc.command)
			for _, answer := range tc.answers {
				s.Submit(answer)
			}
			p := s.CurrentPrompt()
			if p == nil || len(p.Keywords) == 0 {
				t.Fatal("command did not reach its keyword prompt")
			}
			if !reflect.DeepEqual(i18n.KeywordGroups[tc.group], p.Keywords) {
				t.Fatalf("registry %q=%v; actual prompt offers %v", tc.group, i18n.KeywordGroups[tc.group], p.Keywords)
			}
		})
	}
}
