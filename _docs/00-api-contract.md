# `gpui` API contract

This file is the **coordination contract** for the Go port of GPUI. Every file in
package `gpui` (and `gpui/ebitenplatform`) must conform to the signatures below.
If an implementation needs a signature to change, change it **here first**, then
in the code, so the other agents stay in sync.

Module: `go-gpui`. Core package `go-gpui/gpui` must depend only on the Go
standard library. `golang.org/x/image` and `github.com/go-text/typesetting` are
allowed **only** in `font.go` / `text.go`. `github.com/hajimehoshi/ebiten/v2`
is allowed **only** in `gpui/ebitenplatform/`.

Units: all geometry is in **logical pixels** (`float32`), origin top-left, +y down.
`Pixels` is a distinct float32 type used where GPUI distinguishes logical px.

---

## 1. `geometry.go`

```go
type Pixels float32
type Point struct{ X, Y Pixels }
type Size struct{ W, H Pixels }
type Bounds struct{ Origin Point; Size Size }
type Edges struct{ Top, Right, Bottom, Left Pixels }
type Corners struct{ TopLeft, TopRight, BottomLeft, BottomRight Pixels }

func (b Bounds) Min() Point
func (b Bounds) Max() Point
func (b Bounds) Width() Pixels
func (b Bounds) Height() Pixels
func (b Bounds) Contains(p Point) bool
func (b Bounds) Intersects(o Bounds) bool
func (b Bounds) Translate(d Point) Bounds
func (b Bounds) Inset(e Edges) Bounds
func (b Bounds) Union(o Bounds) Bounds
```

## 2. `color.go`

```go
type Rgba struct{ R, G, B, A uint8 } // straight (non-premultiplied)
type Hsla struct{ H, S, L, A float32 }

func RgbaOf(r, g, b, a uint8) Rgba
func ParseHex(s string) (Rgba, error)      // "#rrggbb", "#rrggbbaa", "#rgb"
func (c Rgba) Premultiply() Rgba           // returns premultiplied-encoded Rgba
func (c Rgba) ToHsla() Hsla
func (c Hsla) ToRgba() Rgba
func (c Rgba) Opacity(f float32) Rgba      // multiplies alpha by f

// Package-level common colors (straight alpha, a=255):
var (
    White, Black, Transparent Rgba
    Gray50, Gray100, Gray200, Gray300, Gray400, Gray500, Gray600, Gray700, Gray800, Gray900 Rgba
    Blue500, Blue600, Red500, Green600, Yellow400 Rgba
)
```

## 3. `style.go` + `styled.go`

```go
type LengthKind uint8
const (
    LengthAuto LengthKind = iota // content-based; also the automatic min size
    LengthPx
    LengthFlex                    // flex-basis:0 + flex-grow:Val (CSS `flex: Val`)
)
type Length struct {
    Kind LengthKind
    Val  Pixels
}
func Auto() Length
func Px(v float32) Length
func Flex(g float32) Length

type Direction uint8
const (DirectionColumn Direction = iota; DirectionRow)

type Justify uint8
const (
    JustifyStart Justify = iota
    JustifyCenter
    JustifyEnd
    JustifySpaceBetween
    JustifySpaceAround
)

type Align uint8
const (
    AlignStretch Align = iota
    AlignStart
    AlignCenter
    AlignEnd
)
const AlignSelfAuto Align = 255

type OverflowKind uint8
const (
    OverflowVisible OverflowKind = iota
    OverflowHidden
    OverflowScroll
)

// Style is the resolved, per-element style used by layout and paint.
type Style struct {
    Display  bool // false => hidden (Display:none), skipped entirely
    Direction Direction

    Width, Height             Length
    MinWidth, MinHeight       Length // LengthAuto => automatic minimum size
    MaxWidth, MaxHeight       Length // LengthAuto => no maximum

    Margin, Padding Edges
    Gap            Pixels // main-axis gutter between adjacent children

    FlexGrow   float32 // default 0
    FlexShrink float32 // default 1

    Justify   Justify
    AlignItems Align
    AlignSelf Align  // AlignSelfAuto means "inherit AlignItems"
    Overflow  OverflowKind

    BorderWidths Edges
    BorderColor  Rgba
    CornerRadii  Corners
    Background   Rgba // Transparent means "no background quad"

    TextStyle TextStyle
}

// StyleRefinement is the partial/fluent form; Refine(base) fills unset fields
// from base. All fields are pointers so "unset" is distinguishable.
type StyleRefinement struct {
    Display   *bool
    Direction *Direction
    Width, Height             *Length
    MinWidth, MinHeight       *Length
    MaxWidth, MaxHeight       *Length
    Margin, Padding           *Edges
    Gap                       *Pixels
    FlexGrow, FlexShrink      *float32
    Justify                   *Justify
    AlignItems                *Align
    AlignSelf                 *Align
    Overflow                  *OverflowKind
    BorderWidths              *Edges
    BorderColor               *Rgba
    CornerRadii               *Corners
    Background                *Rgba
    TextStyle                 *TextStyle
}
func (r *StyleRefinement) Refine(base Style) Style

type TextStyle struct {
    FontFamily  string // "" => default family
    FontSize    Pixels // default 14
    FontWeight  int    // 400 normal, 700 bold
    Color       Rgba   // default black
    LineHeight  Pixels // 0 => 1.2 * FontSize
    Strikethrough bool
}

func DefaultTextStyle() TextStyle
```

`styled.go` provides the fluent builder. `Styled` is implemented by `*Div` and
`*Text`. Every method returns the receiver for chaining.

```go
type Styled interface {
    StyleRef() *StyleRefinement
}

// Every one of these exists on both *Div and *Text (via a shared embedded struct
// `StyledMixin` that holds a StyleRefinement and the methods):
func (m *StyledMixin) StyleRef() *StyleRefinement
func (m *StyledMixin) SizeFull() *T                 // w=100%,h=100%  (Px 100% not modeled: use Flex(1) w/ parent)
func (m *StyledMixin) SizeWH(w, h float32) *T
func (m *StyledMixin) SizeW(w float32) *T
func (m *StyledMixin) SizeH(h float32) *T
func (m *StyledMixin) FlexGrow(v float32) *T
func (m *StyledMixin) FlexShrink(v float32) *T
func (m *StyledMixin) Flex1() *T                    // FlexGrow(1)
func (m *StyledMixin) Flex() *T                     // display:flex, default DirectionColumn
func (m *StyledMixin) FlexRow() *T                  // Direction=Row
func (m *StyledMixin) FlexCol() *T                  // Direction=Column
func (m *StyledMixin) Gap(v float32) *T
func (m *StyledMixin) P(v float32) *T
func (m *StyledMixin) Px_(v float32) *T             // padding-left  (named to avoid clash with Px())
func (m *StyledMixin) Py_(v float32) *T
func (m *StyledMixin) Pt(v float32) *T
func (m *StyledMixin) Pr(v float32) *T
func (m *StyledMixin) Pb(v float32) *T
func (m *StyledMixin) M(v float32) *T
func (m *StyledMixin) Bg(c Rgba) *T
func (m *StyledMixin) TextColor(c Rgba) *T
func (m *StyledMixin) TextSize(px float32) *T
func (m *StyledMixin) FontWeight(w int) *T
func (m *StyledMixin) TextStrikethrough() *T
func (m *StyledMixin) Rounded(px float32) *T
func (m *StyledMixin) RoundedAll(tl, tr, br, bl float32) *T
func (m *StyledMixin) Border(w float32, c Rgba) *T
func (m *StyledMixin) ItemsStart() *T
func (m *StyledMixin) ItemsCenter() *T
func (m *StyledMixin) ItemsEnd() *T
func (m *StyledMixin) ItemsStretch() *T
func (m *StyledMixin) JustifyStart() *T
func (m *StyledMixin) JustifyCenter() *T
func (m *StyledMixin) JustifyEnd() *T
func (m *StyledMixin) JustifyBetween() *T
func (m *StyledMixin) JustifyAround() *T
func (m *StyledMixin) OverflowHidden() *T
func (m *StyledMixin) OverflowScroll() *T
func (m *StyledMixin) MinW(v float32) *T
func (m *StyledMixin) MinH(v float32) *T
func (m *StyledMixin) MaxW(v float32) *T
func (m *StyledMixin) MaxH(v float32) *T
```

> **Note on `SizeFull()`.** CSS `100%` is not modeled. `SizeFull()` sets
> `Width=Flex(1)` and `Height=Flex(1)` when the parent is a flex container of the
> matching direction, and additionally sets `MinWidth/MinHeight` to `Px(0)` so the
> automatic minimum size does not block shrinking. Document this deviation.

## 4. `layout.go` — the flexbox engine

The engine is a plain function-based two-pass layout. It knows nothing about
elements, scenes or windows. Nodes are built by the element pipeline and
discarded after each frame.

```go
type Available struct {
    W, H      Pixels
    BoundedW  bool // false => W unbounded (max-content sizing)
    BoundedH  bool
}
func Definite(w, h float32) Available
func UnboundedHeight(availW float32) Available   // {W: availW, BoundedW:true, BoundedH:false}
func UnboundedWidth(availH float32) Available

type LayoutNode struct {
    Style    Style
    Children []*LayoutNode

    // Measure overrides leaf sizing. Only used when len(Children)==0.
    // maxWidth is the wrap width (math.MaxFloat32 for max-content).
    Measure func(text string, ts TextStyle, maxWidth float32) (w, h float32)
    Text    string // non-empty + Measure != nil => leaf sized by Measure

    // Outputs, written by Layout:
    Rect        Bounds // border box, parent-relative
    ContentSize Size   // scrollable extent from the padding-box origin (>= Rect size)
}

// Layout runs measure+arrange. available is the constraint for the root node.
func Layout(n *LayoutNode, available Available) Bounds

// ScrollMax returns max(0, ContentSize - Rect size) per axis.
func (n *LayoutNode) ScrollMax() (w, h Pixels)
```

Semantics (from the CSS flexbox spec + Taffy's `flexbox.rs`):

- One flex line, LTR, no wrap, no margins.
- `Gap` is a fixed gutter: `gapsTotal = Gap*(k-1)`, always subtracted before
  `justify-content` sees free space, always re-added between items.
- Flex base size: `LengthPx`→val; `LengthFlex(g)`→base 0, grow g, shrink 1;
  `LengthAuto`→measure under (definite-or-unbounded main, definite cross).
- Automatic minimum size (`Min*=LengthAuto`): **0 if the node is a scroll
  container** (`Overflow != Visible`), else its min-content size in that axis
  (measured with that axis unbounded, the other axis bounded), capped by a
  definite `Max*`.
- Hypothetical main size = clamp(base, min, max), floored at 0.
- Resolve flexible lengths per CSS §9.7 (freeze inflexible items, iterate,
  fix min/max violations, freeze violators, repeat until all frozen). If the
  container's used inner main size is indefinite (unbounded main) **skip the
  loop entirely**: every item keeps its hypothetical size.
- Cross size: if the container's cross is definite, `lineCross = innerCross`;
  else `lineCross = max_i(hypCross)`. `align-items: stretch` applies only when
  the child's cross `Length` is `LengthAuto`; it sets the child's cross size to
  `lineCross` (clamped) and **re-measures** the child at that definite cross.
- `justify-content` offsets with the fallback: if `k<=1 || free<=0`, then
  `SpaceBetween→Start` and `SpaceAround→Center`.
- `Overflow` never changes descendant layout. It only affects (a) the automatic
  minimum size rule, (b) clipping, (c) content-size propagation.
- Scroll container: children measured with **unbounded main, definite cross**.
  `ContentSize.main = max(usedMain, max_i(childEndAlongMain)+endPaddingMain)`,
  and the same on the cross axis. Non-scroll children with `OverflowVisible`
  propagate their own `ContentSize` upward.
- Text leaf: `wrapWidth =` the `LengthPx` width if set, else the available main
  or cross width if that axis is bounded, else `math.MaxFloat32` (max-content).

## 5. `element.go`, `div.go`, `text_elem.go`

```go
// Element is the per-frame UI node. Everything is rebuilt each frame.
type Element interface {
    ID() (ElementID, bool)
    RequestLayout(w *Window, cx *App) (LayoutID, any)
    Prepaint(w *Window, cx *App, id ElementID, bounds Bounds, layoutState any) any
    Paint(w *Window, cx *App, id ElementID, bounds Bounds, layoutState, prepaintState any)
}

type IntoElement interface{ IntoElement() Element }

type ParentElement interface {
    Child(children ...IntoElement) ParentElement
    Children(children ...IntoElement) ParentElement
}

// ElementID identifies an element across frames. Stable ids keep element state
// (scroll offsets, hover) alive; missing ids fall back to the child index path.
type ElementID struct {
    Path []ElementPathKey
}
type ElementPathKey struct {
    Index  int
    Key    string // "" when keyed by index
}

// GlobalElementID is the fully-resolved id used as a map key for retained state.
type GlobalElementID struct {
    Path []ElementPathKey
}
func (a GlobalElementID) Equal(b GlobalElementID) bool

// LayoutID indexes into the Window's per-frame layout engine.
type LayoutID int

// AnyElement erases an Element for the children slice.
type AnyElement struct{ elem Element }
func NewAnyElement(e IntoElement) AnyElement
func (a AnyElement) Element() Element

// Normalize converts anything IntoElement into an Element.
func IntoElementOf(e IntoElement) Element
```

### `div.go`

```go
func Div() *Div

type Div struct {
    StyledMixin
    id         string
    children   []AnyElement
    // listeners (all optional, nil = none)
    onMouseDown  func(*MouseDownEvent, *Window, *App)
    onMouseUp    func(*MouseUpEvent, *Window, *App)
    onMouseMove  func(*MouseMoveEvent, *Window, *App)
    onClick      func(*ClickEvent, *Window, *App)
    onScrollWheel func(*ScrollWheelEvent, *Window, *App)
    onKeyDown    func(*KeyDownEvent, *Window, *App)
    onKeyUp      func(*KeyUpEvent, *Window, *App)
    onHoverStart func(*Window, *App)
    onHoverEnd   func(*Window, *App)
    onAction     map[reflect.Type]func(Action, *Window, *App)
    focusHandle  *FocusHandle
    keyContext   string
    tracksFocus  bool
    tabIndex     int
}

func (d *Div) ID(id string) *Div
func (d *Div) Child(children ...IntoElement) *Div
func (d *Div) Children(children ...IntoElement) *Div
func (d *Div) OnMouseDown(f func(*MouseDownEvent, *Window, *App)) *Div
func (d *Div) OnMouseUp(f func(*MouseUpEvent, *Window, *App)) *Div
func (d *Div) OnMouseMove(f func(*MouseMoveEvent, *Window, *App)) *Div
func (d *Div) OnClick(f func(*ClickEvent, *Window, *App)) *Div
func (d *Div) OnScrollWheel(f func(*ScrollWheelEvent, *Window, *App)) *Div
func (d *Div) OnKeyDown(f func(*KeyDownEvent, *Window, *App)) *Div
func (d *Div) OnKeyUp(f func(*KeyUpEvent, *Window, *App)) *Div
func (d *Div) OnHoverStart(f func(*Window, *App)) *Div
func (d *Div) OnHoverEnd(f func(*Window, *App)) *Div
func (d *Div) OnAction(action Action, f func(Action, *Window, *App)) *Div
func (d *Div) TrackFocus(h *FocusHandle) *Div
func (d *Div) KeyContext(ctx string) *Div
func (d *Div) TabIndex(i int) *Div
```

### `text_elem.go`

```go
// Text is a leaf that renders a single run of styled text.
type Text struct {
    StyledMixin
    text string
}
// NewText is named NewText rather than Text because Go does not allow a
// type and a package-level function to share a name.
func NewText(s string) *Text
func (t *Text) Text(s string) *Text

// StyledText renders multiple runs of differing styles within one paragraph.
type StyledText struct {
    StyledMixin
    runs []TextRun
}
type TextRun struct {
    Text  string
    Style TextStyle
}
func NewStyledText(runs ...TextRun) *StyledText
```

Both implement `Element`. `Text` sizes itself by calling the window's text
shaper (`w.shapeLine` / `w.measureText`) with the wrap width supplied by the
layout constraint. Painting emits `GlyphSprite`s into the scene.

Also provide `func (s String) IntoElement() Element` equivalent: a helper
`func Str(s string) IntoElement` returning a `*Text` so `Child(Str("hi"))` works.

## 6. `scene.go`, `raster.go`, `bounds.go`

```go
type ContentMask struct{ Bounds Bounds } // clip rect; infinite = whole window

type BackgroundKind uint8
const (
    BackgroundNone BackgroundKind = iota
    BackgroundSolid
)

type Quad struct {
    Order         uint32
    Bounds        Bounds
    ContentMask   ContentMask
    Background    BackgroundKind
    Fill          Rgba
    BorderColor   Rgba
    BorderWidths  Edges
    CornerRadii   Corners
}

type GlyphSprite struct {
    Order       uint32
    Bounds      Bounds      // destination rect in window space
    ContentMask ContentMask
    Mask        *GlyphMask  // grayscale alpha mask
    Color       Rgba
}

type GlyphMask struct {
    Pix  []uint8 // 1 byte per pixel, alpha only, row-major, Stride = W
    W, H int
    // Ink bounds in font pixels (y-up) relative to the pen position. The
    // mask covers exactly this rect expanded by one pixel on every side,
    // screen-oriented: row 0 is the top of the ink. GlyphSprite.Bounds is
    // derived from these at paint time.
    MinX, MinY, MaxX, MaxY float32
}

type Underline struct {
    Order       uint32
    Bounds      Bounds
    ContentMask ContentMask
    Color       Rgba
    Thickness   Pixels
}

type Scene struct {
    Quads   []Quad
    Sprites []GlyphSprite
    Underlines []Underline
}

func (s *Scene) Clear()
func (s *Scene) InsertQuad(q Quad)
func (s *Scene) InsertSprite(g GlyphSprite)
func (s *Scene) InsertUnderline(u Underline)
// PushLayer records a z-order boundary; all subsequently inserted primitives
// get their Order bumped. Called during prepaint for stacking contexts.
func (s *Scene) PushLayer()
```

### `raster.go`

```go
// Rasterizer paints a Scene into an RGBA buffer.
type Rasterizer struct {
    pix   []byte // premultiplied RGBA, 4*W*H
    w, h  int
}
func NewRasterizer(w, h int) *Rasterizer
func (r *Rasterizer) Size() (w, h int)
func (r *Rasterizer) Pixels() []byte            // premultiplied, tightly packed
func (r *Rasterizer) Reset(clear Rgba)          // fill with premultiplied clear color
func (r *Rasterizer) Render(s *Scene)           // draw all primitives in Order
func (r *Rasterizer) Image() *image.RGBA        // non-premultiplied view (for png.Encode)
```

Clipping: each primitive is intersected with its `ContentMask.Bounds` and drawn
with that clip rect. Corner radii are drawn by an anti-aliased rounded-rect
mask; borders are drawn as four strips (or as a rounded-rect stroke). Text is
drawn by alpha-blending the `GlyphMask` scaled to `GlyphSprite.Bounds` with
bilinear sampling.

`raster.go` may import `image`, `image/draw`, `image/color`, `math`, but **not**
`golang.org/x/*` or ebiten.

### `bounds.go`

```go
// Hitbox is a hit-testable region registered during prepaint.
type Hitbox struct {
    Bounds  Bounds
    Order   uint32 // paint order; higher = later = on top
    // listeners index into the frame's listener arrays (nil = none)
    hasMouseListeners bool
}

type BoundsTree struct {
    boxes []Hitbox
}
func (t *BoundsTree) Clear()
func (t *BoundsTree) Insert(h Hitbox) int
// HitTest returns the index of the deepest, topmost box containing p, or -1.
func (t *BoundsTree) HitTest(p Point) int
func (t *BoundsTree) Len() int
```

Hit test order: among boxes containing `p`, prefer the one with the highest
`Order`; ties broken by the later insert.

## 7. `events.go`

```go
type DispatchPhase uint8
const (
    Capture DispatchPhase = iota
    Bubble
)

type Modifiers uint8
const (
    Shift   Modifiers = 1 << iota
    Control
    Alt
    Platform // Cmd on macOS, Ctrl on Windows/Linux
    Function
)

type MouseButton uint8
const (
    MouseLeft MouseButton = iota
    MouseMiddle
    MouseRight
)

type MouseDownEvent struct {
    Position Point
    Button   MouseButton
    Modifiers Modifiers
    ClickCount int
}
type MouseUpEvent struct {
    Position Point
    Button   MouseButton
    Modifiers Modifiers
    ClickCount int
}
type MouseMoveEvent struct {
    Position Point
    Modifiers Modifiers
}
type ScrollWheelEvent struct {
    Position Point
    DeltaX, DeltaY Pixels
    Modifiers Modifiers
}
type ClickEvent struct {
    Up *MouseUpEvent
}
type KeyDownEvent struct {
    Keystroke Keystroke
    // PreferCharacterInput is true for events that should go to a focused text
    // input rather than to keybindings (i.e. printable chars).
    PreferCharacterInput bool
}
type KeyUpEvent struct {
    Keystroke Keystroke
}
type ModifiersChangedEvent struct{ Modifiers Modifiers }

// Keystroke mirrors gpui's platform/keystroke.rs.
// Key is a lower-case symbolic name: "enter", "escape", "backspace", "a", "1",
// "left", "up", "space", "tab", "delete", ...
// KeyChar holds the printable character when the key produces one.
type Keystroke struct {
    Modifiers Modifiers
    Key       string
    KeyChar   *string
}
func ParseKeystroke(s string) (Keystroke, error) // "cmd-shift-p", "enter", "ctrl-a"
func (k Keystroke) String() string
func (k Keystroke) Equals(o Keystroke) bool

// PlatformInput is any of the events above; the dispatcher type-switches.
type PlatformInput interface{ isPlatformInput() }
```

All event structs get `func (...) isPlatformInput() {}`.

## 8. `keymap.go`

```go
// Action is any named, cloneable, comparable command.
type Action interface {
    Name() string
    Clone() Action
    Equals(other Action) bool
}

// NewAction builds a simple named action (a value type for the POC).
func NewAction(name string) Action

type KeyBinding struct {
    Action    Action
    Keystrokes []Keystroke
    Context   string // "" => matches any context; supports "&&", "||", "!" of
                     // simple key=value tokens and bare keys
}

func NewKeyBinding(keystrokeStr string, action Action, context string) KeyBinding

type Keymap struct {
    bindings []KeyBinding
}
func NewKeymap() *Keymap
func (k *Keymap) AddBindings(bindings ...KeyBinding)
// MatchKeystroke finds the binding whose last keystroke equals typed and whose
// context predicate is satisfied by any entry in ctxStack. Returns nil if none.
func (k *Keymap) MatchKeystroke(typed Keystroke, ctxStack []string) Action

// KeyContext is a predicate. "editor && !readonly" is parsed into tokens.
type keyContextPredicate struct{ /* ... */ }
func parseKeyContext(s string) *keyContextPredicate
func (p *keyContextPredicate) matches(ctxStack []string) bool
```

`App` owns the keymap: `cx.Keymap().AddBindings(...)`, `cx.Keymap()` returns
`*Keymap`.

## 9. `focus.go`

```go
type FocusID uint64

// FocusHandle is a refcounted, copyable handle to a focusable element.
// The zero value is invalid; obtain one from App.NewFocusHandle.
type FocusHandle struct {
    id       FocusID
    app      *App
    tabIndex int
    tabStop  bool
}

func (h *FocusHandle) ID() FocusID
func (h *FocusHandle) IsValid() bool
func (h *FocusHandle) Focus(w *Window)
func (h *FocusHandle) Blur(w *Window)
func (h *FocusHandle) IsFocused(w *Window) bool
func (h *FocusHandle) ContainsFocused(w *Window) bool
func (h *FocusHandle) SetTabIndex(i int)
func (h *FocusHandle) SetTabStop(b bool)
```

`App` holds a `focusMap` from `FocusID` to a refcount. Dropping the last handle
releases the id during `flushEffects`. `Window` stores `focus FocusID` (0 =
none). `App.NewFocusHandle() *FocusHandle` allocates.

## 10. `input.go` — the textbox plumbing

```go
// IntRange is a half-open [Start, End) range of UTF-16 code unit offsets.
type IntRange struct{ Start, End int }

type UTF16Selection struct {
    Range        IntRange
    Reversed     bool
    IsNil        bool
}

// PlatformInputHandler is what a focused text field must provide. This is the
// trimmed-down gpui::EntityInputHandler.
type PlatformInputHandler interface {
    // TextForRange returns the text in the given UTF-16 range.
    TextForRange(rng IntRange) (string, bool)
    SelectedTextRange() (UTF16Selection, bool)
    MarkedTextRange() (IntRange, bool)
    UnmarkText()
    // ReplaceTextInRange replaces the given range (nil range = insert at
    // caret) with text and moves the caret to the end of the inserted text.
    ReplaceTextInRange(rng *IntRange, text string)
    BoundsForRange(rng IntRange, elementBounds Bounds) (Bounds, bool)
    CharacterIndexForPoint(p Point, elementBounds Bounds) (int, bool)
    SetSelectedTextRange(rng IntRange)
    TextLengthUTF16() (int, bool)
    AcceptsTextInput() bool
}

// TextInput is a ready-to-use single-line text field view.
type TextInput struct {
    // unexported
}
func NewTextInput(placeholder string) *TextInput
// Text returns the current value.
func (t *TextInput) Text() string
func (t *TextInput) SetText(s string)
func (t *TextInput) FocusHandle() *FocusHandle
// Render returns the element to render; it emits TextSubmitted on Enter. It
// returns IntoElement (not Element) so it composes directly inside Child.
func (t *TextInput) Render(w *Window, cx *App) IntoElement
func (t *TextInput) OnSubmit(f func(string, *Window, *App)) *TextInput
func (t *TextInput) OnChange(f func(string, *Window, *App)) *TextInput
```

`TextInput` implements `PlatformInputHandler` internally. It is **not** an
`Entity[T]` for the POC — it is a plain struct the root view owns by value, so
`cx.Notify()` on the owning entity re-renders it. Document this simplification
relative to GPUI's entity-per-textbox design.

## 11. `entity.go`, `context.go`, `app.go`, `subscription.go`

```go
// EntityID mirrors gpui's slotmap key: index + generation.
type EntityID struct {
    index      uint32
    generation uint32
}
func (id EntityID) IsNil() bool
func (id EntityID) String() string

// Entity is a cheap, copyable handle to state stored in the App.
type Entity[T any] struct {
    id  EntityID
    app *App
}
func NewEntity[T any](cx *App, build func(cx *Context[T]) T) Entity[T]
func (e Entity[T]) ID() EntityID
func (e Entity[T]) Read(cx *App) *T
func (e Entity[T]) Update(cx *App, f func(state *T, cx *Context[T]))
func (e Entity[T]) Downgrade() WeakEntity[T]
func (e Entity[T]) Notify(cx *App)
func (e Entity[T]) SubscribeEvent[E any](cx *App, f func(ev *E)) *Subscription

type WeakEntity[T any] struct {
    id  EntityID
    app *App
}
func (w WeakEntity[T]) Upgrade() (Entity[T], bool)

// Context is the per-entity view handed to Render and Update.
type Context[T any] struct {
    App   *App
    entity EntityID
}
func (c *Context[T]) Notify()
func (c *Context[T]) Emit[E any](event *E)
func (c *Context[T]) Entity() Entity[T]
func (c *Context[T]) WeakEntity() WeakEntity[T]
func (c *Context[T]) Observe(other Entity[T], f func(Entity[T], *App)) *Subscription
func (c *Context[T]) Subscribe(other Entity[T], f func(*E)) *Subscription  // generic over E
func (c *Context[T]) Spawn(f func() ) *Task
func (c *Context[T]) Defer(f func(*App))
func (c *Context[T]) Listener[E any](f func(state *T, ev *E, w *Window, cx *App)) func(*E, *Window, *App)

type Task struct{ /* ... */ }
func (t *Task) Cancel()
func (t *Task) Detach()
```

```go
type App struct {
    // unexported
}
func NewApp() *App
func (a *App) Update(f func(*App))   // reentrancy-guarded; outermost runs flushEffects
func (a *App) Quit()
func (a *App) IsQuitting() bool
func (a *App) OpenWindow(o WindowOptions, build func(*Window, *App) EntityRoot) error
func (a *App) Windows() []*Window
func (a *App) Notify(id EntityID)
func (a *App) Defer(f func(*App))
func (a *App) Observe[T any](e Entity[T], f func(Entity[T], *App)) *Subscription
func (a *App) Subscribe[T any, E any](e Entity[T], f func(Entity[T], *E)) *Subscription
func (a *App) SetGlobal(v any)
func (a *App) Global(out any) bool
func (a *App) UpdateGlobal(f func(any))
func (a *App) Keymap() *Keymap
func (a *App) NewFocusHandle() *FocusHandle
func (a *App) Platform() Platform
func (a *App) flushEffects()   // unexported
```

`EntityRoot` is the root-view interface a window renders:

```go
// EntityRoot is implemented by any view type via the Render method.
type EntityRoot interface {
    Render(w *Window, cx *App) Element
}
```

Provide a helper so plain functions work as roots:

```go
type RenderFunc func(w *Window, cx *App) Element
func (f RenderFunc) Render(w *Window, cx *App) Element { return f(w, cx) }
```

Effects (unexported enum):

```go
type effectKind uint8
const (
    effectNotify effectKind = iota
    effectEmit
    effectDefer
    effectRefreshWindows
)
type effect struct {
    kind     effectKind
    entity   EntityID
    handler  func(*App)
    event    any
    eventType reflect.Type
}
```

`flushEffects` drains `pendingEffects` in a loop. On `effectNotify` it runs the
observers registered for that `EntityID`. After the queue empties it calls
`scheduleFrame()` on every window whose `invalid` flag is set. Nested
`Update` calls only flush at the outermost (`pendingUpdates` counter).

`Subscription`:

```go
type Subscription struct {
    close func()
}
func (s *Subscription) Close()
func (s *Subscription) IsClosed() bool
```

## 12. `window.go`

```go
type WindowOptions struct {
    Width  float32 // logical px; default 800
    Height float32 // logical px; default 600
    Title  string
    Resizable bool
}

type Window struct {
    // unexported: platform window, renderedFrame, nextFrame, layoutEngine,
    // focus, mouse position, invalid flag, root EntityRoot, scale factor,
    // text shaper cache, element state map, scroll offsets.
}
func (w *Window) Bounds() Bounds          // in logical px
func (w *Window) ScaleFactor() float32
func (w *Window) Refresh()                // mark dirty, request frame
func (w *Window) RequestFocus(h *FocusHandle)
func (w *Window) Focus() *FocusHandle
func (w *Window) Blur()
func (w *Window) StopPropagation()
func (w *Window) HandleInput(h PlatformInputHandler)
func (w *Window) InsertHitbox(b Bounds) int
func (w *Window) PaintQuad(q Quad)
func (w *Window) PaintUnderline(u Underline)
func (w *Window) PaintGlyph(g GlyphSprite)
func (w *Window) PushLayer()
func (w *Window) ElementState(id GlobalElementID, out any) bool   // retained state lookup
func (w *Window) SetElementState(id GlobalElementID, v any)
func (w *Window) DispatchEvent(input PlatformInput, cx *App)
func (w *Window) Draw(cx *App)            // full pipeline; returns nothing
func (w *Window) Scene() *Scene
func (w *Window) SetBackground(c Rgba)   // clear color for rasterization; default opaque white
func (w *Window) HitTest(p Point) (idx int, ok bool) // topmost hitbox of the rendered frame; idx stable until next Draw
```

`Frame` (unexported fields, but exposed for tests via accessors):

```go
type Frame struct {
    scene       Scene
    hitboxes    BoundsTree
    // mouseListeners[i] pairs with hitboxes[i]
    mouseListeners []mouseListener
    keyNodes       []keyNode
    inputHandlers  []PlatformInputHandler
    elementStates  map[GlobalElementID]any
    order          uint32
}
type mouseListener struct {
    onMouseDown, onMouseUp, onMouseMove func(any, *Window, *App)
    onClick      func(*ClickEvent, *Window, *App)
    onScrollWheel func(*ScrollWheelEvent, *Window, *App)
    keyContext   string
}
type keyNode struct {
    keyContext string
    focusID    FocusID
    onKeyDown  func(*KeyDownEvent, *Window, *App)
    onKeyUp    func(*KeyUpEvent, *Window, *App)
    onAction   map[reflect.Type]func(Action, *Window, *App)
}
func (f *Frame) SceneRef() *Scene
func (f *Frame) HitboxCount() int
func (f *Frame) HitTest(p Point) int
```

### The draw pipeline (must follow this order)

1. `w.nextFrame.clear()`
2. `root.Render(w, cx)` → `Element` tree.
3. Build a `LayoutNode` tree: recursively `RequestLayout` each element, collect
   child `LayoutID`s, call the layout engine with the window's available size.
4. Walk the tree assigning final `Bounds` (root at origin, window size).
5. `Prepaint` each element in tree order: registers hitboxes, listeners, scroll
   state, input handlers; returns per-element state.
6. `Paint` each element in tree order: emits quads/glyphs/underlines with the
   scene order incremented per primitive.
7. Swap `nextFrame` into `renderedFrame`.
8. If there is a `PlatformWindow`: re-read its `ScaleFactor()`, scale the scene
   from logical to device px, rasterize (cleared to the background color) and
   call `Present(pix, round(w*scale), round(h*scale))`; then call
   `SetInputHandler(focusedInputHandler())` (nil when none is focused).

`Prepaint` is where `Interactivity` decides hover/active/focus style refinements
and where the content mask (for `Overflow`) is pushed onto a stack on the window.

## 13. `platform.go`

```go
// Platform is the OS/backend abstraction. Implemented by
// gpui/ebitenplatform and by the headless test platform.
type Platform interface {
    // Run blocks until Quit. onLaunch is called once on the main thread
    // before the first frame.
    Run(onLaunch func(*App)) error
    OpenWindow(o WindowOptions) (PlatformWindow, error)
    Quit()
    Name() string // "macos", "windows", "linux", "headless"
}

// PlatformWindow is one OS window. All methods are called on the main thread.
type PlatformWindow interface {
    Bounds() (w, h Pixels)      // logical px
    ScaleFactor() float32
    RequestFrame()              // ask the platform to draw soon
    Present(pix []byte, w, h int) // premultiplied RGBA, 4*w*h
    // InputHandler is installed/removed as focus changes.
    SetInputHandler(h PlatformInputHandler)
    TakeInputHandler() PlatformInputHandler
    Quit()
}

// RequestFrameCallback is registered by the Window on its PlatformWindow.
type FrameRequester interface {
    OnRequestFrame(f func())
}
```

The `App` wires `platform.OpenWindow` → creates a `Window` → calls
`window.RequestFrame()` when dirty → the platform calls back into
`Window.DispatchEvent` / `Window.Draw`.

## 14. `headless.go` (in package `gpui`, no external deps)

```go
// RenderToPNG builds root, lays it out at w x h, paints, rasterizes, and
// writes a PNG to path. Used by visual tests and the todo app's -png flag.
func RenderToPNG(path string, w, h int, scale float32, root EntityRoot, opts ...RenderOption) error

type RenderOption func(*renderConfig)
func WithBackgroundColor(c Rgba) RenderOption
func WithKeymap(k *Keymap) RenderOption
func WithTitle(t string) RenderOption

// RenderToImage is the same but returns the image.
func RenderToImage(w, h int, scale float32, root EntityRoot, opts ...RenderOption) (*image.RGBA, error)

// HeadlessPlatform implements Platform with no window; used by RenderToPNG
// and by tests that need to drive frames and dispatch synthetic input.
type HeadlessPlatform struct{ /* ... */ }
func NewHeadlessPlatform() *HeadlessPlatform
func (p *HeadlessPlatform) Run(onLaunch func(*App)) error
func (p *HeadlessPlatform) OpenWindow(o WindowOptions) (PlatformWindow, error)
func (p *HeadlessPlatform) Quit()
func (p *HeadlessPlatform) Name() string

// HeadlessWindow is the PlatformWindow returned by HeadlessPlatform; it keeps
// the last presented pixels and exposes helpers for tests.
type HeadlessWindow struct{ /* ... */ }
func (w *HeadlessWindow) Pixels() []byte
func (w *HeadlessWindow) Image() *image.RGBA
func (w *HeadlessWindow) SavePNG(path string) error
func (w *HeadlessWindow) Dispatch(input PlatformInput)  // feed synthetic input, then draw if invalidated

// Additions (implemented):
func (p *HeadlessPlatform) SetScaleFactor(s float32)     // scale of windows opened afterwards
func (p *HeadlessPlatform) SetBackgroundColor(c Rgba)    // clear color of windows opened afterwards
func (p *HeadlessPlatform) Windows() []*HeadlessWindow
func (w *HeadlessWindow) Window() *Window
```

Semantics: `w`,`h` are logical px; the output image is `round(w*scale) x
round(h*scale)` device px. Default background is opaque white. HeadlessWindow
draws lazily: `Pixels()` /
`Image()` / `Dispatch` draw if the window is invalid; `Window.Draw` rasterizes
and `Present`s to the HeadlessWindow, which keeps the pixels. `HeadlessWindow` implements
no `FrameRequester`; `RequestFrame` only sets a pending flag, so redraws never
re-enter an in-flight event dispatch. A text measurer is installed
automatically when `DefaultFontPath() != ""`.

`RenderToPNG` uses `image/png` from the standard library.

## 15. `font.go` + `text.go`

```go
// Font is an opaque loaded font face.
type Font struct{ /* ... */ }

// FontManager loads and caches fonts, and resolves the default family.
type FontManager struct{ /* ... */ }
func NewFontManager() *FontManager
// DefaultFontPath resolves the platform's default sans font. Honors the
// GPUI_FONT environment variable. Candidate lists per runtime.GOOS.
func DefaultFontPath() string
func (m *FontManager) Face(family string, size float32, weight int) (*Font, error)
func (m *FontManager) DefaultFace(size float32, weight int) (*Font, error)

// Shaper shapes text into positioned glyphs.
type Shaper struct{ /* ... */ }
func NewShaper(m *FontManager) *Shaper

// ShapedLine is the result of shaping one line of text.
type ShapedLine struct {
    Width  Pixels
    Ascent Pixels
    Descent Pixels
    Glyphs []ShapedGlyph
}
type ShapedGlyph struct {
    GlyphID uint32
    X, Y    Pixels   // baseline-relative pen position
    Advance Pixels
    Cluster int      // byte offset into the source string
}

// ShapeLine shapes text at a given size. It does not wrap.
func (s *Shaper) ShapeLine(text string, face *Font) ShapedLine
// ShapeWrap shapes text, wrapping at wrapWidth (math.MaxFloat32 = no wrap),
// returning one ShapedLine per visual line.
func (s *Shaper) ShapeWrap(text string, face *Font, wrapWidth float32) []ShapedLine
// Measure returns the natural (w,h) of wrapped text.
func (s *Shaper) Measure(text string, face *Font, wrapWidth float32) (w, h Pixels)
// GlyphMask rasterizes a single glyph into a grayscale mask at the given size.
func (s *Shaper) GlyphMask(face *Font, glyphID uint32, size float32) *GlyphMask
```

`font.go`/`text.go` may import `github.com/go-text/typesetting/...` and
`golang.org/x/image/font/sfnt`, `golang.org/x/image/vector`, `golang.org/x/image/math/fixed`.

Default font resolution order (first existing path wins), overridable with the
`GPUI_FONT` env var:

- darwin: `/System/Library/Fonts/SFNS.ttf`, `/System/Library/Fonts/Helvetica.ttc`,
  `/Library/Fonts/Arial.ttf`
- windows: `%WINDIR%\Fonts\segoeui.ttf`, `arial.ttf`, `seguisb.ttf`
- linux: `/usr/share/fonts/truetype/dejavu/DejaVuSans.ttf`,
  `/usr/share/fonts/truetype/liberation/LiberationSans-Regular.ttf`,
  `/usr/share/fonts/TTF/DejaVuSans.ttf`, `~/.fonts/DejaVuSans.ttf`

## 16. `clipboard.go`

```go
// Clipboard reads/writes the system clipboard via OS tools.
func ClipboardRead() (string, error)
func ClipboardWrite(s string) error
// ClipboardAvailable reports whether an OS clipboard tool was found.
func ClipboardAvailable() bool
```

- darwin: `pbpaste` / `pbcopy`
- windows: `powershell -NoProfile -Command Get-Clipboard` /
  `Set-Clipboard -Value ...` (pass via stdin to avoid quoting bugs)
- linux: prefer `wl-paste`/`wl-copy`, fall back to `xclip`, then `xsel`

## 17. `gpui/ebitenplatform/`

```go
package ebitenplatform

import "go-gpui/gpui"

type Platform struct{ /* ... */ }
func New() gpui.Platform     // gpui.Platform is an interface
```

Implements `gpui.Platform`:

- `Run(onLaunch)` → `ebiten.RunGame(g)` where `g` implements
  `Update() error`, `Draw(screen *ebiten.Image)`, `Layout(ow, oh int) (int, int)`.
- `Layout` returns **device pixels** (`outside * Monitor().DeviceScaleFactor()`)
  so the rasterizer and `CursorPosition` share one coordinate space.
- `Update` polls input (see below) and forwards to the window's
  `DispatchEvent`, wrapped in `app.Update`.
- `Draw` calls `screen.WritePixels(pix)` with premultiplied RGBA; skips the
  upload when not dirty. `ebiten.SetScreenClearedEveryFrame(false)` at startup.
- `Present(pix, w, h)` stores the buffer for the next `Draw`.
- `RequestFrame` sets a dirty flag that `Update`/`Draw` observe; the Window
  calls it from `flushEffects`.
- Clean quit: return `ebiten.Termination` from `Update` when `app.IsQuitting()`.
- Window title/size: `ebiten.SetWindowTitle`, `ebiten.SetWindowSize` (DIP).
- `ebiten.SetWindowResizingMode(ebiten.WindowResizingModeEnabled)`.

Input mapping in `Update`:

- `inpututil.IsMouseButtonJustPressed` → `MouseDownEvent`; `JustReleased` →
  `MouseUpEvent` + a `ClickEvent` if `Window.HitTest` yields the same hitbox
  index at press and release;
  `ebiten.CursorPosition()` → `Position` each tick, synthesized into
  `MouseMoveEvent` when it changed.
- `ebiten.Wheel()` → `ScrollWheelEvent` (note: ebiten's wheel y is typically
  inverted relative to scroll-down-positive; normalize so positive `DeltaY`
  means scroll down / content moves up).
- `ebiten.AppendInputChars` → `KeyDownEvent{PreferCharacterInput:true}` per rune.
- Special keys via `inpututil.IsKeyJustPressed` (only valid in `Update`):
  `KeyEnter→"enter"`, `KeyEscape→"escape"`, `KeyBackspace`, `KeyDelete`,
  `KeyLeft/Right/Up/Down`, `KeySpace→"space"`, `KeyTab→"tab"`, letters/digits.
- Modifiers: `ebiten.IsKeyPressed(ebiten.KeyMeta)` → `ModPlatform` (Cmd on macOS);
  on Windows/Linux map `KeyControl` → `ModPlatform` and `KeyMeta` → `Control`
  (so Cmd/Ctrl behave the same for the app author). Detect this with
  `runtime.GOOS`. `KeyShift`, `KeyAlt`.
- Clipboard via `gpui.ClipboardRead/Write` for cmd/ctrl+c/v/x.

## 18. `examples/todo/`

```go
package main

type Todo struct {
    Title string
    Done  bool
}
type TodoApp struct {
    todos   []Todo
    input   *gpui.TextInput
    nextID  int
}
func NewTodoApp() *gpui.TodoApp  // or just construct in main
func (t *TodoApp) Render(w *gpui.Window, cx *gpui.App) gpui.Element
```

`Render` returns:

```
Div().FlexCol().SizeFull().Gap(12).P(16).Bg(Gray50)
  ├─ Text("Todos").TextSize(24).FontWeight(700)
  ├─ Div().FlexRow().Gap(8).ItemsCenter()
  │    ├─ input.Render(w, cx)      // Flex(1)
  │    └─ Div().ID("add").OnClick(add).P(8).Bg(Blue500).Rounded(6).Child(Text("Add").TextColor(White))
  └─ Div().ID("list").Flex(1).OverflowScroll().FlexCol().Gap(4)
       └─ per todo: Div().ID(fmt.Sprintf("todo-%d", i)).FlexRow().ItemsCenter().Gap(8).P(8).Bg(White).Rounded(6)
             .OnClick(toggle)
             ├─ Text(done? "☑":"☐")  (or a small quad indicator)
             └─ Text(title).TextStrikethrough(if done)
```

`main`:

```go
func main() {
    flags: -png PATH, -width, -height, -smoke MS
    if *png != "" { gpui.RenderToPNG(...); return }
    p := ebitenplatform.New()
    app := gpui.NewAppWithPlatform(p)
    app.Keymap().AddBindings(
        gpui.NewKeyBinding("enter", gpui.NewAction("add-todo"), "textbox"),
        gpui.NewKeyBinding("escape", gpui.NewAction("blur"), "textbox"),
    )
    app.Run(func(cx *gpui.App) {
        cx.OpenWindow(opts, func(w *gpui.Window, cx *gpui.App) gpui.EntityRoot { return todoApp })
    })
}
```

---

## Conventions every agent must follow

1. **Package `gpui` imports only stdlib** (+ `x/image` and `go-text/typesetting`
   in `font.go`/`text.go`). No ebiten in `gpui`.
2. **No comments unless the code is genuinely non-obvious**; prefer clear names.
   (A short package doc comment and a README paragraph per file group are fine.)
3. **All exported identifiers get a doc comment** — this is a library.
4. `gofmt -l .` must be clean. `go vet ./...` must pass.
5. Tests live in `gpui/*_test.go`, package `gpui_test` (external test package)
   unless internal access is required.
6. Prefer `float32` for geometry; convert to `int` only at raster/pixel boundaries.
7. Never hold a `*T` from an entity across a call that can re-enter the same
   entity — upgrade the weak handle at invoke time instead (see
   `Context.Listener`).
