package gpui

import (
	"image"
	"image/png"
	"math"
	"os"
	"sync"
)

type renderConfig struct {
	background Rgba
	keymap     *Keymap
	title      string
}

// RenderOption customizes RenderToPNG and RenderToImage.
type RenderOption func(*renderConfig)

// WithBackgroundColor sets the color the frame is cleared to (default white).
func WithBackgroundColor(c Rgba) RenderOption {
	return func(rc *renderConfig) { rc.background = c }
}

// WithKeymap installs k as the app's keymap before the first draw.
func WithKeymap(k *Keymap) RenderOption {
	return func(rc *renderConfig) { rc.keymap = k }
}

// WithTitle sets the window title.
func WithTitle(t string) RenderOption {
	return func(rc *renderConfig) { rc.title = t }
}

// RenderToPNG builds root, lays it out at w x h logical pixels, paints,
// rasterizes at the given device scale (the PNG is round(w*scale) x
// round(h*scale)), and writes a PNG to path.
func RenderToPNG(path string, w, h int, scale float32, root EntityRoot, opts ...RenderOption) error {
	img, err := RenderToImage(w, h, scale, root, opts...)
	if err != nil {
		return err
	}
	return writePNG(path, img)
}

// RenderToImage is RenderToPNG but returns the non-premultiplied image.
func RenderToImage(w, h int, scale float32, root EntityRoot, opts ...RenderOption) (*image.RGBA, error) {
	cfg := renderConfig{background: White, title: "gpui"}
	for _, o := range opts {
		o(&cfg)
	}
	p := NewHeadlessPlatform()
	p.SetScaleFactor(scale)
	p.SetBackgroundColor(cfg.background)
	app := NewAppWithPlatform(p)
	if cfg.keymap != nil {
		app.keymap = cfg.keymap
	}
	wo := WindowOptions{Width: float32(w), Height: float32(h), Title: cfg.title}
	if err := app.OpenWindow(wo, func(*Window, *App) EntityRoot { return root }); err != nil {
		return nil, err
	}
	return p.Windows()[0].Image(), nil
}

func writePNG(path string, img image.Image) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	if err := png.Encode(f, img); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}

// HeadlessPlatform implements Platform with no window; used by RenderToPNG
// and by tests that need to drive frames and dispatch synthetic input.
type HeadlessPlatform struct {
	mu         sync.Mutex
	windows    []*HeadlessWindow
	scale      float32
	background Rgba
	quit       bool
}

// NewHeadlessPlatform returns a headless platform with scale 1 and a white
// background.
func NewHeadlessPlatform() *HeadlessPlatform {
	return &HeadlessPlatform{scale: 1, background: White}
}

// SetScaleFactor sets the device scale of windows opened afterwards.
func (p *HeadlessPlatform) SetScaleFactor(s float32) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if s <= 0 {
		s = 1
	}
	p.scale = s
}

// SetBackgroundColor sets the clear color of windows opened afterwards.
func (p *HeadlessPlatform) SetBackgroundColor(c Rgba) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.background = c
}

// Windows returns the windows opened so far, in creation order.
func (p *HeadlessPlatform) Windows() []*HeadlessWindow {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]*HeadlessWindow(nil), p.windows...)
}

// Run calls onLaunch with an App driven by this platform and returns.
func (p *HeadlessPlatform) Run(onLaunch func(*App)) error {
	if onLaunch != nil {
		onLaunch(NewAppWithPlatform(p))
	}
	return nil
}

// OpenWindow returns an in-memory HeadlessWindow sized by o.
func (p *HeadlessPlatform) OpenWindow(o WindowOptions) (PlatformWindow, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	w, h := o.Width, o.Height
	if w <= 0 {
		w = 800
	}
	if h <= 0 {
		h = 600
	}
	hw := &HeadlessWindow{w: Pixels(w), h: Pixels(h), scale: p.scale, background: p.background}
	p.windows = append(p.windows, hw)
	return hw, nil
}

// Quit marks the platform as quit.
func (p *HeadlessPlatform) Quit() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.quit = true
}

// Name returns "headless".
func (p *HeadlessPlatform) Name() string { return "headless" }

// HeadlessWindow is the PlatformWindow returned by HeadlessPlatform; it keeps
// the last presented pixels and exposes helpers for tests.
type HeadlessWindow struct {
	w, h       Pixels
	scale      float32
	background Rgba
	win        *Window
	handler    PlatformInputHandler
	pending    bool
	pix        []byte
	pw, ph     int
}

func (hw *HeadlessWindow) bindWindow(w *Window) {
	hw.win = w
	if w.text == nil && DefaultFontPath() != "" {
		w.SetTextMeasurer(NewShaper(NewFontManager()))
	}
}

// Bounds returns the logical window size.
func (hw *HeadlessWindow) Bounds() (Pixels, Pixels) { return hw.w, hw.h }

// ScaleFactor returns the device scale.
func (hw *HeadlessWindow) ScaleFactor() float32 { return hw.scale }

// RequestFrame records that a redraw is wanted; the next Dispatch, Pixels or
// Image call draws it.
func (hw *HeadlessWindow) RequestFrame() { hw.pending = true }

// Present stores a copy of the premultiplied RGBA frame.
func (hw *HeadlessWindow) Present(pix []byte, w, h int) {
	hw.pix = append(hw.pix[:0], pix...)
	hw.pw, hw.ph = w, h
}

// SetInputHandler installs the focused text field's editing surface.
func (hw *HeadlessWindow) SetInputHandler(h PlatformInputHandler) { hw.handler = h }

// TakeInputHandler removes and returns the installed input handler.
func (hw *HeadlessWindow) TakeInputHandler() PlatformInputHandler {
	h := hw.handler
	hw.handler = nil
	return h
}

// Quit is a no-op.
func (hw *HeadlessWindow) Quit() {}

// Window returns the Window this platform window backs.
func (hw *HeadlessWindow) Window() *Window { return hw.win }

// Dispatch feeds a synthetic input event to the window, then draws any frame
// the event invalidated.
func (hw *HeadlessWindow) Dispatch(input PlatformInput) {
	if hw.win == nil {
		return
	}
	app := hw.win.app
	app.Update(func(cx *App) { hw.win.DispatchEvent(input, cx) })
	hw.flush()
}

func (hw *HeadlessWindow) flush() {
	if hw.win == nil {
		return
	}
	if hw.win.invalid || hw.pending || hw.pix == nil {
		hw.win.Draw(hw.win.app)
		hw.pending = false
		hw.win.invalid = false
		hw.render()
	}
}

func (hw *HeadlessWindow) render() {
	pw := int(math.Round(float64(hw.w) * float64(hw.scale)))
	ph := int(math.Round(float64(hw.h) * float64(hw.scale)))
	r := NewRasterizer(pw, ph)
	r.Reset(hw.background)
	r.Render(scaleScene(hw.win.Scene(), hw.scale))
	hw.Present(r.Pixels(), pw, ph)
}

// Pixels returns the latest frame as premultiplied RGBA, 4*w*h bytes in
// device pixels, drawing first if the window is invalid.
func (hw *HeadlessWindow) Pixels() []byte {
	hw.flush()
	return hw.pix
}

// Image returns the latest frame as a non-premultiplied image.
func (hw *HeadlessWindow) Image() *image.RGBA {
	hw.flush()
	pix := make([]byte, len(hw.pix))
	for i := 0; i < len(pix); i += 4 {
		a := uint32(hw.pix[i+3])
		switch a {
		case 0:
		case 255:
			copy(pix[i:i+4], hw.pix[i:i+4])
		default:
			for c := 0; c < 3; c++ {
				pix[i+c] = uint8(min(255, (uint32(hw.pix[i+c])*255+a/2)/a))
			}
			pix[i+3] = hw.pix[i+3]
		}
	}
	return &image.RGBA{Pix: pix, Stride: 4 * hw.pw, Rect: image.Rect(0, 0, hw.pw, hw.ph)}
}

// SavePNG writes the latest frame to path as a PNG.
func (hw *HeadlessWindow) SavePNG(path string) error {
	return writePNG(path, hw.Image())
}

func scaleScene(s *Scene, scale float32) *Scene {
	k := Pixels(scale)
	if scale == 1 {
		return s
	}
	sb := func(b Bounds) Bounds {
		return Bounds{
			Origin: Point{X: b.Origin.X * k, Y: b.Origin.Y * k},
			Size:   Size{W: b.Size.W * k, H: b.Size.H * k},
		}
	}
	sm := func(m ContentMask) ContentMask {
		if m.Bounds.Size.W >= 1e6 {
			return m
		}
		return ContentMask{Bounds: sb(m.Bounds)}
	}
	out := &Scene{}
	for _, q := range s.Quads {
		q.Bounds = sb(q.Bounds)
		q.ContentMask = sm(q.ContentMask)
		q.BorderWidths = Edges{Top: q.BorderWidths.Top * k, Right: q.BorderWidths.Right * k, Bottom: q.BorderWidths.Bottom * k, Left: q.BorderWidths.Left * k}
		q.CornerRadii = Corners{TopLeft: q.CornerRadii.TopLeft * k, TopRight: q.CornerRadii.TopRight * k, BottomLeft: q.CornerRadii.BottomLeft * k, BottomRight: q.CornerRadii.BottomRight * k}
		out.Quads = append(out.Quads, q)
	}
	for _, u := range s.Underlines {
		u.Bounds = sb(u.Bounds)
		u.ContentMask = sm(u.ContentMask)
		u.Thickness *= k
		out.Underlines = append(out.Underlines, u)
	}
	for _, g := range s.Sprites {
		g.Bounds = sb(g.Bounds)
		g.ContentMask = sm(g.ContentMask)
		out.Sprites = append(out.Sprites, g)
	}
	return out
}
