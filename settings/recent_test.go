package settings

import (
	"fmt"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestRecentAddFileMovesToFrontAndDedups(t *testing.T) {
	dir := t.TempDir()
	a, b := filepath.Join(dir, "a.dxf"), filepath.Join(dir, "sub", "b.dxf")
	var r Recent
	r.AddFile(a)
	r.AddFile(b)
	r.AddFile(filepath.Join(dir, "sub", "..", "a.dxf"))
	if want := []string{a, b}; !reflect.DeepEqual(r.Files, want) {
		t.Fatalf("files %v, want %v", r.Files, want)
	}
	if want := []string{dir, filepath.Dir(b)}; !reflect.DeepEqual(r.Folders, want) {
		t.Fatalf("folders %v, want %v", r.Folders, want)
	}
}

func TestRecentRelativePathBecomesAbsolute(t *testing.T) {
	var r Recent
	r.AddFile("plan.dxf")
	if len(r.Files) != 1 || !filepath.IsAbs(r.Files[0]) {
		t.Fatalf("files %v, want one absolute path", r.Files)
	}
}

func TestRecentCaps(t *testing.T) {
	dir := t.TempDir()
	var r Recent
	for i := range MaxRecentFiles + 5 {
		r.AddFile(filepath.Join(dir, fmt.Sprint(i), "d.dxf"))
	}
	if len(r.Files) != MaxRecentFiles || len(r.Folders) != MaxRecentFolders {
		t.Fatalf("%d files, %d folders", len(r.Files), len(r.Folders))
	}
	if r.Files[0] != filepath.Join(dir, fmt.Sprint(MaxRecentFiles+4), "d.dxf") {
		t.Fatalf("newest is not first: %v", r.Files)
	}
}

func TestRecentRemoveAndClear(t *testing.T) {
	dir := t.TempDir()
	var r Recent
	r.AddFile(filepath.Join(dir, "a.dxf"))
	r.AddFile(filepath.Join(dir, "b.dxf"))
	r.RemoveFile(filepath.Join(dir, "a.dxf"))
	if len(r.Files) != 1 || filepath.Base(r.Files[0]) != "b.dxf" {
		t.Fatalf("files %v", r.Files)
	}
	r.RemoveFolder(dir)
	if len(r.Folders) != 0 {
		t.Fatalf("folders %v", r.Folders)
	}
	r.Clear()
	if !r.Empty() {
		t.Fatalf("not empty: %+v", r)
	}
}

func TestRecentRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.toml")
	s := Default()
	s.Recent.AddFile(filepath.Join(dir, `odd "name".dxf`))
	s.Recent.AddFile(filepath.Join(dir, "x", "plan.dxf"))
	if err := Save(path, s); err != nil {
		t.Fatal(err)
	}
	got, w, err := LoadWarn(path)
	if err != nil || len(w) != 0 {
		t.Fatalf("load: %v %v", w, err)
	}
	if !reflect.DeepEqual(got.Recent, s.Recent) {
		t.Fatalf("round trip\n got %+v\nwant %+v", got.Recent, s.Recent)
	}
	if !strings.Contains(string(Marshal(s)), "[recent]\n") {
		t.Fatalf("no [recent] table:\n%s", Marshal(s))
	}
}

// A file written before the recent lists existed loads with empty lists, and one with bad
// entries keeps what it can.
func TestRecentBackwardCompatibleAndTolerant(t *testing.T) {
	s, w, err := Parse([]byte("[appearance]\ntheme = \"light\"\n"))
	if err != nil || len(w) != 0 || !s.Recent.Empty() {
		t.Fatalf("old file: %+v %v %v", s.Recent, w, err)
	}
	s, w, err = Parse([]byte(`
[recent]
files = ["/a/b.dxf", 3, "", "/a/b.dxf", "/c/d.dxf"]
folders = "nope"
other = 1
`))
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{filepath.Clean("/a/b.dxf"), filepath.Clean("/c/d.dxf")}; !reflect.DeepEqual(s.Recent.Files, want) {
		t.Fatalf("files %v, want %v", s.Recent.Files, want)
	}
	if len(w) != 1 || !strings.Contains(w[0], "recent.folders") {
		t.Fatalf("warnings %v, want one for recent.folders", w)
	}
	if s.Extra["recent.other"] != int64(1) {
		t.Fatalf("unknown key not kept: %v", s.Extra)
	}
}

func TestCloneDoesNotShareRecent(t *testing.T) {
	s := Default()
	s.Recent.AddFile(filepath.Join(t.TempDir(), "a.dxf"))
	c := s.Clone()
	c.Recent.Files[0] = "changed"
	if s.Recent.Files[0] == "changed" {
		t.Fatal("Clone shares the recent files slice")
	}
}
