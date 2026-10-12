// Command styling demonstrates styling patterns: interactive states (hover,
// focus, focus-visible), conditional styling, group hover and a themed colour
// palette.
//
// Ported from gpui-ce crates/gpui/examples/learn/styling.rs
//
// Not ported: shadows (focus_visible shadow_sm), element opacity (the disabled
// item and the group-hover arrow blend colours instead), cursor styles,
// active() (its colour equals the base colour in the source), the application
// menu, flex_wrap (the swatches are split into two rows) and native
// focus_next/focus_prev (Tab cycles the three buttons in this example).
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

type quitAction struct{}

func (quitAction) Name() string              { return "styling_example::Quit" }
func (quitAction) Clone() gpui.Action        { return quitAction{} }
func (quitAction) Equals(o gpui.Action) bool { _, ok := o.(quitAction); return ok }

type tabAction struct{}

func (tabAction) Name() string              { return "styling_example::Tab" }
func (tabAction) Clone() gpui.Action        { return tabAction{} }
func (tabAction) Equals(o gpui.Action) bool { _, ok := o.(tabAction); return ok }

type tabPrevAction struct{}

func (tabPrevAction) Name() string              { return "styling_example::TabPrev" }
func (tabPrevAction) Clone() gpui.Action        { return tabPrevAction{} }
func (tabPrevAction) Equals(o gpui.Action) bool { _, ok := o.(tabPrevAction); return ok }

func registerKeybinds(km *gpui.Keymap) {
	km.AddBindings(
		gpui.NewKeyBinding("cmd-q", quitAction{}, ""),
		gpui.NewKeyBinding("tab", tabAction{}, ""),
		gpui.NewKeyBinding("shift-tab", tabPrevAction{}, ""),
	)
}

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

var (
	successGreen = gpui.MustParseHex("#388e3c")
	warningAmber = gpui.MustParseHex("#f9a825")
	errorRed     = gpui.MustParseHex("#d32f2f")
	focusRing    = gpui.MustParseHex("#60a5fa")
)

func label(s string, size float32, c gpui.Rgba) *gpui.Text {
	return gpui.NewText(s).TextSize(size).TextColor(c)
}

func col(gap float32, children ...gpui.IntoElement) *gpui.Div {
	return gpui.NewDiv().FlexCol().Gap(gap).Child(children...)
}

func row(gap float32, children ...gpui.IntoElement) *gpui.Div {
	return gpui.NewDiv().FlexRow().Gap(gap).Child(children...)
}

// StylingExample is the root view.
type StylingExample struct {
	hovered string
	// kbFocus is true when focus last moved by keyboard (focus-visible).
	kbFocus bool
	// seedFocus, when >= 0, focuses that button on the next render.
	seedFocus int

	root    *gpui.FocusHandle
	buttons []*gpui.FocusHandle
}

func (s *StylingExample) ensure(w *gpui.Window, cx *gpui.App) {
	if s.root != nil {
		return
	}
	s.root = cx.NewFocusHandle()
	for i := 0; i < 3; i++ {
		s.buttons = append(s.buttons, cx.NewFocusHandle())
	}
	if s.seedFocus >= 0 {
		w.RequestFocus(s.buttons[s.seedFocus])
	} else {
		w.RequestFocus(s.root)
	}
}

func (s *StylingExample) hover(d *gpui.Div, id string) *gpui.Div {
	return d.ID(id).
		OnHoverStart(func(w *gpui.Window, _ *gpui.App) { s.hovered = id; w.Refresh() }).
		OnHoverEnd(func(w *gpui.Window, _ *gpui.App) {
			if s.hovered == id {
				s.hovered = ""
				w.Refresh()
			}
		})
}

func (s *StylingExample) cycle(w *gpui.Window, delta int) {
	cur := -1
	for i, b := range s.buttons {
		if b.IsFocused(w) {
			cur = i
		}
	}
	n := len(s.buttons)
	next := 0
	if cur >= 0 {
		next = ((cur+delta)%n + n) % n
	} else if delta < 0 {
		next = n - 1
	}
	s.kbFocus = true
	s.buttons[next].Focus(w)
}

func (s *StylingExample) interactiveButton(c colors, id, text string) *gpui.Div {
	return s.hover(gpui.NewDiv().Px_(16).Py_(8).Rounded(6).Bg(c.selected).
		Child(label(text, 14, c.selectedText)), id)
}

func (s *StylingExample) focusButton(c colors, w *gpui.Window, i int) *gpui.Div {
	id := fmt.Sprintf("focus-%d", i+1)
	h := s.buttons[i]
	bg := c.container
	text := c.text
	if s.hovered == id {
		bg = c.selected
	}
	border := gpui.Transparent
	if h.IsFocused(w) {
		border = c.selected
		if s.kbFocus {
			border = focusRing
		}
	}
	d := gpui.NewDiv().TrackFocus(h).Px_(16).Py_(8).Rounded(6).Bg(bg).Border(2, border).
		OnMouseDown(func(_ *gpui.MouseDownEvent, w *gpui.Window, _ *gpui.App) {
			s.kbFocus = false
			h.Focus(w)
		}).
		Child(label(fmt.Sprintf("Button %d", i+1), 14, text))
	return s.hover(d, id)
}

func interactiveStates(s *StylingExample, c colors) gpui.IntoElement {
	return col(12,
		label("hover() / active() - Mouse interaction states", 12, c.disabled),
		row(8, s.interactiveButton(c, "btn-1", "Hover me"), s.interactiveButton(c, "btn-2", "Click me")),
	)
}

func (s *StylingExample) listItem(c colors, id, text string, selected, disabled bool) *gpui.Div {
	d := gpui.NewDiv().Px_(12).Py_(8).Rounded(6)
	switch {
	case disabled:
		d.Bg(c.container.Opacity(0.5)).Border(1, gpui.Transparent).
			Child(label(text, 14, c.disabled.Opacity(0.5)))
	case selected:
		d.Bg(c.selected.Opacity(0.2)).Border(1, c.selected).Child(label(text, 14, c.text))
	default:
		bg := c.container
		if s.hovered == id {
			bg = c.selected
		}
		d.Bg(bg).Border(1, gpui.Transparent).Child(label(text, 14, c.text))
	}
	return s.hover(d, id)
}

func statusBadge(c colors, text string, bg, fg gpui.Rgba) *gpui.Div {
	return gpui.NewDiv().Px_(8).Py_(2).Rounded(9999).Bg(bg).Child(label(text, 12, fg))
}

func conditional(s *StylingExample, c colors) gpui.IntoElement {
	return col(12,
		label("when() - Apply styles conditionally", 12, c.disabled),
		col(4,
			s.listItem(c, "item-1", "Normal item", false, false),
			s.listItem(c, "item-2", "Selected item", true, false),
			s.listItem(c, "item-3", "Disabled item", false, true),
		),
		gpui.NewDiv().Pt(8).Child(label("Status badges with variant-based styling", 12, c.disabled)),
		row(8,
			statusBadge(c, "Success", successGreen, c.selectedText),
			statusBadge(c, "Warning", warningAmber, gpui.Black),
			statusBadge(c, "Error", errorRed, c.selectedText),
			statusBadge(c, "Neutral", c.container, c.text),
		),
	)
}

func (s *StylingExample) card(c colors, id, title, desc string) *gpui.Div {
	border := c.border
	arrow := gpui.Transparent
	if s.hovered == id {
		border = c.selected
		arrow = c.disabled
	}
	d := gpui.NewDiv().P(16).Rounded(8).Bg(c.container).Border(1, border).FlexCol().Gap(4).Child(
		gpui.NewDiv().FlexRow().JustifyBetween().ItemsCenter().Child(
			label(title, 14, c.text).FontWeight(600),
			label("→", 12, arrow),
		),
		label(desc, 12, c.disabled),
	)
	return s.hover(d, id)
}

func groupHover(s *StylingExample, c colors) gpui.IntoElement {
	return col(12,
		label("group() / group_hover() - Parent hover affects children", 12, c.disabled),
		col(8,
			s.card(c, "card-1", "Documents", "View and manage your documents"),
			s.card(c, "card-2", "Settings", "Configure application settings"),
		),
	)
}

func swatch(c colors, name string, color gpui.Rgba) *gpui.Div {
	return gpui.NewDiv().FlexCol().ItemsCenter().Gap(4).Child(
		gpui.NewDiv().SizeWH(32, 32).Rounded(6).Bg(color).Border(1, gpui.White.Opacity(0.2)),
		label(name, 12, c.disabled),
	)
}

func section(c colors, title string, content gpui.IntoElement) *gpui.Div {
	return gpui.NewDiv().FlexCol().Gap(12).P(16).Bg(c.container.Opacity(0.3)).Rounded(8).
		Border(1, c.border.Opacity(0.5)).Child(
		label(title, 14, c.text).FontWeight(600),
		content,
	)
}

// Render builds the element tree.
func (s *StylingExample) Render(w *gpui.Window, cx *gpui.App) gpui.IntoElement {
	s.ensure(w, cx)
	c := lightColors()
	focusSection := col(12,
		label("focus() / focus_visible() - Keyboard navigation", 12, c.disabled),
		row(8, s.focusButton(c, w, 0), s.focusButton(c, w, 1), s.focusButton(c, w, 2)),
	)
	palette := col(8,
		label("Using Colors::for_appearance() for consistent theming", 12, c.disabled),
		row(8,
			swatch(c, "background", c.background),
			swatch(c, "container", c.container),
			swatch(c, "selected", c.selected),
			swatch(c, "success", successGreen),
		),
		row(8,
			swatch(c, "warning", warningAmber),
			swatch(c, "error", errorRed),
			swatch(c, "border", c.border),
		),
	)
	return gpui.NewDiv().ID("app").TrackFocus(s.root).
		OnAction(quitAction{}, func(_ gpui.Action, _ *gpui.Window, cx *gpui.App) { cx.Quit() }).
		OnAction(tabAction{}, func(_ gpui.Action, w *gpui.Window, _ *gpui.App) { s.cycle(w, 1) }).
		OnAction(tabPrevAction{}, func(_ gpui.Action, w *gpui.Window, _ *gpui.App) { s.cycle(w, -1) }).
		SizeFull().P(24).Bg(c.background).OverflowScroll().Child(
		gpui.NewDiv().FlexCol().Gap(24).MaxW(500).Child(
			col(4,
				label("Styling Patterns", 20, c.text).FontWeight(700),
				label("Interactive states, conditional styling, and theming", 14, c.disabled),
			),
			section(c, "Interactive States", interactiveStates(s, c)),
			section(c, "Focus States (Tab to navigate)", focusSection),
			section(c, "Conditional Styling", conditional(s, c)),
			section(c, "Group Hover", groupHover(s, c)),
			section(c, "Default Colors", palette),
		),
	)
}

func main() {
	png := flag.String("png", "", "render one frame to this PNG path and exit")
	width := flag.Int("width", 550, "window width in logical pixels")
	height := flag.Int("height", 800, "window height in logical pixels")
	smoke := flag.Int("smoke", 0, "quit after this many milliseconds (interactive mode)")
	flag.Parse()

	if *png != "" {
		km := gpui.NewKeymap()
		registerKeybinds(km)
		root := &StylingExample{seedFocus: 1, kbFocus: true, hovered: "card-1"}
		if err := gpui.RenderToPNG(*png, *width, *height, 1, root, gpui.WithKeymap(km)); err != nil {
			log.Fatal(err)
		}
		return
	}

	err := ebitenplatform.New().Run(func(cx *gpui.App) {
		registerKeybinds(cx.Keymap())
		opts := gpui.WindowOptions{Width: float32(*width), Height: float32(*height), Title: "Styling", Resizable: true}
		root := &StylingExample{seedFocus: -1}
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
