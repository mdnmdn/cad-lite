---
id: feat-dxf-files
title: DXF files
kind: feature
status: current
summary: Opening DXF and DWG drawings and saving DXF from the toolbar buttons, the native dialogs, the recent files menu, the command line, the -open flag and files the OS hands over (dropped, opened from Finder or the Dock, file arguments), how warnings and errors are reported, and where the lossy edges are.
read_when: you are changing OPEN, SAVE, the Open and Save buttons, the recent files and folders, the file dialogs, the -open flag, opening dropped or OS-opened files, which file types they accept, how file problems are reported to the user, or the Windows file associations
updated: 2026-10-24
verified: 2026-10-24
code_anchors: [fileio/dxf/dimension.go, ui/doctabs.go, packaging/windows/cad-lite.iss, filedialog/filedialog.go, filedialog/build.go, ui/filedialog.go, ui/saveformat.go, ui/titlebar.go, ui/recent.go, settings/recent.go, ui/openpaths.go, packaging/macos/Info.plist, packaging/linux/cad-lite.desktop, cmdsys/commands/files.go, cmd/cad/main.go, fileio/format.go, doc/unknown.go, cmdsys/command.go, fileio/dwg/read.go]
depends_on: [tech-file-io, tech-command-system]
review_cycle: quarterly
---

# DXF files

## Purpose

The drafter opens a DXF or DWG drawing, edits it and saves it as DXF. Anything Cad-lite cannot read
is reported rather than hidden, so the drafter knows what the opened drawing lacks.

## Behaviour

- **OPEN** asks `Enter path of drawing to open (.dxf or .dwg)`. On success the drawing opens in a
  [document tab](document-tabs.md) of its own (in place of the untouched untitled drawing, or the tab that
  has the file open), the view zooms to its extents and that tab's history reads `Opened <path> (<n> entities).`
- **SAVE** asks `Enter path of drawing to save` and writes the current drawing as DXF; the history
  reads `Saved <path> (<n> entities).`
- **QSAVE** (Ctrl+S) saves silently under the current path and asks for one only when the drawing
  has none. **SAVEAS** (Ctrl+Shift+S) and **SAVE** ask for the path, offering the current one as the default.
- **File format.** Typed SAVEAS first prints `Current file format: AutoCAD 2000/LT2000 DXF` and asks
  `Enter file format [R12/2000] <2000>:`, as AutoCAD's -SAVEAS does; the default is the format a save
  would write. `R12` writes AutoCAD R12/LT2 DXF (AC1009), `2000` AutoCAD 2000/LT2000 DXF (AC1015); the
  keywords are the same in Italian (`Formato file corrente: ...`, `Immettere il formato del file`).
  The drawing keeps the chosen format: QSAVE, SAVE, the `Save changes` question and the next SAVEAS
  save in it. A path typed at the format prompt is the file name, saved in the drawing's format
  without choosing one, so scripts that answer `SAVEAS plan.dxf` work as before (a file literally
  named `r`, `r12` or `2000` is taken as the keyword). SAVE and QSAVE never ask for a format.
- **Which format a drawing has.** A drawing read from an AutoCAD 2000 or newer DXF (`$ACADVER`
  AC1015 and up, and AC1012 and AC1014, R13 and R14) or from any DWG saves as AutoCAD 2000 DXF, the
  nearest version written. A new drawing, and one read from R12 or an older DXF, has no format of its
  own: it saves as R12 unless it has groups or object records, which need AutoCAD 2000 (the message
  then reads `Current file format: AutoCAD R12/LT2 DXF (automatic)`). Saving to R12 a drawing that
  needs AutoCAD 2000 leaves out its groups and object records and writes its MTEXT as TEXT, with one
  warning each, such as `groups not written (R12 has no groups; save as AutoCAD 2000 DXF to keep them): 1 groups`;
  the drawing itself keeps them.
- **A drawing opened from a DWG** has the `.dwg` path, which cannot be written (see below), so QSAVE
  asks for a path instead of failing, the Save button shows the save dialog, and SAVE, SAVEAS, the
  dialog and the `Save changes` question offer the same name with `.dxf` as the default.
- **Open and Save buttons.** The title bar has icon-only Open (Ctrl+O) and Save (Ctrl+S) buttons after the document chip; Save as
  is a right click on Save and Ctrl+Shift+S. Open shows the system open dialog and, when a file is
  chosen, runs OPEN with that path, so the tab choice, the project path and the agent history switch
  happen as for a typed OPEN. Save with a file runs QSAVE; Save without one shows the system save
  dialog (the current name, or `drawing`, pre-filled) and runs SAVEAS with the chosen path, in the
  drawing's format. Save as first shows a `Save as type` menu under the Save button listing
  `AutoCAD 2000/LT2000 DXF (*.dxf)` and `AutoCAD R12/LT2 DXF (*.dxf)`, the drawing's current format
  ticked; choosing one shows the save dialog listing that format alone and runs SAVEAS with the format
  and the path (AutoCAD picks the type inside its own save dialog; the system dialogs Cad-lite shows
  cannot report a chosen type, hence the menu, and Save of an untitled drawing offers no choice). A
  press outside the menu, or Save as again, closes it. Cancelling the dialog does
  nothing. One dialog is open at a time; the window stays usable behind it.
- **Recent files and folders.** Every drawing a successful OPEN or SAVE, SAVEAS or QSAVE names (typed, from the
  buttons, the dialogs, the recent menu or `-open`) moves to the front of the recent files, and its folder to the
  front of the recent folders; choosing a folder in the menu moves it to the front too. Paths are absolute and
  cleaned, duplicates are dropped (ignoring case on Windows and macOS), and each list keeps 10. NEW and CLOSE add
  nothing. The lists are saved in the settings file at once (`recent` table, see [Settings](settings.md)).
- **Open menu.** The chevron beside Open lists `Recent files` (format, name, folder; the tooltip has the full path)
  and `Recent folders`, then `Clear recent`, which empties both. A file opens through the same path as the Open
  dialog's answer, in a tab of its own. A folder shows the Open dialog starting in it (without
  a dialog, OPEN prompts on the command line). A file or folder that was not found when the menu opened is greyed
  but kept: clicking a missing file runs OPEN anyway, which fails with the usual history line, and then drops it
  from the list; a missing folder is dropped and the dialog starts where it otherwise would. The same file rows
  appear in the start panel over an empty canvas at startup (see [Drawing canvas](canvas.md)).
- **Dialog start folder.** The dialogs start in the drawing's folder; an untitled drawing's start in the most
  recent folder, else the working directory.
- **Dialog file types.** Open lists `All supported files` (every extension a registered reader reads),
  one entry per extension (`DXF drawing`) and `All files`; a reader added to the registry (DWG) appears
  with no UI change. Save lists one entry per extension a writer writes, and a name typed without an
  extension gets the first one.
- **Fallback.** Where no dialog can be shown (no `zenity` or `kdialog`, no display, a browser, a helper
  that fails) the buttons run OPEN, QSAVE or SAVEAS on the command line, so the user types the path as
  before. Typed OPEN, SAVE and SAVEAS always stay on the command line; the `FILEDIA` variable is stored
  but does not change that.
- **NEW** opens an empty untitled drawing in a new tab and does not touch files. **CLOSE** closes the
  tab, or empties the last one to an untitled drawing (see [Document tabs](document-tabs.md)).
- **Unsaved changes.** The session keeps a modified counter: every committed command, undo and redo adds
  one, and creating, opening or saving a drawing resets it (`Session.Dirty`, `Session.Modified`; the read-only
  variable `DBMOD` is 1 while it is not zero). The counter is per tab. In the desktop window, CLOSE of a
  dirty drawing, QUIT and EXIT with any dirty tab, and the window's close button show the unsaved-changes
  dialog, with **Close anyway** and **Cancel** and, when quitting, the list of every dirty drawing (see
  Document tabs); unlike AutoCAD it offers no Save: Cancel, then SAVE, keeps the
  changes. In a script, or a session without a window, CLOSE, QUIT and EXIT on a dirty drawing
  (QUIT and EXIT on every dirty tab in turn), and NEW and OPEN where no document host opens another tab, ask
  `Save changes to NAME? [Yes/No/Cancel] <Yes>` (`NAME` is the file name, or `Untitled`): Yes saves
  (asking for a path when there is none) and goes on, No goes on, Cancel stops. OPEN reads the file first, so
  a bad path fails before the question. A failed save keeps the drawing. The title bar's document chip shows a dot before the name while the
  drawing is dirty, as each document tab does (with a `*` after the name there). Layer table edits do not count, because they are not journalled.
- **QUIT** and **EXIT** end the application through the host (`Session.SetQuitHandler`); without a host they
  print `Quitting is not available on this platform.`
- The file commands are human-only: an AI agent that tries them is refused with
  `<NAME> is not available to agents.` (see the [command system](../tech/command-system.md)).
- **SAVE is atomic.** The file is written beside the target and renamed over it, so a failed save
  (full disk, write error) leaves the previous file intact.
- The path is typed text, so Space does not submit it. The file format is chosen by the extension,
  case-insensitively: `.dxf` is read and written, `.dwg` is read only.
- `-open PATH` on the command line opens a file at startup and zooms to its extents.
- **Files from the operating system.** A file dropped on the window, a file opened from Finder or
  dropped on the Dock icon (macOS), and files named after the flags (`cad-lite plan.dxf`, which is how
  Windows "Open with" and the Linux desktop entry pass them) all take one path: the first file whose
  extension a reader reads runs OPEN with that path, so the tab choice and the project switch happen as for
  a typed OPEN, and a launch file is fitted once the canvas has its
  size. Every such path opens, one tab each and in order (see [Document tabs](document-tabs.md)). Every
  other path is reported in the history and nothing else happens:
  `Cannot open notes.txt: Cad-lite opens DWG, DXF files.` for a file no reader reads (a dropped folder
  included). A drop
  lands anywhere on the window, the settings panel included. File arguments and `-open` do not mix;
  given both, the application stops with a usage error. The macOS bundle declares DXF (editor) and DWG
  (viewer) as document types at the `Alternate` rank, so Cad-lite is offered in "Open With" without
  becoming the default; the Linux desktop entry lists the DXF and DWG MIME types and passes `%F`.
- Opening into the untouched untitled tab replaces that drawing, its undo history and the selection;
  any other open leaves the current tab as it is.
- Reading and writing keep handles, layers (colour, off, frozen, locked, plot flag, lineweight, linetype name, description), colours and geometry for `LINE`, `CIRCLE`, `ARC`,
  polylines and `TEXT` (position, height, rotation, justification, width factor, obliquing, style name).
  Text sizes outside the ranges AutoCAD allows (a width factor past 100, a slant past 85°, a height past
  1e15) are clamped on reading, so a damaged file cannot make a text infinitely large.
  `MTEXT` opens as multiline text; saved to R12, which has no MTEXT, it becomes one TEXT per line,
  with a warning, and loses its box and wrapping. Linear, aligned and arc length dimensions open as
  dimensions with their style (the DIMSTYLE table and AutoCAD's per-dimension overrides), text
  override, moved text and text angle, and save in both formats with a regenerated drawing block, so
  other programs draw them; an arc length dimension, which R12 and AutoCAD 2000 lack, saves as a block
  reference that only Cad-lite reads back as a dimension. Other dimension types (angular, radial,
  ordinate) stay unknown objects and save unchanged with their blocks. What else is read, what is kept and which DXF version is written are owned by
  [File I/O](../tech/file-io.md).
- **DWG** (R14, R2000, R2004, R2007, R2010, R2013 and R2018) opens with its layers, units and the
  `LINE`, `CIRCLE`, `ARC`, `LWPOLYLINE`, 2D polyline and `TEXT` entities of model space. Every other entity
  is skipped, not kept: the history shows one warning such as `skipped 15 MULTILEADER, 11
  DIMENSION, ... (not read from DWG files; the DXF reader keeps them)`, and paper space is counted in a
  second. Saving to a `.dwg` path fails with `Save failed: DWG writing is not supported; save as
  .dxf`; the drawing stays as it was. Agents see the opened drawing like any other; they cannot run
  OPEN themselves.
- **Unknown entities.** An entity of a class Cad-lite does not model opens without error. It is not
  shown and cannot be selected, snapped to or edited, but it stays in the drawing and SAVE writes it
  again when R12 has its class. Entities of classes R12 lacks (SPLINE, HATCH, ELLIPSE and
  custom classes) are left out of the saved file, with a warning that names each one. The
  `list_entities` agent tool lists them with `unknown: true`.

Named blocks open and save as shared definitions with INSERT references and attributes. Groups use
native AC1015 GROUP objects and dictionaries; plain symbol drawings retain R12 output. Invalid,
cyclic or over-budget references remain preserved without display and produce warnings. INSERT also
loads DXF symbol libraries, and WBLOCK exports them; both external-file operations require a human.
DWG block import remains unsupported and keeps its existing skip warning.

### Opening from Windows Explorer

The Windows installer registers Cad-lite with the shell; the keys are listed in
[Operations](../tech/operations.md).

- Double-clicking a `.dxf` file opens it in Cad-lite when the installer's *Open .dxf files with
  Cad-lite by default* box was ticked (it is by default) and the user has not picked another program
  in Explorer. `.dwg` files become Cad-lite's only when *Open .dwg files with Cad-lite by default* is
  ticked (it is not by default).
- Cad-lite is always listed under "Open with" for `.dxf` and `.dwg`, and in Settings → Default apps.
- *Add "Open in Cad-lite" to the right-click menu* (ticked by default) adds that entry to `.dxf` and
  `.dwg` files whatever program opens them.
- Each of these starts `cad-lite.exe "<path>"`: the path as the first argument opens the drawing as
  `-open` does.
- Uninstalling removes all of it and gives a default back only while it is still Cad-lite's.

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

Layer-list order and drawing stacking survive save/reopen through the DXF LAYER table order,
including layer 0 in the middle or at the end. Both the ordinary R12 path and GROUP drawings' AC1015
path preserve it; loading restores it without an edit-history entry.

## Contract

- Entry point: `Workspace.OpenPaths(paths)` is the one function for files the OS hands over; it runs
  OPEN once per supported path, and OPEN puts each in a tab of its own.

- Commands: OPEN, SAVE, NEW, registered in `cmdsys/commands`; prompt kind `InputText`. SAVEAS's
  format prompt takes `InputKeyword | InputText` (keywords `R12`, `2000`; text is the path).
- Document: `doc.Header.Version` is the drawing's file format; SAVEAS sets it after a successful
  save with a chosen format.
- Interface: `cmdsys.FileHandler` (`Open`, `Save`), implemented by `fileio.Registry`; the session
  receives it through `Session.SetFiles`. A handler that also has `WritablePath(path) string`
  (`Registry` does) tells the file commands which name a read-only format can be saved under.
  receives it through `Session.SetFiles`. `fileio.Registry.ReadExtensions` and `WriteExtensions` feed the
  dialog filters. `ui.FileDialog` (`Open`, `Save`) is what the workspace shows dialogs with, installed by
  `Workspace.SetFileDialog`; `cmd/cad` installs `filedialog.System{}` for the window, not for `-png`.
- Document: a successful open into the same tab calls `Context.ReplaceDrawing`, which cancels the running
  command and clears the selection; otherwise `cmdsys.DocumentHost.OpenNew` gives the drawing a tab (`Context.Documents`).

## Implementation

`fileio/dxf/blocks.go`, `groups.go` and `symbolwrite.go` read and write native symbols and
groups; the document model owns validation and undo. `cmdsys/commands/block.go` uses the file
registry for WBLOCK and library INSERT, keeping atomic file writes and caller restrictions.

`fileCmd` in `cmdsys/commands/files.go` serves OPEN and SAVE: `Start` checks for a handler and
prompts, `Input` calls `Files.Open` or `Files.Save`, prints the warnings through `printWarnings`
(the limit of five) and the result line, and `ZoomExtents` frames the new drawing. `fileio.Registry`
in `fileio/format.go` picks the codec by extension; `fileio/dxf.Codec` reads and writes and
`fileio/dwg.Codec` reads. `writablePath` in `cmdsys/commands/files.go` swaps a `.dwg` default for
`.dxf`. `cmd/cad`
builds the registry, opens `-open` before creating the workspace and calls
`Workspace.FitOnFirstFrame`. File arguments go through `fileArgs` (which refuses them together with
`-open`) and `launchArgs` (which drops the `-psn_` argument older macOS launches add) and then to
`Workspace.OpenPaths`; `cmd/cad` also hands `gpui.App.OnOpenPaths` requests (the macOS open-documents
event) to it.

`ui/openpaths.go` holds `Workspace.OpenPaths`: it checks each path with the handler's `CanRead`,
reports the others through `Session.Say`, and for each supported one runs OPEN and submits the path in
the active tab (`openPath`); the document host (`ui/doctabs.go`) then makes the new tab active. The root view's `OnFileDrop` listener (`onFileDrop`) feeds it the paths of a
`gpui.FileDropEvent`. How the platform produces the drop and the open-documents event, and their
limits, is in the gpui contract (`internal/gpui/_docs/api-contract.md`, the ebiten platform section).

SAVEAS's format prompt is `fileCmd.formatPrompt`: it runs only when the handler has `SaveVersions`
and `SaveVersion` (`fileio.Registry` does; test doubles and the browser do not, and go straight to
the path), offers the versions that have a keyword in `formatKeywords`, and `fileCmd.save` sets
`Header.Version` around the save, restoring it when the save fails. `commands.FormatName` and
`FormatKeyword` name a version for the prompt and the UI (`dxf.version.*` in the catalogs).

`ui/filedialog.go` holds the button wiring: `Workspace.openFile`, `saveFile` and `saveFileAs` call
`startDialog`, which runs the `ui.FileDialog` on a goroutine and polls its answer with a `gpui.Window.After`
timer, so no gpui state is touched off the UI goroutine (the pattern of the agent discovery probe).
`ui/saveformat.go` holds the Save as format menu: `Workspace.saveFileAs` toggles it when the handler
offers formats (`saveFormats`), `saveButton` records the Save button's bounds to hang it under,
`saveFormatOverlay` draws it (mounted through `openMenuOverlay`), and `chooseSaveFormat` starts the
save dialog with `formatFilters` through `startSaveDialog`; `runSaveAs` answers SAVEAS's format
prompt with `_R12` or `_2000` and then the path. `pollDialog` then opens the chosen file through `Workspace.openFilePath`, the one function every "open this
path" in the UI goes through (it runs OPEN and submits the path, and OPEN picks the tab), or runs SAVEAS and submits the path, or falls back to the command on any error
other than `filedialog.ErrCancelled`. `openFilters` and `saveFilters` build the type lists from the
registry. `ui/recent.go` holds the recent lists' UI: `Workspace.noteRecentFile` is a `Session.OnFilePath`
listener, the single place a successful open or save is reported (`Context.SetFilePath`), and
`LoadSettingsFrom` passes it the file opened before the settings were read; `openButtons`, `openMenuOverlay`,
`openRecentFile`, `openRecentFolder` and `startPanel` build the menu and the start panel. The lists are
`settings.Recent` (`settings/recent.go`). The `filedialog` package (core, no gpui) builds one helper command per system, as pure
functions tested on any OS, and runs it without cgo: `osascript` (`choose file`, `choose file name`) on
macOS, `powershell.exe -STA` with `System.Windows.Forms` open and save dialogs on Windows, `zenity
--file-selection`, else `kdialog --getopenfilename`, on Linux and the BSDs when `DISPLAY` or
`WAYLAND_DISPLAY` is set. A browser build returns `ErrUnavailable`.

## Failure

- Warnings never fail an operation; the file opens or saves with what could be handled.
- Saving writes unknown entities of R12 classes and the preserved tables and blocks. Unknown
  entities of other classes are absent from the saved file, each with a warning, so a round trip of
  a file that has them is lossy.
- A dropped or OS-opened file that fails to read reports `Open failed: ...` like a typed path and opens
  no tab; an unsupported one is reported before OPEN starts, so no command runs.
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
- Keyboard navigation (arrows, Enter, Escape) in the Save as format menu, which takes clicks only.
- Write DWG, and read more DWG entities (MTEXT, INSERT and blocks first).
- Browser file access through the browser's own APIs.
- Pin a recent drawing so it stays at the top of the list.
- Open a file from the OS that is open in a tab but changed on disk as a reload, not a tab switch.
