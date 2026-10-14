// Package cmdsys is the command framework: AutoCAD-style commands as prompt
// state machines, typed-input parsing, object snaps, ortho and selection.
// It drives the document through transactions and produces previews as
// display lists; it never imports a UI or graphics package.
package cmdsys

import (
	"cad-lite/geom"
	"cad-lite/render"
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

// HumanOnly is implemented by commands an AI agent may not run: the ones that read or write
// files or discard the drawing (NEW, OPEN, SAVE, and any future QUIT or SAVEAS). The session
// refuses them while Session.SetAgentCaller is on, however they were named.
type HumanOnly interface {
	// HumanOnly reports true for a command agents must not run.
	HumanOnly() bool
}
