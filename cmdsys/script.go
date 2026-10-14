package cmdsys

import "strings"

// StripScriptComments removes AutoCAD script comments from text: a ";" starts a comment that
// runs to the end of its line. The text before it is kept (without the spaces that preceded the
// ";"); a line that holds only a comment disappears, while an empty line stays, because in a
// script it is an Enter.
func StripScriptComments(text string) string {
	if !strings.Contains(text, ";") {
		return text
	}
	lines := strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n")
	out := lines[:0]
	for _, l := range lines {
		if i := strings.Index(l, ";"); i >= 0 {
			l = strings.TrimRight(l[:i], " \t")
			if l == "" {
				continue
			}
		}
		out = append(out, l)
	}
	return strings.Join(out, "\n")
}

// Script runs AutoCAD script text: ";" comments are dropped (StripScriptComments), then tokens
// separated by single spaces or newlines are each submitted in turn; an empty token (two
// separators in a row) is Enter. A final newline is still a separator, so it sends one more
// Enter (a script file's last newline is a terminator in AutoCAD; that rule, RSCRIPT, RESUME and
// DELAY belong to the SCRIPT work).
func (s *Session) Script(text string) {
	text = strings.ReplaceAll(strings.ReplaceAll(StripScriptComments(text), "\r\n", "\n"), "\n", " ")
	for _, tok := range strings.Split(text, " ") {
		s.Submit(tok)
	}
}
