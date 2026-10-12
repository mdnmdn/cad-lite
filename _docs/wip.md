# WIP registry

Activity registry for the GPUI port. Each activity has an id, a status, and
dependencies. Statuses: `done`, `in-progress`, `blocked`, `todo`. Contract:
`_docs/00-api-contract.md` (change signatures there first). The original plan
(`_docs/plan.md`) is historical.

Last updated: 2026-10-12 (rows A17-A18 added; follow-ups refreshed).

## Registry

| ID  | Activity | Status | Depends on | Files |
|-----|----------|--------|------------|-------|
| A1  | Core UI (geometry, color, style, flexbox layout, div, scene, raster, events, keymap, focus, entity/context/app, window pipeline, font shaper) | done | — | `gpui/{geometry,color,style,styled,layout,element,div,scene,raster,bounds,events,keymap,focus,entity,context,app,subscription,window,platform,font,text}.go` |
| A2  | Text elements: `Text`, `StyledText`, `Str`, style helpers | done | A1 | `gpui/text_elem.go` |
| A3  | TextInput (PlatformInputHandler, editing keys, submit/blur) | done | A1 | `gpui/input.go` |
| A4  | Headless platform + `RenderToPNG`/`RenderToImage` | done | A1 | `gpui/headless.go` (new) |
| A5  | Clipboard via OS tools (pbpaste/pbcopy, powershell, wl/xclip/xsel) | done | — | `gpui/clipboard.go` (new) |
| A6  | Ebiten backend (`gpui.Platform`, input mapping, present/draw loop) | done | A5 | `gpui/ebitenplatform/` (new) |
| A7 | Todo app (`examples/todo`, `-png` headless + interactive modes) | done | A2, A3, A4, A6 | `examples/todo/main.go` (new) |
| A7b | Counter example (`examples/counter`: buttons with hover styles, up/down keybindings) | done | A2, A4, A6 | `examples/counter/main.go` (new) |
| A7c | Gallery example (`examples/gallery`: layout/styling/StyledText showcase) | done | A2, A4, A6 | `examples/gallery/main.go` (new) |
| A8 | Verification: `gofmt -l .`, `go vet ./...`, `go test ./...`, `go mod tidy`, PNG smoke test | done | A4–A7 | — |
| A9  | Port gpui-ce examples: `hello_world`, `actions_and_keybinds`, `layout`, `styling`, `text`, `tree`, `interactive_elements` | done | A7 | `examples/*/main.go` |
| A10 | Core features wave: linear gradients (sRGB/Oklab), box shadows, element opacity, scroll containers + `ScrollHandle` + scrollbars, `UniformList`, tab stops (`FocusNext/FocusPrev`), `Hover`/`Focus`/`FocusVisible` styles | done | A1 | `gpui/{background,shadow,scroll,uniform_list,div,focus,window}.go` |
| A11 | Port gpui-ce examples: `scrollable`, `uniform_list`, `opacity`, `border_and_shadow_backgrounds`, `tab_stop`, `focus_visible`, `gradient` | done | A9, A10 | `examples/*/main.go` |
| A12 | Text fixes: text style inheritance (`TextFields`), line height, newlines, font family lookup + system fallback, TextInput placeholder/caret, shaper installed for every window, retina glyphs | done | A2, A3 | `gpui/{text,text_elem,font,input,style,styled}.go` |
| A13 | Layout/events/app fixes: margins + auto margins, `Flex1` basis 0 / `FlexAuto` / `FlexNone` / `FlexBasis`, rounded child clips, scene-order paint, custom element layout, action keys + bare-key bindings, entity release, dispatch paths, click identity, clipboard timeouts | done | A1 | `gpui/{layout,styled,styled_layout,div,window,keymap,entity,app,clipboard}.go` |
| A14 | Performance: bounds-limited rasterizer fills (2.5s to 1ms/frame at 2x), shaping cache | done | A1 | `gpui/{raster,text}.go` |
| A15 | Frame snapshot key in the ebiten backend (`cmd/ctrl-shift-s`, `WithSnapshotKey`) | done | A6 | `gpui/ebitenplatform/snapshot.go` |
| A16 | Docs: `docs/` (architecture, concepts, getting-started, examples), root `README.md` | done | A1-A15 | `docs/`, `README.md` |
| A17 | Review-fix wave (commit `1e6fbfd`): entity slot created before `build` (Observe/OnRelease from build attach), `Release` during a flush only schedules, effect flush counts as an `Update`, `DispatchEvent` runs inside `App.Update`; `UniformList` and `RequestLayoutWithChildren` text-style inheritance; paint-time hover/focus text overrides (colour/strikethrough only); mouse-down auto-focus skipped if a handler moved focus; `Text`/`StyledText` `Opacity`; `"ui-monospace"` generic family; scroll offsets limited to scroll axes (`OverflowHidden` not programmatically scrollable); half-leading text baseline; `TextInput` paint fix; element-state pruning per frame. Docs refreshed (A18) | done | A10-A13 | `gpui/{app,entity,window,div,text_elem,uniform_list,scroll,font,input}.go`, `gpui/review_fixes*_test.go` |
| A18 | Docs audit against A17: architecture (element-state lifecycle, effect flush/Update nesting, paint-time text overrides, scene-order painting, rounded content masks), concept pages, plan marked historical | done | A16, A17 | `docs/`, `_docs/plan.md`, `AGENTS.md` |

## In progress

None. Pick the next `todo` activity with unmet dependencies; move its status to
`in-progress` here before starting.

## Notes

- **A3 fix (2026-10-12):** `onKeyDown` treated a *collapsed* selection
  (`Start == End`) as a range, so backspace/delete were no-ops and arrow keys
  didn't move the caret. Now only non-collapsed selections replace; collapsed
  selections move the caret. See `gpui/input_test.go`
  (`TestTextInputBackspaceThroughDispatch`, `TestTextInputArrowKeysMoveCaret`).
- **Contract conformance:** `IntRange` is UTF-16 code unit offsets (contract
  §10). `TestTextInputHandlerContract` expected `TextLengthUTF16("héllo") == 6`
  (precomposed é), which only holds under byte semantics; corrected to 5 and an
  astral-plane case (`"h😀llo"` → 6) added to pin UTF-16 semantics.
- A6 input mapping: ebiten wheel Y is inverted (normalized to positive = scroll
  down). Modifiers follow GPUI: Ctrl → `Control`, Cmd/Super → `ModPlatform`.
- **A6 (2026-10-12):** `gpui/ebitenplatform/{platform,input}.go` + tests. The
  window rasterizes and calls `Present`, and `SetInputHandler` is wired from
  focus. `New()` returns `gpui.Platform` (an interface), not `*gpui.Platform`.
- **A7 notes (2026-10-12):** examples build, vet and gofmt clean; `-png`,
  `-width`, `-height`, `-smoke` supported. Counter hover uses
  `OnHoverStart`/`OnHoverEnd` (each handler calls `Refresh`).

## Open follow-ups

Missing features:

- Absolute positioning (`position: absolute`); examples approximate it.
- `flex-wrap`; examples split wrapped rows by hand.
- `Active()` (pressed) style.
- Animation frames: no API to request the next frame on a timer; examples
  step state in `Render` and call `Refresh`.
- Images and SVG elements.
- Colour emoji (bitmap fonts draw as tofu).
- Scrollbar thumb drag (scrollbars are display-only).

Gaps and limitations:

- Headless keyboard-focus seeding: add a `RenderOption` so `RenderToPNG` can
  start with a focused element and keyboard modality (examples use flags).
- Hover/focus text refinements cascade at paint time for colour and
  strikethrough only (`paintTextFields`); `TextSize`, `FontWeight`,
  `FontFamily` and `TextLineHeight` in `Hover`/`Focus`/`FocusVisible` are
  ignored for descendants because layout is already done.
- The hover/focus text override stops at custom `RequestLayoutWithChildren`
  containers: they push no paint-time override, so an outer div's hover colour
  keeps applying inside them and they cannot re-assert their own.
- `Text.Bg`/`Border`/`Shadow` builders exist (via `StyledMixin`) but text
  leaves paint no box; wrap the text in a styled `Div`. Either paint the box
  in `Text`/`StyledText` or drop the builders.
- Hitboxes are clipped to rectangles, not to rounded corners.
- Interactive ebiten window has never been run in the agent environment (no
  Metal device); headless rendering is what the tests cover.
- `Keystroke.String` prints `ModPlatform` as `cmd-` on Linux/Windows.
- Windows cannot clear the clipboard.
- Sprite raster loop iterates the clip window rather than the sprite bounds.
- TextInput measures text at scale 1.
- StyledText runs wrap independently of each other.
- Borders are not included in layout sizes.
