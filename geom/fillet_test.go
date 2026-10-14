package geom

import (
	"errors"
	"math"
	"testing"
)

func TestFilletLinesRightAngle(t *testing.T) {
	l1 := Line{V(0, 0), V(10, 0)}
	l2 := Line{V(0, 0), V(0, 10)}
	c, err := FilletLines(l1, l2, V(5, 0), V(0, 5), 2)
	if err != nil {
		t.Fatal(err)
	}
	vNear(t, "join1", c.Join1, V(2, 0))
	vNear(t, "join2", c.Join2, V(0, 2))
	vNear(t, "far1", c.Far1, V(10, 0))
	vNear(t, "far2", c.Far2, V(0, 10))
	if c.Arc == nil || c.Bevel != nil {
		t.Fatalf("corner = %+v", c)
	}
	vNear(t, "centre", c.Arc.C, V(2, 2))
	if c.Arc.R != 2 || math.Abs(c.Arc.Sweep()-math.Pi/2) > 1e-9 {
		t.Fatalf("arc = %+v sweep %v", c.Arc, c.Arc.Sweep())
	}
	// The arc is tangent: its end points are the join points.
	// The arc is counter-clockwise, so it runs from join2 to join1.
	vNear(t, "arc start", c.Arc.PointAt(c.Arc.Start), V(0, 2))
	vNear(t, "arc end", c.Arc.PointAt(c.Arc.Start+c.Arc.Sweep()), V(2, 0))
}

func TestFilletLinesObtuseAndExtend(t *testing.T) {
	// Lines that do not reach each other: extended to the corner. 120° corner.
	l1 := Line{V(5, 0), V(10, 0)}
	l2 := Line{V(-3, 3*math.Sqrt(3)), V(-1.5, 1.5*math.Sqrt(3))} // on the 120° ray from the origin
	c, err := FilletLines(l1, l2, V(8, 0), V(-3, 3*math.Sqrt(3)), 0)
	if err != nil {
		t.Fatal(err)
	}
	vNear(t, "sharp1", c.Join1, V(0, 0))
	vNear(t, "sharp2", c.Join2, V(0, 0))
	if c.Arc != nil {
		t.Fatal("radius 0 must not make an arc")
	}
	c, err = FilletLines(l1, Line{V(-3, 3*math.Sqrt(3)), V(-6, 6*math.Sqrt(3))}, V(8, 0), V(-4, 4*math.Sqrt(3)), 1)
	if err != nil {
		t.Fatal(err)
	}
	// Half the 120° opening is 60°: tangent length = r / tan(60°).
	tl := 1 / math.Sqrt(3)
	vNear(t, "join1", c.Join1, V(tl, 0))
	if math.Abs(c.Join2.Len()-tl) > 1e-9 {
		t.Fatalf("join2 = %v", c.Join2)
	}
	for _, p := range []Vec2{c.Join1, c.Join2} {
		if math.Abs(p.Dist(c.Arc.C)-1) > 1e-9 {
			t.Fatalf("join %v not on the arc", p)
		}
	}
}

func TestFilletLinesPickSideChoosesWedge(t *testing.T) {
	l1 := Line{V(-10, 0), V(10, 0)}
	l2 := Line{V(0, -10), V(0, 10)}
	c, err := FilletLines(l1, l2, V(-5, 0), V(0, -5), 1)
	if err != nil {
		t.Fatal(err)
	}
	vNear(t, "join1", c.Join1, V(-1, 0))
	vNear(t, "join2", c.Join2, V(0, -1))
	vNear(t, "far1", c.Far1, V(-10, 0))
	vNear(t, "far2", c.Far2, V(0, -10))
	vNear(t, "centre", c.Arc.C, V(-1, -1))
	if c.Arc.Sweep() > math.Pi {
		t.Fatalf("fillet took the long way round: sweep %v", c.Arc.Sweep())
	}
}

func TestFilletLinesErrors(t *testing.T) {
	if _, err := FilletLines(Line{V(0, 0), V(10, 0)}, Line{V(0, 1), V(10, 1)}, V(5, 0), V(5, 1), 1); !errors.Is(err, ErrParallel) {
		t.Fatalf("parallel err = %v", err)
	}
	if _, err := FilletLines(Line{V(0, 0), V(10, 0)}, Line{V(0, 0), V(0, 10)}, V(5, 0), V(0, 5), 20); !errors.Is(err, ErrTooLarge) {
		t.Fatalf("too large err = %v", err)
	}
}

func TestChamferLines(t *testing.T) {
	l1 := Line{V(0, 0), V(10, 0)}
	l2 := Line{V(0, 0), V(0, 10)}
	c, err := ChamferLines(l1, l2, V(5, 0), V(0, 5), 2, 3)
	if err != nil {
		t.Fatal(err)
	}
	vNear(t, "join1", c.Join1, V(2, 0))
	vNear(t, "join2", c.Join2, V(0, 3))
	if c.Bevel == nil || c.Arc != nil {
		t.Fatalf("corner = %+v", c)
	}
	vNear(t, "bevel a", c.Bevel.A, V(2, 0))
	vNear(t, "bevel b", c.Bevel.B, V(0, 3))
	c, err = ChamferLines(l1, l2, V(5, 0), V(0, 5), 0, 0)
	if err != nil || c.Bevel != nil {
		t.Fatalf("zero distances: %+v %v", c, err)
	}
	if _, err := ChamferLines(l1, l2, V(5, 0), V(0, 5), 20, 1); !errors.Is(err, ErrTooLarge) {
		t.Fatalf("too large err = %v", err)
	}
}
