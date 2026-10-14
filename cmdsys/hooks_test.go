package cmdsys_test

import (
	"math"
	"strings"
	"testing"

	"go-draw/cmdsys"
	"go-draw/doc"
	"go-draw/geom"
	"go-draw/render"
)

// probe is a one-shot command that runs f, so a test can reach a Context.
type probe struct {
	f           func(ctx *cmdsys.Context)
	transparent bool
}

func (*probe) Name() string { return "PROBE" }
func (p *probe) Start(ctx *cmdsys.Context) *cmdsys.Prompt {
	p.f(ctx)
	return nil
}
func (*probe) Input(*cmdsys.Context, cmdsys.Input) *cmdsys.Prompt      { return nil }
func (*probe) Preview(*cmdsys.Context, geom.Vec2, *render.DisplayList) {}
func (*probe) Cancel(*cmdsys.Context)                                  {}
func (p *probe) Transparent() bool                                     { return p.transparent }

func runProbe(s *cmdsys.Session, f func(ctx *cmdsys.Context)) {
	s.Registry().Register("PROBE", func() cmdsys.Command { return &probe{f: f} })
	s.Run("PROBE")
}

func TestContextFmt(t *testing.T) {
	s := newSession()
	runProbe(s, func(ctx *cmdsys.Context) {
		if got := ctx.Fmt(3.14159265); got != "3.1416" {
			t.Errorf("Fmt default = %q, want the FormatNumber form", got)
		}
		if got := ctx.FmtAngle(math.Pi / 2); got != "90" {
			t.Errorf("FmtAngle(pi/2) = %q", got)
		}
		_ = ctx.Vars().Set("LUPREC", 2)
		_ = ctx.Vars().Set("AUPREC", 1)
		if got := ctx.Fmt(3.14159265); got != "3.14" {
			t.Errorf("Fmt with LUPREC 2 = %q", got)
		}
		if got := ctx.FmtAngle(1); got != "57.3" {
			t.Errorf("FmtAngle with AUPREC 1 = %q", got)
		}
		_ = ctx.Vars().Set("LUPREC", 0)
		if got := ctx.Fmt(100); got != "100" {
			t.Errorf("Fmt(100) with LUPREC 0 = %q", got)
		}
	})
}

func TestLastEntityAndPrevious(t *testing.T) {
	s := newSession()
	s.Script("LINE 0,0 10,0 10,10 ")
	var last doc.Handle
	var prevLen int
	runProbe(s, func(ctx *cmdsys.Context) {
		h, ok := ctx.LastEntity()
		last = h
		if !ok {
			t.Error("LastEntity must be set after a command added entities")
		}
		prevLen = len(ctx.PreviousSelection())
		if p := ctx.LastPoint(); p != geom.V(10, 10) {
			t.Errorf("LastPoint = %v", p)
		}
		if _, _, ok := ctx.LastEnd(); ok {
			t.Error("LastEnd is unset until a draw command records it")
		}
	})
	hs := s.Drawing().Handles()
	if last != hs[len(hs)-1] || prevLen != 0 {
		t.Fatalf("last entity %v want %v; previous %d", last, hs[len(hs)-1], prevLen)
	}
}

func TestPreviousSelection(t *testing.T) {
	s := newSession()
	s.Script("LINE 0,0 10,0 ")
	s.Script("LINE 0,5 10,5 ")
	s.Run("MOVE")
	s.Click(geom.V(5, 0), false)
	s.Submit("")
	var prev []doc.Handle
	runProbe(s, func(ctx *cmdsys.Context) { prev = ctx.PreviousSelection() })
	if len(prev) != 1 {
		t.Fatalf("Previous after MOVE of one line = %v", prev)
	}
	s.Run("ERASE")
	s.Click(geom.V(5, 0), false)
	s.Submit("")
	runProbe(s, func(ctx *cmdsys.Context) { prev = ctx.PreviousSelection() })
	if len(prev) != 0 {
		t.Fatalf("Previous must drop erased handles, got %v", prev)
	}
}

func TestTransparentMarker(t *testing.T) {
	if cmdsys.IsTransparent(&probe{}) || !cmdsys.IsTransparent(&probe{transparent: true}) {
		t.Fatal("IsTransparent must follow the marker")
	}
	r := newSession().Registry()
	for _, n := range []string{"ZOOM", "LAYER", "DIST", "ID"} {
		c, _ := r.Lookup(n)
		if !cmdsys.IsTransparent(c) {
			t.Errorf("%s must be transparent", n)
		}
	}
	for _, n := range []string{"LINE", "ERASE", "UNDO", "SAVE"} {
		c, _ := r.Lookup(n)
		if cmdsys.IsTransparent(c) {
			t.Errorf("%s must not be transparent", n)
		}
	}
}

func TestTextStagesRunInOrder(t *testing.T) {
	s := newSession()
	var seen []string
	s.AddTextHandler(cmdsys.TextOrderVars, func(_ *cmdsys.Session, text string) bool {
		seen = append(seen, "90:"+text)
		return text == "EAT"
	})
	s.AddTextFilter(cmdsys.TextOrderSelection, func(_ *cmdsys.Session, text string) string {
		seen = append(seen, "20:"+text)
		return strings.ToUpper(text)
	})
	s.Submit("_.eat")
	if got := strings.Join(seen, ","); got != "20:eat,90:EAT" {
		t.Fatalf("stages ran %q; the name prefixes (order 10) must run first and a handler consumes the line", got)
	}
	if strings.Contains(strings.Join(s.History(), "\n"), "EAT") {
		t.Fatal("a handled line must not reach the command line")
	}
}

func TestCommandPrefixes(t *testing.T) {
	for _, in := range []string{"_LINE", ".line", "_.LINE", "._line", "'_LINE"} {
		s := newSession()
		s.Submit(in)
		if s.Active() != "LINE" {
			t.Errorf("%q started %q, want LINE", in, s.Active())
		}
	}
	s := newSession()
	s.Submit("_")
	if s.Active() != "" || !strings.Contains(strings.Join(s.History(), "\n"), "Unknown") {
		t.Fatalf("a bare prefix is an unknown command, got active %q history %v", s.Active(), s.History())
	}
	// At a prompt the text is a response: nothing is stripped.
	s = newSession()
	s.Submit("LINE")
	s.Submit(".5,.5")
	if s.Active() != "LINE" {
		t.Fatalf("a response starting with . must stay a response, active %q", s.Active())
	}
	if got := cmdsys.StripCommandPrefixes("_-LAYER"); got != "-LAYER" {
		t.Fatalf("the - belongs to the name: %q", got)
	}
}

func TestScriptComments(t *testing.T) {
	s := newSession()
	s.Script("; header comment\nLINE 0,0 ; start\n10,0 ;end\n\n")
	ls := s.Drawing().Handles()
	if len(ls) != 1 {
		t.Fatalf("entities = %d, history %v", len(ls), s.History())
	}
	if l := s.Drawing().Entity(ls[0]).(*doc.Line); l.End != geom.V(10, 0) {
		t.Fatalf("line = %v..%v", l.Start, l.End)
	}
	if got := cmdsys.StripScriptComments("a ; b\n;only\n\nc"); got != "a\n\nc" {
		t.Fatalf("StripScriptComments = %q", got)
	}
	if got := cmdsys.StripScriptComments("no comment\n"); got != "no comment\n" {
		t.Fatalf("text without ; must be untouched: %q", got)
	}
}
