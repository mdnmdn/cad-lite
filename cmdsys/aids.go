package cmdsys

import (
	"math"

	"cad-lite/geom"
)

// This file is the drafting-aid state a Session keeps besides Snaps, SnapOn and Ortho (which stay
// public fields so the UI and the F-keys need no accessor): the grid, snap-to-grid and the
// OSMODE, GRIDMODE, SNAPMODE, GRIDUNIT, SNAPUNIT variables that read and write them. The ORTHO,
// GRID, SNAP and -OSNAP commands (cmdsys/commands/aids.go) go through those variables, so a
// command, SETVAR and an F-key never disagree.

// aidState is the Session's drafting-aid state.
type aidState struct {
	grid, snapGrid     bool
	gridUnit, snapUnit geom.Vec2
	ov                 overrideState
	// polar, polarAng, autosnap and lwDisplay are polar.go's: polar tracking, its increment in
	// degrees, the AUTOSNAP bits and the LWT display flag.
	polar, lwDisplay bool
	polarAng         float64
	autosnap         int
}

func init() { RegisterVarGroup(aidVars) }

func aidVars(s *Session, v *Vars) {
	s.aids.grid = true
	s.aids.gridUnit, s.aids.snapUnit = geom.V(10, 10), geom.V(10, 10)
	v.Register(Var{Name: "OSMODE", Kind: VarInt, Default: 4133, Min: 0, Max: 32767,
		Read: func() any {
			n := int(s.Snaps)
			if !s.SnapOn {
				n |= osmodeOff
			}
			return n
		},
		OnChange: func(_, n any) {
			m := n.(int)
			s.Snaps, s.SnapOn = SnapMode(m&^osmodeOff), m&osmodeOff == 0
		}})
	v.Register(Var{Name: "GRIDMODE", Kind: VarInt, Default: 1, Min: 0, Max: 1,
		Read:     func() any { return boolInt(s.aids.grid) },
		OnChange: func(_, n any) { s.aids.grid = n.(int) != 0 }})
	v.Register(Var{Name: "SNAPMODE", Kind: VarInt, Default: 0, Min: 0, Max: 1,
		Read:     func() any { return boolInt(s.aids.snapGrid) },
		OnChange: func(_, n any) { s.aids.snapGrid = n.(int) != 0 }})
	v.Register(Var{Name: "GRIDUNIT", Kind: VarPoint, Default: geom.V(10, 10),
		Read:     func() any { return s.aids.gridUnit },
		OnChange: func(_, n any) { s.aids.gridUnit = n.(geom.Vec2) }})
	v.Register(Var{Name: "SNAPUNIT", Kind: VarPoint, Default: geom.V(10, 10),
		Read:     func() any { return s.aids.snapUnit },
		OnChange: func(_, n any) { s.aids.snapUnit = n.(geom.Vec2) }})
	v.Register(Var{Name: "GRIDMAJOR", Kind: VarInt, Default: 5, Min: 1, Max: 100})
}

// Grid reports whether the grid is shown (GRIDMODE, F7).
func (s *Session) Grid() bool { return s.aids.grid }

// SetGrid shows or hides the grid.
func (s *Session) SetGrid(on bool) { s.aids.grid = on }

// SnapGrid reports whether the cursor snaps to the snap grid (SNAPMODE, F9).
func (s *Session) SnapGrid() bool { return s.aids.snapGrid }

// SetSnapGrid turns snap-to-grid on or off.
func (s *Session) SetSnapGrid(on bool) { s.aids.snapGrid = on }

// GridSpacing returns the grid spacing in x and y (GRIDUNIT); a non-positive setting reads as 10.
func (s *Session) GridSpacing() geom.Vec2 { return positiveUnit(s.aids.gridUnit) }

// SnapSpacing returns the snap grid spacing in x and y (SNAPUNIT); a non-positive setting reads as 10.
func (s *Session) SnapSpacing() geom.Vec2 { return positiveUnit(s.aids.snapUnit) }

// GridMajor returns how many grid lines lie between the stronger ones (GRIDMAJOR).
func (s *Session) GridMajor() int {
	if n := s.vars.Int("GRIDMAJOR"); n >= 1 {
		return n
	}
	return 5
}

func positiveUnit(u geom.Vec2) geom.Vec2 {
	if !(u.X > 0) || math.IsInf(u.X, 0) {
		u.X = 10
	}
	if !(u.Y > 0) || math.IsInf(u.Y, 0) {
		u.Y = 10
	}
	return u
}

// snapToGrid rounds p to the snap grid.
func (s *Session) snapToGrid(p geom.Vec2) geom.Vec2 {
	u := s.SnapSpacing()
	return geom.V(math.Round(p.X/u.X)*u.X, math.Round(p.Y/u.Y)*u.Y)
}
