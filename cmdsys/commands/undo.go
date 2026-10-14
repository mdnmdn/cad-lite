package commands

import (
	"strings"

	"cad-lite/cmdsys"
)

// undoSteps undoes up to n steps as one undo operation (REDO redoes them together) and says what
// it did: the command name for a single step, "Everything has been undone" when the history ran
// out. UNDO Control None turns it off and UNDO Control One limits it to one step. A group left
// open by UNDO Begin is closed first, so its steps are one.
func undoSteps(ctx *cmdsys.Context, n int) {
	h := ctx.History()
	switch h.Control {
	case cmdsys.UndoNone:
		ctx.Say("cmd.undo.msg.disabled")
		return
	case cmdsys.UndoOne:
		n = 1
	}
	if ctx.Doc.InGroup() {
		ctx.Doc.EndGroup()
	}
	name := ctx.Doc.UndoName()
	done := ctx.Doc.UndoN(n)
	h.Undone(ctx.Doc, done)
	switch {
	case done < n:
		ctx.Say("cmd.undo.msg.everything_undone")
	case n == 1 && name != "":
		ctx.Say("cmd.undo.msg.undone", strings.ToUpper(ctx.Translator().CommandName(name)))
	}
}

// undoBack undoes everything back to the latest UNDO Mark, as one undo operation.
func undoBack(ctx *cmdsys.Context) {
	if ctx.Doc.InGroup() {
		ctx.Doc.EndGroup()
	}
	ctx.History().Undone(ctx.Doc, ctx.Doc.UndoToMark())
}

func undo(ctx *cmdsys.Context) { undoSteps(ctx, 1) }

// redoSteps redoes up to n steps and records them against the undo batches.
func redoSteps(ctx *cmdsys.Context, n int) {
	if n < 1 {
		ctx.Say("cmd.redo.msg.nothing_to_redo")
		return
	}
	ctx.History().Redone(ctx.Doc.RedoN(n))
}

// redo reverses the latest undo operation: one U, one UNDO n, one UNDO Back.
func redo(ctx *cmdsys.Context) {
	redoSteps(ctx, ctx.History().Redoable(ctx.Doc))
}

// undoCmd is UNDO: a count, or Auto, Control, BEgin, End, Mark or Back.
type undoCmd struct {
	base
	stage int
}

const (
	undoMain = iota
	undoAuto
	undoControl
	undoBackAsk
)

func (*undoCmd) Name() string { return "UNDO" }

func (c *undoCmd) Start(ctx *cmdsys.Context) *cmdsys.Prompt {
	return &cmdsys.Prompt{Message: ctx.T("cmd.undo.prompt.count"), Accepts: cmdsys.InputInteger | cmdsys.InputKeyword,
		Keywords: []string{"Auto", "Control", "BEgin", "End", "Mark", "Back"},
		Default:  &cmdsys.Input{Kind: cmdsys.InputInteger, Value: 1}}
}

func (c *undoCmd) Input(ctx *cmdsys.Context, in cmdsys.Input) *cmdsys.Prompt {
	switch c.stage {
	case undoAuto:
		ctx.History().Auto = in.Keyword == "ON"
		return nil
	case undoControl:
		switch in.Keyword {
		case "All":
			ctx.History().Control = cmdsys.UndoAll
		case "None":
			ctx.History().Control = cmdsys.UndoNone
		case "One":
			ctx.History().Control = cmdsys.UndoOne
		}
		return nil
	case undoBackAsk:
		if in.Keyword == "Yes" {
			undoSteps(ctx, ctx.Doc.UndoDepth())
		}
		return nil
	}
	if in.Kind == cmdsys.InputInteger {
		if in.Value < 1 {
			ctx.Say("cmd.undo.msg.positive")
			return c.Start(ctx)
		}
		undoSteps(ctx, int(in.Value))
		return nil
	}
	switch in.Keyword {
	case "Auto":
		c.stage = undoAuto
		return &cmdsys.Prompt{Message: ctx.T("cmd.undo.prompt.auto"), Accepts: cmdsys.InputKeyword, NoOr: true,
			Keywords: []string{"ON", "OFF"}, Default: &cmdsys.Input{Kind: cmdsys.InputKeyword, Keyword: onOff(ctx.History().Auto)}}
	case "Control":
		c.stage = undoControl
		return &cmdsys.Prompt{Message: ctx.T("cmd.undo.prompt.control"), Accepts: cmdsys.InputKeyword, NoOr: true,
			Keywords: []string{"All", "None", "One", "Combine", "Layer"}, Default: &cmdsys.Input{Kind: cmdsys.InputKeyword, Keyword: "All"}}
	case "BEgin":
		ctx.Doc.BeginGroup()
	case "End":
		if !ctx.Doc.InGroup() {
			ctx.Say("cmd.undo.msg.no_group")
			return nil
		}
		ctx.Doc.EndGroup()
	case "Mark":
		ctx.Doc.Mark()
	case "Back":
		if ctx.Doc.HasMark() {
			undoBack(ctx)
			return nil
		}
		c.stage = undoBackAsk
		return &cmdsys.Prompt{Message: ctx.T("cmd.undo.prompt.back_all"), Accepts: cmdsys.InputKeyword, NoOr: true,
			Keywords: []string{"Yes", "No"}, Default: &cmdsys.Input{Kind: cmdsys.InputKeyword, Keyword: "Yes"}}
	}
	return nil
}

func onOff(on bool) string {
	if on {
		return "ON"
	}
	return "OFF"
}

// mredoCmd is MREDO: redo a number of steps, All of them, or the Last undo.
type mredoCmd struct{ base }

func (*mredoCmd) Name() string { return "MREDO" }

func (c *mredoCmd) Start(ctx *cmdsys.Context) *cmdsys.Prompt {
	return &cmdsys.Prompt{Message: ctx.T("cmd.mredo.prompt.count"), Accepts: cmdsys.InputInteger | cmdsys.InputKeyword,
		Keywords: []string{"All", "Last"}}
}

func (c *mredoCmd) Input(ctx *cmdsys.Context, in cmdsys.Input) *cmdsys.Prompt {
	switch {
	case in.Kind == cmdsys.InputInteger && in.Value < 1:
		ctx.Say("cmd.undo.msg.positive")
		return c.Start(ctx)
	case in.Kind == cmdsys.InputInteger:
		redoSteps(ctx, int(in.Value))
	case in.Keyword == "All":
		redoSteps(ctx, ctx.Doc.RedoDepth())
	default:
		redo(ctx)
	}
	return nil
}

// registerUndo adds U, UNDO, REDO and MREDO.
func registerUndo(r *cmdsys.Registry) {
	r.Register("U", func() cmdsys.Command { return &immediate{name: "U", run: undo} })
	r.Register("UNDO", func() cmdsys.Command { return &undoCmd{} })
	r.Register("REDO", func() cmdsys.Command { return &immediate{name: "REDO", run: redo} })
	r.Register("MREDO", func() cmdsys.Command { return &mredoCmd{} })
}
