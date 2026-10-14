package gpui

import (
	"encoding/base64"
	"errors"
	"os/exec"
	"reflect"
	"testing"
	"time"
)

func imageToolNames(ts []imageTool) []string {
	var out []string
	for _, t := range ts {
		out = append(out, t.argv[0])
	}
	return out
}

func TestClipboardImageTools(t *testing.T) {
	env := func(m map[string]string) func(string) string { return func(k string) string { return m[k] } }
	if got := imageToolNames(clipboardImageTools("linux", env(nil))); !reflect.DeepEqual(got, []string{"xclip"}) {
		t.Errorf("X11 image tools = %v", got)
	}
	if got := imageToolNames(clipboardImageTools("linux", env(map[string]string{"WAYLAND_DISPLAY": "w"}))); !reflect.DeepEqual(got, []string{"wl-paste", "xclip"}) {
		t.Errorf("wayland image tools = %v", got)
	}
	if got := imageToolNames(clipboardImageTools("darwin", env(nil))); !reflect.DeepEqual(got, []string{"osascript"}) {
		t.Errorf("darwin image tools = %v", got)
	}
	win := clipboardImageTools("windows", env(nil))
	png, err := win[0].decode([]byte(base64.StdEncoding.EncodeToString(pngSignature) + "\r\n"))
	if err != nil || !reflect.DeepEqual(png, pngSignature) {
		t.Errorf("windows decode = %v, %v", png, err)
	}
}

func TestDecodeAppleScriptData(t *testing.T) {
	got, err := decodeAppleScriptData([]byte("«data PNGf89504E470D0A1A0A00»\n"))
	if err != nil || !reflect.DeepEqual(got, append(append([]byte(nil), pngSignature...), 0)) {
		t.Fatalf("decode = %x, %v", got, err)
	}
}

func TestReadImageWithChecksSignature(t *testing.T) {
	if _, err := exec.LookPath("printf"); err != nil {
		t.Skip("no printf")
	}
	raw := func(b []byte) ([]byte, error) { return b, nil }
	tools := []imageTool{
		{[]string{"gpui-no-such-tool"}, raw},
		{[]string{"printf", "not a png"}, raw},
		{[]string{"printf", `\211PNG\r\n\032\nrest`}, raw},
	}
	b, mime, err := readImageWith(tools, time.Second)
	if err != nil || mime != "image/png" || string(b) != "\x89PNG\r\n\x1a\nrest" {
		t.Fatalf("readImageWith = %q, %q, %v", b, mime, err)
	}
	if _, _, err := readImageWith(tools[:2], time.Second); !errors.Is(err, ErrNoClipboardImage) {
		t.Fatalf("text-only clipboard: %v", err)
	}
}
