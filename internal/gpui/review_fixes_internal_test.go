package gpui

import (
	"strings"
	"testing"
)

func TestGenericFamilyKeysAreNormalized(t *testing.T) {
	for k := range genericFamilies {
		if normalizeFontName(k) != k {
			t.Errorf("genericFamilies key %q never matches: normalizes to %q", k, normalizeFontName(k))
		}
	}
}

func TestTextInputPaintsPrepaintStyle(t *testing.T) {
	p := NewHeadlessPlatform()
	app := NewAppWithPlatform(p)
	ti := NewTextInput("x")
	if err := app.OpenWindow(WindowOptions{Width: 200, Height: 100}, func(*Window, *App) EntityRoot { return ti }); err != nil {
		t.Fatal(err)
	}
	w := app.Windows()[0]
	ie := ti.Render(w, app).(inputElement)
	st := &divPrepaint{d: ie.div, resolved: true, style: ie.div.resolvedStyle()}
	st.style.BorderColor = Red500
	w.nextFrame.reset()
	ie.Paint(w, app, ElementID{}, Bounds{Size: Size{W: 100, H: 30}}, nil, st)
	for _, q := range w.nextFrame.scene.Quads {
		if q.BorderColor == Red500 {
			return
		}
	}
	t.Fatal("TextInput ignored its resolved prepaint (paint) style")
}

func TestImplicitFocusStatePrunedWhenElementGone(t *testing.T) {
	show := true
	p := NewHeadlessPlatform()
	app := NewAppWithPlatform(p)
	err := app.OpenWindow(WindowOptions{Width: 100, Height: 100}, func(*Window, *App) EntityRoot {
		return RenderFunc(func(*Window, *App) IntoElement {
			d := NewDiv().SizeFull()
			if show {
				d.Child(NewDiv().ID("t").TabIndex(0).SizeWH(10, 10))
			}
			return d
		})
	})
	if err != nil {
		t.Fatal(err)
	}
	w := app.Windows()[0]
	focusStates := func() (n int, ids []FocusID) {
		for k, v := range w.elementStates {
			if strings.HasSuffix(k, "#focus") {
				n++
				ids = append(ids, v.(*FocusHandle).id)
			}
		}
		return
	}
	w.Draw(app)
	w.Draw(app)
	n, ids := focusStates()
	if n != 1 {
		t.Fatalf("implicit focus states = %d, want 1", n)
	}
	show = false
	w.Draw(app)
	if n, _ := focusStates(); n != 0 {
		t.Fatalf("implicit focus state kept after its element disappeared (%d)", n)
	}
	if app.focusMap.alive(ids[0]) {
		t.Fatal("pruned implicit focus handle still holds its focus id")
	}
	show = true
	w.Draw(app)
	if n, _ := focusStates(); n != 1 {
		t.Fatalf("implicit focus state not recreated (%d)", n)
	}
}
