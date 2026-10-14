// Command tree renders a div with a deep hierarchy of nested children, each
// padded by 2px and coloured in a red/blue/green/yellow cycle. The depth
// defaults to 50 and can be changed with the GPUI_TREE_DEPTH environment
// variable or the -depth flag.
//
// Ported from gpui-ce crates/gpui/examples/legacy/tree.rs
package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"strconv"
	"time"

	"cad-lite/internal/gpui"
	"cad-lite/internal/gpui/ebitenplatform"
)

var palette = []gpui.Rgba{
	gpui.MustParseHex("#ff0000"),
	gpui.MustParseHex("#0000ff"),
	gpui.MustParseHex("#008000"),
	gpui.MustParseHex("#ffff00"),
}

// Tree is the root view.
type Tree struct {
	depth int
}

// Render builds the nested div hierarchy, innermost first.
func (t *Tree) Render(w *gpui.Window, cx *gpui.App) gpui.IntoElement {
	next := 0
	nextDiv := func() *gpui.Div {
		d := gpui.NewDiv().P(2).Bg(palette[next%len(palette)])
		next++
		return d
	}
	node := nextDiv()
	for i := 0; i < t.depth; i++ {
		node = nextDiv().Child(node)
	}
	return node.SizeFull()
}

func defaultDepth() int {
	if v, err := strconv.Atoi(os.Getenv("GPUI_TREE_DEPTH")); err == nil {
		return v
	}
	return 50
}

func main() {
	png := flag.String("png", "", "render one frame to this PNG path and exit")
	width := flag.Int("width", 300, "window width in logical pixels")
	height := flag.Int("height", 300, "window height in logical pixels")
	smoke := flag.Int("smoke", 0, "quit after this many milliseconds (interactive mode)")
	depth := flag.Int("depth", defaultDepth(), "nesting depth")
	flag.Parse()

	root := &Tree{depth: *depth}

	if *png != "" {
		if err := gpui.RenderToPNG(*png, *width, *height, 1, root); err != nil {
			log.Fatal(err)
		}
		return
	}

	err := ebitenplatform.New().Run(func(cx *gpui.App) {
		opts := gpui.WindowOptions{Width: float32(*width), Height: float32(*height), Title: "Tree", Resizable: true}
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
