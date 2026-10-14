package cmdsys

import "sort"

// Text stage orders. The command line text of Submit runs through the stages in ascending order;
// a package adds its stage at its own order, with AddTextFilter or AddTextHandler, from the
// session setup it owns, so no two packages edit one function.
const (
	// TextOrderNames is the command-name prefixes: ' _ . (names.go).
	TextOrderNames = 10
	// TextOrderSelection is the typed selection options W C WP CP F ALL L P ... (selection work).
	TextOrderSelection = 20
	// TextOrderSnap is typed object snaps (END, MID, ...) (drafting aids).
	TextOrderSnap = 30
	// TextOrderHash is the # and * point prefixes (drafting aids).
	TextOrderHash = 31
	// TextOrderVars is a system variable typed by name (SETVAR work).
	TextOrderVars = 90
)

// TextFilter rewrites a line of command-line text and returns the text the next stage sees. It
// must return "" for "" (Enter keeps meaning Enter) and may inspect the session (idle or at a
// prompt).
type TextFilter func(s *Session, text string) string

// TextHandler may consume a line: it returns true when it handled it, and Submit then does
// nothing more with it. A handler that does not recognise the text returns false and leaves
// everything unchanged.
type TextHandler func(s *Session, text string) bool

type textStage struct {
	order   int
	filter  TextFilter
	handler TextHandler
}

// AddTextFilter adds a rewriting stage at order. Stages with equal order run in the order they
// were added.
func (s *Session) AddTextFilter(order int, f TextFilter) {
	s.addStage(textStage{order: order, filter: f})
}

// AddTextHandler adds a consuming stage at order, with the same ordering as AddTextFilter.
func (s *Session) AddTextHandler(order int, f TextHandler) {
	s.addStage(textStage{order: order, handler: f})
}

func (s *Session) addStage(st textStage) {
	s.stages = append(s.stages, st)
	sort.SliceStable(s.stages, func(i, j int) bool { return s.stages[i].order < s.stages[j].order })
}

// preprocess runs text through the stages. It returns the rewritten text and whether a handler
// consumed the line.
func (s *Session) preprocess(text string) (string, bool) {
	for _, st := range s.stages {
		switch {
		case st.filter != nil:
			text = st.filter(s, text)
		case st.handler != nil:
			if st.handler(s, text) {
				return text, true
			}
		}
	}
	return text, false
}
