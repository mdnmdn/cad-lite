---
id: feat-dxf-files
title: DXF files
kind: feature
status: current
summary: Opening and saving DXF drawings from the command line and the -open flag, how warnings and errors are reported, and where the lossy edges are.
read_when: you are changing OPEN, SAVE or the -open flag, or how file problems are reported to the user
updated: 2026-10-14
verified: 2026-10-14
code_anchors: [cmdsys/commands/util.go, cmd/cad/main.go, fileio/format.go, doc/unknown.go, cmdsys/command.go]
depends_on: [tech-file-io, tech-command-system]
review_cycle: quarterly
---

# DXF files

## Purpose

The drafter opens a DXF drawing, edits it and saves it back to DXF. Anything go-draw cannot read is
reported rather than hidden, so the drafter knows what the opened drawing lacks.

## Behaviour

- **OPEN** asks `Enter path of drawing to open`. On success the open drawing is replaced, the view
  zooms to its extents and the history reads `Opened <path> (<n> entities).`
- **SAVE** asks `Enter path of drawing to save` and writes the current drawing; the history reads
  `Saved <path> (<n> entities).`
- **NEW** discards the drawing for an empty one and does not touch files.
- NEW, OPEN and SAVE are human-only: an AI agent that tries them is refused with
  `<NAME> is not available to agents.` (see the [command system](../tech/command-system.md)).
- **SAVE is atomic.** The file is written beside the target and renamed over it, so a failed save
  (full disk, write error) leaves the previous file intact.
- The path is typed text, so Space does not submit it. The file format is chosen by the extension,
  case-insensitively; `.dxf` is the only one.
- `-open PATH` on the command line opens a file at startup and zooms to its extents.
- Opening replaces the drawing, its undo history and the selection; the previous drawing is not saved first.
- Reading and writing keep handles, layers, colours and geometry for `LINE`, `CIRCLE`, `ARC` and
  polylines. What else is read, what is kept and which DXF version is written are owned by
  [File I/O](../tech/file-io.md).
- **Unknown entities.** An entity of a class go-draw does not model opens without error. It is not
  shown and cannot be selected, snapped to or edited, but it stays in the drawing and SAVE writes it
  again when R12 has its class. Entities of classes R12 lacks (SPLINE, MTEXT, HATCH, ELLIPSE and
  custom classes) are left out of the saved file, with a warning that names each one. The
  `list_entities` agent tool lists them with `unknown: true`.

### Warnings

A reader or writer that keeps, skips or drops something reports a warning, such as
`kept 12 SPLINE entities as unknown objects (not displayed or editable)` on reading, one per unknown
class, or `... not written (not supported by the R12 writer)` on saving. Each shows in the history as `Warning: <text>`. At most
five show; any further ones collapse into `... and N more warnings.` With `-open`, warnings go to the
standard error log instead, one line each, prefixed with the path.

Warning and error sentences are in the language active when the file is read or written; the
examples here are the English text. DXF tokens inside them (`SPLINE`, `ENDSEC`, handles) stay as
written.

### Errors

A failure ends the command and leaves the drawing as it was:

| Cause | History line |
|---|---|
| File missing or unreadable | `Open failed: <operating system message>` |
| Malformed or binary DXF | `Open failed: dxf: line N: ...` |
| Truncated DXF (a section without `ENDSEC`) | `Open failed: dxf: section "ENTITIES" has no ENDSEC (truncated file?)` |
| Extension with no reader or writer | `Open failed: no reader for ".dwg" files`, or `Save failed: no writer for ...` |
| Path not writable | `Save failed: <operating system message>` |

With `-open`, a failure prints the error and exits with a non-zero status. In a build with no file
handler (the browser), OPEN and SAVE print `File commands are not available on this platform.`

## Contract

- Commands: OPEN, SAVE, NEW, registered in `cmdsys/commands`; prompt kind `InputText`.
- Interface: `cmdsys.FileHandler` (`Open`, `Save`), implemented by `fileio.Registry`; the session
  receives it through `Session.SetFiles`.
- Document: a successful open calls `Context.ReplaceDrawing`, which cancels the running command and clears the selection.

## Implementation

`fileCmd` in `cmdsys/commands/util.go` serves OPEN and SAVE: `Start` checks for a handler and
prompts, `Input` calls `Files.Open` or `Files.Save`, prints the warnings through `printWarnings`
(the limit of five) and the result line, and `ZoomExtents` frames the new drawing. `fileio.Registry`
in `fileio/format.go` picks the codec by extension; `fileio/dxf.Codec` reads and writes. `cmd/cad`
builds the registry, opens `-open` before creating the workspace and calls
`Workspace.FitOnFirstFrame`.

## Failure

- Warnings never fail an operation; the file opens or saves with what could be handled.
- Saving writes unknown entities of R12 classes and the preserved tables and blocks. Unknown
  entities of other classes are absent from the saved file, each with a warning, so a round trip of
  a file that has them is lossy.
- The reader rejects binary DXF, files that end inside a group-code pair and sections without `ENDSEC`.
  A file without the `EOF` marker opens with a warning, and non-finite numbers (`nan`, `inf`) are read as
  0 with a warning.

## Related docs

- [File I/O](../tech/file-io.md) — the codec, the supported entities and the DXF version written
- [Drawing and editing commands](drawing-and-editing.md) — the command list that names OPEN and SAVE
- [Backlog](../backlog.md) — unknown objects that cannot be saved, newer DXF versions and binary DXF

## Next steps

- Write newer DXF versions so unknown entities of classes R12 lacks survive a save.
- A native open and save dialog.
- Browser file access through the browser's own APIs.
