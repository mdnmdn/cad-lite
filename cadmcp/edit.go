package cadmcp

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strings"

	"cad-lite/cmdsys"
	"cad-lite/cmdsys/commands"
	"cad-lite/doc"
	"cad-lite/geom"
)

// Point is an [x, y] pair; decoding rejects any other length.
type Point [2]float64

// UnmarshalJSON implements json.Unmarshaler.
func (p *Point) UnmarshalJSON(b []byte) error {
	var v []float64
	if err := json.Unmarshal(b, &v); err != nil {
		return err
	}
	if len(v) != 2 {
		return fmt.Errorf("a point must be [x, y], got %d numbers", len(v))
	}
	copy(p[:], v)
	return inRange(v...)
}

func (p Point) vec() geom.Vec2 { return geom.V(p[0], p[1]) }

type created struct {
	Handle string `json:"handle"`
	Class  string `json:"class"`
	Layer  string `json:"layer"`
}

// withTx runs fn in one transaction, which is one undo step, after cancelling
// any running command. An error aborts the transaction.
func withTx(s *cmdsys.Session, fn func(tx *doc.Txn) error) error {
	_, err := withTxChanges(s, fn)
	return err
}

// withTxChanges is withTx returning the committed change set.
func withTxChanges(s *cmdsys.Session, fn func(tx *doc.Txn) error) (doc.ChangeSet, error) {
	idle(s)
	tx, err := s.Drawing().Begin()
	if err != nil {
		return doc.ChangeSet{}, err
	}
	defer func() {
		if r := recover(); r != nil {
			tx.Abort()
			panic(r)
		}
	}()
	if err := fn(tx); err != nil {
		tx.Abort()
		return doc.ChangeSet{}, err
	}
	return tx.Commit(), nil
}

func checkLayer(s *cmdsys.Session, name string) error {
	if name != "" && s.Drawing().Layer(name) == nil {
		return fmt.Errorf("layer %q does not exist (create_layer first)", name)
	}
	return nil
}

func addEntity(s *cmdsys.Session, e doc.Entity, layer string) (any, error) {
	if err := checkLayer(s, layer); err != nil {
		return nil, err
	}
	e.Common().Layer = layer
	var h doc.Handle
	if err := withTx(s, func(tx *doc.Txn) error { h = tx.Add(e); return checkBounds(s.Drawing(), h) }); err != nil {
		return nil, err
	}
	return created{Handle: h.String(), Class: e.Class(), Layer: e.Common().Layer}, nil
}

type lineArgs struct {
	X1, Y1, X2, Y2 float64
	Layer          string
}

// The *Entity builders validate a tool's arguments and build the entity without touching the
// drawing; the single tools and batch share them.

func lineEntity(a lineArgs) (doc.Entity, error) {
	if err := inRange(a.X1, a.Y1, a.X2, a.Y2); err != nil {
		return nil, err
	}
	if a.X1 == a.X2 && a.Y1 == a.Y2 {
		return nil, errors.New("line endpoints must differ")
	}
	return &doc.Line{Start: geom.V(a.X1, a.Y1), End: geom.V(a.X2, a.Y2)}, nil
}

func addLine(s *cmdsys.Session, a lineArgs) (any, error) {
	return addBuilt(s, a.Layer)(lineEntity(a))
}

type circleArgs struct {
	CX, CY, R float64
	Layer     string
}

func circleEntity(a circleArgs) (doc.Entity, error) {
	if err := inRange(a.CX, a.CY, a.R); err != nil {
		return nil, err
	}
	if a.R <= 0 {
		return nil, errors.New("r must be positive")
	}
	return &doc.Circle{Center: geom.V(a.CX, a.CY), Radius: a.R}, nil
}

func addCircle(s *cmdsys.Session, a circleArgs) (any, error) {
	return addBuilt(s, a.Layer)(circleEntity(a))
}

type arcArgs struct {
	P1, P2, P3 *Point
	Center     *Point
	Radius     *float64
	StartAngle *float64 `json:"start_angle"`
	EndAngle   *float64 `json:"end_angle"`
	Layer      string
}

// arcEntity builds an ARC from three points, or from a centre, radius and start and end angles
// (AutoCAD's ARC Center option and the DXF form: degrees, counter-clockwise from +X, the arc runs
// counter-clockwise from start to end). Exactly one form must be given in full.
func arcEntity(a arcArgs) (doc.Entity, error) {
	three := a.P1 != nil || a.P2 != nil || a.P3 != nil
	centre := a.Center != nil || a.Radius != nil || a.StartAngle != nil || a.EndAngle != nil
	switch {
	case three && centre:
		return nil, errors.New("give either p1, p2, p3 or center, radius, start_angle, end_angle, not both")
	case three:
		if a.P1 == nil || a.P2 == nil || a.P3 == nil {
			return nil, errors.New("the three-point form needs p1, p2 and p3")
		}
		arc, ok := doc.ArcFrom3Points(a.P1.vec(), a.P2.vec(), a.P3.vec())
		if !ok {
			return nil, errors.New("the three points are collinear or coincident; no arc passes through them")
		}
		return &doc.Arc{Center: arc.C, Radius: arc.R, StartAngle: arc.Start, EndAngle: arc.End}, nil
	case centre:
		if a.Center == nil || a.Radius == nil || a.StartAngle == nil || a.EndAngle == nil {
			return nil, errors.New("the centre form needs center, radius, start_angle and end_angle")
		}
		if err := inRange(*a.Radius, *a.StartAngle, *a.EndAngle); err != nil {
			return nil, err
		}
		if *a.Radius <= 0 {
			return nil, errors.New("radius must be positive")
		}
		start, end := radians(*a.StartAngle), radians(*a.EndAngle)
		if sweep := normRad(end - start); sweep < angleTol || sweep > 2*math.Pi-angleTol {
			return nil, errors.New("start_angle and end_angle must differ (modulo 360); use add_circle for a full circle")
		}
		return &doc.Arc{Center: a.Center.vec(), Radius: *a.Radius, StartAngle: start, EndAngle: end}, nil
	}
	return nil, errors.New("give p1, p2, p3 (three points) or center, radius, start_angle, end_angle")
}

// angleTol is the angular tolerance (radians) below which two arc angles are the same.
const angleTol = 1e-10

// normRad reduces an angle to [0, 2π). geom.NormAngle can return 2π itself for a tiny negative
// angle (its a+2π rounds up), which is mapped to 0 here.
func normRad(a float64) float64 {
	a = geom.NormAngle(a)
	if a >= 2*math.Pi {
		a = 0
	}
	return a
}

// radians converts degrees to an angle in [0, 2π).
func radians(deg float64) float64 { return normRad(deg * math.Pi / 180) }

func addArc(s *cmdsys.Session, a arcArgs) (any, error) {
	return addBuilt(s, a.Layer)(arcEntity(a))
}

type polylineArgs struct {
	Points []Point
	Bulges []float64
	Closed bool
	Layer  string
}

// maxPolylinePoints caps the vertices of one polyline.
const maxPolylinePoints = 100000

// polylineEntity builds an LWPOLYLINE. bulges follow DXF group 42: one per vertex, the bulge of
// the segment leaving it (tan of a quarter of the included angle, positive counter-clockwise);
// the last one applies only when the polyline is closed.
func polylineEntity(a polylineArgs) (doc.Entity, error) {
	if len(a.Points) < 2 {
		return nil, errors.New("a polyline needs at least 2 points")
	}
	if len(a.Points) > maxPolylinePoints {
		return nil, fmt.Errorf("too many points (max %d)", maxPolylinePoints)
	}
	pts := make([]geom.Vec2, len(a.Points))
	for i, p := range a.Points {
		pts[i] = p.vec()
	}
	var bulges []float64
	if a.Bulges != nil {
		if len(a.Bulges) != len(a.Points) {
			return nil, fmt.Errorf("bulges must have one value per point (%d), got %d", len(a.Points), len(a.Bulges))
		}
		if err := finite(a.Bulges...); err != nil {
			return nil, err
		}
		for _, b := range a.Bulges {
			if math.Abs(b) > MaxBulge {
				return nil, fmt.Errorf("a bulge must be within ±%g", float64(MaxBulge))
			}
		}
		if hasBulge(a.Bulges) {
			bulges = append([]float64(nil), a.Bulges...)
		}
	}
	return &doc.LWPolyline{Points: pts, Bulges: bulges, Closed: a.Closed}, nil
}

func addPolyline(s *cmdsys.Session, a polylineArgs) (any, error) {
	return addBuilt(s, a.Layer)(polylineEntity(a))
}

type rectArgs struct {
	X1, Y1, X2, Y2 *float64
	Origin         *Point
	Width, Height  *float64
	Fillet         *float64
	Chamfer        []float64
	Layer          string
}

// rectEntity builds a closed rectangular LWPOLYLINE like RECTANG, from two opposite corners or
// from an origin corner plus a signed width (along X) and height (along Y), with an optional
// Fillet radius or Chamfer distances [along X edges, along Y edges]. The corners are cut by
// geom.CutPolyline; a fillet or chamfer that does not fit is an error.
func rectEntity(a rectArgs) (doc.Entity, error) {
	corners := a.X1 != nil || a.Y1 != nil || a.X2 != nil || a.Y2 != nil
	sized := a.Origin != nil || a.Width != nil || a.Height != nil
	var x1, y1, x2, y2 float64
	switch {
	case corners && sized:
		return nil, errors.New("give either x1, y1, x2, y2 (opposite corners) or origin, width, height, not both")
	case corners:
		if a.X1 == nil || a.Y1 == nil || a.X2 == nil || a.Y2 == nil {
			return nil, errors.New("the corner form needs x1, y1, x2 and y2")
		}
		x1, y1, x2, y2 = *a.X1, *a.Y1, *a.X2, *a.Y2
	case sized:
		if a.Origin == nil || a.Width == nil || a.Height == nil {
			return nil, errors.New("the size form needs origin, width and height")
		}
		if err := inRange(*a.Width, *a.Height); err != nil {
			return nil, err
		}
		x1, y1 = a.Origin[0], a.Origin[1]
		x2, y2 = x1+*a.Width, y1+*a.Height
	default:
		return nil, errors.New("give x1, y1, x2, y2 (opposite corners) or origin, width, height")
	}
	if err := inRange(x1, y1, x2, y2); err != nil {
		return nil, err
	}
	if x1 == x2 || y1 == y2 {
		return nil, errors.New("rectangle must have a non-zero width and height")
	}
	pts := []geom.Vec2{{X: x1, Y: y1}, {X: x2, Y: y1}, {X: x2, Y: y2}, {X: x1, Y: y2}}
	cut, err := rectCut(a.Fillet, a.Chamfer)
	if err != nil || cut == nil {
		return &doc.LWPolyline{Points: pts, Closed: true}, err
	}
	out, _, _, skipped := geom.CutPolyline(geom.Polyline{Pts: pts, Closed: true}, nil, cut)
	if skipped > 0 {
		return nil, fmt.Errorf("the fillet or chamfer does not fit a %g x %g rectangle", math.Abs(x2-x1), math.Abs(y2-y1))
	}
	pts, bulges := mergeCoincident(out.Pts, out.Bulges, 1e-9*math.Max(math.Abs(x2-x1), math.Abs(y2-y1)))
	return &doc.LWPolyline{Points: pts, Bulges: bulges, Closed: true}, nil
}

// mergeCoincident drops each vertex of a closed polyline that lies within eps of the vertex kept
// before it, keeping the dropped vertex's bulge for the segment that now leaves the kept one.
// geom.CutPolyline leaves such a pair, a few ulps apart and in reverse order, where a fillet or
// chamfer uses up a whole side.
func mergeCoincident(pts []geom.Vec2, bulges []float64, eps float64) ([]geom.Vec2, []float64) {
	var outP []geom.Vec2
	var outB []float64
	for i, p := range pts {
		b := 0.0
		if i < len(bulges) {
			b = bulges[i]
		}
		if n := len(outP); n > 0 && outP[n-1].Dist(p) <= eps {
			outB[n-1] = b
			continue
		}
		outP, outB = append(outP, p), append(outB, b)
	}
	if n := len(outP); n > 2 && outP[n-1].Dist(outP[0]) <= eps {
		outP, outB = outP[:n-1], outB[:n-1]
	}
	if !hasBulge(outB) {
		outB = nil
	}
	return outP, outB
}

// rectCut returns the corner cut for RECTANG's Fillet or Chamfer option, nil for square corners.
// Chamfer distance d1 is taken off the edges along X and d2 off those along Y, as RECTANG does.
func rectCut(fillet *float64, chamfer []float64) (geom.CutFunc, error) {
	if fillet != nil && chamfer != nil {
		return nil, errors.New("give fillet or chamfer, not both")
	}
	if fillet != nil {
		if err := inRange(*fillet); err != nil || *fillet < 0 {
			return nil, errors.Join(err, errors.New("fillet must be a radius >= 0"))
		}
		if *fillet == 0 {
			return nil, nil
		}
		return geom.FilletCut(*fillet), nil
	}
	if chamfer == nil {
		return nil, nil
	}
	if len(chamfer) != 2 {
		return nil, errors.New("chamfer must be [d1, d2]: the distance off the edges along X, then along Y")
	}
	d1, d2 := chamfer[0], chamfer[1]
	if err := inRange(d1, d2); err != nil || d1 < 0 || d2 < 0 {
		return nil, errors.Join(err, errors.New("chamfer distances must be >= 0"))
	}
	if d1 == 0 && d2 == 0 {
		return nil, nil
	}
	alongX, alongY := geom.ChamferCut(d1, d2), geom.ChamferCut(d2, d1)
	return func(u1, u2 geom.Vec2, la, lb, turn float64) (geom.CornerCut, bool) {
		if math.Abs(u1.X) > math.Abs(u1.Y) { // arriving along X
			return alongX(u1, u2, la, lb, turn)
		}
		return alongY(u1, u2, la, lb, turn)
	}, nil
}

type polygonArgs struct {
	Sides    int
	Center   *Point
	Radius   *float64
	Mode     string
	Rotation *float64
	Edge     []Point
	Layer    string
}

// polygonEntity builds a regular polygon as POLYGON does, through commands.PolygonFromCenter or
// commands.PolygonFromEdge: from a centre and the radius of the circle it is inscribed in (the
// circumradius, the default) or circumscribed about (the apothem), turned by rotation degrees
// from AutoCAD's typed-radius placement (bottom edge horizontal); or from its first edge, with
// the polygon to the left of edge[0] to edge[1].
func polygonEntity(a polygonArgs) (doc.Entity, error) {
	if a.Sides < 3 || a.Sides > 1024 {
		return nil, errors.New("sides must be 3-1024")
	}
	byCentre := a.Center != nil || a.Radius != nil || a.Mode != "" || a.Rotation != nil
	var pts []geom.Vec2
	switch {
	case byCentre && a.Edge != nil:
		return nil, errors.New("give either center and radius (with mode and rotation) or edge, not both")
	case a.Edge != nil:
		if len(a.Edge) != 2 {
			return nil, errors.New("edge must be two points [[x1,y1],[x2,y2]]")
		}
		if pts = commands.PolygonFromEdge(a.Edge[0].vec(), a.Edge[1].vec(), a.Sides); pts == nil {
			return nil, errors.New("the edge's points must differ")
		}
	case byCentre:
		if a.Center == nil || a.Radius == nil {
			return nil, errors.New("the centre form needs center and radius")
		}
		if err := inRange(*a.Radius); err != nil {
			return nil, err
		}
		inscribed := true
		switch strings.ToLower(a.Mode) {
		case "", "inscribed", "i":
		case "circumscribed", "c":
			inscribed = false
		default:
			return nil, fmt.Errorf("mode %q must be inscribed or circumscribed", a.Mode)
		}
		rot := 0.0
		if a.Rotation != nil {
			if err := inRange(*a.Rotation); err != nil {
				return nil, err
			}
			rot = *a.Rotation * math.Pi / 180
		}
		if pts = commands.PolygonFromCenter(a.Center.vec(), *a.Radius, a.Sides, inscribed, rot); pts == nil {
			return nil, errors.New("radius must be positive")
		}
	default:
		return nil, errors.New("give center and radius, or edge")
	}
	return &doc.LWPolyline{Points: pts, Closed: true}, nil
}

func addPolygon(s *cmdsys.Session, a polygonArgs) (any, error) {
	return addBuilt(s, a.Layer)(polygonEntity(a))
}

func addRect(s *cmdsys.Session, a rectArgs) (any, error) {
	return addBuilt(s, a.Layer)(rectEntity(a))
}

// addBuilt returns a function that adds a built entity on layer, or passes the builder's error on.
func addBuilt(s *cmdsys.Session, layer string) func(doc.Entity, error) (any, error) {
	return func(e doc.Entity, err error) (any, error) {
		if err == nil {
			err = resolveStyles(&styleResolver{d: s.Drawing()}, e, editOp{})
		}
		if err != nil {
			return nil, err
		}
		return addEntity(s, e, layer)
	}
}

type handlesArgs struct{ Handles []string }

type changed struct {
	Count   int      `json:"count"`
	Handles []string `json:"handles"`
}

func handleStrings(hs []doc.Handle) []string {
	out := make([]string, len(hs))
	for i, h := range hs {
		out[i] = h.String()
	}
	return out
}

// checkEditable rejects an entity on a locked layer.
func checkEditable(d *doc.Drawing, h doc.Handle) error {
	if l := d.Layer(d.Entity(h).Common().Layer); l != nil && l.Locked {
		return fmt.Errorf("entity %s is on locked layer %q", h, l.Name)
	}
	return nil
}

// editable parses handles and rejects entities on locked layers.
func editable(s *cmdsys.Session, in []string) ([]doc.Handle, error) {
	hs, err := parseHandles(s, in)
	if err != nil {
		return nil, err
	}
	for _, h := range hs {
		if err := checkEditable(s.Drawing(), h); err != nil {
			return nil, err
		}
	}
	return hs, nil
}

// editOp is a validated edit of entities by handle: an ERASE, a transformation (MOVE, ROTATE,
// SCALE), a layer change or a text update. The single tools and batch share it.
type editOp struct {
	erase bool
	m     *geom.Mat3
	layer string
	text  *textUpdate
}

// apply performs the edit on hs inside tx. Only a text update can fail; the caller aborts the
// transaction then.
func (op editOp) apply(tx *doc.Txn, hs []doc.Handle) error {
	for _, h := range hs {
		var err error
		switch {
		case op.erase:
			tx.Erase(h)
		case op.m != nil:
			m := *op.m
			tx.Modify(h, func(e doc.Entity) { e.Transform(m) })
		case op.text != nil:
			tx.Modify(h, func(e doc.Entity) { err = op.text.apply(e) })
		default:
			tx.Modify(h, func(e doc.Entity) { e.Common().Layer = op.layer })
		}
		if err != nil {
			return fmt.Errorf("entity %s: %w", h, err)
		}
	}
	return nil
}

// runEdit is the body of every single edit tool: build the edit, resolve the handles, apply it
// in one transaction.
func runEdit(s *cmdsys.Session, in []string, op editOp, err error) (any, error) {
	if err != nil {
		return nil, err
	}
	if op.layer != "" {
		if err := checkLayer(s, op.layer); err != nil {
			return nil, err
		}
	}
	hs, err := editable(s, in)
	if err != nil {
		return nil, err
	}
	err = withTx(s, func(tx *doc.Txn) error {
		if err := op.apply(tx, hs); err != nil {
			return err
		}
		return checkBounds(s.Drawing(), hs...)
	})
	if err != nil {
		return nil, err
	}
	return changed{len(hs), handleStrings(hs)}, nil
}

func eraseEdit(handlesArgs) (editOp, error) { return editOp{erase: true}, nil }

func erase(s *cmdsys.Session, a handlesArgs) (any, error) {
	op, err := eraseEdit(a)
	return runEdit(s, a.Handles, op, err)
}

type moveArgs struct {
	Handles []string
	DX, DY  float64
}

func moveEdit(a moveArgs) (editOp, error) {
	if err := inRange(a.DX, a.DY); err != nil {
		return editOp{}, err
	}
	m := geom.Translate(geom.V(a.DX, a.DY))
	return editOp{m: &m}, nil
}

func move(s *cmdsys.Session, a moveArgs) (any, error) {
	op, err := moveEdit(a)
	return runEdit(s, a.Handles, op, err)
}

type rotateArgs struct {
	Handles []string
	CX, CY  float64
	Angle   float64
}

func rotateEdit(a rotateArgs) (editOp, error) {
	if err := inRange(a.CX, a.CY, a.Angle); err != nil {
		return editOp{}, err
	}
	m := geom.Rotate(geom.V(a.CX, a.CY), a.Angle*math.Pi/180)
	return editOp{m: &m}, nil
}

func rotate(s *cmdsys.Session, a rotateArgs) (any, error) {
	op, err := rotateEdit(a)
	return runEdit(s, a.Handles, op, err)
}

type scaleArgs struct {
	Handles []string
	CX, CY  float64
	Factor  float64
}

func scaleEdit(a scaleArgs) (editOp, error) {
	if err := inRange(a.CX, a.CY, a.Factor); err != nil {
		return editOp{}, err
	}
	if a.Factor <= 0 {
		return editOp{}, errors.New("factor must be positive")
	}
	m := geom.ScaleAbout(geom.V(a.CX, a.CY), a.Factor)
	return editOp{m: &m}, nil
}

func scale(s *cmdsys.Session, a scaleArgs) (any, error) {
	op, err := scaleEdit(a)
	return runEdit(s, a.Handles, op, err)
}

type entityLayerArgs struct {
	Handles []string
	Layer   string
}

func entityLayerEdit(a entityLayerArgs) (editOp, error) {
	if a.Layer == "" {
		return editOp{}, errors.New("layer is required")
	}
	return editOp{layer: a.Layer}, nil
}

func setEntityLayer(s *cmdsys.Session, a entityLayerArgs) (any, error) {
	op, err := entityLayerEdit(a)
	return runEdit(s, a.Handles, op, err)
}

type layerNameArgs struct{ Name string }

func setLayer(s *cmdsys.Session, a layerNameArgs) (any, error) {
	if err := checkLayer(s, a.Name); err != nil || a.Name == "" {
		return nil, errors.Join(err, requireName(a.Name))
	}
	idle(s)
	s.Drawing().CurrentLayer = a.Name
	return map[string]any{"current_layer": a.Name}, nil
}

func requireName(n string) error {
	if strings.TrimSpace(n) == "" {
		return errors.New("name is required")
	}
	return nil
}

type createLayerArgs struct {
	Name        string
	Color       *int
	Off, Locked *bool
	MakeCurrent bool `json:"make_current"`
}

func createLayer(s *cmdsys.Session, a createLayerArgs) (any, error) {
	if err := requireName(a.Name); err != nil {
		return nil, err
	}
	if a.Name != strings.TrimSpace(a.Name) || strings.ContainsAny(a.Name, "=\r\n") {
		return nil, errors.New("layer names cannot have surrounding spaces, '=' or newlines")
	}
	if a.Color != nil && (*a.Color < 1 || *a.Color > 255) {
		return nil, errors.New("color must be 1-255")
	}
	idle(s)
	d := s.Drawing()
	existed := d.Layer(a.Name) != nil
	color := 7
	if a.Color != nil {
		color = *a.Color
	}
	l := d.AddLayer(a.Name, color)
	if existed && a.Color != nil {
		l.Color = color
	}
	if a.Off != nil {
		l.Off = *a.Off
	}
	if a.Locked != nil {
		l.Locked = *a.Locked
	}
	if a.MakeCurrent {
		d.CurrentLayer = a.Name
	}
	return map[string]any{"layer": layerInfo(l), "created": !existed, "current_layer": d.CurrentLayer}, nil
}

type stepsArgs struct{ Steps *int }

func steps(a stepsArgs) (int, error) {
	if a.Steps == nil {
		return 1, nil
	}
	if *a.Steps < 1 || *a.Steps > 100 {
		return 0, errors.New("steps must be 1-100")
	}
	return *a.Steps, nil
}

func undoRedo(s *cmdsys.Session, a stepsArgs, step func(*doc.Drawing) bool) (any, error) {
	n, err := steps(a)
	if err != nil {
		return nil, err
	}
	idle(s)
	d := s.Drawing()
	done := 0
	for done < n && step(d) {
		done++
	}
	j := d.Journal()
	return map[string]any{"steps_done": done, "can_undo": j.CanUndo(), "can_redo": j.CanRedo()}, nil
}

func undo(s *cmdsys.Session, a stepsArgs) (any, error) {
	return undoRedo(s, a, (*doc.Drawing).Undo)
}

func redo(s *cmdsys.Session, a stepsArgs) (any, error) {
	return undoRedo(s, a, (*doc.Drawing).Redo)
}

func zoomExtents(s *cmdsys.Session, _ struct{}) (any, error) {
	d := s.Drawing()
	if d.Len() == 0 {
		return nil, errors.New("the drawing is empty")
	}
	idle(s)
	commands.ZoomExtents(d, s.View())
	v := s.View()
	return map[string]any{"center": []float64{v.Center.X, v.Center.Y}, "scale": v.Scale}, nil
}

type setSelectionArgs struct {
	Handles []string
	Add     bool
}

func setSelection(s *cmdsys.Session, a setSelectionArgs) (any, error) {
	var hs []doc.Handle
	if len(a.Handles) > 0 {
		var err error
		if hs, err = parseHandles(s, a.Handles); err != nil {
			return nil, err
		}
	}
	sel := s.Selection()
	if !a.Add {
		sel.Clear()
	}
	sel.Add(hs...)
	return map[string]any{"handles": handleStrings(sel.Handles())}, nil
}
