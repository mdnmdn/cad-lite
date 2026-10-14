package fileio_test

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"cad-lite/doc"
	"cad-lite/fileio"
)

type failWriter struct{ partial bool }

func (failWriter) Extensions() []string { return []string{".bad"} }
func (failWriter) Versions() []string   { return nil }
func (w failWriter) Write(out io.Writer, _ *doc.Drawing) ([]string, error) {
	if w.partial {
		io.WriteString(out, "half")
	}
	return nil, errors.New("disk full")
}

type okWriter struct{}

func (okWriter) Extensions() []string { return []string{".bad"} }
func (okWriter) Versions() []string   { return nil }
func (okWriter) Write(out io.Writer, _ *doc.Drawing) ([]string, error) {
	_, err := io.WriteString(out, "new content")
	return nil, err
}

func TestSaveFailureKeepsOriginal(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "a.bad")
	if err := os.WriteFile(path, []byte("original"), 0o600); err != nil {
		t.Fatal(err)
	}
	var r fileio.Registry
	r.AddWriter(failWriter{partial: true})
	if _, err := r.Save(path, doc.New()); err == nil {
		t.Fatal("Save succeeded")
	}
	if b, _ := os.ReadFile(path); string(b) != "original" {
		t.Fatalf("original destroyed: %q", b)
	}
	if es, _ := os.ReadDir(dir); len(es) != 1 {
		t.Fatalf("temp file left behind: %v", es)
	}
}

func TestSaveReplacesAtomicallyAndKeepsMode(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "a.bad")
	os.WriteFile(path, []byte("original"), 0o600)
	var r fileio.Registry
	r.AddWriter(okWriter{})
	if _, err := r.Save(path, doc.New()); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(path); string(b) != "new content" {
		t.Fatalf("content %q", b)
	}
	// Windows has no Unix permission bits: Stat reports 0666 for any writable file.
	if st, _ := os.Stat(path); runtime.GOOS != "windows" && st.Mode().Perm() != 0o600 {
		t.Fatalf("mode %v", st.Mode())
	}
	fresh := filepath.Join(dir, "b.bad")
	if _, err := r.Save(fresh, doc.New()); err != nil {
		t.Fatal(err)
	}
	if es, _ := os.ReadDir(dir); len(es) != 2 {
		t.Fatalf("stray files: %v", es)
	}
}

type stubReader struct{ ext string }

func (r stubReader) Extensions() []string { return []string{r.ext} }
func (stubReader) Read(io.Reader) (*doc.Drawing, []string, error) {
	return doc.New(), nil, nil
}

type extWriter struct{ ext string }

func (w extWriter) Extensions() []string { return []string{w.ext} }
func (extWriter) Versions() []string     { return nil }
func (extWriter) Write(out io.Writer, _ *doc.Drawing) ([]string, error) {
	_, err := io.WriteString(out, "x")
	return nil, err
}

func TestSaveToAFormatWithOnlyAReader(t *testing.T) {
	dir := t.TempDir()
	var r fileio.Registry
	r.AddReader(stubReader{".dwg"})
	r.AddWriter(extWriter{".dxf"})
	path := filepath.Join(dir, "plan.dwg")
	_, err := r.Save(path, doc.New())
	if err == nil || err.Error() != "DWG writing is not supported; save as .dxf" {
		t.Fatalf("Save to .dwg: %v", err)
	}
	if es, _ := os.ReadDir(dir); len(es) != 0 {
		t.Fatalf("a file was left behind: %v", es)
	}
	// An extension nobody handles keeps the generic message.
	if _, err := r.Save(filepath.Join(dir, "plan.xyz"), doc.New()); err == nil || err.Error() != `no writer for ".xyz" files` {
		t.Fatalf("Save to .xyz: %v", err)
	}
	// The extension match is case-insensitive.
	if _, err := r.Save(filepath.Join(dir, "PLAN.DWG"), doc.New()); err == nil || err.Error() != "DWG writing is not supported; save as .dxf" {
		t.Fatalf("Save to .DWG: %v", err)
	}
}

func TestCanReadCanWriteAndWritablePath(t *testing.T) {
	var r fileio.Registry
	if got := r.WritablePath("a.dwg"); got != "a.dwg" {
		t.Errorf("a registry with no writer changed the path: %q", got)
	}
	r.AddReader(stubReader{".dwg"})
	r.AddReader(stubReader{".dxf"})
	r.AddWriter(extWriter{".dxf"})
	for path, want := range map[string][2]bool{"a.dwg": {true, false}, "a.dxf": {true, true}, "a.DXF": {true, true}, "a.txt": {false, false}} {
		if r.CanRead(path) != want[0] || r.CanWrite(path) != want[1] {
			t.Errorf("%s: CanRead %v CanWrite %v, want %v", path, r.CanRead(path), r.CanWrite(path), want)
		}
	}
	for in, want := range map[string]string{
		"/d/plan.dwg": "/d/plan.dxf", "/d/PLAN.DWG": "/d/PLAN.dxf", "/d/plan.dxf": "/d/plan.dxf", "plan": "plan.dxf", "/d.v2/plan.txt": "/d.v2/plan.dxf",
	} {
		if got := r.WritablePath(in); got != want {
			t.Errorf("WritablePath(%q) = %q, want %q", in, got, want)
		}
	}
}
