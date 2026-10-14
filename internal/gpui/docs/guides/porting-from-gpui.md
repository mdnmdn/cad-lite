# Porting from Rust GPUI

go-gpui keeps GPUI's shape (retained state, element tree rebuilt every frame,
entities, actions, key contexts, Tailwind-style styling) but is a Go proof of
concept, so the surface differs where Go or the POC scope forces it. This page
maps the common Rust idioms to their Go counterparts and lists the places where
behaviour differs.

Source of truth for names: [reference](../reference/README.md). If you are new
to the Go side, read [getting-started](../getting-started.md) first.

## The shape of a program

Rust:

```rust
struct Counter { count: usize }

impl Render for Counter {
    fn render(&mut self, _window: &mut Window, cx: &mut Context<Self>) -> impl IntoElement {
        div().flex().flex_col().gap_2().p_4()
            .child(format!("Count: {}", self.count))
            .child(
                div().id("inc").p_2().bg(rgb(0x2196f3)).rounded_md()
                    .hover(|s| s.bg(rgb(0x1e88e5)))
                    .on_click(cx.listener(|this, _ev, _window, cx| { this.count += 1; cx.notify(); }))
                    .child("Increment"),
            )
    }
}

fn main() {
    Application::new().run(|cx| {
        cx.open_window(WindowOptions::default(), |_window, cx| cx.new(|_cx| Counter { count: 0 })).unwrap();
    });
}
```

Go (compiles against the current API; the entity's `Context` is kept in the
struct because `Render` receives the `*App`, not a `Context[Self]`):

```go
package main

import (
	"fmt"
	"log"

	"go-draw/internal/gpui"
	"go-draw/internal/gpui/ebitenplatform"
)

type Counter struct {
	count int
	ctx   *gpui.Context[Counter]
}

func (c *Counter) Render(w *gpui.Window, cx *gpui.App) gpui.IntoElement {
	inc := gpui.Listener(c.ctx, func(this *Counter, _ *gpui.ClickEvent, w *gpui.Window, cx *gpui.App) {
		this.count++
		this.ctx.Notify()
	})
	return gpui.NewDiv().Flex().FlexCol().Gap(8).P(16).
		Child(gpui.NewText(fmt.Sprintf("Count: %d", c.count))).
		Child(
			gpui.NewDiv().ID("inc").P(8).Bg(gpui.Blue500).Rounded(6).
				Hover(func(s *gpui.StyledMixin) *gpui.StyledMixin { return s.Bg(gpui.Blue600) }).
				OnClick(inc).
				Child(gpui.NewText("Increment").TextColor(gpui.White)),
		)
}

func main() {
	err := ebitenplatform.New().Run(func(cx *gpui.App) {
		counter := gpui.NewEntity(cx, func(c *gpui.Context[Counter]) Counter { return Counter{ctx: c} })
		build := func(*gpui.Window, *gpui.App) gpui.EntityRoot { return gpui.EntityView(counter) }
		if err := cx.OpenWindow(gpui.DefaultWindowOptions(), build); err != nil {
			log.Fatal(err)
		}
	})
	if err != nil {
		log.Fatal(err)
	}
}
```

Two things to notice. First, there is no `Application::new()`: the platform
(`ebitenplatform.New()`) owns the loop and hands you the `*App`. Second, the
`Counter` can also be a plain struct that is its own root with no entity at
all, calling `w.Refresh()` from handlers; that is what most examples do
([app and entities](../concepts/app-and-entities.md#root-views)).

## Mapping table

### Elements and children

| Rust GPUI | go-gpui |
|---|---|
| `div()` | `gpui.NewDiv()` |
| `.id("x")` | `.ID("x")` (use `fmt.Sprintf` for dynamic ids) |
| `.child(x)` | `.Child(x)` (variadic; `*Div`, `*Text`, `*StyledText`, `*UniformList`, `AnyElement` or anything with `IntoElement()`) |
| `.children(iter)` | `.Children(items...)` / loop with `.Child(...)` |
| a string as a child (`"hi"`, `SharedString`) | `gpui.NewText("hi")` or `gpui.Str("hi")` |
| `.when(cond, \|d\| d.bg(..))` | `if cond { d.Bg(..) }` (builders mutate in place, so ignore the return value) |
| `impl IntoElement` / `RenderOnce` component | a function returning `gpui.IntoElement` or `*gpui.Div` |
| `StyledText::new(..).with_highlights(..)` | `gpui.NewStyledText(gpui.TextRun{Text, Style}...)` (runs do not flow into one paragraph; see below) |
| `uniform_list(entity, "id", n, \|this, range, w, cx\| ..)` | `gpui.NewUniformList("id", n, func(start, end int, w, cx) []gpui.IntoElement)` (close over your view instead of passing the entity) |
| `UniformListScrollHandle` / `ScrollHandle` | `gpui.NewScrollHandle()` + `.TrackScroll(h)` |
| custom `Element` impl | `gpui.Element` + an `IntoElement` adapter ([cookbook](cookbook.md#custom-element-with-children)) |

### Layout and size

| Rust GPUI | go-gpui |
|---|---|
| `.flex()` | `.Flex()` (**vertical**, see differences) |
| `.flex_col()` / `.flex_row()` | `.FlexCol()` / `.FlexRow()` |
| `.flex_1()` / `.flex_auto()` / `.flex_none()` | `.Flex1()` / `.FlexAuto()` / `.FlexNone()` |
| `.flex_grow()` / `.flex_shrink()` / `.flex_basis(px)` | `.FlexGrow(v)` / `.FlexShrink(v)` / `.FlexBasis(px)` |
| `.items_center()` `.items_start()` `.items_end()` | `.ItemsCenter()` `.ItemsStart()` `.ItemsEnd()` (+ `.ItemsStretch()`) |
| `.justify_center()` `.justify_between()` ... | `.JustifyCenter()` `.JustifyBetween()` `.JustifyStart/End/Around()` |
| `.size_full()` | `.SizeFull()` |
| `.w(px(n))` / `.h(px(n))` / `.size(px(n))` | `.SizeW(n)` / `.SizeH(n)` / `.SizeWH(w, h)` |
| `.min_w` `.max_w` `.min_h` `.max_h` | `.MinW` `.MaxW` `.MinH` `.MaxH` |
| `.gap_2()` (Tailwind scale) | `.Gap(8)` |
| `.p_4()` `.px_2()` `.py_1()` `.pt_1()` ... | `.P(16)` `.Px_(8)` `.Py_(4)` `.Pt(4)` ... |
| `.m_2()` `.mx_auto()` `.mt_1()` ... | `.M(8)` `.MxAuto()` `.Mt(4)` ... |
| `.overflow_hidden()` | `.OverflowHidden()` |
| `.overflow_scroll()` / `.overflow_y_scroll()` | `.OverflowScroll()` / `.OverflowYScroll()` (+ `.ID(..)`) |
| `.hidden()` | `.Hidden()` |
| `.absolute()`, `.relative()`, `.top()`, `.right()`, `.bottom()`, `.left()`, `.inset()` | `Absolute()`, `Relative()`, `Top(v)`, `Right(v)`, `Bottom(v)`, `Left(v)`, `Inset(v)`; pixel offsets, immediate-parent anchoring |
| `.flex_wrap()`, `.flex_nowrap()` | `FlexWrap()`, `FlexNoWrap()` |
| `.gap_x_2()`, `.gap_y_2()` | `ColumnGap(8)`, `RowGap(8)` |
| `.content_start()`, `.content_center()`, `.content_end()`, `.content_stretch()`, `.content_between()`, `.content_around()` | `ContentStart()`, `ContentCenter()`, `ContentEnd()`, `ContentStretch()`, `ContentBetween()`, `ContentAround()` |
| `grid` | not supported |

Tailwind steps at the default 16 px rem: `_1`=4, `_2`=8, `_3`=12, `_4`=16, `_6`=24,
`_8`=32 px. Go takes plain `float32` pixels everywhere.

### Style: colour, border, shadow, text

| Rust GPUI | go-gpui |
|---|---|
| `.bg(rgb(0x2196f3))` | `.Bg(gpui.MustParseHex("#2196f3"))` |
| `rgba(0x00000080)` | `gpui.MustParseHex("#00000080")` or `gpui.RgbaOf(0, 0, 0, 128)` |
| `hsla(h, s, l, a)` (h in 0..1) | `gpui.Hsla{H: h * 360, S: s, L: l, A: a}` (`H` is **degrees**); `.ToRgba()` |
| `white()` / `black()` / `transparent_black()` | `gpui.White` / `gpui.Black` / `gpui.Transparent` |
| `color.opacity(0.5)` | `c.Opacity(0.5)` |
| `.bg(linear_gradient(angle, linear_color_stop(c, 0.), linear_color_stop(c2, 1.)))` | `.Bg(gpui.LinearGradient(angle, gpui.LinearColorStop(c, 0), gpui.LinearColorStop(c2, 1)))` |
| `.text_color(c)` | `.TextColor(c)` |
| `.text_xl()` etc. / `.text_size(px(n))` | `.TextSize(n)` (xs=12, sm=14, base=16, lg=18, xl=20, 2xl=24) |
| `.font_weight(FontWeight::BOLD)` | `.FontWeight(700)` |
| `.font_family("..")` | `.FontFamily("..")` |
| `.line_height(px(n))` | `.TextLineHeight(n)` |
| `.line_through()` | `.TextStrikethrough()` |
| `.border_1().border_color(c)` | `.Border(1, c)` |
| `.rounded_md()` / `.rounded(px(n))` | `.Rounded(6)` / `.Rounded(n)` (sm=2, md=6, lg=8, xl=12) |
| `.shadow_lg()` / `.shadow(vec![BoxShadow{..}])` | `.ShadowLg()` / `.Shadow(gpui.NewBoxShadow(..).WithBlur(..))` |
| `.opacity(0.5)` | `.Opacity(0.5)` |
| `.hover(\|s\| s.bg(c))` | `.Hover(func(s *gpui.StyledMixin) *gpui.StyledMixin { return s.Bg(c) })` |
| `.focus(..)` / `.focus_visible(..)` | `.Focus(..)` / `.FocusVisible(..)` |
| `.active(..)` | `Active(func(*StyledMixin) *StyledMixin)`; primary press, cancelled on leave |
| `.group_hover(..)`, `.cursor_pointer()` | not supported |
| `px(10.)`, `rems(1.)`, `relative(0.5)` | plain `float32` pixels in builders; `gpui.Px(10)` only when you need a `Length`; no `rems`, no percentages (`SizeFull`/`Flex1` instead) |
| `point(px(1.), px(2.))` / `size(..)` / `Bounds::new(..)` | `gpui.Point{X: 1, Y: 2}` / `gpui.Size{W, H}` / `gpui.NewBounds(x, y, w, h)` |

### Events, actions, focus

| Rust GPUI | go-gpui |
|---|---|
| `.on_click(cx.listener(\|this, ev, window, cx\| ..))` | `.OnClick(gpui.Listener(ctx, func(this *T, ev *gpui.ClickEvent, w *gpui.Window, cx *gpui.App) { .. }))`, or a plain closure over your view pointer |
| `.on_click(\|ev, window, cx\| ..)` | `.OnClick(func(ev *gpui.ClickEvent, w *gpui.Window, cx *gpui.App) { .. })` |
| `.on_mouse_down(MouseButton::Left, ..)` | `.OnMouseDown(func(e *gpui.MouseDownEvent, ..) { if !e.IsLeft() { return } .. })` (no button argument) |
| `.on_mouse_up` `.on_mouse_move` `.on_scroll_wheel` | `.OnMouseUp` `.OnMouseMove` `.OnScrollWheel` |
| `.on_hover(\|hovered, ..\| ..)` | `.OnHoverStart(f)` / `.OnHoverEnd(f)` |
| `.on_key_down(..)` / `.on_key_up(..)` | `.OnKeyDown(..)` / `.OnKeyUp(..)` |
| `.capture_*` listeners | not ported (bubble phase only) |
| `cx.stop_propagation()` | `w.StopPropagation()` |
| `actions!(ns, [Save, Quit])` | a type implementing `gpui.Action` (`Name`, `Clone`, `Equals`) or `gpui.NewAction("ns::Save")` |
| `.on_action(cx.listener(\|this, _: &Save, w, cx\| ..))` | `.OnAction(Save{}, func(a gpui.Action, w *gpui.Window, cx *gpui.App) { .. })` |
| `KeyBinding::new("cmd-s", Save, Some("Editor"))` | `gpui.NewKeyBinding("cmd-s", Save{}, "Editor")` (`""` = no context; panics on a malformed string) |
| `cx.bind_keys([..])` | `cx.Keymap().AddBindings(..)` |
| `.key_context("Editor")` | `.KeyContext("Editor")` |
| `cx.focus_handle()` | `cx.NewFocusHandle()` |
| `.track_focus(&handle)` | `.TrackFocus(handle)` (a `*FocusHandle`) |
| `.tab_index(0)` / `.tab_stop(true)` | `.TabIndex(0)` / `.TabStop(true)` |
| `window.focus(&h)` | `h.Focus(w)` or `w.RequestFocus(h)` |
| `window.focus_next()` / `focus_prev()` | `w.FocusNext()` / `w.FocusPrev()` (Tab is not bound by default) |
| `window.refresh()` | `w.Refresh()` |

### Entities and application state

| Rust GPUI | go-gpui |
|---|---|
| `Render::render(&mut self, window, cx: &mut Context<Self>)` | `func (v *T) Render(w *gpui.Window, cx *gpui.App) gpui.IntoElement` (`EntityRoot`) |
| `cx.new(\|cx\| T{..})` | `gpui.NewEntity(cx, func(c *gpui.Context[T]) T { .. })` |
| `entity.read(cx)` | `e.Read(cx)` returns `*T`, **nil** once released |
| `entity.update(cx, \|t, cx\| ..)` | `e.Update(cx, func(t *T, c *gpui.Context[T]) { .. })` |
| `cx.notify()` | `c.Notify()` (entity) or `w.Refresh()` (plain struct root) |
| `entity.downgrade()` / `weak.upgrade()` | `e.Downgrade()` / `weak.Upgrade()` returns `(Entity[T], bool)` |
| `cx.observe(&e, ..)` | `gpui.ObserveOn(cx, e, f)` or `ctx.Observe(e, f)` (same `T` only) |
| `cx.subscribe(&e, ..)` / `cx.emit(ev)` | `gpui.Subscribe(cx, e, func(ev *E) {..})` / `gpui.Emit(ctx, &ev)` |
| `Subscription` (drop to cancel) | `sub.Close()` (idempotent, nil-safe; `defer sub.Close()`) |
| `cx.defer(..)` | `cx.Defer(f)` |
| `cx.spawn(async ..)` / executors | `ctx.Spawn(f)`: runs `f` deferred, **not** concurrently; no executor or cross-goroutine dispatch |
| `cx.set_global(v)` / `cx.global::<T>()` | `cx.SetGlobal(v)` / `cx.Global(&out)` |
| `cx.quit()` | `cx.Quit()` |
| `Application::new().run(..)`, `cx.open_window(opts, ..)` | `ebitenplatform.New().Run(..)`, `cx.OpenWindow(opts, build)` |
| `Entity` as window root | `gpui.EntityView(entity)` |

## Known differences

These are intentional POC choices or limits; the first few catch most ports.

- **`Flex()` is a column.** Rust's `.flex()` alone leaves the default row
  direction. In Go `.Flex()` and `.FlexCol()` are both vertical, and a `Div`
  already lays out as a column, so always write `.FlexRow()` for a row.
- **Scroll offsets are positive.** GPUI stores scroll offsets as negative
  points (content moved up/left). `ScrollHandle.Offset()` and `SetOffset`
  use positive distances scrolled, and `ScrollWheelEvent.DeltaY > 0` scrolls
  down. Drop the sign flips when porting.
- **`Flex1` basis.** `Flex1()` is `flex: 1 1 0%` as in GPUI, so siblings share
  space equally regardless of content. Since there are no percentages the zero
  basis is `Px(0)`; use `FlexAuto()` (`1 1 auto`) when the content size should
  count first, and `FlexNone()` for a fixed item. Each flexed item still stops
  at its automatic minimum size unless it clips (`OverflowHidden`).
- **`SizeFull` is not `100%`.** It sets both axes to `Flex(1)` with zero
  minimums, so in a row it shares width with siblings instead of overflowing.
- **No percentages, rems, relative translation, wrap-reverse, grid,
  `group_hover`, cursors, drag and drop, `img`/`svg`, animations.** Borders
  paint inside the box and do not enter layout (add padding yourself).
- **`TextInput` is not an entity.** GPUI's text field example is an entity
  with a `FocusHandle` and an input-handler implementation. Here
  `gpui.NewTextInput(placeholder)` returns a plain struct that you create once,
  own in your view and `Render` each frame; read it with `Text()`.
- **Action naming.** `actions!` generates unit structs; Go has no macro, so
  either write a three-method type per action or use `gpui.NewAction(name)`,
  where the name is part of the handler key (`NewAction("a")` and
  `NewAction("b")` are different). User-defined action types are keyed by Go
  type, so a zero value can register the handler. `OnAction` takes an action
  *value*, not a type parameter.
- **No `Drop`, so explicit `Release`.** Go cannot run code when a handle goes
  out of scope. `NewEntity` returns the first strong reference; `Clone()` adds
  one and `Release(cx)` drops one; copies, `Context.Entity()` and `Upgrade()`
  results are borrowed and must not be released. Subscriptions are closed with
  `Close()` (or when the entity that created them is released).
- **`Read` can return nil.** A released entity reads as `nil`; check it.
  The `*T` from `Read`/`Update` points into the stored value: do not hold it
  across `Notify`, `Emit`, `Refresh` or another `Update` that can re-enter the
  entity. Capture a weak handle or use `gpui.Listener` in long-lived closures.
- **`Render` gets `*App`, not `Context[Self]`.** Keep the `*Context[T]` from
  the build function in the struct when you need `Notify` or `Listener`.
- **Entity notifications redraw only windows that read the entity during their
  last draw** (as GPUI does), so entity-rooted views redraw on `Notify`;
  plain-struct roots need `w.Refresh()`.
- **One goroutine.** There is no async runtime. `Spawn` defers; background
  work is yours to run and to hand back to the UI goroutine safely (only
  `Quit` is goroutine-safe).
- **Single keystrokes only.** No chords or `KeyBinding` multi-stroke
  sequences. `"cmd-"` is the Super key on Windows/Linux; `"ctrl-"` is always
  Ctrl.
- **Click events carry only `Up`.** `ClickEvent{Up *MouseUpEvent}`; the press
  position is not included.
- **`StyledText` is not a flowing paragraph.** Each run wraps independently
  and runs never share a line break.
- **Text leaves paint no box.** `Text` ignores `Bg`/`Border`/`Shadow`; wrap it in
  a `Div`.
- **Single window per ebiten platform.** The headless platform can open many.
- **Shape of results.** Handlers receive `*Event` pointers; `Keymap` and
  `HeadlessWindow.Dispatch` take events by value.
