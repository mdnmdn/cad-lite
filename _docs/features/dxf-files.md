---
id: feat-dxf-files
title: DXF files
kind: feature
status: current
summary: Opening DXF and DWG drawings and saving DXF from the toolbar buttons, the native dialogs, the command line and the -open flag, how warnings and errors are reported, and where the lossy edges are.
read_when: you are changing OPEN, SAVE, the Open and Save buttons, the file dialogs or the -open flag, which file types they accept, or how file problems are reported to the user
updated: 2026-10-14
verified: 2026-10-14
code_anchors: [filedialog/filedialog.go, filedialog/build.go, ui/filedialog.go, cmdsys/commands/files.go, cmd/cad/main.go, fileio/format.go, doc/unknown.go, cmdsys/command.go, fileio/dwg/read.go]
depends_on: [tech-file-io, tech-command-system]
review_cycle: quarterly
---

# DXF files

## Purpose

The drafter opens a DXF or DWG drawing, edits it and saves it as DXF. Anything Cad-lite cannot read
is reported rather than hidden, so the drafter knows what the opened drawing lacks.

## Behaviour

- **OPEN** asks `Enter path of drawing to open (.dxf or .dwg)`. On success the open drawing is replaced, the view
  zooms to its extents and the history reads `Opened <path> (<n> entities).`
- **SAVE** asks `Enter path of drawing to save` and writes the current drawing as DXF; the history
  reads `Saved <path> (<n> entities).`
- **QSAVE** (Ctrl+S) saves silently under the current path and asks for one only when the drawing
  has none. **SAVEAS** (Ctrl+Shift+S) and **SAVE** ask for the path, offering the current one as the default.
- **A drawing opened from a DWG** has the `.dwg` path, which cannot be written (see below), so QSAVE
  asks for a path instead of failing, the Save button shows the save dialog, and SAVE, SAVEAS, the
  dialog and the `Save changes` question offer the same name with `.dxf` as the default.
- **Open and Save buttons.** The title bar has icon-only Open (Ctrl+O) and Save (Ctrl+S) buttons after the document chip; Save as
  is a right click on Save and Ctrl+Shift+S. Open shows the system open dialog and, when a file is
  chosen, runs OPEN with that path, so the `Save changes` question, the undo reset, the project path and
  the agent history switch happen as for a typed OPEN. Save with a file runs QSAVE; Save without one, and
  Save as, show the system save dialog (the current name, or `drawing`, pre-filled) and run SAVEAS with
  the chosen path. Cancelling the dialog does nothing. One dialog is open at a time; the window stays
  usable behind it.
- **Dialog file types.** Open lists `All supported files` (every extension a registered reader reads),
  one entry per extension (`DXF drawing`) and `All files`; a reader added to the registry (DWG) appears
  with no UI change. Save lists one entry per extension a writer writes, and a name typed without an
  extension gets the first one.
- **Fallback.** Where no dialog can be shown (no `zenity` or `kdialog`, no display, a browser, a helper
  that fails) the buttons run OPEN, QSAVE or SAVEAS on the command line, so the user types the path as
  before. Typed OPEN, SAVE and SAVEAS always stay on the command line; the `FILEDIA` variable is stored
  but does not change that.
- **NEW** discards the drawing for an empty one, forgets the path and does not touch files. **CLOSE** does
  the same (Cad-lite has one drawing).
- **Unsaved changes.** The session keeps a modified counter: every committed command, undo and redo adds
  one, and creating, opening or saving a drawing resets it (`Session.Dirty`, `Session.Modified`; the read-only
  variable `DBMOD` is 1 while it is not zero). NEW, OPEN, CLOSE, QUIT and EXIT on a dirty drawing ask
  `Save changes to NAME? [Yes/No/Cancel] <Yes>` (`NAME` is the file name, or `Untitled`): Yes saves
  (asking for a path when there is none) and goes on, No goes on, Cancel stops. OPEN reads the file first, so
  a bad path fails before the question. A failed save keeps the drawing. The title bar's document chip shows a dot before the name while the
  drawing is dirty. Layer table edits do not count, because they are not journalled. The window's own close
  button does not ask yet: the gpui platform has no close hook.
- **QUIT** and **EXIT** end the application through the host (`Session.SetQuitHandler`); without a host they
  print `Quitting is not available on this platform.`
- The file commands are human-only: an AI agent that tries them is refused with
  `<NAME> is not available to agents.` (see the [command system](../tech/command-system.md)).
- **SAVE is atomic.** The file is written beside the target and renamed over it, so a failed save
  (full disk, write error) leaves the previous file intact.
- The path is typed text, so Space does not submit it. The file format is chosen by the extension,
  case-insensitively: `.dxf` is read and written, `.dwg` is read only.
- `-open PATH` on the command line opens a file at startup and zooms to its extents.
- Opening replaces the drawing, its undo history and the selection; the previous drawing is saved only if the user answers Yes to `Save changes`.
- Reading and writing keep handles, layers (colour, off, frozen, locked, plot flag, lineweight, linetype name, description), colours and geometry for `LINE`, `CIRCLE`, `ARC` and
  polylines. What else is read, what is kept and which DXF version is written are owned by
  [File I/O](../tech/file-io.md).
- **DWG** (R14, R2000, R2004, R2007, R2010, R2013 and R2018) opens with its layers, units and the
  `LINE`, `CIRCLE`, `ARC`, `LWPOLYLINE` and 2D polyline entities of model space. Every other entity
  is skipped, not kept: the history shows one warning such as `skipped 29 TEXT, 15 MULTILEADER, 11
  DIMENSION, ... (not read from DWG files; the DXF reader keeps them)`, and paper space is counted in a
  second. Saving to a `.dwg` path fails with `Save failed: DWG writing is not supported; save as
  .dxf`; the drawing stays as it was. Agents see the opened drawing like any other; they cannot run
  OPEN themselves.
- **Unknown entities.** An entity of a class Cad-lite does not model opens without error. It is not
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
| DWG of a version before R14 | `Open failed: DWG version AC1009 (R11/R12) is not supported; open the drawing in a CAD application and save it as DXF` |
| A `.dwg` that is not DWG, or is damaged | `Open failed: not a DWG file`, or `Open failed: DWG file is damaged or truncated: ...` (a damaged object inside an otherwise sound file only counts in a warning) |
| Truncated DXF (a section without `ENDSEC`) | `Open failed: dxf: section "ENTITIES" has no ENDSEC (truncated file?)` |
| Extension with no reader or writer | `Open failed: no reader for ".xyz" files`, or `Save failed: no writer for ".xyz" files` |
| Save to a `.dwg` path | `Save failed: DWG writing is not supported; save as .dxf` |
| Path not writable | `Save failed: <operating system message>` |
| Dialog helper missing or failing | no message; the command line asks for the path |

With `-open`, a failure prints the error and exits with a non-zero status. In a build with no file
handler (the browser), OPEN and SAVE print `File commands are not available on this platform.`

## Contract

- Commands: OPEN, SAVE, NEW, registered in `cmdsys/commands`; prompt kind `InputText`.
- Interface: `cmdsys.FileHandler` (`Open`, `Save`), implemented by `fileio.Registry`; the session
  receives it through `Session.SetFiles`. A handler that also has `WritablePath(path) string`
  (`Registry` does) tells the file commands which name a read-only format can be saved under.
  receives it through `Session.SetFiles`. `fileio.Registry.ReadExtensions` and `WriteExtensions` feed the
  dialog filters. `ui.FileDialog` (`Open`, `Save`) is what the workspace shows dialogs with, installed by
  `Workspace.SetFileDialog`; `cmd/cad` installs `filedialog.System{}` for the window, not for `-png`.
- Document: a successful open calls `Context.ReplaceDrawing`, which cancels the running command and clears the selection.

## Implementation

`fileCmd` in `cmdsys/commands/files.go` serves OPEN and SAVE: `Start` checks for a handler and
prompts, `Input` calls `Files.Open` or `Files.Save`, prints the warnings through `printWarnings`
(the limit of five) and the result line, and `ZoomExtents` frames the new drawing. `fileio.Registry`
in `fileio/format.go` picks the codec by extension; `fileio/dxf.Codec` reads and writes and
`fileio/dwg.Codec` reads. `writablePath` in `cmdsys/commands/files.go` swaps a `.dwg` default for
`.dxf`. `cmd/cad`
builds the registry, opens `-open` before creating the workspace and calls
`Workspace.FitOnFirstFrame`.

`ui/filedialog.go` holds the button wiring: `Workspace.openFile`, `saveFile` and `saveFileAs` call
`startDialog`, which runs the `ui.FileDialog` on a goroutine and polls its answer with a `gpui.Window.After`
timer, so no gpui state is touched off the UI goroutine (the pattern of the agent discovery probe).
`pollDialog` then runs OPEN or SAVEAS and submits the path, or falls back to the command on any error
other than `filedialog.ErrCancelled`. `openFilters` and `saveFilters` build the type lists from the
registry. The `filedialog` package (core, no gpui) builds one helper command per system, as pure
functions tested on any OS, and runs it without cgo: `osascript` (`choose file`, `choose file name`) on
macOS, `powershell.exe -STA` with `System.Windows.Forms` open and save dialogs on Windows, `zenity
--file-selection`, else `kdialog --getopenfilename`, on Linux and the BSDs when `DISPLAY` or
`WAYLAND_DISPLAY` is set. A browser build returns `ErrUnavailable`.

## Failure

- Warnings never fail an operation; the file opens or saves with what could be handled.
- Saving writes unknown entities of R12 classes and the preserved tables and blocks. Unknown
  entities of other classes are absent from the saved file, each with a warning, so a round trip of
  a file that has them is lossy.
- A DWG that cannot be read to the end opens with what could be: a damaged or CRC-failing object is
  counted in a warning and left out; an unreadable file header or section map is an error.
- The reader rejects binary DXF, files that end inside a group-code pair and sections without `ENDSEC`.
  A file without the `EOF` marker opens with a warning, and non-finite numbers (`nan`, `inf`) are read as
  0 with a warning.

## Related docs

- [File I/O](../tech/file-io.md) — the codecs, the supported entities, the DXF version written and the DWG versions read
- [Drawing and editing commands](drawing-and-editing.md) — the command list that names OPEN and SAVE
- [Backlog](../backlog.md) — unknown objects that cannot be saved, newer DXF versions and binary DXF

## Next steps

- Write newer DXF versions so unknown entities of classes R12 lacks survive a save.
- Write DWG, and read more DWG entities (TEXT, INSERT and blocks first).
- Browser file access through the browser's own APIs.
