// Package fileio is the file I/O plugin layer: format readers and writers
// register here and are chosen by file extension. It is named fileio, not
// io, so it never shadows the standard library package.
package fileio

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"cad-lite/doc"
	"cad-lite/i18n"
)

// Reader reads one file format into a new drawing.
type Reader interface {
	// Extensions returns the lower-case extensions it reads, with the dot.
	Extensions() []string
	// Read parses r. Warnings describe data that was skipped or altered.
	Read(r io.Reader) (*doc.Drawing, []string, error)
}

// Writer writes a drawing in one file format.
type Writer interface {
	// Extensions returns the lower-case extensions it writes, with the dot.
	Extensions() []string
	// Versions returns the format versions it writes, e.g. "AC1009".
	Versions() []string
	// Write serializes d to w.
	Write(w io.Writer, d *doc.Drawing) ([]string, error)
}

// Registry holds the available formats.
type Registry struct {
	readers []Reader
	writers []Writer
}

// AddReader registers a reader.
func (r *Registry) AddReader(rd Reader) { r.readers = append(r.readers, rd) }

// AddWriter registers a writer.
func (r *Registry) AddWriter(w Writer) { r.writers = append(r.writers, w) }

// ReadExtensions returns every extension a registered reader reads, lower-case with the dot,
// sorted and without duplicates.
func (r *Registry) ReadExtensions() []string {
	var all []string
	for _, rd := range r.readers {
		all = append(all, rd.Extensions()...)
	}
	return uniqueSorted(all)
}

// WriteExtensions returns every extension a registered writer writes, in the same form.
func (r *Registry) WriteExtensions() []string {
	var all []string
	for _, w := range r.writers {
		all = append(all, w.Extensions()...)
	}
	return uniqueSorted(all)
}

func uniqueSorted(in []string) []string {
	slices.Sort(in)
	return slices.Compact(in)
}

func ext(path string) string { return strings.ToLower(filepath.Ext(path)) }

// Open reads the file at path with the reader registered for its extension.
func (r *Registry) Open(path string) (*doc.Drawing, []string, error) {
	for _, rd := range r.readers {
		if slices.Contains(rd.Extensions(), ext(path)) {
			f, err := os.Open(path)
			if err != nil {
				return nil, nil, err
			}
			defer f.Close()
			return rd.Read(f)
		}
	}
	return nil, nil, errors.New(i18n.T("fileio.err.no_reader", ext(path)))
}

// Save writes d to path with the writer registered for its extension. The file is written to a
// temporary file in the same directory, synced and renamed over path, so a failed write leaves
// any existing file untouched.
func (r *Registry) Save(path string, d *doc.Drawing) ([]string, error) {
	for _, w := range r.writers {
		if slices.Contains(w.Extensions(), ext(path)) {
			return atomicWrite(path, func(f io.Writer) ([]string, error) { return w.Write(f, d) })
		}
	}
	if r.CanRead(path) {
		alt := ".dxf"
		if len(r.writers) > 0 && len(r.writers[0].Extensions()) > 0 {
			alt = r.writers[0].Extensions()[0]
		}
		return nil, errors.New(i18n.T("fileio.err.read_only", strings.ToUpper(strings.TrimPrefix(ext(path), ".")), alt))
	}
	return nil, errors.New(i18n.T("fileio.err.no_writer", ext(path)))
}

// CanRead reports whether a reader is registered for the extension of path.
func (r *Registry) CanRead(path string) bool {
	for _, rd := range r.readers {
		if slices.Contains(rd.Extensions(), ext(path)) {
			return true
		}
	}
	return false
}

// CanWrite reports whether a writer is registered for the extension of path.
func (r *Registry) CanWrite(path string) bool {
	for _, w := range r.writers {
		if slices.Contains(w.Extensions(), ext(path)) {
			return true
		}
	}
	return false
}

// WritablePath returns path when a writer handles its extension, and otherwise path with the
// extension of the first registered writer (".dxf" for the application), so a drawing opened from a
// read-only format such as DWG is offered a path it can be saved to.
func (r *Registry) WritablePath(path string) string {
	if r.CanWrite(path) || len(r.writers) == 0 || len(r.writers[0].Extensions()) == 0 {
		return path
	}
	return strings.TrimSuffix(path, filepath.Ext(path)) + r.writers[0].Extensions()[0]
}

func atomicWrite(path string, write func(io.Writer) ([]string, error)) (warns []string, err error) {
	mode := os.FileMode(0o644)
	if real, rerr := filepath.EvalSymlinks(path); rerr == nil {
		path = real
		if st, serr := os.Stat(path); serr == nil {
			if !st.Mode().IsRegular() {
				return nil, errors.New(i18n.T("fileio.err.not_regular", path))
			}
			mode = st.Mode().Perm()
		}
	}
	f, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".tmp*")
	if err != nil {
		return nil, err
	}
	tmp := f.Name()
	defer func() {
		if err != nil {
			f.Close()
			os.Remove(tmp)
		}
	}()
	if warns, err = write(f); err != nil {
		return warns, err
	}
	if err = f.Chmod(mode); err != nil {
		return warns, err
	}
	if err = f.Sync(); err != nil {
		return warns, err
	}
	if err = f.Close(); err != nil {
		return warns, err
	}
	if err = os.Rename(tmp, path); err != nil {
		return warns, err
	}
	return warns, nil
}
