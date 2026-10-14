---
id: feat-dxf-files
title: DXF files
kind: feature
status: current
summary: Opening and saving DXF drawings from the command line and the -open flag, how warnings and errors are reported, and where the lossy edges are.
read_when: you are changing OPEN, SAVE or the -open flag, or how file problems are reported to the user
updated: 2026-10-14
verified: 2026-10-14
code_anchors: [cmdsys/commands/util.go, cmd/cad/main.go, fileio/format.go]
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
- The path is typed text, so Space does not submit it. The file format is chosen by the extension,
  case-insensitively; `.dxf` is the only one.
- `-open PATH` on the command line opens a file at startup and zooms to its extents.
- Opening replaces the drawing, its undo history and the selection; the previous drawing is not saved first.
- Reading and writing keep handles, layers, colours and geometry for `LINE`, `CIRCLE`, `ARC` and
  polylines. What else is read, what is skipped and which DXF version is written are owned by
  [File I/O](../tech/file-io.md).

### Warnings

A reader or writer that skips or drops something reports a warning, such as
`skipped 12 TEXT entities (not supported)` on reading or `... not written (not supported by the R12 writer)` on saving. Each shows in the history as `Warning: <text>`. At most
five show; any further ones collapse into `... and N more warnings.` With `-open`, warnings go to the
standard error log instead, one line each, prefixed with the path.

### Errors

A failure ends the command and leaves the drawing as it was:

| Cause | History line |
|---|---|
| File missing or unreadable | `Open failed: <operating system message>` |
| Malformed or binary DXF | `Open failed: dxf: line N: ...` |
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
- Saving a drawing that was opened with warnings writes only what the document holds, so the skipped
  entities are absent from the saved file. The reader does not preserve them, so a round trip of a
  file with unsupported entities is lossy.
- The reader rejects binary DXF and files that end inside a group-code pair.

## Related docs

- [File I/O](../tech/file-io.md) — the codec, the supported entities and the DXF version written
- [Drawing and editing commands](drawing-and-editing.md) — the command list that names OPEN and SAVE
- [Backlog](../backlog.md) — unknown-object preservation, DXF 2018 and binary DXF

## Next steps

- Preserve unsupported entities so unread data survives a save.
- A native open and save dialog.
- Browser file access through the browser's own APIs.
