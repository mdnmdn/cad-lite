package doc

import (
	"cad-lite/geom"
	"fmt"
	"math"
	"strings"
)

// ByBlock is the colour inherited from the containing block reference.
const ByBlock = 0

// Insert references shared geometry with an independent placement and attributes.
type Insert struct {
	EntityCommon
	// BlockName names the shared definition.
	BlockName string
	// Position places the definition base point.
	Position geom.Vec2
	// Rotation is in radians; ScaleX and ScaleY are nonzero axis scales.
	Rotation, ScaleX, ScaleY float64
	// Attributes are per-instance values and optional imported placements.
	Attributes []Attribute
	// Extra retains unmodeled DXF INSERT fields.
	Extra []Pair
	// Source retains the full imported record for unsupported dependency preservation.
	Source []Pair
	d      *Drawing
}

// NewInsert creates an insertion at p with unit scale.
func NewInsert(name string, p geom.Vec2) *Insert {
	return &Insert{BlockName: name, Position: p, ScaleX: 1, ScaleY: 1}
}

// Class implements Object.
func (*Insert) Class() string { return "INSERT" }

// Clone implements Object.
func (e *Insert) Clone() Object {
	c := *e
	c.Attributes = append([]Attribute(nil), e.Attributes...)
	c.Extra = append([]Pair(nil), e.Extra...)
	c.Source = append([]Pair(nil), e.Source...)
	for i := range c.Attributes {
		if c.Attributes[i].Text != nil {
			c.Attributes[i].Text = c.Attributes[i].Text.Clone().(*Text)
		}
		c.Attributes[i].Extra = append([]Pair(nil), c.Attributes[i].Extra...)
	}
	return &c
}

// Bounds implements Entity.
func (e *Insert) Bounds() geom.Box2 {
	if e.d == nil || e.d.InsertError(e) != nil {
		return geom.EmptyBox()
	}
	b := geom.EmptyBox()
	for _, part := range e.parts(geom.Identity(), e.Layer, e.Color, map[string]bool{}, 0, false) {
		for _, c := range part.Geometry {
			b = b.Union(c.Bounds())
		}
	}
	return b.Extend(e.Position)
}

// VisibleBounds returns the bounds of visible child geometry for display extents and selection.
func (e *Insert) VisibleBounds() geom.Box2 {
	b := geom.EmptyBox()
	for _, c := range e.Geometry() {
		b = b.Union(c.Bounds())
	}
	return b
}

// Geometry implements Entity, retaining affine curves for unequal scales.
func (e *Insert) Geometry() []geom.Curve {
	var out []geom.Curve
	for _, part := range e.Parts() {
		out = append(out, part.Geometry...)
	}
	return out
}

// Transform applies a similarity transform to the reference placement.
func (e *Insert) Transform(m geom.Mat3) {
	for i := range e.Attributes {
		if e.Attributes[i].Text != nil {
			e.Attributes[i].Text.Transform(m)
		}
	}
	e.Position = m.Apply(e.Position)
	a := m.ApplyVec(geom.V(math.Cos(e.Rotation), math.Sin(e.Rotation)))
	e.Rotation = a.Angle()
	scale := m.ScaleFactor()
	e.ScaleX *= scale
	e.ScaleY *= scale
	if m.Det() < 0 {
		e.ScaleY = -e.ScaleY
	}
}

// BlockPart is resolved geometry with its effective layer and colour.
type BlockPart struct {
	// Layer is the resolved effective layer.
	Layer string
	// Color is the resolved effective ACI.
	Color int
	// Geometry is transformed model-space geometry.
	Geometry []geom.Curve
}

func (e *Insert) valid() bool {
	return finitePoint(e.Position) && e.ScaleX != 0 && e.ScaleY != 0 && !math.IsNaN(e.Rotation) && !math.IsInf(e.Rotation, 0) && !math.IsNaN(e.ScaleX) && !math.IsInf(e.ScaleX, 0) && !math.IsNaN(e.ScaleY) && !math.IsInf(e.ScaleY, 0)
}

// Parts resolves visible definition geometry with inherited layer and colour.
func (e *Insert) Parts() []BlockPart {
	if e.d == nil || e.d.InsertError(e) != nil {
		return nil
	}
	color := e.Color
	if color == ByLayer || color == ByBlock {
		color = 7
		if l := e.d.Layer(e.Layer); l != nil {
			color = l.Color
		}
	}
	return e.parts(geom.Identity(), e.Layer, color, map[string]bool{}, 0, true)
}
func (e *Insert) parts(parent geom.Mat3, layer string, color int, seen map[string]bool, depth int, visible bool) []BlockPart {
	key := strings.ToUpper(e.BlockName)
	b := e.d.blocks[key]
	if b == nil || seen[key] || depth >= MaxBlockDepth || !e.valid() {
		return nil
	}
	seen[key] = true
	defer delete(seen, key)
	m := parent.Mul(geom.Translate(e.Position)).Mul(geom.Rotate(geom.V(0, 0), e.Rotation)).Mul(geom.Mat3{e.ScaleX, 0, 0, e.ScaleY, 0, 0}).Mul(geom.Translate(b.Base.Scale(-1)))
	values := make(map[string]string, len(e.Attributes))
	placed := make(map[string]bool, len(e.Attributes))
	for _, a := range e.Attributes {
		key := strings.ToUpper(a.Tag)
		if _, exists := values[key]; !exists {
			values[key] = a.Value
		}
		if a.Text != nil {
			placed[key] = true
		}
	}
	var out []BlockPart
	for _, child := range b.Entities {
		l, c := child.Common().Layer, child.Common().Color
		if l == "0" || l == "" {
			l = layer
		}
		if c == ByBlock {
			c = color
		} else if c == ByLayer {
			c = 7
			if ly := e.d.Layer(l); ly != nil {
				c = ly.Color
			}
		}
		if ly := e.d.Layer(l); visible && ly != nil && !ly.Visible() {
			continue
		}
		if in, ok := child.(*Insert); ok {
			out = append(out, in.parts(m, l, c, seen, depth+1, visible)...)
			continue
		}
		var curves []geom.Curve
		if a, ok := child.(*AttributeDefinition); ok {
			if a.Invisible {
				continue
			}
			if placed[strings.ToUpper(a.Tag)] {
				continue
			}
			t := *a.Text.Clone().(*Text)
			if !a.Constant {
				if value, found := values[strings.ToUpper(a.Tag)]; found {
					t.Value = value
				}
			}
			curves = t.Geometry()
		} else {
			curves = child.Geometry()
		}
		part := BlockPart{Layer: l, Color: c}
		for _, curve := range curves {
			for _, primitive := range geom.Primitives(curve) {
				part.Geometry = append(part.Geometry, affineTransform(primitive, m))
			}
		}
		out = append(out, part)
	}
	for _, a := range e.Attributes {
		if a.Text == nil || a.Flags&1 != 0 {
			continue
		}
		t := a.Text.Clone().(*Text)
		t.Value = a.Value
		part := BlockPart{Layer: t.Layer, Color: t.Color}
		if part.Layer == "0" || part.Layer == "" {
			part.Layer = layer
		}
		if part.Color == ByBlock {
			part.Color = color
		}
		if visible && !e.d.Layer(part.Layer).Visible() {
			continue
		}
		for _, c := range t.Geometry() {
			for _, primitive := range geom.Primitives(c) {
				part.Geometry = append(part.Geometry, affineTransform(primitive, parent))
			}
		}
		out = append(out, part)
	}
	return out
}

// SetAttribute changes a known, nonconstant attribute; false means no such tag.
func (e *Insert) SetAttribute(tag, value string) bool {
	for i := range e.Attributes {
		if strings.EqualFold(e.Attributes[i].Tag, tag) {
			if e.Attributes[i].Flags&2 != 0 {
				return false
			}
			e.Attributes[i].Value = value
			return true
		}
	}

	if e.d == nil {
		return false
	}
	b := e.d.blocks[strings.ToUpper(e.BlockName)]
	if b == nil {
		return false
	}
	for _, child := range b.Entities {
		if a, ok := child.(*AttributeDefinition); ok && strings.EqualFold(a.Tag, tag) && !a.Constant {
			for i := range e.Attributes {
				if strings.EqualFold(e.Attributes[i].Tag, tag) {
					e.Attributes[i].Value = value
					return true
				}
			}
			e.Attributes = append(e.Attributes, Attribute{Tag: a.Tag, Value: value})
			return true
		}
	}
	return false
}
func affineTransform(c geom.Curve, m geom.Mat3) geom.Curve {
	x, y := m.ApplyVec(geom.V(1, 0)), m.ApplyVec(geom.V(0, 1))
	lx, ly := math.Hypot(x.X, x.Y), math.Hypot(y.X, y.Y)
	det := m.Det()
	if lx > 0 && ly > 0 && !math.IsInf(lx, 0) && !math.IsInf(ly, 0) && det != 0 && !math.IsInf(det, 0) && !math.IsNaN(det) {
		dot := (x.X/lx)*(y.X/ly) + (x.Y/lx)*(y.Y/ly)
		if math.Abs(dot) <= 1e-12 && math.Abs(lx-ly) <= 1e-12*max(lx, ly) {
			return c.Transform(m)
		}
	}
	if l, ok := c.(geom.Line); ok {
		return l.Transform(m)
	}
	return affineCurve{curve: c, m: m}
}

// affineCurve keeps the original parameterization, so anisotropic circles do not become circles.
type affineCurve struct {
	curve geom.Curve
	m     geom.Mat3
}

func (c affineCurve) PointAt(t float64) geom.Vec2      { return c.m.Apply(c.curve.PointAt(t)) }
func (c affineCurve) Tangent(t float64) geom.Vec2      { return c.m.ApplyVec(c.curve.Tangent(t)) }
func (c affineCurve) ParamRange() (float64, float64)   { return c.curve.ParamRange() }
func (c affineCurve) Transform(m geom.Mat3) geom.Curve { return affineCurve{c.curve, m.Mul(c.m)} }
func (c affineCurve) Tessellate(tol float64, dst []geom.Vec2) []geom.Vec2 {
	scale := max(math.Abs(c.m[0]), math.Abs(c.m[1]), math.Abs(c.m[2]), math.Abs(c.m[3]))
	if scale == 0 {
		return dst
	}
	norm := math.Hypot(c.m[0]/scale, c.m[1]/scale) + math.Hypot(c.m[2]/scale, c.m[3]/scale)
	for _, p := range c.curve.Tessellate((tol/scale)/norm, nil) {
		dst = append(dst, c.m.Apply(p))
	}
	return dst
}
func (c affineCurve) Bounds() geom.Box2 {
	switch v := c.curve.(type) {
	case geom.Line:
		return geom.BoxOf(c.m.Apply(v.A), c.m.Apply(v.B))
	case geom.Polyline:
		b := geom.EmptyBox()
		for _, part := range v.Segments() {
			b = b.Union((affineCurve{part, c.m}).Bounds())
		}
		return b
	case geom.Circle:
		return c.roundBounds(v.C, v.R, 0, 2*math.Pi, true)
	case geom.Arc:
		return c.roundBounds(v.C, v.R, v.Start, v.Start+v.Sweep(), false)
	}
	return geom.EmptyBox()
}
func (c affineCurve) roundBounds(center geom.Vec2, r, start, end float64, full bool) geom.Box2 {
	b := geom.EmptyBox()
	point := func(t float64) geom.Vec2 { return c.m.Apply(center.Polar(r, t)) }
	b = b.Extend(point(start)).Extend(point(end))
	for _, t := range []float64{math.Atan2(c.m[2], c.m[0]), math.Atan2(c.m[2], c.m[0]) + math.Pi, math.Atan2(c.m[3], c.m[1]), math.Atan2(c.m[3], c.m[1]) + math.Pi} {
		if full || geom.NormAngle(t-start) <= end-start {
			b = b.Extend(point(t))
		}
	}
	return b
}
func (c affineCurve) Length() float64 {
	ps := c.Tessellate(1e-5, nil)
	var n float64
	for i := 1; i < len(ps); i++ {
		n += ps[i].Dist(ps[i-1])
	}
	return n
}
func (c affineCurve) ClosestParam(p geom.Vec2) float64 {
	lo, hi := c.ParamRange()
	const steps = 128
	best, bestD := lo, math.Inf(1)
	for i := 0; i <= steps; i++ {
		v := lo + (hi-lo)*float64(i)/steps
		d := c.PointAt(v).Sub(p).Dot(c.PointAt(v).Sub(p))
		if d < bestD {
			best, bestD = v, d
		}
	}
	a, b := max(lo, best-(hi-lo)/steps), min(hi, best+(hi-lo)/steps)
	for i := 0; i < 64; i++ {
		x, y := a+(b-a)/3, b-(b-a)/3
		if c.PointAt(x).Dist(p) < c.PointAt(y).Dist(p) {
			b = y
		} else {
			a = x
		}
	}
	return (a + b) / 2
}

// SnapPoints returns transformed endpoints, midpoints, centres and quadrants.
func (c affineCurve) SnapPoints() (ep, mp, cp, qp []geom.Vec2) {
	switch v := c.curve.(type) {
	case geom.Circle:
		cp = append(cp, c.m.Apply(v.C))
		for i := 0; i < 4; i++ {
			qp = append(qp, c.PointAt(float64(i)*math.Pi/2))
		}
	case geom.Arc:
		ep = []geom.Vec2{c.m.Apply(v.StartPoint()), c.m.Apply(v.EndPoint())}
		mp = []geom.Vec2{c.m.Apply(v.MidPoint())}
		cp = []geom.Vec2{c.m.Apply(v.C)}
		for i := 0; i < 4; i++ {
			t := float64(i) * math.Pi / 2
			if v.ContainsAngle(t) {
				qp = append(qp, c.PointAt(t))
			}
		}
	case geom.Polyline:
		for _, seg := range v.Segments() {
			a, b, d, f := (affineCurve{seg, c.m}).SnapPoints()
			ep = append(ep, a...)
			mp = append(mp, b...)
			cp = append(cp, d...)
			qp = append(qp, f...)
		}
	case geom.Line:
		ep = []geom.Vec2{c.m.Apply(v.A), c.m.Apply(v.B)}
		mp = []geom.Vec2{c.m.Apply(v.Mid())}
	}
	return
}

// Explode returns independent objects for one level of a similarity-scaled insertion.
// An anisotropic insertion stays intact rather than changing circular geometry.
func (e *Insert) Explode() ([]Entity, error) {
	if e.d == nil || e.d.InsertError(e) != nil {
		return nil, fmt.Errorf("invalid block reference")
	}
	if math.Abs(math.Abs(e.ScaleX)-math.Abs(e.ScaleY)) > 1e-10*max(math.Abs(e.ScaleX), math.Abs(e.ScaleY)) {
		return nil, fmt.Errorf("cannot explode a nonuniformly scaled reference")
	}
	b := e.d.blocks[strings.ToUpper(e.BlockName)]
	if b == nil {
		return nil, fmt.Errorf("block definition does not exist")
	}
	m := geom.Translate(e.Position).Mul(geom.Rotate(geom.V(0, 0), e.Rotation)).Mul(geom.Mat3{e.ScaleX, 0, 0, e.ScaleY, 0, 0}).Mul(geom.Translate(b.Base.Scale(-1)))
	color := e.Color
	if color == ByLayer || color == ByBlock {
		color = 7
		if l := e.d.Layer(e.Layer); l != nil {
			color = l.Color
		}
	}
	seen := map[string]bool{}
	var out []Entity
	for _, child := range b.Entities {
		if _, ok := child.(*Unknown); ok {
			return nil, fmt.Errorf("cannot explode unsupported block objects")
		}
		var c Entity
		world := false
		if a, ok := child.(*AttributeDefinition); ok {
			seen[strings.ToUpper(a.Tag)] = true
			if a.Invisible {
				continue
			}
			t := a.Text.Clone().(*Text)
			if !a.Constant {
				var attr *Attribute
				for i := range e.Attributes {
					if strings.EqualFold(a.Tag, e.Attributes[i].Tag) {
						attr = &e.Attributes[i]
						break
					}
				}
				if attr == nil || attr.Flags&1 != 0 {
					continue
				}
				if attr.Text != nil {
					t = attr.Text.Clone().(*Text)
					world = true
				}
				t.Value = attr.Value
			}
			c = t
		} else {
			c = child.Clone().(Entity)
		}
		if c.Common().Layer == "0" || c.Common().Layer == "" {
			c.Common().Layer = e.Layer
		}
		if c.Common().Color == ByBlock {
			c.Common().Color = color
		}
		if !world {
			c.Transform(m)
		}
		out = append(out, c)
	}
	for _, a := range e.Attributes {
		if seen[strings.ToUpper(a.Tag)] || a.Text == nil || a.Flags&1 != 0 {
			continue
		}
		t := a.Text.Clone().(*Text)
		t.Value = a.Value
		if t.Layer == "0" || t.Layer == "" {
			t.Layer = e.Layer
		}
		if t.Color == ByBlock {
			t.Color = color
		}
		out = append(out, t)
	}
	return out, nil
}

func (e *Insert) initializeAttributes(d *Drawing) bool {
	limit, err := d.rootAttributeLimit(e)
	if err != nil {
		return false
	}
	b := d.blocks[strings.ToUpper(e.BlockName)]
	indices := make(map[string]int, len(e.Attributes))
	for i, a := range e.Attributes {
		key := strings.ToUpper(a.Tag)
		if _, seen := indices[key]; !seen {
			indices[key] = i
		}
	}
	missing := 0
	for _, child := range b.Entities {
		if a, ok := child.(*AttributeDefinition); ok && !a.Constant {
			if _, found := indices[strings.ToUpper(a.Tag)]; !found {
				missing++
			}
		}
	}
	if missing > limit-len(e.Attributes) {
		return false
	}

	m := geom.Translate(e.Position).Mul(geom.Rotate(geom.V(0, 0), e.Rotation)).Mul(geom.Mat3{e.ScaleX, 0, 0, e.ScaleY, 0, 0}).Mul(geom.Translate(b.Base.Scale(-1)))
	for _, child := range b.Entities {
		a, ok := child.(*AttributeDefinition)
		if !ok || a.Constant {
			continue
		}
		i, found := indices[strings.ToUpper(a.Tag)]
		if !found {
			i = -1
		}
		if i < 0 {
			flags := 0
			if a.Invisible {
				flags |= 1
			}
			e.Attributes = append(e.Attributes, Attribute{Tag: a.Tag, Value: a.Value, Flags: flags})
			i = len(e.Attributes) - 1
			indices[strings.ToUpper(a.Tag)] = i
		}
		if e.Attributes[i].Text == nil {
			if math.Abs(math.Abs(e.ScaleX)-math.Abs(e.ScaleY)) > 1e-10*max(math.Abs(e.ScaleX), math.Abs(e.ScaleY)) {
				return false
			}
			t := a.Text.Clone().(*Text)
			t.H = 0
			t.Value = e.Attributes[i].Value
			t.Transform(m)
			e.Attributes[i].Text = t
		}
	}
	return true
}

// IntersectCurve implements geom's optional intersection protocol for affine block curves.
func (c affineCurve) IntersectCurve(other geom.Curve, tol float64) ([]geom.Vec2, bool) {
	if p, ok := c.curve.(geom.Polyline); ok {
		var out []geom.Vec2
		for _, seg := range p.Segments() {
			out = append(out, geom.Intersect(affineTransform(seg, c.m), other, tol)...)
		}
		return uniquePoints(out, tol), true
	}
	if p, ok := other.(geom.Polyline); ok {
		var out []geom.Vec2
		for _, seg := range p.Segments() {
			ps, _ := c.IntersectCurve(seg, tol)
			out = append(out, ps...)
		}
		return uniquePoints(out, tol), true
	}
	inv, ok := inverseAffine(c.m)
	if !ok {
		return nil, true
	}
	if line, ok := other.(geom.Line); ok {
		local := geom.Line{A: inv.Apply(line.A), B: inv.Apply(line.B)}
		scale := math.Hypot(inv[0], inv[1]) + math.Hypot(inv[2], inv[3])
		ps := geom.Intersect(c.curve, local, tol*scale)
		for i := range ps {
			ps[i] = c.m.Apply(ps[i])
		}
		return uniquePoints(ps, tol), true
	}
	var center geom.Vec2
	var radius float64
	var arc *geom.Arc
	switch v := c.curve.(type) {
	case geom.Circle:
		center, radius = v.C, v.R
	case geom.Arc:
		center, radius = v.C, v.R
		arc = &v
	default:
		return nil, false
	}
	if radius <= 0 {
		return nil, true
	}
	lo, hi := other.ParamRange()
	value := func(t float64) float64 { q := inv.Apply(other.PointAt(t)).Sub(center); return q.Dot(q) - radius*radius }
	deriv := func(t float64) float64 {
		q := inv.Apply(other.PointAt(t)).Sub(center)
		return 2 * q.Dot(inv.ApplyVec(other.Tangent(t)))
	}
	bisect := func(f func(float64) float64, a, b float64) float64 {
		fa := f(a)
		for i := 0; i < 64; i++ {
			mid := (a + b) / 2
			fm := f(mid)
			if fm == 0 {
				return mid
			}
			if (fm > 0) == (fa > 0) {
				a = mid
				fa = fm
			} else {
				b = mid
			}
		}
		return (a + b) / 2
	}
	cuts := []float64{lo}
	const count = 128
	for i := 1; i <= count; i++ {
		a, b := lo+(hi-lo)*float64(i-1)/count, lo+(hi-lo)*float64(i)/count
		da, db := deriv(a), deriv(b)
		if da == 0 && a > lo {
			cuts = append(cuts, a)
		} else if da*db < 0 {
			cuts = append(cuts, bisect(deriv, a, b))
		}
	}
	cuts = append(cuts, hi)
	localTol := tol * (math.Hypot(inv[0], inv[1]) + math.Hypot(inv[2], inv[3]))
	epsilon := max(1e-12*radius*radius, localTol*2*radius)
	allZero := true
	for i := 0; i <= count; i++ {
		if math.Abs(value(lo+(hi-lo)*float64(i)/count)) > epsilon {
			allZero = false
			break
		}
	}
	if allZero {
		return nil, true
	}
	var params []float64
	for i, t := range cuts {
		if math.Abs(value(t)) <= epsilon {
			params = append(params, t)
		}
		if i > 0 && value(cuts[i-1])*value(t) < 0 {
			params = append(params, bisect(value, cuts[i-1], t))
		}
	}
	var out []geom.Vec2
	for _, t := range params {
		p := other.PointAt(t)
		if arc == nil || arc.ContainsAngle(inv.Apply(p).Sub(center).Angle()) {
			out = append(out, p)
		}
	}
	return uniquePoints(out, max(tol, 1e-9)), true
}
func inverseAffine(m geom.Mat3) (geom.Mat3, bool) {
	det := m.Det()
	if det == 0 || math.IsInf(det, 0) || math.IsNaN(det) {
		return geom.Mat3{}, false
	}
	inv := geom.Mat3{m[3] / det, -m[1] / det, -m[2] / det, m[0] / det, 0, 0}
	p := inv.ApplyVec(geom.V(m[4], m[5])).Scale(-1)
	inv[4], inv[5] = p.X, p.Y
	return inv, true
}
func uniquePoints(ps []geom.Vec2, tol float64) []geom.Vec2 {
	var out []geom.Vec2
	for _, p := range ps {
		found := false
		for _, q := range out {
			if p.Near(q, tol) {
				found = true
				break
			}
		}
		if !found {
			out = append(out, p)
		}
	}
	return out
}
