package cadmcp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/png"
	"math"

	"cad-lite/cmdsys"
	"cad-lite/doc"
	"cad-lite/geom"
	"cad-lite/render"

	"agentmanager/mcp"
)

// SnapshotRequest describes one render_png image. View is already resolved:
// its Width and Height equal the image size and Center/Scale map model space
// to pixels (Y up in the model, down in the image). The size never exceeds
// MaxSnapshotPixels.
type SnapshotRequest struct {
	Width, Height int
	View          render.View
	Grid, Rulers  bool
	// Highlight lists entities to draw in the selection colour.
	Highlight []doc.Handle
}

// snapshotMargin is the space (px) left around the fitted extents so ticks
// and labels never cover the drawing.
const snapshotMargin = 48

const (
	defaultSnapW, defaultSnapH = 1024, 768
	minSnap, maxSnap           = 64, 2048
)

// MaxSnapshotPixels caps the pixel count of a render_png image (2 megapixels, e.g. 2048x1024). A
// larger request is scaled down uniformly, keeping its aspect ratio and the model window it asked
// for, because rendering runs on the UI goroutine and freezes the window while it does.
const MaxSnapshotPixels = 2048 * 1024

// capSize scales w x h down, keeping its aspect ratio, until it holds at most MaxSnapshotPixels
// pixels, and returns the new size and the factor applied (1 when it already fits).
func capSize(w, h int) (int, int, float64) {
	if w*h <= MaxSnapshotPixels {
		return w, h, 1
	}
	k := math.Sqrt(float64(MaxSnapshotPixels) / float64(w*h))
	cw, ch := max(1, int(float64(w)*k)), max(1, int(float64(h)*k))
	return cw, ch, float64(cw) / float64(w)
}

const schemaRenderPNG = `{"type":"object","properties":{` +
	`"width":{"type":"integer","minimum":64,"maximum":2048,"description":"image width in pixels, default 1024; width x height above 2 megapixels is scaled down keeping the aspect ratio and the view"},` +
	`"height":{"type":"integer","minimum":64,"maximum":2048,"description":"image height in pixels, default 768"},` +
	`"extents":{"type":"boolean","description":"fit the drawing extents with a margin (the default when no other view option is given)"},` +
	`"bbox":{"type":"array","items":{"type":"number"},"minItems":4,"maxItems":4,"description":"[minx,miny,maxx,maxy] model window to fit exactly"},` +
	`"center":{"type":"array","items":{"type":"number"},"minItems":2,"maxItems":2,"description":"[x,y] model point at the image centre; use with scale or zoom"},` +
	`"scale":{"type":"number","exclusiveMinimum":0,"description":"pixels per model unit (with center)"},` +
	`"zoom":{"type":"number","exclusiveMinimum":0,"description":"multiplier of the user's current canvas scale (with center; center defaults to the user's view centre)"},` +
	`"use_view":{"type":"boolean","description":"show the window's current canvas view, scaled to the image size"},` +
	`"grid":{"type":"boolean","description":"draw the grid, default true"},` +
	`"rulers":{"type":"boolean","description":"draw labelled coordinate ticks along the bottom and left edges, default true"},` +
	`"highlight":{"type":"array","items":{"type":"string"},"description":"hexadecimal entity handles to draw in the selection colour"}` +
	`},"additionalProperties":false}`

type renderArgs struct {
	Width, Height *int
	Extents       *bool
	BBox          []float64
	Center        []float64
	Scale, Zoom   *float64
	UseView       *bool `json:"use_view"`
	Grid, Rulers  *bool
	Highlight     []string
}

func orTrue(b *bool) bool { return b == nil || *b }

// resolveView turns the arguments into the view of a w x h image.
func resolveView(s *cmdsys.Session, a renderArgs, w, h int) (render.View, error) {
	d := s.Drawing()
	ui := *s.View()
	modes := 0
	if a.Extents != nil && *a.Extents {
		modes++
	}
	if a.BBox != nil {
		modes++
	}
	if a.Center != nil || a.Scale != nil || a.Zoom != nil {
		modes++
	}
	if a.UseView != nil && *a.UseView {
		modes++
	}
	if modes > 1 {
		return render.View{}, errors.New("give at most one of extents, bbox, center/scale/zoom and use_view")
	}
	v := render.DefaultView(float64(w), float64(h))
	clamp := func(sc float64) float64 { return math.Max(render.MinScale, math.Min(render.MaxScale, sc)) }
	switch {
	case a.BBox != nil:
		if len(a.BBox) != 4 {
			return v, errors.New("bbox must be [minx,miny,maxx,maxy]")
		}
		if err := finite(a.BBox...); err != nil {
			return v, err
		}
		if a.BBox[2] < a.BBox[0] || a.BBox[3] < a.BBox[1] || (a.BBox[2] == a.BBox[0] && a.BBox[3] == a.BBox[1]) {
			return v, errors.New("bbox must have maxx >= minx, maxy >= miny and a non-zero size")
		}
		v.Fit(geom.BoxOf(geom.V(a.BBox[0], a.BBox[1]), geom.V(a.BBox[2], a.BBox[3])), 0)
	case a.Center != nil || a.Scale != nil || a.Zoom != nil:
		if a.Center != nil && len(a.Center) != 2 {
			return v, errors.New("center must be [x,y]")
		}
		if a.Scale != nil && a.Zoom != nil {
			return v, errors.New("give scale or zoom, not both")
		}
		v.Center = ui.Center
		if a.Center != nil {
			if err := finite(a.Center...); err != nil {
				return v, err
			}
			v.Center = geom.V(a.Center[0], a.Center[1])
		}
		v.Scale = clamp(ui.Scale)
		switch {
		case a.Scale != nil:
			if !(*a.Scale > 0) || math.IsInf(*a.Scale, 0) {
				return v, errors.New("scale must be a positive number")
			}
			v.Scale = clamp(*a.Scale)
		case a.Zoom != nil:
			if !(*a.Zoom > 0) || math.IsInf(*a.Zoom, 0) {
				return v, errors.New("zoom must be a positive number")
			}
			v.Scale = clamp(ui.Scale * *a.Zoom)
		}
	case a.UseView != nil && *a.UseView && ui.Width > 0 && ui.Height > 0 && ui.Scale > 0:
		v.Fit(ui.Visible(), 0)
	default:
		fitExtents(&v, d.Extents())
	}
	return v, nil
}

func fitExtents(v *render.View, box geom.Box2) {
	if box.IsEmpty() {
		return
	}
	if wd, ht := box.Size(); wd < 1e-9 && ht < 1e-9 {
		v.Center = box.Center()
		return
	}
	v.Fit(box, snapshotMargin)
}

func renderPNG(e Env) mcp.Tool {
	required := requiredKeys(schemaRenderPNG)
	return mcp.Tool{
		Name:        "render_png",
		Description: "Render the drawing (model space only, no UI) to a PNG you can look at. By default fits the drawing extents with a coordinate ruler along the bottom and left edges and a grid. Choose the view with one of extents, bbox [minx,miny,maxx,maxy], center with scale (px per unit) or zoom, or use_view (the user's current canvas view). highlight draws entities in the selection colour. The JSON text result gives the visible model box and the pixel-to-model formula. Read only.",
		InputSchema: json.RawMessage(schemaRenderPNG),
		Handler: func(ctx context.Context, raw json.RawMessage) (mcp.Result, error) {
			var a renderArgs
			if err := decodeArgs(raw, &a, required); err != nil {
				return errResult("invalid arguments: %v", err), nil
			}
			if e.Snapshot == nil {
				return errResult("rendering not available"), nil
			}
			w, h := defaultSnapW, defaultSnapH
			if a.Width != nil {
				w = *a.Width
			}
			if a.Height != nil {
				h = *a.Height
			}
			if w < minSnap || w > maxSnap || h < minSnap || h > maxSnap {
				return errResult("width and height must be between %d and %d", minSnap, maxSnap), nil
			}
			cw, ch, k := capSize(w, h)
			var img image.Image
			var info map[string]any
			var ferr error
			err := e.Dispatch.Do(ctx, func() {
				s := e.Session()
				if s == nil {
					ferr = errors.New("no drawing is open")
					return
				}
				v, err := resolveView(s, a, w, h)
				if err != nil {
					ferr = err
					return
				}
				// The view is resolved at the requested size, then shrunk with the image so that a
				// capped render still shows the model window that was asked for.
				v.Scale *= k
				v.Width, v.Height = float64(cw), float64(ch)
				req := SnapshotRequest{Width: cw, Height: ch, View: v, Grid: orTrue(a.Grid), Rulers: orTrue(a.Rulers)}
				if len(a.Highlight) > 0 {
					if req.Highlight, err = parseHandles(s, a.Highlight); err != nil {
						ferr = err
						return
					}
				}
				if img, err = e.Snapshot(s, req); err != nil {
					ferr = fmt.Errorf("rendering failed: %v", err)
					return
				}
				info = snapshotInfo(s, v, cw, ch)
			})
			switch {
			case err != nil:
				return errResult("%v", err), nil
			case ferr != nil:
				return errResult("%v", ferr), nil
			}
			if k != 1 {
				info["requested"] = []int{w, h}
				info["downscaled"] = true
			}
			// PNG encoding is the costlier half and needs nothing the UI goroutine owns, so it runs
			// here, on the MCP handler's goroutine.
			var buf bytes.Buffer
			if err := png.Encode(&buf, img); err != nil {
				return errResult("encoding the image failed: %v", err), nil
			}
			text, _ := json.Marshal(info)
			return mcp.Result{Content: []mcp.Content{mcp.Image(buf.Bytes()), {Type: "text", Text: string(text)}}}, nil
		},
	}
}

func snapshotInfo(s *cmdsys.Session, v render.View, w, h int) map[string]any {
	d := s.Drawing()
	vis := v.Visible()
	n := 0
	for _, hd := range s.Index().Query(vis) {
		if en := d.Entity(hd); en != nil {
			if l := d.Layer(en.Common().Layer); l == nil || !l.Off {
				n++
			}
		}
	}
	return map[string]any{
		"width": w, "height": h,
		"view":                 map[string]any{"center": []float64{v.Center.X, v.Center.Y}, "scale": v.Scale},
		"bbox":                 []float64{vis.Min.X, vis.Min.Y, vis.Max.X, vis.Max.Y},
		"pixel_to_model":       "x = minx + px/scale, y = maxy - py/scale",
		"entity_count_visible": n,
	}
}
