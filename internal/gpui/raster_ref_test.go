package gpui

// scaleScene is the reference the damage-tracking tests compare against: the
// scene scaled from logical to device pixels into a stored copy, as Window
// rasterized it before primitives were scaled while binning. At scale 1 it
// returns s.
func scaleScene(s *Scene, scale float32, out *Scene) *Scene {
	k := Pixels(scale)
	if scale == 1 {
		return s
	}
	sb := func(b Bounds) Bounds {
		return Bounds{
			Origin: Point{X: b.Origin.X * k, Y: b.Origin.Y * k},
			Size:   Size{W: b.Size.W * k, H: b.Size.H * k},
		}
	}
	sc := func(c Corners) Corners {
		return Corners{TopLeft: c.TopLeft * k, TopRight: c.TopRight * k, BottomLeft: c.BottomLeft * k, BottomRight: c.BottomRight * k}
	}
	sm := func(m ContentMask) ContentMask {
		if m.Bounds.Size.W >= 1e6 {
			return m
		}
		return ContentMask{Bounds: sb(m.Bounds), CornerRadii: sc(m.CornerRadii)}
	}
	out.Clear()
	for _, q := range s.Quads {
		q.Bounds = sb(q.Bounds)
		q.ContentMask = sm(q.ContentMask)
		q.BorderWidths = Edges{Top: q.BorderWidths.Top * k, Right: q.BorderWidths.Right * k, Bottom: q.BorderWidths.Bottom * k, Left: q.BorderWidths.Left * k}
		q.CornerRadii = sc(q.CornerRadii)
		out.Quads = append(out.Quads, q)
	}
	for _, sh := range s.Shadows {
		sh.Bounds = sb(sh.Bounds)
		sh.Clip = sb(sh.Clip)
		sh.ContentMask = sm(sh.ContentMask)
		sh.BlurRadius *= k
		sh.CornerRadii = sc(sh.CornerRadii)
		sh.ClipRadii = sc(sh.ClipRadii)
		out.Shadows = append(out.Shadows, sh)
	}
	for _, u := range s.Underlines {
		u.Bounds = sb(u.Bounds)
		u.ContentMask = sm(u.ContentMask)
		u.Thickness *= k
		out.Underlines = append(out.Underlines, u)
	}
	for _, g := range s.Sprites {
		g.Bounds = sb(g.Bounds)
		g.ContentMask = sm(g.ContentMask)
		out.Sprites = append(out.Sprites, g)
	}
	for _, st := range s.Strokes {
		pts := make([]Point, 0, len(st.Points))
		for _, p := range st.Points {
			pts = append(pts, Point{X: p.X * k, Y: p.Y * k})
		}
		st.Points = pts
		st.ContentMask = sm(st.ContentMask)
		st.Width *= k
		out.Strokes = append(out.Strokes, st)
	}
	return out
}
