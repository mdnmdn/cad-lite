package ui

import (
	"fmt"
	"strings"

	"cad-lite/internal/gpui"
)

// mdView renders parsed Markdown as gpui elements. gpui's StyledText lays its runs side by side
// rather than flowing them, so a paragraph with inline styles is a wrapping row of words, each
// word one element per style run; a paragraph without styles is a single Text, which wraps best.
type mdView struct {
	// id prefixes the ids of clickable links, which must be unique in the transcript.
	id string
	// size is the body font size; muted renders everything in the muted colour (thoughts).
	size  float32
	muted bool
	// open opens a link's URL; nil leaves links styled but inert.
	open func(url string)
	n    int
}

// agentCodeLimit caps a rendered code block, in runes; the copy button still copies all of it.
const agentCodeLimit = 6000

func (v *mdView) text() gpui.Rgba {
	if v.muted {
		return uiMuted
	}
	return uiText
}

// codeFill is the tint behind code: a step from the panel towards the text colour, so it reads in
// light and dark themes alike.
func codeFill() gpui.Rgba { return mix(uiPanel, uiText, 0.08) }

// render renders blocks as a column.
func (v *mdView) render(bs []mdBlock) *gpui.Div {
	col := gpui.NewDiv().FlexCol().Gap(6).MinW(0)
	for _, b := range bs {
		col.Child(v.block(b))
	}
	return col
}

func (v *mdView) block(b mdBlock) gpui.IntoElement {
	switch b.kind {
	case mdHeading:
		size := v.size
		switch b.level {
		case 1:
			size += 5
		case 2:
			size += 3
		case 3:
			size++
		}
		return v.inline(b.inl, size, 700)
	case mdCode:
		code := b.code
		if r := []rune(code); len(r) > agentCodeLimit {
			code = string(r[:agentCodeLimit]) + "\n..."
		}
		box := gpui.NewDiv().FlexCol().Gap(4).P(8).Rounded(4).Bg(codeFill()).MinW(0)
		if b.lang != "" {
			box.Child(gpui.NewText(b.lang).TextSize(10).TextColor(uiMuted))
		}
		// Long lines wrap: a horizontally scrolling box would take the wheel from the transcript.
		return box.Child(gpui.NewText(code).FontFamily(monoFamily).TextSize(v.size - 1).TextColor(v.text()))
	case mdList:
		col := gpui.NewDiv().FlexCol().Gap(3).MinW(0)
		for k, item := range b.items {
			mark := "•"
			if b.ordered {
				mark = fmt.Sprintf("%d.", b.start+k)
			}
			col.Child(gpui.NewDiv().FlexRow().ItemsStart().Gap(6).MinW(0).Child(
				gpui.NewText(mark).TextSize(v.size).TextColor(uiMuted).FlexShrink(0).MinW(12),
				v.render(item).Flex1()))
		}
		return col
	case mdQuote:
		q := *v
		q.muted = true
		inner := q.render(b.sub).Flex1()
		v.n = q.n
		return gpui.NewDiv().FlexRow().Gap(8).MinW(0).Child(
			gpui.NewDiv().SizeW(3).Rounded(1).Bg(uiBorder).FlexShrink(0), inner)
	case mdRule:
		return gpui.NewDiv().SizeH(1).Bg(uiBorder).My(4)
	case mdTable:
		return v.table(b)
	}
	return v.inline(b.inl, v.size, 400)
}

func (v *mdView) table(b mdBlock) gpui.IntoElement {
	t := gpui.NewDiv().FlexCol().Rounded(4).Border(1, uiBorder).MinW(0)
	row := func(cells [][]mdSpan, head bool) *gpui.Div {
		r := gpui.NewDiv().FlexRow().MinW(0)
		if head {
			r.Bg(codeFill())
		}
		for k, c := range cells {
			cell := gpui.NewDiv().FlexCol().Flex1().MinW(0).Px_(6).Py_(3)
			if k < len(b.align) {
				switch b.align[k] {
				case 'r':
					cell.ItemsEnd()
				case 'c':
					cell.ItemsCenter()
				}
			}
			w := 400
			if head {
				w = 700
			}
			r.Child(cell.Child(v.inline(c, v.size-1, w)))
		}
		return r
	}
	t.Child(row(b.head, true))
	for _, r := range b.rows {
		t.Child(gpui.NewDiv().SizeH(1).Bg(uiBorder), row(r, false))
	}
	return t
}

// plainSpans reports whether spans carry no inline style at all.
func plainSpans(spans []mdSpan) bool {
	for _, s := range spans {
		if s.bold || s.italic || s.code || s.strike || s.link != "" || strings.Contains(s.text, "\n") {
			return false
		}
	}
	return true
}

// inline renders a paragraph's spans at size and weight.
func (v *mdView) inline(spans []mdSpan, size float32, weight int) gpui.IntoElement {
	if plainSpans(spans) {
		return gpui.NewText(mdPlain(spans)).TextSize(size).FontWeight(weight).TextColor(v.text())
	}
	// Hard breaks split the paragraph into lines; each line is a wrapping row of words.
	col := gpui.NewDiv().FlexCol().MinW(0)
	var line []mdSpan
	flushLine := func() {
		col.Child(v.flow(line, size, weight))
		line = nil
	}
	for _, s := range spans {
		parts := strings.Split(s.text, "\n")
		for k, part := range parts {
			if k > 0 {
				flushLine()
			}
			if part != "" {
				c := s
				c.text = part
				line = append(line, c)
			}
		}
	}
	flushLine()
	return col
}

// mdWord is one wrap unit: the pieces of text between two spaces, each with its own style.
type mdWord []mdSpan

// words splits spans at spaces; code spans are never split.
func words(spans []mdSpan) []mdWord {
	var out []mdWord
	var cur mdWord
	for _, s := range spans {
		if s.code {
			cur = append(cur, s)
			continue
		}
		fields := strings.Split(s.text, " ")
		for k, f := range fields {
			if k > 0 && len(cur) > 0 {
				out = append(out, cur)
				cur = nil
			}
			if f != "" {
				c := s
				c.text = f
				cur = append(cur, c)
			}
		}
	}
	if len(cur) > 0 {
		out = append(out, cur)
	}
	return out
}

func (v *mdView) flow(spans []mdSpan, size float32, weight int) *gpui.Div {
	row := gpui.NewDiv().FlexRow().FlexWrap().ItemsCenter().ColumnGap(size * 0.28).RowGap(1).MinW(0)
	if len(spans) == 0 {
		return row.SizeH(size * 1.2)
	}
	for _, w := range words(spans) {
		if len(w) == 1 {
			row.Child(v.piece(w[0], size, weight))
			continue
		}
		g := gpui.NewDiv().FlexRow().ItemsCenter().FlexShrink(0)
		for _, s := range w {
			g.Child(v.piece(s, size, weight))
		}
		row.Child(g)
	}
	return row
}

// piece renders one styled run of a word.
func (v *mdView) piece(s mdSpan, size float32, weight int) gpui.IntoElement {
	t := gpui.NewText(s.text).TextSize(size).FontWeight(weight).TextColor(v.text())
	if s.bold {
		t.FontWeight(700)
	}
	if s.italic {
		// gpui has no italic faces: emphasis reads as a softer colour.
		t.TextColor(mix(v.text(), uiMuted, 0.55))
	}
	if s.strike {
		t.TextStrikethrough()
	}
	var el gpui.IntoElement = t
	if s.code {
		t.FontFamily(monoFamily).TextSize(size - 1)
		el = gpui.NewDiv().Px_(3).Rounded(3).Bg(codeFill()).FlexShrink(0).Child(t)
	}
	if s.link == "" {
		return el
	}
	t.TextColor(uiAccent)
	v.n++
	url := s.link
	d := gpui.NewDiv().ID(fmt.Sprintf("%s-link-%d", v.id, v.n)).FlexShrink(0).Tooltip(url).
		Hover(func(m *gpui.StyledMixin) *gpui.StyledMixin { return m.Opacity(0.75) }).Child(el)
	if v.open != nil {
		d.OnClick(func(_ *gpui.ClickEvent, _ *gpui.Window, _ *gpui.App) { v.open(url) })
	}
	return d
}
