package commands_test

import (
	"math"
	"strings"
	"testing"

	"cad-lite/cmdsys"
	"cad-lite/doc"
	"cad-lite/geom"
)

// drawFeed submits each item as one typed line ("" is Enter), the way a drafter types.
func drawFeed(s *cmdsys.Session, items ...string) {
	for _, it := range items {
		s.Submit(it)
	}
}

func plines(s *cmdsys.Session) []*doc.LWPolyline { return entities[*doc.LWPolyline](s) }

func arcNear(t *testing.T, a *doc.Arc, c geom.Vec2, r, s, e float64) {
	t.Helper()
	d := func(x float64) float64 { return x * math.Pi / 180 }
	if !a.Center.Near(c, 1e-9) || math.Abs(a.Radius-r) > 1e-9 ||
		math.Abs(geom.NormAngle(a.StartAngle-d(s))) > 1e-9 && math.Abs(geom.NormAngle(a.StartAngle-d(s))-2*math.Pi) > 1e-9 ||
		math.Abs(geom.NormAngle(a.EndAngle-d(e))) > 1e-9 && math.Abs(geom.NormAngle(a.EndAngle-d(e))-2*math.Pi) > 1e-9 {
		t.Fatalf("arc = %+v, want %v r%v %v..%v", a, c, r, s, e)
	}
}

func TestLineEnterContinuesFromLastEnd(t *testing.T) {
	s := newSession(t)
	s.Script("LINE 0,0 50,0 ")
	s.Script("LINE ")
	s.Script("50,50 ")
	ls := entities[*doc.Line](s)
	if len(ls) != 2 || !ls[1].Start.Near(geom.V(50, 0), 1e-9) || !ls[1].End.Near(geom.V(50, 50), 1e-9) {
		t.Fatalf("lines = %+v", ls)
	}
}

func TestLineAfterArcAsksTangentLength(t *testing.T) {
	s := newSession(t)
	s.Script("ARC 0,0 5,5 10,0")
	s.Script("LINE ")
	if !strings.Contains(s.Prompt(), "line length") {
		t.Fatalf("prompt = %q", s.Prompt())
	}
	s.Script("10")
	ls := entities[*doc.Line](s)
	if len(ls) != 1 || !ls[0].Start.Near(geom.V(10, 0), 1e-9) || !ls[0].End.Near(geom.V(10, -10), 1e-9) {
		t.Fatalf("lines = %+v", ls)
	}
}

func TestCircleOptions(t *testing.T) {
	s := newSession(t)
	s.Script("CIRCLE 3P 0,0 10,10 20,0")
	s.Script("CIRCLE 2P 0,0 10,0")
	s.Script("CIRCLE 50,0 D 10")
	cs := entities[*doc.Circle](s)
	if len(cs) != 3 {
		t.Fatalf("circles = %d", len(cs))
	}
	near(t, "3P centre", cs[0].Center, geom.V(10, 0))
	near(t, "2P centre", cs[1].Center, geom.V(5, 0))
	if cs[1].Radius != 5 || cs[2].Radius != 5 {
		t.Fatalf("radii %v %v", cs[1].Radius, cs[2].Radius)
	}
	if got := s.Vars().Real("CIRCLERAD"); got != 5 {
		t.Fatalf("CIRCLERAD = %v", got)
	}
	s.Script("CIRCLE 0,100")
	if !strings.Contains(s.Prompt(), "<5>") {
		t.Fatalf("prompt lacks the remembered radius: %q", s.Prompt())
	}
	s.Submit("")
	if cs = entities[*doc.Circle](s); len(cs) != 4 || cs[3].Radius != 5 {
		t.Fatalf("default radius not used: %+v", cs)
	}
}

func TestCircleTtr(t *testing.T) {
	s := newSession(t)
	drawFeed(s, "LINE", "0,0", "100,0", "", "LINE", "0,0", "0,100", "")
	s.Script("CIRCLE TTR 50,0 0,50 10")
	cs := entities[*doc.Circle](s)
	if len(cs) != 1 {
		t.Fatalf("circles = %d", len(cs))
	}
	near(t, "ttr centre", cs[0].Center, geom.V(10, 10))
	// A radius no circle fits reports and draws nothing.
	s.Script("CIRCLE TTR 50,0 50,0 10")
	s.Script("CIRCLE TTR 20,0 80,0")
	if len(entities[*doc.Circle](s)) != 1 {
		t.Fatal("an impossible TTR drew a circle")
	}
}

func TestArcMethods(t *testing.T) {
	s := newSession(t)
	s.Script("ARC 110,0 C 100,0 100,10")
	s.Script("ARC 210,0 C 200,0 A 90")
	s.Script("ARC 310,0 C 300,0 A -90")
	s.Script("ARC 410,0 C 400,0 L 10")
	s.Script("ARC C 500,0 510,0 A 90")
	s.Script("ARC 10,0 E 0,10 R 10")
	s.Script("ARC 110,0 E 100,10 D 90")
	s.Script("ARC 10,0 E 0,10 A 90")
	as := entities[*doc.Arc](s)
	if len(as) != 8 {
		t.Fatalf("arcs = %d", len(as))
	}
	arcNear(t, as[0], geom.V(100, 0), 10, 0, 90)
	arcNear(t, as[1], geom.V(200, 0), 10, 0, 90)
	arcNear(t, as[2], geom.V(300, 0), 10, 270, 360)
	arcNear(t, as[3], geom.V(400, 0), 10, 0, 60)
	arcNear(t, as[4], geom.V(500, 0), 10, 0, 90)
	arcNear(t, as[5], geom.V(0, 0), 10, 0, 90)
	arcNear(t, as[6], geom.V(100, 0), 10, 0, 90)
	arcNear(t, as[7], geom.V(0, 0), 10, 0, 90)
	// A negative radius is the major arc.
	s.Script("ARC 10,0 E 0,10 R -10")
	as = entities[*doc.Arc](s)
	arcNear(t, as[8], geom.V(10, 10), 10, 270, 180)
}

func TestArcContinueAndThreePoint(t *testing.T) {
	s := newSession(t)
	drawFeed(s, "LINE", "200,0", "210,0", "", "ARC", "", "220,10")
	as := entities[*doc.Arc](s)
	if len(as) != 1 {
		t.Fatalf("arcs = %d", len(as))
	}
	arcNear(t, as[0], geom.V(210, 10), 10, 270, 360)
	// A clockwise three-point arc is stored counter-clockwise from its end.
	s.Script("ARC 10,0 5,5 0,0")
	as = entities[*doc.Arc](s)
	if len(as) != 2 {
		t.Fatalf("arcs = %d", len(as))
	}
	// A degenerate answer reports and asks again.
	s.Script("ARC 0,0 5,5 10,10")
	if len(entities[*doc.Arc](s)) != 2 {
		t.Fatal("collinear arc was drawn")
	}
}

func TestRectangOptions(t *testing.T) {
	s := newSession(t)
	s.Script("REC 200,0 D 100 50 210,10")
	s.Script("REC 0,0 D 10 5 -1,-1")
	s.Script("REC C 5 5 0,0 100,50")
	ps := plines(s)
	if len(ps) != 3 {
		t.Fatalf("polylines = %d", len(ps))
	}
	near(t, "dim corner", ps[0].Points[2], geom.V(300, 50))
	near(t, "quadrant corner", ps[1].Points[2], geom.V(-10, -5))
	if len(ps[2].Points) != 8 || ps[2].Bulges != nil {
		t.Fatalf("chamfer = %+v", ps[2])
	}
	near(t, "chamfer start", ps[2].Points[0], geom.V(5, 0))
	// Fillet cancels the chamfer and is remembered by the next RECTANG.
	s.Script("REC F 10 0,0 100,50")
	s.Script("REC 0,0 20,20")
	ps = plines(s)
	if len(ps) != 5 || len(ps[3].Points) != 8 || len(ps[4].Points) != 8 {
		t.Fatalf("fillet memory: %+v", ps)
	}
	if math.Abs(ps[3].Bulges[1]-math.Tan(math.Pi/8)) > 1e-12 || ps[3].Bulges[0] != 0 {
		t.Fatalf("fillet bulges = %v", ps[3].Bulges)
	}
	// Too big for the rectangle: drawn plain.
	s.Script("REC 0,0 5,5")
	if ps = plines(s); len(ps[5].Points) != 4 {
		t.Fatalf("oversize fillet = %+v", ps[5])
	}
	// Width is kept in the data and remembered; Fillet 0 clears the corners.
	s.Script("REC F 0 W 2 0,0 10,10")
	ps = plines(s)
	if len(ps[6].Points) != 4 || len(ps[6].Widths) != 4 || ps[6].Widths[2] != [2]float64{2, 2} {
		t.Fatalf("width = %+v", ps[6])
	}
}

func TestRectangRotationAndArea(t *testing.T) {
	s := newSession(t)
	s.Script("REC F 0 W 0 0,0 R 90 -5,10")
	// Rotated by 90 degrees about the first corner: local x is world y.
	ps := plines(s)
	if len(ps) != 1 {
		t.Fatalf("polylines = %d", len(ps))
	}
	near(t, "rotated corner", ps[0].Points[1], geom.V(0, 10))
	s.Script("REC 0,0 R P 0,0 0,10 5,5")
	ps = plines(s)
	near(t, "pick-points rotation", ps[1].Points[1], geom.V(0, 5))
	s.Script("REC 0,0 A 100 L 5 1,1")
	ps = plines(s)
	near(t, "area corner", ps[2].Points[2], geom.V(5, 20))
	s.Script("REC 0,0 A 100 W 5 1,1")
	ps = plines(s)
	near(t, "area by width", ps[3].Points[2], geom.V(20, 5))
}

func TestPolygon(t *testing.T) {
	s := newSession(t)
	s.Script("POLYGON 4 E 0,0 10,0")
	s.Script("POLYGON 4 100,0 I 110,0")
	s.Script("POLYGON 3 200,0 C 210,0")
	ps := plines(s)
	if len(ps) != 3 {
		t.Fatalf("polygons = %d", len(ps))
	}
	near(t, "edge p2", ps[0].Points[2], geom.V(10, 10))
	near(t, "inscribed vertex", ps[1].Points[0], geom.V(110, 0))
	near(t, "circumscribed vertex", ps[2].Points[0], geom.V(210, 10*math.Sqrt(3)))
	if got := s.Vars().Int("POLYSIDES"); got != 3 {
		t.Fatalf("POLYSIDES = %d", got)
	}
	// The last choice (Circumscribed) is the default of the next polygon; Enter takes it.
	drawFeed(s, "POLYGON", "4", "0,0")
	if !strings.Contains(s.Prompt(), "<C>") {
		t.Fatalf("prompt = %q", s.Prompt())
	}
	// A typed radius gives a horizontal bottom edge.
	s.Script(" 10 ")
	ps = plines(s)
	if len(ps) != 4 {
		t.Fatalf("polygons = %d", len(ps))
	}
	b := geom.BoxOf(ps[3].Points...)
	if math.Abs(b.Min.Y+10) > 1e-9 || math.Abs(b.Max.X-10) > 1e-9 {
		t.Fatalf("typed circumscribed square box = %+v", b)
	}
	// Bad side counts ask again.
	s.Script("POLYGON 2")
	if !strings.Contains(s.Prompt(), "number of sides") {
		t.Fatalf("prompt = %q", s.Prompt())
	}
	s.Escape()
	s.Script("POLYGON 4 0,0 I 10")
	b = geom.BoxOf(plines(s)[4].Points...)
	if math.Abs(b.Min.Y+10/math.Sqrt2) > 1e-9 {
		t.Fatalf("typed inscribed square box = %+v", b)
	}
}

func TestPlineWidthsAndLength(t *testing.T) {
	s := newSession(t)
	drawFeed(s, "PLINE", "0,100", "W", "2", "4", "50,100", "H", "1", "1", "80,100", "L", "30", "")
	ps := plines(s)
	if len(ps) != 1 || len(ps[0].Points) != 4 {
		t.Fatalf("pline = %+v", ps)
	}
	near(t, "length end", ps[0].Points[3], geom.V(110, 100))
	w := ps[0].Widths
	if w[0] != [2]float64{2, 4} || w[1] != [2]float64{2, 2} || w[2] != [2]float64{2, 2} {
		t.Fatalf("widths = %v", w)
	}
	if got := s.Vars().Real("PLINEWID"); got != 0 {
		t.Fatalf("W changed PLINEWID: %v", got)
	}
	// PLINEWID is the starting width of the next polyline.
	if err := s.Vars().Set("PLINEWID", 3.0); err != nil {
		t.Fatal(err)
	}
	drawFeed(s, "PLINE", "0,0", "10,0", "")
	if w = plines(s)[1].Widths; w[0] != [2]float64{3, 3} {
		t.Fatalf("PLINEWID start width = %v", w)
	}
}

func TestPlineArcModes(t *testing.T) {
	s := newSession(t)
	// Default tangent arc, then Line, then Close.
	s.Script("PLINE 0,0 100,0 A 100,100 L 0,100 C ")
	p := plines(s)[0]
	if !p.Closed || len(p.Points) != 4 || math.Abs(p.Bulges[1]-1) > 1e-9 {
		t.Fatalf("tangent arc pline = %+v", p)
	}
	// Second pt: the arc through three points.
	s.Script("PLINE 0,0 A S 5,5 10,0  ")
	p = plines(s)[1]
	if len(p.Points) != 2 || math.Abs(math.Abs(p.Bulges[0])-1) > 1e-9 || p.Bulges[0] > 0 {
		t.Fatalf("second-pt pline = %+v", p)
	}
	// Angle then end point; Angle then CEnter; CEnter then end; CEnter Angle; CEnter Length.
	s.Script("PLINE 0,0 A A 90 10,10  ")
	if b := plines(s)[2].Bulges[0]; math.Abs(b-math.Tan(math.Pi/8)) > 1e-9 {
		t.Fatalf("angle bulge = %v", b)
	}
	s.Script("PLINE 10,0 A A 90 CE 0,0  ")
	near(t, "angle+center end", plines(s)[3].Points[1], geom.V(0, 10))
	s.Script("PLINE 10,0 A CE 0,0 0,10  ")
	near(t, "center end", plines(s)[4].Points[1], geom.V(0, 10))
	s.Script("PLINE 10,0 A CE 0,0 A 90  ")
	near(t, "center angle", plines(s)[5].Points[1], geom.V(0, 10))
	s.Script("PLINE 10,0 A CE 0,0 L 14.142135623730951  ")
	near(t, "center length", plines(s)[6].Points[1], geom.V(0, 10))
	// Direction, Radius, Radius+Angle.
	s.Script("PLINE 10,0 A D 90 0,10  ")
	near(t, "direction end", plines(s)[7].Points[1], geom.V(0, 10))
	s.Script("PLINE 10,0 A R 10 0,10  ")
	if b := plines(s)[8].Bulges[0]; math.Abs(b-math.Tan(math.Pi/8)) > 1e-9 {
		t.Fatalf("radius bulge = %v", b)
	}
	s.Script("PLINE 10,0 A R 10 A 90 135  ")
	near(t, "radius angle end", plines(s)[9].Points[1], geom.V(0, 10))
	s.Script("PLINE 10,0 A A 90 R 10 135  ")
	near(t, "angle radius end", plines(s)[10].Points[1], geom.V(0, 10))
	// CLose in arc mode closes with a tangent arc; Undo drops the last arc; Width applies.
	s.Script("PLINE 0,0 10,0 10,10 A CL ")
	if p = plines(s)[11]; !p.Closed || len(p.Points) != 3 || p.Bulges[2] == 0 {
		t.Fatalf("arc close = %+v", p)
	}
	s.Script("PLINE 0,0 A 10,10 U 20,20 ")
	if p = plines(s)[12]; len(p.Points) != 2 {
		t.Fatalf("after undo = %+v", p)
	}
	s.Escape()
}

func TestDrawPromptsInItalian(t *testing.T) {
	italian(t)
	s := newSession(t)
	for _, c := range []string{"LINEA", "POLIGONO", "CERCHIO", "ARCO", "RETTANGOLO", "POLILINEA"} {
		s.Run(c)
		_ = s.Prompt()
		s.Escape()
	}
	drawFeed(s, "POLIGONO", "4", "0,0")
	_ = s.Prompt()
	s.Escape()
	drawFeed(s, "CERCHIO", "0,0")
	_ = s.Prompt()
	s.Escape()
	drawFeed(s, "POLILINEA", "0,0", "10,0", "ARCO")
	for _, k := range []string{"ANgolo", "CEntro", "Direzione", "Raggio", "Secondo punto", "LArghezza", "Metà larghezza", "Linea", "CHiudi"} {
		if !strings.Contains(s.Prompt(), strings.Split(k, "|")[0]) {
			t.Errorf("Italian arc prompt %q lacks %q", s.Prompt(), k)
		}
	}
	s.Escape()
	drawFeed(s, "RETTANGOLO")
	if !strings.Contains(s.Prompt(), "SMusso") {
		t.Errorf("prompt = %q", s.Prompt())
	}
	s.Escape()
	drawFeed(s, "CERCHIO")
	if !strings.Contains(s.Prompt(), "3P/2P/Ttr (tangente tangente raggio)") {
		t.Errorf("prompt = %q", s.Prompt())
	}
	s.Escape()
}
