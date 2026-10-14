package gpui_test

import (
	"go-draw/internal/gpui"
	"math"
	"testing"
	"unicode/utf8"
)

func inputSelectionKey(win *gpui.Window, app *gpui.App, name string, mods gpui.Modifiers) {
	win.DispatchEvent(gpui.KeyDownEvent{Keystroke: gpui.Keystroke{Key: name, Modifiers: mods}}, app)
}
func inputSelectionWant(t *testing.T, ti *gpui.TextInput, start, end int, reversed bool) {
	t.Helper()
	s, ok := ti.SelectedTextRange()
	if !ok || s.Range != (gpui.IntRange{Start: start, End: end}) || s.Reversed != reversed {
		t.Fatalf("selection=%+v ok=%v want [%d,%d) reversed=%v", s, ok, start, end, reversed)
	}
}

func TestTextInputRuneSafeNavigationRegression(t *testing.T) {
	ti := gpui.NewTextInput("")
	ti.SetText("a😀b")
	app, win := openInputApp(t, ti)
	focusInput(t, app, win, ti)
	ti.SetSelectedTextRange(gpui.IntRange{Start: 4, End: 4})
	inputSelectionKey(win, app, "left", 0)
	inputSelectionWant(t, ti, 3, 3, false)
	inputSelectionKey(win, app, "left", 0)
	inputSelectionWant(t, ti, 1, 1, false)
	inputSelectionKey(win, app, "right", 0)
	inputSelectionWant(t, ti, 3, 3, false)
	inputSelectionKey(win, app, "backspace", 0)
	if ti.Text() != "ab" {
		t.Fatalf("backspace split surrogate: %q", ti.Text())
	}
}

func TestTextInputUTF16RangeValidationRegression(t *testing.T) {
	ti := gpui.NewTextInput("")
	ti.SetText("a😀b")
	for _, r := range []gpui.IntRange{{Start: 2, End: 3}, {Start: 1, End: 2}, {Start: 5, End: 5}, {Start: -1, End: 1}, {Start: 3, End: 1}} {
		if text, ok := ti.TextForRange(r); ok {
			t.Errorf("invalid range %+v accepted as %q", r, text)
		}
	}
	ti.SetSelectedTextRange(gpui.IntRange{Start: 2, End: 99})
	inputSelectionWant(t, ti, 1, 4, false)
	ti.SetSelectedTextRange(gpui.IntRange{Start: 3, End: -9})
	inputSelectionWant(t, ti, 0, 3, true)
}

func inputSelectionField(t *testing.T, hw *gpui.HeadlessWindow) gpui.Bounds {
	t.Helper()
	hw.Image()
	for _, q := range hw.Window().Scene().Quads {
		if q.BorderWidths.Left == 1 && q.CornerRadii.TopLeft == 6 {
			return q.Bounds
		}
	}
	t.Fatal("input field not painted")
	return gpui.Bounds{}
}
func inputSelectionPoint(ti *gpui.TextInput, field gpui.Bounds, offset int) gpui.Point {
	b, _ := ti.BoundsForRange(gpui.IntRange{Start: offset, End: offset}, field)
	return gpui.Point{X: b.Origin.X, Y: b.Origin.Y + b.Size.H/2}
}
func inputSelectionPress(hw *gpui.HeadlessWindow, p gpui.Point, count int, mods gpui.Modifiers) {
	hw.Dispatch(gpui.MouseDownEvent{Position: p, Button: gpui.MouseLeft, ClickCount: count, Modifiers: mods})
}

func TestTextInputShiftNavigationAndSelectionEditing(t *testing.T) {
	ti := gpui.NewTextInput("")
	ti.SetText("abcd")
	app, win := openInputApp(t, ti)
	focusInput(t, app, win, ti)
	ti.SetSelectedTextRange(gpui.IntRange{Start: 2, End: 2})
	for _, tc := range []struct {
		key        string
		start, end int
		reverse    bool
	}{{"right", 2, 3, false}, {"left", 2, 2, false}, {"left", 1, 2, true}, {"home", 0, 2, true}, {"end", 2, 4, false}} {
		inputSelectionKey(win, app, tc.key, gpui.Shift)
		inputSelectionWant(t, ti, tc.start, tc.end, tc.reverse)
	}
	inputSelectionKey(win, app, "left", 0)
	inputSelectionWant(t, ti, 2, 2, false)
	ti.SetSelectedTextRange(gpui.IntRange{Start: 3, End: 1})
	inputSelectionKey(win, app, "right", 0)
	inputSelectionWant(t, ti, 3, 3, false)
	inputSelectionKey(win, app, "home", 0)
	inputSelectionWant(t, ti, 0, 0, false)
	inputSelectionKey(win, app, "end", 0)
	inputSelectionWant(t, ti, 4, 4, false)
	changes := 0
	ti.OnChange(func(string, *gpui.Window, *gpui.App) { changes++ })
	ti.SetSelectedTextRange(gpui.IntRange{Start: 1, End: 3})
	win.Draw(app)
	win.DispatchEvent(gpui.KeyDownEvent{Keystroke: gpui.Keystroke{Key: "x", KeyChar: keyChar("X")}, PreferCharacterInput: true}, app)
	if ti.Text() != "aXd" || changes != 1 {
		t.Fatalf("selection typing=%q changes=%d", ti.Text(), changes)
	}
	ti.SetSelectedTextRange(gpui.IntRange{Start: 2, End: 1})
	inputSelectionKey(win, app, "delete", 0)
	if ti.Text() != "ad" {
		t.Fatalf("reverse delete=%q", ti.Text())
	}
	ti.SetText("a😀b")
	ti.SetSelectedTextRange(gpui.IntRange{Start: 1, End: 1})
	inputSelectionKey(win, app, "delete", 0)
	if ti.Text() != "ab" {
		t.Fatalf("delete rune=%q", ti.Text())
	}
	ti.SetText("a😀b")
	ti.SetSelectedTextRange(gpui.IntRange{Start: 1, End: 3})
	selection, _ := ti.SelectedTextRange()
	copied, ok := ti.TextForRange(selection.Range)
	if !ok || copied != "😀" {
		t.Fatalf("copy surface=%q %v", copied, ok)
	}
	ti.ReplaceTextInRange(&selection.Range, "")
	if ti.Text() != "ab" {
		t.Fatalf("cut surface=%q", ti.Text())
	}
}

func TestTextInputMouseDragAndReleaseOutside(t *testing.T) {
	ti := gpui.NewTextInput("")
	ti.SetText("abcdef")
	hw, _ := openHeadless(t, 400, 300, inputTestRoot(ti))
	field := inputSelectionField(t, hw)
	inputSelectionPress(hw, inputSelectionPoint(ti, field, 3), 1, 0)
	inputSelectionWant(t, ti, 3, 3, false)
	hw.Dispatch(gpui.MouseUpEvent{Position: inputSelectionPoint(ti, field, 3), Button: gpui.MouseRight})
	hw.Dispatch(gpui.MouseMoveEvent{Position: inputSelectionPoint(ti, field, 1)})
	inputSelectionWant(t, ti, 1, 3, true)
	hw.Dispatch(gpui.MouseMoveEvent{Position: gpui.Point{X: -100, Y: 250}})
	inputSelectionWant(t, ti, 0, 3, true)
	hw.Dispatch(gpui.MouseMoveEvent{Position: gpui.Point{X: 900, Y: 250}})
	inputSelectionWant(t, ti, 3, 6, false)
	hw.Dispatch(gpui.MouseUpEvent{Position: gpui.Point{X: 900, Y: 250}, Button: gpui.MouseLeft})
	hw.Dispatch(gpui.MouseMoveEvent{Position: inputSelectionPoint(ti, field, 1)})
	inputSelectionWant(t, ti, 3, 6, false)
	ti.SetSelectedTextRange(gpui.IntRange{Start: 4, End: 2})
	inputSelectionPress(hw, inputSelectionPoint(ti, field, 1), 1, gpui.Shift)
	inputSelectionWant(t, ti, 1, 4, true)
}

func TestTextInputDoubleTripleAndWordDrag(t *testing.T) {
	ti := gpui.NewTextInput("")
	ti.SetText("hé_2 😀 !!")
	hw, _ := openHeadless(t, 400, 300, inputTestRoot(ti))
	field := inputSelectionField(t, hw)
	for _, tc := range []struct{ offset, next, start, end int }{{2, 3, 0, 4}, {4, 5, 4, 5}, {5, 7, 5, 7}, {8, 9, 8, 10}} {
		p, q := inputSelectionPoint(ti, field, tc.offset), inputSelectionPoint(ti, field, tc.next)
		p.X = (p.X + q.X) / 2
		inputSelectionPress(hw, p, 2, 0)
		inputSelectionWant(t, ti, tc.start, tc.end, false)
		hw.Dispatch(gpui.MouseUpEvent{Position: p, Button: gpui.MouseLeft})
	}
	inputSelectionPress(hw, inputSelectionPoint(ti, field, 2), 3, 0)
	inputSelectionWant(t, ti, 0, 10, false)
	hw.Dispatch(gpui.MouseMoveEvent{Position: gpui.Point{X: -100, Y: 200}})
	inputSelectionWant(t, ti, 0, 10, false)
	ti.SetText("one two three")
	hw.Image()
	field = inputSelectionField(t, hw)
	inputSelectionPress(hw, inputSelectionPoint(ti, field, 1), 2, 0)
	inputSelectionWant(t, ti, 0, 3, false)
	hw.Dispatch(gpui.MouseMoveEvent{Position: inputSelectionPoint(ti, field, 9)})
	inputSelectionWant(t, ti, 0, 13, false)
	inputSelectionPress(hw, inputSelectionPoint(ti, field, 5), 2, 0)
	inputSelectionWant(t, ti, 4, 7, false)
	hw.Dispatch(gpui.MouseMoveEvent{Position: inputSelectionPoint(ti, field, 1)})
	inputSelectionWant(t, ti, 0, 7, true)
}

func TestTextInputSelectionCaptureLifecycle(t *testing.T) {
	for _, change := range []string{"remove", "replace", "blur", "resize", "ancestor"} {
		t.Run(change, func(t *testing.T) {
			ti := gpui.NewTextInput("")
			ti.SetText("abcdef")
			current := ti
			show := true
			width := float32(200)
			parentID := "same"
			root := gpui.RenderFunc(func(w *gpui.Window, cx *gpui.App) gpui.IntoElement {
				d := gpui.NewDiv().ID(parentID).SizeWH(width, 100)
				if show {
					d.Child(current.Render(w, cx))
				}
				return d
			})
			hw, _ := openHeadless(t, 300, 200, root)
			field := inputSelectionField(t, hw)
			inputSelectionPress(hw, inputSelectionPoint(ti, field, 2), 1, 0)
			switch change {
			case "remove":
				show = false
			case "replace":
				current = gpui.NewTextInput("")
				current.SetText("uvwxyz")
			case "blur":
				hw.Window().Blur()
			case "resize":
				width = 220
			case "ancestor":
				parentID = "different"
			}
			hw.Window().Refresh()
			hw.Image()
			hw.Dispatch(gpui.MouseMoveEvent{Position: gpui.Point{X: 190, Y: 80}})
			inputSelectionWant(t, ti, 2, 2, false)
		})
	}
}

func TestTextInputDownBubblesButCapturedMoveDoesNot(t *testing.T) {
	ti := gpui.NewTextInput("")
	ti.SetText("abc")
	down, moves, up := 0, 0, 0
	root := gpui.RenderFunc(func(w *gpui.Window, cx *gpui.App) gpui.IntoElement {
		return gpui.NewDiv().SizeFull().OnMouseDown(func(*gpui.MouseDownEvent, *gpui.Window, *gpui.App) { down++ }).OnMouseMove(func(*gpui.MouseMoveEvent, *gpui.Window, *gpui.App) { moves++ }).OnMouseUp(func(*gpui.MouseUpEvent, *gpui.Window, *gpui.App) { up++ }).Child(ti.Render(w, cx))
	})
	hw, _ := openHeadless(t, 400, 300, root)
	field := inputSelectionField(t, hw)
	p := inputSelectionPoint(ti, field, 1)
	inputSelectionPress(hw, p, 1, 0)
	hw.Dispatch(gpui.MouseMoveEvent{Position: gpui.Point{X: 300, Y: 250}})
	hw.Dispatch(gpui.MouseUpEvent{Position: gpui.Point{X: 300, Y: 250}, Button: gpui.MouseLeft})
	if down != 1 || moves != 0 || up != 0 {
		t.Fatalf("down/move/up=%d/%d/%d", down, moves, up)
	}
}

func TestTextInputSelectionPaintAndMappingScale(t *testing.T) {
	for _, scale := range []float32{1, 2} {
		p := gpui.NewHeadlessPlatform()
		p.SetScaleFactor(scale)
		app := gpui.NewAppWithPlatform(p)
		ti := gpui.NewTextInput("")
		ti.SetText("AVATAR office")
		root := gpui.RenderFunc(func(w *gpui.Window, cx *gpui.App) gpui.IntoElement {
			return gpui.NewDiv().SizeWH(220, 100).P(8).TextSize(20).Child(ti.Render(w, cx))
		})
		if err := app.OpenWindow(gpui.WindowOptions{Width: 220, Height: 100}, func(*gpui.Window, *gpui.App) gpui.EntityRoot { return root }); err != nil {
			t.Fatal(err)
		}
		hw := p.Windows()[0]
		field := inputSelectionField(t, hw)
		inputSelectionPress(hw, inputSelectionPoint(ti, field, 0), 1, 0)
		ti.SetSelectedTextRange(gpui.IntRange{Start: 1, End: 4})
		hw.Image()
		start, _ := ti.BoundsForRange(gpui.IntRange{Start: 1, End: 1}, field)
		end, _ := ti.BoundsForRange(gpui.IntRange{Start: 4, End: 4}, field)
		highlight := false
		caret := false
		for _, q := range hw.Window().Scene().Quads {
			if q.Fill == gpui.Blue500.Opacity(.25) {
				highlight = true
				wantRect(t, "highlight", q.Bounds, float32(start.Origin.X), float32(start.Origin.Y), float32(end.Origin.X-start.Origin.X), float32(start.Size.H))
				for _, s := range hw.Window().Scene().Sprites {
					if s.Order <= q.Order {
						t.Fatal("selection painted over glyphs")
					}
				}
			}
			if q.Bounds.Size.W == 2 {
				caret = true
				if q.Bounds.Origin.X != end.Origin.X {
					t.Fatalf("caret=%v endpoint=%v", q.Bounds, end)
				}
			}
		}
		if !highlight || !caret {
			t.Fatalf("scale %v highlight=%v caret=%v", scale, highlight, caret)
		}
		for _, index := range []int{0, 1, 4, 8, 12} {
			point := inputSelectionPoint(ti, field, index)
			got, ok := ti.CharacterIndexForPoint(point, field)
			if !ok || got != index {
				t.Fatalf("scale %v index %d maps to %d", scale, index, got)
			}
		}
		ti.SetText("A very long text field with more words than fit inside its narrow bounds")
		ti.SetSelectedTextRange(gpui.IntRange{Start: 0, End: 999})
		hw.Image()
		field = inputSelectionField(t, hw)
		if field.Size.H > 45 {
			t.Fatalf("single-line input wrapped: %v", field)
		}
		for _, q := range hw.Window().Scene().Quads {
			if q.Fill == gpui.Blue500.Opacity(.25) && !field.ContainsBounds(q.ContentMask.Bounds) {
				t.Fatalf("selection mask escaped field: %v", q.ContentMask)
			}
		}
	}
}

type selectionMetricMeasurer struct{ width gpui.Pixels }

func (m selectionMetricMeasurer) MeasureWrapped(text string, ts gpui.TextStyle, _ float32) (gpui.Pixels, gpui.Pixels) {
	return gpui.Pixels(utf8.RuneCountInString(text)) * m.width * ts.FontSize / 14, ts.ResolvedLineHeight()
}
func (m selectionMetricMeasurer) ShapeWrapped(text string, ts gpui.TextStyle, _ float32) []gpui.ShapedLineOut {
	var gs []gpui.ShapedGlyphOut
	var x gpui.Pixels
	for pos := range text {
		advance := m.width * ts.FontSize / 14
		gs = append(gs, gpui.ShapedGlyphOut{X: x, Advance: advance, Cluster: pos})
		x += advance
	}
	return []gpui.ShapedLineOut{{Width: x, Glyphs: gs}}
}

func TestTextInputCaretCacheUsesCurrentMeasurerAndStyle(t *testing.T) {
	ti := gpui.NewTextInput("")
	ti.SetText("abcd")
	size := float32(14)
	root := gpui.RenderFunc(func(w *gpui.Window, cx *gpui.App) gpui.IntoElement {
		return gpui.NewDiv().TextSize(size).Child(ti.Render(w, cx))
	})
	hw, _ := openHeadless(t, 400, 200, root)
	w := hw.Window()
	check := func(want gpui.Pixels) {
		t.Helper()
		w.Refresh()
		hw.Image()
		field := inputSelectionField(t, hw)
		a, _ := ti.BoundsForRange(gpui.IntRange{Start: 0}, field)
		b, _ := ti.BoundsForRange(gpui.IntRange{Start: 2}, field)
		if b.Origin.X-a.Origin.X != want {
			t.Fatalf("caret advance=%v want=%v", b.Origin.X-a.Origin.X, want)
		}
	}
	w.SetTextMeasurer(selectionMetricMeasurer{width: 10})
	check(20)
	w.SetTextMeasurer(selectionMetricMeasurer{width: 20})
	check(40)
	size = 28
	check(80)
}

func TestTextInputCaretUsesFullShapedLineAtBothScales(t *testing.T) {
	t.Setenv(gpui.NoSystemFontsEnv, "1")
	for _, scale := range []float32{1, 2} {
		ti := gpui.NewTextInput("")
		ti.SetText("AVATAR")
		p := gpui.NewHeadlessPlatform()
		p.SetScaleFactor(scale)
		app := gpui.NewAppWithPlatform(p)
		if err := app.OpenWindow(gpui.WindowOptions{Width: 400, Height: 300}, func(*gpui.Window, *gpui.App) gpui.EntityRoot { return inputTestRoot(ti) }); err != nil {
			t.Fatal(err)
		}
		hw := p.Windows()[0]
		field := inputSelectionField(t, hw)
		lines := hw.Window().TextMeasurer().ShapeWrapped(ti.Text(), gpui.DefaultTextStyle(), float32(math.MaxFloat32))
		if len(lines) == 0 || len(lines[0].Glyphs) == 0 {
			t.Skip("no system font")
		}
		var advance gpui.Pixels
		for _, g := range lines[0].Glyphs {
			if g.Cluster >= 1 {
				break
			}
			advance += g.Advance
		}
		zero, _ := ti.BoundsForRange(gpui.IntRange{}, field)
		one, _ := ti.BoundsForRange(gpui.IntRange{Start: 1}, field)
		if math.Abs(float64(one.Origin.X-zero.Origin.X-advance)) > .01 {
			t.Fatalf("scale %v caret does not use full-line advances: %v vs %v", scale, one.Origin.X-zero.Origin.X, advance)
		}
		inputSelectionPress(hw, inputSelectionPoint(ti, field, 1), 1, 0)
		hw.Image()
		caret, _ := ti.BoundsForRange(gpui.IntRange{Start: 1}, field)
		x, y := int(math.Ceil(float64((caret.Origin.X+.5)*gpui.Pixels(scale)))), int(float32(caret.Origin.Y+caret.Size.H/2)*scale)
		pixel := hw.Image().NRGBAAt(x, y)
		if pixel.R > 64 || pixel.G > 64 || pixel.B > 64 {
			t.Fatalf("scale %v caret pixel=%v", scale, pixel)
		}
	}
}
