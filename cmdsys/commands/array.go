package commands

import (
	"math"

	"go-draw/cmdsys"
	"go-draw/geom"
)

// maxArrayCopies bounds the copies one array command may make.
const maxArrayCopies = 100000

// kwPolar and kwSingleMode are keyword ids whose display text (kw.polar, kw.single_mode) carries a
// synonym or differs from the id, so they are named here rather than written as literals.
const (
	kwPolar      = "Polar"
	kwSingleMode = "Single mode"
)

type arrayStage int

const (
	arSelect arrayStage = iota
	arType
	arRows
	arColumns
	arRowDist
	arCell
	arColDist
	arCenter
	arBase
	arItems
	arFill
	arRotate
)

// arrayCmd is -ARRAY (the command-line prompts), ARRAY, ARRAYRECT and ARRAYPOLAR. They share one
// prompt sequence and make explicit, non-associative copies: go-draw has no array object.
type arrayCmd struct {
	base
	selecting
	name  string
	polar bool
	// typed is true once the array type is known (ARRAYRECT and ARRAYPOLAR start with it).
	typed bool
	stage arrayStage

	rows, cols       int
	rowDist, colDist float64
	cell             *geom.Vec2
	center           geom.Vec2
	basePt           *geom.Vec2
	items            int
	fill             float64
}

func (c *arrayCmd) Name() string { return c.name }

// defaultLetter is the shortcut letter a [..] <Y> style default shows, in the session language.
func defaultLetter(ctx *cmdsys.Context, id string) *cmdsys.Input {
	l := []rune(ctx.Translator().KeywordLabel(id))
	for _, r := range l {
		if r >= 'A' && r <= 'Z' || r >= 'À' && r <= 'Ý' {
			return defaultText(string(r))
		}
	}
	return defaultText(string(l[:1]))
}

func (c *arrayCmd) typePrompt(ctx *cmdsys.Context) *cmdsys.Prompt {
	c.stage = arType
	if c.name == "-ARRAY" {
		return &cmdsys.Prompt{Message: ctx.T("cmd.array.prompt.type_cli"), Accepts: cmdsys.InputKeyword,
			Keywords: []string{"Rectangular", kwPolar}, Default: defaultLetter(ctx, "Rectangular")}
	}
	return &cmdsys.Prompt{Message: ctx.T("cmd.array.prompt.type"), Accepts: cmdsys.InputKeyword,
		Keywords: []string{"Rectangular", "PAth", kwPolar}, Default: defaultKeyword("Rectangular")}
}

func (c *arrayCmd) countPrompt(ctx *cmdsys.Context, stage arrayStage, key string) *cmdsys.Prompt {
	c.stage = stage
	return &cmdsys.Prompt{Message: ctx.T(key), Accepts: cmdsys.InputInteger,
		Default: &cmdsys.Input{Kind: cmdsys.InputInteger, Value: 1}}
}

// afterType starts the prompts of the chosen type.
func (c *arrayCmd) afterType(ctx *cmdsys.Context) *cmdsys.Prompt {
	c.typed = true
	t := "Rectangular"
	if c.polar {
		t = kwPolar
	}
	if c.name != "-ARRAY" {
		ctx.Say("cmd.array.msg.type", ctx.Translator().KeywordLabel(t))
	}
	if c.polar {
		return c.centerPrompt(ctx)
	}
	return c.countPrompt(ctx, arRows, "cmd.array.prompt.rows")
}

func (c *arrayCmd) centerPrompt(ctx *cmdsys.Context) *cmdsys.Prompt {
	c.stage = arCenter
	return &cmdsys.Prompt{Message: ctx.T("cmd.array.prompt.center"),
		Accepts: cmdsys.InputPoint | cmdsys.InputKeyword, Keywords: []string{"Base"}}
}

func (c *arrayCmd) itemsPrompt(ctx *cmdsys.Context) *cmdsys.Prompt {
	c.stage = arItems
	return &cmdsys.Prompt{Message: ctx.T("cmd.array.prompt.items"), Accepts: cmdsys.InputInteger}
}

func (c *arrayCmd) fillPrompt(ctx *cmdsys.Context) *cmdsys.Prompt {
	c.stage = arFill
	return &cmdsys.Prompt{Message: ctx.T("cmd.array.prompt.fill"), Accepts: cmdsys.InputAngle,
		Default: &cmdsys.Input{Kind: cmdsys.InputAngle, Value: 2 * math.Pi}}
}

func (c *arrayCmd) rotatePrompt(ctx *cmdsys.Context) *cmdsys.Prompt {
	c.stage = arRotate
	return &cmdsys.Prompt{Message: ctx.T("cmd.array.prompt.rotate"), Accepts: cmdsys.InputKeyword,
		Keywords: []string{"Yes", "No"}, Default: defaultLetter(ctx, "Yes")}
}

func (c *arrayCmd) rowDistPrompt(ctx *cmdsys.Context) *cmdsys.Prompt {
	c.stage = arRowDist
	return &cmdsys.Prompt{Message: ctx.T("cmd.array.prompt.row_dist"), Accepts: cmdsys.InputDistance | cmdsys.InputPoint}
}

func (c *arrayCmd) colDistPrompt(ctx *cmdsys.Context) *cmdsys.Prompt {
	c.stage = arColDist
	return &cmdsys.Prompt{Message: ctx.T("cmd.array.prompt.col_dist"), Accepts: cmdsys.InputDistance}
}

func (c *arrayCmd) Start(ctx *cmdsys.Context) *cmdsys.Prompt {
	if c.begin(ctx) {
		return c.afterSelect(ctx)
	}
	return selectPrompt(ctx)
}

func (c *arrayCmd) afterSelect(ctx *cmdsys.Context) *cmdsys.Prompt {
	if c.typed {
		return c.afterType(ctx)
	}
	return c.typePrompt(ctx)
}

// spacing asks for the next distance the rows and columns still need, or builds the array.
func (c *arrayCmd) spacing(ctx *cmdsys.Context) *cmdsys.Prompt {
	switch {
	case c.rows > 1:
		return c.rowDistPrompt(ctx)
	case c.cols > 1:
		return c.colDistPrompt(ctx)
	}
	return nil
}

func (c *arrayCmd) Input(ctx *cmdsys.Context, in cmdsys.Input) *cmdsys.Prompt {
	switch c.stage {
	case arSelect:
		if !c.take(ctx, in) {
			return nil
		}
		return c.afterSelect(ctx)
	case arType:
		if in.Kind == cmdsys.InputKeyword && in.Keyword == "PAth" {
			ctx.Say("cmd.array.msg.path")
			return c.typePrompt(ctx)
		}
		c.polar = in.Kind == cmdsys.InputKeyword && in.Keyword == kwPolar
		return c.afterType(ctx)
	case arRows, arColumns:
		return c.counts(ctx, in)
	case arRowDist:
		if in.Kind == cmdsys.InputPoint {
			c.cell = pt(in.Point)
			c.stage = arCell
			return &cmdsys.Prompt{Message: ctx.T("cmd.array.prompt.cell"), Accepts: cmdsys.InputPoint, Base: c.cell}
		}
		c.rowDist = in.Value
		if c.cols > 1 {
			return c.colDistPrompt(ctx)
		}
		return c.build(ctx)
	case arCell:
		d := in.Point.Sub(*c.cell)
		c.rowDist, c.colDist = d.Y, d.X
		return c.build(ctx)
	case arColDist:
		c.colDist = in.Value
		return c.build(ctx)
	case arCenter:
		if in.Kind == cmdsys.InputKeyword {
			c.stage = arBase
			return &cmdsys.Prompt{Message: ctx.T("cmd.modify.prompt.base_point"), Accepts: cmdsys.InputPoint}
		}
		c.center = in.Point
		return c.itemsPrompt(ctx)
	case arBase:
		c.basePt = pt(in.Point)
		return c.centerPrompt(ctx)
	case arItems:
		if in.Value < 2 || in.Value > maxArrayCopies {
			ctx.Say("cmd.array.msg.count")
			return c.itemsPrompt(ctx)
		}
		if int(in.Value)*len(c.hs) > maxArrayCopies {
			ctx.Say("cmd.array.msg.too_many")
			return c.itemsPrompt(ctx)
		}
		c.items = int(in.Value)
		return c.fillPrompt(ctx)
	case arFill:
		c.fill = in.Value
		return c.rotatePrompt(ctx)
	}
	yes := in.Keyword == "Yes" || in.Kind == cmdsys.InputText
	return c.buildPolar(ctx, yes)
}

// counts takes the number of rows, then of columns.
func (c *arrayCmd) counts(ctx *cmdsys.Context, in cmdsys.Input) *cmdsys.Prompt {
	n := int(in.Value)
	if in.Value != float64(n) || n < 1 || n > maxArrayCopies {
		ctx.Say("cmd.array.msg.count")
		if c.stage == arRows {
			return c.countPrompt(ctx, arRows, "cmd.array.prompt.rows")
		}
		return c.countPrompt(ctx, arColumns, "cmd.array.prompt.columns")
	}
	if c.stage == arRows {
		c.rows = n
		return c.countPrompt(ctx, arColumns, "cmd.array.prompt.columns")
	}
	c.cols = n
	if c.rows*c.cols*len(c.hs) > maxArrayCopies {
		ctx.Say("cmd.array.msg.too_many")
		return c.countPrompt(ctx, arRows, "cmd.array.prompt.rows")
	}
	if p := c.spacing(ctx); p != nil {
		return p
	}
	return c.build(ctx)
}

// build makes the rectangular array: row r and column k are offset by (k*colDist, r*rowDist), so
// positive distances go up and right and negative ones down and left.
func (c *arrayCmd) build(ctx *cmdsys.Context) *cmdsys.Prompt {
	for r := 0; r < c.rows; r++ {
		for k := 0; k < c.cols; k++ {
			if r == 0 && k == 0 {
				continue
			}
			m := geom.Translate(geom.V(float64(k)*c.colDist, float64(r)*c.rowDist))
			for _, h := range c.hs {
				addCopy(ctx, h, m)
			}
		}
	}
	return nil
}

// buildPolar makes the polar array: item i is the selection turned by i steps about the centre.
// A full circle spreads the items over 360 degrees (n steps), a partial fill puts the last item
// on the fill angle (n-1 steps). Without rotation the copies keep their orientation and only
// the base point travels around the circle.
func (c *arrayCmd) buildPolar(ctx *cmdsys.Context, rotate bool) *cmdsys.Prompt {
	step := c.fill / float64(c.items-1)
	if math.Abs(math.Abs(c.fill)-2*math.Pi) < 1e-9 {
		step = c.fill / float64(c.items)
	}
	bp := c.centroid(ctx)
	for i := 1; i < c.items; i++ {
		rot := geom.Rotate(c.center, step*float64(i))
		m := rot
		if !rotate {
			m = geom.Translate(rot.Apply(bp).Sub(bp))
		}
		for _, h := range c.hs {
			addCopy(ctx, h, m)
		}
	}
	return nil
}

// centroid is the base point of a polar array: the one given, else the centre of the selection's
// bounding box.
func (c *arrayCmd) centroid(ctx *cmdsys.Context) geom.Vec2 {
	if c.basePt != nil {
		return *c.basePt
	}
	var b geom.Box2
	first := true
	for _, h := range c.hs {
		e := ctx.Doc.Entity(h)
		if e == nil {
			continue
		}
		if first {
			b, first = e.Bounds(), false
			continue
		}
		b = b.Union(e.Bounds())
	}
	return b.Center()
}

// registerArray adds ARRAY, ARRAYRECT, ARRAYPOLAR and -ARRAY (alias -AR).
func registerArray(r *cmdsys.Registry) {
	r.Register("ARRAY", func() cmdsys.Command { return &arrayCmd{name: "ARRAY"} })
	r.Register("ARRAYRECT", func() cmdsys.Command { return &arrayCmd{name: "ARRAYRECT", typed: true} })
	r.Register("ARRAYPOLAR", func() cmdsys.Command { return &arrayCmd{name: "ARRAYPOLAR", typed: true, polar: true} })
	cmdsys.RegisterCLI(r, "ARRAY", func() cmdsys.Command { return &arrayCmd{name: "-ARRAY"} }, "AR")
	r.Alias("AR", "ARRAY")
}
