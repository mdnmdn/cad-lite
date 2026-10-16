package commands_test

import (
	"math"
	"strings"
	"testing"

	"cad-lite/cmdsys"
	"cad-lite/doc"
	"cad-lite/geom"
	"cad-lite/i18n"
)

func aidVar(t *testing.T, s *cmdsys.Session, name string) string {
	t.Helper()
	return s.Vars().String(name)
}

func TestOSModeMirrorsSnapsAndSnapOn(t *testing.T) {
	s := newSession(t)
	if got := aidVar(t, s, "OSMODE"); got != "55" { // END MID CEN QUA INT, running snaps on
		t.Fatalf("OSMODE = %s", got)
	}
	s.SnapOn = false
	if got := aidVar(t, s, "OSMODE"); got != "16439" {
		t.Fatalf("OSMODE with snaps off = %s", got)
	}
	if err := s.Vars().SetString("OSMODE", "4133"); err != nil {
		t.Fatal(err)
	}
	if !s.SnapOn || s.Snaps != cmdsys.SnapEndpoint|cmdsys.SnapCenter|cmdsys.SnapIntersection|cmdsys.SnapExtension {
		t.Fatalf("snaps = %b on = %v", s.Snaps, s.SnapOn)
	}
	_ = s.Vars().SetString("OSMODE", "16384")
	if s.SnapOn || s.Snaps != 0 {
		t.Fatalf("16384 must switch the running snaps off: %b %v", s.Snaps, s.SnapOn)
	}
}

func TestOsnapCommand(t *testing.T) {
	s := newSession(t)
	s.Script("-OSNAP\nEND,MID\n")
	if got := aidVar(t, s, "OSMODE"); got != "3" {
		t.Fatalf("OSMODE = %s", got)
	}
	if !strings.Contains(history(s), "Current osnap modes: End,Mid,Cen,Qua,Int") {
		t.Fatalf("history = %q", history(s))
	}
	s.Script("-OSNAP\nNONe\n")
	if got := aidVar(t, s, "OSMODE"); got != "0" {
		t.Fatalf("OSMODE = %s", got)
	}
	s.Script("-OSNAP\nxyz\nper,tan,nea\n")
	if got := aidVar(t, s, "OSMODE"); got != "896" {
		t.Fatalf("OSMODE = %s", got)
	}
	if !strings.Contains(history(s), "Invalid object snap mode: xyz") {
		t.Fatalf("history = %q", history(s))
	}
}

func TestOrthoGridSnapCommands(t *testing.T) {
	s := newSession(t)
	s.Script("ORTHO\nON\n")
	if !s.Ortho || aidVar(t, s, "ORTHOMODE") != "1" {
		t.Fatal("ORTHO ON")
	}
	s.Script("ORTHO\nOFF\n")
	if s.Ortho {
		t.Fatal("ORTHO OFF")
	}
	s.Script("GRID\nOFF\n")
	if s.Grid() || aidVar(t, s, "GRIDMODE") != "0" {
		t.Fatal("GRID OFF")
	}
	s.Script("GRID\n5\n")
	if got := aidVar(t, s, "GRIDUNIT"); got != "5,5" {
		t.Fatalf("GRIDUNIT = %s", got)
	}
	s.Script("SNAP\n2.5\nGRID\n2x\n")
	if got := aidVar(t, s, "GRIDUNIT"); got != "5,5" {
		t.Fatalf("GRIDUNIT after 2x = %s", got)
	}
	s.Script("GRID\nSnap\n")
	if got := aidVar(t, s, "GRIDUNIT"); got != "2.5,2.5" {
		t.Fatalf("GRID Snap = %s", got)
	}
	s.Script("GRID\nMajor\n4\n")
	if got := aidVar(t, s, "GRIDMAJOR"); got != "4" {
		t.Fatalf("GRIDMAJOR = %s", got)
	}
	s.Script("GRID\nAspect\n3\n6\n")
	if got := aidVar(t, s, "GRIDUNIT"); got != "3,6" {
		t.Fatalf("GRID Aspect = %s", got)
	}
	s.Script("SNAP\nON\n")
	if !s.SnapGrid() || aidVar(t, s, "SNAPMODE") != "1" {
		t.Fatal("SNAP ON")
	}
	s.Script("SNAP\nAspect\n4\n8\n")
	if got := aidVar(t, s, "SNAPUNIT"); got != "4,8" {
		t.Fatalf("SNAP Aspect = %s", got)
	}
	s.Script("SNAP\nStyle\nIsometric\n")
	if aidVar(t, s, "SNAPSTYL") != "1" {
		t.Fatal("SNAP Style")
	}
	s.Script("SNAP\nType\nPolar\n")
	if aidVar(t, s, "SNAPTYPE") != "1" {
		t.Fatal("SNAP Type")
	}
	s.Script("SNAP\nOFF\n")
	if s.SnapGrid() {
		t.Fatal("SNAP OFF")
	}
}

func TestSnapToGridAffectsPicksNotTypedPoints(t *testing.T) {
	s := newSession(t)
	s.SnapOn = false
	_ = s.Vars().SetString("SNAPUNIT", "10,10")
	s.Script("SNAP\nON\nLINE\n")
	s.Hover(geom.V(12, 18))
	if got := s.Cursor(); got != geom.V(10, 20) {
		t.Fatalf("cursor = %v", got)
	}
	s.Click(geom.V(12, 18), false)
	s.Submit("3,4")
	s.Submit("")
	ls := sortedLines(s)
	if len(ls) != 1 || !hasLine(ls, geom.V(10, 20), geom.V(3, 4)) {
		t.Fatalf("lines = %v", dumpLines(ls))
	}
}

func TestOrthoConstrainsPicks(t *testing.T) {
	s := newSession(t)
	s.SnapOn = false
	s.Script("ORTHO\nON\nLINE\n0,0\n")
	s.Click(geom.V(50, 3), false)
	s.Submit("")
	if ls := sortedLines(s); len(ls) != 1 || !hasLine(ls, geom.V(0, 0), geom.V(50, 0)) {
		t.Fatalf("lines = %v", dumpLines(ls))
	}
}

func TestTypedSnapOverrides(t *testing.T) {
	cases := []struct {
		name   string
		setup  []doc.Entity
		script string
		a, b   geom.Vec2
	}{
		{"mid", []doc.Entity{line(geom.V(0, 0), geom.V(100, 0))}, "LINE\nMID\n70,0\n0,50\n\n", geom.V(50, 0), geom.V(0, 50)},
		{"end touches only", []doc.Entity{line(geom.V(0, 0), geom.V(100, 0))}, "LINE\nEND\n60,0\n0,50\n\n", geom.V(100, 0), geom.V(0, 50)},
		{"end list", []doc.Entity{line(geom.V(0, 0), geom.V(100, 0))}, "LINE\nEND,MID\n45,0\n0,50\n\n", geom.V(50, 0), geom.V(0, 50)},
		{"underscore and abbreviation", []doc.Entity{line(geom.V(0, 0), geom.V(100, 0))}, "LINE\n_mid\n70,0\n0,50\n\n", geom.V(50, 0), geom.V(0, 50)},
		{"cen", []doc.Entity{&doc.Circle{Center: geom.V(20, 20), Radius: 5}}, "LINE\nCEN\n25,20\n0,0\n\n", geom.V(20, 20), geom.V(0, 0)},
		{"qua", []doc.Entity{&doc.Circle{Center: geom.V(20, 20), Radius: 5}}, "LINE\nQUA\n24,22\n0,0\n\n", geom.V(25, 20), geom.V(0, 0)},
		{"int", []doc.Entity{line(geom.V(0, 0), geom.V(100, 100)), line(geom.V(0, 100), geom.V(100, 0))}, "LINE\nINT\n50.5,50\n0,0\n\n", geom.V(50, 50), geom.V(0, 0)},
		{"nea", []doc.Entity{line(geom.V(0, 0), geom.V(100, 100))}, "LINE\nNEA\n25,25\n0,0\n\n", geom.V(25, 25), geom.V(0, 0)},
		{"non beats running snaps", []doc.Entity{line(geom.V(0, 0), geom.V(100, 0))}, "LINE\nNON\n99.9,0.1\n0,50\n\n", geom.V(99.9, 0.1), geom.V(0, 50)},
		{"from", nil, "LINE\nFROM\n10,10\n@5,5\n0,0\n\n", geom.V(15, 15), geom.V(0, 0)},
		{"m2p", nil, "LINE\nM2P\n0,0\n10,20\n0,0\n\n", geom.V(5, 10), geom.V(0, 0)},
		{"mtp", nil, "LINE\nMTP\n0,0\n10,20\n0,0\n\n", geom.V(5, 10), geom.V(0, 0)},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			s := newSession(t)
			addEntities(t, s, c.setup...)
			n := len(sortedLines(s))
			s.Script(c.script)
			ls := sortedLines(s)
			if len(ls) != n+1 || !hasLine(ls[n:], c.a, c.b) {
				t.Fatalf("lines = %v\nhistory:\n%s", dumpLines(ls), history(s))
			}
		})
	}
}

func TestTypedSnapPerpendicularAndTangent(t *testing.T) {
	s := newSession(t)
	addEntities(t, s, line(geom.V(0, 0), geom.V(100, 100)), &doc.Circle{Center: geom.V(200, 0), Radius: 20})
	s.Script("LINE\n60,0\nPER\n80,80\n\n")
	if !hasLine(sortedLines(s), geom.V(60, 0), geom.V(30, 30)) {
		t.Fatalf("PER: %v", dumpLines(sortedLines(s)))
	}
	// From (200,50) the tangent points of the circle (200,0) r20: x = 200 +- 20*sqrt(21)/5.
	s.Script("LINE\n200,50\nTAN\n210,17\n\n")
	var got geom.Vec2
	for _, l := range sortedLines(s) {
		if l.Start == geom.V(200, 50) {
			got = l.End
		}
	}
	if d := got.Sub(geom.V(200, 0)).Len(); d < 19.999 || d > 20.001 || got.X < 200 {
		t.Fatalf("TAN end = %v", got)
	}
	if v := got.Sub(geom.V(200, 0)).Dot(got.Sub(geom.V(200, 50))); v > 1e-6 || v < -1e-6 {
		t.Fatalf("not tangent: radius . tangent = %v", v)
	}
}

func TestTypedSnapOverrideWorksWithRunningSnapsOffAndClicks(t *testing.T) {
	s := newSession(t)
	s.SnapOn = false
	addEntities(t, s, line(geom.V(0, 0), geom.V(100, 0)))
	s.Script("LINE\nMID\n")
	if got := s.Prompt(); got != "MID of:" {
		t.Fatalf("prompt = %q", got)
	}
	s.Hover(geom.V(70, 1))
	if hit, ok := s.Snap(); !ok || hit.Point != geom.V(50, 0) || hit.Mode != cmdsys.SnapMidpoint {
		t.Fatalf("preview snap = %v %v", hit, ok)
	}
	s.Click(geom.V(70, 1), false)
	if got := s.Prompt(); got != "Specify next point or [Undo]:" && !strings.HasPrefix(got, "Specify next point") {
		t.Fatalf("prompt after the pick = %q", got)
	}
	s.Submit("0,50")
	s.Submit("")
	if !hasLine(sortedLines(s)[1:], geom.V(50, 0), geom.V(0, 50)) {
		t.Fatalf("lines = %v", dumpLines(sortedLines(s)))
	}
}

func TestTypedSnapOverrideAppliesToTheNextPickOnly(t *testing.T) {
	s := newSession(t)
	s.SnapOn = false
	addEntities(t, s, line(geom.V(0, 0), geom.V(100, 0)))
	s.Script("LINE\nMID\n70,0\n")
	s.Submit("70,0")
	s.Submit("")
	if !hasLine(sortedLines(s)[1:], geom.V(50, 0), geom.V(70, 0)) {
		t.Fatalf("lines = %v", dumpLines(sortedLines(s)))
	}
}

func TestTypedSnapNothingFoundKeepsThePromptAndDropsTheOverride(t *testing.T) {
	s := newSession(t)
	s.Script("LINE\nMID\n500,500\n")
	if !strings.Contains(history(s), "No object snap found.") {
		t.Fatalf("history = %q", history(s))
	}
	if !strings.HasPrefix(s.Prompt(), "Specify first point") {
		t.Fatalf("prompt = %q", s.Prompt())
	}
	s.Submit("1,1")
	if !strings.HasPrefix(s.Prompt(), "Specify next point") {
		t.Fatalf("a plain point must be accepted after a failed snap; prompt = %q", s.Prompt())
	}
}

func TestTypedSnapIntersectionNeedsBothObjectsInTheAperture(t *testing.T) {
	s := newSession(t)
	addEntities(t, s, line(geom.V(0, 0), geom.V(100, 100)), line(geom.V(0, 100), geom.V(100, 0)))
	s.Script("LINE\nINT\n10,10\n")
	if !strings.Contains(history(s), "No object snap found.") {
		t.Fatalf("a pick on one line only must not find the crossing; history = %q", history(s))
	}
}

func TestOverrideIsDroppedByEnterAndEscape(t *testing.T) {
	s := newSession(t)
	s.Script("LINE\nMID\n")
	s.Submit("")
	if strings.HasSuffix(s.Prompt(), "of:") {
		t.Fatalf("Enter did not drop the override: %q", s.Prompt())
	}
	s.Escape()
	s.Script("LINE\nEND\n")
	s.Escape()
	s.Script("LINE\n")
	if !strings.HasPrefix(s.Prompt(), "Specify first point") {
		t.Fatalf("prompt = %q", s.Prompt())
	}
	s.Submit("5,5")
	s.Submit("6,6")
	s.Submit("")
	if !hasLine(sortedLines(s), geom.V(5, 5), geom.V(6, 6)) {
		t.Fatalf("a stale override leaked: %v", dumpLines(sortedLines(s)))
	}
}

func TestKeywordWinsOverSnapName(t *testing.T) {
	s := newSession(t)
	s.Script("ARC\n10,0\n")
	s.Submit("End") // the ARC keyword, not the END snap
	if got := s.Prompt(); !strings.Contains(got, "end point") && !strings.Contains(strings.ToLower(got), "end") {
		t.Fatalf("prompt = %q", got)
	}
	if strings.HasSuffix(s.Prompt(), "of:") {
		t.Fatal("End armed the snap instead of the arc keyword")
	}
}

func TestShiftClickSetsInputShiftForTrimExtend(t *testing.T) {
	s := newSession(t)
	standard(t, s)
	addEntities(t, s, line(geom.V(0, 0), geom.V(50, 0)), line(geom.V(100, -10), geom.V(100, 10)))
	// A plain pick of the short line would trim it; Shift extends it to the edge.
	s.Script("TRIM\n\n")
	s.Click(geom.V(40, 0), true)
	s.Submit("")
	if !hasLine(sortedLines(s), geom.V(0, 0), geom.V(100, 0)) {
		t.Fatalf("shift-click did not extend: %v", dumpLines(sortedLines(s)))
	}
}

func TestRemoveModeClickRemovesAndShiftAdds(t *testing.T) {
	s := newSession(t)
	s.SnapOn = false
	addEntities(t, s, line(geom.V(0, 0), geom.V(100, 0)), line(geom.V(0, 50), geom.V(100, 50)))
	s.Script("ERASE\nALL\n")
	if s.Selection().Len() != 2 {
		t.Fatalf("selected %d", s.Selection().Len())
	}
	s.Submit("R")
	s.Click(geom.V(50, 0), false)
	if s.Selection().Len() != 1 {
		t.Fatalf("a plain click in Remove mode left %d selected", s.Selection().Len())
	}
	s.Click(geom.V(50, 0), true) // Shift in Remove mode adds
	if s.Selection().Len() != 2 {
		t.Fatalf("shift-click in Remove mode left %d selected", s.Selection().Len())
	}
}

func TestItalianSnapAliases(t *testing.T) {
	for _, tc := range []struct {
		name, pick string
		objects    []doc.Entity
		want       geom.Vec2
	}{
		{"fine", "70,0", []doc.Entity{line(geom.V(0, 0), geom.V(100, 0))}, geom.V(100, 0)},
		{"  pUnTo   FiNaLe  ", "70,0", []doc.Entity{line(geom.V(0, 0), geom.V(100, 0))}, geom.V(100, 0)},
		{"FINE, INMEZZO", "45,0", []doc.Entity{line(geom.V(0, 0), geom.V(100, 0))}, geom.V(50, 0)},
		{"INMEZZO", "70,0", []doc.Entity{line(geom.V(0, 0), geom.V(100, 0))}, geom.V(50, 0)},
		{"PUNTO MEDIO", "9,1", []doc.Entity{&doc.Arc{Center: geom.V(0, 0), Radius: 10, StartAngle: 0, EndAngle: math.Pi}}, geom.V(0, 10)},
		{"INTERSEZIONE", "50.5,50", []doc.Entity{line(geom.V(0, 0), geom.V(100, 100)), line(geom.V(0, 100), geom.V(100, 0))}, geom.V(50, 50)},
		{"VICINO", "6.3,8.4", []doc.Entity{&doc.Arc{Center: geom.V(0, 0), Radius: 10, StartAngle: 0, EndAngle: math.Pi}}, geom.V(6, 8)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := newSession(t)
			s.SnapOn = false
			addEntities(t, s, tc.objects...)
			n := len(sortedLines(s))
			s.Run("LINE")
			s.Submit(tc.name)
			s.Submit(tc.pick)
			s.Submit("0,50")
			s.Submit("")
			ls := sortedLines(s)
			found := false
			for _, l := range ls[n:] {
				if l.Start.Sub(tc.want).Len() < 1e-8 && l.End == geom.V(0, 50) {
					found = true
				}
			}
			if !found {
				t.Fatalf("snap %q: %v; history %s", tc.name, dumpLines(ls), history(s))
			}
		})
	}
}

func TestItalianSnapListAliases(t *testing.T) {
	for _, text := range []string{"FINE, INMEZZO, INTERSEZIONE, VICINO", " punto finale , punto medio , INT, _nea", "END,MID,INT,NEA"} {
		want := cmdsys.SnapEndpoint | cmdsys.SnapMidpoint | cmdsys.SnapIntersection | cmdsys.SnapNearest
		got, bad, ok := cmdsys.ParseSnapList(text)
		if !ok || got != want {
			t.Errorf("%q = %v, %q, %v", text, got, bad, ok)
		}
		s := newSession(t)
		s.Run("-OSNAP")
		s.Submit(text)
		if s.Snaps != want || !s.SnapOn {
			t.Errorf("-OSNAP %q = %v; %s", text, s.Snaps, history(s))
		}
	}
	for _, text := range []string{"_FINE", "_PUNTO MEDIO", "_INMEZZO", "_INTERSEZIONE", "_VICINO", "IN MEZZO", "FIN", "IN", ""} {
		if _, _, ok := cmdsys.ParseSnapList(text); ok {
			t.Errorf("unexpected accepted snap %q", text)
		}
	}
}

func TestItalianSnapKeywordPriorityAndUnderscore(t *testing.T) {
	italian(t)
	s := newSession(t)
	s.Run("ARC")
	s.Submit("10,0")
	s.Submit("Fine")
	if strings.HasSuffix(s.Prompt(), "di:") || !strings.Contains(s.Prompt(), "finale") {
		t.Fatalf("Fine keyword lost: %s", s.Prompt())
	}
	s.Escape()
	s.Run("LINE")
	before := s.Prompt()
	s.Submit("_FINE")
	if s.Prompt() != before {
		t.Fatalf("_FINE armed localized snap: %s", s.Prompt())
	}
	s.Submit("_END")
	if !strings.Contains(s.Prompt(), "END di:") {
		t.Fatalf("English snap rejected: %s", s.Prompt())
	}
}

func TestSnapAliasesKeepSessionLanguageAndScriptRules(t *testing.T) {
	italian(t)
	english := newSession(t)
	en := i18n.For(i18n.English)
	english.SetTranslator(&en)
	italianSession := newSession(t)
	it := i18n.For(i18n.Italian)
	italianSession.SetTranslator(&it)
	for _, s := range []*cmdsys.Session{english, italianSession} {
		s.SnapOn = false
		addEntities(t, s, line(geom.V(0, 0), geom.V(100, 0)))
		s.Script("LINE\nFINE\n70,0\n0,50\n\n")
		if !hasLine(sortedLines(s), geom.V(100, 0), geom.V(0, 50)) {
			t.Fatalf("script alias failed: %s", history(s))
		}
	}
	if english.Prompt() != "Command:" || italianSession.Prompt() != "Comando:" {
		t.Fatalf("session languages leaked: %q %q", english.Prompt(), italianSession.Prompt())
	}
	for _, s := range []*cmdsys.Session{english, italianSession} {
		s.Run("ARC")
		s.Submit("10,0")
	}
	english.Submit("Fine")
	if !strings.HasSuffix(english.Prompt(), "of:") {
		t.Fatalf("English session unexpectedly matched Italian ARC keyword: %s", english.Prompt())
	}
	italianSession.Submit("Fine")
	if strings.HasSuffix(italianSession.Prompt(), "di:") {
		t.Fatalf("Italian ARC keyword lost priority: %s", italianSession.Prompt())
	}
}

func TestSnapListEnglishRoundTrip(t *testing.T) {
	for mode := cmdsys.SnapMode(0); mode < 1<<14; mode++ {
		text := cmdsys.SnapListString(mode)
		if got, bad, ok := cmdsys.ParseSnapList(text); !ok || got != mode {
			t.Fatalf("roundtrip %d %q: %d %q %v", mode, text, got, bad, ok)
		}
	}
}

func TestInmezzoSnapScript(t *testing.T) {
	s := newSession(t)
	s.SnapOn = false
	addEntities(t, s, line(geom.V(0, 0), geom.V(100, 0)))
	s.Script("LINE\nINMEZZO\n70,0\n0,50\n\n")
	if !hasLine(sortedLines(s), geom.V(50, 0), geom.V(0, 50)) {
		t.Fatalf("INMEZZO script failed: %s", history(s))
	}
}
