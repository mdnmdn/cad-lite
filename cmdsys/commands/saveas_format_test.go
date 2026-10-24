package commands_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"cad-lite/cmdsys"
	"cad-lite/doc"
	"cad-lite/fileio"
	"cad-lite/fileio/dxf"
	"cad-lite/i18n"
)

// dxfSession is a session saving through the real DXF registry, whose writer offers R12 and 2000.
func dxfSession(t *testing.T) *cmdsys.Session {
	t.Helper()
	s := newSession(t)
	r := &fileio.Registry{}
	r.AddReader(dxf.Codec{})
	r.AddWriter(dxf.Codec{})
	s.SetFiles(r)
	return s
}

// acadver returns the $ACADVER a saved file declares.
func acadver(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(string(b), "\n")
	for i, l := range lines {
		if strings.TrimSpace(l) == "$ACADVER" && i+2 < len(lines) {
			return strings.TrimSpace(lines[i+2])
		}
	}
	t.Fatalf("%s has no $ACADVER", path)
	return ""
}

func TestSaveAsAsksForTheFormatAndQsaveKeepsIt(t *testing.T) {
	s := dxfSession(t)
	dirtyIt(s)
	xfeed(s, "SAVEAS")
	if got := lastLines(s, 1); got != "Current file format: AutoCAD R12/LT2 DXF (automatic)" {
		t.Fatalf("history %q", got)
	}
	if got := s.Prompt(); got != "Enter file format [R12/2000] <R12>:" {
		t.Fatalf("prompt %q", got)
	}
	path := filepath.Join(t.TempDir(), "plan.dxf")
	xfeed(s, "2000", path)
	if v := acadver(t, path); v != "AC1015" || s.Drawing().Header.Version != "AC1015" || s.FilePath() != path {
		t.Fatalf("saved %s, drawing version %q, path %q", v, s.Drawing().Header.Version, s.FilePath())
	}
	// QSAVE and SAVE keep the chosen format and do not ask for one.
	os.Remove(path)
	dirtyIt(s)
	xfeed(s, "QSAVE")
	if s.Active() != "" || acadver(t, path) != "AC1015" {
		t.Fatalf("QSAVE: active %q", s.Active())
	}
	xfeed(s, "SAVE")
	if p := s.Prompt(); !strings.Contains(p, "path of drawing to save") {
		t.Fatalf("SAVE prompt %q", p)
	}
	s.Escape()
	// The next SAVEAS offers the chosen format as current and default.
	xfeed(s, "SAVEAS")
	if got := lastLines(s, 1); got != "Current file format: AutoCAD 2000/LT2000 DXF" {
		t.Fatalf("history %q", got)
	}
	if got := s.Prompt(); got != "Enter file format [R12/2000] <2000>:" {
		t.Fatalf("prompt %q", got)
	}
	// Enter takes the default; the path prompt follows with the current path.
	xfeed(s, "")
	if got := s.Prompt(); !strings.HasSuffix(got, "<"+path+">:") {
		t.Fatalf("path prompt %q", got)
	}
}

// A script that answers SAVEAS with the path, as before the format prompt, saves in the drawing's
// format without choosing one.
func TestSaveAsWithAPathAtTheFormatPromptKeepsTheFormat(t *testing.T) {
	s := dxfSession(t)
	dirtyIt(s)
	path := filepath.Join(t.TempDir(), "script.dxf")
	xfeed(s, "SAVEAS "+path)
	if s.Active() != "" || s.FilePath() != path || s.Dirty() {
		t.Fatalf("active %q path %q dirty %v", s.Active(), s.FilePath(), s.Dirty())
	}
	if v := acadver(t, path); v != "AC1009" || s.Drawing().Header.Version != "" {
		t.Fatalf("saved %s, drawing version %q: want automatic R12", v, s.Drawing().Header.Version)
	}
}

func TestSaveAsR12DropsGroupsWithAWarning(t *testing.T) {
	s := dxfSession(t)
	dirtyIt(s)
	d := s.Drawing()
	tx, _ := d.Begin()
	if _, err := tx.AddGroup(&doc.Group{Name: "G", Selectable: true, Members: []doc.Handle{d.Handles()[0]}}); err != nil {
		t.Fatal(err)
	}
	tx.Commit()
	xfeed(s, "SAVEAS")
	if got := s.Prompt(); got != "Enter file format [R12/2000] <2000>:" {
		t.Fatalf("a grouped drawing's automatic format is 2000: %q", got)
	}
	path := filepath.Join(t.TempDir(), "old.dxf")
	xfeed(s, "r12", path)
	if v := acadver(t, path); v != "AC1009" {
		t.Fatalf("saved %s", v)
	}
	if h := strings.Join(s.History(), "\n"); !strings.Contains(h, "Warning: groups not written (R12 has no groups") {
		t.Fatalf("no group warning:\n%s", h)
	}
	if s.Drawing().Header.Version != "AC1009" || len(s.Drawing().Groups()) != 1 {
		t.Fatalf("version %q groups %d", s.Drawing().Header.Version, len(s.Drawing().Groups()))
	}
}

func TestSaveAsFailureKeepsTheFormat(t *testing.T) {
	s := dxfSession(t)
	dirtyIt(s)
	xfeed(s, "SAVEAS", "2000", filepath.Join(t.TempDir(), "missing", "x.dxf"))
	if !strings.Contains(lastLines(s, 1), "Save failed") || s.Drawing().Header.Version != "" || !s.Dirty() {
		t.Fatalf("history %q version %q", lastLines(s, 1), s.Drawing().Header.Version)
	}
}

// An opened AutoCAD 2000 or newer file saves as AutoCAD 2000; R12 files stay automatic.
func TestOpenedFormatIsRemembered(t *testing.T) {
	dir := t.TempDir()
	s := dxfSession(t)
	dirtyIt(s)
	modern := filepath.Join(dir, "modern.dxf")
	xfeed(s, "SAVEAS", "2000", modern)
	o := dxfSession(t)
	xfeed(o, "OPEN", modern)
	if o.Drawing().Header.Version != "AC1015" {
		t.Fatalf("opened version %q", o.Drawing().Header.Version)
	}
	xfeed(o, "SAVEAS")
	if got := lastLines(o, 1); got != "Current file format: AutoCAD 2000/LT2000 DXF" {
		t.Fatalf("history %q", got)
	}
	o.Escape()
	old := filepath.Join(dir, "old.dxf")
	xfeed(s, "SAVEAS", "R12", old)
	r := dxfSession(t)
	xfeed(r, "OPEN", old)
	if r.Drawing().Header.Version != "" {
		t.Fatalf("an R12 file opens automatic, got %q", r.Drawing().Header.Version)
	}
}

func TestSaveAsFormatPromptInItalian(t *testing.T) {
	s := dxfSession(t)
	it := i18n.For(i18n.Italian)
	s.SetTranslator(&it)
	xfeed(s, "SAVEAS")
	if got := lastLines(s, 1); got != "Formato file corrente: AutoCAD R12/LT2 DXF (automatico)" {
		t.Fatalf("history %q", got)
	}
	if got := s.Prompt(); got != "Immettere il formato del file [R12/2000] <R12>:" {
		t.Fatalf("prompt %q", got)
	}
}

// A handler that offers no formats (a test double, the browser) goes straight to the path prompt.
func TestSaveAsWithoutFormatsAsksForThePath(t *testing.T) {
	s, _ := fileSession(t)
	xfeed(s, "SAVEAS")
	if p := s.Prompt(); !strings.Contains(p, "path of drawing to save") {
		t.Fatalf("prompt %q", p)
	}
}
