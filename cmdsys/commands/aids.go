package commands

import (
	"math"
	"strconv"
	"strings"

	"go-draw/cmdsys"
	"go-draw/geom"
)

// This file is the drafting-aid commands: ORTHO, GRID, SNAP and OSNAP / -OSNAP (the command-line
// form; there is no dialog). Each only sets system variables (ORTHOMODE, GRIDMODE, GRIDUNIT,
// GRIDMAJOR, SNAPMODE, SNAPUNIT, SNAPSTYL, SNAPTYPE, OSMODE), so the F-keys, SETVAR and the
// commands share one state (cmdsys/aids.go). All four are transparent. GRID's Limits, aDaptive
// and Follow options and SNAP's Legacy option are not offered; Isometric style and Polar type are
// stored and have no effect.

// registerAids adds ORTHO, GRID, SNAP and OSNAP.
func registerAids(r *cmdsys.Registry) {
	r.Register("ORTHO", func() cmdsys.Command { return &orthoCmd{} })
	r.Register("GRID", func() cmdsys.Command { return &gridCmd{} })
	r.Alias("GR", "GRID")
	r.Register("SNAP", func() cmdsys.Command { return &snapCmd{} })
	r.Alias("SN", "SNAP")
	r.Register("OSNAP", func() cmdsys.Command { return &osnapCmd{name: "OSNAP"} })
	r.Alias("OS", "OSNAP")
	cmdsys.RegisterCLI(r, "OSNAP", func() cmdsys.Command { return &osnapCmd{name: "-OSNAP"} }, "OS")
}

// onOff returns the ON or OFF keyword id for b.
func aidOnOff(b bool) string {
	if b {
		return "ON"
	}
	return "OFF"
}

// spacingText writes a spacing as AutoCAD's prompt defaults do: four decimals.
func spacingText(v float64) string { return strconv.FormatFloat(v, 'f', 4, 64) }

func aidSet(ctx *cmdsys.Context, name string, v any) { _ = ctx.Vars().Set(name, v) }

func aidBool(b bool) int {
	if b {
		return 1
	}
	return 0
}

// --- ORTHO --------------------------------------------------------------

type orthoCmd struct{ base }

func (*orthoCmd) Name() string      { return "ORTHO" }
func (*orthoCmd) Transparent() bool { return true }

func (*orthoCmd) Start(ctx *cmdsys.Context) *cmdsys.Prompt {
	return onOffPrompt(ctx.T("cmd.ortho.prompt.mode"), ctx.Vars().Int("ORTHOMODE") != 0)
}

func onOffPrompt(msg string, cur bool) *cmdsys.Prompt {
	return &cmdsys.Prompt{Message: msg, NoOr: true, Accepts: cmdsys.InputKeyword, Keywords: []string{"ON", "OFF"},
		Default: &cmdsys.Input{Kind: cmdsys.InputKeyword, Keyword: aidOnOff(cur)}}
}

func (*orthoCmd) Input(ctx *cmdsys.Context, in cmdsys.Input) *cmdsys.Prompt {
	aidSet(ctx, "ORTHOMODE", aidBool(in.Keyword == "ON"))
	return nil
}

// --- GRID ---------------------------------------------------------------

type gridStage int

const (
	gridMain gridStage = iota
	gridMajor
	gridAspectH
	gridAspectV
)

type gridCmd struct {
	base
	stage gridStage
	h     float64
}

func (*gridCmd) Name() string      { return "GRID" }
func (*gridCmd) Transparent() bool { return true }

func (c *gridCmd) Start(ctx *cmdsys.Context) *cmdsys.Prompt { return c.mainPrompt(ctx) }

func (c *gridCmd) mainPrompt(ctx *cmdsys.Context) *cmdsys.Prompt {
	c.stage = gridMain
	u := ctx.Vars().Point("GRIDUNIT")
	return &cmdsys.Prompt{Message: ctx.T("cmd.grid.prompt.spacing"),
		Accepts:  cmdsys.InputKeyword | cmdsys.InputDistance | cmdsys.InputText | cmdsys.InputEnter,
		Keywords: []string{"ON", "OFF", "Snap", "Major", "Aspect"},
		Default:  &cmdsys.Input{Kind: cmdsys.InputText, Text: spacingText(u.X)}}
}

func (c *gridCmd) Input(ctx *cmdsys.Context, in cmdsys.Input) *cmdsys.Prompt {
	v := ctx.Vars()
	switch c.stage {
	case gridMajor:
		if in.Kind == cmdsys.InputEnter {
			return nil
		}
		if in.Value < 1 {
			ctx.Say("cmd.aids.msg.positive")
			return c.majorPrompt(ctx)
		}
		aidSet(ctx, "GRIDMAJOR", int(in.Value))
		return nil
	case gridAspectH, gridAspectV:
		u := v.Point("GRIDUNIT")
		cur := u.X
		if c.stage == gridAspectV {
			cur = u.Y
		}
		val := cur
		if in.Kind != cmdsys.InputEnter {
			val = in.Value
		}
		if !(val > 0) {
			ctx.Say("cmd.aids.msg.positive")
			return c.aspectPrompt(ctx, c.stage == gridAspectV)
		}
		if c.stage == gridAspectH {
			c.h = val
			return c.aspectPrompt(ctx, true)
		}
		aidSet(ctx, "GRIDUNIT", geom.V(c.h, val))
		return nil
	}
	switch in.Kind {
	case cmdsys.InputEnter:
		return nil
	case cmdsys.InputKeyword:
		switch in.Keyword {
		case "ON", "OFF":
			aidSet(ctx, "GRIDMODE", aidBool(in.Keyword == "ON"))
		case "Snap":
			u := v.Point("SNAPUNIT")
			aidSet(ctx, "GRIDUNIT", u)
		case "Major":
			return c.majorPrompt(ctx)
		case "Aspect":
			return c.aspectPrompt(ctx, false)
		}
		return nil
	case cmdsys.InputDistance:
		return c.setSpacing(ctx, in.Value, false)
	}
	if n, ok := strings.CutSuffix(strings.ToLower(strings.TrimSpace(in.Text)), "x"); ok {
		if f, err := strconv.ParseFloat(n, 64); err == nil && f > 0 && !math.IsInf(f, 0) {
			return c.setSpacing(ctx, f, true)
		}
	}
	ctx.Say("cmd.aids.msg.invalid_value")
	return c.mainPrompt(ctx)
}

// setSpacing sets GRIDUNIT to v (a multiple of the snap spacing when times); 0 follows the snap spacing.
func (c *gridCmd) setSpacing(ctx *cmdsys.Context, v float64, times bool) *cmdsys.Prompt {
	if v < 0 {
		ctx.Say("cmd.aids.msg.positive")
		return c.mainPrompt(ctx)
	}
	su := ctx.Vars().Point("SNAPUNIT")
	switch {
	case times:
		aidSet(ctx, "GRIDUNIT", geom.V(su.X*v, su.Y*v))
	case v == 0:
		aidSet(ctx, "GRIDUNIT", su)
	default:
		aidSet(ctx, "GRIDUNIT", geom.V(v, v))
	}
	return nil
}

func (c *gridCmd) majorPrompt(ctx *cmdsys.Context) *cmdsys.Prompt {
	c.stage = gridMajor
	return &cmdsys.Prompt{Message: ctx.T("cmd.grid.prompt.major"), Accepts: cmdsys.InputInteger | cmdsys.InputEnter,
		Default: &cmdsys.Input{Kind: cmdsys.InputInteger, Value: float64(ctx.Vars().Int("GRIDMAJOR"))}}
}

func (c *gridCmd) aspectPrompt(ctx *cmdsys.Context, vertical bool) *cmdsys.Prompt {
	u := ctx.Vars().Point("GRIDUNIT")
	key, cur := "cmd.grid.prompt.horizontal", u.X
	c.stage = gridAspectH
	if vertical {
		key, cur, c.stage = "cmd.grid.prompt.vertical", u.Y, gridAspectV
	}
	return &cmdsys.Prompt{Message: ctx.T(key), Accepts: cmdsys.InputDistance | cmdsys.InputEnter,
		Default: &cmdsys.Input{Kind: cmdsys.InputDistance, Value: cur}}
}

// --- SNAP ---------------------------------------------------------------

type snapStage int

const (
	snapMain snapStage = iota
	snapAspectH
	snapAspectV
	snapStyle
	snapType
)

type snapCmd struct {
	base
	stage snapStage
	h     float64
}

func (*snapCmd) Name() string      { return "SNAP" }
func (*snapCmd) Transparent() bool { return true }

func (c *snapCmd) Start(ctx *cmdsys.Context) *cmdsys.Prompt { return c.mainPrompt(ctx) }

func (c *snapCmd) mainPrompt(ctx *cmdsys.Context) *cmdsys.Prompt {
	c.stage = snapMain
	u := ctx.Vars().Point("SNAPUNIT")
	return &cmdsys.Prompt{Message: ctx.T("cmd.snap.prompt.spacing"),
		Accepts:  cmdsys.InputKeyword | cmdsys.InputDistance | cmdsys.InputEnter,
		Keywords: []string{"ON", "OFF", "Aspect", "Style", "Type"},
		Default:  &cmdsys.Input{Kind: cmdsys.InputText, Text: spacingText(u.X)}}
}

func (c *snapCmd) aspectPrompt(ctx *cmdsys.Context, vertical bool) *cmdsys.Prompt {
	u := ctx.Vars().Point("SNAPUNIT")
	key, cur := "cmd.snap.prompt.horizontal", u.X
	c.stage = snapAspectH
	if vertical {
		key, cur, c.stage = "cmd.snap.prompt.vertical", u.Y, snapAspectV
	}
	return &cmdsys.Prompt{Message: ctx.T(key), Accepts: cmdsys.InputDistance | cmdsys.InputEnter,
		Default: &cmdsys.Input{Kind: cmdsys.InputDistance, Value: cur}}
}

func (c *snapCmd) Input(ctx *cmdsys.Context, in cmdsys.Input) *cmdsys.Prompt {
	v := ctx.Vars()
	switch c.stage {
	case snapAspectH, snapAspectV:
		u := v.Point("SNAPUNIT")
		val := u.X
		if c.stage == snapAspectV {
			val = u.Y
		}
		if in.Kind != cmdsys.InputEnter {
			val = in.Value
		}
		if !(val > 0) {
			ctx.Say("cmd.aids.msg.positive")
			return c.aspectPrompt(ctx, c.stage == snapAspectV)
		}
		if c.stage == snapAspectH {
			c.h = val
			return c.aspectPrompt(ctx, true)
		}
		aidSet(ctx, "SNAPUNIT", geom.V(c.h, val))
		return nil
	case snapStyle:
		if in.Kind == cmdsys.InputKeyword {
			aidSet(ctx, "SNAPSTYL", aidBool(in.Keyword == "Isometric"))
		}
		return nil
	case snapType:
		if in.Kind == cmdsys.InputKeyword {
			aidSet(ctx, "SNAPTYPE", aidBool(in.Keyword == kwPolar))
		}
		return nil
	}
	switch in.Kind {
	case cmdsys.InputKeyword:
		switch in.Keyword {
		case "ON", "OFF":
			aidSet(ctx, "SNAPMODE", aidBool(in.Keyword == "ON"))
		case "Aspect":
			return c.aspectPrompt(ctx, false)
		case "Style":
			c.stage = snapStyle
			return &cmdsys.Prompt{Message: ctx.T("cmd.snap.prompt.style"), Accepts: cmdsys.InputKeyword,
				Keywords: []string{"Standard", "Isometric"},
				Default:  &cmdsys.Input{Kind: cmdsys.InputKeyword, Keyword: [2]string{"Standard", "Isometric"}[v.Int("SNAPSTYL")&1]}}
		case "Type":
			c.stage = snapType
			return &cmdsys.Prompt{Message: ctx.T("cmd.snap.prompt.type"), Accepts: cmdsys.InputKeyword,
				Keywords: []string{kwPolar, "Grid"},
				Default:  &cmdsys.Input{Kind: cmdsys.InputKeyword, Keyword: [2]string{"Grid", kwPolar}[v.Int("SNAPTYPE")&1]}}
		}
	case cmdsys.InputDistance:
		if !(in.Value > 0) {
			ctx.Say("cmd.aids.msg.positive")
			return c.mainPrompt(ctx)
		}
		aidSet(ctx, "SNAPUNIT", geom.V(in.Value, in.Value))
	}
	return nil
}

// --- OSNAP / -OSNAP -----------------------------------------------------

type osnapCmd struct {
	base
	name string
}

func (c *osnapCmd) Name() string    { return c.name }
func (*osnapCmd) Transparent() bool { return true }

func (*osnapCmd) Start(ctx *cmdsys.Context) *cmdsys.Prompt {
	cur := cmdsys.SnapListString(cmdsys.SnapMode(ctx.Vars().Int("OSMODE") &^ 16384))
	ctx.Say("cmd.osnap.msg.current", cur)
	return &cmdsys.Prompt{Message: ctx.T("cmd.osnap.prompt.modes"), Accepts: cmdsys.InputText | cmdsys.InputEnter,
		Default: &cmdsys.Input{Kind: cmdsys.InputText, Text: cur}}
}

func (c *osnapCmd) Input(ctx *cmdsys.Context, in cmdsys.Input) *cmdsys.Prompt {
	if in.Kind == cmdsys.InputEnter {
		return nil
	}
	mode, bad, ok := cmdsys.ParseSnapList(in.Text)
	if !ok {
		ctx.Say("cmd.osnap.msg.invalid", bad)
		return c.Start(ctx)
	}
	aidSet(ctx, "OSMODE", int(mode))
	return nil
}
