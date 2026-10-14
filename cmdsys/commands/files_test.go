package commands_test

import (
	"errors"
	"strings"
	"testing"

	"go-draw/cmdsys"
	"go-draw/doc"
)

// recFiles records saves and serves one drawing to Open.
type recFiles struct {
	saved []string
	open  *doc.Drawing
	fail  bool
}

func (f *recFiles) Open(string) (*doc.Drawing, []string, error) {
	if f.open == nil {
		return nil, nil, errors.New("missing")
	}
	return f.open, nil, nil
}

func (f *recFiles) Save(path string, _ *doc.Drawing) ([]string, error) {
	if f.fail {
		return nil, errors.New("disk full")
	}
	f.saved = append(f.saved, path)
	return nil, nil
}

func fileSession(t *testing.T) (*cmdsys.Session, *recFiles) {
	s := newSession(t)
	f := &recFiles{open: doc.New()}
	s.SetFiles(f)
	return s, f
}

func dirtyIt(s *cmdsys.Session) { xfeed(s, "LINE", "0,0", "10,0", "") }

func TestNewOnACleanDrawingAsksNothing(t *testing.T) {
	s, _ := fileSession(t)
	xfeed(s, "NEW")
	if s.Active() != "" {
		t.Fatalf("NEW prompted: %q", s.Prompt())
	}
}

func TestNewAsksSaveChangesAndCancelKeepsTheDrawing(t *testing.T) {
	s, f := fileSession(t)
	dirtyIt(s)
	xfeed(s, "NEW")
	if got := s.Prompt(); got != "Save changes to Untitled? [Yes/No/Cancel] <Yes>:" {
		t.Fatalf("prompt %q", got)
	}
	xfeed(s, "C")
	if lineCount(s) != 1 || len(f.saved) != 0 || s.Active() != "" {
		t.Fatalf("Cancel: %d lines, %v", lineCount(s), f.saved)
	}
}

func TestNewNoDiscardsWithoutSaving(t *testing.T) {
	s, f := fileSession(t)
	dirtyIt(s)
	xfeed(s, "NEW", "N")
	if lineCount(s) != 0 || len(f.saved) != 0 || s.Dirty() {
		t.Fatalf("%d lines, saved %v, dirty %v", lineCount(s), f.saved, s.Dirty())
	}
}

func TestNewYesWithoutPathAsksForOneThenSaves(t *testing.T) {
	s, f := fileSession(t)
	dirtyIt(s)
	xfeed(s, "NEW", "Y")
	if !strings.Contains(s.Prompt(), "path of drawing to save") {
		t.Fatalf("prompt %q", s.Prompt())
	}
	xfeed(s, "a.dxf")
	if len(f.saved) != 1 || f.saved[0] != "a.dxf" || lineCount(s) != 0 || s.FilePath() != "" {
		t.Fatalf("saved %v, lines %d, path %q", f.saved, lineCount(s), s.FilePath())
	}
}

func TestNewYesWithPathSavesAndNamesTheFile(t *testing.T) {
	s, f := fileSession(t)
	s.SetFilePath("/tmp/plan.dxf")
	dirtyIt(s)
	xfeed(s, "NEW")
	if got := s.Prompt(); !strings.HasPrefix(got, "Save changes to plan.dxf?") {
		t.Fatalf("prompt %q", got)
	}
	xfeed(s, "")
	if len(f.saved) != 1 || f.saved[0] != "/tmp/plan.dxf" || lineCount(s) != 0 {
		t.Fatalf("saved %v, lines %d", f.saved, lineCount(s))
	}
}

func TestFailedSaveKeepsTheDrawing(t *testing.T) {
	s, f := fileSession(t)
	f.fail = true
	s.SetFilePath("/tmp/plan.dxf")
	dirtyIt(s)
	xfeed(s, "NEW", "Y")
	if lineCount(s) != 1 || !s.Dirty() || !strings.Contains(lastLines(s, 1), "Save failed") {
		t.Fatalf("%d lines, dirty %v, %s", lineCount(s), s.Dirty(), lastLines(s, 1))
	}
}

func TestQsaveIsSilentWithAPathAndAsksWithout(t *testing.T) {
	s, f := fileSession(t)
	dirtyIt(s)
	xfeed(s, "QSAVE")
	if !strings.Contains(s.Prompt(), "path of drawing to save") {
		t.Fatalf("prompt %q", s.Prompt())
	}
	xfeed(s, "b.dxf")
	if s.FilePath() != "b.dxf" || s.Dirty() || len(f.saved) != 1 {
		t.Fatalf("path %q dirty %v saved %v", s.FilePath(), s.Dirty(), f.saved)
	}
	dirtyIt(s)
	xfeed(s, "QSAVE")
	if s.Active() != "" || len(f.saved) != 2 || s.Dirty() {
		t.Fatalf("QSAVE with a path: active %q saved %v dirty %v", s.Active(), f.saved, s.Dirty())
	}
}

func TestSaveAsOffersTheCurrentPathAsDefault(t *testing.T) {
	s, f := fileSession(t)
	s.SetFilePath("c.dxf")
	xfeed(s, "SAVEAS")
	if !strings.HasSuffix(s.Prompt(), "<c.dxf>:") {
		t.Fatalf("prompt %q", s.Prompt())
	}
	xfeed(s, "d.dxf")
	if s.FilePath() != "d.dxf" || len(f.saved) != 1 {
		t.Fatalf("path %q saved %v", s.FilePath(), f.saved)
	}
}

func TestOpenAsksAfterReadingTheFile(t *testing.T) {
	s, f := fileSession(t)
	other := doc.New()
	f.open = other
	dirtyIt(s)
	xfeed(s, "OPEN", "x.dxf")
	if !strings.HasPrefix(s.Prompt(), "Save changes") || s.Drawing() == other {
		t.Fatalf("prompt %q", s.Prompt())
	}
	xfeed(s, "N")
	if s.Drawing() != other || s.FilePath() != "x.dxf" || s.Dirty() {
		t.Fatalf("path %q dirty %v", s.FilePath(), s.Dirty())
	}
}

func TestOpenOfAMissingFileKeepsTheDrawing(t *testing.T) {
	s, f := fileSession(t)
	f.open = nil
	dirtyIt(s)
	xfeed(s, "OPEN", "x.dxf")
	if s.Active() != "" || lineCount(s) != 1 {
		t.Fatalf("active %q lines %d", s.Active(), lineCount(s))
	}
}

func TestQuitAsksThenCallsTheHost(t *testing.T) {
	s, _ := fileSession(t)
	quit := 0
	s.SetQuitHandler(func() { quit++ })
	xfeed(s, "QUIT")
	if quit != 1 {
		t.Fatalf("clean QUIT called the host %d times", quit)
	}
	dirtyIt(s)
	xfeed(s, "EXIT")
	if quit != 1 || !strings.HasPrefix(s.Prompt(), "Save changes") {
		t.Fatalf("dirty EXIT: quit %d prompt %q", quit, s.Prompt())
	}
	xfeed(s, "C")
	if quit != 1 {
		t.Fatal("Cancel quit anyway")
	}
	xfeed(s, "EXIT", "N")
	if quit != 2 {
		t.Fatalf("EXIT No: quit %d", quit)
	}
}

func TestQuitWithoutAHostSaysSo(t *testing.T) {
	s, _ := fileSession(t)
	xfeed(s, "QUIT")
	if !strings.Contains(lastLines(s, 1), "not available") {
		t.Fatal(lastLines(s, 1))
	}
}

func TestFileCommandsAreRefusedToAgents(t *testing.T) {
	s, _ := fileSession(t)
	s.SetAgentCaller(true)
	for _, n := range []string{"NEW", "OPEN", "SAVE", "QSAVE", "SAVEAS", "CLOSE", "QUIT", "EXIT"} {
		if s.Run(n) {
			t.Errorf("%s ran for an agent", n)
		}
	}
}
