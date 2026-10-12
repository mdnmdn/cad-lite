package ebitenplatform

import (
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"
	"time"

	"go-gpui/gpui"
)

func TestSnapshotKeyOptions(t *testing.T) {
	if got := defaultSnapshotKey("darwin"); got != "cmd-shift-s" {
		t.Errorf("darwin default = %q", got)
	}
	if got := defaultSnapshotKey("linux"); got != "ctrl-shift-s" {
		t.Errorf("linux default = %q", got)
	}
	p := New().(*Platform)
	want, _ := gpui.ParseKeystroke(DefaultSnapshotKey())
	if p.snapshotKey == nil || *p.snapshotKey != want {
		t.Fatalf("default binding = %v, want %v", p.snapshotKey, want)
	}
	if !isSnapshot(p.snapshotKey, want) {
		t.Error("default keystroke should trigger the snapshot")
	}
	if isSnapshot(p.snapshotKey, gpui.Keystroke{Key: "s", Modifiers: want.Modifiers &^ gpui.Shift}) {
		t.Error("keystroke without shift should not trigger the snapshot")
	}
	if p := New(WithSnapshotKey("")).(*Platform); p.snapshotKey != nil || isSnapshot(p.snapshotKey, want) {
		t.Error(`WithSnapshotKey("") should disable the snapshot`)
	}
	p = New(WithSnapshotKey("f12")).(*Platform)
	if !isSnapshot(p.snapshotKey, gpui.Keystroke{Key: "f12"}) || isSnapshot(p.snapshotKey, want) {
		t.Error("custom binding not honoured")
	}
	defer func() {
		if recover() == nil {
			t.Error("malformed keystroke should panic")
		}
	}()
	WithSnapshotKey("bogus-s")
}

func TestWriteSnapshot(t *testing.T) {
	dir := t.TempDir()
	// 2x1 premultiplied: opaque red, half-transparent white.
	pix := []byte{255, 0, 0, 255, 128, 128, 128, 128}
	now := time.Date(2026, 10, 12, 9, 5, 7, 0, time.Local)
	p1, err := writeSnapshot(dir, pix, 2, 1, now)
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(dir, "snapshot-20261012-090507.png"); p1 != want {
		t.Errorf("path = %q, want %q", p1, want)
	}
	p2, err := writeSnapshot(dir, pix, 2, 1, now)
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(dir, "snapshot-20261012-090507-1.png"); p2 != want {
		t.Errorf("second path = %q, want %q", p2, want)
	}
	f, err := os.Open(p1)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	img, err := png.Decode(f)
	if err != nil {
		t.Fatal(err)
	}
	if b := img.Bounds(); b.Dx() != 2 || b.Dy() != 1 {
		t.Fatalf("size = %v", b)
	}
	if got := color.NRGBAModel.Convert(img.At(1, 0)).(color.NRGBA); got != (color.NRGBA{255, 255, 255, 128}) {
		t.Errorf("unpremultiplied pixel = %v", got)
	}
}
