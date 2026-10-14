# Reference: App, entities, contexts, subscriptions and Window

Source: `app.go`, `entity.go`, `context.go`,
`subscription.go`, `window.go`, `background.go`.

Back to the [reference index](README.md). Concepts:
[app and entities](../concepts/app-and-entities.md),
[architecture](../architecture.md).

Everything here runs on one goroutine (the one that called `Platform.Run`);
only `App.Quit` and `App.IsQuitting` are goroutine-safe.

## App

```text
func NewApp() *App                        // stub platform: no-op windows, enough for entity tests
func NewAppWithPlatform(p Platform) *App  // nil p selects the stub
```

In a real program the platform creates the `App` inside `Run` and passes it to
your launch callback; there is no `App.Run` method.

| Method | Meaning |
|---|---|
| `OpenWindow(o WindowOptions, build func(*Window, *App) EntityRoot) error` | Create a platform window, build its root, draw once immediately and append it to `Windows`. |
| `Windows() []*Window` | Open windows in creation order. |
| `Platform() Platform` | The driving platform. |
| `Keymap() *Keymap` | The app's keymap; add bindings with `AddBindings`. |
| `NewFocusHandle() *FocusHandle` | Allocate a fresh focus id. |
| `Update(f func(*App))` | Run `f`; only the outermost call flushes effects when it returns. Nesting is allowed. |
| `Notify(id EntityID)` | Schedule the entity's observers and mark windows that read it during their last draw invalid. Deduplicated per flush. |
| `RefreshWindows()` | Queue a refresh of every window. |
| `Defer(f func(*App))` | Run `f` at the end of the current effect cycle. |
| `SetGlobal(v any)` | Store a value keyed by its concrete type (replaces any previous one). |
| `Global(out any) bool` | Copy the stored value into `out` (a non-nil pointer to the value's type); false when absent. |
| `UpdateGlobal(f func(any))` | Call `f` once per stored global. To change one, store a pointer and mutate it, or call `SetGlobal` from inside `f`. |
| `Quit()` | Mark the app quitting and ask the platform to terminate. Goroutine-safe. |
| `IsQuitting() bool` | Whether `Quit` has been called. Goroutine-safe. |

### WindowOptions

```text
type WindowOptions struct { Width, Height float32; Title string; Resizable bool }
func DefaultWindowOptions() WindowOptions     // 800x600, resizable, "gpui"
```

## Entities

`Entity[T]` is a cheap copyable handle to a `T` stored in the `App`. Go has no
`Drop`, so strong references are counted only where you say so: `NewEntity`
returns the first, `Clone` adds one, `Release` drops one. Plain copies, the
handle from `Context.Entity()` and the result of `WeakEntity.Upgrade()` are
borrowed and must not be released.

```text
func NewEntity[T any](cx *App, build func(cx *Context[T]) T) Entity[T]
func EntityView[T any, P interface{ *T; EntityRoot }](e Entity[T]) EntityRoot   // render *T as a window root; holds a Clone, released by Window.Close

type EntityID struct{ /* opaque */ }
func (id EntityID) IsNil() bool
func (id EntityID) String() string            // "entity(index.generation)"
```

| `Entity[T]` method | Meaning |
|---|---|
| `ID() EntityID` | Slot id. |
| `Read(cx *App) *T` | Pointer to the state, or nil once released. Valid only until the next call that can re-enter the entity. A read during a window draw makes `Notify` redraw that window. |
| `Update(cx *App, f func(state *T, cx *Context[T]))` | Mutate the state; a released entity is a no-op. |
| `Notify(cx *App)` | Schedule observers (same as `App.Notify(e.ID())`). |
| `Clone() Entity[T]` | New strong reference (refcount + 1). |
| `Release(cx *App)` | Drop one strong reference; at zero the entity is released at the end of the current flush (immediately when called outside `Update` and a flush). Releasing a dead entity is a no-op. |
| `Downgrade() WeakEntity[T]` | Non-owning handle. |

```text
type WeakEntity[T any] struct{ /* opaque */ }
func (w WeakEntity[T]) Upgrade() (Entity[T], bool)   // live handle while the slot still exists
```

## Context

`Context[T]` is the per-entity handle passed to `NewEntity`'s build function
and to `Entity.Update`. It has an exported `App *App` field.

| Method | Meaning |
|---|---|
| `Notify()` | Schedule this entity's observers (this is how a view asks its window to redraw). |
| `Entity() Entity[T]` | Borrowed handle to this entity. |
| `WeakEntity() WeakEntity[T]` | Non-owning handle; capture this in long-lived closures. |
| `Observe(other Entity[T], f func(Entity[T], *App)) *Subscription` | Observe an entity of the same `T`; live immediately; closed when this entity is released. |
| `OnRelease(f func(*App)) *Subscription` | Run `f` when this entity is released. |
| `Defer(f func(*App))` | Run `f` at the end of the current effect cycle. |
| `Spawn(f func()) *Task` | `Defer` plus cancellation. No goroutine is started. |

### Package-level generic helpers

Go methods cannot have type parameters, so these are functions:

```text
func Listener[T, E any](c *Context[T], f func(state *T, ev *E, w *Window, cx *App)) func(*E, *Window, *App)
func Emit[T, E any](c *Context[T], event *E)
func ObserveOn[T any](a *App, e Entity[T], f func(Entity[T], *App)) *Subscription
func Subscribe[T, E any](cx *App, e Entity[T], f func(*E)) *Subscription
func SubscribeEvent[T any, E any](cx *App, e Entity[T], f func(ev *E)) *Subscription   // same behaviour as Subscribe
```

#### Listeners and events

`Listener` upgrades a weak handle each time the callback runs and no-ops when
the entity is gone; its result has the shape of every `Div.On*` mouse and key
handler. `Emit` queues an event for subscribers of that event type on the
entity, delivered at the next flush. Observers and subscribers are keyed by
`(entity, event type)`, run in insertion order, and are active immediately.

## Subscription, SubscriberSet, Task

```text
type Subscription struct{ /* opaque */ }
func NewSubscription(close func()) *Subscription   // runs close the first time it is closed
func (s *Subscription) Close()                     // idempotent, nil-safe: defer sub.Close() is always valid
func (s *Subscription) IsClosed() bool             // a nil *Subscription counts as closed

type SubscriberSet[K comparable, T any] struct{ /* opaque */ }
func NewSubscriberSet[K comparable, T any]() *SubscriberSet[K, T]
func (s *SubscriberSet[K, T]) Insert(key K, cb T) *Subscription
func (s *SubscriberSet[K, T]) Emit(key K, emit func(cb T))      // snapshots callbacks; safe against re-entrant insert/close
func (s *SubscriberSet[K, T]) Remove(key K)
func (s *SubscriberSet[K, T]) RemoveWhere(match func(K) bool)
func (s *SubscriberSet[K, T]) Len() int

type Task struct{ /* opaque */ }
func (t *Task) Cancel()    // prevents the work from running if it has not run; nil-safe
func (t *Task) Detach()    // abandon the handle; the work still runs
```

## Window

A `Window` is created by `App.OpenWindow` and driven by the platform. You
mostly meet it as the `*Window` argument of `Render` and event handlers.

### Frame and state

| Method | Meaning |
|---|---|
| `Refresh()` | Mark the window dirty and request a frame. Nothing redraws on its own: call this after changing plain-struct state. |
| `Draw(cx *App)` | Run the full pipeline (render, layout, prepaint, paint, swap, raster, present). Called by the platform. |
| `DispatchEvent(input PlatformInput, cx *App)` | Route one input event through the rendered frame, inside `App.Update`. |
| `Close()` | Remove the window from the app, release an `EntityView` root's reference and quit the platform window. Idempotent. |
| `Bounds() Bounds` | Window size in logical pixels: the platform size divided by `UIScale` (falls back to the options, then 800x600). |
| `ScaleFactor() float32` | Effective raster scale: the platform's device pixel ratio times `UIScale`. |
| `SetUIScale(f float32)` | Zoom the whole UI by `f` (clamped to 0.25..4, default 1): layout sees the platform size divided by `f`, the scene rasterizes at the device scale times `f` (text stays sharp), and `DispatchEvent` divides pointer positions by `f`. Wheel deltas are not divided. Invalidates the window. |
| `UIScale() float32` | The current UI zoom. |
| `SetBackground(c Rgba)` | Colour the frame is cleared to (default opaque white). |
| `Scene() *Scene` | The last drawn frame's scene, for inspection. |
| `HitTest(p Point) (GlobalElementID, bool)` | Id of the element owning the topmost hitbox at `p` in the last frame; compare with `Equal`. |
| `StopPropagation()` | Halt the current dispatch: remaining listeners and keymap dispatch are skipped. |

### Focus

| Method | Meaning |
|---|---|
| `Focus() *FocusHandle` | Handle of the focused element, or nil. |
| `RequestFocus(h *FocusHandle)` | Move keyboard focus to `h`. |
| `Blur()` | Clear focus. |
| `FocusNext()` / `FocusPrev()` | Move through tab stops, wrapping. |
| `FocusVisible() bool` | Whether the last input was the keyboard. |

### Retained element state and scroll offsets

| Method | Meaning |
|---|---|
| `ElementState(id GlobalElementID, out any) bool` | Copy retained state into `out` (non-nil pointer); false if absent or not assignable. |
| `SetElementState(id GlobalElementID, v any)` | Store retained state for `id`. |
| `ScrollOffset(id GlobalElementID) Point` | Stored scroll offset, or the zero point. |
| `SetScrollOffset(id GlobalElementID, p Point)` | Store an offset, clamped to the extent from the last prepaint; a tracking `ScrollHandle` is updated too. |

State that is not accessed during a frame is dropped when the frame
finishes ([architecture](../architecture.md#element-state)).

### Text backend

| Method | Meaning |
|---|---|
| `TextMeasurer() TextMeasurer` | The window's text backend; nil when no system font was found. |
| `SetTextMeasurer(t TextMeasurer)` | Install a text backend (nil makes text fall back to a crude estimator). |

### Custom element and painting hooks

| Method | Meaning |
|---|---|
| `RegisterLayoutNode(n *LayoutNode) LayoutID` | Register a caller-built node (children already linked into `n.Children`); return the id from `RequestLayout`. |
| `LayoutNode(id LayoutID) *LayoutNode` | Look up a node registered this frame, or nil. |
| `RequestLayoutWithChildren(style Style, children []AnyElement, cx *App) (LayoutID, *ChildLayout)` | Lay out each non-nil child, build a node with `style` over them and register it. Pair with `ParentElementChildren`. |
| `InsertHitbox(b Bounds) int` | Register a hit-testable region (clipped to enclosing clips); returns its index. |
| `HandleInput(h PlatformInputHandler)` | Register a text-input handler for this frame (called by focused text elements in prepaint). |
| `PaintQuad(q Quad)` / `PaintShadow(s Shadow)` / `PaintGlyph(g GlyphSprite)` / `PaintUnderline(u Underline)` / `PaintStroke(s Stroke)` | Insert a primitive; an unset `ContentMask` is filled with the current one and the element opacity is applied. |
| `PushClip(b Bounds)` / `PushContentMask(m ContentMask)` / `PopClip()` | Push/pop a clip (intersected with the current one). |
| `CurrentClip() Bounds` / `CurrentContentMask() ContentMask` | Innermost clip; "infinite" when none. |
| `PushLayer()` | Z-order boundary: everything painted afterwards is above everything before. |

Primitive types are in [platform.md](platform.md#scene-and-rasterizer); the
worked custom-element example is in the
[cookbook](../guides/cookbook.md#custom-element-with-children).
