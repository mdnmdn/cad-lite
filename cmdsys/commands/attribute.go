package commands

import (
	"cad-lite/cmdsys"
	"cad-lite/doc"
	"cad-lite/geom"
	"errors"
	"strings"
)

func registerAttribute(r *cmdsys.Registry) {
	r.Register("ATTDEF", func() cmdsys.Command { return &attributeCmd{} })
	r.Register("ATTEDIT", func() cmdsys.Command { return &attributeCmd{edit: true} })
}

type attributeCmd struct {
	base
	edit               bool
	stage              int
	tag, prompt, value string
	point              geom.Vec2
	h                  doc.Handle
}

func (c *attributeCmd) Name() string {
	if c.edit {
		return "ATTEDIT"
	}
	return "ATTDEF"
}
func (c *attributeCmd) Start(ctx *cmdsys.Context) *cmdsys.Prompt {
	if c.edit {
		return &cmdsys.Prompt{Message: ctx.T("cmd.symbol.prompt.pick_insert"), Accepts: cmdsys.InputPoint, Pick: true}
	}
	return symbolPrompt(ctx, "tag", cmdsys.InputText)
}
func (c *attributeCmd) Input(ctx *cmdsys.Context, in cmdsys.Input) *cmdsys.Prompt {
	if c.edit {
		switch c.stage {
		case 0:
			h, ok := cmdsys.Pick(ctx.Doc, ctx.Index, in.Point, ctx.View.PixelSize()*8)
			if !ok {
				return symbolError(ctx, errors.New("no block reference selected"))
			}
			if _, ok := ctx.Doc.Entity(h).(*doc.Insert); !ok {
				return symbolError(ctx, errors.New("object is not a block reference"))
			}
			c.h = h
			c.stage = 1
			return symbolPrompt(ctx, "tag", cmdsys.InputText)
		case 1:
			c.tag = in.Text
			c.stage = 2
			return symbolPrompt(ctx, "value", cmdsys.InputText|cmdsys.InputEnter)
		case 2:
			reference := ctx.Doc.Entity(c.h).(*doc.Insert)
			copy := reference.Clone().(*doc.Insert)
			if !copy.SetAttribute(c.tag, in.Text) {
				return symbolError(ctx, errors.New("editable attribute tag does not exist"))
			}
			ctx.Tx().Modify(c.h, func(e doc.Entity) { e.(*doc.Insert).SetAttribute(c.tag, in.Text) })
			return nil
		}
	}
	switch c.stage {
	case 0:
		c.tag = strings.TrimSpace(in.Text)
		if c.tag == "" || strings.ContainsAny(c.tag, " \t\n") {
			return symbolError(ctx, errors.New("attribute tag must be a nonempty word"))
		}
		c.stage = 1
		return symbolPrompt(ctx, "attribute_prompt", cmdsys.InputText|cmdsys.InputEnter)
	case 1:
		c.prompt = in.Text
		c.stage = 2
		return symbolPrompt(ctx, "default", cmdsys.InputText|cmdsys.InputEnter)
	case 2:
		c.value = in.Text
		c.stage = 3
		return symbolPrompt(ctx, "insert_point", cmdsys.InputPoint)
	case 3:
		c.point = in.Point
		c.stage = 4
		p := symbolPrompt(ctx, "height", cmdsys.InputDistance)
		p.Default = &cmdsys.Input{Kind: cmdsys.InputDistance, Value: 2.5}
		return p
	case 4:
		if in.Value <= 0 || in.Value > doc.TextMaxSize {
			return symbolError(ctx, errors.New("invalid attribute height"))
		}
		ctx.Tx().Add(&doc.AttributeDefinition{Text: *doc.NewText(c.point, in.Value, 0, c.value), Tag: c.tag, Prompt: c.prompt})
		return nil
	}
	return nil
}
