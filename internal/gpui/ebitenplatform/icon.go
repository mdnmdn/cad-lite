package ebitenplatform

import (
	"image"

	"github.com/hajimehoshi/ebiten/v2"
)

// WithWindowIcon sets the window (and, on Windows and Linux, taskbar) icon from imgs, which are
// the same picture at several sizes so the OS can pick the closest; it calls ebiten.SetWindowIcon.
// An empty slice leaves the default icon. macOS ignores it: the Dock icon comes from the .app
// bundle's AppIcon.icns.
func WithWindowIcon(imgs []image.Image) Option {
	return func(*Platform) {
		if len(imgs) > 0 {
			ebiten.SetWindowIcon(imgs)
		}
	}
}
