package commands

import (
	"strings"

	"go-draw/cmdsys"
	"go-draw/doc"
)

// MATCHPROP settings bits (the MATCHPROPSET variable). Entities carry a colour and a layer only,
// so those are the settings there are to switch.
const (
	mpColor = 1 << iota
	mpLayer
)

const mpAll = mpColor | mpLayer

func init() {
	cmdsys.RegisterVarGroup(func(_ *cmdsys.Session, v *cmdsys.Vars) {
		// A go-draw variable standing in for the Property Settings dialog of MATCHPROP: the sum
		// of 1 (Color) and 2 (Layer) that MATCHPROP copies.
		v.Register(cmdsys.Var{Name: "MATCHPROPSET", Kind: cmdsys.VarInt, Default: mpAll, Min: 0, Max: mpAll})
	})
}

type matchStep int

const (
	msSource matchStep = iota
	msDest
	msSettings
)

type matchpropCmd struct {
	base
	step  matchStep
	color int
	layer string
}

func (*matchpropCmd) Name() string { return "MATCHPROP" }

func (c *matchpropCmd) Start(ctx *cmdsys.Context) *cmdsys.Prompt {
	return &cmdsys.Prompt{Message: ctx.T("cmd.matchprop.prompt.source"), Accepts: cmdsys.InputPoint}
}

// settingsText names the properties the current settings copy.
func mpSettingsText(ctx *cmdsys.Context) string {
	set := ctx.Vars().Int("MATCHPROPSET")
	var parts []string
	if set&mpColor != 0 {
		parts = append(parts, ctx.T("cmd.matchprop.setting.color"))
	}
	if set&mpLayer != 0 {
		parts = append(parts, ctx.T("cmd.matchprop.setting.layer"))
	}
	return strings.Join(parts, " ")
}

func (c *matchpropCmd) destPrompt(ctx *cmdsys.Context) *cmdsys.Prompt {
	c.step = msDest
	ctx.Selection.Clear()
	return &cmdsys.Prompt{Message: ctx.T("cmd.matchprop.prompt.dest"), Accepts: cmdsys.InputSelection | cmdsys.InputKeyword, Keywords: []string{"Settings"}}
}

func (c *matchpropCmd) Input(ctx *cmdsys.Context, in cmdsys.Input) *cmdsys.Prompt {
	switch c.step {
	case msSource:
		_, e := pickAny(ctx, in.Point)
		if e == nil {
			ctx.Say("cmd.matchprop.msg.no_object")
			return c.Start(ctx)
		}
		c.color, c.layer = e.Common().Color, e.Common().Layer
		ctx.Say("cmd.matchprop.msg.settings", mpSettingsText(ctx))
		return c.destPrompt(ctx)
	case msDest:
		if in.Kind == cmdsys.InputKeyword {
			c.step = msSettings
			return c.settingsPrompt(ctx)
		}
		c.apply(ctx, in.Selection)
		ctx.Selection.Clear()
		return nil
	case msSettings:
		if in.Kind == cmdsys.InputKeyword {
			bit := mpColor
			if in.Keyword == kwChLayer {
				bit = mpLayer
			}
			_ = ctx.Vars().Set("MATCHPROPSET", ctx.Vars().Int("MATCHPROPSET")^bit)
			ctx.Say("cmd.matchprop.msg.settings", mpSettingsText(ctx))
			return c.settingsPrompt(ctx)
		}
		return c.destPrompt(ctx)
	}
	return nil
}

func (c *matchpropCmd) settingsPrompt(ctx *cmdsys.Context) *cmdsys.Prompt {
	return &cmdsys.Prompt{Message: ctx.T("cmd.matchprop.prompt.settings"), Accepts: cmdsys.InputKeyword | cmdsys.InputEnter, Keywords: []string{kwColor, kwChLayer}, NoOr: true}
}

// apply copies the settings of the source object onto hs; objects on a locked layer are skipped.
func (c *matchpropCmd) apply(ctx *cmdsys.Context, hs []doc.Handle) {
	set := ctx.Vars().Int("MATCHPROPSET")
	locked := 0
	for _, h := range hs {
		e := ctx.Doc.Entity(h)
		if e == nil {
			continue
		}
		if !editable(ctx, e) {
			locked++
			continue
		}
		ctx.Tx().Modify(h, func(e doc.Entity) {
			cm := e.Common()
			if set&mpColor != 0 {
				cm.Color = c.color
			}
			if set&mpLayer != 0 && ctx.Doc.Layer(c.layer) != nil {
				cm.Layer = c.layer
			}
		})
	}
	if locked > 0 {
		ctx.SayPlural("cmd.matchprop.msg.locked", locked)
	}
}

// registerMatchprop adds MATCHPROP (alias MA).
func registerMatchprop(r *cmdsys.Registry) {
	r.Register("MATCHPROP", func() cmdsys.Command { return &matchpropCmd{} })
	r.Alias("MA", "MATCHPROP")
}
