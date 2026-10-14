package commands_test

import (
	"strings"
	"testing"

	"go-draw/cmdsys"
	"go-draw/cmdsys/commands"
	"go-draw/doc"
	"go-draw/geom"
	"go-draw/i18n"
	"go-draw/render"
)

// italian switches the global language for one test and fails it on any catalog gap.
func italian(t *testing.T) {
	t.Helper()
	i18n.Set(i18n.Italian)
	i18n.MissingHandler = func(l i18n.Lang, k string) {
		if l == i18n.Italian {
			t.Errorf("missing Italian key %q", k)
		}
	}
	t.Cleanup(func() { i18n.Set(i18n.English); i18n.MissingHandler = nil })
}

func TestItalianAliasRunsCanonicalCommand(t *testing.T) {
	italian(t)
	s := newSession(t)
	s.Script("LINEA 0,0 10,0")
	s.Submit("")
	if got := len(entities[*doc.Line](s)); got != 1 {
		t.Fatalf("lines = %d, want 1", got)
	}
	if s.Active() != "" {
		t.Fatalf("active = %q, want idle", s.Active())
	}
	h := history(s)
	if !strings.Contains(h, "Comando: LINEA") {
		t.Fatalf("history lacks the Italian echo:\n%s", h)
	}
	if !strings.Contains(h, "Specificare") && !strings.Contains(h, "punto") {
		t.Fatalf("history lacks Italian prompts:\n%s", h)
	}
	if s.Prompt() != "Comando:" {
		t.Fatalf("idle prompt = %q", s.Prompt())
	}
}

func TestItalianAliasesAreRegisteredInEnglishToo(t *testing.T) {
	s := newSession(t) // English
	s.Script("LINEA 0,0 10,0")
	s.Submit("")
	if got := len(entities[*doc.Line](s)); got != 1 {
		t.Fatalf("LINEA in an English session drew %d lines", got)
	}
}

func TestItalianKeywordsAndPrompt(t *testing.T) {
	italian(t)
	s := newSession(t)
	s.Run("LINE")
	s.Submit("0,0")
	s.Submit("10,0")
	s.Submit("10,10")
	if want := "Specificare punto successivo o [Chiudi/Annulla]:"; !strings.HasSuffix(s.Prompt(), want) {
		t.Fatalf("prompt = %q, want suffix %q", s.Prompt(), want)
	}
	s.Submit("Chiudi")
	if s.Active() != "" {
		t.Fatalf("Chiudi did not finish LINE; prompt %q", s.Prompt())
	}
	if got := len(entities[*doc.Line](s)); got != 3 {
		t.Fatalf("lines = %d, want 3 (two segments and the closing one)", got)
	}
}

func TestItalianKeywordShortcutAndEnglishStillMatch(t *testing.T) {
	for _, typed := range []string{"C", "c", "chiudi", "Close", "_C", "_Close"} {
		t.Run(typed, func(t *testing.T) {
			italian(t)
			s := newSession(t)
			s.Script("LINE 0,0 10,0 10,10")
			s.Submit(typed)
			if s.Active() != "" || len(entities[*doc.Line](s)) != 3 {
				t.Fatalf("%q did not close the line (active %q, %d lines)", typed, s.Active(), len(entities[*doc.Line](s)))
			}
		})
	}
}

func TestUnderscoreForcesEnglishKeywords(t *testing.T) {
	italian(t)
	// "Window" is Finestra in Italian; "_W" and "_Window" are English, "F" is Italian.
	for typed, ok := range map[string]bool{"F": true, "_Window": true, "_W": true, "Finestra": true, "_Finestra": false} {
		s := newSession(t)
		s.Run("ZOOM")
		s.Submit(typed)
		got := s.CurrentPrompt() != nil && strings.Contains(s.Prompt(), "primo angolo")
		if got != ok {
			t.Errorf("ZOOM %q: reached the window corner = %v, want %v (prompt %q)", typed, got, ok, s.Prompt())
		}
	}
}

func TestItalianInputErrorsAndSnapNames(t *testing.T) {
	italian(t)
	s := newSession(t)
	s.Run("LINE")
	s.Submit("abc")
	if h := history(s); !strings.Contains(h, "Punto non valido.") {
		t.Fatalf("history lacks the Italian input error:\n%s", h)
	}
	if got, want := cmdsys.SnapEndpoint.String(), i18n.T("snap.endpoint"); got != want || got == "Endpoint" {
		t.Fatalf("SnapEndpoint.String() = %q, want the Italian %q", got, want)
	}
	s.Escape()
	if !strings.Contains(history(s), i18n.T("session.cancel")) {
		t.Fatalf("cancel line not localized:\n%s", history(s))
	}
}

func TestItalianUnknownCommandAndMessages(t *testing.T) {
	italian(t)
	s := newSession(t)
	s.Run("nonesiste")
	if h := history(s); !strings.Contains(h, `"NONESISTE"`) || strings.Contains(h, "Unknown command") {
		t.Fatalf("unknown-command line not localized:\n%s", h)
	}
	s.Script("LINE 0,0 10,0")
	s.Submit("")
	s.Run("ELENCA")
	s.Submit("")
	s.Submit("")
	if strings.Contains(history(s), "from point") {
		t.Fatalf("LIST output not localized:\n%s", history(s))
	}
}

func TestItalianPluralsAndWholeSentences(t *testing.T) {
	italian(t)
	s := newSession(t)
	addEntities(t, s, line(geom.V(0, 0), geom.V(10, 0)), line(geom.V(10, 0), geom.V(20, 0)), line(geom.V(20, 0), geom.V(30, 0)))
	selectEverything(s)
	s.Run("UNISCI")
	if got := lastLine(s); got != "2 oggetti uniti." {
		t.Fatalf("plural form: last = %q", got)
	}
	s = newSession(t)
	addEntities(t, s, line(geom.V(0, 0), geom.V(10, 0)), line(geom.V(10, 0), geom.V(20, 0)))
	selectEverything(s)
	s.Run("UNISCI")
	if got := lastLine(s); got != "1 oggetto unito." {
		t.Fatalf("singular form: last = %q", got)
	}
	s.Run("RACCORDO")
	if got := s.Prompt(); !strings.HasPrefix(got, "Selezionare primo oggetto o [Raggio]") {
		t.Fatalf("FILLET prompt = %q", got)
	}
}

func TestAgentCallerAlwaysSeesEnglish(t *testing.T) {
	italian(t)
	s := newSession(t)
	s.SetAgentCaller(true)
	s.Run("LINEA") // the alias still resolves; output is English
	s.Submit("0,0")
	s.Submit("10,0")
	if p := s.Prompt(); !strings.Contains(p, "Specify next point or [Undo]") && !strings.Contains(p, "Specify next point") {
		t.Fatalf("agent prompt = %q, want English", p)
	}
	h := history(s)
	if !strings.Contains(h, "Command: LINE") || strings.Contains(h, "Comando") {
		t.Fatalf("agent history not English:\n%s", h)
	}
	s.Submit("5,5")
	s.Submit("Close") // English keyword works for agents
	s.Submit("ci")    // nonsense: error line in English
	s.Run("OPEN")
	s.Run("nope")
	h = history(s)
	for _, it := range []string{"Unknown command", "not available to agents"} {
		if !strings.Contains(h, it) {
			t.Fatalf("agent history lacks %q:\n%s", it, h)
		}
	}
	// A human session next to it still speaks Italian, and the agent flag restores cleanly.
	s.SetAgentCaller(false)
	if s.Prompt() != "Comando:" {
		t.Fatalf("after the agent call the prompt is %q", s.Prompt())
	}
}

func TestAgentCallerIgnoresItalianKeywords(t *testing.T) {
	italian(t)
	s := newSession(t)
	s.SetAgentCaller(true)
	s.Run("LINE")
	s.Submit("0,0")
	s.Submit("10,0")
	s.Submit("10,10")
	if strings.Contains(s.Prompt(), "Chiudi") || !strings.Contains(s.Prompt(), "Close") {
		t.Fatalf("agent prompt = %q", s.Prompt())
	}
	s.Submit("Chiudi") // not an agent keyword: text at a point prompt is an input error
	if s.Active() == "" {
		t.Fatalf("an agent's Italian keyword closed the line")
	}
}

func TestPinnedSessionTranslator(t *testing.T) {
	v := render.DefaultView(800, 600)
	s := cmdsys.NewSession(doc.New(), &v, commands.NewRegistry())
	it := i18n.For(i18n.Italian)
	s.SetTranslator(&it)
	s.Run("LINE")
	if got := s.Prompt(); !strings.HasPrefix(got, "Specificare primo punto") {
		t.Fatalf("pinned Italian session prompt = %q", got)
	}
	s.SetTranslator(nil)
	s.Escape()
	if got := s.Prompt(); got != "Command:" {
		t.Fatalf("unpinned idle prompt = %q", got)
	}
}

func TestEveryCommandRunsWithoutMissingItalianKeys(t *testing.T) {
	italian(t)
	s := newSession(t)
	for _, name := range s.Registry().Names() {
		if name == "OPEN" || name == "SAVE" {
			continue
		}
		s.Run(name)
		_ = s.Prompt()
		s.Escape()
	}
	_ = geom.V(0, 0)
}
