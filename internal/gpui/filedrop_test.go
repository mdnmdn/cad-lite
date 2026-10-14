package gpui_test

import (
	"reflect"
	"testing"

	"cad-lite/internal/gpui"
)

var _ gpui.PlatformInput = gpui.FileDropEvent{}

func TestFileDropReachesOnFileDropAlongHitPath(t *testing.T) {
	var got []gpui.FileDropEvent
	var outer int
	root := gpui.RenderFunc(func(*gpui.Window, *gpui.App) gpui.IntoElement {
		return gpui.NewDiv().SizeFull().
			OnFileDrop(func(*gpui.FileDropEvent, *gpui.Window, *gpui.App) { outer++ }).
			Child(gpui.NewDiv().ID("inner").SizeWH(100, 100).
				OnFileDrop(func(e *gpui.FileDropEvent, _ *gpui.Window, _ *gpui.App) { got = append(got, *e) }))
	})
	hw, _ := openHeadless(t, 200, 200, root)
	paths := []string{"/a/one.dxf", "/a/two.dwg"}
	hw.Dispatch(gpui.FileDropEvent{Position: gpui.Point{X: 30, Y: 40}, Paths: paths, Modifiers: gpui.Alt})
	if len(got) != 1 || got[0].Position != (gpui.Point{X: 30, Y: 40}) || !reflect.DeepEqual(got[0].Paths, paths) || got[0].Modifiers != gpui.Alt {
		t.Fatalf("inner got %+v", got)
	}
	if outer != 1 {
		t.Fatalf("outer handler ran %d times, want 1 (bubbling)", outer)
	}
	hw.Dispatch(gpui.FileDropEvent{Position: gpui.Point{X: 150, Y: 150}, Paths: paths})
	if len(got) != 1 || outer != 2 {
		t.Fatalf("drop outside inner: inner=%d outer=%d", len(got), outer)
	}
}

func TestFileDropStopPropagationAndUIScale(t *testing.T) {
	outer := 0
	var at gpui.Point
	root := gpui.RenderFunc(func(*gpui.Window, *gpui.App) gpui.IntoElement {
		return gpui.NewDiv().SizeFull().
			OnFileDrop(func(*gpui.FileDropEvent, *gpui.Window, *gpui.App) { outer++ }).
			Child(gpui.NewDiv().ID("inner").SizeWH(100, 100).
				OnFileDrop(func(e *gpui.FileDropEvent, w *gpui.Window, _ *gpui.App) { at = e.Position; w.StopPropagation() }))
	})
	hw, _ := openHeadless(t, 400, 400, root)
	hw.Window().SetUIScale(2)
	hw.Dispatch(gpui.FileDropEvent{Position: gpui.Point{X: 100, Y: 60}, Paths: []string{"/x.dxf"}})
	if outer != 0 {
		t.Fatalf("outer ran %d times after StopPropagation", outer)
	}
	if at != (gpui.Point{X: 50, Y: 30}) {
		t.Fatalf("position = %v, want 50,30", at)
	}
}

func TestOpenPathsQueuesUntilHandlerIsSet(t *testing.T) {
	app := gpui.NewApp()
	app.OpenPaths(nil)
	src := []string{"/a.dxf"}
	app.OpenPaths(src)
	app.OpenPaths([]string{"/b.dwg", "/c.txt"})
	src[0] = "/changed"
	var got [][]string
	app.OnOpenPaths(func(paths []string, cx *gpui.App) {
		if cx != app {
			t.Errorf("handler got another App")
		}
		got = append(got, paths)
	})
	want := [][]string{{"/a.dxf"}, {"/b.dwg", "/c.txt"}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("queued requests = %v, want %v", got, want)
	}
	app.OpenPaths([]string{"/d.dxf"})
	if len(got) != 3 || got[2][0] != "/d.dxf" {
		t.Fatalf("live request not delivered: %v", got)
	}
	app.OnOpenPaths(nil)
	app.OpenPaths([]string{"/e.dxf"})
	if len(got) != 3 {
		t.Fatalf("request delivered after the handler was removed: %v", got)
	}
}
