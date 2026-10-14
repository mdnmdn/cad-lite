package main

import (
	"go/parser"
	"go/token"
	"testing"
)

func TestScanFile(t *testing.T) {
	src := `package p
func f(ctx *C) {
	ctx.Printf("Nothing to redo.")
	_ = &Prompt{Message: "Specify first point", Keywords: []string{"Close"}}
	_ = gpui.NewText("go-draw")
	_ = gpui.NewText(fmt.Sprintf("%d selected", 3))
	_ = gpui.NewText("Layer: " + name)
	_ = gpui.NewText("x")
	ws.button("a", "Brand new label", nil)
}`
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "p.go", src, 0)
	if err != nil {
		t.Fatal(err)
	}
	got := scanFile(fset, f, map[string]string{"Nothing to redo.": "cmd.redo.msg.nothing_to_redo", "Layer: ": "x"})
	texts := map[string]string{}
	for _, g := range got {
		texts[g.text] = g.key
	}
	want := map[string]string{
		"Nothing to redo.": "cmd.redo.msg.nothing_to_redo", "Specify first point": "", "Close": "",
		"%d selected": "", "Layer: ": "x", "Brand new label": "",
	}
	if len(texts) != len(want) {
		t.Fatalf("got %v", texts)
	}
	for k, v := range want {
		if g, ok := texts[k]; !ok || g != v {
			t.Errorf("%q: key %q, present %v, want %q", k, g, ok, v)
		}
	}
}
