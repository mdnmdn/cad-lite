# App, entities and contexts

Source: `gpui/app.go`, `gpui/entity.go`, `gpui/context.go`,
`gpui/subscription.go`.

## App

`App` owns the platform, all windows, the keymap, focus ids, globals, the
shared text shaper, and the **effect queue**.

```go
app := gpui.NewAppWithPlatform(platform) // nil platform = no-op stub
app.Keymap().AddBindings(...)
err := app.OpenWindow(gpui.WindowOptions{Width: 480, Height: 640, Title: "x"},
    func(w *gpui.Window, cx *gpui.App) gpui.EntityRoot { return root })
```

In a real program you do not construct the `App` yourself; the platform does
inside `Run`, and hands it to your launch callback:

```go
err := ebitenplatform.New().Run(func(cx *gpui.App) {
    cx.Keymap().AddBindings( /* ... */ )
    _ = cx.OpenWindow(gpui.DefaultWindowOptions(), build)
})
```

(There is no `App.Run` method, although the contract's example in
`_docs/00-api-contract.md` §18 uses one.)

Useful `App` methods: `Update`, `Notify`, `RefreshWindows`, `Defer`,
`Quit`/`IsQuitting`, `Keymap`, `NewFocusHandle`, `Windows`,
`SetGlobal`/`Global`/`UpdateGlobal`. `Window.Close()` removes a window (see
below).

### Update and the effect queue

`App.Update(f)` runs `f` and, when the **outermost** `Update` returns, drains
the effect queue:

1. `Notify` effects run the entity's observers (deduplicated per entity per
   flush);
2. `Defer` effects run their `func(*App)`;
3. `Emit` effects deliver events to subscribers;
4. `RefreshWindows` effects mark every window invalid;
5. effects queued while draining are processed in the same flush (an
   `Update` called from a callback during the flush does not start a nested
   one; `Window.DispatchEvent` itself runs inside `Update`);
6. entities whose last strong reference was released are released (their
   release callbacks may queue more effects, which are drained too);
7. every window still `invalid` gets a frame requested.

Nested `Update` calls are allowed (Rust's double-borrow panic has no
equivalent here); they just bump a counter. The platform wraps each input
dispatch and each frame callback in `Update`, so handlers you register already
run inside one.

## Root views

A window's root is an `EntityRoot`: anything with
`Render(w *Window, cx *App) IntoElement`. The simplest root is a plain struct
that holds your state (this is what the examples and `TextInput` do):

```go
type Counter struct{ N int }

func (c *Counter) Render(w *gpui.Window, cx *gpui.App) gpui.IntoElement {
    return gpui.NewDiv().OnClick(func(_ *gpui.ClickEvent, w *gpui.Window, _ *gpui.App) {
        c.N++
        w.Refresh() // nothing redraws unless you ask
    }).Child(gpui.NewText("clicks"))
}
```

`RenderFunc` adapts a plain function to `EntityRoot`.

## Entities

`Entity[T]` is a small copyable handle (slot id + app pointer) to a `T` stored
in the `App`.

```go
ent := gpui.NewEntity(app, func(cx *gpui.Context[Counter]) Counter { return Counter{} })

app.Update(func(cx *gpui.App) {
    ent.Update(cx, func(c *Counter, ecx *gpui.Context[Counter]) {
        c.N++
        ecx.Notify() // schedule observers
    })
})

if c := ent.Read(app); c != nil { _ = c.N } // nil if the entity is gone
```

To use an entity as a window root, wrap it with `EntityView` (its `*T` needs
a `Render(w, cx) IntoElement` method). The view holds a strong reference
that `Window.Close()` releases:

```go
func (c *Counter) Render(w *gpui.Window, cx *gpui.App) gpui.IntoElement {
    return gpui.NewText(fmt.Sprint(c.N))
}

_ = app.OpenWindow(opts, func(*gpui.Window, *gpui.App) gpui.EntityRoot {
    return gpui.EntityView(ent)
})
```

Or close over the handle:

```go
root := gpui.RenderFunc(func(w *gpui.Window, cx *gpui.App) gpui.IntoElement {
    c := ent.Read(cx)
    if c == nil {
        return gpui.NewDiv()
    }
    return gpui.NewText(fmt.Sprint(c.N))
})
```

Facts that differ from Rust GPUI (all intentional POC simplifications):

- **Explicit refcounting.** Go has no `Drop`, so strong references are
  counted only where you say so: `NewEntity` returns the first one,
  `ent.Clone()` adds one and `ent.Release(cx)` drops one. Plain copies of an
  `Entity`, `Context.Entity()` and `WeakEntity.Upgrade()` are *borrowed*
  handles: they do not count and must not be released (releasing a copy
  twice would release the entity early). When the count reaches 0 the entity
  is released at the end of the current effect flush (immediately when
  `Release` is called outside `Update`; an observer, event handler or other
  callback running inside a flush only schedules it). Observers and
  `OnRelease` callbacks registered from `NewEntity`'s build function belong
  to the new entity.
- **Automatic redraw, GPUI-style.** Every `Entity.Read`/`Update` that happens
  while a window draws is recorded on that window. `Notify` on such an entity
  (via `Context.Notify`, `Entity.Notify` or `App.Notify`) marks the window
  invalid, so it redraws at the end of the flush without a `Refresh`.
  Plain-struct roots (not entities) still need `Window.Refresh()`.
  `App.RefreshWindows()` redraws every window.
- **Methods cannot take type parameters**, so generic operations are
  package-level functions: `NewEntity`, `ObserveOn`, `Subscribe`,
  `SubscribeEvent`, `Emit`, `Listener`. `Context.Observe` is a method but only
  observes entities of the same `T`.

### Releasing

```go
ent := gpui.NewEntity(app, build) // refcount 1
keep := ent.Clone()               // refcount 2
ent.Release(app)                  // 1: still alive
keep.Release(app)                 // 0: released at the end of the flush
```

Releasing an entity frees its slot (stale handles `Read` nil, `Update` is a
no-op, `WeakEntity.Upgrade` fails; the slot is reused with a new
generation), runs its `Context.OnRelease` callbacks, drops the observers and
event subscribers registered **on** it, and closes the subscriptions it
created through `Context.Observe` (GPUI drops a view's subscriptions with the
view). `Window.Close()` removes the window from `App.Windows`, releases an
`EntityView` root's reference and calls `PlatformWindow.Quit` (with the
ebiten backend this ends the run loop).

### Weak handles

```go
weak := ent.Downgrade()
if live, ok := weak.Upgrade(); ok { _ = live.Read(app) }
```

`Context[T].WeakEntity()` gives the same for the entity being updated. Weak
handles are what long-lived closures should capture.

### Observing and events

```go
sub := gpui.ObserveOn(app, ent, func(e gpui.Entity[Counter], cx *gpui.App) {
    fmt.Println("count is", e.Read(cx).N)
})
defer sub.Close()

type Saved struct{ Path string }

evSub := gpui.Subscribe(app, ent, func(ev *Saved) { fmt.Println("saved", ev.Path) })
defer evSub.Close()

// inside ent.Update(cx, func(c *Counter, ecx *gpui.Context[Counter]) { ... }):
gpui.Emit(ecx, &Saved{Path: "/tmp/x"}) // queued; delivered at the next flush
```

Events are keyed by `(entity, event type)`. Observers and subscribers become
active immediately on registration (GPUI defers activation to the next flush).
Callbacks run in insertion order; callbacks added during an emit run on the
next emit.

### Subscriptions

`Subscription.Close()` deregisters the callback. It is idempotent and
nil-safe, so `defer sub.Close()` is always valid. `IsClosed()` reports state.
`SubscriberSet[K, T]` (`gpui/subscription.go`) is the underlying keyed set; it
snapshots callbacks before emitting, so it is safe against re-entrant
insert/close.

### Defer, Spawn, globals

- `cx.Defer(f)` / `Context.Defer(f)` run `f` at the end of the current effect
  cycle.
- `Context.Spawn(f)` returns a `*Task` but does **not** use a goroutine: it is
  `Defer` plus `Cancel()`/`Detach()`.
- `SetGlobal(v)` stores one value per concrete type; `Global(&out)` copies it
  back. `UpdateGlobal(f func(any))` visits every global; to change one, store a
  pointer and mutate through it.

## The re-entrancy rule (AGENTS.md rule 7)

> Never hold a `*T` from an entity across a call that can re-enter the same
> entity - upgrade the weak handle at invoke time.

`Entity.Read` and the `*T` passed to `Update` point straight into the stored
value. Any call that can run other code - `Notify`, `Emit`, `Window.Refresh`
(which can trigger a draw), an observer, another `Update` on the same entity -
may mutate that same value or release it (`Release` frees the slot at the end
of the flush). Keep a `*T` only for a
straight-line stretch with no such call; otherwise re-fetch.

For event handlers use `Listener`, which captures a weak handle and upgrades
it each time the handler runs:

```go
func button(cx *gpui.Context[Counter]) gpui.IntoElement {
    click := gpui.Listener(cx, func(c *Counter, ev *gpui.ClickEvent, w *gpui.Window, app *gpui.App) {
        c.N++
        w.Refresh()
    })
    return gpui.NewDiv().OnClick(click)
}
```

`Listener` returns `func(*E, *Window, *App)`, which is exactly the shape of the
`Div.On*` mouse/key handlers. It no-ops if the entity is gone.

The same discipline applies to plain-struct roots: do not stash a pointer into
state that a handler may replace; capture the root pointer and read fields at
call time (as `examples/counter` does).
