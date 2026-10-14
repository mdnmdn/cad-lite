---
id: prod-overview
title: go-draw product overview
kind: product
status: current
summary: What go-draw is for, who it serves, what is in and out of scope, and the milestones that deliver it.
read_when: you are deciding whether a capability belongs in go-draw, or what to build next
updated: 2026-10-14
verified: 2026-10-14
review_cycle: quarterly
---

# go-draw

## Purpose

go-draw is a 2D drafting application in the spirit of AutoCAD. A drafter works from a command line
with typed coordinates and object snaps, on drawings measured in real units with double precision,
organised in layers and blocks. Drawings travel between offices as DWG and DXF files, so go-draw
opens, edits and saves them **without losing data**, including objects it does not understand.

It runs as a desktop application on Windows, macOS and Linux. A browser build is a secondary target
for viewing and light editing, and the application is designed so that build stays possible.

## Users and workflow

The user is a professional drafter or engineer who already knows AutoCAD's interaction model and
expects it to carry over:

- the command line is the primary input: a command name or alias, then answers to its prompts;
- typed points, relative and polar input, and direct distance entry are as fast as clicking;
- object snaps, ortho and tracking make clicked points exact;
- selection works by pick, window and crossing, before or after the command;
- every command undoes as one step;
- pan and zoom stay fluid on drawings of a hundred thousand entities.

## Scope

In scope for the first full version:

- **Drawing:** lines, polylines with arcs, circles, arcs, ellipses, splines, rectangles, polygons,
  points, single- and multi-line text, hatches, blocks and their insertion.
- **Editing:** move, copy, rotate, scale, mirror, stretch, trim, extend, offset, fillet, chamfer,
  break, join, explode, erase, arrays, and grip editing.
- **Annotation:** linear, aligned, radial, diametric and angular dimensions.
- **Organisation:** layers with colour, linetype, lineweight, visibility, freezing and locking; a
  properties palette.
- **Files:** DWG and DXF read and write with lossless round trips; SVG and PDF export.
- **Measurement:** distance, area, point identification, entity listing.

## Non-goals

For the first full version:

- 3D modelling, solids and rendered 3D views;
- parametric constraints;
- full paper-space plotting fidelity (plot styles) — a later milestone;
- collaboration and cloud synchronisation.

## Milestones

| # | Milestone | Done when |
|---|---|---|
| M1 | Kernel, model and DXF read | A DXF file opens and its entity counts and handles match the source |
| M2 | Viewer | Pan, zoom, layers on and off, linetypes, text and hatches render correctly |
| M3 | Editing core | Line, circle, arc and polyline drawing; move, copy and erase; snaps, selection, undo and redo; DXF save with a lossless round trip |
| M4 | DWG | DWG read and write through the chosen library |
| M5 | Full command set | Every command in scope, grips and the properties palette |
| M6 | Annotation and output | Dimensions, multi-line text editing, layouts, SVG and PDF export |
| M7 | Browser build | The application runs in a browser, opening and saving files through it |

Where each milestone stands, and what is missing from it, is tracked in the backlog.

## Related docs

- [Backlog](../backlog.md) — milestone status, open decisions and known gaps
- [Architecture](../tech/architecture.md) — how the application is built
