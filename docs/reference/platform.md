# Reference: platform, headless rendering, scene and clipboard

Source: `gpui/platform.go`, `gpui/headless.go`, `gpui/scene.go`,
`gpui/raster.go`, `gpui/clipboard.go`, `gpui/ebitenplatform/`.

Back to the [reference index](README.md). Concepts:
[platforms](../concepts/platform.md). Guide:
[testing](../guides/testing.md).

## Platform interfaces

```text
type Platform interface {
    Run(onLaunch func(*App)) error                       // blocks until Quit; onLaunch runs once before the first frame
    OpenWindow(o WindowOptions) (PlatformWindow, error)
    Quit()                                               // must be goroutine-safe
    Name() string                                        // "macos", "windows", "linux", "headless"
}

type PlatformWindow interface {                          // all methods on the main goroutine
    Bounds() (w, h Pixels)                               // logical pixels
    ScaleFactor() float32                                // device px per logical px
    RequestFrame()                                       // draw soon
    Present(pix []byte, w, h int)                        // premultiplied RGBA, 4*w*h bytes (device pixels)
    SetInputHandler(h PlatformInputHandler)              // nil removes
    TakeInputHandler() PlatformInputHandler
    Quit()                                               // close this window
}

type FrameRequester interface{ OnRequestFrame(f func()) }  // optional: the Window registers its draw routine here
```

`WindowOptions` / `DefaultWindowOptions` are in
[app-window.md](app-window.md#windowoptions). The pixel buffer given to
`Present` is owned by the rasterizer and rewritten on the next draw: copy it
if you keep it.

### Text input surface

```text
type IntRange struct { Start, End int }               // half-open range of UTF-16 code units
type UTF16Selection struct { Range IntRange; Reversed bool; IsNil bool }   // IsNil: "no selection"

type PlatformInputHandler interface {
    TextForRange(rng IntRange) (string, bool)
    SelectedTextRange() (UTF16Selection, bool)
    MarkedTextRange() (IntRange, bool)
    UnmarkText()
    ReplaceTextInRange(rng *IntRange, text string)     // nil range = insert at the caret
    BoundsForRange(rng IntRange, elementBounds Bounds) (Bounds, bool)
    CharacterIndexForPoint(p Point, elementBounds Bounds) (int, bool)
    SetSelectedTextRange(rng IntRange)
    TextLengthUTF16() (int, bool)
    AcceptsTextInput() bool
}
```

`TextInput` implements it. `PlatformInput` (the event interface) is in
[events.md](events.md#input-events).

`TextInput.TextForRange` rejects invalid UTF-16/rune endpoints. Its range setters
and explicit replacements clamp and snap each endpoint down to rune boundaries;
reversed setter endpoints set caret orientation. `BoundsForRange` and
`CharacterIndexForPoint` use the rendered full-line shaped metrics (rune estimates
before rendering). See [selection](../concepts/events-and-input.md#utf-16-ranges).

## HeadlessPlatform and HeadlessWindow

`HeadlessPlatform` runs the identical pipeline with in-memory windows. The
window does not implement `FrameRequester`: `RequestFrame` just marks it
pending and the next `Dispatch`, `Pixels` or `Image` call draws.

```text
func NewHeadlessPlatform() *HeadlessPlatform      // scale 1, white background

func (p *HeadlessPlatform) Name() string                           // "headless"
func (p *HeadlessPlatform) Run(onLaunch func(*App)) error          // calls onLaunch with a fresh App, then returns
func (p *HeadlessPlatform) OpenWindow(o WindowOptions) (PlatformWindow, error)
func (p *HeadlessPlatform) Quit()
func (p *HeadlessPlatform) SetBackgroundColor(c Rgba)              // windows opened afterwards
func (p *HeadlessPlatform) SetScaleFactor(s float32)               // windows opened afterwards
func (p *HeadlessPlatform) Windows() []*HeadlessWindow             // creation order
```

| `HeadlessWindow` method | Meaning |
|---|---|
| `Dispatch(input PlatformInput)` | Feed a synthetic input event through the real hit-testing and dispatch path, then draw if the event invalidated the frame. Clicks are not synthesised: send `ClickEvent` yourself. |
| `Pixels() []byte` | Latest frame, premultiplied RGBA, `4*w*h` device-pixel bytes; draws first if the window is invalid. |
| `Image() *image.NRGBA` | Latest frame, non-premultiplied; use `NRGBAAt(x, y)` for assertions. |
| `SavePNG(path string) error` | Write the latest frame as a PNG. |
| `Window() *Window` | The `*Window` this platform window backs (for `Scene()`, `HitTest`, ...). |
| `Bounds() (Pixels, Pixels)` | Logical size. |
| `ScaleFactor() float32` | Device scale. |
| `RequestFrame()` | Mark a redraw wanted. |
| `Present(pix []byte, w, h int)` | Store a copy of the frame. |
| `SetInputHandler(h)` / `TakeInputHandler()` | Install/take the focused text field's surface. |
| `Quit()` | No-op. |

`NewApp()` with no platform uses an internal stub whose windows discard frames
and report their requested size at scale 1; enough for entity/effect tests.

## Rendering helpers

```text
func RenderToPNG(path string, w, h int, scale float32, root EntityRoot, opts ...RenderOption) error
func RenderToImage(w, h int, scale float32, root EntityRoot, opts ...RenderOption) (*image.NRGBA, error)

type RenderOption func(*renderConfig)
func WithBackgroundColor(c Rgba) RenderOption    // clear colour (default white)
func WithKeymap(k *Keymap) RenderOption          // install k before the first draw
func WithTitle(t string) RenderOption            // window title
```

`w x h` are logical pixels; the output is `round(w*scale) x round(h*scale)`.
`RenderToImage` returns the non-premultiplied image. Both build the root on a
headless platform, so no window, display or GPU is needed. There is no option
to seed focus or scroll state yet.

## Scene and rasterizer

```text
type Scene struct {
    Quads      []Quad
    Shadows    []Shadow
    Sprites    []GlyphSprite
    Underlines []Underline
}
func (s *Scene) InsertQuad(q Quad)
func (s *Scene) InsertShadow(sh Shadow)
func (s *Scene) InsertSprite(g GlyphSprite)
func (s *Scene) InsertUnderline(u Underline)
func (s *Scene) PushLayer()            // everything inserted afterwards paints above everything before
func (s *Scene) PrimitiveCount() int
func (s *Scene) Clear()
```

Primitives with `Order == 0` get the scene's next auto order, so order is
paint (tree) order across all four kinds; on equal order the kinds draw
shadow, quad, underline, sprite.

| Primitive | Fields |
|---|---|
| `Quad` | `Order uint32`, `Bounds`, `ContentMask`, `Background BackgroundKind`, `Fill Rgba`, `BorderColor Rgba`, `BorderWidths Edges`, `CornerRadii Corners`, `Gradient Gradient` |
| `Shadow` | `Order`, `Bounds` (shape before blur), `CornerRadii`, `ContentMask`, `Color`, `BlurRadius`, `Inset bool`, `Clip Bounds`, `ClipRadii Corners` (the element shape an inset shadow is drawn in) |
| `GlyphSprite` | `Order`, `Bounds`, `ContentMask`, `Mask *GlyphMask`, `Color Rgba` |
| `Underline` | `Order`, `Bounds`, `ContentMask`, `Color`, `Thickness Pixels` (a rule drawn at the bottom of `Bounds`) |
| `ContentMask` | `Bounds Bounds`, `CornerRadii Corners`. The zero value clips everything away; use `InfiniteContentMask()` for "unclipped". |

```text
func InfiniteContentMask() ContentMask

type Rasterizer struct{ /* opaque */ }
func NewRasterizer(w, h int) *Rasterizer       // buffer starts transparent
func (r *Rasterizer) Reset(clear Rgba)         // fill with the premultiplied clear colour
func (r *Rasterizer) Render(s *Scene)          // draw every primitive in scene order
func (r *Rasterizer) Pixels() []byte           // premultiplied RGBA, 4*w*h
func (r *Rasterizer) Image() *image.NRGBA      // un-premultiplied copy
func (r *Rasterizer) Size() (int, int)
```

`GlyphMask` is described in [text-and-fonts.md](text-and-fonts.md#glyph-masks).

## ebitenplatform

Package `go-gpui/gpui/ebitenplatform`: `gpui.Platform` on ebiten (no cgo).
Supports a single window; a second `OpenWindow` returns an error.

```text
func New(opts ...Option) gpui.Platform
type Option func(*Platform)
func WithSnapshotKey(keystroke string) Option   // keymap syntax; "" disables; panics on a malformed keystroke
func DefaultSnapshotKey() string                // "cmd-shift-s" on darwin, "ctrl-shift-s" elsewhere

type Platform struct{ /* opaque */ }            // Name() ("macos"/"windows"/"linux"), OpenWindow, Run, Quit
```

`Run` creates the `App`, calls your launch callback, then blocks in
`ebiten.RunGame`; `ebiten.Termination` becomes a clean `nil` return. The
snapshot shortcut saves the last presented frame as
`snapshot-YYYYMMDD-HHMMSS.png` in the working directory and is swallowed (not
dispatched to the app).

## Clipboard

```text
func ClipboardAvailable() bool                 // an OS clipboard tool was found
func ClipboardRead() (string, error)
func ClipboardWrite(s string) error
```

They shell out (`pbpaste`/`pbcopy`, PowerShell, `wl-paste`/`wl-copy`, `xclip`,
`xsel`) with a one-second timeout and **block**: call them off the UI loop. The
ebiten backend already wires `cmd-c/x/v` (macOS) and `ctrl-c/x/v` (elsewhere)
to the focused text input on a worker goroutine, and those shortcuts are not
dispatched as key events.
