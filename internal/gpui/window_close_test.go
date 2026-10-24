package gpui_test

import (
	"testing"

	"cad-lite/internal/gpui"
)

func TestRequestCloseAsksTheHandler(t *testing.T) {
	app := gpui.NewApp()
	if err := app.OpenWindow(gpui.WindowOptions{Width: 100, Height: 80}, func(*gpui.Window, *gpui.App) gpui.EntityRoot {
		return gpui.RenderFunc(func(*gpui.Window, *gpui.App) gpui.IntoElement { return gpui.NewDiv() })
	}); err != nil {
		t.Fatal(err)
	}
	win := app.Windows()[0]
	allow, asked := false, 0
	win.OnShouldClose(func(*gpui.Window, *gpui.App) bool { asked++; return allow })
	if win.RequestClose(app) || app.IsQuitting() || asked != 1 {
		t.Fatalf("a refusing handler: quitting %v, asked %d", app.IsQuitting(), asked)
	}
	allow = true
	if !win.RequestClose(app) || !app.IsQuitting() {
		t.Fatal("an allowing handler should quit the app")
	}
}

func TestRequestCloseWithoutHandlerQuits(t *testing.T) {
	app := gpui.NewApp()
	if err := app.OpenWindow(gpui.WindowOptions{Width: 100, Height: 80}, func(*gpui.Window, *gpui.App) gpui.EntityRoot {
		return gpui.RenderFunc(func(*gpui.Window, *gpui.App) gpui.IntoElement { return gpui.NewDiv() })
	}); err != nil {
		t.Fatal(err)
	}
	if !app.Windows()[0].RequestClose(app) || !app.IsQuitting() {
		t.Fatal("no handler: the window should close and the app quit")
	}
}
