// Command todo is a small todo-list app built on gpui. With -png it renders a
// single frame headlessly; otherwise it opens an interactive window.
package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"cad-lite/internal/gpui"
	"cad-lite/internal/gpui/ebitenplatform"
)

// Todo is one list entry.
type Todo struct {
	Title string
	Done  bool
}

// TodoApp is the root view: a text field, an Add button and the list.
type TodoApp struct {
	todos  []Todo
	input  *gpui.TextInput
	nextID int
}

type addTodoAction struct{}

func (addTodoAction) Name() string              { return "add-todo" }
func (addTodoAction) Clone() gpui.Action        { return addTodoAction{} }
func (addTodoAction) Equals(o gpui.Action) bool { _, ok := o.(addTodoAction); return ok }

type blurAction struct{}

func (blurAction) Name() string              { return "blur" }
func (blurAction) Clone() gpui.Action        { return blurAction{} }
func (blurAction) Equals(o gpui.Action) bool { _, ok := o.(blurAction); return ok }

// NewTodoApp returns an empty TodoApp whose input submits on Enter.
func NewTodoApp() *TodoApp {
	t := &TodoApp{input: gpui.NewTextInput("What needs doing?")}
	t.input.OnSubmit(func(_ string, w *gpui.Window, _ *gpui.App) { t.add(w) })
	return t
}

func (t *TodoApp) add(w *gpui.Window) {
	title := strings.TrimSpace(t.input.Text())
	if title == "" {
		return
	}
	t.todos = append(t.todos, Todo{Title: title})
	t.nextID++
	t.input.SetText("")
	w.Refresh()
}

func (t *TodoApp) toggle(i int, w *gpui.Window) {
	if i < 0 || i >= len(t.todos) {
		return
	}
	t.todos[i].Done = !t.todos[i].Done
	w.Refresh()
}

// Render builds the element tree for the current state.
func (t *TodoApp) Render(w *gpui.Window, cx *gpui.App) gpui.IntoElement {
	list := gpui.NewDiv().ID("list").Flex1().OverflowScroll().FlexCol().Gap(4)
	for i, todo := range t.todos {
		i, todo := i, todo
		box := gpui.NewDiv().SizeWH(16, 16).Rounded(4).Border(2, gpui.Blue500)
		if todo.Done {
			box.Bg(gpui.Blue500)
		}
		title := gpui.NewText(todo.Title).TextColor(gpui.Gray900)
		if todo.Done {
			title.TextStrikethrough().TextColor(gpui.Gray500)
		}
		list.Child(gpui.NewDiv().
			ID(fmt.Sprintf("todo-%d", i)).
			FlexRow().ItemsCenter().Gap(8).P(8).
			Bg(gpui.White).Rounded(6).
			OnClick(func(_ *gpui.ClickEvent, w *gpui.Window, _ *gpui.App) { t.toggle(i, w) }).
			Child(box, title))
	}

	add := gpui.NewDiv().ID("add").
		P(8).Bg(gpui.Blue500).Rounded(6).
		OnClick(func(_ *gpui.ClickEvent, w *gpui.Window, _ *gpui.App) { t.add(w) }).
		Child(gpui.NewText("Add").TextColor(gpui.White))

	return gpui.NewDiv().
		FlexCol().SizeFull().Gap(12).P(16).Bg(gpui.Gray50).
		OnAction(addTodoAction{}, func(_ gpui.Action, w *gpui.Window, _ *gpui.App) { t.add(w) }).
		OnAction(blurAction{}, func(_ gpui.Action, w *gpui.Window, _ *gpui.App) { t.input.FocusHandle().Blur(w) }).
		Child(
			gpui.NewText("Todos").TextSize(24).FontWeight(700).TextColor(gpui.Gray900),
			gpui.NewDiv().FlexRow().Gap(8).ItemsCenter().Child(
				flexChild(t.input.Render(w, cx)),
				add,
			),
			list,
		)
}

// flexChild wraps el in a div that grows to fill the row.
func flexChild(el gpui.IntoElement) gpui.IntoElement {
	return gpui.NewDiv().Flex1().FlexCol().Child(el)
}

func main() {
	png := flag.String("png", "", "render one frame to this PNG path and exit")
	width := flag.Int("width", 480, "window width in logical pixels")
	height := flag.Int("height", 640, "window height in logical pixels")
	smoke := flag.Int("smoke", 0, "quit after this many milliseconds (interactive mode)")
	flag.Parse()

	app := NewTodoApp()

	if *png != "" {
		app.todos = []Todo{
			{Title: "Port GPUI to Go"},
			{Title: "Write the flexbox engine", Done: true},
			{Title: "Render text with strikethrough", Done: true},
			{Title: "Ship the todo example"},
		}
		app.nextID = len(app.todos)
		if err := gpui.RenderToPNG(*png, *width, *height, 1, app); err != nil {
			log.Fatal(err)
		}
		return
	}

	p := ebitenplatform.New()
	err := p.Run(func(cx *gpui.App) {
		cx.Keymap().AddBindings(
			gpui.NewKeyBinding("enter", addTodoAction{}, "textbox"),
			gpui.NewKeyBinding("escape", blurAction{}, "textbox"),
		)
		opts := gpui.WindowOptions{Width: float32(*width), Height: float32(*height), Title: "Todos", Resizable: true}
		if err := cx.OpenWindow(opts, func(*gpui.Window, *gpui.App) gpui.EntityRoot { return app }); err != nil {
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
