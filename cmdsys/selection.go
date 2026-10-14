package cmdsys

import (
	"slices"

	"go-draw/doc"
)

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
