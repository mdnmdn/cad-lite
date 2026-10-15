package cadmcp

import (
	"errors"
	"fmt"
	"math"
	"strings"

	"cad-lite/cmdsys"
	"cad-lite/doc"
	"cad-lite/geom"
)

func layerInfo(l *doc.Layer) map[string]any {
	return map[string]any{"name": l.Name, "color": l.Color, "off": l.Off, "locked": l.Locked}
}

func boxJSON(b geom.Box2) []float64 {
	if b.IsEmpty() {
		return nil
	}
	return []float64{b.Min.X, b.Min.Y, b.Max.X, b.Max.Y}
}

func pts(ps []geom.Vec2) [][2]float64 {
	out := make([][2]float64, len(ps))
	for i, p := range ps {
		out[i] = [2]float64{p.X, p.Y}
	}
	return out
}

func xy(p geom.Vec2) [2]float64 { return [2]float64{p.X, p.Y} }

func degrees(a float64) float64 { return a * 180 / math.Pi }

// entityInfo describes e for agents: angles in degrees.
func entityInfo(e doc.Entity) map[string]any {
	c := e.Common()
	m := map[string]any{"handle": e.Handle().String(), "class": e.Class(), "layer": c.Layer, "bounds": boxJSON(e.Bounds())}
	if c.Color != doc.ByLayer {
		m["color"] = c.Color
	}
	switch e := e.(type) {
	case *doc.Insert:
		m["block"], m["position"], m["rotation"], m["scale_x"], m["scale_y"], m["attributes"] = e.BlockName, xy(e.Position), degrees(e.Rotation), e.ScaleX, e.ScaleY, e.Attributes
	case *doc.AttributeDefinition:
		m["tag"], m["prompt"], m["value"] = e.Tag, e.Prompt, e.Value
	case *doc.Unknown:
		m["unknown"] = true
	case *doc.Line:
		m["start"], m["end"] = xy(e.Start), xy(e.End)
		m["length"] = e.Start.Dist(e.End)
	case *doc.Circle:
		m["center"], m["radius"] = xy(e.Center), e.Radius
	case *doc.Arc:
		m["center"], m["radius"] = xy(e.Center), e.Radius
		m["start_angle"], m["end_angle"] = degrees(e.StartAngle), degrees(e.EndAngle)
	case *doc.LWPolyline:
		m["points"], m["closed"] = pts(e.Points), e.Closed
		if hasBulge(e.Bulges) {
			m["bulges"] = e.Bulges
		}
	case *doc.Text, *doc.MText:
		textInfo(m, e)
	}
	return m
}

func hasBulge(bs []float64) bool {
	for _, b := range bs {
		if b != 0 {
			return true
		}
	}
	return false
}

type scriptArgs struct{ Script string }

const maxScript = 200000

func recentLines(s *cmdsys.Session, since int) (lines []string, truncated bool) {
	n := s.HistoryTotal() - since
	h := s.History()
	if n > len(h) {
		n, truncated = len(h), true
	}
	return append([]string{}, h[len(h)-n:]...), truncated
}

func handleSet(d *doc.Drawing) map[doc.Handle]bool {
	m := make(map[doc.Handle]bool, d.Len())
	for _, h := range d.Handles() {
		m[h] = true
	}
	return m
}

func runScript(s *cmdsys.Session, a scriptArgs) (any, error) {
	if len(a.Script) > maxScript {
		return nil, fmt.Errorf("script too long (max %d bytes)", maxScript)
	}
	idle(s)
	d := s.Drawing()
	before, nBefore, mark := handleSet(d), d.Len(), s.HistoryTotal()
	s.Script(a.Script)
	cancelledCmd := ""
	if cmd := s.Active(); cmd != "" {
		cancelledCmd = cmd
		s.Escape()
	}
	d = s.Drawing()
	lines, trunc := recentLines(s, mark)
	newHandles := []string{}
	for _, h := range d.Handles() {
		if !before[h] {
			newHandles = append(newHandles, h.String())
		}
	}
	return map[string]any{
		"output": lines, "output_truncated": trunc,
		"entities_before": nBefore, "entities_after": d.Len(),
		"new_handles": newHandles, "cancelled_command": cancelledCmd, "prompt": s.Prompt(),
	}, nil
}

type commandArgs struct{ Input string }

func runCommand(s *cmdsys.Session, a commandArgs) (any, error) {
	if len(a.Input) > 10000 || strings.ContainsAny(a.Input, "\r\n") {
		return nil, errors.New("input must be a single line of at most 10000 bytes")
	}
	mark := s.HistoryTotal()
	s.Submit(a.Input)
	lines, trunc := recentLines(s, mark)
	return map[string]any{"output": lines, "output_truncated": trunc, "prompt": s.Prompt(), "active_command": s.Active()}, nil
}

func listCommands(s *cmdsys.Session, _ struct{}) (any, error) {
	return map[string]any{"commands": s.Registry().Names()}, nil
}

func getDrawing(s *cmdsys.Session, _ struct{}) (any, error) {
	d := s.Drawing()
	layers := []map[string]any{}
	for _, l := range d.Layers() {
		layers = append(layers, layerInfo(l))
	}
	counts := map[string]int{}
	for _, h := range d.Handles() {
		counts[d.Entity(h).Class()]++
	}
	j := d.Journal()
	return map[string]any{
		"layers": layers, "current_layer": d.CurrentLayer,
		"entity_count": d.Len(), "entity_counts": counts,
		"extents": boxJSON(d.Extents()), "units_code": d.Header.InsUnits,
		"can_undo": j.CanUndo(), "can_redo": j.CanRedo(),
		"selection_count": s.Selection().Len(),
		"active_command":  s.Active(), "prompt": s.Prompt(),
	}, nil
}

type listArgs struct {
	Layer  string
	Class  string
	Text   *string
	BBox   []float64 `json:"bbox"`
	Offset int
	Limit  int
}

func listEntities(s *cmdsys.Session, a listArgs) (any, error) {
	if a.Offset < 0 || a.Limit < 0 || a.Limit > 200 {
		return nil, errors.New("offset must be >= 0 and limit 1-200")
	}
	if a.Limit == 0 {
		a.Limit = 50
	}
	var box geom.Box2
	useBox := a.BBox != nil
	if useBox {
		if len(a.BBox) != 4 {
			return nil, errors.New("bbox must be [minx,miny,maxx,maxy]")
		}
		if err := finite(a.BBox...); err != nil {
			return nil, err
		}
		box = geom.BoxOf(geom.V(a.BBox[0], a.BBox[1]), geom.V(a.BBox[2], a.BBox[3]))
	}
	d := s.Drawing()
	if a.Layer != "" && d.Layer(a.Layer) == nil {
		return nil, fmt.Errorf("layer %q does not exist", a.Layer)
	}
	class := strings.ToUpper(a.Class)
	needle := ""
	if a.Text != nil {
		needle = strings.ToLower(*a.Text)
	}
	total := 0
	out := []map[string]any{}
	for _, h := range d.Handles() {
		e := d.Entity(h)
		if a.Layer != "" && e.Common().Layer != a.Layer || class != "" && e.Class() != class {
			continue
		}
		if a.Text != nil {
			plain, ok := plainText(e)
			if !ok || !strings.Contains(strings.ToLower(plain), needle) {
				continue
			}
		}
		if useBox {
			b := e.Bounds()
			if b.Max.X < box.Min.X || b.Min.X > box.Max.X || b.Max.Y < box.Min.Y || b.Min.Y > box.Max.Y {
				continue
			}
		}
		if total >= a.Offset && len(out) < a.Limit {
			out = append(out, entityInfo(e))
		}
		total++
	}
	return map[string]any{"total": total, "offset": a.Offset, "entities": out}, nil
}

func getSelection(s *cmdsys.Session, _ struct{}) (any, error) {
	return map[string]any{"handles": handleStrings(s.Selection().Handles())}, nil
}

type measureArgs struct {
	Handles []string
	Points  []Point
}

func measure(s *cmdsys.Session, a measureArgs) (any, error) {
	if len(a.Handles) == 0 && a.Points == nil {
		return nil, errors.New("give handles and/or points")
	}
	out := map[string]any{}
	if a.Points != nil {
		if len(a.Points) != 2 {
			return nil, errors.New("points must be exactly two points")
		}
		p, q := a.Points[0].vec(), a.Points[1].vec()
		out["distance"] = map[string]any{"distance": p.Dist(q), "dx": q.X - p.X, "dy": q.Y - p.Y, "angle": degrees(q.Sub(p).Angle())}
	}
	if len(a.Handles) > 0 {
		hs, err := parseHandles(s, a.Handles)
		if err != nil {
			return nil, err
		}
		items := []map[string]any{}
		totalLen, totalArea := 0.0, 0.0
		for _, h := range hs {
			e := s.Drawing().Entity(h)
			it := map[string]any{"handle": h.String(), "class": e.Class()}
			l, area, hasArea := measureEntity(e)
			it["length"] = l
			totalLen += l
			if hasArea {
				it["area"] = area
				totalArea += area
			}
			items = append(items, it)
		}
		out["entities"] = items
		out["total_length"] = totalLen
		out["total_area"] = totalArea
		out["note"] = "length is the perimeter or circumference for closed shapes; area of an open polyline or arc is that of the shape closed by its chord"
	}
	return out, nil
}

func measureEntity(e doc.Entity) (length, area float64, hasArea bool) {
	switch e := e.(type) {
	case *doc.Line:
		return e.Start.Dist(e.End), 0, false
	case *doc.Circle:
		return 2 * math.Pi * e.Radius, math.Pi * e.Radius * e.Radius, true
	case *doc.Arc:
		a := e.Geometry()[0].(geom.Arc)
		return a.Length(), math.Abs(geom.ArcSegmentArea(a.R, a.Sweep())), true
	case *doc.LWPolyline:
		p := e.Geometry()[0].(geom.Polyline)
		if len(p.Pts) < 3 {
			return p.Length(), 0, false
		}
		return p.Length(), math.Abs(geom.PolylineArea(p)), true
	}
	return 0, 0, false
}
