# Events and input

Source: `gpui/events.go`, `gpui/window.go` (dispatch), `gpui/keymap.go`,
`gpui/focus.go`, `gpui/input.go`, `gpui/clipboard.go`,
`gpui/ebitenplatform/input.go`.

The platform turns OS input into `PlatformInput` values (`MouseDownEvent`,
`MouseUpEvent`, `MouseMoveEvent`, `ClickEvent`, `ScrollWheelEvent`,
`KeyDownEvent`, `KeyUpEvent`, `ModifiersChangedEvent`) and calls
`Window.DispatchEvent(input, app)` inside `app.Update`. Dispatch reads the
**rendered frame** - the hitboxes and key nodes recorded during the last
prepaint.

Handler signatures all end in `(*Window, *App)`:

```go
OnClick(func(*gpui.ClickEvent, *gpui.Window, *gpui.App))
OnKeyDown(func(*gpui.KeyDownEvent, *gpui.Window, *gpui.App))
OnHoverStart(func(*gpui.Window, *gpui.App))
```

Dispatch does not redraw on its own (except scroll and focus changes): a
handler that changed state calls `Window.Refresh()`, or notifies an entity
the window rendered (`Context.Notify` / `App.Notify` redraw every window that
read that entity during its last draw; see
[app-and-entities.md](app-and-entities.md)).

## Mouse

### Hit testing and the hit path

Every `Div` registers a hitbox during prepaint; `Text` does not. Hitboxes are
stamped with an increasing paint order, so the hitbox with the highest order
containing the point (children after parents, later siblings after earlier
ones) is the target. Bounds are half-open (a point on the right/bottom edge is
outside). The **hit path** is the target hitbox followed by its chain of
ancestor hitboxes (`parent` links), innermost first. Overlapping non-ancestor
elements below the target are not on the path.

Hitboxes are clipped to the enclosing clip rectangles (`Window.InsertHitbox`),
so a child scrolled out of its container's visible area is not hit-testable
there. Clipping is rectangular: hitboxes are not clipped to rounded corners.

### Bubbling and StopPropagation

Mouse listeners run innermost first along the hit path, each once, bubbling
toward the root, until a handler calls `Window.StopPropagation()`. Only the
bubble phase exists: `DispatchPhase` (`Capture`/`Bubble`) is declared, but the
`capture_*` listeners of Rust GPUI are not ported.

```go
gpui.NewDiv().OnClick(outer).Child(
    gpui.NewDiv().OnClick(func(e *gpui.ClickEvent, w *gpui.Window, _ *gpui.App) {
        w.StopPropagation() // outer never sees this click
    }))
```

### Click synthesis

`Window` does not synthesize clicks; it dispatches whatever `ClickEvent` the
platform sends, along the hit path at `Up.Position`. The ebiten backend
(`clickTracker`) records which element (by `GlobalElementID`) each button was
pressed on and emits `MouseUp` followed by `ClickEvent{Up: &up}` only when the
release lands on the same element id as the press - so a press, redraw, release
sequence still counts. `ClickCount` counts presses of the same button within
400 ms and 4 px. Headless tests must dispatch the three events themselves
(see [platform.md](platform.md)).

### Hover

`OnHoverStart` / `OnHoverEnd` fire from mouse-move dispatch, and again after
every `Draw` at the last known pointer position. An element counts as hovered
when it, or a descendant, is on the hit path (so a parent stays hovered while
the pointer is over its children). On a change, `OnHoverEnd` callbacks fire
first, deepest first; then `OnHoverStart` callbacks, root first. Hover is
keyed by the element's global id and callbacks are looked up in the current
frame, so an element that disappeared gets no `OnHoverEnd`. Use stable
`.ID(...)`s. The move event's own `OnMouseMove` listeners run after the hover
update.

### Scroll wheel

`ScrollWheelEvent` deltas are logical pixels, positive `DeltaY` = scroll down
(the ebiten backend negates ebiten's wheel and uses 20 px per line).
`OnScrollWheel` listeners run along the hit path first (bubbling, innermost
first). If one of them calls `Window.StopPropagation()`, no container
scrolls - this is how a handler takes over the wheel. Otherwise the delta is
offered to the `OverflowScroll` containers on the hit path, innermost first,
restricted to each one's `ScrollAxes`; the first whose offset changes takes
it (clamped to `[0, ScrollMax]`) and the window refreshes, so a container at
its limit lets the wheel chain to the next one out.

## Keyboard

### Key events and modifiers

`Keystroke{Modifiers, Key, KeyChar}`: `Key` is a lower-case symbolic name
(`"enter"`, `"escape"`, `"backspace"`, `"delete"`, `"tab"`, `"space"`,
`"left"`, `"up"`, `"home"`, `"pageup"`, `"f1"`..`"f12"`, `"a"`..`"z"`,
`"0"`..`"9"`, and punctuation such as `"-"` `","`). `KeyChar` is set only for
text-input events.

Modifier bits: `Shift`, `Control`, `Alt`, `ModPlatform`, `Function`.
GPUI's convention is used on **every** OS: physical Ctrl is always `Control`,
and the Cmd key (macOS) or Super/Win key (Windows/Linux, ebiten `KeyMeta`) is
`ModPlatform`. A binding `"cmd-s"` therefore means the Super key on Linux, not
Ctrl; register `"ctrl-s"` as well for cross-platform shortcuts.
`Keystroke.String()` always prints `ModPlatform` as `cmd-`.

### The ebiten backend's key stream

- Non-printable keys (enter, arrows, tab, F-keys, ...) emit a `KeyDownEvent`
  on press and repeat after 30 ticks every 3 ticks.
- Printable keys emit a key event **only** when a shortcut modifier is held
  (Cmd/Super, or Ctrl - except AltGr). Plain typing arrives instead as text:
  `ebiten.AppendInputChars` -> `KeyDownEvent{Keystroke{Key, KeyChar: &s},
  PreferCharacterInput: true}`, with Shift/Alt/Control stripped from the
  modifiers. `Key` is the character itself, except a space is `"space"` and
  an ASCII capital is its lower-case letter plus `Shift` (`"A"` ->
  `shift-a`). Option-composed characters on macOS therefore reach text
  fields.
- Bare printable bindings work: `"a"`, `"space"`, `"shift-a"`, `"?"` match
  these char events (bindings match on the key and ignore `KeyChar`, see
  below).
- `KeyUpEvent`s are emitted for named keys.

### Dispatch order

`Window.dispatchKeyDown`:

1. **Text input.** If the focused text field (the last handler
   registered via `Window.HandleInput` this frame) `AcceptsTextInput()` and the
   event is `PreferCharacterInput` with a `KeyChar`, the keymap is consulted
   first, as in GPUI: if a binding matches and an `OnAction` handler on the
   focus path handles its action, dispatch ends there. Otherwise the char is
   inserted via `ReplaceTextInRange` (the current selection, or the caret)
   and dispatch ends (`OnKeyDown` does not see it). Scope bare printable
   bindings with a context (e.g. `"!textbox"`) if they must not steal typing.
2. **Key path.** Key nodes form a tree (a `Div` with an `OnKeyDown`/`OnKeyUp`/
   `OnAction`, a `KeyContext` or a focus handle). The path runs root -> target,
   where the target is the key node whose focus id is the window's focus; if
   nothing focused is in the frame, the path is just the root element's key
   node. `OnKeyDown` handlers run target first, bubbling, until
   `StopPropagation`.
3. **Keymap.** Unless stopped, the keymap is consulted with the key contexts
   on the path (root first). A matched action is delivered to `OnAction`
   handlers along the path, target first.

### Keymap, contexts and actions

```go
type incAction struct{}

func (incAction) Name() string              { return "increment" }
func (incAction) Clone() gpui.Action        { return incAction{} }
func (incAction) Equals(o gpui.Action) bool { _, ok := o.(incAction); return ok }

app.Keymap().AddBindings(gpui.NewKeyBinding("cmd-up", incAction{}, "counter"))

gpui.NewDiv().KeyContext("counter").TrackFocus(focus).
    OnAction(incAction{}, func(_ gpui.Action, w *gpui.Window, _ *gpui.App) {
        n++
        w.Refresh()
    })
```

- `ParseKeystroke` / `NewKeyBinding` take GPUI strings: zero or more modifiers
  (`shift`, `ctrl`/`control`, `alt`/`option`, `cmd`/`command`/`platform`/`meta`,
  `fn`/`function`) then a key, joined by `-` (so `-` itself cannot be a key).
  `NewKeyBinding` **panics** on a malformed keystroke or context.
- Context predicate (third argument): empty matches anything; leaves are bare
  tokens or `key=value` tokens that match a `KeyContext` string on the path
  verbatim; combine with `!`, `&&`, `||` and parentheses
  (`"editor && !readonly"`).
- Bindings are scanned **latest first**, so a later binding overrides an
  earlier one for the same keystroke and context. Only a binding's last
  keystroke is matched: multi-keystroke chords are not supported.
- **Matching** uses `Keystroke.Matches` (GPUI's `should_match`): the
  modifiers must be equal and either the keys are equal (case-insensitive;
  the typed `KeyChar` is ignored) or the typed `KeyChar` equals the binding's
  key. `Keystroke.Equals` stays exact (it compares `KeyChar` too).
- **Action identity** is `ActionKeyOf(action)`: the Go type (GPUI's TypeId)
  plus, for `gpui.NewAction("name")` values, the name. So
  `NewAction("add-todo")` and `NewAction("blur")` get separate `OnAction`
  handlers, and a struct action type can register its handler with a zero
  value (`OnAction(incAction{}, ...)`) whatever fields the bound value
  carries.
- `Div.OnAction` handlers sit on key nodes, so the div (or an ancestor of the
  focused one) must be on the key path; with no focus, only the root element
  receives actions.

### Focus

```go
focus := app.NewFocusHandle()       // once, e.g. lazily in Render
div.TrackFocus(focus)               // mark an element as the focus target
focus.Focus(w); focus.Blur(w)       // request / drop focus (both Refresh)
focus.IsFocused(w); w.Focus()       // query
```

Focus is a single `FocusID` per window. A mouse press focuses the innermost
focusable element on the hit path (a div with `TrackFocus` or `TabIndex`); an
element that is not focusable needs an `OnMouseDown` that calls
`focus.Focus(w)`. A handler that moves focus wins: once an `OnMouseDown` on
the path changed focus, no focusable ancestor re-focuses itself. There is no
focus-parent chain (`ContainsFocused` equals `IsFocused`). Handles are never
released, except the implicit handle of a `TabIndex` div, which is dropped
with the div's element state when the div is not drawn in a frame.

Tab order: `TabIndex(i)` (div) or `FocusHandle.SetTabIndex` / `SetTabStop`
register tab stops; `Window.FocusNext()` / `FocusPrev()` move focus through the
rendered frame's stops (sorted by index, ties in tree order, wrapping). Tab is
**not bound by default**: bind it to an action that calls them
(`examples/tab_stop`).

`Window.FocusVisible()` is true when the last input was a key press and false
after a mouse press; it gates `Div.FocusVisible` styles.

Example with named actions and a bare-key binding:

```go
add, clear := gpui.NewAction("add-todo"), gpui.NewAction("clear")
app.Keymap().AddBindings(
    gpui.NewKeyBinding("enter", add, ""),
    gpui.NewKeyBinding("c", clear, "!textbox"), // not while typing in a field
)
gpui.NewDiv().OnAction(add, onAdd).OnAction(clear, onClear)
```

## TextInput and PlatformInputHandler

`TextInput` (`gpui/input.go`) is a plain struct, not an entity. Own it in your
root view and call `Render` from your `Render` every frame:

```go
ti := gpui.NewTextInput("What needs doing?").
    OnSubmit(func(text string, w *gpui.Window, cx *gpui.App) { /* Enter */ }).
    OnChange(func(text string, w *gpui.Window, cx *gpui.App) { /* edits */ })

// in the root's Render:
row := gpui.NewDiv().FlexRow().Child(ti.Render(w, cx))
```

`Render` lazily creates the focus handle (`ti.FocusHandle()` is `nil` before
the first call), records the window's text measurer, and returns a
focusable `Div` with key context `"textbox"`, 8 px padding, a 1 px border and
rounded corners. Mouse-down focuses it. While focused it registers itself with
`Window.HandleInput` during prepaint and paints a 2 px caret.

Built-in keys (handled on the div's `OnKeyDown`, which stops propagation for
the first two): `enter` -> `OnSubmit`, `escape` -> blur, `backspace`,
`delete`, `left`, `right`. Typed text and pasted text arrive through the
`PlatformInputHandler`. Not implemented: home/end/up/down, word movement,
select-all, shift-selection, mouse caret placement or selection, horizontal
scrolling, undo. Consequently copy/cut only act if the platform has set a
non-empty selection via `SetSelectedTextRange`. Caret measurement runs at
scale 1 (`BoundsForRange` / `CharacterIndexForPoint` use fixed estimates).
`SetText` replaces the content and moves the caret to the end.

`PlatformInputHandler` (`gpui/platform.go`) is the editing surface an OS IME
or clipboard would drive: `TextForRange`, `SelectedTextRange`,
`MarkedTextRange`, `UnmarkText`, `ReplaceTextInRange(*IntRange, string)` (nil
range = insert at the caret), `BoundsForRange`, `CharacterIndexForPoint`,
`SetSelectedTextRange`, `TextLengthUTF16`, `AcceptsTextInput`. After each draw
the window installs the focused handler on the platform window with
`SetInputHandler`.

### UTF-16 ranges

`IntRange` is a half-open `[Start, End)` range of **UTF-16 code units**, as
native text-input APIs expect, not bytes or runes: `"h😀llo"` has length 6
(the emoji is a surrogate pair) while `"héllo"` has length 5. `TextInput`
converts to byte offsets internally. `UTF16Selection{Range, Reversed, IsNil}`
adds the caret orientation (`Reversed` puts the caret at `Range.Start`) and an
explicit "no selection" flag.

## Clipboard

`gpui.ClipboardRead()`, `gpui.ClipboardWrite(s)` and `ClipboardAvailable()`
(`gpui/clipboard.go`) shell out with a 1 s timeout: `pbpaste`/`pbcopy`
(macOS), PowerShell (Windows), `wl-paste`/`wl-copy` when `WAYLAND_DISPLAY` is
set, then `xclip`, then `xsel` (Linux). They return `errors` rather than
panic when no tool exists. On Windows an empty string cannot be written (the
clipboard cannot be cleared).

The ebiten backend wires shortcuts - `cmd-c/x/v` on macOS, `ctrl-c/x/v`
elsewhere - on a worker goroutine, applying results on the next `Update`:
paste is delivered as character input (newlines/tabs become spaces, control
characters dropped); copy needs a focused handler with a non-empty selection;
cut also deletes the text, but only if the write succeeded and the selection
is unchanged. These shortcuts are handled by the platform and are not
dispatched as key events.
