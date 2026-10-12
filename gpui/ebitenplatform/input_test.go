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
	// GPUI convention on every OS: Ctrl is Control, Cmd/Super is Platform.
	if got := modifiersFor(true, false, false, true); got != gpui.Shift|gpui.ModPlatform {
		t.Errorf("shift+meta = %v", got)
	}
	if got := modifiersFor(false, true, false, false); got != gpui.Control {
		t.Errorf("ctrl = %v", got)
	}
	if got := modifiersFor(false, true, true, false); got != gpui.Control|gpui.Alt {
		t.Errorf("ctrl+alt = %v", got)
	}
}

func TestIsShortcut(t *testing.T) {
	cases := []struct {
		goos string
		m    gpui.Modifiers
		want bool
	}{
		{"darwin", 0, false},
		{"darwin", gpui.Shift, false},
		{"darwin", gpui.Alt, false},
		{"darwin", gpui.Alt | gpui.Shift, false},
		{"darwin", gpui.ModPlatform, true},
		{"darwin", gpui.Control, true},
		{"linux", gpui.Control, true},
		{"linux", gpui.Control | gpui.Alt, false},
		{"windows", gpui.Control | gpui.Alt | gpui.Shift, false},
		{"windows", gpui.Alt, false},
		{"linux", gpui.ModPlatform, true},
	}
	for _, c := range cases {
		if got := isShortcut(c.goos, c.m); got != c.want {
			t.Errorf("isShortcut(%s, %v) = %v, want %v", c.goos, c.m, got, c.want)
		}
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
	if clipboardOp("darwin", "v", gpui.ModPlatform) != "paste" || clipboardOp("darwin", "c", gpui.ModPlatform) != "copy" || clipboardOp("darwin", "x", gpui.ModPlatform) != "cut" {
		t.Error("darwin cmd+c/v/x")
	}
	if clipboardOp("darwin", "v", gpui.Control) != "" {
		t.Error("darwin ctrl+v must not paste")
	}
	for _, goos := range []string{"linux", "windows"} {
		if clipboardOp(goos, "v", gpui.Control) != "paste" || clipboardOp(goos, "c", gpui.Control) != "copy" || clipboardOp(goos, "x", gpui.Control) != "cut" {
			t.Errorf("%s ctrl+c/v/x", goos)
		}
		if clipboardOp(goos, "v", gpui.ModPlatform) != "" {
			t.Errorf("%s super+v must not paste", goos)
		}
	}
	if clipboardOp("darwin", "v", gpui.ModPlatform|gpui.Shift) != "" || clipboardOp("darwin", "v", 0) != "" || clipboardOp("darwin", "a", gpui.ModPlatform) != "" {
		t.Error("non-clipboard combos must not match")
	}
}

func TestPrintableRunes(t *testing.T) {
	if got := printableRunes([]rune{'a', '\r', 'é', 0x7f, '\t'}); got != "aé" {
		t.Errorf("got %q", got)
	}
}

func gid(keys ...string) gpui.GlobalElementID {
	var id gpui.GlobalElementID
	for i, k := range keys {
		id.Path = append(id.Path, gpui.ElementPathKey{Index: i, Key: k})
	}
	return id
}

func TestClickTracker(t *testing.T) {
	var c clickTracker
	t0 := time.Now()
	p := gpui.Point{X: 10, Y: 10}
	btn := gid("root", "btn")
	if n := c.press(gpui.MouseLeft, p, btn, true, t0); n != 1 {
		t.Fatalf("first press count %d", n)
	}
	// A redraw between press and release yields a fresh but equal id.
	if ok, _ := c.release(gpui.MouseLeft, gid("root", "btn"), true); !ok {
		t.Error("release on same element should click")
	}
	if n := c.press(gpui.MouseLeft, p, btn, true, t0.Add(100*time.Millisecond)); n != 2 {
		t.Errorf("double click count %d", n)
	}
	if ok, _ := c.release(gpui.MouseLeft, gid("root", "other"), true); ok {
		t.Error("release on another element should not click")
	}
	if n := c.press(gpui.MouseLeft, p, btn, true, t0.Add(2*time.Second)); n != 1 {
		t.Errorf("slow press count %d", n)
	}
	if ok, _ := c.release(gpui.MouseRight, btn, true); ok {
		t.Error("different button should not click")
	}
	if ok, _ := c.release(gpui.MouseLeft, btn, true); !ok {
		t.Error("left release after stray right release should still click")
	}
	c.press(gpui.MouseLeft, p, gpui.GlobalElementID{}, false, t0.Add(5*time.Second))
	if ok, _ := c.release(gpui.MouseLeft, gpui.GlobalElementID{}, false); ok {
		t.Error("press and release on nothing should not click")
	}
}

func TestClickTrackerPerButton(t *testing.T) {
	var c clickTracker
	t0 := time.Now()
	p := gpui.Point{X: 10, Y: 10}
	a, b := gid("a"), gid("b")
	c.press(gpui.MouseLeft, p, a, true, t0)
	c.press(gpui.MouseRight, p, b, true, t0)
	if ok, _ := c.release(gpui.MouseRight, b, true); !ok {
		t.Error("right click on b lost")
	}
	if ok, n := c.release(gpui.MouseLeft, a, true); !ok || n != 1 {
		t.Errorf("left click on a lost by interleaved right press: ok=%v n=%d", ok, n)
	}
}

func TestCharKeystrokeMatchesBareBindings(t *testing.T) {
	cases := []struct {
		r    rune
		mods gpui.Modifiers
		key  string
		want gpui.Modifiers
	}{
		{'a', 0, "a", 0},
		{'A', 0, "a", gpui.Shift},
		{' ', 0, "space", 0},
		{'?', 0, "?", 0},
		{'é', gpui.Function, "é", gpui.Function},
	}
	for _, c := range cases {
		ks := charKeystroke(c.r, c.mods)
		if ks.Key != c.key || ks.Modifiers != c.want || ks.KeyChar == nil || *ks.KeyChar != string(c.r) {
			t.Errorf("charKeystroke(%q) = %+v (char %v), want key %q mods %v", c.r, ks, ks.KeyChar, c.key, c.want)
		}
	}
	for _, b := range []string{"a", "space", "?", "shift-a"} {
		bk, err := gpui.ParseKeystroke(b)
		if err != nil {
			t.Fatal(err)
		}
		r := map[string]rune{"a": 'a', "space": ' ', "?": '?', "shift-a": 'A'}[b]
		if !bk.Matches(charKeystroke(r, 0)) {
			t.Errorf("binding %q does not match typed %q", b, r)
		}
	}
	if bk, _ := gpui.ParseKeystroke("a"); bk.Matches(charKeystroke('A', 0)) {
		t.Error(`binding "a" matches typed "A"`)
	}
}
