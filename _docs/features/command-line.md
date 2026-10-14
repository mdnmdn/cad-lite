---
id: feat-command-line
title: Command line
kind: feature
status: current
summary: The command line under the canvas — typing commands and answers, the prompt, AutoComplete suggestions, Up/Down recall, the resizable history and recent commands, Enter, Space, Escape and right-click, and the keyboard shortcuts.
read_when: you are changing how the user types commands or answers prompts, or adding a shortcut
updated: 2026-10-15
verified: 2026-10-15
code_anchors: [ui/workspace.go, ui/cmdarea.go, ui/cmdcomplete.go, cmdsys/session.go, cmdsys/input.go, cmdsys/names.go, cmdsys/transparent.go, cmdsys/script.go, cmdsys/sysvars.go, cmdsys/wording.go, cmdsys/commands/sysvar.go, cmdsys/commands/script.go, i18n/keyword.go]
depends_on: [tech-command-system, tech-ui-framework]
review_cycle: quarterly
---

# Command line

## Purpose

The command line is the drafter's primary input, as in AutoCAD: type a command name or alias, then
answer its prompts with coordinates, distances, keywords or text. It always has keyboard focus, so
typing never needs a click first, and it shows the active prompt and the last lines of the command
history. It sits under the canvas only: the layers and agent columns run down beside it.

## Behaviour

- The command box is a rounded, bordered row: the prompt in monospace, the input field
  (monospace, placeholder `Type a command`) and a clock button at the right. The box's border turns to the accent
  colour while the field has focus. The prompt shows `Command:` when no command runs, otherwise the
  active prompt in AutoCAD's form, e.g. `Specify next point or [Close/Undo]:`.
- The most recent history lines show above the box, bottom-aligned, in muted monospace. Each response is echoed after its
  prompt; errors, warnings and command output (`LIST`, `DIST`) are printed there. The history keeps
  the last 200 lines. The region is 96 px high (less in a window shorter than 860 px, down to 54 px) until the user
  drags the small bar centred on its top edge: moving the pointer up grows it, down shrinks it, between one line and 480 px, and the
  height is saved with the settings. A double click on the bar resets it, and the chevron at the right end of the bar's row folds the
  history so only the input row remains (the height comes back when it is unfolded; the bar does not drag while it is folded).
- **Recent commands.** The clock button opens a menu above the box of the last 8 distinct commands the user started, newest first
  (typed or from a button), under their display names; clicking one runs it again (cancelling a running command, as the buttons do) and
  closes the menu. Escape closes it too. The list is not saved between sessions.
- **AutoComplete.** At `Command:` (no command running), typing a name opens a list above the box, at its left end, of the
  commands whose name or an alias contains the typed text (a lone `-` or `_` lists nothing: a name needs a character
  after it), one row per command, at most 10. A row shows the name
  that matched, an alias with the command it runs in brackets (`L (LINE)`, `LIVELLO (LAYER)`), and in muted text the
  command's display name when it is more than the name in another case (`Polyline`). Order: an exact name or alias
  first (so `L` then Enter runs LINE, `LI` LIST, as the aliases say), then the recent commands newest first (the same
  list as the clock menu), then names starting with the text before names containing it, alphabetical within each.
  The `_`, `.` and `'` prefixes are ignored. Text that is not a name (`10,10`, `@5<30`) shows no list, and neither does a
  system variable's exact name that no command matches (`OSMODE`), so Enter sets the variable. The first row is
  highlighted; **Up** and **Down**, and **Tab** and **Shift+Tab**, move the highlight and wrap at either end;
  **Enter** or **Space** runs the highlighted command, and a click runs a row. **Escape** closes the list and leaves the
  text, so the next Enter submits the text as typed; a second Escape cancels as usual. The list never shows while a
  command asks for a point, a value or text, and it closes when a command starts from a button. It hides the recent
  commands menu while it is open.
- **Recall.** With the list closed, **Up** at `Command:` puts the previous command line in the field, newest first, and
  **Down** goes forward again; Down past the newest brings back what was typed before. The lines are the ones entered
  at `Command:` (typed, recalled or accepted from the list; not empty Enters or button clicks), the last 100, without
  a repeat of the newest; they are not saved between sessions. A recalled line does not open the list; editing it
  does. So with the list open the arrows move its highlight, and Escape first, or an empty field, gives the arrows to
  recall.
- **Enter** submits the field. At `Command:` an empty Enter repeats the last command.
- **Space** submits like Enter, except at a prompt that takes text (a layer name, a file path, TEXT content), where
  it types a space.
- **Right-click** on the canvas is Enter.
- **Escape** cancels the running command and prints `*Cancel*`; with no command running it clears
  the selection. What a command committed before Escape stays in the drawing. It also closes the Open menu and
  the start panel (see [Drawing canvas](canvas.md)).
- **Language.** Prompts, history lines, errors and keywords follow the interface language
  ([Settings](settings.md)); in Italian the idle prompt is `Comando:`, a running command echoes as
  `Comando: LINEA` and `LINE` asks `Specificare punto successivo o [Chiudi/Annulla]:`.
- **Italian names.** The Italian names of the commands (`LINEA`, `POLILINEA`, `CERCHIO`, `ARCO`,
  `RETTANGOLO`, `CANCELLA`, `SPOSTA`, `COPIA`, `RUOTA`, `SCALA`, `ANNULLA`, `ZOOM`, `DISTANZA`,
  `TAGLIA`, `ESTENDI`, `SPEZZA`, `UNISCI`, `ESPLODI` and the rest of the catalog) are aliases that
  run the English command. They are registered in every language, so they work in an English
  session too. The canonical names stay English: `LINE`, `L` and a script written in English run
  unchanged in an Italian session.
- **Keywords.** An option is typed by its full name or its shortcut in the current language
  (`Chiudi` or `C`), and always by its English name or shortcut as well (`Close`). A leading `_`
  (`_C`, `_Close`) means English only, as in AutoCAD, which a script can use to stay language
  independent.
- Command names and aliases are case-insensitive, and at `Command:` a name may carry the prefixes `_`, `.`
  and `'` (`_LINE`, `.ERASE`, `_.LINE`), which are ignored. An unknown name prints
  `Unknown command "NAME".  Press F1 for more help.` (`Comando "NAME" sconosciuto.  Premere F1 per ulteriore aiuto.` in Italian) and starts nothing.
- **Command-line versions.** `-NAME` is the command-line version of a dialog command (`-LAYER`, `-OSNAP`,
  `-ARRAY`): its own command with its own prompts, registered as `-NAME` with its aliases as `-ALIAS`.
  `_-LAYER`, `-_LAYER` and the Italian `-LIVELLO` reach it too. Which `-` commands exist is the
  [drawing and editing](drawing-and-editing.md) document's list.
- **Transparent commands.** `'ZOOM`, `'PAN`, `'LAYER`, `'DIST`, `'ID`, `'SETVAR` and any other command
  that marks itself transparent run at a prompt of another command. The prompt shows `>>` while one runs (`>>>>` for a second level),
  and when it ends the interrupted prompt comes back unchanged. A text-entry prompt takes `'` as text; a command that is
  not transparent prints `NAME command not allowed transparently.` and the prompt stays. Escape cancels
  the whole stack. Enter at `Command:` repeats the last ordinary command, never a transparent one.
- **System variables.** Typing a variable name at `Command:` (`FILLETRAD`) or `SETVAR` asks
  `Enter new value for FILLETRAD <0.0000>:`; Enter keeps the value. `SETVAR` asks `Enter variable name or [?] <last>:`,
  and `?` lists variables matching a pattern (`*`, `ORTHO*`), read-only ones marked `(read only)`. A wrong value prints
  `Requires an integer value.`, `Requires numeric value.` or `Requires an integer value between 0 and 1.` and asks again;
  an unknown name prints `Unknown variable name.`. Variables with no effect here (`FILEDIA`, `DIMSCALE`)
  accept a value and read it back.
- **Scripts.** `SCRIPT` (alias `SCR`) asks `Enter script file name <last>:` and runs the file; `.scr` is added when the name has no
  extension. The nested script runs to its end and the calling script does not resume. `RSCRIPT` restarts the
  running script (at most 1000 times in a row: there is no Esc during a synchronous script) and `DELAY n` accepts 0 to 32767 and does not wait. In script text `;` starts a comment, a tab is a space,
  the file's last newline terminates the last response (it sends no Enter; a blank line before it does) and
  two separators in a row, or a trailing space, send Enter. At TEXT's `Enter text:` and MTEXT's `MText:` a space is
  part of the answer and only a newline ends it, so one script line is one line of text. `RESUME` is not provided.
- Typed answers take the forms the [command system](../tech/command-system.md) lists: `x,y`,
  `@dx,dy`, `d<angle`, `@d<angle`, `#x,y` (absolute), `@` alone (the last point), `<angle` (locks the
  direction of the next point: a bare number then is a distance along it), a bare number for direct
  distance entry, keywords by name or shortcut. Relative forms before any point was entered measure from
  the origin. Enter at a prompt that needs a point or distance and has no default ends the command quietly.
- An invalid answer prints the error and the prompt stays, worded by the kind of prompt: `Invalid point.`,
  `Point or option keyword required.`, `Requires numeric distance or second point.`,
  `Requires numeric angle or second point.`, `Invalid option keyword.`.
- Ribbon buttons run their command as if typed; the [canvas](canvas.md) document owns the ribbon's layout and icons.
- The field's placeholder, the handle's and the buttons' tooltips and the `Command:` label come from the catalog (`ui.cmdline.*`); the placeholder follows a language change at once.
- The field clears after every submission, cancel and canvas click that answers a prompt.

Shortcuts, active anywhere in the window:

| Keys | Action |
|---|---|
| Ctrl+Z, Cmd+Z | U (undo one step) |
| Ctrl+Y, Cmd+Shift+Z | REDO |
| Ctrl+O, Cmd+O | OPEN, through the system file dialog when there is one |
| Ctrl+S, Cmd+S | QSAVE; the save dialog first when the drawing has no file |
| Ctrl+Shift+S, Cmd+Shift+S | SAVEAS, through the system file dialog when there is one |
| Ctrl+Tab, Ctrl+Shift+Tab | Next or previous [document tab](document-tabs.md) |
| F2 | Agent panel on or off |
| F3 | Object snap on or off |
| F7 | Grid on or off |
| F8 | Ortho on or off |
| F9 | Snap to the snap grid on or off |
| F10 | Polar tracking on or off (turns ortho off) |
| Ctrl+E | Zoom to extents |
| Ctrl+, , Cmd+, | Settings panel on or off (the `OPTIONS` command opens it too) |

## Contract

- Session: `Session.Submit`, `Session.Escape`, `Session.Run`, `Session.Prompt`,
  `Session.CurrentPrompt`, `Session.History`, `Session.RunCount` and `Session.LastCommand` (which feed the recent list).
- Prompt input kinds, which decide whether Space submits (`InputText`).
- AutoComplete: `Registry.Names`, `Registry.AliasesOf` and `Vars.Has`, read through `Session.Registry` and `Session.Vars`.
- Key context `cad` and the actions in `ui.Bindings`.

## Implementation

`ui.NewWorkspace` creates the `gpui.TextInput` and wires its callbacks: `OnSubmit` →
`Workspace.submit` → `Session.Submit`; `OnEscape` → `Workspace.escape` → `Session.Escape`;
`OnChange` turns a trailing space into a submission unless `Workspace.acceptsText`.
`Workspace.wireCommandLine` (`ui/cmdcomplete.go`) installs those callbacks: `OnSubmit` → `Workspace.enterLine`, which
runs the highlighted suggestion (`acceptSuggestion` → `Workspace.run`) while `Workspace.suggesting`, else records the
line (`rememberLine`) and calls `Workspace.submit`; `OnEscape` closes the list or calls `Workspace.escape`; `OnChange`
recomputes the list with `Workspace.matchCommands`, which reads `Registry.Names`, `Registry.AliasesOf`, `Vars.Has` and the
recent list. `Workspace.commandKeyDown`, on the element around the field, takes Up, Down and Tab, which the field leaves
to its ancestors, for the highlight (`moveSuggestion`) or recall (`recallLine`); `Workspace.suggestList` draws the list.
`Workspace.commandArea` (`ui/cmdarea.go`) renders the handle, the history and the box; the field is
styled by `themeBareField`, so the box carries the border. `cmdAreaState` holds the dragged height
(`Workspace.dragMove` in `ui/layout.go`, fed by the root element's mouse move and up; the folded flag is in `layoutState`) and the recent list (`noteCommand`, called
by `Workspace.submit` and `Workspace.run` when `Session.RunCount` rose). `Workspace.Render` focuses the field on
the first frame and binds the shortcut actions on the root element. Parsing and the history live in
`cmdsys/session.go` and `cmdsys/input.go`.

## Failure

- A parse error leaves the command at the same prompt and prints the reason (see the wording list in
  Behaviour).
- A command that fails its own check (a zero radius, collinear arc points) prints the reason and
  either re-prompts or ends without changing the drawing.

## Related docs

- [Command system](../tech/command-system.md) — parsing order, script rules, the session
- [Drawing and editing](drawing-and-editing.md) — the commands and their aliases
- [UI framework](../tech/ui-framework.md) — why the field keeps focus

## Next steps

- Recall of earlier answers at a command's prompt (AutoCAD's Up at a point prompt), and recall and recent lists that
  survive a restart.
- AutoComplete for system variables, with icons and a scrolling list beyond 10 rows, as AutoCAD's has.
- F1 help listing the commands.
