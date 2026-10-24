package ebitenplatform

import (
	"image"
	"time"

	"github.com/hajimehoshi/ebiten/v2"

	"cad-lite/internal/gpui"
)

var _ gpui.DamagePresenter = (*window)(nil)

// damageTile is the side of the cells damage is merged on, in device pixels;
// it matches the rasterizer's tile size.
const damageTile = 64

// fullUploadEvery is the safety net against a lost device: the whole frame is
// uploaded at least this often.
const fullUploadEvery = 5 * time.Second

// PresentDamage keeps a reference to the frame, like Present, and accumulates
// the damaged rects until the next Draw (several Updates may run per Draw).
// A size change uploads the whole frame.
func (w *window) PresentDamage(pix []byte, pw, ph int, damage []image.Rectangle) {
	if pw != w.pixW || ph != w.pixH || len(pix) != len(w.pix) {
		w.fullUpload = true
	}
	w.pix = pix
	w.pixW, w.pixH = pw, ph
	w.damage = append(w.damage, damage...)
	if len(damage) > 0 || w.fullUpload {
		w.fresh = true
	}
}

// upload writes the frame to screen: the whole frame when needed (a size or
// scale change, a plain Present, or the periodic safety net), else only the
// strips covering the accumulated damage.
func (w *window) upload(screen *ebiten.Image, now time.Time) {
	if w.fullUpload || now.Sub(w.lastFull) >= fullUploadEvery {
		screen.WritePixels(w.pix)
		w.lastFull = now
	} else {
		w.strips, w.tileBits = damageStrips(w.strips[:0], w.tileBits, w.damage, w.pixW, w.pixH)
		for _, r := range w.strips {
			var packed []byte
			packed, w.scratch = packRect(w.scratch, w.pix, w.pixW, r)
			screen.SubImage(r).(*ebiten.Image).WritePixels(packed)
		}
	}
	w.fullUpload = false
	w.fresh = false
	w.damage = w.damage[:0]
}

// damageStrips covers rects (clipped to w×h) with tile-aligned strips: the
// tiles any rect touches are marked, each tile row's runs of marked tiles
// become one rect, and a run equal to the run right above it extends that
// rect downwards. Strips are clipped to w×h, appended to dst in row order,
// and never overlap. bits is scratch, returned for reuse.
func damageStrips(dst []image.Rectangle, bits []bool, rects []image.Rectangle, w, h int) ([]image.Rectangle, []bool) {
	if w <= 0 || h <= 0 {
		return dst, bits
	}
	nx, ny := (w+damageTile-1)/damageTile, (h+damageTile-1)/damageTile
	if cap(bits) < nx*ny {
		bits = make([]bool, nx*ny)
	}
	bits = bits[:nx*ny]
	clear(bits)
	marked := false
	for _, r := range rects {
		r = r.Intersect(image.Rect(0, 0, w, h))
		if r.Empty() {
			continue
		}
		marked = true
		for ty := r.Min.Y / damageTile; ty <= (r.Max.Y-1)/damageTile; ty++ {
			for tx := r.Min.X / damageTile; tx <= (r.Max.X-1)/damageTile; tx++ {
				bits[ty*nx+tx] = true
			}
		}
	}
	if !marked {
		return dst, bits
	}
	start := len(dst)
	for ty := range ny {
		rowStart := len(dst)
		y0, y1 := ty*damageTile, min((ty+1)*damageTile, h)
		for tx := 0; tx < nx; {
			if !bits[ty*nx+tx] {
				tx++
				continue
			}
			end := tx
			for end < nx && bits[ty*nx+end] {
				end++
			}
			x0, x1 := tx*damageTile, min(end*damageTile, w)
			merged := false
			for i := start; i < rowStart; i++ {
				if d := &dst[i]; d.Min.X == x0 && d.Max.X == x1 && d.Max.Y == y0 {
					d.Max.Y = y1
					merged = true
					break
				}
			}
			if !merged {
				dst = append(dst, image.Rect(x0, y0, x1, y1))
			}
			tx = end
		}
	}
	return dst, bits
}

// packRect returns the pixels of r from pix (premultiplied RGBA, w pixels per
// row) tightly packed, as WritePixels on a sub-image wants them, and the
// scratch buffer for reuse. A strip as wide as the frame is already
// contiguous and is returned as a sub-slice of pix; otherwise the rows are
// copied into scratch, grown as needed.
func packRect(scratch, pix []byte, w int, r image.Rectangle) (packed, newScratch []byte) {
	if r.Min.X == 0 && r.Max.X == w {
		return pix[4*r.Min.Y*w : 4*r.Max.Y*w], scratch
	}
	n := 4 * r.Dx() * r.Dy()
	if cap(scratch) < n {
		scratch = make([]byte, n)
	}
	packed = scratch[:n]
	row := 4 * r.Dx()
	for y := r.Min.Y; y < r.Max.Y; y++ {
		copy(packed[(y-r.Min.Y)*row:], pix[4*(y*w+r.Min.X):4*(y*w+r.Max.X)])
	}
	return packed, scratch
}
