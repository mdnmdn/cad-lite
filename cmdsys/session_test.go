package cmdsys_test

import (
	"fmt"
	"testing"

	"go-draw/cmdsys"
	"go-draw/cmdsys/commands"
	"go-draw/doc"
	"go-draw/render"
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
