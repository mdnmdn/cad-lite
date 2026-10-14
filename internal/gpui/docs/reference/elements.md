# Reference: elements

Source: `element.go`, `div.go`, `text_elem.go`,
`uniform_list.go`, `scroll.go`, `input.go`, `layout.go`.

Back to the [reference index](README.md). Concepts:
[elements](../concepts/elements.md), [layout](../concepts/layout.md).
The style builders are in [style-builders.md](style-builders.md), event
payloads in [events.md](events.md).

## Core types

```text
type IntoElement interface{ IntoElement() Element }   // *Div, *Text, *StyledText, *UniformList, AnyElement, your own
type Element interface {
    ID() (ElementID, bool)
    RequestLayout(w *Window, cx *App) (LayoutID, any)
    Prepaint(w *Window, cx *App, id ElementID, bounds Bounds, layoutState any) any
    Paint(w *Window, cx *App, id ElementID, bounds Bounds, layoutState, prepaintState any)
}

type EntityRoot interface{ Render(w *Window, cx *App) IntoElement }  // a window's root view
type RenderFunc func(w *Window, cx *App) IntoElement                 // adapts a function to EntityRoot
func (f RenderFunc) Render(w *Window, cx *App) IntoElement

func IntoElementOf(e IntoElement) Element        // nil-safe normalisation
func Str(s string) IntoElement                   // a string as a child: Child(Str("hi"))

type AnyElement struct{ /* opaque */ }
func NewAnyElement(e IntoElement) AnyElement     // nil yields an empty AnyElement
func (a AnyElement) Element() Element            // nil when empty

type ElementPathKey struct { Index int; Key string }
type ElementID struct { Path []ElementPathKey }          // the element's self-declared key
type GlobalElementID struct { Path []ElementPathKey }   // ancestor path + local key; retained-state map key
func (a GlobalElementID) Equal(b GlobalElementID) bool
```

`EntityView(e Entity[T])` adapts an entity as a root; it is listed in
[app-window.md](app-window.md#entities).

## Div

```text
func NewDiv() *Div            // starts from DefaultStyle(): visible, column, auto size
func (d *Div) IntoElement() Element
```

All style builders return `*Div`
([style-builders.md](style-builders.md)). The non-style methods:

### Identity and children

| Method | Meaning |
|---|---|
| `ID(id string) *Div` | Local element id. Keys retained state (scroll offset, implicit focus handle), hover/active bookkeeping and click pairing; give interactive and scrollable divs a stable one. |
| `Child(children ...IntoElement) *Div` | Append children (nil children are skipped). |
| `Children(children ...IntoElement) *Div` | Alias of `Child`. |
| `IntoElement() Element` | Adapter that makes `*Div` an `Element` (see the adapter pattern in [concepts/elements.md](../concepts/elements.md)). |

### Mouse and keyboard listeners

Each setter holds one handler; a second call replaces the first.

| Method | Handler signature |
|---|---|
| `OnMouseDown(f)` | `func(*MouseDownEvent, *Window, *App)` |
| `OnMouseUp(f)` | `func(*MouseUpEvent, *Window, *App)` |
| `OnMouseMove(f)` | `func(*MouseMoveEvent, *Window, *App)` |
| `OnClick(f)` | `func(*ClickEvent, *Window, *App)` |
| `OnScrollWheel(f)` | `func(*ScrollWheelEvent, *Window, *App)` |
| `OnHoverStart(f)` | `func(*Window, *App)` |
| `OnHoverEnd(f)` | `func(*Window, *App)` |
| `OnKeyDown(f)` | `func(*KeyDownEvent, *Window, *App)` |
| `OnKeyUp(f)` | `func(*KeyUpEvent, *Window, *App)` |
| `Tooltip(text)` | `string`; hover tooltip, see [events-and-input.md](../concepts/events-and-input.md#tooltips) |
| `OnAction(action Action, f)` | `f` is `func(Action, *Window, *App)`; keyed by `ActionKeyOf(action)` |

`gpui.Listener(ctx, func(state *T, ev *E, w, cx))` produces the
`func(*E, *Window, *App)` shape for the first eight (see
[app-window.md](app-window.md#listeners-and-events)).

### Focus, key context, tab order

| Method | Meaning |
|---|---|
| `TrackFocus(h *FocusHandle) *Div` | Make the div the keyboard-focus target for `h`. |
| `KeyContext(ctx string) *Div` | Name the key context visible to keymap predicates while the div is on the focus path. |
| `TabIndex(i int) *Div` | Tab-order position; makes the div a tab stop. Without `TrackFocus` it gets an implicit handle retained per element id. |
| `TabStop(b bool) *Div` | Whether Tab traversal may stop here, overriding the tracked handle. |

### Scrolling and interaction styles

| Method | Meaning |
|---|---|
| `TrackScroll(h *ScrollHandle) *Div` | Bind `h` to this scroll container. |
| `ShowScrollbar() *Div` | Thin overlay scrollbar with primary thumb dragging and track paging when content overflows. |
| `Hover(f)`, `Focus(f)`, `FocusVisible(f)`, `Active(f)` | Interaction style refinements; see [style-builders.md](style-builders.md#interaction-styles-div-only). |

What a `Div` does per phase: it registers a hitbox always, mouse listeners, a
key node when it has key handlers, actions, a key context or a focus handle,
and paints a background quad, a border quad and shadows; an
`OverflowHidden`/`OverflowScroll` div clips its children.

## Text

```text
func NewText(s string) *Text
func (t *Text) Text(s string) *Text      // replace the string
func (t *Text) IntoElement() Element
```

A single-run leaf. It registers no hitbox (clicks pass to the parent) and
paints no box. All style methods listed for `*Text` in
[style-builders.md](style-builders.md#which-builders-exist-on-which-type-summary)
return `*Text`.

## StyledText and TextRun

```text
type TextRun struct { Text string; Style TextStyle }
func NewStyledText(runs ...TextRun) *StyledText
func (s *StyledText) IntoElement() Element
```

Multiple runs drawn left to right; each run wraps independently at the same
width and runs never share a line break. Start each run from
`DefaultTextStyle()` (a run with `FontSize <= 0` is replaced by it wholesale).
`StyledText` embeds `StyledMixin` but re-declares none of the builders, so
chains on it end with `*StyledMixin`.

## Str

`Str(s)` returns an `IntoElement` that renders `s` as a default-styled text
child; it inherits the text style cascaded from ancestors.

## TextInput

A single-line field. Not an `Entity`: create it once, keep it in your view
struct and call `Render` every frame. Setters that mutate it call
`Window.Refresh`.

| Method | Meaning |
|---|---|
| `NewTextInput(placeholder string) *TextInput` | New empty field showing `placeholder` until text is entered. |
| `Render(w *Window, cx *App) IntoElement` | The focusable box (key context `"textbox"`, 8 px padding, 1 px border) with one clipped text/placeholder line, focused selection highlight and caret. |
| `Text() string` | Current value. |
| `SetText(s string)` | Replace the value; caret moves to end, cancels selection capture and refreshes if rendered. |
| `OnChange(f func(string, *Window, *App)) *TextInput` | Called on every edit made through input dispatch. |
| `OnSubmit(f func(string, *Window, *App)) *TextInput` | Called with the text on Enter while focused. |
| `OnEscape(f func(*Window, *App)) *TextInput` | Called on Escape while focused. With any callback registered the field keeps focus; without one, Escape blurs it. |
| `Style(f func(*Div) *Div) *TextInput` | Applied to the field's box after the defaults (white background, 1 px `Gray300` border, `Rounded(6)`, `P(8)`), so it can override them. Set background, border, padding and text colour/size here; the text colour also colours the caret; `Div.Focus` styles the focused border. A later call replaces the earlier one. |
| `PlaceholderColor(c Rgba) *TextInput` | Placeholder colour (default `Gray500`). |
| `SelectionColor(c Rgba) *TextInput` | Selection highlight fill (default `Blue500` at 25% opacity). |
| `FocusHandle() *FocusHandle` | The field's handle; valid only after `Render` has run once. |

```go
dark := gpui.RgbaOf(24, 28, 33, 255)
in := gpui.NewTextInput("Search").
	Style(func(d *gpui.Div) *gpui.Div {
		return d.Bg(dark).Border(1, gpui.Gray700).TextColor(gpui.Gray100).
			Focus(func(s *gpui.StyledMixin) *gpui.StyledMixin { return s.Border(1, gpui.Blue500) })
	}).
	PlaceholderColor(gpui.Gray500)
```

It also implements `PlatformInputHandler`
([platform.md](platform.md#text-input-surface)): `TextForRange`,
`SelectedTextRange`, `SetSelectedTextRange`, `MarkedTextRange`, `UnmarkText`,
`ReplaceTextInRange`, `BoundsForRange`, `CharacterIndexForPoint`,
`TextLengthUTF16`, `AcceptsTextInput`. Built-in keys: `enter`, `escape`
(blur), `backspace`, `delete`, `left`, `right`, `home`, `end`; Shift extends
with these navigation keys. Primary click/drag selects, double selects a Unicode
word-class run, triple selects the line. See [selection semantics](../concepts/events-and-input.md#textinput-and-platforminputhandler).

`TextForRange` validates UTF-16 rune boundaries. `SetSelectedTextRange` clamps and
snaps down, normalizes reversed endpoints and refreshes the field; it does not
fire OnChange. `BoundsForRange` and `CharacterIndexForPoint` use full-line shaped
caret stops after rendering, rune-count estimates before rendering. Capture
cancels on removal/replacement, blur, field bounds change or text/programmatic
selection changes. There is no horizontal autoscroll, grapheme navigation or
BiDi visual selection.

## UniformList

```text
func NewUniformList(id string, count int,
    render func(start, end int, w *Window, cx *App) []IntoElement) *UniformList
func (u *UniformList) IntoElement() Element
func (u *UniformList) TrackScroll(h *ScrollHandle) *UniformList
func (u *UniformList) ShowScrollbar() *UniformList
```

A virtualised, vertically scrolling list of equal-height rows. Row height is
measured from item 0 at the list's content width; `render` is called once per
frame with the half-open range of rows intersecting the viewport and returns
exactly the elements of rows `[start, end)`. With an auto height the list sizes
itself to `count` rows. The chain-preserving builders are `Bg`, `Border`,
`Flex1`, `MaxH`, `MinH`, `Opacity`, `P`, `Px_`, `Py_`, `Rounded`, `SizeFull`,
`SizeH`, `SizeW`, `SizeWH`, `TextColor`, plus the two above.

`ShowScrollbar` uses the same interactive track behavior on Div and UniformList:
primary thumb dragging continues outside bounds until release, and a track press
pages one viewport toward the pointer. Geometry/extent changes or programmatic
offset changes cancel capture. Stable IDs preserve it across ordinary redraws.
See [scrollbar input](../concepts/events-and-input.md#scrollbar-input).

## ScrollHandle

Offsets are **positive** distances scrolled (unlike GPUI's negative offsets).

```text
func NewScrollHandle() *ScrollHandle
func (h *ScrollHandle) Offset() Point          // current offset
func (h *ScrollHandle) SetOffset(p Point)      // clamped at the next frame; refresh the window to see it
func (h *ScrollHandle) MaxOffset() Size        // scrollable extent as of the last frame
func (h *ScrollHandle) Bounds() Bounds         // container border box as of the last frame
func (h *ScrollHandle) ScrollToTop()           // next frame
func (h *ScrollHandle) ScrollToBottom()        // next frame
func (h *ScrollHandle) ScrollToItem(ix int)    // minimum scroll to fully reveal child ix of a Div (or row ix of a UniformList)
```

Bind it with `Div.TrackScroll` or `UniformList.TrackScroll`; one handle should
track one container at a time.

## Canvas

```text
func NewCanvas(paint func(bounds Bounds, w *Window, cx *App)) *Canvas
func (c *Canvas) IntoElement() Element
```

The port of GPUI's `canvas()`: a leaf that embeds `StyledMixin` (it fills its
parent by default; any size builder overrides that) and calls `paint` with its
laid-out bounds during the paint phase. Emit primitives from the callback with
the `Window.Paint*` methods. A canvas does not clip; push a clip in the
callback when the drawing may overflow. It has no hitbox of its own: wrap it in
a `Div` for mouse listeners.

## Custom elements

Implement `IntoElement` on your builder type and return a small adapter that
implements `Element` (needed because the fluent `ID(string)` and
`Element.ID()` cannot share a name). Container elements also implement
`ParentElementChildren`. The `Window` hooks custom elements use are listed in
[app-window.md](app-window.md#custom-element-and-painting-hooks); a complete
worked example is in the
[cookbook](../guides/cookbook.md#custom-element-with-children).

```text
type ParentElementChildren interface {
    Element
    ElementChildren() []AnyElement        // the children passed to RequestLayoutWithChildren, same order
}

type ChildLayout struct {
    Node *LayoutNode                      // valid from Prepaint on: Rect and ContentSize
    /* unexported per-child layout state */
}
// Return the *ChildLayout (or a struct embedding it) from RequestLayout.
```

### Layout engine types

```text
type LayoutID int

type LayoutNode struct {
    Style    Style
    Children []*LayoutNode
    Measure  func(text string, ts TextStyle, maxWidth float32) (w, h float32) // sizes a text leaf; maxWidth = math.MaxFloat32 for max-content, <= 0 for min-content
    Text     string
    Rect        Bounds                    // output: parent-relative border box
    ContentSize Size                      // output
}
func (n *LayoutNode) ScrollMax() (Pixels, Pixels)   // max(0, ContentSize - Rect.Size) per axis

func Layout(n *LayoutNode, available Available) Bounds
```

`Layout` sizes the tree and assigns every node its parent-relative border box;
the root is forced to `available` on each bounded axis whose length is not
`LengthPx`. The window calls it for you; call it directly only to unit-test
layout (see [testing](../guides/testing.md)).

### Scene primitives a custom `Paint` can emit

`Quad`, `Shadow`, `GlyphSprite`, `Underline` and `ContentMask` are described
in [platform.md](platform.md#scene-and-rasterizer); emit them with
`Window.PaintQuad`, `PaintShadow`, `PaintGlyph`, `PaintUnderline`, `PaintStroke`.
