# Architecture

## Package layout

```
internal/gpui/           core (package gpui), import path go-draw/internal/gpui
    geometry.go color.go   Pixels, Point, Size, Bounds, Edges, Corners, Rgba/Hsla
    style.go styled.go     Style, StyleRefinement, StyledMixin (fluent builders)
    layout.go layout_wrap.go flexbox lines and absolute positioning over LayoutNode
    element.go div.go      Element interface, Div
    text_elem.go           Text, StyledText, Str
    background.go shadow.go Fill/gradients, BoxShadow, paintBox
    scroll.go uniform_list.go ScrollHandle, scrollbars, UniformList
    styled_layout.go       Div/Text wrappers for margins, positioning and flex shorthands
    scene.go raster.go     Scene primitives, CPU rasterizer, scaleScene
    bounds.go              hitbox list (BoundsTree)
    events.go keymap.go    input events, keystrokes, keybindings, key contexts
    focus.go input.go      FocusHandle, TextInput
    entity.go context.go   Entity[T], Context[T], observe/emit
    app.go subscription.go App, effect queue, SubscriberSet
    window.go              Window: the draw pipeline and event dispatch
    platform.go            Platform / PlatformWindow / PlatformInputHandler
    headless.go            HeadlessPlatform, RenderToPNG / RenderToImage
    font.go text.go        FontManager, Shaper (HarfBuzz via go-text)
    clipboard.go           OS-tool clipboard
    ebitenplatform/        gpui.Platform on ebiten (no cgo); snapshot.go = frame-snapshot key
  examples/                17 runnable apps (see examples.md)
  _docs/                   internal contract, plan, WIP registry
  docs/                    these pages
```

## Dependency boundaries

- `gpui` imports the standard library, plus `golang.org/x/image` and
  `github.com/go-text/typesetting` - and only from `font.go` / `text.go`.
- `github.com/hajimehoshi/ebiten/v2` is imported only inside
  `ebitenplatform/`. A program that never imports that package never links
  ebiten.
- The core talks to a backend only through the `gpui.Platform` and
  `gpui.PlatformWindow` interfaces (`platform.go`).

## The frame pipeline

A frame is produced by `Window.Draw` (`window.go`). Every frame rebuilds
the element tree from scratch; only the application's own state, the
`Window`'s retained per-element state (scroll offsets, implicit focus
handles, custom `SetElementState` values; see [Element state](#element-state)),
the focus, and the shaper caches survive between frames.

```
 state change / input
        |
        v
  Window.Refresh()  ---->  PlatformWindow.RequestFrame()  (sets a "pending" flag)
                                   |
                                   v   (platform loop, same goroutine)
                           Window.Draw(app)
   +----------------------------------------------------------------------+
   | root.Render(w, app)          -> IntoElement -> Element tree          |
   | el.RequestLayout(w, app)     -> LayoutNode tree (registered by id)   |
   | Layout(root, Definite(win))  -> parent-relative Rect per node        |
   | buildRenderNode              -> absolute bounds, scroll offsets      |
   | prepaintTree                 -> hitboxes, mouse/key listeners,       |
   |                                 focus nodes, input handlers          |
   | paintTree                    -> Quad / Shadow / GlyphSprite /        |
   |                                 Underline in Scene (content masks,   |
   |                                 paint-time text overrides applied)   |
   +----------------------------------------------------------------------+
                                   |  prune unaccessed element state
                                   |  swap nextFrame <-> renderedFrame
                                   v
                 scaleScene(scene, scale)      logical -> device px
                                   |
                                   v
                 Rasterizer.Reset(bg); Render()   premultiplied RGBA
                                   |
                                   v
                 PlatformWindow.Present(pix, w, h)
                                   |
        ebiten: stored; next game.Draw -> screen.WritePixels(pix)
        headless: copied; read via Pixels() / Image() / SavePNG()
                                   |
                 SetInputHandler(focused handler); re-run hover
```

Step by step:

1. **Render.** `EntityRoot.Render(w, cx)` returns an `IntoElement`. `Div`,
   `Text` and `StyledText` are plain per-frame values (`div.go`,
   `text_elem.go`).
2. **RequestLayout.** Each element registers a `LayoutNode` with the window
   and returns its `LayoutID` plus opaque state. `Div` recurses into its
   children here, so the layout tree mirrors the element tree. Text leaves
   install a `Measure` callback that calls the window's `TextMeasurer`. The
   window keeps a text-style stack for the walk: a `Div`, a `UniformList` and
   `RequestLayoutWithChildren` push their explicitly set text fields, and text
   leaves resolve their own fields over the innermost entry.
3. **Layout.** `Layout(root, Definite(windowW, windowH))`
   (`layout.go`). Output: every node's `Rect` relative to its parent.
4. **Render tree.** `buildRenderNode` pairs elements with layout nodes,
   converts to absolute bounds, and shifts children of scroll containers by
   the retained scroll offset.
5. **Prepaint.** A depth-first walk calls `Element.Prepaint`. `Div` registers
   one hitbox (always), its mouse listeners, scroll extent, and - when it has
   key handlers, a key context or a focus handle - a key node. Ancestor links
   (`parent`) are recorded so dispatch can walk the hit/focus path later.
   `Display: false` nodes are skipped. Scrollbar tracks register after a
   container's descendants so their input order matches their overlay paint.
6. **Paint.** Another depth-first walk calls `Element.Paint`; a clipped div
   (`OverflowHidden`/`OverflowScroll`) pushes a `ContentMask` (its bounds plus
   corner radii) around its subtree. Primitives go into the frame's `Scene`,
   each stamped with the current mask. See [Paint-time state](#paint-time-state).
7. **Prune and swap.** Element state not accessed this frame is dropped
   (below). The finished frame becomes `renderedFrame`; all event dispatch
   reads it. `invalid` is cleared.
8. **Raster + present.** `present()` scales the scene to device pixels
   (`scaleScene`), clears the buffer to the window background (default white),
   rasterizes, and hands the buffer to `PlatformWindow.Present`.
9. **Post-draw.** The focused text input (if any) is installed with
   `SetInputHandler`, and hover state is re-evaluated at the last pointer
   position because the tree under a stationary mouse may have changed.

`App.OpenWindow` draws once immediately; after that, frames are driven by
`Window.Refresh` / `RequestFrame` (see below).

## Scene and rasterization

`Scene` holds four primitive lists: `Quad` (fill and/or border, per-corner
radii, solid or gradient fill), `Shadow` (outset/inset box shadows),
`GlyphSprite` (an 8-bit coverage mask plus colour) and `Underline` (used for
strikethrough). Every primitive carries an `Order` and a `ContentMask`.
Primitives inserted with `Order == 0` get the scene's next auto order, so
order is **paint (tree) order**: `Rasterizer.Render` (`raster.go`) sorts
each list stably by `Order` and merges the four, so a later background covers
earlier text, as in GPUI. On equal `Order` the kinds draw shadow, quad,
underline, sprite. `Window.PushLayer` reserves a gap (`layerStride`, 1<<20
orders) so everything inserted afterwards paints above everything before it.

**Rounded content masks.** `ContentMask` is a clip rect plus optional
`CornerRadii`. Pushing one (`Window.PushContentMask`, or `PushClip` for a plain
rect) intersects it with the current mask; each corner of the intersection keeps
the radius of the mask that contributes it, reduced by how far the corner lies
inside that mask's own corner (exact for equal offsets, conservative
otherwise). The rasterizer applies a mask
with radii as an anti-aliased rounded-rect coverage (`setMask` /
`maskCoverage`: only the corner squares evaluate the SDF), so an
`OverflowHidden` + `Rounded` div clips its children to the rounded shape;
rect-only masks are applied through the pixel windows. Hitboxes are still
clipped to rectangles only.

Quads and shadows are anti-aliased with a signed-distance rounded-rect
coverage function. Sprites are blitted 1:1 when the mask matches the
device-pixel bounds (the normal case, because glyph masks are rasterized at
the window scale) and bilinearly sampled otherwise.

## Element state

`Window` retains per-element values across frames in `elementStates`, keyed by
the string form of the element's `GlobalElementID`: scroll offsets, the
implicit focus handle of a `TabIndex` div without `TrackFocus` (key suffix
`#focus`), and anything an application stores with `SetElementState`.

Lifecycle, as in GPUI: state that is not **accessed** during a frame is dropped
when that frame finishes. "Accessed" means `ElementState`, `SetElementState`,
`ScrollOffset`/`SetScrollOffset`, or the internal scroll and focus-handle
lookups, each of which records the key in `stateSeen`. `Window.Draw` calls
`pruneElementStates` after the element walk: every key not in `stateSeen` is
deleted (an implicit focus handle also releases its focus id) and `stateSeen`
is cleared. Consequences: a scroll container or `TabIndex` div that is not
rendered for a frame (a hidden branch, a conditional) loses its offset or
handle; custom elements must touch their state every frame they are drawn.

## Effects and Update nesting

`App.Update(f)` bumps `pendingUpdates`, runs `f`, and only when the outermost
call returns does it call `flushEffects` (`app.go`). The flush itself also
counts as an update, so callbacks it runs (observers, defers, event
subscribers, release callbacks) may call `Update` or `Entity.Release` freely:
those calls do not start a nested flush, they queue work that the same loop
picks up. The loop runs until no effects and no pending releases remain, then
requests a frame for every window still `invalid`.

Consequences for code you write:

- `Entity.Release` on the last reference only *schedules* the release when
  called inside `Update` or a flush; outside both it runs its own `Update`, so
  the entity is gone when `Release` returns.
- `Window.DispatchEvent` wraps dispatch in `App.Update` itself, so effects
  queued by handlers (notifies, emits, releases) flush once, after the event
  is fully dispatched. Platforms may still wrap a batch of events in one
  `Update`.
- `NewEntity` creates the entity slot before calling `build`, so `Observe`
  and `OnRelease` registered from `build` attach to the new entity; reading
  the entity from inside its own `build` finds no value yet.

## Paint-time state

Layout is finished before paint, so anything resolved at paint cannot change
sizes. Two things are resolved per frame at paint:

- **Interaction styles.** `Div.paintOpacity` (`div.go`) merges the
  `Focus`, `FocusVisible`, `Hover` and `Active` refinements over the base style and
  returns the div's opacity, which `paintTree` multiplies into `Window.opacity`
  (nested opacities multiply; `Text` and `StyledText` carry theirs through
  `textPrepaint`).
- **Text override stack.** Text style cascades at layout, but hover and focus
  styles change at paint, so `Window.paintTexts` holds a stack of
  `textOverride{set, ts}` entries. `paintTree` pushes a node's entry (from
  prepaint states implementing `paintTextStyler`: `Div`, `UniformList`) when an
  interaction style contributed to it, or when an entry is already on the
  stack (so a nearer ancestor that sets `TextColor` itself re-asserts it over an
  outer hover colour). `Text`/`StyledText` call `Window.paintTextStyle`,
  which walks the stack innermost first and overlays, per field, the first
  entry that sets it, skipping fields the text element sets itself. Only
  `paintTextFields` (colour and strikethrough) are overridden; size, weight,
  family and line height affect layout and are ignored in interaction styles
  for descendants. A custom container built on `RequestLayoutWithChildren`
  pushes no entry, so an outer hover colour keeps applying inside it.

## Threading model

- The entire UI runs on **one goroutine**: the one that calls
  `Platform.Run`. With ebiten that is ebiten's `Update`/`Draw` goroutine.
  `App`, `Window`, entities, the shaper and the rasterizer are not
  synchronized.
- `App.Quit()` and `App.IsQuitting()` are goroutine-safe (atomic flag), and
  `Platform.Quit()` implementations must be too. The examples use
  `time.AfterFunc(d, cx.Quit)` for smoke tests.
- Everything else from other goroutines must be marshalled onto the UI
  goroutine by the application. There is no cross-goroutine dispatch API;
  `Context.Spawn` does not start a goroutine (it defers to the end of the
  current effect cycle).
- Clipboard access shells out to OS tools and is done on a worker goroutine in
  the ebiten backend; results are drained on the UI goroutine in `Update`.
- `App.Update(f)` can nest. Only the outermost call flushes queued effects
  (notifications, deferred funcs, emitted events, entity releases) and then
  requests frames for invalid windows; see
  [Effects and Update nesting](#effects-and-update-nesting). `DispatchEvent`
  runs inside `Update`, and platforms wrap frame callbacks in it.

## When frames are drawn

Nothing redraws on a timer. A frame is drawn when:

- `Window.Refresh()` is called (it sets `invalid` and calls
  `RequestFrame`), or `RequestFocus`/`Blur` change focus;
- a scroll container's offset changes through wheel input or scrollbar drag/paging;
- the platform resizes the window or changes scale (ebiten's `Layout`);
- effects flush and a window is `invalid`.

Entity notifications do **not** invalidate windows by themselves; an observer
or event handler must call `Window.Refresh` (see
[app-and-entities.md](concepts/app-and-entities.md)).

In the ebiten backend `RequestFrame` sets a flag; the next `Update` calls the
window's registered draw routine, and `Draw` uploads the buffer only if a new
frame was presented (`ebitenplatform/platform.go`).

## The headless platform

`HeadlessPlatform` (`headless.go`) implements `Platform` with in-memory
windows. It runs the identical pipeline - layout, prepaint, paint, raster -
and keeps the last presented pixels. `RenderToPNG` / `RenderToImage` wrap it
for one-shot renders; `HeadlessWindow.Dispatch` feeds synthetic input for
tests. `NewApp()` with no platform uses a no-op stub whose windows discard
frames. The headless window does not implement `FrameRequester`: a
`RequestFrame` just marks the window pending, and the next `Dispatch`,
`Pixels()` or `Image()` call draws. See
[concepts/platform.md](concepts/platform.md).

## Performance notes

- **Bounds-limited rasterization.** Each primitive's pixel loops are limited
  to its own bounds (plus a one-pixel anti-aliasing margin) intersected with
  its content mask; fully-covered interior spans of rounded rects skip the
  SDF. The buffer itself is reset and every primitive redrawn on every frame -
  there is no damage tracking. The known gap: the 1:1 sprite blit loops over
  the clip window intersected with the sprite rather than only the sprite's
  bounds (noted in `_docs/roadmap.md`).
- **Buffer reuse.** The `Rasterizer` and the device-scale scratch `Scene` are
  reused while the window size is unchanged; at scale 1 `scaleScene` returns
  the scene without copying.
- **Layout.** Each `LayoutNode` memoizes measurements per `(available,
  forced)` pair for the duration of one `Layout` call, so repeated flex
  measurements are free.
- **Shaping and glyph caches.** `Shaper` caches shaped runs keyed by
  `(face, text)` (limit 2048, cleared wholesale when full) and rasterized
  glyph masks keyed by `(face, glyph, size, scale)` (limit 4096). Text that
  does not change between frames costs a map lookup, not a re-shape.
- **Redraw only on demand.** Idle apps do no layout, raster or upload.
  Hover/Active styles redraw on their transitions; other pointer handlers redraw
  when they call `Refresh`.
- **Hit testing** is a linear scan over the frame's hitboxes (highest paint
  order wins), which is fine at POC scale.

See also: [reference/app-window.md](reference/app-window.md) and [reference/platform.md](reference/platform.md) for the exported API behind these stages, [guides/testing.md](guides/testing.md) for exercising the pipeline headlessly, and [guides/troubleshooting.md](guides/troubleshooting.md#slow-frames) for performance symptoms.
