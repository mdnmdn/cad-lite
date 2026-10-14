package cadmcp

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strings"

	"go-draw/cmdsys"
	"go-draw/cmdsys/commands"
	"go-draw/doc"
	"go-draw/geom"
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
	return finite(v...)
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
	idle(s)
	tx, err := s.Drawing().Begin()
	if err != nil {
		return err
	}
	defer func() {
		if r := recover(); r != nil {
			tx.Abort()
			panic(r)
		}
	}()
	if err := fn(tx); err != nil {
		tx.Abort()
		return err
	}
	tx.Commit()
	return nil
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
	if err := withTx(s, func(tx *doc.Txn) error { h = tx.Add(e); return nil }); err != nil {
		return nil, err
	}
	return created{Handle: h.String(), Class: e.Class(), Layer: e.Common().Layer}, nil
}

type lineArgs struct {
	X1, Y1, X2, Y2 float64
	Layer          string
}

func addLine(s *cmdsys.Session, a lineArgs) (any, error) {
	if err := finite(a.X1, a.Y1, a.X2, a.Y2); err != nil {
		return nil, err
	}
	if a.X1 == a.X2 && a.Y1 == a.Y2 {
		return nil, errors.New("line endpoints must differ")
	}
	return addEntity(s, &doc.Line{Start: geom.V(a.X1, a.Y1), End: geom.V(a.X2, a.Y2)}, a.Layer)
}

type circleArgs struct {
	CX, CY, R float64
	Layer     string
}

func addCircle(s *cmdsys.Session, a circleArgs) (any, error) {
	if err := finite(a.CX, a.CY, a.R); err != nil {
		return nil, err
	}
	if a.R <= 0 {
		return nil, errors.New("r must be positive")
	}
	return addEntity(s, &doc.Circle{Center: geom.V(a.CX, a.CY), Radius: a.R}, a.Layer)
}

type arcArgs struct {
	P1, P2, P3 Point
	Layer      string
}

func addArc(s *cmdsys.Session, a arcArgs) (any, error) {
	arc, ok := doc.ArcFrom3Points(a.P1.vec(), a.P2.vec(), a.P3.vec())
	if !ok {
		return nil, errors.New("the three points are collinear or coincident; no arc passes through them")
	}
	return addEntity(s, &doc.Arc{Center: arc.C, Radius: arc.R, StartAngle: arc.Start, EndAngle: arc.End}, a.Layer)
}

type polylineArgs struct {
	Points []Point
	Closed bool
	Layer  string
}

func addPolyline(s *cmdsys.Session, a polylineArgs) (any, error) {
	if len(a.Points) < 2 {
		return nil, errors.New("a polyline needs at least 2 points")
	}
	if len(a.Points) > 100000 {
		return nil, errors.New("too many points (max 100000)")
	}
	pts := make([]geom.Vec2, len(a.Points))
	for i, p := range a.Points {
		pts[i] = p.vec()
	}
	return addEntity(s, &doc.LWPolyline{Points: pts, Closed: a.Closed}, a.Layer)
}

func addRect(s *cmdsys.Session, a lineArgs) (any, error) {
	if err := finite(a.X1, a.Y1, a.X2, a.Y2); err != nil {
		return nil, err
	}
	if a.X1 == a.X2 || a.Y1 == a.Y2 {
		return nil, errors.New("rectangle must have a non-zero width and height")
	}
	pts := []geom.Vec2{{X: a.X1, Y: a.Y1}, {X: a.X2, Y: a.Y1}, {X: a.X2, Y: a.Y2}, {X: a.X1, Y: a.Y2}}
	return addEntity(s, &doc.LWPolyline{Points: pts, Closed: true}, a.Layer)
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

// editable parses handles and rejects entities on locked layers.
func editable(s *cmdsys.Session, in []string) ([]doc.Handle, error) {
	hs, err := parseHandles(s, in)
	if err != nil {
		return nil, err
	}
	for _, h := range hs {
		if l := s.Drawing().Layer(s.Drawing().Entity(h).Common().Layer); l != nil && l.Locked {
			return nil, fmt.Errorf("entity %s is on locked layer %q", h, l.Name)
		}
	}
	return hs, nil
}

func erase(s *cmdsys.Session, a handlesArgs) (any, error) {
	hs, err := editable(s, a.Handles)
	if err != nil {
		return nil, err
	}
	err = withTx(s, func(tx *doc.Txn) error {
		for _, h := range hs {
			tx.Erase(h)
		}
		return nil
	})
	return changed{len(hs), handleStrings(hs)}, err
}

func transformAll(s *cmdsys.Session, in []string, m geom.Mat3) (any, error) {
	hs, err := editable(s, in)
	if err != nil {
		return nil, err
	}
	err = withTx(s, func(tx *doc.Txn) error {
		for _, h := range hs {
			tx.Modify(h, func(e doc.Entity) { e.Transform(m) })
		}
		return nil
	})
	return changed{len(hs), handleStrings(hs)}, err
}

type moveArgs struct {
	Handles []string
	DX, DY  float64
}

func move(s *cmdsys.Session, a moveArgs) (any, error) {
	if err := finite(a.DX, a.DY); err != nil {
		return nil, err
	}
	return transformAll(s, a.Handles, geom.Translate(geom.V(a.DX, a.DY)))
}

type rotateArgs struct {
	Handles []string
	CX, CY  float64
	Angle   float64
}

func rotate(s *cmdsys.Session, a rotateArgs) (any, error) {
	if err := finite(a.CX, a.CY, a.Angle); err != nil {
		return nil, err
	}
	return transformAll(s, a.Handles, geom.Rotate(geom.V(a.CX, a.CY), a.Angle*math.Pi/180))
}

type scaleArgs struct {
	Handles []string
	CX, CY  float64
	Factor  float64
}

func scale(s *cmdsys.Session, a scaleArgs) (any, error) {
	if err := finite(a.CX, a.CY, a.Factor); err != nil {
		return nil, err
	}
	if a.Factor <= 0 {
		return nil, errors.New("factor must be positive")
	}
	return transformAll(s, a.Handles, geom.ScaleAbout(geom.V(a.CX, a.CY), a.Factor))
}

type entityLayerArgs struct {
	Handles []string
	Layer   string
}

func setEntityLayer(s *cmdsys.Session, a entityLayerArgs) (any, error) {
	if a.Layer == "" {
		return nil, errors.New("layer is required")
	}
	if err := checkLayer(s, a.Layer); err != nil {
		return nil, err
	}
	hs, err := editable(s, a.Handles)
	if err != nil {
		return nil, err
	}
	err = withTx(s, func(tx *doc.Txn) error {
		for _, h := range hs {
			tx.Modify(h, func(e doc.Entity) { e.Common().Layer = a.Layer })
		}
		return nil
	})
	return changed{len(hs), handleStrings(hs)}, err
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
