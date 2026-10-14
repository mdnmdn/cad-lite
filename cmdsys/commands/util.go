package commands

import (
	"fmt"
	"math"
	"strconv"
	"strings"

	"go-draw/cmdsys"
	"go-draw/doc"
	"go-draw/geom"
	"go-draw/render"
)

// immediate is a command that runs entirely in Start.
type immediate struct {
	base
	name string
	run  func(ctx *cmdsys.Context)
}

func (c *immediate) Name() string { return c.name }

func (c *immediate) Start(ctx *cmdsys.Context) *cmdsys.Prompt {
	c.run(ctx)
	return nil
}

func (c *immediate) Input(*cmdsys.Context, cmdsys.Input) *cmdsys.Prompt { return nil }

func undo(ctx *cmdsys.Context) {
	if !ctx.Doc.Undo() {
		ctx.Printf("Everything has been undone.")
	}
}

func redo(ctx *cmdsys.Context) {
	if !ctx.Doc.Redo() {
		ctx.Printf("Nothing to redo.")
	}
}

// ZoomMargin is the pixel margin ZOOM Extents leaves around the drawing.
const ZoomMargin = 24

// ZoomExtents fits the view to the drawing's extents.
func ZoomExtents(d *doc.Drawing, v *render.View) { v.Fit(d.Extents(), ZoomMargin) }

// --- ZOOM ---------------------------------------------------------------

type zoomCmd struct {
	base
	corner *geom.Vec2
}

func (*zoomCmd) Name() string { return "ZOOM" }

func (c *zoomCmd) Start(*cmdsys.Context) *cmdsys.Prompt {
	ext := cmdsys.Input{Kind: cmdsys.InputKeyword, Keyword: "Extents"}
	return &cmdsys.Prompt{Message: "Specify corner of window", Accepts: cmdsys.InputPoint | cmdsys.InputKeyword, Keywords: []string{"Extents", "Window"}, Default: &ext}
}

func (c *zoomCmd) Input(ctx *cmdsys.Context, in cmdsys.Input) *cmdsys.Prompt {
	switch {
	case in.Keyword == "Extents":
		ZoomExtents(ctx.Doc, ctx.View)
		return nil
	case in.Keyword == "Window":
		return &cmdsys.Prompt{Message: "Specify first corner", Accepts: cmdsys.InputPoint}
	case c.corner == nil:
		c.corner = pt(in.Point)
		return &cmdsys.Prompt{Message: "Specify opposite corner", Accepts: cmdsys.InputPoint, Base: c.corner}
	}
	ctx.View.Fit(geom.BoxOf(*c.corner, in.Point), 0)
	return nil
}

func (c *zoomCmd) Preview(_ *cmdsys.Context, cur geom.Vec2, dl *render.DisplayList) {
	if c.corner != nil {
		dl.Polyline(rectPts(*c.corner, cur), PreviewColor, 0, true)
	}
}

// --- LAYER --------------------------------------------------------------

type layerCmd struct{ base }

func (*layerCmd) Name() string { return "LAYER" }

func (*layerCmd) Start(ctx *cmdsys.Context) *cmdsys.Prompt {
	return &cmdsys.Prompt{Message: fmt.Sprintf("Layer name to make current, or name=color <%s>", ctx.Doc.CurrentLayer), Accepts: cmdsys.InputText | cmdsys.InputEnter}
}

func (*layerCmd) Input(ctx *cmdsys.Context, in cmdsys.Input) *cmdsys.Prompt {
	if in.Kind == cmdsys.InputEnter {
		return nil
	}
	name, col, hasColor := strings.Cut(in.Text, "=")
	name = strings.TrimSpace(name)
	color := 7
	if hasColor {
		n, err := strconv.Atoi(strings.TrimSpace(col))
		if err != nil || n < 1 || n > 255 {
			ctx.Printf("Color must be an AutoCAD Color Index from 1 to 255.")
			return nil
		}
		color = n
	}
	l := ctx.Doc.Layer(name)
	if l == nil {
		l = ctx.Doc.AddLayer(name, color)
		ctx.Printf("Layer %q created.", name)
	} else if hasColor {
		l.Color = color
	}
	ctx.Doc.CurrentLayer = l.Name
	return nil
}

// --- DIST, ID, LIST -----------------------------------------------------

type distCmd struct {
	base
	first *geom.Vec2
	multi bool
	pts   []geom.Vec2
	total float64
}

func (*distCmd) Name() string { return "DIST" }

func (*distCmd) Start(*cmdsys.Context) *cmdsys.Prompt {
	return &cmdsys.Prompt{Message: "Specify first point", Accepts: cmdsys.InputPoint | cmdsys.InputKeyword, Keywords: []string{"Multiple points"}}
}

func (c *distCmd) Input(ctx *cmdsys.Context, in cmdsys.Input) *cmdsys.Prompt {
	if in.Keyword == "Multiple points" {
		c.multi = true
		return &cmdsys.Prompt{Message: "Specify first point", Accepts: cmdsys.InputPoint}
	}
	if c.multi {
		if in.Kind == cmdsys.InputEnter {
			ctx.Printf("Distance = %s", cmdsys.FormatNumber(c.total))
			return nil
		}
		if n := len(c.pts); n > 0 {
			c.total += c.pts[n-1].Dist(in.Point)
		}
		c.pts = append(c.pts, in.Point)
		return &cmdsys.Prompt{Message: "Specify next point", Accepts: cmdsys.InputPoint | cmdsys.InputEnter, Base: pt(in.Point)}
	}
	if c.first == nil {
		c.first = pt(in.Point)
		return &cmdsys.Prompt{Message: "Specify second point", Accepts: cmdsys.InputPoint, Base: c.first}
	}
	d := in.Point.Sub(*c.first)
	ctx.Printf("Distance = %s,  Angle = %s°,  Delta X = %s,  Delta Y = %s",
		cmdsys.FormatNumber(d.Len()), cmdsys.FormatNumber(geom.NormAngle(d.Angle())*180/math.Pi),
		cmdsys.FormatNumber(d.X), cmdsys.FormatNumber(d.Y))
	return nil
}

func (c *distCmd) Preview(_ *cmdsys.Context, cur geom.Vec2, dl *render.DisplayList) {
	if c.multi && len(c.pts) > 0 {
		dl.Polyline(append(append([]geom.Vec2(nil), c.pts...), cur), PreviewColor, 0, false)
		return
	}
	if c.first != nil {
		dl.Polyline([]geom.Vec2{*c.first, cur}, PreviewColor, 0, false)
	}
}

type idCmd struct{ base }

func (*idCmd) Name() string { return "ID" }

func (*idCmd) Start(*cmdsys.Context) *cmdsys.Prompt {
	return &cmdsys.Prompt{Message: "Specify point", Accepts: cmdsys.InputPoint}
}

func (*idCmd) Input(ctx *cmdsys.Context, in cmdsys.Input) *cmdsys.Prompt {
	ctx.Printf("X = %s     Y = %s", cmdsys.FormatNumber(in.Point.X), cmdsys.FormatNumber(in.Point.Y))
	return nil
}

type listCmd struct {
	base
	selecting
}

func (*listCmd) Name() string { return "LIST" }

func (c *listCmd) Start(ctx *cmdsys.Context) *cmdsys.Prompt {
	if c.begin(ctx) {
		c.list(ctx)
		return nil
	}
	return selectPrompt()
}

func (c *listCmd) Input(ctx *cmdsys.Context, in cmdsys.Input) *cmdsys.Prompt {
	if c.take(ctx, in) {
		c.list(ctx)
	}
	return nil
}

func (c *listCmd) list(ctx *cmdsys.Context) {
	for _, h := range c.hs {
		e := ctx.Doc.Entity(h)
		if e == nil {
			continue
		}
		ctx.Printf("%s  Layer: %q  Handle = %s", e.Class(), e.Common().Layer, h)
		switch e := e.(type) {
		case *doc.Line:
			ctx.Printf("    from point %s  to point %s  Length = %s", cmdsys.FormatPoint(e.Start), cmdsys.FormatPoint(e.End), cmdsys.FormatNumber(e.Start.Dist(e.End)))
		case *doc.Circle:
			ctx.Printf("    center point %s  radius %s", cmdsys.FormatPoint(e.Center), cmdsys.FormatNumber(e.Radius))
		case *doc.Arc:
			ctx.Printf("    center point %s  radius %s  start angle %s°  end angle %s°", cmdsys.FormatPoint(e.Center), cmdsys.FormatNumber(e.Radius),
				cmdsys.FormatNumber(e.StartAngle*180/math.Pi), cmdsys.FormatNumber(e.EndAngle*180/math.Pi))
		case *doc.LWPolyline:
			ctx.Printf("    %d vertices  closed: %v  length %s", len(e.Points), e.Closed, cmdsys.FormatNumber(e.Geometry()[0].Length()))
		}
	}
}

// --- NEW, OPEN, SAVE ----------------------------------------------------

type fileCmd struct {
	base
	name string
}

func (c *fileCmd) Name() string { return c.name }

func (c *fileCmd) Start(ctx *cmdsys.Context) *cmdsys.Prompt {
	if ctx.Files == nil {
		ctx.Printf("File commands are not available on this platform.")
		return nil
	}
	verb := "open"
	if c.name == "SAVE" {
		verb = "save"
	}
	return &cmdsys.Prompt{Message: "Enter path of drawing to " + verb, Accepts: cmdsys.InputText}
}

func (c *fileCmd) Input(ctx *cmdsys.Context, in cmdsys.Input) *cmdsys.Prompt {
	if c.name == "SAVE" {
		warns, err := ctx.Files.Save(in.Text, ctx.Doc)
		if err != nil {
			ctx.Printf("Save failed: %v", err)
			return nil
		}
		printWarnings(ctx, warns)
		ctx.Printf("Saved %s (%d entities).", in.Text, ctx.Doc.Len())
		return nil
	}
	d, warns, err := ctx.Files.Open(in.Text)
	if err != nil {
		ctx.Printf("Open failed: %v", err)
		return nil
	}
	printWarnings(ctx, warns)
	ctx.ReplaceDrawing(d)
	ZoomExtents(d, ctx.View)
	ctx.Printf("Opened %s (%d entities).", in.Text, d.Len())
	return nil
}

func printWarnings(ctx *cmdsys.Context, warns []string) {
	const shown = 5
	for i, w := range warns {
		if i == shown {
			ctx.Printf("... and %d more warnings.", len(warns)-shown)
			break
		}
		ctx.Printf("Warning: %s", w)
	}
}

// Aliases are the default command aliases, in acad.pgp format.
const Aliases = `; go-draw default aliases (acad.pgp format)
L,     *LINE
PL,    *PLINE
C,     *CIRCLE
A,     *ARC
REC,   *RECTANG
E,     *ERASE
M,     *MOVE
CO,    *COPY
CP,    *COPY
RO,    *ROTATE
SC,    *SCALE
U,     *UNDO
Z,     *ZOOM
LA,    *LAYER
DI,    *DIST
LI,    *LIST
AA,    *AREA
MEA,   *MEASUREGEOM
`

// Register adds every built-in command and the default aliases to r.
func Register(r *cmdsys.Registry) {
	r.Register("LINE", func() cmdsys.Command { return &lineCmd{} })
	r.Register("PLINE", func() cmdsys.Command { return &plineCmd{} })
	r.Register("CIRCLE", func() cmdsys.Command { return &circleCmd{} })
	r.Register("ARC", func() cmdsys.Command { return &arcCmd{} })
	r.Register("RECTANG", func() cmdsys.Command { return &rectCmd{} })
	r.Register("ERASE", func() cmdsys.Command { return &eraseCmd{} })
	r.Register("MOVE", func() cmdsys.Command { return &moveCmd{} })
	r.Register("COPY", func() cmdsys.Command { return &moveCmd{copy: true} })
	r.Register("ROTATE", func() cmdsys.Command { return &transformCmd{} })
	r.Register("SCALE", func() cmdsys.Command { return &transformCmd{scale: true} })
	r.Register("UNDO", func() cmdsys.Command { return &immediate{name: "UNDO", run: undo} })
	r.Register("REDO", func() cmdsys.Command { return &immediate{name: "REDO", run: redo} })
	r.Register("ZOOM", func() cmdsys.Command { return &zoomCmd{} })
	r.Register("LAYER", func() cmdsys.Command { return &layerCmd{} })
	r.Register("DIST", func() cmdsys.Command { return &distCmd{} })
	r.Register("AREA", func() cmdsys.Command { return &areaCmd{} })
	r.Register("MEASUREGEOM", func() cmdsys.Command { return &measureGeomCmd{} })
	r.Register("ID", func() cmdsys.Command { return &idCmd{} })
	r.Register("LIST", func() cmdsys.Command { return &listCmd{} })
	r.Register("NEW", func() cmdsys.Command {
		return &immediate{name: "NEW", run: func(ctx *cmdsys.Context) { ctx.ReplaceDrawing(doc.New()) }}
	})
	r.Register("OPEN", func() cmdsys.Command { return &fileCmd{name: "OPEN"} })
	r.Register("SAVE", func() cmdsys.Command { return &fileCmd{name: "SAVE"} })
	r.LoadAliases(Aliases)
}

// NewRegistry returns a registry with every built-in command registered.
func NewRegistry() *cmdsys.Registry {
	r := cmdsys.NewRegistry()
	Register(r)
	return r
}
