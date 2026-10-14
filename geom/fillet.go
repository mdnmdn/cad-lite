package geom

import (
	"errors"
	"math"
)

// Errors returned by FilletLines and ChamferLines.
var (
	// ErrParallel is returned for lines that never meet.
	ErrParallel = errors.New("lines are parallel")
	// ErrTooLarge is returned when the radius or distances leave nothing of a line.
	ErrTooLarge = errors.New("radius or distance is too large for the lines")
)

// Corner is the result of cutting two lines back to a common corner.
// Each line keeps the part from its Far end up to its Join point; the
// part on the corner side of Join is discarded. At most one of Arc and
// Bevel is set; both nil means a sharp corner.
type Corner struct {
	// Join1 and Join2 are the new ends of line 1 and line 2 at the corner.
	Join1, Join2 Vec2
	// Far1 and Far2 are the retained ends of the lines, unchanged.
	Far1, Far2 Vec2
	// Arc is the fillet arc (counter-clockwise, tangent to both lines at
	// Join1 and Join2), nil for a sharp corner or a chamfer.
	Arc *Arc
	// Bevel is the chamfer line from Join1 to Join2, nil otherwise.
	Bevel *Line
}

// cornerRays resolves the geometry shared by FILLET and CHAMFER: the meeting
// point x of the infinite lines, the unit directions u1 and u2 leaving x
// towards the picked side of each line, and the retained far ends.
func cornerRays(l1, l2 Line, p1, p2 Vec2) (x, u1, u2, f1, f2 Vec2, err error) {
	d1, d2 := l1.B.Sub(l1.A), l2.B.Sub(l2.A)
	den := d1.Cross(d2)
	if den == 0 || math.Abs(den) <= 1e-12*d1.Len()*d2.Len() {
		return x, u1, u2, f1, f2, ErrParallel
	}
	t := l2.A.Sub(l1.A).Cross(d2) / den
	x = l1.A.Add(d1.Scale(t))
	side := func(l Line, p Vec2) (u, far Vec2) {
		u = l.B.Sub(l.A).Norm()
		if p.Sub(x).Dot(u) < 0 {
			u = u.Scale(-1)
		}
		far = l.A
		if l.B.Sub(x).Dot(u) > l.A.Sub(x).Dot(u) {
			far = l.B
		}
		return u, far
	}
	u1, f1 = side(l1, p1)
	u2, f2 = side(l2, p2)
	return x, u1, u2, f1, f2, nil
}

// FilletLines rounds the corner of the infinite lines l1 and l2 with an
// arc of radius r, keeping on each line the side that the pick points p1
// and p2 lie on. A zero radius gives a sharp corner at the intersection.
func FilletLines(l1, l2 Line, p1, p2 Vec2, r float64) (Corner, error) {
	x, u1, u2, f1, f2, err := cornerRays(l1, l2, p1, p2)
	if err != nil {
		return Corner{}, err
	}
	c := Corner{Join1: x, Join2: x, Far1: f1, Far2: f2}
	if r <= 0 {
		return c, nil
	}
	half := math.Acos(math.Max(-1, math.Min(1, u1.Dot(u2)))) / 2
	t := r / math.Tan(half)
	if f1.Sub(x).Dot(u1) <= t+1e-12 || f2.Sub(x).Dot(u2) <= t+1e-12 {
		return Corner{}, ErrTooLarge
	}
	c.Join1, c.Join2 = x.Add(u1.Scale(t)), x.Add(u2.Scale(t))
	ctr := x.Add(u1.Add(u2).Norm().Scale(r / math.Sin(half)))
	a1, a2 := c.Join1.Sub(ctr).Angle(), c.Join2.Sub(ctr).Angle()
	if c.Join1.Sub(ctr).Cross(c.Join2.Sub(ctr)) < 0 {
		a1, a2 = a2, a1
	}
	c.Arc = &Arc{C: ctr, R: r, Start: NormAngle(a1), End: NormAngle(a2)}
	return c, nil
}

// ChamferLines bevels the corner of the infinite lines l1 and l2, cutting
// d1 back along line 1 and d2 along line 2 from their intersection, and
// keeping the picked side of each line. Two zero distances give a sharp
// corner at the intersection.
func ChamferLines(l1, l2 Line, p1, p2 Vec2, d1, d2 float64) (Corner, error) {
	x, u1, u2, f1, f2, err := cornerRays(l1, l2, p1, p2)
	if err != nil {
		return Corner{}, err
	}
	c := Corner{Join1: x, Join2: x, Far1: f1, Far2: f2}
	if d1 <= 0 && d2 <= 0 {
		return c, nil
	}
	if f1.Sub(x).Dot(u1) <= d1+1e-12 || f2.Sub(x).Dot(u2) <= d2+1e-12 {
		return Corner{}, ErrTooLarge
	}
	c.Join1, c.Join2 = x.Add(u1.Scale(d1)), x.Add(u2.Scale(d2))
	c.Bevel = &Line{c.Join1, c.Join2}
	return c, nil
}
