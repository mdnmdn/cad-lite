package geom

import "math"

// Tol is the tolerance policy for geometric comparisons. Tolerances are
// explicit and drawing-relative; nothing in geom compares floats exactly.
type Tol struct {
	// Linear is the distance below which two points are equal.
	Linear float64
	// Angular is the angle in radians below which two directions are equal.
	Angular float64
}

// DefaultTol returns the tolerance for a drawing whose extent (the larger
// side of its bounding box) is extent: 1e-9 × extent, at least 1e-10.
func DefaultTol(extent float64) Tol {
	return Tol{Linear: math.Max(1e-9*extent, 1e-10), Angular: 1e-10}
}

// NormAngle returns a reduced to [0, 2π).
func NormAngle(a float64) float64 {
	a = math.Mod(a, 2*math.Pi)
	if a < 0 {
		a += 2 * math.Pi
	}
	return a
}
