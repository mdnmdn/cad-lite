package cmdsys

import (
	"errors"
	"path"
	"path/filepath"
	"strconv"
	"strings"

	"cad-lite/geom"
	"cad-lite/render"
)

// This file registers the system variables that mirror state the session already keeps, the
// stored placeholders for the AutoCAD variables no package has claimed, and the SETVAR machinery
// (SETVAR itself, and a variable typed by name at Command:, which is a one-prompt SETVAR).
//
// The live variables read and write the session's own field, so the F8 toggle, the aperture
// setting and a typed ORTHOMODE never disagree. Command defaults are registered by the package
// that uses them (cmdsys/commands), drafting aids by WP3, each from its own file through
// RegisterVarGroup; registerStoredVars fills in, last, every name of _docs/inbox/autocad-commands.md
// section 1.11 that nobody has registered.
func init() { RegisterVarGroup(coreVars) }

func coreVars(s *Session, v *Vars) {
	v.Register(Var{Name: "ORTHOMODE", Kind: VarInt, Default: 0, Min: 0, Max: 1,
		Read:     func() any { return boolInt(s.Ortho) },
		OnChange: func(_, n any) { s.SetOrtho(n.(int) != 0) }})
	v.Register(Var{Name: "APERTURE", Kind: VarInt, Default: 8, Min: 1, Max: 50,
		Read:     func() any { return int(s.Aperture) },
		OnChange: func(_, n any) { s.Aperture = float64(n.(int)) }})
	v.Register(Var{Name: "LASTPOINT", Kind: VarPoint, Default: geom.Vec2{},
		Read:     func() any { return s.mem.Point() },
		OnChange: func(_, n any) { s.mem.SetPoint(n.(geom.Vec2)) }})
	v.Register(Var{Name: "CLAYER", Kind: VarText, Default: "0",
		Read: func() any { return s.ctx.Doc.CurrentLayer },
		OnChange: func(_, n any) {
			if name := n.(string); s.ctx.Doc.Layer(name) != nil {
				s.ctx.Doc.CurrentLayer = name
			}
		}})
	v.Register(Var{Name: "LUPREC", Kind: VarInt, Default: 4, Min: 0, Max: 8})
	v.Register(Var{Name: "AUPREC", Kind: VarInt, Default: 0, Min: 0, Max: 8})
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// storedVar is a placeholder variable: it reads back what was set and changes nothing else.
type storedVar struct {
	name     string
	kind     VarKind
	def      any
	min, max float64
	readOnly bool
}

// storedVars are the AutoCAD variables (acadiso.dwt defaults) a script or a drafter may set that
// no Cad-lite package owns yet: the ones with no meaning here (FILEDIA, MIRRTEXT, DIMSCALE) are
// permanent no-ops, the rest hold the value until their package registers the real variable, which
// then takes the name (registerStoredVars skips a registered name). Angle variables (POLARANG,
// SNAPANG, ANGBASE, CHAMFERD) are not listed: their unit is the units package's.
var storedVars = []storedVar{
	// Command line and scripts.
	{name: "CMDECHO", kind: VarInt, def: 1, max: 1},
	{name: "ERRNO", kind: VarInt, def: 0, max: 1 << 20},
	{name: "FILEDIA", kind: VarInt, def: 1, max: 1},
	{name: "CMDDIA", kind: VarInt, def: 1, max: 2},
	{name: "EXPERT", kind: VarInt, def: 0, max: 5},
	{name: "NOMUTT", kind: VarInt, def: 0, max: 1},
	{name: "DYNMODE", kind: VarInt, def: 3, min: -3, max: 3},
	{name: "SHORTCUTMENU", kind: VarInt, def: 11, max: 31},
	{name: "MIRRTEXT", kind: VarInt, def: 0, max: 1},
	{name: "UCSFOLLOW", kind: VarInt, def: 0, max: 1},
	{name: "DIMSCALE", kind: VarReal, def: 1.0},
	{name: "TEXTSIZE", kind: VarReal, def: 2.5},
	{name: "FILLMODE", kind: VarInt, def: 1, max: 1},
	{name: "BLIPMODE", kind: VarInt, def: 0, max: 1},
	{name: "REGENMODE", kind: VarInt, def: 1, max: 1},
	// Drafting aids (WP3 registers the live ones).
	{name: "OSMODE", kind: VarInt, def: 4133, max: 32767},
	{name: "PICKBOX", kind: VarInt, def: 3, max: 50},
	{name: "POLARMODE", kind: VarInt, def: 0, max: 15},
	{name: "GRIDMODE", kind: VarInt, def: 1, max: 1},
	{name: "GRIDUNIT", kind: VarPoint, def: geom.V(10, 10)},
	{name: "GRIDMAJOR", kind: VarInt, def: 5, min: 1, max: 100},
	{name: "SNAPMODE", kind: VarInt, def: 0, max: 1},
	{name: "SNAPUNIT", kind: VarPoint, def: geom.V(10, 10)},
	{name: "SNAPTYPE", kind: VarInt, def: 0, max: 1},
	{name: "SNAPSTYL", kind: VarInt, def: 0, max: 1},
	// Selection (WP2 registers the live ones).
	{name: "PICKFIRST", kind: VarInt, def: 1, max: 1},
	{name: "PICKADD", kind: VarInt, def: 2, max: 2},
	{name: "PICKAUTO", kind: VarInt, def: 1, max: 1},
	{name: "PICKDRAG", kind: VarInt, def: 0, max: 1},
	{name: "PICKSTYLE", kind: VarInt, def: 1, max: 3},
	{name: "HIGHLIGHT", kind: VarInt, def: 1, max: 1},
	{name: "SELECTIONPREVIEW", kind: VarInt, def: 3, max: 3},
	// Command defaults (the command packages register the ones they use).
	{name: "CIRCLERAD", kind: VarReal, def: 0.0},
	{name: "CHAMFERC", kind: VarReal, def: 0.0},
	{name: "CHAMMODE", kind: VarInt, def: 0, max: 1},
	{name: "TRIMMODE", kind: VarInt, def: 1, max: 1},
	{name: "TRIMEXTENDMODE", kind: VarInt, def: 1, max: 1},
	{name: "EDGEMODE", kind: VarInt, def: 0, max: 1},
	{name: "PROJMODE", kind: VarInt, def: 1, max: 2},
	{name: "OFFSETGAPTYPE", kind: VarInt, def: 0, max: 2},
	{name: "POLYSIDES", kind: VarInt, def: 4, min: 3, max: 1024},
	{name: "PLINEWID", kind: VarReal, def: 0.0},
	{name: "PLINEGEN", kind: VarInt, def: 0, max: 1},
	{name: "COPYMODE", kind: VarInt, def: 0, max: 1},
	{name: "ELEVATION", kind: VarReal, def: 0.0},
	{name: "THICKNESS", kind: VarReal, def: 0.0},
	// Current properties (the layer and property commands register the real ones).
	{name: "CECOLOR", kind: VarText, def: "BYLAYER"},
	{name: "CELTYPE", kind: VarText, def: "BYLAYER"},
	{name: "CELWEIGHT", kind: VarInt, def: -1, min: -3, max: 211},
	{name: "LTSCALE", kind: VarReal, def: 1.0},
	// Units (the units package registers the real ones).
	{name: "LUNITS", kind: VarInt, def: 2, min: 1, max: 5},
	{name: "AUNITS", kind: VarInt, def: 0, max: 4},
	{name: "INSUNITS", kind: VarInt, def: 4, max: 24},
	{name: "UNITMODE", kind: VarInt, def: 0, max: 1},
	// Inquiry results (the inquiry commands Store them).
	{name: "DISTANCE", kind: VarReal, def: 0.0, readOnly: true},
	{name: "AREA", kind: VarReal, def: 0.0, readOnly: true},
	{name: "PERIMETER", kind: VarReal, def: 0.0, readOnly: true},
}

// registerStoredVars adds the stored placeholders and the live read-only state variables
// (VIEWCTR, VIEWSIZE, SCREENSIZE, LASTPROMPT, CMDNAMES, CMDACTIVE, DWGNAME, DWGPREFIX) for every
// name no package registered, so the two never collide whatever the init order.
func registerStoredVars(s *Session) {
	v := s.vars
	add := func(d Var) {
		if !v.Has(d.Name) {
			v.Register(d)
		}
	}
	for _, sv := range storedVars {
		add(Var{Name: sv.name, Kind: sv.kind, Default: sv.def, Min: sv.min, Max: sv.max, ReadOnly: sv.readOnly})
	}
	view := func() render.View {
		if s.ctx.View == nil {
			return render.View{Scale: 1}
		}
		return *s.ctx.View
	}
	add(Var{Name: "VIEWCTR", Kind: VarPoint, Default: geom.Vec2{}, ReadOnly: true,
		Read: func() any { return view().Center }})
	add(Var{Name: "VIEWSIZE", Kind: VarReal, Default: 0.0, ReadOnly: true,
		Read: func() any {
			if vw := view(); vw.Scale > 0 {
				return vw.Height / vw.Scale
			}
			return 0.0
		}})
	add(Var{Name: "SCREENSIZE", Kind: VarPoint, Default: geom.Vec2{}, ReadOnly: true,
		Read: func() any { vw := view(); return geom.V(vw.Width, vw.Height) }})
	add(Var{Name: "LASTPROMPT", Kind: VarText, Default: "", ReadOnly: true,
		Read: func() any { return s.Prompt() }})
	add(Var{Name: "CMDNAMES", Kind: VarText, Default: "", ReadOnly: true,
		Read: func() any { return s.cmdNames() }})
	add(Var{Name: "CMDACTIVE", Kind: VarInt, Default: 0, ReadOnly: true,
		Read: func() any { return boolInt(s.active != nil) }})
	add(Var{Name: "DWGNAME", Kind: VarText, Default: "", ReadOnly: true,
		Read: func() any {
			if s.filePath == "" {
				return "Drawing1.dxf"
			}
			return filepath.Base(s.filePath)
		}})
	add(Var{Name: "DWGPREFIX", Kind: VarText, Default: "", ReadOnly: true,
		Read: func() any {
			if s.filePath == "" {
				return ""
			}
			return filepath.Dir(s.filePath) + string(filepath.Separator)
		}})
}

// cmdNames is CMDNAMES: the running commands from the outermost, a transparent one after a
// quote ("LINE'ZOOM").
func (s *Session) cmdNames() string {
	var parts []string
	for _, f := range s.cl.frames {
		parts = append(parts, f.cmd.Name())
	}
	if s.active != nil {
		parts = append(parts, s.active.Name())
	}
	return strings.Join(parts, "'")
}

// --- SETVAR and variables typed by name ---------------------------------

const (
	setvarName = iota
	setvarValue
	setvarList
)

// setvarCmd is SETVAR, and the one-prompt command a variable typed by name runs.
type setvarCmd struct {
	name  string
	phase int
	typed bool
}

// NewSetvar returns the SETVAR command: it asks for a variable name (or ? to list them), then
// for the new value. cmdsys/commands registers it; a variable typed by name at Command: runs the
// same value prompt without the name step.
func NewSetvar() Command { return &setvarCmd{} }

func newVarCommand(name string) Command {
	return &setvarCmd{name: name, phase: setvarValue, typed: true}
}

func (c *setvarCmd) Name() string {
	if c.typed {
		return c.name
	}
	return "SETVAR"
}

// Transparent lets SETVAR and a variable name run at another command's prompt ('SETVAR).
func (*setvarCmd) Transparent() bool { return true }

func (c *setvarCmd) Start(ctx *Context) *Prompt {
	if c.typed {
		return c.valuePrompt(ctx)
	}
	return c.namePrompt(ctx)
}

func (c *setvarCmd) namePrompt(ctx *Context) *Prompt {
	p := &Prompt{Message: ctx.T("session.setvar.prompt_name"), Accepts: InputText | InputKeyword, Keywords: []string{"?"}}
	if last := ctx.last.setvar; last != "" {
		p.Default = &Input{Kind: InputText, Text: last}
	}
	return p
}

func (c *setvarCmd) valuePrompt(ctx *Context) *Prompt {
	return &Prompt{Message: ctx.T("session.setvar.prompt_value", c.name), Accepts: InputText,
		Default: &Input{Kind: InputText, Text: varDisplay(ctx, c.name)}}
}

func (c *setvarCmd) Input(ctx *Context, in Input) *Prompt {
	vars := ctx.Vars()
	switch c.phase {
	case setvarName:
		if in.Kind == InputKeyword {
			c.phase = setvarList
			return &Prompt{Message: ctx.T("session.setvar.prompt_list"), Accepts: InputText,
				Default: &Input{Kind: InputText, Text: "*"}}
		}
		name := strings.ToUpper(strings.TrimSpace(in.Text))
		if !vars.Has(name) {
			ctx.Say("session.setvar.unknown")
			return c.namePrompt(ctx)
		}
		c.name, c.phase = name, setvarValue
		ctx.last.setvar = name
		return c.valuePrompt(ctx)
	case setvarList:
		listVars(ctx, in.Text)
		return nil
	}
	if in.Text == varDisplay(ctx, c.name) {
		return nil
	}
	text := in.Text
	if d, ok := vars.Lookup(c.name); ok && d.Kind == VarText {
		text = strings.Trim(text, `"`)
	}
	err := vars.SetString(c.name, text)
	if err == nil {
		return nil
	}
	if errors.Is(err, ErrVarReadOnly) {
		ctx.Say("session.setvar.read_only", c.name)
		return nil
	}
	ctx.Say(VarErrorKey(err, vars, c.name), varErrorArgs(err, vars, c.name)...)
	return c.valuePrompt(ctx)
}

func (*setvarCmd) Preview(*Context, geom.Vec2, *render.DisplayList) {}
func (*setvarCmd) Cancel(*Context)                                  {}

// VarErrorKey returns the catalog key of the sentence SETVAR prints when assigning to variable
// name failed with err: the type of value it needs, the range, read-only or unknown (spec 1.10,
// verify V28). varErrorArgs gives the arguments of the range sentences.
func VarErrorKey(err error, v *Vars, name string) string {
	d, ok := v.Lookup(name)
	switch {
	case errors.Is(err, ErrVarUnknown):
		return "session.setvar.unknown"
	case errors.Is(err, ErrVarReadOnly):
		return "session.setvar.read_only"
	case !ok:
		return MsgInvalidValue
	case errors.Is(err, ErrVarRange):
		if d.Kind == VarInt {
			return "session.setvar.need_int_range"
		}
		return "session.setvar.need_real_range"
	case d.Kind == VarInt:
		return "session.setvar.need_int"
	case d.Kind == VarPoint:
		return MsgInvalidPoint
	}
	return "session.setvar.need_real"
}

func varErrorArgs(err error, v *Vars, name string) []any {
	d, ok := v.Lookup(name)
	switch {
	case errors.Is(err, ErrVarRange) && ok:
		return []any{FormatNumber(d.Min), FormatNumber(d.Max)}
	case errors.Is(err, ErrVarReadOnly):
		return []any{name}
	}
	return nil
}

// varDisplay is a variable's value as the value prompt's <default> and SETVAR ? show it: reals
// with the unit precision, text in quotes, a point as x,y,0.
func varDisplay(ctx *Context, name string) string {
	vars := ctx.Vars()
	d, ok := vars.Lookup(name)
	if !ok {
		return ""
	}
	switch d.Kind {
	case VarReal:
		return ctx.Fmt(vars.Real(name))
	case VarText:
		return strconv.Quote(vars.Text(name))
	case VarPoint:
		return FormatPoint(vars.Point(name)) + ",0"
	}
	return vars.String(name)
}

// listVars prints "NAME  value" for each variable matching the comma-separated patterns of
// SETVAR ? (* and ? wildcards), read-only ones marked.
func listVars(ctx *Context, patterns string) {
	vars := ctx.Vars()
	for _, name := range vars.Names() {
		for _, pat := range strings.Split(patterns, ",") {
			if ok, _ := path.Match(strings.ToUpper(strings.TrimSpace(pat)), name); !ok {
				continue
			}
			line := ctx.T("session.setvar.list_line", name, varDisplay(ctx, name))
			if d, _ := vars.Lookup(name); d.ReadOnly {
				line += ctx.T("session.setvar.read_only_mark")
			}
			ctx.out(line)
			break
		}
	}
}
