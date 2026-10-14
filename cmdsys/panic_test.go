package cmdsys_test

import (
	"strings"
	"testing"

	"cad-lite/cmdsys"
	"cad-lite/doc"
	"cad-lite/geom"
	"cad-lite/render"
)

// boom adds a line in its transaction, then panics at the stage named by at.
type boom struct{ at string }

func (b *boom) Name() string { return "BOOM" }
func (b *boom) Start(ctx *cmdsys.Context) *cmdsys.Prompt {
	ctx.Tx().Add(&doc.Line{End: geom.V(1, 1)})
	if b.at == "start" {
		panic("start boom")
	}
	return &cmdsys.Prompt{Message: "next", Accepts: cmdsys.InputText}
}
func (b *boom) Input(ctx *cmdsys.Context, in cmdsys.Input) *cmdsys.Prompt {
	if b.at == "input" {
		panic("input boom")
	}
	return &cmdsys.Prompt{Message: "more", Accepts: cmdsys.InputText}
}
func (b *boom) Preview(ctx *cmdsys.Context, c geom.Vec2, dl *render.DisplayList) {
	if b.at == "preview" {
		panic("preview boom")
	}
}
func (b *boom) Cancel(ctx *cmdsys.Context) {
	if b.at == "cancel" {
		panic("cancel boom")
	}
}

func TestPanicInCommandLeavesSessionUsable(t *testing.T) {
	for _, at := range []string{"start", "input", "preview", "cancel"} {
		t.Run(at, func(t *testing.T) {
			reg := cmdsys.NewRegistry()
			reg.Register("BOOM", func() cmdsys.Command { return &boom{at: at} })
			v := render.DefaultView(800, 600)
			d := doc.New()
			s := cmdsys.NewSession(d, &v, reg)
			s.Run("BOOM")
			switch at {
			case "input":
				s.Submit("x")
			case "preview":
				s.Overlay(&render.DisplayList{})
			case "cancel":
				s.Escape()
			}
			if s.Active() != "" {
				t.Fatalf("command still active: %q", s.Active())
			}
			if !strings.Contains(strings.Join(s.History(), "\n"), "Error: BOOM failed") && at != "cancel" {
				t.Errorf("no error line: %v", s.History())
			}
			if _, err := d.Begin(); err != nil {
				t.Fatalf("transaction left open: %v", err)
			}
			d.AbortOpen()
			if d.Len() != 0 {
				t.Errorf("aborted txn left %d entities", d.Len())
			}
			s.Run("BOOM") // session accepts a new command
			if s.Active() == "" && at == "preview" {
				t.Error("could not run a command after the panic")
			}
		})
	}
}
