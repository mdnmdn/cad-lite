package commands

import (
	"strings"

	"cad-lite/cmdsys"
	"cad-lite/doc"
)

// The ids of the keywords of CHPROP and the MATCHPROP settings; the catalog gives each its display
// text, shortcut and Italian name.
const (
	kwColor   = "Color"
	kwChLayer = "chLayer"
	kwChLtype = "chLtype"
	kwLtScale = "ltScale"
	kwLWeight = "LWeight"
)

// chpropKeywords are CHPROP's properties in AutoCAD's order (Thickness, TRansparency, Material and
// Annotative are not modelled).
var chpropKeywords = []string{kwColor, kwChLayer, kwChLtype, kwLtScale, kwLWeight}

// colorText renders a colour index as the CECOLOR value: BYBLOCK (0), BYLAYER or the number.
func colorText(c int) string { return cmdsys.ColorText(c) }

// setVar sets a system variable from a command and prints SETVAR's sentence when the value is
// refused; it reports success.
func setVar(ctx *cmdsys.Context, name string, value any) bool {
	err := ctx.Vars().Set(name, value)
	if err != nil {
		ctx.Say(cmdsys.VarErrorKey(err, ctx.Vars(), name))
	}
	return err == nil
}

// --- CHPROP -------------------------------------------------------------

type chpropStep int

const (
	cpSelect chpropStep = iota
	cpProperty
	cpColor
	cpLayer
	cpLtype
	cpLtScale
	cpLWeight
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
	return &cmdsys.Prompt{Message: ctx.T("cmd.chprop.prompt.property"), Accepts: cmdsys.InputKeyword | cmdsys.InputEnter, Keywords: chpropKeywords, NoOr: true}
}

// first returns the common properties of the first selected object that still exists, or nil.
func (c *chpropCmd) first(ctx *cmdsys.Context) *doc.EntityCommon {
	for _, h := range c.hs {
		if e := ctx.Doc.Entity(h); e != nil {
			return e.Common()
		}
	}
	return nil
}

func textPrompt(msg, def string) *cmdsys.Prompt {
	d := cmdsys.Input{Kind: cmdsys.InputText, Text: def}
	return &cmdsys.Prompt{Message: msg, Accepts: cmdsys.InputText | cmdsys.InputEnter, Default: &d}
}

func (c *chpropCmd) Input(ctx *cmdsys.Context, in cmdsys.Input) *cmdsys.Prompt {
	switch c.step {
	case cpSelect:
		if !c.take(ctx, in) {
			return nil
		}
		return c.property(ctx)
	case cpProperty:
		first := c.first(ctx)
		if first == nil {
			first = &doc.EntityCommon{Layer: ctx.Doc.CurrentLayer, Color: doc.ByLayer}
		}
		switch in.Keyword {
		case kwColor:
			c.step = cpColor
			return textPrompt(ctx.T("cmd.chprop.prompt.color"), colorText(first.Color))
		case kwChLayer:
			c.step = cpLayer
			return &cmdsys.Prompt{Message: ctx.T("cmd.chprop.prompt.layer", first.Layer), Accepts: cmdsys.InputText | cmdsys.InputEnter,
				Default: &cmdsys.Input{Kind: cmdsys.InputText, Text: first.Layer}}
		case kwChLtype:
			c.step = cpLtype
			return textPrompt(ctx.T("cmd.chprop.prompt.ltype"), cmdsys.LinetypeText(first.Linetype))
		case kwLtScale:
			c.step = cpLtScale
			def := cmdsys.Input{Kind: cmdsys.InputDistance, Value: first.LtScale()}
			return &cmdsys.Prompt{Message: ctx.T("cmd.chprop.prompt.ltscale"), Accepts: cmdsys.InputDistance, Default: &def}
		case kwLWeight:
			c.step = cpLWeight
			return textPrompt(ctx.T("cmd.chprop.prompt.lweight"), cmdsys.LineweightText(first.Lineweight))
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
			ctx.Say("cmd.chprop.msg.bad_color")
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
	case cpLtype:
		if in.Kind == cmdsys.InputText {
			name, ok := cmdsys.CanonicalLinetype(ctx.Doc, in.Text)
			if !ok {
				ctx.Say("cmd.linetype.msg.not_loaded", strings.TrimSpace(in.Text))
			} else {
				c.setAll(ctx, func(cm *doc.EntityCommon) { cm.Linetype = name })
			}
		}
	case cpLtScale:
		if in.Value <= 0 {
			ctx.Say("cmd.ltscale.msg.positive")
		} else {
			v := in.Value
			c.setAll(ctx, func(cm *doc.EntityCommon) { cm.LinetypeScale = v })
		}
	case cpLWeight:
		if in.Kind == cmdsys.InputText {
			w, ok := cmdsys.ParseLineweight(in.Text)
			if !ok {
				ctx.Say("cmd.lweight.msg.bad")
			} else {
				c.setAll(ctx, func(cm *doc.EntityCommon) { cm.Lineweight = w })
			}
		}
	}
	return c.property(ctx)
}

// setAll applies fn to the common fields of every selected object that may be edited and that fn
// changes.
func (c *chpropCmd) setAll(ctx *cmdsys.Context, fn func(*doc.EntityCommon)) {
	for _, h := range c.hs {
		e := ctx.Doc.Entity(h)
		if e == nil || !editable(ctx, e) {
			continue
		}
		probe := *e.Common()
		fn(&probe)
		if probe != *e.Common() {
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
	return textPrompt(ctx.T("cmd.color.prompt.color"), colorText(ctx.Doc.CurrentColor))
}

func (c *colorCmd) Input(ctx *cmdsys.Context, in cmdsys.Input) *cmdsys.Prompt {
	if in.Kind != cmdsys.InputText {
		return nil
	}
	if _, ok := ParseColor(in.Text); !ok {
		ctx.Say("cmd.chprop.msg.bad_color")
		return nil
	}
	setVar(ctx, "CECOLOR", in.Text)
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
	setVar(ctx, "LTSCALE", in.Value)
	return nil
}

// --- -LINETYPE ----------------------------------------------------------

// The -LINETYPE options. AutoCAD's Create, which writes a .lin file, is not offered.
const (
	ltList = "?"
	ltLoad = "Load"
	ltSet  = "Set"
)

var linetypeOptions = []string{ltList, ltLoad, ltSet}

type linetypeStep int

const (
	ltsOption linetypeStep = iota
	ltsList
	ltsLoad
	ltsSet
)

// linetypeCmd is -LINETYPE, and LINETYPE, which has no Linetype Manager here and asks the same:
// it lists loaded linetypes, loads them from the built-in acadiso.lin library
// (Txn.LoadLibraryLinetype) and sets CELTYPE, repeating the option prompt until Enter.
type linetypeCmd struct {
	base
	name string
	step linetypeStep
}

func (c *linetypeCmd) Name() string { return c.name }

func (c *linetypeCmd) Start(ctx *cmdsys.Context) *cmdsys.Prompt {
	ctx.Say("cmd.linetype.msg.current", ctx.Vars().Text("CELTYPE"))
	return c.options(ctx)
}

func (c *linetypeCmd) options(ctx *cmdsys.Context) *cmdsys.Prompt {
	c.step = ltsOption
	return &cmdsys.Prompt{Message: ctx.T("cmd.linetype.prompt.option"), Accepts: cmdsys.InputKeyword | cmdsys.InputEnter, Keywords: linetypeOptions, NoOr: true}
}

func (c *linetypeCmd) setPrompt(ctx *cmdsys.Context) *cmdsys.Prompt {
	c.step = ltsSet
	def := cmdsys.Input{Kind: cmdsys.InputText, Text: ctx.Vars().Text("CELTYPE")}
	return &cmdsys.Prompt{Message: ctx.T("cmd.linetype.prompt.set"), Accepts: cmdsys.InputText | cmdsys.InputKeyword | cmdsys.InputEnter,
		Keywords: []string{ltList}, Default: &def}
}

func (c *linetypeCmd) Input(ctx *cmdsys.Context, in cmdsys.Input) *cmdsys.Prompt {
	switch c.step {
	case ltsOption:
		switch in.Keyword {
		case ltList:
			c.step = ltsList
			return textPrompt(ctx.T("cmd.linetype.prompt.list"), "*")
		case ltLoad:
			c.step = ltsLoad
			return &cmdsys.Prompt{Message: ctx.T("cmd.linetype.prompt.load"), Accepts: cmdsys.InputText | cmdsys.InputEnter}
		case ltSet:
			return c.setPrompt(ctx)
		}
		return nil
	case ltsList:
		pat := "*"
		if in.Kind == cmdsys.InputText {
			pat = in.Text
		}
		listLinetypes(ctx, pat)
	case ltsLoad:
		if in.Kind == cmdsys.InputText {
			loadLinetypes(ctx, in.Text)
		}
	case ltsSet:
		switch {
		case in.Kind == cmdsys.InputKeyword:
			listLinetypes(ctx, "*")
			return c.setPrompt(ctx)
		case in.Kind == cmdsys.InputText:
			name := strings.TrimSpace(in.Text)
			if _, ok := cmdsys.CanonicalLinetype(ctx.Doc, name); !ok {
				ctx.Say("cmd.linetype.msg.not_loaded", name)
				return c.setPrompt(ctx)
			}
			setVar(ctx, "CELTYPE", name)
		}
	}
	return c.options(ctx)
}

// listLinetypes prints the loaded linetypes whose names match the comma-separated patterns.
func listLinetypes(ctx *cmdsys.Context, patterns string) {
	ctx.Say("cmd.linetype.msg.list_header")
	for _, l := range ctx.Doc.Linetypes() {
		if matchAny(patterns, l.Name) {
			ctx.Say("cmd.linetype.msg.row", l.Name, l.Description)
		}
	}
}

// loadLinetypes loads the library linetypes matching the comma-separated names and wildcards.
func loadLinetypes(ctx *cmdsys.Context, patterns string) {
	for _, pat := range strings.Split(patterns, ",") {
		pat = strings.TrimSpace(pat)
		if pat == "" {
			continue
		}
		found := false
		for _, l := range doc.LibraryLinetypes() {
			if !wildcardMatch(pat, l.Name) {
				continue
			}
			found = true
			if _, ok := ctx.Doc.Linetype(l.Name); ok {
				ctx.Say("cmd.linetype.msg.already", l.Name)
				continue
			}
			if err := ctx.Tx().LoadLibraryLinetype(l.Name); err == nil {
				ctx.Say("cmd.linetype.msg.loaded", l.Name)
			}
		}
		if !found {
			ctx.Say("cmd.linetype.msg.not_found", pat)
		}
	}
}

// matchAny reports whether name matches one of the comma-separated wildcard patterns.
func matchAny(patterns, name string) bool {
	for _, p := range strings.Split(patterns, ",") {
		if p = strings.TrimSpace(p); p != "" && wildcardMatch(p, name) {
			return true
		}
	}
	return false
}

// --- LWEIGHT ------------------------------------------------------------

// lweightCmd is -LWEIGHT, and LWEIGHT, which has no Lineweight Settings dialog here and asks the
// same: it sets CELWEIGHT, the lineweight of new objects.
type lweightCmd struct {
	base
	name string
}

func (c *lweightCmd) Name() string { return c.name }

func (c *lweightCmd) Start(ctx *cmdsys.Context) *cmdsys.Prompt {
	cur := cmdsys.LineweightText(ctx.Doc.CurrentLineweight)
	ctx.Say("cmd.lweight.msg.current", cur)
	return c.prompt(ctx, cur)
}

func (c *lweightCmd) prompt(ctx *cmdsys.Context, cur string) *cmdsys.Prompt {
	def := cmdsys.Input{Kind: cmdsys.InputText, Text: cur}
	return &cmdsys.Prompt{Message: ctx.T("cmd.lweight.prompt.value"), Accepts: cmdsys.InputText | cmdsys.InputKeyword | cmdsys.InputEnter,
		Keywords: []string{ltList}, Default: &def}
}

func (c *lweightCmd) Input(ctx *cmdsys.Context, in cmdsys.Input) *cmdsys.Prompt {
	switch in.Kind {
	case cmdsys.InputKeyword:
		parts := []string{"BYLAYER", "BYBLOCK", "DEFAULT"}
		for _, w := range doc.StandardLineweights {
			if w == 0 {
				parts = append(parts, cmdsys.LineweightText(doc.LineweightZero))
				continue
			}
			parts = append(parts, cmdsys.LineweightText(doc.Lineweight(w)))
		}
		ctx.Say("cmd.lweight.msg.list", strings.Join(parts, " "))
		return c.prompt(ctx, cmdsys.LineweightText(ctx.Doc.CurrentLineweight))
	case cmdsys.InputText:
		w, ok := cmdsys.ParseLineweight(in.Text)
		if !ok {
			ctx.Say("cmd.lweight.msg.bad")
			return c.prompt(ctx, cmdsys.LineweightText(ctx.Doc.CurrentLineweight))
		}
		setVar(ctx, "CELWEIGHT", w.DXF())
	}
	return nil
}

// registerProps adds CHPROP, -COLOR, LTSCALE, -LINETYPE and -LWEIGHT (with COLOR, LINETYPE and
// LWEIGHT asking the same, since their dialogs do not exist here).
func registerProps(r *cmdsys.Registry) {
	r.Register("CHPROP", func() cmdsys.Command { return &chpropCmd{} })
	r.Register("COLOR", func() cmdsys.Command { return &colorCmd{name: "COLOR"} })
	r.Alias("COL", "COLOR")
	cmdsys.RegisterCLI(r, "COLOR", func() cmdsys.Command { return &colorCmd{name: "-COLOR"} }, "COL")
	r.Register("LTSCALE", func() cmdsys.Command { return &ltscaleCmd{} })
	r.Alias("LTS", "LTSCALE")
	r.Register("LINETYPE", func() cmdsys.Command { return &linetypeCmd{name: "LINETYPE"} })
	r.Alias("LT", "LINETYPE")
	r.Alias("LTYPE", "LINETYPE")
	cmdsys.RegisterCLI(r, "LINETYPE", func() cmdsys.Command { return &linetypeCmd{name: "-LINETYPE"} }, "LT")
	r.Register("LWEIGHT", func() cmdsys.Command { return &lweightCmd{name: "LWEIGHT"} })
	r.Alias("LW", "LWEIGHT")
	r.Alias("LINEWEIGHT", "LWEIGHT")
	cmdsys.RegisterCLI(r, "LWEIGHT", func() cmdsys.Command { return &lweightCmd{name: "-LWEIGHT"} }, "LW")
}
