// Package fileio is the file I/O plugin layer: format readers and writers
// register here and are chosen by file extension. It is named fileio, not
// io, so it never shadows the standard library package.
package fileio

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"go-draw/doc"
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
	return nil, nil, fmt.Errorf("no reader for %q files", ext(path))
}

// Save writes d to path with the writer registered for its extension.
func (r *Registry) Save(path string, d *doc.Drawing) ([]string, error) {
	for _, w := range r.writers {
		if slices.Contains(w.Extensions(), ext(path)) {
			f, err := os.Create(path)
			if err != nil {
				return nil, err
			}
			warns, err := w.Write(f, d)
			if cerr := f.Close(); err == nil {
				err = cerr
			}
			return warns, err
		}
	}
	return nil, fmt.Errorf("no writer for %q files", ext(path))
}
