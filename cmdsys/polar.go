package cmdsys

import (
	"math"

	"cad-lite/geom"
)

// polarTolerance is how far (degrees) the rubber-band angle may be from a multiple of POLARANG
// for polar tracking to lock onto it.
const polarTolerance = 5.0

// polarAutosnap is the AUTOSNAP bit that switches polar tracking on, as in AutoCAD.
const polarAutosnap = 8

func init() { RegisterVarGroup(polarVars) }

func polarVars(s *Session, v *Vars) {
	v.Register(Var{Name: "POLARANG", Kind: VarReal, Default: 45.0, Min: 1, Max: 180,
		Read:     func() any { return s.aids.polarAng },
		OnChange: func(_, n any) { s.aids.polarAng = n.(float64) }})
	// AUTOSNAP is live only for the polar bit; the other bits are kept as set. The default has
	// polar tracking off, unlike the AutoCAD value 63.
	v.Register(Var{Name: "AUTOSNAP", Kind: VarInt, Default: 55, Min: 0, Max: 127,
		Read: func() any {
			n := s.aids.autosnap &^ polarAutosnap
			if s.aids.polar {
				n |= polarAutosnap
			}
			return n
		},
		OnChange: func(_, n any) {
			s.aids.autosnap = n.(int)
			s.SetPolar(s.aids.autosnap&polarAutosnap != 0)
		}})
	v.Register(Var{Name: "LWDISPLAY", Kind: VarInt, Default: 0, Min: 0, Max: 1,
		Read:     func() any { return boolInt(s.aids.lwDisplay) },
		OnChange: func(_, n any) { s.aids.lwDisplay = n.(int) != 0 }})
	s.aids.polarAng, s.aids.autosnap = 45, 55
}

// Polar reports whether polar tracking is on (F10, the POLAR status toggle, AUTOSNAP bit 8).
func (s *Session) Polar() bool { return s.aids.polar }

// SetPolar turns polar tracking on or off. Ortho and polar tracking exclude each other, so
// turning polar on turns ortho off.
func (s *Session) SetPolar(on bool) {
	s.aids.polar = on
	if on {
		s.Ortho = false
	}
}

// SetOrtho turns ortho on or off; turning it on turns polar tracking off.
func (s *Session) SetOrtho(on bool) {
	s.Ortho = on
	if on {
		s.aids.polar = false
	}
}

// PolarAngle returns the polar tracking increment in degrees (POLARANG).
func (s *Session) PolarAngle() float64 {
	if a := s.aids.polarAng; a >= 1 && a <= 180 {
		return a
	}
	return 45
}

// LineweightDisplay reports whether lineweights are drawn (LWDISPLAY, the LWT status toggle).
func (s *Session) LineweightDisplay() bool { return s.aids.lwDisplay }

// SetLineweightDisplay shows or hides lineweights.
func (s *Session) SetLineweightDisplay(on bool) { s.aids.lwDisplay = on }

// polarConstrain moves p onto the polar ray through base whose angle is the multiple of the
// increment nearest to p's, when p is within polarTolerance of it; otherwise p is unchanged. The
// angle is absolute, measured from the X axis.
func (s *Session) polarConstrain(p, base geom.Vec2) geom.Vec2 {
	d := p.Sub(base)
	r := math.Hypot(d.X, d.Y)
	if r == 0 {
		return p
	}
	inc := s.PolarAngle()
	deg := math.Atan2(d.Y, d.X) * 180 / math.Pi
	snapped := math.Round(deg/inc) * inc
	if math.Abs(snapped-deg) > polarTolerance {
		return p
	}
	rad := snapped * math.Pi / 180
	u := geom.V(math.Cos(rad), math.Sin(rad))
	t := d.X*u.X + d.Y*u.Y
	return base.Add(u.Scale(t))
}
