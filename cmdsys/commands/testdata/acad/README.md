# AutoCAD acceptance corpus

Small AutoCAD scripts (`NAME.scr`) with the drawing state AutoCAD 2024/2025 leaves after running
them (`NAME.expect`). They are the acceptance criteria for the command work in
[`_docs/inbox/autocad-commands.md`](../../../../_docs/inbox/autocad-commands.md): a command is
AutoCAD-compatible when the scripts that exercise it pass.

The expectations were derived from AutoCAD's documented behaviour, **not yet run in an installed
AutoCAD**. Every `.expect` says so (`source:`), and the ones that rest on a fact I am unsure of
carry a `# (verify)` line. Run each script once in a real AutoCAD, compare, and fix the `.expect`
before treating a failure in go-draw as a go-draw bug.

## Running

```
GODRAW_ACAD_CORPUS=report go test ./cmdsys/commands -run TestACADCorpus -v   # log every outcome
GODRAW_ACAD_CORPUS=1      go test ./cmdsys/commands -run TestACADCorpus      # regression tracker
GODRAW_ACAD_CORPUS=strict go test ./cmdsys/commands -run TestACADCorpus      # every script must pass
```

Without the variable the test is skipped, so `go test ./...` stays green. The harness is
`cmdsys/commands/acad_corpus_test.go`. Mode `1` compares each script's outcome with the
`go-draw:` line of its `.expect` (`pass` or `fail`), so a script that starts passing, and a
regression, both fail the test until the declaration is edited on purpose. When a command work
package makes scripts pass, flip their `go-draw:` lines in the same commit.

In real AutoCAD: start a new drawing from `acadiso.dwt`, run `_preamble.scr`, then the script
(`SCRIPT`, FILEDIA 0), then inspect the drawing against `.expect`.

## Script files

Real AutoCAD script syntax:

- Each line is one response; a space also separates responses; **a blank line is Enter**.
- `;` starts a comment to the end of the line. A line that is only a comment is ignored.
- No trailing spaces on a line (a trailing space is an extra Enter in AutoCAD).
- The file's last newline terminates the last line. To finish a command with Enter, the file ends
  with a blank line.
- Typed points are exact, so results are zoom-independent. Where a script picks an object it types
  a point that lies exactly on it (a pick has to touch the object whatever the zoom). `L` is the
  last object created, `ALL` everything; both avoid depending on the pickbox size.
- Scripts start from an empty drawing, layer `0` current, and assume the preamble: object snap,
  ortho, snap and polar off, dynamic input off, the default values of FILLETRAD, CHAMFERA/B,
  CIRCLERAD, OFFSETDIST, COPYMODE reset. The go-draw harness sets `SnapOn=false` and
  `Ortho=false` for the same reason.
- A script leaves no command running. The harness fails a script that ends inside a command.

`Session.Script` splits on spaces and newlines, so the harness does two things first: it removes
`;` comments (and the lines they leave empty), and it drops exactly one final newline, because
`Script` reads a newline as a separator where a script file reads it as a terminator. Doing the
same inside `Session.Script` is a gap listed in the specification (the SCRIPT work package).

## Expect files

Line oriented; `#` starts a comment line; blank lines are ignored; fields are separated by
spaces; numbers match to 1e-6 (angles in degrees, circularly).

```
go-draw: pass|fail           the outcome today; the regression tracker reads it
source: ...                  provenance note (ignored)
line   x1,y1 x2,y2 [layer=NAME] [color=N|bylayer]
circle cx,cy r [layer=NAME] [color=N|bylayer]
arc    cx,cy r a0 a1 [layer=NAME] [color=N|bylayer]       counterclockwise from a0 to a1, degrees
lwpolyline open|closed x,y[/bulge] x,y[/bulge] ... [layer=NAME] [color=N|bylayer]
count [CLASS] N              total entities, or of one class (LINE CIRCLE ARC LWPOLYLINE)
layer NAME [color=N] [on|off] [locked|unlocked]   NAME exists with these properties
current-layer NAME
var NAME VALUE[,VALUE...]    a system variable after the script (numbers to 1e-4, other text exact)
message TEXT                 TEXT appears (case-insensitive substring) in the command history
selected N                   N objects are in the selection set
active none|NAME             the running command
```

Rules:

- **Entities match as a set.** Each entity line needs a distinct actual entity of that class, and
  every actual entity needs a line: extra entities fail. `count` lines are extra checks.
- A line matches in either direction. A closed polyline matches from any start vertex and, when
  neither side has bulges, in either direction; an open one matches reversed when it has no
  bulges. Bulges are AutoCAD's (positive = counterclockwise); the bulge belongs to the segment
  leaving that vertex.
- `layer=` and `color=` are checked only when given. `color=bylayer` is 256.
- Layer `0` always exists. `layer` lines check the layers they name; other layers are allowed.
- A `var` the harness cannot read fails the script with "system variable X is not implemented".
  Today the harness reads `CLAYER`, `ORTHOMODE` and `VIEWCTR`; extend `acadVar` as variables land.
- Not checkable yet: polyline widths, linetype, lineweight, text, hatch (the document has none).

## Scripts

Outcome today is `go-draw:` in each `.expect` (3 of 40 pass: scripts that exercise only what exists).

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

