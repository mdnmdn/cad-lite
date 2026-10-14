# Reference: events, keys, actions and focus

Source: `events.go`, `keymap.go`, `focus.go`,
`bounds.go`, dispatch in `window.go`.

Back to the [reference index](README.md). Concepts:
[events and input](../concepts/events-and-input.md). Listener builders live
on `*Div` ([elements.md](elements.md#mouse-and-keyboard-listeners)).

## Input events

Every event is a value type implementing `PlatformInput` (an interface with an
unexported marker method, so only these types qualify). Platforms and tests
pass them **by value** to `Window.DispatchEvent` / `HeadlessWindow.Dispatch`;
handlers receive **pointers**.

| Event | Fields | Notes |
|---|---|---|
| `MouseDownEvent` | `Position Point`, `Button MouseButton`, `Modifiers Modifiers`, `ClickCount int` | `IsLeft()`, `IsRight()`. Focuses the innermost focusable element on the hit path unless a handler moved focus. |
| `MouseUpEvent` | `Position Point`, `Button MouseButton`, `Modifiers Modifiers`, `ClickCount int` | `IsLeft()`. |
| `MouseMoveEvent` | `Position Point`, `Modifiers Modifiers` | Drives hover. |
| `ClickEvent` | `Up *MouseUpEvent` | A completed press-release on the same element. Not synthesised by the window or the headless platform; the ebiten backend emits it. |
| `ScrollWheelEvent` | `Position Point`, `DeltaX, DeltaY Pixels`, `Modifiers Modifiers`, `Precise bool` | Positive `DeltaY` scrolls down (content moves up). `Precise` is true for pixel-precise deltas (a trackpad) and false for notch steps (a mouse wheel); the ebiten backend infers it, other sources leave it false. |
| `PinchEvent` | `Position Point`, `Delta float32`, `Modifiers Modifiers` | A trackpad pinch step; `Delta` is the magnification as a fraction (+0.1 = 10% bigger, scale factor `1+Delta`). Sent by the macOS ebiten backend only; a browser or Windows pinch is a Ctrl+`ScrollWheelEvent`. |
| `FileDropEvent` | `Position Point`, `Paths []string`, `Modifiers Modifiers` | Files or folders dropped on the window from the OS; `Paths` are absolute real paths sorted by name. Sent by the desktop ebiten backends; a browser drop has no real paths and sends nothing. |
| `KeyDownEvent` | `Keystroke Keystroke`, `PreferCharacterInput bool` | `PreferCharacterInput` is true for printable characters meant for a focused text field. |
| `KeyUpEvent` | `Keystroke Keystroke` | |
| `ModifiersChangedEvent` | `Modifiers Modifiers` | Accepted by dispatch but currently has no listeners. |

```text
type MouseButton uint8   // MouseLeft, MouseMiddle, MouseRight

type Modifiers uint8     // bit set
const ( Shift Modifiers = 1 << iota; Control; Alt; ModPlatform; Function )
```

`ModPlatform` is Cmd on macOS and Super/Win elsewhere; physical Ctrl is always
`Control` (GPUI's convention).

```text
type DispatchPhase uint8   // Capture, Bubble
```

`DispatchPhase` is declared but only the bubble phase exists; the Rust
`capture_*` listeners are not ported.

### Listener builders (on `*Div`)

See the signature table in [elements.md](elements.md#mouse-and-keyboard-listeners):
`OnMouseDown`, `OnMouseUp`, `OnMouseMove`, `OnClick`, `OnScrollWheel`, `OnPinch`, `OnFileDrop`,
`OnHoverStart`, `OnHoverEnd`, `OnKeyDown`, `OnKeyUp`, `OnAction`, plus
`Tooltip(text string)` (see [events-and-input.md](../concepts/events-and-input.md#tooltips)). Mouse
listeners run innermost first along the hit path (bubbling); call
`Window.StopPropagation()` to halt the traversal.

### Hit testing types

```text
type Hitbox struct { Bounds Bounds; Order uint32; HasMouseListeners bool }
func NewHitbox(b Bounds, order uint32) Hitbox

type BoundsTree struct{ /* zero value is ready */ }   // one frame's hitboxes
func (t *BoundsTree) Insert(h Hitbox) int
func (t *BoundsTree) HitTest(p Point) int     // highest Order wins, later insert on ties; -1 for none
func (t *BoundsTree) Box(i int) Hitbox
func (t *BoundsTree) Boxes() []Hitbox         // copy, insertion order
func (t *BoundsTree) Len() int
func (t *BoundsTree) Clear()

type MouseListener struct {                   // per-hitbox dispatch table recorded during prepaint
    Bounds Bounds
    OnMouseDown func(*MouseDownEvent, *Window, *App)
    OnMouseUp   func(*MouseUpEvent, *Window, *App)
    OnMouseMove func(*MouseMoveEvent, *Window, *App)
    OnClick     func(*ClickEvent, *Window, *App)
    OnScrollWheel func(*ScrollWheelEvent, *Window, *App)
    OnPinch       func(*PinchEvent, *Window, *App)
    OnFileDrop    func(*FileDropEvent, *Window, *App)
    OnHoverStart, OnHoverEnd func(*Window, *App)
    KeyContext string
}

type Frame struct{ /* opaque */ }             // one Draw's scene + hitboxes + listeners
func (f *Frame) HitTest(p Point) int
func (f *Frame) HitboxCount() int
func (f *Frame) SceneRef() *Scene
```

`Bounds.Contains` is half-open, so a point on a box's right or bottom edge is
outside it. To ask "which element is at this point" from application code use
`Window.HitTest(p)` ([app-window.md](app-window.md#window)).

## Keystrokes

```text
type Keystroke struct { Modifiers Modifiers; Key string; KeyChar *string }

func NewKeystroke(key string, mods Modifiers) Keystroke     // key is lower-cased by the caller
func ParseKeystroke(s string) (Keystroke, error)            // "cmd-shift-s", "ctrl-enter", "a"
func (k Keystroke) String() string                          // canonical order cmd-, ctrl-, alt-, shift-, fn-, key
func (k Keystroke) Equals(o Keystroke) bool                 // exact: modifiers, key (case-insensitive) and KeyChar
func (k Keystroke) Matches(typed Keystroke) bool            // binding match (GPUI should_match); typed.KeyChar is ignored
```

`Key` is a lower-case symbolic name: `"enter"`, `"escape"`, `"backspace"`,
`"delete"`, `"tab"`, `"space"`, `"left"`, `"right"`, `"up"`, `"down"`,
`"home"`, `"pageup"`, `"f1"`..`"f12"`, `"a"`..`"z"`, `"0"`..`"9"` and
punctuation. `KeyChar` is set only for text-input events.

`ParseKeystroke` modifiers: `shift`, `ctrl`/`control`, `alt`/`option`,
`cmd`/`command`/`platform`/`meta`, `fn`/`function`, then the key, joined by
`-` (so `-` cannot be a key name). An empty string, a trailing `-` or an
unknown modifier is an error. Chords (several keystrokes) are not supported.

## Actions

```text
type Action interface { Name() string; Clone() Action; Equals(other Action) bool }
func NewAction(name string) Action             // simple named action value

type ActionKey struct { Type reflect.Type; Name string }
func ActionKeyOf(a Action) ActionKey           // nil yields the zero key
```

`ActionKey` is the identity `OnAction` handlers register under: the action's
concrete Go type plus, for `NewAction` values, the name. `NewAction("a")` and
`NewAction("b")` are distinct; any two values of a user-defined action struct
type share a key, so a zero value can register the handler.

## Keymap and bindings

```text
type KeyBinding struct {
    Action     Action
    Keystrokes []Keystroke
    Context    string      // predicate over the key-context stack; "" matches any
}
func NewKeyBinding(keystroke string, action Action, context string) KeyBinding   // panics on a malformed keystroke or context

type Keymap struct{ /* opaque */ }
func NewKeymap() *Keymap
func (k *Keymap) AddBindings(bindings ...KeyBinding)           // appends, preserving order
func (k *Keymap) Bindings() []KeyBinding                       // copy
func (k *Keymap) Len() int
func (k *Keymap) Clear()
func (k *Keymap) MatchKeystroke(typed Keystroke, ctxStack []string) Action
func (k *Keymap) MatchKeystrokeWithContext(typed Keystroke, ctxStack []string) (Action, bool)
```

- The context predicate is a boolean expression over the `KeyContext` strings
  on the focus path: bare tokens, `key=value` tokens, `!`, `&&`, `||` and
  parentheses (`"editor && !readonly"`).
- Bindings are scanned in reverse insertion order: a later binding overrides
  an earlier one for the same keystroke and context.
- `App.Keymap()` is the app's keymap; `WithKeymap` installs one for
  `RenderToPNG` ([platform.md](platform.md#rendering-helpers)).

## Focus

```text
type FocusID uint64                        // zero means "no focus"
type FocusHandle struct{ /* opaque */ }    // obtain from (*App).NewFocusHandle()

func (h *FocusHandle) ID() FocusID
func (h *FocusHandle) IsValid() bool
func (h *FocusHandle) Focus(w *Window)               // no-op on nil/invalid handle or nil window
func (h *FocusHandle) Blur(w *Window)                // only if this handle holds focus
func (h *FocusHandle) IsFocused(w *Window) bool
func (h *FocusHandle) ContainsFocused(w *Window) bool  // same as IsFocused: no focus-parent chain
func (h *FocusHandle) IsFocusVisible(w *Window) bool   // focused and last input was the keyboard
func (h *FocusHandle) SetTabIndex(i int) *FocusHandle  // chains
func (h *FocusHandle) SetTabStop(b bool) *FocusHandle  // chains
func (h *FocusHandle) TabIndex() int
func (h *FocusHandle) TabStop() bool
```

Window-level focus API (see also [app-window.md](app-window.md#window)):

| Method | Meaning |
|---|---|
| `Window.Focus() *FocusHandle` | Handle of the focused element, or nil. |
| `Window.RequestFocus(h *FocusHandle)` | Move focus to `h`. |
| `Window.Blur()` | Clear focus. |
| `Window.FocusNext()` / `FocusPrev()` | Move to the next/previous tab stop (by tab index, ties in tree order, wrapping). Tab is **not** bound by default: bind it to an action that calls these. |
| `Window.FocusVisible() bool` | True when the last input was a key press; gates `Div.FocusVisible` styles. |

On the element side: `Div.TrackFocus(h)`, `Div.TabIndex(i)`, `Div.TabStop(b)`
and `Div.KeyContext(ctx)` ([elements.md](elements.md#focus-key-context-tab-order)).
Focus changes request a redraw.

## Dispatch order for a key press

1. A focused text field that accepts text input and a `PreferCharacterInput`
   event: the keymap is consulted first; if a binding matches and an
   `OnAction` handler on the focus path handles it, dispatch ends. Otherwise
   the character is inserted into the field.
2. `OnKeyDown` handlers along the key path, target first, until
   `StopPropagation`.
3. The keymap, with the key contexts on the path (root first); the matched
   action goes to `OnAction` handlers, target first.

The key path is root to the focused element's key node, or just the root
element's node when nothing is focused.
