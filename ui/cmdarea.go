package ui

import (
	"strings"

	"cad-lite/assets/icons"
	"cad-lite/i18n"
	"cad-lite/internal/gpui"
)

// Command area metrics, in logical pixels.
const (
	// cmdLineHeight is the height of one history line.
	cmdLineHeight = 18
	// defaultHistoryHeight is the history region's height until the user drags the handle, in a
	// window at least tallMinHeight high; a shorter window gets less, down to minDefaultHistory.
	defaultHistoryHeight = 96
	minDefaultHistory    = 54
	tallMinHeight        = 860
	// minHistoryHeight and maxHistoryHeight bound the dragged history height.
	minHistoryHeight = 18
	maxHistoryHeight = 480
	// cmdHandleHeight is the height of the handle row on the history's top edge, which also holds
	// the collapse button.
	cmdHandleHeight = 22
	// recentLimit is how many distinct recent commands are kept; recentShown how many the menu lists.
	recentLimit = 12
	recentShown = 8
)

// cmdAreaState is the command area's own state: the history height the handle drags, and the
// recent commands behind the clock button. Whether the history is collapsed lives in the
// workspace layout, which also tracks the handle's drag.
type cmdAreaState struct {
	// histH is the history region's height; 0 means defaultHistoryHeight.
	histH float32
	// recent holds the commands run lately, newest first; open shows the menu of them.
	recent []string
	open   bool
	// ac is the AutoComplete list and the recall of typed lines (cmdcomplete.go).
	ac completeState
}

func (ws *Workspace) historyHeight() float32 {
	if ws.cmd.histH <= 0 {
		if ws.winH > 0 && ws.winH < tallMinHeight {
			return max(minDefaultHistory, defaultHistoryHeight*ws.winH/tallMinHeight)
		}
		return defaultHistoryHeight
	}
	return min(maxHistoryHeight, max(minHistoryHeight, ws.cmd.histH))
}

// noteCommand puts a command the user ran at the front of the recent list.
func (ws *Workspace) noteCommand(name string) {
	name = strings.TrimSpace(name)
	if name == "" {
		return
	}
	c := &ws.cmd
	out := []string{name}
	for _, r := range c.recent {
		if !strings.EqualFold(r, name) && len(out) < recentLimit {
			out = append(out, r)
		}
	}
	c.recent = out
}

// commandArea is the strip under the canvas: the history, with a handle on its top edge that
// resizes it, and the rounded command box (current prompt, input, recent commands), with the
// AutoComplete list or the recent commands menu above it.
func (ws *Workspace) commandArea(cmdline gpui.IntoElement, w *gpui.Window) *gpui.Div {
	hist := ws.session.History()
	h := ws.historyHeight()
	n := max(1, int(h/cmdLineHeight))
	off := ws.lay.histOff
	lines := gpui.NewDiv().ID("cmd-history").FlexCol().JustifyEnd().SizeH(h).FlexShrink(0).Px_(16).OverflowHidden()
	for i := max(0, len(hist)-n); i < len(hist); i++ {
		lines.Child(monoText(hist[i], fontUI).TextColor(uiMuted).TextLineHeight(cmdLineHeight))
	}
	fold := collapseButton("cmd-collapse", i18n.T("ui.cmdline.collapse_tip"), icons.AppChevronDown, func(w *gpui.Window) { ws.toggleFold(&ws.lay.histOff, w) })
	if off {
		fold = collapseButton("cmd-collapse", i18n.T("ui.cmdline.expand_tip"), icons.AppChevronUp, func(w *gpui.Window) { ws.toggleFold(&ws.lay.histOff, w) })
	}
	handle := gpui.NewDiv().ID("cmd-handle").Relative().FlexRow().ItemsCenter().JustifyCenter().SizeH(cmdHandleHeight).FlexShrink(0)
	if !off {
		handle.Tooltip(i18n.T("ui.cmdline.handle_tip")).
			OnMouseDown(func(e *gpui.MouseDownEvent, w *gpui.Window, _ *gpui.App) {
				if e.Button != gpui.MouseLeft {
					return
				}
				w.StopPropagation()
				if e.ClickCount >= 2 {
					ws.cmd.histH = 0
					ws.persistLayout(w)
					w.Refresh()
					return
				}
				ws.beginDrag(splitHistory, float32(e.Position.Y), ws.historyHeight())
			}).
			Child(gpui.NewDiv().SizeWH(44, 3).Rounded(2).Bg(uiBorder))
	}
	handle.Child(gpui.NewDiv().Absolute().Right(8).Top(0).Child(fold))

	focused := false
	if fh := ws.input.FocusHandle(); fh != nil {
		focused = fh.IsFocused(w)
	}
	border := uiBorder
	if focused {
		border = uiAccent
	}
	recent := iconButton("cmd-recent", i18n.T("ui.cmdline.history_tip"), icons.AppHistory, ws.cmd.open, func(w *gpui.Window) {
		ws.cmd.open = !ws.cmd.open
		w.Refresh()
	})
	recent.SizeWH(28, 28)
	box := gpui.NewDiv().ID("cmd-box").FlexRow().ItemsCenter().Gap(10).SizeH(40).FlexShrink(0).Pl(12).Pr(6).Rounded(8).Border(1, border).Bg(uiField).Child(
		gpui.NewDiv().ID("cmd-prompt").FlexShrink(0).Tooltip(i18n.T("ui.cmdline.prompt_tip")).
			Child(monoText(ws.session.Prompt(), fontUI+1).TextColor(uiText)),
		gpui.NewDiv().Flex1().MinW(0).FlexCol().OnKeyDown(ws.commandKeyDown).Child(cmdline),
		recent,
	)
	area := gpui.NewDiv().ID("cmd-area").Relative().FlexCol().FlexShrink(0).Bg(uiPanel).
		Child(handle)
	if !off {
		area.Child(lines)
	}
	area.Child(gpui.NewDiv().Px_(10).Pb(10).Child(box))
	if ws.suggesting() {
		area.Child(ws.suggestList())
	} else if ws.cmd.open {
		area.Child(ws.recentMenu())
	}
	return area
}

// recentMenu lists the recent commands above the command box; a click runs one again.
func (ws *Workspace) recentMenu() *gpui.Div {
	menu := gpui.NewDiv().ID("cmd-recent-menu").Absolute().Right(14).Bottom(58).SizeW(220).FlexCol().Gap(2).P(4).Rounded(8).
		Bg(uiPanelAlt).Border(1, uiBorder)
	if len(ws.cmd.recent) == 0 {
		return menu.Child(gpui.NewText(i18n.T("ui.cmdline.history_empty")).TextColor(uiMuted).Px_(6).Py_(3))
	}
	for i, name := range ws.cmd.recent {
		if i == recentShown {
			break
		}
		name := name
		menu.Child(gpui.NewDiv().ID("cmd-recent-" + name).FlexRow().ItemsCenter().Px_(8).Py_(4).Rounded(4).
			Hover(func(m *gpui.StyledMixin) *gpui.StyledMixin { return m.Bg(uiBorder) }).
			OnClick(func(_ *gpui.ClickEvent, w *gpui.Window, _ *gpui.App) {
				ws.cmd.open = false
				ws.run(name, w)
			}).
			Child(monoText(i18n.For(i18n.Current()).CommandName(name), fontUI)))
	}
	return menu
}
