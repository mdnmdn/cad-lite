# Getting started

A walk from an empty module to a tested, snapshot-able app. Every snippet here
compiles against the current API (checked in a scratch module with
`replace go-gpui => ../..`).

## Install

The module path is `go-gpui`; it is not published, so use a checkout and a
`replace` directive:

```
mkdir myapp && cd myapp
go mod init myapp
go mod edit -require=go-gpui@v0.0.0 -replace=go-gpui=/path/to/go-gpui
go mod tidy
```

Requires Go 1.26+. There is no cgo, and all drawing is done by a CPU
rasterizer; ebiten only presents the finished pixels. A system font is
needed for text (see [concepts/text.md](concepts/text.md); `GPUI_FONT` points
at a specific file). Try the repo itself first:

```
go run ./examples/todo
go run ./examples/todo -png todo.png   # headless: render one frame and exit
```

## First window

A window root is any value with `Render(w *gpui.Window, cx *gpui.App)
gpui.IntoElement`. A platform owns the event loop; `Run` hands you the `App`:

```go
package main

import (
	"log"

	"go-gpui/gpui"
	"go-gpui/gpui/ebitenplatform"
)

type Hello struct{}

func (Hello) Render(w *gpui.Window, cx *gpui.App) gpui.IntoElement {
	return gpui.NewDiv().FlexCol().SizeFull().Bg(gpui.Gray50).ItemsCenter().JustifyCenter().
		Child(gpui.NewText("Hello, GPUI").TextSize(32).FontWeight(700).TextColor(gpui.Gray900))
}

func main() {
	err := ebitenplatform.New().Run(func(cx *gpui.App) {
		opts := gpui.WindowOptions{Width: 400, Height: 200, Title: "Hello", Resizable: true}
		build := func(*gpui.Window, *gpui.App) gpui.EntityRoot { return Hello{} }
		if err := cx.OpenWindow(opts, build); err != nil {
			log.Fatal(err)
		}
	})
	if err != nil {
		log.Fatal(err)
	}
}
```

Elements are rebuilt from scratch on every `Render`: `NewDiv()` and `NewText()`
return builders, `.Child(...)` nests, and the Tailwind-style methods set the
style ([concepts/styling.md](concepts/styling.md)). `Div` is a flex container
(direction defaults to column; `FlexRow()`/`FlexCol()` set it explicitly), and
`SizeFull()` makes it fill its parent. See
[concepts/elements.md](concepts/elements.md) and
[concepts/layout.md](concepts/layout.md).

## State, click handlers and Refresh

The simplest view is a struct that holds its own state. Handlers mutate it and
call `w.Refresh()` to request a redraw; the next frame calls `Render` again.

```go
type Counter struct{ count int }

func (c *Counter) Render(w *gpui.Window, cx *gpui.App) gpui.IntoElement {
	button := gpui.NewDiv().ID("inc").P(12).Bg(gpui.Blue500).Rounded(8).
		Hover(func(s *gpui.StyledMixin) *gpui.StyledMixin { return s.Bg(gpui.Blue600) }).
		OnClick(func(_ *gpui.ClickEvent, w *gpui.Window, _ *gpui.App) {
			c.count++
			w.Refresh()
		}).
		Child(gpui.NewText("Increment").TextColor(gpui.White))
	return gpui.NewDiv().FlexCol().SizeFull().Gap(12).P(16).
		Child(gpui.NewText(fmt.Sprintf("Count: %d", c.count)).TextSize(24), button)
}
```

Give interactive elements a stable `.ID(...)`: hover, click identity and
scroll offsets are keyed by it. `Hover`/`Focus`/`FocusVisible` take a closure
that returns a style refinement (visual properties only; see styling.md).
Open it with `return &Counter{}` in the `build` function.

## Entities

For state shared between views, or observed from elsewhere, use an
`Entity[T]`. `Context[T]` is the per-entity handle: `Notify()` marks observers
and windows rendering the entity as dirty. Closures that outlive a call must
not hold a `*T`; `gpui.Listener` upgrades a weak handle when it runs.

```go
type Counter struct {
	count int
	ctx   *gpui.Context[Counter]
}

func (c *Counter) Render(w *gpui.Window, cx *gpui.App) gpui.IntoElement {
	inc := gpui.Listener(c.ctx, func(st *Counter, _ *gpui.ClickEvent, w *gpui.Window, cx *gpui.App) {
		st.count++
		st.ctx.Notify()
	})
	return gpui.NewDiv().FlexCol().Gap(8).P(16).Child(
		gpui.NewText(fmt.Sprint(c.count)),
		gpui.NewDiv().ID("inc").P(8).Bg(gpui.Blue500).OnClick(inc).Child(gpui.NewText("+1").TextColor(gpui.White)),
	)
}

// inside the Run callback:
counter := gpui.NewEntity(cx, func(c *gpui.Context[Counter]) Counter { return Counter{ctx: c} })
gpui.ObserveOn(cx, counter, func(e gpui.Entity[Counter], cx *gpui.App) {
	fmt.Println("count is now", e.Read(cx).count)
})
build := func(*gpui.Window, *gpui.App) gpui.EntityRoot { return gpui.EntityView(counter) }
_ = cx.OpenWindow(gpui.DefaultWindowOptions(), build)
cx.Update(func(cx *gpui.App) {
	counter.Update(cx, func(c *Counter, cx *gpui.Context[Counter]) {
		c.count = 10
		cx.Notify()
	})
})
```

`Entity.Update` does not flush effects itself: observers and window redraws
run when the outermost `App.Update` returns. Event listeners already run
inside one; code outside any listener, such as the launch callback, wraps its
mutations in `cx.Update` as above, otherwise the `ObserveOn` callback does not
fire until some later update completes.

`EntityView` renders the entity as a window root and releases it when the
window closes. Details and the re-entrancy rule:
[concepts/app-and-entities.md](concepts/app-and-entities.md).

## Keybindings and actions

An action is a value implementing `gpui.Action` (`Name`, `Clone`, `Equals`) or
a `gpui.NewAction(name)`. Bind keystrokes to actions in a **key context**, set
the context on a div with `KeyContext`, and handle the action with `OnAction`.
Bindings dispatch to the focused element's path (or the root when nothing is
focused); `TabIndex` makes a div focusable.

```go
type incAction struct{}

func (incAction) Name() string              { return "increment" }
func (incAction) Clone() gpui.Action        { return incAction{} }
func (incAction) Equals(o gpui.Action) bool { _, ok := o.(incAction); return ok }

var reset = gpui.NewAction("reset")

type View struct{ count int }

func (v *View) Render(w *gpui.Window, cx *gpui.App) gpui.IntoElement {
	return gpui.NewDiv().KeyContext("counter").TabIndex(0).SizeFull().P(16).
		OnAction(incAction{}, func(_ gpui.Action, w *gpui.Window, _ *gpui.App) { v.count++; w.Refresh() }).
		OnAction(reset, func(_ gpui.Action, w *gpui.Window, _ *gpui.App) { v.count = 0; w.Refresh() }).
		Child(gpui.NewText(fmt.Sprint(v.count)))
}

// inside the Run callback:
cx.Keymap().AddBindings(
	gpui.NewKeyBinding("up", incAction{}, "counter"),
	gpui.NewKeyBinding("cmd-r", reset, "counter"),
)
```

Single keystrokes only (no chords). Printable-key bindings such as `"a"` are
matched before a focused `TextInput` sees the character, so scope them with a
context predicate such as `"!textbox"`. Rules and ordering:
[concepts/events-and-input.md](concepts/events-and-input.md).

## TextInput

`TextInput` is a single-line field with caret, selection, clipboard and IME
plumbing. Create it once (not in `Render`), render it each frame, and read it
with `Text()`:

```go
type Form struct {
	name *gpui.TextInput
	last string
}

func NewForm() *Form {
	f := &Form{name: gpui.NewTextInput("Your name")}
	f.name.OnSubmit(func(text string, w *gpui.Window, _ *gpui.App) {
		f.last = text
		f.name.SetText("")
		w.Refresh()
	})
	return f
}

func (f *Form) Render(w *gpui.Window, cx *gpui.App) gpui.IntoElement {
	return gpui.NewDiv().FlexCol().Gap(8).P(16).Child(
		f.name.Render(w, cx),
		gpui.NewText("Hello, "+f.last),
	)
}
```

Clicking the field focuses it. `OnSubmit` fires on Enter, `OnChange` on every
edit. The field lives in the `"textbox"` key context. `TextInput` handles
`enter` (runs `OnSubmit`) and `escape` (blurs) itself and stops propagation,
so key bindings for them in that context never fire while the field is
focused. `examples/todo` still binds `enter`/`escape` in `"textbox"`, but those
bindings are redundant; `OnSubmit` is enough.

## Rendering to PNG

No window is needed to rasterize a view: `RenderToPNG` and `RenderToImage`
build the root on a headless platform, lay out at `w x h` logical pixels, and
draw at the given scale (the file is `w*scale x h*scale`).

```go
err := gpui.RenderToPNG("card.png", 320, 120, 2, Card{},
	gpui.WithBackgroundColor(gpui.White), gpui.WithTitle("card"))

img, err := gpui.RenderToImage(320, 120, 1, Card{}) // *image.NRGBA
```

Options: `WithBackgroundColor`, `WithKeymap`, `WithTitle`. This is the basis of
the `-png PATH` flag every example has. In a running window, press
`cmd-shift-s` (macOS) or `ctrl-shift-s` to save the last frame as
`snapshot-YYYYMMDD-HHMMSS.png` in the working directory; configure with
`ebitenplatform.WithSnapshotKey` ([concepts/platform.md](concepts/platform.md)).

Headless rendering has no pointer state, and a window starts unfocused.
Examples that want to show a focused or scrolled state seed it from a flag
(`-focus`, `-scroll`); focus is seeded by calling `w.RequestFocus` during the
first `Render` (see `examples/tab_stop`).

## Headless testing

`HeadlessPlatform` gives each window a `HeadlessWindow` whose `Dispatch` feeds
a synthetic input event through the real hit-testing and dispatch path, then
redraws if the event invalidated the frame. Assert on your state, the pixels
(`Image()`, `Pixels()`) or save a PNG (`SavePNG`).

```go
func TestClick(t *testing.T) {
	c := &Counter{}
	p := gpui.NewHeadlessPlatform()
	app := gpui.NewAppWithPlatform(p)
	err := app.OpenWindow(gpui.WindowOptions{Width: 200, Height: 100},
		func(*gpui.Window, *gpui.App) gpui.EntityRoot { return c })
	if err != nil {
		t.Fatal(err)
	}
	hw := p.Windows()[0]
	hw.Dispatch(gpui.ClickEvent{Up: &gpui.MouseUpEvent{Position: gpui.Point{X: 10, Y: 10}}})
	if c.count != 1 {
		t.Fatalf("count = %d, want 1", c.count)
	}
}
```

Notes:

- The platform does not synthesize clicks: send a `ClickEvent` for click
  handlers (a real press sends `MouseDownEvent`, `MouseUpEvent`, then
  `ClickEvent`). Hover needs `MouseMoveEvent`.
- To type into a `TextInput`, dispatch a `MouseDownEvent` on it to focus it,
  then a `KeyDownEvent` per character with `PreferCharacterInput: true` and
  `Keystroke.KeyChar` set; non-printable keys (`"enter"`, `"backspace"`) need
  only `Keystroke.Key`.
- A window starts with no focus. Seed it by calling `w.RequestFocus(handle)`
  in the first `Render` (as `examples/tab_stop` does), which works for both
  `RenderToPNG` and tests.

## Where next

[examples.md](examples.md) lists all 17 example programs; the
[concepts](README.md) pages cover each subsystem in depth.

See also: [cookbook](guides/cookbook.md) for ready-made recipes, [testing](guides/testing.md) for headless tests and snapshots, [porting from Rust GPUI](guides/porting-from-gpui.md), [troubleshooting](guides/troubleshooting.md) and the [API reference](reference/README.md).
