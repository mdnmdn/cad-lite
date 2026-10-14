# 2D CAD Application — Architecture Specification

**Status:** Draft v0.1
**Date:** 2026-10-14
**Language / stack:** Go + Ebitengine (rendering) + Dear ImGui via cimgui-go (tool UI)
**Target formats:** DWG (native, read/write), DXF (read/write), SVG and PDF (export)

---

## 1. Overview

A 2D drafting application in the spirit of AutoCAD: command-line driven, precise (double-precision) geometry, layers, blocks, object snaps, and lossless round-tripping of DWG/DXF drawings.

### 1.1 Goals

- Open, edit and save DWG/DXF files **without losing data**, including objects the app does not understand.
- AutoCAD-style interaction: command line, prompts, object snaps, ortho/polar tracking, grips, window/crossing selection.
- Smooth pan/zoom on drawings of 100k+ entities.
- Cross-platform desktop (Windows, macOS, Linux); browser (WASM) as a secondary target.
- A core that is pure Go, headless-testable and independent of the rendering backend.

### 1.2 Non-goals (v1)

- 3D modelling, solids, rendering of 3D views.
- Parametric constraints.
- Full paper-space plotting fidelity (plot styles/CTB) — later milestone.
- Collaboration / cloud sync.

---

## 2. Layered architecture

```
┌─────────────────────────────────────────────────────────────┐
│ UI shell        command line · palettes · layer manager      │
├──────────────────────────────┬──────────────────────────────┤
│ Commands & tools             │ Renderer                      │
│ snaps · grips · selection    │ display list · Ebitengine     │
├──────────────────────────────┴──────────────────────────────┤
│ Document database   entities · tables · blocks · handles     │
│                     transactions · undo journal              │
├─────────────────────────────────────────────────────────────┤
│ File I/O plugins    DXF · DWG · SVG export · PDF export      │
├──────────────────────────────┬──────────────────────────────┤
│ Geometry kernel              │ Spatial index (R-tree)        │
└──────────────────────────────┴──────────────────────────────┘
```

**Dependency rule:** arrows point downward only. The document database, geometry kernel, spatial index and file I/O must never import Ebitengine or ImGui. Only `render/ebitenrender` and `ui/` touch the graphics stack.

**Data flow for an edit:**

1. User input (keyboard, mouse) → UI shell → active command.
2. Command opens a transaction on the document and modifies entities.
3. Transaction commit emits a change set.
4. Change set updates the spatial index and invalidates renderer caches.
5. Renderer rebuilds affected display-list items and marks the frame dirty.

---

## 3. Go package layout

```
cad/
├── cmd/cad/                 main: wires everything, starts ebiten.RunGame
├── geom/                    geometry kernel (pure Go, no deps)
│   ├── vec.go               Vec2, Mat3 (affine 2D), Box2
│   ├── curves.go            Line, Arc, Circle, Ellipse, Polyline (bulge), Spline (NURBS)
│   ├── intersect.go         curve/curve intersections
│   ├── offset.go            offsets, fillet, chamfer
│   ├── tessellate.go        curve → polyline at a given chord tolerance
│   └── tol.go               tolerance policy
├── doc/                     document database
│   ├── handle.go            Handle (uint64), allocator
│   ├── object.go            Object interface, ObjectID
│   ├── entities/            Line, Arc, Circle, LWPolyline, Text, MText, Insert, Hatch, Dimension …
│   ├── tables/              Layer, Linetype, TextStyle, DimStyle, BlockRecord, Layout
│   ├── unknown.go           opaque preservation of unsupported objects
│   ├── txn.go               Transaction, ChangeSet, Journal (undo/redo)
│   └── events.go            change notifications
├── spatial/                 R-tree over entity bounding boxes
├── cmdsys/                  command framework
│   ├── registry.go          command name/alias → factory
│   ├── prompt.go            prompt/input state machine (point, distance, angle, keyword, selection)
│   ├── snap/                object snaps
│   ├── track/               ortho, polar, object snap tracking
│   ├── selection/           pick, window, crossing, fence, selection sets
│   ├── grips/               grip editing
│   └── commands/            LINE, CIRCLE, ARC, PLINE, MOVE, COPY, ROTATE, TRIM, EXTEND, OFFSET …
├── render/
│   ├── view.go              View (camera): origin float64, scale, rotation, viewport size
│   ├── displaylist.go       backend-neutral primitives
│   ├── builder.go           entity → display-list items (with cache)
│   └── ebitenrender/        Ebitengine adapter (the only package importing ebiten)
├── io/
│   ├── format.go            Reader / Writer interfaces, registry
│   ├── dxf/                 native Go DXF reader/writer
│   ├── dwg/                 DWG plugin (cgo; build tag `dwg`)
│   ├── svgexport/
│   └── pdfexport/
├── ui/                      ImGui panels: command line, properties, layers, status bar
└── internal/testutil/       golden-file helpers, fixture drawings
```

---

## 4. Geometry kernel (`geom`)

### 4.1 Numeric policy

- All model-space coordinates are `float64`.
- Tolerances are explicit and drawing-relative:
  - `LinearTol` default `1e-9 × drawing extent`, clamped to a minimum of `1e-10`.
  - `AngularTol` default `1e-10` rad.
- No `float32` anywhere in `geom` or `doc`.

### 4.2 Core types

```go
type Vec2 struct{ X, Y float64 }
type Box2 struct{ Min, Max Vec2 }
type Mat3 [6]float64 // 2D affine: a b c d e f

type Curve interface {
    Bounds() Box2
    PointAt(t float64) Vec2
    Tangent(t float64) Vec2
    ParamRange() (t0, t1 float64)
    ClosestParam(p Vec2) float64
    Length() float64
    Transform(m Mat3) Curve
    Tessellate(chordTol float64, dst []Vec2) []Vec2
}
```

Implementations: `Line`, `Arc`, `Circle`, `Ellipse` (incl. elliptical arcs), `Polyline` (vertices with **bulge** values, matching DWG LWPOLYLINE), `Spline` (NURBS with control points, knots, weights; fit points kept for round-trip).

### 4.3 Operations

| Operation | Needed by |
|---|---|
| Curve/curve intersection (all pairs) | TRIM, EXTEND, INTersection snap, FILLET |
| Closest point / perpendicular foot | NEArest / PERpendicular snap, picking |
| Tangent points from external point | TANgent snap, CIRCLE TTR |
| Offset (incl. polyline with arcs) | OFFSET |
| Split at parameter | TRIM, BREAK |
| Fillet / chamfer | FILLET, CHAMFER |
| Tessellation by chord tolerance | renderer, SVG/PDF export, hatch |
| Boolean regions (polygon clipping) | HATCH boundary detection |

---

## 5. Document database (`doc`)

The internal model **mirrors the DWG/DXF object model** so that every file round-trips losslessly.

### 5.1 Objects and handles

- Every object has a unique 64-bit `Handle`, preserved from the source file. New objects get handles from the drawing's `HANDSEED`.
- Objects reference each other by handle (owner, layer, linetype, block record…), never by Go pointer in persisted state. A resolved-pointer cache may exist in memory.

```go
type Handle uint64

type Object interface {
    Handle() Handle
    OwnerHandle() Handle
    Class() string            // DXF class name, e.g. "LINE", "LAYER"
    XData() []XDataGroup      // application data, preserved verbatim
    ExtDict() Handle          // extension dictionary (0 if none)
    Clone() Object
}

type Entity interface {
    Object
    Common() *EntityCommon    // layer, linetype, color, lineweight, ltscale, visibility, transparency
    Bounds() geom.Box2
    Geometry() []geom.Curve   // for snapping, picking, tessellation
    Transform(m geom.Mat3)
}
```

### 5.2 Symbol tables and dictionaries

- Tables: `LAYER`, `LTYPE`, `STYLE` (text), `DIMSTYLE`, `BLOCK_RECORD`, `UCS`, `VIEW`, `VPORT`, `APPID`.
- Named object dictionary: `LAYOUT`, `GROUP`, `MLINESTYLE`, etc.
- Header variables (`$INSUNITS`, `$LTSCALE`, `$DIMSCALE`, `$HANDSEED`…) stored in a typed struct plus a map for unrecognised variables.

### 5.3 Spaces and blocks

- **Model space** and each **paper-space layout** are block records owning entity lists.
- **Block definitions** are block records; **INSERT** entities reference them with position, scale, rotation and attributes.
- Nested inserts are supported; the renderer and snapper walk them with a transform stack.

### 5.4 Entity support (v1)

LINE, CIRCLE, ARC, ELLIPSE, LWPOLYLINE, POLYLINE (2D), SPLINE, POINT, TEXT, MTEXT, INSERT, ATTDEF/ATTRIB, HATCH (solid + pattern), DIMENSION (linear, aligned, radial, diametric, angular — display via anonymous block), LEADER/MLEADER (display only), SOLID, WIPEOUT, IMAGE (reference only), VIEWPORT.

### 5.5 Unknown-object preservation

Any object class the app does not model (custom/proxy objects, vertical-app entities) is stored as an `UnknownObject` holding its raw DXF group codes or DWG bit-stream, its handle and its owner. It is:

- displayed via its proxy graphics if present,
- never editable,
- written back byte-for-byte on save.

### 5.6 Transactions and undo

All modifications happen inside a transaction. Commands never mutate entities directly.

```go
type Txn interface {
    Add(e Entity) Handle
    Modify(h Handle, fn func(Object))   // snapshot before fn runs
    Erase(h Handle)
    Commit() ChangeSet
    Abort()
}

type ChangeSet struct {
    Added, Modified, Erased []Handle
}
```

- The **journal** stores before/after snapshots per transaction → undo/redo.
- `UNDO` groups: a command may span several transactions (e.g. interactive TRIM) but undoes as one unit.
- Commit publishes the `ChangeSet` on an event bus consumed by the spatial index, renderer cache, and UI panels.

---

## 6. Spatial index (`spatial`)

- R-tree (or R*-tree) of entity bounding boxes per space (model space, each layout).
- Insert entities indexed by their transformed extents; nested contents resolved on query.
- Queries:
  - `Query(box)` — viewport culling, window/crossing selection.
  - `Nearest(point, radius)` — picking, snap candidate gathering.
- Updated incrementally from `ChangeSet`s; bulk-loaded (STR packing) after file open.

---

## 7. Command system (`cmdsys`)

### 7.1 Command model

Each command is a state machine that asks for typed input through prompts, mirroring the AutoCAD command line.

```go
type Command interface {
    Name() string
    Start(ctx *Context) Prompt          // first prompt
    Input(ctx *Context, in Input) Prompt // returns next prompt, or nil when done
    Preview(ctx *Context, cursor geom.Vec2, dl *render.DisplayList) // rubber-band
    Cancel(ctx *Context)
}

type Prompt struct {
    Message  string        // "Specify next point or [Undo]:"
    Accepts  InputKind     // Point | Distance | Angle | Keyword | Selection | Text | Integer
    Keywords []string
    Default  *Input
}
```

- `Context` gives access to the document, the active transaction, the current view, snap settings and the selection set.
- Command aliases (`L` → LINE, `C` → CIRCLE, `TR` → TRIM) live in a registry loaded from a user-editable file.
- Typed input parsing: absolute `x,y`, relative `@dx,dy`, polar `@dist<angle`, direct distance entry along the tracked direction.

### 7.2 Input pipeline

```
raw mouse (screen px)
  → view inverse transform (float64)        → model point
  → object snap (if enabled)                → snapped point
  → ortho / polar / object-snap tracking    → constrained point
  → command.Input or command.Preview
```

### 7.3 Object snaps

Endpoint, Midpoint, Center, Geometric center, Node, Quadrant, Intersection, Apparent intersection, Extension, Insertion, Perpendicular, Tangent, Nearest, Parallel.

- Candidates gathered from `spatial.Nearest` within an aperture radius (screen px → model units).
- Ranked by snap priority then distance; the winning snap shows a marker and tooltip.

### 7.4 Selection

- Pick (single), Window (left→right drag), Crossing (right→left drag), Fence, All, Previous, Last.
- Respects locked/frozen/off layers.
- Selection set lives in the session, not the document (not persisted, not undoable).

### 7.5 Grips

- Each entity type exposes grip points and a grip-edit function (stretch, move, rotate, scale, mirror cycling via spacebar).
- Grip edits run inside a transaction like any command.

### 7.6 v1 command list

Draw: LINE, PLINE, CIRCLE, ARC, ELLIPSE, SPLINE, RECTANG, POLYGON, POINT, TEXT, MTEXT, HATCH, INSERT, BLOCK.
Modify: MOVE, COPY, ROTATE, SCALE, MIRROR, STRETCH, TRIM, EXTEND, OFFSET, FILLET, CHAMFER, BREAK, JOIN, EXPLODE, ERASE, ARRAY (rectangular/polar).
Annotate: DIMLINEAR, DIMALIGNED, DIMRADIUS, DIMDIAMETER, DIMANGULAR.
Utility: UNDO, REDO, ZOOM, PAN, REGEN, LAYER, PROPERTIES, MATCHPROP, DIST, AREA, ID, LIST.

---

## 8. Rendering

### 8.1 Principle

Rendering is split in two:

1. **`render` (backend-neutral):** turns entities into a display list of simple primitives.
2. **`render/ebitenrender` (adapter):** draws the display list with Ebitengine. This is the only code that knows about Ebitengine.

This keeps the core headless-testable and allows a different backend later.

### 8.2 View (camera)

```go
type View struct {
    Center   geom.Vec2 // model space, float64
    Scale    float64   // pixels per model unit
    Rotation float64
    Width, Height int  // viewport in pixels
}
```

**Camera-relative coordinates (mandatory):** Ebitengine vertices are `float32`. Before converting, subtract `View.Center` in `float64`, apply scale/rotation, then cast. This avoids jitter on drawings with large coordinates (survey/GIS data, coordinates in the millions).

### 8.3 Display list

```go
type DisplayItem struct {
    Owner     doc.Handle
    Kind      ItemKind     // Polyline | Fill | Text | Image
    Points    []geom.Vec2  // model space, pre-tessellated at a tolerance band
    Color     RGBA
    Weight    float32      // lineweight in screen px (0 = hairline)
    Dash      *DashPattern // resolved linetype
    Closed    bool
}
```

- Tessellation is cached per entity and per **tolerance band** (zoom level bucket), so zooming doesn't re-tessellate every frame.
- Caches are invalidated from `ChangeSet`s.
- Block inserts are expanded with a transform stack; repeated blocks share cached geometry.

### 8.4 Ebitengine adapter

Constraints of Ebitengine and how they are handled:

| Constraint | Mitigation |
|---|---|
| Game loop redraws continuously | Keep a `dirty` flag. Disable per-frame screen clearing (`ebiten.SetScreenClearedEveryFrame(false)`) and return early from `Draw` when nothing changed. |
| No persistent GPU vertex buffers; vertices re-submitted each draw | **Layered caching:** render the drawing into an offscreen `*ebiten.Image` only when the view or document changes. While panning, blit the cached image shifted by the pan offset; re-render once the pan settles. |
| `float32` vertices | Camera-relative transform (§8.2). |
| Constant-width hairlines and dashed linetypes | Kage shader: expand line segments to screen-space quads; dash pattern evaluated in the fragment shader from accumulated length. |
| Batch limits | Group display items by shader/color state into large `DrawTriangles` batches, respecting the index-count limit per call. |

**Frame composition:**

1. `base` layer — cached offscreen image of the drawing (rebuilt only when dirty).
2. `overlay` layer — redrawn every frame: crosshair cursor, snap marker, tracking lines, rubber-band preview, selection highlight, grips.
3. ImGui — tool panels on top.

### 8.5 Text

- Fonts loaded through Ebitengine `text/v2`; SHX fonts mapped to TrueType substitutes (configurable map) in v1, native SHX rendering in a later milestone.
- MTEXT formatting codes parsed into runs (font, height, color, stacking) in `render`, not in the adapter.

### 8.6 Hatch

- Solid fills triangulated (ear clipping or constrained Delaunay) in `geom`.
- Pattern hatches generated as clipped line families from the `.pat` definition, cached per hatch.

---

## 9. UI shell (`ui`)

- **Dear ImGui via cimgui-go**, using its Ebitengine renderer backend.
- Panels:
  - **Command line** — history, current prompt, keyword buttons, autocomplete of command names.
  - **Properties palette** — edits common and type-specific properties of the selection (each edit is a transaction).
  - **Layer manager** — on/off, freeze, lock, color, linetype, lineweight, current layer.
  - **Status bar** — coordinates, snap/ortho/polar toggles, model/layout tabs.
  - **Block browser** — insert from current drawing.
- Native file dialogs via a separate library (ImGui has none).
- Keyboard focus rule: when no ImGui text field has focus, all keystrokes go to the command line (AutoCAD behaviour).

---

## 10. File I/O (`io`)

### 10.1 Plugin interface

```go
type Reader interface {
    Extensions() []string
    Read(r io.Reader, opts ReadOptions) (*doc.Drawing, []Warning, error)
}

type Writer interface {
    Extensions() []string
    Versions() []string // e.g. "AC1032" (2018)
    Write(w io.Writer, d *doc.Drawing, opts WriteOptions) ([]Warning, error)
}
```

Formats register themselves in a registry; the app picks by extension or content sniffing.

### 10.2 DXF (first)

- Native Go implementation, ASCII and binary DXF.
- Reads all versions R12 → current; writes 2018 (AC1032) by default, plus R12 for legacy exchange.
- Unknown entities/objects captured as raw group-code lists (§5.5).
- Used to prove the document model before DWG work starts.

### 10.3 DWG (second)

DWG is proprietary and undocumented by Autodesk. Two routes, both behind the same `Reader`/`Writer` interface and compiled only with the `dwg` build tag (cgo):

| Option | Pros | Cons |
|---|---|---|
| **Open Design Alliance (ODA) Drawings SDK** | Most complete read/write coverage of all versions | Commercial membership; C++ API needs a C shim for cgo |
| **LibreDWG** | Free software, C API | GPLv3 (affects licensing of the whole app); write support partial |

**Decision required before M4** (see §14). Until then, DWG files can be converted to DXF externally.

### 10.4 Exports

- **SVG** — tessellated geometry, lineweights as stroke widths, text as `<text>` with font fallback; one `<g>` per layer.
- **PDF** — vector output of a layout or a window of model space at a chosen scale.

---

## 11. Testing strategy

| Level | What | How |
|---|---|---|
| Unit | Geometry kernel (intersections, offsets, tessellation) | Table-driven tests, property-based tests for symmetry/invariance |
| Unit | Document transactions, undo/redo | Apply random edit sequences, undo all, compare with original |
| Round-trip | DXF/DWG I/O | Corpus of real drawings: read → write → read, deep-compare object graphs and handles |
| Command | Each command end-to-end | Scripted input sequences fed to `cmdsys` headlessly; golden-file comparison of resulting document |
| Render | Display-list builder | Golden display lists (backend-neutral, no GPU) |
| Integration | Full app | Ebitengine headless/virtualization mode with injected input and captured frames, compared against reference images with tolerance |

---

## 12. Performance targets

| Scenario | Target |
|---|---|
| Open 10 MB DXF (~100k entities) | < 2 s to first frame |
| Pan/zoom on 100k entities | 60 fps during interaction (cached base layer) |
| Full re-render after zoom settles | < 100 ms |
| Snap candidate search | < 2 ms per mouse move |
| Idle CPU with nothing changing | ~0% (no redraw when not dirty) |

---

## 13. Milestones

| # | Milestone | Done when |
|---|---|---|
| M1 | Kernel + model + DXF read | DXF opens, entity counts and handles match source |
| M2 | Viewer | Pan, zoom, layers on/off, linetypes, text, hatch render correctly |
| M3 | Editing core | LINE/CIRCLE/ARC/PLINE, MOVE/COPY/ERASE, snaps, selection, undo/redo, DXF save with lossless round-trip |
| M4 | DWG | DWG read/write via the chosen library |
| M5 | Full v1 command set | All commands in §7.6, grips, properties palette |
| M6 | Annotation + output | Dimensions, MTEXT editing, layouts, SVG/PDF export |
| M7 | Browser build | WASM target with file open/save through the browser |

---

## 14. Open decisions and risks

| # | Item | Notes |
|---|---|---|
| D1 | DWG library: ODA vs LibreDWG | Licensing (commercial vs GPLv3) drives the whole project's license. Decide before M4. |
| D2 | App license | Depends on D1. |
| D3 | SHX font support | Substitute TrueType in v1 or implement an SHX parser. |
| D4 | WASM + DWG | cgo libraries won't run in WASM; browser build may be DXF-only. |
| R1 | Ebitengine on very large drawings | Vertex re-submission cost; mitigated by base-layer caching. Fallback: swap the adapter for a backend with retained GPU buffers — possible because the core is backend-neutral. |
| R2 | Numerical robustness | TRIM/EXTEND/OFFSET on near-tangent geometry; needs a tolerance policy and a large regression corpus. |
| R3 | ImGui look and feel | Functional but not native-looking; acceptable for v1. |