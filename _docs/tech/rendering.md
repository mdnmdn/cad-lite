---
id: tech-rendering
title: Rendering
kind: tech
status: current
summary: How entities become pixels — the view, camera-relative coordinates, the display list, the per-band tessellation cache and the gpui adapter.
read_when: you are changing what or how the canvas draws, adding a display primitive, or chasing jitter or a stale render
updated: 2026-10-15
verified: 2026-10-15
code_anchors: [doc/textfont.go, render/view.go, render/viewstack.go, render/displaylist.go, render/builder.go, doc/insert.go, render/gpuirender/paint.go, doc/aci.go, ui/agent.go]
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
`[MinScale, MaxScale]`. `View` does not react to a viewport resize; the canvas does (it rescales so the
visible box stays visible, see [Drawing canvas](../features/canvas.md)).
`View.Saved` is the stack of the last 10 views behind `ZOOM Previous` (`render/viewstack.go`,
created by `View.History`). A command that changes the view calls `Save` before and `Settle` after
(`changeView` in `cmdsys/commands/view.go`); `Settle` remembers the view the command left, so the next
`Save` or `Pop` notices a change made elsewhere (wheel, drag) and keeps it as a step of its own,
without the UI having to call the stack.

**Camera-relative coordinates.** `View.ToScreen` subtracts `Center` in `float64` before scaling, and
`gpuirender` narrows to `float32` only after that. Large coordinates (survey data in the millions)
therefore do not jitter: the narrowed values are always viewport-sized.

**Display list.** `render.DisplayList` is an ordered list of `DisplayItem`s, drawn first to last:

| Kind | Draws |
|---|---|
| `ItemPolyline` | `Points` stroked as a polyline of width `Weight` (`Builder.Lineweights`, the LWT toggle, widens it to the layer's lineweight) (0 is a one-device-pixel hairline), optionally `Closed` |
| `ItemRect` | The axis-aligned box spanned by `Points[0]` and `Points[1]`, filled with `Color` and outlined in `Outline` |

Points are model space and pre-tessellated; `Owner` names the entity an item draws, 0 for overlays.

**Text** has no primitive of its own (D34). TEXT and MTEXT are strokes of the built-in single-line font, laid
out in `doc` (see [Document model](document-model.md)), so `Entity.Geometry` hands the builder glyph polylines
like any other curves: rotation, obliquing, mirroring and justification are model-space geometry, the
camera-relative narrowing applies unchanged, and the canvas, `-png` renders, highlights and any future exporter
draw the same text. A text whose capital height is under `textGreekPx` (3 pixels at the lowest scale of the zoom
band, so between 3 and 6 pixels on screen; 3000 labels of 3.5 px render in 0.35 s for a whole `-png` run against
0.65 s with their glyphs, which are an unreadable smudge at that size) draws as one line per row of text (`doc.Annotation.Greek`), which costs one item instead of its glyphs.

**Builder and cache.** `render.Builder` turns an entity into display items: each curve of
`Entity.Geometry` tessellated, coloured by `Builder.EntityColor`. Items are cached per entity and
per **zoom band** — `render.Band(scale)` is `floor(log2(scale))`, and the band's chord tolerance
(`BandTolerance`) is a quarter pixel at the band's lowest scale — so zooming within a band reuses the
tessellation. `Invalidate` drops the entries of a change set's handles; `InvalidateAll` drops
everything (a background, theme or lineweight-display change). Layer edits are not journalled, so
after every command-line submission and every mutating MCP call the workspace calls `SyncLayers`
instead (D27): it drops everything only when the layer table's names, colours or lineweights differ
from those the cache was built against, so a single-entity edit keeps every other entity's
tessellation.

**Colour.** `ByLayer` resolves through the entity's layer. ACI 7 is black or white, whichever
contrasts with the background. `render.ACI` maps the other indices through `doc.ACIRGB`: 1–9 and the greys 250–255 are
AutoCAD's values, 10–249 follow the palette's hue and shade layout closely but not exactly.
`doc.NearestACI` is the inverse, used where a true colour must become an index (the DWG reader).

**Blocks.** `Builder.Items` resolves `Insert.Parts`, drawing each child with layer 0 and ByBlock
inheritance. Bounds include hidden children so later layer toggles need no index rebuild; picking and
display use visible geometry. `SyncLayers` includes off/frozen state, invalidating cached reference
parts when a child layer changes visibility. Definition changes and restoration invalidate every
reference through document change sets; live edits reach the same handles through `Txn.TakeEdits`.
Anisotropic references use affine circular curves rather than a similarity-scaled circle. Similarity
detection compares normalized axis directions and relative lengths across coordinate magnitudes;
unsafe determinant magnitudes retain the affine adapter. Tessellation scales its local tolerance
without an absolute minimum transform scale. Attribute
instances retain their own placement and values, and invisible attributes stay hidden.

## The frame

`ui.Workspace.paintCanvas` builds each frame in this order, so later items draw on top:

1. the canvas background and the grid;
2. the visible entities: `spatial.Index.Query(View.Visible())`, layers that are off or frozen skipped (`Drawing.EntityVisible`).
   The running command's work is among them: the session indexes its edits after every answer, and the canvas first
   passes `Session.TakeLiveEdits` to `Builder.Invalidate`, so an entity the command changed before it ends (a TRIM
   pick) draws in its current state (see the [command system](command-system.md));
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
- [Backlog](../backlog.md) — linetypes, lineweights, hatches and base-layer caching
