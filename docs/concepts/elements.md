# Elements

Source: `gpui/element.go`, `gpui/div.go`, `gpui/text_elem.go`.

An element is a **per-frame value**. `Render` builds a fresh tree every frame;
nothing about an element survives to the next frame except what you store in
your own state or in `Window.SetElementState`.

## The Element interface

```go
type Element interface {
    ID() (ElementID, bool)
    RequestLayout(w *Window, cx *App) (LayoutID, any)
    Prepaint(w *Window, cx *App, id ElementID, bounds Bounds, layoutState any) any
    Paint(w *Window, cx *App, id ElementID, bounds Bounds, layoutState, prepaintState any)
}

type IntoElement interface{ IntoElement() Element }
```

Phases, in the order the window runs them (see
[architecture.md](../architecture.md)):

1. **RequestLayout** - register a `LayoutNode` and return its `LayoutID` plus
   state that is handed back to the later phases.
2. **Prepaint** - bounds are now known (absolute, logical pixels). Register
   hitboxes, listeners, key nodes and input handlers; return state for Paint.
3. **Paint** - emit `Quad`, `GlyphSprite`, `Underline` primitives through the
   window. Paint draws **only the element itself**; the window walks children
   in tree order.

`Child`/`Children` accept any `IntoElement`, which is satisfied by `*Div`,
`*Text`, `*StyledText`, `AnyElement` (via `NewAnyElement`) and your own types.
`Str("hello")` is shorthand for a text child.

### IDs

`Div.ID("name")` sets a local key. The window forms a `GlobalElementID` by
appending `{Index, Key}` for every ancestor-to-child step. Ids matter for:

- retained state (`Window.ElementState`, scroll offsets - a scroll container
  must have a stable path across frames, so give lists an `.ID(...)`);
- hover bookkeeping and click pairing (`Window.HitTest` returns the id of the
  topmost element under a point; two ids compare with `Equal`).

Without an explicit id the path is just the child index, which is stable as
long as the tree shape is.

## The Div/Element adapter pattern

Go has no overloading, but Zed's API wants both a fluent builder
`div.id("x")` and the trait method `Element::id()`. In Go, `*Div` has
`ID(string) *Div` (the builder), so it **cannot** also have
`ID() (ElementID, bool)`. The fix, used by every element type in the package:

- the builder type (`*Div`, `*Text`, `*StyledText`) carries the fluent
  methods and implements only `IntoElement`;
- `IntoElement()` returns a tiny adapter value (`divElement`, `textElement`,
  `styledTextElement`) that implements `Element` and delegates to the builder.

Follow this pattern for new elements (AGENTS.md rule 6).

## Div

`NewDiv()` starts from `DefaultStyle()`: visible, column direction, auto
sizes, shrink 1, stretch cross-axis. Builder methods return `*Div`:

```go
gpui.NewDiv().ID("card").FlexCol().Gap(8).P(12).
    Bg(gpui.White).Rounded(8).Border(1, gpui.Gray300).
    OnClick(func(e *gpui.ClickEvent, w *gpui.Window, cx *gpui.App) { /* ... */ }).
    Child(
        gpui.NewText("Title").TextSize(20).FontWeight(700),
        gpui.Str("shorthand"),
    )
```

Chaining: `*Div` and `*Text` re-declare every `StyledMixin` method so the
chain keeps its concrete type (`gpui/div.go`, `gpui/text_elem.go`). Methods
promoted from the embedded `StyledMixin` return `*StyledMixin` and end the
chain - this is what happens with `*StyledText`, which re-declares none of
them. Style a `StyledText` through a variable (`st.StyleRef()` /
`st.P(4)` as a statement) or wrap it in a styled `Div`.

Event/focus builders on `Div`: `OnMouseDown`, `OnMouseUp`, `OnMouseMove`,
`OnClick`, `OnScrollWheel`, `OnKeyDown`, `OnKeyUp`, `OnHoverStart`,
`OnHoverEnd`, `OnAction`, `TrackFocus`, `KeyContext`, `TabIndex` (stored, not
yet used for traversal). Each setter holds one handler; a second call replaces
the first. See [events-and-input.md](events-and-input.md).

What a Div does per phase:

- **RequestLayout:** a `LayoutNode` with its resolved style; recurses into
  children (nil children are skipped).
- **Prepaint:** always registers a hitbox (even with no listeners); a key node
  if it has key handlers, actions, a key context or a focus handle; scroll
  extent for non-visible overflow.
- **Paint:** background quad (if alpha > 0) and border quad (if border width
  and colour are set). The border is painted inside the bounds.

## Text, StyledText, Str

```go
gpui.NewText("hello").TextSize(16).TextColor(gpui.Gray900).FontWeight(700)

runs := gpui.DefaultTextStyle()
bold := runs
bold.FontWeight = 700
gpui.NewStyledText(
    gpui.TextRun{Text: "plain ", Style: runs},
    gpui.TextRun{Text: "bold", Style: bold},
)
```

- `Text` is a single-run leaf. `(*Text).Text(s)` replaces the string. It
  lays out as a leaf whose size comes from the shaper and wraps at the width
  the layout gives it. It registers no hitbox, so clicks pass to the parent.
- `StyledText` is a leaf made of `TextRun{Text, Style}`s drawn left to right.
  Each run is measured and wrapped independently at the same width, and run
  widths are summed - it is **not** a flowing paragraph: runs never share a
  line break. A run whose `Style.FontSize <= 0` is replaced wholesale by
  `DefaultTextStyle()`, so start from `DefaultTextStyle()` rather than a bare
  `TextStyle{}`.
- Both embed `StyledMixin` (box styling: padding, background, size, etc.).
  `Text` re-declares the style methods to keep a `*Text` chain; `StyledText`
  does not, so style it with statements on a variable or wrap it in a `Div`.
- Text-colour/size/weight set on a **`Div` do not cascade** to child text in
  the current code (each `Text` resolves its own style from `DefaultStyle()`).
  Set text styles on the `Text` itself. See [styling.md](styling.md).

## Composition: functions, not types

The idiomatic way to build components is a function that returns an
`IntoElement`:

```go
func badge(label string, bg gpui.Rgba) gpui.IntoElement {
    return gpui.NewDiv().Px_(8).Py_(2).Bg(bg).Rounded(4).
        Child(gpui.NewText(label).TextColor(gpui.White))
}
```

Stateful components are plain structs with a `Render(w, cx) IntoElement`
method called from the parent's `Render` every frame (this is how
`TextInput` works).

## Writing a custom element

You need a custom `Element` only when you must paint your own primitives
(a chart, a swatch, a caret). Constraints of the current API:

- The window walks children through an unexported interface
  (`hasChildren`, implemented by `*Div` adapters), so a custom element is
  effectively a **leaf**: its children are not visited.
- `RequestLayout` must return a `LayoutID`, and the only way to obtain one is
  the unexported `Window.registerLayoutNode`. From outside the package the
  practical approach is to **delegate to a childless `Div`'s element** for
  `ID`/`RequestLayout`/`Prepaint`, and add your own painting in `Paint`.
  (Inside package `gpui` you can build a `LayoutNode` directly, as `Text` does.)

A leaf element that paints a filled square, compiled against the current API:

```go
type Swatch struct {
    color gpui.Rgba
    size  float32
}

func NewSwatch(c gpui.Rgba, size float32) *Swatch { return &Swatch{color: c, size: size} }

// IntoElement returns the adapter, which carries the Element methods.
func (s *Swatch) IntoElement() gpui.Element {
    return swatchElement{s: s, inner: gpui.NewDiv().SizeWH(s.size, s.size).IntoElement()}
}

type swatchElement struct {
    s     *Swatch
    inner gpui.Element
}

func (e swatchElement) ID() (gpui.ElementID, bool) { return e.inner.ID() }

func (e swatchElement) RequestLayout(w *gpui.Window, cx *gpui.App) (gpui.LayoutID, any) {
    return e.inner.RequestLayout(w, cx)
}

func (e swatchElement) Prepaint(w *gpui.Window, cx *gpui.App, id gpui.ElementID, b gpui.Bounds, ls any) any {
    return e.inner.Prepaint(w, cx, id, b, ls) // registers the hitbox
}

func (e swatchElement) Paint(w *gpui.Window, cx *gpui.App, id gpui.ElementID, b gpui.Bounds, ls, ps any) {
    e.inner.Paint(w, cx, id, b, ls, ps)
    w.PaintQuad(gpui.Quad{Bounds: b, Background: gpui.BackgroundSolid, Fill: e.s.color})
}
```

Use it like any child: `gpui.NewDiv().P(10).Child(NewSwatch(gpui.Red500, 40))`.

Notes for authors:

- `bounds` are absolute logical pixels, already shifted by scroll offsets and
  ancestors. Paint primitives in those coordinates.
- `PaintQuad`/`PaintGlyph`/`PaintUnderline` fill an unset `ContentMask` with
  the current clip; you normally leave `ContentMask` zero.
- Text: `Window.TextMeasurer()` exposes `MeasureWrapped` / `ShapeWrapped`;
  it is `nil` when no system font was found.
- Retained state across frames: `Window.SetElementState(gid, v)` /
  `ElementState(gid, &out)` keyed by `GlobalElementID`; `Prepaint`/`Paint` do
  not receive the global id directly (`id` is the path, so
  `gpui.GlobalElementID{Path: id.Path}` is the key).
