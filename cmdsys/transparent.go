package cmdsys

import (
	"strings"

	"go-draw/doc"
)

// Transparent is implemented by commands that may run inside another command, typed with a
// leading ' at a prompt (ZOOM, PAN, LAYER, DIST, ID, SETVAR and the other view and settings
// commands). A transparent command must not open a transaction: whatever it commits is its own
// step, and the interrupted command keeps its transaction.
type Transparent interface {
	// Transparent reports true for a command that may run at another command's prompt.
	Transparent() bool
}

// IsTransparent reports whether c may run transparently.
func IsTransparent(c Command) bool {
	t, ok := c.(Transparent)
	return ok && t.Transparent()
}

// transparentMark is the prompt prefix of one level of nesting: ">>" for the first transparent
// command, ">>>>" for one run inside it.
const transparentMark = ">>"

// frame is an interrupted command: what the session needs to put back when the transparent
// command above it ends.
type frame struct {
	cmd    Command
	prompt *Prompt
	txn    *doc.Txn
}

// TransparentDepth returns how many transparent commands are running over the interrupted
// ones: 0 for an ordinary command or at Command:.
func (s *Session) TransparentDepth() int { return len(s.cl.frames) }

// transparentStage is the order-15 text handler. A line starting with ' at a prompt runs the
// named command over it: the interrupted command and its prompt are parked in a frame, the
// transparent command runs (prompt prefixed with >>), and when it ends finish restores the
// frame, so the interrupted prompt comes back unchanged. A text-entry prompt takes ' as text, a
// command that is not transparent is refused, and the prompt stays either way. Only at
// Command: the ' is just stripped (names.go).
func transparentStage(s *Session, text string) bool {
	t := strings.TrimSpace(text)
	if s.active == nil || s.prompt == nil || len(t) < 2 || t[0] != '\'' {
		return false
	}
	if s.prompt.Accepts&InputText != 0 {
		return false
	}
	name := StripCommandPrefixes(t)
	cmd, ok := s.lookupCommand(name)
	tr := s.Translator()
	s.print(s.Prompt() + " " + t)
	switch {
	case !ok:
		s.print(tr.T(MsgUnknownCmd, strings.ToUpper(name)))
		return true
	case s.agentCaller && humanOnly(cmd):
		s.print(tr.T("session.agent_forbidden", cmd.Name()))
		return true
	case !IsTransparent(cmd):
		s.print(tr.T(MsgNotTransparent, strings.ToUpper(tr.CommandName(cmd.Name()))))
		return true
	}
	depth := len(s.cl.frames)
	s.cl.frames = append(s.cl.frames, frame{cmd: s.active, prompt: s.prompt, txn: s.ctx.txn})
	s.active, s.prompt, s.ctx.txn = nil, nil, nil
	last := s.last
	s.Run(name)
	s.last = last
	if s.active == nil && len(s.cl.frames) > depth {
		s.resumeFrame()
	}
	return true
}

// resumeFrame puts the innermost interrupted command back when nothing is running.
func (s *Session) resumeFrame() {
	n := len(s.cl.frames)
	if n == 0 || s.active != nil {
		return
	}
	f := s.cl.frames[n-1]
	s.cl.frames = s.cl.frames[:n-1]
	s.active, s.prompt, s.ctx.txn = f.cmd, f.prompt, f.txn
}

// unwindFrames is Escape under transparent commands: AutoCAD cancels the whole stack, so each
// transparent command is cancelled and its interrupted command restored in turn; the caller then
// cancels the outermost as usual.
func (s *Session) unwindFrames() {
	for len(s.cl.frames) > 0 {
		if s.active != nil {
			s.guard(func() { s.active.Cancel(&s.ctx) })
		}
		if s.ctx.txn != nil {
			s.ctx.txn.Commit()
			s.ctx.txn = nil
		}
		s.active, s.prompt = nil, nil
		s.resumeFrame()
	}
}
