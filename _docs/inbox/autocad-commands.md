# AutoCAD command compatibility specification

Raw planning material (inbox). Written 2026-10-14 against the working tree of that day, which held
other agents' uncommitted edits (settings, review fixes, i18n). Nothing here is implemented; no
code or other document was touched to produce it. Its acceptance corpus is
`cmdsys/commands/testdata/acad/` (scripts, expected states, a runner test): 40 scripts, each named
below as *corpus NN*.

**What this is.** The exact command-line behaviour of AutoCAD 2024/2025 (English, `acadiso.dwt`
defaults) that go-draw must reproduce so that a drafter's typed commands, an AutoCAD `.scr` script
and muscle memory all work: command names, prompt sequences, options, defaults, system variables,
messages. It **complements** [`autocad-parity.md`](autocad-parity.md), which owns keys, mouse,
grips, drafting-aid pipeline and chrome; this document owns the *language* of the command line and
the per-command option sets, and it feeds that plan's **WP5** (commands), **WP2** (selection
prompt options) and **WP3** (typed overrides, `#`, `<angle`). Plan ids (`WP5`, `D-C`, `G7`, section
numbers) are referenced, not repeated. The parity plan's Wave 0 "seam" split is assumed (section 4).

**Legend.** Priority: **P1** a drafter hits it in the first hour, it breaks muscle memory, or it
breaks AutoCAD scripts; **P2** expected within the first week; **P3** polish or niche. Effort:
**S** under a day, **M** a few days, **L** a week or more. *(verify Vn)* marks an AutoCAD fact I am
not certain of; every Vn is in the checklist at the end. Facts are from memory of the 2024/2025
documentation and behaviour, not from an install.

**Scope.** 2D model space, WCS only (no UCS, no layouts, no 3D). Entity types today: LINE, CIRCLE,
ARC, LWPOLYLINE. ELLIPSE, POINT, SPLINE, TEXT, HATCH, blocks and dimensions need new entity types
(backlog G7) and are listed but not specified in depth.

---------------------------------------------------------------------------------------------------

## 1. Global compatibility rules

Each rule ends with the go-draw gap, as `Gn-m` rows in the table at the end of the section.

### 1.1 Command-name resolution

Typed at `Command:` (or fed by a script). Case-insensitive. Resolution order AutoCAD uses:

| Form | Meaning | Example |
|---|---|---|
| `NAME` | Alias table (`acad.pgp`), then the command table; a command removed with UNDEFINE is not reachable | `L`, `LINE`, `CO` |
| `_NAME` | The global (English) name: the same command in any language | `_LINE`, `_circle` |
| `.NAME` | The built-in command, even when a name has been redefined or undefined | `.ERASE` |
| `_.NAME` | Both; also `._NAME` *(verify V1)* | `_.LINE` |
| `-NAME` | The command-line version of a dialog command; its own command with its own prompts | `-LAYER`, `-INSERT`, `-OSNAP`, `-ARRAY`, `-UNITS`, `-PURGE`, `-PAN` |
| `'NAME` | Transparent: runs at a prompt of another command (section 1.2) | `'ZOOM`, `'PAN` |
| `*NAME` in `acad.pgp` | The alias file's own marker (`L, *LINE`); never typed | |
| `MULTIPLE NAME` | Repeats NAME until Esc; the name is run again after each completion | `MULTIPLE CIRCLE` |

Prefix stripping order: `'`, `_`, `.`, then a leading `-` is part of the command name (`-LAYER` is
registered as its own command, aliased `-LA`). `'_ZOOM` and `_'ZOOM` both work *(verify V1)*.
Names resolve against aliases first, so a user alias may shadow a command.

Unknown name: `Unknown command "FOO".  Press F1 for more help.` (two spaces; *verify V10*) and no
command starts. Names are never abbreviated automatically: `LIN` is unknown (the suggestion list
of the parity plan, WP1, is a UI aid).

**Alias file.** `acad.pgp` lines are `ALIAS, *COMMAND`; go-draw's `Registry.LoadAliases` reads the
same format (user `aliases.pgp` override is P3 in the parity plan 7.1). Italian aliases arrive
through `i18n.RegisterAliases` and are *added*, never replacing English ones; **command names stay
English** and `_NAME` always means the English name.

### 1.2 Transparent commands (`'`)

A transparent command typed with a leading `'` at **any prompt except** a text-entry prompt and
selection (`Select objects:` accepts it) runs, changes only view or settings, then returns to the
interrupted prompt unchanged (same Base, same defaults). While it runs the prompt shows `>>`
(`>>>>` for a second level) *(verify V44)*. A command that is not transparent typed with `'` prints
an error naming the command and the prompt stays (exact text *verify V2*).

Transparent (view and settings), required in go-draw: **ZOOM, PAN, REDRAW, SETVAR, OSNAP, ORTHO,
GRID, SNAP, DSETTINGS, DIST, ID, LAYER/-LAYER, COLOR, LINETYPE, LWEIGHT, LTSCALE, UNITS, HELP,
TEXTSCR/GRAPHSCR, CAL** and, with *(verify V2)*, REGEN, MEASUREGEOM, VIEW. A transparent command
must not open a transaction. ZOOM and PAN have a transparent-only restriction: in a transparent
ZOOM the options `All`, `Extents` and `Previous` regenerate and are refused if the drawing needs it
(not an issue in go-draw).
Never transparent: anything that creates or edits objects, UNDO/U/REDO, selection commands, file
commands, SCRIPT.

### 1.3 Prompt format

`<Message> [or [Opt1/Opt2/...]] [<default>]:` rendered as
`Specify next point or [Close/Undo]:`. Rules:

- The message is a sentence fragment starting with a capital, no trailing space, ending `:`.
- Keywords are written in the brackets separated by `/`; the capital letters of a keyword are its
  shortcut: `[Undo/Width]` → `U`, `W`; `cuTting edges` → `T`; `ARea` → `AR` (so `A` alone is the
  prompt's other `A…` keyword). A keyword with no capitals uses its first letter. Inside one prompt
  no two shortcuts may collide.
- The default is in angle brackets: `<0.0000>`, `<Extents>`, `<Exit>`, `<N>`. A *number* default is
  printed with the unit precision (`LUPREC` 4 decimals, **trailing zeros kept**: `<10.0000>`); an
  *angle* with `AUPREC`; a *keyword* default by its shortcut for Yes/No (`<N>`), by its word
  otherwise (`<Exit>`, `<Through>`).
- Options that need a sub-prompt print the sub-prompt as a new line of the same command; Enter at it
  usually accepts its own `<default>`.
- The command echo is `Command: LINE` (toolbar clicks echo `Command: _line`).
- Selection prompts print `Select objects:`; after input, `1 found`, `3 found, 4 total`
  *(verify V8)*.
- After a transparent command the interrupted prompt is shown again.

go-draw's `Prompt.String` already renders this form; gaps are the default formatting
(`<10>` vs `<10.0000>`) and Yes/No defaults (`<No>` vs `<N>`).

### 1.4 Keyword input

- Typed text is matched, case-insensitively, against the keywords of the *current prompt only*, as
  the full word (`Close`), or the capital-letter shortcut (`C`). A typed prefix that is neither is
  *not* accepted (`Clo` is invalid).
- `_X` is the **English** keyword or shortcut whatever the UI language: `_Close`, `_C`. Required so
  a script written in English runs in an Italian session (see `i18n/keyword.go`, which already
  strips `_` and `'`).
- Keywords win over everything else at a prompt that accepts both a point and a keyword; a point
  always contains a digit, `,`, `<`, `@` or `#`, so there is no real ambiguity, except layer or
  block names at text prompts (text prompts take no keywords unless the prompt shows them).
- Keyword prompts that offer a point or a number too take either (`Specify radius of circle or
  [Diameter]`): a bare number is the radius, `D` the keyword.
- An unrecognised input at a keyword-only prompt: `Invalid option keyword.`; at a point prompt with
  keywords: `Point or option keyword required.`; at a point prompt without: `Invalid point.`;
  at a distance prompt: `Requires numeric distance or second point.`; at an angle prompt:
  `Requires numeric angle or second point.` *(verify V9)*. The prompt is shown again.

### 1.5 Enter, Space and right-click, per prompt type

Space equals Enter everywhere except inside text being entered (TEXT/MTEXT content, a filename or
a layer name with embedded spaces at text prompts that say so) *(verify V45)*. Right-click is Enter
when `SHORTCUTMENU` has no menu for the state, otherwise it opens a menu whose Enter item is the
same (parity plan 2.2).

| Prompt | Enter / Space |
|---|---|
| `Command:` | Repeats the previous command (`MULTIPLE` aside). Previous = the last command *started*, not a transparent one |
| Point with a `<default>` or `<use first point as displacement>` | Accepts the default |
| "Next point" prompts of LINE, PLINE, COPY (second point), MOVE (second point: displacement form), DIST multiple, AREA | Ends the command (or the repeat loop) |
| Required point with no default (`Specify center point for circle`) | Ends the command silently *(verify V3)* |
| `Specify first point:` of LINE, `Specify start point of arc:` | Continues from the last point (section 1.7) |
| Distance / angle with `<default>` | Accepts it; without: ends the command |
| Keyword with `<default>` | Accepts the default |
| `Select objects:` | Ends selection (zero objects: the command ends without change, no message) |
| Text | Ends the text entry |

### 1.6 Defaults remembered between commands

Two kinds: **system variables** (named, saved in the drawing or session, settable with SETVAR or
by typing the name) and **command memory** (kept for the session, not a variable). go-draw today
keeps both as package-level variables of `cmdsys/commands/construct.go` (`filletRadius`,
`chamferD1/D2`, `lastOffset`), **shared by every session in the process**, and some inside the
Session (`lastPoint`). Target: all of them are session-owned variables in the Wave 0 `Vars`
registry (parity plan 4.0.3).

| Variable | Type, default | Written by | Read by |
|---|---|---|---|
| `LASTPOINT` | point, 0,0,0 | every point the user enters (typed or picked), `ID` *(verify V46)* | `@`, relative input, LINE Enter-continue |
| `CIRCLERAD` | real, 0 | CIRCLE (radius used) *(verify V13)*, SETVAR | CIRCLE radius default `<r>` (0 = none) |
| `FILLETRAD` | real, 0 | FILLET `Radius`, SETVAR | FILLET; also RECTANG `Fillet`? no (RECTANG keeps its own memory) |
| `CHAMFERA`, `CHAMFERB` | real, 0 | CHAMFER `Distance` | CHAMFER (method distance) |
| `CHAMFERC`, `CHAMFERD` | real, 0 / angle | CHAMFER `Angle` | CHAMFER (method angle) |
| `CHAMMODE` | int, 0 (0 distance, 1 length+angle) | CHAMFER `Method` | CHAMFER |
| `TRIMMODE` | int, 1 (1 trim, 0 no trim) | FILLET/CHAMFER `Trim` | FILLET, CHAMFER |
| `TRIMEXTENDMODE` | int, 1 (0 Standard, 1 Quick) *(verify V11)* | TRIM/EXTEND `mOde` | TRIM, EXTEND |
| `EDGEMODE`, `PROJMODE` | int, 0 / 1 | TRIM/EXTEND `Edge`, `Project` | TRIM, EXTEND |
| `OFFSETDIST` | real, -1 (negative = Through) | OFFSET distance / `Through` | OFFSET default |
| `OFFSETGAPTYPE` | int, 0 (extend, fillet, chamfer) | SETVAR | OFFSET closed polyline gaps |
| `POLYSIDES` | int, 4 | POLYGON | POLYGON |
| `PLINEWID`, `PLINEGEN` | real 0, int 0 | SETVAR (the W option does not set it *(verify V16)*) | PLINE |
| `COPYMODE` | int, 0 (0 repeat, 1 single) | COPY `mOde` | COPY |
| `ANGBASE`, `ANGDIR` | angle 0, int 0 (0 ccw, 1 cw) | UNITS | all angle input and output |
| `ELEVATION`, `THICKNESS` | real 0 | | LINE/CIRCLE... (2D: ignore, keep readable) |
| `DISTANCE`, `AREA`, `PERIMETER` | real, read-only | DIST, AREA/MEASUREGEOM | scripts, `CAL` |
| `CLAYER`, `CECOLOR`, `CELTYPE`, `CELWEIGHT`, `LTSCALE` | text/int | LAYER, COLOR, LINETYPE, LWEIGHT | new objects |
| `OSMODE`, `AUTOSNAP`, `APERTURE`, `PICKBOX`, `ORTHOMODE`, `GRIDMODE`, `GRIDUNIT`, `SNAPMODE`, `SNAPUNIT`, `SNAPANG`, `POLARMODE`, `POLARANG` | see 1.11 | ORTHO, GRID, SNAP, -OSNAP, F-keys | cursor pipeline |
| `PICKFIRST`, `PICKADD`, `PICKAUTO`, `PICKDRAG`, `HIGHLIGHT`, `SELECTIONPREVIEW` | see 1.11 | | selection |
| `VIEWCTR`, `VIEWSIZE`, `SCREENSIZE` | read-only | ZOOM, PAN | scripts |
| `LUNITS`, `LUPREC`, `AUNITS`, `AUPREC`, `INSUNITS` | see UNITS | UNITS | all printing |
| `CMDECHO`, `ERRNO`, `LASTPROMPT`, `CMDNAMES` | 1 / 0 / text / text | | scripts, host |

### 1.7 Point input forms

Typed at any point prompt; none is ever snapped or constrained by ortho, polar or running osnaps
(only a *pick* is). Relative forms measure from the previous point of the command (the prompt's
`Base`), else from `LASTPOINT`.

| Form | Meaning |
|---|---|
| `x,y` | Absolute WCS |
| `x,y,z` | Absolute with Z (2D: Z ignored, kept in LASTPOINT as 0) |
| `@dx,dy` | Relative cartesian |
| `d<a` | Absolute polar: distance d at angle a from the origin (angle in the current `AUNITS`, `ANGBASE`, `ANGDIR`) |
| `@d<a` | Relative polar |
| `@` | The last point (`LASTPOINT`) |
| `#x,y` | Absolute, overriding the relative default of dynamic input; with `DYNMODE` 0 a plain `x,y` is already absolute and `#` is accepted *(verify V4)* |
| `*x,y` | Absolute in WCS when a UCS is active (identical here; accept and strip) |
| `<a` | **Angle override**: locks the direction to angle a for the next point; then a bare number is a distance along it, or a pick is projected onto it *(verify V5)* |
| `d` (a bare number) | **Direct distance**: that far from the base along the cursor direction (and honouring ortho/polar). In a script the cursor direction is undefined, so scripts must not use it; use `@d<a` |
| `FROM`, `M2P`, `MID`, `END`... | Typed object snaps, below |
| `.X`, `.XY`... | Point filters. P3, *not planned* |
| `x,y` with spaces | Never: a space ends the token (section 1.9) |

Missing or invalid pieces: `1,x` → `Invalid point.` and the prompt is asked again. `@1,1` with no
previous point is valid in AutoCAD (`LASTPOINT` starts at 0,0,0); go-draw prints an error
(`ErrNoBase`).

**Typed object snaps.** At a point prompt, a snap name (or a comma list) arms an override for the
*next pick only*; the prompt then asks the object (`of`, or the specific wording) and the next token
is the pick. Case-insensitive, three letters suffice.

| Name | Snap | Typed as | Pick rule |
|---|---|---|---|
| `END` | Endpoint | `END` + pick | The object endpoint nearest the pick, object must be within the aperture |
| `MID` | Midpoint | `MID` + pick | The pick only needs to touch the object |
| `CEN` | Center | `CEN` + pick | On the arc or circle |
| `GCE` | Geometric center | `GCE` | P3 |
| `NOD` | Node | `NOD` + pick | POINT entities; blocked by G7 |
| `QUA` | Quadrant | `QUA` + pick | The quadrant nearest the pick |
| `INT` | Intersection | `INT` + pick | Both objects must be within the aperture of the pick (type the exact crossing in a script) *(verify V31)* |
| `APP` | Apparent / extended intersection | `APP` then two picks | Two objects, even if they do not touch |
| `EXT` | Extension | `EXT` + pick near an end | Lies on the extension of a line/arc end |
| `INS` | Insertion | | Blocks/text; blocked by G7 |
| `PER` | Perpendicular | `PER` + pick | From the previous point to the object (deferred when it is the first point) |
| `TAN` | Tangent | `TAN` + pick | Tangent to a circle/arc from the previous point (deferred) |
| `NEA` | Nearest | `NEA` + pick | The nearest point on the object (the pick itself when it lies on it) |
| `PAR` | Parallel | `PAR` + pick + cursor | Needs a cursor direction; not scriptable |
| `FROM` | From | `FROM` then base point then `@dx,dy` | `Base point:` then `<Offset>:`; the result is the first point |
| `M2P` | Mid between 2 points | `M2P` then two points | `First point of mid:` `Second point of mid:` |
| `TT` | Temporary tracking point | `TT` + point | Starts tracking (needs the polar/otrack pipeline, parity WP3) |
| `NON` | None | `NON` + point | Ignores running snaps for this point |
| `QUI` | Quick | | P3 |
| `END,MID` | A list | `END,MID` + pick | The nearest of the listed |

Corpus 29 and 30 exercise END MID CEN QUA INT NEA PER FROM M2P NON.

### 1.8 Selection prompt options

At every `Select objects:` prompt (and inside `SELECT`) typed text is one of the options below, a
point (a pick), or Enter. A point pick adds the object that touches it; an empty pick starts
a window/crossing (`AUto`, below). Shortcuts are the capitals.

| Option | Prompt that follows | Effect |
|---|---|---|
| point | | The topmost object touching the pick (pickbox `PICKBOX`, default 3 px) is added; in Remove mode removed |
| `Window` `W` | `Specify first corner:` `Specify opposite corner:` | Objects entirely inside; the corners may be given in any order |
| `Crossing` `C` | same | Objects inside or crossing the rectangle |
| `BOX` | same | Window if the second corner is right of the first, Crossing otherwise |
| `ALL` | | Every object on thawed, unlocked layers *including layers that are off* *(verify V6)* |
| `Fence` `F` | `Specify first fence point:` `Specify next fence point or [Undo]:` Enter | Objects the polyline fence crosses |
| `WPolygon` `WP` | `First polygon point:` `Specify endpoint of line or [Undo]:` Enter | Objects entirely inside the polygon (Enter closes it) |
| `CPolygon` `CP` | same | Objects inside or crossing the polygon |
| `Group` `G` | `Enter group name:` | Needs groups; not planned |
| `Last` `L` | | The most recently *created* visible object (not the last edited) |
| `Previous` `P` | | The previous selection set: the last selection a command completed (including `SELECT`), valid until the objects are erased or the drawing is replaced *(verify V7)* |
| `Add` `A` | | Back to add mode after `Remove` |
| `Remove` `R` | prompt becomes `Remove objects:` | Picks and window options now remove; Shift+pick also removes |
| `Multiple` `M` | | Several picks processed without highlight, one search |
| `Undo` `U` | | Removes the most recent selection operation |
| `AUto` `AU` | | The default: a pick on an object selects it, a pick on empty space starts BOX |
| `SIngle` `SI` | | Ends the selection after the first successful selection (no Enter needed) |
| `SUbobject` `SU`, `OBject` `O` | | 3D / solids; not planned |
| Enter / Space / right-click | | Ends |

Feedback: after each operation `N found` (first) or `N found, M total`; Remove prints `N found, N
removed, M total` *(verify V8)*; objects already selected: `1 found, 1 duplicate`. Anything else
typed: `Invalid selection` newline `Expects a point or Window/Last/Crossing/BOX/ALL/Fence/WPolygon/
CPolygon/Group/Add/Remove/Multiple/Previous/Undo/AUto/SIngle` *(verify V9)*.

`PICKFIRST` 1: a command that takes a selection uses the existing set without prompting, then
clears it. **Typing `ALL`, `L`, `W`... in a script requires that no set exists**; scripts that
SELECT first and then run a command should use `P` (corpus 28).
Objects on locked layers can be selected but not edited; objects on frozen/off layers cannot be
picked (`ALL` differs, above).

go-draw today answers `Select objects:` only from clicks, box drags and Enter; typed text gets
`Invalid value.`. This is the single largest cause of script failure (corpus 15-18, 24-28 fail on
it): **G8-1, P1**.

### 1.9 Scripts and `.scr` semantics

A script is a text file; each response is fed to the command line exactly as typed.

- Tokens are separated by a space or a newline; **each separator is one Enter-terminated
  response**; **two separators in a row, or a blank line, send an Enter**. A trailing space on a
  line is therefore an extra Enter.
- `;` starts a comment to the end of the line (the whole line is ignored; text before `;` is
  still sent). *go-draw's `Session.Script` has no comment rule* (G9-1).
- The first token of a line is typically a command, but nothing distinguishes commands from
  responses: the script is a flat input stream. If a command ends earlier than the script expects,
  the remaining tokens run as commands at `Command:` and usually print `Unknown command`; AutoCAD
  does not abort the script on an error.
- Spaces end input at every prompt except a text being typed (TEXT, MTEXT, DTEXT content);
  a name with a space (layer, file) cannot be supplied *(verify V45)*. Tabs count as spaces
  *(verify V32)*.
- `SCRIPT` (alias `SCR` *verify V12*): `Enter script file name <default>:` with `FILEDIA` 0 (else a
  dialog); the file is run from the current search path or the given path. A script may call
  `SCRIPT`; the nested script runs to its end and the outer one does not resume *(verify V32)*. `RSCRIPT` restarts the current script, `DELAY n`
  pauses n ms (script only), `RESUME` continues a script after Esc interrupted it. Esc during
  a script cancels the running command and suspends the script.
- `FILEDIA` 0 and `CMDDIA` 0 make dialog commands prompt on the command line; scripts rely on
  them, plus `-` commands. A script must not leave a command running.
- A script's `Command:` echo is controlled by `CMDECHO` *(verify V43)*.
- Layers, linetypes and similar names in scripts are text and must not contain spaces.
- `(command ...)` AutoLISP is out of scope.

`Session.Script` today: splits on single spaces and newlines, empty token = Enter, no comments, a
final newline sends an extra Enter (corpus harness compensates, README). Target: `Session.Script`
implements the rules above (strip `;` comments, treat one final newline as a terminator), and the
`SCRIPT` command runs a file through it (needs `Context.Files` read, so it is a HumanOnly command).

### 1.10 System variables by name and SETVAR

- Typing a variable name at `Command:` prompts `Enter new value for NAME <current>:`; Enter keeps
  it. Typing the name with a value in a script is two tokens: `FILLETRAD` then `3`.
- `SETVAR` prompts `Enter variable name or [?] <last>:` then the same value prompt; `?` then a name
  or `*` lists variables as `NAME value` (read-only ones marked `(read only)`).
- Value prompts: integers `Enter new value for X <1>:`; reals show 4 decimals; points `x,y,z`; text
  in quotes in listings. Wrong type or range: `Requires an integer value.`,
  `Requires numeric value.`, `Requires an integer value between 0 and 1.` *(verify V28)*;
  unknown name: `Unknown variable name.` *(verify V28)*; read-only: `Variable is read-only.`
  *(verify V28)*.
- Variables take effect at once and a few feed back: `ORTHOMODE`, `OSMODE`, `GRIDMODE`... change
  the status bar toggles (the Wave 0 `Vars` registry `OnChange`).
- Some variables hold a bit set: `OSMODE` below.
- Only variables go-draw has a use for exist; an unknown valid AutoCAD name (`MIRRTEXT`, `DIMSCALE`)
  should be accepted as a **stored no-op** (read back what was set) so scripts that set them run
  (P2). The initial set is in 1.11.

### 1.11 System variables go-draw must provide

Defaults are `acadiso.dwt`. "Today" is where the state lives now.

| Variable | Type, default, range | Meaning | Today | Pri |
|---|---|---|---|---|
| `OSMODE` | int 4133 (END CEN INT EXT); bits END 1, MID 2, CEN 4, NOD 8, QUA 16, INT 32, INS 64, PER 128, TAN 256, NEA 512, QUI 1024, APP 2048, EXT 4096, PAR 8192, **16384 = running snaps suppressed** *(verify V29)* | Running object snaps | `Session.Snaps` (different bit layout: END 1, MID 2, CEN 4, QUA 8, INT 16, NEA 32) + `SnapOn` | P1 |
| `AUTOSNAP` | int 63 | Marker 1, tooltip 2, magnet 4, polar 8, otrack 16, tracking tooltip 32 *(verify, parity V2)* | none | P2 |
| `APERTURE`, `PICKBOX` | int 10 px, 3 px | Snap aperture, pickbox | `Session.Aperture` 8 px | P2 |
| `ORTHOMODE` | 0/1, 0 | Ortho | `Session.Ortho` | P1 |
| `POLARMODE`, `POLARANG`, `POLARADDANG` | int 0, angle 90 | Polar tracking | none | P1 (WP3) |
| `GRIDMODE`, `GRIDUNIT`, `GRIDMAJOR` | 1, 10,10, 5 | Grid | `Workspace.Grid` | P1 |
| `SNAPMODE`, `SNAPUNIT`, `SNAPANG`, `SNAPTYPE`, `SNAPSTYL` | 0, 10,10, 0 | Snap to grid | none | P1 |
| `DYNMODE` | int 3 | Dynamic input; scripts set 0 | none | P2 |
| `PICKFIRST`, `PICKADD`, `PICKAUTO`, `PICKDRAG`, `PICKSTYLE`, `HIGHLIGHT`, `SELECTIONPREVIEW` | 1, 2, 1, 0, 1, 1, 3 | Selection behaviour | pickfirst always on | P2 |
| `FILLETRAD`, `CHAMFERA/B/C/D`, `CHAMMODE`, `TRIMMODE`, `TRIMEXTENDMODE`, `EDGEMODE`, `PROJMODE`, `OFFSETDIST`, `OFFSETGAPTYPE`, `CIRCLERAD`, `POLYSIDES`, `PLINEWID`, `PLINEGEN`, `COPYMODE`, `ELEVATION`, `THICKNESS` | section 1.6 | Command defaults | package variables in `construct.go`, per process | P1 |
| `LASTPOINT` | point | Last point | `Session.lastPoint` | P1 |
| `DISTANCE`, `AREA`, `PERIMETER` | real, read-only | Results of inquiries | none | P1 |
| `VIEWCTR`, `VIEWSIZE`, `VIEWDIR`, `SCREENSIZE` | read-only | View | `render.View` | P2 |
| `CLAYER`, `CECOLOR`, `CELTYPE`, `CELWEIGHT`, `LTSCALE` | text, `0`, `BYLAYER`, `BYLAYER`, -1, 1 | Current properties | `Drawing.CurrentLayer` only | P1 (CLAYER) / P2 |
| `LUNITS`, `LUPREC`, `AUNITS`, `AUPREC`, `ANGBASE`, `ANGDIR`, `INSUNITS`, `UNITMODE` | 2, 4, 0, 0, 0, 0, 4 mm, 0 | Units and precision of everything printed | `Header.InsUnits` | P2 |
| `CMDECHO`, `ERRNO`, `LASTPROMPT`, `CMDNAMES`, `CMDACTIVE` | 1, 0, text, text, bits | Command line state | none | P2 |
| `FILEDIA`, `CMDDIA`, `EXPERT`, `NOMUTT`, `MIRRTEXT`, `UCSFOLLOW`... | various | Accepted, stored, no effect | none | P2 |
| `DBMOD`, `DWGNAME`, `DWGPREFIX`, `SAVENAME` | read-only | File state | `Session.FilePath` | P2 |

### 1.12 Messages and wording

A table for the Failure sections; every text is *(verify V9)*. The corpus checks only the ones
marked ✓ (script 40).

| Situation | AutoCAD | go-draw today |
|---|---|---|
| Unknown command | `Unknown command "FOO".  Press F1 for more help.` | `Unknown command "FOO". Press F1 for help.` |
| Bad point, no keywords | `Invalid point.` ✓ | `Invalid point.` |
| Bad point, keywords offered | `Point or option keyword required.` ✓ | `Invalid point.` |
| Bad number at distance prompt | `Requires numeric distance or second point.` | `Invalid value.` |
| Bad number at angle prompt | `Requires numeric angle or second point.` | `Invalid value.` |
| Zero or negative radius | `Requires a positive and nonzero value.` ✓ (substring `positive and nonzero`) | `Value must be positive and nonzero.` |
| Bad keyword (keyword-only) | `Invalid option keyword.` | `Invalid value.` |
| Selection garbage | `Invalid selection` / `Expects a point or Window/Last/...` | `Invalid value.` |
| After Esc | `*Cancel*` | `*Cancel*` |
| Nothing at the pick | `Nothing found at that point.`? *(verify)* | `Nothing found at that point.` |
| Offset distance | `Requires a positive and nonzero value.`? | `Requires a positive, nonzero distance.` |
| Nothing to undo | `Everything has been undone.` | same |
| Nothing to redo | `Nothing to redo.`? *(verify V19)* | same |
| TRIM no cut | `Object does not intersect an edge.` | same |
| FILLET parallel | `Lines are parallel.`? *(verify V38)* | `Lines are parallel.` |
| Selection total | `3 found, 4 total` | `3 found` |

### 1.13 Global gaps

| Id | Gap | Pri | Eff | Owner (section 4) | Corpus |
|---|---|---|---|---|---|
| G1-1 | Accept `_`, `.`, `_.` prefixes (strip; `.` bypasses user aliases); `-NAME` as separate commands; `MULTIPLE` | P1 | S | CW-G | 31 |
| G1-2 | `_X` keyword matching at every prompt (exists in `i18n.MatchKeywords`; verify wired into `ParseInput`) | P1 | S | CW-G / i18n | 31 |
| G2-1 | Transparent command frames (`'`, `>>`, resume the prompt) | P2 | M | CW-G | 36 |
| G3-1 | Prompt default formatting `<10.0000>`, Yes/No `<N>`, LUPREC | P3 | S | CW-C | 39 |
| G4-1 | Error wording per prompt kind (1.12), Invalid option keyword, selection message | P3 | S | CW-G | 40 |
| G5-1 | Enter at a required point prompt cancels the command quietly instead of an error loop | P2 | S | CW-G | |
| G6-1 | Defaults live in `Vars`, per session; remove the package-level `filletRadius`, `chamferD1/2`, `lastOffset` shared across sessions | P1 | M | CW-G + CW-B2 | 33 |
| G7-1 | `#`, `*` strip; `<angle` override; `@` with no base = `LASTPOINT` (0,0 start) | P2 | S | WP3 | 03, 04 |
| G7-2 | Typed object snaps END MID CEN INT QUA NEA (P1), PER TAN EXT APP FROM M2P NON TT NOD PAR INS (P2) | P1 / P2 | M | WP3 | 29, 30 |
| G8-1 | Typed selection options W C WP CP F BOX ALL L P R A U M AU SI | P1 | M | WP2 | 15-18, 22-28 |
| G8-2 | Selection memory (`Previous`), "N found, M total" messages, `Invalid selection` text | P2 | S | WP2 | 28 |
| G9-1 | `Session.Script` comments (`;`) and final-newline rule; SCRIPT/RSCRIPT/RESUME/DELAY | P1 | S | CW-G | all |
| G10-1 | `SETVAR`, variables typed by name, `SETVAR ?`, the 1.11 registry, stored no-ops | P1 | M | CW-G | 33 |
| G10-2 | Initial variable set persistent through `settings/` (read its API when it lands) | P3 | S | CW-G | |

---------------------------------------------------------------------------------------------------

## 2. Commands

One section per command. **AutoCAD** gives the prompt sequence in order; `[…]` are keywords,
`<…>` defaults; sub-prompts are indented under their keyword. **Today** is the code on disk
(`cmdsys/commands/*.go`). Gap rows carry the priority, effort and the corpus script that proves the
fix. Aliases are the `acad.pgp` defaults; go-draw's current ones are `L PL C A REC E M CO CP RO SC U Z
LA DI LI AA MEA MI O F CHA TR EX BR J X` and `AGENT` (for AI). Command index with priorities:

| Group | Commands |
|---|---|
| 2.1 Draw | LINE, PLINE, CIRCLE, ARC, RECTANG, POLYGON, ELLIPSE, POINT |
| 2.2 Modify | ERASE, MOVE, COPY, ROTATE, SCALE, MIRROR, OFFSET, FILLET, CHAMFER, TRIM, EXTEND, BREAK, BREAKATPOINT, JOIN, EXPLODE, STRETCH, ARRAY / ARRAYRECT / ARRAYPOLAR / -ARRAY |
| 2.3 History and view | UNDO, U, REDO, MREDO, OOPS, ZOOM, PAN, REGEN, REDRAW |
| 2.4 Layers and properties | LAYER / -LAYER, MATCHPROP, PROPERTIES / CHPROP, COLOR, LINETYPE, LWEIGHT, PURGE |
| 2.5 Inquiry | DIST, ID, AREA, MEASUREGEOM, LIST |
| 2.6 Drawing aids and settings | OSNAP / -OSNAP, ORTHO, GRID, SNAP, UNITS / -UNITS, SETVAR, SCRIPT |
| 2.7 Files | NEW, OPEN, SAVE, QSAVE, SAVEAS, QUIT / EXIT / CLOSE |
| 2.8 Selection | SELECT (and the prompt options of 1.8) |
| 2.9 go-draw only | AI / AGENT |

### 2.1 Draw

#### LINE  (`L`)

**AutoCAD.** `Specify first point:` (Enter: continue from the last line's or arc's end point; after an
arc the new line starts tangent and asks `Specify line length:`) → `Specify next point or [Undo]:`
→ from the third point `Specify next point or [Close/Undo]:`. Enter/Space/right-click ends. `Undo`
(U) removes the last segment and restarts the rubber band at the previous point; `Close` (C) draws to
the first point and ends. Each segment is its own LINE; `LASTPOINT` is the last end. Respects ortho,
polar, snaps; typed forms 1.7. Alias `L`. `Close` appears after **two** segments exist (the third
prompt).
**Today.** First point, next points, Undo, Close (after the third point is entered, same count).
Enter at first point: `Invalid input.`
| Id | Gap | Pri | Eff | Corpus |
|---|---|---|---|---|
| LINE-1 | Enter at the first prompt continues from `LASTPOINT`/the last line end | P1 | S | 02 |
| LINE-2 | After an ARC, Enter starts tangent to it with the length prompt | P3 | S | 08 |
| LINE-3 | A single line is committed per segment but Esc keeps them (met); `U` at the first prompt when nothing is drawn restarts (met) | - | - | 01 |

#### PLINE  (`PL`)

**AutoCAD.** `Specify start point:` → `Current line-width is 0.0000` → `Specify next point or
[Arc/Halfwidth/Length/Undo/Width]:` (from the third vertex also `Close`). Enter ends (open);
Close joins to the start (closed).
- `Width` (W): `Specify starting width <0.0000>:` `Specify ending width <start>:`; `Halfwidth`
  (H): same asked as half-widths. The width applies to following segments; `PLINEWID` is not updated
  *(verify V16)*.
- `Length` (L): `Specify length of line segment:` a segment in the direction of the previous one
  (tangent to a previous arc) *(verify V16)*.
- `Undo` (U): drops the last vertex or arc.
- `Arc` (A) enters arc mode: `Specify endpoint of arc or [Angle/CEnter/CLose/Direction/Halfwidth/Line/
  Radius/Second pt/Undo/Width]:`, default a tangent arc to the next point.
  - `Angle`: `Specify included angle:` then `Specify endpoint of arc or [CEnter/Radius]:`
  - `CEnter`: `Specify center point of arc:` then `Specify endpoint of arc or [Angle/Length]:`
  - `CLose`: closes with an arc to the start
  - `Direction`: `Specify the tangent direction for the start point of arc:` then the endpoint
  - `Line` (L): back to line mode; `Radius`: `Specify radius of arc:` then `Specify endpoint of arc
    or [Angle]:` (then direction of chord); `Second pt` (S): `Specify second point on arc:` then
    `Specify end point of arc:`
Entity: one LWPOLYLINE per command with start/end width per segment and bulges. `PLINETYPE` 2.
Alias `PL`. `PEDIT` is the editor (2.2 P3 list).
**Today.** Start, next points, Close, Undo (`[Undo]` then `[Close/Undo]`), one polyline on Enter/Close/Esc.
Lines only; the document model has no widths.
| Id | Gap | Pri | Eff | Corpus |
|---|---|---|---|---|
| PLINE-1 | `Arc` mode with the 8 sub-options (bulges exist in `doc.LWPolyline`) | P1 | L | 13 |
| PLINE-2 | `Width`/`Halfwidth` (needs per-vertex widths in `doc.LWPolyline`, `fileio/dxf`, render: cross-owner `doc` change) | P1 | M | 12 |
| PLINE-3 | `Length`, and `Close` listed from the third vertex only (met) | P2 | S | 12 |
| PLINE-4 | A one-segment PLINE with Enter creates an LWPOLYLINE of two points (met) | - | - | |

#### CIRCLE  (`C`)

**AutoCAD.** `Specify center point for circle or [3P/2P/Ttr (tan tan radius)]:`
- centre then `Specify radius of circle or [Diameter] <r>:` where `<r>` is `CIRCLERAD` when non-zero;
  `Diameter` (D): `Specify diameter of circle <2r>:`. A pick sets the radius to the distance;
- `3P`: `Specify first point on circle:` `Specify second point on circle:` `Specify third point on
  circle:` (the points may be typed osnaps `TAN`);
- `2P`: `Specify first end point of circle's diameter:` `Specify second end point of circle's
  diameter:`;
- `Ttr`: `Specify point on object for first tangent of circle:` `Specify point on object for
  second tangent of circle:` `Specify radius of circle <r>:`; the result is the solution nearest the
  picks *(verify V14)*;
- TTT is `CIRCLE 3P` with `TAN TAN TAN`.
Zero/negative radius: `Requires a positive and nonzero value.` and ask again. `CIRCLERAD` is the
default radius *(verify V13)*. Alias `C`.
**Today.** Centre, radius or `[Diameter]`; radius refused when ≤ 0.
| Id | Gap | Pri | Eff | Corpus |
|---|---|---|---|---|
| CIRC-1 | `3P`, `2P` keywords at the centre prompt | P1 | S | 05 |
| CIRC-2 | `Ttr` (tangent circle to two lines/arcs/circles: `geom` solver) | P1 | M | 06 |
| CIRC-3 | Default radius `<r>` from `CIRCLERAD`; Enter accepts it | P2 | S | 33 |
| CIRC-4 | TTT / `3P` with `TAN` snaps (needs the TAN snap, WP3) | P3 | M | |
| CIRC-5 | Typed `TAN` / the Diameter default `<2r>` | P3 | S | |

#### ARC  (`A`)

**AutoCAD.** `Specify start point of arc or [Center]:` (Enter continues from the last line/arc end,
tangent) then, by the keywords given:

| Method | Sequence |
|---|---|
| 3 points (default) | start → `Specify second point of arc or [Center/End]:` point → `Specify end point of arc:` |
| Start, Center, End | start, `C`, `Specify center point of arc:`, `Specify end point of arc or [Angle/chord Length]:` |
| Start, Center, Angle | …`A`: `Specify included angle:` (positive ccw, negative cw) |
| Start, Center, Length | …`L`: `Specify length of chord:` (positive: minor arc ccw) |
| Start, End, Angle | start, `E`, `Specify end point of arc:`, `Specify center point of arc or [Angle/Direction/Radius]:` `A` angle |
| Start, End, Direction | …`D`: `Specify tangent direction for the start point of arc:` |
| Start, End, Radius | …`R`: `Specify radius of arc:` (positive: minor arc ccw) |
| Center, Start, End / Angle / Length | `C` first: `Specify center point of arc:` `Specify start point of arc:` then as above |
| Continue | Enter at the first prompt, then only the end point |

Arcs are counterclockwise from start angle to end angle; the points are *not* all on a ccw path:
a 3-point arc is stored ccw from whichever end makes it pass the second point (corpus 07). The
Ctrl key reverses the direction in some methods. Alias `A`.
**Today.** Three points only.
| Id | Gap | Pri | Eff | Corpus |
|---|---|---|---|---|
| ARC-1 | `Center` keyword and Start-Center-End/Angle/Length | P1 | M | 07 |
| ARC-2 | `End` keyword and Start-End-Angle/Direction/Radius | P1 | M | 08 |
| ARC-3 | Center-first (`ARC C`), Enter continue | P2 | S | 08 |
| ARC-4 | Collinear 3 points message (`The points are collinear; no arc was drawn.` vs AutoCAD, which prints nothing and cancels) *(verify)* | P3 | S | |

#### RECTANG  (`REC`)

**AutoCAD.** `Specify first corner point or [Chamfer/Elevation/Fillet/Thickness/Width]:` then
`Specify other corner point or [Area/Dimensions/Rotation]:`.
- `Chamfer`: `Specify first chamfer distance for rectangles <0.0000>:` `Specify second chamfer
  distance for rectangles <d1>:` · `Fillet`: `Specify fillet radius for rectangles <0.0000>:` ·
  `Width`: `Specify line width for rectangles <0.0000>:` · `Elevation`, `Thickness` (2D: store).
  These options stay set for later RECTANG commands until changed; Chamfer and Fillet reset each
  other *(verify V15)*; they are command memory, not system variables.
- `Dimensions`: `Specify length for rectangles <10.0000>:` `Specify width for rectangles <10.0000>:`
  then `Specify other corner point or [Area/Dimensions/Rotation]:` whose *side* (quadrant of the pick
  relative to the first corner) picks the direction.
- `Area`: `Enter area of rectangle in current units <100.0000>:` `Calculate rectangle dimensions
  based on [Length/Width] <Length>:` `Enter rectangle length <10.0000>:`.
- `Rotation`: `Specify rotation angle or [Pick points] <0>:` then the other corner.
The result is a closed LWPOLYLINE of 4 vertices (8 with chamfer or fillet; fillet corners carry
bulge tan(π/8) = 0.4142135624), counterclockwise from the first corner, vertex order *(verify V15)*.
Alias `REC` (and `RECT`? *verify V12*).
**Today.** Two corners; 4-vertex closed polyline.
| Id | Gap | Pri | Eff | Corpus |
|---|---|---|---|---|
| RECT-1 | `Dimensions` | P2 | S | 09 |
| RECT-2 | `Chamfer`, `Fillet` (8-vertex polyline, bulges) with session memory | P2 | M | 10, 11 |
| RECT-3 | `Rotation`, `Area` | P3 | M | |
| RECT-4 | `Width`, `Elevation`, `Thickness` (needs widths: PLINE-2) | P3 | S | |

#### POLYGON  (`POL`)

**AutoCAD.** `Enter number of sides <4>:` (3..1024; `POLYSIDES`) → `Specify center of polygon or
[Edge]:` → `Enter an option [Inscribed in circle/Circumscribed about circle] <I>:` → `Specify radius of
circle:`. A *pick* for the radius puts a vertex (inscribed) or an edge midpoint (circumscribed) at
that point; a *typed* radius orients the polygon with a vertex or edge on the snap angle *(verify
V17)*. `Edge`: `Specify first endpoint of edge:` `Specify second endpoint of edge:`; the polygon is
built counterclockwise from the edge. Bad count: `Requires an integer between 3 and 1024.` Result: a
closed LWPOLYLINE. Alias `POL`.
**Today.** None.
| Id | Gap | Pri | Eff | Corpus |
|---|---|---|---|---|
| POLY-1 | Whole command (a closed polyline factory over `geom`) | P2 | S | 14 |

#### ELLIPSE `EL` and POINT `PO` (blocked by G7)

ELLIPSE: `Specify axis endpoint of ellipse or [Arc/Center]:` … needs an ellipse entity (DXF ELLIPSE
with ratio and parameters) or conversion to a polyline approximation (not AutoCAD-compatible).
POINT: `Current point modes: PDMODE=0  PDSIZE=0.0000` `Specify a point:`; `PDMODE`/`PDSIZE` control the
look; the first point snap `NOD` needs it. Priority: POINT P2 once the entity exists, ELLIPSE P2
(common), both **blocked by G7**; effort M each (entity, render, DXF, snaps).

### 2.2 Modify

All modify commands share the selection phase (1.8) and `PICKFIRST` noun-verb: with a pre-selection
the `Select objects:` prompt is skipped and the set is cleared afterwards.

#### ERASE  (`E`)

**AutoCAD.** `Select objects:` (selection options) → erases. One undo step. `OOPS` restores the last
erase; the Delete key erases a pre-selection (parity plan). Alias `E`.
**Today.** Met except the typed options.
| Id | Gap | Pri | Eff | Corpus |
|---|---|---|---|---|
| ERASE-1 | Typed selection options | P1 | - | 27, 28 |
| ERASE-2 | `OOPS` | P3 | S | |

#### MOVE  (`M`)

**AutoCAD.** `Select objects:` → `Specify base point or [Displacement] <Displacement>:` → `Specify
second point or <use first point as displacement>:` (Enter: the base point *is* the displacement).
`Displacement` (D): `Specify displacement <0.0000, 0.0000, 0.0000>:`. Alias `M`.
**Today.** Base point, second point.
| Id | Gap | Pri | Eff | Corpus |
|---|---|---|---|---|
| MOVE-1 | Enter at second point = displacement from the first; `Displacement` option | P1 | S | 15 |

#### COPY  (`CO`, `CP`)

**AutoCAD.** `Select objects:` → `Specify base point or [Displacement/mOde] <Displacement>:` →
`Specify second point or [Array] <use first point as displacement>:` then repeated `Specify second
point or [Array/Exit/Undo] <Exit>:` until Enter/Exit. `mOde`: `Enter a copy mode option
[Single/Multiple] <Multiple>:` (`COPYMODE`: Single ends after one copy). `Array`: `Enter number of items to
array:` then `Specify second point or [Fit]:` places a row. `Undo` removes the last copy. Copies
keep layer and properties; handles are new. Alias `CO`, `CP` *(verify V18)*.
**Today.** Base point, repeated second points until Enter.
| Id | Gap | Pri | Eff | Corpus |
|---|---|---|---|---|
| COPY-1 | `Displacement`, `mOde`, `Undo`, `Exit`, Enter-as-displacement | P2 | S | 15 |
| COPY-2 | `Array` | P3 | M | |

#### ROTATE  (`RO`)

**AutoCAD.** `Current positive angle in UCS:  ANGDIR=counterclockwise  ANGBASE=0` → `Select objects:`
→ `Specify base point:` → `Specify rotation angle or [Copy/Reference] <0>:`. `Copy` (C): keeps the
original (`Rotating a copy of the selected objects.`) and asks again. `Reference` (R): `Specify the
reference angle <0>:` (a number, or two points) then `Specify the new angle or [Points] <0>:`; the
rotation is new − reference. A pick measures the angle from the base point. Alias `RO`.
**Today.** Base point, angle (typed or picked).
| Id | Gap | Pri | Eff | Corpus |
|---|---|---|---|---|
| ROT-1 | `Copy`, `Reference` (with the two-point forms) | P1 | S | 16 |
| ROT-2 | `ANGBASE`, `ANGDIR` honoured, the header line printed | P3 | S | |

#### SCALE  (`SC`)

**AutoCAD.** `Select objects:` → `Specify base point:` → `Specify scale factor or [Copy/Reference]
<1.0000>:`. `Copy`: keep the original. `Reference`: `Specify reference length <1>:` `Specify new
length or [Points] <1>:` (factor = new/reference). Zero/negative: `Requires a positive and nonzero
value.` *(verify V9)*. Alias `SC`.
**Today.** Base, factor (typed or the click distance), non-positive refused.
| Id | Gap | Pri | Eff | Corpus |
|---|---|---|---|---|
| SCALE-1 | `Copy`, `Reference` | P1 | S | 17 |

#### MIRROR  (`MI`)

**AutoCAD.** `Select objects:` → `Specify first point of mirror line:` → `Specify second point of
mirror line:` → `Erase source objects? [Yes/No] <N>:`. The reflection keeps layer and colour; arcs
stay ccw, polyline bulges flip. `MIRRTEXT` irrelevant (no text). Alias `MI`.
**Today.** Met (prompt text `Erase source objects [Yes/No] <No>:`, default printed `No`).
| Id | Gap | Pri | Eff | Corpus |
|---|---|---|---|---|
| MIRR-1 | Prompt wording and `<N>` default (cosmetic) | P3 | S | 18 |

#### OFFSET  (`O`)

**AutoCAD.** `Current settings: Erase source=No  Layer=Source  OFFSETGAPTYPE=0` → `Specify offset
distance or [Through/Erase/Layer] <Through>:` (`<d>` when `OFFSETDIST` > 0) → `Select object to offset
or [Exit/Undo] <Exit>:` → `Specify point on side to offset or [Exit/Multiple/Undo] <Exit>:` → back to
select. `Through`: the distance is replaced by `Specify through point or [Exit/Multiple/Undo]
<Exit>:`; `OFFSETDIST` becomes -1. `Erase`: `Erase source object after offsetting? [Yes/No] <No>:`
(returns to the distance prompt). `Layer`: `Enter layer option for offset objects [Current/Source]
<Source>:`. `Multiple`: keeps offsetting from the new object. `Undo`: removes the last offset. A
distance can be given as two points. Lines, circles, arcs, polylines, (also xlines, splines).
`Requires a positive and nonzero value.` for ≤ 0 *(verify V9)*. The variable holding Erase/Layer
*(verify V37)*. Alias `O`.
**Today.** Distance (typed / two points / `Through`), `Exit`, remembered distance; polylines mitre.
| Id | Gap | Pri | Eff | Corpus |
|---|---|---|---|---|
| OFFS-1 | `Erase`, `Layer` options (the offset lands on the source layer today) | P2 | S | 19 |
| OFFS-2 | `Multiple`, `Undo` | P2 | S | |
| OFFS-3 | `OFFSETDIST` as a variable (default `<d>`, -1 for Through) and per session | P1 | S | 33 |
| OFFS-4 | `OFFSETGAPTYPE` (fillet/chamfer gaps on closed polylines) | P3 | M | |

#### FILLET  (`F`)

**AutoCAD.** `Current settings: Mode = TRIM, Radius = 0.0000` → `Select first object or
[Undo/Polyline/Radius/Trim/mUltiple]:` → `Select second object or shift-select to apply corner or
[Radius]:`. `Radius`: `Specify fillet radius <r>:`. `Trim`: `Enter Trim mode option [Trim/No trim]
<Trim>:`. `Polyline`: `Select 2D polyline or [Radius]:` fillets every vertex (`N lines were
filleted`). `mUltiple`: repeat. `Undo`. Shift while picking the second object applies radius 0 for
that corner. Works on lines, arcs, circles, polylines, xlines, rays, splines; parallel lines need
radius ≥ half the gap or fail. Variables `FILLETRAD`, `TRIMMODE`. Alias `F`.
**Today.** Two lines; `Radius`; trims/extends; messages for parallel lines and too-large radius.
| Id | Gap | Pri | Eff | Corpus |
|---|---|---|---|---|
| FIL-1 | `Trim`, `mUltiple`, `Undo`, `Polyline` | P2 | M | |
| FIL-2 | Lines + arcs/circles/polylines (needs `geom` fillet on arcs) | P2 | L | |
| FIL-3 | `FILLETRAD` per session as a variable | P1 | S | 20, 33 |
| FIL-4 | Shift-pick radius 0 (needs modifier state, parity 5.2) | P3 | S | |
| FIL-5 | Second prompt keyword `[Radius]` | P3 | S | |

#### CHAMFER  (`CHA`)

**AutoCAD.** `(TRIM mode) Current chamfer Dist1 = 0.0000, Dist2 = 0.0000` → `Select first line or
[Undo/Polyline/Distance/Angle/Trim/mEthod/Multiple]:` → `Select second line or shift-select to apply
corner or [Distance/Angle/Method]:`. `Distance`: `Specify first chamfer distance <a>:` `Specify second
chamfer distance <b>:` (the first applies to the first line picked). `Angle`: `Specify chamfer length on
the first line <c>:` `Specify chamfer angle from the first line <d>:`. `Method`: `Enter trim
method [Distance/Angle] <Distance>:`. Variables `CHAMFERA/B/C/D`, `CHAMMODE`, `TRIMMODE`. Alias
`CHA`. Lines, polylines, xlines, rays.
**Today.** Two lines, `Distance`, remembered distances, sharp corner at 0,0.
| Id | Gap | Pri | Eff | Corpus |
|---|---|---|---|---|
| CHA-1 | `Angle`, `Method`, `Trim`, `Undo`, `Multiple`, `Polyline` | P2 | M | |
| CHA-2 | `CHAMFERA/B` as variables per session | P1 | S | 21 |
| CHA-3 | Polylines and non-line objects | P3 | M | |

#### TRIM  (`TR`)

**AutoCAD, Standard mode (`TRIMEXTENDMODE` 0)** *(verify V11)*: `Current settings: Projection=UCS,
Edge=None` → `Select cutting edges ...` `Select objects or <select all>:` (Enter = every object is an
edge) → `Select object to trim or shift-select to extend or [Fence/Crossing/Project/Edge/eRase/Undo]:`
repeated until Enter. `Fence` / `Crossing` pick several objects by a fence or window; `eRase`:
`Select objects to erase:` ; `Edge`: `Enter an implied edge extension mode [Extend/No extend]
<No extend>:` (extend edges to their projection); `Project` `[None/Ucs/View]`; `Undo`; Shift+pick
extends instead. A pick removes the piece of the object between the nearest cuts, or from the one cut
to the end; no cut: `Object does not intersect an edge.`
**Quick mode (`TRIMEXTENDMODE` 1, default since 2021)**: `Current settings: Projection=UCS, Edge=None,
Mode=Quick` → `Select object to trim or shift-select to extend or [Cutting edges/Crossing/mOde/
Project/eRase]:`: every visible object is an edge; `Cutting edges` switches to explicit edges. In
Quick mode a leading Enter ends the command, so a script that sends an Enter for "select all edges"
needs `TRIMEXTENDMODE 0` (the corpus preamble sets it).
**Today.** Quick-style: `Select object to trim [cuTting edges]`, Enter ends; `T` asks for edges.
| Id | Gap | Pri | Eff | Corpus |
|---|---|---|---|---|
| TRIM-1 | `TRIMEXTENDMODE`: Standard-mode prompt order (edges first, Enter = all) and the Quick/Standard switch (`mOde`) | P1 | M | 22 |
| TRIM-2 | `Fence`, `Crossing`, `eRase`, `Undo`, `Project`, `Edge` | P1 (Fence, Crossing, Undo) / P2 | M | 22 |
| TRIM-3 | Shift+pick extends (needs modifier state, parity 5.2) | P2 | S | |
| TRIM-4 | Keyword prompt text (`Cutting edges` for `cuTting edges`) | P3 | S | |

#### EXTEND  (`EX`)

**AutoCAD.** Mirror of TRIM. Standard: `Select boundary edges ...` `Select objects or <select all>:` →
`Select object to extend or shift-select to trim or [Fence/Crossing/Project/Edge/Undo]:`. Quick:
`Select object to extend or shift-select to trim or [Boundary edges/Crossing/mOde/Project/eRase]:`.
The end nearest the pick grows to the next boundary; lines, arcs, open polylines (end segment),
ellipses, splines. Closed or no boundary: `Object does not intersect an edge.`? *(verify V36)*.
**Today.** Quick-style, Enter ends; pick extends, with own messages.
| Id | Gap | Pri | Eff | Corpus |
|---|---|---|---|---|
| EXT-1 | As TRIM-1 and TRIM-2 (shared edge engine) | P1 | M | 23 |

#### BREAK  (`BR`)  and  BREAKATPOINT

**AutoCAD.** `BREAK`: `Select object:` (the pick point is the first break point) → `Specify second
break point or [First point]:`. `First point`: `Specify first break point:` `Specify second break
point:`. The second point `@` breaks at a point (no gap). The part between the points is removed; a
circle loses the arc ccw from the first to the second point. `BREAKATPOINT`: `Select object:` →
`Specify break point:` splits into two (not for closed objects). Alias `BR`; none for BREAKATPOINT.
**Today.** BREAK met incl. `First point`; `@` not accepted as "the same point" (typed `@` yields
last point, equivalent). BREAKATPOINT missing.
| Id | Gap | Pri | Eff | Corpus |
|---|---|---|---|---|
| BRK-1 | `BREAKATPOINT` (one command over the split primitives) | P2 | S | 24 |
| BRK-2 | `@` as second point (equivalent via `LASTPOINT` once defined) | P3 | S | 24 |

#### JOIN  (`J`)  and  EXPLODE  (`X`)

**AutoCAD.** `JOIN`: `Select source object or multiple objects to join at once:` → (one source)
`Select objects to join to source:`; arcs: `[cLose]`; lines → line if collinear, arcs of one circle →
arc, others → polyline; messages `1 line joined to source`, `2 objects joined`. `EXPLODE`: `Select
objects:` → polylines to lines and arcs (width lost), messages `N objects were not exploded`?
*(verify V39)*. Alias `J`, `X`.
**Today.** Met (messages `N objects joined.` / `cannot be joined.`; `N objects exploded.`).
| Id | Gap | Pri | Eff | Corpus |
|---|---|---|---|---|
| JOIN-1 | Typed selection options (W) and message wording | P1 (via G8-1) | - | 24 |
| JOIN-2 | `cLose` for a single arc | P3 | S | |

#### STRETCH  (`S`)

**AutoCAD.** `Select objects to stretch by crossing-window or crossing-polygon...` `Select objects:`
(only `C`/`CP`, or a drag; the last window is used) → `Specify base point or [Displacement]
<Displacement>:` → `Specify second point or <use first point as displacement>:`. Objects wholly inside move;
crossing objects move only the endpoints/vertices inside; a circle moves only if its centre is inside;
arcs keep the radius while the endpoints move (the centre adjusts) *(verify V40)*. Alias `S`.
**Today.** Missing (parity plan: grips WP4 owns the command, `cmdsys/commands/stretch.go`).
| Id | Gap | Pri | Eff | Corpus |
|---|---|---|---|---|
| STR-1 | The command (crossing window, base/second point, `Displacement`) | P1 | M | 25 |

#### ARRAY  (`AR`), ARRAYRECT, ARRAYPOLAR, ARRAYPATH, -ARRAY

**AutoCAD.** `ARRAY`: `Select objects:` → `Enter array type [Rectangular/PAth/POlar] <Rectangular>:`.
`ARRAYRECT`: `Type = Rectangular  Associative = Yes` `Select grip to edit array or
[ASsociative/Base point/COUnt/Spacing/COLumns/Rows/Levels/eXit] <eXit>:` (counts and spacing by
prompts `Enter the number of rows or [Expression] <4>:`, `Specify the distance between rows or
[Total/Expression]...`). `ARRAYPOLAR`: `Specify center point of array or [Base point/Axis of
rotation]:` `Select grip ... [ASsociative/Base point/Items/Angle between/Fill angle/ROWs/Levels/ROTate
items/eXit] <eXit>:`. Associative arrays create an array object (not in the model). **Non-associative
copies** (`ASsociative` No, or the legacy `-ARRAY`): `-ARRAY` `Select objects:` `Enter the type of array
[Rectangular/Polar] <R>:` `Enter the number of rows (---) <1>:` `Enter the number of columns (|||) <1>:`
`Enter the distance between rows or specify unit cell (---):` `Specify the distance between columns
(|||):` (a negative distance goes down/left); polar: `Specify center point of array or [Base]:` `Enter the
number of items in the array:` `Specify the angle to fill (+=ccw, -=cw) <360>:` `Rotate arrayed
objects as they are copied? [Yes/No] <Y>:` *(verify V24)*. Alias `AR`.
**Today.** Missing.
| Id | Gap | Pri | Eff | Corpus |
|---|---|---|---|---|
| ARR-1 | Non-associative rectangular and polar arrays with the `-ARRAY` prompt sequence and `ARRAYRECT`/`ARRAYPOLAR` taking counts and spacing (explicit copies) | P1 | M | 26 |
| ARR-2 | Associative arrays, `ARRAYPATH`, grips (need a new array entity: not planned) | P3 | L | |

### 2.3 History and view

#### UNDO, U, REDO, MREDO, OOPS

**AutoCAD.** `U`: undoes one step and names it (`LINE`); at the start `Everything has been undone.` `UNDO`:
`Enter the number of operations to undo or [Auto/Control/BEgin/End/Mark/Back] <1>:`. A number undoes that many
steps. `Mark` places a mark; `Back` undoes to the last mark (`This will undo everything. OK <Y>` when
there is none, otherwise no prompt *(verify V20)*); `BEgin`/`End` bracket a group that one `U` undoes *(verify V21)*; `Auto`, `Control`
(`[All/None/One/Combine/Layer] <All>`) are state. `UNDO` inside a command (LINE `U`) is that command's own
keyword. `REDO`: reverses one `U` or `UNDO` (an `UNDO 2` counts as one: *verify V19*); `MREDO`: `Enter number of actions to redo or
[All/Last]:`. `OOPS`: restores the last erase. Aliases `U` (the command `U`, not UNDO), none for UNDO.
Not undoable: selection, view, system variables (except those stored in the drawing: not here).
**Today.** `U` and `UNDO` are one command (`Drawing.Undo`); REDO; `Doc.Undo` per command.
| Id | Gap | Pri | Eff | Corpus |
|---|---|---|---|---|
| UNDO-1 | `UNDO n`; `Mark`/`Back`; `BEgin`/`End` (needs journal marks and groups in `doc`: cross-owner, small API) | P2 | M | 34, 35 |
| UNDO-2 | `U` names the undone command; `UNDO` is a distinct command; the U alias maps to `U` | P3 | S | 35 |
| UNDO-3 | `MREDO`, `OOPS` | P3 | S | |
| UNDO-4 | Undo of variable changes: not required | - | - | |

#### ZOOM  (`Z`)

**AutoCAD.** `Specify corner of window, enter a scale factor (nX or nXP), or [All/Center/Dynamic/Extents/
Previous/Scale/Window/Object] <real time>:`. A point starts a window (`Specify opposite corner:`); a
number is a scale (`2` relative to the full view / limits, `2X` relative to the current view, `2XP`
paper space); `Window`: two corners; `Extents`; `All` (limits or extents, whichever is larger);
`Center`: `Specify center point:` `Enter magnification or height <h>:`; `Previous`: the last of 10
saved views; `Scale`: `Enter a scale factor (nX or nXP):`; `Object`: `Select objects:`;
`Dynamic`: a view box. Enter (real time) is an interactive mode with no scriptable form. Transparent
(`'ZOOM`). Sets `VIEWCTR`, `VIEWSIZE`. Alias `Z`.
**Today.** Window by two corners or `[Extents/Window]`; Enter = Extents (D-H). No `All`, `Center`,
`Previous`, `Scale`, `Object`, numeric scale, transparent use, view stack.
| Id | Gap | Pri | Eff | Corpus |
|---|---|---|---|---|
| ZOOM-1 | `All`, `Center`, `Scale` and a typed scale (`2`, `.5X`), `Object`, `In`/`Out` | P1 | M | |
| ZOOM-2 | `Previous` (a view stack of ~10 views, shared with PAN and the wheel) | P1 | M | |
| ZOOM-3 | Transparent `'ZOOM` | P2 | M (with CW-G frames) | 36 |
| ZOOM-4 | `Dynamic` | P3 | M | |
| ZOOM-5 | The semantics of a bare number relative to "the full view" with no limits *(verify V22)* | P3 | S | |

#### PAN  (`P`)  and  -PAN

**AutoCAD.** `PAN`: real-time (`Press ESC or ENTER to exit, or right-click to display shortcut menu.`),
no scriptable form. `-PAN`: `Specify base point or displacement:` `Specify second point:` (or Enter if
the first was a displacement). Transparent. Aliases `P`, `-P`.
**Today.** Missing (drag and wheel pan work in the UI).
| Id | Gap | Pri | Eff | Corpus |
|---|---|---|---|---|
| PAN-1 | `PAN` (enter real-time pan: mouse drags pan until Enter/Esc) and `-PAN` | P1 | S | |

#### REGEN `RE`, REDRAW

**AutoCAD.** No prompt. `REGEN` rebuilds geometry caches; `REGENALL`; `REDRAW` refreshes. Prints nothing.
**Today.** Missing. | **Gap.** `REGEN`/`REDRAW`/`REGENALL` as no-ops that repaint (P2, S, CW-D).

### 2.4 Layers and properties

#### LAYER  (`LA`)  and  -LAYER  (`-LA`)

**AutoCAD.** `LAYER` opens the Layer Properties Manager (palette). `-LAYER` (scriptable): `Current
layer: 0` `Enter an option [?/Make/Set/New/ON/OFF/Color/Ltype/LWeight/TRansparency/MATerial/Plot/Freeze/
Thaw/LOck/Unlock/stAte/Description/rEconcile]:` repeated until Enter. Names are comma lists with
wildcards (`*`, `?`); `<select objects>` is the default of many prompts.
- `?`: lists layers (`Enter layer name(s) to list <*>:`).
- `Make`: `Enter name for new layer (becomes the current layer) <0>:` creates and sets current.
- `Set`: `Enter layer name to make current or <select object>:`.
- `New`: `Enter name list for new layer(s):` (colour 7, continuous).
- `ON`/`OFF`: `Enter layer name(s) to turn on|off or <select objects>:`; turning the *current* layer
  off asks `Really want layer X (the CURRENT layer) off? <N>`.
- `Color`: `New color [Truecolor/COlorbook]:` (1-255, a name `red`, or `BYLAYER` n/a) then `Enter name
  list of layer(s) for color N <cur>:`.
- `Ltype`: `Enter loaded linetype name or [?] <Continuous>:` then the layers; `LWeight`: `Enter
  lineweight (0.0mm...) :`; `Plot`: `[Plot/No plot]`.
- `Freeze`/`Thaw`: freezing the current layer is refused; `LOck`/`Unlock`: locked layers display but can't be
  edited (a locked current layer still accepts new objects).
- `stAte`, `Description`, `rEconcile`, `MATerial`, `TRansparency`: not planned.
`LAYER` also exposes the transparent forms. Wording of the prompts and refusals *(verify V23)*. Aliases `LA`, `-LA`. Layer names are case-insensitive and
may not contain `< > / \ " : ; ? * | , = \``.
**Today.** `LAYER` only, as a free-text prompt: a name makes it current (creating it, colour 7);
`name=color` (1-255) sets the colour. Not AutoCAD syntax. `doc.Layer` has Name, Color, Off, Locked
(no Frozen, linetype, lineweight, plot, description). The UI layer panel changes the same fields.
| Id | Gap | Pri | Eff | Corpus |
|---|---|---|---|---|
| LAY-1 | `-LAYER` with `?`, Make, Set, New, ON, OFF, Color, LOck, Unlock | P1 | M | 32 |
| LAY-2 | Comma lists, wildcards, the `Really want ... off?` prompt, the refusal messages | P1 | S | 32 |
| LAY-3 | `Freeze`/`Thaw` (needs `Frozen` in `doc.Layer`, render and select behaviour: cross-owner `doc`) | P2 | M | |
| LAY-4 | `Ltype`, `LWeight`, `Plot`, `Description` (blocked by G7: linetype/lineweight model) | P3 | M | |
| LAY-5 | `LAYER` (no dash) = focus the layers panel; keep today's `name=color` form hidden as an extension | P2 | S | |

#### MATCHPROP  (`MA`)

**AutoCAD.** `Select source object:` → `Current active settings = Color Layer Ltype Ltscale Lineweight
Thickness Transparency Plotstyle Dim Text Hatch Polyline Viewport Table Material Shadow display
Multileader` → `Select destination object(s) or [Settings]:` repeated until Enter. `Settings` opens a
dialog (not scriptable). Copies colour, layer, linetype, scale, lineweight *(verify V34)*. Alias `MA`.
**Today.** Missing. Gap: P2, S (layer and colour today; the rest with G7); corpus 38.

#### PROPERTIES `PR`/`CH`/`MO`, CHPROP, CHANGE

**AutoCAD.** `PROPERTIES` toggles the palette (Ctrl+1; parity plan). `CHPROP` is the command-line
editor: `Select objects:` → `Enter property to change [Color/LAyer/LType/ltScale/LWeight/Thickness/
TRansparency/Material/Annotative]:` (each asks its value, then repeats until Enter): `Enter new color
[Truecolor/COlorbook] <BYLAYER>:`, `Enter new layer name <cur>:` (the layer must exist). `CHANGE` edits
points and properties (`Properties` option). Whether a `-PROPERTIES` form exists *(verify V33)*.
**Today.** Read-only side panel; no command. Gap: `CHPROP` (Color, LAyer first): P2, S; `CHANGE` P3;
`PROPERTIES` toggle is parity WP1/WP4. Corpus 38.

#### COLOR  (`COL`), LINETYPE  (`LT`), LTSCALE  (`LTS`), LWEIGHT  (`LW`)

**AutoCAD.** `COLOR` opens the dialog; the command-line forms `-COLOR` (`New object color [Truecolor/
COlorbook] <BYLAYER>:`: `1`, `RED`, `BYLAYER`, `BYBLOCK`) and `-LINETYPE` (`Enter an option [?/Create/Load/Set]:`; `Set`: `Enter
linetype name or [?] <BYLAYER>:` for loaded types) set `CECOLOR`, `CELTYPE` *(verify V33)*; `LTSCALE`: `Enter new
linetype scale factor <1.0000>:`; `LWEIGHT` sets `CELWEIGHT` (hundredths of a mm; -1 ByLayer, -2 ByBlock) with a
dialog, or the variable directly. New objects get these properties.
**Today.** Entities have a layer and a colour (ACI) only; new entities use the current layer and `ByLayer`.
| Id | Gap | Pri | Eff | Corpus |
|---|---|---|---|---|
| PROP-1 | `-COLOR` and `CECOLOR` (current object colour used by `Txn.Add`) | P2 | S | |
| PROP-2 | `LINETYPE`, `LTSCALE`, `LWEIGHT` (blocked by G7: no linetype, no lineweight on entities) | P3 | L | |

#### PURGE  (`PU`)  and  -PURGE

**AutoCAD.** `PURGE` opens a dialog; `-PURGE`: `Enter type of unused objects to purge
[Blocks/DEtailviewstyles/Dimstyles/Groups/LAyers/LTypes/MAterials/MLeaderstyles/MLinestyles/Plotstyles/
SHapes/textSTyles/Tablestyles/Visualstyles/Regapps/Zero-length geometry/Empty text objects/Orphaned
data/All]:` → `Enter name(s) to purge <*>:` → `Verify each name to be purged? [Yes/No] <Y>:` →
`Deleted layer "X".`; layer `0`, the current layer and layers holding objects are kept *(verify V41)*.
**Today.** Missing. Gap: `-PURGE LAyers/Zero-length geometry/All` P3, S. Corpus: none.

### 2.5 Inquiry

All inquiry results go to the command line (and the text window) with the unit precision
(`LUPREC`, 4 decimals **with trailing zeros**: `5.0000`) and set variables.

#### DIST  (`DI`)

**AutoCAD.** `Specify first point:` → `Specify second point or [Multiple points]:` → `Distance = 5.0000,
Angle in XY Plane = 53,  Angle from XY Plane = 0` / `Delta X = 3.0000,  Delta Y = 4.0000,  Delta Z =
0.0000` *(verify V26)*. Sets `DISTANCE` *(verify V26)*. `Multiple points` is the `MEASUREGEOM` mode. Transparent.
Alias `DI`.
**Today.** `Distance = d,  Angle = a°,  Delta X = ...,  Delta Y = ...` with trimmed decimals; `[Multiple points]` adds `Distance = total`.
| Id | Gap | Pri | Eff | Corpus |
|---|---|---|---|---|
| DIST-1 | Output format (`Angle in XY Plane`, `Delta Z`, 4 decimals) and `DISTANCE` | P2 | S | 39 |

#### ID

**AutoCAD.** `Specify point:` → `X = 7.0000  Y = 8.0000  Z = 0.0000`; sets `LASTPOINT` (so a following `@`
works). Transparent. Alias `ID`.
**Today.** `X = 7     Y = 8` (trimmed). | **Gap.** Format and `LASTPOINT`: P2, S, corpus 39.

#### AREA  (`AA`)

**AutoCAD.** `Specify first corner point or [Object/Add area/Subtract area/eXit]:` → `Specify next corner
point or press ENTER for total` (2024: `Specify next point or [Arc/Length/Undo/Total] <Total>:`) →
`Area = 6.0000, Perimeter = 12.0000`; `Object`: `Select objects:` (circle: `Circumference =`; open
polylines add the closing chord); `Add area` / `Subtract area` keep a running total `Total area = ...`.
Sets `AREA`, `PERIMETER`. Alias `AA`.
**Today.** Corners, `Object`; no Add/Subtract/Arc/Length/Undo; output with trimmed decimals.
| Id | Gap | Pri | Eff | Corpus |
|---|---|---|---|---|
| AREA-1 | `Add area`, `Subtract area`, `eXit`, running total | P2 | M | |
| AREA-2 | `AREA`/`PERIMETER` variables, 4-decimal output | P2 | S | 39 |
| AREA-3 | `Arc`/`Length`/`Undo` while digitising | P3 | M | |

#### MEASUREGEOM  (`MEA`)

**AutoCAD.** `Enter an option [Distance/Radius/Angle/ARea/Volume/Quick/Mode] <Distance>:` and, after each
measurement, the same prompt with `eXit` until the user exits *(verify V27)*; `Radius`: `Select arc or circle:` →
`Radius = 5.0000  Diameter = 10.0000`; `Angle`: `Select arc, circle, line or <specify vertex>:`
→ `Angle = 90°`; `ARea`: as AREA; `Quick` hover measurement (UI only). Transparent *(verify V2)*.
**Today.** One measurement per run, no `eXit`, `Radius`/`Angle`/`ARea`/`Distance` done.
| Id | Gap | Pri | Eff | Corpus |
|---|---|---|---|---|
| MEAS-1 | Loop until `eXit`; `Mode`; Volume (2D: n/a) | P3 | S | |

#### LIST  (`LI`, `LS`)

**AutoCAD.** `Select objects:` → per object a block of text: `LINE  Layer: "0"` / `Space: Model space` /
`Handle = 3A` / `from point, X=  0.0000  Y=  0.0000  Z=  0.0000` / `to point, X= 10.0000 ...` / `Length =
10.0000,  Angle in XY Plane =  0` / `Delta X = ...` *(verify V35)*. Closed polylines add `Area`, `Perimeter`.
Aliases `LI`, `LS`.
**Today.** One-line-per-property form, no `Space`, no `LS`. | **Gap.** Format and `LS`: P3, S. Corpus: none (free text).

### 2.6 Drawing aids and settings

#### OSNAP  (`OS`)  and  -OSNAP  (`-OS`)

**AutoCAD.** `OSNAP` opens Drafting Settings (alias `DS`, tab Object Snap). `-OSNAP`: `Current osnap modes:
End,Cen,Int,Ext` / `Enter list of object snap modes or [?] <NEA>:` takes a comma list (`END,MID`) or `NONe`; sets `OSMODE`
(bits in 1.11, the 16384 bit is "running snaps off", which the F3 key and `OSNAP` on/off set). The list
form `OSMODE` typed by value (`OSMODE 3`) is equivalent. Prompt text *(verify V30)*.
**Today.** `Session.Snaps` / `SnapOn` (F3, status bar); no command; bit layout differs.
| Id | Gap | Pri | Eff | Corpus |
|---|---|---|---|---|
| OS-1 | `-OSNAP` and the `OSMODE` variable with AutoCAD's bit values (translate to the internal `SnapMode`) | P1 | S | 37 |

#### ORTHO

**AutoCAD.** `Enter mode [ON/OFF] <OFF>:`; sets `ORTHOMODE`; F8 toggles; also turns polar off.
**Today.** F8 / status bar. | **Gap.** `ORTHO` and `ORTHOMODE`: P1, S, corpus 37.

#### GRID  (`GR`)

**AutoCAD.** `Specify grid spacing(X) or [ON/OFF/Snap/Major/aDaptive/Limits/Follow/Aspect] <10.0000>:`: a number sets
`GRIDUNIT`, `nX` is a multiple of the snap, `ON`/`OFF` set `GRIDMODE`, `Snap` locks to snap spacing, `Major`:
`Specify major grid line frequency <5>:`. F7.
**Today.** F7 toggles `Workspace.Grid`; spacing is a power-of-ten policy. | **Gap.** Variable and command: P1, S;
`GRIDUNIT` honoured by the renderer: P2, S (WP3). Corpus 37.

#### SNAP  (`SN`)

**AutoCAD.** `Specify snap spacing or [ON/OFF/Aspect/Legacy/Style/Type] <10.0000>:`; `Aspect`: `Specify horizontal spacing <10>:`
`Specify vertical spacing <10>:`; `Style` `[Standard/Isometric]`; `Type` `[Polar/Grid]`. F9. Sets `SNAPMODE`, `SNAPUNIT`, `SNAPANG`.
**Today.** Not implemented (parity plan WP3 builds snap-to-grid). | **Gap.** Command and variables: P1, M, corpus 37.

#### UNITS  (`UN`)  and  -UNITS  (`-UN`)

**AutoCAD.** `UNITS` is a dialog; `-UNITS`: `Report formats: (Examples)` `1. Scientific 1.55E+01` `2. Decimal 15.50`
`3. Engineering` `4. Architectural` `5. Fractional` `Enter choice, 1 to 5 <2>:` → `Enter number of digits to right
of decimal point (0 to 8) <4>:` → `Systems of angle measure: ... Enter choice, 1 to 5 <1>:` → `Enter number of
fractional places for display of angles (0 to 8) <0>:` → `Systems of angle direction: ... Enter choice, 1 to 5 <1>:` → `Direction for
angle 0:`/`Do you want angle measured clockwise? [Yes/No] <N>:` *(verify V42)*. Sets `LUNITS`, `LUPREC`, `AUNITS`, `AUPREC`,
`ANGBASE`, `ANGDIR`. `INSUNITS` is set by the dialog / `-INSUNITS`. Transparent.
**Today.** `Header.InsUnits` only; output precision is fixed. | **Gap.** `-UNITS` decimal/degrees and
`LUPREC`/`AUPREC` applied to all printing: P2, M. (Architectural/engineering/fractional: P3.)

#### SETVAR

**AutoCAD.** `Enter variable name or [?] <CIRCLERAD>:` → `Enter new value for CIRCLERAD <0.0000>:`; `?`:
`Enter variable(s) to list <*>:`. Also by typing the name (1.10). Transparent.
**Today.** Missing; no variable registry (parity Wave 0 creates `Vars`). | **Gap.** G10-1: P1, M, corpus 33.

#### SCRIPT, RSCRIPT, RESUME, DELAY

**AutoCAD.** As 1.9. `SCRIPT`: `Enter script file name <last>:`; `RSCRIPT`: restart; `RESUME`; `DELAY`.
**Today.** No `SCRIPT` command (the host runs scripts through `Session.Script`; the agent MCP uses it).
| Id | Gap | Pri | Eff | Corpus |
|---|---|---|---|---|
| SCR-1 | `Session.Script` comments and terminator rule (G9-1) | P1 | S | all |
| SCR-2 | `SCRIPT file` command (HumanOnly, reads through `FileHandler`), `RSCRIPT`, `DELAY`, `RESUME` | P2 | M | |

### 2.7 Files

#### NEW, OPEN, SAVE, QSAVE, SAVEAS, QUIT/EXIT, CLOSE

**AutoCAD.** `NEW`/`QNEW`: dialog, or with `FILEDIA` 0 `Enter template file name or [. (none)] <acadiso.dwt>:`;
`OPEN`: dialog, or `Enter name of drawing to open <...>:`; `SAVE`: acts as `SAVEAS` (a dialog; with `FILEDIA` 0
`Save current drawing as <name>:`) *(verify V25)*; `QSAVE`: saves under the current name silently, a dialog
only for an unnamed drawing; `SAVEAS`: `Save drawing as <name>:` with format options `[2018/2013/2010/...]`;
`CLOSE`: closes the drawing (asks to save when `DBMOD` ≠ 0: `Save changes to X? [Yes/No/Cancel] <Y>`); `QUIT`/`EXIT`:
same for all. Aliases: none for these (Ctrl+N, Ctrl+O, Ctrl+S, Ctrl+Shift+S, Ctrl+Q).
**Today.** `NEW` (no unsaved check), `OPEN`/`SAVE` with a typed path (HumanOnly); the host records the path
(`Session.FilePath`).
| Id | Gap | Pri | Eff | Corpus |
|---|---|---|---|---|
| FILE-1 | `QSAVE` (silent when `FilePath` is set), `SAVEAS`, `Session.Modified()` (D-I), `NEW`/`OPEN`/`QUIT` ask `Save changes?` | P1 | S | |
| FILE-2 | `QUIT`/`EXIT`/`CLOSE` (host callbacks) | P1 | S | |
| FILE-3 | `SAVE` semantics and the DXF version prompt | P3 | S | |

### 2.8 Selection

#### SELECT

**AutoCAD.** `Select objects:` with every option of 1.8; the objects become the *previous selection*
(`P` in the next command), they are not left selected for noun-verb use unless `PICKFIRST` highlights
them *(verify V7)*; prints `N found`. `SELECT` is the scripting idiom `SELECT ALL`, then
`MOVE P`. Related: `SELECTSIMILAR`, `QSELECT`, `FILTER` (P3).
**Today.** None. | **Gap.** `SELECT` over the WP2 engine: P2, S (after G8-1); corpus 28 uses ALL/P directly.

### 2.9 go-draw only

`AI` / `AGENT`: kept as is (no AutoCAD equivalent); it is HumanOnly-adjacent and must never shadow an
AutoCAD name. `OPTIONS` (`OP`) is the settings dialog (i18n `Commands`, `ui/settings*.go`): out of this
document, alias `OP` is listed in the parity plan.

---------------------------------------------------------------------------------------------------

## 3. Alias table (`acad.pgp` defaults)

Every AutoCAD default alias for a command in section 2, with go-draw's state today. "Add with" is the
work package (section 4) that registers it in the same commit as its command
(`Registry.Alias` in its own `register*` function, so no two packages edit one alias list).
Dialog-only and entity-blocked commands (HATCH `H`, MTEXT `T`, TEXT `DT`, BLOCK `B`, INSERT `I`,
dimension aliases, STYLE `ST`, XLINE `XL`, SPLINE `SPL`) are in the parity plan 7.1.

| Alias | Command | Today | Add with |
|---|---|---|---|
| `L` `PL` `C` `A` `REC` `E` `M` `CO` `CP` `RO` `SC` `MI` `O` `F` `CHA` `TR` `EX` `BR` `J` `X` `Z` `LA` `DI` `AA` `MEA` `LI` | the commands in section 2 | present | |
| `U` | `U` (single undo) | present (runs UNDO) | CW-E, with the UNDO/U split |
| `ID` | `ID` | a command name, no alias needed | |
| `LS` | LIST | missing | CW-C |
| `POL` | POLYGON | missing | CW-A |
| `EL` | ELLIPSE | blocked (G7) | |
| `PO` | POINT | blocked (G7) | |
| `DO` | DONUT | missing (P3) | CW-A |
| `S` | STRETCH | missing | WP4 |
| `AR` | ARRAY | missing | CW-B1 |
| `P`, `-P` | PAN, -PAN | missing | CW-D |
| `RE` | REGEN | missing | CW-D |
| `-LA` | -LAYER | missing | CW-D |
| `MA` | MATCHPROP | missing | CW-D |
| `PR`, `CH`, `MO` | PROPERTIES | missing | WP1 toggles, WP4 edits |
| `COL`, `LT`, `LTS`, `LW` | COLOR, LINETYPE, LTSCALE, LWEIGHT | missing | CW-D (COLOR); rest blocked (G7) |
| `PU`, `-PU` | PURGE, -PURGE *(verify V12)* | missing | CW-D |
| `UN`, `-UN` | UNITS, -UNITS | missing | CW-C |
| `OS`, `-OS` | OSNAP, -OSNAP | missing | WP3 |
| `SN`, `GR`, `DS` | SNAP, GRID, DSETTINGS | missing | WP3 |
| `OP` | OPTIONS | missing | UI (settings) |
| `SCR` | SCRIPT *(verify V12)* | missing | CW-G |
| `REC`, `RECT` | RECTANG (both?) *(verify V12)* | `REC` | CW-A |
| *(none)* | `ORTHO`, `SETVAR`, `SELECT`, `QSAVE`, `SAVEAS`, `QUIT`, `EXIT`, `CLOSE`, `OOPS`, `REDO`, `MREDO`, `MULTIPLE`, `BREAKATPOINT`, `ARRAYRECT`, `ARRAYPOLAR`, `-ARRAY` | commands without alias | |

Italian aliases (`i18n.RegisterAliases`, `LINEA`, `CERCHIO`, ...) are additive; they must not take any
English alias above. The `i18n.Commands` list needs every command of section 2 as it lands (its Verify
requires name, description, tip and aliases per language).

---------------------------------------------------------------------------------------------------

## 4. Work breakdown

**Assumptions.** The parity plan's Wave 0 has run: `session.go`, `command.go` and `commands/util.go` are
split, the extension points exist (`Vars`, `AddTextHandler`, `AddStage`, `Session.Script` in its own file,
`commands/aliases.go`, `commands/base.go`) and the hot files are committed. The command packages below
are the parity plan's **WP5 broken into parallel pieces by file ownership**, plus the pieces of **WP2**
(selection options), **WP3** (typed snaps, `#`, `<angle`, ORTHO/GRID/SNAP/-OSNAP) and **WP4** (STRETCH)
that this document specifies. They start in **Wave 2** with WP1-WP4 and WP6; they write only their own
files; docs and corpus flips follow the parity plan's hand-off rule (6.3).

**Wave 0 additions this document asks for** (one agent, no behaviour change; they are what lets the
packages below run in parallel):

1. `commands/util.go` is split into `view.go` (ZOOM), `layer.go` (LAYER), `inquiry.go` (DIST, ID, LIST),
   `files.go` (NEW, OPEN, SAVE, `printWarnings`), `register.go` (`Register`, `NewRegistry`) and
   `aliases.go` (only the acad.pgp text), each command file calling its own `registerX` from `Register`
   (one line each). Today the whole group lives in one 350-line file.
2. `Context` gains the hooks of the next table as *stubs with the final signatures* (so each package
   compiles against them before the real code lands).
3. The shared command memory (`filletRadius`, `chamferD1/D2`, `lastOffset`) moves from package variables to
   `Vars` (G6-1), because two packages (CW-B2 and CW-G) touch it.

**Shared hooks** (all in `cmdsys`, provided by Wave 0 stubs; the owner fills them in):

| # | Hook | Signature (proposal) | Used by | Real code |
|---|---|---|---|---|
| H1 | Variables | `Context.Vars() *Vars`; `Vars.Real/Int/Text/Point(name)`, `Vars.Set(name, v) error`, `Register(Var{Name, Kind, Default, Min, Max, ReadOnly, OnChange})`; per-package `RegisterVarGroup` | every package | CW-G fills the 1.11 table; each CW registers its own variables in its own file |
| H2 | Text pre-processors | `Session.AddTextHandler(order int, f func(*Session, string) bool)`; orders: 10 names/prefixes, 20 selection options, 30 typed snaps, 31 `#`/`*`, 90 variable by name | CW-G, WP2, WP3 | CW-G (10, 90), WP2 (20), WP3 (30, 31) |
| H3 | Last state | `Context.LastPoint() geom.Vec2`, `SetLastPoint`, `LastEntity() (doc.Handle, bool)`, `LastEnd() (p geom.Vec2, tangent float64, ok bool)` | CW-A (LINE-1, ARC-3), CW-C (ID), everyone for `@` | CW-G |
| H4 | Selection memory and engine | `Context.PreviousSelection() []doc.Handle`, `SetPrevious`; `selecting.take` unchanged for commands | WP2 (engine), every selecting command | WP2 |
| H5 | Transparent marker | `type Transparent interface{ Transparent() bool }`; the session pushes a frame, prints `>>`, resumes the parent prompt | CW-D, CW-C, WP3, CW-G (SETVAR) | CW-G |
| H6 | Undo journal | `Drawing.Mark()`, `UndoToMark()`, `BeginGroup()/EndGroup()`, `UndoN(n)`, `RedoN` | CW-E | `doc` owner (small, additive); CW-E consumes |
| H7 | Layer fields | `Layer.Frozen`, later `Plot`, `Linetype`, `Lineweight`, `Description` | CW-D | `doc` owner |
| H8 | Polyline widths | `LWPolyline.Widths [][2]float64` + DXF codes 40/41/43 + render | CW-A (PLINE W/H) | `doc`, `fileio/dxf`, `render` owners |
| H9 | Unit formatting | `Context.Fmt(v float64) string`, `FmtAngle` honouring `LUPREC`/`AUPREC` | CW-C first, then all printing | CW-C |
| H10 | Wording | `cmdsys/wording.go` constants for the 1.12 messages; `ParseInput` picks the kind-specific error | CW-G; used by every command | CW-G (needs the `input.go` owner, below) |

`cmdsys/input.go` is hot today (the i18n work rewrites `Prompt.String` and keyword matching). Its next
owner is **CW-G**, after that agent commits; WP3 and WP2 do not edit it (they use H2).

### Packages

Every package lists: files it owns exclusively (new files unless marked *existing*), hooks it needs,
corpus scripts it must flip to `go-draw: pass`, rows it closes, priority and effort. Tests are new
`*_test.go` files per package (`draw_test.go`, `modify_test.go`, ...); nobody edits `commands_test.go`.

**CW-A Draw commands** (LINE, PLINE, CIRCLE, ARC, RECTANG, POLYGON, ELLIPSE, POINT). *Closes* LINE-1..2,
PLINE-1..3, CIRC-1..5, ARC-1..4, RECT-1..4, POLY-1 (ELLIPSE/POINT when G7 lands). *Owns:* `commands/draw.go`
(*existing*; the owner splits it into `draw_line.go`, `draw_pline.go`, `draw_circle.go`, `draw_arc.go`,
`draw_rect.go`), `commands/polygon.go`, `commands/point.go`, `commands/ellipse.go` (later), new `geom/tangent.go`
(TTR, TTT, arc by end/radius/direction/centre solvers) with `geom/tangent_test.go`. *Needs:* H1, H3, H8 (PLINE
widths, can follow), H2 only through WP3. *Corpus:* 01-14 (01 passes today). *P1 rows:* LINE-1,
PLINE-1, PLINE-2, CIRC-1, CIRC-2, ARC-1, ARC-2. *Effort:* L.

**CW-B1 Transform commands** (ERASE, MOVE, COPY, ROTATE, SCALE, ARRAY family, OOPS). *Closes* ERASE-2, MOVE-1,
COPY-1..2, ROT-1..2, SCALE-1, ARR-1. *Owns:* `commands/modify.go` (*existing*), `commands/array.go`,
`commands/oops.go`. *Needs:* H1 (`COPYMODE`, `ANGBASE`, `ANGDIR`), H4 (via `selecting`, unchanged for commands).
*Corpus:* 15, 16, 17, 26. *P1 rows:* MOVE-1, ROT-1, SCALE-1, ARR-1. *Effort:* M.
(STRETCH stays with WP4 per the parity plan, `commands/stretch.go`, corpus 25.)

**CW-B2 Construct commands** (MIRROR, OFFSET, FILLET, CHAMFER). *Closes* MIRR-1, OFFS-1..4, FIL-1..5, CHA-1..3.
*Owns:* `commands/construct.go` (*existing*; may split into `mirror.go`, `offset.go`, `corner.go`), `geom/offset.go` and
`geom/fillet.go` (*existing*, arcs and circles), `construct_test.go` additions as new files. *Needs:* H1 (G6-1: move
`filletRadius`, `chamferD1/D2`, `lastOffset` into `Vars`, the first thing to do), H5 for nothing. *Corpus:* 18, 19, 20,
21, 33. *P1 rows:* OFFS-3, FIL-3, CHA-2. *Effort:* M-L (arc fillet is the long tail).

**CW-B3 Trim family** (TRIM, EXTEND, BREAK, BREAKATPOINT, JOIN, EXPLODE). *Closes* TRIM-1..4, EXT-1, BRK-1..2, JOIN-1..2.
*Owns:* `commands/trim.go` (*existing*; may split into `trimextend.go`, `breakjoin.go`, `explode.go`),
`commands/edges.go` (shared cutting-edge engine for Standard and Quick modes), `geom/split.go` (*existing*). *Needs:*
H1 (`TRIMEXTENDMODE`, `EDGEMODE`, `PROJMODE`). *Corpus:* 22, 23, 24. *P1 rows:* TRIM-1, TRIM-2 (Fence, Crossing,
Undo), EXT-1. *Effort:* M-L.

**CW-C Inquiry and units** (DIST, ID, AREA, MEASUREGEOM, LIST, UNITS/-UNITS, number formatting). *Closes* DIST-1, AREA-1..3,
MEAS-1, G3-1, units rows. *Owns:* `commands/inquiry.go`, `commands/measure.go` (*existing*), `commands/units.go`,
`cmdsys/format.go` (H9). *Needs:* H1 (`DISTANCE`, `AREA`, `PERIMETER`, `LUPREC`, `LASTPOINT`), H3, H5 (transparent). *Corpus:*
39. *P1 rows:* none (all P2/P3; DIST/ID/AREA already work in substance). *Effort:* M.

**CW-D View, layers and properties** (ZOOM, PAN, REGEN, REDRAW, LAYER/-LAYER, MATCHPROP, CHPROP, -COLOR, PURGE). *Closes* ZOOM-1..5,
PAN-1, LAY-1..5, PROP-1, MATCHPROP, PURGE. *Owns:* `commands/view.go` (view stack, shared with the wheel through a
`render.ViewStack` new file `render/viewstack.go` owned by this package), `commands/layer.go`, `commands/matchprop.go`,
`commands/props.go`, `commands/purge.go`. *Needs:* H1, H5 (transparent ZOOM/PAN/LAYER), H7 (Frozen). *Corpus:* 32, 36 (with
CW-G), 38. *P1 rows:* ZOOM-1, ZOOM-2, PAN-1, LAY-1, LAY-2. *Effort:* M-L.

**CW-E File and history** (UNDO/U/REDO/MREDO, NEW, OPEN, SAVE, QSAVE, SAVEAS, QUIT/EXIT/CLOSE). *Closes* UNDO-1..4, FILE-1..3.
*Owns:* `commands/undo.go`, `commands/files.go`, `cmdsys/dirty.go` (`Session.Modified`, D-I). *Needs:* H6 (the `doc`
journal API: asks the `doc` owner), H1 (`DBMOD`). *Corpus:* 34, 35. *P1 rows:* FILE-1, FILE-2. *Effort:* M.

**CW-G Command-line core** (prefixes, variables, SETVAR, SCRIPT, transparent frames, wording). *Closes* G1-1..2, G2-1, G4-1,
G5-1, G6-1, G9-1, G10-1..2, SCR-1..2, OS/ORTHO variable plumbing for WP3. *Owns:* `cmdsys/names.go`, `cmdsys/sysvars.go`,
`cmdsys/transparent.go`, `cmdsys/script.go` (the `Script` method from Wave 0), `cmdsys/wording.go`, `cmdsys/input.go`
(*existing*, after the i18n agent commits), `commands/sysvar.go`, `commands/script.go`, and the documents: `features/drawing-and-editing.md`
(single owner, other packages send notes) and `tech/command-system.md` (the prefix, variable, script and transparent rules; WP3
keeps its object-snap rows). *Needs:* H1-H3 are its own. *Corpus:* 31, 33, 37 (variables), 36 (frames), and the script
rules for all. *P1 rows:* G1-1, G1-2, G6-1, G9-1, G10-1. *Effort:* L.

**WP2 selection options** (parity plan): typed `W C WP CP F BOX ALL L P R A U M AU SI`, `Previous` memory, messages, `SELECT`.
Owns `cmdsys/selectopts.go`, `cmdsys/select.go`, `cmdsys/commands/select.go` (new, SELECT). *Closes* G8-1..2. *Corpus:* 27,
28, and unlocks 15-18, 22-26. *P1.* *Effort:* M. The handler is a H2 order-20 text handler active only while the prompt
accepts a selection, so no command changes.

**WP3 aids** (parity plan): typed object snaps, `#`, `*`, `<angle`, ORTHO, GRID, SNAP, -OSNAP and their variables. Owns
`cmdsys/override.go`, `cmdsys/aids.go`, `commands/aids.go` and the others in the parity plan. *Closes* G7-1..2, OS-1,
ORTHO, GRID, SNAP rows. *Corpus:* 03, 04, 29, 30, 37. *P1.*

**WP4 grips** (parity plan): STRETCH. *Closes* STR-1. *Corpus:* 25.

### Order and dependencies

```
Wave 0   seam + util.go split + hook stubs + Vars move (G6-1)                    one agent
Wave 2   CW-G  CW-A  CW-B1  CW-B2  CW-B3  CW-C  CW-D  CW-E  WP2  WP3  WP4        parallel, own files
         first days of CW-G: H1 (Vars), H2 (handlers), H3, H5; the others stub against Wave 0
         CW-D and CW-E wait on one small additive change each in `doc` (H6, H7)
Wave 3   integration: aliases registered, `go-draw:` lines flipped, drawing-and-editing.md, INDEX, backlog
```

Suggested order when agents are scarce (value per effort): WP2 + CW-G (scripts start working at all: corpus 15-18,
22-28, 31, 33), then CW-B1/CW-B3 (move/copy/rotate/scale, trim), CW-A (circle/arc/pline options), CW-D (-LAYER, ZOOM, PAN), WP3,
CW-E, CW-B2, CW-C.

Collision rules specific to this document: only CW-G edits `cmdsys/input.go`; only CW-G edits `features/drawing-and-editing.md`;
`Aliases` text in `commands/aliases.go` is frozen after Wave 0 (packages register their own aliases); the corpus `.expect`
files are edited by the package whose scripts they are, only to flip `go-draw:` or to correct an expectation after a real AutoCAD
check (a correction needs a note).

---------------------------------------------------------------------------------------------------

## 5. Acceptance corpus

`cmdsys/commands/testdata/acad/`: `README.md` defines the `.scr` rules and the `.expect` line format; `_preamble.scr` is what real
AutoCAD runs first. `cmdsys/commands/acad_corpus_test.go` runs every script through `Session.Script` and compares; it is skipped
unless `GODRAW_ACAD_CORPUS` is `report`, `1` (compare with each file's `go-draw:` declaration) or `strict`.

40 scripts; **3 pass today** (01 LINE, 20 FILLET, 21 CHAMFER), 37 fail. Per script (details in the corpus README):

| Script | Covers | go-draw | Main gap | (verify) |
|---|---|---|---|---|
| `01_line_basic_close_undo` | LINE: continued points, relative input, Undo, Close, Enter | pass |  |  |
| `02_line_continue` | LINE Enter continues from the last endpoint | fail | Enter at the first-point prompt (continue) |  |
| `03_line_coordinate_forms` | Point input forms: absolute, relative, polar, # | fail | `#` absolute prefix | yes |
| `04_line_angle_override` | Angle override <angle then a distance | fail | `<angle` override | yes |
| `05_circle_options` | CIRCLE: centre-radius, Diameter, 2P, 3P | fail | CIRCLE 2P, 3P |  |
| `06_circle_ttr` | CIRCLE TTR tangent to two lines | fail | CIRCLE TTR | yes |
| `07_arc_start_options` | ARC: three points, Start-Center-End/Angle/Length | fail | ARC Center/Angle/chord Length |  |
| `08_arc_end_options_continue` | ARC: Start-End-Radius/Direction, Enter to continue | fail | ARC End/Radius/Direction, Enter to continue |  |
| `09_rectang_corners_dimensions` | RECTANG: two corners, Dimensions | fail | RECTANG Dimensions |  |
| `10_rectang_chamfer` | RECTANG Chamfer | fail | RECTANG Chamfer | yes |
| `11_rectang_fillet` | RECTANG Fillet | fail | RECTANG Fillet | yes |
| `12_pline_close_undo_length` | PLINE: Undo, Width, Length, Close | fail | PLINE Width, Length | yes |
| `13_pline_arc_line` | PLINE: Arc and Line modes | fail | PLINE Arc |  |
| `14_polygon` | POLYGON: Edge, Inscribed, Circumscribed | fail | POLYGON |  |
| `15_move_copy` | MOVE and COPY with Displacement, Multiple, mOde | fail | `L` selection; MOVE Displacement; COPY mOde |  |
| `16_rotate` | ROTATE: angle, Copy, Reference | fail | `L` selection; ROTATE Copy, Reference |  |
| `17_scale` | SCALE: factor, Copy, Reference | fail | `L` selection; SCALE Copy, Reference |  |
| `18_mirror` | MIRROR: keep and erase source | fail | `L` selection |  |
| `19_offset` | OFFSET: distance, Through, Erase | fail | OFFSET Erase |  |
| `20_fillet` | FILLET: Radius, radius 0 corner | pass |  |  |
| `21_chamfer` | CHAMFER: Distance | pass |  |  |
| `22_trim` | TRIM: select-all edges, Fence | fail | TRIM select-all prompt, Fence |  |
| `23_extend` | EXTEND: select-all boundaries | fail | EXTEND select-all prompt |  |
| `24_break_join_explode` | BREAK, BREAKATPOINT, JOIN, EXPLODE | fail | BREAKATPOINT; JOIN and EXPLODE selection options |  |
| `25_stretch` | STRETCH: crossing window | fail | STRETCH |  |
| `26_array` | -ARRAY: rectangular and polar | fail | -ARRAY | yes |
| `27_select_window_crossing_polygon` | Selection: W, C, WP, CP | fail | W, C, WP, CP |  |
| `28_select_last_previous_fence_remove_single` | Selection: L, P, ALL+R, F, SI | fail | L, P, ALL, R, F, SI |  |
| `29_snap_end_mid_cen_qua` | Typed object snaps END MID CEN QUA | fail | typed object snaps |  |
| `30_snap_int_nea_per_from_m2p` | Typed snaps INT NEA PER FROM M2P NON | fail | typed object snaps |  |
| `31_prefixes_and_keywords` | _ and . prefixes, keyword spellings | fail | `_` and `.` prefixes |  |
| `32_layer_options` | -LAYER: New, Color, Make, OFF, LOck, Set | fail | -LAYER |  |
| `33_sysvars` | SETVAR and typed variable names feed command defaults | fail | SETVAR, variables by name |  |
| `34_undo_mark_back` | UNDO Mark and Back | fail | UNDO Mark/Back |  |
| `35_undo_number_group_redo` | UNDO n, BEgin/End, U, REDO | fail | UNDO n, BEgin/End |  |
| `36_transparent_zoom` | Transparent 'ZOOM inside LINE | fail | transparent commands |  |
| `37_drafting_aids` | ORTHO GRID SNAP -OSNAP commands, typed points unaffected | fail | ORTHO, GRID, SNAP, -OSNAP |  |
| `38_matchprop_chprop` | MATCHPROP and CHPROP | fail | MATCHPROP, CHPROP |  |
| `39_inquiry_vars` | DIST, AREA, ID results and variables | fail | DIST/AREA/ID variables, precision | yes |
| `40_error_messages` | Error wording at bad input | fail | error wording | yes |

How the corpus is used: a package's definition of done includes flipping its scripts to `go-draw: pass` in the same commit (mode `1`
fails until the declaration is edited, so a regression and an unannounced improvement both show up). The expectations are from
documentation, not from an install: the first real-AutoCAD run of the 40 scripts is a task of its own (section 6, V-checklist), and a
mismatch is fixed in the `.expect` (with a note) before it is counted as a go-draw defect.

---------------------------------------------------------------------------------------------------

## 6. Checklist: facts to verify in an installed AutoCAD

Each item is marked *(verify Vn)* where it is used above. Record the answer here (or in the owning feature document) and in the
affected `.expect` files, which list `# (verify)` where one of these decides the expectation.

| # | What to check | Used in |
|---|---|---|
| V1 | `_.LINE`, `._LINE` and `'_ZOOM` accepted; the order of prefixes | 1.1 |
| V2 | Exact text for a non-transparent command typed with `'`; membership of REGEN, MEASUREGEOM, VIEW, CAL, UNITS, COLOR, LINETYPE, LWEIGHT in the transparent list | 1.2 |
| V3 | Enter at a required point prompt with no default (CIRCLE centre): silent end, `*Cancel*`, or re-ask | 1.5 |
| V4 | `#x,y` accepted when `DYNMODE` is 0 | 1.7, corpus 03 |
| V5 | `<angle` then a bare distance in a script; the exact angle-override prompt text | 1.7, corpus 04 |
| V6 | `ALL` selects objects on layers that are OFF (and not frozen/locked) | 1.8 |
| V7 | `SELECT` leaves a Previous set but not a pickfirst set; Previous is cleared by which operations | 1.8, 2.8 |
| V8 | Exact `N found`, `N found, M total`, `N duplicate`, `N removed` messages | 1.3, 1.8 |
| V9 | The error texts of 1.12 (Invalid point, Point or option keyword required, Requires numeric distance or second point, Requires a positive and nonzero value, Invalid option keyword, Invalid selection ...) | 1.4, 1.12, corpus 40 |
| V10 | Exact unknown-command text (two spaces, `Press F1 for more help.`) | 1.1 |
| V11 | `TRIMEXTENDMODE` is the variable name, default 1 (Quick) in 2021+, and the Quick/Standard prompts of TRIM and EXTEND | 1.6, 2.2, corpus 22-23 |
| V12 | acad.pgp aliases marked: `RECT`, `SCR`, `PU`, `-PU` and any other not in the 2024 file | 3 |
| V13 | CIRCLE updates `CIRCLERAD` (default radius = last radius) or only SETVAR does | 1.6, 2.1 |
| V14 | CIRCLE TTR chooses the solution nearest the picked points | corpus 06 |
| V15 | RECTANG Chamfer/Fillet persistence, mutual reset, and the start vertex of the resulting polyline | corpus 10, 11 |
| V16 | PLINE Length direction after an arc; whether `W` updates `PLINEWID` | corpus 12 |
| V17 | POLYGON orientation for a typed radius; the circumscribed pick rule | corpus 14 |
| V18 | COPY: default Multiple, the 2024 prompt with `[Array/Exit/Undo]`, `COPYMODE`; aliases `CO`/`CP` | corpus 15 |
| V19 | REDO after `UNDO 2` restores both steps; the `Nothing to redo` wording | corpus 35 |
| V20 | `UNDO Back` with a mark asks nothing; the messages UNDO prints | corpus 34 |
| V21 | A `UNDO BEgin`/`End` group is undone by one `U` | corpus 35 |
| V22 | A bare number at ZOOM relative to what (limits vs. extents) | 2.3 |
| V23 | `-LAYER` prompt wording, option letters, the refusal messages for the current layer | corpus 32 |
| V24 | `-ARRAY` prompt order and wording; present in 2024/2025 | corpus 26 |
| V25 | SAVE vs SAVEAS vs QSAVE under `FILEDIA` 0 | 2.7 |
| V26 | DIST output text and the variables set (`DISTANCE`, `ANGLE`?) | corpus 39 |
| V27 | MEASUREGEOM loops with `eXit` after each measurement | 2.5 |
| V28 | SETVAR messages: unknown name, read-only, wrong type, out of range | 1.10 |
| V29 | OSMODE bit values and the default 4133 (END CEN INT EXT); 16384 | 1.11 |
| V30 | `-OSNAP` prompt text and result for a list, `NONe` | 2.6 |
| V31 | `INT` snap needs both objects inside the aperture of the pick; `PER`, `TAN` deferral | 1.7, corpus 30 |
| V32 | Nested `SCRIPT`, tabs, errors inside scripts, `RSCRIPT`/`RESUME`/`DELAY` | 1.9 |
| V33 | Existence of `-COLOR`, `-LINETYPE`, a command-line LWEIGHT, `-PROPERTIES`; CHPROP prompts | 2.4 |
| V34 | MATCHPROP prompts and the `Settings` option from a script | 2.4, corpus 38 |
| V35 | LIST output format | 2.5 |
| V36 | EXTEND/TRIM Shift behaviour, Fence prompts, `eRase`, `Undo`, messages | 2.2 |
| V37 | Which variables store OFFSET's Erase and Layer options | 2.2 |
| V38 | FILLET/CHAMFER messages (parallel lines, radius too large), CHAMFER Angle/Method prompts | 2.2 |
| V39 | BREAK `@`, BREAKATPOINT prompts, JOIN and EXPLODE messages | 2.2 |
| V40 | STRETCH selection rules (circles, arcs, polylines) | 2.2 |
| V41 | `-PURGE` prompts | 2.4 |
| V42 | `-UNITS` prompt sequence | 2.6 |
| V43 | `CMDECHO` 0 suppresses the echo of scripted input | 1.9 |
| V44 | The `>>` prompt prefix and its nesting | 1.2 |
| V45 | Spaces inside text entry in scripts (TEXT content, layer descriptions) | 1.5, 1.9 |
| V46 | `LASTPOINT` is set by which commands (the first point of LINE, ID, picks...) and what `@` gives right after a new drawing | 1.6 |
