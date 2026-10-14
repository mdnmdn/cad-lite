---
id: tech-command-system
title: Command system
kind: tech
status: current
summary: The command model, the session state machine, typed-input parsing, the input pipeline from pointer to point, object snaps, ortho and selection.
read_when: you are writing a command, adding an input form or a snap mode, or changing how clicks and keys reach commands
updated: 2026-10-15
verified: 2026-10-15
code_anchors: [cmdsys/inflight.go, cmdsys/polar.go, cmdsys/command.go, cmdsys/context.go, cmdsys/registry.go, cmdsys/selection.go, cmdsys/session.go, cmdsys/cursor.go, cmdsys/input.go, cmdsys/snap.go, cmdsys/override.go, cmdsys/aids.go, cmdsys/vars.go, cmdsys/sysvars.go, cmdsys/textin.go, cmdsys/names.go, cmdsys/script.go, cmdsys/lastmem.go, cmdsys/selmem.go, cmdsys/select.go, cmdsys/selectopts.go, cmdsys/transparent.go, cmdsys/format.go, cmdsys/dirty.go, cmdsys/wording.go, cmdsys/commands/register.go, cmdsys/commands/base.go, cmdsys/commands/text.go]
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

**Work in progress** (`inflight.go`). What a command adds, changes or erases before it ends is real
document state at once, as in AutoCAD (each LINE segment is an entity the moment it is placed). The
drawing publishes a change set only at the commit, so after every `Start` and `Input` the session takes
the transaction's new edits (`doc.Txn.TakeEdits`), applies them to the spatial index, prunes the
selection, and queues them for the host's display cache: `Session.TakeLiveEdits` returns and forgets
them, and the canvas drops their cached items before drawing. Snaps, picks and the canvas therefore see
the running command's work; nothing is journalled or published early, so the command is still one undo
step and the commit's change set still reaches every listener. A command's `Preview` draws only what is
not an entity yet (the rubber band, PLINE's polyline in progress, TEXT's next-line box).
Every commit and abort of a command's transaction goes through the session (`commitOpen`, `abortOpen`;
`Context.CommitStep` too), which syncs first: an entity indexed in one step and erased in the step that
commits is absent from the commit's change set (one transaction added and erased it), so only the sync
removes it from the index. The commit's change set is then not re-applied to the index, which by then
holds it, and the live-edit queue keeps only what the change set cannot report (an entity added and
erased), for the host's cache.

**Prompt.** `cmdsys.Prompt` has the message, the set of accepted input kinds, keywords, an optional
default, and an optional `Base` point. `Prompt.StringIn` renders AutoCAD's form:
`Specify next point or [Close/Undo] <default>:`, in the language of the translator it is given
(`Prompt.String` uses `i18n.Current()`). A keyword is its English id (`eXit`); the shortcut, the
label shown and the matching come from the `i18n` catalog, see Language below. `Prompt.NoOr` drops the
`or` before the keywords, which AutoCAD omits in its `Enter a ... option [A/B] <A>:` prompts.
`Prompt.Literal` marks a text-entry prompt (TEXT and MTEXT content) whose answer may hold spaces: the UI
takes a space as a character at any prompt that accepts text, a script ends a Literal answer only at a newline, and
`ParseInput` keeps its leading and trailing spaces (an all-blank answer is still Enter).
`Prompt.Pick` marks a point prompt that selects an object by the click (AREA's `Select object:`, MEASUREGEOM's
Radius and Angle object prompts): the cursor is the raw pointer, with no object snap, grid snap, ortho or polar,
and the canvas shows the pickbox, as AutoCAD's object selection does.

**Input kinds.** Point, distance, angle (radians internally, degrees typed), keyword, selection,
text, integer and Enter. `cmdsys.Input` carries the response.

**File and history state** (`cmdsys/dirty.go`). Each session counts the changes published by its drawing's
change listener (a committed transaction, an undo, a redo): `Session.Modified`, `Dirty`, `MarkSaved`, and the
read-only variable `DBMOD`. `Session.MarkModified` increments that counter once when the host actually
changes drawing metadata outside the entity journal, such as a layer property. It sets Dirty and DBMOD
without adding an undo step; the host calls it only after a change, never for a cancelled or unchanged
edit. The counter is reset when the drawing is replaced (`SetDrawing`) and by a save.
The host reads `Dirty` for its title, status and restart decisions and installs `SetQuitHandler` for QUIT and
EXIT. A host that keeps several drawings, one session each, installs a `DocumentHost` per session
(`SetDocumentHost`, read by commands through `Context.Documents`): OPEN asks it to show a tab that has the
file (`ShowOpen`) before reading and to open the drawing it read in a new one (`OpenNew`, which returns
that tab's session, or nil to replace this drawing), NEW asks `New`, and CLOSE asks `Close` once the
unsaved changes are dealt with. `Context.History()` holds what UNDO sets (Control, Auto) and the sizes of the latest undo operations, so
REDO reverses a whole `UNDO n`. When a command ends the session names the undo step after it
(`Drawing.NameLastStep`), so `U` can print the command it undid.

**Registry.** `cmdsys.Registry` maps upper-case names and aliases to command factories.
`Registry.LoadAliases` reads `ALIAS,*COMMAND` lines in the `acad.pgp` format. `Registry.AliasesOf` lists the aliases of one command, shortest first; the toolbar tooltips read their keyboard hint from it.

## Rules

1. **One command, one transaction, one undo step.** The session commits the command's transaction
   when `Input` returns nil, and also on cancel, so what a command committed before Escape stays (as
   AutoCAD keeps the segments of a cancelled LINE). Its edits are visible before the commit (Work in
   progress, above).
2. **Starting a command cancels the running one.** Replacing the drawing (OPEN, NEW) from inside a
   command does not print a cancel.
3. **Enter at `Command:` repeats the last command.** An unknown name prints
   `Unknown command "NAME".  Press F1 for more help.` and starts nothing. At `Command:` the name prefixes
   `'`, `_` and `.` are stripped in any order (`_.LINE`, `'_ZOOM`; `cmdsys/names.go`); a leading `-` is part
   of the name (`-LAYER` is its own command, see below), and at a prompt only a leading `'` means
   something (a transparent command).
   `Session.lookupCommand` resolves a name in this order: the registry (aliases first), then
   `-NAME` through the aliases of `NAME` (so `-LIVELLO` runs `-LAYER`), then a system variable typed by
   name, which runs as a one-prompt SETVAR. Commands win over variables.
4. **Typed input** is parsed by `cmdsys.ParseInputIn` (`ParseInput` for the current language), in this order:
   - empty → Enter if accepted, else the default, else an error;
   - a keyword or its shortcut, case-insensitive, in the session's language or English; a leading
     `_` forces English;
   - a point: `x,y`, `@dx,dy`, `d<angle`, `@d<angle`, `#x,y` and `*x,y` (absolute: a plain `x,y`
     is, so the prefix is stripped; spec V4); `@` forms measure from the prompt's `Base`, or from
     the last point entered, which starts at the origin (so `@1,1` is valid before any point). `@` alone
     and `<angle` are text stages (below), not part of `ParseInput`;
   - a bare number: a distance, an angle in degrees, or an integer when the prompt takes one;
     otherwise, at a point prompt with a base, a point that distance along the cursor direction
     (direct distance entry);
   - free text, when accepted.
   A parse error is printed to the history and the prompt stays. Numbers must be finite: `nan`,
   `inf` and values that overflow (`1e999`) are rejected as an invalid value, and a point whose result overflows
   to infinity as an invalid point.
5. **Scripts** follow AutoCAD script rules (`runScriptText`): `Session.Script` drops `;` comments to the end of the line
   (`StripScriptComments`; a comment-only line vanishes, an empty line stays), treats a tab as a space, drops
   exactly one final newline (the file's terminator: a script that ends after its last answer sends no Enter, one that
   ends with a blank line sends one), splits on single spaces and newlines and submits each token; an empty token (two
   separators in a row, a trailing space) is Enter. At a `Prompt.Literal` prompt only a newline ends the answer, so
   `TEXT` content keeps its spaces (V45). Scripts queue: the `SCRIPT` command hands the file's text to the
   session (`Context.RunScript`), which runs it once that command ended and abandons the script that called it
   (V32); `RSCRIPT` (`Context.RestartScript`) queues the running text again, at most 1000 times in a row. Nothing
   aborts a script on an error, as in AutoCAD. `DELAY` validates its argument and does not wait; `RESUME` does
   not exist, because Esc cannot interrupt a synchronous script.
6. **A selection prompt answers with the selection set** when the user presses Enter. Modify commands
   take a non-empty pre-selection without prompting (noun-verb editing) and clear the selection.

7. **A panic in a command does not end the session.** `Session.guard` wraps the command's `Start`,
   `Input`, `Cancel` and `Preview`. On a panic it aborts the open transaction (`doc.Drawing.AbortOpen`),
   ends the command and its prompt, and prints `Error: NAME failed: ...` (key `session.internal_error`); the session accepts the next
   command.
8. **Human-only commands.** A command that implements `cmdsys.HumanOnly` (NEW, OPEN, SAVE) is refused
   with `NAME is not available to agents.` while `Session.SetAgentCaller(true)` is on, by name or alias,
   and an agent cannot answer the prompt of one a human started. `cadmcp` sets the flag around every
   tool body; see [Agent integration](agent-integration.md).
9. **Enter ends a required prompt.** Enter at a prompt that accepts no Enter and has no default ends the
   command quietly, keeping what it committed (no `*Cancel*`, no error loop; spec G5-1).

## Session extension points

The seam split gives each later change its own file instead of one shared function. Each point has a
default that changes nothing.

- **System variables** (`vars.go`). `Session.Vars()` and `Context.Vars()` return the session's `Vars`
  registry: `Register(Var{Name, Kind, Default, Min, Max, ReadOnly, Read, OnChange})`, typed getters
  `Int/Real/Text/Point`, `Set` (typed value) and `SetString` (parsed text, for SETVAR) returning
  `ErrVarUnknown`, `ErrVarReadOnly`, `ErrVarType` or `ErrVarRange`, `Store` (the owner of a read-only
  variable), `Names`, `String`, `Subscribe` and `Session.OnVarChange`. `Register` panics on a duplicate
  name. A package registers its variables from an `init` in its own file with `RegisterVarGroup`;
  `NewSession` runs the groups. `sysvars.go` registers the live ones (`ORTHOMODE`, `APERTURE`,
  `LASTPOINT`, `CLAYER`, which read and write session state) and `LUPREC`, `AUPREC`; `commands/units.go` registers `LUNITS`, `AUNITS`, `ANGBASE`, `ANGDIR`, `UNITMODE` and `INSUNITS` (live on the drawing header); `commands/construct.go`
  registers `OFFSETDIST`, `FILLETRAD`, `CHAMFERA`, `CHAMFERB`, `CHAMFERD`, `OFFSETERASE`, `OFFSETLAYER`, so the construct defaults are per session (`TRIMMODE`, `CHAMFERC`, `CHAMMODE` are the stored placeholders the session registers).
  After every group ran, `registerStoredVars` adds what section 1.11 of the AutoCAD specification lists and
  nobody registered: **stored placeholders** that read back what was set (`MIRRTEXT`, `FILEDIA`, `DIMSCALE`
  and also names a package will own, such as `OSMODE`, `CIRCLERAD`, `CECOLOR`), and live read-only state
  (`VIEWCTR`, `VIEWSIZE`, `SCREENSIZE`, `LASTPROMPT`, `CMDNAMES`, `CMDACTIVE`, `DWGNAME`, `DWGPREFIX`). A package that
  owns a variable just registers it: the placeholder steps aside whatever the init order. Angle variables
  are not placeholders (their unit belongs to the units package).
- **SETVAR** (`sysvars.go`, registered by `commands/sysvar.go`). `cmdsys.NewSetvar` asks for a name (`?` lists
  by pattern) and then for the value; a variable typed by name at `Command:` runs the same value prompt
  without the name step. The value arrives as text and goes through `Vars.SetString`; `VarErrorKey` maps the four
  errors to the AutoCAD sentences. Both are transparent (`'SETVAR`, `'ORTHOMODE`).
- **Command-line versions** (`names.go`). `RegisterCLI(r, "LAYER", factory, "LA")` registers `-LAYER` and `-LA`;
  the factory's command is a separate command whose `Name` is `-LAYER`. A package adds its `-NAME` from its own
  `registerX`.
- **Text stages** (`textin.go`). `Session.AddTextFilter(order, f)` rewrites the line and
  `AddTextHandler(order, f)` may consume it; `Submit` runs them in ascending `TextOrder*` order before
  anything else. The stages: name prefixes 10, transparent commands 15 (`transparentStage`), typed selection
  options 20, snaps 30, `#` 31, the point forms 35 (`angleOverrideStage` consumes `<angle`) and 36
  (`pointFormsFilter` turns `@` alone into the last point and a bare distance after `<angle` into
  `@d<angle`).
- **Last state** (`lastmem.go`). `Context.LastPoint`/`SetLastPoint` (what `@` measures from; the session
  records every point answered), `LastEntity` (set when a command's transaction commits) and
  `LastEnd`/`SetLastEnd` (end and tangent of the last line or arc, set by the draw commands).
- **Previous selection** (`selmem.go`). `Context.PreviousSelection`/`SetPrevious`; `selecting.take` records
  each non-empty selection, and erased handles are pruned.
- **Typed selection options** (`select.go`, `selectopts.go`). A text handler at `TextOrderSelection`,
  installed per session by a var group, reads the options of a selection prompt (a prompt that accepts
  a selection and no point, and whose own keywords do not match). Operations add to or remove from
  `Context.Selection`, so Enter still answers with the set. W, C, BOX, WP, CP and F swap the active
  command for a `selSub` until the shape is complete: it presents a point prompt, so clicks and typed
  points reach it, and hands back to the command's own selection prompt. The prompt pointer identifies
  the prompt; Remove mode, SI and the U history reset when it changes. The geometry (`PointInPolygon`,
  `PathsInsidePolygon`, `PathsCrossPolygon`, `PathsCrossFence`) is pure.
- **Transparent commands** (`transparent.go`). `cmdsys.Transparent` and `IsTransparent`; ZOOM, LAYER, DIST,
  ID and SETVAR implement it. `'NAME` at a prompt (not a text prompt) parks the interrupted command, its
  prompt and its open transaction in a frame, runs NAME with `>>` before its prompts (`>>>>` one level
  deeper), and `finish` restores the frame when NAME ends, so the interrupted prompt is shown again unchanged; a
  command that is not transparent prints `NAME command not allowed transparently.` (V2). Escape cancels every
  level and then the outermost command. `Session.TransparentDepth` reports the nesting; `CMDNAMES` joins the
  stack with `'`. A transparent command must not open a transaction (it would commit as a step of its own). The
  repeat command (`Enter` at `Command:`) is not changed by a transparent one.
- **Number formatting** (`format.go`). `Context.Fmt` prints a length in the `LUNITS` format with `LUPREC`
  decimals (decimal keeps trailing zeros: `5.0000`; scientific, engineering and architectural in inches,
  fractional); `FmtAngle` prints a direction measured from `ANGBASE` in the `ANGDIR` sense and `FmtAngleRel` an
  angle between two things, both in the `AUNITS` format with `AUPREC` places (default degrees, no decimals).
  `FormatLength` and `FormatAngle` are the pure functions. `ANGBASE` is stored in degrees. A typed angle at an
  angle prompt goes through `angleInputFilter` (`input.go`, text order `TextOrderPointForms+2`), which reads
  `ParseAngleText` and applies `ANGBASE` and `ANGDIR`; with the defaults a plain number is untouched.
- **Wording** (`wording.go`). Catalog-key constants for the session's own messages and `InputErrorKey`,
  the one place that maps a parse error and a prompt to its sentence: a point prompt with keywords says
  `Point or option keyword required.`, a distance prompt `Requires numeric distance or second point.`, an angle prompt
  `Requires numeric angle or second point.`, an integer prompt `Requires an integer value.`, a keyword-only prompt
  `Invalid option keyword.`, anything else the generic sentences (spec V9).

The command package mirrors this split: one file per command group, each with a `registerX` function that
`commands.Register` calls, a frozen `aliases.go` (a package registers its own aliases with `Registry.Alias`
in its `registerX`) and `base.go` for the shared helpers. The table of which package owns which file is in
the AutoCAD command specification's Wave 0 result (`_docs/inbox/autocad-commands.md`).

## Language

Every line a session prints, every prompt and every keyword match goes through one translator.
`Session.Translator()` is English while `SetAgentCaller(true)` is on, else the translator pinned with
`Session.SetTranslator`, else `i18n.Current()`, so a language change in the settings applies to the
next line printed. Catalogs and the language choice are owned by `i18n` and the
[settings](../features/settings.md) feature.

- **Commands build text from whole sentences.** A command calls `Context.T`, `Context.Say` or
  `Context.SayPlural` with a key (`cmd.<name>.prompt.*`, `msg.*`), never assembling a sentence from
  fragments, so word order stays the catalog's. `Context.Cmd` gives a command's display name.
  `Session.Say` prints a catalog line the same way for the UI, when no command is running to say it
  (a dropped file Cad-lite cannot open).
- **Keywords keep their English id** in `Prompt.Keywords` and in `Input.Keyword`; the display
  (`i18n.PromptKeywords`) and the matching (`i18n.MatchKeywords`) are localized. English always
  matches, whatever the language. `i18n.KeywordGroups` includes TEXT's insertion, justification and
  style prompts and MTEXT's corner, attachment and style prompts, so `i18n.Verify` checks their
  localized labels and shortcuts for collisions. Tests compare those groups with the commands'
  actual prompts.
- **Names.** `Register` ends with `i18n.RegisterAliases`, so the Italian command names are aliases in
  every language; canonical names stay English. `SnapMode.String` is localized (`SnapMode.In` takes
  a translator).
- **Parse errors** map to `input.err.*` keys through `InputErrorKey` in `Session.inputError`; SETVAR's
  sentences are `session.setvar.*`.

## Input pipeline

```
pointer (window px) → ui: canvas-local px → View.ToModel → model point
  → Session.Hover: at a Pick prompt the raw point, nothing else; otherwise
                   pending typed override (override.go), else running object snap (if on),
                   else the snap grid (SNAPMODE), then ortho, or polar tracking, (if on and the prompt has a Base)   → Session.Cursor
  → Session.Click: point prompt → the cursor as a point
                   distance / angle prompt with a Base → measured from the Base
                   point prompt answers carry Input.Shift when Shift is held
                   selection prompt or idle → pick (Shift removes; plain click removes in Remove mode)
```

At a `Prompt.Pick` prompt a typed snap name is not an override: the text stage leaves it to the
prompt, which rejects it. Object snap wins over ortho and polar tracking. Typed coordinates are never snapped or constrained. Polar
tracking (`polar.go`, `Session.Polar`, `SetPolar`) locks the point onto the ray at the nearest multiple of `POLARANG`
within 5 degrees; ortho and polar tracking exclude each other (`SetOrtho` and `SetPolar` turn the other off). `polar.go`
also registers `POLARANG` (live, degrees, default 45), `AUTOSNAP` (live for bit 8, polar tracking; default 55, so off) and `LWDISPLAY`
(live, the lineweight display flag the status bar's LWT toggle sets).

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
| Insertion | The insertion point (`Anchor`) of TEXT and MTEXT (`doc.Annotation`); text offers no other snap point, because its strokes are glyphs |
| Perpendicular, Tangent | Feet and tangent points from the prompt's `Base` (`FindSnapFrom`); nothing without a base |
| Nearest | The closest point on any candidate, only when nothing else is in range |

The closest point within the aperture wins; on a tie the mode listed first wins. Entities on layers
that are off or frozen are ignored. `cmdsys.DefaultSnaps` enables every mode except Nearest; F3 toggles them
all (`Session.SnapOn`). `SnapMode` bits are AutoCAD's `OSMODE` values; the `OSMODE` variable (aids.go) is
`Snaps` plus 16384 when `SnapOn` is off, and `GRIDMODE`, `SNAPMODE`, `GRIDUNIT`, `SNAPUNIT` read and write
the grid state (`Session.Grid`, `SnapGrid`, `GridSpacing`, `SnapSpacing`).

**Typed overrides** (`override.go`, text stage `TextOrderSnap`). `cmdsys.FindTyped` resolves one: the objects within
the aperture of the pick are candidates, and the nearest listed snap point on them wins wherever it lies
(`INT` needs both curves within the aperture). A name arms an `overrideState` on the session and swaps in a
substitute prompt (`MID of:`, `Base point:`, `<Offset>:`); the next click or typed point (read as the pick
position) is resolved, FROM and M2P chain, and the command's own prompt is restored before the answer reaches
it. State whose substitute prompt has been replaced (Escape, the command moved on) is dropped.

## Selection

`cmdsys.Pick` returns the entity whose curves come closest to the point within the aperture, the
newest on a tie; a point inside a text's box (`doc.Annotation.HitBox`) picks the text, ranked as a
stroke at the edge of the aperture, so a curve nearer the point (a line across the text) wins. `cmdsys.SelectBox` selects by box: dragged left to right it is a **window**
(entities entirely inside); right to left it is a **crossing** (entities inside or touching, tested
on their tessellation). A text counts by its text box rather than its glyph strokes (`selectBounds`,
`touchesBox`; for WP, CP and F `entityPaths` and `inTextBox`): a window must hold the whole box, and a
crossing window, polygon or fence that meets the box selects the text, even between two letters or
inside an O. Entities on locked, off or frozen layers are not selectable (`doc.Layer.Editable`). The selection set is
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
