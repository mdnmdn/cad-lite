package fileio_test

import (
	"io"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"cad-lite/doc"
	"cad-lite/fileio"
)

func TestNearestVersion(t *testing.T) {
	w := []string{"AC1015", "AC1009"}
	for in, want := range map[string]string{"": "", "AC1006": "AC1009", "AC1009": "AC1009", "AC1012": "AC1015", "AC1014": "AC1015", "AC1015": "AC1015", "AC1032": "AC1015"} {
		if got := fileio.NearestVersion(in, w); got != want {
			t.Errorf("NearestVersion(%q) = %q, want %q", in, got, want)
		}
	}
	if got := fileio.NearestVersion("AC1015", nil); got != "" {
		t.Errorf("no writable versions: %q", got)
	}
}

// versionWriter writes the version it was asked for, and "auto" for its own choice.
type versionWriter struct{}

func (versionWriter) Extensions() []string { return []string{".v"} }
func (versionWriter) Versions() []string   { return []string{"AC1015", "AC1009"} }
func (w versionWriter) Write(out io.Writer, d *doc.Drawing) ([]string, error) {
	return w.WriteVersion(out, d, "")
}
func (versionWriter) WriteVersion(out io.Writer, _ *doc.Drawing, v string) ([]string, error) {
	if v == "" {
		v = "auto"
	}
	_, err := io.WriteString(out, v)
	return nil, err
}
func (versionWriter) AutoVersion(*doc.Drawing) string { return "AC1009" }

func TestSaveWritesTheDrawingsVersionAndSaveWithOverridesIt(t *testing.T) {
	var r fileio.Registry
	r.AddWriter(versionWriter{})
	path := filepath.Join(t.TempDir(), "a.v")
	read := func() string { b, _ := os.ReadFile(path); return string(b) }
	d := doc.New()
	if _, err := r.Save(path, d); err != nil || read() != "auto" {
		t.Fatalf("new drawing: %q %v", read(), err)
	}
	d.Header.Version = "AC1032"
	if _, err := r.Save(path, d); err != nil || read() != "AC1015" {
		t.Fatalf("AC1032 drawing: %q %v", read(), err)
	}
	if _, err := r.SaveWith(path, d, fileio.SaveOptions{Version: "AC1009"}); err != nil || read() != "AC1009" {
		t.Fatalf("SaveWith R12: %q %v", read(), err)
	}
	if d.Header.Version != "AC1032" {
		t.Fatalf("SaveWith changed the drawing's version to %q", d.Header.Version)
	}
	if got := r.SaveVersions(""); !slices.Equal(got, []string{"AC1009", "AC1015"}) {
		t.Fatalf("SaveVersions = %q", got)
	}
	if got := r.SaveVersion("x.v", d, ""); got != "AC1009" {
		t.Fatalf("SaveVersion auto = %q", got)
	}
	if got := r.SaveVersion("x.dwg", d, "AC1014"); got != "AC1015" {
		t.Fatalf("SaveVersion AC1014 = %q", got)
	}
	// A plain Writer writes its one version whatever the options say.
	var plain fileio.Registry
	plain.AddWriter(okWriter{})
	bad := filepath.Join(t.TempDir(), "a.bad")
	if _, err := plain.SaveWith(bad, d, fileio.SaveOptions{Version: "AC1009"}); err != nil {
		t.Fatal(err)
	}
}
