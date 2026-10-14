package gpui_test

import (
	"testing"

	"go-draw/internal/gpui"
)

func TestNewAction(t *testing.T) {
	a := gpui.NewAction("x")
	if a.Name() != "x" {
		t.Errorf("Name() = %q, want \"x\"", a.Name())
	}
	c := a.Clone()
	if !c.Equals(a) {
		t.Error("Clone should equal the original")
	}
	if c.Name() != "x" {
		t.Errorf("Clone().Name() = %q, want \"x\"", c.Name())
	}
	if !a.Equals(gpui.NewAction("x")) {
		t.Error("actions with the same name should be equal")
	}
	if a.Equals(gpui.NewAction("y")) {
		t.Error("actions with different names should not be equal")
	}
}

func TestNewKeyBinding(t *testing.T) {
	b := gpui.NewKeyBinding("cmd-enter", gpui.NewAction("add"), "textbox")
	if b.Action.Name() != "add" {
		t.Errorf("Action.Name() = %q, want \"add\"", b.Action.Name())
	}
	if len(b.Keystrokes) != 1 {
		t.Fatalf("len(Keystrokes) = %d, want 1", len(b.Keystrokes))
	}
	want := mustKeystroke(t, "cmd-enter")
	if !b.Keystrokes[0].Equals(want) {
		t.Errorf("Keystrokes[0] = %v, want %v", b.Keystrokes[0], want)
	}
	if b.Context != "textbox" {
		t.Errorf("Context = %q, want \"textbox\"", b.Context)
	}
}

func TestNewKeyBindingPanics(t *testing.T) {
	mustPanic(t, "malformed keystroke", func() {
		gpui.NewKeyBinding("cmd-", gpui.NewAction("a"), "")
	})
	mustPanic(t, "malformed context", func() {
		gpui.NewKeyBinding("a", gpui.NewAction("a"), "editor &&")
	})
	mustPanic(t, "unbalanced paren context", func() {
		gpui.NewKeyBinding("a", gpui.NewAction("a"), "(editor")
	})
}

func TestMatchKeystroke(t *testing.T) {
	km := gpui.NewKeymap()
	km.AddBindings(gpui.NewKeyBinding("cmd-enter", gpui.NewAction("add"), "textbox"))

	ks := mustKeystroke(t, "cmd-enter")
	a, ok := km.MatchKeystrokeWithContext(ks, []string{"textbox"})
	if !ok || a == nil || a.Name() != "add" {
		t.Fatalf("MatchKeystroke(cmd-enter, [textbox]) = %v, %v; want add", a, ok)
	}
	if _, ok := km.MatchKeystrokeWithContext(mustKeystroke(t, "enter"), []string{"textbox"}); ok {
		t.Error("non-matching keystroke should not match")
	}
}

func TestMatchKeystrokeContext(t *testing.T) {
	km := gpui.NewKeymap()
	km.AddBindings(gpui.NewKeyBinding("escape", gpui.NewAction("blur"), "textbox"))

	ks := mustKeystroke(t, "escape")
	if a := km.MatchKeystroke(ks, []string{"textbox"}); a == nil || a.Name() != "blur" {
		t.Errorf("context [textbox] against \"textbox\": got %v, want blur", a)
	}
	if a := km.MatchKeystroke(ks, []string{"menu"}); a != nil {
		t.Errorf("context [menu] against \"textbox\": got %v, want nil", a)
	}

	km2 := gpui.NewKeymap()
	km2.AddBindings(gpui.NewKeyBinding("escape", gpui.NewAction("anywhere"), ""))
	if a := km2.MatchKeystroke(ks, nil); a == nil || a.Name() != "anywhere" {
		t.Errorf("empty context against nil stack: got %v, want anywhere", a)
	}
	if a := km2.MatchKeystroke(ks, []string{"menu", "editor"}); a == nil || a.Name() != "anywhere" {
		t.Errorf("empty context against [menu editor]: got %v, want anywhere", a)
	}
}

func TestMatchKeystrokePredicates(t *testing.T) {
	ks := mustKeystroke(t, "a")

	andNot := gpui.NewKeymap()
	andNot.AddBindings(gpui.NewKeyBinding("a", gpui.NewAction("act"), "editor && !readonly"))

	if a := andNot.MatchKeystroke(ks, []string{"editor"}); a == nil || a.Name() != "act" {
		t.Errorf("\"editor && !readonly\" vs [editor]: got %v, want act", a)
	}
	if a := andNot.MatchKeystroke(ks, []string{"editor", "readonly"}); a != nil {
		t.Errorf("\"editor && !readonly\" vs [editor readonly]: got %v, want nil", a)
	}
	if a := andNot.MatchKeystroke(ks, []string{"menu"}); a != nil {
		t.Errorf("\"editor && !readonly\" vs [menu]: got %v, want nil", a)
	}

	or := gpui.NewKeymap()
	or.AddBindings(gpui.NewKeyBinding("a", gpui.NewAction("act"), "a || b"))
	if a := or.MatchKeystroke(ks, []string{"b"}); a == nil || a.Name() != "act" {
		t.Errorf("\"a || b\" vs [b]: got %v, want act", a)
	}
	if a := or.MatchKeystroke(ks, []string{"z"}); a != nil {
		t.Errorf("\"a || b\" vs [z]: got %v, want nil", a)
	}

	val := gpui.NewKeymap()
	val.AddBindings(gpui.NewKeyBinding("a", gpui.NewAction("act"), "mode=normal"))
	if a := val.MatchKeystroke(ks, []string{"mode=normal"}); a == nil || a.Name() != "act" {
		t.Errorf("\"mode=normal\" vs [mode=normal]: got %v, want act", a)
	}
	if a := val.MatchKeystroke(ks, []string{"mode=insert"}); a != nil {
		t.Errorf("\"mode=normal\" vs [mode=insert]: got %v, want nil", a)
	}
}

func TestMatchKeystrokePredicatePrecedence(t *testing.T) {
	ks := mustKeystroke(t, "a")
	// ! binds tighter than &&, which binds tighter than ||:
	// "a && !b || c" == "(a && (!b)) || c"
	km := gpui.NewKeymap()
	km.AddBindings(gpui.NewKeyBinding("a", gpui.NewAction("act"), "a && !b || c"))

	if a := km.MatchKeystroke(ks, []string{"c"}); a == nil || a.Name() != "act" {
		t.Errorf("\"a && !b || c\" vs [c]: got %v, want act", a)
	}
	if a := km.MatchKeystroke(ks, []string{"a"}); a == nil || a.Name() != "act" {
		t.Errorf("\"a && !b || c\" vs [a]: got %v, want act", a)
	}
	if a := km.MatchKeystroke(ks, []string{"a", "b"}); a != nil {
		t.Errorf("\"a && !b || c\" vs [a b]: got %v, want nil", a)
	}

	parens := gpui.NewKeymap()
	parens.AddBindings(gpui.NewKeyBinding("a", gpui.NewAction("act"), "(a || b) && !c"))
	if a := parens.MatchKeystroke(ks, []string{"b"}); a == nil || a.Name() != "act" {
		t.Errorf("parens vs [b]: got %v, want act", a)
	}
	if a := parens.MatchKeystroke(ks, []string{"b", "c"}); a != nil {
		t.Errorf("parens vs [b c]: got %v, want nil", a)
	}
}

func TestMatchKeystrokeReverseInsertionOrder(t *testing.T) {
	km := gpui.NewKeymap()
	km.AddBindings(
		gpui.NewKeyBinding("cmd-p", gpui.NewAction("first"), ""),
		gpui.NewKeyBinding("cmd-p", gpui.NewAction("second"), ""),
	)
	ks := mustKeystroke(t, "cmd-p")
	if a := km.MatchKeystroke(ks, nil); a == nil || a.Name() != "second" {
		t.Errorf("MatchKeystroke = %v; want the later binding \"second\"", a)
	}
}

func TestMatchKeystrokeContextBeatsGlobal(t *testing.T) {
	km := gpui.NewKeymap()
	km.AddBindings(
		gpui.NewKeyBinding("escape", gpui.NewAction("global"), ""),
		gpui.NewKeyBinding("escape", gpui.NewAction("textbox"), "textbox"),
	)
	ks := mustKeystroke(t, "escape")
	if a := km.MatchKeystroke(ks, []string{"textbox"}); a == nil || a.Name() != "textbox" {
		t.Errorf("MatchKeystroke(escape, [textbox]) = %v; want the context binding", a)
	}
	if a := km.MatchKeystroke(ks, []string{"menu"}); a == nil || a.Name() != "global" {
		t.Errorf("MatchKeystroke(escape, [menu]) = %v; want the global binding", a)
	}
}

func TestKeymapLenAndClear(t *testing.T) {
	km := gpui.NewKeymap()
	if km.Len() != 0 {
		t.Errorf("fresh Keymap Len = %d, want 0", km.Len())
	}
	km.AddBindings(
		gpui.NewKeyBinding("a", gpui.NewAction("one"), ""),
		gpui.NewKeyBinding("b", gpui.NewAction("two"), ""),
	)
	if km.Len() != 2 {
		t.Errorf("Len = %d, want 2", km.Len())
	}
	bindings := km.Bindings()
	if len(bindings) != 2 {
		t.Errorf("len(Bindings) = %d, want 2", len(bindings))
	}
	km.Clear()
	if km.Len() != 0 {
		t.Errorf("Len after Clear = %d, want 0", km.Len())
	}
	if a := km.MatchKeystroke(mustKeystroke(t, "a"), nil); a != nil {
		t.Errorf("MatchKeystroke after Clear = %v, want nil", a)
	}
}

func mustKeystroke(t *testing.T, s string) gpui.Keystroke {
	t.Helper()
	ks, err := gpui.ParseKeystroke(s)
	if err != nil {
		t.Fatalf("ParseKeystroke(%q): %v", s, err)
	}
	return ks
}

func mustPanic(t *testing.T, what string, f func()) {
	t.Helper()
	defer func() {
		if recover() == nil {
			t.Errorf("expected panic for %s", what)
		}
	}()
	f()
}
