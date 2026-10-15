---
id: tech-file-io
title: File I/O
kind: tech
status: current
summary: The format plugin interface and registry, what the DXF codec reads and writes, and what the pure-Go DWG reader reads, skips and refuses.
read_when: you are adding a file format, changing what the DXF codec reads or writes, changing what the DWG reader imports, or asked to write DWG
updated: 2026-10-15
verified: 2026-10-15
code_anchors: [fileio/format.go, fileio/dxf/read.go, fileio/dxf/write.go, fileio/dxf/blocks.go, fileio/dxf/groups.go, fileio/dxf/symbolwrite.go, fileio/dxf/text_test.go, doc/unknown.go, fileio/dxf/dxf_test.go, fileio/format_test.go, fileio/dwg/read.go, fileio/dwg/container.go, fileio/dwg/objects.go, fileio/dwg/entities.go, fileio/dwg/import_entities.go, fileio/dwg/header.go, fileio/dwg/read_test.go, doc/aci.go]
depends_on: [tech-document-model]
review_cycle: quarterly
---

# File I/O

## What it is

`fileio` is the plugin layer between files and the document. A format implements `fileio.Reader`
(extensions, and `Read` returning a new drawing plus warnings) and/or `fileio.Writer` (extensions,
the format versions it writes, and `Write` returning warnings). A `fileio.Registry` holds the
formats and picks one by file extension in `Open` and `Save`; `cmd/cad` registers the DXF codec
(reader and writer) and the DWG reader, and passes the registry to the session as its
`cmdsys.FileHandler`.

`Registry.Save` is atomic: the codec writes a temporary file in the target's directory, which is
synced, given the existing file's permission bits and renamed over the path, so a failed write leaves
any existing file untouched. A symbolic link is resolved first and the link is kept; a target that is
not a regular file is an error. The package is named `fileio` so it
never shadows the standard library's `io`.

A format with a reader and no writer is read-only: `Registry.Save` to such a path fails with
`<FORMAT> writing is not supported; save as <ext of the first writer>` (for DWG, `DWG writing is not
supported; save as .dxf`) and touches nothing. `Registry.CanRead`, `CanWrite` and `WritablePath`
(the path with the first writer's extension) let the file commands offer a DXF name after a DWG was
opened.

Warnings and errors are localized sentences (`dxf.warn.*`, `dxf.err.*`, `dwg.warn.*`, `dwg.err.*`
and `fileio.err.*` in the `i18n` catalogs), in the language active when the file is read or
written; DXF tokens inside them (class names, section names, handles) stay as written in the file.

## Groups and reusable block files

`fileio/dxf/blocks.go`, `groups.go` and `symbolwrite.go` own native BLOCK, INSERT, ATTDEF and ATTRIB records round-trip named symbols and their instance values.
Ordinary drawings retain AC1009 output. Drawings with groups use AC1015 with block records, owners,
reactors and the named-object/ACAD_GROUP dictionaries, so GROUP membership survives in other CAD tools.
This branch models GROUP rather than substituting private application data. Unsupported block content
and object records remain preserved; warnings identify unsupported reference forms. File-space owner
handles and object-dictionary pointers are remapped on output; logical `doc.ModelSpace` does not
reserve a foreign persistent handle. The output hand seed follows every generated handle. Preserved external blocks reuse the original
BLOCK_RECORD handle carried by their owner links, keeping BLOCK, ENDBLK and dependent object pointers
consistent; blocks missing an owner receive one generated consistently across their records. Imported
attribute records retain placement, formatting, invisibility and additional fields. WBLOCK exports a
symbol with its dependency definitions; INSERT imports a DXF library drawing through the file registry.
DWG block import remains outside this contract.

## The DXF codec

`fileio/dxf.Codec` is a native Go reader and writer.

**Reading** accepts ASCII DXF from R12 through current versions:

| Section | What is read |
|---|---|
| HEADER | `$HANDSEED`, `$INSUNITS`, `$CLAYER`; every other variable is kept in `Header.Extra` |
| TABLES | `LAYER` records: name, colour (negative means off), the frozen (1) and locked (4) flags, the linetype name (6), and the plot flag (290) and lineweight (370) of newer versions; the `CADLITE_LAYER` extended data (below; `GODRAW_LAYER`, the name before the rename, is still read) |
| ENTITIES | `LINE`, `CIRCLE`, `ARC` (degrees converted to radians), `LWPOLYLINE` with bulges, 2D `POLYLINE`/`VERTEX`/`SEQEND` as an `LWPolyline`, `TEXT`, `MTEXT`, `INSERT` and `ATTDEF` (below) |

**TEXT** (`readText`) keeps the first and second alignment points (10, 11; without 11 the second is the first),
height (40), value (1, spaces kept, `\U+XXXX` escapes decoded), rotation (50) and obliquing angle (51) in
radians, width factor (41, 1 when absent), style name (7, `Standard` when absent), generation flags (71,
backward and upside down) and justification (72, 73). For a justified text AutoCAD writes group 10 as a
computed baseline start; the document recomputes it from 11. A text whose extrusion is -Z (230 < 0) is
converted to world coordinates as upside-down text at the mirrored point, which draws the same; any other
extrusion is ignored, and thickness and linetype are dropped. **MTEXT** (`readMText`) keeps the insertion
point (10), height (40), box width (41), attachment (71), rotation (50, or the direction vector 11), line
spacing factor (44), style (7) and the value, the group 3 chunks followed by group 1, format codes included;
its extrusion (210), drawing direction (72), line spacing style (73), columns and background mask are dropped,
and one warning counts the MTEXT whose extrusion is not +Z or whose direction is top to bottom, which draw
differently here.

Entity handles (code 5), layers (8) and colours (62) are kept. Every other entity class, and 3D
polylines and meshes, is kept as a `doc.Unknown` holding its raw group list and the sub-records it
owns (ATTRIB, VERTEX, SEQEND); the reader returns one warning per class with the count. Tables the
document does not model (VPORT, LTYPE other than CONTINUOUS, STYLE, VIEW, UCS, APPID, DIMSTYLE) and
unsupported external-reference blocks go to `Drawing.Preserved`; ordinary named blocks become `doc.Block`. An entity whose
handle duplicates one read earlier is renumbered, with a warning. OBJECTS supplies GROUP definitions and preserves other records; CLASSES and the thumbnail are not
read. A file whose group codes are not integers (binary DXF), that ends inside a pair, or that has a
section without `ENDSEC` (a truncated file, including a `SECTION` nested in another) is an error. A
file with no `EOF` marker reads with the warning `file has no EOF marker (it may be truncated)`. A
non-finite number (`nan`, `inf`, `1e999`) in a numeric group (codes 10 to 59, 110 to 149 and 210 to 239)
is read as 0, with one warning that counts them, so the drawing never holds a non-finite coordinate.

**Writing** emits R12 for plain drawings and native symbols; GROUP or preserved OBJECTS content uses
the targeted AC1015 path described above. The R12 path emits R12 (`AC1009`), the most widely readable version, in this order: the header
(`$ACADVER`, `$CLAYER`, `$HANDLING`, `$HANDSEED`), the `CONTINUOUS` linetype followed by the
preserved linetypes, the layer table with colours and the off, frozen and locked states and the linetype name (only when the LTYPE table defines it, else `CONTINUOUS`; R12 has no plot flag, lineweight or description, so a layer with any of them carries them as extended data of the application `CADLITE_LAYER`, registered in the APPID table, which the reader restores and does not preserve), the other preserved
tables, the preserved blocks, and the entities with their handles. An `LWPolyline` is written as
`POLYLINE`/`VERTEX`/`SEQEND`; the vertex and sequence-end records take handles above the drawing's
seed, which are not part of the document. Header variables the document does not model are not
written.

A `Text` is written as `TEXT` the way AutoCAD writes it: group 10 is the computed baseline start, 11
the alignment point when the justification is not left-baseline, and 40, 50 and 41 the height,
rotation and width factor in effect (Aligned derives the height and Fit the width factor from the two
points); characters outside ASCII are written as `\U+XXXX` (the writer writes no `$DWGCODEPAGE` and every other
string as UTF-8, so an escape is the one portable form): AutoCAD from R13 on, ezdxf and this reader decode it,
while a strict R12 reader shows the escape literally. TEXT's `%%` codes are written as typed. The STYLE table is the preserved records plus a `txt` record for `Standard` and for every style a
text names that the file lacked. R12 has no MTEXT, so an `MText` is written as one left-justified `TEXT`
per line of its layout (wrapped, format codes gone) at the line's baseline start, the first with the
MTEXT's handle and the others with handles above the seed, and one warning counts the MTEXT objects
written this way (their box, wrapping width, attachment and line spacing are lost; positions are kept): reading the file back gives those TEXT lines, not an MTEXT (backlog G2).

An `Unknown` whose class R12 defines (POINT, TRACE, SOLID, SHAPE, INSERT, ATTDEF, POLYLINE,
3DLINE, 3DFACE, VIEWPORT, DIMENSION) is written group for group, minus the groups that exist only
from R13 on: 100, 102, 330 and 360. A strict R12 reader such as ezdxf rejects them; a file written
this way passes an ezdxf 1.4.4 audit clean. An `Unknown` of any other class (SPLINE, HATCH,
ELLIPSE, custom classes) is skipped with a warning, as is a non-R12 entity inside a preserved block.

A written file reads back with the same handles, layers, colours and geometry; the round-trip test
in `fileio/dxf` holds the codec to that.

## DWG

`fileio/dwg.Codec` is a native Go **reader**; there is no DWG writer. It is pure Go, with no cgo
and no external library, so it builds for `js/wasm`, Windows, macOS and Linux with `CGO_ENABLED=0`.
It is a port of the DWG reading logic of [ACadSharp](https://github.com/DomCR/ACadSharp) (MIT,
Copyright (c) 2021 Albert Domenech; the notice is `fileio/dwg/LICENSE-ACadSharp`), written against
the Open Design Alliance's public specification of the format. No LibreDWG or other GPL or LGPL
code was read or used ([decision register](decisions.md), D24).

**Versions.**

| Magic | Release | Container | Status |
|---|---|---|---|
| `AC1014` | R14 | Section locators at the start of the file, objects at absolute offsets | read |
| `AC1015` | R2000 | Same container, bit-coded object records | read |
| `AC1018` | R2004 | Encrypted file header, page map and section map, LZ77 pages | read |
| `AC1021` | R2007 | Reed-Solomon encoded header and pages, a second LZ77 variant, UTF-16 strings in a string stream | read |
| `AC1024`, `AC1027`, `AC1032` | R2010, R2013, R2018 | The R2004 container; object records add a handle-stream size and a bit-coded type | read |
| `AC1012` | R13 | Read as R14 | accepted, untested: no sample |
| `AC1009` and older | R12 and before | | refused |

An older version, or a file that is not DWG, fails with `DWG version AC1009 (R11/R12) is not
supported; open the drawing in a CAD application and save it as DXF` or `not a DWG file`. The
Reed-Solomon data of R2007 is de-interleaved; its parity bytes are ignored, so errors are not corrected.

**What is imported.** The header's units (`$INSUNITS`, from R2000), handle seed and current layer;
the layer table (name, colour, off, frozen, locked, plot flag, lineweight and linetype name; R14 has
no plot flag or lineweight); and the model space entities `LINE`, `CIRCLE`, `ARC`, `LWPOLYLINE`
(bulges and widths), 2D `POLYLINE` with its vertices, as an `LWPolyline` (3D polylines and
meshes are skipped; a vertex that is a spline frame control point is left out), and `TEXT`
(`readText`, R14 and the R2000 data-flag layout: the fields the DXF reader keeps, the style name
resolved through the STYLE records, which `run` reads for their names). Each entity keeps its
handle, layer and colour: an ACI index, with by-block and by-layer as `ByLayer`; a true colour or
colour-book colour keeps the nearest index AutoCAD stored beside it, and a true-colour layer takes
`doc.NearestACI` of its RGB. Entity linetype, lineweight, thickness and elevation are not kept. An
entity whose normal is `(0,0,-1)` is mirrored into world coordinates; any other normal is ignored, as
the DXF reader ignores code 210.

Model space is the entities whose entity mode says so, or whose owner is the `*Model_Space` block
record. Paper space, block definitions and `INSERT` contents are not read.

**What is skipped.** Every other entity (MTEXT, POINT, INSERT, HATCH, SPLINE, ELLIPSE,
DIMENSION, MULTILEADER and the rest) is skipped, not kept as `doc.Unknown`, and counted in one
warning, `skipped 15 MULTILEADER, 11 DIMENSION, 8 HATCH, ... (not read from DWG files; the DXF
reader keeps them)`, by DXF class name, most frequent first. A second warning counts the paper space
entities. Tables other than layers, blocks, dictionaries, layouts and extended data are not read.

**Robustness.** A bit reader bounds-checks every read and panics with a sentinel that each section,
object and the top of `Codec.Read` recover, so a damaged object is counted (`N DWG objects could not
be read`) and the rest imports. An object whose CRC fails is skipped and counted. Sizes a file claims
are checked before anything is allocated: the file is capped at 1 GiB, a section at 256 MiB and at 64
times the file, a page at 64 MiB, and decompression stops at the size the page declares. Polyline
vertex reads share a budget proportional to the object count. `FuzzRead` seeds with the fixtures; the
mutation tests damage the container and the decoded objects.

**Fixtures** are ACadSharp's `sample_AC10xx.dwg` files (MIT; origin and checksums in
`fileio/dwg/testdata/README.md`): one drawing in every version, whose expected values are those of
ACadSharp's ASCII DXF export read by the DXF reader. `TestParityWithDXF` (with `DWG_PARITY_DIR`)
compares every entity of every version with that export.

## Rationale

**DXF first.** DXF is documented, text-based and carries the same object model as DWG, so it proved
the document model — handles, ownership, unknown-object preservation — before the DWG reader was
written. The reader fills the same `doc.Drawing`, so everything downstream of it (editing, the DXF
writer, the agent tools) treats a DWG drawing like a DXF one.

**Reader only.** DWG writing needs the object map, classes, the section container and the
bit-coded records of every object the document holds, and a wrong byte makes AutoCAD refuse the
file. A drawing opened from DWG is saved as DXF (backlog).

**R12 for writing in the spike.** An R12 file needs no object dictionary, classes or owner chains,
and every CAD application reads it. The cost is that the writer cannot carry newer data (lightweight
polylines stay heavy, no lineweights, no true colour); the decision register records it.

## Related docs

- [Document model](document-model.md) — what a reader fills and a writer serializes
- [DXF files](../features/dxf-files.md) — opening and saving from the application
- [Decision register](decisions.md) — R12 writing, the package name and the DWG reader
