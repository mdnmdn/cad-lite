package commands

import (
	"go-draw/cmdsys"
)

func undo(ctx *cmdsys.Context) {
	if !ctx.Doc.Undo() {
		ctx.Say("cmd.undo.msg.everything_undone")
	}
}

func redo(ctx *cmdsys.Context) {
	if !ctx.Doc.Redo() {
		ctx.Say("cmd.redo.msg.nothing_to_redo")
	}
}

// registerUndo adds UNDO and REDO.
func registerUndo(r *cmdsys.Registry) {
	r.Register("UNDO", func() cmdsys.Command { return &immediate{name: "UNDO", run: undo} })
	r.Register("REDO", func() cmdsys.Command { return &immediate{name: "REDO", run: redo} })
}
