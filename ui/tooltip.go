package ui

import (
	"runtime"
	"strings"
	"sync"

	"cad-lite/i18n"
	"cad-lite/internal/gpui"
)

// Toolbar tooltips are composed, never written out: the first line is the command's translated
// name with its keyboard hint in brackets ("Line (L)", "Save (Ctrl+S / QSAVE)"), the second is
// its translated description. The shortcut is read back from Bindings and the alias from the
// command registry, so a tooltip cannot name a key or an alias that does not work.

// commandKeys maps each command a key binding runs to the binding's action. Render registers the
// handlers from it and commandTip reads the shortcut back through it.
var commandKeys = map[string]gpui.Action{"U": actionUndo, "REDO": actionRedo}

// shortcutLabels maps an action's name to the display form of its binding on this platform.
var shortcutLabels = sync.OnceValue(func() map[string]string {
	return shortcutTable(Bindings(), runtime.GOOS == "darwin")
})

// shortcutTable maps each action of bindings to the label of its binding for the platform: on
// macOS a Cmd binding wins over a Ctrl one; elsewhere Cmd bindings (the Super key there) are
// skipped. The first binding of an action wins among equals.
func shortcutTable(bindings []gpui.KeyBinding, mac bool) map[string]string {
	out := map[string]string{}
	platform := map[string]bool{}
	for _, b := range bindings {
		if b.Action == nil || len(b.Keystrokes) != 1 {
			continue
		}
		k := b.Keystrokes[0]
		cmd := k.Modifiers&gpui.ModPlatform != 0
		if cmd && !mac {
			continue
		}
		n := b.Action.Name()
		if _, ok := out[n]; ok && (platform[n] || !cmd) {
			continue
		}
		out[n], platform[n] = keystrokeLabel(k), cmd
	}
	return out
}

// keystrokeLabel is the way a key is written in a tooltip: "Ctrl+Shift+S", "F2", "Ctrl+,".
func keystrokeLabel(k gpui.Keystroke) string {
	var parts []string
	for _, m := range []struct {
		mod  gpui.Modifiers
		name string
	}{{gpui.ModPlatform, "Cmd"}, {gpui.Control, "Ctrl"}, {gpui.Alt, "Alt"}, {gpui.Shift, "Shift"}} {
		if k.Modifiers&m.mod != 0 {
			parts = append(parts, m.name)
		}
	}
	key := k.Key
	if len(key) == 1 || (len(key) <= 3 && key[0] == 'f') {
		key = strings.ToUpper(key)
	} else if key != "" {
		key = strings.ToUpper(key[:1]) + key[1:]
	}
	return strings.Join(append(parts, key), "+")
}

// shortcutOf returns the label of action's key binding, or "" when it has none.
func shortcutOf(action gpui.Action) string {
	if action == nil {
		return ""
	}
	return shortcutLabels()[action.Name()]
}

// typedName is the shortest thing to type to run command: its shortest registered alias, or the
// command's own name when no alias is shorter.
func (ws *Workspace) typedName(command string) string {
	command = strings.ToUpper(command)
	if ws.session == nil || ws.session.Registry() == nil {
		return command
	}
	if a := ws.session.Registry().AliasesOf(command); len(a) > 0 && len(a[0]) < len(command) {
		return a[0]
	}
	return command
}

// keyHint is the bracketed part of a tooltip: action's shortcut, then the typed name of command,
// joined by " / ". Either may be empty.
func (ws *Workspace) keyHint(action gpui.Action, command string) string {
	var parts []string
	if s := shortcutOf(action); s != "" {
		parts = append(parts, s)
	}
	if command != "" {
		parts = append(parts, ws.typedName(command))
	}
	return strings.Join(parts, " / ")
}

// toolTip joins a button's name, keyboard hint and description into a tooltip.
func toolTip(name, hint, desc string) string {
	s := name
	if hint != "" {
		s += " (" + hint + ")"
	}
	if desc != "" {
		s += "\n" + desc
	}
	return s
}

// commandTip is the tooltip of a button that runs command.
func (ws *Workspace) commandTip(command string) string {
	c := i18n.Command(command)
	return toolTip(c.Name, ws.keyHint(commandKeys[strings.ToUpper(command)], command), c.Desc)
}

// openTip is the Open button's tooltip.
func (ws *Workspace) openTip() string {
	return toolTip(i18n.CommandName("OPEN"), ws.keyHint(actionOpen, "OPEN"), tb("open_tip"))
}

// saveTip is the Save button's tooltip; its description names Save as, on the right click.
func (ws *Workspace) saveTip() string {
	return toolTip(i18n.CommandName("SAVE"), ws.keyHint(actionSave, "QSAVE"),
		i18n.T("ui.toolbar.save_tip", ws.keyHint(actionSaveAs, "SAVEAS")))
}
