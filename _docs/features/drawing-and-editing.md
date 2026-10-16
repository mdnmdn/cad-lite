---
id: feat-drawing-and-editing
title: Drawing and editing commands
kind: feature
status: current
summary: Every built-in command with its prompts and aliases, how selection, object snaps and ortho behave for the user, and undo.
read_when: you are adding or changing a command, an alias, or what a click does while a command runs
updated: 2026-10-16
verified: 2026-10-16
code_anchors: [i18n/command.go, i18n/snap.go, i18n/locales/en.toml, i18n/locales/it.toml, cmdsys/commands/group.go, cmdsys/commands/block.go, cmdsys/commands/attribute.go, cmdsys/commands/text.go, cmdsys/commands/mtext.go, cmdsys/polar.go, cmdsys/commands/array.go, cmdsys/commands/oops.go, cmdsys/commands/draw.go, cmdsys/commands/pline.go, cmdsys/commands/circle.go, cmdsys/commands/arc.go, cmdsys/commands/arcgeom.go, cmdsys/commands/rect.go, cmdsys/commands/polygon.go, geom/tangent.go, cmdsys/commands/modify.go, cmdsys/commands/view.go, cmdsys/commands/layer.go, cmdsys/commands/matchprop.go, cmdsys/commands/props.go, cmdsys/commands/purge.go, render/viewstack.go, cmdsys/commands/inquiry.go, cmdsys/commands/files.go, cmdsys/commands/undo.go, cmdsys/commands/register.go, cmdsys/commands/measure.go, cmdsys/commands/construct.go, cmdsys/commands/trim.go, cmdsys/commands/edges.go, geom/extend_helpers.go, cmdsys/commands/select.go, cmdsys/select.go, cmdsys/selectopts.go, cmdsys/snap.go, cmdsys/override.go, cmdsys/aids.go, cmdsys/cursor.go, cmdsys/commands/aids.go, cmdsys/commands/construct_corner.go, cmdsys/commands/textedit.go]
depends_on: [tech-command-system, tech-document-model]
review_cycle: monthly
---

# Drawing and editing commands

## Purpose

The drafter draws and edits geometry with AutoCAD's commands, prompts and aliases, using clicks
made exact by object snaps and ortho, or typed coordinates. Every command undoes as one step.

## Behaviour

### Commands

This table is the owner of the command list and the default aliases. The prompts and messages
quoted here are the English text. Each command also has Italian names, registered as aliases in every
language (`LINEA`, `CERCHIO`, ...); the `aliases` and `name` entries of `i18n/locales/it.toml` own them. A leading `_`
on a typed keyword forces the English one.

| Command | Aliases | Prompts and behaviour |
|---|---|---|
| LINE | L | First point (Enter continues from the last line or arc end, else `LASTPOINT`; after an arc it asks `Specify line length:` for a tangent line), then next points `[Undo]`, `[Close/Undo]` from the third point; each segment is a separate line, drawn and snapped to as soon as it is placed (as in AutoCAD). Enter ends; Undo removes the last segment from the drawing and the canvas; Close draws back to the first point and ends. Each segment records the end and direction for the next Enter-continue |
| PLINE | PL | Start point (prints `Current line-width is …`, starting from `PLINEWID`), then `[Arc/Close/Halfwidth/Length/Undo/Width]` (Close from the third vertex). Width and Halfwidth ask start then end width and apply to later segments (stored in `LWPolyline.Widths`; the renderer and the DXF codec do not use them yet); Length draws along the previous direction; Undo drops a segment. Arc mode `[Angle/CEnter/CLose/Direction/Halfwidth/Line/Radius/Second pt/Undo/Width]` draws a tangent arc to the point by default and the Angle, CEnter, Direction, Radius and Second pt sequences otherwise, as bulges; Line returns to line mode. One polyline is created when the command ends (Enter, Close or Escape) |
| CIRCLE | C | Center, then radius `[Diameter] <CIRCLERAD>` (Enter takes the remembered radius; a click gives the distance from the centre). `3P`, `2P` and `Ttr` at the first prompt: three points, two diameter ends, or two tangent objects (line, circle, arc or polyline segment) and a radius, choosing the solution nearest the picks (`geom.NearestTangentCircle`; `Circle does not exist.` when none) |
| ARC | A | Start `[Center]` (Enter continues tangent from the last line or arc end), then second point `[Center/End]` and end point (three points; collinear points draw nothing). Start-Center-End/`[Angle/chord Length]`, Start-End-`[Angle/Direction/Radius]` and Center-first sequences; a positive angle, chord or radius is the minor arc counter-clockwise, a negative one clockwise or major |
| RECTANG | REC | First corner `[Chamfer/Elevation/Fillet/Thickness/Width]`, then other corner `[Area/Dimensions/Rotation]`. Chamfer and Fillet cancel each other, Width, Elevation and Thickness stay set for later RECTANGs (session variables `DRAW_RECT*`). Dimensions and Area fix the size and the pick chooses the quadrant; Rotation takes an angle or `[Pick points]`. A closed polyline of 4 vertices, 8 with chamfer or fillet (bulge tan(π/8)); too big a chamfer or fillet draws a plain rectangle |
| POLYGON | POL | `Enter number of sides <POLYSIDES>` (3 to 1024), then centre `[Edge]` and `[Inscribed in circle/Circumscribed about circle] <I>` (remembered) and the radius: a click puts a vertex (inscribed) or an edge midpoint (circumscribed) at the point, a typed radius gives a horizontal bottom edge. `Edge` takes two endpoints and builds counter-clockwise from them. A closed polyline |
| GROUP, -GROUP | — | Enter a group name, or Enter for an unnamed group, then select members (a preselection is used). `List`, `Rename`, `Add`, `Remove`, `Selectable` and `Delete` manage existing groups; an entity may be in several groups. Default `PICKSTYLE` 1 selects connected selectable groups together, excluding hidden, locked and unsupported members. `PICKSTYLE` 0 selects individual objects. Membership editing uses individual selection |
| UNGROUP | — | Select objects; dissolve all groups containing them without erasing geometry |
| BLOCK, -BLOCK | B | Name, placement base point and objects (or preselection); replace the objects with one reference. Names match without regard to case. Existing names require BLOCKREDEFINE. A definition is shared by every reference in the drawing |
| INSERT, -INSERT | I | Existing block name or DXF library path, insertion point, positive uniform scale, rotation and nonconstant attribute values. Defaults are scale 1, rotation 0 and each attribute's default text. A library imports its named dependencies, layers and STYLE/LTYPE records only when placement finishes; cancellation or a validation error leaves the drawing unchanged. Conflicting definition names refuse import. Named insertion is agent-safe; paths are human-only |
| ATTDEF | — | Attribute tag, insertion question, default value, point and text height (default 2.5). Include the resulting definition when making a BLOCK. Tags are unique within a definition; the model and DXF also retain constant/invisible flags |
| ATTEDIT | — | Pick a reference, enter an existing nonconstant tag and its replacement value |
| BLOCKREDEFINE | — | Existing definition name, replacement base point and objects; update all direct and nested references in one undo step. Instances retain their attribute values and placements |
| WBLOCK | — | Definition name and DXF output path; export shared definitions, layers and style/linetype support for reuse through INSERT. Human-only |
| TEXT, DTEXT | DT | Prints `Current text style:  "Standard"  Text height:  2.5000  Annotative:  No  Justify:  Left`, then `Specify start point of text or [Justify/Style]:` (the message follows the justification: `center point`, `right endpoint of text baseline`, `top-left point`...), `Specify height <TEXTSIZE>:` (a number or a click's distance from the point; remembered in `TEXTSIZE`), `Specify rotation angle of text <last>:` (0 at first, then the last angle used) and `Enter text:` once per line: each line is one TEXT, the next starts 5/3 of the height below, and an empty line ends the command (all lines are one undo step). Spaces are part of the text, leading and trailing ones included, in scripts too; a script line is one text line. `Justify` asks `[Left/Center/Right/Align/Middle/Fit/TL/TC/TR/ML/MC/MR/BL/BC/BR]` and is remembered in `TEXTJUSTIFY`; `Align` asks the two baseline endpoints and no height or rotation (the height follows the length), `Fit` the two endpoints and the height (the width factor follows). `Style` asks `Enter name of text style or [?] <Standard>:` among Standard, the STYLE records of the file and the styles texts use (`Cannot find text style "X".` otherwise); it is kept in `TEXTSTYLE`. Enter at the first prompt continues one line under the last TEXT made, with its style, justification, height and rotation. The `%%d`, `%%p`, `%%c`, `%%%`, `%%nnn`, `%%u` and `%%o` codes draw degree, plus-minus, diameter, percent, a character by code, underline and overline. Deviation: AutoCAD types the text in place on the canvas; here it is typed at the command line (AutoCAD's own script form), each line entered draws at once, as the TEXT it is, and the preview shows a box where the next one goes |
| MTEXT | T, MT | The command-line MTEXT AutoCAD runs from scripts: `Specify first corner:`, then `Specify opposite corner or [Height/Justify/Rotation/Style/Width]:` (Height as TEXT's, Justify `[TL/TC/TR/ML/MC/MR/BL/BC/BR] <TL>` picks the attachment point of the box, Rotation the text direction, Style as TEXT's, Width a box width that skips the second corner, 0 for no wrapping), then `MText:` once per paragraph until an empty line (typed backslashes and braces are kept as text, escaped with `doc.MTextEscape`). One MTEXT is made: the box is as wide as the corners are apart along the rotation and is placed by its attachment point; the paragraphs wrap at word boundaries. Deviation: AutoCAD opens its in-place editor; `Line spacing` and `Columns` are missing, and inline formatting (fonts, heights, colours inside the text) is kept in the value but not drawn |
| TEXTEDIT | ED, DDEDIT | Opens the basic text editor for exactly one eligible preselected TEXT or MTEXT, otherwise asks `Select TEXT or MTEXT to edit:` with an unsnapped object pick. Only editable model-space text is accepted: off, frozen or locked layers, other entity classes and text inside blocks are excluded. The editor changes contents (blank is valid), height, rotation in degrees, justification or MTEXT attachment, an existing named style, and MTEXT wrapping width (0 for no wrapping). Apply modifies the existing handle in one TEXTEDIT undo step; Cancel, Escape and an unchanged Apply leave the document, dirty state and undo history unchanged. MTEXT shows plain paragraphs: property-only edits keep its original raw format codes byte for byte; changed contents replace them with escaped plain text, keeping literal backslashes, braces and paragraph breaks. TEXT keeps its single-line value and %% codes. Justification changes keep the anchor; Aligned/Fit keep their baseline constraint, a rotation change rotates that baseline about its first endpoint, and changing Aligned height resizes its span. No fonts or inline-formatting controls are offered. The command is human-only; agents use `update_text` |
| STRETCH | S | Prints `Select objects to stretch by crossing-window or crossing-polygon...`, then `Select objects [Crossing/CPolygon]:`: a click starts a crossing window, `C` a window by two corners, `CP` a polygon closed by Enter; several shapes accumulate and Enter ends the selection (none ends the command). Then `Specify base point or [Displacement] <Displacement>:` and `Specify second point or <use first point as displacement>:` (Enter uses the base point as the displacement). Vertices of a polyline and endpoints of a line or arc inside a shape move; an object wholly inside moves whole; a circle moves when its centre is inside, a TEXT or MTEXT when its insertion point is; an arc keeps its radius and side, so its centre follows. Objects on locked, frozen or off layers are left alone |
| MULTIPLE | — | `Enter command name to repeat:`, then runs that command again each time it completes, one undo step per run, until Escape. A command that finishes without asking anything is run once |
| ERASE | E | Erases the selection. The set is remembered for OOPS |
| OOPS | — | Restores the objects the last ERASE removed (layer and properties kept, new handles), once, as one undo step; `Nothing to restore.` when there is none |
| MOVE | M | Selection, `Specify base point or [Displacement] <Displacement>:`, then `Specify second point or <use first point as displacement>:`. Enter at the second point moves by the base point taken as a vector; `Displacement` (or Enter at the base prompt) asks `Specify displacement <0.0000, 0.0000, 0.0000>:` for a vector |
| COPY | CO, CP | As MOVE, with `[Displacement/mOde]` at the base point and `[Array]`, then `[Array/Exit/Undo] <Exit>` after the first copy: second points make one copy each until Enter or `Exit`; `Undo` takes the last copy back; Enter at the first second-point prompt copies by the base point as a vector. `mOde` `[Single/Multiple]` sets `COPYMODE` (Single ends after one copy). `Array` asks `Enter number of items to array <2>:` (the original counts) then `Specify second point or [Fit]:`: the second point is the spacing, or with `Fit` the position of the last item. Copies keep layer and properties |
| ROTATE | RO | Prints `Current positive angle in UCS:  ANGDIR=counterclockwise  ANGBASE=0` (read from the variables), selection, base point, `Specify rotation angle or [Copy/Reference] <0>:` (typed degrees, or a click measured from the base). `Copy` keeps the originals and asks again. `Reference` asks the reference angle (a number, or two points) and `Specify the new angle or [Points] <0>:` (a number, a click from the base, or `Points`: two points); the rotation is new minus reference |
| SCALE | SC | Selection, base point, `Specify scale factor or [Copy/Reference] <1>:` (typed, or the click's distance from the base). `Copy` keeps the originals. `Reference` asks the reference length (a number, or two points) and `Specify new length or [Points] <1>:`; the factor is new over reference. Zero or negative values are refused |
| ARRAY, ARRAYRECT, ARRAYPOLAR, -ARRAY | AR, -AR, SERIE | Explicit, non-associative copies (Cad-lite has no array object). `-ARRAY`: selection, `[Rectangular/Polar] <R>`; rectangular asks rows, columns, `distance between rows or specify unit cell` (two corners) and the column distance (a distance is skipped for a single row or column; negative goes down or left); polar asks `center point of array or [Base]`, the number of items (2 or more), `angle to fill (+=ccw, -=cw) <360>` (360 spreads the items over the circle, less puts the last on the fill angle) and `Rotate arrayed objects as they are copied? <Y>` (No translates the base point around the centre, the selection's centre by default). `ARRAY` first asks `[Rectangular/PAth/Polar]` (PAth is refused), `ARRAYRECT` and `ARRAYPOLAR` skip the type and print `Type = … Associative = No`; all share the `-ARRAY` prompts. At most 100000 copies. The last point of each command, a click or typed point, becomes `LASTPOINT` |
| U | U | Undoes one step and prints its command name (`LINE`); `Everything has been undone.` when nothing is left. Ctrl+Z and the toolbar run it |
| UNDO | — | `Enter the number of operations to undo or [Auto/Control/BEgin/End/Mark/Back] <1>`: a number undoes that many steps as one undo operation. `Mark` sets a mark; `Back` undoes to the latest mark with no question, or without a mark asks `This will undo everything. OK [Yes/No] <Yes>`. `BEgin` and `End` bracket a group that one `U` undoes (a `U` inside an open group closes it first; `End` without `BEgin` prints `There is no UNDO Begin group to end.`). `Control` `[All/None/One/Combine/Layer] <All>`: `None` turns U and UNDO off (`UNDO is turned off (UNDO Control None).`), `One` limits them to one step, `Combine` and `Layer` are accepted and change nothing. `Auto` `[ON/OFF]` is stored and changes nothing (a command is one step anyway). Inside LINE and PLINE `U` stays that command's own option |
| REDO | — | Reverses the latest U or UNDO as a whole (after `UNDO 2`, one REDO restores both steps); `Nothing to redo.` when there is none. A new change discards the redo steps |
| MREDO | — | `Enter number of actions to redo [All/Last]`: a number of steps, `ALL`, or `Last` (the latest undo operation, as REDO) |
| ZOOM | Z | `Specify corner of window, enter a scale factor (nX or nXP), or [All/Center/Extents/Previous/Scale/Window/Object] <Extents>`: a point starts a window (`Specify opposite corner`); a number scales the full view (the displayed extents), `2X` and `2XP` scale the current view (`XP` is the same as `X`: there is no paper space); `Scale` asks for the factor; `Center` asks `Specify center point` then `Enter magnification or height <h>` (a number is the view height in model units, `nX` a magnification, Enter keeps the height); `Object` fits the selected objects; `All` is Extents (there are no limits). `Previous` steps back through the last 10 views and prints `No previous view saved.` when none is left; a mouse-wheel or drag change made since the last zoom is saved as a step of its own. Enter (real time) and `Dynamic` do not exist, Enter is Extents. Extents ignore objects on off or frozen layers. Transparent |
| PAN, -PAN | P, -P | `Specify base point or displacement`, then `Specify second point or <use first point as displacement>`: the drawing moves from the first point to the second (Enter takes the first point as the displacement). The real-time pan of AutoCAD has no scriptable form; drag and wheel pan the canvas. Transparent. `ZOOM Previous` undoes a pan |
| REGEN, REGENALL, REDRAW, REDRAWALL | RE, REA, R, RA | Do nothing and print nothing: the canvas is rebuilt from the document on every frame |
| LAYER | LA | A layer name makes it current, creating it with colour 7 when new; `name=color` sets its colour (1–255). A Cad-lite form: AutoCAD's `LAYER` opens a palette; use `-LAYER` for the command-line options |
| -LAYER | -LA | Prints `Current layer: X`, then repeats `Enter an option [?/Make/Set/New/ON/OFF/Color/Ltype/LWeight/Plot/Freeze/Thaw/LOck/Unlock/Description]` until Enter. Names are comma lists with `*` and `?` wildcards, case-insensitive; a name that matches nothing prints `Cannot find layer "X".`. `?` lists layers with state, colour and linetype. `Make` creates (colour 7) or finds a layer, turns it on and thaws it, and makes it current. `Set` makes an existing, unfrozen layer current. `New` makes layers (colour 7; a name may not contain `< > / \ " : ; ? * \| , = \``). `ON`, `OFF`, `Freeze`, `Thaw`, `LOck`, `Unlock` take a name list; Enter at those prompts (and at `Set`) asks `Select object whose layer to use` and takes the layer of the picked object. `OFF` of the current layer asks `Really want layer X (the CURRENT layer) off? <N>`; `Freeze` skips the current layer with a message. `Color` takes a number 1-255 or a name, then a layer list (Enter = current). `Ltype` takes `CONTINUOUS` or a linetype a loaded file defines (`?` lists them). `LWeight` takes a standard lineweight in mm (`0.25`) or `DEFAULT`; `Plot` takes `[Plot/No plot]`; `Description` asks for one layer, then the text. Layer table edits are not undoable. `Rename`, `stAte`, `TRansparency` and `MATerial` do not exist |
| MATCHPROP | MA | `Select source object`, prints `Current active settings = Color Layer`, then `Select destination object(s) or [Settings]`; the settings are copied on Enter to every selected object not on a locked layer, as one undo step. `Settings` asks `Enter setting to switch [Color/LAyer]` (Enter returns); the choice is kept in `MATCHPROPSET` (1 colour, 2 layer). Linetype, lineweight and the other AutoCAD settings need the entity model to grow first |
| CHPROP | — | Selection (or the pick-first set), then `Enter property to change [Color/LAyer]` repeating until Enter. `Color` takes a number 1-255, a name or `BYLAYER` (default); `LAyer` takes the name of an existing layer (default the first object's). LType, ltScale, LWeight, Thickness and CHANGE are missing |
| -COLOR, COLOR | -COL, COL | `New object color <BYLAYER>`: a number, a name or `BYLAYER`; sets `CECOLOR`, the colour new objects get. COLOR has no dialog, so it asks the same |
| LTSCALE | LTS | `Enter new linetype scale factor <1>`; stored in `LTSCALE` (no linetype is drawn yet) |
| PURGE, -PURGE | PU, -PU | `Enter type of unused objects to purge [LAyers/Zero-length geometry/All]`. `LAyers` and `All` ask `Enter name(s) to purge <*>` and `Verify each name to be purged? [Yes/No] <Yes>`, then delete the unused layers they match (never layer 0, the current layer or a layer with objects) and print `Deleted layer "X".`; `Zero-length geometry` and `All` erase lines, circles, arcs and polylines with no length, as one undo step. PURGE has no dialog, so it asks the same |
| DIST | DI | Two points; prints `Distance = 5.0000,  Angle in XY Plane = 53,  Angle from XY Plane = 0` and `Delta X = ...,  Delta Y = ...,  Delta Z = ...` in the `UNITS` formats, and sets `DISTANCE`. `[Multiple points]` (`M`) adds up consecutive segments and prints `Distance = total` on Enter. Transparent |
| AREA | AA | Corner points until Enter, or `[Object/Add area/Subtract area]`; prints `Area = 6.0000, Perimeter = 12.0000` (`Circumference` for a circle) and sets `AREA` and `PERIMETER`. `Add area` and `Subtract area` keep a running total (`Total area = ...`, also `AREA`) and loop until `Exit`. Open polylines and arcs close by their chord; polyline bulges count; a line has no area. `Arc`, `Length` and `Undo` while digitising are not done. The corner prompts snap like any point prompt; `Select object:` picks by the raw click (see Object snaps) |
| MEASUREGEOM | MEA | `Enter an option [Distance/Radius/Angle/ARea/Exit] <Distance>`; after each measurement the same prompt returns until `Exit`. Radius picks an arc or circle (`Radius = r, Diameter = d`); Angle measures two lines, an arc's included angle, a circle centre and two points, or a vertex and two points (`Angle = 90°`, in the `AUNITS` format); ARea is AREA. Its point prompts (DIST's, the vertex and endpoints of Angle, ARea's corners) snap; the object prompts of Radius and Angle pick by the raw click (see Object snaps). `Volume`, `Quick` and `Mode` are not done |
| MIRROR | MI, SPECCHIO, SPECCHIA | Selection, two points of the mirror line, then `Erase source objects [Yes/No] <No>`; the reflection is added on the source's layer and colour, arcs stay counter-clockwise and bulges flip. With `MIRRTEXT` 0 (the default) text keeps reading normally: its box lands on the mirrored box: across the mirror line left and right justification swap, along it top and bottom swap and any other text moves across its baseline, keeping its justification; with `MIRRTEXT` 1 TEXT is mirrored (backward or upside down). MTEXT is never mirrored, as in AutoCAD. Two equal points print `The two points of the mirror line must differ.` |
| OFFSET | O | Prints `Current settings: Erase source=No  Layer=Source  OFFSETGAPTYPE=0`, then `Specify offset distance or [Through/Erase/Layer] <Through>` (`<d>` once a distance is set): a typed distance, two points, or `Through` (`OFFSETDIST` becomes -1 and the side point sets the distance per pick). `Erase` asks `Erase source object after offsetting? [Yes/No]` (`OFFSETERASE`) and `Layer` `[Current/Source]` (`OFFSETLAYER`), both returning to the distance prompt. Then loops `Select object to offset or [Exit/Undo]` and `Specify point on side to offset or [Exit/Multiple/Undo]`; Enter exits. `Multiple` keeps asking sides and offsets each from the newest copy (Enter returns to the select prompt); `Undo` takes the last offset back (an erased source returns as a new object). Lines, circles, arcs and open or closed polylines; the copy lands on the source layer (or the current one) and keeps the polyline widths when the vertex count is unchanged. Polyline corners mitre, arc segments take the new radius, segments that collapse are dropped; a radius that would reach zero creates nothing and prints why. `OFFSETGAPTYPE` is stored only |
| FILLET | F, RACCORDO, RACCORDA | `Select first object or [Undo/Polyline/Radius/Trim/Multiple]`, then `Select second object or shift-select to apply corner or [Radius]`. Two lines, trimmed or extended to the corner and joined by a tangent arc; the picked sides are kept. Radius 0 makes a sharp corner. `Radius` (`FILLETRAD`) and `Trim` (`TRIMMODE`; No trim leaves the lines and adds only the arc) are remembered. `Polyline` rounds every vertex between two straight segments of one polyline (`N lines were filleted`, `N were too short` for a radius that does not fit) and interpolates the segment widths across each arc. `Multiple` repeats until Enter; `Undo` takes the last corner back. A pick whose `Input.Text` is `shift` applies radius 0; the session does not set it yet. Parallel lines print `Lines are parallel.` and a radius that does not fit prints `Radius is too large for the lines.`, both leaving the drawing unchanged; other objects print `Fillet supports two lines.` |
| CHAMFER | CHA, SMUSSO, CIMATURA, SMUSSA, CIMA | `Select first line or [Undo/Polyline/Distance/Angle/Trim/mEthod/Multiple]`, then `Select second line or shift-select to apply corner or [Distance/Angle/Method]`. Two lines, trimmed to the corner and joined by a bevel line. `Distance` asks the two distances (`CHAMFERA`, `CHAMFERB`; the first applies to the first line picked) and `Angle` the length on the first line and the angle from it (`CHAMFERC`, `CHAMFERD` in degrees); each sets the method `CHAMMODE`, `Method` switches it. `Trim`, `Polyline`, `Multiple` and `Undo` work as in FILLET, with the bevel cut by distance or by length and angle at each vertex. Distances 0 and 0 make a sharp corner; too large prints `Chamfer distances are too large for the lines.` |
| TRIM | TR | `TRIMEXTENDMODE` 1 (Quick, the default) takes every visible object as a cutting edge: `Select object to trim or shift-select to extend or [cuTting edges/Crossing/mOde/Project/eRase/Undo]`, Enter ends. `TRIMEXTENDMODE` 0 (Standard) first asks `Select objects or <select all>` for the edges (typed selection options work, Enter = all). Options: `Fence` and `Crossing` remove every piece between cuts that the fence or window touches, as one undo step; `Project` stores `PROJMODE` (no effect in 2D); `Edge [Extend/No extend]` sets `EDGEMODE` so edges act at their projection; `eRase` erases a selection; `Undo` reverts the last pick, fence or erase; `mOde` switches Quick and Standard. A pick removes the piece of the object between the two nearest cuts, or from the one cut to the end. Lines, circles, arcs and polylines (a polyline's bulges are kept). Prints `Object does not intersect an edge.` when no edge cuts it. A point picked with Shift held (`Input.Shift`, set by `Session.Click`) extends instead |
| EXTEND | EX | The mirror of TRIM over the same engine: `Select boundary edges ...` in Standard mode, `Select object to extend or shift-select to trim or [...]` (Quick adds `Boundary edges`, no `eRase` in Standard). The end nearest the pick grows along its line or its own circle to the nearest boundary ahead of it (with `Edge` = Extend, to the projected boundary). Lines, arcs and open polylines (the end segment grows); a closed polyline prints `Cannot extend a closed polyline.` and no boundary ahead prints `No boundary edge in the extension of that end.` `Fence` and `Crossing` extend each object they touch |
| BREAK | BR | `Select object`, `Specify second break point [First point]`; the part between the two points is removed and the object splits in two, or at one point when both are equal. `F` re-asks the first point. A circle loses its arc counter-clockwise from the first point to the second |
| BREAKATPOINT | — | `Select object`, `Specify break point`: the object splits in two at the point (lines, arcs, open polylines); a circle or closed polyline prints `Cannot break a closed object at a single point.` |
| JOIN | J | `Select source object or multiple objects to join at once`, then `Select objects to join to source` unless several were selected. Collinear lines become one line, arcs of one circle one arc (a circle when they close it), and lines, arcs and open polylines that meet end to end become one polyline. With several objects selected at once it prints `N objects joined.`, with a source pick `N line joined to source` (or `arc`, `object`), and how many `cannot be joined.` A single arc as source offers `Select arcs to join to source or [Close]`: `Close` makes it a circle |
| EXPLODE | X | `Select objects`; each polyline becomes its lines and arcs, on its layer; other objects print `N objects cannot be exploded.` |
| ID | — | One point; prints `X = 7.0000  Y = 8.0000  Z = 0.0000` and sets `LASTPOINT`, so a following `@` starts there. Transparent |
| LIST | LI, LS | Prints each selected entity in the AutoCAD block form: `LINE  Layer: "0"`, `Space: Model space`, `Handle = ..`, the coordinates as `from point, X=  0.0000  Y=  0.0000  Z=  0.0000` and the length, angle and deltas; circles add circumference and area, closed polylines area and perimeter, then every vertex; TEXT prints its style, start point (and alignment point when justified), justification, height, text, rotation, width factor, obliquing angle and generation; MTEXT its style, insertion point, attachment, height, width, rotation and contents (paragraphs separated by ` / `) |
| NEW, OPEN, SAVE, QSAVE, SAVEAS, CLOSE, QUIT, EXIT | — | Files and the unsaved-changes question; see [DXF files](dxf-files.md); NEW, OPEN and CLOSE open and close [document tabs](document-tabs.md) |
| OPTIONS | OP | Opens the [settings](settings.md) panel; prints `Settings are not available.` where the host has none |
| SETVAR | SETV | A system variable name (`?` lists), then its value; a variable typed by name at `Command:` asks for the value too; see [Command line](command-line.md) |
| UNITS, -UNITS | UN, -UN | The command-line form (Cad-lite has no dialog, so both run it): length format 1 to 5 and its precision (digits, or the denominator for architectural and fractional), angle format 1 to 5 and its places, `Enter direction for angle 0` and `Do you want angle measured clockwise? [Yes/No] <N>`. Sets `LUNITS`, `LUPREC`, `AUNITS`, `AUPREC`, `ANGBASE` (stored in degrees), `ANGDIR`; `INSUNITS` and `UNITMODE` are variables only. Every result line honours them: decimal prints trailing zeros (`5.0000`), scientific, engineering and architectural (inches), fractional; angles as decimal degrees, `d'"`, grads, radians or a surveyor's bearing. A typed angle accepts the unit systems and the `d` `'` `"` `g` `r` suffixes and is read from `ANGBASE` in the `ANGDIR` sense. Transparent |
| SCRIPT, RSCRIPT, DELAY | SCR | Runs a script file (the nested script replaces the caller); restarts it; accepts a delay and does not wait |
| ORTHO | ORTO | `Enter mode [ON/OFF] <OFF>:` sets `ORTHOMODE`. Transparent |
| GRID | GR | `Specify grid spacing(X) or [ON/OFF/Snap/Major/Aspect] <10.0000>:` a number sets `GRIDUNIT` (0 follows the snap spacing, `2x` is a multiple of it); `ON`/`OFF` set `GRIDMODE`; `Snap` copies `SNAPUNIT`; `Major` asks `GRIDMAJOR`; `Aspect` asks x then y spacing. No Limits, aDaptive or Follow. Transparent |
| SNAP | SN | `Specify snap spacing or [ON/OFF/Aspect/Style/Type] <10.0000>:` sets `SNAPUNIT` and `SNAPMODE`; `Style` (`SNAPSTYL`) and `Type` (`SNAPTYPE`) are stored and have no effect. No Legacy. Transparent |
| OSNAP, -OSNAP | OS | The command-line form for both (no dialog): prints `Current osnap modes: End,Cen,Int`, then `Enter list of object snap modes <...>:` takes a comma list (`END,MID`, three letters suffice) or `NONe` and sets `OSMODE`. Exact Italian names also work (see Typed overrides below). Transparent |
| AI | AGENT | `Ask the agent` takes free text (Space does not submit): text opens the [agent panel](agent-panel.md) and sends it; an empty answer toggles the panel |

Commands that take a selection (ERASE, MOVE, COPY, ROTATE, SCALE, LIST) use the current selection
without asking when there is one, then clear it. Otherwise they prompt `Select objects:`; the user
picks or drags boxes and presses Enter.

### Selection

- With no command running, or at `Select objects:`, a click picks the entity under the pickbox and
  adds it to the selection; Shift-click removes it (in `Remove` mode of `Select objects:` a plain click removes and Shift-click adds).
- Dragging a box selects too: left to right is a **window** (blue; entities entirely inside), right
  to left a **crossing** (green; entities inside or touching).
- Picking chooses the nearest geometry; equal-distance ties prefer the front layer, then the newest
  entity within that layer.
- The entity under the pickbox is highlighted while hovering; selected entities are highlighted in
  blue.
- Entities on locked layers or layers that are off cannot be selected.
- **Typed options** at `Select objects:` (and in `SELECT`): `W` window, `C` crossing, `BOX`, `ALL`,
  `L` last (the newest visible object), `P` previous selection, `F` fence, `WP` and `CP` polygon,
  `A` and `R` to switch between adding and removing (the prompt reads `Remove objects:`), `M`, `AU`,
  `SI` (ends the prompt after the first selection), `U` (undoes the last operation); Enter ends. W,
  C and BOX ask `Specify first corner:` and `Specify opposite corner:`; WP, CP and F take points
  until Enter (`U` drops the last one; Enter closes a polygon). A typed point picks the object under
  it, or starts an automatic box when it is empty. Each operation reports `N found`, then
  `N found, M total`, `N found, K duplicate` or `N found, K removed, M total`; other text prints
  `Invalid selection` with the list of options. `ALL` includes objects on layers that are off but not
  frozen or locked ones. Clicks reach the corner and polygon prompts as points. `SELECT` runs the
  prompt and leaves the set as the previous selection for the next command's `P`; the set is not left
  highlighted.

### Object snaps and ortho

- While a command asks for a point, a distance or an angle, the cursor snaps to the nearest snap
  point within the aperture (8 pixels) and a marker shows the snap: square endpoint, triangle
  midpoint, circle centre, diamond quadrant, X intersection.
- The snap modes and their priority are owned by the [command system](../tech/command-system.md).
- Every point prompt snaps, the measuring ones too: DIST, ID, AREA's corners and MEASUREGEOM's points
  (also typed transparently, `'DIST` inside LINE). The prompts that pick an object by a click (AREA
  `Object`, MEASUREGEOM Radius and Angle's `Select arc, circle, line...` and `Select second line:`, the
  object prompt of TRIM and EXTEND, and `Select object:` of BREAK and BREAKATPOINT) do not snap, take no
  typed snap override and show the pickbox, as AutoCAD's object selection does: a snap would move the
  pick to an intersection or a centre and select the wrong object. BREAK's pick is also its first break
  point, as in AutoCAD, so that point is where the click was.
- What the running command has made so far counts at once, as in AutoCAD: LINE's segments, COPY's
  and OFFSET's copies, TEXT's lines and each TRIM or EXTEND pick are drawn, snapped to and pickable
  before the command ends, which stays one undo step.
- **Text.** TEXT and MTEXT snap only at their insertion point, with the Insertion snap (`INS`, off by
  default as in AutoCAD; its marker is the generic one). A click anywhere inside a text's box
  picks it. Box selection uses the text box, as AutoCAD does: a window must hold the whole box, and a
  crossing window, crossing polygon or fence that meets the box selects the text, also between two
  letters or inside an O. As cutting or boundary edges for TRIM and EXTEND a text counts by the four
  sides of its box, as in AutoCAD; text is never trimmed or extended itself.
- **Text sizes from files.** A TEXT or MTEXT read from DXF or DWG keeps a height and MTEXT width up to
  `doc.TextMaxSize` (1e15), a width factor of 0.01 to 100, an obliquing angle within ±85° and an MTEXT line
  spacing factor of 0.25 to 4 (AutoCAD's own ranges); values beyond are clamped on reading
  (`Text.Sanitize`, `MText.Sanitize`), and the layout applies the same limits, so no text has an infinite
  box. Deviation: a file with an out-of-range value is written back with the clamped one.
- **Typed overrides.** At a point prompt `END MID CEN QUA INT NEA PER TAN NON` (or a comma list, `_` prefix
  allowed for English names) arm a snap for the next answer only, working with the running snaps off: the prompt becomes
  `MID of:` and the next click, or typed coordinates read as the pick position, snaps to that object
  (the object must lie within the aperture; `INT` needs both objects within it). `FROM` asks `Base point:`
  then `<Offset>:`; `M2P` or `MTP` asks two points and answers their midpoint; both nest with the snaps.
  Exact Italian input aliases work in every interface language: `FINE` / `PUNTO FINALE` for
  Endpoint, `INMEZZO` / `PUNTO MEDIO` for Midpoint, `INTERSEZIONE` for Intersection, and
  `VICINO` for Nearest. Matching ignores case and repeated whitespace; these names also work in
  `-OSNAP` comma lists. A leading `_` restricts that name to English. At a point prompt Space submits the answer, including `INMEZZO`;
  multiword aliases require a complete answer submitted through `Session.Submit` or pasted into
  the command line. Prompt keywords still win, and object-pick prompts do not accept these
  aliases. Scripts split at spaces, so use `INMEZZO`, another single-word alias or English forms
  there, and omit spaces in script comma lists.
  A failed snap prints `No object snap found.` and drops the override. A prompt keyword wins over a
  snap name (`End` at ARC). `PER` and `TAN` need a previous point and find nothing at a first point
  (no deferral). `NOD`, `QUI`, `APP`, `EXT`, `PAR`, `GCE`, `TT` print `Object snap ... is not supported.`
- `OSMODE` holds the running snaps with AutoCAD's bit values (the 16384 bit is F3 off); `SNAPMODE` turns on
  snapping the cursor to the `SNAPUNIT` grid (F9), applied after object snaps and before ortho.
- With ortho on, rubber-band points are constrained horizontal or vertical from the base point; a snap
  overrides ortho; typed coordinates are never changed.
- **Polar tracking** (F10, the POLAR toggle; on when `AUTOSNAP` has bit 8, off by default): the rubber-band point
  locks onto the ray through the base point at the nearest multiple of `POLARANG` (degrees from the X axis, 45 by
  default) when its own angle is within 5 degrees of it; the point keeps its distance along the ray. Ortho and polar
  tracking exclude each other: turning one on turns the other off. A snap overrides polar tracking like ortho. The
  angle does not follow `ANGBASE` or `ANGDIR`.
- `LWDISPLAY` (the LWT toggle) draws each layer at its lineweight; it starts off.

### Undo

Each command, including one cancelled part-way, is one undo step; the `Undo` option inside LINE,
PLINE, OFFSET and TRIM steps back within it. Undo and redo restore entities and transactional layer order
with their handles. Selections, view changes and layer table edits are not undoable. UNDO Mark and Back,
BEgin and End, counts, REDO and MREDO are in the command table; `DBMOD` is 1 while the drawing has unsaved
changes (every committed command, undo and redo counts).

## Contract

- Commands registered by `commands.Register`; aliases in `commands.Aliases` (`acad.pgp` format).
- Document: `Txn.Add`, `Txn.Modify`, `Txn.Erase`, `Drawing.Undo`, `Drawing.Redo`, the layer table.
- Session state: the selection, `SnapOn`, `Snaps`, `Ortho`, `Polar()`, `Aperture`, `Grid()`, `SnapGrid()`, `LineweightDisplay()`.

## Implementation

`cmdsys/commands/group.go`, `block.go` and `attribute.go` own the symbol commands; `doc.Txn` owns
membership, definition and support-table snapshots. `Insert.Parts` resolves geometry for drawing and
picking; `Insert.Explode` preserves one level of independent objects for similarity-scaled references.
Nonuniform references refuse EXPLODE rather than changing circular geometry. Library operations use
`Context.Files`, with caller checks at each input before file access.

The draw commands are in `cmdsys/commands/draw.go` (`lineCmd`, `plineCmd`, `circleCmd`, `arcCmd`,
`rectCmd`), the modify commands in `modify.go` (`eraseCmd`, `moveCmd` for MOVE and COPY,
`transformCmd` for ROTATE and SCALE, all sharing `selecting`), `array.go` (`arrayCmd`) and `oops.go` (`rememberErased`, OOPS), measurement (`areaCmd`, `measureGeomCmd`, with the area formulas in `geom/area.go`) in `measure.go`, the construct commands in `construct.go` (`mirrorCmd`, `offsetCmd`) and `construct_corner.go` (`cornerCmd` for FILLET and CHAMFER), on `geom/offset.go`, `geom/fillet.go` and `geom/filletpoly.go`, TEXT and MTEXT in `text.go` (`textCmd`) and `mtext.go` (`mtextCmd`), the trim family in `trim.go` (`trimCmd`, `extendCmd`, `breakCmd`, `joinCmd`, `explodeCmd`, on `geom/split.go`), and one file per remaining group: `view.go` (ZOOM, PAN, REGEN, `ZoomExtents`), `layer.go` (LAYER, -LAYER), `matchprop.go`, `props.go` (CHPROP, -COLOR, LTSCALE, `CECOLOR`), `purge.go`, `inquiry.go` (DIST, ID, LIST), `units.go` (UNITS, -UNITS and the units variables),
`files.go` (`fileCmd`: NEW, OPEN, SAVE, QSAVE, SAVEAS, CLOSE, QUIT, EXIT), `undo.go` (U, UNDO, REDO, MREDO; the modified counter and the undo state are `cmdsys/dirty.go`). `register.go` holds `Register`, which calls one `registerX` per
file, and `aliases.go` the `Aliases` text; `base.go` holds the helpers every command shares (`base`, `selecting`,
`immediate`). Snaps, picking and box selection are
`cmdsys.FindSnap`, `cmdsys.Pick` and `cmdsys.SelectBox`; the UI forwards clicks and drags to
`Session.Click` and `Session.SelectBox` from `ui/canvas.go`.

`i18n/command.go` registers the catalog metadata aliases. `cmdsys/override.go` shares the English
snap classifier between point overrides and `ParseSnapList`; `i18n/snap.go`, `CanonicalSnapName`,
resolves exact catalog snap aliases before that classifier.

Snaps, picks and the canvas see a running command's work because the session applies its open
transaction's edits to the spatial index after every answer and queues them for the canvas's display
cache (`cmdsys/inflight.go`, see the [command system](../tech/command-system.md)); so OFFSET, FILLET
and CHAMFER can pick an entity made earlier in the same command. Their
remembered distance, radius, chamfer settings and the OFFSET Erase and Layer options are session variables
(`OFFSETDIST`, `FILLETRAD`, `CHAMFERA` to `D`, `CHAMMODE`, `TRIMMODE`, `OFFSETERASE`, `OFFSETLAYER`). The point-picking prompts of the trim family pick within a fixed 8
pixels, because a command's `Context` does not carry `Session.Aperture`; their object prompts are
`Prompt.Pick` prompts. TRIM's cuts read only the edges whose boxes meet the trimmed curve, through the
spatial index (`edgeCurves`), and a pick goes through `cmdsys.Pick` (`pickEntity`).

## Failure

- A zero or negative radius or scale factor prints `Requires a positive and nonzero value.` and asks
  again.
- An empty selection ends the command without a change.
- `LAYER` with a colour outside 1–255 prints the valid range and changes nothing.
- A frozen layer is not drawn, picked, snapped to or used by Extents; a locked layer is drawn and snapped to but cannot be selected or edited; an off layer is not drawn, picked or snapped to.

## Related docs

- [Command system](../tech/command-system.md) — the framework under every command
- [Command line](command-line.md) — how commands are typed and answered
- [Drawing canvas](canvas.md) — highlights, markers and the crosshair on screen

## Next steps

- MATCHPROP; associative arrays and ARRAYPATH.
- FILLET and CHAMFER on arcs, circles and mixed line and arc pairs (the polyline option and the two-line case exist); `OFFSETGAPTYPE`; Shift-pick for the corner (the session does not pass the modifier yet).
- Grip editing and a properties palette.
- JUSTIFYTEXT, a STYLE command and table with fonts, the in-place rich text editor, MTEXT Line spacing and Columns, and an Insertion snap marker.
- Object snap tracking, polar additional angles and a tracking line drawn on the canvas, the Extension and Apparent snaps, and PER/TAN deferred at a first point.
