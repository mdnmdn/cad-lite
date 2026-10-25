package commands

import (
	"slices"
	"strconv"
	"strings"

	"cad-lite/cmdsys"
	"cad-lite/doc"
	"cad-lite/geom"
)

// --- LAYER --------------------------------------------------------------

// layerCmd is LAYER without the dash: no Layer Properties Manager exists as a command here, so
// it keeps the free-text form, a name that becomes current (created on demand), and "name=color".
type layerCmd struct{ base }

func (*layerCmd) Name() string { return "LAYER" }

func (*layerCmd) Start(ctx *cmdsys.Context) *cmdsys.Prompt {
	return &cmdsys.Prompt{Message: ctx.T("cmd.layer.prompt.name", ctx.Doc.CurrentLayer), Accepts: cmdsys.InputText | cmdsys.InputEnter}
}

func (*layerCmd) Input(ctx *cmdsys.Context, in cmdsys.Input) *cmdsys.Prompt {
	if in.Kind == cmdsys.InputEnter {
		return nil
	}
	name, col, hasColor := strings.Cut(in.Text, "=")
	name = strings.TrimSpace(name)
	color := 7
	if hasColor {
		n, err := strconv.Atoi(strings.TrimSpace(col))
		if err != nil || n < 1 || n > 255 {
			ctx.Say("cmd.layer.msg.bad_color")
			return nil
		}
		color = n
	}
	l := ctx.Doc.Layer(name)
	if l == nil {
		if !validLayerName(name) {
			ctx.Say("cmd.layer.msg.bad_name")
			return nil
		}
		if l = addLayer(ctx, name, color); l == nil {
			return nil
		}
		ctx.Say("cmd.layer.msg.created", name)
	} else if hasColor {
		modifyLayer(ctx, l, func(l *doc.Layer) { l.Color = color })
	}
	setCurrentLayer(ctx, l)
	return nil
}

// Transparent marks LAYER as runnable inside another command (cmdsys.Transparent).
func (*layerCmd) Transparent() bool { return true }

// --- -LAYER -------------------------------------------------------------

// The -LAYER keywords. Each id is its English spelling; the catalog ([kw]) gives the display
// text, shortcut and Italian name.
const (
	lyList   = "?"
	lyMake   = "Make"
	lySet    = "Set"
	lyNew    = "New"
	lyOn     = "ON"
	lyOff    = "OFF"
	lyColor  = "Color"
	lyLtype  = "Ltype"
	lyLWt    = "LWeight"
	lyPlot   = "Plot"
	lyFreeze = "Freeze"
	lyThaw   = "Thaw"
	lyLock   = "LOck"
	lyUnlock = "Unlock"
	lyDesc   = "Description"
	lyNoPlot = "No plot"
)

var layerOptions = []string{lyList, lyMake, lySet, lyNew, lyOn, lyOff, lyColor, lyLtype, lyLWt, lyPlot, lyFreeze, lyThaw, lyLock, lyUnlock, lyDesc}

// layerStep is what -LAYER waits for.
type layerStep int

const (
	lsOption layerStep = iota // the option prompt
	lsNames                   // a layer name list for a state option
	lsList                    // the name list of "?"
	lsMake
	lsSet
	lsNew
	lsReallyOff
	lsColor      // the colour, then lsNames with the colour applied
	lsLtype      // the linetype
	lsLWeight    // the lineweight
	lsPlot       // Plot or No plot
	lsDescName   // the layer to describe
	lsDescText   // its description
	lsApplyNames // layer names for the value collected so far
)

type layerCLI struct {
	base
	step layerStep
	op   string
	// color, ltype, lweight, plot are the value collected before the layer names.
	color   int
	ltype   string
	lweight int
	noPlot  bool
	// pending holds the layers an OFF waits to confirm; desc the layer to describe.
	pending []*doc.Layer
	desc    *doc.Layer
	// picking is set while a name prompt waits for an object instead (Enter at the prompt).
	picking bool
}

func (*layerCLI) Name() string { return "-LAYER" }

func (c *layerCLI) Start(ctx *cmdsys.Context) *cmdsys.Prompt {
	ctx.Say("cmd.layer.msg.current", ctx.Doc.CurrentLayer)
	return c.options(ctx)
}

// pickPrompt asks for an object whose layer the option works on: the <select object> default of
// the name prompts, reached by Enter (a typed number there is a layer name, not a point).
func (c *layerCLI) pickPrompt(ctx *cmdsys.Context) *cmdsys.Prompt {
	c.picking = true
	return &cmdsys.Prompt{Message: ctx.T("cmd.layer.prompt.pick"), Accepts: cmdsys.InputPoint | cmdsys.InputEnter}
}

func (c *layerCLI) options(ctx *cmdsys.Context) *cmdsys.Prompt {
	c.step, c.op, c.picking = lsOption, "", false
	return &cmdsys.Prompt{Message: ctx.T("cmd.layer.prompt.option"), Accepts: cmdsys.InputKeyword | cmdsys.InputEnter, Keywords: layerOptions, NoOr: true}
}

func (c *layerCLI) text(ctx *cmdsys.Context, key string, def *cmdsys.Input, extra cmdsys.InputKind, args ...any) *cmdsys.Prompt {
	return &cmdsys.Prompt{Message: ctx.T(key, args...), Accepts: cmdsys.InputText | cmdsys.InputEnter | extra, Default: def}
}

func (c *layerCLI) Input(ctx *cmdsys.Context, in cmdsys.Input) *cmdsys.Prompt {
	switch c.step {
	case lsOption:
		return c.option(ctx, in)
	case lsList:
		pat := "*"
		if in.Kind == cmdsys.InputText {
			pat = in.Text
		}
		c.list(ctx, pat)
		return c.options(ctx)
	case lsMake:
		if in.Kind == cmdsys.InputText {
			c.make(ctx, in.Text)
		} else if l := ctx.Doc.Layer(ctx.Doc.CurrentLayer); l != nil {
			modifyLayer(ctx, l, func(l *doc.Layer) { l.Off, l.Frozen = false, false })
		}
		return c.options(ctx)
	case lsSet:
		return c.set(ctx, in)
	case lsNew:
		if in.Kind == cmdsys.InputText {
			c.newLayers(ctx, in.Text)
		}
		return c.options(ctx)
	case lsNames:
		return c.names(ctx, in)
	case lsReallyOff:
		c.reallyOff(ctx, in)
		return c.options(ctx)
	case lsColor:
		if in.Kind != cmdsys.InputText {
			return c.options(ctx)
		}
		n, ok := ParseColor(in.Text)
		if !ok || n == doc.ByLayer || n == doc.ByBlock {
			ctx.Say("cmd.layer.msg.bad_color")
			return c.options(ctx)
		}
		c.color, c.step = n, lsApplyNames
		return c.applyPrompt(ctx)
	case lsLtype:
		return c.ltypeValue(ctx, in)
	case lsLWeight:
		if in.Kind != cmdsys.InputText {
			return c.options(ctx)
		}
		w, ok := parseLineweight(in.Text)
		if !ok {
			ctx.Say("cmd.layer.msg.bad_lineweight")
			return c.options(ctx)
		}
		c.lweight, c.step = w, lsApplyNames
		return c.applyPrompt(ctx)
	case lsPlot:
		c.noPlot, c.step = in.Keyword == lyNoPlot, lsApplyNames
		return c.applyPrompt(ctx)
	case lsApplyNames:
		return c.apply(ctx, in)
	case lsDescName:
		if in.Kind != cmdsys.InputText {
			return c.options(ctx)
		}
		ls, _ := resolveLayers(ctx, in.Text)
		if len(ls) == 0 {
			return c.options(ctx)
		}
		c.desc, c.step = ls[0], lsDescText
		return c.text(ctx, "cmd.layer.prompt.description", nil, 0, c.desc.Name)
	case lsDescText:
		text := ""
		if in.Kind == cmdsys.InputText {
			text = in.Text
		}
		modifyLayer(ctx, c.desc, func(l *doc.Layer) { l.Description = text })
		return c.options(ctx)
	}
	return nil
}

func (c *layerCLI) option(ctx *cmdsys.Context, in cmdsys.Input) *cmdsys.Prompt {
	if in.Kind == cmdsys.InputEnter {
		return nil
	}
	c.op, c.picking = in.Keyword, false
	switch in.Keyword {
	case lyList:
		c.step = lsList
		return c.text(ctx, "cmd.layer.prompt.list", &cmdsys.Input{Kind: cmdsys.InputText, Text: "*"}, 0)
	case lyMake:
		c.step = lsMake
		cur := ctx.Doc.CurrentLayer
		return c.text(ctx, "cmd.layer.prompt.make", &cmdsys.Input{Kind: cmdsys.InputText, Text: cur}, 0)
	case lySet:
		c.step = lsSet
		return c.text(ctx, "cmd.layer.prompt.set", nil, 0)
	case lyNew:
		c.step = lsNew
		return c.text(ctx, "cmd.layer.prompt.new", nil, 0)
	case lyOn, lyOff, lyFreeze, lyThaw, lyLock, lyUnlock:
		c.step = lsNames
		return c.text(ctx, "cmd.layer.prompt.names_"+layerOpKey(in.Keyword), nil, 0)
	case lyColor:
		c.step = lsColor
		return c.text(ctx, "cmd.layer.prompt.color", nil, 0)
	case lyLtype:
		c.step = lsLtype
		return c.text(ctx, "cmd.layer.prompt.ltype", nil, 0)
	case lyLWt:
		c.step = lsLWeight
		return c.text(ctx, "cmd.layer.prompt.lweight", nil, 0)
	case lyPlot:
		c.step = lsPlot
		def := cmdsys.Input{Kind: cmdsys.InputKeyword, Keyword: lyPlot}
		return &cmdsys.Prompt{Message: ctx.T("cmd.layer.prompt.plot"), Accepts: cmdsys.InputKeyword, Keywords: []string{lyPlot, lyNoPlot}, Default: &def}
	case lyDesc:
		c.step = lsDescName
		return c.text(ctx, "cmd.layer.prompt.desc_name", nil, 0)
	}
	return c.options(ctx)
}

func layerOpKey(op string) string {
	return strings.ToLower(strings.ReplaceAll(op, " ", "_"))
}

// applyPrompt asks for the layers a collected colour, linetype, lineweight or plot flag applies
// to; Enter means the current layer.
func (c *layerCLI) applyPrompt(ctx *cmdsys.Context) *cmdsys.Prompt {
	cur := ctx.Doc.CurrentLayer
	def := &cmdsys.Input{Kind: cmdsys.InputText, Text: cur}
	switch c.op {
	case lyColor:
		return c.text(ctx, "cmd.layer.prompt.color_names", def, 0, c.color)
	case lyLtype:
		return c.text(ctx, "cmd.layer.prompt.ltype_names", def, 0)
	case lyLWt:
		return c.text(ctx, "cmd.layer.prompt.lweight_names", def, 0)
	}
	return c.text(ctx, "cmd.layer.prompt.plot_names", def, 0)
}

// apply sets the collected value on the named layers.
func (c *layerCLI) apply(ctx *cmdsys.Context, in cmdsys.Input) *cmdsys.Prompt {
	text := ctx.Doc.CurrentLayer
	if in.Kind == cmdsys.InputText {
		text = in.Text
	}
	ls, _ := resolveLayers(ctx, text)
	for _, l := range ls {
		modifyLayer(ctx, l, func(l *doc.Layer) {
			switch c.op {
			case lyColor:
				l.Color = c.color
			case lyLtype:
				l.Linetype = c.ltype
			case lyLWt:
				l.Lineweight = c.lweight
			case lyPlot:
				l.NoPlot = c.noPlot
			}
		})
	}
	return c.options(ctx)
}

func (c *layerCLI) names(ctx *cmdsys.Context, in cmdsys.Input) *cmdsys.Prompt {
	var ls []*doc.Layer
	switch in.Kind {
	case cmdsys.InputText:
		ls, _ = resolveLayers(ctx, in.Text)
	case cmdsys.InputPoint:
		if l := layerAt(ctx, in.Point); l != nil {
			ls = []*doc.Layer{l}
		} else {
			ctx.Say("cmd.layer.msg.nothing_picked")
		}
	default:
		if !c.picking {
			return c.pickPrompt(ctx)
		}
		return c.options(ctx)
	}
	cur := ctx.Doc.CurrentLayer
	switch c.op {
	case lyOn:
		for _, l := range ls {
			modifyLayer(ctx, l, func(l *doc.Layer) { l.Off = false })
		}
	case lyOff:
		if i := slices.IndexFunc(ls, func(l *doc.Layer) bool { return l.Name == cur }); i >= 0 {
			c.pending = ls
			c.step = lsReallyOff
			def := cmdsys.Input{Kind: cmdsys.InputKeyword, Keyword: "No"}
			return &cmdsys.Prompt{Message: ctx.T("cmd.layer.prompt.really_off", cur), Accepts: cmdsys.InputKeyword, Keywords: []string{"Yes", "No"}, Default: &def}
		}
		for _, l := range ls {
			modifyLayer(ctx, l, func(l *doc.Layer) { l.Off = true })
		}
	case lyFreeze:
		for _, l := range ls {
			if l.Name == cur {
				ctx.Say("cmd.layer.msg.freeze_current", l.Name)
				continue
			}
			modifyLayer(ctx, l, func(l *doc.Layer) { l.Frozen = true })
		}
	case lyThaw:
		for _, l := range ls {
			modifyLayer(ctx, l, func(l *doc.Layer) { l.Frozen = false })
		}
	case lyLock:
		for _, l := range ls {
			modifyLayer(ctx, l, func(l *doc.Layer) { l.Locked = true })
		}
	case lyUnlock:
		for _, l := range ls {
			modifyLayer(ctx, l, func(l *doc.Layer) { l.Locked = false })
		}
	}
	return c.options(ctx)
}

func (c *layerCLI) reallyOff(ctx *cmdsys.Context, in cmdsys.Input) {
	for _, l := range c.pending {
		if l.Name == ctx.Doc.CurrentLayer && in.Keyword != "Yes" {
			continue
		}
		modifyLayer(ctx, l, func(l *doc.Layer) { l.Off = true })
	}
	c.pending = nil
}

func (c *layerCLI) set(ctx *cmdsys.Context, in cmdsys.Input) *cmdsys.Prompt {
	var l *doc.Layer
	switch in.Kind {
	case cmdsys.InputText:
		if l = ctx.Doc.Layer(strings.TrimSpace(in.Text)); l == nil {
			ctx.Say("cmd.layer.msg.not_found", in.Text)
		}
	case cmdsys.InputPoint:
		if l = layerAt(ctx, in.Point); l == nil {
			ctx.Say("cmd.layer.msg.nothing_picked")
		}
	default:
		if !c.picking {
			return c.pickPrompt(ctx)
		}
		return c.options(ctx)
	}
	switch {
	case l == nil:
	case l.Frozen:
		ctx.Say("cmd.layer.msg.set_frozen", l.Name)
	default:
		setCurrentLayer(ctx, l)
	}
	return c.options(ctx)
}

func (c *layerCLI) make(ctx *cmdsys.Context, name string) {
	name = strings.TrimSpace(name)
	if !validLayerName(name) {
		ctx.Say("cmd.layer.msg.bad_name")
		return
	}
	l := ctx.Doc.Layer(name)
	if l == nil {
		if l = addLayer(ctx, name, 7); l == nil {
			return
		}
	}
	modifyLayer(ctx, l, func(l *doc.Layer) { l.Off, l.Frozen = false, false })
	setCurrentLayer(ctx, l)
}

func (c *layerCLI) newLayers(ctx *cmdsys.Context, list string) {
	for _, n := range strings.Split(list, ",") {
		n = strings.TrimSpace(n)
		switch {
		case n == "":
		case !validLayerName(n):
			ctx.Say("cmd.layer.msg.bad_name")
		case ctx.Doc.Layer(n) != nil:
			ctx.Say("cmd.layer.msg.exists", n)
		default:
			addLayer(ctx, n, 7)
		}
	}
}

func (c *layerCLI) ltypeValue(ctx *cmdsys.Context, in cmdsys.Input) *cmdsys.Prompt {
	name := "CONTINUOUS"
	if in.Kind == cmdsys.InputText {
		name = strings.TrimSpace(in.Text)
	}
	if name == "?" {
		ctx.Say("cmd.layer.msg.ltype_header")
		for _, n := range loadedLinetypes(ctx.Doc) {
			ctx.Printf("%s", n)
		}
		return c.options(ctx)
	}
	for _, n := range loadedLinetypes(ctx.Doc) {
		if strings.EqualFold(n, name) {
			if strings.EqualFold(n, "CONTINUOUS") {
				n = ""
			}
			c.ltype, c.step = n, lsApplyNames
			return c.applyPrompt(ctx)
		}
	}
	ctx.Say("cmd.layer.msg.ltype_not_loaded", name)
	return c.options(ctx)
}

// list prints the layers matching pat as a table.
func (c *layerCLI) list(ctx *cmdsys.Context, pat string) {
	ls, _ := resolveLayers(ctx, pat)
	if len(ls) == 0 {
		return
	}
	ctx.Say("cmd.layer.msg.list_header")
	for _, l := range ls {
		var st []string
		switch {
		case l.Frozen:
			st = append(st, ctx.T("cmd.layer.state.frozen"))
		case l.Off:
			st = append(st, ctx.T("cmd.layer.state.off"))
		default:
			st = append(st, ctx.T("cmd.layer.state.on"))
		}
		if l.Locked {
			st = append(st, ctx.T("cmd.layer.state.locked"))
		}
		if l.NoPlot {
			st = append(st, ctx.T("cmd.layer.state.noplot"))
		}
		lt := l.Linetype
		if lt == "" {
			lt = "Continuous"
		}
		ctx.Say("cmd.layer.msg.row", l.Name, strings.Join(st, ","), l.Color, lt)
	}
}

// --- helpers ------------------------------------------------------------

// modifyLayer applies fn to layer l as part of the command's undo step (Txn.ModifyLayer), and
// leaves the step alone when fn changes nothing.
func modifyLayer(ctx *cmdsys.Context, l *doc.Layer, fn func(*doc.Layer)) {
	probe := *l
	if fn(&probe); probe == *l {
		return
	}
	_ = ctx.TableTx().ModifyLayer(l.Name, fn)
}

// addLayer creates the named layer with color as part of the command's undo step; nil when the
// name is refused.
func addLayer(ctx *cmdsys.Context, name string, color int) *doc.Layer {
	l, err := ctx.TableTx().AddLayer(name, color)
	if err != nil {
		ctx.Say("cmd.layer.msg.bad_name")
		return nil
	}
	return l
}

// setCurrentLayer makes l current through CLAYER, as part of the command's undo step.
func setCurrentLayer(ctx *cmdsys.Context, l *doc.Layer) {
	if ctx.Doc.CurrentLayer != l.Name {
		_ = ctx.Vars().Set("CLAYER", l.Name)
	}
}

// validLayerName reports whether name is a legal layer name: non-empty and free of the
// characters AutoCAD reserves (< > / \ " : ; ? * | , = `).
func validLayerName(name string) bool {
	return name != "" && len(name) <= 255 && !strings.ContainsAny(name, "<>/\\\":;?*|,=`")
}

// resolveLayers expands a comma-separated list of names and wildcards (* and ?) into the
// matching layers, in table order without repeats. A pattern that matches nothing is reported to
// the history and returned in missing.
func resolveLayers(ctx *cmdsys.Context, list string) (out []*doc.Layer, missing []string) {
	for _, pat := range strings.Split(list, ",") {
		pat = strings.TrimSpace(pat)
		if pat == "" {
			continue
		}
		found := false
		for _, l := range ctx.Doc.Layers() {
			if wildcardMatch(pat, l.Name) {
				found = true
				if !slices.Contains(out, l) {
					out = append(out, l)
				}
			}
		}
		if !found {
			missing = append(missing, pat)
			ctx.Say("cmd.layer.msg.not_found", pat)
		}
	}
	return out, missing
}

// wildcardMatch matches name against pat case-insensitively; "*" is any run of characters and
// "?" any one character.
func wildcardMatch(pat, name string) bool {
	p, n := []rune(strings.ToUpper(pat)), []rune(strings.ToUpper(name))
	var rec func(i, j int) bool
	rec = func(i, j int) bool {
		for i < len(p) {
			switch p[i] {
			case '*':
				for k := j; k <= len(n); k++ {
					if rec(i+1, k) {
						return true
					}
				}
				return false
			case '?':
				if j >= len(n) {
					return false
				}
			default:
				if j >= len(n) || p[i] != n[j] {
					return false
				}
			}
			i, j = i+1, j+1
		}
		return j == len(n)
	}
	return rec(0, 0)
}

// pickAny returns the visible entity nearest p within the pick radius, locked layers included
// (a locked object may still be inspected or used as a source).
func pickAny(ctx *cmdsys.Context, p geom.Vec2) (doc.Handle, doc.Entity) {
	aperture := pickPixels * ctx.View.PixelSize()
	var best doc.Handle
	bestD := aperture
	for _, h := range ctx.Doc.Handles() {
		e := ctx.Doc.Entity(h)
		if e == nil || !ctx.Doc.EntityVisible(e) {
			continue
		}
		if _, unknown := e.(*doc.Unknown); unknown {
			continue
		}
		for _, g := range e.Geometry() {
			if d := geom.Closest(g, p).Dist(p); d <= bestD {
				best, bestD = h, d
			}
		}
	}
	if best == 0 {
		return 0, nil
	}
	return best, ctx.Doc.Entity(best)
}

// layerAt returns the layer of the object picked at p, or nil.
func layerAt(ctx *cmdsys.Context, p geom.Vec2) *doc.Layer {
	if _, e := pickAny(ctx, p); e != nil {
		return ctx.Doc.Layer(e.Common().Layer)
	}
	return nil
}

// ParseColor reads a colour answer; see cmdsys.ParseColor.
func ParseColor(text string) (int, bool) { return cmdsys.ParseColor(text) }

// lineweights are the standard lineweights in hundredths of a millimetre.
var lineweights = []int{0, 5, 9, 13, 15, 18, 20, 25, 30, 35, 40, 50, 53, 60, 70, 80, 90, 100, 106, 120, 140, 158, 200, 211}

// parseLineweight reads a lineweight in millimetres ("0.25", "0.25mm") or DEFAULT and returns it
// as Layer.Lineweight stores it (doc.Lineweight.LayerValue): hundredths, int(doc.LineweightZero)
// for 0.00 mm, 0 for DEFAULT; only the standard values are valid.
func parseLineweight(text string) (int, bool) {
	t := strings.ToUpper(strings.TrimSpace(text))
	if t == "DEFAULT" || t == "PREDEFINITO" {
		return 0, true
	}
	t = strings.TrimSpace(strings.TrimSuffix(t, "MM"))
	v, err := strconv.ParseFloat(strings.ReplaceAll(t, ",", "."), 64)
	if err != nil {
		return 0, false
	}
	w := int(v*100 + 0.5)
	if !slices.Contains(lineweights, w) {
		return 0, false
	}
	if w == 0 {
		return doc.LineweightZero.LayerValue(), true
	}
	return w, true
}

// loadedLinetypes returns the linetype names the drawing's linetype table has, CONTINUOUS first.
func loadedLinetypes(d *doc.Drawing) []string {
	var out []string
	for _, l := range d.Linetypes() {
		out = append(out, strings.ToUpper(l.Name))
	}
	return out
}

// registerLayer adds LAYER and -LAYER.
func registerLayer(r *cmdsys.Registry) {
	r.Register("LAYER", func() cmdsys.Command { return &layerCmd{} })
	cmdsys.RegisterCLI(r, "LAYER", func() cmdsys.Command { return &layerCLI{} }, "LA")
}
