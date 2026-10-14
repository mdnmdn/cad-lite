package ebitenplatform

import (
	"io/fs"
	"os"
	"path"
)

// absPather is ebiten.AbsPather: a dropped entry backed by a real path.
type absPather interface{ AbsPath() string }

// droppedPaths turns the virtual file system ebiten.DroppedFiles returns into
// the real paths that were dropped, sorted by name as the file system lists
// them. ebiten roots it at the deepest directory holding every dropped path,
// so a root entry is a dropped file or folder, or (files dropped from several
// folders) a directory that only leads to them. readDir reads a real
// directory; a virtual one listing fewer entries is such a lead and is walked.
// Entries without a real path (a browser drop) are skipped.
func droppedPaths(fsys fs.FS, readDir func(string) ([]os.DirEntry, error)) []string {
	var out []string
	var walk func(dir string, depth int)
	walk = func(dir string, depth int) {
		ents, err := fs.ReadDir(fsys, dir)
		if err != nil {
			return
		}
		for _, e := range ents {
			a, ok := e.(absPather)
			if !ok || a.AbsPath() == "" {
				continue
			}
			p := a.AbsPath()
			if e.IsDir() && depth < maxDropDepth {
				sub := path.Join(dir, e.Name())
				if leadsOnly(fsys, sub, p, readDir) {
					walk(sub, depth+1)
					continue
				}
			}
			out = append(out, p)
		}
	}
	walk(".", 0)
	return out
}

// maxDropDepth bounds the walk through lead directories; real drops need one
// level per directory between the common root and the dropped paths.
const maxDropDepth = 64

// leadsOnly reports whether the virtual directory sub lists fewer entries than
// the real directory p, which makes it a way to dropped paths rather than a
// dropped folder.
func leadsOnly(fsys fs.FS, sub, p string, readDir func(string) ([]os.DirEntry, error)) bool {
	virt, err := fs.ReadDir(fsys, sub)
	if err != nil {
		return false
	}
	onDisk, err := readDir(p)
	if err != nil {
		return false
	}
	return len(virt) < len(onDisk)
}
