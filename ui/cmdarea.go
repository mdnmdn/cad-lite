package ui

import (
	"math"
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
	// cmdHandleHeight is the height of the drag strip on the history's top edge; the fold button
	// overlaps the history's top right corner below it.
	cmdHandleHeight = 8
	// cmdFoldRowHeight is the height of the row a folded history leaves: the last line and the
	// fold button.
	cmdFoldRowHeight = collapseButtonSize
	// cmdPadX is the space either side of the command box; cmdHistPadL indents the history to the
	// prompt's left edge inside the box, and cmdHistPadR keeps it clear of the fold button.
	cmdPadX     = 8
	cmdHistPadL = cmdPadX + 1 + cmdBoxPadL
	cmdHistPadR = cmdFoldRight + collapseButtonSize + 8
	// cmdFoldRight is the fold button's distance from the area's right edge.
	cmdFoldRight = 8
	// cmdBoxHeight is the command box's height; cmdBoxPadL its left padding; cmdBoxGap the space
	// between the history and the box; cmdBoxPadB the space under the box.
	cmdBoxHeight = 32
	cmdBoxPadL   = 10
	cmdBoxGap    = 2
	cmdBoxPadB   = 6
	// cmdRecentSize is the edge of the clock button inside the box.
	cmdRecentSize = 24
	// cmdMenuBottom places the AutoComplete list and the recent commands menu above the box.
	cmdMenuBottom = cmdBoxPadB + cmdBoxHeight + 6
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

// commandArea is the strip under the canvas, width wide: the history, with a drag strip on its
// top edge that resizes it and a fold button in its top right corner, and the rounded command box
// (current prompt, input, recent commands), with the AutoComplete list or the recent commands menu
// above it. Folded, the history shrinks to one row holding its last line and the fold button.
func (ws *Workspace) commandArea(cmdline gpui.IntoElement, width float32, w *gpui.Window) *gpui.Div {
	hist := ws.session.History()
	off := ws.lay.histOff
	toggle := func(w *gpui.Window) { ws.toggleFold(&ws.lay.histOff, w) }
	fold := collapseButton("cmd-collapse", i18n.T("ui.cmdline.collapse_tip"), icons.AppChevronDown, toggle)
	if off {
		fold = collapseButton("cmd-collapse", i18n.T("ui.cmdline.expand_tip"), icons.AppChevronUp, toggle)
	}

	area := gpui.NewDiv().ID("cmd-area").Relative().FlexCol().FlexShrink(0).Bg(uiPanel)
	if off {
		row := gpui.NewDiv().ID("cmd-last").FlexRow().ItemsCenter().SizeH(cmdFoldRowHeight).FlexShrink(0).
			Pl(cmdHistPadL).Pr(cmdHistPadR).OverflowHidden()
		if line := ws.foldedLine(w, width); line != "" {
			row.Child(monoText(line, fontUI).TextColor(uiMuted).TextLineHeight(cmdLineHeight))
		}
		area.Child(row)
	} else {
		h := ws.historyHeight()
		n := max(1, int(h/cmdLineHeight))
		lines := gpui.NewDiv().ID("cmd-history").FlexCol().JustifyEnd().SizeH(h).FlexShrink(0).
			Pl(cmdHistPadL).Pr(cmdHistPadR).OverflowHidden()
		for i := max(0, len(hist)-n); i < len(hist); i++ {
			lines.Child(monoText(hist[i], fontUI).TextColor(uiMuted).TextLineHeight(cmdLineHeight))
		}
		handle := gpui.NewDiv().ID("cmd-handle").FlexRow().ItemsCenter().JustifyCenter().SizeH(cmdHandleHeight).FlexShrink(0).
			Tooltip(i18n.T("ui.cmdline.handle_tip")).
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
		area.Child(handle, lines)
	}

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
	recent.SizeWH(cmdRecentSize, cmdRecentSize)
	box := gpui.NewDiv().ID("cmd-box").FlexRow().ItemsCenter().Gap(8).SizeH(cmdBoxHeight).FlexShrink(0).Pl(cmdBoxPadL).Pr(3).Rounded(8).Border(1, border).Bg(uiField).Child(
		gpui.NewDiv().ID("cmd-prompt").FlexShrink(0).Tooltip(i18n.T("ui.cmdline.prompt_tip")).
			Child(monoText(ws.session.Prompt(), fontUI+1).TextColor(uiText)),
		gpui.NewDiv().Flex1().MinW(0).FlexCol().OnKeyDown(ws.commandKeyDown).Child(cmdline),
		recent,
	)
	area.Child(gpui.NewDiv().Px_(cmdPadX).Pt(cmdBoxGap).Pb(cmdBoxPadB).Child(box))
	// The fold button goes last so it paints, and takes the click, over the history's corner.
	area.Child(gpui.NewDiv().Absolute().Right(cmdFoldRight).Top(0).Child(fold))
	if ws.suggesting() {
		area.Child(ws.suggestList())
	} else if ws.cmd.open {
		area.Child(ws.recentMenu())
	}
	return area
}

// foldedLine is what the folded history shows in a command area width wide: its last line, cut
// to one row; "" when the history is empty.
func (ws *Workspace) foldedLine(w *gpui.Window, width float32) string {
	hist := ws.session.History()
	if len(hist) == 0 {
		return ""
	}
	return fitLine(w, hist[len(hist)-1], width-cmdHistPadL-cmdHistPadR)
}

// fitLine returns s cut to fit maxW at the history's type, with an ellipsis where it was cut;
// s itself when it fits or there is nothing to measure with.
func fitLine(w *gpui.Window, s string, maxW float32) string {
	m := w.TextMeasurer()
	if m == nil || maxW <= 0 {
		return s
	}
	ts := gpui.TextStyle{FontFamily: monoFamily, FontSize: fontUI, FontWeight: 400, LineHeight: cmdLineHeight}
	fits := func(t string) bool {
		tw, _ := m.MeasureWrapped(t, ts, math.MaxFloat32)
		return float32(tw) <= maxW
	}
	if fits(s) {
		return s
	}
	r := []rune(s)
	lo, hi := 0, len(r) // r[:lo]+"…" fits; r[:hi]+"…" does not, unless hi is 0
	for lo < hi {
		mid := (lo + hi + 1) / 2
		if fits(strings.TrimRight(string(r[:mid]), " ") + "…") {
			lo = mid
		} else {
			hi = mid - 1
		}
	}
	return strings.TrimRight(string(r[:lo]), " ") + "…"
}

// recentMenu lists the recent commands above the command box; a click runs one again.
func (ws *Workspace) recentMenu() *gpui.Div {
	menu := gpui.NewDiv().ID("cmd-recent-menu").Absolute().Right(cmdPadX).Bottom(cmdMenuBottom).SizeW(220).FlexCol().Gap(2).P(4).Rounded(8).
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
