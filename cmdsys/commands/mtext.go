package commands

import (
	"math"
	"strings"

	"cad-lite/cmdsys"
	"cad-lite/doc"
	"cad-lite/geom"
	"cad-lite/render"
)

// --- MTEXT --------------------------------------------------------------

// mtextJustifyKeywords are MTEXT's attachment options, in the order of doc's attachment values.
var mtextJustifyKeywords = []string{"TL", "TC", "TR", "ML", "MC", "MR", "BL", "BC", "BR"}

type mtextStep int

const (
	mtFirst   mtextStep = iota // first corner
	mtCorner                   // opposite corner or [Height/Justify/Rotation/Style/Width]
	mtHeight                   // height
	mtJustify                  // attachment
	mtAngle                    // rotation
	mtStyle                    // style name
	mtWidth                    // width
	mtText                     // the text, one paragraph per answer
)

// mtextCmd is MTEXT (alias T and MT) in its command-line form, the one AutoCAD runs from scripts:
// a box from two corners (or a width), then the paragraphs, one per line, until an empty line.
// The options are AutoCAD's except Line spacing and Columns.
type mtextCmd struct {
	base
	step   mtextStep
	p1     geom.Vec2
	height float64
	angle  float64
	width  float64
	attach int
	style  string
	paras  []string
}

func (*mtextCmd) Name() string { return "MTEXT" }

func (c *mtextCmd) Start(ctx *cmdsys.Context) *cmdsys.Prompt {
	c.style = ctx.Vars().Text("TEXTSTYLE")
	if c.style == "" {
		c.style = doc.DefaultTextStyle
	}
	c.height = ctx.Vars().Real("TEXTSIZE")
	c.attach = doc.MTextTopLeft
	ctx.Say("cmd.mtext.msg.current", c.style, ctx.Fmt(c.height))
	return c.prompt(ctx)
}

func (c *mtextCmd) prompt(ctx *cmdsys.Context) *cmdsys.Prompt {
	switch c.step {
	case mtCorner:
		return &cmdsys.Prompt{Message: ctx.T("cmd.mtext.prompt.corner"), Accepts: cmdsys.InputPoint | cmdsys.InputKeyword,
			Keywords: []string{"Height", "Justify", "Rotation", "Style", "Width"}, Base: pt(c.p1)}
	case mtHeight:
		return &cmdsys.Prompt{Message: ctx.T("cmd.text.prompt.height"), Accepts: cmdsys.InputDistance, Base: pt(c.p1),
			Default: &cmdsys.Input{Kind: cmdsys.InputDistance, Value: c.height}}
	case mtJustify:
		return &cmdsys.Prompt{Message: ctx.T("cmd.mtext.prompt.justify"), Accepts: cmdsys.InputKeyword, Keywords: mtextJustifyKeywords, NoOr: true,
			Default: &cmdsys.Input{Kind: cmdsys.InputKeyword, Keyword: mtextJustifyKeywords[c.attach-1]}}
	case mtAngle:
		return &cmdsys.Prompt{Message: ctx.T("cmd.mtext.prompt.rotation"), Accepts: cmdsys.InputAngle, Base: pt(c.p1),
			Default: &cmdsys.Input{Kind: cmdsys.InputAngle, Value: c.angle}}
	case mtStyle:
		return &cmdsys.Prompt{Message: ctx.T("cmd.text.prompt.style"), Accepts: cmdsys.InputText | cmdsys.InputKeyword, Keywords: []string{"?"},
			NoOr: true, Default: &cmdsys.Input{Kind: cmdsys.InputText, Text: c.style}}
	case mtWidth:
		return &cmdsys.Prompt{Message: ctx.T("cmd.mtext.prompt.width"), Accepts: cmdsys.InputDistance, Base: pt(c.p1)}
	case mtText:
		return &cmdsys.Prompt{Message: ctx.T("cmd.mtext.prompt.text"), Accepts: cmdsys.InputText | cmdsys.InputEnter, Literal: true}
	}
	return &cmdsys.Prompt{Message: ctx.T("cmd.mtext.prompt.first"), Accepts: cmdsys.InputPoint}
}

func (c *mtextCmd) Input(ctx *cmdsys.Context, in cmdsys.Input) *cmdsys.Prompt {
	switch c.step {
	case mtFirst:
		c.p1, c.step = in.Point, mtCorner
	case mtCorner:
		switch in.Keyword {
		case "Height":
			c.step = mtHeight
		case "Justify":
			c.step = mtJustify
		case "Rotation":
			c.step = mtAngle
		case "Style":
			c.step = mtStyle
		case "Width":
			c.step = mtWidth
		default:
			if in.Kind == cmdsys.InputPoint {
				c.corner(in.Point)
				c.step = mtText
			}
		}
	case mtHeight:
		if !(in.Value > 0) || math.IsInf(in.Value, 0) {
			ctx.Say("cmd.common.msg.positive")
			break
		}
		c.height = in.Value
		_ = ctx.Vars().Set("TEXTSIZE", in.Value)
		c.step = mtCorner
	case mtJustify:
		for i, k := range mtextJustifyKeywords {
			if in.Keyword == k {
				c.attach = i + 1
			}
		}
		c.step = mtCorner
	case mtAngle:
		c.angle, c.step = in.Value, mtCorner
	case mtStyle:
		switch {
		case in.Keyword == "?":
			ctx.Say("cmd.text.msg.styles")
			for _, s := range doc.TextStyles(ctx.Doc) {
				ctx.Say("cmd.text.msg.style_item", s)
			}
			return c.prompt(ctx)
		case in.Kind == cmdsys.InputText:
			name, ok := doc.FindTextStyle(ctx.Doc, in.Text)
			if !ok {
				ctx.Say("cmd.text.msg.no_style", in.Text)
				return c.prompt(ctx)
			}
			c.style = name
			_ = ctx.Vars().Set("TEXTSTYLE", name)
		}
		c.step = mtCorner
	case mtWidth:
		if !(in.Value >= 0) || math.IsInf(in.Value, 0) {
			ctx.Say("cmd.common.msg.positive")
			break
		}
		c.width, c.step = in.Value, mtText
	case mtText:
		if in.Kind == cmdsys.InputText && in.Text != "" {
			c.paras = append(c.paras, doc.MTextEscape(in.Text))
			break
		}
		if len(c.paras) > 0 {
			ctx.Tx().Add(c.entity(strings.Join(c.paras, `\P`)))
		}
		return nil
	}
	return c.prompt(ctx)
}

// corner sets the width and moves the insertion point to the attachment point of the box from
// the first corner to p, measured along the rotation.
func (c *mtextCmd) corner(p geom.Vec2) {
	d := geom.V(math.Cos(c.angle), math.Sin(c.angle))
	v := p.Sub(c.p1)
	x, y := v.Dot(d), v.Dot(d.Perp())
	c.width = math.Abs(x)
	row, col := (c.attach-1)/3, (c.attach-1)%3
	ax := []float64{min(0, x), x / 2, max(0, x)}[col]
	ay := []float64{max(0, y), y / 2, min(0, y)}[row]
	c.p1 = c.p1.Add(d.Scale(ax)).Add(d.Perp().Scale(ay))
}

func (c *mtextCmd) entity(value string) *doc.MText {
	t := doc.NewMText(c.p1, c.height, c.width, value)
	t.Attachment, t.Rotation, t.Style = c.attach, c.angle, c.style
	return t
}

func (c *mtextCmd) Preview(ctx *cmdsys.Context, cur geom.Vec2, dl *render.DisplayList) {
	switch c.step {
	case mtCorner:
		d := geom.V(math.Cos(c.angle), math.Sin(c.angle))
		v := cur.Sub(c.p1)
		x, y := d.Scale(v.Dot(d)), d.Perp().Scale(v.Dot(d.Perp()))
		dl.Polyline([]geom.Vec2{c.p1, c.p1.Add(x), c.p1.Add(x).Add(y), c.p1.Add(y)}, PreviewColor, 1, true)
	case mtText:
		t := c.entity(strings.Join(append(append([]string(nil), c.paras...), "_"), `\P`))
		for _, g := range t.Geometry() {
			dl.Curve(g, previewTol(ctx), PreviewColor, 1)
		}
		b := t.Box()
		dl.Polyline(b[:], PreviewColor.WithAlpha(120), 0, true)
	}
}

// registerMText adds MTEXT and its aliases T and MT.
func registerMText(r *cmdsys.Registry) {
	r.Register("MTEXT", func() cmdsys.Command { return &mtextCmd{} })
	r.Alias("T", "MTEXT")
	r.Alias("MT", "MTEXT")
}
