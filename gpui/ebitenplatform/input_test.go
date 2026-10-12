package ebitenplatform

import (
	"testing"
	"time"

	"github.com/hajimehoshi/ebiten/v2"

	"go-gpui/gpui"
)

func TestKeyName(t *testing.T) {
	cases := []struct {
		k         ebiten.Key
		name      string
		printable bool
	}{
		{ebiten.KeyA, "a", true},
		{ebiten.KeyZ, "z", true},
		{ebiten.KeyDigit7, "7", true},
		{ebiten.KeyEnter, "enter", false},
		{ebiten.KeyNumpadEnter, "enter", false},
		{ebiten.KeySpace, "space", true},
		{ebiten.KeyF12, "f12", false},
		{ebiten.KeyArrowLeft, "left", false},
		{ebiten.KeyBackspace, "backspace", false},
	}
	for _, c := range cases {
		name, printable, ok := keyName(c.k)
		if !ok || name != c.name || printable != c.printable {
			t.Errorf("keyName(%v) = %q,%v,%v", c.k, name, printable, ok)
		}
	}
	if _, _, ok := keyName(ebiten.KeyShiftLeft); ok {
		t.Error("modifier keys must not map")
	}
}

func TestModifiersFor(t *testing.T) {
	if got := modifiersFor("darwin", true, false, false, true); got != gpui.Shift|gpui.ModPlatform {
		t.Errorf("darwin shift+cmd = %v", got)
	}
	if got := modifiersFor("darwin", false, true, false, false); got != gpui.Control {
		t.Errorf("darwin ctrl = %v", got)
	}
	if got := modifiersFor("linux", false, true, true, false); got != gpui.ModPlatform|gpui.Alt {
		t.Errorf("linux ctrl+alt = %v", got)
	}
	if got := modifiersFor("windows", false, false, false, true); got != gpui.Control {
		t.Errorf("windows meta = %v", got)
	}
}

func TestWheelDelta(t *testing.T) {
	dx, dy := wheelDelta(0, -1)
	if dx != 0 || dy <= 0 {
		t.Errorf("wheel down should be positive DeltaY, got %v,%v", dx, dy)
	}
	if _, dy := wheelDelta(0, 2); dy >= 0 {
		t.Errorf("wheel up should be negative DeltaY, got %v", dy)
	}
}

func TestShouldFire(t *testing.T) {
	if !shouldFire(1, false) || shouldFire(2, false) || shouldFire(repeatDelay, false) {
		t.Error("non-repeatable keys fire only on press")
	}
	if !shouldFire(repeatDelay, true) || shouldFire(repeatDelay+1, true) || !shouldFire(repeatDelay+repeatInterval, true) {
		t.Error("repeatable key cadence wrong")
	}
}

func TestClipboardOp(t *testing.T) {
	if clipboardOp("v", gpui.ModPlatform) != "paste" || clipboardOp("c", gpui.ModPlatform) != "copy" || clipboardOp("x", gpui.ModPlatform) != "cut" {
		t.Error("platform+c/v/x")
	}
	if clipboardOp("v", gpui.ModPlatform|gpui.Shift) != "" || clipboardOp("v", 0) != "" || clipboardOp("a", gpui.ModPlatform) != "" {
		t.Error("non-clipboard combos must not match")
	}
}

func TestPrintableRunes(t *testing.T) {
	if got := printableRunes([]rune{'a', '\r', 'é', 0x7f, '\t'}); got != "aé" {
		t.Errorf("got %q", got)
	}
}

func TestClickTracker(t *testing.T) {
	var c clickTracker
	t0 := time.Now()
	p := gpui.Point{X: 10, Y: 10}
	if n := c.press(gpui.MouseLeft, p, t0); n != 1 {
		t.Fatalf("first press count %d", n)
	}
	if ok, _ := c.release(gpui.MouseLeft, gpui.Point{X: 12, Y: 11}); !ok {
		t.Error("release near press should click")
	}
	if n := c.press(gpui.MouseLeft, p, t0.Add(100*time.Millisecond)); n != 2 {
		t.Errorf("double click count %d", n)
	}
	if ok, _ := c.release(gpui.MouseLeft, gpui.Point{X: 100, Y: 100}); ok {
		t.Error("release far away should not click")
	}
	if n := c.press(gpui.MouseLeft, p, t0.Add(2*time.Second)); n != 1 {
		t.Errorf("slow press count %d", n)
	}
	if ok, _ := c.release(gpui.MouseRight, p); ok {
		t.Error("different button should not click")
	}
}
