package ui

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"cad-lite/doc"
	"cad-lite/internal/gpui"
)

// typeCmd types s into the focused command line, one key at a time.
func (h *sHarness) typeCmd(s string) {
	for _, r := range s {
		c := string(r)
		key := c
		if c == " " {
			key = "space"
		}
		h.hw.Dispatch(gpui.KeyDownEvent{Keystroke: gpui.Keystroke{Key: key, KeyChar: &c}, PreferCharacterInput: true})
	}
	h.frame()
}

// suggestions returns the commands the AutoComplete list shows, top first; nil when it is hidden.
func (h *sHarness) suggestions() []string {
	if !h.ws.suggesting() {
		return nil
	}
	var out []string
	for _, s := range h.ws.cmd.ac.items {
		out = append(out, s.command)
	}
	return out
}

func completeRig(t *testing.T) *sHarness {
	t.Helper()
	return openSettingsWS(t, NewWorkspace(doc.New(), nil), 1000, 700, 1)
}

func (h *sHarness) savePNG(dir, name string) {
	h.t.Helper()
	d := filepath.Join("..", "_tmp", dir)
	if err := os.MkdirAll(d, 0o755); err != nil {
		h.t.Fatal(err)
	}
	if err := h.hw.SavePNG(filepath.Join(d, name)); err != nil {
		h.t.Fatal(err)
	}
}

func TestSuggestionsPutRecentCommandsFirst(t *testing.T) {
	h := completeRig(t)
	h.typeCmd("li")
	got := h.suggestions()
	line, pline := slices.Index(got, "LINE"), slices.Index(got, "PLINE")
	if line < 0 || pline < 0 || line > pline {
		t.Fatalf("LI suggests %v: want LINE (prefix) before PLINE (substring)", got)
	}
	if !h.has("cmd-suggest-0") {
		t.Fatal("the list is not drawn")
	}
	// The ranks never go down the list: prefix matches before substring ones, alphabetical within.
	items := h.ws.cmd.ac.items
	for i := 1; i < len(items); i++ {
		a, b := items[i-1], items[i]
		if a.rank > b.rank || a.rank == b.rank && a.key > b.key {
			t.Fatalf("out of order without recent commands: %+v before %+v", a, b)
		}
	}
	h.savePNG("cmdline", "suggest.png")

	h.key("escape")
	h.key("escape")
	h.ws.run("PLINE", h.hw.Window())
	h.key("escape")
	h.typeCmd("lin")
	if got := h.suggestions(); len(got) == 0 || got[0] != "PLINE" {
		t.Fatalf("after PLINE, LIN suggests %v: want the recent PLINE first", got)
	}
	h.savePNG("cmdline", "suggest-recent.png")
}

func TestExactAliasBeatsRecent(t *testing.T) {
	h := completeRig(t)
	h.ws.run("LAYER", h.hw.Window())
	h.key("escape")
	h.typeCmd("l")
	if got := h.suggestions(); len(got) == 0 || got[0] != "LINE" {
		t.Fatalf("L suggests %v: the alias L (LINE) must come first", got)
	}
	if k := h.ws.cmd.ac.items[0].key; k != "L" {
		t.Fatalf("LINE listed under %q, want the alias L", k)
	}
	h.key("enter")
	if a := h.ws.Session().Active(); a != "LINE" {
		t.Fatalf("L Enter started %q", a)
	}
}

func TestEnterSpaceAndArrowsAcceptSuggestion(t *testing.T) {
	h := completeRig(t)
	h.typeCmd("circ")
	if got := h.suggestions(); len(got) == 0 || got[0] != "CIRCLE" {
		t.Fatalf("CIRC suggests %v", got)
	}
	h.key("enter")
	if a := h.ws.Session().Active(); a != "CIRCLE" {
		t.Fatalf("Enter started %q, want CIRCLE", a)
	}
	if h.ws.input.Text() != "" || h.ws.suggesting() {
		t.Fatal("accepting did not clear the field and close the list")
	}
	h.key("escape")

	h.typeCmd("pol")
	if got := h.suggestions(); len(got) == 0 || got[0] != "POLYGON" {
		t.Fatalf("POL suggests %v", got)
	}
	h.typeCmd(" ")
	if a := h.ws.Session().Active(); a != "POLYGON" {
		t.Fatalf("Space started %q, want POLYGON", a)
	}
	h.key("escape")

	h.typeCmd("li")
	items := h.suggestions()
	if len(items) < 3 {
		t.Fatalf("LI suggests %v", items)
	}
	h.key("down")
	h.key("tab")
	if h.ws.cmd.ac.sel != 2 {
		t.Fatalf("Down and Tab highlight row %d, want 2", h.ws.cmd.ac.sel)
	}
	h.hw.Dispatch(gpui.KeyDownEvent{Keystroke: gpui.Keystroke{Key: "tab", Modifiers: gpui.Shift}})
	h.key("up")
	h.key("up")
	if h.ws.cmd.ac.sel != len(items)-1 {
		t.Fatalf("Shift+Tab and Up twice from row 2 highlight %d, want the last row %d (wrap)", h.ws.cmd.ac.sel, len(items)-1)
	}
	h.key("down")
	h.key("down")
	h.frame()
	h.savePNG("cmdline", "suggest-highlight.png")
	h.key("enter")
	if a := h.ws.Session().Active(); a != items[1] {
		t.Fatalf("Enter on row 1 started %q, want %q", a, items[1])
	}
	h.key("escape")

	h.typeCmd("li")
	h.clickID("cmd-suggest-1")
	if a := h.ws.Session().Active(); a != h.ws.cmd.recent[0] || a == "" {
		t.Fatalf("clicking row 1 started %q", a)
	}
}

func TestEscapeClosesSuggestionsFirst(t *testing.T) {
	h := completeRig(t)
	h.typeCmd("m")
	if !h.ws.suggesting() {
		t.Fatal("no list for M")
	}
	h.key("escape")
	h.frame()
	if h.ws.suggesting() || h.has("cmd-suggest-0") {
		t.Fatal("Escape did not close the list")
	}
	if h.ws.input.Text() != "m" {
		t.Fatalf("Escape with the list open changed the field to %q", h.ws.input.Text())
	}
	h.key("enter")
	if a := h.ws.Session().Active(); a != "MOVE" {
		t.Fatalf("Enter with the list closed started %q; the typed M runs MOVE through its alias", a)
	}
	h.key("escape")
	h.typeCmd("m")
	h.key("escape")
	h.key("escape")
	if h.ws.input.Text() != "" {
		t.Fatalf("the second Escape left %q", h.ws.input.Text())
	}
}

func TestNoSuggestionsAtACommandPrompt(t *testing.T) {
	h := completeRig(t)
	h.ws.run("LINE", h.hw.Window())
	h.frame()
	h.typeCmd("ci")
	if h.ws.suggesting() || h.has("cmd-suggest-0") {
		t.Fatal("suggestions while LINE asks for a point")
	}
	h.key("up")
	if h.ws.input.Text() != "ci" {
		t.Fatalf("Up at a command's prompt changed the field to %q", h.ws.input.Text())
	}
	h.key("escape")
	// A coordinate or a system variable is not a command name.
	for _, s := range []string{"10,10", "osmode"} {
		h.typeCmd(s)
		if h.ws.suggesting() {
			t.Fatalf("%q suggests %v", s, h.suggestions())
		}
		h.key("escape")
	}
}

func TestUpAndDownRecallCommandLines(t *testing.T) {
	h := completeRig(t)
	for _, s := range []string{"circle", "line"} {
		h.typeCmd(s)
		h.key("escape") // close the list: the typed line is entered as it stands
		h.key("enter")
		h.key("escape")
	}
	h.typeCmd("z")
	h.key("escape")
	field := func() string { return h.ws.input.Text() }
	for _, c := range []struct{ key, want string }{
		{"up", "line"}, {"up", "circle"}, {"up", "circle"}, {"down", "line"}, {"down", "z"}, {"down", "z"},
	} {
		h.key(c.key)
		if field() != c.want {
			t.Fatalf("%s: field %q, want %q (lines %v)", c.key, field(), c.want, h.ws.cmd.ac.lines)
		}
	}
	h.ws.input.SetText("")
	h.key("up")
	h.frame()
	if h.ws.suggesting() {
		t.Fatal("a recalled line opened the list")
	}
	h.key("enter")
	if a := h.ws.Session().Active(); a != "LINE" {
		t.Fatalf("Enter on the recalled line started %q", a)
	}
	h.key("escape")
	// With the list open, Up moves the highlight instead.
	h.typeCmd("li")
	h.key("up")
	if !strings.EqualFold(field(), "li") || h.ws.cmd.ac.sel == 0 {
		t.Fatalf("Up with the list open: field %q, highlight %d", field(), h.ws.cmd.ac.sel)
	}
}

func TestPolygonRibbonButtonRunsPolygon(t *testing.T) {
	h := completeRig(t)
	h.clickID("tool-polygon")
	if a := h.ws.Session().Active(); a != "POLYGON" {
		t.Fatalf("the Polygon button started %q", a)
	}
	h.frame()
	h.savePNG("cmdline", "ribbon-polygon.png")
}

// A dash alone lists nothing; one letter after it lists the dash commands that match.
func TestLoneDashSuggestsNothing(t *testing.T) {
	h := completeRig(t)
	h.typeCmd("-")
	if got := h.suggestions(); got != nil {
		t.Fatalf("\"-\" suggests %v", got)
	}
	h.typeCmd("l")
	got := h.suggestions()
	if len(got) == 0 || !slices.Contains(got, "-LAYER") {
		t.Fatalf("\"-l\" suggests %v, want -LAYER among them", got)
	}
}
