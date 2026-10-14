package ui

import (
	"go-draw/settings"
	"math"

	"go-draw/cmdsys"
	"go-draw/cmdsys/commands"
	"go-draw/doc"
	"go-draw/geom"
	"go-draw/internal/gpui"
	"go-draw/render"
	"go-draw/render/gpuirender"
)

// canvasState is the canvas's interaction state between events.
type canvasState struct {
	bounds   gpui.Bounds
	hover    bool
	mouse    gpui.Point // window coordinates
	pressed  bool
	pressAt  gpui.Point
	boxing   bool
	panning  bool
	panLast  gpui.Point
	dl       render.DisplayList
	hoverHit doc.Handle
	// sized is true once the view has been sized to a painted canvas.
	sized bool
	// ref is the scale and canvas size the view was last framed at by the
	// user (or a command); resizeView derives the scale for any later size
	// from it, so a resize and its undoing restore the view exactly.
	// resized is the scale resizeView last set; a different scale means the
	// user zoomed since and ref is stale.
	refScale, refW, refH, resized float64
}

// dragThreshold is how far (px) the pointer must move before a press
// becomes a box selection.
const dragThreshold = 4

// zoomPerPixel converts notch wheel pixels to a zoom factor exponent: one
// 20 px notch is a factor of about 1.087.
const zoomPerPixel = 1.0 / 240

// pinchPerPixel is the zoom exponent per pixel of precise Ctrl+wheel, which
// is how browsers and some platforms deliver a trackpad pinch. A single event
// is clamped to pinchMaxPixels so a burst cannot jump the view.
const (
	pinchPerPixel  = 1.0 / 100
	pinchMaxPixels = 200
)

func (ws *Workspace) local(p gpui.Point) (float64, float64) {
	return float64(p.X - ws.canvas.bounds.Origin.X), float64(p.Y - ws.canvas.bounds.Origin.Y)
}

func (ws *Workspace) model(p gpui.Point) geom.Vec2 {
	x, y := ws.local(p)
	return ws.view.ToModel(x, y)
}

// wantsPoint reports whether a click answers the prompt directly rather
// than picking or starting a box.
func (ws *Workspace) wantsPoint() bool {
	p := ws.session.CurrentPrompt()
	return p != nil && p.Accepts&(cmdsys.InputPoint|cmdsys.InputDistance|cmdsys.InputAngle) != 0
}

func (ws *Workspace) canvasElement() *gpui.Div {
	c := &ws.canvas
	return gpui.NewDiv().ID("canvas").Flex1().MinW(0).OverflowHidden().
		OnMouseDown(func(e *gpui.MouseDownEvent, w *gpui.Window, _ *gpui.App) {
			if fh := ws.input.FocusHandle(); fh != nil {
				w.RequestFocus(fh)
			}
			switch e.Button {
			case gpui.MouseMiddle:
				if e.ClickCount == 2 {
					c.panning = false
					commands.ZoomExtents(ws.session.Drawing(), ws.view)
					ws.pointerMoved(e.Position)
					break
				}
				c.panning, c.panLast = true, e.Position
			case gpui.MouseRight:
				ws.submit("", w)
			case gpui.MouseLeft:
				if ws.wantsPoint() {
					ws.session.Click(ws.model(e.Position), e.Modifiers&gpui.Shift != 0)
					ws.input.SetText("")
				} else {
					c.pressed, c.pressAt, c.boxing = true, e.Position, false
				}
			}
			w.Refresh()
		}).
		OnMouseMove(func(e *gpui.MouseMoveEvent, w *gpui.Window, _ *gpui.App) {
			ws.pointerMoved(e.Position)
			w.Refresh()
		}).
		OnHoverEnd(func(w *gpui.Window, _ *gpui.App) {
			c.hover = false
			w.Refresh()
		}).
		OnMouseUp(func(e *gpui.MouseUpEvent, w *gpui.Window, _ *gpui.App) {
			switch e.Button {
			case gpui.MouseMiddle:
				c.panning = false
			case gpui.MouseLeft:
				if c.pressed {
					if c.boxing {
						ws.session.SelectBox(ws.model(c.pressAt), ws.model(e.Position), e.Modifiers&gpui.Shift != 0)
					} else {
						ws.session.Click(ws.model(e.Position), e.Modifiers&gpui.Shift != 0)
					}
				}
				c.pressed, c.boxing = false, false
			}
			w.Refresh()
		}).
		OnScrollWheel(func(e *gpui.ScrollWheelEvent, w *gpui.Window, _ *gpui.App) {
			ws.wheel(e)
			ws.pointerMoved(e.Position)
			w.Refresh()
		}).
		Child(gpui.NewCanvas(ws.paintCanvas))
}

// wheel maps a scroll event to a view change:
//
//   - Ctrl or Cmd: zoom about the pointer, smoothly for precise deltas (a
//     pinch delivered as Ctrl+wheel) and by notch steps otherwise;
//   - Shift, or a horizontal notch: pan;
//   - precise deltas (a trackpad's two-finger scroll): pan by the deltas;
//   - a vertical notch: zoom about the pointer.
//
// With WheelZoom set, or the wheel setting "zoom", a precise vertical scroll
// zooms too, like a mouse wheel. With the setting "pan" every unmodified
// scroll, notch or precise, pans, and only Ctrl or Cmd zooms; "auto" is the
// behaviour above.
func (ws *Workspace) wheel(e *gpui.ScrollWheelEvent) {
	dx, dy := float64(e.DeltaX), float64(e.DeltaY)
	x, y := ws.local(e.Position)
	mode := ws.WheelMode()
	wheelZoom := ws.WheelZoom || mode == settings.WheelZoom
	switch {
	case e.Modifiers&(gpui.Control|gpui.ModPlatform) != 0:
		k := zoomPerPixel
		if e.Precise {
			k = pinchPerPixel
		}
		ws.view.ZoomAt(x, y, math.Exp(-math.Max(-pinchMaxPixels, math.Min(pinchMaxPixels, dy))*k))
	case e.Modifiers&gpui.Shift != 0:
		ws.view.PanBy(-dx, -dy)
	case mode == settings.WheelPan, e.Precise && !wheelZoom:
		ws.view.PanBy(-dx, -dy)
	case dx != 0 && dy == 0:
		ws.view.PanBy(-dx, 0)
	default:
		ws.view.ZoomAt(x, y, math.Exp(-dy*zoomPerPixel))
	}
}

// pointerMoved updates hover, pan, box and the session cursor.
func (ws *Workspace) pointerMoved(p gpui.Point) {
	c := &ws.canvas
	if c.panning {
		ws.view.PanBy(float64(p.X-c.panLast.X), float64(p.Y-c.panLast.Y))
		c.panLast = p
	}
	if c.pressed && !c.boxing {
		dx, dy := p.X-c.pressAt.X, p.Y-c.pressAt.Y
		c.boxing = dx*dx+dy*dy > dragThreshold*dragThreshold
	}
	c.mouse, c.hover = p, true
	m := ws.model(p)
	ws.session.Hover(m)
	c.hoverHit = 0
	if !ws.wantsPoint() && !c.boxing {
		if h, ok := cmdsys.Pick(ws.session.Drawing(), ws.session.Index(), m, ws.session.Aperture*ws.view.PixelSize()); ok {
			c.hoverHit = h
		}
	}
}

// paintCanvas is the Canvas callback: it sizes the view to the canvas and
// draws grid, entities, highlights, the command preview and the cursor.
func (ws *Workspace) paintCanvas(b gpui.Bounds, w *gpui.Window, _ *gpui.App) {
	c := &ws.canvas
	c.bounds = b
	v := ws.view
	before := *v
	ws.resizeView(b)
	if ws.fitPending && v.Width > 0 {
		commands.ZoomExtents(ws.session.Drawing(), v)
		ws.fitPending = false
	}
	if *v != before {
		// The model point under the pointer moved, and the status bar was
		// built before this frame changed the view.
		if c.hover {
			ws.pointerMoved(c.mouse)
		}
		w.Refresh()
	}
	w.PushClip(b)
	defer w.PopClip()
	w.PaintQuad(gpui.Quad{Bounds: b, Background: gpui.BackgroundSolid, Fill: gpuiColor(colCanvas)})

	dl := &c.dl
	dl.Reset()
	if ws.session.Grid() {
		ws.gridItems(dl)
	}
	d := ws.session.Drawing()
	ws.builder.Build(ws.session.Index().Query(v.Visible()), *v, dl)
	highlight := func(h doc.Handle, col render.Color, weight float32) {
		for _, it := range ws.builder.Items(h, *v) {
			it.Color, it.Weight = col, weight
			dl.Add(it)
		}
	}
	if c.hoverHit != 0 && d.Entity(c.hoverHit) != nil && !ws.session.Selection().Has(c.hoverHit) {
		highlight(c.hoverHit, colHover, 2)
	}
	for _, h := range ws.session.Selection().Handles() {
		highlight(h, colSelected, 2)
	}
	ws.session.Overlay(dl)
	if c.boxing {
		a, z := ws.model(c.pressAt), ws.model(c.mouse)
		if z.X < a.X {
			dl.Rect(a, z, colCrossSel, colCrossOut, 1)
		} else {
			dl.Rect(a, z, colWindowSel, colWindowOut, 1)
		}
	}
	ws.ucsIcon(dl)
	if ws.pointerOnCanvas() {
		ws.cursorItems(dl)
	}
	gpuirender.Painter{Origin: b.Origin, View: *v}.Paint(w, dl)
}

// resizeView sizes the view to the canvas. When the size of a canvas that
// has been shown changes (a window resize, the agent panel opening), the
// scale is adjusted so the model box that was visible stays fully visible
// around the same centre: the view shrinks or grows with the canvas rather
// than clipping. The new scale is always derived from the last scale the
// user chose (canvasState.refScale), so undoing a resize restores the view.
func (ws *Workspace) resizeView(b gpui.Bounds) {
	c, v := &ws.canvas, ws.view
	w, h := float64(b.Size.W), float64(b.Size.H)
	if w <= 0 || h <= 0 {
		return
	}
	if c.sized && !ws.fitPending && v.Width > 0 && v.Height > 0 && (w != v.Width || h != v.Height) {
		if v.Scale != c.resized {
			c.refScale, c.refW, c.refH = v.Scale, v.Width, v.Height
		}
		v.Scale = math.Max(render.MinScale, math.Min(render.MaxScale, c.refScale*math.Min(w/c.refW, h/c.refH)))
		c.resized = v.Scale
	}
	v.Width, v.Height = w, h
	c.sized = true
}

// gridItems draws the grid: lines every GRIDUNIT (x and y), widened by powers of ten until they
// are at least 16 px apart, with every GRIDMAJOR-th line stronger.
func (ws *Workspace) gridItems(dl *render.DisplayList) {
	v := ws.view
	sp, major := ws.session.GridSpacing(), ws.session.GridMajor()
	step := func(u float64) float64 {
		for u < 16*v.PixelSize() {
			u *= 10
		}
		return u
	}
	sx, sy := step(sp.X), step(sp.Y)
	vis := v.Visible()
	x0, x1 := math.Floor(vis.Min.X/sx), math.Ceil(vis.Max.X/sx)
	y0, y1 := math.Floor(vis.Min.Y/sy), math.Ceil(vis.Max.Y/sy)
	if x1-x0 > 400 || y1-y0 > 400 {
		return
	}
	for i := x0; i <= x1; i++ {
		col := colGrid
		if math.Mod(i, float64(major)) == 0 {
			col = colGridMajor
		}
		dl.Polyline([]geom.Vec2{{X: i * sx, Y: vis.Min.Y}, {X: i * sx, Y: vis.Max.Y}}, col, 0, false)
	}
	for j := y0; j <= y1; j++ {
		col := colGrid
		if math.Mod(j, float64(major)) == 0 {
			col = colGridMajor
		}
		dl.Polyline([]geom.Vec2{{X: vis.Min.X, Y: j * sy}, {X: vis.Max.X, Y: j * sy}}, col, 0, false)
	}
}

// ucsIcon draws the X/Y axis icon in the bottom-left corner.
func (ws *Workspace) ucsIcon(dl *render.DisplayList) {
	v := ws.view
	o := v.ToModel(24, v.Height-24)
	l := 36 * v.PixelSize()
	dl.Polyline([]geom.Vec2{o, o.Add(geom.V(l, 0))}, colAxisX, 2, false)
	dl.Polyline([]geom.Vec2{o, o.Add(geom.V(0, l))}, colAxisY, 2, false)
}

// cursorItems draws the crosshair (or pickbox when idle), and the snap
// marker of the current object snap.
func (ws *Workspace) cursorItems(dl *render.DisplayList) {
	v := ws.view
	px := v.PixelSize()
	cur := ws.model(ws.canvas.mouse)
	if ws.wantsPoint() {
		cur = ws.session.Cursor()
	}
	vis := v.Visible()
	dl.Polyline([]geom.Vec2{{X: vis.Min.X, Y: cur.Y}, {X: vis.Max.X, Y: cur.Y}}, colCrosshair, 0, false)
	dl.Polyline([]geom.Vec2{{X: cur.X, Y: vis.Min.Y}, {X: cur.X, Y: vis.Max.Y}}, colCrosshair, 0, false)
	if !ws.wantsPoint() {
		r := ws.session.Aperture * px
		dl.Polyline(square(cur, r), colCrosshair, 0, true)
	}
	if hit, ok := ws.session.Snap(); ok && ws.wantsPoint() {
		snapMarker(dl, hit, 6*px)
	}
}

func square(c geom.Vec2, r float64) []geom.Vec2 {
	return []geom.Vec2{{X: c.X - r, Y: c.Y - r}, {X: c.X + r, Y: c.Y - r}, {X: c.X + r, Y: c.Y + r}, {X: c.X - r, Y: c.Y + r}}
}

// snapMarker draws AutoCAD's marker shapes: square endpoint, triangle
// midpoint, circle center, diamond quadrant, X intersection, hourglass
// nearest.
func snapMarker(dl *render.DisplayList, hit cmdsys.SnapHit, r float64) {
	p := hit.Point
	switch hit.Mode {
	case cmdsys.SnapEndpoint:
		dl.Polyline(square(p, r), colSnap, 2, true)
	case cmdsys.SnapMidpoint:
		dl.Polyline([]geom.Vec2{{X: p.X - r, Y: p.Y - r}, {X: p.X + r, Y: p.Y - r}, {X: p.X, Y: p.Y + r}}, colSnap, 2, true)
	case cmdsys.SnapCenter:
		dl.Curve(geom.Circle{C: p, R: r}, r/20, colSnap, 2)
	case cmdsys.SnapQuadrant:
		dl.Polyline([]geom.Vec2{{X: p.X - r, Y: p.Y}, {X: p.X, Y: p.Y - r}, {X: p.X + r, Y: p.Y}, {X: p.X, Y: p.Y + r}}, colSnap, 2, true)
	case cmdsys.SnapIntersection:
		dl.Polyline([]geom.Vec2{{X: p.X - r, Y: p.Y - r}, {X: p.X + r, Y: p.Y + r}}, colSnap, 2, false)
		dl.Polyline([]geom.Vec2{{X: p.X - r, Y: p.Y + r}, {X: p.X + r, Y: p.Y - r}}, colSnap, 2, false)
	case cmdsys.SnapPerpendicular:
		dl.Polyline([]geom.Vec2{{X: p.X - r, Y: p.Y + r}, {X: p.X - r, Y: p.Y - r}, {X: p.X + r, Y: p.Y - r}}, colSnap, 2, false)
		dl.Polyline(square(geom.V(p.X-r/2, p.Y-r/2), r/2), colSnap, 2, true)
	case cmdsys.SnapTangent:
		dl.Curve(geom.Circle{C: geom.V(p.X, p.Y-r/4), R: r * 0.75}, r/20, colSnap, 2)
		dl.Polyline([]geom.Vec2{{X: p.X - r, Y: p.Y + r/2}, {X: p.X + r, Y: p.Y + r/2}}, colSnap, 2, false)
	default:
		dl.Polyline([]geom.Vec2{{X: p.X - r, Y: p.Y + r}, {X: p.X + r, Y: p.Y + r}, {X: p.X - r, Y: p.Y - r}, {X: p.X + r, Y: p.Y - r}}, colSnap, 2, true)
	}
}

// pointerOnCanvas reports whether a real pointer position is known and
// over the canvas, which is when the crosshair, pickbox and coordinates show.
func (ws *Workspace) pointerOnCanvas() bool {
	return ws.canvas.hover && ws.canvas.bounds.Contains(ws.canvas.mouse)
}

// ScreenPoint returns where the model point p appears in window
// coordinates, as of the last painted frame.
func (ws *Workspace) ScreenPoint(p geom.Vec2) gpui.Point {
	x, y := ws.view.ToScreen(p)
	o := ws.canvas.bounds.Origin
	return gpui.Point{X: o.X + gpui.Pixels(x), Y: o.Y + gpui.Pixels(y)}
}
