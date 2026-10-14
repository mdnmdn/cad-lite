# AutoCAD parity audit and implementation plan

Raw planning material (inbox). Written 2026-10-14 against the working tree as it stood that day, **with
other agents' uncommitted edits on disk** (section 1). Nothing here is implemented; no code or other
document was touched to produce it.

Scope: keyboard, mouse and command-line *paradigm* of AutoCAD 2024/2025 defaults versus Cad-lite.
Out of scope: new entity types (TEXT, HATCH, blocks, dimensions: backlog G7) except where a missing
command is only blocked by them, and anything about DWG.

Reading guide: section 2 is the gap matrix, 3 the decisions the product owner must make before
waves start, 4 the work packages (the part to execute), 5 what gpui must add, 6 ordering and
collision rules, 7 the alias diff and the AutoCAD facts that need a check against a real install.

Priority: **P1** a drafter hits it in the first hour and it breaks muscle memory or loses work;
**P2** expected within the first week; **P3** polish, advanced or niche. Effort: **S** under a day,
**M** a few days, **L** a week or more.

AutoCAD facts are from memory of 2024/2025 defaults. Items marked *(verify)* are the ones I am least
sure of; section 7 lists them for a check in a real install before they become acceptance criteria.

---------------------------------------------------------------------------------------------------

## 1. What is on disk right now (do not collide)

`git status` shows uncommitted work by other agents. Read-only observations, as of this audit; they
may change.

| Area | What is being added | Consequence for this plan |
|---|---|---|
| `ui/canvas.go`, `ui/canvas_test.go` | `Workspace.wheel`: Ctrl/Cmd+wheel zooms (pinch compat), Shift+wheel pans, precise (trackpad) scroll pans unless `WheelZoom`, horizontal notch pans; middle double-click = zoom extents; `OnHoverEnd` hides crosshair; `pointerOnCanvas`; `resizeView` keeps the visible box on resize | Wheel semantics are *already being redesigned*: this plan does not touch `wheel()` (decision D-F). Middle double-click extents is done, drop it from the gap list |
| `ui/workspace.go` | Lazy `AgentPanel()`, `agentCommand` (AI command), `pathChanged`, `win` field, `WheelZoom`, coords show `—` until the pointer is known | `actionAgent` stays bound to F2 for now (D-A changes it); `ws.win` exists for popups to refresh |
| `ui/agent*.go`, `agentmanager/store/` | Agent panel, transcript store, history per drawing path | Out of bounds for every package below, except the one-line key rebind in `ui/keys.go` |
| `cmdsys/session.go`, `cmdsys/command.go`, `cmdsys/commands/{agent,util}.go` | `Session.FilePath/SetFilePath/OnFilePath`, `SetAgentHandler`, `Context.Agent/HasAgent/SetFilePath`, `AI`/`AGENT` command, `SAVE`/`OPEN` record the path | `FilePath()` is the hook QSAVE and Ctrl+S need (WP5). Session core is hot: seam work waits (section 6) |
| `internal/gpui` events, window, ebitenplatform | `ScrollWheelEvent.Precise` + `wheelClassifier`; `pointerTracker` (pointer unknown until it moves or clicks); `Window.SetUIScale/UIScale` (UI zoom) | gpui work (WP6) must start from their result. UI zoom will want key bindings: keep `Ctrl+=`, `Ctrl+-` and **not** `Ctrl+0` (AutoCAD: clean screen) |
| `tools/gui` | New verbs `hwheel`, `hold MOD`, `release MOD`; scenario `gestures.gui` | Held Shift can already be simulated for the Shift override tests. Still missing: state assertions (section 4, Wave 0) |
| `settings/` | Not on disk yet; `go.mod` gained `BurntSushi/toml` (indirect) so a settings package is coming | System variables (WP5) should persist through it. Re-read when it lands |
| `assets/appicon`, `packaging/`, `.github/`, `justfile` | Unrelated | Disjoint from every package below |

**Hot files** (owned by someone else until they commit): `ui/canvas.go`, `ui/workspace.go`,
`ui/agent*.go`, `cmdsys/session.go`, `cmdsys/command.go`, `cmdsys/commands/util.go`,
`internal/gpui/{events.go,window.go,ebitenplatform/*,_docs/api-contract.md}`, `tools/gui/*`, `settings/`.

Real defects found while reading (each is folded into a package below):

1. **F-keys auto-repeat.** `ebitenplatform.shouldFire` repeats non-printable keys after 30 ticks every
   3 ticks and the event carries no repeat flag, so holding F3, F7 or F8 flips the toggle repeatedly.
   (Ctrl+letter shortcuts do not repeat.)
2. **No mouse capture.** A press-drag released outside the canvas (over the toolbar or side panel)
   never delivers the mouse-up, so `canvasState.pressed/boxing/panning` stay set until the next click.
3. **`Unknown command ... Press F1 for help.`** points at a key that is not bound.
4. **Modifier changes are dropped.** The ebiten backend emits `ModifiersChangedEvent`, but
   `Window.dispatchEvent` ignores it ("The POC has no modifier-state listeners"), so Shift-hold
   overrides cannot be built on it today.
5. **Ctrl+C / Ctrl+X / Ctrl+V never reach the keymap**: `ebitenplatform` turns them into clipboard
   operations on the focused `TextInput` before dispatch, so an entity clipboard cannot be bound.
6. **Delete, Up, Down, Tab, Ctrl+A** are either swallowed by `TextInput.onKeyDown` (Delete deletes a
   character) or unhandled, and `TextInput` has no select-all and no key pre-hook.

---------------------------------------------------------------------------------------------------

## 2. Gap matrix

"Today" is the code on disk (including the in-progress edits above). `S/M/L` is effort for the gap
as stated, assuming the gpui pieces in section 5 exist. WP = the package in section 4 that owns it.

### 2.1 Function keys (AutoCAD 2024 Windows defaults)

| Area | AutoCAD behaviour | Cad-lite today | Gap | Pri | Eff | WP |
|---|---|---|---|---|---|---|
| F1 | Help | Unbound, though the unknown-command message tells the user to press it | Bind F1: HELP command and overlay listing commands, aliases and keys (a real help URL later) | P1 | S | 1 |
| F2 | Text window (`TEXTSCR`): the full scrollable command history, toggled | F2 toggles the **agent panel** | Conflicts. New text-window overlay on F2; agent panel moves (D-A) | P1 | M | 1, 6 |
| F3 | Object snap on/off (`OSMODE` bit 16384, running modes kept). Hold = temporary override | Toggles `Session.SnapOn`; no hold semantics; auto-repeat flips it (defect 1) | Fix repeat; hold-override (P3, needs key-hold, section 5) | P1 / P3 | S / M | 1, 3 |
| F4 | 3D object snap (`3DOSMODE`) | Unbound | N/A in 2D: print "3D object snap is not available" | P3 | S | 1 |
| F5 | Cycle isometric plane (`ISOPLANE`) | Unbound | Needs isometric snap/grid; defer | P3 | L | 3 |
| F6 | Dynamic UCS toggle (`DUCS`); coordinate display cycle is `Ctrl+D` (`COORDS` 0/1/2) *(verify)* | Unbound; status coordinates always follow the pointer | `Ctrl+D` cycles COORDS (off/absolute/polar-while-rubber-banding); F6 no-op message | P3 | S | 1 |
| F7 | Grid (`GRIDMODE`) | Toggles `Workspace.Grid` (a UI field, not a session variable, no `GRID` command) | Make it a variable; `GRID` command (On/Off/spacing/Snap/Aspect); user `GRIDUNIT` instead of power-of-ten only | P2 | S | 3 |
| F8 | Ortho (`ORTHOMODE`); hold Shift = temporary opposite | Toggles `Session.Ortho`; no Shift override; no `ORTHO` command; not mutually exclusive with polar (no polar yet) | `ORTHO` command; Shift override (WP3); turning ortho on turns polar off | P1 / P2 | S / M | 3 |
| F9 | Snap to grid (`SNAPMODE`, `SNAPUNIT` default 10,10, `SNAPANG`, snap type) | **Not implemented at all** (the status bar has no SNAP) | Snap-to-grid stage in the cursor pipeline; `SNAP` command; status toggle | P1 | M | 3 |
| F10 | Polar tracking (`POLARMODE`, `POLARANG` 90, `POLARADDANG`, relative/absolute) with tooltip `Polar: 12.5 < 45°` | Not implemented | Polar stage, tracking line drawn on canvas, status toggle, `POLAR` settings via `DSETTINGS`/sysvars | P1 | M | 3 |
| F11 | Object snap tracking (`AUTOSNAP` bit 16, tracking vectors from acquired points) | Not implemented | Acquire points by hovering, alignment vectors; builds on polar | P2 | L | 3 |
| F12 | Dynamic input (`DYNMODE` 3 by default: pointer input, dimensional input, prompts at the cursor) | Not implemented | Whole subsystem (4.6) | P2 | L | 6 |
| Status bar | Toggles in order: GRID, SNAP, DYN, ORTHO, POLAR, ISODRAFT, OTRACK, OSNAP, LWT, TRANSPARENCY, SELECTION CYCLING ... | GRID, OSNAP, ORTHO | Add SNAP, POLAR, OTRACK, DYN (as each lands); right-click a toggle opens its settings | P1 | S each | 3, 6 |

### 2.2 Control-key shortcuts and other keys

| Area | AutoCAD behaviour | Cad-lite today | Gap | Pri | Eff | WP |
|---|---|---|---|---|---|---|
| Ctrl+A | Select all (selectable) objects | `TextInput` has no select-all; nothing bound | Select all; with typed text on the line, select the text instead *(verify)* | P1 | S | 1, 2 |
| Delete | With a selection and the command line empty: ERASE immediately (`PICKFIRST`) | Deletes a character in the field (consumed by `TextInput`) | Pre-hook: empty line + selection + idle -> ERASE | P1 | S | 1 |
| Ctrl+Z / Ctrl+Y | `U` / `REDO`. Inside a command, Ctrl+Z acts as the command's Undo option (LINE, PLINE) | Always runs UNDO, which *cancels the running command* then undoes the previous one | At a prompt with an Undo keyword, send that keyword; otherwise as today. Also: Ctrl+Y is redo, Ctrl+Shift+Z is not AutoCAD (keep for macOS) | P2 | S | 1 |
| Ctrl+S | `QSAVE`: save to the current file, ask only if untitled | Not bound; SAVE prompts for a path every time | Needs a dirty flag and `Session.FilePath` (arriving). QSAVE, SAVEAS, bind both | P1 | S | 5, 1 |
| Ctrl+Shift+S | `SAVEAS` | Not bound | as above | P1 | S | 5, 1 |
| Ctrl+O / Ctrl+N | OPEN / NEW (dialogs) | Not bound; OPEN/NEW exist, OPEN types a path | Bind; dialogs wait for gpui file dialog (G18, 5.8). NEW must warn when unsaved | P1 | S | 1, 5 |
| Ctrl+Q | Quit (asks to save) | Not bound | Dirty tracking, "Save changes?" prompt | P1 | S | 5 |
| Ctrl+P | PLOT | None | No plot yet (G17); later EXPORT PNG/SVG/PDF | P3 | L | - |
| Ctrl+C / V / X | COPYCLIP / PASTECLIP / CUTCLIP | Consumed by the command-line text field (defect 5) | Entity clipboard; route keys via keymap first (5.7) | P2 | M | 1, 6 |
| Ctrl+Shift+C / V | COPYBASE (asks for a base point) / PASTEBLOCK | None | After the above; PASTEBLOCK needs blocks (G7), so paste as plain entities, insertion point | P2 | M | 5 |
| Ctrl+1 | Properties palette toggle | The properties panel is always visible and read-only | Toggle visibility; make it editable (G9) | P2 | S / L | 1 / 4 |
| Ctrl+2, 3, 4, 6, 7, 8 | DesignCenter, Tool Palettes, Sheet Set Mgr, Data Connectivity, Markup, QuickCalc | None | Not applicable, leave unbound (do not reuse for other meanings) | - | - | - |
| Ctrl+9 | Show/hide command line | Fixed | `COMMANDLINE`/`COMMANDLINEHIDE`, hide the area | P3 | S | 1 |
| Ctrl+0 | Clean screen (hide toolbars/panels) | None (and the UI-zoom work must avoid it) | Hide chrome | P3 | S | 1 |
| Ctrl+F / G / L / B / D | OSNAP / GRID / ORTHO / SNAP toggles; coordinates cycle | None (only F-keys) | Cheap synonyms once each variable exists | P2 | S | 3 |
| Ctrl+[ or Ctrl+\ | Cancel (like Esc) | None | Bind to escape action | P3 | S | 1 |
| Ctrl+J, Ctrl+M | Enter | None | Bind | P3 | S | 1 |
| Ctrl+W, Shift+Space | Selection cycling toggle / cycle overlapping objects | None | See 2.4 | P3 | M | 2 |
| Ctrl+Tab | Next drawing | Single document | N/A | - | - | - |
| Ctrl+E | Cycle isometric plane | **Zoom extents** (Cad-lite's own) | Non-AutoCAD binding, harmless; keep until isometric exists (D-G) | P3 | S | 1 |
| Escape | One press cancels the running command (and typed text); at idle clears the selection and grips; cancels a pending window; a second Esc is needed to leave nested states (a hot grip, a transparent command inside a command) *(verify the single/double rules)* | Cancels the command (`*Cancel*`) or, idle, clears the selection; clears the field | Pending window (WP2), grip cancel (WP4), transparent unwinding (WP5), and the double-Esc rules | P1 | S | 1, 2 |
| Up / Down | Recall previous command-line inputs (commands at `Command:`, values at prompts) (`INPUTHISTORYMODE` 15, `CMDINPUTHISTORYMAX` 20) | Ignored | Input history ring, per prompt kind; also PgUp/PgDn scroll the history | P1 | M | 1 |
| Tab / suggestions | `AUTOCOMPLETEMODE` 15 *(verify bits)*: as you type a command a suggestion list (commands, aliases, system variables, mid-string and content matches) appears; Tab / arrows choose; Enter runs what is typed unless an item is highlighted | None (`Registry.Complete` exists, unused). Tab does nothing | Suggestion popup over the command line, Tab cycles, click picks; autocorrect (bit 2) later | P2 | M | 1, 6 |
| Typing anywhere | Letters typed with the pointer over the drawing go to the command line | Focus returns to the field every frame it is lost; clicks on the canvas refocus; typing in the agent field stays there by design | Mostly met. Keep contexts so new bindings do not fire while the agent field has focus (4.1 rule) | P3 | S | 1 |
| Space / Enter | Both submit; Space repeats the last command at `Command:`; Space is literal in text prompts | Met (D9). Space is implemented in `OnChange`, so pasted text containing a space submits | Handle paste separately (only a *typed* trailing space submits) | P3 | S | 1 |
| Right-click | Default: `SHORTCUTMENU` 11: a context menu (idle: Repeat last, Recent Input, Clipboard, Undo, Pan, Zoom, Find...; with a selection: Edit menu; **during a command**: Command-mode menu with Enter, Cancel, Recent Input, Pan, Zoom and the command's options, but only if the command has options; otherwise plain right-click = Enter). Time-sensitive right-click (quick = Enter, long = menu) is an option *(verify default)* | Always Enter (so at `Command:` it repeats the last command) | Context-menu element (5.3), menus per state; Enter/Cancel/option items. Needs `Prompt.Keywords` reachable from the UI (exists) | P2 | M | 6 |
| Shift+right-click | Object-snap override menu (`MBUTTONPAN`=0 puts it on the middle button) | None | Popup menu of snap modes that arms a one-shot override | P2 | M | 6, 3 |
| Temporary override keys | Hold Shift = toggle ORTHO (and polar) for the moment; hold F3 = OSNAP, F9 = SNAP, F10 = POLAR, F11 = OTRACK override; Shift+A toggles all running osnaps; Shift+S = snap override; all defined in the CUI "Temporary Override" section *(verify the full table in a real CUI)* | None | Key-hold + modifier state in gpui (5.2), override flags in the aids pipeline | P2 | M | 3, 6 |
| Typed snap overrides | At any point prompt type `END`, `MID`, `CEN`, `INT`, `PER`, `TAN`, `NEA`, `QUA`, `NOD`, `INS`, `APP`, `EXT`, `PAR`, `FRO`, `TT`, `M2P`, `NON`, comma lists (`END,MID`); the next pick uses it once | `ParseInput` rejects the text as `Invalid point.` | One-shot override set on the session; needs the missing snap modes (PER, TAN, NOD, EXT, PAR, FRO, TT) | P1 (END MID CEN INT QUA NEA) / P2 (rest) | M | 3 |
| Transparent commands | `'ZOOM`, `'PAN`, `'LAYER`... typed at any prompt; prompt shows `>>`; the interrupted command resumes | Not supported (the wheel/middle-drag work transparently, the typed forms do not) | Nested command frame in the session; view commands first | P2 | M | 5 |
| Coordinate forms | `x,y` absolute, `@dx,dy`, `@d<a`, `d<a`, bare number (direct distance), `#x,y` absolute with dynamic input on, `@` last point, `<a` angle override (locks the angle, then a distance) | All except `#` and `<a`; `@` alone works | Add `#`, `<a` override | P2 | S | 3 |
| Command prefixes | `_LINE` (language-independent), `.LINE` (built-in), `-LAYER` (command-line version of a dialog), `'` (transparent) | None accepted | Accept `_` and `.` (strip); `-` per command; `'` per WP5 | P2 | S | 5 |

### 2.3 Mouse and selection

| Area | AutoCAD behaviour | Cad-lite today | Gap | Pri | Eff | WP |
|---|---|---|---|---|---|---|
| Implicit window | Click empty space, move, click again: window (blue, L->R, fully inside) or crossing (green dashed, R->L, touching). Works idle and at `Select objects:` (`PICKAUTO` 1) | Only press-drag-release draws a box (D8); a click on empty space does nothing | Add click-click (D-B); keep press-drag | P1 | M | 2 |
| Lasso | Press-drag-release in empty space draws a freehand lasso (clockwise = window, anticlockwise = crossing); Space while dragging cycles Window/Crossing/Fence *(verify direction rule)*. `PICKDRAG` 0 (default) = two-click rectangle | Press-drag draws a rectangle | Lasso behind a setting (D-B) | P2 | M | 2 |
| `PICKADD` | Default 2: each pick adds; Shift+click removes; (`PICKADD`=0: a new pick replaces, Shift adds) | Pick adds, Shift removes (the default behaviour) | `PICKADD` 0/1/2 variable | P3 | S | 2 |
| Empty click | Does not deselect; Esc does (a window that finds nothing leaves the set alone) | Same | Met | - | - | - |
| `PICKFIRST` | Select, then run the command, which uses the set and releases it | Met for ERASE, MOVE, COPY, ROTATE, SCALE, LIST, MIRROR..., selection cleared after | Check every selecting command honours it (TRIM/EXTEND edges, BREAK, JOIN, EXPLODE, OFFSET, FILLET) | P2 | S | 5 |
| Selection preview | `SELECTIONPREVIEW` 3: objects highlight under the pickbox, and while a window/crossing is being dragged those inside highlight before the click | Hover highlight when no point is asked for; none during a box drag | Live preview during box drag and click-click | P2 | S | 2 |
| Selection style | Selected objects drawn dashed/halo with grips; unselected highlight lighter | Thick blue | Cosmetic; grips (WP4) change the look anyway | P3 | S | 2 |
| `Select objects:` options | `W`, `C`, `WP`, `CP`, `F`, `ALL`, `L`, `P`, `R`, `A`, `BOX`, `SI`, `M`, `AU`, `G`; messages `1 found, 3 total` | Only click, box and Enter; prints `N found` | One selection engine with typed options; memory of the previous selection for `P`/`Previous` | P2 | M | 2 |
| `SELECT`, `QSELECT`, `FILTER`, `SELECTSIMILAR` | Present | None | Later | P3 | M | 2 |
| Selection cycling | `Shift+Space` cycles overlapping objects (`SELECTIONCYCLING`) | None | Later | P3 | M | 2 |
| Grips | A selected object shows grips (`GRIPS` 2, size 5 px: unselected blue, hover green, hot red). Click a grip = hot grip, STRETCH on it; Space/Enter cycles STRETCH, MOVE, ROTATE, SCALE, MIRROR; Ctrl while stretching = copy; Shift+click makes several grips hot; hover a polyline grip with Ctrl for the context grip menu (add/remove vertex, convert segment) | None (G9). The properties panel is read-only | Whole subsystem (4.4). Needs the STRETCH command | P1 (stretch/move grips) / P2 (cycling, copy, multi-hot) / P3 (polyline grip menu, GRIPOBJLIMIT) | L | 4 |
| Double-click an object | `DBLCLKEDIT`: opens the editor for the type (Properties/Quick Properties, PEDIT for polylines, TEXTEDIT for text) | Nothing; also `ClickCount` is only read for the middle button | Double-click -> properties panel focused (P2); type editors later | P2 | S | 2 |
| Middle button | Hold-drag pans (`MBUTTONPAN` 1); double-click = zoom extents; Shift+middle drag = orbit (3D, N/A); `MBUTTONPAN` 0 = osnap menu | Pan met; double-click extents arriving (canvas.go) | `MBUTTONPAN` variable (P3) | P3 | S | 2 |
| Wheel | Forward = zoom in about the cursor (`ZOOMWHEEL` reverses); `ZOOMFACTOR` 3..100 default 60 sets the step; Ctrl+wheel = pan left/right, Shift+wheel = pan up/down *(verify)* | Zoom about the cursor, 1.087 per 20 px notch; Ctrl zooms, Shift pans (the in-progress design) | No `ZOOMFACTOR`/`ZOOMWHEEL`; Ctrl/Shift roles differ from AutoCAD (D-F) | P3 | S | 2 |
| Cursor | Crosshair of `CURSORSIZE` 5 % of the screen, small pickbox (`PICKBOX` 3), system arrow hidden over the drawing, `APERTURE` 10 px | Full-length hairlines, 8 px pickbox, OS arrow shown (G15) | `CURSORSIZE`, `PICKBOX`; cursor hide needs gpui (5.5) | P2 | S | 2, 6 |
| Drag past the canvas | A window drag keeps working outside the canvas | Mouse-up outside the canvas is lost (defect 2) | gpui mouse capture (5.6) | P1 | S | 6, 2 |
| Rollover tips | Hover an object: type/layer tooltip (`ROLLOVERTIPS`) | None | After the tooltip element exists | P3 | S | 6 |
| Snap tooltips and marker | Marker + magnet + tooltip naming the snap (`Endpoint`, `Midpoint`...) (`AUTOSNAP` 63 *(verify bits)*) | Marker and magnet exist; `SnapMode.String` exists but is unused; no tooltip | Tooltip near the cursor | P1 | S | 3, 6 |

### 2.4 Command line and command set

| Area | AutoCAD behaviour | Cad-lite today | Gap | Pri | Eff | WP |
|---|---|---|---|---|---|---|
| Prompt format | `Specify next point or [Close/Undo]:` `<default>`; keywords answered by capitals | Met | - | - | - | - |
| History | Last `CLIPROMPTLINES` (3) lines above the prompt; F2 shows the full scrollback | 4 lines above; 200 kept (`HistoryLimit`) | F2 overlay, larger cap (e.g. 2000), copy | P1 | M | 1 |
| Repeat | Enter/Space/right-click at `Command:` repeats the last command; `MULTIPLE cmd` repeats until Esc; Recent Input list in the right-click menu | Enter/Space/right-click repeat (met) | `MULTIPLE`; recent-commands menu | P3 | S | 5, 6 |
| Dynamic input | Prompts, distance, angle and coordinate fields in tooltips at the cursor; Tab moves between fields; typing there replaces the command-line entry | None | 4.6 | P2 | L | 6 |
| Aliases | `acad.pgp` (about 150 defaults), user-editable | 27 entries incl. `AGENT` (section 7 has the diff) | Missing: `P`, `LS`, `S`, `AR`, `POL`, `EL`, `PO`, `H`, `DO`, `XL`, `T`/`DT`, `B`, `I`, `PE`, `MA`, `PR`/`CH`, `LEN`, `AL`, `DIV`, `ME`, `RE`, `OP`, `UN`, `OS`, `SN`, `GR`, `DS` and the dimension aliases (the last group blocked by G7) | P1 for those whose command exists or lands in WP5 | S | 5 |
| `PAN` / realtime zoom | `P`; `Z` Enter = realtime; `Z` options `All`, `Center`, `Dynamic`, `Extents`, `Previous`, `Scale` (`2X`, `0.5XP`), `Window`, `Object`; `ZOOM P` view stack | ZOOM: window by two corners or `Extents/Window`; Enter = Extents (AutoCAD: realtime) | PAN; Zoom Previous (needs a view stack), All, Center, Scale, Object; Enter semantic note (D-H) | P1 | M | 5 |
| `STRETCH` | `S`: crossing selection, base point, second point | Missing (G8 lists it; not yet built) | Needed by grips | P1 | M | 4 |
| `ARRAY` | `AR`: rectangular, polar, path (associative by default) | Missing | Non-associative ARRAYRECT/ARRAYPOLAR first | P1 | M | 5 |
| `POLYGON`, `DONUT`, `POINT`, `DIVIDE`, `MEASURE`, `XLINE`, `RAY`, `SPLINE`, `ELLIPSE` | Present | Missing (ELLIPSE, SPLINE have no entity type: G7) | POLYGON (a closed polyline) and DONUT, POINT need no new type except POINT | P2 | S-M | 5 |
| Draw-command options | `ARC` Center/End/Angle/Length/Direction/Radius/Second point; `CIRCLE` 2P/3P/TTR/TTT; `PLINE` Arc/Close/Halfwidth/Length/Undo/Width; `RECTANG` Chamfer/Elevation/Fillet/Thickness/Width + Area/Dimensions/Rotation; `LINE` Close/Undo (met) | ARC 3-point only; CIRCLE center-radius/diameter only; PLINE lines only (G11); RECTANG corners only | Each is its own work item inside the command files | P1 (CIRCLE 2P/3P/TTR, ARC start-center-end, PLINE Arc) / P2 | M | 5 |
| Modify options | `COPY` Displacement/mOde/Multiple/Array; `ROTATE` Copy/Reference; `SCALE` Copy/Reference; `TRIM` Fence/Crossing/Project/Edge/eRase/Undo, Shift to extend; `EXTEND` Shift to trim; `OFFSET` Through/Erase/Layer/Multiple; `FILLET` Polyline/Radius/Trim/Multiple; `MOVE` Displacement | Partial (see `trim.go`, `construct.go`) | Option-parity pass per command | P2 | M | 5 |
| Match / properties | `MATCHPROP` (`MA`), `PROPERTIES` (`PR`, Ctrl+1), `CHANGE`, `PEDIT` (`PE`), `LENGTHEN` (`LEN`), `ALIGN` (`AL`), `LAYER` palette, `LAYISO`, `LAYOFF`, `LINETYPE`, `LTSCALE`, `LWEIGHT` | LAYER (name/colour by text) only; panel read-only | MATCHPROP, PEDIT, LENGTHEN, ALIGN P2; the rest depends on G7 | P2 | M | 5, 4 |
| `UNDO` | `U` one step; `UNDO` prompts `[Auto/Control/BEgin/End/Mark/Back]` and a count; `REDO` once; `OOPS` | `U` and `UNDO` are the same single step | `UNDO n`, Mark/Back, BEgin/End; `OOPS` | P2 | M | 5 |
| System variables | `SETVAR`, any variable typed by name (`OSMODE`, `ORTHOMODE`, `GRIDMODE`, `PICKADD`...), persisted in the profile/drawing, listed by `SETVAR ?` | None. `Session.Snaps/SnapOn/Ortho/Aperture` and `Workspace.Grid/WheelZoom` are loose fields | Variable registry with change callbacks; every toggle becomes a variable; typed name prints/sets it | P1 | M | 5 (foundation in Wave 0) |
| Settings commands | `GRID`, `SNAP`, `ORTHO`, `OSNAP`, `DSETTINGS` (`DS`, `OS`, `SN`, `GR`), `-OSNAP` | None; only keys and status buttons | Typed toggles with prompts | P1 | S-M | 3 |
| File commands | `QSAVE`, `SAVEAS`, `CLOSE`, `QUIT`/`EXIT`, `SAVE` (a legacy alias of SAVEAS), `OPEN`, `NEW`, `RECOVER`, `PURGE`, `AUDIT`, `UNITS`, `LIMITS`, `REGEN`, `REDRAW`, `PLOT`, `EXPORT` | OPEN, SAVE (path each time), NEW | QSAVE, SAVEAS, QUIT/EXIT, REGEN/REDRAW, UNITS (read-only), PURGE | P1 (QSAVE, SAVEAS, QUIT) / P3 | S | 5 |
| Clipboard commands | `COPYCLIP`, `CUTCLIP`, `PASTECLIP`, `COPYBASE`, `PASTEBLOCK`, `PASTEORIG` | None | In-process entity clipboard first, DXF text on the OS clipboard later | P2 | M | 5 |
| Command echo | `Command: _line` for toolbar clicks and underscore forms | `Command: LINE` | Fine | - | - | - |
| Command search | Typing a partial name and Enter runs the first match only via the suggestion list; `AUTOCORRECT` fixes typos | Unknown command message | With the suggestion list | P2 | S | 1 |

---------------------------------------------------------------------------------------------------

## 3. Decisions needed

Recommendations are mine; each needs a yes/no from the product owner before its package starts.
Answers go to `tech/decisions.md` as new D entries, appended by the Wave 0 agent (section 6), so
that file has one writer.

**D-A. F2 and the agent panel.** AutoCAD's F2 is the text window (full history). Cad-lite gives F2 to
the agent panel (`actionAgent`, `agent-panel.md`).
Recommendation: **F2 becomes the text window; the agent panel moves to `Ctrl+Shift+A`** (also the
existing `AI` command and toolbar button). Why not F12: it is Dynamic Input (WP6) and users will
expect the real thing. Why `Ctrl+Shift+A`: AutoCAD's use of it (toggle groups) is rarely touched and
Cad-lite has no groups. If groups ever land, revisit. Cost: update `agent-panel.md`, `command-line.md`,
`tools/gui/scenarios/{agent_panel,gestures}.gui`, `ui/{agent_test,agent_store_test,canvas_test}.go`
(they press F2) in the same commit as the rebind; one agent does all of it (WP1, with WP6 only
consulted).

**D-B. Box selection (supersedes D8).** Three AutoCAD facts: click-click window/crossing (`PICKAUTO`);
press-drag-release is a *lasso* in 2015+ (`PICKDRAG` 0); `PICKDRAG` 1 makes press-drag a rectangle.
D8 chose press-drag rectangles for trackpad/test friendliness.
Recommendation: **implement both rectangle gestures now**: click-click (the AutoCAD default) *and*
keep press-drag-release as a rectangle (exactly AutoCAD's `PICKDRAG`=1 behaviour, so it is a
documented setting, not an invention). Add lasso as a third mode behind a `PICKDRAG` value (0 =
AutoCAD default lasso on drag; 1 = rectangle on drag, **Cad-lite default for now**; 2 = off). Revisit
the default after the lasso is verified in the real window (it has to be: no display here). Click vs.
click-click must be told apart by the 4 px drag threshold the code already has; a press that does
not travel starts or finishes a click-click window.

**D-C. Where typed overrides, transparent commands and system variables live.** Recommendation: in
`cmdsys` (core, headless-testable, scriptable), never in `ui`. The UI only forwards text and mouse.
That keeps the dependency rule (core never imports gpui) and lets `Session.Script` test them.

**D-D. Dynamic input scope.** Full DYNMODE (3 fields, Tab, `#`/`@` semantics) is L. Recommendation:
ship in two steps: step 1 a *cursor tooltip* showing the prompt and the live coordinate or
distance < angle (read-only; also carries the snap name and the polar text, which P1 needs anyway);
step 2 editable fields. F12 toggles both.

**D-E. Native file dialogs (G18).** Recommendation: build an in-app file picker on the gpui overlay
(works in the WASM build, headless-testable) rather than shelling out per platform; keep command-line
OPEN/SAVE typing as the fallback. Ctrl+O/Ctrl+S use it.

**D-F. Wheel and pan modifiers.** The in-progress edit makes Ctrl+wheel zoom (pinch compatibility)
and Shift+wheel pan both axes. AutoCAD's Ctrl+wheel/Shift+wheel are the two pan directions *(verify)*.
Recommendation: keep the in-progress design (pinch must keep working), make it a documented
difference, and expose `ZOOMFACTOR`/`ZOOMWHEEL` later. Do not touch `Workspace.wheel` in this plan.

**D-G. `Ctrl+E` (zoom extents).** Non-AutoCAD. Keep until an isometric plane exists, then rebind.

**D-H. ZOOM with Enter.** AutoCAD's Enter at `ZOOM` is *real-time* zoom. Cad-lite's default is
Extents. Recommendation: keep Extents as the Enter default (no real-time zoom without a drag mode),
document it, and make `Z` `E` / `Z` `A` / middle double-click the parity paths.

**D-I. Dirty flag.** QSAVE/Quit/NEW need "unsaved changes". Recommendation: `doc.Drawing` gets a
monotonically increasing change counter and `Session` remembers the counter at the last save
(`Session.Modified()`); no new transaction semantics. Owner: WP5.

**D-J. Undo inside a command.** `Ctrl+Z` during LINE should remove the last segment, not cancel the
command. Recommendation: yes (P2), through a new optional `cmdsys.Undoable` interface on commands
(`UndoStep(ctx) *Prompt`) in addition to the keyword; commands that already have an `Undo` keyword
get it for free by routing the keyword.

---------------------------------------------------------------------------------------------------

## 4. Work packages

### 4.0 Wave 0: seam (one agent, serial, S/M), plus the test harness

**Why.** The files every package needs (`ui/workspace.go`, `ui/canvas.go`, `cmdsys/session.go`,
`cmdsys/commands/util.go`) are also where today's agents are working, and two packages editing one
file is the collision to avoid. Wave 0 changes no behaviour: it splits those files so each later
package owns *new* files, and adds one registration line per package.

**Starts when** the agents in section 1 have committed (hot files clean). Until then WPs may only
write brand-new files and tests.

**Owns:** `ui/workspace.go`, `ui/canvas.go`, `cmdsys/session.go`, `cmdsys/command.go`,
`cmdsys/commands/util.go`, `cmdsys/commands/aliases.go` (new), `cmd/cad/main.go` (dev flag),
`tools/gui/*`, `_docs/tech/decisions.md`, `_docs/backlog.md`.

Deliverables:

1. **UI split, no behaviour change** (moves only; same tests green):

   | New file | Receives | Later owner |
   |---|---|---|
   | `ui/keys.go` | `action*` vars, `Bindings()`, the `OnAction` wiring, escape/submit helpers | WP1 |
   | `ui/cmdline.go` | `commandArea`, history rendering, `acceptsText` | WP1 |
   | `ui/statusbar.go` | `statusBar` | WP3 (flags) / WP6 (menus) |
   | `ui/toolbar.go`, `ui/sidepanel.go` | `toolbar`, `button`, `sidePanel` | WP6 / WP4 (properties) |
   | `ui/canvas_input.go` | the mouse handlers of `canvasElement`, `pointerMoved` (not `wheel`) | WP2 |
   | `ui/paint.go` | `paintCanvas` reduced to calling the overlay painters below in order | Wave 0 only |
   | `ui/paint_selection.go`, `ui/paint_aids.go`, `ui/paint_grips.go`, `ui/paint_dyn.go` | `highlight`/box code; `cursorItems`+`snapMarker`; (empty); (empty) | WP2, WP3, WP4, WP6 |

   `canvas.go` keeps `canvasState`, view sizing, grid, UCS icon, `wheel` (still owned by the wheel
   agent until it lands).
2. **Init hooks.** `NewWorkspace` calls `ws.initKeys()`, `ws.initSelection()`, `ws.initAids()`,
   `ws.initGrips()`, `ws.initChrome()`, each a stub in its owner's file. Canvas mouse down/move/up
   run ordered hook lists, **first consumer wins**: `chrome` (open popup) -> `grips` -> `selection`
   -> default (answer point / pick):

   ```go
   // in ui/canvas_input.go (Wave 0 defines; owners append in their init*)
   type pointerHook func(ev pointerEvent, w *gpui.Window) (consumed bool)
   func (ws *Workspace) addPointerHook(phase hookPhase, order int, h pointerHook)
   // overlay painters, ordered, called by paintCanvas
   func (ws *Workspace) addOverlay(order int, f func(dl *render.DisplayList))
   // popups / chrome elements rendered above the window content
   func (ws *Workspace) addChrome(f func(w *gpui.Window, cx *gpui.App) gpui.IntoElement)
   ```
3. **cmdsys split and extension points** (`cmdsys/session.go` shrinks, behaviour unchanged):

   | New file | Content | Later owner |
   |---|---|---|
   | `cmdsys/vars.go` | `Vars` registry: `Register(Var{Name, Kind, Default, Min/Max, OnChange})`, `Get/Set/String`, `Session.Vars()`, `RegisterVarGroup(func(*Vars))` run by `NewSession`. `Snaps`, `SnapOn`, `Ortho`, `Aperture` become variables (`OSMODE`, `ORTHOMODE`, `APERTURE`) with the old fields kept as accessors | Wave 0 creates; WP1-5 each register their own in their own file |
   | `cmdsys/pipeline.go` | Cursor pipeline: ordered `PointStage` list used by `Hover`: `type PointStage interface{ Apply(s *Session, st *PointState) }` where `PointState{Raw, Cursor geom.Vec2; Base *geom.Vec2; Snap *SnapHit; Tags []string}`; today's snap and ortho become stages 1 and 2 | WP3 |
   | `cmdsys/textin.go` | Ordered text pre-processors: `Session.AddTextHandler(order int, func(s *Session, text string) (handled bool))`, run at the top of `Submit` | WP1 (history), WP3 (override names), WP5 (`'`, `_`, `.`, `-`) |
   | `cmdsys/events.go` | Notifications the UI renders: `Session.OnPromptChange`, `OnSelectionChange`, `OnVarChange` | WP1/2/6 |
   | `cmdsys/commands/aliases.go` | `Aliases` const moved out of `util.go`, plus `Register` split so each command file registers itself (`registerDraw`, `registerModify`, ...) | WP5 |
   | `cmdsys/commands/base.go` | the shared helpers (`base`, `selecting`) if still in `util.go` | WP5 |

4. **Harness for state assertions** (needed by every package's real-window tests). `cmd/cad -dev-state
   PATH` (dev flag like `-save-on-exit`) rewrites a small JSON file on every change: prompt, last 8
   history lines, selection count and handles, flags (`snap`, `ortho`, `grid`, `polar`...), view
   centre and scale, active command, `modified`. New `tools/gui` verbs: `expect-prompt "text"`,
   `expect-history "substring"`, `expect-selected N`, `expect-var NAME VALUE`, `expect-view-changed`,
   `keydown KEY` / `keyup KEY` (named keys, for held F-keys; `hold`/`release` already cover
   modifiers), `drag-path X1 Y1 X2 Y2 ...` (lasso and multi-segment gestures). Document in
   `tools/gui/README.md`.
5. **Decision records.** Append D-A..D-J outcomes to `decisions.md`; mark G-items in the backlog
   that this plan covers (`G9`, `G10`, `G11`, `G18`, `G20`, `G21`) with "planned: autocad-parity".

**Tests.** The moved code keeps `ui/workspace_test.go`, `ui/canvas_test.go`, `cmdsys/*_test.go` and
`go test ./...` green and `-png` renders byte-identical before/after (AGENTS.md rule 8); plus a unit
test for `Vars` and for stage/handler ordering.

### Waves

```
Wave 0  seam + harness (serial)                       after current agents commit
Wave 1  WP6a gpui contract (document first)           parallel with Wave 0 only if gpui is clean
Wave 2  WP1  WP2  WP3  WP4  WP5  WP6b  in parallel    all write only their own files
Wave 3  integration pass: docs index, backlog, GUI scenarios across packages
```

Dependencies inside Wave 2 are by *interface*, not by code order:
WP4 (grips) needs STRETCH from itself and the pointer hook order from Wave 0; WP2 and WP4 both read
`Session.Cursor()` and the selection; WP3 supplies snap/polar to everyone through the pipeline;
WP1/WP3/WP2 need gpui 5.1, 5.2, 5.6, 5.7 (each can stub against the contract and add the real calls
when WP6a's code lands).

---

### WP1. Keyboard and command line

**Goal.** Every key in 2.1/2.2 that is not a drafting-aid toggle, plus the command-line behaviours:
F1 help, F2 text window, Delete, Ctrl+A, file shortcuts, Ctrl+Z inside commands, history recall,
suggestions, Ctrl+9/0, F-key repeat fix, agent panel rebind.

**Owns (exclusive):**
`ui/keys.go`, `ui/cmdline.go`, `ui/textwindow.go` (new), `ui/suggest.go` (new),
`cmdsys/history.go` (new), `cmdsys/complete.go` (new), `cmdsys/help.go` (new),
`cmdsys/commands/help.go` (new), tests `ui/keys_test.go`, `ui/cmdline_test.go`,
`cmdsys/history_test.go`, `cmdsys/complete_test.go`, `tools/gui/scenarios/{keys,cmdline,textwindow}.gui`;
docs `features/command-line.md`, `features/agent-panel.md` (key lines only), `tech/testing.md`
(rows for the new tests).

**Does not touch.** `ui/agent*.go` (the rebind is one `NewKeyBinding` line in `ui/keys.go`; the
panel's own code is unchanged), `internal/gpui` (asks WP6), `cmdsys/session.go`.

**API / hooks.**

| Provides | To | Signature |
|---|---|---|
| Input history | WP6 (Up/Down in menus), UI | `cmdsys.InputHistory{ Record(kind PromptKind, text string); Prev(kind) (string, bool); Next(kind) (string, bool); Reset() }`, `Session.History2()` is not needed: the ring lives on `Session.Input()` returned by Wave 0 |
| Suggestions | WP6 renders the popup | `Registry.Suggest(prefix string, max int) []Suggestion{Name, Alias, Kind}` (extends `Complete`: commands, aliases, variables from `Vars`, mid-string) |
| Text window | | `Workspace.ToggleTextWindow()`; renders `Session.History` (cap raised via `cmdsys.HistoryLimit`, e.g. 2000) in an overlay, selectable, PgUp/PgDn/Home/End |
| Key actions | WP3/WP5 add theirs | `ui/keys.go` exposes `bind(keys, action, ctx string, handler)`; each WP registers its own keys from its `init*` (so `Bindings()` stays WP1's file but is built from registrations) |

| Consumes | From |
|---|---|
| `TextInput.OnKeyDown` pre-hook, `SelectAll`, `Caret`, paste vs type distinction | WP6 (5.1) |
| `KeyDownEvent.Repeat` | WP6 (5.2) |
| `Session.Modified`, QSAVE/SAVEAS/QUIT commands | WP5 |
| Selection `All` | WP2 (`Session.SelectAll()`) |

**Rules.** Key contexts: the command-line field gets key context `cmdline`; canvas-level bindings
use `cad`; a binding for Delete/Up/Down/Tab/Ctrl+A is registered in `cmdline` only, so it cannot
fire while the agent panel's field has focus.

**Tests.**
Headless (`ui/keys_test.go` etc., harness `open(t)`):
F1 prints the command list; F2 opens and closes the text window and `Ctrl+Shift+A` the agent panel;
Delete erases a selection with an empty line and deletes a character with text; Ctrl+A selects all;
Up/Down recall (commands at `Command:`, values at a prompt, per kind); Tab/suggestion pick; Ctrl+Z
at LINE removes the last segment; F-key repeat dispatches once; Ctrl+O/N/S/Shift+S/Q reach the
right commands; no binding fires while the agent field is focused (regression for the context rule).
Real window (`.gui`): `keys.gui` holds F7 for 1 s and expects the grid toggled exactly once
(`expect-var`), types `L`, Up, Up and compares the history; `textwindow.gui` shots F2 open/closed
(`expect-diff/same`); Delete after a click selection (`expect-count`).

**Docs.** `command-line.md` (shortcut table, F-keys, history, suggestions, Next steps trimmed),
`agent-panel.md` (F2 -> Ctrl+Shift+A), `testing.md`; hand the key table for the aids to WP3's notes.

**Effort.** M-L.

---

### WP2. Selection and pointer

**Goal.** AutoCAD selection paradigm: click-click window/crossing, lasso, preview, `Select objects:`
options, select all/previous, double-click, `PICKADD`, cursor size and pickbox, drag capture on the
canvas side.

**Owns (exclusive):**
`ui/canvas_input.go`, `ui/paint_selection.go`, `ui/selection.go` (new: pending-window and lasso
state), `cmdsys/select.go` (new: `SelectEngine`), `cmdsys/selectopts.go` (new: typed `W/C/ALL/L/P/R/A/F/WP/CP`),
`cmdsys/pick.go` (moves `Pick`, `SelectBox`, `touchesBox` out of `snap.go`; WP3 keeps `snap.go`),
`cmdsys/lasso.go` (new: polygon/fence tests), tests `ui/selection_test.go`, `cmdsys/select_test.go`,
`tools/gui/scenarios/selection.gui`; docs `features/canvas.md` (Pointer, selection boxes, cursor),
selection section of `features/drawing-and-editing.md` handed to WP5 as a note (WP5 owns the file).

**Does not touch.** `Workspace.wheel` (D-F), `ui/canvas.go` view code, grips.

**API / hooks.**

| Provides | To | Signature |
|---|---|---|
| Selection engine | WP1 (Ctrl+A), WP4 (grips reuse crossing for STRETCH), WP5 | `Session.SelectAll()`, `Session.SelectPrevious()`, `Session.SelectRegion(Region, mode SelectMode, remove bool)` where `Region` is a rect or polygon; `SelectMode` is Window, Crossing, Fence |
| Pending window | UI | `Session.BeginWindow(p geom.Vec2)`, `Session.UpdateWindow(p)`, `Session.EndWindow(p, remove bool)`; `Session.PendingWindow() (a, b geom.Vec2, ok bool)` so headless tests drive click-click without pixels |
| Selection prompt keywords | | `Prompt` gains no new field: `selectopts.go` intercepts typed `W/C/ALL...` when `Accepts&InputSelection != 0`, using `AddTextHandler` |
| Preview | | `Session.PreviewSelection() []doc.Handle` for the painter (hover or in-progress window) |
| Variables | | `PICKADD`, `PICKAUTO`, `PICKDRAG`, `PICKBOX`, `SELECTIONPREVIEW`, `CURSORSIZE` registered in `selectvars.go` |

The default click path stays: point prompt answered first, otherwise pick; an empty pick **starts** a
pending window (D-B). Hook order puts grips before this, so a click on a grip never starts a window.

| Consumes | From |
|---|---|
| Mouse capture, cursor style | WP6 (5.5, 5.6) |
| Grip hit-test first in the hook order | WP4 |

**Tests.**
Headless: click empty, click again L->R selects entirely-inside; R->L selects touching; a second click
at the same place selects nothing and keeps the set; Esc cancels the pending window; click at
`Select objects:` then Enter; `ALL`/`L`/`P`/`W`/`C` typed at the prompt (scripts, `commands_test.go`
style); Shift removes; `PICKDRAG` 1 press-drag rectangle still works (regression for D8); a press that
travels 3 px is a click, 5 px a drag; mouse-up outside the canvas ends a drag (after 5.6);
double-click focuses the properties panel. Real window: `selection.gui` does click-click both
directions with `shot` + `expect-diff`, `expect-selected N` for each, a lasso via `drag-path`.

**Docs.** `canvas.md`; supersede note in `decisions.md` is Wave 0's; backlog line for lasso stays
open until the real-window check passes.

**Effort.** M.

---

### WP3. Drafting aids: snaps, ortho, polar, tracking, grid snap, overrides

**Goal.** F8/F9/F10/F11 and the typed and temporary overrides: complete the cursor pipeline.

**Owns (exclusive):**
`cmdsys/snap.go` (existing: keep `FindSnap`; `Pick`/`SelectBox` leave for WP2), `cmdsys/snapmodes.go`
(new: Perpendicular, Tangent, Node, Extension, Parallel, From, Tracking-point, Geometric centre),
`cmdsys/aids.go` (new: pipeline stages: override, snap, grid snap, polar, ortho, otrack),
`cmdsys/override.go` (new: one-shot typed overrides, `END,MID` lists, temporary override flags),
`cmdsys/polar.go`, `cmdsys/otrack.go`, `cmdsys/aidsvars.go` (`OSMODE`, `AUTOSNAP`, `ORTHOMODE`,
`POLARMODE`, `POLARANG`, `POLARADDANG`, `SNAPMODE`, `SNAPUNIT`, `SNAPANG`, `GRIDMODE`, `GRIDUNIT`),
`cmdsys/commands/aids.go` (new: `GRID`, `SNAP`, `ORTHO`, `OSNAP`, `POLAR`, `DSETTINGS`),
`ui/statusbar.go`, `ui/paint_aids.go`, `ui/aids.go` (new: toggles, grid state moved onto the
session variable), tests `cmdsys/aids_test.go`, `cmdsys/snapmodes_test.go`,
`ui/aids_test.go`, `tools/gui/scenarios/aids.gui`; docs `tech/command-system.md` (Object snaps,
Input pipeline), the aids rows of `features/command-line.md` as notes to WP1.

**Does not touch.** `ui/canvas.go`, selection code, gpui.

**API / hooks.**

| Provides | To | Signature |
|---|---|---|
| Pipeline | every consumer of `Session.Cursor()` | stages 10 override, 20 snap, 30 grid snap, 40 polar/ortho, 50 otrack, in `aids.go`, via Wave 0's `AddStage` |
| Overrides | WP6 (Shift+right-click menu, status right-click) | `Session.ArmOverride(SnapMode)` one-shot; `Session.SetTemporary(TempOverride, on bool)` for hold-keys (`TempOrtho`, `TempOsnap`, `TempSnap`, `TempPolar`, `TempOtrack`) |
| Visual state | `paint_aids.go` | `Session.Aids()` returns tracking lines, polar vector, snap hit, tooltip text (`Snap().Mode.String()`, `Polar: 12.5 < 45°`) |
| Tooltip text | WP6 | `Session.CursorTip() string`, the single source for the cursor tooltip |

Rules: ortho and polar are mutually exclusive; typed coordinates are never changed; the Shift override
reads `Window.Modifiers()` (5.2) on pointer events and on `ModifiersChanged`; until then it is driven
from `MouseMoveEvent.Modifiers`, which the platform already fills.

**Tests.**
Headless: grid-snap lands on `SNAPUNIT` multiples; polar locks at multiples of `POLARANG` within the
tolerance and prints `Polar: ...`; Shift override inverts ortho for the move only; typed `END` at a
point prompt arms one pick and then clears; `END,MID` list; PER/TAN/NOD/EXT/PAR results against fixture
geometry; priority and tie rules (document in `command-system.md`); F-key variable round trip
(`OSMODE` typed sets the bits); `GRID`/`SNAP`/`ORTHO` commands via `Session.Script`.
Real window: `aids.gui`: `hold shift` while moving with ORTHO on and a LINE rubber band, `shot` +
`expect-diff`; F9 snaps a click to the grid (`expect-count` and the saved DXF coordinates);
`keydown F3` held vs tapped.

**Docs.** `command-system.md`, status bar rows to `canvas.md` via WP2 note, backlog G10 update by
Wave 3.

**Effort.** L (snap modes M, polar M, otrack L).

---

### WP4. Grips and noun-verb editing

**Goal.** Grips on selected entities and the hot-grip edit cycle; STRETCH; editable properties
(basic).

**Owns (exclusive):**
`cmdsys/grips.go` (new: `Grip{Entity, Index, Pt, Kind}`; `GripsOf(e doc.Entity) []Grip`;
`MoveGrip(e, i, to geom.Vec2) (doc.Entity-modifying)` for LINE, ARC, CIRCLE, polyline, RECTANG),
`cmdsys/gripedit.go` (new: state machine STRETCH->MOVE->ROTATE->SCALE->MIRROR, Ctrl copy, Shift multi-hot,
Esc, one `Txn`), `cmdsys/gripvars.go` (`GRIPS`, `GRIPSIZE`, `GRIPCOLOR` family, `GRIPOBJLIMIT`),
`cmdsys/commands/stretch.go` (new: `STRETCH`/`S`, crossing selection, `Displacement`),
`ui/grips.go` (hit test, hover, hook), `ui/paint_grips.go`, `ui/sidepanel.go` (properties editing,
after WP6's popup/field components), tests `cmdsys/grips_test.go`, `cmdsys/commands/stretch_test.go`,
`ui/grips_test.go`, `tools/gui/scenarios/grips.gui`; docs: new `features/grips.md` (registered by
the Wave 3 index pass), properties editing text to WP2's `canvas.md` as a note.

**Does not touch.** `doc` (entity internals are read through `Geometry()` and modified through
`Txn.Modify`; if a type needs vertex access it is added in `cmdsys/grips.go` with a type switch, per
the package rule that core packages other than `doc` own their own views); selection engine (calls it).

**API / hooks.**

| Provides | To | Signature |
|---|---|---|
| Hit test | UI | `Session.GripAt(p geom.Vec2, tolModel float64) (Grip, bool)`, `Session.Grips() []Grip` (for the selection, capped by `GRIPOBJLIMIT`) |
| Hot-grip lifecycle | UI | `Session.GripPress(g Grip, shift bool)` puts the session into the grip-edit command (a normal `Command`, so prompts, Esc, the history and one undo step all work); mouse moves use `Hover`, the click answers the point prompt |
| STRETCH | WP5 aliases `S` | registered in `registerStretch`; alias line added by WP5 |

The grip edit **is a command** (`GRIP_STRETCH`... shown as `** STRETCH **` in the prompt), so every
existing mechanism applies and the UI needs only the hit test and the painter.

| Consumes | From |
|---|---|
| Pointer hook ahead of selection | Wave 0 |
| Snaps while hot | WP3 pipeline (free: it is just `Cursor()`) |
| Ctrl state during stretch | WP6 `Window.Modifiers()` |

**Tests.**
Headless: grips of each entity type (counts and positions); click a line's endpoint grip then a point
moves only that end; Space cycles the mode names; Ctrl copy leaves the original; Esc leaves no change;
one undo step per edit; locked/off layer entities show no grips; `STRETCH` crossing window moves
vertices inside, translates entities fully inside. Real window: `grips.gui`: click a line, `shot`
(grips visible), click a grip, click a point, `expect-count LINE 1` and DXF coordinates.

**Docs.** `features/grips.md` (new), a Next steps edit in `drawing-and-editing.md` handed to WP5,
backlog G9.

**Effort.** L.

---

### WP5. Commands, aliases, transparent commands, system variables, files

**Goal.** The command set and the command-line machinery that is not a key or a mouse gesture.

**Owns (exclusive):**
`cmdsys/commands/aliases.go`, `cmdsys/commands/view.go` (new: `PAN`, ZOOM options, view stack, `REGEN`,
`REDRAW`), `cmdsys/commands/array.go`, `cmdsys/commands/polygon.go` (new: `POLYGON`, `DONUT`, `POINT` if an entity
exists), `cmdsys/commands/matchprop.go`, `cmdsys/commands/clip.go` (COPYCLIP/CUTCLIP/PASTECLIP/COPYBASE),
`cmdsys/commands/files.go` (QSAVE, SAVEAS, CLOSE, QUIT/EXIT, units readout), `cmdsys/commands/sysvar.go`
(`SETVAR`, a variable typed at `Command:` prints/sets it, `SETVAR ?`), `cmdsys/commands/undo.go`
(`UNDO n`, Mark/Back/BEgin/End, `OOPS`), `cmdsys/commands/draw.go` and `modify.go` option work (ARC
options, CIRCLE 2P/3P/TTR, PLINE arc segments, COPY/ROTATE/SCALE options, TRIM Fence/eRase/Undo...),
`cmdsys/transparent.go` (new: nested frames), `cmdsys/dirty.go` (new: `Session.Modified()`),
`cmdsys/clipboard.go` (new: in-process entity clipboard), tests beside each, `commands_test.go`
additions in `cmdsys/commands/*_test.go` (new files only), `tools/gui/scenarios/commands.gui`; docs
`features/drawing-and-editing.md` (the single owner: command table, aliases, selection section from
WP2 and Next steps from WP4/WP3 as notes), `features/dxf-files.md` (QSAVE rules).

**Does not touch.** `ui/*`, `internal/gpui`, `cmdsys/session.go` (extension points exist from Wave 0).
`cmdsys/commands/trim.go` and `construct.go` only for the option-parity pass, in dedicated commits,
never at the same time as another agent edits them (WP4 touches neither).

**API / hooks.**

| Provides | To | Signature |
|---|---|---|
| Transparent frames | all prompts | `type Transparent interface{ Transparent() bool }` marker on view/settings commands; `Session.Submit` text handler for a leading `'` pushes the frame, the prompt shows `>>`, finishing pops it and re-asks the parent prompt; transparent commands may not open a transaction |
| Dirty state | WP1 (Ctrl+S, Ctrl+Q, Ctrl+N), UI title | `Session.Modified() bool`, `Session.MarkSaved()`; `doc.Drawing` change counter (D-I) |
| QSAVE | WP1 keys | `Session.Run("QSAVE")`: `FilePath()` set -> saves silently; else prompts like SAVEAS |
| Clipboard | WP1/WP6 | `Session.Clipboard() *Clip` (entities + base point), `Session.PasteAt(p)` |
| Variables | all | `SETVAR`, typed variable names, persistence through `settings/` once it exists (read its API then) |
| Aliases | all | `Aliases` extended per section 7 (each alias in the same commit as its command) |

**Tests.**
`Session.Script` end-to-end tests per command (the existing style); `'ZOOM` inside LINE resumes the
LINE at the same prompt and base; `U` after QSAVE marks modified; Zoom Previous stack; ARRAY counts
and spacing; UNDO Mark/Back; clipboard round trip with base point; every new alias resolves; unknown
`SETVAR` name message. Real window: `commands.gui` for PAN by drag after `'P`, QSAVE producing the
file (`-save-on-exit` path).

**Docs.** `drawing-and-editing.md` (owner), `dxf-files.md`, `testing.md` row via WP1 note.

**Effort.** L; the option-parity pass is the long tail and can run for as long as wanted without
blocking anything.

---

### WP6. gpui additions and application chrome (popups, tooltips, dynamic input)

Two halves, one owner, **contract first**.

**WP6a (gpui).** Owns `internal/gpui/**` (this package alone edits it; per `internal/gpui/AGENTS.md`
`api-contract.md` is edited *before* the code), including `ebitenplatform`. Delivers the items of
section 5 as small separate commits, each with headless tests in `internal/gpui/*_test.go`.

**WP6b (application chrome).** Owns `ui/chrome.go`, `ui/contextmenu.go`, `ui/tooltip.go`, `ui/dyninput.go`,
`ui/paint_dyn.go`, `ui/filepicker.go` (D-E), `ui/toolbar.go` (tooltips and G21 hints),
`ui/suggestpopup.go`; tests `ui/chrome_test.go`, `ui/dyninput_test.go`, `tools/gui/scenarios/chrome.gui`;
docs `tech/ui-framework.md`, `internal/gpui/_docs/api-contract.md`, `features/canvas.md` additions as
notes to WP2 (WP2 owns the file).

**API / hooks.**

| Provides | To | Signature |
|---|---|---|
| Context menu | the right-click pointer hook (hook order 0) | `ws.openMenu(at gpui.Point, items []MenuItem)` with `MenuItem{Label, Shortcut string; Enabled bool; Run func(); Sub []MenuItem}`; builders `ws.menuForState()` (idle, selection, command with options) and `ws.osnapMenu()` (calls `Session.ArmOverride`) |
| Cursor tooltip | WP3 text | renders `Session.CursorTip()` and the prompt at the pointer (step 1 of D-D); step 2 `dyninput` fields call `Session.Submit` |
| Suggestion popup | WP1 | renders `Registry.Suggest` anchored above the field, keyboard owned by WP1's hooks |
| File picker | WP5 QSAVE/OPEN | `ws.pickFile(mode, done func(path string))`, wired in `Workspace.SetFiles`-style hook; fallback: typed path |

**Tests.** gpui: per item in section 5. UI: right-click at `Command:`, with a selection, inside LINE
shows the matching items; choosing `Close` types the keyword; click outside dismisses and does not hit
the canvas underneath; Esc dismisses; Shift+right-click arms `END` (`expect-var` / next pick lands on
the endpoint). Real window: `chrome.gui` with `shot` of each menu and the tooltip.

**Effort.** L (menu M, tooltip S, dyn-input step 2 L, file picker M, gpui items S-M each).

---

## 5. What gpui must add (contract-first)

Everything below goes to `internal/gpui/_docs/api-contract.md` *first*; the code follows the contract
(`internal/gpui/AGENTS.md`). Today the framework has: key names including `f1`..`f12`, `up`, `down`,
`tab`, `delete`; `Modifiers` = Shift, Control, Alt, ModPlatform, Function; `MouseDownEvent.ClickCount`;
`KeyUpEvent`; `ModifiersChangedEvent` (emitted by ebiten, dropped by the window); `Div.OnKeyDown/OnKeyUp`;
`TextInput` with `OnSubmit/OnEscape/OnChange`. It has **no**: overlay/deferred layer, tooltip, cursor
style, mouse capture, modifier state query, key-repeat flag, file dialog, Tab focus traversal.

| # | Addition | Proposed contract | Used by | Notes |
|---|---|---|---|---|
| 5.1 | `TextInput` hooks | `OnKeyDown(func(*KeyDownEvent, *Window, *App) (handled bool))` run **before** the built-in editing keys; `SelectAll()`, `Caret() int`, `SetCaret(int)`, `Insert(string)`; `OnChange` receives a source (`Typed`/`Paste`/`Programmatic`) so Space-submit ignores pastes | WP1 (Up/Down/Tab/Delete/Ctrl+A), D9 fix | The hook must run when the input is focused only |
| 5.2 | Key repeat and key/modifier state | `KeyDownEvent.Repeat bool` (ebiten sets it when duration > 1); `KeyBinding` ignores repeats unless `Repeat: true` is set on the binding; `Window.Modifiers() Modifiers` (maintained from `ModifiersChangedEvent` and from every pointer/key event); `Window.IsKeyDown(key string) bool` (from KeyDown/KeyUp); `Div.OnModifiersChanged(func(Modifiers, *Window, *App))`; all state cleared when the window loses activation | WP1 (F-key repeat), WP3 (Shift/F-key hold overrides), WP4 (Ctrl copy) | Fix `Window.dispatchEvent` which currently ignores `ModifiersChangedEvent`. Left/right Shift distinction not needed |
| 5.3 | Overlay layer and popup positioning | `Window.Overlay(id string, build func(*Window, *App) IntoElement) (close func())` rendering above the root in window coordinates, in priority order; `Anchored{Position Point, Corner Corner, FitWindow bool}` element (GPUI's `anchored`); `Div.Occlude()` so hit-testing stops at the overlay and the canvas below gets no mouse events; `OnClickOutside(func)` | WP6b menus, suggestions, tooltip near cursor, file picker | GPUI has `deferred` + `anchored`; port both (the code's note says capture listeners were not ported, which is fine here) |
| 5.4 | Tooltips | `Div.Tooltip(func(*Window, *App) IntoElement)` with hover delay (500 ms), dismissed on press/scroll/leave; `Window.SetCursorTip(text string, at Point)` is *not* needed (use 5.3 for the cursor tooltip) | G21 toolbar hints, rollover tips | Delay uses the existing timer API |
| 5.5 | Cursor style | `Window.SetCursorStyle(CursorStyle)` with `CursorArrow`, `CursorCrosshair`, `CursorNone`, `CursorPointer`, `CursorResizeH/V`, `CursorGrab`; ebitenplatform maps to `ebiten.SetCursorMode/SetCursorShape`; per-element default `Div.Cursor(CursorStyle)` | G15 (hide the arrow over the canvas), grips (hover cursor), pan (grab) | Browser build: CSS cursor |
| 5.6 | Mouse capture | `Window.CaptureMouse(id ElementID)` / `ReleaseMouse()`: while captured, `MouseMove`, `MouseUp` and `Scroll` go to that element even outside it; auto-release on mouse-up and on deactivation; headless `HeadlessWindow` supports it | WP2/WP4 drag gestures, middle-drag pan (defect 2) | The `active` press path in `window.go` is the natural seed |
| 5.7 | Clipboard routing | `ebitenplatform` dispatches `ctrl-c/x/v` as ordinary `KeyDownEvent`s **first**; only if no binding/handler consumed it does it perform the text clipboard operation on the focused input. New `App.Clipboard()` returning text now and a typed item (`ClipboardItem{Text, Custom map[string][]byte}`) later | WP1/WP5 entity clipboard | Today `startClipboard(...)` runs before dispatch |
| 5.8 | File dialogs | Contract only for now: `App.PromptForPath(opts FileDialogOptions) (path string, ok bool)` asynchronous via callback; ebitenplatform returns "unsupported", so the application's in-app picker (WP6b) is the implementation; a native backend may be added later per OS | G18, Ctrl+O/S | Decide D-E first |
| 5.9 | Tab focus traversal | `Tab`/`Shift+Tab` move focus between focusable elements unless the focused element handles `tab` (the command line does: suggestions) | Accessibility, dialogs | Low priority; needed by the file picker |
| 5.10 | Window title / dirty | `Window.SetTitle(string)` (verify it exists; the unsaved marker uses it) | Ctrl+S flow | Probably exists |

`HeadlessWindow` must support every item (overlay hit-testing, capture, cursor style recorded,
modifiers) so the UI tests in section 4 run without a display.

---------------------------------------------------------------------------------------------------

## 6. Ordering, ownership and collision rules

1. **Gate.** No package edits a hot file (section 1) until its owner has committed. Until then a
   package may create brand-new files and tests only. Wave 0 and WP6a start the moment the gpui
   edits land; WP6a is also where `api-contract.md` is edited, which today carries another agent's
   uncommitted change.
2. **One owner per file**, listed per package above. A package that needs a change in someone
   else's file stops and sends a note (AGENTS.md "Parallel agents"). The shared files are
   `Workspace` (Wave 0 hooks), `cmdsys/session.go` (Wave 0 stages), `drawing-and-editing.md` (WP5),
   `canvas.md` (WP2), `command-line.md` (WP1), `command-system.md` (WP3), `decisions.md` and
   `backlog.md` (Wave 0, updated again in Wave 3), `INDEX.md` (generated; Wave 3 runs
   `go run ./tools/docs index`).
3. **Doc hand-off.** The duty (AGENTS.md) says a change updates the documents it touched in the same
   commit. A package updates the documents *it owns* in its own commit; for a document it does not
   own it commits a one-paragraph note in its report and the owner folds it in. Wave 3 verifies
   with `go run ./tools/docs touched` and `lint`.
4. **Registration, not editing.** Packages add behaviour through `init*` functions, `Register*Vars`,
   `AddTextHandler`, `AddStage`, `addPointerHook`, `addOverlay`, `addChrome`, `Registry.Register`,
   each in the package's own file.
5. **Verification per package** (AGENTS.md rule 6): `gofmt -l .` empty, `go vet ./...`,
   `go test ./...`, `go run ./tools/docs lint`, `just check-agentmanager`, plus
   `CADLITE_GUI_TEST=1 go test -count=1 ./tools/gui` on a machine with Xvfb for the package's
   `.gui` scenario. Rendering changes: `-png` before and after, looked at. The agent shell has no
   display, so every package's report must say which interactive behaviour ran only in headless tests.
6. **Test file names are per package** (`keys_test.go`, `selection_test.go`, `aids_test.go`,
   `grips_test.go`, `chrome_test.go`) so no two packages edit one test file; shared helpers
   (`harness`, `open`) live in `ui/export_test.go` (Wave 0, then frozen).
7. **Rollout order if agents are scarce.** Wave 0, then WP1 + WP3 + WP6a, then WP2, WP5, then WP4 and
   WP6b. The first demonstrably "AutoCAD-like" milestone is WP1 (Delete, Ctrl+A, Up/Down, F2,
   F1, QSAVE) + WP3 (F9, F10, typed END/MID...) + WP2 click-click.

---------------------------------------------------------------------------------------------------

## 7. Appendix

### 7.1 Alias diff (existing commands first)

Cad-lite today (`cmdsys/commands/util.go`, `construct.go`, `trim.go`, `agent.go`): `L PL C A REC E M CO CP
RO SC U Z LA DI LI AA MEA` plus `MI O F CHA TR EX BR J X` and `AGENT`.

| Alias | AutoCAD command | Cad-lite |
|---|---|---|
| L, PL, C, A, REC, E, M, CO, CP, RO, SC, MI, O, F, CHA, TR, EX, BR, J, X, Z, LA, DI, AA, MEA, LI, U | same | present |
| LS | LIST | missing (add, S) |
| ID | ID (a command, no alias needed) | present as a command |
| REDO | no alias (Ctrl+Y) | present |
| P | PAN | missing: needs PAN (WP5) |
| S | STRETCH | missing (WP4 command, WP5 alias) |
| AR | ARRAY | missing (WP5) |
| POL | POLYGON | missing (WP5) |
| EL | ELLIPSE | blocked (G7) |
| SPL | SPLINE | blocked (G7) |
| PO | POINT | missing (needs a point entity) |
| DO | DONUT | missing (WP5) |
| XL | XLINE | missing (G7) |
| H, BH | HATCH | blocked (G7) |
| T, MT | MTEXT | blocked (G7) |
| DT | TEXT | blocked (G7) |
| B, I, W | BLOCK, INSERT, WBLOCK | blocked (G7) |
| D, DLI, DAL, DRA, DDI, DAN, DCO | dimension commands | blocked (G7) |
| MA | MATCHPROP | missing (WP5) |
| PR, CH, MO | PROPERTIES | missing (WP1 toggles, WP4 edits) |
| PE | PEDIT | missing (WP5) |
| LEN | LENGTHEN | missing (WP5) |
| AL | ALIGN | missing (WP5) |
| DIV, ME | DIVIDE, MEASURE | missing (WP5) |
| LT, LTS, LW | LINETYPE, LTSCALE, LWEIGHT | blocked (G7) |
| RE, R | REGEN, REDRAW | missing (WP5) |
| OS, SN, GR, DS | OSNAP, SNAP, GRID, DSETTINGS | missing (WP3) |
| UN | UNITS | missing (WP5) |
| OP | OPTIONS | missing (later; the settings dialog) |
| PU | PURGE | missing (P3) |
| ST | STYLE | blocked (G7) |
| QSAVE, SAVEAS, CLOSE, QUIT, EXIT | no alias | missing (WP5) |

Also: user-editable aliases. AutoCAD reads `acad.pgp`; Cad-lite keeps the same format
(`Registry.LoadAliases`), so a settings-directory `aliases.pgp` override (P3, S) fits the
`settings/` work.

### 7.2 AutoCAD facts to verify before acceptance criteria are written

Check each in an installed AutoCAD 2024/2025 (or its documentation) and record the answer in the
owning feature document:

1. The exact "Temporary Override" table in the default CUI (which keys, hold vs toggle): Shift for
   ortho/polar, F3/F9/F10/F11 held, Shift+A, Shift+S, Shift+X.
2. `AUTOSNAP` bit layout (1 marker, 2 tooltip, 4 magnet, 8 polar, 16 otrack, 32 tracking tooltips;
   default 63).
3. `AUTOCOMPLETEMODE` default (15) and the meaning of each bit; whether Enter runs the typed text or
   the highlighted suggestion.
4. Single versus double Esc in each state (idle with a selection, typed text, active command, hot
   grip, transparent command).
5. Lasso direction rule (clockwise window / anticlockwise crossing) and Space cycling.
6. Whether Ctrl+wheel is horizontal pan and Shift+wheel vertical pan in the default settings.
7. `SHORTCUTMENU` default (11) and the "no options -> Enter" rule; whether time-sensitive right-click
   is on by default.
8. `F6` (Dynamic UCS) versus `Ctrl+D` (COORDS) in 2024.
9. `ZOOMFACTOR` 60 step size per notch (to calibrate the 1.087 per notch of the in-progress wheel
   code).
10. Ctrl+A with text typed on the command line.
11. Which `Ctrl+Shift+...` combos are free (`Ctrl+Shift+A` for the agent panel in D-A).
