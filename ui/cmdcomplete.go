package ui

import (
	"fmt"
	"slices"
	"strings"
	"unicode"

	"cad-lite/cmdsys"
	"cad-lite/i18n"
	"cad-lite/internal/gpui"
)

// The command line's AutoComplete list and input recall, as in AutoCAD. At Command: typing a
// name opens a list of the matching commands above the command box; Up, Down and Tab move its
// highlight, Enter or Space runs the highlighted command and Escape closes the list. With the
// list closed, Up and Down at Command: recall the command lines typed there, newest first.

const (
	// suggestShown is how many matches the list shows.
	suggestShown = 10
	// recallLimit is how many typed command lines Up and Down can recall.
	recallLimit = 100
)

// Match ranks of a typed name against a command name or alias, best first.
const (
	matchExact = iota
	matchPrefix
	matchSubstring
	matchNone
)

// suggestion is one row of the AutoComplete list: the command it runs and the name or alias the
// typed text matched.
type suggestion struct {
	command, key string
	rank         int
}

// completeState is the AutoComplete list and the recall of typed command lines.
type completeState struct {
	// items are the matches of the field's text, best first; sel is the highlighted one. open is
	// false once Escape closed the list, until the text changes again.
	items []suggestion
	sel   int
	open  bool
	// lines are the command lines typed at Command:, oldest first. recall is the index of the
	// line the field shows while Up and Down browse them, -1 when they do not; draft is what the
	// field held before browsing began, which Down past the newest line brings back.
	lines  []string
	recall int
	draft  string
}

// wireCommandLine connects the command line field: Enter and Space submit or accept the
// highlighted suggestion, Escape closes the list or cancels, and any other edit refreshes the list.
func (ws *Workspace) wireCommandLine() {
	ws.cmd.ac.recall = -1
	ws.input.OnSubmit(func(text string, w *gpui.Window, _ *gpui.App) { ws.enterLine(text, w) })
	ws.input.OnEscape(func(w *gpui.Window, _ *gpui.App) {
		if ws.suggesting() {
			ws.cmd.ac.open = false
			w.Refresh()
			return
		}
		ws.escape(w)
	})
	ws.input.OnChange(func(text string, w *gpui.Window, _ *gpui.App) {
		if text != "" {
			ws.dismissStart()
		}
		ws.cmd.ac.recall = -1
		// Space is Enter, as in AutoCAD, except where the prompt takes text.
		if strings.HasSuffix(text, " ") && !ws.acceptsText() {
			ws.enterLine(strings.TrimSuffix(text, " "), w)
			return
		}
		ws.updateSuggestions(text)
	})
}

// enterLine is Enter (or Space) in the field: it runs the highlighted suggestion while the list
// shows, else submits text, remembering it for recall when it was typed at Command:.
func (ws *Workspace) enterLine(text string, w *gpui.Window) {
	if ws.suggesting() {
		ws.acceptSuggestion(ws.cmd.ac.sel, w)
		return
	}
	ws.cmd.ac.open = false
	ws.rememberLine(text)
	ws.submit(text, w)
}

// suggesting reports whether the AutoComplete list shows: only at Command:, with text typed and
// at least one match.
func (ws *Workspace) suggesting() bool {
	ac := &ws.cmd.ac
	return ac.open && len(ac.items) > 0 && ws.session.CurrentPrompt() == nil && strings.TrimSpace(ws.input.Text()) != ""
}

// updateSuggestions recomputes the list for the field's text and highlights the best match.
func (ws *Workspace) updateSuggestions(text string) {
	ac := &ws.cmd.ac
	ac.items, ac.sel = nil, 0
	if ws.session.CurrentPrompt() == nil {
		ac.items = ws.matchCommands(text)
	}
	ac.open = len(ac.items) > 0
	if ac.open {
		ws.cmd.open = false
	}
}

// acceptSuggestion runs suggestion i and closes the list.
func (ws *Workspace) acceptSuggestion(i int, w *gpui.Window) {
	ac := &ws.cmd.ac
	if i < 0 || i >= len(ac.items) {
		return
	}
	name := ac.items[i].command
	ac.open, ac.items = false, nil
	ws.rememberLine(name)
	ws.run(name, w)
}

// moveSuggestion moves the highlight by d, wrapping at either end.
func (ws *Workspace) moveSuggestion(d int) {
	ac := &ws.cmd.ac
	n := len(ac.items)
	ac.sel = ((ac.sel+d)%n + n) % n
}

// nameLike reports whether t can be the start of a command name: letters, digits, - and _.
func nameLike(t string) bool {
	for _, r := range t {
		if !unicode.IsLetter(r) && !unicode.IsDigit(r) && r != '-' && r != '_' {
			return false
		}
	}
	return true
}

// matchRank ranks typed name t against key, both upper case.
func matchRank(key, t string) int {
	switch {
	case key == t:
		return matchExact
	case strings.HasPrefix(key, t):
		return matchPrefix
	case strings.Contains(key, t):
		return matchSubstring
	}
	return matchNone
}

// matchCommands returns the commands whose name or an alias contains the typed text, one row
// per command under its best-matching name, at most suggestShown. Order: an exact name or alias
// first (so L and Enter run LINE whatever ran lately), then the recent commands newest first,
// then prefix matches before substring matches, each alphabetical. Text that is not a name (a
// coordinate), is only dashes or underscores, or names a system variable but no command matches
// nothing.
func (ws *Workspace) matchCommands(text string) []suggestion {
	t := strings.ToUpper(cmdsys.StripCommandPrefixes(strings.TrimSpace(text)))
	// A lone "-" (or "_") would match every dash command: a name needs a character after it.
	if strings.TrimLeft(t, "-_") == "" || !nameLike(t) {
		return nil
	}
	reg := ws.session.Registry()
	if reg == nil {
		return nil
	}
	var out []suggestion
	exact := false
	for _, name := range reg.Names() {
		best := suggestion{command: name, key: name, rank: matchRank(name, t)}
		for _, a := range reg.AliasesOf(name) {
			if r := matchRank(a, t); r < best.rank {
				best.key, best.rank = a, r
			}
		}
		if best.rank != matchNone {
			exact = exact || best.rank == matchExact
			out = append(out, best)
		}
	}
	if !exact {
		if v := ws.session.Vars(); v != nil && v.Has(t) {
			return nil
		}
	}
	recency := func(name string) int {
		if i := slices.IndexFunc(ws.cmd.recent, func(r string) bool { return strings.EqualFold(r, name) }); i >= 0 {
			return i
		}
		return len(ws.cmd.recent)
	}
	slices.SortFunc(out, func(a, b suggestion) int {
		if ea, eb := a.rank == matchExact, b.rank == matchExact; ea != eb {
			if ea {
				return -1
			}
			return 1
		}
		if ra, rb := recency(a.command), recency(b.command); ra != rb {
			return ra - rb
		}
		if a.rank != b.rank {
			return a.rank - b.rank
		}
		return strings.Compare(a.key, b.key)
	})
	if len(out) > suggestShown {
		out = out[:suggestShown]
	}
	return out
}

// rememberLine records a command line entered at Command: for Up and Down recall, skipping an
// empty line and a repeat of the newest.
func (ws *Workspace) rememberLine(text string) {
	ac := &ws.cmd.ac
	ac.recall = -1
	t := strings.TrimSpace(text)
	if t == "" || ws.session.CurrentPrompt() != nil {
		return
	}
	if n := len(ac.lines); n > 0 && ac.lines[n-1] == t {
		return
	}
	ac.lines = append(ac.lines, t)
	if n := len(ac.lines) - recallLimit; n > 0 {
		ac.lines = ac.lines[n:]
	}
}

// recallLine puts an earlier command line in the field: dir -1 (Up) goes back one line, +1 (Down)
// forward, and Down past the newest line restores what was typed before browsing.
func (ws *Workspace) recallLine(dir int) {
	ac := &ws.cmd.ac
	if len(ac.lines) == 0 {
		return
	}
	if ac.recall < 0 {
		if dir > 0 {
			return
		}
		ac.draft = ws.input.Text()
		ac.recall = len(ac.lines)
	}
	next := max(0, ac.recall+dir)
	if next >= len(ac.lines) {
		ac.recall = -1
		ws.input.SetText(ac.draft)
		return
	}
	ac.recall = next
	ws.input.SetText(ac.lines[next])
}

// commandKeyDown handles the keys the field leaves alone: Up, Down and Tab (Shift+Tab backwards)
// move through the list while it shows; otherwise Up and Down recall typed lines at Command:.
func (ws *Workspace) commandKeyDown(e *gpui.KeyDownEvent, w *gpui.Window, _ *gpui.App) {
	k := e.Keystroke
	if k.Modifiers&^gpui.Shift != 0 {
		return
	}
	switch k.Key {
	case "up", "down", "tab":
	default:
		return
	}
	switch {
	case ws.suggesting():
		d := 1
		if k.Key == "up" || k.Key == "tab" && k.Modifiers&gpui.Shift != 0 {
			d = -1
		}
		ws.moveSuggestion(d)
	case k.Key == "tab" || ws.session.CurrentPrompt() != nil:
		return
	case k.Key == "up":
		ws.recallLine(-1)
	default:
		ws.recallLine(1)
	}
	w.StopPropagation()
	w.Refresh()
}

// suggestList is the AutoComplete list above the command box: one row per match, the
// highlighted one in the soft accent fill. A row shows the matched name, the command an alias
// runs in brackets, and the command's display name when it is not just the name in other case.
func (ws *Workspace) suggestList() *gpui.Div {
	ac := &ws.cmd.ac
	t := i18n.For(i18n.Current())
	menu := gpui.NewDiv().ID("cmd-suggest").Absolute().Left(10).Bottom(58).MinW(240).FlexCol().Gap(1).P(4).Rounded(8).
		Bg(uiPanelAlt).Border(1, uiBorder)
	for i, s := range ac.items {
		col, sub := uiText, uiMuted
		label := s.key
		if s.key != s.command {
			label = fmt.Sprintf("%s (%s)", s.key, s.command)
		}
		row := gpui.NewDiv().ID(fmt.Sprintf("cmd-suggest-%d", i)).FlexRow().ItemsCenter().JustifyBetween().Gap(16).Px_(8).Py_(3).Rounded(4).
			OnClick(func(_ *gpui.ClickEvent, w *gpui.Window, _ *gpui.App) { ws.acceptSuggestion(i, w) })
		if i == ac.sel {
			row.Bg(activeFill())
			col, sub = uiOn, uiOn
		} else {
			row.Hover(func(m *gpui.StyledMixin) *gpui.StyledMixin { return m.Bg(uiBorder) })
		}
		row.Child(monoText(label, fontUI).TextColor(col))
		if name := t.CommandName(s.command); !strings.EqualFold(name, s.command) {
			row.Child(gpui.NewText(name).TextSize(fontSmall).TextColor(sub))
		}
		menu.Child(row)
	}
	return menu
}
