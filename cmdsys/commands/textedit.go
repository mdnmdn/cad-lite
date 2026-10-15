package commands

import (
	"cad-lite/cmdsys"
	"cad-lite/doc"
)

type textEditCmd struct{ base }

func (*textEditCmd) Name() string    { return "TEXTEDIT" }
func (*textEditCmd) HumanOnly() bool { return true }

func (c *textEditCmd) Start(ctx *cmdsys.Context) *cmdsys.Prompt {
	if hs := ctx.Selection.Handles(); len(hs) == 1 && editableText(ctx.Doc, hs[0]) {
		return c.open(ctx, hs[0])
	}
	return c.prompt(ctx)
}

func (*textEditCmd) prompt(ctx *cmdsys.Context) *cmdsys.Prompt {
	return &cmdsys.Prompt{Message: ctx.T("cmd.textedit.prompt.pick"), Accepts: cmdsys.InputPoint | cmdsys.InputEnter, Pick: true}
}

func (c *textEditCmd) Input(ctx *cmdsys.Context, in cmdsys.Input) *cmdsys.Prompt {
	if in.Kind == cmdsys.InputEnter {
		return nil
	}
	h, ok := cmdsys.Pick(ctx.Doc, ctx.Index, in.Point, ctx.View.PixelSize()*8)
	if !ok || !editableText(ctx.Doc, h) {
		ctx.Say("cmd.textedit.msg.target")
		return c.prompt(ctx)
	}
	return c.open(ctx, h)
}

func (*textEditCmd) open(ctx *cmdsys.Context, h doc.Handle) *cmdsys.Prompt {
	if !ctx.OpenTextEditor(h) {
		ctx.Say("cmd.textedit.msg.unavailable")
	}
	return nil
}

func editableText(d *doc.Drawing, h doc.Handle) bool {
	e := d.Entity(h)
	if e == nil || !d.EntityEditable(e) || e.OwnerHandle() != doc.ModelSpace {
		return false
	}
	switch e.(type) {
	case *doc.Text, *doc.MText:
		return true
	}
	return false
}

func registerTextEdit(r *cmdsys.Registry) {
	r.Register("TEXTEDIT", func() cmdsys.Command { return &textEditCmd{} })
	r.Alias("DDEDIT", "TEXTEDIT")
	r.Alias("ED", "TEXTEDIT")
}
