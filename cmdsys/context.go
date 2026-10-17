package cmdsys

import (
	"fmt"
	"strings"

	"cad-lite/doc"
	"cad-lite/i18n"
	"cad-lite/render"
	"cad-lite/spatial"
)

// Context is what a command may touch: the document, its transaction, the
// view, the spatial index and the selection.
type Context struct {
	Doc       *doc.Drawing
	View      *render.View
	Index     *spatial.Index
	Selection *Selection
	// Files is how file commands reach the I/O layer; nil disables them.
	Files FileHandler

	agentCaller func() bool
	txn         *doc.Txn
	// commit commits txn the way the session does (Session.commitOpen).
	commit  func() doc.ChangeSet
	out     func(string)
	say     func(string, ...any)
	tr      func() i18n.Translator
	replace func(*doc.Drawing)
	setPath func(string)
	agent   func(string)
	// settings opens the host's settings panel; nil when the host has none.
	settings func()
	textEdit func(doc.Handle) bool

	// vars, last and prev are the session's system variables and its Last and Previous
	// memories (Vars, LastPoint, PreviousSelection).
	vars *Vars
	last *lastState
	prev *selMemory
	// script hands script text to the session (SCRIPT, RSCRIPT).
	script func(text string, restart bool)
	// files is the session's modified counter, quit hook and undo state (dirty.go).
	files *fileState
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

// Printf writes a line to the command history. The format is a finished sentence; user-facing
// text goes through Say or T so it can be translated.
func (c *Context) Printf(format string, args ...any) { c.out(fmt.Sprintf(format, args...)) }

// Translator returns the translator commands must use for user-facing text: the session's
// language, or English while an AI agent drives the session.
func (c *Context) Translator() i18n.Translator {
	if c.tr == nil {
		return i18n.For(i18n.Current())
	}
	return c.tr()
}

// T translates key in the session's language (fmt.Sprintf style arguments).
func (c *Context) T(key string, args ...any) string { return c.Translator().T(key, args...) }

// Plural translates the counted message key (key_one / key_other) with n as first argument.
func (c *Context) Plural(key string, n int, args ...any) string {
	return c.Translator().Plural(key, n, args...)
}

// Say writes the translated message key to the command history.
func (c *Context) Say(key string, args ...any) {
	if c.say != nil {
		c.say(key, args...)
		return
	}
	c.out(c.T(key, args...))
}

// SayPlural writes the translated counted message key to the command history.
func (c *Context) SayPlural(key string, n int, args ...any) { c.out(c.Plural(key, n, args...)) }

// Cmd returns the display name of command name for the session's language, upper-cased as it is
// typed ("LINEA"); the canonical name stays English.
func (c *Context) Cmd(name string) string {
	return strings.ToUpper(c.Translator().CommandName(name))
}

// ReplaceDrawing swaps the open document for d (OPEN and NEW).
func (c *Context) ReplaceDrawing(d *doc.Drawing) { c.replace(d) }

// SetFilePath records path as the drawing's file after a successful OPEN or SAVE.
func (c *Context) SetFilePath(path string) {
	if c.setPath != nil {
		c.setPath(path)
	}
}

// HasSettings reports whether the host installed a settings handler.
func (c *Context) HasSettings() bool { return c.settings != nil }

// OpenTextEditor asks the host to edit the model-space text h without changing it yet. It reports
// false when no editor is installed or the host refuses the target.
func (c *Context) OpenTextEditor(h doc.Handle) bool {
	return c.textEdit != nil && c.textEdit(h)
}

// OpenSettings asks the host to show (or toggle) its settings panel. It reports false when the
// host has none.
func (c *Context) OpenSettings() bool {
	if c.settings == nil {
		return false
	}
	c.settings()
	return true
}

// HasAgent reports whether the host installed an AI agent handler.
func (c *Context) HasAgent() bool { return c.agent != nil }

// Agent hands prompt to the host's AI agent; an empty prompt asks the host to toggle its agent
// panel. It reports false when the host has no agent.
func (c *Context) Agent(prompt string) bool {
	if c.agent == nil {
		return false
	}
	c.agent(prompt)
	return true
}

// AgentCaller reports whether an agent is supplying the current input.
func (c *Context) AgentCaller() bool { return c.agentCaller != nil && c.agentCaller() }
