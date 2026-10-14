---
id: tech-geometry
title: Geometry kernel and spatial index
kind: tech
status: current
summary: The numeric policy, the curve types and operations of geom, and the spatial index that serves culling, picking and snapping.
read_when: you are adding a curve type or a geometric operation, comparing floats, or querying entities by location
updated: 2026-10-14
verified: 2026-10-14
code_anchors: [geom/vec.go, geom/curves.go, geom/intersect.go, geom/area.go, geom/offset.go, geom/fillet.go, geom/filletpoly.go, geom/split.go, geom/tol.go, spatial/index.go]
depends_on: [tech-architecture]
review_cycle: quarterly
---

# Geometry kernel and spatial index

## What it is

`geom` is the geometry kernel: 2D vectors, boxes and affine transforms in `float64`, the curve types
every entity is made of, and the operations commands and snaps need. It has no dependencies outside
the standard library and knows nothing about documents or rendering. `spatial` indexes entity
bounding boxes so the layers above can ask "what is near this point" without scanning the drawing.

## Numeric policy

- Every model-space coordinate is `float64`; there is no `float32` in `geom`.
- Tolerances are explicit and drawing-relative: `geom.DefaultTol(extent)` gives a linear tolerance
  of `1e-9 × extent` (at least `1e-10`) and an angular tolerance of `1e-10` radians.
- Nothing compares floats for equality except to detect exact degeneracy (a zero-length segment);
  every other comparison takes a tolerance argument, as `geom.Intersect` and `Vec2.Near` do.
- Angles are radians, measured counter-clockwise from +x. `geom.NormAngle` reduces to `[0, 2π)`.
  Degrees exist only at the boundaries: typed input and DXF files.

## Model

| Type | Meaning |
|---|---|
| `Vec2` | A point or vector |
| `Box2` | An axis-aligned box; `EmptyBox()` is the accumulator that grows with `Extend` |
| `Mat3` | A 2D affine transform `[a b c d e f]` (SVG/PDF order); `Mul` applies the right operand first |
| `Curve` | The interface every curve implements: bounds, point and tangent at a parameter, parameter range, closest parameter, length, transform and tessellation |
| `Line` | A segment; `t` runs from 0 to 1 |
| `Circle` | A full circle; `t` is the angle |
| `Arc` | A counter-clockwise arc from `Start` to `End`; equal angles are a full turn |
| `Polyline` | Vertices with per-segment **bulges** (`tan(θ/4)`, positive counter-clockwise), the DWG LWPOLYLINE convention; `t` runs one unit per segment |

Operations:

| Operation | Function | Used by |
|---|---|---|
| Closest point | `Curve.ClosestParam`, `geom.Closest` | picking, the Nearest snap |
| Curve/curve intersection | `geom.Intersect` over `geom.Primitives` | the Intersection snap |
| Tessellation by chord tolerance | `Curve.Tessellate` | the renderer, previews, crossing selection |
| Bulge to arc | `geom.BulgeArc` | polyline segments, tessellation |
| Box/segment overlap | `geom.SegmentIntersectsBox` | crossing selection |
| Arc through three points | `doc.ArcFrom3Points` | the ARC command |
| Signed area of a polygon or a bulged polyline (closed by its chord) | `geom.PolygonArea`, `geom.PolylineArea`, `geom.ArcSegmentArea` | AREA, MEASUREGEOM |
| Parallel offset of a line, circle, arc or polyline; positive is left of the direction of travel (outward for a circle or arc); `ok` is false for a radius that reaches zero or a degenerate polyline | `geom.OffsetLine`, `OffsetCircle`, `OffsetArc`, `OffsetPolyline` | OFFSET |
| Round or bevel the corner of two lines, keeping the picked sides: a `Corner` with each line's retained far end and new join point and a tangent `Arc` or a `Bevel`, neither for a sharp corner; `ErrParallel` and `ErrTooLarge` refuse | `geom.FilletLines`, `ChamferLines` | FILLET, CHAMFER |
| Cut the corners of a polyline at every vertex between two straight segments with an arc (`FilletCut`) or a bevel (`ChamferCut`, `ChamferAngleDistance` for length and angle), skipping a cut that does not fit and interpolating the segment widths; `CornerWedge` is the angle between two picked lines | `geom.CutPolyline`, `CornerWedge` | FILLET, CHAMFER `Polyline`, CHAMFER `Angle` |
| Which spans of a curve survive a trim at the cut parameters, or a break between two; sub-curves and splits of a curve or polyline | `geom.TrimSpan`, `KeepOutside`, `SubCurve`, `SubPolyline`, `SplitCurve` | TRIM, BREAK |
| Convert a bulged polyline segment to its arc and back | `geom.ArcFromBulge`, `BulgeFromArc` | polyline splitting, JOIN |
| Flatten a polyline into `Piece`s (a line or arc, forwards or reversed), chain pieces end to end within a tolerance, rebuild a polyline | `geom.PolylinePieces`, `ChainPieces`, `PolylineFromPieces` | JOIN |
| Merge collinear lines or co-circular arcs that touch or overlap | `geom.JoinLines`, `JoinArcs` | JOIN |
| Extend one end of a line or arc to the nearest boundary ahead | `geom.ExtendEnd` | EXTEND |

`geom.Intersect` reports no points for overlapping collinear lines or concentric overlapping arcs, so
TRIM and EXTEND see no cut where two pieces coincide.

`Curve.Transform` assumes a similarity transform (translate, rotate, uniform scale, mirror): a circle
stays a circle with its radius scaled by `Mat3.ScaleFactor`. A mirroring transform keeps arcs
counter-clockwise by swapping their ends, and negates polyline bulges.

## Spatial index

`spatial.Index` maps entity handles to bounding boxes. It is built once per drawing (`Build`) and
updated from every change set (`Apply`). Two queries serve the layers above:

- `Query(box)` — the handles whose boxes intersect a box: viewport culling and box selection;
- `Near(p, r)` — the handles whose boxes come within `r` of `p`: picking and snap candidates.

Results are in ascending handle order, which is draw order, so a tie between overlapping entities
goes to the newest one.

## Rationale

**Bulges rather than arc segments in polylines.** Bulges are how DWG and DXF store polyline arcs;
keeping the file representation avoids a lossy conversion on every round trip.

**The index API is the R-tree's.** The index is a linear scan over cached boxes, which is fast enough
for the drawings the spike handles. The query functions are the ones an R-tree serves, so replacing
the implementation does not touch any caller.

## Related docs

- [Document model](document-model.md) — the entities built from these curves
- [Command system](command-system.md) — snaps and selection, the main users of the queries
- [Backlog](../backlog.md) — the R-tree and the other operations still to build
