# Cookbook

Short, copy-pasteable recipes for common UI pieces. Each snippet is a complete
set of declarations that compiles against the current API (checked in a
scratch module with `replace go-gpui => ../..`). Unless a snippet starts with
`package`, it assumes

```text
import (
	"fmt"
	"strings"

	"cad-lite/internal/gpui"
)
```

Every recipe defines a view type with a `Render(w, cx)` method, so mount it as
a window root:

```text
cx.OpenWindow(gpui.DefaultWindowOptions(),
	func(*gpui.Window, *gpui.App) gpui.EntityRoot { return &Clicks{} })
```

or render it to a PNG with `gpui.RenderToPNG("out.png", 400, 300, 1, &Clicks{})`
([getting-started](../getting-started.md#rendering-to-png)). Background reading:
[elements](../concepts/elements.md), [styling](../concepts/styling.md),
[events and input](../concepts/events-and-input.md). Full method lists:
[reference](../reference/README.md).

Recipes:
[button with hover](#button-with-hover) -
[toggle and checkbox](#toggle-and-checkbox) -
[list with selection](#list-with-selection) -
[scrollable panel](#scrollable-panel) -
[virtualized list](#virtualized-list) -
[modal overlay with paint order](#modal-overlay-with-paint-order) -
[form with TextInput and submit](#form-with-textinput-and-submit) -
[keyboard shortcuts with key contexts](#keyboard-shortcuts-with-key-contexts) -
[tab navigation](#tab-navigation) -
[card with shadow and gradient](#card-with-shadow-and-gradient) -
[custom element with children](#custom-element-with-children).

## Button with hover

A reusable button is just a function returning `*gpui.Div`. Give it a stable
`ID` (hover and click identity are keyed by it). `Hover` takes a closure that
returns a style refinement; it is applied at paint time, so only visual
properties (colours, borders, shadows, opacity) change, never layout. Hover
redraws on its own; you only call `Refresh` when your state changes.

```go
func button(id, label string, onClick func(w *gpui.Window)) *gpui.Div {
	return gpui.NewDiv().ID(id).Px_(14).Py_(8).Rounded(6).Bg(gpui.Blue500).
		Hover(func(s *gpui.StyledMixin) *gpui.StyledMixin { return s.Bg(gpui.Blue600) }).
		OnClick(func(_ *gpui.ClickEvent, w *gpui.Window, _ *gpui.App) { onClick(w) }).
		Child(gpui.NewText(label).TextColor(gpui.White))
}

type Clicks struct{ n int }

func (c *Clicks) Render(w *gpui.Window, cx *gpui.App) gpui.IntoElement {
	return gpui.NewDiv().FlexRow().ItemsCenter().Gap(12).P(16).Child(
		button("inc", "Click me", func(w *gpui.Window) { c.n++; w.Refresh() }),
		gpui.NewText(fmt.Sprintf("clicked %d times", c.n)),
	)
}
```

`OnClick` fires on a completed press and release on the same element. For a
press-only reaction use `OnMouseDown`. If you want a `Text` to look like a
button, wrap it in a `Div` as above: a text leaf paints no background.

## Toggle and checkbox

State lives in your struct; the element is rebuilt from it each frame.

```go
type Prefs struct{ notify, dark bool }

func checkbox(id, label string, on bool, flip func(*gpui.Window)) *gpui.Div {
	box := gpui.NewDiv().SizeWH(18, 18).Rounded(4).Border(2, gpui.Blue500).
		FlexCol().ItemsCenter().JustifyCenter()
	if on {
		box.Bg(gpui.Blue500).Child(gpui.NewDiv().SizeWH(8, 8).Rounded(2).Bg(gpui.White))
	}
	return gpui.NewDiv().ID(id).FlexRow().ItemsCenter().Gap(8).
		OnClick(func(_ *gpui.ClickEvent, w *gpui.Window, _ *gpui.App) { flip(w) }).
		Child(box, gpui.NewText(label))
}

func toggle(id string, on bool, flip func(*gpui.Window)) *gpui.Div {
	track := gpui.NewDiv().ID(id).SizeWH(40, 22).Rounded(11).P(2).FlexRow().ItemsCenter().Bg(gpui.Gray400)
	if on {
		track.Bg(gpui.Green600).JustifyEnd()
	}
	return track.
		OnClick(func(_ *gpui.ClickEvent, w *gpui.Window, _ *gpui.App) { flip(w) }).
		Child(gpui.NewDiv().SizeWH(18, 18).Rounded(9).Bg(gpui.White))
}

func (p *Prefs) Render(w *gpui.Window, cx *gpui.App) gpui.IntoElement {
	return gpui.NewDiv().FlexCol().Gap(12).P(16).Child(
		checkbox("notify", "Notifications", p.notify, func(w *gpui.Window) { p.notify = !p.notify; w.Refresh() }),
		toggle("dark", p.dark, func(w *gpui.Window) { p.dark = !p.dark; w.Refresh() }),
	)
}
```

The checkmark is a small filled square rather than a glyph, so it does not
depend on the system font. Borders are painted inside the box and do not
affect layout, which is why the box is sized explicitly.

## List with selection

Give every row a stable id (derived from your data, not just the index, if the
list can reorder). The selected row gets a fill and white text; `TextColor` set
on the row cascades to its text children.

```go
type Picker struct {
	items    []string
	selected int
}

func (p *Picker) Render(w *gpui.Window, cx *gpui.App) gpui.IntoElement {
	list := gpui.NewDiv().FlexCol().Gap(2).P(8)
	for i, name := range p.items {
		row := gpui.NewDiv().ID(fmt.Sprintf("row-%d", i)).Px_(10).Py_(6).Rounded(4).
			OnClick(func(_ *gpui.ClickEvent, w *gpui.Window, _ *gpui.App) {
				p.selected = i
				w.Refresh()
			})
		if i == p.selected {
			row.Bg(gpui.Blue500).TextColor(gpui.White)
		} else {
			row.Hover(func(s *gpui.StyledMixin) *gpui.StyledMixin { return s.Bg(gpui.Gray200) })
		}
		list.Child(row.Child(gpui.NewText(name)))
	}
	return list
}
```

Go 1.22+ gives each loop iteration its own `i`, so capturing it in the click
closure is safe (the module requires Go 1.26).

## Scrollable panel

An `OverflowYScroll` div scrolls with the wheel. Three things matter: a fixed
(or flexed) height so there is something to overflow, a stable `ID` so the
offset survives re-renders, and optionally a `ScrollHandle` to read or drive the
offset. Offsets are **positive** distances scrolled.

```go
type Log struct {
	scroll *gpui.ScrollHandle
	lines  int
}

func NewLog() *Log { return &Log{scroll: gpui.NewScrollHandle(), lines: 40} }

func (l *Log) Render(w *gpui.Window, cx *gpui.App) gpui.IntoElement {
	panel := gpui.NewDiv().ID("log").SizeWH(320, 200).FlexCol().Gap(4).P(8).
		Border(1, gpui.Gray300).Rounded(6).
		OverflowYScroll().ShowScrollbar().TrackScroll(l.scroll)
	for i := 0; i < l.lines; i++ {
		panel.Child(gpui.NewText(fmt.Sprintf("line %d", i)))
	}
	toTop := gpui.NewDiv().ID("top").Px_(10).Py_(6).Bg(gpui.Gray200).Rounded(4).
		OnClick(func(_ *gpui.ClickEvent, w *gpui.Window, _ *gpui.App) {
			l.scroll.ScrollToTop()
			w.Refresh()
		}).Child(gpui.NewText("Top"))
	return gpui.NewDiv().FlexCol().Gap(8).P(16).Child(panel, toTop)
}
```

`ScrollToTop`, `ScrollToBottom`, `ScrollToItem(ix)` and `SetOffset` take effect
at the next frame, so refresh the window afterwards. `ScrollToItem` reveals
child `ix` of the div with the minimum movement. `Offset()` and `MaxOffset()`
report the state as of the last frame. A scroll container that is not rendered
for a frame loses its offset (element state is pruned).

## Virtualized list

`UniformList` renders only the visible rows of a list of equal-height rows, so
10,000 items cost the same as 20. The render callback receives the half-open
range `[start, end)` and must return exactly those rows. The row height is
measured from row 0, so give rows a fixed height.

```go
type BigList struct {
	scroll   *gpui.ScrollHandle
	selected int
}

func NewBigList() *BigList { return &BigList{scroll: gpui.NewScrollHandle(), selected: -1} }

func (b *BigList) Render(w *gpui.Window, cx *gpui.App) gpui.IntoElement {
	list := gpui.NewUniformList("big", 10000,
		func(start, end int, w *gpui.Window, cx *gpui.App) []gpui.IntoElement {
			rows := make([]gpui.IntoElement, 0, end-start)
			for i := start; i < end; i++ {
				row := gpui.NewDiv().ID(fmt.Sprintf("r%d", i)).SizeH(28).Px_(8).
					FlexRow().ItemsCenter().
					OnClick(func(_ *gpui.ClickEvent, w *gpui.Window, _ *gpui.App) {
						b.selected = i
						w.Refresh()
					})
				if i == b.selected {
					row.Bg(gpui.Blue500).TextColor(gpui.White)
				}
				rows = append(rows, row.Child(gpui.NewText(fmt.Sprintf("Item %d", i))))
			}
			return rows
		}).Flex1().TrackScroll(b.scroll).ShowScrollbar()

	jump := gpui.NewDiv().ID("jump").Px_(10).Py_(6).Bg(gpui.Gray200).Rounded(4).
		OnClick(func(_ *gpui.ClickEvent, w *gpui.Window, _ *gpui.App) {
			b.scroll.ScrollToItem(5000)
			w.Refresh()
		}).Child(gpui.NewText("Jump to 5000"))

	return gpui.NewDiv().FlexCol().SizeFull().Gap(8).P(8).Child(jump, list)
}
```

`Flex1()` makes the list take the remaining height; without a bounded height
the list sizes itself to all `count` rows and nothing is virtualized. The
row `ID`s only need to be unique among the rows being rendered.
`examples/uniform_list` is the full version.

When rows differ in height (wrapped text, cards, expandable sections), use
`List` with a `ListState` the view keeps: it builds only the items near the
viewport and remembers each one's measured height. Give it an `ItemKey` that
changes with an item's content, so an item that changed while off-screen is
measured again.

```go
type Chat struct {
	msgs   []Msg // Msg{Text string; Rev uint64}
	state  *gpui.ListState
	scroll *gpui.ScrollHandle
}

func (c *Chat) Render(w *gpui.Window, cx *gpui.App) gpui.IntoElement {
	return gpui.NewList("chat", c.state, len(c.msgs), func(ix int, _ *gpui.Window, _ *gpui.App) gpui.IntoElement {
		return gpui.NewText(c.msgs[ix].Text)
	}).ItemKey(func(ix int) uint64 { return c.msgs[ix].Rev }).
		Gap(8).P(12).Flex1().MinH(0).TrackScroll(c.scroll)
}
```

Call `c.scroll.ScrollToBottom()` when a message arrives to follow the tail; the
list reaches the true end even through items it never measured.

## Modal overlay with paint order

Paint order is tree order: a later sibling paints on top and wins hit testing.
An `Absolute().Inset(0)` overlay fills the parent without consuming flex space.
Place the scrim last so it covers the page.

```go
const (
	winW, winH = 400, 300
)

type Page struct{ open bool }

func (p *Page) Render(w *gpui.Window, cx *gpui.App) gpui.IntoElement {
	open := gpui.NewDiv().ID("open").Px_(14).Py_(8).Bg(gpui.Blue500).Rounded(6).
		OnClick(func(_ *gpui.ClickEvent, w *gpui.Window, _ *gpui.App) { p.open = true; w.Refresh() }).
		Child(gpui.NewText("Open dialog").TextColor(gpui.White))

	page := gpui.NewDiv().ID("page").SizeWH(winW, winH).FlexNone().FlexCol().Gap(8).P(16).Bg(gpui.Gray50).
		Child(gpui.NewText("Page content").TextSize(20), open)

	root := gpui.NewDiv().SizeWH(winW, winH).FlexCol().Child(page)
	if !p.open {
		return root
	}

	dialog := gpui.NewDiv().ID("dialog").P(20).Gap(12).FlexCol().Bg(gpui.White).Rounded(10).ShadowLg().
		OnClick(func(_ *gpui.ClickEvent, w *gpui.Window, _ *gpui.App) { w.StopPropagation() }).
		Child(
			gpui.NewText("Delete everything?").TextSize(18).FontWeight(700),
			gpui.NewDiv().ID("close").Px_(14).Py_(8).Bg(gpui.Red500).Rounded(6).
				OnClick(func(_ *gpui.ClickEvent, w *gpui.Window, _ *gpui.App) { p.open = false; w.Refresh() }).
				Child(gpui.NewText("Close").TextColor(gpui.White)),
		)

	scrim := gpui.NewDiv().ID("scrim").Absolute().Inset(0).
		FlexCol().ItemsCenter().JustifyCenter().Bg(gpui.Black.Opacity(0.5)).
		OnClick(func(_ *gpui.ClickEvent, w *gpui.Window, _ *gpui.App) { p.open = false; w.Refresh() }).
		Child(dialog)

	return root.Child(scrim)
}
```

Because the scrim is the topmost hitbox, clicks and hover on the page below
never reach it while the dialog is open. The dialog stops propagation so a
click inside it does not bubble to the scrim and close it. Use a
`FocusHandle` plus `Escape` binding if the dialog should also close from the
keyboard ([shortcuts](#keyboard-shortcuts-with-key-contexts)). For a custom
element that needs to paint above everything it can call `Window.PushLayer()`
before painting ([custom element](#custom-element-with-children)).

## Form with TextInput and submit

Create each `TextInput` **once** (not in `Render`), render it each frame and
read it with `Text()`. `OnSubmit` fires on Enter while the field is focused;
a submit button handles the mouse. Both go through one method.

```go
type SignUp struct {
	name, email *gpui.TextInput
	status      string
}

func NewSignUp() *SignUp {
	s := &SignUp{name: gpui.NewTextInput("Name"), email: gpui.NewTextInput("Email")}
	onEnter := func(_ string, w *gpui.Window, _ *gpui.App) { s.submit(w) }
	s.name.OnSubmit(onEnter)
	s.email.OnSubmit(onEnter)
	return s
}

func (s *SignUp) submit(w *gpui.Window) {
	switch {
	case strings.TrimSpace(s.name.Text()) == "":
		s.status = "Name is required"
	case !strings.Contains(s.email.Text(), "@"):
		s.status = "Email looks wrong"
	default:
		s.status = fmt.Sprintf("Welcome, %s!", strings.TrimSpace(s.name.Text()))
		s.name.SetText("")
		s.email.SetText("")
	}
	w.Refresh()
}

func (s *SignUp) Render(w *gpui.Window, cx *gpui.App) gpui.IntoElement {
	submit := gpui.NewDiv().ID("submit").Px_(14).Py_(8).Bg(gpui.Blue500).Rounded(6).
		Hover(func(m *gpui.StyledMixin) *gpui.StyledMixin { return m.Bg(gpui.Blue600) }).
		OnClick(func(_ *gpui.ClickEvent, w *gpui.Window, _ *gpui.App) { s.submit(w) }).
		Child(gpui.NewText("Sign up").TextColor(gpui.White))
	return gpui.NewDiv().FlexCol().Gap(8).P(16).Child(
		gpui.NewText("Create account").TextSize(20).FontWeight(700),
		s.name.Render(w, cx),
		s.email.Render(w, cx),
		submit,
		gpui.NewText(s.status).TextColor(gpui.Gray600),
	)
}
```

Clicking a field focuses it. `OnChange` fires on every edit if you want live
validation. The field edits only what the platform delivers: typed text,
backspace/delete, left/right/Home/End with Shift, paste and mouse selection;
word-arrow movement is not implemented ([events and input](../concepts/events-and-input.md#textinput-and-platforminputhandler)).

## Keyboard shortcuts with key contexts

A binding connects a keystroke to an **action** in a **context**. The view
declares the context with `KeyContext`, becomes a focus target with `TabIndex`
(or `TrackFocus`), and handles the action with `OnAction`. Actions are values
implementing `gpui.Action`, or `gpui.NewAction(name)`.

```go
type saveAction struct{}

func (saveAction) Name() string              { return "editor::Save" }
func (saveAction) Clone() gpui.Action        { return saveAction{} }
func (saveAction) Equals(o gpui.Action) bool { _, ok := o.(saveAction); return ok }

var (
	clearAll = gpui.NewAction("editor::Clear")
	showHelp = gpui.NewAction("editor::Help")
)

func RegisterKeys(km *gpui.Keymap) {
	km.AddBindings(
		gpui.NewKeyBinding("cmd-s", saveAction{}, "editor"),
		gpui.NewKeyBinding("ctrl-s", saveAction{}, "editor"),
		gpui.NewKeyBinding("ctrl-l", clearAll, "editor"),
		gpui.NewKeyBinding("?", showHelp, "editor && !textbox"),
	)
}

type Editor struct {
	input *gpui.TextInput
	log   []string
}

func NewEditor() *Editor { return &Editor{input: gpui.NewTextInput("type here")} }

func (e *Editor) note(w *gpui.Window, s string) { e.log = append(e.log, s); w.Refresh() }

func (e *Editor) Render(w *gpui.Window, cx *gpui.App) gpui.IntoElement {
	root := gpui.NewDiv().ID("editor").KeyContext("editor").TabIndex(0).FlexCol().Gap(8).P(16).
		OnAction(saveAction{}, func(_ gpui.Action, w *gpui.Window, _ *gpui.App) { e.note(w, "saved") }).
		OnAction(clearAll, func(_ gpui.Action, w *gpui.Window, _ *gpui.App) { e.log = nil; w.Refresh() }).
		OnAction(showHelp, func(_ gpui.Action, w *gpui.Window, _ *gpui.App) { e.note(w, "help") }).
		Child(e.input.Render(w, cx))
	for _, line := range e.log {
		root.Child(gpui.NewText(line))
	}
	return root
}
```

Call `RegisterKeys(cx.Keymap())` in the launch callback (or pass the keymap with
`gpui.WithKeymap` when rendering to PNG). How it resolves:

- The context predicate is matched against the `KeyContext` strings on the path
  from the root to the focused element. `TextInput` adds `"textbox"`, so
  `"editor && !textbox"` is active everywhere in the editor except while typing.
  Without that guard a bare `?` binding would swallow the character before the
  field sees it ([troubleshooting](troubleshooting.md#bare-key-bindings-swallow-typing)).
- `"cmd-s"` is the Cmd key on macOS and the Super/Win key elsewhere; register
  `"ctrl-s"` as well for a cross-platform shortcut.
- Later bindings override earlier ones for the same keystroke and context.
- Single keystrokes only; chords are not supported.
- The action is delivered to `OnAction` handlers on the focus path, target
  first. With nothing focused only the root element sees it, so keep the
  handlers on the root view, as above.

## Tab navigation

Tab is not bound by default. Bind `tab` and `shift-tab` to two actions whose
handlers call `Window.FocusNext()` and `Window.FocusPrev()`, and make the
widgets tab stops with `TabIndex`. Order is by index, ties in tree order,
wrapping around. Use `Focus` / `FocusVisible` to show where focus is.

```go
type nextField struct{}

func (nextField) Name() string              { return "form::Next" }
func (nextField) Clone() gpui.Action        { return nextField{} }
func (nextField) Equals(o gpui.Action) bool { _, ok := o.(nextField); return ok }

type prevField struct{}

func (prevField) Name() string              { return "form::Prev" }
func (prevField) Clone() gpui.Action        { return prevField{} }
func (prevField) Equals(o gpui.Action) bool { _, ok := o.(prevField); return ok }

func RegisterTabKeys(km *gpui.Keymap) {
	km.AddBindings(
		gpui.NewKeyBinding("tab", nextField{}, ""),
		gpui.NewKeyBinding("shift-tab", prevField{}, ""),
	)
}

type Toolbar struct{ last string }

func (t *Toolbar) Render(w *gpui.Window, cx *gpui.App) gpui.IntoElement {
	root := gpui.NewDiv().ID("toolbar").FlexCol().Gap(8).P(16).
		OnAction(nextField{}, func(_ gpui.Action, w *gpui.Window, _ *gpui.App) { w.FocusNext() }).
		OnAction(prevField{}, func(_ gpui.Action, w *gpui.Window, _ *gpui.App) { w.FocusPrev() })
	row := gpui.NewDiv().FlexRow().Gap(8)
	for i, name := range []string{"Cut", "Copy", "Paste"} {
		row.Child(gpui.NewDiv().ID("tb-"+name).TabIndex(i).Px_(12).Py_(6).Rounded(6).
			Border(2, gpui.Gray300).
			Focus(func(s *gpui.StyledMixin) *gpui.StyledMixin { return s.Border(2, gpui.Blue500) }).
			FocusVisible(func(s *gpui.StyledMixin) *gpui.StyledMixin { return s.Bg(gpui.Gray100) }).
			OnClick(func(_ *gpui.ClickEvent, w *gpui.Window, _ *gpui.App) { t.last = name; w.Refresh() }).
			Child(gpui.NewText(name)))
	}
	return root.Child(row, gpui.NewText("last: "+t.last))
}
```

`Focus` styles apply whenever the div is focused; `FocusVisible` styles apply
only after keyboard input (the last input was a key press), matching CSS
`:focus-visible`. The root `Div` holds the `OnAction` handlers so they run even
when nothing is focused yet. A `Div` with `TabIndex` and no `TrackFocus` gets an
implicit focus handle retained per element id, so give each tab stop a stable
`ID`. To control a handle yourself, allocate it once with `cx.NewFocusHandle()`
and pass it to `TrackFocus` ([`examples/tab_stop`](../examples.md)).

## Card with shadow and gradient

Gradients are two-stop linear fills; angles follow CSS (0 is towards the top,
90 towards the right). Shadows are `BoxShadow` values or the presets. An
`OverflowHidden` + `Rounded` container clips its children to the rounded shape,
which is what rounds the gradient header's top corners.

```go
func card(title, body string) *gpui.Div {
	brand := gpui.LinearGradient(90,
		gpui.LinearColorStop(gpui.Blue500, 0),
		gpui.LinearColorStop(gpui.MustParseHex("#7e57c2"), 1),
	)
	header := gpui.NewDiv().Px_(16).Py_(12).Bg(brand).
		Child(gpui.NewText(title).TextSize(18).FontWeight(700).TextColor(gpui.White))
	content := gpui.NewDiv().P(16).Child(gpui.NewText(body).TextColor(gpui.Gray700))
	return gpui.NewDiv().SizeW(280).FlexCol().OverflowHidden().Rounded(12).
		Bg(gpui.White).Border(1, gpui.Gray200).ShadowLg().
		Child(header, content)
}

func glowingCard() *gpui.Div {
	glow := gpui.NewBoxShadow(0, 0, gpui.Blue500.Opacity(0.35)).WithBlur(16).WithSpread(2)
	return card("Glow", "Custom shadow").Shadow(glow)
}

type Cards struct{}

func (Cards) Render(w *gpui.Window, cx *gpui.App) gpui.IntoElement {
	return gpui.NewDiv().FlexRow().Gap(24).P(24).Bg(gpui.Gray100).Child(
		card("Plain", "ShadowLg preset on a rounded, clipped card."),
		glowingCard(),
	)
}
```

Give the page some padding around cards: shadows paint outside the box and are
not clipped by the container's own overflow. `Opacity` on a card multiplies into
the whole subtree. Gradients can interpolate in Oklab with
`.ColorSpace(gpui.ColorSpaceOklab)`.

## Custom element with children

Write a custom element when you must paint your own primitives or lay out
children your own way. The container below lays children out as a padded row
with a gap, paints a tinted frame behind them and exposes a fluent `Child`
builder like `Div`. The pattern: a builder type with `IntoElement()`, and a
small adapter implementing `Element` and `ParentElementChildren`.

```go
type Frame struct {
	tint     gpui.Rgba
	children []gpui.AnyElement
}

func NewFrame(tint gpui.Rgba) *Frame { return &Frame{tint: tint} }

func (f *Frame) Child(cs ...gpui.IntoElement) *Frame {
	for _, c := range cs {
		f.children = append(f.children, gpui.NewAnyElement(c))
	}
	return f
}

func (f *Frame) IntoElement() gpui.Element { return frameElement{f} }

type frameElement struct{ f *Frame }

func (e frameElement) ID() (gpui.ElementID, bool)         { return gpui.ElementID{}, false }
func (e frameElement) ElementChildren() []gpui.AnyElement { return e.f.children }

func (e frameElement) RequestLayout(w *gpui.Window, cx *gpui.App) (gpui.LayoutID, any) {
	st := gpui.DefaultStyle()
	st.Direction = gpui.DirectionRow
	st.Gap = 8
	st.Padding = gpui.Edges{}.All(8)
	return w.RequestLayoutWithChildren(st, e.f.children, cx)
}

func (e frameElement) Prepaint(w *gpui.Window, cx *gpui.App, id gpui.ElementID, b gpui.Bounds, ls any) any {
	return nil
}

func (e frameElement) Paint(w *gpui.Window, cx *gpui.App, id gpui.ElementID, b gpui.Bounds, ls, ps any) {
	w.PaintQuad(gpui.Quad{
		Bounds:       b,
		Background:   gpui.BackgroundSolid,
		Fill:         e.f.tint,
		BorderColor:  gpui.Gray400,
		BorderWidths: gpui.Edges{}.All(1),
		CornerRadii:  gpui.Corners{}.Uniform(6),
	})
}

type FrameDemo struct{}

func (FrameDemo) Render(w *gpui.Window, cx *gpui.App) gpui.IntoElement {
	return gpui.NewDiv().P(16).Child(
		NewFrame(gpui.Gray100).Child(
			gpui.NewText("left"),
			gpui.NewDiv().SizeWH(24, 24).Bg(gpui.Blue500).Rounded(4),
			gpui.NewText("right"),
		),
	)
}
```

Notes:

- `RequestLayoutWithChildren` runs `RequestLayout` on every child, builds a node
  with your style and returns the `*ChildLayout` that the window needs to pair
  children with nodes. Return it (or a struct embedding it) as your layout state.
- `Paint` draws only your own primitives; the window walks the children
  afterwards, so they paint on top of your frame. Leave `Quad.ContentMask`
  zero: `PaintQuad` fills it with the current clip.
- For a leaf (no children), register a node yourself with
  `w.RegisterLayoutNode(&gpui.LayoutNode{Style: st})`.
- State that must survive a frame goes in `Window.SetElementState` keyed by
  `GlobalElementID{Path: id.Path}`, and must be touched every frame the element
  is drawn.
- A custom container cannot clip its children or take part in wheel scrolling:
  wrap the children in a `Div` with `OverflowHidden()` for that. See the limits
  in [elements](../concepts/elements.md#writing-a-custom-element).
- To paint above everything drawn so far, call `w.PushLayer()` first in
  `Paint`.
