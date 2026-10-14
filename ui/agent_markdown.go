package ui

import (
	"regexp"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

// This file is the agent panel's Markdown parser: the subset agents write (CommonMark blocks and
// inlines, GitHub tables), parsed leniently so a message still streaming in renders sensibly. An
// unclosed code fence is an open code block, an unclosed emphasis or link is literal text, and a
// table shows once its delimiter row has arrived. It is pure: no gpui, no theme.

// mdKind is the kind of a Markdown block.
type mdKind int

const (
	mdPara mdKind = iota
	mdHeading
	mdCode
	mdList
	mdQuote
	mdRule
	mdTable
)

// mdBlock is one Markdown block.
type mdBlock struct {
	kind mdKind
	// level is the heading level (1-6).
	level int
	// inl is the inline content of a paragraph or heading.
	inl []mdSpan
	// code and lang are a code block's text and info string; open is set while its closing fence
	// has not arrived.
	code, lang string
	open       bool
	// ordered, start and items describe a list.
	ordered bool
	start   int
	items   [][]mdBlock
	// sub is a block quote's content.
	sub []mdBlock
	// head and rows are a table's header cells and body rows; align is per column: 'l', 'c',
	// 'r' or 0.
	head  [][]mdSpan
	rows  [][][]mdSpan
	align []byte
}

// mdSpan is a run of inline text with one style.
type mdSpan struct {
	text                       string
	bold, italic, code, strike bool
	link                       string
}

// mdMaxDepth caps the nesting of quotes and lists; deeper content is a plain paragraph, so input
// such as "- - - - ..." costs linear time rather than one level of recursion per marker.
const mdMaxDepth = 16

// parseMarkdown parses src into blocks.
func parseMarkdown(src string) []mdBlock {
	src = strings.ReplaceAll(src, "\r\n", "\n")
	src = strings.ReplaceAll(src, "\t", "    ")
	return parseBlocks(strings.Split(src, "\n"), 0)
}

var (
	mdHeadingRe = regexp.MustCompile(`^ {0,3}(#{1,6})(?:[ \t]+(.*?))?(?:[ \t]+#+)?[ \t]*$`)
	mdRuleRe    = regexp.MustCompile(`^ {0,3}(?:(?:\*[ \t]*){3,}|(?:-[ \t]*){3,}|(?:_[ \t]*){3,})$`)
	mdFenceRe   = regexp.MustCompile("^( {0,3})(`{3,}|~{3,})[ \t]*([^`]*)$")
	mdBulletRe  = regexp.MustCompile(`^( *)([-*+])(?:[ \t]+|$)`)
	mdOrderedRe = regexp.MustCompile(`^( *)(\d{1,9})([.)])(?:[ \t]+|$)`)
	mdQuoteRe   = regexp.MustCompile(`^ {0,3}> ?`)
	mdDelimRe   = regexp.MustCompile(`^ *\|? *:?-+:? *(?:\| *:?-+:? *)*\|? *$`)
	// mdSetextRe wants three marks, so a list item's "-" arriving alone mid-stream does not turn
	// the paragraph above into a heading for a frame.
	mdSetextRe = regexp.MustCompile(`^ {0,3}(={3,}|-{3,})[ \t]*$`)
)

// indentOf returns the number of leading spaces.
func indentOf(s string) int { return len(s) - len(strings.TrimLeft(s, " ")) }

func blank(s string) bool { return strings.TrimSpace(s) == "" }

// listMarker reports a list item marker at the start of line: its indent, the indent of its
// content, whether it is ordered, its number and its delimiter character.
func listMarker(line string) (indent, content int, ordered bool, num int, delim byte, ok bool) {
	if m := mdBulletRe.FindStringSubmatch(line); m != nil && !mdRuleRe.MatchString(line) {
		return len(m[1]), len(m[0]), false, 0, m[2][0], true
	}
	if m := mdOrderedRe.FindStringSubmatch(line); m != nil {
		n, _ := strconv.Atoi(m[2])
		return len(m[1]), len(m[0]), true, n, m[3][0], true
	}
	return 0, 0, false, 0, 0, false
}

// startsBlock reports whether line begins a block that interrupts a paragraph: a heading, rule,
// fence, quote or a list item with content.
func startsBlock(line string) bool {
	if mdHeadingRe.MatchString(line) || mdRuleRe.MatchString(line) || mdFenceRe.MatchString(line) || mdQuoteRe.MatchString(line) {
		return true
	}
	_, content, _, _, _, ok := listMarker(line)
	return ok && !blank(line[content:])
}

// parseBlocks parses lines, nested depth deep, into blocks.
func parseBlocks(lines []string, depth int) []mdBlock {
	if depth >= mdMaxDepth {
		var text []string
		for _, l := range lines {
			if !blank(l) {
				text = append(text, l)
			}
		}
		if len(text) == 0 {
			return nil
		}
		return []mdBlock{{kind: mdPara, inl: parseInline(joinPara(text))}}
	}
	var out []mdBlock
	for i := 0; i < len(lines); {
		line := lines[i]
		switch {
		case blank(line):
			i++
		case mdFenceRe.MatchString(line):
			b, n := parseFence(lines[i:])
			out = append(out, b)
			i += n
		case mdHeadingRe.MatchString(line):
			m := mdHeadingRe.FindStringSubmatch(line)
			out = append(out, mdBlock{kind: mdHeading, level: len(m[1]), inl: parseInline(m[2])})
			i++
		case mdRuleRe.MatchString(line):
			out = append(out, mdBlock{kind: mdRule})
			i++
		case mdQuoteRe.MatchString(line):
			var sub []string
			for i < len(lines) && !blank(lines[i]) {
				l := lines[i]
				if loc := mdQuoteRe.FindStringIndex(l); loc != nil {
					l = l[loc[1]:]
				} else if startsBlock(l) {
					break
				}
				sub = append(sub, l)
				i++
			}
			out = append(out, mdBlock{kind: mdQuote, sub: parseBlocks(sub, depth+1)})
		default:
			if _, _, _, _, _, ok := listMarker(line); ok {
				b, n := parseList(lines[i:], depth)
				out = append(out, b)
				i += n
				continue
			}
			if i+1 < len(lines) && strings.Contains(line, "|") && mdDelimRe.MatchString(lines[i+1]) && strings.Contains(lines[i+1], "-") {
				b, n := parseTable(lines[i:])
				out = append(out, b)
				i += n
				continue
			}
			var para []string
			for i < len(lines) && !blank(lines[i]) {
				if len(para) > 0 {
					if m := mdSetextRe.FindStringSubmatch(lines[i]); m != nil {
						lvl := 1
						if m[1][0] == '-' {
							lvl = 2
						}
						out = append(out, mdBlock{kind: mdHeading, level: lvl, inl: parseInline(joinPara(para))})
						para = nil
						i++
						break
					}
					if startsBlock(lines[i]) {
						break
					}
				}
				para = append(para, lines[i])
				i++
			}
			if len(para) > 0 {
				out = append(out, mdBlock{kind: mdPara, inl: parseInline(joinPara(para))})
			}
		}
	}
	return out
}

// joinPara joins paragraph lines: a line ending in two spaces or a backslash is a hard break,
// any other line end a space.
func joinPara(lines []string) string {
	var sb strings.Builder
	for i, l := range lines {
		t := strings.TrimLeft(l, " ")
		hard := strings.HasSuffix(t, "  ") || strings.HasSuffix(t, "\\")
		t = strings.TrimRight(t, " ")
		if hard {
			t = strings.TrimSuffix(t, "\\")
		}
		sb.WriteString(t)
		if i < len(lines)-1 {
			if hard {
				sb.WriteByte('\n')
			} else {
				sb.WriteByte(' ')
			}
		}
	}
	return sb.String()
}

// parseFence parses a fenced code block at lines[0]; without a closing fence it runs to the end
// and is open.
func parseFence(lines []string) (mdBlock, int) {
	m := mdFenceRe.FindStringSubmatch(lines[0])
	ind, fence := len(m[1]), m[2]
	b := mdBlock{kind: mdCode, lang: strings.TrimSpace(m[3]), open: true}
	var code []string
	i := 1
	for ; i < len(lines); i++ {
		l := lines[i]
		t := strings.TrimSpace(l)
		if indentOf(l) < 4 && strings.HasPrefix(t, fence[:1]) && strings.Trim(t, fence[:1]) == "" && len(t) >= len(fence) {
			b.open = false
			i++
			break
		}
		cut := min(ind, indentOf(l))
		code = append(code, l[cut:])
	}
	b.code = strings.Join(code, "\n")
	if b.open {
		b.code = strings.TrimRight(b.code, "\n")
	}
	return b, i
}

// parseList parses the list starting at lines[0], with nested blocks per item.
func parseList(lines []string, depth int) (mdBlock, int) {
	ind0, _, ordered, num, delim, _ := listMarker(lines[0])
	b := mdBlock{kind: mdList, ordered: ordered, start: num}
	i := 0
	for i < len(lines) {
		ind, content, ord, _, d, ok := listMarker(lines[i])
		if !ok || ord != ordered || d != delim || ind > ind0+1 || ind < ind0 {
			break
		}
		item := []string{lines[i][content:]}
		i++
		for i < len(lines) {
			l := lines[i]
			if blank(l) {
				// A blank line continues the item only if indented content follows.
				j := i
				for j < len(lines) && blank(lines[j]) {
					j++
				}
				if j < len(lines) && indentOf(lines[j]) > ind {
					for ; i < j; i++ {
						item = append(item, "")
					}
					continue
				}
				break
			}
			li := indentOf(l)
			if li > ind {
				item = append(item, l[min(li, content):])
				i++
				continue
			}
			// Lazy continuation of the item's paragraph.
			if !startsBlock(l) && !blank(item[len(item)-1]) {
				item = append(item, l)
				i++
				continue
			}
			break
		}
		b.items = append(b.items, parseBlocks(item, depth+1))
		// Skip blank lines between items of the same list.
		j := i
		for j < len(lines) && blank(lines[j]) {
			j++
		}
		if j > i && j < len(lines) {
			if ind2, _, ord2, _, d2, ok := listMarker(lines[j]); ok && ord2 == ordered && d2 == delim && ind2 >= ind0 && ind2 <= ind0+1 {
				i = j
			}
		}
	}
	return b, i
}

// splitRow splits a table row into its cells.
func splitRow(line string) []string {
	t := strings.TrimSpace(line)
	t = strings.TrimPrefix(t, "|")
	if strings.HasSuffix(t, "|") && !strings.HasSuffix(t, "\\|") {
		t = t[:len(t)-1]
	}
	var cells []string
	var sb strings.Builder
	code := false
	for k := 0; k < len(t); k++ {
		c := t[k]
		switch {
		case c == '\\' && k+1 < len(t) && t[k+1] == '|':
			sb.WriteByte('|')
			k++
		case c == '`':
			code = !code
			sb.WriteByte(c)
		case c == '|' && !code:
			cells = append(cells, strings.TrimSpace(sb.String()))
			sb.Reset()
		default:
			sb.WriteByte(c)
		}
	}
	return append(cells, strings.TrimSpace(sb.String()))
}

// parseTable parses a table whose delimiter row is lines[1].
func parseTable(lines []string) (mdBlock, int) {
	b := mdBlock{kind: mdTable}
	for _, c := range splitRow(lines[0]) {
		b.head = append(b.head, parseInline(c))
	}
	for _, c := range splitRow(lines[1]) {
		var a byte
		switch l, r := strings.HasPrefix(c, ":"), strings.HasSuffix(c, ":"); {
		case l && r:
			a = 'c'
		case r:
			a = 'r'
		case l:
			a = 'l'
		}
		b.align = append(b.align, a)
	}
	i := 2
	for ; i < len(lines) && !blank(lines[i]) && strings.Contains(lines[i], "|"); i++ {
		var row [][]mdSpan
		for k, c := range splitRow(lines[i]) {
			if k >= len(b.head) {
				break
			}
			row = append(row, parseInline(c))
		}
		for len(row) < len(b.head) {
			row = append(row, nil)
		}
		b.rows = append(b.rows, row)
	}
	return b, i
}

// inline parsing ----------------------------------------------------------------------------

// mdStyle is the style inline parsing applies to the text it emits.
type mdStyle struct {
	bold, italic, strike bool
	link                 string
}

// parseInline parses inline Markdown: code spans, emphasis, strikethrough, links, autolinks,
// bare URLs, escapes and hard breaks ("\n").
func parseInline(s string) []mdSpan {
	var out []mdSpan
	inlineInto(&out, s, mdStyle{})
	return mergeSpans(out)
}

func emit(out *[]mdSpan, text string, st mdStyle, code bool) {
	if text == "" {
		return
	}
	*out = append(*out, mdSpan{text: text, bold: st.bold, italic: st.italic, strike: st.strike, code: code, link: st.link})
}

// mergeSpans joins adjacent spans of equal style.
func mergeSpans(in []mdSpan) []mdSpan {
	var out []mdSpan
	for i := 0; i < len(in); {
		span := in[i]
		end, size := i+1, len(span.text)
		for end < len(in) {
			next := in[end]
			if span.bold != next.bold || span.italic != next.italic || span.code != next.code ||
				span.link != next.link || span.strike != next.strike {
				break
			}
			size += len(next.text)
			end++
		}
		if end > i+1 {
			var text strings.Builder
			text.Grow(size)
			for _, next := range in[i:end] {
				text.WriteString(next.text)
			}
			span.text = text.String()
		}
		out = append(out, span)
		i = end
	}
	return out
}

func isPunct(c byte) bool { return strings.IndexByte("!\"#$%&'()*+,-./:;<=>?@[\\]^_`{|}~", c) >= 0 }

func isAlnumByte(s string, i int) bool {
	if i < 0 || i >= len(s) {
		return false
	}
	r, _ := utf8.DecodeRuneInString(s[i:])
	if i > 0 && !utf8.RuneStart(s[i]) {
		r, _ = utf8.DecodeLastRuneInString(s[:i+1])
	}
	return unicode.IsLetter(r) || unicode.IsDigit(r)
}

// closer finds the index of the delimiter delim closing an emphasis opened at i (whose opener is
// len(delim) long), or -1. A closer is not preceded by a space and, for "_", not followed by a
// letter or digit; code spans are skipped.
func closer(s string, from int, delim string) int {
	for j := from; j < len(s); j++ {
		switch {
		case s[j] == '\\':
			j++
		case s[j] == '`':
			n := runLen(s, j, '`')
			if k := strings.Index(s[j+n:], strings.Repeat("`", n)); k >= 0 {
				j += n + k + n - 1
			} else {
				j += n - 1
			}
		case strings.HasPrefix(s[j:], delim) && j > from && s[j-1] != ' ':
			if delim[0] == '_' && isAlnumByte(s, j+len(delim)) {
				continue
			}
			if len(delim) == 1 && runLen(s, j, delim[0]) > 1 {
				// "**" inside "*...*": let the outer pair decide only on a lone delimiter.
				if j+2 <= len(s) && runLen(s, j, delim[0])%2 == 0 {
					j += runLen(s, j, delim[0]) - 1
					continue
				}
			}
			return j
		}
	}
	return -1
}

func runLen(s string, i int, c byte) int {
	n := 0
	for i+n < len(s) && s[i+n] == c {
		n++
	}
	return n
}

// hasBidi reports whether s holds a bidirectional embedding, override or isolate control, which
// can make a URL display differently from where it leads.
func hasBidi(s string) bool {
	for _, r := range s {
		if (r >= '\u202A' && r <= '\u202E') || (r >= '\u2066' && r <= '\u2069') {
			return true
		}
	}
	return false
}

// bareURL returns the http(s) URL at the start of s, or "": it runs to a space or one of <>[]"
// and loses trailing punctuation and any ")" that closes no "(" inside it, so a URL wrapped in
// parentheses or ending a sentence comes out clean while "Foo_(bar)" keeps its pair. A URL with
// bidi controls is not a URL.
func bareURL(s string) string {
	if !strings.HasPrefix(s, "https://") && !strings.HasPrefix(s, "http://") {
		return ""
	}
	end := strings.IndexAny(s, " \t\n<>[]\"`")
	if end < 0 {
		end = len(s)
	}
	u := s[:end]
	extraParens := strings.Count(u, ")") - strings.Count(u, "(")
	for len(u) > 0 {
		c := u[len(u)-1]
		switch {
		case strings.IndexByte(".,;:!?'*_~", c) >= 0:
			u = u[:len(u)-1]
			continue
		case c == ')' && extraParens > 0:
			u = u[:len(u)-1]
			extraParens--
			continue
		}
		break
	}
	if strings.HasSuffix(u, "://") || hasBidi(u) {
		return ""
	}
	return u
}

func inlineInto(out *[]mdSpan, s string, st mdStyle) {
	// Remember suffix searches, including failures, so repeated unfinished links or autolinks
	// do not scan the remaining message again for each opener.
	positions := map[string]int{}
	find := func(from int, token string) int {
		if at, ok := positions[token]; ok && (at < 0 || at >= from) {
			return at
		}
		at := strings.Index(s[from:], token)
		if at >= 0 {
			at += from
		}
		positions[token] = at
		return at
	}
	// noCloser[delim] is the smallest search start that found no closer for delim; a later
	// opener, further right, cannot find one either, so each delimiter scans s at most once.
	noCloser := map[string]int{}
	findCloser := func(from int, delim string) int {
		if f, ok := noCloser[delim]; ok && from >= f {
			return -1
		}
		j := closer(s, from, delim)
		if j < 0 {
			noCloser[delim] = from
		}
		return j
	}
	var sb strings.Builder
	flush := func() { emit(out, sb.String(), st, false); sb.Reset() }
	for i := 0; i < len(s); {
		c := s[i]
		switch {
		case c == '\\' && i+1 < len(s) && isPunct(s[i+1]):
			sb.WriteByte(s[i+1])
			i += 2
			continue
		case c == '\n':
			flush()
			emit(out, "\n", st, false)
			i++
			continue
		case c == '`':
			n := runLen(s, i, '`')
			if close := find(i+n, strings.Repeat("`", n)); close >= 0 {
				code := s[i+n : close]
				if len(code) > 2 && code[0] == ' ' && code[len(code)-1] == ' ' && strings.TrimSpace(code) != "" {
					code = code[1 : len(code)-1]
				}
				flush()
				emit(out, code, st, true)
				i = close + n
				continue
			}
			sb.WriteString(s[i : i+n])
			i += n
			continue
		case c == '*' || c == '_' || c == '~':
			n := runLen(s, i, c)
			if c == '~' && n != 2 {
				break
			}
			if c == '_' && isAlnumByte(s, i-1) {
				sb.WriteString(s[i : i+n])
				i += n
				continue
			}
			if i+n < len(s) && s[i+n] != ' ' {
				// Try the strongest opener first: *** then ** then *.
				for k := min(n, 3); k >= 1; k-- {
					if c == '~' && k != 2 {
						continue
					}
					delim := strings.Repeat(string(c), k)
					if j := findCloser(i+k, delim); j > i+k {
						flush()
						in := st
						switch {
						case c == '~':
							in.strike = true
						case k == 3:
							in.bold, in.italic = true, true
						case k == 2:
							in.bold = true
						default:
							in.italic = true
						}
						sb.WriteString(s[i : i+n-k])
						flush()
						inlineInto(out, s[i+n:j], in)
						i = j + k
						goto next
					}
				}
			}
			sb.WriteString(s[i : i+n])
			i += n
			continue
		case c == '[':
			if textEnd := find(i, "]("); textEnd > i {
				if close := find(textEnd+2, ")"); close >= 0 {
					url := strings.TrimSpace(s[textEnd+2 : close])
					if url != "" && !strings.ContainsAny(url, " \n") && !hasBidi(url) {
						flush()
						in := st
						in.link = url
						inlineInto(out, s[i+1:textEnd], in)
						i = close + 1
						continue
					}
				}
			}
		case c == '<':
			if end := find(i, ">"); end > i {
				inner := s[i+1 : end]
				if bareURL(inner) == inner && inner != "" {
					flush()
					in := st
					in.link = inner
					emit(out, inner, in, false)
					i = end + 1
					continue
				}
			}
		case c == 'h' && st.link == "" && !isAlnumByte(s, i-1) && !strings.HasSuffix(s[:i], "]("):
			if u := bareURL(s[i:]); u != "" {
				flush()
				in := st
				in.link = u
				emit(out, u, in, false)
				i += len(u)
				continue
			}
		}
		sb.WriteByte(c)
		i++
	next:
	}
	flush()
}

// mdPlain returns the text of spans without markup.
func mdPlain(spans []mdSpan) string {
	var sb strings.Builder
	for _, s := range spans {
		sb.WriteString(s.text)
	}
	return sb.String()
}
