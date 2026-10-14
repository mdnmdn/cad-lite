# Roadmap

Single source of truth for project status, completed work and open work.
It replaces the former first-wave plan and work-in-progress notes
(activity registry). Contract: [api-contract.md](api-contract.md) (change
signatures there first). Layout of the repo: [project-structure.md](project-structure.md).

Last updated: 2026-10-14.

Current wave: **E1 — in progress**, owned by `l1`, coordinated by Jake.
Scope: animation frames, UI-thread timers, easing and deterministic headless
clock advancement; contract design precedes implementation.
I2 is committed in `4c93e7c`, I1 in `e6f5130`; I3 is unblocked.
E1 combined verification and independent review are pending.
Requested gpui-kit investigation **K1 — done**, researched by `gpui_kit`,
independently source-reviewed by `review_l1`; document/roadmap committed in
`a59d2f6`. Integration implementation remains the K2 backlog proposal.
L1, L2 and S1 are committed in `f9666fd`. Commits are authorized without
coauthor trailers.

## 1. Status summary

What works today:

- Core UI in `internal/gpui/` (stdlib + `x/image` + `go-text/typesetting`): geometry,
  colour, flexbox layout (grow/shrink/basis, margins incl. auto, gaps, min/max,
  row/column wrapping, axis gaps and align-content),
  parent-relative absolute positioning with pixel offsets and inset,
  `Div`, `Text`, `StyledText`, `TextInput` (selection highlight, drag/word/line
  selection, rune-safe caret navigation), `UniformList`, scroll containers
  with draggable scrollbar thumbs and track paging, borders, rounded corners and clips, linear
  gradients (sRGB/Oklab), box shadows (outset/inset), opacity, active / hover / focus /
  focus-visible styles, text style inheritance, font families with system
  fallback, HarfBuzz shaping and wrapping.
- Retained entities with observe/emit/release, keymap + actions + key contexts,
  tab order, focus handles, mouse/keyboard events, clipboard (OS tools).
- CPU rasterizer (Scene to premultiplied RGBA), PNG output, headless platform
  with synthetic input injection.
- Ebiten backend (`ebitenplatform`): window, input mapping, present loop,
  clipboard, snapshot key (`cmd/ctrl-shift-s`).
- 17 examples, each with `-png PATH` headless rendering; user docs in `docs/`.

Not yet working: see section 3 (animation, images/SVG,
grid, colour emoji, IME, multi-window, in-app drag and drop,
italic/intermediate weights, key chords). The interactive ebiten window has
never been run in the agent environment (no Metal device); headless rendering is
what the tests cover.

Verification commands (all must be clean before a change is done):

```
gofmt -l .          # no output
go vet ./...
go test ./...
go mod tidy         # after adding imports
go run ./examples/todo -png _tmp/todo.png   # pipeline smoke test
```

## 2. Done

Commit hashes from `git log --oneline` (oldest first within a group).

### Core port

| Commit | What |
|--------|------|
| `36b319e` | Baseline: gpui core (geometry, colour, style/styled, layout, element, div, scene, raster, bounds, events, keymap, focus, entity, context, app, subscription, window, platform, font/text shaper), text elements, `TextInput` |
| `fbd8f5f` | Flexbox layout fixes, text wrapping, flipped glyphs, bold faces, hover events, fluent wrappers |
| `182a5db` | Review fixes: dispatch paths, hover, click identity, modifiers, clipboard timeouts, flex resolution, retina glyphs, NRGBA output |

### Platform and headless

| Commit | What |
|--------|------|
| `e6b9568` | Headless platform (`RenderToPNG`/`RenderToImage`, `HeadlessWindow`), clipboard (pbpaste/pbcopy, powershell, wl-clipboard/xclip/xsel), ebiten backend |
| `8fdf9a2` | `Window.Draw` rasterizes and presents; input handler, `HitTest`, scale refresh |
| `74e1df8` | Default text shaper installed for every window, not just headless |
| `cb6d403` | Ebiten snapshot key (`cmd/ctrl-shift-s`, `WithSnapshotKey`) |
| `c48c7ec` | `ebitenplatform.WithWindowIcon` sets the window icon from several image sizes |

### Examples

| Commit | What |
|--------|------|
| `b62b0fc` | `todo`, `counter`, `gallery` |
| `94c6d73` | Counter hover via `OnHoverStart/End` |
| `f29241c` | gpui-ce ports: `hello_world`, `actions_and_keybinds`, `layout`, `styling`, `text`, `tree`, `interactive_elements` |
| `6fcdcb2` | gpui-ce ports: `scrollable`, `uniform_list`, `opacity`, `border_and_shadow_backgrounds`, `tab_stop`, `focus_visible`, `gradient` |

### Performance

| Commit | What |
|--------|------|
| `cb6d403` | Bounds-limited rasterizer fills (2.5 s to 1 ms/frame at 2x), shaping cache |

### Feature waves

| Commit | What |
|--------|------|
| `f0b549e` | Linear gradients, box shadows, element opacity, scroll containers + `ScrollHandle` + scrollbars, `UniformList`, tab stops (`FocusNext/FocusPrev`), `Hover`/`Focus`/`FocusVisible` styles |
| `5dbd7ea` | Text style inheritance, line height, newlines, font family + fallback; margins and auto margins, `Flex1` basis 0 / `FlexAuto` / `FlexNone` / `FlexBasis`, rounded child clips, scene-order paint, action keys + bare-key bindings, entity release, custom element layout |
| `69be5b1` | `TextInput` keeps placeholder and height when focused and empty; caret centred |
| `bbb9bac` (2026-10-14) | Themeable `TextInput` (`Style`, `PlaceholderColor`, `SelectionColor`); bundled Go Regular/Bold fallback font (always on `js`, `GPUI_NO_SYSTEM_FONTS=1` elsewhere). Evidence: all 17 example `-png` renders byte-identical (`cmp`) before and after; `TestTextInputStyleAppliesBackgroundAndPlaceholder`, `TestTextInputSelectionColor`, `TestBundledFontWhenSystemFontsDisabled`, `TestBundledFontRendersGlyphs`; `TestTextInputCaretUsesFullShapedLineAtBothScales` now pinned to the bundled font and passes (its caret-pixel sample depends on the system font's fractional advances); `GOOS=js GOARCH=wasm go build ./cmd/cad` grows the binary by about 0.29 MB |
| 2026-10-14 | Window-level UI zoom: `Window.SetUIScale`/`UIScale`; `Bounds` is the platform size / zoom, `ScaleFactor` the device scale x zoom, `DispatchEvent` divides pointer positions (mouse, click, wheel) by the zoom, so platforms need no change. Evidence: all 17 example `-png` renders byte-identical (`cmp`) at the default zoom of 1; `TestUIScaleShrinksLogicalSize`, `TestUIScaleMatchesDeviceScaleRender` (zoom 1.5 at device 1 renders byte-identically to device scale 1.5), `TestUIScaleClicksHitScaledElements`, `TestSetUIScaleClampsAndInvalidates` |
| 2026-10-14 | Files from the OS: new `FileDropEvent` and `Div.OnFileDrop` (hit-path dispatch like pinch), `App.OnOpenPaths`/`App.OpenPaths` for window-less open requests, queued until a handler is set (contract first). The ebiten backend turns `ebiten.DroppedFiles` into a `FileDropEvent` with the real paths (`ebiten.AbsPather`, ebiten 2.10; `droppedPaths` walks back from the common root ebiten picks, telling a dropped folder from a lead directory by comparing its listing with the disk) at the cursor, which GLFW moves to the drop point; on macOS `Run` installs an `NSAppleEventManager` handler for `kAEOpenDocuments` through `purego/objc` before `ebiten.RunGame`, and again from an `NSApplicationWillFinishLaunchingNotification` observer, pushing to a mutex-guarded queue drained into `App.OpenPaths` in `Update`. Evidence: `TestFileDrop*` (bubbling, StopPropagation, UI scale), `TestOpenPathsQueuesUntilHandlerIsSet`, `TestDroppedPaths*`, `TestOpenQueueDrainsBatchesInOrder`; vet for darwin, windows and js. **Not run: a real drop on any OS, a macOS Finder or Dock open.** A browser drop (no real paths) sends nothing; drop order is lost (sorted by name). |
| 2026-10-14 | Trackpad pinch: new `PinchEvent` and `Div.OnPinch` (contract first), headless-testable; macOS ebiten backend installs an `NSEvent` local monitor for `NSEventMaskMagnify` through `purego/objc` (`pinch_darwin.go`, installed via `dispatch_async_f` on the main queue; the main-thread block pushes to a mutex-guarded queue drained in `Update`; install failure logs and leaves pinch unsupported); js backend ORs `WheelEvent.ctrlKey` into wheel modifiers (`wheelctrl_js.go`) because ebiten derives Control only from key events, so a browser pinch was a plain scroll before. Evidence: `TestPinch*` (headless dispatch, bubbling, StopPropagation, UI scale), `TestCocoaPoint*`, `TestCocoaModifiers`, `TestPinchDelta`, `TestPinchQueue*`; vet and build for darwin arm64/amd64, windows, js. **Not run: a real macOS trackpad or browser pinch.** Smart magnify (double-tap) is not handled. |
| 2026-10-14 | `ScrollWheelEvent.Precise` (trackpad vs notch), set by the ebiten backend from the raw values (`wheelClassifier`: fractional, two-axis or just-after-precise deltas are precise at 10 px/unit, other whole deltas are notches at 20 px; `GOOS=js` passes CSS pixels through); the backend withholds the first pointer position until it changes or a button or the wheel is used (`pointerTracker`), since ebiten reports a meaningless position before the pointer enters. Evidence: all 17 example `-png` renders byte-identical (`cmp`) before and after; `TestWheelNotchDirectionAndScale`, `TestWheelPreciseHeuristic`, `TestWheelBrowserPixels`, `TestPointerTrackerIgnoresInitialPosition`; real window under Xvfb: `GODRAW_GUI_TEST=1 go test ./tools/gui` (`gestures.gui`). ebiten 2.10 has no pinch, touchpad or cursor-enter API on desktop, so pinch only arrives as Ctrl+wheel (macOS is covered by `PinchEvent`, above) and pointer-leave is not reported. |
| 2026-10-14 | Multi-line `TextInput` (`Multiline(maxLines)`, contract first): visual rows from hard and soft breaks, Up/Down with a goal column, per-row Home/End, Shift+Enter newline and Enter submit, triple-click selects the logical line, growth up to the line cap then vertical scroll that follows the caret, wheel scrolling; `ReplaceTextInRange` keeps newlines in a multi-line field and turns them into spaces in a single-line one, and the ebiten paste keeps `\n` (`printableText`). Evidence: `TestMultiline*`, `TestTextInputPasteNewlines`, `TestClipboardPasteIsAsync` (newlines); todo and gallery `-png` renders byte-identical (`cmp`) before and after; Cad-lite agent panel rendered with a 3-line and a wrapped 10-line prompt and inspected. **Not run: a real window (typing, IME, clipboard).** |
| 2026-10-14 | Tooltip bubbles size to wrapped text (relayout at the `MaxWidth` cap; `\n` is a hard break); `TextInput.SetPlaceholder`. Evidence: `TestTooltipWrappedTextBubbleCoversAllLines` (failed first), `TestTextInputSetPlaceholderRerenders`; no example renders changed |

### Verified roadmap milestones

Verified milestones; later waves also record their commit hashes here.

| ID | Status | What and evidence |
|----|--------|-------------------|
| L1 | done, `f9666fd` (2026-10-13) | Absolute positioning: `Position`, `Absolute` / `Relative`, edge and inset builders; out-of-flow sizing, constrained stretching, nested placement, overflow and scene-order overlap tests. Contract and user docs updated; new layout and revised cookbook snippets compile. Independent review found no correctness issues. `gofmt -l .`, `go vet ./...`, `go test ./...`, `git diff --check` and todo PNG smoke pass. Before/after PNGs inspected in `_tmp/l1-before` and `_tmp/l1-after`: layout changes only its stack region, opacity intentionally overlaps background text, todo is pixel-identical. Todo scale-2 benchmark: 1.29 ms before, 1.21 ms after (single runs; no regression observed). Real-window checks not run: no display/GPU access. |
| L2 | done, `f9666fd` (2026-10-13) | Row/column `FlexWrap` / `FlexNoWrap`, `RowGap` / `ColumnGap`, six align-content modes; per-line flex resolution, constraint and absolute-child coverage. Contract, user docs and project map updated; wrapping snippet compiles. Independent review found no blocking issues; stale contract wording corrected. `gofmt -l .`, `go vet ./...`, `go test ./...`, `git diff --check` and todo PNG smoke pass. Before/after PNGs in `_tmp/l2-before` and `_tmp/l2-after` inspected: default styling/border and todo/layout/opacity controls pixel-identical; narrow styling palette reflows and narrow border cards form one column instead of clipped pairs. Todo scale-2 benchmark: 1.30 ms/frame (single run, within baseline variation); allocation count unchanged. No real-window checks available. |
| S1 | done, `f9666fd` (2026-10-13) | `Div.Active` primary-button paint refinement with stable hit-path identity, ancestor activation, release/leave/removal/occlusion cancellation and precedence after Hover. Headless tests include stopped propagation, redraw lifecycle, inherited text/opacity and ignored layout refinements. Styling and gradient examples, contract and user docs updated; three snippets compile. Independent review found no blocking issues. `gofmt -l .`, `go vet ./...`, `go test ./...`, `git diff --check` and todo PNG smoke pass. Resting styling/gradient and todo/layout/opacity renders pixel-identical; pressed/released previews inspected in `_tmp/s1-after`, release matches hovered baseline. Paired isolated before/current benchmarks (1s, count 3) show no material S1 slowdown: Todo2 medians 1.964/2.009 ms, Todo1 .835/.866 ms, Gallery2 3.303/3.216 ms, allocation counts unchanged; elevated absolute times occur in both copies. No real-window checks available. |
| I1 | done, `e6f5130` (2026-10-13) | Shared scrollbar paint/hit geometry for Div and UniformList; primary thumb dragging is proportional and clamped, continues outside, releases outside; track press pages one viewport. Overlay/clipping, both axes, lifecycle/programmatic cancellation, tiny tracks, state/handle sync and content-click suppression covered by headless tests. Independent review found no blocking issues. `gofmt -l .`, `go vet ./...`, `go test ./...`, `git diff --check`, compiled drag snippet and todo PNG smoke pass. Seven resting example renders pixel-identical; virtual-list drag/release previews inspected in `_tmp/i1-after`. Todo2 benchmark 2.05 ms, Gallery2 3.21 ms on current host (prior same-host S1 medians 2.01/3.22 ms); allocation counts unchanged. No real-window checks available. |
| K1 | done, `a59d2f6` (2026-10-13) | [GPUI Kit integration investigation](gpui-kit-integration.md), pinned upstream `c5f8eca4a0759f8c0c7ed624eddfd03f1c93a5f2`; architecture, manifest versions, source paths and licence notices independently checked. Recommends selective native Go controls; package/API/activation decisions remain for contract review. No component integration, dependencies or Rust builds performed. |
| C1 | done (2026-10-14) | `Stroke` scene primitive (anti-aliased polyline, round joins/caps, union coverage so joints blend once, sub-pixel widths as alpha-scaled hairlines) with `Scene.InsertStroke`, `Window.PaintStroke`, scaling in `scaleScene`, merge in `Rasterizer.Render`; `Canvas` element (GPUI `canvas()`). Added for the go-draw CAD canvas, together with `TextInput.OnEscape` (Escape runs the callbacks and keeps focus, for a command line; regression test `TestTextInputOnEscapeKeepsFocus`). Unit tests for coverage, joints, clip, hairline, scaling, ordering and canvas bounds. Todo, gallery and border renders pixel-identical; todo benchmark within host noise, allocations unchanged. |
| I2 | done, `4c93e7c` (2026-10-13) | TextInput selection highlight, primary click/drag outside/release, Unicode word double-click and whole-line triple-click, Shift anchors/reversed ranges and Home/End. Rune-safe navigation/deletion and invalid UTF-16 range rejection have failing-before regressions. Capture identity/lifecycle, bubbling, shaping/cache, clip/layer and scale-1/2 pixels tested. Contract and user docs updated; new selection snippet compiles. Independent review found no blocking issues. `gofmt -l .`, `go vet ./...`, `go test ./...`, `git diff --check`, tidy (manifests unchanged) and todo PNG smoke pass. Eight resting renders pixel-identical; selection previews inspected. Long-input preview intentionally changes from wrapped to one clipped line; focused caret aligns to label/full-line advances. Paired isolated benchmarks show no I2 regression established under unstable host timings (Todo2 before/current medians 5.231/4.119 ms, ranges 5.064–5.408/3.584–10.531 ms); Todo allocations 1477→1421, gallery unchanged. No real-window checks available. |

L1 limits: logical-pixel offsets only; immediate parent border box is the anchor.
No relative translation, CSS static-position placement, fixed positioning or
z-index; absolute auto margins count as zero. These are documented in the contract.

L2 limit: wrap-reverse remains unsupported (optional in the milestone).

I2 limits: rune rather than grapheme navigation; no BiDi visual-order selection,
horizontal autoscroll, select-all or word-arrow shortcuts. L7 scale API work
remains open. Embedded CR/LF display as spaces while stored text is preserved.

I1 limits: no held track-repeat; bounds/content/track changes and programmatic
offsets cancel capture. Input clipping is rectangular; no generic drag/drop API.

S1 limits: primary button only, paint-time refinements and rectangular hit paths.
Leave cancels until another press; secondary event positions can also cancel
if they report a pointer outside the captured path.

### Review waves

| Commit | What |
|--------|------|
| `182a5db` | First review wave (see Core port) |
| `1e6fbfd` | Entity slot created before `build`; `Release` during a flush only schedules; effect flush counts as an `Update`; `DispatchEvent` runs inside `App.Update`; `UniformList` / `RequestLayoutWithChildren` text inheritance; paint-time hover/focus text overrides (colour/strikethrough); mouse-down auto-focus skipped if a handler moved focus; text `Opacity`; `"ui-monospace"` family; scroll offsets limited to scroll axes; half-leading baseline; `TextInput` paint fix; element-state pruning |

Notable correction (in `36b319e`..`182a5db` era): `onKeyDown` treated a
collapsed selection as a range (backspace/delete no-ops); fixed. `IntRange` is
UTF-16 code unit offsets (contract section 10).

### gpui-ce ports

Covered in Examples above. See section 3.6 for the per-example port table.

### Docs

| Commit | What |
|--------|------|
| `b840748` | `docs/` architecture and concepts |
| `e850904` | README, getting started, examples index, styling update |
| `c51c8a1` | Docs audit against code after the review-fix wave |
| this commit | `_docs/` restructure (`api-contract.md` rename, this roadmap, `project-structure.md`, `index.md`), `docs/` reference and guides, consistency pass |

## 3. Open work

Size: S under half a day, M about a day or two, L larger or cross-cutting.
Priority: P1 (blocks real apps), P2 (valuable), P3 (nice to have). IDs are
stable; "Deps" lists IDs that should land first. Every milestone also needs the
standard verification (section 1) and a contract update when signatures change.

### 3.1 Layout

| ID | Pri | Size | Deps | Milestone | Acceptance criteria |
|----|-----|------|------|-----------|---------------------|
| L3 | P3 | L | L1, L2 | CSS grid (`grid_cols`, `col_span`, `row_span`) - decide first whether to ship at all | Decision recorded here; if shipped, fixed and fractional tracks, spans, gap; `layout`, `interactive_elements`, `text` use it |
| L4 | P2 | S | - | Borders included in layout sizes (currently not) | A bordered box with fixed width keeps content width minus border; tests |
| L5 | P3 | S | - | Resize and scale-change test coverage and API. Handling itself exists: `ebitenplatform` `Layout` re-reads the monitor scale and size every frame, updates the window bounds and requests a redraw, and `Window.Draw` re-reads `ScaleFactor` | `HeadlessWindow` has a `Resize` (and scale setter) so a test can resize, relayout and re-rasterize; test covers relayout at a new size and scale; optionally a resize callback for apps |
| L6 | P3 | M | - | `StyledText` runs wrap jointly (currently wrap independently) | A paragraph of multiple runs breaks lines across run boundaries; test |
| L7 | P3 | S | - | `TextInput` measures text at the window scale (currently scale 1) | Caret x matches rendered glyphs at scale 2; test |

### 3.2 Styling and painting

| ID | Pri | Size | Deps | Milestone | Acceptance criteria |
|----|-----|------|------|-----------|---------------------|
| S2 | P2 | M | - | Size/weight/family/line-height in `Hover`/`Focus`/`FocusVisible` refinements | Refinements that change text metrics trigger relayout (or are documented as unsupported with a panic-free no-op); tests |
| S3 | P2 | S | - | Hover text override inside custom `RequestLayoutWithChildren` containers | Custom containers push and pop paint-time overrides so they can re-assert their own colour; test |
| S4 | P2 | S | - | `Text`/`StyledText` Bg/Border/Shadow: paint the box, or drop the builders | Decision recorded; either the box paints (test with pixel check) or the builders are removed from `Text` and the contract |
| S5 | P2 | M | - | Hitboxes clipped to rounded corners | A click in the transparent corner of a rounded box misses it; test |
| S6 | P3 | M | - | Italic faces and intermediate weights (THIN..MEDIUM, SEMIBOLD..BLACK) | Font loader picks the nearest face; `text` example shows all weights |
| S7 | P3 | S | - | Dashed borders, gradient borders/shadows, rounded smoothing | Dashed style paints; `hello_world`/`border_and_shadow_backgrounds` updated |
| S8 | P3 | S | - | Underline colour and strikethrough colour, wavy decoration | `text` example drops its 1px-bar emulation |
| S9 | P3 | S | - | Sprite raster loop iterates sprite bounds rather than the clip window | Benchmark in `bench_test.go` shows no regression; perf only |
| S10 | P2 | M | - | Colour emoji (bitmap/COLR fonts currently draw as tofu) | Emoji row in `opacity` renders glyphs; raster supports RGBA glyph sprites |

### 3.3 Interaction and input

| ID | Pri | Size | Deps | Milestone | Acceptance criteria |
|----|-----|------|------|-----------|---------------------|
| I3 | P2 | L | I2 | Multi-line text area (`TextArea`) | Enter inserts newline, up/down caret movement, wrapping, scroll to caret; example |
| I4 | P2 | M | - | IME/composition (marked text UI, candidate window position via `PlatformInputHandler`) | Composition text rendered underlined; platform reports caret bounds; documented per platform |
| I5 | P2 | M | - | Drag and drop API (`OnDrag`, `OnDrop`, drag preview, drag-over style) | `interactive_elements` uses the real API instead of mouse-down/up emulation |
| I6 | P2 | S | - | Cursor styles (pointer, text, resize ...) | `Div.Cursor(...)` forwarded to `Platform`; ebiten sets `ebiten.SetCursorShape`; stub records it |
| I7 | P2 | S | - | `Keystroke.String` prints `ModPlatform` per OS (`cmd-` on macOS, `super-`/`win-` elsewhere) | Table-driven test with GOOS override |
| I8 | P3 | S | - | Key chords (multi-keystroke bindings) | `ctrl-k ctrl-c` style bindings dispatch; pending-state API |
| I9 | P2 | S | - | Windows clipboard clear (`ClipboardWrite("")` cannot clear today) | Clearing works on Windows or is documented; test on the fake tool path |

### 3.4 Elements and runtime

| ID | Pri | Size | Deps | Milestone | Acceptance criteria |
|----|-----|------|------|-----------|---------------------|
| E1 | P1 | L | - | Animation frames and timers: request-next-frame API, `Timer`/`After`, `Animation` with easing | An element animates without the caller calling `Refresh` in `Render`; headless test advances a fake clock; platform contract gains the hook |
| E2 | P2 | M | E1 | Style transitions (`transitions`, `style_transitions`, `motion_showcase`) | Hover colour fades over a duration; test samples intermediate frames |
| E3 | P1 | L | - | Images: PNG/JPEG via `image/*`, `Img` element, object-fit, asset source | `Img` loads and scales; raster supports image sprites; example; golden test |
| E4 | P2 | L | E3 | SVG element (rasterized to image sprites; stdlib-only subset or an approved dependency) | `svg` legacy example renders; dependency decision recorded |
| E5 | P1 | L | - | Async tasks / executor (`cx.Spawn`, background executor, results back on the main loop) | `async_tasks` port runs; entity safety rules preserved; race-detector test |
| E6 | P2 | L | L1 | Overlays: popovers, deferred and anchored elements (plain-text `Div.Tooltip` done, 2026-10-14: window-level overlay painted after the tree, no hitboxes; builder-style tooltips and a general deferred layer remain) | Overlay paints above siblings regardless of tree order; anchored to a bounds with edge snapping; hover tooltip example |
| E7 | P2 | M | - | `List` with variable row heights (virtualized) next to `UniformList` | Measures visible rows only; scroll-to-index; test with mixed heights |
| E8 | P3 | M | E1 | Blur, corner smoothing, custom drawing (canvas / paint_path) | Per-example acceptance as in 3.6 |
| E9 | P2 | L | - | Multi-window in the ebiten backend (`OpenWindow` more than once) | Two windows open, receive their own input, close independently; ebiten limits documented; headless already supports it |
| E10 | P2 | S | - | Headless keyboard-focus seeding `RenderOption` (`WithFocus(id)`, keyboard modality) | `focus_visible` and `tab_stop` drop their `-focus` flag emulation |

### 3.5 Tooling, CI and verification

| ID | Pri | Size | Deps | Milestone | Acceptance criteria |
|----|-----|------|------|-----------|---------------------|
| T1 | P1 | S | - | CI (GitHub Actions): `gofmt -l`, `go vet`, `go test ./...` on linux/macOS/windows | Workflow green on a PR; caches modules |
| T2 | P1 | M | T1 | Golden-image test harness: render each example headless and compare to `testdata/golden/*.png` with a tolerance; `-update` flag | Each example has a golden; a one-pixel layout change fails; fonts pinned or tolerance justified for CI |
| T3 | P1 | S | - | Real-window verification checklist (run on a machine with a display) | Checklist in this file (below) executed and results recorded per OS |
| T4 | P3 | S | T1 | Benchmark tracking (`bench_test.go`) in CI as non-blocking | Numbers uploaded as artifact |

Real-window checklist (T3), to run manually per OS:

1. `go run ./examples/todo`: window opens, typing works, Enter adds, click toggles.
2. Retina/HiDPI: text sharp, no blur; move window between displays.
3. Resize window: relayout works; automated coverage is L5.
4. Scroll wheel and trackpad in `scrollable` and `uniform_list`; direction correct.
5. Hover, focus, tab order in `tab_stop` and `focus_visible`.
6. Clipboard: cmd/ctrl-c/v/x in `TextInput`.
7. `cmd/ctrl-shift-s` writes a snapshot PNG.
8. Close button quits cleanly (`ebiten.Termination`).

### 3.6 gpui-ce examples

Source: <https://github.com/gpui-ce/gpui-ce/tree/main/crates/gpui/examples>
(checked 2026-10-12). Notes on partial ports are the "Not ported" comments at the
top of each `examples/*/main.go`.

| gpui-ce example | Status | Local example | Blocked by / not ported |
|-----------------|--------|---------------|--------------------------|
| `border_and_shadow_backgrounds` | ported | `border_and_shadow_backgrounds` | gradient borders/shadows (S7), rounded smoothing (E8) |
| `learn/actions_and_keybinds` | ported | `actions_and_keybinds` | - |
| `learn/interactive_elements` | ported | `interactive_elements` | drag API (I5), dashed border (S7), cursors (I6), grid (L3) |
| `learn/layout` | ported | `layout` | grid (L3) |
| `learn/styling` | ported | `styling` | cursors (I6), app menu |
| `learn/text` | ported | `text` | italic/weights (S6), underline/strike colour (S8), ellipsis/line-clamp, grid (L3), tofu glyphs |
| `learn/uniform_list` | ported | `uniform_list` | `Colors::for_appearance` |
| `legacy/focus_visible` | ported | `focus_visible` | cursor_pointer (I6), E10 |
| `legacy/gradient` | ported | `gradient` | canvas/paint_path (E8) |
| `legacy/hello_world` | ported | `hello_world` | shadow_lg root, dashed borders (S7) |
| `legacy/opacity` | ported | `opacity` | img/svg (E3, E4), emoji (S10), wavy decoration (S8) |
| `legacy/scrollable` | ported | `scrollable` | example font loader |
| `legacy/tab_stop` | ported | `tab_stop` | E10 |
| `legacy/tree` | ported | `tree` | - |
| `learn/animation` | todo | - | E1 |
| `learn/async_tasks` | todo | - | E5 |
| `learn/blur` | todo | - | E8 |
| `learn/corner_smoothing` | todo | - | E8 |
| `learn/creating_components` | todo (likely portable now) | - | - |
| `learn/custom_drawing` | todo | - | E8 |
| `learn/haptic_feedback`, `learn/keyring` | skip | - | OS specific, out of scope |
| `learn/motion_showcase`, `learn/style_transitions`, `learn/transitions` | todo | - | E1, E2 |
| `legacy/gif_viewer`, `legacy/image`, `legacy/image_loading` | todo | - | E3 |
| `legacy/svg` | todo | - | E4 |
| `legacy/inline_layout` | todo | - | S4, L6 |
| `legacy/window`, `legacy/window_positioning`, `legacy/window_shadow` | todo | - | E9, L5 |
| `legacy/layer_shell`, `legacy/on_window_close_quit` | skip / todo | - | layer_shell is Wayland only; close-quit needs E9 |
| `direction`, `text_system`, `text_transform_preview` | todo | - | `text_system` needs S6; others likely portable |
| `system_notifications`, `window_movable`, `window_visibility` | skip / later | - | Platform features outside the POC scope |
| `view_example/*` (editor, input, text_area) | todo | - | I2, I3 |
| `bench/*` (data_table, paths_bench, pattern, shadow) | todo | - | E7; perf references for `bench_test.go` |

Local-only examples (not from gpui-ce): `counter`, `gallery`, `todo`.

### 3.7 Component toolkit integration

Requested upstream reference: [longbridge/gpui-kit](https://github.com/longbridge/gpui-kit).
Research and proposal are scoped separately from implementation. K1 is complete:
[`gpui-kit-integration.md`](gpui-kit-integration.md), upstream revision
`c5f8eca4a0759f8c0c7ed624eddfd03f1c93a5f2` (2026-10-13), independently checked
against source manifests, architecture, linked paths and notices. No integration
code, dependency additions or Rust build occurred. K2 is a backlog proposal;
package/API and activation choices remain for contract review before dispatch.
Later overlay/data phases remain proposals inside the document.

| ID | Pri | Size | Deps | Milestone | Acceptance criteria |
|----|-----|------|------|-----------|---------------------|
| K2 | P2 | M | L2, S1 | First native Go controls inspired by gpui-kit (backlog proposal) | Settle package/test and activation contracts first; semantic theme, text button and controlled checkbox with one headless example; disabled/focus/keyboard dispatch tests and light/dark visual checks; exclude assets/overlays/motion and document accessibility gap |

## 4. Known limitations and POC deviations from GPUI

- `TextInput` is a plain struct owned by the root view, not an `Entity[T]`
  (contract section 10); `ID`-based focus works but it cannot be observed as an entity.
- Scroll offsets are positive (distance scrolled), unlike GPUI's negative
  offsets. `OverflowHidden` is not programmatically scrollable.
- CPU rasterizer only: no GPU path, no shader effects; the ebiten backend uploads
  premultiplied pixels with `WritePixels` and skips frames when not dirty.
- Go has no overloading: fluent builders (`ID(string) *Div`) coexist with the
  `Element` interface through adapter types (`divElement`).
- `New()` in `ebitenplatform` returns `gpui.Platform` (an interface).
- Hover/focus text refinements cascade at paint time for colour and
  strikethrough only (`paintTextFields`); see S2, S3.
- Hitboxes are rectangles (S5); borders are not part of layout sizes (L4).
- `StyledText` runs wrap independently (L6); `TextInput` measures at scale 1 (L7).
- Only regular and bold faces; no italics (S6). Missing glyphs render as tofu.
- Single window in the ebiten backend (E9); no animation clock (E1).
- `Keystroke.String` prints `cmd-` for `ModPlatform` everywhere (I7).
- Interactive ebiten window never run in the agent environment (T3).

## 5. Process

Picking work:

1. Choose an open milestone in section 3 whose dependencies are done; prefer
   P1, then smaller size.
2. Note in the commit or PR description which ID you are working on. Keep one
   milestone per commit where practical.
3. If the milestone is blocked or you decide to drop it, record the decision
   in this file instead of leaving it implicit.

Contract first: if an exported signature changes or is added, edit
[api-contract.md](api-contract.md) before the code, then the code, then the
user docs under `docs/` (snippets must compile; see `_tmp/docchk2/` in
[project-structure.md](project-structure.md)).

Verification: the commands in section 1, plus a `-png` render of any example
you touched. Add tests in `*_test.go` (package `gpui_test` unless internal
access is needed).

Commits: small, imperative subject (see `git log --oneline`), no commit
until the user asks. When a milestone lands, move its row to section 2 with the
commit hash and update the status summary and the examples table.
