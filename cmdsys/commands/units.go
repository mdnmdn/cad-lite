package commands

import (
	"math"
	"strconv"

	"cad-lite/cmdsys"
)

// The units variables. LUPREC and AUPREC are registered in cmdsys (sysvars.go); the rest live
// here. ANGBASE is stored in degrees whatever AUNITS is, like the other real variables;
// INSUNITS reads and writes the drawing's header, so it follows the open drawing.
func init() { cmdsys.RegisterVarGroup(unitsVars) }

func unitsVars(s *cmdsys.Session, v *cmdsys.Vars) {
	v.Register(cmdsys.Var{Name: "LUNITS", Kind: cmdsys.VarInt, Default: 2, Min: 1, Max: 5})
	v.Register(cmdsys.Var{Name: "AUNITS", Kind: cmdsys.VarInt, Default: 0, Min: 0, Max: 4})
	v.Register(cmdsys.Var{Name: "ANGBASE", Kind: cmdsys.VarReal, Default: 0.0})
	v.Register(cmdsys.Var{Name: "ANGDIR", Kind: cmdsys.VarInt, Default: 0, Min: 0, Max: 1})
	v.Register(cmdsys.Var{Name: "UNITMODE", Kind: cmdsys.VarInt, Default: 0, Min: 0, Max: 1})
	v.Register(cmdsys.Var{Name: "INSUNITS", Kind: cmdsys.VarInt, Default: 4, Min: 0, Max: 24,
		Read:     func() any { return s.Drawing().Header.InsUnits },
		OnChange: func(_, n any) { s.Drawing().Header.InsUnits = n.(int) }})
}

// --- UNITS, -UNITS ------------------------------------------------------

type unitsStage int

const (
	unitsLength unitsStage = iota
	unitsLengthPrec
	unitsAngle
	unitsAnglePrec
	unitsBase
	unitsClockwise
)

// unitsCmd is the command-line form of UNITS (AutoCAD's -UNITS; Cad-lite has no dialog, so UNITS
// runs the same prompts). Answers are applied when the last prompt is answered.
type unitsCmd struct {
	base
	name  string
	stage unitsStage

	lunits, lprec int
	aunits, aprec int
	baseDeg       float64 // ANGBASE in degrees
	clockwise     bool
}

func (c *unitsCmd) Name() string { return c.name }

// Transparent marks UNITS as runnable inside another command (cmdsys.Transparent).
func (*unitsCmd) Transparent() bool { return true }

func (c *unitsCmd) Start(ctx *cmdsys.Context) *cmdsys.Prompt {
	v := ctx.Vars()
	c.lunits, c.lprec = v.Int("LUNITS"), v.Int("LUPREC")
	c.aunits, c.aprec = v.Int("AUNITS"), v.Int("AUPREC")
	c.baseDeg, c.clockwise = v.Real("ANGBASE"), v.Int("ANGDIR") == 1
	for _, k := range []string{"report_formats", "length_1", "length_2", "length_3", "length_4", "length_5"} {
		ctx.Say("cmd.units.msg." + k)
	}
	return c.prompt(ctx)
}

func intDefault(v int) *cmdsys.Input {
	return &cmdsys.Input{Kind: cmdsys.InputInteger, Value: float64(v)}
}

func (c *unitsCmd) prompt(ctx *cmdsys.Context) *cmdsys.Prompt {
	switch c.stage {
	case unitsLength:
		return &cmdsys.Prompt{Message: ctx.T("cmd.units.prompt.length_format"), Accepts: cmdsys.InputInteger, Default: intDefault(c.lunits)}
	case unitsLengthPrec:
		if c.lunits >= cmdsys.LUnitsArchitectural {
			return &cmdsys.Prompt{Message: ctx.T("cmd.units.prompt.denominator"), Accepts: cmdsys.InputInteger, Default: intDefault(1 << uint(c.lprec))}
		}
		return &cmdsys.Prompt{Message: ctx.T("cmd.units.prompt.decimals"), Accepts: cmdsys.InputInteger, Default: intDefault(c.lprec)}
	case unitsAngle:
		for _, k := range []string{"angle_systems", "angle_1", "angle_2", "angle_3", "angle_4", "angle_5"} {
			ctx.Say("cmd.units.msg." + k)
		}
		return &cmdsys.Prompt{Message: ctx.T("cmd.units.prompt.angle_format"), Accepts: cmdsys.InputInteger, Default: intDefault(c.aunits + 1)}
	case unitsAnglePrec:
		return &cmdsys.Prompt{Message: ctx.T("cmd.units.prompt.angle_places"), Accepts: cmdsys.InputInteger, Default: intDefault(c.aprec)}
	case unitsBase:
		for _, k := range []string{"direction_systems", "direction_east", "direction_north", "direction_west", "direction_south"} {
			ctx.Say("cmd.units.msg." + k)
		}
		return &cmdsys.Prompt{Message: ctx.T("cmd.units.prompt.base"), Accepts: cmdsys.InputDistance | cmdsys.InputText,
			Default: &cmdsys.Input{Kind: cmdsys.InputDistance, Value: unitsBaseDefault(c.baseDeg, c.aunits)}}
	}
	def := "No"
	if c.clockwise {
		def = "Yes"
	}
	return &cmdsys.Prompt{Message: ctx.T("cmd.units.prompt.clockwise"), Accepts: cmdsys.InputKeyword, Keywords: []string{"Yes", "No"},
		Default: &cmdsys.Input{Kind: cmdsys.InputKeyword, Keyword: def}}
}

// unitsBaseDefault is ANGBASE (degrees) written as a bare number of the AUNITS system.
func unitsBaseDefault(deg float64, aunits int) float64 {
	switch aunits {
	case cmdsys.AUnitsGrads:
		return deg * 10 / 9
	case cmdsys.AUnitsRadians:
		return deg * math.Pi / 180
	}
	return deg
}

func (c *unitsCmd) Input(ctx *cmdsys.Context, in cmdsys.Input) *cmdsys.Prompt {
	n := int(in.Value)
	switch c.stage {
	case unitsLength:
		if n < 1 || n > 5 {
			return c.reject(ctx, "cmd.units.msg.range_1_5")
		}
		c.lunits = n
	case unitsLengthPrec:
		if c.lunits >= cmdsys.LUnitsArchitectural {
			e := math.Log2(in.Value)
			if in.Value < 1 || in.Value > 256 || e != math.Trunc(e) {
				return c.reject(ctx, "cmd.units.msg.bad_denominator")
			}
			c.lprec = int(e)
		} else {
			if n < 0 || n > 8 {
				return c.reject(ctx, "cmd.units.msg.range_0_8")
			}
			c.lprec = n
		}
	case unitsAngle:
		if n < 1 || n > 5 {
			return c.reject(ctx, "cmd.units.msg.range_1_5")
		}
		c.aunits = n - 1
	case unitsAnglePrec:
		if n < 0 || n > 8 {
			return c.reject(ctx, "cmd.units.msg.range_0_8")
		}
		c.aprec = n
	case unitsBase:
		text := in.Text
		if in.Kind == cmdsys.InputDistance {
			text = strconv.FormatFloat(in.Value, 'g', -1, 64)
		}
		rad, ok := cmdsys.ParseAngleText(text, c.aunits)
		if !ok {
			return c.reject(ctx, "cmd.units.msg.bad_angle")
		}
		c.baseDeg = rad * 180 / math.Pi
	case unitsClockwise:
		c.clockwise = in.Keyword == "Yes"
		c.apply(ctx)
		return nil
	}
	c.stage++
	return c.prompt(ctx)
}

func (c *unitsCmd) reject(ctx *cmdsys.Context, key string) *cmdsys.Prompt {
	ctx.Say(key)
	return c.prompt(ctx)
}

func (c *unitsCmd) apply(ctx *cmdsys.Context) {
	v := ctx.Vars()
	dir := 0
	if c.clockwise {
		dir = 1
	}
	for _, kv := range []struct {
		name string
		val  any
	}{{"LUNITS", c.lunits}, {"LUPREC", c.lprec}, {"AUNITS", c.aunits}, {"AUPREC", c.aprec}, {"ANGBASE", c.baseDeg}, {"ANGDIR", dir}} {
		_ = v.Set(kv.name, kv.val)
	}
}

// registerUnits adds UNITS (UN) and its command-line form -UNITS (-UN).
func registerUnits(r *cmdsys.Registry) {
	r.Register("UNITS", func() cmdsys.Command { return &unitsCmd{name: "UNITS"} })
	cmdsys.RegisterCLI(r, "UNITS", func() cmdsys.Command { return &unitsCmd{name: "-UNITS"} }, "UN")
	r.Alias("UN", "UNITS")
}
