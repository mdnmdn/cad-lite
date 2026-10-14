# i18n migration guide

How to move Cad-lite's user-facing literals onto the `i18n` package (English and Italian). The
package, the catalogs (`locales/en.toml`, `locales/it.toml`, embedded) and the checker already
exist; this file is the work order for replacing literals with keys, file by file. Nothing outside
`i18n/` was edited when this was written.

`go run ./i18n/cmd/i18ncheck -scan ui,cmdsys,cadmcp,fileio,settings` lists what is left (and with
`-known` which keys each remaining literal maps to). `-strict` exits 1 while a literal remains.

## 1. API in five lines

```go
i18n.Set(i18n.FromSetting(settings.Language))   // "auto" | "en" | "it"; at start-up and on change
i18n.T("cmd.layer.msg.created", name)           // fmt.Sprintf style; missing key -> English -> the key
i18n.Plural("cmd.join.msg.joined", n)           // picks _one / _other, n is the first argument
i18n.KeywordLabel("Close"); i18n.MatchKeywords(ids, text); i18n.PromptKeywords(ids)
i18n.Command("LINE")                            // Name, Desc, Tip, Aliases; i18n.RegisterAliases(reg.Alias)
```

* **Placeholders are fmt verbs.** Chosen over `{name}` because every call site is already a
  `Printf`/`Sprintf`, so the replacement is mechanical: the arguments do not change. A translation
  that needs another word order uses `%[2]s ... %[1]s`. `Verify` compares the verb of every
  argument index between languages.
* A value used with no arguments is returned as written; with arguments it goes through
  `fmt.Sprintf` (so `%%` for a percent sign). `ui.settings.ui_scale_value` is `"%d%%"` for that
  reason.
* **Plurals.** Keys `<base>_one` and `<base>_other`; `Plural(base, n, extra...)` formats with `n`
  first. 1 is "one", everything else "other" (right for English and Italian). The existing
  `plural(n)` noun helper in `trim.go` goes away.
* `For(lang)` returns a `Translator` bound to a language (`For(i18n.English).T(...)`), for text that
  must not follow the UI language.
* `Detect()` reads `LC_ALL`, `LC_MESSAGES`, `LANG` in POSIX order (first non-empty decides, `C`
  is English). Windows and a macOS app started from Finder have none of these: Detect then calls the
  optional `i18n.PlatformLang` hook, which `cmd/cad` may set (registry/`defaults` lookups need
  `x/sys` or exec, which does not belong in a leaf package). With neither, and on js/wasm, the
  result is English.
* Key naming: `ui.<area>.<thing>`, `cmd.<command>.prompt.<id>` (text of a prompt, no colon),
  `cmd.<command>.msg.<id>` (a history line), `cmd.<command>.name|desc|aliases`, `input.err.*`,
  `snap.*`, `kw.<slug>`, `dxf.warn.*`, `dxf.err.*`, `settings.warn.*`. A `_tip` suffix marks a
  tooltip. Tooltip format: `Title — what it does (Shortcut / alias)`; a toolbar button that runs a
  command composes its tooltip instead (§3).

## 2. Keywords

Today a keyword is one English string that is at once id, display text and match pattern
(`"cuTting edges"`: capitals are the shortcut). The design keeps that string as the **id** so no
command logic changes (`in.Keyword == "Close"` stays), and moves display and matching into i18n:

| Step | cmdsys change |
|---|---|
| Display | `Prompt.String` writes `i18n.PromptKeywords(p.Keywords)` instead of `strings.Join(p.Keywords, "/")`, and `" " + i18n.T("input.or") + " ["` instead of `" or ["`. Italian shows `Specificare punto successivo o [Chiudi/Annulla]:`. |
| Matching | `matchKeyword(kws, s)` becomes `i18n.MatchKeywords(kws, s)` (returns the id). Order: current language (full name, synonyms, shortcut), then English (full name, shortcut). A leading `_` or `'_` (AutoCAD's `_Close`) means English only, in any language. Case-insensitive. |
| Default echo | `formatInput` for `InputKeyword` returns `i18n.KeywordLabel(in.Keyword)` so `<Estensioni>` is localized. |
| `keywordShortcut` | delete; use `i18n.KeywordShortcut(id)` if still needed. |

Catalog data: table `[kw]`, one line per id, value `Display|synonym|synonym`, shortcut letters in
capitals (none: the first letter), key = the id lower-cased with every non-alphanumeric turned into
`_` (`cuTting edges` -> `kw.cutting_edges`). Both catalogs carry all of them:

| id | en | it | it shortcut |
|---|---|---|---|
| Close | Close | Chiudi | C |
| Undo | Undo | Annulla | A |
| Yes | Yes | Sì (also `si`) | S |
| No | No | No | N |
| Through | Through | Passaggio (verify) | P |
| Exit | Exit | Esci | E |
| Radius | Radius | Raggio | R |
| Distance | Distance | Distanza | D |
| Diameter | Diameter | Diametro | D |
| Extents | Extents | Estensioni | E |
| Window | Window | Finestra | F |
| Object | Object | Oggetto | O |
| Angle | Angle | Angolo | A |
| ARea | ARea | ARea | AR |
| cuTting edges | cuTting edges | Bordi di Taglio (verify) | BT |
| First point | First point | Primo punto | P |
| Multiple points | Multiple points | Più punti (verify) | P |

The Italian shortcut of `cuTting edges` is `BT` because both capitals count; that matches how
`ARea` already works. If a single letter is wanted write `Bordi di taglio`.

**Conflict-freedom** is a tested property: `i18n.KeywordGroups` lists the keyword set of every
prompt that has keywords (`MEASUREGEOM` offers Distance/Radius/Angle/ARea, etc.), and `Verify`
fails when two keywords of one group share a shortcut or name in any language. A command that adds
a keyword prompt adds its group there and its `kw.*` lines to both catalogs; an id with no catalog
line still works (it shows and matches as written), so a new keyword can ship untranslated.

Keywords typed in a script stay portable: English always matches, so `LINE 0,0 10,10 C` and
`_C` work in an Italian session.

## 3. Commands, aliases, tooltips

Every command in `i18n.Commands` has three catalog entries per language, under `cmd.<name>.`:
`name` (display, e.g. "Linea"), `desc` (one line) and `aliases` (array of extra typed names). `i18n.Command("LINE")` returns them for the current language.

**Aliases.** Replace the hand-kept pgp table's role for localized names with the catalog:

```go
// cmdsys/commands/util.go, end of Register
i18n.RegisterAliases(r.Alias)   // English short aliases (L, PL...) and Italian names (LINEA, CERCHIO...)
```

`RegisterAliases` registers the aliases of **every** language regardless of the UI language, so a
drafter who switches language keeps muscle memory and scripts keep working. The English arrays in
`en.toml` repeat today's `Aliases` const exactly (`L`, `PL`, `C`, `A`, `REC`, `E`, `M`, `CO`, `CP`,
...); the const and `LoadAliases` may stay (same mapping, registered twice is harmless) or the const
may be dropped later. `Verify` guarantees no alias shadows a command name and none maps to two
commands. Italian names: LINEA, POLILINEA, CERCHIO, ARCO, RETTANGOLO/RETTANG, CANCELLA, SPOSTA, COPIA,
RUOTA, SCALA, ANNULLA, RIPETI/RIPRISTINA, LAYER/LIVELLO, DISTANZA, MISURAGEOM, ELENCA, NUOVO, APRI,
SALVA, SPECCHIO, RACCORDO, SMUSSO/CIMATURA/SMUSSA, TAGLIA, ESTENDI, SPEZZA, UNISCI, ESPLODI,
AGENTE/IA, OPZIONI. `OFFSET`, `ZOOM`, `AREA`, `ID` are the same in both languages.

Uncertain terms are marked `# (verify)` in `it.toml` (CAD-fluent review wanted): RIPETI vs
RIPRISTINA for REDO (AutoCAD Italian says RIPRISTINA), CIMATURA/SMUSSA for CHAMFER (AutoCAD
Italian says SMUSSO), LIVELLO, MISURAGEOM, ELENCA, the keyword words Passaggio / Bordi di taglio /
Più punti, `on`/`off` for layer state, `SNAP OGG.`, and `Pan`.

**Echo.** `s.print("Command: " + cmd.Name())` becomes
`T("session.command_echo", strings.ToUpper(i18n.CommandName(cmd.Name())))` (shows `Comando: LINEA`).
`Command.Name()` itself keeps returning the English canonical name: lookup, history keys, the
agent and `HumanOnly` all depend on it.

**Toolbar tooltips** are composed by `ui` (`Workspace.commandTip`, `ui/tooltip.go`): the
command's `name` with a keyboard hint in brackets, then its `desc` — `Line (L)`. The hint comes
from the key bindings and the command registry, never from the catalog, so no catalog string
repeats an alias or a shortcut.

## 4. Session and input errors

* `ErrInvalidPoint` & co. keep their English `Error()` (they are matched with `errors.Is` and logged).
  Where the session prints them, map with a switch instead of `capitalize(err.Error()) + "."`:
  `ErrInvalidPoint` -> `input.err.invalid_point`, `ErrInvalidValue` -> `input.err.invalid_value`,
  `ErrNoBase` -> `input.err.no_base`, anything else -> `input.err.invalid_input`.
* `SnapMode.String()` returns `i18n.T("snap.endpoint")` etc. (`snap.generic` for the default).
* `LIST`'s `closed: %v` becomes `%s` with `T("common.yes")` or `T("common.no")`.
* `FILLET`/`CHAMFER`'s `"%s supports two lines."`: pass `strings.ToUpper(i18n.CommandName(name))`.
* `"Select first " + c.noun()` and `"Select second " + c.noun()` become four whole sentences
  (`select_first_object`/`_line`, `select_second_object`/`_line`): do not assemble sentences from
  fragments, word order differs.
* `"Enter path of drawing to " + verb` becomes `open_path` / `save_path`.
* `printWarnings`' `w` strings come from `fileio`; they are localized where they are created (below).

## 5. What deliberately stays English

* **Command names** (`LINE`, `PLINE`...): canonical, registered, typed, stored in history and used by
  the MCP tools. Italian names are aliases, not replacements.
* **DXF**: group codes, section and entity names (`LWPOLYLINE`, `ENDSEC`), file contents. The
  `dxf.warn.*` and `dxf.err.*` sentences are localized; the tokens inside them (`%s` = a DXF class)
  are not translated.
* **cadmcp**: tool names, parameter names, tool descriptions and `Instructions()` are written for AI
  agents and stay English, as do their error strings. Because a `Session` prints through the global
  language, an agent driving an Italian session would read Italian history lines. Either have cadmcp
  read results with `i18n.For(i18n.English)` (the cleanest: give `cmdsys.Session` an optional
  translator, default `For(Current())`, and let `SetAgentCaller(true)` switch it to English), or
  accept that agents see the user's language. The first is recommended.
* **Logs and debug output**, `agentmanager` log levels and protocol strings, panic messages,
  `ui/snapshot.go` errors, `settings.Path` errors (`settings: %w`), the demo transcript in
  `ui/agent_demo.go`, test fixtures.
* **Product and technical names**: `Cad-lite`, `AI` (the toolbar glyph), `OSNAP`-style acronyms
  inside tooltips, harness names (`opencode`, `claude`, `codex`), `Handle`, `ACI`, key names.
* **Permission option names** (`Allow once`, `Reject`) come from `agentmanager` and are shown as
  received.

## 6. Where each package plugs in

| Package | Import i18n for |
|---|---|
| `cmdsys` | `Prompt.String`, `matchKeyword`, session messages, `input.err.*`, `SnapMode.String` |
| `cmdsys/commands` | every prompt `Message`, `ctx.Printf` format, `RegisterAliases` |
| `fileio`, `fileio/dxf` | `dxf.warn.*`, `dxf.err.*`, `fileio.err.*` at creation time |
| `settings` | `settings.warn.*` (optional; they are shown in the panel footer) |
| `ui` | everything under `ui.*`, theme labels (`ui.settings.theme.<name>`, `-` as `_`), `i18n.Set` on load and on a settings change (the frame re-renders from `T`, so no refresh plumbing) |
| `cadmcp` | nothing, except possibly `For(English)` as above |

Startup, in `cmd/cad` after loading settings: `i18n.Set(i18n.FromSetting(ws.Settings().Language))`.
The settings panel's `ui.settings.language_*` rows set `settings.Language` to `auto`, `en` or `it`
(constants `settings.LangAuto/LangEnglish/LangItalian` already exist) and call `i18n.Set` again.

Docs duty (AGENTS.md): add `i18n` to `_docs/tech/code-map.md` and `architecture.md` as a leaf package,
document the language setting in `_docs/features/settings.md`, and run `go run ./tools/docs touched`
and `lint`.

## 7. Verification checklist

```
gofmt -l i18n && go vet ./i18n/... && go test ./i18n/...
GOOS=js GOARCH=wasm go build ./i18n/...
go run ./i18n/cmd/i18ncheck -scan ui,cmdsys,cadmcp,fileio,settings -strict
```

A bug found in a translation is fixed in the catalog and covered by `Verify` or a test when it is
structural (placeholder, key, shortcut conflict). Render with `-png` in both languages (set
`LANG=it_IT.UTF-8`) and look at the toolbar: Italian labels are longer (`Polilinea`, `Cancella`), the
button width must cope.

## 8. Literal to key, per file

Each row is a literal as it appears in the source (or the construct it comes from) and the key that
replaces it. `Printf` literals keep their arguments: `ctx.Printf("Layer %q created.", name)` becomes
`ctx.Printf("%s", i18n.T("cmd.layer.msg.created", name))`, or better a new `ctx.Say(key, args...)`.
Rows for the `cmd.<name>.name|desc|aliases`, `ui.toolbar.*_tip`, `ui.status.*_tip`,
`ui.layers.*_tip`, `ui.agent.*_tip` and `ui.settings.*_tip` entries have no literal in the code:
they are new tooltips and new language-picker rows.

### `(also`

| Literal | Key |
|---|---|
| `"Nothing found at that point."` | `cmd.offset.msg.nothing_found` |

### `FILLET/CHAMFER:`

| Literal | Key |
|---|---|
| `"Nothing found at that point."` | `cmd.offset.msg.nothing_found` |

### `cmd.corner.msg.nothing_found)`

| Literal | Key |
|---|---|
| `"Nothing found at that point."` | `cmd.offset.msg.nothing_found` |

### `cmdsys/commands/*.go`

| Literal | Key |
|---|---|
| `Keywords: []string{"Close"} (id stays "Close"; display via KeywordLabel)` | `kw.close` |
| `Keywords: []string{"Undo"} (id stays "Undo"; display via KeywordLabel)` | `kw.undo` |
| `Keywords: []string{"Yes"} (id stays "Yes"; display via KeywordLabel)` | `kw.yes` |
| `Keywords: []string{"No"} (id stays "No"; display via KeywordLabel)` | `kw.no` |
| `Keywords: []string{"Through"} (id stays "Through"; display via KeywordLabel)` | `kw.through` |
| `Keywords: []string{"Exit"} (id stays "Exit"; display via KeywordLabel)` | `kw.exit` |
| `Keywords: []string{"Radius"} (id stays "Radius"; display via KeywordLabel)` | `kw.radius` |
| `Keywords: []string{"Distance"} (id stays "Distance"; display via KeywordLabel)` | `kw.distance` |
| `Keywords: []string{"Diameter"} (id stays "Diameter"; display via KeywordLabel)` | `kw.diameter` |
| `Keywords: []string{"Extents"} (id stays "Extents"; display via KeywordLabel)` | `kw.extents` |
| `Keywords: []string{"Window"} (id stays "Window"; display via KeywordLabel)` | `kw.window` |
| `Keywords: []string{"Object"} (id stays "Object"; display via KeywordLabel)` | `kw.object` |
| `Keywords: []string{"Angle"} (id stays "Angle"; display via KeywordLabel)` | `kw.angle` |
| `Keywords: []string{"ARea"} (id stays "ARea"; display via KeywordLabel)` | `kw.area` |
| `Keywords: []string{"cuTting edges"} (id stays "cuTting edges"; display via KeywordLabel)` | `kw.cutting_edges` |
| `Keywords: []string{"First point"} (id stays "First point"; display via KeywordLabel)` | `kw.first_point` |
| `Keywords: []string{"Multiple points"} (id stays "Multiple points"; display via KeywordLabel)` | `kw.multiple_points` |
| `"Line"` | `cmd.line.name` |
| `"Polyline"` | `cmd.pline.name` |
| `"Circle"` | `cmd.circle.name` |
| `"Arc"` | `cmd.arc.name` |
| `"Rectangle"` | `cmd.rectang.name` |
| `"Erase"` | `cmd.erase.name` |
| `"Move"` | `cmd.move.name` |
| `"Copy"` | `cmd.copy.name` |
| `"Rotate"` | `cmd.rotate.name` |
| `"Scale"` | `cmd.scale.name` |
| `"Undo"` | `cmd.undo.name` |
| `"Redo"` | `cmd.redo.name` |
| `"Zoom"` | `cmd.zoom.name` |
| `"Layer"` | `cmd.layer.name` |
| `"Distance"` | `cmd.dist.name` |
| `"Area"` | `cmd.area.name` |
| `"Measure geometry"` | `cmd.measuregeom.name` |
| `"ID point"` | `cmd.id.name` |
| `"List"` | `cmd.list.name` |
| `"New"` | `cmd.new.name` |
| `"Open"` | `cmd.open.name` |
| `"Save"` | `cmd.save.name` |
| `"Mirror"` | `cmd.mirror.name` |
| `"Offset"` | `cmd.offset.name` |
| `"Fillet"` | `cmd.fillet.name` |
| `"Chamfer"` | `cmd.chamfer.name` |
| `"Trim"` | `cmd.trim.name` |
| `"Extend"` | `cmd.extend.name` |
| `"Break"` | `cmd.break.name` |
| `"Join"` | `cmd.join.name` |
| `"Explode"` | `cmd.explode.name` |
| `"AI agent"` | `cmd.ai.name` |
| `"Options"` | `cmd.options.name` |

### `cmdsys/commands/agent.go`

| Literal | Key |
|---|---|
| `"The AI agent is not available on this platform."` | `cmd.ai.msg.unavailable` |
| `"Ask the agent"` | `cmd.ai.prompt.ask` |

### `cmdsys/commands/construct.go`

| Literal | Key |
|---|---|
| `"Specify first point of mirror line"` | `cmd.mirror.prompt.first_point` |
| `"Specify second point of mirror line"` | `cmd.mirror.prompt.second_point` |
| `"Erase source objects"` | `cmd.mirror.prompt.erase_source` |
| `"The two points of the mirror line must differ."` | `cmd.mirror.msg.points_differ` |
| `"Specify offset distance"` | `cmd.offset.prompt.distance` |
| `"Specify second point"` | `cmd.offset.prompt.second_point` |
| `"Select object to offset"` | `cmd.offset.prompt.select` |
| `"Specify point on side to offset"` | `cmd.offset.prompt.side` |
| `"Specify through point"` | `cmd.offset.prompt.through` |
| `"Requires a positive, nonzero distance."` | `cmd.offset.msg.positive_distance` |
| `"Nothing found at that point."` | `cmd.offset.msg.nothing_found` |
| `two sites: Input and offsetEntity default case` | `cmd.offset.msg.cannot_offset` |
| `const degenerateOffset` | `cmd.offset.msg.degenerate` |
| `four sites in offsetEntity` | `cmd.offset.msg.through_on_object` |
| `"Select first " + c.noun()  (FILLET: object)` | `cmd.corner.prompt.select_first_object` |
| `"Select first " + c.noun()  (CHAMFER: line)` | `cmd.corner.prompt.select_first_line` |
| `"Select second " + c.noun()  (FILLET)` | `cmd.corner.prompt.select_second_object` |
| `"Select second " + c.noun()  (CHAMFER)` | `cmd.corner.prompt.select_second_line` |
| `"Specify fillet radius"` | `cmd.corner.prompt.fillet_radius` |
| `"Specify first chamfer distance"` | `cmd.corner.prompt.chamfer_d1` |
| `"Specify second chamfer distance"` | `cmd.corner.prompt.chamfer_d2` |
| `"(TRIM mode) Current chamfer Dist1 = %s, Dist2 = %s"` | `cmd.corner.msg.chamfer_settings` |
| `"Current settings: Mode = TRIM, Radius = %s"` | `cmd.corner.msg.fillet_settings` |
| `"Requires a non-negative distance."` | `cmd.corner.msg.non_negative` |
| `"Nothing found at that point."` | `cmd.corner.msg.nothing_found` |
| `arg: i18n.CommandName("FILLET"/"CHAMFER") upper-cased (c.title())` | `cmd.corner.msg.supports_two_lines` |
| `"Select a different line."` | `cmd.corner.msg.select_different` |
| `"Lines are parallel."` | `cmd.corner.msg.parallel` |
| `"Chamfer distances are too large for the lines."` | `cmd.corner.msg.chamfer_too_large` |
| `"Radius is too large for the lines."` | `cmd.corner.msg.radius_too_large` |

### `cmdsys/commands/draw.go`

| Literal | Key |
|---|---|
| `"Specify first point"` | `cmd.line.prompt.first_point` |
| `"Specify next point"` | `cmd.line.prompt.next_point` |
| `"Specify start point"` | `cmd.pline.prompt.start_point` |
| `"Specify next point"` | `cmd.pline.prompt.next_point` |
| `"Specify center point for circle"` | `cmd.circle.prompt.center` |
| `"Specify diameter of circle"` | `cmd.circle.prompt.diameter` |
| `"Specify radius of circle"` | `cmd.circle.prompt.radius` |
| `"Value must be positive and nonzero."` | `cmd.common.msg.positive` |
| `"Specify start point of arc"` | `cmd.arc.prompt.start_point` |
| `"Specify second point of arc"` | `cmd.arc.prompt.second_point` |
| `"Specify end point of arc"` | `cmd.arc.prompt.end_point` |
| `"The points are collinear; no arc was drawn."` | `cmd.arc.msg.collinear` |
| `"Specify first corner point"` | `cmd.rectang.prompt.first_corner` |
| `"Specify other corner point"` | `cmd.rectang.prompt.other_corner` |

### `cmdsys/commands/measure.go`

| Literal | Key |
|---|---|
| `"Specify first corner point"` | `cmd.area.prompt.first_corner` |
| `"Specify next corner point or press ENTER for total"` | `cmd.area.prompt.next_corner` |
| `"Area = %s, Perimeter = %s"` | `cmd.area.msg.result` |
| `label arg comes from entityArea: use cmd.area.label.*` | `cmd.area.msg.result_object` |
| `"Circumference"` | `cmd.area.label.circumference` |
| `"Perimeter"` | `cmd.area.label.perimeter` |
| `"Object has no area."` | `cmd.area.msg.no_area` |
| `"Select object"` | `cmd.common.prompt.select_object` |
| `"No object found."` | `cmd.common.msg.no_object_found` |
| `"Select arc or circle"` | `cmd.radius.prompt.select` |
| `"Select an arc or a circle."` | `cmd.radius.msg.select_arc_or_circle` |
| `"Radius = %s, Diameter = %s"` | `cmd.radius.msg.result` |
| `"Select arc, circle, line or <specify vertex>"` | `cmd.angle.prompt.select` |
| `"Specify angle vertex"` | `cmd.angle.prompt.vertex` |
| `"Specify first angle endpoint"` | `cmd.angle.prompt.first_endpoint` |
| `"Specify second angle endpoint"` | `cmd.angle.prompt.second_endpoint` |
| `"Select second line"` | `cmd.angle.prompt.second_line` |
| `"Select an arc, a circle or a line."` | `cmd.angle.msg.select_object` |
| `"Select a line."` | `cmd.angle.msg.select_line` |
| `"Angle = %s°"` | `cmd.angle.msg.result` |
| `"Enter an option"` | `cmd.measuregeom.prompt.option` |

### `cmdsys/commands/modify.go`

| Literal | Key |
|---|---|
| `"Value must be positive and nonzero."` | `cmd.common.msg.positive` |
| `"Select objects"` | `cmd.common.prompt.select_objects` |
| `ctx.Printf("%d found", n)  -> Plural("cmd.common.msg.found", n)` | `cmd.common.msg.found_one` |
| `(plural form of cmd.common.msg.found)` | `cmd.common.msg.found_other` |
| `"Specify base point"` | `cmd.modify.prompt.base_point` |
| `"Specify second point"` | `cmd.modify.prompt.second_point` |
| `"Specify scale factor"` | `cmd.modify.prompt.scale_factor` |
| `"Specify rotation angle"` | `cmd.modify.prompt.rotation_angle` |

### `cmdsys/commands/settings.go`

| Literal | Key |
|---|---|
| `"Settings are not available."` | `cmd.options.msg.unavailable` |

### `cmdsys/commands/trim.go`

| Literal | Key |
|---|---|
| `ctx.Printf("%d found", n)  -> Plural("cmd.common.msg.found", n)` | `cmd.common.msg.found_one` |
| `(plural form of cmd.common.msg.found)` | `cmd.common.msg.found_other` |
| `"Select object"` | `cmd.common.prompt.select_object` |
| `"No object found at that point."` | `cmd.common.msg.no_object_at_point` |
| `"Select object to trim"` | `cmd.trim.prompt.select_object` |
| `"Select cutting edges <select all>"` | `cmd.trim.prompt.select_edges` |
| `"Cannot trim this object."` | `cmd.trim.msg.cannot_trim` |
| `"Object does not intersect an edge."` | `cmd.trim.msg.no_intersection` |
| `"Select object to extend"` | `cmd.extend.prompt.select_object` |
| `"Cannot extend a closed polyline."` | `cmd.extend.msg.closed_polyline` |
| `"Cannot extend this object."` | `cmd.extend.msg.cannot_extend` |
| `"No boundary edge in the extension of that end."` | `cmd.extend.msg.no_boundary` |
| `"Specify second break point"` | `cmd.break.prompt.second_point` |
| `"Specify first break point"` | `cmd.break.prompt.first_point` |
| `"Cannot break this object."` | `cmd.break.msg.cannot_break` |
| `"Cannot break a circle at a single point."` | `cmd.break.msg.circle_one_point` |
| `"Select source object or multiple objects to join at once"` | `cmd.join.prompt.select_source` |
| `"Select objects to join to source"` | `cmd.join.prompt.select_others` |
| `"Cannot join the source object."` | `cmd.join.msg.cannot_join_source` |
| `"%d %s cannot be joined." plural(n)` | `cmd.join.msg.skipped_one` |
| `"%d objects cannot be joined."` | `cmd.join.msg.skipped_other` |
| `"%d %s joined." plural(n)` | `cmd.join.msg.joined_one` |
| `"%d objects joined."` | `cmd.join.msg.joined_other` |
| `"0 objects joined."` | `cmd.join.msg.none_joined` |
| `"Select objects to explode"` | `cmd.explode.prompt.select` |
| `"%d %s cannot be exploded." plural(n)` | `cmd.explode.msg.refused_one` |
| `"%d objects cannot be exploded."` | `cmd.explode.msg.refused_other` |
| `"%d %s exploded." plural(n)` | `cmd.explode.msg.exploded_one` |
| `"%d objects exploded."` | `cmd.explode.msg.exploded_other` |

### `cmdsys/commands/util.go`

| Literal | Key |
|---|---|
| `closed: %v (bool printed as true/false)` | `common.yes` |
| `closed: %v (bool printed as true/false)` | `common.no` |
| `"Everything has been undone."` | `cmd.undo.msg.everything_undone` |
| `"Nothing to redo."` | `cmd.redo.msg.nothing_to_redo` |
| `"Specify corner of window"` | `cmd.zoom.prompt.corner` |
| `"Specify first corner"` | `cmd.zoom.prompt.first_corner` |
| `"Specify opposite corner"` | `cmd.zoom.prompt.opposite_corner` |
| `"Layer name to make current, or name=color <%s>"` | `cmd.layer.prompt.name` |
| `"Color must be an AutoCAD Color Index from 1 to 255."` | `cmd.layer.msg.bad_color` |
| `"Layer %q created."` | `cmd.layer.msg.created` |
| `"Specify first point"` | `cmd.dist.prompt.first_point` |
| `"Specify next point"` | `cmd.dist.prompt.next_point` |
| `"Specify second point"` | `cmd.dist.prompt.second_point` |
| `"Distance = %s"` | `cmd.dist.msg.total` |
| `"Distance = %s,  Angle = %s°,  Delta X = %s,  Delta Y = %s"` | `cmd.dist.msg.result` |
| `"Specify point"` | `cmd.id.prompt.point` |
| `"X = %s     Y = %s"` | `cmd.id.msg.result` |
| `first arg e.Class() is the DXF class name; keep` | `cmd.list.msg.header` |
| `"    from point %s  to point %s  Length = %s"` | `cmd.list.msg.line` |
| `"    center point %s  radius %s"` | `cmd.list.msg.circle` |
| `"    center point %s  radius %s  start angle %s°  end angle %s°"` | `cmd.list.msg.arc` |
| `closed: %v  -> pass T("common.yes"/"common.no") as %s` | `cmd.list.msg.polyline` |
| `"File commands are not available on this platform."` | `cmd.file.msg.unavailable` |
| `"Enter path of drawing to " + verb` | `cmd.file.prompt.open_path` |
| `"Enter path of drawing to " + verb` | `cmd.file.prompt.save_path` |
| `"Save failed: %v"` | `cmd.file.msg.save_failed` |
| `"Saved %s (%d entities)."` | `cmd.file.msg.saved` |
| `"Open failed: %v"` | `cmd.file.msg.open_failed` |
| `"Opened %s (%d entities)."` | `cmd.file.msg.opened` |
| `"... and %d more warnings."` | `cmd.file.msg.more_warnings` |
| `"Warning: %s"` | `cmd.file.msg.warning` |

### `cmdsys/input.go`

| Literal | Key |
|---|---|
| `" or ["  (Prompt.String; use " " + T("input.or") + " [")` | `input.or` |
| `ErrInvalidPoint -> capitalize(err.Error()) + "."` | `input.err.invalid_point` |
| `ErrInvalidValue` | `input.err.invalid_value` |
| `ErrNoBase` | `input.err.no_base` |
| `ErrInvalidInput` | `input.err.invalid_input` |

### `cmdsys/session.go`

| Literal | Key |
|---|---|
| `"Command:"` | `session.command_prompt` |
| `"Command: " + cmd.Name()` | `session.command_echo` |
| `"Unknown command \"" + NAME + "\". Press F1 for help."` | `session.unknown_command` |
| `cmd.Name() + " is not available to agents."` | `session.agent_forbidden` |
| `"*Cancel*"` | `session.cancel` |
| `"Error: %s failed: %v."` | `session.internal_error` |
| `name := "command"` | `session.fallback_command_name` |
| `ErrInvalidPoint -> capitalize(err.Error()) + "."` | `input.err.invalid_point` |
| `ErrInvalidValue` | `input.err.invalid_value` |
| `ErrNoBase` | `input.err.no_base` |
| `ErrInvalidInput` | `input.err.invalid_input` |

### `cmdsys/snap.go`

| Literal | Key |
|---|---|
| `"Endpoint"` | `snap.endpoint` |
| `"Midpoint"` | `snap.midpoint` |
| `"Center"` | `snap.center` |
| `"Quadrant"` | `snap.quadrant` |
| `"Intersection"` | `snap.intersection` |
| `"Nearest"` | `snap.nearest` |
| `"Snap"` | `snap.generic` |

### `fileio/dxf/read.go`

| Literal | Key |
|---|---|
| `"kept %d %s entities as unknown objects (not displayed or editable)"` | `dxf.warn.kept_unknown` |
| `"%s handle %s is already used; renumbered to %s"` | `dxf.warn.handle_renumbered` |
| `"dxf: line %d: group code %q has no value"` | `dxf.err.no_value` |
| `"dxf: line %d: bad group code %q (binary DXF is not supported)"` | `dxf.err.bad_group_code` |
| `"dxf: section %q has no ENDSEC (truncated file?)"` | `dxf.err.no_endsec` |

### `fileio/dxf/write.go`

| Literal | Key |
|---|---|
| `"%s %s not written (the class does not exist in R12; it needs the newer writer, backlog G2)"` | `dxf.warn.not_written_r12` |
| `"%s %s not written (not supported by the R12 writer)"` | `dxf.warn.not_written_unsupported` |
| `"%s in block %s not written (the class does not exist in R12; it needs the newer writer, backlog G2)"` | `dxf.warn.block_not_written` |

### `fileio/format.go`

| Literal | Key |
|---|---|
| `"no reader for %q files"` | `fileio.err.no_reader` |
| `"no writer for %q files"` | `fileio.err.no_writer` |

### `settings/file.go`

| Literal | Key |
|---|---|
| `"%s must be %s, got %v; using the default"` | `settings.warn.wrong_type` |

### `settings/settings.go`

| Literal | Key |
|---|---|
| `"theme is empty; using %q"` | `settings.warn.theme_empty` |
| `"ui_scale %v is out of range or off-step; using %v"` | `settings.warn.ui_scale` |
| `"language %q is not one of %v; using %q"` | `settings.warn.language` |
| `"wheel %q is not one of %v; using %q"` | `settings.warn.wheel` |
| `"%s %d is out of range; using %d"` | `settings.warn.out_of_range` |

### `ui/agent.go`

| Literal | Key |
|---|---|
| `"Ask the agent"` | `ui.agent.input_placeholder` |
| `"starting %s..."` | `ui.agent.status.starting` |
| `"working..."` | `ui.agent.status.working` |
| `"ready"` | `ui.agent.status.ready` |
| `"stopped"` | `ui.agent.status.stopped` |
| `"No agent is available: install opencode, claude or codex, or pass -agent-cmd."` | `ui.agent.warn.no_agent` |
| `"The agent is still working; wait or cancel before asking again."` | `ui.agent.warn.busy` |
| `"cannot start the MCP server: %v"` | `ui.agent.err.mcp` |
| `"cannot start " + harness + ": " + err` | `ui.agent.err.start` |

### `ui/agent_store.go`

| Literal | Key |
|---|---|
| `return "New"` | `ui.agent.tab_new` |
| `"just now"` | `ui.agent.time.just_now` |
| `"%d min ago"` | `ui.agent.time.minutes_ago` |
| `"%d h ago"` | `ui.agent.time.hours_ago` |
| `"%d d ago"` | `ui.agent.time.days_ago` |
| `Go reference-time layout, not prose` | `ui.agent.time.date_layout` |
| `"agent history unavailable: %v"` | `ui.agent.warn.history_unavailable` |
| `"cannot list agent sessions: %v"` | `ui.agent.warn.cannot_list` |
| `"cannot load that agent session: %v"` | `ui.agent.warn.cannot_load` |
| `"agent history is not saved: %v"` | `ui.agent.warn.not_saved` |
| `"agent history write failed: %v"` | `ui.agent.warn.write_failed` |

### `ui/agent_view.go`

| Literal | Key |
|---|---|
| `NewText("Agent") on the AI button` | `ui.toolbar.agent` |
| `"Opens the agent panel: describe a drawing in words and the agent draws it"` | `ui.toolbar.agent_tip` |
| `"AGENT"` | `ui.agent.title` |
| `"+"` | `ui.agent.new_tab` |
| `"New conversation — opens another tab"` | `ui.agent.new_tab_tip` |
| `"Close this conversation tab"` | `ui.agent.close_tab_tip` |
| `"History"` | `ui.agent.history` |
| `"History — reopen an earlier conversation of this project"` | `ui.agent.history_tip` |
| `"No earlier conversations in this project"` | `ui.agent.history_empty` |
| `"(empty)"` | `ui.agent.history_untitled` |
| `sub += ", open"` | `ui.agent.history_open` |
| `"Agent — chooses which agent CLI runs this conversation"` | `ui.agent.pick_tip` |
| `"No agent CLI on PATH (opencode, claude, codex)"` | `ui.agent.no_cli` |
| `"Start"` | `ui.agent.start` |
| `"Start — launches the selected agent"` | `ui.agent.start_tip` |
| `"Stop"` | `ui.agent.stop` |
| `"Stop — ends the agent process"` | `ui.agent.stop_tip` |
| `"Describe a drawing and the agent will draw it with the same commands you use."` | `ui.agent.empty_hint` |
| `"Send"` | `ui.agent.send` |
| `"Send — sends your message to the agent (Enter)"` | `ui.agent.send_tip` |
| `"Cancel"` | `ui.agent.cancel` |
| `"Cancel — interrupts the agent's current turn"` | `ui.agent.cancel_tip` |
| `"You"` | `ui.agent.you` |
| `"> Thinking" / "v Thinking": keep the marker, translate the word` | `ui.agent.thinking` |
| `"a tool"` | `ui.agent.tool_default` |
| `"Permission requested"` | `ui.agent.permission` |
| `"answered"` | `ui.agent.answered` |
| `"Plan"` | `ui.agent.plan` |

### `ui/settings_view.go`

| Literal | Key |
|---|---|
| `NewText("Settings") on the gear button` | `ui.toolbar.settings` |
| `"Settings — theme, UI scale, drawing aids, language (Ctrl+, / OPTIONS)"` | `ui.toolbar.settings_tip` |
| `NewText("Settings").FontWeight(700).TextSize(16)` | `ui.settings.title` |
| `"Close  Esc"` | `ui.settings.close` |
| `"Close — closes the settings panel (Esc)"` | `ui.settings.close_tip` |
| `"APPEARANCE"` | `ui.settings.section_appearance` |
| `"Theme — applies the %s theme"` | `ui.settings.theme_tip` |
| `"UI scale"` | `ui.settings.ui_scale` |
| `fmt.Sprintf("%d%%", ...)` | `ui.settings.ui_scale_value` |
| `"UI scale — enlarges or reduces the whole interface, text included (75 to 200 percent)"` | `ui.settings.ui_scale_tip` |
| `"Smaller — reduces the UI scale by 5%"` | `ui.settings.ui_scale_down_tip` |
| `"Larger — increases the UI scale by 5%"` | `ui.settings.ui_scale_up_tip` |
| `"Reset — back to 100%"` | `ui.settings.ui_scale_reset_tip` |
| `"DRAWING"` | `ui.settings.section_drawing` |
| `"What a drawing starts with, and what applies right now."` | `ui.settings.drawing_hint` |
| `"Grid (F7)"` | `ui.settings.grid` |
| `"Grid — whether the grid starts on (F7 toggles it now)"` | `ui.settings.grid_tip` |
| `"Object snap (F3)"` | `ui.settings.osnap` |
| `"Object snap — whether object snap starts on (F3 toggles it now)"` | `ui.settings.osnap_tip` |
| `"Ortho (F8)"` | `ui.settings.ortho` |
| `"Ortho — whether ortho starts on (F8 toggles it now)"` | `ui.settings.ortho_tip` |
| `"Mouse wheel"` | `ui.settings.wheel` |
| `"Mouse wheel — what the wheel does over the drawing"` | `ui.settings.wheel_tip` |
| `"Auto"` | `ui.settings.wheel_auto` |
| `"Zoom"` | `ui.settings.wheel_zoom` |
| `"Pan"` | `ui.settings.wheel_pan` |
| `"Auto: the wheel zooms, Shift pans."` | `ui.settings.wheel_hint_auto` |
| `"Zoom: the wheel zooms, Shift pans."` | `ui.settings.wheel_hint_zoom` |
| `"Pan: the wheel pans, Ctrl or Cmd zooms."` | `ui.settings.wheel_hint_pan` |
| `new section (settings agent adds settings.Language)` | `ui.settings.section_language` |
| `"Language"` | `ui.settings.language` |
| `"Language — language of menus, prompts and messages; command names stay valid in English"` | `ui.settings.language_tip` |
| `"Automatic"` | `ui.settings.language_auto` |
| `"English"` | `ui.settings.language_en` |
| `"Italiano"` | `ui.settings.language_it` |
| `"Automatic follows the system language (LC_ALL, LC_MESSAGES, LANG)."` | `ui.settings.language_hint` |
| `"AGENT"` | `ui.settings.section_agent` |
| `"Default harness"` | `ui.settings.default_harness` |
| `"Default agent — the agent CLI the panel selects first"` | `ui.settings.default_harness_tip` |
| `{"", "Auto"} in the harness options` | `ui.settings.harness_auto` |
| `"Auto picks the first agent installed on PATH."` | `ui.settings.agent_auto_hint` |
| `"Kept in memory: no config folder here."` | `ui.settings.in_memory` |
| `"Saved to " + p.path` | `ui.settings.saved_to` |
| `"Reset to defaults"` | `ui.settings.reset` |
| `"Reset — restores every setting to its default; the window size is kept"` | `ui.settings.reset_tip` |

### `ui/themes.go`

| Literal | Key |
|---|---|
| `Label: "Dark" (ui/themes.go; key = theme Name, "-" -> "_")` | `ui.settings.theme.dark` |
| `Label: "Light"` | `ui.settings.theme.light` |
| `Label: "Classic"` | `ui.settings.theme.classic` |
| `Label: "High contrast"` | `ui.settings.theme.high_contrast` |

### `ui/workspace.go`

| Literal | Key |
|---|---|
| `"Type a command"` | `ui.cmdline.placeholder` |
| `ws.button("line", "Line", ...)` | `ui.toolbar.line` |
| `ws.button("pline", "Polyline", ...)` | `ui.toolbar.pline` |
| `ws.button("circle", "Circle", ...)` | `ui.toolbar.circle` |
| `ws.button("arc", "Arc", ...)` | `ui.toolbar.arc` |
| `ws.button("rect", "Rect", ...)` | `ui.toolbar.rect` |
| `ws.button("move", "Move", ...)` | `ui.toolbar.move` |
| `ws.button("copy", "Copy", ...)` | `ui.toolbar.copy` |
| `ws.button("rotate", "Rotate", ...)` | `ui.toolbar.rotate` |
| `ws.button("scale", "Scale", ...)` | `ui.toolbar.scale` |
| `ws.button("erase", "Erase", ...)` | `ui.toolbar.erase` |
| `ws.button("dist", "Dist", ...)` | `ui.toolbar.dist` |
| `ws.button("area", "Area", ...)` | `ui.toolbar.area` |
| `ws.button("undo", "Undo", ...)` | `ui.toolbar.undo` |
| `ws.button("redo", "Redo", ...)` | `ui.toolbar.redo` |
| `ws.button("zoom", "Zoom", ...)` | `ui.toolbar.zoom` |
| `heading("LAYERS")` | `ui.panel.layers` |
| `heading("PROPERTIES")` | `ui.panel.properties` |
| `state := "on"` | `ui.layers.on` |
| `"off"` | `ui.layers.off` |
| `"Layer visibility — click to show or hide this layer"` | `ui.layers.visibility_tip` |
| `"Make current — new objects are drawn on this layer (LAYER / LA)"` | `ui.layers.current_tip` |
| `"Layer colour (ACI %d)"` | `ui.layers.swatch_tip` |
| `"No selection"` | `ui.props.none` |
| `fmt.Sprintf("%d selected", n) -> Plural("ui.props.selected", n)` | `ui.props.selected_one` |
| `"%d selected"` | `ui.props.selected_other` |
| `"Layer: "+e.Common().Layer` | `ui.props.layer` |
| `"Handle: "+e.Handle().String()` | `ui.props.handle` |
| `"Cursor position — model units"` | `ui.status.coords_tip` |
| `"GRID"` | `ui.status.grid` |
| `"Grid — shows or hides the drawing grid (F7)"` | `ui.status.grid_tip` |
| `"OSNAP"` | `ui.status.osnap` |
| `"Object snap — locks the pointer onto endpoints, midpoints, centres, quadrants and intersections (F3)"` | `ui.status.osnap_tip` |
| `"ORTHO"` | `ui.status.ortho` |
| `"Ortho — constrains movement to horizontal and vertical (F8)"` | `ui.status.ortho_tip` |
| `(count is not pluralised; 1 oggetti is accepted)` | `ui.status.summary` |
