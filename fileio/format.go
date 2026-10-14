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

	"go-draw/doc"
	"go-draw/i18n"
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
	return nil, errors.New(i18n.T("fileio.err.no_writer", ext(path)))
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
