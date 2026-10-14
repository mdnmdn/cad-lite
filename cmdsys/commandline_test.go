package cmdsys_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"go-draw/cmdsys"
	"go-draw/doc"
	"go-draw/geom"
)

func hist(s *cmdsys.Session) string { return strings.Join(s.History(), "\n") }

func lines(s *cmdsys.Session) []*doc.Line {
	var out []*doc.Line
	for _, h := range s.Drawing().Handles() {
		if l, ok := s.Drawing().Entity(h).(*doc.Line); ok {
			out = append(out, l)
		}
	}
	return out
}

func TestStripPrefixesAfterDash(t *testing.T) {
	for in, want := range map[string]string{"-_LAYER": "-LAYER", "_-LAYER": "-LAYER", "-LAYER": "-LAYER", "'_.ZOOM": "ZOOM", "-": "-"} {
		if got := cmdsys.StripCommandPrefixes(in); got != want {
			t.Errorf("StripCommandPrefixes(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestRegisterCLIResolvesDashNames(t *testing.T) {
	s := newSession()
	s.Registry().Alias("LIVELLO", "PROBE")
	s.Registry().Register("PROBE", func() cmdsys.Command { return &probe{f: func(*cmdsys.Context) {}} })
	cmdsys.RegisterCLI(s.Registry(), "PROBE", func() cmdsys.Command { return &namedProbe{} }, "PR")
	for _, typed := range []string{"-PROBE", "-pr", "-_PROBE", "-LIVELLO"} {
		s.Submit(typed)
		if !strings.Contains(hist(s), "Command: -PROBE") {
			t.Fatalf("%s did not run -PROBE:\n%s", typed, hist(s))
		}
		s = newSession()
		s.Registry().Alias("LIVELLO", "PROBE")
		cmdsys.RegisterCLI(s.Registry(), "PROBE", func() cmdsys.Command { return &namedProbe{} }, "PR")
	}
	if !cmdsys.IsCLIName("-LAYER") || cmdsys.IsCLIName("LAYER") {
		t.Error("IsCLIName")
	}
}

type namedProbe struct{ probe }

func (*namedProbe) Name() string                           { return "-PROBE" }
func (p *namedProbe) Start(*cmdsys.Context) *cmdsys.Prompt { return nil }

func TestTransparentCommandResumesPrompt(t *testing.T) {
	s := newSession()
	s.Script("LINE\n0,0\n")
	before := s.Prompt()
	s.Submit("'ZOOM")
	if !strings.HasPrefix(s.Prompt(), ">>") || s.TransparentDepth() != 1 {
		t.Fatalf("prompt while transparent = %q, depth %d", s.Prompt(), s.TransparentDepth())
	}
	if s.Active() != "ZOOM" {
		t.Fatalf("active = %q", s.Active())
	}
	s.Script("W\n-10,-10\n110,110")
	if s.Prompt() != before || s.Active() != "LINE" || s.TransparentDepth() != 0 {
		t.Fatalf("after ZOOM: prompt %q (want %q), active %q", s.Prompt(), before, s.Active())
	}
	if c := s.View().Center; c != geom.V(50, 50) {
		t.Fatalf("view centre %v", c)
	}
	s.Script("50,0\n\n")
	if got := lines(s); len(got) != 1 || got[0].End != geom.V(50, 0) {
		t.Fatalf("lines = %v", got)
	}
	// Enter at Command: must still repeat LINE, not ZOOM.
	s.Submit("")
	if s.Active() != "LINE" {
		t.Fatalf("Enter repeated %q", s.Active())
	}
}

func TestTransparentRefusals(t *testing.T) {
	s := newSession()
	s.Script("LINE\n0,0\n")
	s.Submit("'CIRCLE")
	if !strings.Contains(hist(s), "CIRCLE command not allowed transparently.") || s.Active() != "LINE" || s.TransparentDepth() != 0 {
		t.Fatalf("history:\n%s", hist(s))
	}
	s.Submit("'NOSUCH")
	if !strings.Contains(hist(s), `Unknown command "NOSUCH".  Press F1 for more help.`) || s.Active() != "LINE" {
		t.Fatalf("history:\n%s", hist(s))
	}
}

func TestTransparentEscapeCancelsAll(t *testing.T) {
	s := newSession()
	s.Script("LINE\n0,0\n10,0\n'ZOOM\n")
	s.Escape()
	if s.Active() != "" || s.TransparentDepth() != 0 || s.Prompt() != "Command:" {
		t.Fatalf("after Escape: active %q depth %d prompt %q", s.Active(), s.TransparentDepth(), s.Prompt())
	}
	if len(lines(s)) != 1 {
		t.Fatalf("the committed segment is kept: %d lines", len(lines(s)))
	}
}

func TestTransparentSetvarOverText(t *testing.T) {
	s := newSession()
	s.Script("LINE\n0,0\n'ORTHOMODE\n1\n")
	if !s.Ortho || s.Active() != "LINE" {
		t.Fatalf("ortho %v, active %q", s.Ortho, s.Active())
	}
}

func TestSetvarPromptsAndMessages(t *testing.T) {
	s := newSession()
	s.Submit("SETVAR")
	if got := s.Prompt(); got != "Enter variable name or [?]:" {
		t.Fatalf("name prompt = %q", got)
	}
	s.Submit("NOPE")
	if !strings.Contains(hist(s), "Unknown variable name.") || s.Active() != "SETVAR" {
		t.Fatalf("history:\n%s", hist(s))
	}
	s.Submit("polysides")
	if got := s.Prompt(); got != "Enter new value for POLYSIDES <4>:" {
		t.Fatalf("value prompt = %q", got)
	}
	s.Submit("2")
	if !strings.Contains(hist(s), "Requires an integer value between 3 and 1024.") || s.Active() != "SETVAR" {
		t.Fatalf("history:\n%s", hist(s))
	}
	s.Submit("x")
	if !strings.Contains(hist(s), "Requires an integer value.\n") && !strings.HasSuffix(hist(s), "Requires an integer value.") {
		// the sentence is printed after the echo of x
		if !strings.Contains(hist(s), "Requires an integer value.") {
			t.Fatalf("history:\n%s", hist(s))
		}
	}
	s.Submit("6")
	if s.Vars().Int("POLYSIDES") != 6 || s.Active() != "" {
		t.Fatalf("POLYSIDES = %d, active %q", s.Vars().Int("POLYSIDES"), s.Active())
	}
	// SETVAR remembers the name: Enter takes it.
	s.Script("SETVAR\n\n")
	if got := s.Prompt(); got != "Enter new value for POLYSIDES <6>:" {
		t.Fatalf("remembered default: %q", got)
	}
	s.Submit("")
	if s.Active() != "" || s.Vars().Int("POLYSIDES") != 6 {
		t.Fatalf("Enter must keep the value")
	}
}

func TestVariableTypedByName(t *testing.T) {
	s := newSession()
	s.Script("FILLETRAD")
	if got := s.Prompt(); got != "Enter new value for FILLETRAD <0.0000>:" && got != "Enter new value for FILLETRAD <0>:" {
		t.Fatalf("prompt = %q", got)
	}
	s.Submit("3.5")
	if s.Vars().Real("FILLETRAD") != 3.5 || s.Active() != "" {
		t.Fatalf("FILLETRAD = %v", s.Vars().Real("FILLETRAD"))
	}
	s.Script("clayer\n0\n")
	s.Script("MIRRTEXT\n1\n")
	if s.Vars().Int("MIRRTEXT") != 1 {
		t.Fatal("a stored no-op variable reads back what was set")
	}
	s.Script("DISTANCE\n5\n")
	if !strings.Contains(hist(s), "Variable DISTANCE is read-only.") || s.Active() != "" {
		t.Fatalf("history:\n%s", hist(s))
	}
}

func TestSetvarList(t *testing.T) {
	s := newSession()
	s.Script("SETVAR\n?\nORTHO*")
	h := hist(s)
	if !strings.Contains(h, "ORTHOMODE") || strings.Contains(h, "APERTURE") || s.Active() != "" {
		t.Fatalf("history:\n%s", h)
	}
	s.Script("SETVAR\n?\nDISTANCE\n")
	if !strings.Contains(hist(s), "(read only)") {
		t.Fatalf("history:\n%s", hist(s))
	}
}

func TestPointInputForms(t *testing.T) {
	s := newSession()
	// "@" alone is the last point; "#" forces absolute; relative before any point starts at 0,0.
	s.Script("LINE\n@3,4\n#10,10\n@\n")
	if got := lines(s); len(got) != 2 || got[0].Start != geom.V(3, 4) || got[0].End != geom.V(10, 10) || got[1].End != geom.V(10, 10) {
		t.Fatalf("lines = %+v", got)
	}
	s.Submit("")
	s = newSession()
	s.Script("LINE\n1,1\n<90\n5\n")
	if got := lines(s); len(got) != 1 || got[0].End.Sub(geom.V(1, 6)).Len() > 1e-9 {
		t.Fatalf("angle override: %+v", got)
	}
	// The override lasts one answer: a typed point ends it.
	s.Script("<0\n2,2\n")
	if got := lines(s); len(got) != 2 || got[1].End != geom.V(2, 2) {
		t.Fatalf("lines = %+v", got)
	}
	if s.CurrentPrompt() == nil {
		t.Fatal("LINE should still ask for the next point")
	}
}

func TestEnterEndsRequiredPointQuietly(t *testing.T) {
	s := newSession()
	s.Script("CIRCLE\n\n")
	if s.Active() != "" || strings.Contains(hist(s), "*Cancel*") || strings.Contains(hist(s), "Invalid input") {
		t.Fatalf("active %q, history:\n%s", s.Active(), hist(s))
	}
}

func TestInputErrorWording(t *testing.T) {
	s := newSession()
	s.Script("LINE\nabc\n0,0\nfoo")
	h := hist(s)
	for _, w := range []string{"Invalid point.", "Point or option keyword required."} {
		if !strings.Contains(h, w) {
			t.Errorf("missing %q in\n%s", w, h)
		}
	}
	s.Escape()
	s.Script("CIRCLE\n0,0\nx")
	if !strings.Contains(hist(s), "Requires numeric distance or second point.") {
		t.Errorf("history:\n%s", hist(s))
	}
	s.Escape()
	s.Script("ZOOM\nbad")
	s.Escape()
	if got := cmdsys.InputErrorKey(cmdsys.ErrInvalidValue, &cmdsys.Prompt{Accepts: cmdsys.InputKeyword, Keywords: []string{"Yes"}}); got != cmdsys.MsgInvalidKeyword {
		t.Errorf("keyword-only prompt key = %s", got)
	}
	if got := cmdsys.InputErrorKey(cmdsys.ErrInvalidValue, &cmdsys.Prompt{Accepts: cmdsys.InputAngle}); got != cmdsys.MsgNeedAngle {
		t.Errorf("angle prompt key = %s", got)
	}
	if got := cmdsys.InputErrorKey(cmdsys.ErrInvalidValue, nil); got != cmdsys.MsgInvalidValue {
		t.Errorf("nil prompt key = %s", got)
	}
}

func TestScriptTerminatorAndTabs(t *testing.T) {
	// The file's last newline terminates the last response; a blank line after it is an Enter.
	s := newSession()
	s.Script("LINE\n0,0\n10,0\n")
	if s.Active() != "LINE" {
		t.Fatalf("final newline sent an Enter: active %q", s.Active())
	}
	s.Script("\n")
	s = newSession()
	s.Script("LINE\n0,0\n10,0\n\n")
	if s.Active() != "" {
		t.Fatalf("blank line before the end must be Enter: active %q", s.Active())
	}
	s = newSession()
	s.Script("LINE\t0,0\t10,0 ; comment\n\n")
	if s.Active() != "" || len(lines(s)) != 1 {
		t.Fatalf("tabs separate: active %q, lines %d", s.Active(), len(lines(s)))
	}
	s = newSession()
	s.Script("LINE 0,0 10,0 ")
	if s.Active() != "" {
		t.Fatalf("a trailing space is an Enter: active %q", s.Active())
	}
}

func TestScriptCommandRunsFileAndDropsCaller(t *testing.T) {
	dir := t.TempDir()
	inner := filepath.Join(dir, "inner.scr")
	if err := os.WriteFile(inner, []byte("LINE\n0,0\n10,0\n\nLINE\n0,5\n10,5\n\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	s := newSession()
	s.Script("SCRIPT\n" + inner + "\nLINE\n0,50\n50,50\n\n")
	if got := len(lines(s)); got != 2 {
		t.Fatalf("the nested script runs to its end and the outer one does not resume: %d lines\n%s", got, hist(s))
	}
	// Interactively too, and the extension may be left out.
	s = newSession()
	s.Submit("SCR")
	s.Submit(strings.TrimSuffix(inner, ".scr"))
	if got := len(lines(s)); got != 2 || s.Active() != "" {
		t.Fatalf("interactive SCRIPT: %d lines, active %q", got, s.Active())
	}
	s.Script("SCRIPT\n" + filepath.Join(dir, "missing.scr"))
	if !strings.Contains(hist(s), "Cannot open script file") {
		t.Fatalf("history:\n%s", hist(s))
	}
	s.SetAgentCaller(true)
	s.Submit("SCRIPT")
	if !strings.Contains(hist(s), "not available to agents") {
		t.Fatalf("SCRIPT must be human only:\n%s", hist(s))
	}
}

func TestRScriptRestartsBounded(t *testing.T) {
	s := newSession()
	s.Script("LINE\n0,0\n1,1\n\nRSCRIPT\n")
	if got := len(lines(s)); got != 1001 {
		t.Fatalf("RSCRIPT restarts until the cap: %d lines", got)
	}
	// Outside a script there is nothing to restart.
	s = newSession()
	s.Submit("RSCRIPT")
	if s.Active() != "" || len(lines(s)) != 0 {
		t.Fatal("RSCRIPT outside a script does nothing")
	}
}

func TestDelayValidates(t *testing.T) {
	s := newSession()
	s.Script("DELAY\n99999")
	if !strings.Contains(hist(s), "between 0 and 32767") || s.Active() != "DELAY" {
		t.Fatalf("history:\n%s", hist(s))
	}
	s.Submit("500")
	if s.Active() != "" {
		t.Fatal("DELAY should end")
	}
}

func TestLiveStateVariables(t *testing.T) {
	s := newSession()
	if got := s.Vars().String("CMDACTIVE"); got != "0" {
		t.Errorf("CMDACTIVE = %s", got)
	}
	s.Submit("LINE")
	if s.Vars().Text("CMDNAMES") != "LINE" || s.Vars().Int("CMDACTIVE") != 1 {
		t.Errorf("CMDNAMES %q", s.Vars().Text("CMDNAMES"))
	}
	s.Submit("'ZOOM")
	if s.Vars().Text("CMDNAMES") != "LINE'ZOOM" {
		t.Errorf("CMDNAMES %q", s.Vars().Text("CMDNAMES"))
	}
	if !s.Vars().Has("VIEWCTR") || !s.Vars().Has("SCREENSIZE") {
		t.Error("view variables")
	}
}
