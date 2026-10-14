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

// maxScriptRestarts bounds consecutive RSCRIPT restarts: AutoCAD loops until the user presses
// Esc, which a synchronous Script cannot see, so the loop ends after this many.
const maxScriptRestarts = 1000

// scriptTokens splits script text into the responses it feeds: comments are dropped, one final
// newline is the file's terminator rather than a separator (a file that ends with a blank line
// sends one Enter, one that ends after the last answer sends none), tabs count as spaces, and
// every other space or newline separates two responses, an empty one being Enter.
func scriptTokens(text string) []string {
	text = StripScriptComments(strings.ReplaceAll(text, "\r\n", "\n"))
	text = strings.TrimSuffix(text, "\n")
	text = strings.NewReplacer("\t", " ", "\n", " ").Replace(text)
	return strings.Split(text, " ")
}

// Script runs AutoCAD script text (scriptTokens): each token is submitted in turn, as typed. It
// does not stop at an error, as AutoCAD does not. A SCRIPT command run by the text starts the
// named file and the rest of the calling text is dropped (the nested script runs to its end and
// the outer one does not resume); RSCRIPT restarts the text that is running.
func (s *Session) Script(text string) {
	s.cl.pending = append(s.cl.pending, text)
	s.pumpScripts()
}

// queueScript is Context.RunScript: it queues text, or the running script again when restart is
// set, to run once the command that asked has ended.
func (s *Session) queueScript(text string, restart bool) {
	if restart {
		if s.cl.cur == "" || s.cl.restarts >= maxScriptRestarts {
			return
		}
		s.cl.restarts++
		text = s.cl.cur
	}
	s.cl.pending = append(s.cl.pending, text)
}

func (s *Session) pumpScripts() {
	if s.cl.pumping {
		return
	}
	s.cl.pumping = true
	defer func() { s.cl.pumping, s.cl.cur, s.cl.restarts = false, "", 0 }()
	for len(s.cl.pending) > 0 {
		text := s.cl.pending[0]
		s.cl.pending = s.cl.pending[1:]
		s.cl.cur = text
		for _, tok := range scriptTokens(text) {
			s.Submit(tok)
			if len(s.cl.pending) > 0 {
				break
			}
		}
	}
}

// RunScript runs script text after the current command ends (SCRIPT calls it with the file's
// text); a script that is already running is abandoned in its favour.
func (c *Context) RunScript(text string) {
	if c.script != nil {
		c.script(text, false)
	}
}

// RestartScript runs the script that is running again from its first line (RSCRIPT).
func (c *Context) RestartScript() {
	if c.script != nil {
		c.script("", true)
	}
}
