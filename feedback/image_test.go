package feedback

import (
	"bytes"
	"image"
	"image/color"
	"image/draw"
	"testing"
)

func TestEditorPenEraseUndoAndIsolation(t *testing.T) {
	original := image.NewNRGBA(image.Rect(0, 0, 100, 80))
	draw.Draw(original, original.Bounds(), &image.Uniform{color.NRGBA{20, 40, 60, 255}}, image.Point{}, draw.Src)
	before := bytes.Clone(original.Pix)
	e := NewEditor(original)
	ink := color.NRGBA{255, 100, 0, 255}
	e.Begin(Point{10, 40}, 8, ink, false)
	e.Move(Point{90, 40})
	e.End()
	if e.Image().NRGBAAt(50, 40) != ink {
		t.Fatal("stroke did not interpolate between pointer events")
	}
	pen := bytes.Clone(e.Image().Pix)
	e.Begin(Point{50, 40}, 12, ink, true)
	e.End()
	if e.Image().NRGBAAt(50, 40) != original.NRGBAAt(50, 40) {
		t.Fatal("eraser did not restore original capture")
	}
	e.Undo()
	if !bytes.Equal(pen, e.Image().Pix) {
		t.Fatal("undo did not restore pen stroke")
	}
	saved := e.Snapshot()
	saved.SetNRGBA(0, 0, ink)
	if e.Image().NRGBAAt(0, 0) == ink {
		t.Fatal("snapshot aliases editor")
	}
	e.Undo()
	if e.CanUndo() || !bytes.Equal(before, e.Image().Pix) || !bytes.Equal(before, original.Pix) {
		t.Fatal("cancel/undo changed original screenshot")
	}
}

func TestImageBoundsAndInvalidImports(t *testing.T) {
	img := PrepareImage(image.NewNRGBA(image.Rect(100, 100, 3100, 1600)))
	if img.Bounds().Min != (image.Point{}) || img.Bounds().Dx() != 2048 || img.Bounds().Dy() != 1024 {
		t.Fatalf("downscale: %v", img.Bounds())
	}
	if _, err := DecodeImage(bytes.NewReader([]byte("not an image"))); err == nil {
		t.Fatal("accepted invalid image")
	}
	if encoded, err := EncodeScreenshot(nil); err != nil || encoded != "" {
		t.Fatal("empty attachment not optional")
	}
}

func TestEraserRestoresTransparentPixels(t *testing.T) {
	original := image.NewNRGBA(image.Rect(0, 0, 20, 20))
	e := NewEditor(original)
	e.Begin(Point{10, 10}, 10, color.NRGBA{255, 0, 0, 255}, false)
	e.End()
	e.Begin(Point{10, 10}, 20, color.NRGBA{}, true)
	e.End()
	if e.Image().NRGBAAt(10, 10) != original.NRGBAAt(10, 10) {
		t.Fatal("eraser did not restore transparent original pixel")
	}
}

func TestEditorVersionChangesWithPixels(t *testing.T) {
	e := NewEditor(image.NewNRGBA(image.Rect(0, 0, 20, 20)))
	v := e.Version()
	if v == 0 {
		t.Fatal("version 0 means unversioned to gpui")
	}
	e.Begin(Point{X: 5, Y: 5}, 4, color.NRGBA{255, 0, 0, 255}, false)
	if e.Version() == v {
		t.Fatal("Begin changed pixels without a new version")
	}
	v = e.Version()
	e.Move(Point{X: 10, Y: 5})
	if e.Version() == v {
		t.Fatal("Move changed pixels without a new version")
	}
	v = e.Version()
	e.End()
	if e.Version() != v {
		t.Fatal("End changed the version without changing pixels")
	}
	e.Undo()
	if e.Version() == v {
		t.Fatal("Undo changed pixels without a new version")
	}
}
