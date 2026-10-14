package settings

import (
	"path/filepath"
	"runtime"
	"strings"
)

// Caps of the recent lists.
const (
	// MaxRecentFiles is how many recent drawings Recent.Files keeps.
	MaxRecentFiles = 10
	// MaxRecentFolders is how many recent folders Recent.Folders keeps.
	MaxRecentFolders = 10
)

// Recent is the drawings opened or saved lately and the folders they and the
// file dialogs used, most recent first. Paths are absolute and cleaned; a
// file that no longer exists stays until the user picks it (the UI then
// drops it) or clears the lists.
type Recent struct {
	// Files are drawing paths, newest first, at most MaxRecentFiles.
	Files []string
	// Folders are directory paths, newest first, at most MaxRecentFolders.
	Folders []string
}

// CleanPath returns path made absolute (against the working directory) and
// cleaned, the form the recent lists store and compare. An empty path stays
// empty.
func CleanPath(path string) string {
	if strings.TrimSpace(path) == "" {
		return ""
	}
	if abs, err := filepath.Abs(path); err == nil {
		return abs
	}
	return filepath.Clean(path)
}

// samePath compares two cleaned paths; Windows and macOS file systems ignore
// case by default, so the lists do too there.
func samePath(a, b string) bool {
	if runtime.GOOS == "windows" || runtime.GOOS == "darwin" {
		return strings.EqualFold(a, b)
	}
	return a == b
}

// pushFront returns list with p at the front, any other spelling of p removed
// and at most limit entries.
func pushFront(list []string, p string, limit int) []string {
	out := make([]string, 0, min(len(list)+1, limit))
	out = append(out, p)
	for _, x := range list {
		if len(out) == limit {
			break
		}
		if !samePath(x, p) {
			out = append(out, x)
		}
	}
	return out
}

// AddFile records a drawing that was opened or saved: it moves to the front
// of Files and its folder to the front of Folders.
func (r *Recent) AddFile(path string) {
	p := CleanPath(path)
	if p == "" {
		return
	}
	r.Files = pushFront(r.Files, p, MaxRecentFiles)
	r.AddFolder(filepath.Dir(p))
}

// AddFolder records a folder the user worked in; it moves to the front of
// Folders.
func (r *Recent) AddFolder(dir string) {
	d := CleanPath(dir)
	if d == "" {
		return
	}
	r.Folders = pushFront(r.Folders, d, MaxRecentFolders)
}

// RemoveFile drops path from Files (a drawing that could not be found).
func (r *Recent) RemoveFile(path string) {
	p := CleanPath(path)
	out := r.Files[:0:0]
	for _, x := range r.Files {
		if !samePath(x, p) {
			out = append(out, x)
		}
	}
	r.Files = out
}

// RemoveFolder drops dir from Folders.
func (r *Recent) RemoveFolder(dir string) {
	d := CleanPath(dir)
	out := r.Folders[:0:0]
	for _, x := range r.Folders {
		if !samePath(x, d) {
			out = append(out, x)
		}
	}
	r.Folders = out
}

// Clear empties both lists.
func (r *Recent) Clear() { r.Files, r.Folders = nil, nil }

// Empty reports whether both lists are empty.
func (r Recent) Empty() bool { return len(r.Files) == 0 && len(r.Folders) == 0 }

// normalize drops empty entries and duplicates, cleans each path and applies
// the caps. Entries keep their order; relative paths stay relative, because
// the working directory at load time says nothing about where they were.
func (r *Recent) normalize() {
	r.Files = dedupPaths(r.Files, MaxRecentFiles)
	r.Folders = dedupPaths(r.Folders, MaxRecentFolders)
}

func dedupPaths(list []string, limit int) []string {
	var out []string
	for _, x := range list {
		if strings.TrimSpace(x) == "" || len(out) == limit {
			continue
		}
		x = filepath.Clean(x)
		dup := false
		for _, y := range out {
			dup = dup || samePath(x, y)
		}
		if !dup {
			out = append(out, x)
		}
	}
	return out
}

// clone returns a copy that shares no slice with r.
func (r Recent) clone() Recent {
	return Recent{Files: append([]string(nil), r.Files...), Folders: append([]string(nil), r.Folders...)}
}

// marshalPaths writes a TOML multi-line array of strings, one path per line.
func marshalPaths(key string, list []string) string {
	if len(list) == 0 {
		return key + " = []"
	}
	var b strings.Builder
	b.WriteString(key + " = [\n")
	for _, p := range list {
		b.WriteString("  " + quote(p) + ",\n")
	}
	b.WriteString("]")
	return b.String()
}

// parsePaths reads a TOML array of strings; ok is false when v is not an
// array, and elements that are not strings are skipped.
func parsePaths(v any) (paths []string, ok bool) {
	arr, ok := v.([]any)
	if !ok {
		return nil, false
	}
	for _, e := range arr {
		if s, isStr := e.(string); isStr {
			paths = append(paths, s)
		}
	}
	return paths, true
}
