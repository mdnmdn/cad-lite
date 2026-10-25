---
id: tech-rendering
title: Rendering
kind: tech
status: current
summary: How entities become pixels — the view, camera-relative coordinates, the display list, the per-band tessellation cache, the gpui adapter, and how frames are presented (render modes, software fallback).
read_when: you are changing what or how the canvas draws, adding a display primitive, chasing jitter or a stale render, or changing how the window presents frames (render modes, the software fallback)
updated: 2026-10-25
verified: 2026-10-25
code_anchors: [render/linetype.go, render/dimension.go, doc/properties.go, doc/linetype.go, doc/dimarrow.go, internal/gpui/raster_tiles.go, internal/gpui/ebitenplatform/damage.go, internal/gpui/ebitenplatform/graphics.go, cmd/cad/render.go, ui/rendermode.go, doc/textfont.go, render/view.go, render/viewstack.go, render/displaylist.go, render/builder.go, doc/insert.go, render/gpuirender/paint.go, doc/aci.go, ui/agent.go]
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
| `ItemPolyline` | `Points` stroked as a polyline of width `Weight` (0 is a one-device-pixel hairline), optionally `Closed`. With `Starts` set it is several open polylines, run `i` being `Points[Starts[i]:Starts[i+1]]`: the dashes of a linetype, one item per curve |
| `ItemRect` | The axis-aligned box spanned by `Points[0]` and `Points[1]`, filled with `Color` and outlined in `Outline` |
| `ItemFill` | The polygon `Points` (closed implicitly, even-odd) filled with `Color`, without an outline: filled arrowheads, dots and the architectural tick |

Points are model space and pre-tessellated; `Owner` names the entity an item draws, 0 for overlays.

**Text** has no primitive of its own (D34). TEXT and MTEXT are strokes of the built-in single-line font, laid
out in `doc` (see [Document model](document-model.md)), so `Entity.Geometry` hands the builder glyph polylines
like any other curves: rotation, obliquing, mirroring and justification are model-space geometry, the
camera-relative narrowing applies unchanged, and the canvas, `-png` renders, highlights and any future exporter
draw the same text. A text whose capital height is under `textGreekPx` (3 pixels at the lowest scale of the zoom
band, so between 3 and 6 pixels on screen; 3000 labels of 3.5 px render in 0.35 s for a whole `-png` run against
0.65 s with their glyphs, which are an unreadable smudge at that size) draws as one line per row of text (`doc.Annotation.Greek`), which costs one item instead of its glyphs.

**Builder and cache.** `render.Builder` turns an entity into display items: each curve of
`Entity.Geometry` tessellated and drawn with the entity's **effective appearance**, which the document
resolves (`Drawing.ResolveEntity`, `Insert.Parts`; see [Document model](document-model.md#property-inheritance-and-resolution)):
colour, linetype, linetype scale and lineweight come from the nearest level that sets them, the
entity, the enclosing reference (ByBlock), the layer (ByLayer) or the global defaults. The builder
never resolves a property itself. Items are cached per entity and
per **zoom band** — `render.Band(scale)` is `floor(log2(scale))`, and the band's chord tolerance
(`BandTolerance`) is a quarter pixel at the band's lowest scale — so zooming within a band reuses the
tessellation and the dashes. `Invalidate` drops the entries of a change set's handles and, when the
change set reports `Support` (a layer, linetype, dimension style, current property or global
default edited in a transaction, committed, undone or redone), syncs; `InvalidateAll` drops
everything (a background, theme or lineweight-display change). **Sync** (`SyncLayers`) compares what
entities inherit from against the state the cache was built with: each layer's name, colour,
linetype, lineweight, on and frozen state, the linetype table's names and patterns, the dimension
style table, LTSCALE and LWDEFAULT. It drops the whole cache only when one of them differs, so
changing only the current layer or colour keeps it; a full sync reuses the previous state's storage
(about 2 µs with the whole linetype library and 50 layers, `BenchmarkSyncUnchanged`; the copies left
are the document's own `Linetypes` and `DimStyles`). `Build` checks at the start of every frame, in
place and without copying, the parts that can change outside a transaction: the layers (handed out
as pointers), LTSCALE and LWDEFAULT; it syncs fully only when one of them moved. The linetype and
dimension style tables change only in a transaction, whose change set syncs, or when a file reader
loads a fresh drawing. So a layer field or a header value written in place by a host is still drawn
at the next frame, and a cached frame allocates nothing (its draw-order scratch is kept too). The workspace also syncs after every command-line
submission and every mutating MCP call (D27). A single-entity edit keeps every other entity's
tessellation.

**Linetypes** follow AutoCAD's display rules (`render/linetype.go`). The pattern
(`doc.Linetype.Pattern`: a positive length a dash, a negative one a gap, 0 a dot) runs in model
units, scaled by the resolved linetype scale (the entity's own times LTSCALE) and, inside a block
reference, by the accumulated scale of the references around the part (`BlockPart.Scale`, the
product over nested levels, so a 2x reference holding a 3x one scales it by 6). Its phase is fixed in
model space from the start of each curve, so a pan never moves a dash. It restarts on every entity
and on every polyline segment (AutoCAD's PLINEGEN 0, the default; the document has no PLINEGEN flag,
so a polyline never runs the pattern through its vertices). An open run whose pattern starts with
a dash is centred as AutoCAD lays it out: whole repetitions plus a closing first dash, with the slack
shared between the first and last dash, so a line begins and ends with a dash (a run with room for
one repetition but not the closing dash shortens both end dashes equally). Circles and other closed
curves run the pattern from their start point (angle 0 for a circle) without centring. A run shorter
than one repetition draws solid, as AutoCAD draws it. A dot draws as a dash one pixel long at the
band's lowest scale (1 to 2 px on screen), so it is visible at any zoom. Three cases draw solid:
- **Dense pattern.** A pattern whose repetition is under `minPeriodPx` (2 px at the band's lowest
  scale) would blur into a grey smear, so it draws solid, as AutoCAD regenerates it. The test is per
  band, so the cache stays valid within a band.
- **Too many dashes.** A run needing more than `maxDashes` (32768) elements draws solid. This
  bounds the cache for a line many screens long under a fine pattern, a deviation from AutoCAD,
  which would dash it.
- **Complex or missing linetype.** A complex linetype (`Linetype.Raw`, with shapes or text), a name
  the table lacks and a pattern without gaps draw solid.

Text (TEXT, MTEXT, attribute definitions and a dimension's text) always draws solid. A curve's dashes
are one `ItemPolyline` with `Starts`, not an item per dash, which keeps the cache and the display list
small.

**Lineweights.** With `Builder.Lineweights` on (the LWT toggle, `LWDISPLAY`), an entity draws as wide
as its resolved lineweight at 96 dpi (1.00 mm is 3.8 px), never thinner than `Weight`. Like AutoCAD's
model space, the width is in pixels and does not change with the zoom. Default resolves to LWDEFAULT
in the document, and 0.00 mm draws at `Weight`.

**Dimensions** draw from their layout (`Dimension.Layout`, `render/dimension.go`), not from
`Geometry`. The extension lines are drawn in DIMCLRE, the dimension line, its arc and the arrowheads
in DIMCLRD, and the text and the arc length symbol in DIMCLRT. Each colour is resolved against the
dimension's own appearance with `Drawing.ResolveColorIn`, where ByBlock is the dimension's colour.
The dimension and extension lines take the dimension's linetype (AutoCAD's DIMLTYPE and DIMLTEX1/2
default to ByBlock and are not modelled); the arrowheads and the text draw solid. Each arrowhead is
drawn as its `doc.DimArrow` describes it:
- the closed, small, box and datum shapes are a closed outline, filled with an `ItemFill` when
  `Filled`;
- the open V shapes and the oblique stroke are lines;
- the architectural tick is a filled bar `Width` wide in model units;
- the dots and the origin indicators are circles (dots filled);
- the integral sign is two arcs, and `_None` draws nothing.

Every fill also gets its outline, so an arrowhead smaller than a pixel still shows. A dimension
inside a block reference draws from `Insert.Parts`, which splits it by `BlockPart.Kind` into
the same colours and linetype treatment: lines patterned, arrowheads (`PartSolid`) and text
(`PartText`) solid. Parts carry curves only, so a block's arrowheads read solid through doc's fill
strokes rather than `ItemFill`.

**Colour.** The resolved colour is an ACI from 1 to 255 (`Builder.EntityColor` for a model-space
entity): `ByLayer` through the entity's layer, `ByBlock` through the enclosing reference and 7
outside one. ACI 7 is black or white, whichever
contrasts with the background. `render.ACI` maps the other indices through `doc.ACIRGB`: 1–9 and the greys 250–255 are
AutoCAD's values, 10–249 follow the palette's hue and shade layout closely but not exactly.
`doc.NearestACI` is the inverse, used where a true colour must become an index (the DWG reader).

**Blocks.** `Builder.Items` resolves `Insert.Parts`, drawing each child with the colour, linetype,
linetype scale and lineweight the part resolved to (layer 0 and ByBlock inheritance, through nested
references). Text parts (`PartText`, also the text of a dimension) draw solid whatever the linetype,
as top-level text does. Bounds include hidden children so later layer toggles need no index rebuild; picking and
display use visible geometry. `SyncLayers` includes off/frozen state, invalidating cached reference
parts when a child layer changes visibility. Definition changes and restoration invalidate every
reference through document change sets; live edits reach the same handles through `Txn.TakeEdits`.
Anisotropic references use affine circular curves rather than a similarity-scaled circle. Similarity
detection compares normalized axis directions and relative lengths across coordinate magnitudes;
unsafe determinant magnitudes retain the affine adapter. Tessellation scales its local tolerance
without an absolute minimum transform scale. Attribute
instances retain their own placement and values, and invisible attributes stay hidden.

**Layer stacking.** `Builder.Build` copies its candidates and sorts back to front by
`Drawing.LayerRank`, then ascending handle within each layer. The caller's handles and spatial index
remain unchanged. Layer-order notifications do not require retessellation: each Build reads the current
order. INSERT display items remain contiguous in their definition's authored order; the root entity's
layer positions the complete reference, while explicit child layers retain visibility and appearance.

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
and emits a `gpui.Stroke` per polyline and a `gpui.Quad` per rectangle. A dashed item emits one
stroke per dash and culls each dash on its own. gpui has no polygon fill, so an `ItemFill` is
scanned in window space into horizontal strokes one logical pixel apart and 1.25 px wide, over the
rows the viewport shows only. The strokes overlap, so no seam shows. Stroke points live in slices
allocated per frame, because the gpui scene reads them at raster time. A fill's polygon in window
coordinates and its row crossings never reach the scene, so they live in a `gpuirender.Scratch`
that the canvas keeps from frame to frame (`Painter.Scratch`; without one, one per `Paint` call).

**Cost of linetypes.** `render/bench_test.go` and `render/gpuirender/paint_test.go` measure 20k
entities spread over a 1600×1000 view, zoomed in so that ISO dashes are a few pixels long (about
410k dashes), on an Apple M2. A full rebuild (a new zoom band) takes 61 ms dashed against 34 ms
solid (`BenchmarkBuildDashed`, `BenchmarkBuildSolid`). A frame within the band reuses the cache
and builds in 7 ms without allocating (`BenchmarkBuildDashedCached`). A full headless frame, the
scene build plus the CPU raster, takes 127 ms dashed against 81 ms solid (`BenchmarkPaintDashed`,
`BenchmarkPaintSolid`). `BenchmarkPaintFill` paints 5,000 filled 16-gons; the fill scratch keeps its
allocations from growing with the number of fills.

## Presentation and render modes

Every frame is rasterized on the CPU by gpui (`internal/gpui/raster.go`) into an RGBA buffer, in
every mode, in the window and in `-png` renders alike. The window's platform, `ebitenplatform`,
only uploads the changed parts of the finished buffer with `WritePixels` and lets the graphics
device show it, so the graphics card does no drawing. What fails on old machines is the start: ebiten needs an OpenGL 3.2
core context, or on Windows a Direct3D 11 device at feature level 10, before the first frame.

Every image the application takes of itself comes from that CPU buffer, never from the device: the
feedback screenshot (`Window.Snapshot`, in `ui/feedback.go`), the snapshot key of `ebitenplatform`
and `-png` renders (a headless window, which ignores `-render`). They are identical in every mode.

The renderer therefore only chooses the device that presents:

| Mode | Device | Falls back |
|---|---|---|
| `auto` (default) | ebiten's own choice: DirectX or OpenGL on Windows, OpenGL on Linux, Metal on macOS | to `software`, on Windows, Linux and macOS |
| `gpu` | the same, chosen explicitly | never; a failed start exits with a hint to use `-render software` |
| `software` | Windows: Direct3D 11 WARP (`GraphicsLibraryDirectX`, `EBITENGINE_DIRECTX=warp`); Linux and the BSDs: Mesa llvmpipe (`GraphicsLibraryOpenGL`, `LIBGL_ALWAYS_SOFTWARE=1`, `GALLIUM_DRIVER=llvmpipe`); macOS: OpenGL instead of Metal (`GraphicsLibraryOpenGL`; macOS has no CPU presentation ebiten can reach) | not applicable |

The browser has no software switch: there the mode is always `auto` and the setting is hidden. The `-render` flag beats `CADLITE_RENDER`, which beats the `display.renderer` setting
(`resolveRenderMode` in `cmd/cad/render.go`; flags and variables in [Operations](operations.md)).

**Automatic fallback.** An `auto` start on Windows, Linux or macOS writes the marker file `render-probe`
beside the settings file and removes it once the first frame is presented
(`ebitenplatform.WithOnFirstFrame`). If `Platform.Run` returns an error wrapping
`ebitenplatform.ErrStartup` (the device or the window could not be created), `cmd/cad` starts its
own executable again with the same arguments, `CADLITE_RENDER=software` and
`CADLITE_RENDER_FALLBACK=1` (through `updater.Relaunch`, detached), and exits; the child does not
fall back again. A marker that is still there at the next `auto` start means the last start died
before its first frame (a driver crash rather than an error), so that start goes to software
directly and removes the marker. Either way the workspace shows a one-time notice with *Always use
software rendering*, which sets the setting (`ui/rendermode.go`). The child removes the two
variables at startup, and `ebitenplatform` restores the graphics variables after the first frame, so
a process the application starts later (a restart, an update) follows the setting again.

**Pacing.** `ebitenplatform` runs ebiten with `SetTPS(SyncWithFPS)`: one `Update`, and so at most
one rasterized frame, per presented frame. With the default 60 ticks a second, a frame slower than
16 ms (likely under WARP or llvmpipe on a large window) would make ebiten run catch-up `Update`s, each of which could
rasterize again. ebiten paces the loop itself when vsync is ignored or nothing is presented, so
timed redraws (`RequestFrameAt` deadlines) are still polled at least about 60 times a
second.

## Rationale

**Display list rather than direct drawing.** Commands draw their previews into the same list the
canvas uses, without knowing the backend; an SVG or PDF exporter, or a rasterizer that runs on the
graphics card, could consume the same lists.

**Software presentation rather than a native presenter.** Because frames are CPU-rasterized in any case,
software mode needs nothing but a device that can show a buffer; WARP and llvmpipe provide one
through the libraries ebiten uses anyway, so the window, input and clipboard code stay the same in
every mode (D46).

**What a pointer move costs.** The crosshair moves with the pointer, so every move builds a new
frame: the display list is painted into a fresh gpui scene. gpui then repaints only the 64x64
device-pixel tiles whose primitives changed (`Rasterizer.RenderDamage`: each primitive is hashed
into the tiles it touches, and a tile whose hash differs is cleared and redrawn, byte-identical to
a full render), and `ebitenplatform` uploads only those tiles, merged into strips, with sub-image
`WritePixels`. A crosshair move repaints the tiles along its two lines and the status readout, not
the window; pan and zoom change every tile and repaint in one pass. The scene build and the per-frame
hashing of every visible stroke still grow with the drawing: base-layer caching, which would keep
the drawing out of the per-move frame, is the remaining answer for large drawings and is in the
backlog.

## Related docs

- [UI framework](ui-framework.md) — the `Stroke` primitive and `Canvas` element the adapter uses
- [Drawing canvas](../features/canvas.md) — what the user sees and does on the canvas
- [Backlog](../backlog.md) — hatches, polyline widths and base-layer caching
