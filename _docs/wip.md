# WIP registry

Activity registry for the GPUI port. Each activity has an id, a status, and
dependencies. Statuses: `done`, `in-progress`, `blocked`, `todo`. Contract:
`_docs/00-api-contract.md` (change signatures there first). Plan narrative:
`_docs/plan.md`.

Last updated: 2026-10-12.

## Registry

| ID  | Activity | Status | Depends on | Files |
|-----|----------|--------|------------|-------|
| A1  | Core UI (geometry, color, style, flexbox layout, div, scene, raster, events, keymap, focus, entity/context/app, window pipeline, font shaper) | done | — | `gpui/{geometry,color,style,styled,layout,element,div,scene,raster,bounds,events,keymap,focus,entity,context,app,subscription,window,platform,font,text}.go` |
| A2  | Text elements: `Text`, `StyledText`, `Str`, style helpers | done | A1 | `gpui/text_elem.go` |
| A3  | TextInput (PlatformInputHandler, editing keys, submit/blur) | done | A1 | `gpui/input.go` |
| A4  | Headless platform + `RenderToPNG`/`RenderToImage` | todo | A1 | `gpui/headless.go` (new) |
| A5  | Clipboard via OS tools (pbpaste/pbcopy, powershell, wl/xclip/xsel) | todo | — | `gpui/clipboard.go` (new) |
| A6  | Ebiten backend (`gpui.Platform`, input mapping, present/draw loop) | todo | A5 | `gpui/ebitenplatform/` (new) |
| A7 | Todo app (`examples/todo`, `-png` headless + interactive modes) | todo | A2, A3, A4, A6 | `examples/todo/main.go` (new) |
| A8 | Verification: `gofmt -l .`, `go vet ./...`, `go test ./...`, `go mod tidy`, PNG smoke test | todo | A4–A7 | — |

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
- `go.mod` already lists ebiten deps as indirect; `go mod tidy` in A8 will
  promote them to direct once `gpui/ebitenplatform` imports them.
- A6 input mapping per contract §17: ebiten wheel Y is inverted (normalize to
  positive = scroll down); `KeyMeta` → `Platform` on darwin, `KeyControl` →
  `Platform` on windows/linux.
