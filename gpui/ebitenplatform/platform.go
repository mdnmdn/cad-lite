// Package ebitenplatform implements gpui.Platform on top of ebiten.
package ebitenplatform

import (
	"errors"
	"runtime"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/inpututil"

	"go-gpui/gpui"
)

// Platform is the ebiten-backed gpui.Platform. It supports a single window.
type Platform struct {
	win  *window
	quit bool
}

// New returns an ebiten-backed gpui.Platform.
func New() gpui.Platform { return &Platform{} }

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
func (p *Platform) Quit() { p.quit = true }

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
// the app quits.
func (p *Platform) Run(onLaunch func(*gpui.App)) error {
	ebiten.SetScreenClearedEveryFrame(false)
	app := gpui.NewAppWithPlatform(p)
	if onLaunch != nil {
		onLaunch(app)
	}
	g := &game{p: p, app: app}
	err := ebiten.RunGame(g)
	if errors.Is(err, ebiten.Termination) {
		return nil
	}
	return err
}

// window is the single PlatformWindow.
type window struct {
	p       *Platform
	w, h    gpui.Pixels
	scale   float32
	pending bool
	onFrame func()
	handler gpui.PlatformInputHandler

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

// OnRequestFrame registers the window's draw routine.
func (w *window) OnRequestFrame(f func()) { w.onFrame = f }

// Present stores the rasterized frame for the next ebiten Draw.
func (w *window) Present(pix []byte, pw, ph int) {
	w.pix = append(w.pix[:0], pix...)
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
func (w *window) Quit() { w.p.quit = true }

type game struct {
	p   *Platform
	app *gpui.App

	lastPos  gpui.Point
	havePos  bool
	lastMods gpui.Modifiers
	clicks   clickTracker
	keyBuf   []ebiten.Key
	runeBuf  []rune
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
	if g.p.quit || g.app.IsQuitting() {
		return ebiten.Termination
	}
	w := g.p.win
	wins := g.app.Windows()
	if w == nil || len(wins) == 0 {
		return nil
	}
	win := wins[0]
	g.app.Update(func(app *gpui.App) {
		g.pollInput(win, app, w)
	})
	if g.p.quit || g.app.IsQuitting() {
		return ebiten.Termination
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
	return modifiersFor(runtime.GOOS,
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

	cx, cy := ebiten.CursorPositionF()
	pos := gpui.Point{X: gpui.Pixels(float32(cx) / w.scale), Y: gpui.Pixels(float32(cy) / w.scale)}
	if !g.havePos || pos != g.lastPos {
		g.havePos = true
		g.lastPos = pos
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
			hit, _ := win.HitTest(pos)
			n := g.clicks.press(b.gb, pos, hit, time.Now())
			win.DispatchEvent(gpui.MouseDownEvent{Position: pos, Button: b.gb, Modifiers: mods, ClickCount: n}, app)
		}
		if inpututil.IsMouseButtonJustReleased(b.eb) {
			hit, _ := win.HitTest(pos)
			clicked, n := g.clicks.release(b.gb, hit)
			up := gpui.MouseUpEvent{Position: pos, Button: b.gb, Modifiers: mods, ClickCount: n}
			win.DispatchEvent(up, app)
			if clicked {
				win.DispatchEvent(gpui.ClickEvent{Up: &up}, app)
			}
		}
	}

	if wx, wy := ebiten.Wheel(); wx != 0 || wy != 0 {
		dx, dy := wheelDelta(wx, wy)
		win.DispatchEvent(gpui.ScrollWheelEvent{Position: pos, DeltaX: dx, DeltaY: dy, Modifiers: mods}, app)
	}

	g.pollKeys(win, app, w, mods)
}

func (g *game) pollKeys(win *gpui.Window, app *gpui.App, w *window, mods gpui.Modifiers) {
	g.keyBuf = inpututil.AppendJustReleasedKeys(g.keyBuf[:0])
	for _, k := range g.keyBuf {
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
		if printable && !isShortcut(mods) {
			continue
		}
		if printable && inpututil.KeyPressDuration(k) != 1 {
			continue
		}
		ks := gpui.Keystroke{Modifiers: mods, Key: name}
		if op := clipboardOp(name, mods); op != "" && g.clipboard(win, app, w, op) {
			continue
		}
		win.DispatchEvent(gpui.KeyDownEvent{Keystroke: ks}, app)
	}

	if !isShortcut(mods) {
		g.runeBuf = ebiten.AppendInputChars(g.runeBuf[:0])
		for _, r := range printableRunes(g.runeBuf) {
			s := string(r)
			win.DispatchEvent(gpui.KeyDownEvent{
				Keystroke:            gpui.Keystroke{Modifiers: mods &^ gpui.Shift, Key: s, KeyChar: &s},
				PreferCharacterInput: true,
			}, app)
		}
	}
}

// clipboard performs a clipboard shortcut and reports whether it consumed the
// key press. Paste is delivered as character input so it reaches the focused
// text field through the normal dispatch path; copy and cut need the installed
// input handler to read the selection.
func (g *game) clipboard(win *gpui.Window, app *gpui.App, w *window, op string) bool {
	switch op {
	case "paste":
		text, err := gpui.ClipboardRead()
		if err != nil || text == "" {
			return false
		}
		text = printableLine(text)
		if text == "" {
			return false
		}
		win.DispatchEvent(gpui.KeyDownEvent{
			Keystroke:            gpui.Keystroke{Key: text, KeyChar: &text},
			PreferCharacterInput: true,
		}, app)
		return true
	case "copy", "cut":
		h := w.handler
		if h == nil {
			return false
		}
		sel, ok := h.SelectedTextRange()
		if !ok || sel.IsNil || sel.Range.Start == sel.Range.End {
			return false
		}
		text, ok := h.TextForRange(sel.Range)
		if !ok {
			return false
		}
		if gpui.ClipboardWrite(text) != nil {
			return false
		}
		if op == "cut" {
			r := sel.Range
			h.ReplaceTextInRange(&r, "")
		}
		return true
	}
	return false
}

func printableLine(s string) string {
	out := make([]rune, 0, len(s))
	for _, r := range s {
		switch {
		case r == '\n' || r == '\r' || r == '\t':
			out = append(out, ' ')
		case r >= 0x20 && r != 0x7f:
			out = append(out, r)
		}
	}
	return string(out)
}
