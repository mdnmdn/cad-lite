package commands

import (
	"strconv"
	"strings"

	"cad-lite/cmdsys"
	"cad-lite/doc"
)

// The ids of the keywords of CHPROP and the MATCHPROP settings; the catalog gives each its display
// text, shortcut and Italian name.
const (
	kwColor   = "Color"
	kwChLayer = "chLayer"
)

// colorText renders a colour index as the CECOLOR value: BYLAYER or the number.
func colorText(c int) string {
	if c < 1 || c > 255 {
		return "BYLAYER"
	}
	return strconv.Itoa(c)
}

func init() {
	cmdsys.RegisterVarGroup(func(s *cmdsys.Session, v *cmdsys.Vars) {
		// CECOLOR is the colour new objects get (doc.Drawing.CurrentColor, read by Txn.Add).
		v.Register(cmdsys.Var{Name: "CECOLOR", Kind: cmdsys.VarText, Default: "BYLAYER",
			Read: func() any { return colorText(s.Drawing().CurrentColor) },
			OnChange: func(_, n any) {
				if c, ok := ParseColor(n.(string)); ok {
					s.Drawing().CurrentColor = c % doc.ByLayer
				}
			}})
	})
}

// --- CHPROP -------------------------------------------------------------

type chpropStep int

const (
	cpSelect chpropStep = iota
	cpProperty
	cpColor
	cpLayer
)

type chpropCmd struct {
	base
	selecting
	step chpropStep
}

func (*chpropCmd) Name() string { return "CHPROP" }

func (c *chpropCmd) Start(ctx *cmdsys.Context) *cmdsys.Prompt {
	if c.begin(ctx) {
		return c.property(ctx)
	}
	return selectPrompt(ctx)
}

func (c *chpropCmd) property(ctx *cmdsys.Context) *cmdsys.Prompt {
	c.step = cpProperty
	return &cmdsys.Prompt{Message: ctx.T("cmd.chprop.prompt.property"), Accepts: cmdsys.InputKeyword | cmdsys.InputEnter, Keywords: []string{kwColor, kwChLayer}, NoOr: true}
}

func (c *chpropCmd) Input(ctx *cmdsys.Context, in cmdsys.Input) *cmdsys.Prompt {
	switch c.step {
	case cpSelect:
		if !c.take(ctx, in) {
			return nil
		}
		return c.property(ctx)
	case cpProperty:
		switch in.Keyword {
		case kwColor:
			c.step = cpColor
			def := cmdsys.Input{Kind: cmdsys.InputText, Text: "BYLAYER"}
			return &cmdsys.Prompt{Message: ctx.T("cmd.chprop.prompt.color"), Accepts: cmdsys.InputText | cmdsys.InputEnter, Default: &def}
		case kwChLayer:
			c.step = cpLayer
			cur := ctx.Doc.CurrentLayer
			if len(c.hs) > 0 {
				if e := ctx.Doc.Entity(c.hs[0]); e != nil {
					cur = e.Common().Layer
				}
			}
			def := cmdsys.Input{Kind: cmdsys.InputText, Text: cur}
			return &cmdsys.Prompt{Message: ctx.T("cmd.chprop.prompt.layer", cur), Accepts: cmdsys.InputText | cmdsys.InputEnter, Default: &def}
		}
		return nil
	case cpColor:
		color, ok := doc.ByLayer, true
		if in.Kind == cmdsys.InputText {
			color, ok = ParseColor(in.Text)
		}
		if ok {
			c.setAll(ctx, func(cm *doc.EntityCommon) { cm.Color = color })
		} else {
			ctx.Say("cmd.layer.msg.bad_color")
		}
	case cpLayer:
		if in.Kind == cmdsys.InputText {
			l := ctx.Doc.Layer(strings.TrimSpace(in.Text))
			if l == nil {
				ctx.Say("cmd.layer.msg.not_found", in.Text)
			} else {
				c.setAll(ctx, func(cm *doc.EntityCommon) { cm.Layer = l.Name })
			}
		}
	}
	return c.property(ctx)
}

// setAll applies fn to the common fields of every selected object that may be edited.
func (c *chpropCmd) setAll(ctx *cmdsys.Context, fn func(*doc.EntityCommon)) {
	for _, h := range c.hs {
		if e := ctx.Doc.Entity(h); e != nil && editable(ctx, e) {
			ctx.Tx().Modify(h, func(e doc.Entity) { fn(e.Common()) })
		}
	}
}

// --- -COLOR -------------------------------------------------------------

// colorCmd is -COLOR (and COLOR, which has no dialog here): it sets CECOLOR, the colour of new
// objects.
type colorCmd struct {
	base
	name string
}

func (c *colorCmd) Name() string { return c.name }

func (c *colorCmd) Start(ctx *cmdsys.Context) *cmdsys.Prompt {
	def := cmdsys.Input{Kind: cmdsys.InputText, Text: colorText(ctx.Doc.CurrentColor)}
	return &cmdsys.Prompt{Message: ctx.T("cmd.color.prompt.color"), Accepts: cmdsys.InputText | cmdsys.InputEnter, Default: &def}
}

func (c *colorCmd) Input(ctx *cmdsys.Context, in cmdsys.Input) *cmdsys.Prompt {
	if in.Kind != cmdsys.InputText {
		return nil
	}
	col, ok := ParseColor(in.Text)
	if !ok {
		ctx.Say("cmd.layer.msg.bad_color")
		return nil
	}
	ctx.Doc.CurrentColor = col % doc.ByLayer
	return nil
}

// --- LTSCALE ------------------------------------------------------------

type ltscaleCmd struct{ base }

func (*ltscaleCmd) Name() string { return "LTSCALE" }

func (*ltscaleCmd) Start(ctx *cmdsys.Context) *cmdsys.Prompt {
	def := cmdsys.Input{Kind: cmdsys.InputDistance, Value: ctx.Vars().Real("LTSCALE")}
	return &cmdsys.Prompt{Message: ctx.T("cmd.ltscale.prompt.factor"), Accepts: cmdsys.InputDistance, Default: &def}
}

func (*ltscaleCmd) Input(ctx *cmdsys.Context, in cmdsys.Input) *cmdsys.Prompt {
	if in.Value <= 0 {
		ctx.Say("cmd.ltscale.msg.positive")
		return nil
	}
	_ = ctx.Vars().Set("LTSCALE", in.Value)
	return nil
}

// registerProps adds CHPROP, -COLOR and LTSCALE. Entities carry no linetype or lineweight, so
// -LINETYPE, LWEIGHT and the LType and LWeight options of CHPROP wait for that model.
func registerProps(r *cmdsys.Registry) {
	r.Register("CHPROP", func() cmdsys.Command { return &chpropCmd{} })
	r.Register("COLOR", func() cmdsys.Command { return &colorCmd{name: "COLOR"} })
	r.Alias("COL", "COLOR")
	cmdsys.RegisterCLI(r, "COLOR", func() cmdsys.Command { return &colorCmd{name: "-COLOR"} }, "COL")
	r.Register("LTSCALE", func() cmdsys.Command { return &ltscaleCmd{} })
	r.Alias("LTS", "LTSCALE")
}
