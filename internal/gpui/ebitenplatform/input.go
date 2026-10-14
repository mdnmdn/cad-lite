package ebitenplatform

import (
	"math"
	"time"

	"github.com/hajimehoshi/ebiten/v2"

	"cad-lite/internal/gpui"
)

const (
	wheelLinePixels = 20
	// wheelPrecisePixels is the logical pixels per ebiten wheel unit for
	// precise (trackpad) events; GLFW already scales macOS precise deltas by 0.1.
	wheelPrecisePixels = 10
	// wheelPreciseHold is how long an integral delta after a precise event
	// still counts as precise (a trackpad swipe emits the odd whole value).
	wheelPreciseHold = 250 * time.Millisecond
	// wheelNotchPixels is the smallest integral browser delta (CSS pixels)
	// taken as a mouse notch; browsers report a notch as 100 or 120.
	wheelNotchPixels = 50
	clickSlop        = 4
	multiClickGap    = 400 * time.Millisecond
	repeatDelay      = 30
	repeatInterval   = 3
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

// modifiersFor builds gpui modifiers from raw key state using GPUI's
// convention on every OS: Ctrl is Control and the meta key (Cmd on macOS,
// Super/Win elsewhere) is ModPlatform.
func modifiersFor(shift, ctrl, alt, meta bool) gpui.Modifiers {
	var m gpui.Modifiers
	if shift {
		m |= gpui.Shift
	}
	if ctrl {
		m |= gpui.Control
	}
	if alt {
		m |= gpui.Alt
	}
	if meta {
		m |= gpui.ModPlatform
	}
	return m
}

// isShortcut reports whether m turns a printable key into a keybinding
// rather than text input: ModPlatform (Cmd/Super), or Ctrl unless it is
// AltGr (reported as Ctrl+Alt off macOS). Alt/Option alone never does, so
// Option-composed characters (@ # [ ] on many layouts) reach text input.
func isShortcut(goos string, m gpui.Modifiers) bool {
	if m&gpui.ModPlatform != 0 {
		return true
	}
	if m&gpui.Control == 0 {
		return false
	}
	return goos == "darwin" || m&gpui.Alt == 0
}

// wheelClassifier decides whether a wheel event is precise (trackpad) or a
// notch (mouse). ebiten exposes no device kind, so it works from the values:
// a fractional delta or a two-axis delta is precise, and so is an integral
// delta within wheelPreciseHold of a precise one; any other integral delta is
// a notch. pixels is true when ebiten's units are CSS pixels (GOOS=js): there
// every integral delta of wheelNotchPixels or more is a notch.
type wheelClassifier struct {
	pixels      bool
	lastPrecise time.Time
}

func integral(v float64) bool { return math.Abs(v-math.Round(v)) < 1e-6 }

// classify reports whether the event at now is precise and returns its
// logical-pixel deltas (positive dy = scroll down).
func (c *wheelClassifier) classify(x, y float64, now time.Time) (dx, dy gpui.Pixels, precise bool) {
	switch {
	case !integral(x) || !integral(y) || (x != 0 && y != 0):
		precise = true
	case c.pixels:
		precise = math.Max(math.Abs(x), math.Abs(y)) < wheelNotchPixels
	default:
		precise = !c.lastPrecise.IsZero() && now.Sub(c.lastPrecise) < wheelPreciseHold
	}
	if precise {
		c.lastPrecise = now
	}
	scale := float64(wheelLinePixels)
	switch {
	case c.pixels:
		scale = 1
	case precise:
		scale = wheelPrecisePixels
	}
	return gpui.Pixels(-x * scale), gpui.Pixels(-y * scale), precise
}

// pointerTracker withholds the pointer position until it is known to be a
// real one. Before the pointer enters the window the backend reports a
// meaningless position (the window centre on some platforms), so the first
// sample only counts once it changes or the user clicks or scrolls.
type pointerTracker struct {
	sampled, known bool
	first, last    gpui.Point
}

// observe reports whether pos should be dispatched as a MouseMoveEvent.
// interacted is true when a button was pressed or the wheel turned.
func (t *pointerTracker) observe(pos gpui.Point, interacted bool) bool {
	if !t.sampled {
		t.sampled, t.first = true, pos
	}
	if !t.known {
		if pos == t.first && !interacted {
			return false
		}
		t.known, t.last = true, pos
		return true
	}
	if pos == t.last {
		return false
	}
	t.last = pos
	return true
}

// shouldFire reports whether a key held for duration ticks should emit a key
// event this tick: on press, and for repeatable keys after the repeat delay.
func shouldFire(duration int, repeatable bool) bool {
	if duration == 1 {
		return true
	}
	return repeatable && duration >= repeatDelay && (duration-repeatDelay)%repeatInterval == 0
}

// clipboardOp classifies a key press as a clipboard shortcut — cmd-c/x/v on
// darwin, ctrl-c/x/v elsewhere — returning "copy", "cut", "paste", or "".
func clipboardOp(goos, key string, m gpui.Modifiers) string {
	want := gpui.Control
	if goos == "darwin" {
		want = gpui.ModPlatform
	}
	if m&(gpui.Shift|gpui.Control|gpui.Alt|gpui.ModPlatform) != want {
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

// charKeystroke builds the keystroke of a typed character so that bare
// bindings match it (GPUI's key vs key_char): space is "space", an ASCII
// capital is its lower-case key plus Shift, anything else is the character
// itself. KeyChar always carries the character for text input.
func charKeystroke(r rune, mods gpui.Modifiers) gpui.Keystroke {
	s := string(r)
	key := s
	switch {
	case r == ' ':
		key = "space"
	case r >= 'A' && r <= 'Z':
		key = string(r - 'A' + 'a')
		mods |= gpui.Shift
	}
	return gpui.Keystroke{Modifiers: mods, Key: key, KeyChar: &s}
}

// buttonPress is the pending press of one mouse button.
type buttonPress struct {
	down   bool
	hit    gpui.GlobalElementID
	hasHit bool
	count  int
}

// clickTracker pairs presses with releases per button and counts
// multi-clicks. Targets are element ids, which survive redraws.
type clickTracker struct {
	buttons  [3]buttonPress
	lastAt   time.Time
	lastPos  gpui.Point
	lastBtn  gpui.MouseButton
	count    int
	hasPrior bool
}

func near(a, b gpui.Point) bool {
	return math.Abs(float64(a.X-b.X)) <= clickSlop && math.Abs(float64(a.Y-b.Y)) <= clickSlop
}

// press records a press of b over hit (hasHit false when nothing was hit)
// and returns its click count (1, 2, 3, ... for presses in quick succession
// at the same spot).
func (c *clickTracker) press(b gpui.MouseButton, p gpui.Point, hit gpui.GlobalElementID, hasHit bool, now time.Time) int {
	if c.hasPrior && c.lastBtn == b && now.Sub(c.lastAt) <= multiClickGap && near(c.lastPos, p) {
		c.count++
	} else {
		c.count = 1
	}
	c.lastAt, c.lastPos, c.lastBtn, c.hasPrior = now, p, b, true
	if int(b) < len(c.buttons) {
		c.buttons[b] = buttonPress{down: true, hit: hit, hasHit: hasHit, count: c.count}
	}
	return c.count
}

// release ends b's press and reports whether it completes a click, meaning
// the release landed on the same element as the press.
func (c *clickTracker) release(b gpui.MouseButton, hit gpui.GlobalElementID, hasHit bool) (clicked bool, count int) {
	if int(b) >= len(c.buttons) || !c.buttons[b].down {
		return false, 0
	}
	st := c.buttons[b]
	c.buttons[b].down = false
	return st.hasHit && hasHit && st.hit.Equal(hit), st.count
}
