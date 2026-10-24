---
id: tech-file-io
title: File I/O
kind: tech
status: current
summary: The format plugin interface and registry, what the DXF codec reads and writes, and what the pure-Go DWG reader reads, skips and refuses.
read_when: you are adding a file format, changing what the DXF codec reads or writes, changing what the DWG reader imports, or asked to write DWG
updated: 2026-10-24
verified: 2026-10-24
code_anchors: [fileio/format.go, fileio/dxf/read.go, fileio/dxf/write.go, fileio/dxf/dimension.go, fileio/dxf/dimension_test.go, doc/dimension.go, fileio/dxf/blocks.go, fileio/dxf/groups.go, fileio/dxf/symbolwrite.go, fileio/dxf/version.go, fileio/dxf/version_test.go, fileio/version_test.go, fileio/dxf/text_test.go, doc/unknown.go, fileio/dxf/dxf_test.go, fileio/format_test.go, fileio/dwg/read.go, fileio/dwg/container.go, fileio/dwg/objects.go, fileio/dwg/entities.go, fileio/dwg/import_entities.go, fileio/dwg/header.go, fileio/dwg/read_test.go, doc/aci.go]
depends_on: [tech-document-model]
review_cycle: quarterly
---

# File I/O

## What it is

`fileio` is the plugin layer between files and the document. A format implements `fileio.Reader`
(extensions, and `Read` returning a new drawing plus warnings) and/or `fileio.Writer` (extensions,
the format versions it writes, and `Write` returning warnings, in the version the writer chooses). A
writer of several versions also implements `fileio.VersionWriter`: `WriteVersion` writes the version
the caller names ("" is `Write`'s choice) and `AutoVersion` says which one `Write` would pick. A `fileio.Registry` holds the
formats and picks one by file extension in `Open` and `Save`; `cmd/cad` registers the DXF codec
(reader and writer) and the DWG reader, and passes the registry to the session as its
`cmdsys.FileHandler`.

**Versions.** `Registry.Save(path, d)` writes the drawing's own version, `d.Header.Version`;
`Registry.SaveWith(path, d, fileio.SaveOptions{Version})` names one instead and leaves the header
alone. Either maps the version through `fileio.NearestVersion`: a writable version stays, any other
becomes the oldest writable one newer than it (AC1012 and AC1014 give AC1015), else the newest
writable one (AC1018 to AC1032 give AC1015); "" lets the writer choose. A writer that is not a
`VersionWriter` writes its one version whatever is asked. `Registry.SaveVersions(path)` lists the
versions the writer for path writes, oldest first, and `SaveVersion(path, d, version)` the one a save
would write; both fall back to the first writer for a path no writer handles, which is how SAVEAS
offers formats before it knows the path. Readers set `Header.Version`: the DXF reader from
`$ACADVER` when it is newer than AC1009 (R12 and older files keep "", so the writer chooses and a
group added to them is not lost; decision D44), the DWG reader to the file's release (`AC1014` and
up).

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

Layer order is the LAYER table record order in both R12 and AC1015 output. The reader explicitly
applies that order after loading the records: preinstalled layer 0 stays at its file position rather
than being forced first. Duplicate records retain one position (the first occurrence) with the existing
last-record metadata policy. Required default layers absent from the table remain present after the
file's ordered records. Loading creates no undo step or dirty notification.

## The DXF codec

`fileio/dxf.Codec` is a native Go reader and writer.

**Reading** accepts ASCII DXF from R12 through current versions:

| Section | What is read |
|---|---|
| HEADER | `$HANDSEED`, `$INSUNITS`, `$CLAYER`; every other variable is kept in `Header.Extra` |
| TABLES | `LAYER` records: name, colour (negative means off), the frozen (1) and locked (4) flags, the linetype name (6), and the plot flag (290) and lineweight (370) of newer versions; the `CADLITE_LAYER` extended data (below; `GODRAW_LAYER`, the name before the rename, is still read) |
| ENTITIES | `LINE`, `CIRCLE`, `ARC` (degrees converted to radians), `LWPOLYLINE` with bulges, 2D `POLYLINE`/`VERTEX`/`SEQEND` as an `LWPolyline`, `TEXT`, `MTEXT`, `INSERT` and `ATTDEF`, linear `DIMENSION` and `ARC_DIMENSION` (below) |

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

**Dimensions** (`fileio/dxf/dimension.go`). A `DIMENSION` of type 0 (rotated, horizontal, vertical)
or 1 (aligned) becomes a `doc.Dimension` (`readDimension`): 10 the dimension line point, 11 the text
middle, 13 and 14 the extension line origins, 50 the rotation of type 0 (degrees to radians; R2000
writes it inside `AcDbAlignedDimension`), 1 the text override, 70 bit 128 the user-positioned text
(bit 32, "block referenced by this dimension only", is ignored), and 53 the text angle. Group 53 is
read as the absolute text angle, from the X axis, which is how AutoCAD and ezdxf draw it; 0 is the
default orientation (AutoCAD: an angle of 0 restores it), which matches `TextAngle` 0 in the
document. An explicit horizontal angle written as 360 is kept as 180°, which draws the same since the
text is made readable; ezdxf draws a present 53 of 0 as horizontal text, and that file reads here as
the default orientation. Group 51 (horizontal direction) is ignored. The style is the `DIMSTYLE`
record that group 3 names: groups 40 DIMSCALE, 41 DIMASZ, 42 DIMEXO, 44 DIMEXE, 140 DIMTXT, 147
DIMGAP, 271 DIMDEC, 278 DIMDSEP (a character code), 78 DIMZIN, 77 DIMTAD and 90 DIMARCSYM, the same
codes in R12 and R2000 records (R12 has no DIMDEC or DIMDSEP). A variable the record leaves out takes
AutoCAD's built-in default (DIMSCALE 1, DIMASZ 0.18, DIMTXT 0.18, DIMEXO 0.0625, DIMEXE 0.18, DIMGAP
0.09, DIMDEC 4, DIMDSEP `.`, DIMZIN 0, DIMTAD 0), because AutoCAD writes only what differs from them;
a style the table lacks is ISO-25 (`doc.DefaultDimStyle`) under that name. The entity's ACAD `DSTYLE`
extended data (`{1070 code, value}` pairs) then overrides the record. An `ARC_DIMENSION` (AutoCAD 2006,
AC1018 and later) becomes a `DimArcLength`: 15 the arc centre, 13 and 14 the ends of the measured
arc, the arc running counter-clockwise between them on the side of the text (11), 10 a point on the
dimension arc, and with the partial flag (70 of `AcDbArcDimension`) 40 and 41 the whole arc's start and
end angles in radians. An `INSERT` carrying `CADLITE_DIM` extended data (written below) becomes the
arc length dimension it defines. Each converted dimension calls `Recompute`. Dimensions of other
types (angular, radial, diameter, ordinate), with obliqued extension lines (52), an extrusion other
than +Z, or an arc length leader stay `doc.Unknown` with their blocks. The anonymous block (`*D1`...)
of a converted dimension is regenerated on writing, so `dimensionBlocks` finds those names before the
BLOCKS section is read and the block is not loaded, unless a dimension that stays unknown also
references it.

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

**Writing** without a version (`Write`, `AutoVersion`) emits R12 for plain drawings, native
symbols and dimensions; GROUP or preserved OBJECTS content uses the targeted AC1015 path described above.
`WriteVersion` (`fileio/dxf/version.go`) with `AC1015` always takes that AC1015 path, with
`LWPOLYLINE`, native `MTEXT` and the layer plot flag and lineweight. With `AC1009` it takes the R12
path, or the symbol writer's R12 branch for a drawing with blocks, groups or object records; groups
and object records are then left out and each MTEXT becomes TEXT, with one warning per kind
(`dxf.warn.groups_r12`, `dxf.warn.objects_r12`, `dxf.warn.mtext_as_text`). Any other version is
the error `DXF version AC1018 cannot be written (Cad-lite writes AC1009, AC1015)`. `dxf.Version*`
names the two versions, and `dxf.version.ac1009` and `ac1015` in the catalogs their AutoCAD names. The R12 path emits R12 (`AC1009`), the most widely readable version, in this order: the header
(`$ACADVER`, `$CLAYER`, `$HANDLING`, `$HANDSEED`), the `CONTINUOUS` linetype followed by the
preserved linetypes, the layer table with colours and the off, frozen and locked states and the linetype name (only when the LTYPE table defines it, else `CONTINUOUS`; R12 has no plot flag, lineweight or description, so a layer with any of them carries them as extended data of the application `CADLITE_LAYER`, registered in the APPID table, which the reader restores and does not preserve), the other preserved
tables, the preserved blocks, and the entities with their handles. An `LWPolyline` is written as
`POLYLINE`/`VERTEX`/`SEQEND`; the vertex and sequence-end records take handles above the drawing's
seed, which are not part of the document. Header variables the document does not model are not
written.

**Dimensions** are written by the symbol writer in both versions, since each needs a block (a drawing
with one takes that path, as a drawing with blocks does; the automatic version stays R12). Every
`doc.Dimension`, in model space or in a block, gets a fresh anonymous block `*D<n>` (block flag 1, an
unused number; in AC1015 with its `BLOCK_RECORD`) holding its `Layout`: `LINE` for the extension and
dimension lines, `ARC` for an arc length dimension's arc and symbol, a `SOLID` per arrowhead, a
middle-centred `TEXT` (justification 1/2, so its position is exact) on layer 0 with colour BYBLOCK,
and `POINT` definition points (13, 14, 10) on layer `DEFPOINTS`, whose record is added, not plotting,
when the drawing lacks the layer. A linear dimension is a `DIMENSION` record: 2 the block, 10, 11, 70
(type, bit 32, bit 128 for moved text), 1 when there is an override, 53 when `TextAngle` is set, 3 the
style, 13, 14 and 50 (rotated only); AC1015 adds the `AcDbDimension`, `AcDbAlignedDimension` and
`AcDbRotatedDimension` markers, 71 (5, middle centre) and 42 (the measurement). The style values that
differ from the written `DIMSTYLE` record, read as the reader will read it, are written as ACAD
`DSTYLE` extended data (1040 for sizes, 1070 for DIMDEC, DIMDSEP, DIMZIN and DIMTAD). A style the
table lacks gets a record with the ISO-25 values (`generatedDimStyle`; R12 records hold no DIMDEC or
DIMDSEP, so in R12 those two are overrides), and the `ACAD` APPID is added when absent. R12 and
AutoCAD 2000 have no `ARC_DIMENSION`, so an arc length dimension is written as an `INSERT` of its
block, which other programs draw, carrying the whole definition as `CADLITE_DIM` extended data
(points, arc, flags, the full style and the text in 250-character chunks; layout in
`dimXDataKind`), with the `CADLITE_DIM` APPID; Cad-lite reads it back as the dimension, other
programs see a block reference. In AC1015 a preserved `DIMSTYLE` record from an R12 file loses groups 5
to 7 (R12's arrow block names, handles from R13 on), and preserved `VPORT`, `VIEW`, `UCS` and
`DIMSTYLE` records without subclass markers get their record subclass. A file written with
dimensions passes an ezdxf 1.4.2 audit clean in both versions.

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
