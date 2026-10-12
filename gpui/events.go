package gpui

import (
	"fmt"
	"strings"
)

// DispatchPhase describes which half of event dispatch is running: the
// downward Capture phase (root toward target) or the upward Bubble phase
// (target back toward root).
type DispatchPhase uint8

const (
	// Capture is the downward phase, root toward the target element.
	Capture DispatchPhase = iota
	// Bubble is the upward phase, target back toward the root.
	Bubble
)

// Modifiers is a bitset of keyboard modifier keys held during an event.
type Modifiers uint8

const (
	// Shift is the shift key.
	Shift Modifiers = 1 << iota
	// Control is the control key.
	Control
	// Alt is the alt/option key.
	Alt
	// ModPlatform is Cmd on macOS and Super/Win on Windows/Linux (GPUI's
	// convention); physical Ctrl is always Control.
	ModPlatform
	// Function is the fn/function key.
	Function
)

// MouseButton identifies which physical button generated a mouse event.
type MouseButton uint8

const (
	// MouseLeft is the primary button.
	MouseLeft MouseButton = iota
	// MouseMiddle is the middle/auxiliary button.
	MouseMiddle
	// MouseRight is the secondary button.
	MouseRight
)

// MouseDownEvent is a button press at Position.
type MouseDownEvent struct {
	Position   Point
	Button     MouseButton
	Modifiers  Modifiers
	ClickCount int
}

// IsLeft reports whether the pressed button is the primary (left) button.
func (e MouseDownEvent) IsLeft() bool { return e.Button == MouseLeft }

// IsRight reports whether the pressed button is the secondary (right) button.
func (e MouseDownEvent) IsRight() bool { return e.Button == MouseRight }

func (e MouseDownEvent) isPlatformInput() {}

// MouseUpEvent is a button release at Position.
type MouseUpEvent struct {
	Position   Point
	Button     MouseButton
	Modifiers  Modifiers
	ClickCount int
}

// IsLeft reports whether the released button is the primary (left) button.
func (e MouseUpEvent) IsLeft() bool { return e.Button == MouseLeft }

func (e MouseUpEvent) isPlatformInput() {}

// MouseMoveEvent is a pointer motion to Position.
type MouseMoveEvent struct {
	Position  Point
	Modifiers Modifiers
}

func (e MouseMoveEvent) isPlatformInput() {}

// ScrollWheelEvent is a scroll at Position by DeltaX/DeltaY logical pixels.
// Positive DeltaY means scroll down (content moves up).
type ScrollWheelEvent struct {
	Position  Point
	DeltaX    Pixels
	DeltaY    Pixels
	Modifiers Modifiers
}

func (e ScrollWheelEvent) isPlatformInput() {}

// ClickEvent is a completed click (a press-release pair on the same target).
type ClickEvent struct {
	Up *MouseUpEvent
}

func (e ClickEvent) isPlatformInput() {}

// KeyDownEvent is a key press. PreferCharacterInput is true for events that
// should go to a focused text input rather than to keybindings (printable chars).
type KeyDownEvent struct {
	Keystroke            Keystroke
	PreferCharacterInput bool
}

func (e KeyDownEvent) isPlatformInput() {}

// KeyUpEvent is a key release.
type KeyUpEvent struct {
	Keystroke Keystroke
}

func (e KeyUpEvent) isPlatformInput() {}

// ModifiersChangedEvent reports that the set of held modifiers changed.
type ModifiersChangedEvent struct{ Modifiers Modifiers }

func (e ModifiersChangedEvent) isPlatformInput() {}

// PlatformInput is any of the input event structs above; the dispatcher
// type-switches on it.
type PlatformInput interface{ isPlatformInput() }

// Keystroke mirrors gpui's platform/keystroke.rs: a key plus the modifiers
// held with it. Key is a lower-case symbolic name such as "enter", "escape",
// "backspace", "a", "1", "left", "up", "space", "tab", "delete". KeyChar holds
// the printable character when the key produces one.
type Keystroke struct {
	Modifiers Modifiers
	Key       string
	KeyChar   *string
}

// NewKeystroke builds a Keystroke from a (lower-cased) key name and modifiers.
func NewKeystroke(key string, mods Modifiers) Keystroke {
	return Keystroke{Modifiers: mods, Key: strings.ToLower(key)}
}

// ParseKeystroke parses a GPUI-style binding string: zero or more modifier
// prefixes followed by a final key token, separated by "-". Modifiers are
// "shift", "control"/"ctrl", "alt"/"option", "cmd"/"command"/"platform"/"meta"
// and "fn"/"function". The final token is lower-cased as-is. Because "-" is
// the separator, it cannot appear inside a key name. An empty string, a
// trailing "-", or an unknown modifier is an error.
func ParseKeystroke(s string) (Keystroke, error) {
	if s == "" {
		return Keystroke{}, fmt.Errorf("gpui: empty keystroke")
	}
	parts := strings.Split(s, "-")
	var mods Modifiers
	for _, p := range parts[:len(parts)-1] {
		m, ok := parseModifierToken(p)
		if !ok {
			return Keystroke{}, fmt.Errorf("gpui: unknown modifier %q in keystroke %q", p, s)
		}
		mods |= m
	}
	key := parts[len(parts)-1]
	if key == "" {
		return Keystroke{}, fmt.Errorf("gpui: keystroke %q has an empty key", s)
	}
	return Keystroke{Modifiers: mods, Key: strings.ToLower(key)}, nil
}

// String renders the keystroke in the canonical modifier order
// "cmd-", "ctrl-", "alt-", "shift-", "fn-", then the key, so that
// ParseKeystroke(k.String()) round-trips for valid keystrokes.
func (k Keystroke) String() string {
	var b strings.Builder
	if k.hasModifier(ModPlatform) {
		b.WriteString("cmd-")
	}
	if k.hasModifier(Control) {
		b.WriteString("ctrl-")
	}
	if k.hasModifier(Alt) {
		b.WriteString("alt-")
	}
	if k.hasModifier(Shift) {
		b.WriteString("shift-")
	}
	if k.hasModifier(Function) {
		b.WriteString("fn-")
	}
	b.WriteString(k.Key)
	return b.String()
}

// Equals reports whether two keystrokes are the same binding: identical
// modifiers, the same key case-insensitively, and equal KeyChars (both nil, or
// both non-nil with the same value).
func (k Keystroke) Equals(o Keystroke) bool {
	if k.Modifiers != o.Modifiers || !strings.EqualFold(k.Key, o.Key) {
		return false
	}
	if k.KeyChar == nil || o.KeyChar == nil {
		return k.KeyChar == nil && o.KeyChar == nil
	}
	return *k.KeyChar == *o.KeyChar
}

// Matches reports whether typed satisfies this binding keystroke, as GPUI's
// should_match: the modifiers are equal and either the keys are equal
// (case-insensitively; typed.KeyChar is ignored) or typed.KeyChar equals this
// key, so a "?" binding matches a typed "?" whatever the physical key.
func (k Keystroke) Matches(typed Keystroke) bool {
	if k.Modifiers != typed.Modifiers {
		return false
	}
	if strings.EqualFold(k.Key, typed.Key) {
		return true
	}
	return typed.KeyChar != nil && *typed.KeyChar == k.Key
}

// hasModifier reports whether any bit of m is set.
func (k Keystroke) hasModifier(m Modifiers) bool { return k.Modifiers&m != 0 }

func parseModifierToken(s string) (Modifiers, bool) {
	switch strings.ToLower(s) {
	case "shift":
		return Shift, true
	case "control", "ctrl":
		return Control, true
	case "alt", "option":
		return Alt, true
	case "cmd", "command", "platform", "meta":
		return ModPlatform, true
	case "fn", "function":
		return Function, true
	}
	return 0, false
}
