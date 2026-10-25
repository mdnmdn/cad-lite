---
id: feat-document-tabs
title: Document tabs
kind: feature
status: current
summary: Several drawings open at once, one tab each above the canvas, each with its own session, undo history, view and agent conversations; how OPEN, NEW, CLOSE, QUIT and files from the OS choose a tab, and what follows the active tab.
read_when: you are changing the document tab strip, which tab a file opens in, closing or quitting with several drawings, or what is per drawing and what the window shares
updated: 2026-10-25
verified: 2026-10-25
code_anchors: [ui/doctabs.go, ui/discard.go, ui/workspace.go, ui/openpaths.go, ui/agent.go, ui/agent_store.go, cmdsys/dirty.go, cmdsys/commands/files.go, cmd/cad/main.go]
depends_on: [feat-dxf-files, feat-canvas, feat-agent-panel, tech-command-system]
review_cycle: monthly
---

# Document tabs

## Purpose

The drafter keeps several drawings open and moves between them as in AutoCAD's file tabs. Each
drawing has a tab of its own with its own command session, undo history, selection, view and agent
conversations; the window around them (ribbon, panels, settings, agent panel, recent files, the
command line field) is shared and shows the active tab.

## Behaviour

- **Tab strip.** A 30 px row above the canvas, in the canvas column (the side and agent panels keep
  their height). Each tab shows the file name, or `Untitled`, and an `x`; while the drawing has unsaved
  changes an 8 px dot in the accent colour leads the name (as on the title bar's document chip) and a
  `*` follows it, as on AutoCAD's file tabs (`plan.dxf*`), on active and inactive tabs and in every
  theme. The active tab is filled with the panel colour, the others are muted. The
  tooltip gives the path (or says the drawing is untitled), the unsaved-changes note and the keys.
  `+` after the tabs runs NEW. There is always at least one tab; the strip shows with one.
- **Unsaved changes.** Each tab's session tracks entity commits, undo and redo and host-recorded
  metadata edits. A layer switch or current-layer click in the UI is a `LAYER` undo step of that tab and
  marks it unsaved; saving clears the marker and replacing the drawing resets it.
- **Switching.** Clicking a tab, Ctrl+Tab (next) and Ctrl+Shift+Tab (previous; both wrap) make a tab
  active. The canvas, the command history and prompt, the status bar, the layers and properties
  panels, the title bar's document chip and the agent panel then show that drawing. A command running
  in the tab left keeps its prompt there and goes on when the tab is shown again; the command line's
  text is cleared. The view (centre and scale) and the drafting toggles are per tab.
- **Properties panel.** Its state is per tab: the target (a layer clicked in the Layers section, else
  the selection, else the defaults), the object-type filter and polyline vertex chosen for that
  selection, and the folded categories; a tab shows again as it was left. An open panel field belongs
  to the tab it was opened in: switching tabs (a click, Ctrl+Tab, Ctrl+Shift+Tab), closing that tab,
  Save, Save As, Open, New and quitting commit it there first, as one `PROPERTIES` undo step of that
  tab. A value that tab refuses is dropped on the switch, and an open drop-down closes.
  TEXTEDIT's modal blocks tab switching through clicks and shortcuts. If the host switches tabs or
  replaces the drawing, its uncommitted draft is discarded without changing the old document.
- **OPEN.** A typed OPEN, the Open button and dialog, the recent files menu, the start panel and files
  from the operating system all run OPEN, which opens the file in a new tab and makes it active; the
  current drawing is not discarded, so no `Save changes` question is asked. The file opens in place of
  the current drawing only when that is the untouched untitled one (no file, no changes, no entities),
  as at startup. A file open in a tab (same cleaned absolute path; case-insensitive on Windows
  and macOS) is not read again: its tab becomes active. Warnings and the `Opened ...` line go to the
  new tab's history.
- **Files from the operating system.** Every path a reader supports opens, one tab each, in order
  (the first replaces the untouched untitled drawing); each unsupported path is reported in the
  active tab's history. See [DXF files](dxf-files.md).
- **NEW** opens an empty untitled drawing in a new active tab and discards nothing.
- **CLOSE**, the `x` and a middle click close a tab. A tab with unsaved changes is made active and
  CLOSE shows the unsaved-changes dialog: *Unsaved changes*, `NAME has unsaved changes. Close it
  anyway?`, a hint to cancel and save first, and two buttons, **Close anyway** (red) and **Cancel**
  (Italian *Chiudi comunque* and *Annulla*). Cancel has the focus; Escape, Cancel or Enter on it keeps
  the drawing; Tab moves between the buttons, Enter and Space press the focused one. Close anyway
  closes without saving. A clean tab closes at once. The tab to the right becomes active, else the one
  to the left. The last tab is not removed: CLOSE empties it to an untitled drawing.
- **QUIT** and **EXIT**, typed or run from any tab, and the **window's close button** (on macOS also
  Cmd+Q and the app menu's Quit, which GLFW turns into the same close request) quit at once when no
  drawing has unsaved changes. Otherwise one dialog lists every drawing with unsaved changes (name,
  the path in the row's tooltip), asks whether to close Cad-lite anyway, and offers the same two
  buttons: Close anyway quits without saving any of them, Cancel keeps every tab as it was. Clean
  tabs are not listed. While the dialog is open the window's close button does nothing more.
- **Saving first.** The dialog has no Save button (a deviation from AutoCAD's Yes/No/Cancel, chosen
  for two clear outcomes): to keep the changes, Cancel, then SAVE (Ctrl+S, Cmd+S) or SAVEAS, then
  close again. Its hint says so and names the keys.
- **Scripts and hosts without a window** keep the command line's question: CLOSE asks
  `Save changes to NAME? [Yes/No/Cancel]`, and QUIT asks about that tab's drawing and then shows each
  other tab with unsaved changes in turn, asking the same question; Cancel at any of them stops the
  quit, and a later QUIT asks every dirty tab again. A script's next lines answer the question.
- **Agent panel.** The panel shows the active tab's conversations (per drawing, as in
  [Agent panel](agent-panel.md)). An agent session stays bound to the tab it was started in: its
  drawing tools act on that tab's drawing whichever tab is active, and its session keeps running while
  another tab is shown. Every new drawing tab starts with a fresh empty chat; switching an
  existing tab keeps its current conversations. Closing a tab ends its agent sessions;
  reopening the drawing starts fresh, with the current run's conversations available explicitly
  through History. Restart clears previous project history on first agent use. The agent button's live and attention dot covers every tab.
- **Settings.** The drawing aid defaults (grid, object snap, ortho) apply to every open tab when the
  settings change and to each new tab; a theme change redraws every tab.

## Contract

- Commands: NEW, OPEN, CLOSE, QUIT, EXIT reach the tabs through `cmdsys.DocumentHost`, installed per
  session with `Session.SetDocumentHost` (methods `ShowOpen`, `OpenNew`, `New`, `Close`); a session without
  one keeps one drawing. A host that also implements `cmdsys.DiscardConfirmer` (`ConfirmClose`,
  `ConfirmQuit`) asks CLOSE's and QUIT's question with its dialog, except while `Context.Scripted`.
- Window: `gpui.Window.OnShouldClose`, which `Workspace.Render` installs on the first frame.
- Workspace API: `Workspace.Documents` (`DocumentTab`: name, path, dirty, active), `DocumentSession`,
  `SwitchDocument`, `CloseDocument`, `SetQuitHandler`, `DiscardDialog`; `Workspace.Session` and `View` are the active tab's.
- Actions `cad-next-tab` (Ctrl+Tab) and `cad-prev-tab` (Ctrl+Shift+Tab) in key context `cad`.
- Element ids `doc-tabs`, `doc-tab-<i>`, `doc-tab-chip-<i>`, `doc-tab-close-<i>`, `doc-tab-new`;
  the dialog's `discard-overlay`, `discard-dialog`, `discard-list`, `discard-item-<i>`, `discard-cancel`,
  `discard-close`.
- Catalog keys `ui.tabs.*`, `ui.discard.*`. The design is [decision](../tech/decisions.md) D32.

## Implementation

`ui/doctabs.go` holds `docTab`, the per-drawing state (session, view, `render.Builder`, canvas state,
fit flag, project path, agent state), and `Workspace.newDocTab`, which builds the session and installs
its listeners and its `tabHost`, the `cmdsys.DocumentHost` bound to that tab. `Workspace` embeds the
active `*docTab`, so the shell code reads `ws.session`, `ws.view` and `ws.builder` of the active tab
unchanged; `activate` swaps it, carries the canvas bounds and pointer over, and calls
`AgentPanel.follow`. `removeTab`, `closeTab`, `cycleTab`, the quit chain (`quitState`, `quitFrom`) and
the strip (`tabStrip`, `docTabElement`) are there too. In `cmdsys/commands/files.go` OPEN asks the host's
`ShowOpen` before reading and `OpenNew` after, and prints to the session it returns; NEW asks `New`; CLOSE
calls `Close` after the unsaved-changes question. `Workspace.submit` and `run` hold the tab they started
in, since the answer may close it.
`ui/discard.go` is the dialog: `tabHost.ConfirmClose` and `ConfirmQuit` (only with a window, and for
QUIT with a quit handler and outside a command-line quit chain, `quitState.chaining`) call
`Workspace.askClose` and `askQuit`, which fill `Workspace.discard`; `Workspace.windowClosing` is the
window's `OnShouldClose` handler (true, so the app quits, when nothing is unsaved; else `askQuit` and
false); `withDiscard` lays `discardOverlay` over whatever `Workspace.render` built, outside the `cad`
key context, and `finishDiscard` closes it, gives the focus back to the command line and, for Close
anyway, runs CLOSE's continuation or the quit handler. In `cmdsys/commands/files.go`, `confirmer`
returns the host when it is a `DiscardConfirmer` and the session is not `Scripted`; CLOSE of a dirty
drawing and QUIT hand over to it. `docTab.caption` adds the `*`.
`cmdsys/dirty.go`, `Session.MarkModified` records host metadata edits in that session's modified
counter; `Dirty` drives the tab marker and the file commands' save question.

The agent panel (`ui/agent.go`) embeds the active tab's `agentSet` (conversation tabs, history state, and
the set's own cad MCP server, whose `cadmcp.Env.Session` returns that tab's session); `pump` drains every
set, `stopTab` closes a set's server with its last live session, and `closeDocument` stops a closing tab's
sessions. `ui/agent_store.go` keys the project directory and drawing name off the tab (`docTab.projectDir`,
`drawingKey`), and `pathChanged` reports only the active tab's path changes to the panel.

## Failure

- A file that fails to read opens no tab and reports `Open failed: ...` in the current tab's history.
- A failed save in the `Save changes` question of CLOSE or QUIT keeps the tab and stops the quit.
- Closing a tab while its agent is starting abandons the start; the session the start returns is closed.

## Related docs

- [DXF files](dxf-files.md) — OPEN, SAVE, the unsaved-changes question and files from the OS
- [Agent panel](agent-panel.md) — the conversations each drawing keeps
- [Drawing canvas](canvas.md) — the chrome around the canvas

## Next steps

- Reorder tabs by dragging, and a menu of all tabs when they overflow the strip.
- Remember the open tabs across restarts.
