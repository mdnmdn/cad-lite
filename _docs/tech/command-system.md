---
id: tech-command-system
title: Command system
kind: tech
status: current
summary: The command model, the session state machine, typed-input parsing, the input pipeline from pointer to point, object snaps, ortho and selection.
read_when: you are writing a command, adding an input form or a snap mode, or changing how clicks and keys reach commands
updated: 2026-10-14
verified: 2026-10-14
code_anchors: [cmdsys/command.go, cmdsys/session.go, cmdsys/input.go, cmdsys/snap.go]
depends_on: [tech-architecture, tech-document-model, tech-geometry]
review_cycle: quarterly
---

# Command system

## What it is

`cmdsys` runs AutoCAD-style commands. A command is a state machine that asks for typed input through
prompts, as the AutoCAD command line does. A `cmdsys.Session` owns one drawing's interaction state —
the active command and prompt, the selection, the snap and ortho settings, the history — and turns
raw input from the UI into responses for the active prompt. The built-in commands live in
`cmdsys/commands`; which commands exist is owned by the
[drawing and editing](../features/drawing-and-editing.md) feature document.

## Model

**Command.** `cmdsys.Command` has `Name`, `Start` (the first prompt, or nil to finish at once),
`Input` (consume one response, return the next prompt or nil when done), `Preview` (append
rubber-band geometry for the cursor to a display list) and `Cancel`.

**Context.** A command reaches the document, the view, the spatial index, the selection and the file
handler through `cmdsys.Context`. `Context.Tx` opens the command's transaction on first use.

**Prompt.** `cmdsys.Prompt` has the message, the set of accepted input kinds, keywords, an optional
default, and an optional `Base` point. `Prompt.String` renders AutoCAD's form:
`Specify next point or [Close/Undo] <default>:`. A keyword's shortcut is its capital letters
(`eXit` → `X`), or its first letter when it has none.

**Input kinds.** Point, distance, angle (radians internally, degrees typed), keyword, selection,
text, integer and Enter. `cmdsys.Input` carries the response.

**Registry.** `cmdsys.Registry` maps upper-case names and aliases to command factories.
`Registry.LoadAliases` reads `ALIAS,*COMMAND` lines in the `acad.pgp` format.

## Rules

1. **One command, one transaction, one undo step.** The session commits the command's transaction
   when `Input` returns nil, and also on cancel, so what a command committed before Escape stays (as
   AutoCAD keeps the segments of a cancelled LINE).
2. **Starting a command cancels the running one.** Replacing the drawing (OPEN, NEW) from inside a
   command does not print a cancel.
3. **Enter at `Command:` repeats the last command.** An unknown name prints
   `Unknown command "NAME".` and starts nothing.
4. **Typed input** is parsed by `cmdsys.ParseInput`, in this order:
   - empty → Enter if accepted, else the default, else an error;
   - a keyword or its shortcut, case-insensitive;
   - a point: `x,y`, `@dx,dy`, `d<angle`, `@d<angle`; `@` measures from the prompt's `Base`, or
     from the last point entered when the prompt has none;
   - a bare number: a distance, an angle in degrees, or an integer when the prompt takes one;
     otherwise, at a point prompt with a base, a point that distance along the cursor direction
     (direct distance entry);
   - free text, when accepted.
   A parse error is printed to the history and the prompt stays. Numbers must be finite: `nan`,
   `inf` and values that overflow (`1e999`) are rejected as an invalid value, and a point whose result overflows
   to infinity as an invalid point.
5. **Scripts** follow AutoCAD script rules: `Session.Script` splits on single spaces and newlines and
   submits each token; an empty token (two separators in a row) is Enter.
6. **A selection prompt answers with the selection set** when the user presses Enter. Modify commands
   take a non-empty pre-selection without prompting (noun-verb editing) and clear the selection.

7. **A panic in a command does not end the session.** `Session.guard` wraps the command's `Start`,
   `Input`, `Cancel` and `Preview`. On a panic it aborts the open transaction (`doc.Drawing.AbortOpen`),
   ends the command and its prompt, and prints `Error: NAME failed: ...`; the session accepts the next
   command.
8. **Human-only commands.** A command that implements `cmdsys.HumanOnly` (NEW, OPEN, SAVE) is refused
   with `NAME is not available to agents.` while `Session.SetAgentCaller(true)` is on, by name or alias,
   and an agent cannot answer the prompt of one a human started. `cadmcp` sets the flag around every
   tool body; see [Agent integration](agent-integration.md).

## Input pipeline

```
pointer (window px) → ui: canvas-local px → View.ToModel → model point
  → Session.Hover: object snap (if on and the prompt takes a point, distance or angle)
                   else ortho (if on and the prompt has a Base)   → Session.Cursor
  → Session.Click: point prompt → the cursor as a point
                   distance / angle prompt with a Base → measured from the Base
                   selection prompt or idle → pick (Shift removes)
```

Object snap wins over ortho. Typed coordinates are never snapped or constrained.

## Object snaps

`cmdsys.FindSnap` gathers the entities whose boxes lie within the aperture (`Session.Aperture`
logical pixels, converted to model units by the view; at most 24 entities), decomposes their curves,
and considers:

| Mode | Points |
|---|---|
| Endpoint | Line and arc ends, polyline vertices |
| Midpoint | Line, arc and polyline-segment middles |
| Center | Circle and arc centres |
| Quadrant | The 0°, 90°, 180° and 270° points of circles, and of arcs where they lie on the arc |
| Intersection | Every pairwise crossing of the candidate curves |
| Nearest | The closest point on any candidate, only when nothing else is in range |

The closest point within the aperture wins; on a tie the mode listed first wins. Entities on layers
that are off are ignored. `cmdsys.DefaultSnaps` enables every mode except Nearest; F3 toggles them
all (`Session.SnapOn`).

## Selection

`cmdsys.Pick` returns the entity whose curves come closest to the point within the aperture, the
newest on a tie. `cmdsys.SelectBox` selects by box: dragged left to right it is a **window**
(entities entirely inside); right to left it is a **crossing** (entities inside or touching, tested
on their tessellation). Entities on locked or off layers are not selectable. The selection set is
`cmdsys.Selection`; it drops handles that leave the document.

`Session.HistoryTotal` is the monotonic count of lines ever printed; `Session.History` keeps the last
200, so a caller that wants exactly the lines one operation produced (the agent's `run_script`) records
the total before and after.

## Rationale

**The session owns the prompt, not the UI.** The command line, scripts, tests and a future browser
shell all drive the same `Session` methods, so a command behaves identically under each.

## Related docs

- [Command line](../features/command-line.md) — what the user types and sees
- [Drawing and editing](../features/drawing-and-editing.md) — the commands themselves
- [Document model](document-model.md) — the transaction contract commands rely on
