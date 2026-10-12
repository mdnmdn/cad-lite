// Command interactive_elements demonstrates interactive patterns: click events
// (single, double and triple click counts), hover callbacks, low-level mouse
// down/up/move events and a drag-and-drop interaction.
//
// Ported from gpui-ce crates/gpui/examples/learn/interactive_elements.rs
//
// Not ported: gpui's on_drag/on_drop (there is no drag API) are emulated with
// mouse down on an item, mouse up on the drop zone, and a "Dragging" label in
// place of the floating drag preview with its shadow; dashed drop-zone border
// (drawn solid); cursor styles; the 2-column CSS grid (built from flex rows);
// scrolling beyond the window.
package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"time"

	"go-gpui/gpui"
	"go-gpui/gpui/ebitenplatform"
)

// colors mirrors gpui::colors::Colors::light().
type colors struct {
	text, selectedText, background, disabled, selected, border, container gpui.Rgba
}

func lightColors() colors {
	return colors{
		text:         gpui.MustParseHex("#252525"),
		selectedText: gpui.MustParseHex("#ffffff"),
		background:   gpui.MustParseHex("#ffffff"),
		disabled:     gpui.MustParseHex("#b0b0b0"),
		selected:     gpui.MustParseHex("#2a63d9"),
		border:       gpui.MustParseHex("#d9d9d9"),
		container:    gpui.MustParseHex("#f4f5f5"),
	}
}

var itemColors = []gpui.Rgba{
	gpui.MustParseHex("#d32f2f"),
	gpui.MustParseHex("#388e3c"),
	gpui.MustParseHex("#f9a825"),
}

func label(s string, size float32, c gpui.Rgba) *gpui.Text {
	return gpui.NewText(s).TextSize(size).TextColor(c)
}

func card(c colors, title, hint string) *gpui.Div {
	d := gpui.NewDiv().Flex1().MinW(0).FlexCol().Gap(12).P(16).Rounded(8).Bg(c.container).Child(
		label(title, 14, c.text).FontWeight(600),
		label(hint, 12, c.disabled),
	)
	equal := gpui.Flex(1)
	d.StyleRef().Width = &equal
	return d
}

// Example is the root view, holding the state of all four demos.
type Example struct {
	clickCount int
	lastClick  string

	hovered    bool
	hoverCount int

	mouseX, mouseY float32
	hasMouse       bool
	pressed        bool
	eventLog       []string

	dragging    int // -1 when idle
	dropHover   bool
	dropped     int // -1 when empty
	hoveredItem int // -1 when none
}

func (e *Example) logEvent(s string) {
	e.eventLog = append(e.eventLog, s)
	if len(e.eventLog) > 5 {
		e.eventLog = e.eventLog[1:]
	}
}

func (e *Example) clickDemo(c colors) *gpui.Div {
	target := gpui.NewDiv().ID("click-target").Px_(16).Py_(8).Rounded(6).Bg(c.selected).
		Child(label("Click Me!", 14, c.selectedText)).
		OnClick(func(ev *gpui.ClickEvent, w *gpui.Window, _ *gpui.App) {
			e.clickCount++
			n := 1
			if ev != nil && ev.Up != nil {
				n = ev.Up.ClickCount
			}
			switch n {
			case 1:
				e.lastClick = "Single Click"
			case 2:
				e.lastClick = "Double Click"
			case 3:
				e.lastClick = "Triple Click"
			default:
				e.lastClick = fmt.Sprintf("%dx Click", n)
			}
			w.Refresh()
		})
	return card(c, "Click Events", "Single click, double click, or triple click the button").Child(
		gpui.NewDiv().FlexRow().Child(target),
		gpui.NewDiv().Pt(8).FlexCol().Gap(4).Child(
			label(fmt.Sprintf("Total clicks: %d", e.clickCount), 12, c.disabled),
			label("Last: "+e.lastClick, 12, c.disabled),
		),
	)
}

func (e *Example) hoverDemo(c colors) *gpui.Div {
	border, fg, text := c.border, c.text, "Hover Over Me"
	if e.hovered {
		border, fg, text = c.selected, c.selectedText, "Mouse Inside!"
	}
	target := gpui.NewDiv().ID("hover-target").Px_(16).Py_(12).Rounded(6).Border(2, border).Bg(c.selected).
		Child(label(text, 14, fg)).
		OnHoverStart(func(w *gpui.Window, _ *gpui.App) {
			e.hovered = true
			e.hoverCount++
			w.Refresh()
		}).
		OnHoverEnd(func(w *gpui.Window, _ *gpui.App) {
			e.hovered = false
			w.Refresh()
		})
	return card(c, "Hover Events", "Move your mouse in and out of the target").Child(
		gpui.NewDiv().FlexRow().Child(target),
		gpui.NewDiv().Pt(8).Child(label(fmt.Sprintf("Times hovered: %d", e.hoverCount), 12, c.disabled)),
	)
}

func (e *Example) mouseDemo(c colors) *gpui.Div {
	pos := "—"
	if e.hasMouse {
		pos = fmt.Sprintf("(%.0f, %.0f)", e.mouseX, e.mouseY)
	}
	border := c.border
	if e.pressed {
		border = c.selected
	}
	target := gpui.NewDiv().ID("mouse-events-target").SizeH(80).Rounded(6).Border(2, border).Bg(c.selected).
		FlexRow().ItemsCenter().
		Child(label("Position: "+pos, 14, c.text)).
		OnMouseDown(func(ev *gpui.MouseDownEvent, w *gpui.Window, _ *gpui.App) {
			if !ev.IsLeft() {
				return
			}
			e.pressed = true
			e.logEvent("Mouse Down")
			w.Refresh()
		}).
		OnMouseUp(func(ev *gpui.MouseUpEvent, w *gpui.Window, _ *gpui.App) {
			if !ev.IsLeft() {
				return
			}
			e.pressed = false
			e.logEvent("Mouse Up")
			w.Refresh()
		}).
		OnMouseMove(func(ev *gpui.MouseMoveEvent, w *gpui.Window, _ *gpui.App) {
			e.mouseX, e.mouseY, e.hasMouse = float32(ev.Position.X), float32(ev.Position.Y), true
			w.Refresh()
		})
	target.JustifyCenter()
	log := gpui.NewDiv().Pt(8).FlexCol().Gap(2)
	for _, l := range e.eventLog {
		log.Child(label("• "+l, 12, c.disabled))
	}
	return card(c, "Mouse Events", "Move and click within the target area").Child(target, log)
}

func (e *Example) dragDemo(c colors) *gpui.Div {
	items := gpui.NewDiv().FlexRow().Gap(8)
	for i, col := range itemColors {
		i, col := i, col
		bg := gpui.Transparent
		if e.hoveredItem == i {
			bg = col.Opacity(0.1)
		}
		items.Child(gpui.NewDiv().ID(fmt.Sprintf("drag-item-%d", i)).Px_(12).Py_(8).Rounded(6).
			Border(2, col).Bg(bg).
			Child(label(fmt.Sprintf("Item %d", i+1), 12, col)).
			OnHoverStart(func(w *gpui.Window, _ *gpui.App) { e.hoveredItem = i; w.Refresh() }).
			OnHoverEnd(func(w *gpui.Window, _ *gpui.App) {
				if e.hoveredItem == i {
					e.hoveredItem = -1
					w.Refresh()
				}
			}).
			OnMouseDown(func(ev *gpui.MouseDownEvent, w *gpui.Window, _ *gpui.App) {
				e.dragging = i
				w.Refresh()
			}))
	}
	border, bg, text := c.border, gpui.Transparent, "Drop Zone"
	if e.dropped >= 0 {
		border = itemColors[e.dropped]
		bg = border.Opacity(0.2)
		text = fmt.Sprintf("Dropped: Item %d", e.dropped+1)
	}
	zone := gpui.NewDiv().ID("drop-target").SizeH(64).Rounded(6).Border(2, border).Bg(bg).
		FlexRow().ItemsCenter().
		Child(label(text, 12, c.disabled)).
		OnMouseUp(func(_ *gpui.MouseUpEvent, w *gpui.Window, _ *gpui.App) {
			if e.dragging >= 0 {
				e.dropped = e.dragging
				e.dragging = -1
				w.Refresh()
			}
		})
	zone.JustifyCenter()
	status := " "
	if e.dragging >= 0 {
		status = fmt.Sprintf("Dragging: Item %d", e.dragging+1)
	}
	return card(c, "Drag and Drop", "Drag items to the drop zone below").Child(
		items,
		gpui.NewDiv().Pt(8).Child(zone),
		label(status, 12, c.disabled),
	)
}

// Render builds the element tree.
func (e *Example) Render(w *gpui.Window, cx *gpui.App) gpui.IntoElement {
	c := lightColors()
	return gpui.NewDiv().ID("main").SizeFull().P(24).Bg(c.background).OverflowScroll().
		OnMouseUp(func(_ *gpui.MouseUpEvent, w *gpui.Window, _ *gpui.App) {
			if e.dragging >= 0 {
				e.dragging = -1
				w.Refresh()
			}
		}).
		Child(
			gpui.NewDiv().FlexCol().Gap(24).MaxW(800).Child(
				gpui.NewDiv().FlexCol().Gap(4).Child(
					label("Interactive Elements", 20, c.text).FontWeight(700),
					label("Click, hover, mouse events, and drag-and-drop in GPUI", 14, c.disabled),
				),
				gpui.NewDiv().FlexCol().Gap(16).Child(
					gpui.NewDiv().FlexRow().Gap(16).Child(e.clickDemo(c), e.hoverDemo(c)),
					gpui.NewDiv().FlexRow().Gap(16).Child(e.mouseDemo(c), e.dragDemo(c)),
				),
			),
		)
}

func main() {
	png := flag.String("png", "", "render one frame to this PNG path and exit")
	width := flag.Int("width", 700, "window width in logical pixels")
	height := flag.Int("height", 650, "window height in logical pixels")
	smoke := flag.Int("smoke", 0, "quit after this many milliseconds (interactive mode)")
	flag.Parse()

	root := &Example{lastClick: "None", dragging: -1, dropped: -1, hoveredItem: -1}

	if *png != "" {
		root.clickCount, root.lastClick = 3, "Double Click"
		root.hovered, root.hoverCount = true, 2
		root.mouseX, root.mouseY, root.hasMouse, root.pressed = 410, 322, true, true
		root.eventLog = []string{"Mouse Down", "Mouse Up", "Mouse Down"}
		root.dropped = 1
		if err := gpui.RenderToPNG(*png, *width, *height, 1, root); err != nil {
			log.Fatal(err)
		}
		return
	}

	err := ebitenplatform.New().Run(func(cx *gpui.App) {
		opts := gpui.WindowOptions{Width: float32(*width), Height: float32(*height), Title: "Interactive Elements", Resizable: true}
		if err := cx.OpenWindow(opts, func(*gpui.Window, *gpui.App) gpui.EntityRoot { return root }); err != nil {
			log.Fatal(err)
		}
		if *smoke > 0 {
			time.AfterFunc(time.Duration(*smoke)*time.Millisecond, cx.Quit)
		}
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
