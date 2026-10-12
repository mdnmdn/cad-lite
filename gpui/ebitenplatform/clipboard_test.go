package ebitenplatform

import (
	"errors"
	"testing"
	"time"

	"go-gpui/gpui"
)

type fakeHandler struct {
	text     string
	sel      gpui.IntRange
	replaced []string
}

func (h *fakeHandler) TextForRange(r gpui.IntRange) (string, bool) {
	if r.Start < 0 || r.End > len(h.text) || r.Start > r.End {
		return "", false
	}
	return h.text[r.Start:r.End], true
}
func (h *fakeHandler) SelectedTextRange() (gpui.UTF16Selection, bool) {
	return gpui.UTF16Selection{Range: h.sel}, true
}
func (h *fakeHandler) MarkedTextRange() (gpui.IntRange, bool) { return gpui.IntRange{}, false }
func (h *fakeHandler) UnmarkText()                            {}
func (h *fakeHandler) ReplaceTextInRange(r *gpui.IntRange, s string) {
	h.text = h.text[:r.Start] + s + h.text[r.End:]
	h.sel = gpui.IntRange{Start: r.Start + len(s), End: r.Start + len(s)}
	h.replaced = append(h.replaced, s)
}
func (h *fakeHandler) BoundsForRange(gpui.IntRange, gpui.Bounds) (gpui.Bounds, bool) {
	return gpui.Bounds{}, false
}
func (h *fakeHandler) CharacterIndexForPoint(gpui.Point, gpui.Bounds) (int, bool) { return 0, false }
func (h *fakeHandler) SetSelectedTextRange(r gpui.IntRange)                       { h.sel = r }
func (h *fakeHandler) TextLengthUTF16() (int, bool)                               { return len(h.text), true }
func (h *fakeHandler) AcceptsTextInput() bool                                     { return true }

// waitResult drains the worker until one result is applied.
func waitResult(t *testing.T, cw *clipWorker, apply func(clipResult)) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		got := false
		cw.drain(func(r clipResult) { got = true; apply(r) })
		if got {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("clipboard result never arrived")
}

func TestClipboardPasteIsAsync(t *testing.T) {
	cw := newClipWorker()
	release := make(chan struct{})
	read := func() (string, error) { <-release; return "a\nb", nil }
	start := time.Now()
	if !startClipboard(cw, "paste", nil, read, nil) {
		t.Fatal("paste not consumed")
	}
	if time.Since(start) > 500*time.Millisecond {
		t.Fatal("paste blocked the caller")
	}
	cw.drain(func(clipResult) { t.Fatal("result before the read finished") })
	close(release)
	var got []gpui.PlatformInput
	waitResult(t, cw, func(r clipResult) {
		applyClipboard(r, nil, func(ev gpui.PlatformInput) { got = append(got, ev) })
	})
	if len(got) != 1 {
		t.Fatalf("dispatched %d events", len(got))
	}
	kd := got[0].(gpui.KeyDownEvent)
	if !kd.PreferCharacterInput || kd.Keystroke.KeyChar == nil || *kd.Keystroke.KeyChar != "a b" {
		t.Fatalf("paste event = %+v", kd)
	}
}

func TestClipboardCutDeletesOnlyUnchangedSelection(t *testing.T) {
	var written string
	write := func(s string) error { written = s; return nil }

	h := &fakeHandler{text: "hello world", sel: gpui.IntRange{Start: 0, End: 5}}
	cw := newClipWorker()
	if !startClipboard(cw, "cut", h, nil, write) {
		t.Fatal("cut not consumed")
	}
	waitResult(t, cw, func(r clipResult) { applyClipboard(r, h, nil) })
	if written != "hello" || h.text != " world" {
		t.Fatalf("written=%q text=%q", written, h.text)
	}

	h = &fakeHandler{text: "hello world", sel: gpui.IntRange{Start: 0, End: 5}}
	startClipboard(cw, "cut", h, nil, write)
	h.sel = gpui.IntRange{Start: 6, End: 11}
	waitResult(t, cw, func(r clipResult) { applyClipboard(r, h, nil) })
	if h.text != "hello world" {
		t.Fatalf("stale cut modified text: %q", h.text)
	}

	h = &fakeHandler{text: "hello world", sel: gpui.IntRange{Start: 0, End: 5}}
	startClipboard(cw, "cut", h, nil, func(string) error { return errors.New("boom") })
	waitResult(t, cw, func(r clipResult) { applyClipboard(r, h, nil) })
	if h.text != "hello world" {
		t.Fatalf("failed write still cut: %q", h.text)
	}
}

func TestClipboardCopyNeedsSelection(t *testing.T) {
	cw := newClipWorker()
	if startClipboard(cw, "copy", nil, nil, nil) {
		t.Error("copy without handler consumed")
	}
	h := &fakeHandler{text: "abc", sel: gpui.IntRange{Start: 1, End: 1}}
	if startClipboard(cw, "copy", h, nil, nil) {
		t.Error("copy with empty selection consumed")
	}
}
