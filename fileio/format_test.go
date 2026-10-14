package fileio_test

import (
	"errors"
	"io"
	"os"
	"path/filepath"
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
	if st, _ := os.Stat(path); st.Mode().Perm() != 0o600 {
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
