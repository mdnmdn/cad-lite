package commands

import (
	"strings"

	"cad-lite/cmdsys"
	"cad-lite/doc"
)

// MATCHPROP settings bits (the MATCHPROPSET variable), the settings of AutoCAD's Property Settings
// dialog that Cad-lite's objects carry: the basic properties colour, layer, linetype, linetype
// scale and lineweight, and the special property Dimension (the dimension style and overrides).
const (
	mpColor = 1 << iota
	mpLayer
	mpLtype
	mpLtScale
	mpLWeight
	mpDim
)

const mpAll = mpColor | mpLayer | mpLtype | mpLtScale | mpLWeight | mpDim

// kwDimension is MATCHPROP's Dimension setting.
const kwDimension = "Dimension"

// matchpropKeywords are the settings MATCHPROP's Settings option switches.
var matchpropKeywords = append(append([]string{}, chpropKeywords...), kwDimension)

// mpSettings pairs each setting bit with its keyword and its name in the settings line.
var mpSettings = []struct {
	bit     int
	keyword string
	key     string
}{
	{mpColor, kwColor, "cmd.matchprop.setting.color"},
	{mpLayer, kwChLayer, "cmd.matchprop.setting.layer"},
	{mpLtype, kwChLtype, "cmd.matchprop.setting.ltype"},
	{mpLtScale, kwLtScale, "cmd.matchprop.setting.ltscale"},
	{mpLWeight, kwLWeight, "cmd.matchprop.setting.lweight"},
	{mpDim, kwDimension, "cmd.matchprop.setting.dim"},
}

func init() {
	cmdsys.RegisterVarGroup(func(_ *cmdsys.Session, v *cmdsys.Vars) {
		// A Cad-lite variable standing in for the Property Settings dialog of MATCHPROP: the sum
		// of 1 (Color), 2 (Layer), 4 (Linetype), 8 (Linetype scale), 16 (Lineweight) and 32
		// (Dimension) that MATCHPROP copies.
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
	step matchStep
	// src holds the source object's properties.
	src doc.EntityCommon
	// srcDim is the source when it is a dimension, for the Dimension setting.
	srcDim *doc.Dimension
}

func (*matchpropCmd) Name() string { return "MATCHPROP" }

func (c *matchpropCmd) Start(ctx *cmdsys.Context) *cmdsys.Prompt {
	return &cmdsys.Prompt{Message: ctx.T("cmd.matchprop.prompt.source"), Accepts: cmdsys.InputPoint}
}

// settingsText names the properties the current settings copy.
func mpSettingsText(ctx *cmdsys.Context) string {
	set := ctx.Vars().Int("MATCHPROPSET")
	var parts []string
	for _, st := range mpSettings {
		if set&st.bit != 0 {
			parts = append(parts, ctx.T(st.key))
		}
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
		c.src = *e.Common()
		c.srcDim, _ = e.(*doc.Dimension)
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
			bit := 0
			for _, st := range mpSettings {
				if st.keyword == in.Keyword {
					bit = st.bit
				}
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
	return &cmdsys.Prompt{Message: ctx.T("cmd.matchprop.prompt.settings"), Accepts: cmdsys.InputKeyword | cmdsys.InputEnter, Keywords: matchpropKeywords, NoOr: true}
}

// apply copies the settings of the source object onto hs; objects on a locked layer are skipped,
// and an object the settings would not change is left out of the undo step. Dimension copies a
// source dimension's style name and overrides onto the dimensions among hs.
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
		match := func(cm *doc.EntityCommon) {
			if set&mpColor != 0 {
				cm.Color = c.src.Color
			}
			if set&mpLayer != 0 && ctx.Doc.Layer(c.src.Layer) != nil {
				cm.Layer = c.src.Layer
			}
			if set&mpLtype != 0 {
				cm.Linetype = c.src.Linetype
			}
			if set&mpLtScale != 0 {
				cm.LinetypeScale = c.src.LinetypeScale
			}
			if set&mpLWeight != 0 {
				cm.Lineweight = c.src.Lineweight
			}
		}
		dm, isDim := e.(*doc.Dimension)
		dimStyle := set&mpDim != 0 && c.srcDim != nil && isDim &&
			(dm.StyleName != c.srcDim.StyleName || dm.Overrides != c.srcDim.Overrides)
		probe := *e.Common()
		if match(&probe); probe != *e.Common() || dimStyle {
			ctx.Tx().Modify(h, func(e doc.Entity) {
				match(e.Common())
				if dimStyle {
					dm := e.(*doc.Dimension)
					dm.StyleName, dm.Overrides = c.srcDim.StyleName, c.srcDim.Overrides
					dm.Recompute()
				}
			})
		}
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
