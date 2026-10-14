// Package cmdsys is the command framework: AutoCAD-style commands as prompt
// state machines, typed-input parsing, object snaps, ortho and selection.
// It drives the document through transactions and produces previews as
// display lists; it never imports a UI or graphics package.
package cmdsys

import (
	"fmt"
	"slices"
	"sort"
	"strings"

	"go-draw/doc"
	"go-draw/geom"
	"go-draw/render"
	"go-draw/spatial"
)

// Command is one AutoCAD-style command. The session calls Start once, then
// Input with each response until Input returns nil.
type Command interface {
	// Name returns the command's canonical upper-case name.
	Name() string
	// Start returns the first prompt, or nil when the command completes
	// immediately.
	Start(ctx *Context) *Prompt
	// Input consumes one response and returns the next prompt, or nil when
	// the command is done.
	Input(ctx *Context, in Input) *Prompt
	// Preview appends rubber-band geometry for the cursor position.
	Preview(ctx *Context, cursor geom.Vec2, dl *render.DisplayList)
	// Cancel is called when the user cancels (Escape).
	Cancel(ctx *Context)
}

// Context is what a command may touch: the document, its transaction, the
// view, the spatial index and the selection.
type Context struct {
	Doc       *doc.Drawing
	View      *render.View
	Index     *spatial.Index
	Selection *Selection
	// Files is how file commands reach the I/O layer; nil disables them.
	Files FileHandler

	txn     *doc.Txn
	out     func(string)
	replace func(*doc.Drawing)
}

// FileHandler reads and writes drawings for the file commands.
type FileHandler interface {
	Open(path string) (*doc.Drawing, []string, error)
	Save(path string, d *doc.Drawing) ([]string, error)
}

// Tx returns the command's transaction, opening it on first use. The
// session commits it when the command ends, so a command is one undo step.
func (c *Context) Tx() *doc.Txn {
	if c.txn == nil {
		t, err := c.Doc.Begin()
		if err != nil {
			panic(err)
		}
		c.txn = t
	}
	return c.txn
}

// Printf writes a line to the command history.
func (c *Context) Printf(format string, args ...any) { c.out(fmt.Sprintf(format, args...)) }

// ReplaceDrawing swaps the open document for d (OPEN and NEW).
func (c *Context) ReplaceDrawing(d *doc.Drawing) { c.replace(d) }

// Registry maps command names and aliases to command factories.
type Registry struct {
	factories map[string]func() Command
	aliases   map[string]string
}

// NewRegistry returns an empty registry.
func NewRegistry() *Registry {
	return &Registry{factories: map[string]func() Command{}, aliases: map[string]string{}}
}

// Register adds a command under its upper-case name.
func (r *Registry) Register(name string, f func() Command) {
	r.factories[strings.ToUpper(name)] = f
}

// Alias makes alias run name (e.g. "L" runs "LINE").
func (r *Registry) Alias(alias, name string) {
	r.aliases[strings.ToUpper(alias)] = strings.ToUpper(name)
}

// LoadAliases reads "ALIAS,*COMMAND" lines in the acad.pgp format; other
// lines and ";" comments are ignored.
func (r *Registry) LoadAliases(pgp string) {
	for _, line := range strings.Split(pgp, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, ";") {
			continue
		}
		a, n, ok := strings.Cut(line, ",")
		if !ok {
			continue
		}
		r.Alias(strings.TrimSpace(a), strings.TrimPrefix(strings.TrimSpace(n), "*"))
	}
}

// Lookup returns a new instance of the command name or alias.
func (r *Registry) Lookup(name string) (Command, bool) {
	n := strings.ToUpper(strings.TrimSpace(name))
	if a, ok := r.aliases[n]; ok {
		n = a
	}
	f, ok := r.factories[n]
	if !ok {
		return nil, false
	}
	return f(), true
}

// Names returns every command name, sorted.
func (r *Registry) Names() []string {
	out := make([]string, 0, len(r.factories))
	for n := range r.factories {
		out = append(out, n)
	}
	sort.Strings(out)
	return out
}

// Complete returns the command names and aliases starting with prefix.
func (r *Registry) Complete(prefix string) []string {
	p := strings.ToUpper(prefix)
	var out []string
	for _, n := range r.Names() {
		if strings.HasPrefix(n, p) {
			out = append(out, n)
		}
	}
	return out
}

// Selection is the session's selection set: not part of the document, not
// persisted and not undoable.
type Selection struct {
	hs []doc.Handle
}

// Handles returns the selected handles in selection order.
func (s *Selection) Handles() []doc.Handle { return s.hs }

// Len returns the number of selected entities.
func (s *Selection) Len() int { return len(s.hs) }

// Has reports whether h is selected.
func (s *Selection) Has(h doc.Handle) bool { return slices.Contains(s.hs, h) }

// Add selects hs, skipping duplicates.
func (s *Selection) Add(hs ...doc.Handle) {
	for _, h := range hs {
		if !s.Has(h) {
			s.hs = append(s.hs, h)
		}
	}
}

// Remove deselects hs.
func (s *Selection) Remove(hs ...doc.Handle) {
	s.hs = slices.DeleteFunc(s.hs, func(h doc.Handle) bool { return slices.Contains(hs, h) })
}

// Clear deselects everything.
func (s *Selection) Clear() { s.hs = s.hs[:0] }

// Prune drops handles that no longer exist in d.
func (s *Selection) Prune(d *doc.Drawing) {
	s.hs = slices.DeleteFunc(s.hs, func(h doc.Handle) bool { return d.Entity(h) == nil })
}
