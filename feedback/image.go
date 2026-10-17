package feedback

import (
	"bytes"
	"encoding/base64"
	"errors"
	"image"
	"image/color"
	"image/draw"
	_ "image/jpeg"
	"image/png"
	"io"
	"math"

	xdraw "golang.org/x/image/draw"
)

// MaxImageSide bounds screenshot memory and upload size without changing annotation coordinates after capture.
const MaxImageSide = 2048

// DecodeImage reads a bounded PNG or JPEG attachment and converts it into the editor's image format.
func DecodeImage(r io.Reader) (*image.NRGBA, error) {
	data, err := io.ReadAll(io.LimitReader(r, (16<<20)+1))
	if err != nil || len(data) > 16<<20 {
		return nil, errors.New("Screenshot file exceeds 16 MB.")
	}
	config, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil || config.Width <= 0 || config.Height <= 0 || int64(config.Width)*int64(config.Height) > 32000000 {
		return nil, errors.New("Choose a PNG or JPEG screenshot under 32 megapixels.")
	}
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, errors.New("Cannot read this screenshot.")
	}
	return PrepareImage(img), nil
}

// PrepareImage copies and, when necessary, downscales a capture to a bounded, origin-zero image.
func PrepareImage(src image.Image) *image.NRGBA {
	if src == nil || src.Bounds().Empty() {
		return nil
	}
	b := src.Bounds()
	width, height := b.Dx(), b.Dy()
	if max(width, height) > MaxImageSide {
		scale := float64(MaxImageSide) / float64(max(width, height))
		width = max(1, int(float64(width)*scale))
		height = max(1, int(float64(height)*scale))
	}
	out := image.NewNRGBA(image.Rect(0, 0, width, height))
	if width == b.Dx() && height == b.Dy() {
		draw.Draw(out, out.Bounds(), src, b.Min, draw.Src)
	} else {
		xdraw.ApproxBiLinear.Scale(out, out.Bounds(), src, b, draw.Src, nil)
	}
	return out
}

// EncodeScreenshot encodes the edited screenshot as raw PNG base64 accepted by the intake service.
func EncodeScreenshot(img image.Image) (string, error) {
	if img == nil {
		return "", nil
	}
	var buf bytes.Buffer
	encoder := png.Encoder{CompressionLevel: png.BestSpeed}
	if err := encoder.Encode(&buf, img); err != nil {
		return "", errors.New("Cannot encode the screenshot.")
	}
	if base64.StdEncoding.EncodedLen(buf.Len()) > 14000000 {
		return "", errors.New("Screenshot is too large. Remove it and try again.")
	}
	return base64.StdEncoding.EncodeToString(buf.Bytes()), nil
}

// Point is a position in screenshot pixels.
type Point struct {
	// X and Y are image coordinates in pixels.
	X, Y float64
}

type annotation struct {
	points []Point
	width  float64
	color  color.NRGBA
	erase  bool
}

// Editor owns a screenshot and reversible pen/eraser strokes. Its methods belong to the UI goroutine.
type Editor struct {
	base, preview *image.NRGBA
	strokes       []annotation
	active        bool
}

// NewEditor copies the screenshot so cancelling edits never changes the original attachment.
func NewEditor(src image.Image) *Editor {
	base := PrepareImage(src)
	return &Editor{base: base, preview: PrepareImage(base)}
}

// Image returns the current preview, which the caller must treat as read-only.
func (e *Editor) Image() *image.NRGBA { return e.preview }

// Snapshot returns an independent flattened copy suitable for attachment to a report.
func (e *Editor) Snapshot() *image.NRGBA { return PrepareImage(e.preview) }

// CanUndo reports whether a completed or current stroke can be removed.
func (e *Editor) CanUndo() bool { return len(e.strokes) > 0 }

// Begin starts a pen or eraser stroke, including a visible dot for a click without dragging.
func (e *Editor) Begin(p Point, width float64, ink color.NRGBA, erase bool) {
	if e.preview == nil {
		return
	}
	e.End()
	p = e.clamp(p)
	e.strokes = append(e.strokes, annotation{points: []Point{p}, width: math.Max(1, math.Min(width, 100)), color: ink, erase: erase})
	e.active = true
	e.segment(p, p, e.strokes[len(e.strokes)-1])
}

// Move extends the active stroke, interpolating between pointer events.
func (e *Editor) Move(p Point) {
	if !e.active || len(e.strokes) == 0 {
		return
	}
	p = e.clamp(p)
	s := &e.strokes[len(e.strokes)-1]
	last := s.points[len(s.points)-1]
	if p == last {
		return
	}
	s.points = append(s.points, p)
	e.segment(last, p, *s)
}

// End completes the active stroke.
func (e *Editor) End() { e.active = false }

// Undo removes one entire stroke and rebuilds the preview, including erased annotations.
func (e *Editor) Undo() {
	e.End()
	if len(e.strokes) == 0 {
		return
	}
	e.strokes = e.strokes[:len(e.strokes)-1]
	draw.Draw(e.preview, e.preview.Bounds(), e.base, image.Point{}, draw.Src)
	for _, s := range e.strokes {
		e.segment(s.points[0], s.points[0], s)
		for i := 1; i < len(s.points); i++ {
			e.segment(s.points[i-1], s.points[i], s)
		}
	}
}

func (e *Editor) clamp(p Point) Point {
	p.X = math.Max(0, math.Min(p.X, float64(e.preview.Bounds().Dx()-1)))
	p.Y = math.Max(0, math.Min(p.Y, float64(e.preview.Bounds().Dy()-1)))
	return p
}

func (e *Editor) segment(a, b Point, s annotation) {
	distance := math.Hypot(b.X-a.X, b.Y-a.Y)
	steps := max(1, int(math.Ceil(distance/math.Max(1, s.width/4))))
	for i := 0; i <= steps; i++ {
		t := float64(i) / float64(steps)
		e.dab(a.X+(b.X-a.X)*t, a.Y+(b.Y-a.Y)*t, s)
	}
}

func (e *Editor) dab(x, y float64, s annotation) {
	radius := s.width / 2
	bounds := e.preview.Bounds()
	for py := max(0, int(math.Floor(y-radius-1))); py < min(bounds.Dy(), int(math.Ceil(y+radius+1))); py++ {
		for px := max(0, int(math.Floor(x-radius-1))); px < min(bounds.Dx(), int(math.Ceil(x+radius+1))); px++ {
			coverage := math.Max(0, math.Min(1, radius+0.5-math.Hypot(float64(px)+0.5-x, float64(py)+0.5-y)))
			if coverage == 0 {
				continue
			}
			src := s.color
			if s.erase {
				src = e.base.NRGBAAt(px, py)
			}
			dst := e.preview.NRGBAAt(px, py)
			if s.erase {
				blend := func(a, b uint8) uint8 { return uint8(math.Round(float64(a)*coverage + float64(b)*(1-coverage))) }
				e.preview.SetNRGBA(px, py, color.NRGBA{R: blend(src.R, dst.R), G: blend(src.G, dst.G), B: blend(src.B, dst.B), A: blend(src.A, dst.A)})
				continue
			}
			alpha := coverage * float64(src.A) / 255
			oldAlpha := float64(dst.A) / 255
			outAlpha := alpha + oldAlpha*(1-alpha)
			if outAlpha == 0 {
				continue
			}
			blend := func(a, b uint8) uint8 {
				return uint8(math.Round((float64(a)*alpha + float64(b)*oldAlpha*(1-alpha)) / outAlpha))
			}
			e.preview.SetNRGBA(px, py, color.NRGBA{R: blend(src.R, dst.R), G: blend(src.G, dst.G), B: blend(src.B, dst.B), A: uint8(math.Round(outAlpha * 255))})
		}
	}
}
