package commands

import (
	"go-draw/cmdsys"
	"go-draw/doc"
)

// --- NEW, OPEN, SAVE ----------------------------------------------------

type fileCmd struct {
	base
	name string
}

func (c *fileCmd) Name() string { return c.name }

// HumanOnly marks OPEN and SAVE as unavailable to agents.
func (c *fileCmd) HumanOnly() bool { return true }

func (c *fileCmd) Start(ctx *cmdsys.Context) *cmdsys.Prompt {
	if ctx.Files == nil {
		ctx.Say("cmd.file.msg.unavailable")
		return nil
	}
	key := "cmd.file.prompt.open_path"
	if c.name == "SAVE" {
		key = "cmd.file.prompt.save_path"
	}
	return &cmdsys.Prompt{Message: ctx.T(key), Accepts: cmdsys.InputText}
}

func (c *fileCmd) Input(ctx *cmdsys.Context, in cmdsys.Input) *cmdsys.Prompt {
	if c.name == "SAVE" {
		warns, err := ctx.Files.Save(in.Text, ctx.Doc)
		if err != nil {
			ctx.Say("cmd.file.msg.save_failed", err)
			return nil
		}
		printWarnings(ctx, warns)
		ctx.SetFilePath(in.Text)
		ctx.Say("cmd.file.msg.saved", in.Text, ctx.Doc.Len())
		return nil
	}
	d, warns, err := ctx.Files.Open(in.Text)
	if err != nil {
		ctx.Say("cmd.file.msg.open_failed", err)
		return nil
	}
	printWarnings(ctx, warns)
	ctx.ReplaceDrawing(d)
	ctx.SetFilePath(in.Text)
	ZoomExtents(d, ctx.View)
	ctx.Say("cmd.file.msg.opened", in.Text, d.Len())
	return nil
}

func printWarnings(ctx *cmdsys.Context, warns []string) {
	const shown = 5
	for i, w := range warns {
		if i == shown {
			ctx.Say("cmd.file.msg.more_warnings", len(warns)-shown)
			break
		}
		ctx.Say("cmd.file.msg.warning", w)
	}
}

// registerFiles adds NEW, OPEN and SAVE.
func registerFiles(r *cmdsys.Registry) {
	r.Register("NEW", func() cmdsys.Command {
		return &immediate{name: "NEW", humanOnly: true, run: func(ctx *cmdsys.Context) { ctx.ReplaceDrawing(doc.New()) }}
	})
	r.Register("OPEN", func() cmdsys.Command { return &fileCmd{name: "OPEN"} })
	r.Register("SAVE", func() cmdsys.Command { return &fileCmd{name: "SAVE"} })
}
