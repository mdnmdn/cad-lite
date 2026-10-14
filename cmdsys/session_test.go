package cmdsys_test

import (
	"fmt"
	"testing"

	"cad-lite/cmdsys"
	"cad-lite/cmdsys/commands"
	"cad-lite/doc"
	"cad-lite/render"
)

func TestHistoryTotalSurvivesTrimming(t *testing.T) {
	v := render.DefaultView(800, 600)
	s := cmdsys.NewSession(doc.New(), &v, commands.NewRegistry())
	if s.HistoryTotal() != 0 {
		t.Fatalf("fresh session total = %d", s.HistoryTotal())
	}
	s.Run("ID")
	if s.HistoryTotal() != len(s.History()) || s.HistoryTotal() == 0 {
		t.Fatalf("total %d vs history %d", s.HistoryTotal(), len(s.History()))
	}
	s.Escape()
	start := s.HistoryTotal()
	for i := range 300 {
		s.Submit(fmt.Sprintf("NOSUCHCOMMAND%d", i))
	}
	if got := s.HistoryTotal() - start; got != 300 {
		t.Fatalf("printed %d lines, want 300", got)
	}
	if len(s.History()) != cmdsys.HistoryLimit {
		t.Fatalf("history len = %d, want %d", len(s.History()), cmdsys.HistoryLimit)
	}
}

func TestRunCountAndLastCommandFollowStartedCommands(t *testing.T) {
	v := render.DefaultView(800, 600)
	s := cmdsys.NewSession(doc.New(), &v, commands.NewRegistry())
	if s.RunCount() != 0 || s.LastCommand() != "" {
		t.Fatalf("fresh session: %d %q", s.RunCount(), s.LastCommand())
	}
	s.Submit("line")
	s.Escape()
	s.Submit("line")
	s.Escape()
	s.Submit("nosuchcommand")
	if s.RunCount() != 2 || s.LastCommand() != "LINE" {
		t.Fatalf("after two LINE and an unknown word: %d %q", s.RunCount(), s.LastCommand())
	}
}
