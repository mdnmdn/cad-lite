// Package ebitenplatform implements gpui.Platform on top of ebiten.
package ebitenplatform

import (
	"errors"
	"log"
	"os"
	"runtime"
	"sync/atomic"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/inpututil"

	"cad-lite/internal/gpui"
)

// Platform is the ebiten-backed gpui.Platform. It supports a single window.
type Platform struct {
	win  *window
	quit atomic.Bool
	// snapshotKey is the frame-snapshot binding; nil disables it.
	snapshotKey *gpui.Keystroke
}

// New returns an ebiten-backed gpui.Platform configured by opts. The
// frame-snapshot shortcut defaults to DefaultSnapshotKey.
func New(opts ...Option) gpui.Platform {
	p := &Platform{}
	WithSnapshotKey(DefaultSnapshotKey())(p)
	for _, o := range opts {
		o(p)
	}
	return p
}

// Name reports the host OS name: "macos", "windows" or "linux".
func (p *Platform) Name() string {
	switch runtime.GOOS {
	case "darwin":
		return "macos"
	default:
		return runtime.GOOS
	}
}

// Quit asks the run loop to terminate on the next update.
func (p *Platform) Quit() { p.quit.Store(true) }

// OpenWindow configures the single ebiten window from o.
func (p *Platform) OpenWindow(o gpui.WindowOptions) (gpui.PlatformWindow, error) {
	if p.win != nil {
		return nil, errors.New("ebitenplatform: only one window is supported")
	}
	if o.Width <= 0 {
		o.Width = 800
	}
	if o.Height <= 0 {
		o.Height = 600
	}
	scale := float32(1)
	if m := ebiten.Monitor(); m != nil {
		if s := float32(m.DeviceScaleFactor()); s > 0 {
			scale = s
		}
	}
	ebiten.SetWindowTitle(o.Title)
	ebiten.SetWindowSize(int(o.Width), int(o.Height))
	if o.Resizable {
		ebiten.SetWindowResizingMode(ebiten.WindowResizingModeEnabled)
	} else {
		ebiten.SetWindowResizingMode(ebiten.WindowResizingModeDisabled)
	}
	p.win = &window{p: p, w: gpui.Pixels(o.Width), h: gpui.Pixels(o.Height), scale: scale}
	return p.win, nil
}

// Run creates the App, calls onLaunch, then blocks in ebiten.RunGame until
// the app quits. On macOS it first installs the open-documents handler, whose
// requests reach App.OpenPaths.
func (p *Platform) Run(onLaunch func(*gpui.App)) error {
	ebiten.SetScreenClearedEveryFrame(false)
	app := gpui.NewAppWithPlatform(p)
	if onLaunch != nil {
		onLaunch(app)
	}
	g := &game{p: p, app: app, clip: newClipWorker(), wheel: wheelClassifier{pixels: runtime.GOOS == "js"}, pinch: newPinchQueue(), opens: newOpenQueue()}
	if err := installOpenFilesHandler(g.opens); err != nil && !errors.Is(err, errOpenFilesUnsupported) {
		log.Printf("gpui: opening files from the OS unavailable: %v", err)
	}
	err := ebiten.RunGame(g)
	if errors.Is(err, ebiten.Termination) {
		return nil
	}
	return err
}

// window is the single PlatformWindow.
type window struct {
	p        *Platform
	w, h     gpui.Pixels
	scale    float32
	pending  bool
	deadline time.Time
	onFrame  func()
	handler  gpui.PlatformInputHandler

	pix     []byte
	pixW    int
	pixH    int
	fresh   bool
	screenW int
	screenH int
}

// Bounds returns the window size in logical pixels.
func (w *window) Bounds() (gpui.Pixels, gpui.Pixels) { return w.w, w.h }

// ScaleFactor returns the monitor's device pixel ratio.
func (w *window) ScaleFactor() float32 { return w.scale }

// RequestFrame marks the window dirty so the next update redraws it.
func (w *window) RequestFrame() { w.pending = true }

// RequestFrameAt replaces the timed deadline; zero clears it. Update queues the
// core UI pump when it becomes due without a background goroutine.
func (w *window) RequestFrameAt(deadline time.Time) { w.deadline = deadline }

// OnRequestFrame registers the window's draw routine.
func (w *window) OnRequestFrame(f func()) { w.onFrame = f }

// Present keeps a reference to the rasterized frame for the next ebiten Draw.
// No copy is needed: the buffer only changes in Window.Draw, which runs inside
// Update on the same goroutine as Draw.
func (w *window) Present(pix []byte, pw, ph int) {
	w.pix = pix
	w.pixW, w.pixH = pw, ph
	w.fresh = true
}

// SetInputHandler installs the focused text field's editing surface.
func (w *window) SetInputHandler(h gpui.PlatformInputHandler) { w.handler = h }

// TakeInputHandler removes and returns the installed input handler.
func (w *window) TakeInputHandler() gpui.PlatformInputHandler {
	h := w.handler
	w.handler = nil
	return h
}

// Quit closes the window by terminating the run loop.
func (w *window) Quit() { w.p.quit.Store(true) }

type game struct {
	p   *Platform
	app *gpui.App

	pointer  pointerTracker
	wheel    wheelClassifier
	pinch    *pinchQueue
	pinchSet bool
	// opens carries the OS's open-documents requests (macOS) to Update.
	opens    *openQueue
	lastMods gpui.Modifiers
	clicks   clickTracker
	keyBuf   []ebiten.Key
	runeBuf  []rune
	clip     *clipWorker
	// swallowed holds keys whose press was consumed by the platform (the
	// snapshot shortcut), so their release is not dispatched either.
	swallowed map[ebiten.Key]bool
}

func (g *game) Layout(ow, oh int) (int, int) {
	w := g.p.win
	if w == nil {
		return ow, oh
	}
	scale := w.scale
	if m := ebiten.Monitor(); m != nil {
		if s := float32(m.DeviceScaleFactor()); s > 0 {
			scale = s
		}
	}
	dw, dh := int(float32(ow)*scale), int(float32(oh)*scale)
	if dw < 1 {
		dw = 1
	}
	if dh < 1 {
		dh = 1
	}
	if dw != w.screenW || dh != w.screenH || scale != w.scale {
		w.screenW, w.screenH, w.scale = dw, dh, scale
		w.w, w.h = gpui.Pixels(float32(dw)/scale), gpui.Pixels(float32(dh)/scale)
		w.pending = true
	}
	return dw, dh
}

func (g *game) Update() error {
	if g.p.quit.Load() || g.app.IsQuitting() {
		return ebiten.Termination
	}
	w := g.p.win
	wins := g.app.Windows()
	if w == nil || len(wins) == 0 {
		return nil
	}
	win := wins[0]
	g.app.Update(func(app *gpui.App) {
		g.clip.drain(func(r clipResult) {
			applyClipboard(r, w.handler, func(ev gpui.PlatformInput) { win.DispatchEvent(ev, app) })
		})
		g.pollInput(win, app, w)
		for _, paths := range g.opens.drain() {
			app.OpenPaths(paths)
		}
	})
	if g.p.quit.Load() || g.app.IsQuitting() {
		return ebiten.Termination
	}
	if !w.deadline.IsZero() && !time.Now().Before(w.deadline) {
		w.deadline = time.Time{}
		w.pending = true
	}
	if w.pending && w.onFrame != nil {
		w.pending = false
		g.app.Update(func(*gpui.App) { w.onFrame() })
	}
	return nil
}

func (g *game) Draw(screen *ebiten.Image) {
	w := g.p.win
	if w == nil || !w.fresh {
		return
	}
	b := screen.Bounds()
	if w.pixW != b.Dx() || w.pixH != b.Dy() || len(w.pix) != 4*w.pixW*w.pixH {
		w.pending = true
		return
	}
	screen.WritePixels(w.pix)
	w.fresh = false
}

func (g *game) modifiers() gpui.Modifiers {
	return modifiersFor(
		ebiten.IsKeyPressed(ebiten.KeyShift),
		ebiten.IsKeyPressed(ebiten.KeyControl),
		ebiten.IsKeyPressed(ebiten.KeyAlt),
		ebiten.IsKeyPressed(ebiten.KeyMeta))
}

func (g *game) pollInput(win *gpui.Window, app *gpui.App, w *window) {
	mods := g.modifiers()
	if mods != g.lastMods {
		g.lastMods = mods
		win.DispatchEvent(gpui.ModifiersChangedEvent{Modifiers: mods}, app)
	}

	if !g.pinchSet {
		g.pinchSet = true
		installWheelCtrlListener()
		if err := installPinchMonitor(g.pinch); err != nil && !errors.Is(err, errPinchUnsupported) {
			log.Printf("gpui: pinch gestures unavailable: %v", err)
		}
	}
	for _, e := range g.pinch.drain() {
		win.DispatchEvent(e, app)
	}

	cx, cy := ebiten.CursorPositionF()
	pos := gpui.Point{X: gpui.Pixels(float32(cx) / w.scale), Y: gpui.Pixels(float32(cy) / w.scale)}
	// GLFW moves the cursor to the drop point before it reports a drop.
	if dropped := ebiten.DroppedFiles(); dropped != nil {
		if paths := droppedPaths(dropped, os.ReadDir); len(paths) > 0 {
			win.DispatchEvent(gpui.FileDropEvent{Position: pos, Paths: paths, Modifiers: mods}, app)
		}
	}
	wx, wy := ebiten.Wheel()
	interacted := wx != 0 || wy != 0 ||
		inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonLeft) ||
		inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonMiddle) ||
		inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonRight)
	if g.pointer.observe(pos, interacted) {
		win.DispatchEvent(gpui.MouseMoveEvent{Position: pos, Modifiers: mods}, app)
	}

	buttons := [...]struct {
		eb ebiten.MouseButton
		gb gpui.MouseButton
	}{
		{ebiten.MouseButtonLeft, gpui.MouseLeft},
		{ebiten.MouseButtonMiddle, gpui.MouseMiddle},
		{ebiten.MouseButtonRight, gpui.MouseRight},
	}
	for _, b := range buttons {
		if inpututil.IsMouseButtonJustPressed(b.eb) {
			hit, ok := win.HitTest(pos)
			n := g.clicks.press(b.gb, pos, hit, ok, time.Now())
			win.DispatchEvent(gpui.MouseDownEvent{Position: pos, Button: b.gb, Modifiers: mods, ClickCount: n}, app)
		}
		if inpututil.IsMouseButtonJustReleased(b.eb) {
			hit, ok := win.HitTest(pos)
			clicked, n := g.clicks.release(b.gb, hit, ok)
			up := gpui.MouseUpEvent{Position: pos, Button: b.gb, Modifiers: mods, ClickCount: n}
			win.DispatchEvent(up, app)
			if clicked {
				win.DispatchEvent(gpui.ClickEvent{Up: &up}, app)
			}
		}
	}

	if wx != 0 || wy != 0 {
		dx, dy, precise := g.wheel.classify(wx, wy, time.Now())
		wm := mods
		if takeWheelCtrl() {
			wm |= gpui.Control
		}
		win.DispatchEvent(gpui.ScrollWheelEvent{Position: pos, DeltaX: dx, DeltaY: dy, Modifiers: wm, Precise: precise}, app)
	}

	g.pollKeys(win, app, w, mods)
}

func (g *game) pollKeys(win *gpui.Window, app *gpui.App, w *window, mods gpui.Modifiers) {
	g.keyBuf = inpututil.AppendJustReleasedKeys(g.keyBuf[:0])
	for _, k := range g.keyBuf {
		if g.swallowed[k] {
			delete(g.swallowed, k)
			continue
		}
		if name, _, ok := keyName(k); ok {
			win.DispatchEvent(gpui.KeyUpEvent{Keystroke: gpui.Keystroke{Modifiers: mods, Key: name}}, app)
		}
	}

	g.keyBuf = inpututil.AppendPressedKeys(g.keyBuf[:0])
	for _, k := range g.keyBuf {
		name, printable, ok := keyName(k)
		if !ok {
			continue
		}
		if !shouldFire(inpututil.KeyPressDuration(k), !printable) {
			continue
		}
		if printable && !isShortcut(runtime.GOOS, mods) {
			continue
		}
		if printable && inpututil.KeyPressDuration(k) != 1 {
			continue
		}
		ks := gpui.Keystroke{Modifiers: mods, Key: name}
		if inpututil.KeyPressDuration(k) == 1 && isSnapshot(g.p.snapshotKey, ks) {
			if g.swallowed == nil {
				g.swallowed = make(map[ebiten.Key]bool)
			}
			g.swallowed[k] = true
			snapshot(".", w.pix, w.pixW, w.pixH, time.Now())
			continue
		}
		if g.swallowed[k] {
			continue
		}
		if op := clipboardOp(runtime.GOOS, name, mods); op != "" &&
			startClipboard(g.clip, op, w.handler, gpui.ClipboardRead, gpui.ClipboardWrite) {
			continue
		}
		win.DispatchEvent(gpui.KeyDownEvent{Keystroke: ks}, app)
	}

	if !isShortcut(runtime.GOOS, mods) {
		g.runeBuf = ebiten.AppendInputChars(g.runeBuf[:0])
		// Shift, Alt/Option and AltGr's Ctrl were consumed composing the rune.
		charMods := mods &^ (gpui.Shift | gpui.Alt | gpui.Control)
		for _, r := range printableRunes(g.runeBuf) {
			win.DispatchEvent(gpui.KeyDownEvent{
				Keystroke:            charKeystroke(r, charMods),
				PreferCharacterInput: true,
			}, app)
		}
	}
}
