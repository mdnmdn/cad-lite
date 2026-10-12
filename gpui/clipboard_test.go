package gpui_test

import (
	"testing"

	"go-gpui/gpui"
)

func TestClipboardRoundTrip(t *testing.T) {
	if !gpui.ClipboardAvailable() {
		t.Skip("no clipboard tool available")
	}
	orig, _ := gpui.ClipboardRead()
	defer gpui.ClipboardWrite(orig)
	const want = "gpui clipboard test"
	if err := gpui.ClipboardWrite(want); err != nil {
		t.Skipf("clipboard not writable here: %v", err)
	}
	got, err := gpui.ClipboardRead()
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}
