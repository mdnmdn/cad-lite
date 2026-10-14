package i18n

import (
	"strings"
	"unicode"
)

// Keywords are the bracketed options of a prompt: "Specify next point or [Close/Undo]:".
//
// Design. A keyword has a canonical id, its English spelling exactly as the command code uses it
// today ("Close", "cuTting edges", "ARea"). Commands keep comparing in.Keyword to that id and
// need no change. What changes is display and matching, both of which go through this file:
//
//   - KeywordLabel(id) is what the prompt shows: "Chiudi" in Italian. Capital letters mark the
//     shortcut ("ARea" shortcut AR), as in English; with no capitals the shortcut is the first
//     letter.
//   - MatchKeywords(ids, text) is what the parser calls: it accepts the localized full name, its
//     synonyms and its shortcut first, then the English full name and shortcut. A leading "_"
//     (AutoCAD's "_Close", also "'_") means English only. Matching is case-insensitive.
//
// The data lives in the [kw] table of each catalog: kw.<slug> = "Display|synonym|synonym" where
// slug is the id lower-cased with every other character turned into "_". A keyword missing from a
// catalog falls back to English, then to the id itself, so commands can add keywords before
// they are translated.
//
// Within one prompt the shortcuts and names of the keywords must not collide in any language;
// KeywordGroups lists every prompt's keyword set and Verify checks it.

type kwEntry struct {
	label    string   // display text, capitals mark the shortcut
	shortcut string   // upper-case
	names    []string // lower-case full names that match: label and synonyms
}

var keywords = map[Lang]map[string]kwEntry{}

func buildKeywords() {
	for _, l := range Langs {
		m := map[string]kwEntry{}
		for k, v := range catalogs[l].msgs {
			if s, ok := strings.CutPrefix(k, "kw."); ok {
				m[s] = parseKeyword(v)
			}
		}
		keywords[l] = m
	}
}

func parseKeyword(v string) kwEntry {
	parts := strings.Split(v, "|")
	e := kwEntry{label: parts[0], shortcut: shortcutOf(parts[0])}
	for _, p := range parts {
		e.names = append(e.names, strings.ToLower(p))
	}
	return e
}

// shortcutOf returns the capital letters of kw, or its upper-cased first letter.
func shortcutOf(kw string) string {
	var b strings.Builder
	for _, r := range kw {
		if unicode.IsUpper(r) {
			b.WriteRune(r)
		}
	}
	if b.Len() == 0 && kw != "" {
		r := []rune(kw)[0]
		return string(unicode.ToUpper(r))
	}
	return b.String()
}

func keywordIn(l Lang, id string) kwEntry {
	if e, ok := keywords[l][slug(id)]; ok {
		return e
	}
	if e, ok := keywords[English][slug(id)]; ok && l != English {
		return e
	}
	return parseKeyword(id)
}

// KeywordLabel returns keyword id as the current language displays it, with the shortcut letters
// capitalised: KeywordLabel("Close") is "Chiudi" in Italian.
func KeywordLabel(id string) string { return keywordIn(Current(), id).label }

// KeywordShortcut returns the upper-case shortcut of keyword id in the current language.
func KeywordShortcut(id string) string { return keywordIn(Current(), id).shortcut }

// PromptKeywords renders the bracket content of a prompt: "Chiudi/Annulla".
func PromptKeywords(ids []string) string {
	l := Current()
	labels := make([]string, len(ids))
	for i, id := range ids {
		labels[i] = keywordIn(l, id).label
	}
	return strings.Join(labels, "/")
}

// MatchKeywords resolves what the user typed against a prompt's keyword ids and returns the
// matching id. The current language is tried first (full name, synonyms, shortcut), then English;
// "_" before the text restricts it to English.
func MatchKeywords(ids []string, text string) (string, bool) {
	t := strings.TrimSpace(text)
	t = strings.TrimPrefix(t, "'")
	english := false
	if strings.HasPrefix(t, "_") {
		english, t = true, t[1:]
	}
	if t == "" {
		return "", false
	}
	pass := []Lang{Current(), English}
	if english || Current() == English {
		pass = []Lang{English}
	}
	for _, l := range pass {
		for _, id := range ids {
			if keywordMatches(keywordIn(l, id), t) {
				return id, true
			}
		}
	}
	return "", false
}

func keywordMatches(e kwEntry, t string) bool {
	if strings.EqualFold(e.shortcut, t) {
		return true
	}
	for _, n := range e.names {
		if strings.EqualFold(n, t) {
			return true
		}
	}
	return false
}

// KeywordGroups lists, for every prompt that has keywords, the keyword ids it offers. Verify
// proves each group conflict-free in every language; add a group when a command gains a keyword
// prompt.
var KeywordGroups = map[string][]string{
	"LINE/PLINE next point": {"Close", "Undo"},
	"LINE/PLINE (no close)": {"Undo"},
	"CIRCLE radius":         {"Diameter"},
	"MIRROR erase source":   {"Yes", "No"},
	"OFFSET distance":       {"Through"},
	"OFFSET select/side":    {"Exit"},
	"FILLET first":          {"Radius"},
	"CHAMFER first":         {"Distance"},
	"ZOOM corner":           {"Extents", "Window"},
	"AREA first corner":     {"Object"},
	"DIST first point":      {"Multiple points"},
	"BREAK second point":    {"First point"},
	"TRIM select":           {"cuTting edges"},
	"MEASUREGEOM option":    {"Distance", "Radius", "Angle", "ARea"},
}
