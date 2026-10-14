---
id: tech-file-io
title: File I/O
kind: tech
status: current
summary: The format plugin interface and registry, what the DXF codec reads and writes, and the routes to DWG.
read_when: you are adding a file format, changing what the DXF codec reads or writes, or deciding how DWG support lands
updated: 2026-10-14
verified: 2026-10-14
code_anchors: [fileio/format.go, fileio/dxf/read.go, fileio/dxf/write.go]
depends_on: [tech-document-model]
review_cycle: quarterly
---

# File I/O

## What it is

`fileio` is the plugin layer between files and the document. A format implements `fileio.Reader`
(extensions, and `Read` returning a new drawing plus warnings) and/or `fileio.Writer` (extensions,
the format versions it writes, and `Write` returning warnings). A `fileio.Registry` holds the
formats and picks one by file extension in `Open` and `Save`; `cmd/cad` registers the DXF codec and
passes the registry to the session as its `cmdsys.FileHandler`. The package is named `fileio` so it
never shadows the standard library's `io`.

## The DXF codec

`fileio/dxf.Codec` is a native Go reader and writer.

**Reading** accepts ASCII DXF from R12 through current versions:

| Section | What is read |
|---|---|
| HEADER | `$HANDSEED`, `$INSUNITS`, `$CLAYER`; every other variable is kept in `Header.Extra` |
| TABLES | `LAYER` records: name, colour (negative means off), the locked flag |
| ENTITIES | `LINE`, `CIRCLE`, `ARC` (degrees converted to radians), `LWPOLYLINE` with bulges, and 2D `POLYLINE`/`VERTEX`/`SEQEND` as an `LWPolyline` |

Entity handles (code 5), layers (8) and colours (62) are kept. Every other entity type, and 3D
polylines and meshes, is skipped and counted: the reader returns one warning per skipped type. A
file whose group codes are not integers (binary DXF) or that ends inside a pair is an error.

**Writing** emits R12 (`AC1009`), the most widely readable version: the header (`$ACADVER`,
`$CLAYER`, `$HANDLING`, `$HANDSEED`), a `CONTINUOUS` linetype, the layer table with colours, the
off and locked states, and the entities with their handles. An `LWPolyline` is written as
`POLYLINE`/`VERTEX`/`SEQEND`; the vertex and sequence-end records take handles above the drawing's
seed, which are not part of the document. Header variables the document does not model are not
written.

A written file reads back with the same handles, layers, colours and geometry; the round-trip test
in `fileio/dxf` holds the codec to that.

## DWG

DWG is proprietary and undocumented by Autodesk. Two routes, both behind the same reader and writer
interfaces and built only with a `dwg` build tag, because each needs cgo:

| Option | For | Against |
|---|---|---|
| Open Design Alliance Drawings SDK | The most complete read/write coverage of every version | Commercial membership; a C++ API needing a C shim for cgo |
| LibreDWG | Free software with a C API | GPLv3, which would license the whole application; partial write support |

The choice is open (it decides the application's licence) and is in the backlog. Until it is made,
DWG files are converted to DXF outside go-draw. A cgo plugin cannot run in a browser, so the browser
build reads and writes DXF only.

## Rationale

**DXF first.** DXF is documented, text-based and carries the same object model as DWG, so it proves
the document model — handles, ownership, unknown-object preservation — before the DWG work starts.

**R12 for writing in the spike.** An R12 file needs no object dictionary, classes or owner chains,
and every CAD application reads it. The cost is that the writer cannot carry newer data (lightweight
polylines stay heavy, no lineweights, no true colour); the decision register records it.

## Related docs

- [Document model](document-model.md) — what a reader fills and a writer serializes
- [DXF files](../features/dxf-files.md) — opening and saving from the application
- [Decision register](decisions.md) — R12 writing and the package name
