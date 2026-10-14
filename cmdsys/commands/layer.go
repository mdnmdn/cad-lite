package commands

import (
	"strconv"
	"strings"

	"go-draw/cmdsys"
)

// --- LAYER --------------------------------------------------------------

type layerCmd struct{ base }

func (*layerCmd) Name() string { return "LAYER" }

func (*layerCmd) Start(ctx *cmdsys.Context) *cmdsys.Prompt {
	return &cmdsys.Prompt{Message: ctx.T("cmd.layer.prompt.name", ctx.Doc.CurrentLayer), Accepts: cmdsys.InputText | cmdsys.InputEnter}
}

func (*layerCmd) Input(ctx *cmdsys.Context, in cmdsys.Input) *cmdsys.Prompt {
	if in.Kind == cmdsys.InputEnter {
		return nil
	}
	name, col, hasColor := strings.Cut(in.Text, "=")
	name = strings.TrimSpace(name)
	color := 7
	if hasColor {
		n, err := strconv.Atoi(strings.TrimSpace(col))
		if err != nil || n < 1 || n > 255 {
			ctx.Say("cmd.layer.msg.bad_color")
			return nil
		}
		color = n
	}
	l := ctx.Doc.Layer(name)
	if l == nil {
		l = ctx.Doc.AddLayer(name, color)
		ctx.Say("cmd.layer.msg.created", name)
	} else if hasColor {
		l.Color = color
	}
	ctx.Doc.CurrentLayer = l.Name
	return nil
}

// Transparent marks LAYER as runnable inside another command (cmdsys.Transparent).
func (*layerCmd) Transparent() bool { return true }

// registerLayer adds LAYER.
func registerLayer(r *cmdsys.Registry) {
	r.Register("LAYER", func() cmdsys.Command { return &layerCmd{} })
}
