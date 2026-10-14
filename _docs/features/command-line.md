---
id: feat-command-line
title: Command line
kind: feature
status: current
summary: The command line under the canvas — typing commands and answers, the prompt and history, Enter, Space, Escape and right-click, and the keyboard shortcuts.
read_when: you are changing how the user types commands or answers prompts, or adding a shortcut
updated: 2026-10-14
verified: 2026-10-14
code_anchors: [ui/workspace.go, cmdsys/session.go, cmdsys/input.go]
depends_on: [tech-command-system, tech-ui-framework]
review_cycle: quarterly
---

# Command line

## Purpose

The command line is the drafter's primary input, as in AutoCAD: type a command name or alias, then
answer its prompts with coordinates, distances, keywords or text. It always has keyboard focus, so
typing never needs a click first, and it shows the active prompt and the last lines of the command
history.

## Behaviour

- The prompt label shows `Command:` when no command runs, otherwise the active prompt in AutoCAD's
  form, e.g. `Specify next point or [Close/Undo]:`.
- The four most recent history lines show above the prompt. Each response is echoed after its
  prompt; errors, warnings and command output (`LIST`, `DIST`) are printed there. The history keeps
  the last 200 lines.
- **Enter** submits the field. At `Command:` an empty Enter repeats the last command.
- **Space** submits like Enter, except at a prompt that takes text (a layer name, a file path), where
  it types a space.
- **Right-click** on the canvas is Enter.
- **Escape** cancels the running command and prints `*Cancel*`; with no command running it clears
  the selection. What a command committed before Escape stays in the drawing.
- Command names and aliases are case-insensitive. An unknown name prints
  `Unknown command "NAME". Press F1 for help.` and starts nothing.
- Typed answers take the forms the [command system](../tech/command-system.md) lists: `x,y`,
  `@dx,dy`, `d<angle`, `@d<angle`, a bare number for direct distance entry, keywords by name or
  shortcut. An invalid answer prints the error and the prompt stays.
- Toolbar buttons run their command as if typed; the button of the running command is highlighted.
- The field clears after every submission, cancel and canvas click that answers a prompt.

Shortcuts, active anywhere in the window:

| Keys | Action |
|---|---|
| Ctrl+Z, Cmd+Z | UNDO |
| Ctrl+Y, Cmd+Shift+Z | REDO |
| F3 | Object snap on or off |
| F7 | Grid on or off |
| F8 | Ortho on or off |
| Ctrl+E | Zoom to extents |

## Contract

- Session: `Session.Submit`, `Session.Escape`, `Session.Run`, `Session.Prompt`,
  `Session.CurrentPrompt`, `Session.History`.
- Prompt input kinds, which decide whether Space submits (`InputText`).
- Key context `cad` and the actions in `ui.Bindings`.

## Implementation

`ui.NewWorkspace` creates the `gpui.TextInput` and wires its callbacks: `OnSubmit` →
`Workspace.submit` → `Session.Submit`; `OnEscape` → `Workspace.escape` → `Session.Escape`;
`OnChange` turns a trailing space into a submission unless `Workspace.acceptsText`.
`Workspace.commandArea` renders the history and the prompt; `Workspace.Render` focuses the field on
the first frame and binds the shortcut actions on the root element. Parsing and the history live in
`cmdsys/session.go` and `cmdsys/input.go`.

## Failure

- A parse error leaves the command at the same prompt and prints the reason (`Invalid point.`,
  `No base point for relative input.`).
- A command that fails its own check (a zero radius, collinear arc points) prints the reason and
  either re-prompts or ends without changing the drawing.

## Related docs

- [Command system](../tech/command-system.md) — parsing order, script rules, the session
- [Drawing and editing](drawing-and-editing.md) — the commands and their aliases
- [UI framework](../tech/ui-framework.md) — why the field keeps focus

## Next steps

- Autocomplete command names as they are typed (`Registry.Complete` exists).
- Up and down arrows to recall earlier input.
- F1 help listing the commands.
