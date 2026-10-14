package ebitenplatform

import (
	"errors"
	"fmt"
	"image"
	"image/png"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"cad-lite/internal/gpui"
)

// Option configures a Platform at construction.
type Option func(*Platform)

// DefaultSnapshotKey is the default frame-snapshot binding: "cmd-shift-s" on
// darwin, "ctrl-shift-s" elsewhere.
func DefaultSnapshotKey() string { return defaultSnapshotKey(runtime.GOOS) }

func defaultSnapshotKey(goos string) string {
	if goos == "darwin" {
		return "cmd-shift-s"
	}
	return "ctrl-shift-s"
}

// WithSnapshotKey binds the frame-snapshot shortcut to keystroke, written in
// keymap syntax (e.g. "cmd-shift-s"); "" disables the shortcut. It panics on
// a malformed keystroke, like gpui.NewKeyBinding.
func WithSnapshotKey(keystroke string) Option {
	if keystroke == "" {
		return func(p *Platform) { p.snapshotKey = nil }
	}
	ks, err := gpui.ParseKeystroke(keystroke)
	if err != nil {
		panic(err)
	}
	return func(p *Platform) { p.snapshotKey = &ks }
}

// isSnapshot reports whether the pressed keystroke ks triggers the snapshot
// binding (nil = disabled).
func isSnapshot(binding *gpui.Keystroke, ks gpui.Keystroke) bool {
	return binding != nil && binding.Key == ks.Key && binding.Modifiers == ks.Modifiers
}

// snapshot copies the last presented frame and encodes it to a PNG in dir on
// a separate goroutine, logging the resulting path (or error) to stderr.
func snapshot(dir string, pix []byte, w, h int, now time.Time) {
	if w <= 0 || h <= 0 || len(pix) != 4*w*h {
		fmt.Fprintln(os.Stderr, "gpui: snapshot: no frame presented yet")
		return
	}
	frame := append([]byte(nil), pix...)
	go func() {
		path, err := writeSnapshot(dir, frame, w, h, now)
		if err != nil {
			fmt.Fprintln(os.Stderr, "gpui: snapshot:", err)
			return
		}
		fmt.Fprintln(os.Stderr, "gpui: snapshot saved to", path)
	}()
}

// writeSnapshot encodes the premultiplied RGBA frame as a PNG named
// snapshot-YYYYMMDD-HHMMSS.png in dir (adding -N when the name is taken) and
// returns its path.
func writeSnapshot(dir string, pix []byte, w, h int, now time.Time) (string, error) {
	img := unpremultiply(pix, w, h)
	base := "snapshot-" + now.Format("20060102-150405")
	for n := 0; ; n++ {
		name := base + ".png"
		if n > 0 {
			name = fmt.Sprintf("%s-%d.png", base, n)
		}
		path := filepath.Join(dir, name)
		f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
		if errors.Is(err, fs.ErrExist) {
			continue
		}
		if err != nil {
			return "", err
		}
		if err := png.Encode(f, img); err != nil {
			f.Close()
			return "", err
		}
		return path, f.Close()
	}
}

// unpremultiply converts a premultiplied RGBA buffer to an *image.NRGBA.
func unpremultiply(pix []byte, w, h int) *image.NRGBA {
	out := make([]byte, len(pix))
	for i := 0; i+3 < len(pix); i += 4 {
		a := uint32(pix[i+3])
		switch a {
		case 0:
		case 255:
			copy(out[i:i+4], pix[i:i+4])
		default:
			for c := 0; c < 3; c++ {
				out[i+c] = uint8(min(255, (uint32(pix[i+c])*255+a/2)/a))
			}
			out[i+3] = pix[i+3]
		}
	}
	return &image.NRGBA{Pix: out, Stride: 4 * w, Rect: image.Rect(0, 0, w, h)}
}
