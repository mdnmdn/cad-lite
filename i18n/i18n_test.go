package i18n

import (
	"reflect"
	"strings"
	"testing"
)

// use sets the language for one test and restores it.
func use(t *testing.T, l Lang) {
	t.Helper()
	old := Current()
	Set(l)
	t.Cleanup(func() { Set(old) })
}

func TestLookup(t *testing.T) {
	use(t, English)
	if got := T("ui.status.grid"); got != "GRID" {
		t.Errorf("en grid = %q", got)
	}
	Set(Italian)
	if got := T("ui.status.grid"); got != "GRIGLIA" {
		t.Errorf("it grid = %q", got)
	}
	if got := T("cmd.layer.msg.created", "Muri"); got != `Layer "Muri" creato.` {
		t.Errorf("it created = %q", got)
	}
	if got := For(English).T("session.cancel"); got != "*Cancel*" {
		t.Errorf("For(English) = %q", got)
	}
}

func TestNoArgsIsRaw(t *testing.T) {
	use(t, English)
	if got := T("ui.settings.ui_scale_value"); got != "%d%%" {
		t.Errorf("no-arg value must be returned as written, got %q", got)
	}
	if got := T("ui.settings.ui_scale_value", 150); got != "150%" {
		t.Errorf("with args = %q", got)
	}
}

func TestFallback(t *testing.T) {
	use(t, Italian)
	var missing []string
	MissingHandler = func(l Lang, k string) { missing = append(missing, string(l)+":"+k) }
	defer func() { MissingHandler = nil }()
	if got := T("no.such.key"); got != "no.such.key" {
		t.Errorf("missing key = %q, want the key", got)
	}
	if len(missing) == 0 || missing[0] != "it:no.such.key" {
		t.Errorf("MissingHandler got %v", missing)
	}
	// A key present only in English falls back to it.
	delete(catalogs[Italian].msgs, "session.cancel")
	defer func() { catalogs[Italian].msgs["session.cancel"] = "*Annulla*" }()
	if got := T("session.cancel"); got != "*Cancel*" {
		t.Errorf("English fallback = %q", got)
	}
}

func TestUnsupportedLangIsEnglish(t *testing.T) {
	use(t, "de")
	if Current() != English {
		t.Errorf("Current = %q", Current())
	}
	if For("fr").Lang() != English {
		t.Error("For(unsupported) must be English")
	}
}

func TestPlural(t *testing.T) {
	cases := []struct {
		l    Lang
		n    int
		want string
	}{
		{English, 1, "1 object joined."}, {English, 0, "0 objects joined."}, {English, 3, "3 objects joined."},
		{Italian, 1, "1 oggetto unito."}, {Italian, 0, "0 oggetti uniti."}, {Italian, 5, "5 oggetti uniti."},
	}
	for _, c := range cases {
		if got := For(c.l).Plural("cmd.join.msg.joined", c.n); got != c.want {
			t.Errorf("%s %d = %q, want %q", c.l, c.n, got, c.want)
		}
	}
	if got := For(Italian).Plural("cmd.common.msg.found", 2); got != "2 trovati" {
		t.Errorf("found = %q", got)
	}
}

func TestParseLang(t *testing.T) {
	ok := map[string]Lang{"it": Italian, "it_IT.UTF-8": Italian, "it-IT": Italian, "IT_it@euro": Italian,
		"en_US.UTF-8": English, "en": English, "English": English, "italiano": Italian}
	for in, want := range ok {
		if got, found := ParseLang(in); !found || got != want {
			t.Errorf("ParseLang(%q) = %q, %v", in, got, found)
		}
	}
	for _, in := range []string{"", "auto", "C", "POSIX", "de_DE.UTF-8", "xx"} {
		if _, found := ParseLang(in); found {
			t.Errorf("ParseLang(%q) should not match", in)
		}
	}
}

func env(m map[string]string) func(string) string { return func(k string) string { return m[k] } }

func TestDetect(t *testing.T) {
	cases := []struct {
		name string
		env  map[string]string
		want Lang
	}{
		{"none", nil, English},
		{"LANG it", map[string]string{"LANG": "it_IT.UTF-8"}, Italian},
		{"LC_MESSAGES beats LANG", map[string]string{"LC_MESSAGES": "en_US", "LANG": "it_IT"}, English},
		{"LC_ALL beats all", map[string]string{"LC_ALL": "it_IT", "LC_MESSAGES": "en_US", "LANG": "en_US"}, Italian},
		{"LC_ALL=C wins over LANG", map[string]string{"LC_ALL": "C", "LANG": "it_IT"}, English},
		{"unsupported stops the search", map[string]string{"LC_ALL": "de_DE", "LANG": "it_IT"}, English},
		{"empty LC_ALL is skipped", map[string]string{"LC_ALL": "", "LANG": "it_IT"}, Italian},
	}
	for _, c := range cases {
		if got := DetectWith(env(c.env)); got != c.want {
			t.Errorf("%s: %q, want %q", c.name, got, c.want)
		}
	}
}

func TestPlatformHook(t *testing.T) {
	PlatformLang = func() Lang { return Italian }
	defer func() { PlatformLang = nil }()
	if got := DetectWith(env(nil)); got != Italian {
		t.Errorf("hook with empty env = %q", got)
	}
	if got := DetectWith(env(map[string]string{"LANG": "en_US"})); got != English {
		t.Errorf("env must win over the hook, got %q", got)
	}
}

func TestFromSetting(t *testing.T) {
	if FromSetting("it") != Italian || FromSetting("en") != English {
		t.Error("explicit settings must win")
	}
	t.Setenv("LC_ALL", "it_IT.UTF-8")
	if FromSetting("auto") != Italian || FromSetting("") != Italian {
		t.Error("auto must detect")
	}
}

func TestPlaceholders(t *testing.T) {
	cases := map[string][]string{
		"plain":             nil,
		"%s of %d":          {"1:s", "2:d"},
		"%[2]s then %[1]s":  {"1:s", "2:s"},
		"%d%%":              {"1:d"},
		"Layer %q created.": {"1:q"},
		"%-8s|%5.2f":        {"1:s", "2:f"},
		"%s %[1]s":          {"1:s", "1:s"},
	}
	for in, want := range cases {
		if got := Placeholders(in); !reflect.DeepEqual(got, want) {
			t.Errorf("Placeholders(%q) = %v, want %v", in, got, want)
		}
	}
}

func TestCatalogParityAndSanity(t *testing.T) {
	if p := Verify(); len(p) > 0 {
		t.Fatalf("catalog problems:\n%s", strings.Join(p, "\n"))
	}
	if len(catalogs[English].msgs) < 400 {
		t.Errorf("suspiciously small catalog: %d keys", len(catalogs[English].msgs))
	}
}

func TestVerifyCatchesMismatch(t *testing.T) {
	catalogs[Italian].msgs["session.command_echo"] = "Comando: %d"
	catalogs[Italian].msgs["zz.extra"] = "x"
	defer func() {
		catalogs[Italian].msgs["session.command_echo"] = "Comando: %s"
		delete(catalogs[Italian].msgs, "zz.extra")
	}()
	p := strings.Join(Verify(), "\n")
	for _, want := range []string{`"session.command_echo": placeholders differ`, `"zz.extra" is in it.toml but not in en.toml`} {
		if !strings.Contains(p, want) {
			t.Errorf("Verify missed %q in:\n%s", want, p)
		}
	}
}

func TestKeywords(t *testing.T) {
	use(t, Italian)
	if got := KeywordLabel("Close"); got != "Chiudi" {
		t.Errorf("label = %q", got)
	}
	if got := KeywordShortcut("ARea"); got != "AR" {
		t.Errorf("shortcut = %q", got)
	}
	if got := PromptKeywords([]string{"Close", "Undo"}); got != "Chiudi/Annulla" {
		t.Errorf("prompt keywords = %q", got)
	}
	ids := []string{"Close", "Undo"}
	for _, in := range []string{"chiudi", "C", "c", "Close", "_close", "'_Close"} {
		if id, ok := MatchKeywords(ids, in); !ok || id != "Close" {
			t.Errorf("MatchKeywords(%q) = %q, %v", in, id, ok)
		}
	}
	// English shortcut still works in an Italian session; the Italian one wins when both exist.
	if id, _ := MatchKeywords(ids, "u"); id != "Undo" {
		t.Errorf("English shortcut U = %q", id)
	}
	if id, _ := MatchKeywords(ids, "a"); id != "Undo" {
		t.Errorf("Italian shortcut A = %q", id)
	}
	// "_" forces English: "_a" must not hit Annulla's Italian shortcut, and nothing in English matches it.
	if _, ok := MatchKeywords(ids, "_a"); ok {
		t.Error("_a must be English-only")
	}
	if id, ok := MatchKeywords([]string{"Yes", "No"}, "si"); !ok || id != "Yes" {
		t.Errorf("synonym si = %q, %v", id, ok)
	}
	if id, ok := MatchKeywords([]string{"Yes", "No"}, "Sì"); !ok || id != "Yes" {
		t.Errorf("Sì = %q, %v", id, ok)
	}
	if _, ok := MatchKeywords(ids, "zzz"); ok {
		t.Error("garbage matched")
	}
	if _, ok := MatchKeywords(ids, ""); ok {
		t.Error("empty matched")
	}
}

func TestKeywordsEnglishAndUnknown(t *testing.T) {
	use(t, English)
	if got := PromptKeywords([]string{"Extents", "Window"}); got != "Extents/Window" {
		t.Errorf("got %q", got)
	}
	if id, ok := MatchKeywords([]string{"cuTting edges"}, "t"); !ok || id != "cuTting edges" {
		t.Errorf("shortcut t = %q, %v", id, ok)
	}
	if id, ok := MatchKeywords([]string{"Extents", "Window"}, "w"); !ok || id != "Window" {
		t.Errorf("w = %q %v", id, ok)
	}
	// A keyword with no catalog entry behaves as it does today.
	if got := KeywordLabel("Frobnicate"); got != "Frobnicate" || KeywordShortcut("Frobnicate") != "F" {
		t.Errorf("unknown keyword: %q %q", got, KeywordShortcut("Frobnicate"))
	}
	if id, ok := MatchKeywords([]string{"Frobnicate"}, "frobnicate"); !ok || id != "Frobnicate" {
		t.Error("unknown keyword must still match by name")
	}
	if id, ok := MatchKeywords([]string{"ARea", "Angle"}, "ar"); !ok || id != "ARea" {
		t.Errorf("ar = %q %v", id, ok)
	}
	if id, ok := MatchKeywords([]string{"ARea", "Angle"}, "a"); !ok || id != "Angle" {
		t.Errorf("a = %q %v", id, ok)
	}
}

func TestEveryKeywordIDResolves(t *testing.T) {
	for g, ids := range KeywordGroups {
		for _, id := range ids {
			for _, l := range Langs {
				Set(l)
				got, ok := MatchKeywords(ids, KeywordLabel(id))
				if !ok || got != id {
					t.Errorf("%s/%s: label %q resolves to %q, %v", g, l, KeywordLabel(id), got, ok)
				}
				got, ok = MatchKeywords(ids, KeywordShortcut(id))
				if !ok || got != id {
					t.Errorf("%s/%s: shortcut %q resolves to %q, %v", g, l, KeywordShortcut(id), got, ok)
				}
			}
		}
	}
	Set(English)
}

func TestCommandMetadata(t *testing.T) {
	use(t, Italian)
	c := Command("line")
	if c.Name != "Linea" || !reflect.DeepEqual(c.Aliases, []string{"LINEA"}) {
		t.Errorf("LINE it = %+v", c)
	}
	if !strings.HasPrefix(c.Tip, "Linea — ") || !strings.HasSuffix(c.Tip, "(LINEA / L)") {
		t.Errorf("tip format: %q", c.Tip)
	}
	if got := CommandName("CHAMFER"); got != "Smusso" {
		t.Errorf("CHAMFER = %q", got)
	}
	want := map[string]string{"LINEA": "LINE", "CERCHIO": "CIRCLE", "ARCO": "ARC", "RETTANGOLO": "RECTANG",
		"CANCELLA": "ERASE", "SPOSTA": "MOVE", "COPIA": "COPY", "RUOTA": "ROTATE", "SCALA": "SCALE",
		"SPECCHIO": "MIRROR", "RACCORDO": "FILLET", "CIMATURA": "CHAMFER", "TAGLIA": "TRIM",
		"ESTENDI": "EXTEND", "SPEZZA": "BREAK", "UNISCI": "JOIN", "ESPLODI": "EXPLODE",
		"ANNULLA": "UNDO", "RIPETI": "REDO", "L": "LINE", "U": "UNDO"}
	got := map[string]string{}
	RegisterAliases(func(a, c string) { got[a] = c })
	for a, c := range want {
		if got[a] != c {
			t.Errorf("alias %s = %q, want %q", a, got[a], c)
		}
	}
}

func TestToolbarTooltipsExist(t *testing.T) {
	toolbar := map[string]string{"line": "LINE", "pline": "PLINE", "circle": "CIRCLE", "arc": "ARC", "rect": "RECTANG",
		"move": "MOVE", "copy": "COPY", "rotate": "ROTATE", "scale": "SCALE", "erase": "ERASE", "dist": "DIST",
		"area": "AREA", "undo": "UNDO", "redo": "REDO", "zoom": "ZOOM"}
	for _, l := range Langs {
		for id, cmd := range toolbar {
			if !Has(l, "ui.toolbar."+id) {
				t.Errorf("%s: no label ui.toolbar.%s", l, id)
			}
			if tip := CommandIn(l, cmd).Tip; !strings.Contains(tip, " — ") || !strings.HasSuffix(tip, ")") {
				t.Errorf("%s: tooltip for %s is not \"Title — text (keys)\": %q", l, cmd, tip)
			}
		}
	}
}

func TestSnapNamesTranslated(t *testing.T) {
	use(t, Italian)
	for k, want := range map[string]string{"snap.endpoint": "Punto finale", "snap.center": "Centro",
		"snap.quadrant": "Quadrante", "snap.intersection": "Intersezione", "snap.nearest": "Vicino"} {
		if got := T(k); got != want {
			t.Errorf("%s = %q", k, got)
		}
	}
}
