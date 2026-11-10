---
id: feat-plot
title: Plot and print
kind: feature
status: current
summary: The PLOT command and its dialog — all or the selection, extents, display or a picked window, paper size, orientation, colour or black and white, a live preview — and how the sheet is printed through the system spooler or saved as a vector PDF.
read_when: you are changing PLOT or PRINT, the plot dialog, its preview, paper sizes, how a drawing is fitted on paper, the PDF it writes or how a job reaches the printer
updated: 2026-11-10
verified: 2026-11-10
code_anchors: [plot/plot.go, plot/paper.go, plot/pdf.go, plot/raster.go, plot/spool.go, cmdsys/plot_hook.go, cmdsys/commands/plot.go, ui/plot.go, ui/plot_view.go, ui/titlebar.go]
depends_on: [tech-architecture, tech-rendering, tech-command-system]
review_cycle: quarterly
---

# Plot and print

## Purpose

The user puts the drawing, or only the objects they selected, on a sheet of paper: they choose the
part of model space to show, the paper size and orientation and whether it plots in colour or black
and white, see the sheet in a preview, and then print it on the default printer or save it as a PDF.

## Behaviour

- **Opening.** `PLOT` (alias `PRINT`, Italian `STAMPA`), Ctrl+P (Cmd+P) and the printer button in
  the title bar open the dialog over the dimmed window. Escape and Cancel close it; Enter is Print.
  The drawing's actions and keys do not reach the canvas while it is open.
- **What to plot.** *All* plots every entity on a layer that is on, thawed and set to plot (a layer
  with NoPlot is left out, as AutoCAD does); *Selection (n)* plots only the selected entities and is
  disabled with nothing selected. The dialog opens on Selection when something is selected and on
  All otherwise.
- **Plot area.** *Extents* fits the extents of what is plotted; *Display* plots the box the canvas
  showed when the dialog opened; *Window* plots a window picked on the drawing. Choosing Window
  before any window was picked, or *Pick window…*, hides the dialog and runs PLOT's two prompts,
  `Specify first corner of plot window` and `Specify opposite corner`, with a rubber-band rectangle
  and object snaps; the dialog comes back on Window with the new box. Escape during the pick brings
  the dialog back as it was. A picked window lasts until the dialog is opened afresh. Display and
  Window clip what crosses their edge.
- **Paper.** A4, A3, A2, A1, A0, Letter, Legal and Tabloid, portrait or landscape. The area is
  scaled uniformly to fit the paper inside a 10 mm margin and centred on it; the dialog shows the
  sheet (`A4 Landscape, 297 × 210 mm`) and the scale (`Fit to paper: 1 mm = 0.7942 drawing units`).
- **Colour or black and white.** Colour plots each entity in its resolved colour, with ACI 7 black
  on the white paper; Black and white plots every line and fill black.
- **Lines on paper.** Every line is as wide as its resolved lineweight in millimetres (Default is
  `LWDEFAULT`, 0.25 mm), a lineweight of 0 the finest line; linetypes, text, dimensions, hatches and
  block references plot as the canvas draws them.
- **Preview.** The right of the dialog shows the sheet on white paper, redrawn whenever an option
  changes. When nothing plottable falls in the area it says `Nothing to plot in this area.` and Print
  and Save PDF are disabled.
- **Print** writes the sheet to a temporary PDF and hands it to the default printer; the dialog
  closes and the command line reports `Plot sent to the printer (A4).` **Save PDF…** asks for a file
  in the system save dialog (named after the drawing, `.pdf` added when missing), writes it there,
  closes and reports `Plot saved to <path>.` The options outlast the dialog for the rest of the
  session.
- **Agents.** PLOT is refused to AI agents, like the file commands.

## Contract

- Commands: `PLOT` (`cmd.plot.*`), aliases `PRINT`, `STAMPA`; prompts `cmd.plot.prompt.first_corner`
  and `other_corner`; `cmd.plot.msg.unavailable` where the host has no dialog.
- Session: `cmdsys.PlotHost` (`PlotPicking`, `ShowPlot`), installed with `Session.SetPlotHost`;
  `Context.HasPlot`, `PlotPicking`, `ShowPlot`.
- Package `plot`: `Options`, `What`, `Area`, `Paper`, `Papers`, `PaperByName`, `Compose`,
  `Extents`, `Plottable`, `Sheet` (`WritePDF`, `Raster`), `Item`, `Print`, `ErrNothing`,
  `ErrNoPrinting`, `DefaultMargin`.
- Workspace: `SetPrinter`, `PlotOpen`; action `cad-plot`; catalog keys `ui.plot.*`. The drawing,
  its undo history and the settings file are not changed.

## Implementation

`ui/titlebar.go` and the `cad-plot` binding run `PLOT` (`cmdsys/commands/plot.go`), which calls the
tab's `plotHost` (`ui/plot.go`). `plotHost.ShowPlot` either starts a fresh dialog
(`Workspace.startPlot` takes the selection and `View.Visible`) or returns from a pick with the box.
`ui/plot_view.go` mounts the dialog in `Workspace.Render` through `withPlot`; each frame
`refreshPlot` composes the sheet again only after `setPlot` marked it stale.

`plot.Compose` picks the plottable handles, works out the area, and fits it on the paper; it builds
the items with a `render.Builder` of its own on a white background, with `Lineweights` on and
`Weight` 0, so a resolved lineweight comes back as pixels at 96 dpi and converts exactly to
millimetres. The view scale handed to the builder tessellates curves to 0.02 mm on paper. Items are
mapped to millimetres from the paper's lower-left corner, y up, the same sense as PDF.
`Sheet.WritePDF` writes one page with a Flate-compressed content stream of `m`/`l`/`S` strokes and
`f*` fills inside a clip rectangle; `Sheet.Raster` draws the preview with `rasterx` (the
rasteriser the icons use). Composing reads the drawing, so it runs on the UI goroutine; the
finished sheet is plain data, and writing the PDF, the save dialog and printing run on a goroutine
whose result `Workspace.pollPlot` collects.

`plot.Print` runs `lp` (or `lpr`) on Linux, the BSDs and macOS, and PowerShell's `Start-Process
-Verb Print` on Windows, which prints through the program registered for PDFs; the browser build has
no spooler and its Print button is disabled.

## Failure

- Nothing plottable in the area: the preview is blank paper with the message, Print and Save PDF do
  nothing.
- No spooler (`lp`/`lpr` missing): `Printing is not available on this system. Save a PDF instead.`
  in the dialog, which stays open; a spooler error (no default printer) shows its message the same
  way.
- A save that fails shows `Could not save the PDF: …`; a cancelled save dialog changes nothing.
- Without a file dialog (the browser, tests) Save PDF is disabled.

## Related docs

- [Drawing and editing commands](drawing-and-editing.md) — the command list
- [Rendering](../tech/rendering.md) — the builder and display list a sheet is made from
- [Architecture](../tech/architecture.md) — where `plot` sits

## Next steps

- Choose the printer, the number of copies and a fixed scale (1:50, 1:100) instead of fit.
- Plot paper-space layouts once they exist (G17).
- Remember the plot options in the settings file or the drawing.
