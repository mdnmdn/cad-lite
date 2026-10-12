package gpui_test

import (
	"testing"

	"go-gpui/gpui"
)

var _ gpui.PlatformInput = gpui.MouseDownEvent{}
var _ gpui.PlatformInput = gpui.MouseUpEvent{}
var _ gpui.PlatformInput = gpui.MouseMoveEvent{}
var _ gpui.PlatformInput = gpui.ScrollWheelEvent{}
var _ gpui.PlatformInput = gpui.ClickEvent{}
var _ gpui.PlatformInput = gpui.KeyDownEvent{}
var _ gpui.PlatformInput = gpui.KeyUpEvent{}
var _ gpui.PlatformInput = gpui.ModifiersChangedEvent{}

func TestParseKeystroke(t *testing.T) {
	tests := []struct {
		in   string
		mods gpui.Modifiers
		key  string
	}{
		{"cmd-shift-p", gpui.ModPlatform | gpui.Shift, "p"},
		{"enter", 0, "enter"},
		{"escape", 0, "escape"},
		{"ctrl-a", gpui.Control, "a"},
		{"shift-left", gpui.Shift, "left"},
		{"cmd-enter", gpui.ModPlatform, "enter"},
		{"backspace", 0, "backspace"},
		{"space", 0, "space"},
		{"tab", 0, "tab"},
		{"1", 0, "1"},
		{"a", 0, "a"},
	}
	for _, tt := range tests {
		ks, err := gpui.ParseKeystroke(tt.in)
		if err != nil {
			t.Errorf("ParseKeystroke(%q) error: %v", tt.in, err)
			continue
		}
		if ks.Modifiers != tt.mods {
			t.Errorf("ParseKeystroke(%q).Modifiers = %b, want %b", tt.in, ks.Modifiers, tt.mods)
		}
		if ks.Key != tt.key {
			t.Errorf("ParseKeystroke(%q).Key = %q, want %q", tt.in, ks.Key, tt.key)
		}
		if ks.KeyChar != nil {
			t.Errorf("ParseKeystroke(%q).KeyChar = %v, want nil", tt.in, *ks.KeyChar)
		}
	}
}

func TestParseKeystrokeAliases(t *testing.T) {
	aliases := []struct {
		in       string
		wantMods gpui.Modifiers
	}{
		{"command-a", gpui.ModPlatform},
		{"platform-a", gpui.ModPlatform},
		{"meta-a", gpui.ModPlatform},
		{"control-a", gpui.Control},
		{"option-a", gpui.Alt},
		{"fn-a", gpui.Function},
		{"function-a", gpui.Function},
		{"COMMAND-ENTER", gpui.ModPlatform},
	}
	for _, tt := range aliases {
		ks, err := gpui.ParseKeystroke(tt.in)
		if err != nil {
			t.Errorf("ParseKeystroke(%q) error: %v", tt.in, err)
			continue
		}
		if ks.Modifiers != tt.wantMods {
			t.Errorf("ParseKeystroke(%q).Modifiers = %b, want %b", tt.in, ks.Modifiers, tt.wantMods)
		}
	}
	if ks, err := gpui.ParseKeystroke("cmd-Enter"); err != nil || ks.Key != "enter" {
		t.Errorf("ParseKeystroke(\"cmd-Enter\") = %v, %v; want key \"enter\"", ks, err)
	}
}

func TestParseKeystrokeErrors(t *testing.T) {
	for _, in := range []string{"", "cmd-", "bogus-a", "-a"} {
		if _, err := gpui.ParseKeystroke(in); err == nil {
			t.Errorf("ParseKeystroke(%q) succeeded, want error", in)
		}
	}
}

func TestKeystrokeStringRoundTrip(t *testing.T) {
	inputs := []string{
		"cmd-shift-p",
		"enter",
		"escape",
		"ctrl-a",
		"shift-left",
		"cmd-enter",
		"backspace",
		"space",
		"tab",
		"1",
		"a",
		"cmd-ctrl-alt-shift-fn-x",
	}
	for _, in := range inputs {
		ks, err := gpui.ParseKeystroke(in)
		if err != nil {
			t.Fatalf("ParseKeystroke(%q) error: %v", in, err)
		}
		again, err := gpui.ParseKeystroke(ks.String())
		if err != nil {
			t.Errorf("ParseKeystroke(%q.String()=%q) error: %v", in, ks.String(), err)
			continue
		}
		if !ks.Equals(again) {
			t.Errorf("round-trip of %q: %v != %v", in, ks, again)
		}
	}
}

func strPtr(s string) *string { return &s }

func TestKeystrokeEquals(t *testing.T) {
	base := gpui.Keystroke{Modifiers: gpui.ModPlatform | gpui.Shift, Key: "p"}

	if !base.Equals(gpui.Keystroke{Modifiers: gpui.ModPlatform | gpui.Shift, Key: "p"}) {
		t.Error("same modifiers+key should be equal")
	}
	if base.Equals(gpui.Keystroke{Modifiers: gpui.ModPlatform, Key: "p"}) {
		t.Error("differing modifiers should not be equal")
	}
	if !(gpui.Keystroke{Key: "A"}).Equals(gpui.Keystroke{Key: "a"}) {
		t.Error("keys differing only by case should be equal")
	}
	withChar := gpui.Keystroke{Key: "a", KeyChar: strPtr("a")}
	if withChar.Equals(gpui.Keystroke{Key: "a"}) {
		t.Error("KeyChar nil vs non-nil should not be equal")
	}
	if !withChar.Equals(gpui.Keystroke{Key: "a", KeyChar: strPtr("a")}) {
		t.Error("equal non-nil KeyChars should be equal")
	}
	other := strPtr("b")
	if withChar.Equals(gpui.Keystroke{Key: "a", KeyChar: other}) {
		t.Error("differing KeyChars should not be equal")
	}
}

func TestNewKeystroke(t *testing.T) {
	ks := gpui.NewKeystroke("Enter", gpui.ModPlatform)
	if ks.Key != "enter" || ks.Modifiers != gpui.ModPlatform {
		t.Errorf("NewKeystroke(\"Enter\", Platform) = %v, want key \"enter\" mods %b", ks, gpui.ModPlatform)
	}
}

// classify mirrors the type switch the window dispatcher performs. It also
// proves every event type is reachable through the PlatformInput interface,
// which the compile-time assertions at the top of this file already pin.
func classify(in gpui.PlatformInput) string {
	switch in.(type) {
	case gpui.MouseDownEvent:
		return "mouse-down"
	case gpui.MouseUpEvent:
		return "mouse-up"
	case gpui.MouseMoveEvent:
		return "mouse-move"
	case gpui.ScrollWheelEvent:
		return "scroll"
	case gpui.ClickEvent:
		return "click"
	case gpui.KeyDownEvent:
		return "key-down"
	case gpui.KeyUpEvent:
		return "key-up"
	case gpui.ModifiersChangedEvent:
		return "modifiers"
	default:
		return "unknown"
	}
}

func TestPlatformInputTypeSwitch(t *testing.T) {
	cases := []struct {
		in   gpui.PlatformInput
		want string
	}{
		{gpui.MouseDownEvent{}, "mouse-down"},
		{gpui.MouseUpEvent{}, "mouse-up"},
		{gpui.MouseMoveEvent{}, "mouse-move"},
		{gpui.ScrollWheelEvent{}, "scroll"},
		{gpui.ClickEvent{}, "click"},
		{gpui.KeyDownEvent{}, "key-down"},
		{gpui.KeyUpEvent{}, "key-up"},
		{gpui.ModifiersChangedEvent{}, "modifiers"},
	}
	for _, c := range cases {
		if got := classify(c.in); got != c.want {
			t.Errorf("classify(%T) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestMouseButtonHelpers(t *testing.T) {
	down := gpui.MouseDownEvent{Button: gpui.MouseLeft}
	if !down.IsLeft() || down.IsRight() {
		t.Error("MouseDownEvent{MouseLeft}: IsLeft want true, IsRight false")
	}
	down = gpui.MouseDownEvent{Button: gpui.MouseRight}
	if down.IsLeft() || !down.IsRight() {
		t.Error("MouseDownEvent{MouseRight}: IsLeft want false, IsRight true")
	}
	up := gpui.MouseUpEvent{Button: gpui.MouseLeft}
	if !up.IsLeft() {
		t.Error("MouseUpEvent{MouseLeft}: IsLeft want true")
	}
	up = gpui.MouseUpEvent{Button: gpui.MouseMiddle}
	if up.IsLeft() {
		t.Error("MouseUpEvent{MouseMiddle}: IsLeft want false")
	}
}
