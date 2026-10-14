// Package filedialog shows the operating system's native open and save dialogs without cgo, by
// running the tool each platform ships: osascript on macOS, PowerShell with System.Windows.Forms on
// Windows, zenity or kdialog on Linux and the BSDs. It is a core package: it knows nothing of the
// UI, and a dialog blocks, so callers run it off their UI goroutine.
package filedialog

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
)

var (
	// ErrCancelled is returned when the user dismissed the dialog without choosing a file.
	ErrCancelled = errors.New("filedialog: cancelled")
	// ErrUnavailable is returned when this system has no dialog backend: no helper tool installed,
	// no display, or a platform (a browser) that cannot run programs.
	ErrUnavailable = errors.New("filedialog: no dialog available")
)

// Filter is one entry of the dialog's file-type list.
type Filter struct {
	// Name is the label the user reads, without the patterns.
	Name string
	// Extensions are the file extensions the entry matches, with or without the leading dot and in
	// any case. An empty list matches every file.
	Extensions []string
}

// Open shows an open-file dialog titled title, starting in startDir (the system default when empty
// or not a directory), and returns the chosen path. The first filter is the one initially selected.
func Open(ctx context.Context, title, startDir string, filters []Filter) (string, error) {
	return run(ctx, request{title: title, startDir: usableDir(startDir), filters: normalize(filters)})
}

// Save shows a save-file dialog with defaultName pre-filled and returns the chosen path. A name
// typed without an extension gets the first extension of the filter list. Overwriting an existing
// file is confirmed by the dialog itself.
func Save(ctx context.Context, title, startDir, defaultName string, filters []Filter) (string, error) {
	return run(ctx, request{save: true, title: title, startDir: usableDir(startDir), defaultName: defaultName, filters: normalize(filters)})
}

// System is the real dialog, as a value a caller can hold behind an interface.
type System struct{}

// Open calls the package function Open.
func (System) Open(ctx context.Context, title, startDir string, filters []Filter) (string, error) {
	return Open(ctx, title, startDir, filters)
}

// Save calls the package function Save.
func (System) Save(ctx context.Context, title, startDir, defaultName string, filters []Filter) (string, error) {
	return Save(ctx, title, startDir, defaultName, filters)
}

// request is one dialog to show.
type request struct {
	save        bool
	title       string
	startDir    string
	defaultName string
	filters     []Filter
}

func usableDir(dir string) string {
	if dir == "" {
		return ""
	}
	if st, err := os.Stat(dir); err != nil || !st.IsDir() {
		return ""
	}
	if abs, err := filepath.Abs(dir); err == nil {
		return abs
	}
	return dir
}

// normalize lower-cases the extensions and drops their dots and any empty entries.
func normalize(in []Filter) []Filter {
	out := make([]Filter, 0, len(in))
	for _, f := range in {
		n := Filter{Name: f.Name}
		for _, e := range f.Extensions {
			e = strings.ToLower(strings.TrimLeft(strings.TrimSpace(e), "*."))
			if e != "" {
				n.Extensions = append(n.Extensions, e)
			}
		}
		out = append(out, n)
	}
	return out
}

// allExtensions is the union of every filter's extensions, or nil when some filter matches every
// file (a type restriction would then hide files the user asked to see).
func allExtensions(filters []Filter) []string {
	var all []string
	seen := map[string]bool{}
	for _, f := range filters {
		if len(f.Extensions) == 0 {
			return nil
		}
		for _, e := range f.Extensions {
			if !seen[e] {
				seen[e] = true
				all = append(all, e)
			}
		}
	}
	return all
}

// defaultExtension is the extension a save name without one gets: the first extension of the first
// filter that has any.
func defaultExtension(filters []Filter) string {
	for _, f := range filters {
		if len(f.Extensions) > 0 {
			return f.Extensions[0]
		}
	}
	return ""
}

// withDefaultExt appends ".ext" to a path with no extension.
func withDefaultExt(path, ext string) string {
	if ext == "" || filepath.Ext(path) != "" {
		return path
	}
	return path + "." + ext
}
