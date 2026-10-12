# Architecture

## Package layout

```
go-gpui/
  gpui/                  core (package gpui)
    geometry.go color.go   Pixels, Point, Size, Bounds, Edges, Corners, Rgba/Hsla
    style.go styled.go     Style, StyleRefinement, StyledMixin (fluent builders)
    layout.go              flexbox engine over LayoutNode
    element.go div.go      Element interface, Div
    text_elem.go           Text, StyledText, Str
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
    ebitenplatform/        gpui.Platform on ebiten (no cgo)
  examples/                runnable apps (counter, gallery, todo)
  _docs/                   internal contract, plan, WIP registry
  docs/                    these pages
```

## Dependency boundaries

- `gpui` imports the standard library, plus `golang.org/x/image` and
  `github.com/go-text/typesetting` - and only from `font.go` / `text.go`.
- `github.com/hajimehoshi/ebiten/v2` is imported only inside
  `gpui/ebitenplatform/`. A program that never imports that package never links
  ebiten.
- The core talks to a backend only through the `gpui.Platform` and
  `gpui.PlatformWindow` interfaces (`gpui/platform.go`).

## The frame pipeline

A frame is produced by `Window.Draw` (`gpui/window.go`). Every frame rebuilds
the element tree from scratch; only the application's own state, the
`Window`'s retained per-element state (scroll offsets), the focus, and the
shaper caches survive between frames.

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
   | paintTree                    -> Quad / GlyphSprite / Underline       |
   |                                 in Scene (clip stack applied)        |
   +----------------------------------------------------------------------+
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
   `Text` and `StyledText` are plain per-frame values (`gpui/div.go`,
   `gpui/text_elem.go`).
2. **RequestLayout.** Each element registers a `LayoutNode` with the window
   and returns its `LayoutID` plus opaque state. `Div` recurses into its
   children here, so the layout tree mirrors the element tree. Text leaves
   install a `Measure` callback that calls the window's `TextMeasurer`.
3. **Layout.** `Layout(root, Definite(windowW, windowH))`
   (`gpui/layout.go`). Output: every node's `Rect` relative to its parent.
4. **Render tree.** `buildRenderNode` pairs elements with layout nodes,
   converts to absolute bounds, and shifts children of scroll containers by
   the retained scroll offset.
5. **Prepaint.** A depth-first walk calls `Element.Prepaint`. `Div` registers
   one hitbox (always), its mouse listeners, scroll extent, and - when it has
   key handlers, a key context or a focus handle - a key node. Ancestor links
   (`parent`) are recorded so dispatch can walk the hit/focus path later.
   `Display: false` nodes are skipped.
6. **Paint.** Another depth-first walk calls `Element.Paint`; a clipped div
   (`OverflowHidden`/`OverflowScroll`) pushes its bounds on the clip stack
   around its subtree. Primitives go into the frame's `Scene`, each stamped
   with the current clip as a `ContentMask`.
7. **Swap.** The finished frame becomes `renderedFrame`; all event dispatch
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

`Scene` holds three primitive lists: `Quad` (fill and/or border, per-corner
radii, solid fills only), `GlyphSprite` (an 8-bit coverage mask plus colour),
and `Underline` (used for strikethrough). `Rasterizer.Render`
(`gpui/raster.go`) draws **all quads, then all underlines, then all
sprites**, each group stably ordered by `Order`. Consequence: text is always
on top of every background, even a background painted later by an overlapping
sibling. `Window.PushLayer` only advances the order counter inside a group.

Quads are anti-aliased with a signed-distance rounded-rect coverage function.
Sprites are blitted 1:1 when the mask matches the device-pixel bounds
(the normal case, because glyph masks are rasterized at the window scale) and
bilinearly sampled otherwise.

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
  (notifications, deferred funcs, emitted events) and then requests frames for
  invalid windows. Platforms wrap all input dispatch and frame callbacks in
  `app.Update`.

## When frames are drawn

Nothing redraws on a timer. A frame is drawn when:

- `Window.Refresh()` is called (it sets `invalid` and calls
  `RequestFrame`), or `RequestFocus`/`Blur` change focus;
- a scroll container's offset changes in `dispatchScroll`;
- the platform resizes the window or changes scale (ebiten's `Layout`);
- effects flush and a window is `invalid`.

Entity notifications do **not** invalidate windows by themselves; an observer
or event handler must call `Window.Refresh` (see
[app-and-entities.md](concepts/app-and-entities.md)).

In the ebiten backend `RequestFrame` sets a flag; the next `Update` calls the
window's registered draw routine, and `Draw` uploads the buffer only if a new
frame was presented (`gpui/ebitenplatform/platform.go`).

## The headless platform

`HeadlessPlatform` (`gpui/headless.go`) implements `Platform` with in-memory
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
  bounds (noted in `_docs/wip.md`).
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
  Hover and pointer motion only redraw if a handler calls `Refresh`.
- **Hit testing** is a linear scan over the frame's hitboxes (highest paint
  order wins), which is fine at POC scale.
