# Platforms

Source: `gpui/platform.go`, `gpui/headless.go`, `gpui/ebitenplatform/`.

## The interfaces

```go
type Platform interface {
    Run(onLaunch func(*App)) error            // blocks until Quit
    OpenWindow(o WindowOptions) (PlatformWindow, error)
    Quit()                                    // must be goroutine-safe
    Name() string                             // "macos", "windows", "linux", "headless"
}

type PlatformWindow interface {
    Bounds() (w, h Pixels)                    // logical pixels
    ScaleFactor() float32                     // device px per logical px
    RequestFrame()                            // "please draw soon"
    Present(pix []byte, w, h int)             // premultiplied RGBA, 4*w*h bytes
    SetInputHandler(h PlatformInputHandler)
    TakeInputHandler() PlatformInputHandler
    Quit()
}

type FrameRequester interface{ OnRequestFrame(f func()) } // optional
```

`WindowOptions{Width, Height, Title, Resizable}`; `DefaultWindowOptions()` is
800x600, resizable, titled "gpui".

Flow: your `main` calls `platform.Run(onLaunch)`. The platform creates the
`App` (`NewAppWithPlatform(p)`), calls `onLaunch(app)`, where you add keymap
bindings and call `app.OpenWindow(opts, build)`. `OpenWindow` calls
`Platform.OpenWindow`, wraps the result in a `Window`, hooks
`FrameRequester.OnRequestFrame` (if implemented) to `Window.Draw`, and draws the
first frame.

The pixel buffer given to `Present` is owned by the rasterizer and rewritten on
the next draw. Copy it if you keep it past the call (the ebiten backend does
not copy, because it only reads it on the same goroutine before the next draw).

## The ebiten backend

`ebitenplatform.New(opts ...Option) gpui.Platform` (`gpui/ebitenplatform/`).
It supports **one window**; a second `OpenWindow` returns an error.

`OpenWindow` sets the title, size and resize mode through ebiten and reads the
monitor's `DeviceScaleFactor` as the window scale. `Run` calls
`ebiten.SetScreenClearedEveryFrame(false)`, creates the `App`, calls
`onLaunch`, then `ebiten.RunGame`. `ebiten.Termination` is mapped to a clean
`nil` return.

The game loop (`gpui/ebitenplatform/platform.go`):

- **`Layout(outsideW, outsideH)`** returns *device* pixels
  (`outside x scale`), recomputes the logical size (`device / scale`) and marks
  the window pending when the size or scale changed. Resizing therefore
  re-layouts and re-rasterizes at the new size.
- **`Update()`** (once per tick):
  1. quits (`ebiten.Termination`) if `Platform.Quit` or `App.Quit` was called;
  2. inside one `app.Update`: drains finished clipboard jobs, then polls input
     (modifiers, cursor position, mouse buttons, wheel, keys, text);
  3. if `RequestFrame` set the pending flag, calls the window's draw routine
     (also inside `app.Update`). This is what runs
     `Window.Draw` -> `Present`.
- **`Draw(screen)`** does `screen.WritePixels(pix)` only if a fresh frame was
  presented since the last call (and the buffer size matches the screen;
  otherwise it requests a new frame). No uploads happen on idle ticks.
- **`RequestFrame`** only sets a flag, so any number of requests per tick
  collapse into a single draw in the next `Update`.

Input mapping (`gpui/ebitenplatform/input.go`):

| Source | Result |
|---|---|
| Cursor movement | `MouseMoveEvent` (logical px = cursor / scale) |
| Button press / release | `MouseDownEvent` / `MouseUpEvent`, plus `ClickEvent` when release is on the same element id as the press |
| Wheel | `ScrollWheelEvent`, `20 px` per line, positive `DeltaY` = down |
| Shift / Ctrl / Alt / Meta | `Shift` / `Control` / `Alt` / `ModPlatform` |
| Named keys, shortcuts | `KeyDownEvent` / `KeyUpEvent` |
| Typed text | `KeyDownEvent{PreferCharacterInput: true}`; `Key` is the char (`"space"` for a space, lower-case letter + `Shift` for a capital), `KeyChar` the text, so bare bindings like `"a"`/`"space"` fire |
| cmd/ctrl-c, x, v | async clipboard, then edits via the input handler |

Details of each are in [events-and-input.md](events-and-input.md).

### Snapshot key

Pressing the snapshot shortcut saves the last presented frame as
`snapshot-YYYYMMDD-HHMMSS.png` (device pixels, un-premultiplied, `-N` suffix on
collision) in the current working directory, encoded on a goroutine, with the
path or error logged to stderr. The key press is swallowed: it is not
dispatched to the app. The default is `cmd-shift-s` on macOS and
`ctrl-shift-s` elsewhere (`DefaultSnapshotKey()`); change or disable it:

```go
p := ebitenplatform.New(ebitenplatform.WithSnapshotKey("cmd-shift-p"))
p = ebitenplatform.New(ebitenplatform.WithSnapshotKey("")) // disabled
```

`WithSnapshotKey` panics on a malformed keystroke.

### Quitting

`App.Quit()` sets an atomic flag and calls `Platform.Quit()`; both are safe from
any goroutine, and the next `Update` terminates the loop:

```go
time.AfterFunc(2*time.Second, cx.Quit) // as the examples' -smoke flag does
```

Caveat (`_docs/wip.md`): the interactive window has not been run in the CI
environment (no GPU/Metal device); headless rendering is what is covered by
tests.

## Headless platform and RenderToPNG

`NewHeadlessPlatform()` returns a `*HeadlessPlatform` whose windows live in
memory but run the full pipeline, so rendering is byte-for-byte the same code
path as the real backend.

One-shot rendering:

```go
err := gpui.RenderToPNG("out.png", 480, 640, 2, root,
    gpui.WithBackgroundColor(gpui.Gray50), gpui.WithTitle("t"))
img, err := gpui.RenderToImage(480, 640, 1, root) // *image.NRGBA
```

`w x h` are logical pixels; the image is `round(w*scale) x round(h*scale)`.
Options: `WithBackgroundColor`, `WithKeymap(*Keymap)`, `WithTitle`. The examples'
`-png PATH` flag uses exactly this.

Driving an app in tests:

```go
p := gpui.NewHeadlessPlatform()
p.SetScaleFactor(2)
app := gpui.NewAppWithPlatform(p)
app.Keymap().AddBindings(gpui.NewKeyBinding("cmd-up", incAction{}, "counter"))
_ = app.OpenWindow(gpui.WindowOptions{Width: 200, Height: 100},
    func(*gpui.Window, *gpui.App) gpui.EntityRoot { return root })

hw := p.Windows()[0]
pos := gpui.Point{X: 10, Y: 10}
hw.Dispatch(gpui.MouseDownEvent{Position: pos, Button: gpui.MouseLeft, ClickCount: 1})
up := gpui.MouseUpEvent{Position: pos, Button: gpui.MouseLeft, ClickCount: 1}
hw.Dispatch(up)
hw.Dispatch(gpui.ClickEvent{Up: &up}) // clicks are not synthesized for you
hw.Dispatch(gpui.KeyDownEvent{Keystroke: gpui.NewKeystroke("up", gpui.ModPlatform)})
_ = hw.SavePNG("frame.png")
```

`HeadlessWindow` methods: `Dispatch(input)` (runs `DispatchEvent` inside
`app.Update`, then draws if the window was invalidated), `Pixels()` (premultiplied
RGBA), `Image()` (`*image.NRGBA`), `SavePNG(path)`, `Window()` (the `*Window`,
for `Scene()`, `HitTest`, ...). `Pixels`/`Image` draw first if a frame is
pending. `HeadlessPlatform.Run` calls `onLaunch` with a fresh app and returns
immediately; `SetBackgroundColor` and `SetScaleFactor` affect windows opened
afterwards.

Inspecting the scene is also useful for assertions: `win.Scene()` exposes the
`Quads`, `Shadows`, `Sprites` and `Underlines` of the last frame. The
rasterizer draws them merged by `Order` (scene order, across kinds), so a
later quad covers earlier text; each primitive's `ContentMask` may carry
`CornerRadii` (a rounded `OverflowHidden` ancestor), which the rasterizer
applies as an anti-aliased mask.

`gpui.NewApp()` (nil platform) uses an internal stub: windows report their
requested size at scale 1 and discard frames. It is enough for entity/effect
tests that never look at pixels.

## Writing a new backend

Implement `Platform` and `PlatformWindow`. A skeleton that compiles against the
current API:

```go
type Platform struct {
    quit atomic.Bool
    app  *gpui.App
    win  *window
}

func (p *Platform) Name() string { return "mybackend" }
func (p *Platform) Quit()        { p.quit.Store(true) } // goroutine-safe

func (p *Platform) OpenWindow(o gpui.WindowOptions) (gpui.PlatformWindow, error) {
    p.win = &window{w: gpui.Pixels(o.Width), h: gpui.Pixels(o.Height), scale: 1}
    return p.win, nil
}

func (p *Platform) Run(onLaunch func(*gpui.App)) error {
    p.app = gpui.NewAppWithPlatform(p)
    onLaunch(p.app)
    for !p.quit.Load() && !p.app.IsQuitting() {
        win := p.app.Windows()[0]
        p.app.Update(func(app *gpui.App) {
            // poll the OS; for each event:
            win.DispatchEvent(gpui.MouseMoveEvent{Position: gpui.Point{X: 1, Y: 1}}, app)
        })
        if p.win.pending && p.win.onFrame != nil {
            p.win.pending = false
            p.app.Update(func(*gpui.App) { p.win.onFrame() })
        }
    }
    return nil
}

type window struct {
    w, h    gpui.Pixels
    scale   float32
    pending bool
    onFrame func()
    handler gpui.PlatformInputHandler
}

func (w *window) Bounds() (gpui.Pixels, gpui.Pixels) { return w.w, w.h }
func (w *window) ScaleFactor() float32               { return w.scale }
func (w *window) RequestFrame()                      { w.pending = true }
func (w *window) OnRequestFrame(f func())            { w.onFrame = f }
func (w *window) Present(pix []byte, pw, ph int)     { /* blit to the screen */ }
func (w *window) SetInputHandler(h gpui.PlatformInputHandler) { w.handler = h }
func (w *window) TakeInputHandler() gpui.PlatformInputHandler {
    h := w.handler
    w.handler = nil
    return h
}
func (w *window) Quit() {}
```

Checklist (mirrors what `ebitenplatform` does):

1. **Sizes.** `Bounds` is logical; the framebuffer handed to `Present` is
   `round(bounds * ScaleFactor)` in each dimension. On resize or scale change,
   update what `Bounds`/`ScaleFactor` report and call `RequestFrame`.
2. **Frames.** Implement `FrameRequester` and call the registered callback
   (inside `app.Update`) when pending; otherwise call `Window.Draw(app)`
   yourself. Coalesce `RequestFrame` calls.
3. **Input.** `Window.DispatchEvent` already runs inside `app.Update`; wrap a
   whole batch of events in one `app.Update` to flush effects once per batch. Translate to GPUI's conventions: logical-pixel
   positions, lower-case key names, Ctrl -> `Control`, Cmd/Super ->
   `ModPlatform`, wheel positive = down, text as
   `KeyDownEvent{PreferCharacterInput: true, Keystroke.KeyChar}` with a
   `Key` a binding can match (`"space"`, `shift-a` for `"A"`, else the char).
4. **Clicks.** Synthesize `ClickEvent` yourself: record `win.HitTest(pos)` on
   press and release and emit `ClickEvent{Up: &up}` when the ids are `Equal`.
   Compute `ClickCount` for multi-clicks.
5. **Text input.** `SetInputHandler` is called after each draw with the focused
   field (or `nil`); route IME/clipboard through it
   (`ReplaceTextInRange`, `SelectedTextRange`, ...). Ranges are UTF-16.
6. **Quit.** `Platform.Quit` and `App.Quit` can be called from any goroutine;
   observe the flag from your loop. `Window.Close()` calls
   `PlatformWindow.Quit()` after removing the window from `App.Windows`.
7. **Single goroutine.** Call into `gpui` only from the loop's goroutine.
8. **Present** gets premultiplied RGBA; convert if your display wants straight
   alpha or a different channel order.
