package cmdsys

import "go-draw/geom"

// This file registers the system variables that mirror state the session already keeps. They
// are live: Get reads the session's own field and Set writes it back, so the F8 toggle, the
// aperture setting and a typed ORTHOMODE never disagree. The rest of the table in
// _docs/inbox/autocad-commands.md section 1.11 is registered by the package that uses each
// variable (command defaults in cmdsys/commands, drafting aids by WP3), each in its own file
// through RegisterVarGroup; the stored no-op variables are the SETVAR package's.
func init() { RegisterVarGroup(coreVars) }

func coreVars(s *Session, v *Vars) {
	v.Register(Var{Name: "ORTHOMODE", Kind: VarInt, Default: 0, Min: 0, Max: 1,
		Read:     func() any { return boolInt(s.Ortho) },
		OnChange: func(_, n any) { s.Ortho = n.(int) != 0 }})
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
