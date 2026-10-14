# Testing

go-gpui is built to be tested without a window, a display or a GPU. The
`HeadlessPlatform` runs the same layout, prepaint, paint and raster pipeline as
the ebiten backend and keeps the last frame in memory; you drive it with
synthetic input events and assert on your state, the pixels, the scene or a
saved PNG. The repo's own suite (`*_test.go`) uses exactly these
techniques.

Background: [platform concepts](../concepts/platform.md#headless-platform-and-rendertopng),
[reference for the headless types](../reference/platform.md#headlessplatform-and-headlesswindow),
[architecture](../architecture.md#the-headless-platform).

The snippets on this page share one test file; its imports are:

```text
import (
	"flag"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"cad-lite/internal/gpui"
)
```

## The view under test

```go
type Counter struct {
	count int
	input *gpui.TextInput
}

func NewCounter() *Counter { return &Counter{input: gpui.NewTextInput("name")} }

func (c *Counter) Render(w *gpui.Window, cx *gpui.App) gpui.IntoElement {
	inc := gpui.NewDiv().ID("inc").SizeWH(80, 40).Bg(gpui.Blue500).
		Hover(func(s *gpui.StyledMixin) *gpui.StyledMixin { return s.Bg(gpui.Red500) }).
		OnClick(func(_ *gpui.ClickEvent, w *gpui.Window, _ *gpui.App) {
			c.count++
			w.Refresh()
		}).
		Child(gpui.NewText(fmt.Sprint(c.count)).TextColor(gpui.White))
	return gpui.NewDiv().FlexCol().Gap(8).P(10).Child(inc, c.input.Render(w, cx))
}
```

## A headless harness

Open the root on a `HeadlessPlatform`, then grab the `*HeadlessWindow`.
`OpenWindow` draws once immediately.

```go
func open(t *testing.T, w, h float32, scale float32, root gpui.EntityRoot) (*gpui.HeadlessWindow, *gpui.App) {
	t.Helper()
	p := gpui.NewHeadlessPlatform()
	p.SetScaleFactor(scale) // optional; defaults to 1
	app := gpui.NewAppWithPlatform(p)
	err := app.OpenWindow(gpui.WindowOptions{Width: w, Height: h},
		func(*gpui.Window, *gpui.App) gpui.EntityRoot { return root })
	if err != nil {
		t.Fatal(err)
	}
	return p.Windows()[0], app
}
```

`SetScaleFactor` and `SetBackgroundColor` apply to windows opened afterwards.
Register key bindings with `app.Keymap().AddBindings(...)` before dispatching
keys.

## Dispatching input

`HeadlessWindow.Dispatch(input)` feeds an event through the real hit-testing
and dispatch path (inside `App.Update`, so effects flush once), then draws if
the event invalidated the frame. Events are passed by value.

**The platform does not synthesise clicks.** A real press produces
`MouseDownEvent`, `MouseUpEvent`, then `ClickEvent{Up: &up}`; `OnClick`
handlers only run on the `ClickEvent`, so send all three:

```go
func click(hw *gpui.HeadlessWindow, x, y float32) {
	pos := gpui.Point{X: gpui.Pixels(x), Y: gpui.Pixels(y)}
	hw.Dispatch(gpui.MouseDownEvent{Position: pos, Button: gpui.MouseLeft, ClickCount: 1})
	up := gpui.MouseUpEvent{Position: pos, Button: gpui.MouseLeft, ClickCount: 1}
	hw.Dispatch(up)
	hw.Dispatch(gpui.ClickEvent{Up: &up})
}

func hover(hw *gpui.HeadlessWindow, x, y float32) {
	hw.Dispatch(gpui.MouseMoveEvent{Position: gpui.Point{X: gpui.Pixels(x), Y: gpui.Pixels(y)}})
}

func wheel(hw *gpui.HeadlessWindow, x, y, dy float32) {
	hw.Dispatch(gpui.ScrollWheelEvent{Position: gpui.Point{X: gpui.Pixels(x), Y: gpui.Pixels(y)}, DeltaY: gpui.Pixels(dy)})
}
```

Positions are logical pixels. Hover needs a `MouseMoveEvent`; the mouse-down
also focuses the innermost focusable element under the pointer, which is how a
test focuses a `TextInput`.

Keys: printable text arrives as a `KeyDownEvent` with `PreferCharacterInput`
and `Keystroke.KeyChar` set (what the ebiten backend sends for typing);
named keys and shortcuts need only the `Keystroke`.

```go
func typeText(hw *gpui.HeadlessWindow, s string) {
	for _, r := range s {
		c := string(r)
		hw.Dispatch(gpui.KeyDownEvent{
			Keystroke:            gpui.Keystroke{Key: c, KeyChar: &c},
			PreferCharacterInput: true,
		})
	}
}

func press(hw *gpui.HeadlessWindow, keystroke string) {
	ks, err := gpui.ParseKeystroke(keystroke) // "enter", "ctrl-s", "shift-tab"
	if err != nil {
		panic(err)
	}
	hw.Dispatch(gpui.KeyDownEvent{Keystroke: ks})
}

func TestInteraction(t *testing.T) {
	c := NewCounter()
	hw, _ := open(t, 200, 150, 1, c)

	click(hw, 20, 20) // inside the 80x40 button at (10, 10)
	if c.count != 1 {
		t.Fatalf("count = %d, want 1", c.count)
	}

	click(hw, 50, 80) // focus the TextInput, then type into it
	typeText(hw, "Ada")
	press(hw, "backspace")
	if got := c.input.Text(); got != "Ad" {
		t.Fatalf("text = %q, want %q", got, "Ad")
	}
}
```

If a test dispatches an event while nothing is focused, the keymap and
`OnAction` handlers see only the root element. To start from a focused element,
click it, or call `w.RequestFocus(handle)` from the view's first `Render` (as
`examples/tab_stop` does for its `-focus` flag).

## Asserting on pixels

`hw.Image()` returns the latest frame as a non-premultiplied `*image.NRGBA`
(drawing first if the window is invalid), so `NRGBAAt(x, y)` compares directly
against the `Rgba` you styled with. Coordinates are **device** pixels: multiply
logical positions by the scale. `hw.Pixels()` is the premultiplied RGBA byte
buffer (`4*w*h`), only useful if you want to hash or diff frames.

```go
func TestPixels(t *testing.T) {
	c := NewCounter()
	hw, _ := open(t, 200, 150, 2, c) // scale 2: image is 400 x 300

	img := hw.Image()
	if b := img.Bounds(); b.Dx() != 400 || b.Dy() != 300 {
		t.Fatalf("size = %v", b)
	}
	want := color.NRGBA{R: gpui.Blue500.R, G: gpui.Blue500.G, B: gpui.Blue500.B, A: 255}
	if got := img.NRGBAAt(2*20, 2*25); got != want {
		t.Errorf("button pixel = %v, want %v", got, want)
	}

	hover(hw, 20, 20) // hover styles apply and redraw automatically
	red := hw.Image().NRGBAAt(2*20, 2*25)
	if red.R < 200 || red.B > 100 {
		t.Errorf("hovered pixel = %v, want red", red)
	}
}
```

Tips for stable pixel tests:

- Sample the **interior** of a solid box, away from anti-aliased edges, rounded
  corners and text.
- Compare exact colours only for opaque fills. For translucent fills, shadows
  and gradients assert ranges (`got.R > 200`) or compare two pixels.
- Assert on text by structure (the scene or your state), not by glyph pixels:
  glyph shapes depend on the system font.
- Overlays and ordering: draw order is tree order across all primitive kinds, so
  a later sibling that overlaps earlier text covers it. Test that by sampling a
  pixel where the two overlap.

## Asserting on the scene and layout

`hw.Window().Scene()` exposes the last frame's primitives: `Quads`, `Shadows`,
`Sprites`, `Underlines`. That is a robust way to check "a quad with this fill
exists" or "no glyphs were drawn" without pixel coordinates.
`Window.HitTest(p)` tells you which element owns a point.

```go
func TestScene(t *testing.T) {
	hw, _ := open(t, 200, 150, 1, NewCounter())
	scene := hw.Window().Scene()
	found := false
	for _, q := range scene.Quads {
		if q.Background == gpui.BackgroundSolid && q.Fill == gpui.Blue500 && q.Bounds.Size.W == 80 {
			found = true
		}
	}
	if !found {
		t.Fatalf("no blue 80px quad among %d quads", len(scene.Quads))
	}
	if _, ok := hw.Window().HitTest(gpui.Point{X: 20, Y: 20}); !ok {
		t.Fatal("expected an element under the button")
	}
}
```

The layout engine can also be tested with no window at all: build a
`LayoutNode` tree and call `Layout`.

```go
func TestLayoutDirect(t *testing.T) {
	row := gpui.DefaultStyle()
	row.Direction = gpui.DirectionRow
	box := gpui.DefaultStyle()
	box.Width, box.Height = gpui.Px(40), gpui.Px(20)
	a, b := &gpui.LayoutNode{Style: box}, &gpui.LayoutNode{Style: box}
	root := &gpui.LayoutNode{Style: row, Children: []*gpui.LayoutNode{a, b}}
	gpui.Layout(root, gpui.Definite(300, 100))
	if b.Rect.Origin.X != 40 {
		t.Fatalf("second box at x=%v, want 40", b.Rect.Origin.X)
	}
}
```

## Entities and effects without a window

`gpui.NewApp()` uses a stub platform whose windows discard frames, which is all
entity, observer and event tests need. Wrap mutations in `app.Update`:
`Entity.Update` itself does not flush effects, so an observer only runs when
the outermost `App.Update` returns.

```go
func TestEntity(t *testing.T) {
	app := gpui.NewApp()
	type counter struct{ n int }
	ent := gpui.NewEntity(app, func(cx *gpui.Context[counter]) counter { return counter{} })

	notified := 0
	sub := gpui.ObserveOn(app, ent, func(gpui.Entity[counter], *gpui.App) { notified++ })
	defer sub.Close()

	app.Update(func(app *gpui.App) { // effects (observers) flush when the outermost Update returns
		ent.Update(app, func(c *counter, cx *gpui.Context[counter]) {
			c.n++
			cx.Notify()
		})
	})
	if ent.Read(app).n != 1 || notified != 1 {
		t.Fatalf("n=%d notified=%d", ent.Read(app).n, notified)
	}
	ent.Release(app)
	if ent.Read(app) != nil {
		t.Fatal("entity should be gone after Release")
	}
}
```

## Snapshots with RenderToPNG

`gpui.RenderToPNG(path, w, h, scale, root, opts...)` and `RenderToImage` build
the root, lay it out at `w x h` logical pixels, paint and rasterise at `scale`,
with no window. Options: `WithBackgroundColor`, `WithKeymap`, `WithTitle`.
Use them to look at a frame while developing (`-png out.png` in every
example) and to take golden images in tests.

```go
func TestSnapshotFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "counter.png")
	if err := gpui.RenderToPNG(path, 200, 150, 2, NewCounter(), gpui.WithBackgroundColor(gpui.White)); err != nil {
		t.Fatal(err)
	}
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	cfg, err := png.DecodeConfig(f)
	if err != nil || cfg.Width != 400 || cfg.Height != 300 {
		t.Fatalf("png = %+v, %v", cfg, err)
	}
}
```

`RenderToPNG` renders the first frame only: no pointer, no focus, no scroll
offset. To show a focused, hovered or scrolled state, either seed it from your
view's first `Render` (the examples do this behind flags), or open a headless
window, `Dispatch` events, and call `hw.SavePNG(path)` / `hw.Image()`.

## Golden images

A golden test renders a view and compares it to a checked-in PNG, with a
`-update` flag to rewrite goldens. A small tolerance absorbs anti-aliasing noise
between platforms.

```go
var update = flag.Bool("update", false, "rewrite golden images in testdata/")

func assertGolden(t *testing.T, name string, got *image.NRGBA) {
	t.Helper()
	path := filepath.Join("testdata", name+".png")
	if *update {
		if err := os.MkdirAll("testdata", 0o755); err != nil {
			t.Fatal(err)
		}
		f, err := os.Create(path)
		if err != nil {
			t.Fatal(err)
		}
		defer f.Close()
		if err := png.Encode(f, got); err != nil {
			t.Fatal(err)
		}
		return
	}
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("missing golden %s (run with -update): %v", path, err)
	}
	defer f.Close()
	want, err := png.Decode(f)
	if err != nil {
		t.Fatal(err)
	}
	if want.Bounds() != got.Bounds() {
		t.Fatalf("size %v, golden %v", got.Bounds(), want.Bounds())
	}
	const tol = 3 // per-channel
	bad := 0
	for y := got.Bounds().Min.Y; y < got.Bounds().Max.Y; y++ {
		for x := got.Bounds().Min.X; x < got.Bounds().Max.X; x++ {
			w := color.NRGBAModel.Convert(want.At(x, y)).(color.NRGBA)
			g := got.NRGBAAt(x, y)
			if absDiff(w.R, g.R) > tol || absDiff(w.G, g.G) > tol || absDiff(w.B, g.B) > tol || absDiff(w.A, g.A) > tol {
				bad++
			}
		}
	}
	if bad > 0 {
		t.Errorf("%d pixels differ from %s", bad, path)
	}
}

func absDiff(a, b uint8) int {
	if a > b {
		return int(a - b)
	}
	return int(b - a)
}

func TestGolden(t *testing.T) {
	img, err := gpui.RenderToImage(200, 150, 1, NewCounter(), gpui.WithBackgroundColor(gpui.White))
	if err != nil {
		t.Fatal(err)
	}
	assertGolden(t, "counter", img)
}
```

Workflow: `go test -run TestGolden -update` once, look at
`testdata/counter.png`, commit it, and run plain `go test` from then on.

Practical notes:

- The repository's `.gitignore` contains `*.png`. To commit goldens add an
  exception such as `!**/testdata/*.png`.
- **Text depends on the system font.** Glyph shapes and metrics differ between
  machines, so goldens with text are fragile. Either keep goldens text-free,
  or pin a font for the test binary by setting `GPUI_FONT` to a bundled file
  in `TestMain`, before the first draw:

  ```text
  func TestMain(m *testing.M) {
  	os.Setenv("GPUI_FONT", "testdata/DejaVuSans.ttf")
  	os.Exit(m.Run())
  }
  ```

- Render at scale 1 for compact goldens, and add one scale-2 case if you care
  about HiDPI text.
- `go run ./examples/<name> -png out.png` renders an example's first frame
  (the repo's examples are the quickest way to eyeball a change).

## Benchmarks

`bench_test.go` benchmarks the full frame pipeline on two realistic views
(a todo list and a gallery), at scale 1 and 2, plus the idle and hover paths:

| Benchmark | Measures |
|---|---|
| `BenchmarkDrawTodoScale2`, `BenchmarkDrawTodoScale1` | One `Window.Draw`: render, layout, prepaint, paint, rasterise at device scale, present. |
| `BenchmarkDrawGalleryScale2` | The same for a styling showcase (colours, corners, flex rows, styled runs, wrapping, nested cards). |
| `BenchmarkIdleTodoScale2` | Fetching the frame when nothing changed; no redraw may happen. |
| `BenchmarkHoverMoveTodoScale2` | A mouse move over an unstyled list; must not redraw. |

```
go test . -run '^$' -bench . -benchmem
go test . -run '^$' -bench DrawTodo -benchtime 2s -cpuprofile cpu.out
```

Write your own the same way: open a headless window, then loop on `Draw`.

```go
func BenchmarkMyView(b *testing.B) {
	p := gpui.NewHeadlessPlatform()
	app := gpui.NewAppWithPlatform(p)
	if err := app.OpenWindow(gpui.WindowOptions{Width: 480, Height: 640},
		func(*gpui.Window, *gpui.App) gpui.EntityRoot { return NewCounter() }); err != nil {
		b.Fatal(err)
	}
	win := p.Windows()[0].Window()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		win.Draw(app)
	}
}
```

Frame cost is dominated by the CPU rasteriser; there is no damage tracking, so
a frame costs the same whether one pixel or all changed. See the performance
notes in [architecture](../architecture.md#performance-notes).

## Race detector

```
go test -race ./...
```

The UI core is single-goroutine by design and unsynchronised, so tests should
call into `gpui` from the test goroutine only. Things that are safe from other
goroutines: `App.Quit` and `App.IsQuitting`. Anything else (updating an entity
from a worker, say) must be marshalled onto the UI goroutine by your code;
`Context.Spawn` does not start a goroutine. A `-race` failure in your tests
almost always means a goroutine touched an `App`, `Window` or entity directly.

## Conventions and verification

The repo's checklist (`gofmt -l .` clean, `go vet ./...`, `go test ./...`),
the placement rules for tests (`*_test.go`, package `gpui_test` unless
internals are needed) and the doc-snippet rule are in
[contributing](contributing.md#verification).
