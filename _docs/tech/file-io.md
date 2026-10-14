---
id: tech-file-io
title: File I/O
kind: tech
status: current
summary: The format plugin interface and registry, what the DXF codec reads and writes, and the routes to DWG.
read_when: you are adding a file format, changing what the DXF codec reads or writes, or deciding how DWG support lands
updated: 2026-10-14
verified: 2026-10-14
code_anchors: [fileio/format.go, fileio/dxf/read.go, fileio/dxf/write.go, doc/unknown.go, fileio/dxf/dxf_test.go, fileio/format_test.go]
depends_on: [tech-document-model]
review_cycle: quarterly
---

# File I/O

## What it is

`fileio` is the plugin layer between files and the document. A format implements `fileio.Reader`
(extensions, and `Read` returning a new drawing plus warnings) and/or `fileio.Writer` (extensions,
the format versions it writes, and `Write` returning warnings). A `fileio.Registry` holds the
formats and picks one by file extension in `Open` and `Save`; `cmd/cad` registers the DXF codec and
passes the registry to the session as its `cmdsys.FileHandler`.

`Registry.Save` is atomic: the codec writes a temporary file in the target's directory, which is
synced, given the existing file's permission bits and renamed over the path, so a failed write leaves
any existing file untouched. A symbolic link is resolved first and the link is kept; a target that is
not a regular file is an error. The package is named `fileio` so it
never shadows the standard library's `io`.

Warnings and errors are localized sentences (`dxf.warn.*`, `dxf.err.*` and `fileio.err.*` in the
`i18n` catalogs), in the language active when the file is read or written; DXF tokens inside them
(class names, section names, handles) stay as written in the file.

## The DXF codec

`fileio/dxf.Codec` is a native Go reader and writer.

**Reading** accepts ASCII DXF from R12 through current versions:

| Section | What is read |
|---|---|
| HEADER | `$HANDSEED`, `$INSUNITS`, `$CLAYER`; every other variable is kept in `Header.Extra` |
| TABLES | `LAYER` records: name, colour (negative means off), the locked flag |
| ENTITIES | `LINE`, `CIRCLE`, `ARC` (degrees converted to radians), `LWPOLYLINE` with bulges, and 2D `POLYLINE`/`VERTEX`/`SEQEND` as an `LWPolyline` |

Entity handles (code 5), layers (8) and colours (62) are kept. Every other entity class, and 3D
polylines and meshes, is kept as a `doc.Unknown` holding its raw group list and the sub-records it
owns (ATTRIB, VERTEX, SEQEND); the reader returns one warning per class with the count. Tables the
document does not model (VPORT, LTYPE other than CONTINUOUS, STYLE, VIEW, UCS, APPID, DIMSTYLE) and
every block except `*Model_Space` and `*Paper_Space` go to `Drawing.Preserved`. An entity whose
handle duplicates one read earlier is renumbered, with a warning. OBJECTS, CLASSES and the thumbnail are not
read. A file whose group codes are not integers (binary DXF), that ends inside a pair, or that has a
section without `ENDSEC` (a truncated file, including a `SECTION` nested in another) is an error. A
file with no `EOF` marker reads with the warning `file has no EOF marker (it may be truncated)`. A
non-finite number (`nan`, `inf`, `1e999`) in a numeric group (codes 10 to 59, 110 to 149 and 210 to 239)
is read as 0, with one warning that counts them, so the drawing never holds a non-finite coordinate.

**Writing** emits R12 (`AC1009`), the most widely readable version, in this order: the header
(`$ACADVER`, `$CLAYER`, `$HANDLING`, `$HANDSEED`), the `CONTINUOUS` linetype followed by the
preserved linetypes, the layer table with colours and the off and locked states, the other preserved
tables, the preserved blocks, and the entities with their handles. An `LWPolyline` is written as
`POLYLINE`/`VERTEX`/`SEQEND`; the vertex and sequence-end records take handles above the drawing's
seed, which are not part of the document. Header variables the document does not model are not
written.

An `Unknown` whose class R12 defines (POINT, TRACE, SOLID, TEXT, SHAPE, INSERT, ATTDEF, POLYLINE,
3DLINE, 3DFACE, VIEWPORT, DIMENSION) is written group for group, minus the groups that exist only
from R13 on: 100, 102, 330 and 360. A strict R12 reader such as ezdxf rejects them; a file written
this way passes an ezdxf 1.4.4 audit clean. An `Unknown` of any other class (SPLINE, MTEXT, HATCH,
ELLIPSE, custom classes) is skipped with a warning, as is a non-R12 entity inside a preserved block.

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
