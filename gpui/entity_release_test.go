package gpui_test

import (
	"fmt"
	"testing"

	"go-gpui/gpui"
)

type relCounter struct{ N int }

func (c *relCounter) Render(*gpui.Window, *gpui.App) gpui.IntoElement {
	return gpui.NewDiv().SizeFull().Child(gpui.NewText(fmt.Sprint(c.N)))
}

func TestEntityReleaseWhenLastStrongHandleDropped(t *testing.T) {
	app := gpui.NewApp()
	ent := gpui.NewEntity(app, func(*gpui.Context[relCounter]) relCounter { return relCounter{N: 1} })
	weak := ent.Downgrade()
	extra := ent.Clone()

	var released, observed int
	ent.Update(app, func(_ *relCounter, cx *gpui.Context[relCounter]) {
		cx.OnRelease(func(*gpui.App) { released++ })
	})
	sub := gpui.ObserveOn(app, ent, func(gpui.Entity[relCounter], *gpui.App) { observed++ })

	ent.Release(app)
	if _, ok := weak.Upgrade(); !ok {
		t.Fatal("entity released while a cloned handle is still alive")
	}
	extra.Release(app)
	if _, ok := weak.Upgrade(); ok {
		t.Fatal("Upgrade succeeded after the last strong handle was released")
	}
	if ent.Read(app) != nil {
		t.Fatal("Read of a released entity returned state")
	}
	if released != 1 {
		t.Fatalf("OnRelease ran %d times, want 1", released)
	}
	app.Notify(ent.ID())
	app.Update(func(*gpui.App) {})
	if observed != 0 {
		t.Fatalf("observer of a released entity ran %d times", observed)
	}
	sub.Close() // closing after teardown is a no-op

	// The slot is reused with a new generation: the old handle stays dead.
	ent2 := gpui.NewEntity(app, func(*gpui.Context[relCounter]) relCounter { return relCounter{N: 2} })
	if ent2.ID() == ent.ID() {
		t.Fatal("reused slot kept the old generation")
	}
	if ent.Read(app) != nil {
		t.Fatal("stale handle reads the new entity")
	}
}

func TestEntityReleaseDefersToEndOfUpdate(t *testing.T) {
	app := gpui.NewApp()
	ent := gpui.NewEntity(app, func(*gpui.Context[relCounter]) relCounter { return relCounter{} })
	app.Update(func(cx *gpui.App) {
		ent.Release(cx)
		if ent.Read(cx) == nil {
			t.Error("entity released before the effect flush")
		}
	})
	if ent.Read(app) != nil {
		t.Error("entity not released at the end of Update")
	}
}

func TestContextObserveClosedOnRelease(t *testing.T) {
	app := gpui.NewApp()
	target := gpui.NewEntity(app, func(*gpui.Context[relCounter]) relCounter { return relCounter{} })
	watcher := gpui.NewEntity(app, func(*gpui.Context[relCounter]) relCounter { return relCounter{} })
	calls := 0
	watcher.Update(app, func(_ *relCounter, cx *gpui.Context[relCounter]) {
		cx.Observe(target, func(gpui.Entity[relCounter], *gpui.App) { calls++ })
	})
	app.Update(func(cx *gpui.App) { target.Notify(cx) })
	if calls != 1 {
		t.Fatalf("calls = %d, want 1", calls)
	}
	watcher.Release(app)
	app.Update(func(cx *gpui.App) { target.Notify(cx) })
	if calls != 1 {
		t.Fatalf("observer owned by a released entity still ran (calls = %d)", calls)
	}
}

func TestNotifyRedrawsWindowsThatRenderedTheEntity(t *testing.T) {
	p := gpui.NewHeadlessPlatform()
	app := gpui.NewAppWithPlatform(p)
	ent := gpui.NewEntity(app, func(*gpui.Context[relCounter]) relCounter { return relCounter{} })
	other := gpui.NewEntity(app, func(*gpui.Context[relCounter]) relCounter { return relCounter{} })
	draws := 0
	_ = app.OpenWindow(gpui.WindowOptions{Width: 100, Height: 50}, func(*gpui.Window, *gpui.App) gpui.EntityRoot {
		return gpui.RenderFunc(func(w *gpui.Window, cx *gpui.App) gpui.IntoElement {
			draws++
			return gpui.NewText(fmt.Sprint(ent.Read(cx).N))
		})
	})
	hw := p.Windows()[0]
	hw.Pixels()
	before := draws

	app.Update(func(cx *gpui.App) {
		other.Update(cx, func(c *relCounter, ecx *gpui.Context[relCounter]) { c.N++; ecx.Notify() })
	})
	hw.Pixels()
	if draws != before {
		t.Fatalf("notifying an entity the window never rendered redrew it")
	}

	app.Update(func(cx *gpui.App) {
		ent.Update(cx, func(c *relCounter, ecx *gpui.Context[relCounter]) { c.N++; ecx.Notify() })
	})
	hw.Pixels()
	if draws != before+1 {
		t.Fatalf("draws = %d, want %d: Notify must redraw a window that rendered the entity", draws, before+1)
	}

	app.RefreshWindows()
	app.Update(func(*gpui.App) {})
	hw.Pixels()
	if draws != before+2 {
		t.Fatalf("draws = %d, want %d after RefreshWindows", draws, before+2)
	}
}

func TestWindowCloseReleasesEntityViewRoot(t *testing.T) {
	p := gpui.NewHeadlessPlatform()
	app := gpui.NewAppWithPlatform(p)
	ent := gpui.NewEntity(app, func(*gpui.Context[relCounter]) relCounter { return relCounter{N: 7} })
	_ = app.OpenWindow(gpui.WindowOptions{Width: 100, Height: 50}, func(*gpui.Window, *gpui.App) gpui.EntityRoot {
		return gpui.EntityView(ent)
	})
	win := app.Windows()[0]
	ent.Release(app) // the view still holds a strong reference
	if ent.Read(app) == nil {
		t.Fatal("entity released while the window's view holds it")
	}
	win.Close()
	if len(app.Windows()) != 0 {
		t.Fatalf("Windows() = %d after Close, want 0", len(app.Windows()))
	}
	if ent.Read(app) != nil {
		t.Fatal("closing the window did not release the root view's entity")
	}
	win.Close() // idempotent
}
