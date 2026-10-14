package ui

import (
	"strings"
	"testing"

	"cad-lite/doc"
	"cad-lite/i18n"
	"cad-lite/internal/gpui"
)

// buttonCommands are the commands of every ribbon and title bar command button.
var buttonCommands = []string{"LINE", "PLINE", "CIRCLE", "ARC", "RECTANG", "MOVE", "COPY", "ROTATE", "SCALE", "ERASE",
	"MIRROR", "OFFSET", "FILLET", "CHAMFER", "TRIM", "EXTEND", "BREAK", "JOIN", "EXPLODE", "DIST", "AREA", "ID", "ZOOM",
	"U", "REDO"}

func TestCommandTipHeadlines(t *testing.T) {
	ws := NewWorkspace(doc.New(), nil)
	want := map[string]string{
		"LINE": "Line (L)", "PLINE": "Polyline (PL)", "CIRCLE": "Circle (C)", "ARC": "Arc (A)",
		"RECTANG": "Rectangle (REC)", "MOVE": "Move (M)", "COPY": "Copy (CO)", "ROTATE": "Rotate (RO)",
		"SCALE": "Scale (SC)", "ERASE": "Erase (E)", "MIRROR": "Mirror (MI)", "OFFSET": "Offset (O)",
		"FILLET": "Fillet (F)", "CHAMFER": "Chamfer (CHA)", "TRIM": "Trim (TR)", "EXTEND": "Extend (EX)",
		"BREAK": "Break (BR)", "JOIN": "Join (J)", "EXPLODE": "Explode (X)", "DIST": "Distance (DI)",
		"AREA": "Area (AA)", "ID": "ID point (ID)", "ZOOM": "Zoom (Z)",
		"U": "Undo (Ctrl+Z / U)", "REDO": "Redo (Ctrl+Y / REDO)",
	}
	for _, c := range buttonCommands {
		head, desc, _ := strings.Cut(ws.commandTip(c), "\n")
		if head != want[c] {
			t.Errorf("%s: headline %q, want %q", c, head, want[c])
		}
		if desc != i18n.Command(c).Desc || desc == "" {
			t.Errorf("%s: description %q", c, desc)
		}
	}
}

// TestCommandTipAliasesRun checks the hint against the registry: whatever a tooltip says to type
// runs the button's command, in every language.
func TestCommandTipAliasesRun(t *testing.T) {
	t.Cleanup(func() { i18n.Set(i18n.English) })
	ws := NewWorkspace(doc.New(), nil)
	reg := ws.session.Registry()
	for _, l := range i18n.Langs {
		i18n.Set(l)
		for _, c := range buttonCommands {
			hint := ws.keyHint(nil, c)
			got, ok := reg.Lookup(hint)
			want, _ := reg.Lookup(c)
			if !ok || got == nil || want == nil {
				t.Errorf("%s/%s: hint %q does not run a command", l, c, hint)
				continue
			}
			if !strings.HasPrefix(ws.commandTip(c), i18n.Command(c).Name+" (") {
				t.Errorf("%s/%s: tooltip %q does not start with the translated name", l, c, ws.commandTip(c))
			}
		}
	}
}

func TestFileAndSettingsTips(t *testing.T) {
	t.Cleanup(func() { i18n.Set(i18n.English) })
	ws := NewWorkspace(doc.New(), nil)
	for _, tc := range []struct{ got, head, has string }{
		{ws.openTip(), "Open (Ctrl+O / OPEN)", "file to open"},
		{ws.saveTip(), "Save (Ctrl+S / QSAVE)", "Save as (Ctrl+Shift+S / SAVEAS)"},
		{ws.settingsTip(), "Options (Ctrl+, / OP)", "language"},
	} {
		head, desc, _ := strings.Cut(tc.got, "\n")
		if head != tc.head || !strings.Contains(desc, tc.has) {
			t.Errorf("tip %q, want headline %q and %q", tc.got, tc.head, tc.has)
		}
	}
	i18n.Set(i18n.Italian)
	if got := ws.commandTip("LINE"); !strings.HasPrefix(got, "Linea (L)\n") {
		t.Errorf("LINE it: %q", got)
	}
	if got := ws.saveTip(); !strings.HasPrefix(got, "Salva (Ctrl+S / QSAVE)\n") || !strings.Contains(got, "Salva con nome (Ctrl+Shift+S / SAVEAS)") {
		t.Errorf("save it: %q", got)
	}
}

func TestShortcutTablePerPlatform(t *testing.T) {
	a, b := gpui.NewAction("a"), gpui.NewAction("b")
	bs := []gpui.KeyBinding{
		gpui.NewKeyBinding("ctrl-shift-s", a, ""),
		gpui.NewKeyBinding("cmd-shift-s", a, ""),
		gpui.NewKeyBinding("f10", b, ""),
	}
	if got := shortcutTable(bs, false); got["a"] != "Ctrl+Shift+S" || got["b"] != "F10" {
		t.Errorf("linux: %v", got)
	}
	if got := shortcutTable(bs, true); got["a"] != "Cmd+Shift+S" || got["b"] != "F10" {
		t.Errorf("mac: %v", got)
	}
	for in, want := range map[string]string{"ctrl-,": "Ctrl+,", "f2": "F2", "alt-escape": "Alt+Escape", "ctrl-e": "Ctrl+E"} {
		k, err := gpui.ParseKeystroke(in)
		if err != nil {
			t.Fatal(err)
		}
		if got := keystrokeLabel(k); got != want {
			t.Errorf("%s: %q, want %q", in, got, want)
		}
	}
}

// TestBoundCommandsHaveShortcut checks every command listed in commandKeys has a key binding: the
// shortcut a tooltip names is the one Render handles.
func TestBoundCommandsHaveShortcut(t *testing.T) {
	for command, action := range commandKeys {
		if shortcutOf(action) == "" {
			t.Errorf("%s: %s has no key binding", command, action.Name())
		}
	}
}
