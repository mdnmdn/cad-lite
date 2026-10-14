package ui

import (
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"
)

// spanString renders spans compactly: *b*, /i/, `c`, [t](u), ~s~.
func spanString(spans []mdSpan) string {
	var sb strings.Builder
	for _, s := range spans {
		t := s.text
		if s.code {
			t = "`" + t + "`"
		}
		if s.italic {
			t = "/" + t + "/"
		}
		if s.bold {
			t = "*" + t + "*"
		}
		if s.strike {
			t = "~" + t + "~"
		}
		if s.link != "" {
			t = "[" + t + "](" + s.link + ")"
		}
		sb.WriteString(t)
	}
	return sb.String()
}

func TestMarkdownInline(t *testing.T) {
	for in, want := range map[string]string{
		"plain text":                        "plain text",
		"a **bold** b":                      "a *bold* b",
		"a __bold__ b":                      "a *bold* b",
		"an *italic* word":                  "an /italic/ word",
		"an _italic_ word":                  "an /italic/ word",
		"***both***":                        "*/both/*",
		"**bold *and italic* tail**":        "*bold *[/and italic/]* tail*",
		"use `x := 1` here":                 "use `x := 1` here",
		"``code with ` tick``":              "`code with ` tick`",
		"snake_case_name stays":             "snake_case_name stays",
		"2 * 3 * 4":                         "2 * 3 * 4",
		"~~gone~~ kept":                     "~gone~ kept",
		`\*not italic\*`:                    "*not italic*",
		"see [the docs](https://x.io/a) ok": "see [the docs](https://x.io/a) ok",
		"go to https://example.com/p.":      "go to [https://example.com/p](https://example.com/p).",
		"<https://a.b/c>":                   "[https://a.b/c](https://a.b/c)",
		"`**not bold**`":                    "`**not bold**`",
		"[**bold link**](u)":                "[*bold link*](u)",
	} {
		got := spanString(parseInline(in))
		// The nested-style case renders its inner span separately; normalise the marker.
		got = strings.ReplaceAll(got, "**/", "*[/")
		got = strings.ReplaceAll(got, "/**", "/]*")
		if in == "**bold *and italic* tail**" {
			spans := parseInline(in)
			if len(spans) != 3 || !spans[0].bold || spans[0].italic || !spans[1].bold || !spans[1].italic || spans[1].text != "and italic" {
				t.Errorf("%q = %+v", in, spans)
			}
			continue
		}
		if got != want {
			t.Errorf("%q = %q, want %q", in, got, want)
		}
	}
}

func TestMarkdownInlineStreamingPartials(t *testing.T) {
	// Unclosed markup while a message streams in stays literal text, never swallowed.
	for in, want := range map[string]string{
		"a **bold wor":        "a **bold wor",
		"an *ital":            "an *ital",
		"code `x := ":         "code `x := ",
		"a [link](https://ex": "a [link](https://ex",
		"a [link":             "a [link",
		"trailing *":          "trailing *",
	} {
		if got := spanString(parseInline(in)); got != want {
			t.Errorf("%q = %q, want %q", in, got, want)
		}
	}
}

func TestMarkdownBlocks(t *testing.T) {
	src := strings.Join([]string{
		"# Title",
		"Intro with **bold**",
		"and a second line.",
		"",
		"## Steps",
		"1. first",
		"2. second",
		"   - nested a",
		"   - nested b",
		"3. third",
		"",
		"- bullet",
		"  continued lazily",
		"",
		"> quoted **text**",
		"> more",
		"",
		"---",
		"",
		"```go",
		"func main() {",
		"    fmt.Println(1)",
		"}",
		"```",
		"",
		"| Layer | Colour |",
		"|:------|------:|",
		"| holes | red |",
		"| outline | `cyan` |",
		"",
		"Setext",
		"======",
	}, "\n")
	bs := parseMarkdown(src)
	kinds := []mdKind{mdHeading, mdPara, mdHeading, mdList, mdList, mdQuote, mdRule, mdCode, mdTable, mdHeading}
	if len(bs) != len(kinds) {
		t.Fatalf("got %d blocks: %+v", len(bs), bs)
	}
	for i, k := range kinds {
		if bs[i].kind != k {
			t.Fatalf("block %d kind %d, want %d", i, bs[i].kind, k)
		}
	}
	if bs[0].level != 1 || mdPlain(bs[0].inl) != "Title" || bs[2].level != 2 {
		t.Errorf("headings: %+v %+v", bs[0], bs[2])
	}
	if got := spanString(bs[1].inl); got != "Intro with *bold* and a second line." {
		t.Errorf("paragraph = %q", got)
	}
	ol := bs[3]
	if !ol.ordered || ol.start != 1 || len(ol.items) != 3 {
		t.Fatalf("ordered list: %+v", ol)
	}
	if n := ol.items[1]; len(n) != 2 || n[1].kind != mdList || len(n[1].items) != 2 || n[1].ordered {
		t.Fatalf("nested list under item 2: %+v", n)
	}
	if ul := bs[4]; ul.ordered || len(ul.items) != 1 || mdPlain(ul.items[0][0].inl) != "bullet continued lazily" {
		t.Errorf("bullet list: %+v", ul)
	}
	if q := bs[5]; len(q.sub) != 1 || spanString(q.sub[0].inl) != "quoted *text* more" {
		t.Errorf("quote: %+v", q.sub)
	}
	if c := bs[7]; c.lang != "go" || c.open || c.code != "func main() {\n    fmt.Println(1)\n}" {
		t.Errorf("code block: %+v", c)
	}
	tb := bs[8]
	if len(tb.head) != 2 || len(tb.rows) != 2 || string(tb.align) != "lr" || spanString(tb.rows[1][1]) != "`cyan`" {
		t.Errorf("table: %+v", tb)
	}
	if bs[9].level != 1 || mdPlain(bs[9].inl) != "Setext" {
		t.Errorf("setext heading: %+v", bs[9])
	}
}

func TestMarkdownBlocksStreamingPartials(t *testing.T) {
	// An unclosed fence is an open code block holding what arrived so far.
	bs := parseMarkdown("Here:\n```python\nprint(1)\nprint(")
	if len(bs) != 2 || bs[1].kind != mdCode || !bs[1].open || bs[1].code != "print(1)\nprint(" || bs[1].lang != "python" {
		t.Fatalf("open fence: %+v", bs)
	}
	// A fence that just opened is an empty open block, not text.
	if bs := parseMarkdown("```"); len(bs) != 1 || bs[0].kind != mdCode || !bs[0].open || bs[0].code != "" {
		t.Fatalf("bare fence: %+v", bs)
	}
	// A table whose delimiter row has not arrived is a paragraph for now.
	if bs := parseMarkdown("| a | b |"); len(bs) != 1 || bs[0].kind != mdPara {
		t.Fatalf("table header alone: %+v", bs)
	}
	// A partial delimiter row with a dash already makes the table, without body rows.
	if bs := parseMarkdown("| a | b |\n|--"); len(bs) != 1 || bs[0].kind != mdTable || len(bs[0].rows) != 0 {
		t.Fatalf("partial delimiter: %+v", bs)
	}
	// A list marker with nothing after it yet does not break the paragraph above.
	if bs := parseMarkdown("text\n-"); len(bs) != 1 || bs[0].kind != mdPara {
		t.Fatalf("dangling marker: %+v", bs)
	}
	// CRLF and tabs are normalised.
	if bs := parseMarkdown("- a\r\n\t- b"); len(bs) != 1 || len(bs[0].items) != 1 || len(bs[0].items[0]) != 2 {
		t.Fatalf("crlf/tab nesting: %+v", bs)
	}
}

func TestMarkdownListEdgeCases(t *testing.T) {
	// Loose items (blank lines between) stay one list; a different marker type ends it.
	bs := parseMarkdown("- a\n\n- b\n\n1. c")
	if len(bs) != 2 || len(bs[0].items) != 2 || !bs[1].ordered {
		t.Fatalf("loose list then ordered: %+v", bs)
	}
	// A rule made of dashes is not a list.
	if bs := parseMarkdown("- - -"); len(bs) != 1 || bs[0].kind != mdRule {
		t.Fatalf("rule: %+v", bs)
	}
	// An ordered list keeps its start number.
	if bs := parseMarkdown("3) x\n4) y"); bs[0].start != 3 || len(bs[0].items) != 2 {
		t.Fatalf("start: %+v", bs)
	}
	// A code block inside a list item.
	bs = parseMarkdown("1. run:\n   ```sh\n   ls -l\n   ```\n2. done")
	if len(bs) != 1 || len(bs[0].items) != 2 || len(bs[0].items[0]) != 2 || bs[0].items[0][1].kind != mdCode || bs[0].items[0][1].code != "ls -l" {
		t.Fatalf("fence in item: %+v", bs)
	}
}

func TestMarkdownMatchedRunsStayBounded(t *testing.T) {
	const count = 50000
	for _, tc := range []struct {
		name, token string
		style       mdSpan
	}{
		{"code", "`a`", mdSpan{code: true}},
		{"link", "[a](u)", mdSpan{link: "u"}},
		{"bold", "**a**", mdSpan{bold: true}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			input := strings.Repeat(tc.token, count)
			start := time.Now()
			got := parseInline(input)
			elapsed := time.Since(start)
			want := tc.style
			want.text = strings.Repeat("a", count)
			if len(got) != 1 || got[0] != want {
				t.Fatalf("matched run changed content/style: %d spans", len(got))
			}
			if elapsed > 50*time.Millisecond {
				t.Fatalf("%d matched spans took %v", count, elapsed)
			}
		})
	}
}

func TestMarkdownURLTrailingParensStayBounded(t *testing.T) {
	input := "https://a.b/" + strings.Repeat(")", 50000)
	start := time.Now()
	got := bareURL(input)
	elapsed := time.Since(start)
	if got != "https://a.b/" {
		t.Fatalf("trimmed URL = %q", got)
	}
	if elapsed > 50*time.Millisecond {
		t.Fatalf("trimming unmatched parentheses took %v", elapsed)
	}
	if got := bareURL("https://a.b/Foo_(bar)))!"); got != "https://a.b/Foo_(bar)" {
		t.Fatalf("balanced URL = %q", got)
	}
}

func BenchmarkMarkdownMatchedRuns(b *testing.B) {
	for _, token := range []string{"`a`", "[a](u)", "**a**"} {
		for _, count := range []int{1000, 10000, 50000} {
			b.Run(fmt.Sprintf("%s/%d", token, count), func(b *testing.B) {
				text := strings.Repeat(token, count)
				b.SetBytes(int64(len(text)))
				b.ReportAllocs()
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					parseInline(text)
				}
			})
		}
	}
}

func BenchmarkMarkdownURLTrailingParens(b *testing.B) {
	for _, count := range []int{1000, 10000, 50000} {
		b.Run(fmt.Sprint(count), func(b *testing.B) {
			text := "https://a.b/" + strings.Repeat(")", count)
			b.SetBytes(int64(len(text)))
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				bareURL(text)
			}
		})
	}
}

func TestMarkdownMergePreservesStyleBoundaries(t *testing.T) {
	input := []mdSpan{
		{text: "a"}, {text: "b"},
		{text: "c", bold: true}, {text: "d", bold: true},
		{text: "e", italic: true}, {text: "f", code: true},
		{text: "g", strike: true}, {text: "h", link: "u"},
		{text: "i", link: "v"}, {text: "j"},
	}
	want := []mdSpan{
		{text: "ab"}, {text: "cd", bold: true},
		{text: "e", italic: true}, {text: "f", code: true},
		{text: "g", strike: true}, {text: "h", link: "u"},
		{text: "i", link: "v"}, {text: "j"},
	}
	if got := mergeSpans(input); !slices.Equal(got, want) {
		t.Fatalf("merged style boundaries = %+v, want %+v", got, want)
	}
}
