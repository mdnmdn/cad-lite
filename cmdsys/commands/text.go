package commands

import (
	"math"
	"strings"

	"cad-lite/cmdsys"
	"cad-lite/doc"
	"cad-lite/geom"
	"cad-lite/render"
)

// --- TEXT ---------------------------------------------------------------

// TEXT keeps AutoCAD's variables: TEXTSIZE (the height it offers), TEXTSTYLE (the current style)
// and TEXTJUSTIFY (the justification it starts with, an option name such as L, C or TL).
// DRAW_TEXTANGLE is the rotation it offered last, in radians: command memory AutoCAD keeps
// outside the variable list.
func init() {
	cmdsys.RegisterVarGroup(func(_ *cmdsys.Session, v *cmdsys.Vars) {
		v.Register(cmdsys.Var{Name: "TEXTSIZE", Kind: cmdsys.VarReal, Default: 2.5})
		v.Register(cmdsys.Var{Name: "TEXTSTYLE", Kind: cmdsys.VarText, Default: doc.DefaultTextStyle})
		v.Register(cmdsys.Var{Name: "TEXTJUSTIFY", Kind: cmdsys.VarText, Default: "L"})
		v.Register(cmdsys.Var{Name: "DRAW_TEXTANGLE", Kind: cmdsys.VarReal, Default: 0.0})
	})
}

// textLineSpacing is the distance between successive lines of TEXT, in text heights.
const textLineSpacing = 5.0 / 3

// textJustifyKeywords are the options of the Justify prompt, in AutoCAD's order.
var textJustifyKeywords = []string{"Left", "Center", "Right", "Align", "Middle", "Fit", "TL", "TC", "TR", "ML", "MC", "MR", "BL", "BC", "BR"}

type textStep int

const (
	textStart   textStep = iota // start point or [Justify/Style]
	textJustify                 // justification option
	textStyle                   // style name or [?]
	textSecond                  // Align and Fit: second endpoint of the baseline
	textHeight                  // height
	textAngle                   // rotation angle
	textValue                   // the text itself, one line per answer
)

// textCmd is TEXT (alias DT, and DTEXT): single-line text, one TEXT entity per line typed, each
// line below the last, until an empty line.
type textCmd struct {
	base
	step   textStep
	h      doc.TextHAlign
	v      doc.TextVAlign
	style  string
	p1, p2 geom.Vec2
	height float64
	angle  float64
}

func (*textCmd) Name() string { return "TEXT" }

func (c *textCmd) Start(ctx *cmdsys.Context) *cmdsys.Prompt {
	c.style = ctx.Vars().Text("TEXTSTYLE")
	if c.style == "" {
		c.style = doc.DefaultTextStyle
	}
	var ok bool
	if c.h, c.v, ok = doc.ParseJustify(ctx.Vars().Text("TEXTJUSTIFY")); !ok {
		c.h, c.v = doc.TextLeft, doc.TextBaseline
	}
	c.height = ctx.Vars().Real("TEXTSIZE")
	c.angle = ctx.Vars().Real("DRAW_TEXTANGLE")
	ctx.Say("cmd.text.msg.current", c.style, ctx.Fmt(c.height), doc.JustifyName(c.h, c.v))
	return c.prompt(ctx)
}

func (c *textCmd) pair() bool { return c.h == doc.TextAligned || c.h == doc.TextFit }

// pointKey is the message of the point prompt for the current justification.
func (c *textCmd) pointKey() string {
	switch c.h {
	case doc.TextAligned, doc.TextFit:
		return "cmd.text.prompt.first_endpoint"
	case doc.TextMiddle:
		return "cmd.text.prompt.middle"
	}
	if c.v == doc.TextBaseline {
		return [...]string{"cmd.text.prompt.start", "cmd.text.prompt.center", "cmd.text.prompt.right"}[c.h]
	}
	return "cmd.text.prompt." + strings.ToLower(doc.JustifyName(c.h, c.v))
}

// lastText returns the TEXT the latest command made last, which Enter at the first prompt
// continues below.
func lastText(ctx *cmdsys.Context) *doc.Text {
	h, ok := ctx.LastEntity()
	if !ok {
		return nil
	}
	t, _ := ctx.Doc.Entity(h).(*doc.Text)
	return t
}

func (c *textCmd) prompt(ctx *cmdsys.Context) *cmdsys.Prompt {
	switch c.step {
	case textJustify:
		return &cmdsys.Prompt{Message: ctx.T("cmd.text.prompt.justify"), Accepts: cmdsys.InputKeyword, Keywords: textJustifyKeywords, NoOr: true}
	case textStyle:
		return &cmdsys.Prompt{Message: ctx.T("cmd.text.prompt.style"), Accepts: cmdsys.InputText | cmdsys.InputKeyword, Keywords: []string{"?"},
			NoOr: true, Default: &cmdsys.Input{Kind: cmdsys.InputText, Text: c.style}}
	case textSecond:
		return &cmdsys.Prompt{Message: ctx.T("cmd.text.prompt.second_endpoint"), Accepts: cmdsys.InputPoint, Base: pt(c.p1)}
	case textHeight:
		p := &cmdsys.Prompt{Message: ctx.T("cmd.text.prompt.height"), Accepts: cmdsys.InputDistance, Base: pt(c.p1)}
		if c.height > 0 {
			p.Default = &cmdsys.Input{Kind: cmdsys.InputDistance, Value: c.height}
		}
		return p
	case textAngle:
		return &cmdsys.Prompt{Message: ctx.T("cmd.text.prompt.rotation"), Accepts: cmdsys.InputAngle, Base: pt(c.p1),
			Default: &cmdsys.Input{Kind: cmdsys.InputAngle, Value: c.angle}}
	case textValue:
		return &cmdsys.Prompt{Message: ctx.T("cmd.text.prompt.text"), Accepts: cmdsys.InputText | cmdsys.InputEnter, Literal: true}
	}
	p := &cmdsys.Prompt{Message: ctx.T(c.pointKey()), Accepts: cmdsys.InputPoint | cmdsys.InputKeyword, Keywords: []string{"Justify", "Style"}}
	if lastText(ctx) != nil {
		p.Accepts |= cmdsys.InputEnter
	}
	return p
}

func (c *textCmd) Input(ctx *cmdsys.Context, in cmdsys.Input) *cmdsys.Prompt {
	switch c.step {
	case textStart:
		switch {
		case in.Keyword == "Justify":
			c.step = textJustify
		case in.Keyword == "Style":
			c.step = textStyle
		case in.Kind == cmdsys.InputEnter:
			c.continueBelow(ctx)
		case in.Kind == cmdsys.InputPoint:
			c.p1 = in.Point
			switch {
			case c.pair():
				c.step = textSecond
			default:
				c.step = textHeight
			}
		}
	case textJustify:
		if h, v, ok := doc.ParseJustify(in.Keyword); ok {
			c.h, c.v = h, v
			_ = ctx.Vars().Set("TEXTJUSTIFY", justifyVar(h, v))
		}
		c.step = textStart
	case textStyle:
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
		c.step = textStart
	case textSecond:
		if in.Point.Near(c.p1, 1e-12) {
			ctx.Say("cmd.text.msg.points_differ")
			break
		}
		c.p2 = in.Point
		c.step = textValue
		if c.h == doc.TextFit {
			c.step = textHeight
		}
	case textHeight:
		if !(in.Value > 0) || math.IsInf(in.Value, 0) {
			ctx.Say("cmd.common.msg.positive")
			break
		}
		c.height = in.Value
		_ = ctx.Vars().Set("TEXTSIZE", in.Value)
		c.step = textAngle
		if c.pair() {
			c.step = textValue
		}
	case textAngle:
		c.angle = in.Value
		_ = ctx.Vars().Set("DRAW_TEXTANGLE", in.Value)
		c.step = textValue
	case textValue:
		if in.Kind != cmdsys.InputText || in.Text == "" {
			return nil
		}
		c.add(ctx, in.Text)
	}
	return c.prompt(ctx)
}

// add makes one line of text at the current position and moves the position one line down.
func (c *textCmd) add(ctx *cmdsys.Context, value string) {
	t := c.entity(value)
	ctx.Tx().Add(t)
	down := t.Layout().Dir().Perp().Scale(-textLineSpacing * t.Layout().Height)
	c.p1, c.p2 = c.p1.Add(down), c.p2.Add(down)
}

// entity returns the text the command would make with value at the current position.
func (c *textCmd) entity(value string) *doc.Text {
	t := doc.NewText(c.p1, c.height, c.angle, value)
	t.Style = c.style
	t.HAlign, t.VAlign = c.h, c.v
	switch {
	case c.pair():
		t.Align = c.p2
	case c.h != doc.TextLeft || c.v != doc.TextBaseline:
		t.Align = c.p1
		t.Insert = t.Layout().Origin
	}
	return t
}

// continueBelow sets the command up to write under the last TEXT made, with its style,
// justification, height and rotation, as AutoCAD does for Enter at the first prompt.
func (c *textCmd) continueBelow(ctx *cmdsys.Context) {
	t := lastText(ctx)
	if t == nil {
		return
	}
	lay := t.Layout()
	down := lay.Dir().Perp().Scale(-textLineSpacing * lay.Height)
	c.h, c.v, c.style = t.HAlign, t.VAlign, t.Style
	c.height, c.angle = lay.Height, lay.Rotation
	if c.pair() {
		c.p1, c.p2 = t.Insert.Add(down), t.Align.Add(down)
		if c.h == doc.TextAligned {
			c.height = t.Height
		}
	} else {
		c.p1 = t.Anchor().Add(down)
	}
	c.step = textValue
}

// justifyVar is the TEXTJUSTIFY value of a justification: its option shortcut.
func justifyVar(h doc.TextHAlign, v doc.TextVAlign) string {
	n := doc.JustifyName(h, v)
	if len(n) > 2 {
		return n[:1]
	}
	return n
}

func (c *textCmd) Preview(ctx *cmdsys.Context, cur geom.Vec2, dl *render.DisplayList) {
	// The lines already written are entities of the open transaction, which the canvas draws.
	switch c.step {
	case textSecond, textHeight, textAngle:
		dl.Polyline([]geom.Vec2{c.p1, cur}, PreviewColor.WithAlpha(120), 0, false)
	case textValue:
		// The text cursor: a box one capital high and a third as wide at the insertion point.
		t := c.entity("")
		lay := t.Layout()
		d, u := lay.Dir(), lay.Dir().Perp()
		o := lay.Origin
		w := lay.Height / 3
		if c.pair() {
			w = c.p2.Dist(c.p1)
		}
		dl.Polyline([]geom.Vec2{o, o.Add(d.Scale(w)), o.Add(d.Scale(w)).Add(u.Scale(lay.Height)), o.Add(u.Scale(lay.Height))}, PreviewColor, 1, true)
	}
}

// registerText adds TEXT and its aliases DT and DTEXT.
func registerText(r *cmdsys.Registry) {
	r.Register("TEXT", func() cmdsys.Command { return &textCmd{} })
	r.Alias("DT", "TEXT")
	r.Alias("DTEXT", "TEXT")
}
