package ebitenplatform

import (
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"reflect"
	"testing"
	"testing/fstest"
)

// absFS is a stand-in for ebiten's dropped-files file system: a MapFS whose
// directory entries carry the real path root/name, as ebiten's do on desktops.
type absFS struct {
	fstest.MapFS
	root string
}

type absEntry struct {
	fs.DirEntry
	abs string
}

func (e absEntry) AbsPath() string { return e.abs }

func (a absFS) ReadDir(name string) ([]fs.DirEntry, error) {
	ents, err := a.MapFS.ReadDir(name)
	if err != nil {
		return nil, err
	}
	out := make([]fs.DirEntry, len(ents))
	for i, e := range ents {
		out[i] = absEntry{e, filepath.Join(a.root, filepath.FromSlash(path.Join(name, e.Name())))}
	}
	return out, nil
}

func touch(t *testing.T, p string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, nil, 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestDroppedPathsFilesInOneFolder(t *testing.T) {
	root := t.TempDir()
	fsys := absFS{fstest.MapFS{"b.dwg": {}, "a.dxf": {}}, root}
	got := droppedPaths(fsys, os.ReadDir)
	want := []string{filepath.Join(root, "a.dxf"), filepath.Join(root, "b.dwg")}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("droppedPaths = %v, want %v", got, want)
	}
}

func TestDroppedPathsFromSeveralFoldersAndADroppedFolder(t *testing.T) {
	root := t.TempDir()
	// On disk: x/one.dxf next to x/other.txt, and the folder y with two files.
	touch(t, filepath.Join(root, "x", "one.dxf"))
	touch(t, filepath.Join(root, "x", "other.txt"))
	touch(t, filepath.Join(root, "y", "p.dxf"))
	touch(t, filepath.Join(root, "y", "q.dxf"))
	// Dropped: x/one.dxf and the folder y. ebiten lists x as a lead (only
	// one.dxf) and y as the real folder.
	fsys := absFS{fstest.MapFS{
		"x/one.dxf": {},
		"y/p.dxf":   {},
		"y/q.dxf":   {},
	}, root}
	got := droppedPaths(fsys, os.ReadDir)
	want := []string{filepath.Join(root, "x", "one.dxf"), filepath.Join(root, "y")}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("droppedPaths = %v, want %v", got, want)
	}
}

func TestDroppedPathsSkipsEntriesWithoutRealPath(t *testing.T) {
	// A browser drop: the files exist only in memory.
	fsys := fstest.MapFS{"a.dxf": {}}
	if got := droppedPaths(fsys, os.ReadDir); len(got) != 0 {
		t.Fatalf("droppedPaths = %v, want none", got)
	}
}

func TestOpenQueueDrainsBatchesInOrder(t *testing.T) {
	q := newOpenQueue()
	q.push(nil)
	q.push([]string{"/a.dxf"})
	q.push([]string{"/b.dxf", "/c.dwg"})
	got := q.drain()
	want := [][]string{{"/a.dxf"}, {"/b.dxf", "/c.dwg"}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("drain = %v, want %v", got, want)
	}
	if q.drain() != nil {
		t.Fatal("queue not emptied")
	}
	for i := 0; i < openQueueMax+5; i++ {
		q.push([]string{"/x"})
	}
	if n := len(q.drain()); n != openQueueMax {
		t.Fatalf("queue kept %d batches, want %d", n, openQueueMax)
	}
}
