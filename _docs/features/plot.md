---
id: feat-plot
title: Plot and print
kind: feature
status: current
summary: The PLOT command and its dialog — all or the selection; extents, display or a picked window; paper, orientation, fit or a fixed scale, centred or offset; colour, grayscale or black and white, with or without lineweights; printer and copies; a live preview — and how the sheet reaches a printer, a PDF file or the system PDF viewer.
read_when: you are changing PLOT or PRINT, the plot dialog, its preview, paper sizes or scales, how a drawing is placed on paper, the PDF it writes, the printer list or how a job reaches the printer
updated: 2026-11-10
verified: 2026-11-10
code_anchors: [plot/plot.go, plot/clip.go, plot/paper.go, plot/scale.go, plot/pdf.go, plot/raster.go, plot/spool.go, plot/temp.go, cmdsys/plot_hook.go, cmdsys/commands/plot.go, ui/plot.go, ui/plot_view.go, ui/titlebar.go, settings/settings.go, settings/file.go]
depends_on: [tech-architecture, tech-rendering, tech-command-system]
review_cycle: quarterly
---

# Plot and print

## Purpose

The user puts the drawing, or only the objects they selected, on a sheet of paper: they choose the
part of model space to show, the paper, the scale and where the plot sits on it, how colours and
lineweights print, see the sheet in a preview, and then print it on a printer of their choice,
open it in their PDF viewer to print through the system print dialog, or save it as a PDF.

## Behaviour

- **Opening.** `PLOT` (alias `PRINT`, Italian `STAMPA`), Ctrl+P (Cmd+P) and the printer button in
  the title bar open the dialog over the dimmed window. Escape and Cancel close it; Enter is Print,
  except in a text field, where it only ends the typing. The drawing's actions, keys and mouse wheel
  do not reach the canvas while it is open; the wheel scrolls the dialog's options when the window is
  too short for them, and the heading and the buttons stay in place.
- **What to plot.** *All* plots every entity on a layer that is on, thawed and set to plot (a layer
  with NoPlot is left out, as AutoCAD does); *Selection (n)* plots only the selected entities and is
  disabled with nothing selected. The dialog opens on Selection when something is selected and on
  All otherwise.
- **Plot area.** *Extents* takes the extents of what is plotted; *Display* the box the canvas
  showed when the dialog opened; *Window* a window picked on the drawing. Choosing Window
  before any window was picked, or *Pick window…*, hides the dialog and runs PLOT's two prompts,
  `Specify first corner of plot window` and `Specify opposite corner`, with a rubber-band rectangle
  and object snaps; the dialog comes back on Window with the new box. Escape during the pick brings
  the dialog back as it was; starting another command from a button or a shortcut, switching to
  another tab or closing this one ends the pick and leaves the dialog closed. A picked window lasts
  until the dialog is opened afresh. A window with no width or height has nothing to plot. Display
  and Window clip what crosses their edge.
- **Paper.** A4, A3, A2, A1, A0, Letter, Legal and Tabloid, portrait or landscape, with a 10 mm
  margin on every side; nothing prints in the margin.
- **Plot scale.** *Fit* scales the area uniformly to fill the paper inside the margins. A fixed
  scale (1:1, 1:2, 1:5, 1:10, 1:20, 1:50, 1:100, 1:200, 1:500, 1:1000, 2:1, 5:1, 10:1, or any ratio
  typed in the custom field as `1:250`, `1/250` or `1=250`) maps paper millimetres to real
  millimetres through the drawing's units (`INSUNITS`): a drawing in metres at 1:100 plots 1 m as
  10 mm; unitless and unusual units count as millimetres. A field that does not read as a ratio
  leaves the scale as it was. The dialog shows the result (`Fit to paper: 1 mm = 0.7942 drawing
  units`, `Scale 1:50: 1 mm = 50 drawing units`) and, when the area does not fit inside the margins,
  `The plot does not fit on the paper: what lies outside the margins is cut off.`
- **Plot offset.** *Center* centres the area on the paper; *Offset* puts the area's lower-left
  corner X and Y millimetres (negative allowed, up to ±10000) from the lower-left corner of the
  printable area.
- **Plot style.** *Color* plots each entity in its resolved colour, with ACI 7 black on the white
  paper; *Grayscale* plots each colour as the grey of its luminance; *Black and white* plots every
  line and fill black. *Lineweights* on plots every line as wide as its resolved lineweight in
  millimetres (Default is `LWDEFAULT`, 0.25 mm; a lineweight of 0 the finest line); off, every line
  plots 0.1 mm wide. Linetypes, text, dimensions, hatches and block references plot as the canvas
  draws them.
- **Printer and copies.** Opening the dialog lists the system's printers in the background (`Looking
  for printers…` meanwhile). *Default (name)* prints on the system default, each other printer by
  name; a saved printer missing from the list falls back to the default. Copies run from 1 to 99.
  Without a printing system the row says so and Print is disabled.
- **Preview.** The right of the dialog shows the sheet on white paper, redrawn whenever an option
  changes. When nothing plottable falls in the area it says `Nothing to plot in this area.` and Print,
  Open in PDF viewer and Save PDF are disabled.
- **Print** writes the sheet to a PDF in the plot temporary folder and hands it to the chosen
  printer; the dialog closes and the command line reports
  `Plot sent to the printer: A4 on Office, 2 copies.` **Open in PDF viewer** opens that PDF in the
  system's viewer, whose Print command shows the operating system's own print dialog (every printer
  option the driver has). **Save PDF…** asks for a file in the system save dialog (named after the
  drawing, `.pdf` added when missing), writes it there, closes and reports `Plot saved to <path>.`
- **Remembered options.** Paper, orientation, style, scale, area (Extents or Display), placement,
  lineweights, printer and copies are saved in the [settings](settings.md) file's `plot` table when
  they change and are where the next dialog, in this or a later session, starts.
- **Agents.** PLOT is refused to AI agents, like the file commands.

## Contract

- Commands: `PLOT` (`cmd.plot.*`), aliases `PRINT`, `STAMPA`; prompts `cmd.plot.prompt.first_corner`
  and `other_corner`; `cmd.plot.msg.unavailable` where the host has no dialog.
- Session: `cmdsys.PlotHost` (`PlotPicking`, `ShowPlot`), installed with `Session.SetPlotHost`;
  `Context.HasPlot`, `PlotPicking`, `ShowPlot`.
- Package `plot`: `Options`, `What`, `Area`, `Style`, `Scale` (`Fit`, `Scales`, `ParseScale`),
  `UnitMM`, `Paper`, `Papers`, `PaperByName`, `Compose`, `Extents`, `Plottable`, `Sheet`
  (`WritePDF`, `Raster`), `Item`, `ThinWidth`, `DefaultMargin`, `Printer`, `Printers`, `Job`,
  `MaxCopies`, `Print`, `OpenViewer`, `TempDir`, `TempPDF`, `ErrNothing`, `ErrNoPrinting`.
- Settings: the `plot` table (`settings.Plot`).
- Workspace: `SetPrinter` (`ui.Printer`), `SetPrinterLister` (`ui.PrinterLister`), `SetViewer`
  (`ui.Viewer`), `PlotOpen`; action `cad-plot`; catalog keys `ui.plot.*`. The drawing and its undo
  history are not changed.

## Implementation

`ui/titlebar.go` and the `cad-plot` binding run `PLOT` (`cmdsys/commands/plot.go`), which calls the
tab's `plotHost` (`ui/plot.go`). `plotHost.ShowPlot` either starts a fresh dialog
(`Workspace.startPlot` takes the selection and `View.Visible` and starts `listPrinters`) or returns
from a pick with the box. The pick belongs to the tab it started in (`plotState.pickTab`):
`Workspace.escape` marks a user's Escape, the one cancel that reopens the dialog, and
`abandonPlotPick` ends a pick when its tab is left or closed. The state is made on first use from the settings (`loadPlotPrefs`);
`setPlot`, `setScale` and `setJob` change it and save it back through `Workspace.change`
(`savePlotPrefs`). `ui/plot_view.go` mounts the dialog in `Workspace.Render` through `withPlot`;
each frame `refreshPlot` composes the sheet again only after a change marked it stale.

`plot.Compose` picks the plottable handles, works out the area and places it: fit or the fixed
scale times `UnitMM(INSUNITS)`, centred or at the offset; `Sheet.Overflow` records that the placed
area leaves the printable box, and `Sheet.add` (`plot/clip.go`) drops the items outside the clip box
and cuts the ones that cross it (Liang–Barsky for strokes, Sutherland–Hodgman for fills). It builds the items with a `render.Builder` of its own on a white
background, with `Lineweights` on and `Weight` 0, so a resolved lineweight comes back as pixels at
96 dpi and converts exactly to millimetres; the style maps the colours afterwards. The view scale
handed to the builder tessellates curves to 0.02 mm on paper. Items are in millimetres from the
paper's lower-left corner, y up, the same sense as PDF. `Sheet.WritePDF` writes one page with a
Flate-compressed content stream of `m`/`l`/`S` strokes and `f*` fills inside a clip rectangle;
`Sheet.Raster` draws the preview with `rasterx` (the rasteriser the icons use). Composing reads the
drawing, so it runs on the UI goroutine; the finished sheet is plain data, and the printer lookup,
writing the PDF, the save dialog, the viewer and printing run on goroutines whose results
`Workspace.pollPlot` collects.

`plot/spool.go`: `Printers` asks CUPS (`lpstat -e`, else `lpstat -a`, and `lpstat -d` for the
default) on Linux, the BSDs and macOS, and `Get-CimInstance Win32_Printer` on Windows. `Print` runs
`lp -n copies -d printer` (or `lpr -# copies -P printer`), and on Windows PowerShell's
`Start-Process -Verb Print`, or `-Verb PrintTo` with the printer, once per copy, which prints
through the program registered for PDFs; the file and the printer reach that script in the
`CADLITE_PLOT_FILE` and `CADLITE_PLOT_PRINTER` environment variables, never inside it, so no quote
in a path or a printer name can change it. `lpstat` is read from its standard output only. `OpenViewer` runs `xdg-open`, `open` or `rundll32
url.dll,FileProtocolHandler`. `plot/temp.go` writes job PDFs to `cad-lite-plot` in the temporary
folder and removes the ones older than a day before each new one, since a spooler or a viewer may
read the file after the call returns. The browser build has neither spooler nor viewer.

## Failure

- Nothing plottable in the area: the preview is blank paper with the message; Print, the viewer and
  Save PDF do nothing.
- No spooler (`lp`/`lpr` missing): the printer row says `No printing system found.` and Print is
  disabled; a spooler error (no default printer, an unknown queue) shows its message in the dialog,
  which stays open.
- The viewer cannot start: `Could not open the PDF viewer: …`.
- A save that fails shows `Could not save the PDF: …`; a cancelled save dialog changes nothing.
- Without a file dialog (the browser, tests) Save PDF is disabled.
- A settings file with an unknown plot style or area loads with the default and a warning; copies
  out of range are clamped to 1 to 99 with a warning; an unknown paper or scale falls back to A4 or
  Fit.
- A typed scale past 1:100000 or 100000:1 is not read. Geometry far off the paper (a fixed scale on
  a large drawing) is cut at the clip box, so the sheet and the PDF stay small.

## Related docs

- [Drawing and editing commands](drawing-and-editing.md) — the command list
- [Rendering](../tech/rendering.md) — the builder and display list a sheet is made from
- [Architecture](../tech/architecture.md) — where `plot` sits

## Next steps

- Plot paper-space layouts once they exist (G17, N25).
- Named page setups and plot style tables (CTB) per colour.
- Send the paper size and orientation to the spooler (`-o media=`) for printers that do not take them
  from the PDF page.
