# Troubleshooting

Symptoms first, then the reasoning. Most problems here come from a handful of
rules: nothing redraws unless something asks, key bindings need the right focus
path and context, entities are reference-counted by hand, and the interactive
window needs a real display. For how things work see the
[concept pages](../README.md); a failing behaviour is quickest to isolate in a
[headless test](testing.md).

| Symptom | Jump to |
|---|---|
| Window opens but there is no text | [No text in the window](#no-text-in-the-window) |
| Text looks soft or doubled | [Blurry text](#blurry-text) |
| Frames are slow or the fan spins | [Slow frames](#slow-frames) |
| A shortcut does nothing | [Key binding not firing](#key-binding-not-firing) |
| Typing into a field triggers shortcuts or the character vanishes | [Bare-key bindings swallow typing](#bare-key-bindings-swallow-typing) |
| Tab or clicks do not move focus | [Focus not moving](#focus-not-moving) |
| Window creation fails with a pixel-format or context error | [Metal or GL pixel-format error](#metal-or-gl-pixel-format-error) |
| `Read` returns nil | [Entity Read returns nil after release](#entity-read-returns-nil-after-release) |
| State changed but the screen did not | [Nothing redraws](#nothing-redraws) |
| A scroll area does not scroll | [Scroll container does not scroll](#scroll-container-does-not-scroll) |
| `OnClick` never fires in a test | [Clicks in headless tests](#clicks-in-headless-tests) |

## No text in the window

Shapes appear but no glyphs: the font manager found no system font. The shaper
is then `nil`, `Window.TextMeasurer()` returns `nil`, text lays out with a
crude estimate (0.55 em per rune) and **paints nothing**
([text concepts](../concepts/text.md#font-discovery)).

1. See what was found. Empty means no candidate exists:

   ```go
   package main

   import (
   	"fmt"

   	"go-gpui/gpui"
   )

   func main() {
   	fmt.Printf("default font: %q\n", gpui.DefaultFontPath())
   	if _, err := gpui.NewFontManager().DefaultFont(); err != nil {
   		fmt.Println("error:", err) // gpui: no usable system font found
   	}
   }
   ```

2. Point at a font explicitly: `GPUI_FONT=/path/to/Font.ttf go run ./examples/hello_world`
   (the file must exist and parse; a font either parser rejects is rejected;
   `.ttc` collections use their first face).
3. Install a font where the per-OS list looks: DejaVu Sans or Liberation Sans on
   Linux (for example the `fonts-dejavu-core` package), `segoeui.ttf` or
   `arial.ttf` on Windows, `SFNS.ttf` / `Helvetica.ttc` / `Arial.ttf` on macOS.
   Minimal containers and CI images commonly ship no fonts at all.
4. If `DefaultFontPath()` is non-empty but some text is missing, check the
   usual suspects: the text colour equals the background (`TextColor` default
   is black), the text sits in a zero-size or clipped (`OverflowHidden`)
   parent, the element is `Hidden()`, or a `StyledText` run was built from a
   bare `TextStyle{}` (a run with `FontSize <= 0` is replaced by
   `DefaultTextStyle()`, so start from that). A missing glyph for one
   character (CJK, colour emoji) draws as tofu; there is no colour emoji.
5. A named family that is not installed silently falls back to the default
   font; `FontFamily("Menlo")` is looked up by file name.

## Blurry text

Glyphs are rasterised at `size x scale` device pixels and blitted 1:1, so text
is sharp when the window scale is right. When it is not:

- **Scale mismatch.** The window reports the monitor's scale
  (`Window.ScaleFactor()`); a PNG rendered at scale 1 and viewed zoomed on a
  HiDPI display looks soft. Render at the scale you will view it at:
  `gpui.RenderToPNG(path, w, h, 2, root)` gives a `2w x 2h` image. In tests,
  `HeadlessPlatform.SetScaleFactor(2)` before opening the window.
- **Synthetic bold.** If no real bold face is found for the family, bold is
  produced by smearing the regular outlines one pixel horizontally, which looks
  heavy. Install a family with a bold file, or set `FontWeight(400)`.
- **Fractional layout.** Glyph origins sit at fractional logical pixels but are
  blitted at whole device pixels; there is no sub-pixel positioning. Text can
  shift by a pixel as layout changes; it is not blur.
- **Display scaling by the OS** on top of the app (screenshots upscaled,
  remote desktop). Compare with a `-png` render of the example.
- `TextInput` uses full-line shaped logical metrics for caret, highlight and
  hit positions, with glyph masks at the window scale. Its shaping API still
  uses logical scale 1 (roadmap L7); grapheme navigation and BiDi visual
  selection are unsupported.

## Slow frames

Every drawn frame rebuilds the element tree, lays it out, paints it and
re-rasterises the whole buffer on the CPU (no damage tracking). An idle app
costs nothing; a frame costs roughly the same whatever changed
([performance notes](../architecture.md#performance-notes)).

- **Do not redraw needlessly.** Do not call `Refresh()` from `OnMouseMove` or on
  a timer unless something changed. Hover, focus and pressed styles redraw on their own.
  An animation loop that refreshes every frame redraws the whole window every
  frame; there is no animation API.
- **Window size and scale.** Cost grows with pixel count: a window at scale 2
  has four times the pixels. Test with a smaller window to confirm.
- **Blurred shadows** (`ShadowLg`, big `WithBlur`) on many elements are the
  most expensive primitives. Use sparingly on lists.
- **Long lists.** Use `UniformList` (only visible rows are laid out and
  painted) rather than 10,000 `Div`s in a scroll container.
- **Stable text.** Shaped runs are cached by `(face, text)` (2048 entries) and
  glyph masks by `(face, glyph, size, scale)` (4096). Text that changes every
  frame, or more distinct strings than the cache holds, re-shapes; the cache is
  dropped wholesale when full.
- **Measure.** Copy a benchmark from `gpui/bench_test.go`
  ([testing](testing.md#benchmarks)) and profile:
  `go test ./gpui -run '^$' -bench DrawTodo -cpuprofile cpu.out`.
- Deep nesting is not the problem by itself: `go run ./examples/tree -depth 200` renders a 200-level tree.

## Key binding not firing

Work down this list; each item is something that has actually bitten.

1. **Focus path and where the handler lives.** The action goes to `OnAction`
   handlers on the path from the root to the focused element's key node. With
   nothing focused, **only the root element** receives it. Put the `OnAction`
   (and the `KeyContext`) on the root div, or make sure the intended element has
   focus (`TabIndex`/`TrackFocus` and a click).
2. **Context predicate.** `NewKeyBinding(keys, action, "editor")` only matches
   while a `KeyContext("editor")` is on the path. Misspelt or missing context
   means silence. `""` matches everywhere. Leaves are bare tokens or `key=value`
   and combine with `!`, `&&`, `||`, parentheses.
3. **Action identity.** `OnAction` handlers are keyed by the action's Go type,
   plus the name for `NewAction`. `OnAction(gpui.NewAction("save"), ...)` does
   not handle `NewAction("Save")`, and a binding of `NewAction("x")` is not
   handled by a user-defined action type.
4. **Modifiers.** `cmd` is the Cmd key on macOS and the Super/Win key on
   Windows and Linux; physical Ctrl is `ctrl`. Register both for portability.
   The modifiers must match exactly: `"s"` does not fire on `ctrl-s`.
5. **No chords.** `"ctrl-k ctrl-c"` is not supported; only a binding's last
   keystroke is matched. `-` is the separator, so it cannot be a key.
6. **Registration.** Bindings go into `cx.Keymap()` before the keys arrive; for
   `RenderToPNG` use `WithKeymap`. `NewKeyBinding` panics on a malformed
   keystroke or context, so a binding that exists parsed correctly.
7. **Override order.** The keymap is scanned latest-first, so a later binding
   for the same keystroke and context wins; an earlier one can be shadowed.
8. **Something stopped it.** An `OnKeyDown` handler on the path that calls
   `StopPropagation()` skips the keymap. `TextInput` consumes `enter` and
   `escape` itself (submit, blur) and handles `backspace`, `delete`, `left`, `home`, `end`,
   `right`; those never reach the keymap while it is focused.
9. **Printable keys.** The ebiten backend sends plain typing as character
   input, not as key events with a shortcut modifier; bare bindings like `"a"`,
   `"space"` and `"?"` still match those events (see the next section).
   Shifted letters arrive as lower-case plus `shift` (`"A"` is `shift-a`).
10. **Native shortcuts.** The snapshot key (`cmd/ctrl-shift-s`) and the clipboard
    shortcuts (`cmd/ctrl-c/x/v`) are handled by the platform and are not
    dispatched to your app.

To see what the keymap would do without running a window:

```go
package main

import (
	"fmt"

	"go-gpui/gpui"
)

type save struct{}

func (save) Name() string              { return "save" }
func (save) Clone() gpui.Action        { return save{} }
func (save) Equals(o gpui.Action) bool { _, ok := o.(save); return ok }

func main() {
	km := gpui.NewKeymap()
	km.AddBindings(gpui.NewKeyBinding("ctrl-s", save{}, "editor"))
	typed, _ := gpui.ParseKeystroke("ctrl-s")
	fmt.Println(km.MatchKeystroke(typed, []string{"editor"}))   // {} (the save action)
	fmt.Println(km.MatchKeystroke(typed, []string{"terminal"})) // <nil>: context did not match
}
```

## Bare-key bindings swallow typing

A binding with no modifier, such as `NewKeyBinding("a", act, "")` or `"?"`, also
matches the character events produced by typing. Dispatch consults the keymap
**before** inserting a character into a focused `TextInput`, and if a binding
matches and an `OnAction` handler on the focus path handles it, the character is
consumed and never reaches the field. So a global `"c"` binding makes the letter
`c` untypeable.

Scope the binding to exclude text fields: the field's key context is
`"textbox"`, so use `"!textbox"` (or `"editor && !textbox"`):

```text
gpui.NewKeyBinding("c", clear, "!textbox")        // not while typing in a field
gpui.NewKeyBinding("enter", add, "")              // fine: the field's own enter handling runs first
```

If the binding has to work everywhere, give it a modifier (`cmd-`, `ctrl-`), or
handle the keystroke in an `OnKeyDown` on a div outside the field. If an action
is bound but its `OnAction` handler is not on the focus path, the keymap match
is dropped and the character is typed normally, which looks like "the binding
does nothing".

## Focus not moving

- **Tab does nothing.** Tab is not bound by default. Bind `tab` / `shift-tab`
  to actions whose handlers call `Window.FocusNext()` / `FocusPrev()`, with the
  handlers on the root div ([cookbook](cookbook.md#tab-navigation)).
- **Nothing to move to.** Only tab stops are visited: a div needs `TabIndex(i)`
  (or a `FocusHandle` with `SetTabIndex`/`SetTabStop`). A non-stop item is
  skipped. Order is by index, ties in tree order, wrapping.
- **Clicks do not focus.** A mouse press focuses the innermost *focusable*
  element on the hit path (`TrackFocus` or `TabIndex`). A plain div is not
  focusable: add `TabIndex`, or call `handle.Focus(w)` from `OnMouseDown`. If
  any `OnMouseDown` on the path already changed focus, no ancestor re-focuses.
- **The handle disappears.** The implicit handle of a `TabIndex` div (one
  without `TrackFocus`) is retained per element id and dropped when the div is
  not rendered for a frame. Give tab stops stable `ID`s, and keep handles you
  create with `cx.NewFocusHandle()` in your own state (create once, not per
  frame).
- **Handle not ready.** `TextInput.FocusHandle()` is `nil` until `Render` has
  run once.
- **Focus moved but nothing changed visually.** Style the focus state:
  `Focus(...)` applies whenever focused, `FocusVisible(...)` only after a key
  press (CSS `:focus-visible`), so a click focus shows only `Focus` styles. Use
  `Window.FocusVisible()` to check the modality.
- **Seeding focus.** A window starts with no focus and `RenderToPNG` renders one
  frame with no input. To show a focused state, request focus from the view's
  first `Render` (`w.RequestFocus(h)`, as `examples/tab_stop` does).
- `ContainsFocused` equals `IsFocused`: there is no focus parent chain.

## Metal or GL pixel-format error

Opening the interactive window fails at startup with an error about a pixel
format or graphics-context creation. Observed on macOS inside a sandboxed
agent shell:

```text
glfw: NSGL: failed to find a suitable pixel format: the requested format is unavailable
```

and, with `EBITENGINE_GRAPHICS_LIBRARY=metal`:

```text
metal: mtl.CreateSystemDefaultDevice failed: mtl: MTLCreateSystemDefaultDevice returned 0
```

Cause: the process has no access to the GPU or the window server (sandbox,
SSH session, some agent harnesses). This can happen even when
`launchctl managername` reports `Aqua`. Fix: run the app from a normal
terminal on the logged-in desktop session; for automated checks use the `-png`
headless mode instead. Other wording appears on other OSes and ebiten
versions. The ebiten backend needs a real display and a GPU
(Metal on macOS, DirectX on Windows, OpenGL on Linux). It will not work in:

- sandboxed agent and CI environments, Docker containers, SSH sessions with no
  display, and macOS sandboxes without a Metal device;
- Linux without an X11/Wayland session (use `xvfb-run` if you must run it in
  CI, and install the system GL libraries ebiten needs).

Everything except the window itself works without a display, because rendering
is done by the CPU rasteriser:

- render a frame to a file: every example takes `-png out.png`
  (`go run ./examples/todo -png todo.png`), and in code
  `gpui.RenderToPNG(...)` / `RenderToImage(...)`;
- drive the app with synthetic input in tests ([testing](testing.md)).

Do not import `gpui/ebitenplatform` in code that must run headless: a program
that never imports it never links ebiten. The interactive window has not been
exercised in the agent test environment (roadmap item T3), so a window-only
failure on a machine with a display is worth reporting with the OS, GPU and
ebiten version.

## Entity Read returns nil after release

`Entity.Read` returns `nil` when the entity has been released (and `Update` is
then a no-op, `WeakEntity.Upgrade` reports `false`). Entities are reference
counted by hand because Go has no `Drop` ([app and entities](../concepts/app-and-entities.md#releasing)).
Typical causes:

- **Releasing a borrowed handle.** Only the handle from `NewEntity` and each
  `Clone()` are strong references. Plain copies, `Context.Entity()` and
  `Upgrade()` results are borrowed; releasing one drops the owner's reference
  and the entity dies early. Balance `Clone` with `Release` and nothing else.
- **A window root was closed.** `EntityView(e)` holds a clone that
  `Window.Close()` releases. If that was the last reference, the entity is gone;
  keep your own `Clone()` if it must outlive the window.
- **Release inside a flush is deferred, but it still happens.** After the
  outermost `Update` returns, the entity is freed. Code that runs later and
  reads a stale handle sees `nil`; guard with `if st := e.Read(cx); st != nil`.
- **Stale handles after slot reuse.** Slots are reused with a new generation; an
  old `Entity` value never resurrects, it just reads `nil`.
- **Dangling `*T`.** A pointer from `Read` or `Update` is only valid until the
  next call that can re-enter the entity (`Notify`, `Emit`, `Refresh`, another
  `Update`). Re-fetch, or use `gpui.Listener`, which upgrades a weak handle each
  time ([re-entrancy rule](../concepts/app-and-entities.md#the-re-entrancy-rule-agentsmd-rule-7)).
- **Mutations outside `App.Update`.** `Entity.Update` does not flush effects on
  its own: observers and window invalidation run when the outermost
  `App.Update` returns. Wrap tests and `Run`-callback mutations in
  `app.Update(func(app *gpui.App) { ... })` if you expect an observer to fire.

## Nothing redraws

Entity `Notify` redraws windows that **read that entity during their last
draw**. A plain-struct root (the common case) is not an entity: change your
state and call `w.Refresh()` from the handler. A custom `Element` that stores state in
`SetElementState` must touch it every frame it is drawn or it is dropped.

## Scroll container does not scroll

- It needs `OverflowScroll` / `OverflowYScroll` / `OverflowXScroll`
  (`OverflowHidden` clips but never scrolls) and a stable `ID(...)`.
- It needs content larger than its box, which means a **bounded size**: give it
  `SizeH`, `MaxH`, or `Flex1()` inside a parent that has a definite height. An
  auto-height container simply grows to fit its content.
- The wheel goes to the innermost scroll container under the pointer on the
  axes it scrolls; a container at its limit passes it outwards. An
  `OnScrollWheel` handler that calls `StopPropagation()` takes the wheel.
- `ScrollHandle.SetOffset`/`ScrollToItem` apply at the next frame; refresh the
  window.

## Clicks in headless tests

`HeadlessWindow.Dispatch` does not synthesise clicks. `OnClick` runs only on a
`ClickEvent`; send `MouseDownEvent`, `MouseUpEvent` and `ClickEvent{Up: &up}`
([testing](testing.md#dispatching-input)). Hover needs a `MouseMoveEvent`, and
typing needs `KeyDownEvent{PreferCharacterInput: true, Keystroke{KeyChar}}`.

## Other things people trip over

- **Borders eat content.** Borders paint inside the box and do not enter
  layout: add matching padding (roadmap item L4).
- **A `Text` with `Bg` shows no background.** Text leaves paint no box; wrap in a
  `Div` (roadmap item S4).
- **Hover styles that change size do nothing.** Interaction styles apply at
  paint time, after layout; only paint properties take effect, and only text
  colour and strikethrough cascade to descendant text.
- **`.Child` on a `*StyledMixin`.** A call promoted from the embedded mixin
  returns `*StyledMixin`, which ends the typed chain; use the builders
  re-declared on `*Div` / `*Text`, or call them as statements on a variable
  ([style builders](../reference/style-builders.md#how-the-builders-are-organised)).
- **Overlapping elements.** Use `Absolute()` and pixel offsets, or overlap with
  negative margins ([cookbook](cookbook.md#modal-overlay-with-paint-order)).
- **Shortcut works on Linux but not as written.** `cmd-` is the Super key
  there; add a `ctrl-` binding.
