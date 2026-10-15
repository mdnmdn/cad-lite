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

// shortcutOf returns the capital letters of kw, or its upper-cased first letter. A keyword that
// starts with a digit ("3P", "2P") is its own shortcut: its capitals alone would be the same
// letter for both.
func shortcutOf(kw string) string {
	if r := []rune(kw); len(r) > 0 && unicode.IsDigit(r[0]) {
		return strings.ToUpper(kw)
	}
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
func KeywordLabel(id string) string { return For(Current()).KeywordLabel(id) }

// KeywordLabel returns keyword id as the translator's language displays it.
func (t Translator) KeywordLabel(id string) string { return keywordIn(t.lang, id).label }

// KeywordShortcut returns the upper-case shortcut of keyword id in the current language.
func KeywordShortcut(id string) string { return keywordIn(Current(), id).shortcut }

// PromptKeywords renders the bracket content of a prompt: "Chiudi/Annulla".
func PromptKeywords(ids []string) string { return For(Current()).PromptKeywords(ids) }

// PromptKeywords renders the bracket content of a prompt in the translator's language.
func (t Translator) PromptKeywords(ids []string) string {
	l := t.lang
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
	return For(Current()).MatchKeywords(ids, text)
}

// MatchKeywords resolves typed text against a prompt's keyword ids in the translator's language,
// then English; see the package-level MatchKeywords. An English translator (an agent) matches
// English only.
func (tr Translator) MatchKeywords(ids []string, text string) (string, bool) {
	t := strings.TrimSpace(text)
	t = strings.TrimPrefix(t, "'")
	english := false
	if strings.HasPrefix(t, "_") {
		english, t = true, t[1:]
	}
	if t == "" {
		return "", false
	}
	pass := []Lang{tr.lang, English}
	if english || tr.lang == English {
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
	"GROUP operations":      {"List", "REname", "Add", "Remove", "Selectable", "Delete"},
	"ORTHO mode":            {"ON", "OFF"},
	"GRID option":           {"ON", "OFF", "Snap", "Major", "Aspect"},
	"SNAP option":           {"ON", "OFF", "Aspect", "Style", "Type"},
	"SNAP style":            {"Standard", "Isometric"},
	"SNAP type":             {"Polar", "Grid"},
	"LINE/PLINE next point": {"Close", "Undo"},
	"LINE/PLINE (no close)": {"Undo"},
	"TRIM standard":         {"Fence", "Crossing", "Project", "Edge", "eRase", "Undo"},
	"TRIM quick":            {"cuTting edges", "Crossing", "mOde", "Project", "eRase", "Undo"},
	"EXTEND standard":       {"Fence", "Crossing", "Project", "Edge", "Undo"},
	"EXTEND quick":          {"Boundary edges", "Crossing", "mOde", "Project", "eRase", "Undo"},
	"TRIM/EXTEND edge mode": {"Extend", "No extend"},
	"TRIM/EXTEND project":   {"None", "Ucs", "View"},
	"TRIM/EXTEND mode":      {"Quick", "Standard"},
	"CIRCLE radius":         {"Diameter"},
	"CIRCLE center":         {"3P", "2P", "Ttr"},
	"PLINE line mode":       {"Arc", "Close", "Halfwidth", "Length", "Undo", "Width"},
	"PLINE arc mode":        {"Arc angle", "Pline center", "Close arc", "Direction", "Halfwidth", "Line", "Radius", "Second pt", "Undo", "Width"},
	"PLINE arc angle end":   {"Pline center", "Radius"},
	"PLINE arc radius end":  {"Arc angle"},
	"PLINE arc center end":  {"Arc angle", "Length"},
	"ARC start":             {"Center"},
	"ARC second point":      {"Center", "End"},
	"ARC center end":        {"Angle", "chord Length"},
	"ARC end center":        {"Angle", "Direction", "Radius"},
	"RECTANG first corner":  {"Chamfer", "Elevation", "Fillet", "Thickness", "Width"},
	"RECTANG other corner":  {"Rect area", "Dimensions", "Rotation"},
	"RECTANG area basis":    {"Length", "Width"},
	"RECTANG rotation":      {"Pick points"},
	"POLYGON center":        {"Edge"},
	"POLYGON option":        {"Inscribed in circle", "Circumscribed about circle"},
	"TEXT start":            {"Justify", "Style"},
	"TEXT justify":          {"Left", "Center", "Right", "Align", "Middle", "Fit", "TL", "TC", "TR", "ML", "MC", "MR", "BL", "BC", "BR"},
	"TEXT/MTEXT style":      {"?"},
	"MTEXT corner":          {"Height", "Justify", "Rotation", "Style", "Width"},
	"MTEXT justify":         {"TL", "TC", "TR", "ML", "MC", "MR", "BL", "BC", "BR"},
	"MIRROR erase source":   {"Yes", "No"},
	"OFFSET distance":       {"Through", "Erase offset", "Layer"},
	"OFFSET layer":          {"Current", "Source"},
	"OFFSET select":         {"Exit", "Undo"},
	"OFFSET side":           {"Exit", "Multiple", "Undo"},
	"FILLET first":          {"Undo", "Polyline", "Radius", "Trim", "Multiple"},
	"FILLET second":         {"Radius"},
	"CHAMFER first":         {"Undo", "Polyline", "Distance", "Chamfer angle", "Trim", "Method", "Multiple"},
	"CHAMFER second":        {"Distance", "Chamfer angle", "Method"},
	"FILLET/CHAMFER trim":   {"Trim", "No trim"},
	"CHAMFER method":        {"Distance", "Chamfer angle"},
	"ZOOM corner":           {"zAll", "Center", "Extents", "Previous", "Scale", "Window", "Object"},
	"-LAYER option":         {"?", "Make", "Set", "New", "ON", "OFF", "Color", "Ltype", "LWeight", "Plot", "Freeze", "Thaw", "LOck", "Unlock", "Description"},
	"-LAYER plot":           {"Plot", "No plot"},
	"-LAYER off current":    {"Yes", "No"},
	"MATCHPROP dest":        {"Settings"},
	"MATCHPROP/CHPROP prop": {"Color", "chLayer"},
	"PURGE type":            {"LAyers", "Zero-length geometry", "zAll"},
	"PURGE verify":          {"Yes", "No"},
	"AREA first corner":     {"Object", "Add area", "Subtract area"},
	"AREA add mode":         {"Object", "Subtract area", "Exit"},
	"AREA subtract mode":    {"Object", "Add area", "Exit"},
	"AREA measure":          {"Object", "Add area", "Subtract area", "Exit"},
	"DIST first point":      {"Multiple points"},
	"BREAK second point":    {"First point"},
	"TRIM select":           {"cuTting edges"},
	"MEASUREGEOM option":    {"Distance", "Radius", "Angle", "ARea", "Exit"},
	"MOVE/COPY base point":  {"Displacement"},
	"COPY base point":       {"Displacement", "mOde"},
	"COPY mode":             {"Single mode", "Multiple"},
	"STRETCH select":        {"Crossing", "CPolygon"},
	"COPY second point":     {"Array"},
	"COPY repeat":           {"Array", "Exit", "Undo"},
	"COPY array end":        {"Fit"},
	"ROTATE/SCALE value":    {"Copy", "Reference"},
	"ROTATE/SCALE new":      {"Points"},
	"ARRAY type":            {"Rectangular", "PAth", "Polar"},
	"-ARRAY type":           {"Rectangular", "Polar"},
	"ARRAY polar center":    {"Base"},
	"UNDO option":           {"Auto", "Control", "BEgin", "End", "Mark", "Back"},
	"UNDO auto":             {"ON", "OFF"},
	"UNDO control":          {"All", "None", "One", "Combine", "Layer"},
	"UNDO back":             {"Yes", "No"},
	"MREDO count":           {"All", "Last"},
	"Save changes":          {"Yes", "No", "Cancel"},
}
