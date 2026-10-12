# go-gpui plan

> **Historical.** This is the original plan for the first implementation wave
> and is no longer maintained: it predates the examples, gradients, shadows,
> scroll containers, text inheritance and later fixes, and some signatures here
> (e.g. `New() *gpui.Platform`, `app.Run`) differ from the code. For current
> state see the activity registry and open follow-ups in
> [wip.md](wip.md), the user docs in [../docs/](../docs/README.md), and the
> API contract in [00-api-contract.md](00-api-contract.md).

POC port of Zed's GPUI to Go. Core rendering (geometry, color, style, flexbox
layout, elements, scene, raster, events, keymap, focus, entities, window
pipeline) is **done and tested**. This plan tracks the remaining platform and
application layer, in dependency order.

Contract: `_docs/00-api-contract.md` — change signatures there first, then in
code.

## Done

- `gpui/`: geometry, color, style/styled, layout (flexbox), element, div,
  scene, raster, bounds, events, keymap, focus, entity, context, app,
  subscription, window (full draw pipeline), platform interfaces + stub,
  font/text shaper (HarfBuzz via go-text/typesetting).
- `go build ./...` clean, `go test ./...` passes.

## Remaining work

### 1. `gpui/text_elem.go` — text elements (contract §5)

- `Text(s string) *Text`, `(*Text).Text(s)` — single-run leaf element.
- `StyledText` + `TextRun` + `NewStyledText(runs ...)` — multi-run paragraph.
- `Str(s string) IntoElement` helper so `Child(Str("hi"))` works.
- Both implement `Element` via an adapter (same ID-collision pattern as
  `divElement`): `RequestLayout` registers a leaf `LayoutNode` whose `Measure`
  calls the window's `TextMeasurer` (`MeasureWrapped`) with the wrap width from
  the layout constraint; `Paint` emits one `GlyphSprite` per shaped glyph plus
  an `Underline` when `TextStyle.Strikethrough`.
- Style-forwarding helpers on `*Text` mirroring the Div set.

### 2. `gpui/input.go` — TextInput (contract §10)

- `NewTextInput(placeholder)`, `Text()/SetText()`, `FocusHandle()`,
  `Render(w, cx) Element`, `OnSubmit`, `OnChange`.
- Implements `PlatformInputHandler` internally (UTF-16 ranges, selection,
  caret). Renders as a Div (focusable, `KeyContext("textbox")`) containing the
  text/placeholder; registers itself via `w.HandleInput` during prepaint when
  focused. Enter submits, Escape blurs (wired through keymap actions in the
  todo app).
- POC deviation (documented): plain struct owned by the root view, not an
  `Entity[T]`.

### 3. `gpui/headless.go` — headless platform + PNG rendering (contract §14)

- `RenderToPNG(path, w, h, scale, root, opts...)` / `RenderToImage(...)` —
  build root, layout at w×h, paint, rasterize, encode PNG (stdlib `image/png`).
- `RenderOption`s: `WithBackgroundColor`, `WithKeymap`, `WithTitle`.
- `HeadlessPlatform` / `HeadlessWindow` — in-memory `PlatformWindow` that
  keeps the last presented pixels; `Dispatch(input)` feeds synthetic input;
  `Pixels()/Image()/SavePNG(path)` for tests.

### 4. `gpui/clipboard.go` — clipboard (contract §16)

- `ClipboardRead() (string, error)` / `ClipboardWrite(s string) error` via OS
  tools: `pbpaste`/`pbcopy` (darwin), powershell (windows),
  `wl-paste`/`wl-copy` → `xclip` → `xsel` (linux).

### 5. `gpui/ebitenplatform/` — ebiten backend (contract §17)

- `New() *gpui.Platform`. `Run(onLaunch)` → `ebiten.RunGame(g)` with
  `Update/Draw/Layout`. `Layout` returns device pixels
  (`outside * Monitor().DeviceScaleFactor()`).
- `Update` polls input, forwards to `Window.DispatchEvent` inside `app.Update`:
  mouse press/release (+Click on same hitbox), move synthesis, wheel
  (normalize DeltaY so positive = scroll down), `AppendInputChars` →
  `KeyDownEvent{PreferCharacterInput:true}`, special keys via
  `inpututil.IsKeyJustPressed`, modifiers (Cmd/Platform mapping by GOOS),
  clipboard for cmd/ctrl+c/v/x.
- `Draw` → `screen.WritePixels(pix)` (premultiplied), skip when not dirty;
  `SetScreenClearedEveryFrame(false)`. `Present` buffers for next Draw.
- `RequestFrame` sets dirty flag observed by Update/Draw.
- Clean quit: `ebiten.Termination` from Update when `app.IsQuitting()`.
- Window title/size/resizing via `ebiten.SetWindowTitle/SetWindowSize/
  SetWindowResizingMode`.

### 6. `examples/todo/` — todo app (contract §18)

- `TodoApp` struct (todos, `*TextInput`, nextID) with `Render` per the
  contract's div tree; add on Enter/click, toggle on row click.
- `main`: flags `-png PATH`, `-width`, `-height`, `-smoke MS`; PNG mode via
  `gpui.RenderToPNG`; otherwise `ebitenplatform.New()` + keymap bindings
  (`enter`→add-todo, `escape`→blur in "textbox" context) + `app.Run`.

### 7. Verification

- `go mod tidy` (ebiten deps become direct), `gofmt -l .` clean,
  `go vet ./...`, `go test ./...`.
- Run `examples/todo -png /tmp/todo.png` to smoke-test the full pipeline.

## Conventions (from the contract)

1. Package `gpui` imports only stdlib (+ `x/image`, `go-text/typesetting` in
   font.go/text.go). Ebiten only in `gpui/ebitenplatform/`.
2. No comments unless non-obvious; all exported identifiers get doc comments.
3. Tests in `gpui/*_test.go`, external test package `gpui_test` unless
   internal access is needed.
4. `float32` geometry; convert to `int` only at raster boundaries.
