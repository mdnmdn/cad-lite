package commands_test

import (
	"strings"
	"testing"

	"go-draw/cmdsys"
	"go-draw/cmdsys/commands"
	"go-draw/doc"
	"go-draw/render"
)

func TestOptionsCommandOpensSettings(t *testing.T) {
	v := render.DefaultView(800, 600)
	s := cmdsys.NewSession(doc.New(), &v, commands.NewRegistry())
	s.Submit("options")
	if s.Active() != "" {
		t.Fatal("OPTIONS without a handler should end at once")
	}
	if got := strings.Join(s.History(), "\n"); !strings.Contains(got, "Settings are not available.") {
		t.Fatalf("log %q", got)
	}
	n := 0
	s.SetSettingsHandler(func() { n++ })
	s.Submit("OP")
	s.Submit("OPTIONS")
	if n != 2 || s.Active() != "" {
		t.Fatalf("handler calls %d, active %q", n, s.Active())
	}
}
