package commands_test

import (
	"testing"

	"cad-lite/cmdsys"
	"cad-lite/cmdsys/commands"
	"cad-lite/doc"
	"cad-lite/render"
)

// fakeHost is a document host that records what the file commands asked of it.
type fakeHost struct {
	shown, opened []string
	news, closes  int
	// target receives an opened drawing; nil makes OPEN replace the session's drawing.
	target *cmdsys.Session
	open   string
	last   bool
}

func (h *fakeHost) ShowOpen(path string) bool {
	h.shown = append(h.shown, path)
	return path == h.open
}

func (h *fakeHost) OpenNew(path string, d *doc.Drawing) *cmdsys.Session {
	h.opened = append(h.opened, path)
	if h.target != nil {
		h.target.SetDrawing(d)
		h.target.SetFilePath(path)
	}
	return h.target
}

func (h *fakeHost) New() bool { h.news++; return true }

func (h *fakeHost) Close() bool { h.closes++; return !h.last }

func hostSession(t *testing.T) (*cmdsys.Session, *fakeHost, *recFiles) {
	s, f := fileSession(t)
	v := render.DefaultView(800, 600)
	h := &fakeHost{target: cmdsys.NewSession(doc.New(), &v, commands.NewRegistry())}
	s.SetDocumentHost(h)
	return s, h, f
}

func TestOpenWithAHostOpensBesideADirtyDrawing(t *testing.T) {
	s, h, f := hostSession(t)
	other := doc.New()
	f.open = other
	dirtyIt(s)
	xfeed(s, "OPEN", "x.dxf")
	if s.Active() != "" || !s.Dirty() || s.Drawing() == other || s.FilePath() != "" {
		t.Fatalf("the dirty drawing was touched or asked about: prompt %q", s.Prompt())
	}
	if h.target.Drawing() != other || h.target.FilePath() != "x.dxf" {
		t.Fatal("the opened drawing did not reach the host's tab")
	}
	if hist := h.target.History(); len(hist) == 0 || hist[len(hist)-1] != "Opened x.dxf (0 entities)." {
		t.Fatalf("the new tab's history %q", hist)
	}
}

func TestOpenWithAHostShowsAnOpenFileWithoutReading(t *testing.T) {
	s, h, f := hostSession(t)
	f.open = nil // reading would fail
	h.open = "x.dxf"
	xfeed(s, "OPEN", "x.dxf")
	if len(h.shown) != 1 || len(h.opened) != 0 || s.Active() != "" {
		t.Fatalf("shown %v opened %v", h.shown, h.opened)
	}
}

func TestOpenWithAHostReplacesAnUntouchedDrawing(t *testing.T) {
	s, h, f := hostSession(t)
	h.target = nil
	other := doc.New()
	f.open = other
	xfeed(s, "OPEN", "x.dxf")
	if s.Drawing() != other || s.FilePath() != "x.dxf" {
		t.Fatal("OPEN should replace the drawing when the host declines a new tab")
	}
}

func TestNewAndCloseWithAHost(t *testing.T) {
	s, h, _ := hostSession(t)
	dirtyIt(s)
	xfeed(s, "NEW")
	if h.news != 1 || s.Active() != "" || lineCount(s) != 1 {
		t.Fatalf("NEW should open a tab and keep this drawing: news %d, prompt %q", h.news, s.Prompt())
	}
	xfeed(s, "CLOSE")
	if h.closes != 0 || s.Active() != "CLOSE" {
		t.Fatal("CLOSE of a dirty drawing should ask first")
	}
	xfeed(s, "N")
	if h.closes != 1 || lineCount(s) != 1 {
		t.Fatalf("CLOSE should hand the tab to the host: closes %d", h.closes)
	}
	h.last = true
	xfeed(s, "CLOSE")
	xfeed(s, "N")
	if lineCount(s) != 0 {
		t.Fatal("CLOSE of the last tab should empty the drawing")
	}
}
