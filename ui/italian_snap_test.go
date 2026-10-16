package ui

import (
	"strings"
	"testing"

	"cad-lite/cmdsys"
	"cad-lite/doc"
	"cad-lite/geom"
	"cad-lite/render"
)

func TestTypedInmezzoSpaceSnapsMidpoint(t *testing.T) {
	for _, alias := range []string{"INMEZZO", "inmezzo", "MID"} {
		t.Run(alias, func(t *testing.T) {
			h := completeRig(t)
			s := h.ws.Session()
			s.Script("LINE\n0,0\n100,0\n\n")
			s.SnapOn = false
			h.ws.run("LINE", h.hw.Window())
			h.typeCmd(alias + " ")
			if got := h.ws.input.Text(); got != "" || !strings.Contains(s.Prompt(), "of:") {
				t.Fatalf("Space did not arm snap %q: text %q prompt %q", alias, got, s.Prompt())
			}
			h.typeCmd("70,0")
			h.key("enter")
			h.typeCmd("0,50")
			h.key("enter")
			h.key("enter")
			want := geom.V(50, 0)
			hd := s.Drawing().Handles()
			l, ok := s.Drawing().Entity(hd[len(hd)-1]).(*doc.Line)
			if !ok || l.Start != want || l.End != geom.V(0, 50) {
				t.Fatalf("snap outcome: %+v, want %v to 0,50", l, want)
			}
		})
	}
}

func TestSnapSpaceSubmissionCompatibility(t *testing.T) {
	for _, tc := range []struct {
		command, text string
		retained      bool
	}{
		{"LINE", "IN ", false}, {"LINE", "PUNTO ", false},
		{"LINE", "END ", false}, {"LINE", "INMEZZO ", false},
		{"LINE", "_IN ", false}, {"LINE", "_INMEZZO ", false}, {"LINE", "I ", false},
		{"BREAK", "IN ", false},
	} {
		t.Run(tc.command+tc.text, func(t *testing.T) {
			h := completeRig(t)
			h.ws.run(tc.command, h.hw.Window())
			h.typeCmd(tc.text)
			if got := h.ws.input.Text() != ""; got != tc.retained {
				t.Fatalf("retained %q = %v, want %v", h.ws.input.Text(), got, tc.retained)
			}
			if tc.text == "END " && !strings.Contains(h.ws.Session().Prompt(), "of:") {
				t.Fatalf("English END Space did not submit: %s", h.ws.Session().Prompt())
			}
		})
	}
}

type snapKeywordCommand struct{}

func (*snapKeywordCommand) Name() string { return "SNAPKEYWORDTEST" }
func (*snapKeywordCommand) Start(*cmdsys.Context) *cmdsys.Prompt {
	return &cmdsys.Prompt{Message: "Test point", Accepts: cmdsys.InputPoint | cmdsys.InputKeyword, Keywords: []string{"IN"}}
}
func (*snapKeywordCommand) Input(*cmdsys.Context, cmdsys.Input) *cmdsys.Prompt      { return nil }
func (*snapKeywordCommand) Preview(*cmdsys.Context, geom.Vec2, *render.DisplayList) {}
func (*snapKeywordCommand) Cancel(*cmdsys.Context)                                  {}

func TestSnapSpacePreservesPromptKeyword(t *testing.T) {
	h := completeRig(t)
	h.ws.session.Registry().Register("SNAPKEYWORDTEST", func() cmdsys.Command { return &snapKeywordCommand{} })
	h.ws.run("SNAPKEYWORDTEST", h.hw.Window())
	h.typeCmd("IN ")
	if h.ws.session.Active() != "" || h.ws.input.Text() != "" {
		t.Fatalf("IN keyword not submitted: active %s text %q", h.ws.session.Active(), h.ws.input.Text())
	}
}

func TestSnapSpaceKeepsFreeTextEntry(t *testing.T) {
	h := completeRig(t)
	h.ws.session.Script("TEXT\n0,0\n2\n0\n")
	h.frame()
	h.typeCmd("A B")
	if h.ws.input.Text() != "A B" {
		t.Fatalf("free text lost its space: %q", h.ws.input.Text())
	}
}
