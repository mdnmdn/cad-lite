package ebitenplatform

import (
	"math"
	"time"

	"github.com/hajimehoshi/ebiten/v2"

	"go-gpui/gpui"
)

const (
	wheelLinePixels = 20
	clickSlop       = 4
	multiClickGap   = 400 * time.Millisecond
	repeatDelay     = 30
	repeatInterval  = 3
)

type keyInfo struct {
	name      string
	printable bool
}

var keyTable = buildKeyTable()

func buildKeyTable() map[ebiten.Key]keyInfo {
	t := map[ebiten.Key]keyInfo{
		ebiten.KeyEnter:        {"enter", false},
		ebiten.KeyNumpadEnter:  {"enter", false},
		ebiten.KeyEscape:       {"escape", false},
		ebiten.KeyBackspace:    {"backspace", false},
		ebiten.KeyDelete:       {"delete", false},
		ebiten.KeyInsert:       {"insert", false},
		ebiten.KeyLeft:         {"left", false},
		ebiten.KeyRight:        {"right", false},
		ebiten.KeyUp:           {"up", false},
		ebiten.KeyDown:         {"down", false},
		ebiten.KeyHome:         {"home", false},
		ebiten.KeyEnd:          {"end", false},
		ebiten.KeyPageUp:       {"pageup", false},
		ebiten.KeyPageDown:     {"pagedown", false},
		ebiten.KeyTab:          {"tab", false},
		ebiten.KeySpace:        {"space", true},
		ebiten.KeyMinus:        {"-", true},
		ebiten.KeyEqual:        {"=", true},
		ebiten.KeyComma:        {",", true},
		ebiten.KeyPeriod:       {".", true},
		ebiten.KeySlash:        {"/", true},
		ebiten.KeySemicolon:    {";", true},
		ebiten.KeyQuote:        {"'", true},
		ebiten.KeyBracketLeft:  {"[", true},
		ebiten.KeyBracketRight: {"]", true},
		ebiten.KeyBackslash:    {"\\", true},
		ebiten.KeyBackquote:    {"`", true},
	}
	for i := 0; i < 26; i++ {
		t[ebiten.KeyA+ebiten.Key(i)] = keyInfo{string(rune('a' + i)), true}
	}
	for i := 0; i < 10; i++ {
		t[ebiten.KeyDigit0+ebiten.Key(i)] = keyInfo{string(rune('0' + i)), true}
	}
	for i := 0; i < 12; i++ {
		t[ebiten.KeyF1+ebiten.Key(i)] = keyInfo{"f" + itoa(i+1), false}
	}
	return t
}

func itoa(n int) string {
	if n >= 10 {
		return string(rune('0'+n/10)) + string(rune('0'+n%10))
	}
	return string(rune('0' + n))
}

// keyName maps an ebiten key to the gpui symbolic key name. printable
// reports whether the key normally produces a character (and so is delivered
// as text input unless a shortcut modifier is held). ok is false for keys with
// no gpui name, such as bare modifiers.
func keyName(k ebiten.Key) (name string, printable, ok bool) {
	ki, ok := keyTable[k]
	return ki.name, ki.printable, ok
}

// modifiersFor builds gpui modifiers from raw key state. On darwin Cmd
// (meta) maps to ModPlatform; elsewhere Ctrl maps to ModPlatform and the meta
// key maps to Control, so Cmd/Ctrl shortcuts behave the same for app authors.
func modifiersFor(goos string, shift, ctrl, alt, meta bool) gpui.Modifiers {
	var m gpui.Modifiers
	if shift {
		m |= gpui.Shift
	}
	if alt {
		m |= gpui.Alt
	}
	if goos == "darwin" {
		if meta {
			m |= gpui.ModPlatform
		}
		if ctrl {
			m |= gpui.Control
		}
	} else {
		if ctrl {
			m |= gpui.ModPlatform
		}
		if meta {
			m |= gpui.Control
		}
	}
	return m
}

// isShortcut reports whether m holds a modifier that turns a printable key
// into a keybinding rather than text input.
func isShortcut(m gpui.Modifiers) bool {
	return m&(gpui.Control|gpui.Alt|gpui.ModPlatform) != 0
}

// wheelDelta converts ebiten's wheel offsets (positive y = scroll up) into
// gpui logical-pixel deltas where positive DeltaY means scroll down.
func wheelDelta(x, y float64) (dx, dy gpui.Pixels) {
	return gpui.Pixels(-x * wheelLinePixels), gpui.Pixels(-y * wheelLinePixels)
}

// shouldFire reports whether a key held for duration ticks should emit a key
// event this tick: on press, and for repeatable keys after the repeat delay.
func shouldFire(duration int, repeatable bool) bool {
	if duration == 1 {
		return true
	}
	return repeatable && duration >= repeatDelay && (duration-repeatDelay)%repeatInterval == 0
}

// clipboardOp classifies a platform-modifier key press as a clipboard
// shortcut: "copy", "cut", "paste", or "" for anything else.
func clipboardOp(key string, m gpui.Modifiers) string {
	if m&gpui.ModPlatform == 0 || m&(gpui.Alt|gpui.Shift|gpui.Control) != 0 {
		return ""
	}
	switch key {
	case "c":
		return "copy"
	case "x":
		return "cut"
	case "v":
		return "paste"
	}
	return ""
}

// printableRunes filters ebiten input characters down to text a field should
// insert, dropping control characters.
func printableRunes(rs []rune) string {
	out := make([]rune, 0, len(rs))
	for _, r := range rs {
		if r >= 0x20 && r != 0x7f {
			out = append(out, r)
		}
	}
	return string(out)
}

type clickTracker struct {
	down     bool
	button   gpui.MouseButton
	hit      int
	lastAt   time.Time
	lastPos  gpui.Point
	lastBtn  gpui.MouseButton
	count    int
	hasPrior bool
}

func near(a, b gpui.Point) bool {
	return math.Abs(float64(a.X-b.X)) <= clickSlop && math.Abs(float64(a.Y-b.Y)) <= clickSlop
}

// press records a button press and returns its click count (1, 2, 3, ...
// for presses in quick succession at the same spot).
func (c *clickTracker) press(b gpui.MouseButton, p gpui.Point, hit int, now time.Time) int {
	if c.hasPrior && c.lastBtn == b && now.Sub(c.lastAt) <= multiClickGap && near(c.lastPos, p) {
		c.count++
	} else {
		c.count = 1
	}
	c.down, c.button, c.hit = true, b, hit
	c.lastAt, c.lastPos, c.lastBtn, c.hasPrior = now, p, b, true
	return c.count
}

// release ends a press and reports whether it completes a click, meaning the
// release landed on the same hitbox as the press.
func (c *clickTracker) release(b gpui.MouseButton, hit int) (clicked bool, count int) {
	if !c.down || c.button != b {
		return false, 0
	}
	c.down = false
	return c.hit == hit, c.count
}
