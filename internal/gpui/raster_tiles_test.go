package gpui_test

import (
	"bytes"
	"fmt"
	"image"
	"math"
	"testing"

	"cad-lite/internal/gpui"
)

var tileScales = []float32{1, 1.25, 2}

// fullRender is the reference: the scene scaled into a stored copy, then
// Reset+Render into a fresh rasterizer.
func fullRender(s *gpui.Scene, scale float32, w, h int, clear gpui.Rgba) []byte {
	r := gpui.NewRasterizer(w, h)
	r.Reset(clear)
	r.Render(gpui.ScaleSceneRef(s, scale))
	return r.Pixels()
}

// cloneScene deep-copies a scene (stroke points included), so later frames of
// the window it came from cannot change it.
func cloneScene(s *gpui.Scene) *gpui.Scene {
	c := &gpui.Scene{
		Quads:      append([]gpui.Quad(nil), s.Quads...),
		Shadows:    append([]gpui.Shadow(nil), s.Shadows...),
		Sprites:    append([]gpui.GlyphSprite(nil), s.Sprites...),
		Underlines: append([]gpui.Underline(nil), s.Underlines...),
		Strokes:    append([]gpui.Stroke(nil), s.Strokes...),
	}
	for i := range c.Strokes {
		c.Strokes[i].Points = append([]gpui.Point(nil), c.Strokes[i].Points...)
	}
	return c
}

// damageTracker renders frames with RenderDamage and keeps a second buffer
// updated only through the reported damage, as a platform would.
type damageTracker struct {
	t     *testing.T
	r     *gpui.Rasterizer
	shown []byte
	w, h  int
	scale float32
	clear gpui.Rgba
}

func newDamageTracker(t *testing.T, w, h int, scale float32, clear gpui.Rgba) *damageTracker {
	return &damageTracker{t: t, r: gpui.NewRasterizer(w, h), shown: make([]byte, 4*w*h), w: w, h: h, scale: scale, clear: clear}
}

// frame renders s and checks both buffers against the reference.
func (d *damageTracker) frame(name string, s *gpui.Scene) []image.Rectangle {
	d.t.Helper()
	damage := d.r.RenderDamageScaled(s, d.scale, d.clear)
	pix := d.r.Pixels()
	for _, rc := range damage {
		for y := rc.Min.Y; y < rc.Max.Y; y++ {
			i, j := 4*(y*d.w+rc.Min.X), 4*(y*d.w+rc.Max.X)
			copy(d.shown[i:j], pix[i:j])
		}
	}
	want := fullRender(s, d.scale, d.w, d.h, d.clear)
	if !bytes.Equal(pix, want) {
		x, y := firstDiff(pix, want, d.w)
		d.t.Fatalf("%s at scale %v: damage render differs from Reset+Render at (%d,%d)", name, d.scale, x, y)
	}
	if !bytes.Equal(d.shown, want) {
		x, y := firstDiff(d.shown, want, d.w)
		d.t.Fatalf("%s at scale %v: reported damage misses a changed pixel at (%d,%d)", name, d.scale, x, y)
	}
	return damage
}

func firstDiff(a, b []byte, w int) (int, int) {
	for i := range a {
		if a[i] != b[i] {
			return (i / 4) % w, (i / 4) / w
		}
	}
	return -1, -1
}

// mutations derive frame B from frame A: each changes a few primitives the
// way a hover, a moving pointer or a toggled item does.
var mutations = []struct {
	name string
	f    func(s *gpui.Scene)
}{
	{"move-quad", func(s *gpui.Scene) {
		if len(s.Quads) > 2 {
			q := &s.Quads[len(s.Quads)/2]
			q.Bounds.Origin.X += 3.3
			q.Bounds.Origin.Y += 1.7
		}
	}},
	{"recolor-sprites", func(s *gpui.Scene) {
		for i := range s.Sprites {
			if i%7 == 3 {
				s.Sprites[i].Color = gpui.Red500
			}
		}
	}},
	{"drop-primitive", func(s *gpui.Scene) {
		switch {
		case len(s.Shadows) > 0:
			s.Shadows = s.Shadows[1:]
		case len(s.Quads) > 1:
			s.Quads = append(s.Quads[:1:1], s.Quads[2:]...)
		}
	}},
	{"overlay", func(s *gpui.Scene) {
		s.InsertQuad(gpui.Quad{
			Bounds:      gpui.NewBounds(37.5, 61.25, 121, 77),
			ContentMask: gpui.ContentMask{Bounds: gpui.NewBounds(40, 50, 140, 100), CornerRadii: gpui.Corners{TopLeft: 12, BottomRight: 20}},
			Background:  gpui.BackgroundSolid, Fill: gpui.RgbaOf(20, 120, 220, 140),
			BorderColor: gpui.Black, BorderWidths: gpui.Edges{Top: 1.5, Right: 1, Bottom: 2, Left: 1},
			CornerRadii: gpui.Corners{TopLeft: 9, TopRight: 4, BottomLeft: 7, BottomRight: 11},
		})
		s.InsertStroke(gpui.Stroke{ContentMask: gpui.InfiniteContentMask(), Points: []gpui.Point{{X: 2, Y: 3}, {X: 190, Y: 140}, {X: 70, Y: 129.5}}, Width: 1.5, Color: gpui.RgbaOf(200, 30, 30, 200)})
	}},
}

// checkSequence renders A, every B derived from A, and A again, each time
// against the reference.
func checkSequence(t *testing.T, name string, a *gpui.Scene, w, h int, scale float32, clear gpui.Rgba) {
	t.Helper()
	d := newDamageTracker(t, w, h, scale, clear)
	d.frame(name+" A", a)
	if dmg := d.frame(name+" A again", a); len(dmg) != 0 {
		t.Fatalf("%s: unchanged frame reported damage %v", name, dmg)
	}
	for _, m := range mutations {
		b := cloneScene(a)
		m.f(b)
		d.frame(name+" "+m.name+" B", b)
		d.frame(name+" "+m.name+" back to A", a)
	}
}

func borderShadowRoot() gpui.EntityRoot {
	hex := gpui.MustParseHex
	ocean := func(a float32) gpui.Rgba { return gpui.Hsla{H: 0.52, S: 0.95, L: 0.62, A: a}.ToRgba() }
	sunset := func(a float32) gpui.Rgba { return gpui.Hsla{H: 0.98, S: 0.92, L: 0.64, A: a}.ToRgba() }
	card := func(label string) *gpui.Div {
		return gpui.NewDiv().SizeWH(230, 96).Rounded(22).Bg(hex("#f8fafc")).Flex().ItemsCenter().JustifyCenter().
			Child(gpui.NewText(label).TextSize(16).TextColor(hex("#172033")).FontWeight(600))
	}
	return gpui.RenderFunc(func(*gpui.Window, *gpui.App) gpui.IntoElement {
		return gpui.NewDiv().SizeFull().Bg(hex("#080c18")).P(32).FlexRow().FlexWrap().Gap(24).Child(
			card("Gradient border").Border(4, ocean(1)),
			card("Gradient drop shadow").Shadow(
				gpui.NewBoxShadow(-12, 12, ocean(0.58)).WithBlur(22).WithSpread(2),
				gpui.NewBoxShadow(12, 12, sunset(0.46)).WithBlur(22).WithSpread(2)),
			card("Gradient inset shadow").Shadow(gpui.NewBoxShadow(0, 0, sunset(0.9)).WithBlur(12).WithSpread(7).AsInset()),
			card("Combined").Border(4, sunset(1)).Shadow(
				gpui.NewBoxShadow(0, 14, ocean(0.58)).WithBlur(24).WithSpread(3),
				gpui.NewBoxShadow(0, 0, ocean(0.7)).WithBlur(8).WithSpread(4).AsInset()))
	})
}

func gradientRoot() gpui.EntityRoot {
	hex := gpui.MustParseHex
	red, green, blue, yellow := hex("#ff0000"), hex("#008000"), hex("#0000ff"), hex("#ffff00")
	box := func(angle float32, from gpui.Rgba, fp float32, to gpui.Rgba, tp float32, cs gpui.ColorSpace) *gpui.Div {
		return gpui.NewDiv().Flex1().Rounded(12).Bg(gpui.LinearGradient(angle, gpui.LinearColorStop(from, fp), gpui.LinearColorStop(to, tp)).ColorSpace(cs))
	}
	row := func(c ...gpui.IntoElement) *gpui.Div { return gpui.NewDiv().FlexRow().Flex1().Gap(12).Child(c...) }
	return gpui.RenderFunc(func(*gpui.Window, *gpui.App) gpui.IntoElement {
		return gpui.NewDiv().Bg(gpui.White).SizeFull().P(16).FlexCol().Gap(12).Child(
			gpui.NewText("Gradient Examples").TextSize(14).TextColor(gpui.Black),
			row(box(45, red, 0, blue, 1, gpui.ColorSpaceSrgb), box(135, red, 0, green, 1, gpui.ColorSpaceOklab), box(225, green, 0, blue, 1, gpui.ColorSpaceSrgb)),
			row(box(0, red, 0, gpui.White, 1, gpui.ColorSpaceSrgb), box(90, blue, 0.05, red, 0.95, gpui.ColorSpaceOklab), box(315, green, 0, yellow, 1, gpui.ColorSpaceSrgb)),
			box(90, blue, 0.5, red, 0.5, gpui.ColorSpaceSrgb))
	})
}

func opacityRoot() gpui.EntityRoot {
	hex := gpui.MustParseHex
	blue, red, yellow := hex("#0000ff"), hex("#ff0000"), hex("#ffff00")
	return gpui.RenderFunc(func(*gpui.Window, *gpui.App) gpui.IntoElement {
		letters := gpui.NewDiv().FlexRow()
		for _, s := range []string{"A", "B", "C", "D"} {
			letters.Child(gpui.NewDiv().Opacity(0.5).Child(gpui.NewText(s).TextSize(20).TextColor(yellow)))
		}
		panel := gpui.NewDiv().Absolute().Inset(32).Opacity(0.5).FlexCol().JustifyCenter().ItemsCenter().
			Bg(gpui.White).Border(3, red).Child(
			gpui.NewDiv().FlexCol().Gap(8).JustifyCenter().ItemsCenter().SizeWH(200, 160).Bg(blue).Border(3, gpui.Black).
				Shadow(gpui.NewBoxShadow(10, 10, gpui.Black.Opacity(0.5)).WithBlur(1).WithSpread(5)).Child(
				gpui.NewText("Opacity Panel").TextSize(20).TextColor(yellow),
				gpui.NewDiv().P(16).Bg(gpui.Black).Child(gpui.NewText("opacity: 0.5").TextSize(20).TextColor(gpui.White)),
				letters))
		return gpui.NewDiv().FlexCol().SizeFull().Bg(hex("#e0e0e0")).Child(
			gpui.NewDiv().SizeFull().FlexCol().JustifyCenter().ItemsCenter().Border(1, gpui.Black).
				Child(gpui.NewText("This is background text.").TextSize(20).TextColor(blue)),
			panel)
	})
}

func textRoot() gpui.EntityRoot {
	return gpui.RenderFunc(func(*gpui.Window, *gpui.App) gpui.IntoElement {
		col := gpui.NewDiv().SizeFull().P(12).FlexCol().Gap(6).Bg(gpui.White)
		for i, size := range []float32{9, 11, 13.5, 17, 24, 31} {
			col.Child(gpui.NewText(fmt.Sprintf("Size %v: The quick brown fox jumps over the lazy dog", size)).
				TextSize(size).FontWeight(400 + 300*(i%2)).TextColor(gpui.Gray900))
		}
		col.Child(gpui.NewStyledText(
			benchRun("Mixed ", nil),
			benchRun("bold ", func(t *gpui.TextStyle) { t.FontWeight = 700 }),
			benchRun("struck ", func(t *gpui.TextStyle) { t.Strikethrough = true; t.Color = gpui.Gray500 }),
			benchRun("red", func(t *gpui.TextStyle) { t.Color = gpui.Red500 })))
		col.Child(gpui.NewDiv().SizeW(150).Child(gpui.NewText("Wrapped text breaks at word boundaries over several lines.").TextSize(13)))
		return col
	})
}

// sceneOf opens root headlessly at scale and returns a copy of its first
// frame's logical scene and the device size.
func sceneOf(t *testing.T, root gpui.EntityRoot, w, h int, scale float32) (*gpui.Scene, int, int) {
	t.Helper()
	hw, _ := openBench(t, w, h, scale, root)
	hw.Pixels()
	return cloneScene(hw.Window().Scene()), int(math.Round(float64(float32(w) * scale))), int(math.Round(float64(float32(h) * scale)))
}

// TestRenderDamageMatchesFullRender checks RenderDamage against Reset+Render
// for example scenes and synthetic edge cases at fractional and HiDPI scales,
// through sequences that change and restore primitives.
func TestRenderDamageMatchesFullRender(t *testing.T) {
	examples := []struct {
		name string
		root func() gpui.EntityRoot
		w, h int
	}{
		{"gallery", benchGallery, 520, 760},
		{"todo", func() gpui.EntityRoot { return newBenchTodo() }, 480, 640},
		{"border_and_shadow", borderShadowRoot, 560, 300},
		{"gradient", gradientRoot, 400, 330},
		{"opacity", opacityRoot, 330, 300},
		{"text", textRoot, 420, 330},
	}
	for _, scale := range tileScales {
		for _, ex := range examples {
			s, pw, ph := sceneOf(t, ex.root(), ex.w, ex.h, scale)
			checkSequence(t, ex.name, s, pw, ph, scale, gpui.White)
		}
		for _, sy := range syntheticScenes(scale) {
			lw, lh := 203, 147
			pw, ph := int(math.Round(float64(float32(lw)*scale))), int(math.Round(float64(float32(lh)*scale)))
			checkSequence(t, sy.name, sy.s, pw, ph, scale, gpui.RgbaOf(240, 236, 228, 255))
			checkSequence(t, sy.name+" translucent clear", sy.s, pw, ph, scale, gpui.RgbaOf(10, 20, 30, 90))
		}
	}
}

type synthetic struct {
	name string
	s    *gpui.Scene
}

// syntheticScenes are edge cases laid over tile seams (device x or y = 64,
// 128): rounded masks, seam-crossing strokes, translucent fills, shadows,
// image sprites and glyph blits. Glyph masks are sized so the 1:1 blit
// applies at scale.
func syntheticScenes(scale float32) []synthetic {
	k := gpui.Pixels(scale)
	seam := 64 / k
	img := image.NewNRGBA(image.Rect(3, 5, 43, 35))
	for i := range img.Pix {
		img.Pix[i] = uint8(i*61 + i/5)
	}
	for i := 3; i < len(img.Pix); i += 4 {
		if i%9 != 0 {
			img.Pix[i] = 255
		}
	}
	mask := &gpui.GlyphMask{W: 10, H: 12, Pix: make([]uint8, 120), Scale: scale}
	for i := range mask.Pix {
		mask.Pix[i] = uint8(i * 37)
	}
	glyph := func(x, y gpui.Pixels) gpui.GlyphSprite {
		return gpui.GlyphSprite{Bounds: gpui.Bounds{Origin: gpui.Point{X: x, Y: y}, Size: gpui.Size{W: 10 / k, H: 12 / k}}, ContentMask: gpui.InfiniteContentMask(), Mask: mask, Color: gpui.RgbaOf(30, 40, 200, 230)}
	}
	rounded := gpui.ContentMask{Bounds: gpui.NewBounds(float32(seam)-30, float32(seam)-25, 75, 64), CornerRadii: gpui.Corners{TopLeft: 18, TopRight: 6, BottomLeft: 10, BottomRight: 25}}
	inf := gpui.InfiniteContentMask()
	var out []synthetic

	var a gpui.Scene
	a.InsertQuad(gpui.Quad{Bounds: gpui.NewBounds(0, 0, 203, 147), ContentMask: inf, Background: gpui.BackgroundSolid, Fill: gpui.RgbaOf(250, 250, 250, 255)})
	a.InsertQuad(gpui.Quad{Bounds: gpui.NewBounds(float32(seam)-40, 10, 90, 120), ContentMask: rounded, Background: gpui.BackgroundSolid, Fill: gpui.RgbaOf(20, 160, 90, 255), CornerRadii: gpui.Corners{TopLeft: 8, TopRight: 8, BottomLeft: 8, BottomRight: 8}})
	a.InsertQuad(gpui.Quad{Bounds: gpui.NewBounds(5.5, 7.25, 190, 130), ContentMask: inf, Background: gpui.BackgroundSolid, Fill: gpui.RgbaOf(200, 40, 40, 77)})
	a.InsertQuad(gpui.Quad{Bounds: gpui.NewBounds(float32(seam)-3.3, float32(seam)-7.7, 50, 40), ContentMask: inf, Background: gpui.BackgroundLinearGradient,
		Gradient:    gpui.LinearGradient(30, gpui.LinearColorStop(gpui.Blue500, 0), gpui.LinearColorStop(gpui.RgbaOf(255, 200, 0, 120), 1)).Gradient,
		BorderColor: gpui.Gray800, BorderWidths: gpui.Edges{Top: 2, Right: 2, Bottom: 2, Left: 2}, CornerRadii: gpui.Corners{TopLeft: 12, TopRight: 3, BottomLeft: 3, BottomRight: 12}})
	out = append(out, synthetic{"quads", &a})

	var b gpui.Scene
	b.InsertShadow(gpui.Shadow{Bounds: gpui.NewBounds(float32(seam)-20, 20, 60, 50), CornerRadii: gpui.Corners{TopLeft: 10, TopRight: 10, BottomLeft: 10, BottomRight: 10}, ContentMask: inf, Color: gpui.RgbaOf(0, 0, 0, 120), BlurRadius: 14})
	b.InsertShadow(gpui.Shadow{Bounds: gpui.NewBounds(float32(seam)+10, float32(seam)-15, 40, 30), ContentMask: inf, Color: gpui.RgbaOf(255, 0, 0, 200), BlurRadius: 0})
	b.InsertShadow(gpui.Shadow{Inset: true, Clip: gpui.NewBounds(10, float32(seam)-30, 120, 70), ClipRadii: gpui.Corners{TopLeft: 20, TopRight: 20, BottomLeft: 20, BottomRight: 20},
		Bounds: gpui.NewBounds(22, float32(seam)-18, 96, 46), CornerRadii: gpui.Corners{TopLeft: 8, TopRight: 8, BottomLeft: 8, BottomRight: 8}, ContentMask: rounded, Color: gpui.RgbaOf(20, 20, 160, 210), BlurRadius: 9})
	b.InsertShadow(gpui.Shadow{Inset: true, Clip: gpui.NewBounds(140, 10, 50, 50), Bounds: gpui.NewBounds(146, 16, 38, 38), ContentMask: inf, Color: gpui.RgbaOf(0, 120, 0, 255), BlurRadius: 0})
	b.InsertUnderline(gpui.Underline{Bounds: gpui.NewBounds(3, float32(seam)-4.6, 180, 5), ContentMask: inf, Color: gpui.RgbaOf(0, 0, 0, 180), Thickness: 1.5})
	out = append(out, synthetic{"shadows", &b})

	var c gpui.Scene
	c.InsertStroke(gpui.Stroke{ContentMask: inf, Points: []gpui.Point{{X: 1, Y: 1}, {X: 200, Y: 146}}, Width: 3, Color: gpui.RgbaOf(10, 10, 10, 255)})
	c.InsertStroke(gpui.Stroke{ContentMask: rounded, Points: []gpui.Point{{X: seam - 30, Y: seam}, {X: seam + 30, Y: seam - 20}, {X: seam + 10, Y: seam + 40}}, Closed: true, Width: 6, Color: gpui.RgbaOf(200, 100, 0, 160)})
	c.InsertStroke(gpui.Stroke{ContentMask: inf, Points: []gpui.Point{{X: 0, Y: seam}, {X: 203, Y: seam + 0.3}}, Width: 0.3, Color: gpui.RgbaOf(0, 0, 255, 255)})
	c.InsertStroke(gpui.Stroke{ContentMask: inf, Points: []gpui.Point{{X: seam, Y: 0}, {X: seam - 0.5, Y: 147}}, Width: 0, Color: gpui.RgbaOf(0, 128, 0, 255)})
	c.InsertStroke(gpui.Stroke{ContentMask: gpui.ContentMask{Bounds: gpui.NewBounds(20, 20, 100, 100)}, Points: []gpui.Point{{X: seam, Y: seam}}, Width: 9, Color: gpui.RgbaOf(255, 0, 255, 200)})
	c.InsertStroke(gpui.Stroke{ContentMask: inf, Points: []gpui.Point{{X: gpui.Pixels(math.NaN()), Y: 3}, {X: 50, Y: 50}}, Width: 2, Color: gpui.Black})
	pts := make([]gpui.Point, 0, 200)
	for i := range 200 {
		th := float64(i) / 199 * 2 * math.Pi
		pts = append(pts, gpui.Point{X: seam + gpui.Pixels(60*math.Cos(th)), Y: 70 + gpui.Pixels(55*math.Sin(th))})
	}
	c.InsertStroke(gpui.Stroke{ContentMask: inf, Points: pts, Width: 1.25, Color: gpui.RgbaOf(90, 0, 160, 230)})
	out = append(out, synthetic{"strokes", &c})

	var d gpui.Scene
	d.InsertSprite(gpui.GlyphSprite{Bounds: gpui.Bounds{Origin: gpui.Point{X: (64 - 17) / k, Y: (64 - 11) / k}, Size: gpui.Size{W: 40 / k, H: 30 / k}}, ContentMask: inf, Image: img, Color: gpui.White})
	d.InsertSprite(gpui.GlyphSprite{Bounds: gpui.NewBounds(float32(seam)+15, 5, 37, 52), ContentMask: rounded, Image: img, Color: gpui.RgbaOf(255, 255, 255, 200)})
	d.InsertSprite(gpui.GlyphSprite{Bounds: gpui.NewBounds(140, 90, 55, 50), ContentMask: inf, Image: img, Color: gpui.White})
	for i := range 6 {
		d.InsertSprite(glyph(seam-5+gpui.Pixels(i)*2.5/k, seam-6+gpui.Pixels(i%3)/k))
	}
	d.InsertSprite(gpui.GlyphSprite{Bounds: gpui.NewBounds(float32(seam)-7, 100, 23, 31), ContentMask: inf, Mask: mask, Color: gpui.RgbaOf(0, 0, 0, 255)})
	out = append(out, synthetic{"sprites", &d})

	// Explicit orders out of insertion order exercise the per-kind sort.
	var e gpui.Scene
	e.InsertQuad(gpui.Quad{Order: 50, Bounds: gpui.NewBounds(10, 10, 100, 100), ContentMask: inf, Background: gpui.BackgroundSolid, Fill: gpui.Blue500})
	e.InsertQuad(gpui.Quad{Order: 5, Bounds: gpui.NewBounds(50, 50, 120, 80), ContentMask: inf, Background: gpui.BackgroundSolid, Fill: gpui.Red500})
	e.InsertStroke(gpui.Stroke{Order: 50, ContentMask: inf, Points: []gpui.Point{{X: 0, Y: 60}, {X: 203, Y: 70}}, Width: 4, Color: gpui.Green600})
	e.InsertSprite(glyph(seam, seam))
	e.InsertShadow(gpui.Shadow{Order: 50, Bounds: gpui.NewBounds(60, 20, 50, 50), ContentMask: inf, Color: gpui.RgbaOf(0, 0, 0, 100), BlurRadius: 6})
	out = append(out, synthetic{"orders", &e})
	return out
}

// TestDamageOnlyChangedTiles checks the reported damage is the tiles a change
// touches, and the full-repaint triggers.
func TestDamageOnlyChangedTiles(t *testing.T) {
	const w, h = 300, 200
	base := func() *gpui.Scene {
		var s gpui.Scene
		s.InsertQuad(gpui.Quad{Bounds: gpui.NewBounds(0, 0, w, h), ContentMask: gpui.InfiniteContentMask(), Background: gpui.BackgroundSolid, Fill: gpui.Gray100})
		s.InsertQuad(gpui.Quad{Bounds: gpui.NewBounds(100, 100, 10, 10), ContentMask: gpui.InfiniteContentMask(), Background: gpui.BackgroundSolid, Fill: gpui.Blue500})
		return &s
	}
	full := []image.Rectangle{image.Rect(0, 0, w, h)}
	d := newDamageTracker(t, w, h, 1, gpui.White)
	if got := d.frame("first", base()); !equalRects(got, full) {
		t.Fatalf("first frame damage = %v, want %v", got, full)
	}
	if got := d.frame("same", base()); len(got) != 0 {
		t.Fatalf("unchanged frame damage = %v, want none", got)
	}
	moved := base()
	moved.Quads[1].Bounds.Origin.X = 70
	if got, want := d.frame("moved", moved), []image.Rectangle{image.Rect(64, 64, 128, 128)}; !equalRects(got, want) {
		t.Fatalf("moving a quad inside one tile: damage = %v, want %v", got, want)
	}
	seam := base()
	seam.Quads[1].Bounds = gpui.NewBounds(120, 120, 20, 20)
	if got, want := d.frame("seam", seam), []image.Rectangle{image.Rect(64, 64, 192, 128), image.Rect(64, 128, 192, 192)}; !equalRects(got, want) {
		t.Fatalf("moving a quad across a seam: damage = %v, want %v", got, want)
	}
	edge := base()
	edge.Quads[1].Bounds = gpui.NewBounds(290, 190, 5, 5)
	if got, want := d.frame("edge", edge), []image.Rectangle{image.Rect(64, 64, 192, 192), image.Rect(256, 128, 300, 200)}; !equalRectSet(got, want) {
		t.Fatalf("moving a quad to the edge tile: damage = %v, want %v", got, want)
	}
	d.clear = gpui.Black
	if got := d.frame("clear change", edge); !equalRects(got, full) {
		t.Fatalf("clear change damage = %v, want full", got)
	}
	d.r.Invalidate()
	if got := d.frame("invalidate", edge); !equalRects(got, full) {
		t.Fatalf("Invalidate damage = %v, want full", got)
	}
	d.r.Render(edge)
	if got := d.frame("after Render", edge); !equalRects(got, full) {
		t.Fatalf("damage after Render = %v, want full", got)
	}
	d.scale = 1.5
	if got := d.frame("scale change", edge); !equalRects(got, full) {
		t.Fatalf("scale change damage = %v, want full", got)
	}
}

func equalRects(a, b []image.Rectangle) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// equalRectSet compares the pixel sets two rect lists cover.
func equalRectSet(a, b []image.Rectangle) bool {
	in := func(rs []image.Rectangle, x, y int) bool {
		for _, r := range rs {
			if image.Pt(x, y).In(r) {
				return true
			}
		}
		return false
	}
	for y := 0; y < 400; y++ {
		for x := 0; x < 400; x++ {
			if in(a, x, y) != in(b, x, y) {
				return false
			}
		}
	}
	return true
}

// TestImageMutatedInPlaceIsRepainted paints an image, changes its pixels in
// place (as the feedback editor does) and redraws: the presented frame must
// show the new pixels. A versioned image repaints when its version changes.
func TestImageMutatedInPlaceIsRepainted(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 40, 30))
	paint := func(c uint8) {
		for i := 0; i < len(img.Pix); i += 4 {
			img.Pix[i], img.Pix[i+1], img.Pix[i+2], img.Pix[i+3] = c, 255-c, 0, 255
		}
	}
	paint(10)
	var version uint64
	root := gpui.RenderFunc(func(*gpui.Window, *gpui.App) gpui.IntoElement {
		return gpui.NewDiv().SizeFull().Bg(gpui.White).Child(gpui.NewCanvas(func(b gpui.Bounds, w *gpui.Window, _ *gpui.App) {
			w.PaintImage(gpui.NewBounds(0, 0, 40, 30), img)
			w.PaintImageVersion(gpui.NewBounds(130, 100, 40, 30), img, version)
		}))
	})
	for _, scale := range []float32{1, 2} {
		paint(10)
		version = 1
		hw, _ := openBench(t, 200, 150, scale, root)
		hw.Pixels()
		at := func(x, y int) uint8 { return hw.Pixels()[4*(y*int(200*scale)+x)] }
		paint(200)
		hw.Window().Refresh()
		if got := at(int(10*scale), int(10*scale)); got != 200 {
			t.Fatalf("scale %v: image mutated in place shows red %d, want 200", scale, got)
		}
		if got := at(int(140*scale), int(110*scale)); got != 10 {
			t.Fatalf("scale %v: versioned image repainted without a version change (red %d, want the promised stale 10)", scale, got)
		}
		version = 2
		hw.Window().Refresh()
		if got := at(int(140*scale), int(110*scale)); got != 200 {
			t.Fatalf("scale %v: versioned image after a version bump shows red %d, want 200", scale, got)
		}
	}
}

// TestHeadlessWindowShowsDamagedFrames drives a window through state changes
// and checks the headless copy, updated only through PresentDamage, equals a
// full render of each frame.
func TestHeadlessWindowShowsDamagedFrames(t *testing.T) {
	for _, scale := range tileScales {
		todo := newBenchTodo()
		hw, app := openBench(t, 480, 640, scale, todo)
		win := hw.Window()
		pw, ph := int(math.Round(float64(480*scale))), int(math.Round(float64(640*scale)))
		for step := range 8 {
			switch step % 4 {
			case 0:
				todo.done[step%len(todo.done)] = !todo.done[step%len(todo.done)]
			case 1:
				todo.titles[1] += "!"
			case 2:
				hw.Dispatch(gpui.MouseMoveEvent{Position: gpui.Point{X: gpui.Pixels(30 + step*20), Y: 200}})
			case 3:
				todo.titles = todo.titles[:len(todo.titles)-1]
				todo.done = todo.done[:len(todo.done)-1]
			}
			win.Refresh()
			win.Draw(app)
			got := hw.Pixels()
			want := fullRender(win.Scene(), win.ScaleFactor(), pw, ph, gpui.White)
			if !bytes.Equal(got, want) {
				x, y := firstDiff(got, want, pw)
				t.Fatalf("scale %v step %d: presented frame differs from a full render at (%d,%d)", scale, step, x, y)
			}
		}
	}
}

// TestScreenshotAfterPartialFramesIsComplete takes the screenshots the app
// offers (Window.Snapshot, used by the feedback tool and the settings
// backdrop, and the headless image used by -png) after several frames that
// repainted only some tiles: each must equal a full render of the frame.
func TestScreenshotAfterPartialFramesIsComplete(t *testing.T) {
	for _, scale := range tileScales {
		todo := newBenchTodo()
		hw, app := openBench(t, 480, 640, scale, todo)
		win := hw.Window()
		pw, ph := int(math.Round(float64(480*scale))), int(math.Round(float64(640*scale)))
		for i := range 4 {
			todo.done[i] = !todo.done[i]
			win.Refresh()
			win.Draw(app)
		}
		ref := gpui.NewRasterizer(pw, ph)
		ref.Reset(gpui.White)
		ref.Render(gpui.ScaleSceneRef(win.Scene(), win.ScaleFactor()))
		want := ref.Image()
		if got := win.Snapshot(); !bytes.Equal(got.Pix, want.Pix) || got.Rect != want.Rect {
			t.Fatalf("scale %v: Window.Snapshot after partial frames differs from a full render", scale)
		}
		if got := hw.Image(); !bytes.Equal(got.Pix, want.Pix) {
			t.Fatalf("scale %v: headless image after partial frames differs from a full render", scale)
		}
	}
}
