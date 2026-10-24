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
	// Write serializes d to w, in the version the writer chooses for it.
	Write(w io.Writer, d *doc.Drawing) ([]string, error)
}

// VersionWriter is a Writer that writes a version the caller chooses (SAVEAS's file format).
type VersionWriter interface {
	Writer
	// WriteVersion serializes d to w in version, one of Versions(); "" is Write's own choice.
	// Content the version cannot hold is degraded or left out, each loss reported as a warning.
	WriteVersion(w io.Writer, d *doc.Drawing, version string) ([]string, error)
	// AutoVersion returns the version Write chooses for d.
	AutoVersion(d *doc.Drawing) string
}

// SaveOptions are a save's choices beyond the path.
type SaveOptions struct {
	// Version is the format version to write ("AC1009", "AC1015"); a version the writer does not
	// write is mapped to the nearest one it does (NearestVersion), and "" lets the writer choose.
	Version string
}

// NearestVersion maps version to one of writable (versions named "AC1009", "AC1015", ..., which
// sort in release order): version itself when it is writable, else the oldest writable version
// newer than it, which can hold everything it could, else the newest writable one. "" stays "",
// and so does any version when writable is empty.
func NearestVersion(version string, writable []string) string {
	if version == "" || len(writable) == 0 {
		return ""
	}
	vs := slices.Clone(writable)
	slices.Sort(vs)
	for _, v := range vs {
		if v >= version {
			return v
		}
	}
	return vs[len(vs)-1]
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

// Save writes d to path with the writer registered for its extension, in the drawing's own
// version (Header.Version, mapped by NearestVersion; "" lets the writer choose). The file is written
// to a temporary file in the same directory, synced and renamed over path, so a failed write leaves
// any existing file untouched.
func (r *Registry) Save(path string, d *doc.Drawing) ([]string, error) {
	return r.SaveWith(path, d, SaveOptions{Version: d.Header.Version})
}

// SaveWith is Save with explicit options: opt.Version replaces the drawing's own version. A writer
// that is not a VersionWriter writes its one version whatever opt says.
func (r *Registry) SaveWith(path string, d *doc.Drawing, opt SaveOptions) ([]string, error) {
	if w := r.writer(path); w != nil {
		v := NearestVersion(opt.Version, w.Versions())
		vw, ok := w.(VersionWriter)
		return atomicWrite(path, func(f io.Writer) ([]string, error) {
			if ok {
				return vw.WriteVersion(f, d, v)
			}
			return w.Write(f, d)
		})
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

// writer returns the writer registered for the extension of path, or nil.
func (r *Registry) writer(path string) Writer {
	for _, w := range r.writers {
		if slices.Contains(w.Extensions(), ext(path)) {
			return w
		}
	}
	return nil
}

// SaveVersions returns the versions the writer for path writes, oldest first; for a path no writer
// handles (an untitled drawing's "", a .dwg), those of the first registered writer, which
// WritablePath would save it with.
func (r *Registry) SaveVersions(path string) []string {
	w := r.writer(path)
	if w == nil && len(r.writers) > 0 {
		w = r.writers[0]
	}
	if w == nil {
		return nil
	}
	vs := slices.Clone(w.Versions())
	slices.Sort(vs)
	return vs
}

// SaveVersion returns the version saving d to path with version would write: version mapped by
// NearestVersion, or, for "", the writer's own choice for d. Like SaveVersions it falls back to the
// first writer, and it is "" when the writer cannot say.
func (r *Registry) SaveVersion(path string, d *doc.Drawing, version string) string {
	w := r.writer(path)
	if w == nil && len(r.writers) > 0 {
		w = r.writers[0]
	}
	if w == nil {
		return ""
	}
	if v := NearestVersion(version, w.Versions()); v != "" {
		return v
	}
	if vw, ok := w.(VersionWriter); ok {
		return vw.AutoVersion(d)
	}
	if vs := w.Versions(); len(vs) == 1 {
		return vs[0]
	}
	return ""
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
