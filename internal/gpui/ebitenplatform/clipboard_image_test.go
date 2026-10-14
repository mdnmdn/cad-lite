package ebitenplatform

import (
	"errors"
	"testing"

	"cad-lite/internal/gpui"
)

func pasteOnce(t *testing.T, cw *clipWorker, read func() (string, error)) []gpui.PlatformInput {
	t.Helper()
	startClipboard(cw, "paste", nil, read, nil)
	var got []gpui.PlatformInput
	waitResult(t, cw, func(r clipResult) {
		applyClipboard(r, nil, func(ev gpui.PlatformInput) { got = append(got, ev) })
	})
	return got
}

func TestClipboardPasteFallsBackToImage(t *testing.T) {
	cw := newClipWorker()
	cw.readImage = func() ([]byte, string, error) { return []byte("\x89PNG"), "image/png", nil }
	got := pasteOnce(t, cw, func() (string, error) { return "", nil })
	if len(got) != 1 {
		t.Fatalf("dispatched %d events", len(got))
	}
	ip, ok := got[0].(gpui.ImagePasteEvent)
	if !ok || string(ip.Data) != "\x89PNG" || ip.MIME != "image/png" {
		t.Fatalf("paste event = %#v", got[0])
	}

	// Text wins over an image.
	got = pasteOnce(t, cw, func() (string, error) { return "hi", nil })
	if _, ok := got[0].(gpui.KeyDownEvent); !ok || len(got) != 1 {
		t.Fatalf("text paste = %#v", got)
	}
	// Neither text nor an image dispatches nothing.
	cw.readImage = func() ([]byte, string, error) { return nil, "", gpui.ErrNoClipboardImage }
	if got = pasteOnce(t, cw, func() (string, error) { return "", errors.New("no tool") }); len(got) != 0 {
		t.Fatalf("empty clipboard dispatched %#v", got)
	}
}
