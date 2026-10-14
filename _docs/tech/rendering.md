---
id: tech-rendering
title: Rendering
kind: tech
status: current
summary: How entities become pixels — the view, camera-relative coordinates, the display list, the per-band tessellation cache and the gpui adapter.
read_when: you are changing what or how the canvas draws, adding a display primitive, or chasing jitter or a stale render
updated: 2026-10-14
verified: 2026-10-14
code_anchors: [render/view.go, render/displaylist.go, render/builder.go, render/gpuirender/paint.go]
depends_on: [tech-architecture, tech-document-model]
review_cycle: quarterly
---

# Rendering

## What it is

Rendering is split in two. `render` is backend-neutral: it owns the view (camera) and turns entities
into a display list of simple primitives in model space. `render/gpuirender` is the adapter that
draws a display list with the gpui library; it is the only rendering code that knows about gpui.
The canvas in `ui` assembles each frame's display list and hands it to the adapter.

## Model

**View.** `render.View` is the camera: `Center` (the model point at the middle of the viewport),
`Scale` (pixels per model unit) and the viewport size in logical pixels. Model space is y-up, the
viewport y-down. `ToScreen` and `ToModel` convert; `ZoomAt` zooms about a viewport pixel, keeping
the model point under it fixed; `PanBy` moves by pixels; `Fit` frames a box. Scale is clamped to
`[MinScale, MaxScale]`.

**Camera-relative coordinates.** `View.ToScreen` subtracts `Center` in `float64` before scaling, and
`gpuirender` narrows to `float32` only after that. Large coordinates (survey data in the millions)
therefore do not jitter: the narrowed values are always viewport-sized.

**Display list.** `render.DisplayList` is an ordered list of `DisplayItem`s, drawn first to last:

| Kind | Draws |
|---|---|
| `ItemPolyline` | `Points` stroked as a polyline of width `Weight` (0 is a one-device-pixel hairline), optionally `Closed` |
| `ItemRect` | The axis-aligned box spanned by `Points[0]` and `Points[1]`, filled with `Color` and outlined in `Outline` |

Points are model space and pre-tessellated; `Owner` names the entity an item draws, 0 for overlays.

**Builder and cache.** `render.Builder` turns an entity into display items: each curve of
`Entity.Geometry` tessellated, coloured by `Builder.EntityColor`. Items are cached per entity and
per **zoom band** — `render.Band(scale)` is `floor(log2(scale))`, and the band's chord tolerance
(`BandTolerance`) is a quarter pixel at the band's lowest scale — so zooming within a band reuses the
tessellation. `Invalidate` drops the entries of a change set's handles; `InvalidateAll` drops
everything and runs after any command-line input, because layer colour edits are not journalled.

**Colour.** `ByLayer` resolves through the entity's layer. ACI 7 is black or white, whichever
contrasts with the background. `render.ACI` maps the other indices: 1–9 and the greys 250–255 are
AutoCAD's values, 10–249 follow the palette's hue and shade layout closely but not exactly.

## The frame

`ui.Workspace.paintCanvas` builds each frame in this order, so later items draw on top:

1. the canvas background and the grid;
2. the visible entities: `spatial.Index.Query(View.Visible())`, layers that are off skipped;
3. the hover highlight, then the selection highlight (the entities' items recoloured and thickened);
4. the active command's preview (`Session.Overlay`);
5. the box-selection rectangle;
6. the coordinate-system icon, the crosshair and pickbox, and the snap marker.

`gpuirender.Painter.Paint` culls each item against the viewport, maps its points through the view,
and emits a `gpui.Stroke` per polyline and a `gpui.Quad` per rectangle. Stroke points live in one
slice allocated per frame, because the gpui scene reads them at raster time.

## Rationale

**Display list rather than direct drawing.** Commands draw their previews into the same list the
canvas uses, without knowing the backend; an SVG or PDF exporter and a GPU backend can consume the
same lists.

**Why every pointer move redraws the window.** gpui rasterizes the whole window on the CPU each
frame, and the crosshair moves with the pointer. That is fast enough for the drawings the spike
handles; base-layer caching, which keeps the drawing in an offscreen image and redraws only the
overlay, is the planned answer for large drawings and is in the backlog.

## Related docs

- [UI framework](ui-framework.md) — the `Stroke` primitive and `Canvas` element the adapter uses
- [Drawing canvas](../features/canvas.md) — what the user sees and does on the canvas
- [Backlog](../backlog.md) — linetypes, lineweights, text, hatches and base-layer caching
