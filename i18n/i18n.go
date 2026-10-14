// Package i18n is Cad-lite's internationalisation foundation: English and Italian message
// catalogs, language selection and detection, plurals, localized command-line keywords and the
// per-command metadata (display name, description, tooltip, typed aliases).
//
// It is a leaf package: pure Go, no imports from the rest of Cad-lite, no ebiten or gpui, and it
// builds for GOOS=js. cmdsys, cadmcp, fileio, settings and ui may all import it.
//
// # Keys and placeholders
//
// A key is a dotted path such as "ui.status.grid" or "cmd.trim.msg.no_intersection". Values use
// fmt verbs: T("cmd.layer.msg.created", name) formats with fmt.Sprintf. When a language needs a
// different word order it uses explicit argument indexes (%[2]s before %[1]s); the catalogs must
// agree on the verb of every argument, which Verify checks. A value used with no arguments is
// returned exactly as written (no %% processing), so write a literal percent sign as % there and as
// %% in a value that is used with arguments.
//
// A missing key falls back to the English catalog, then to the key itself, so a gap is visible
// and never crashes. Set MissingHandler to be told about gaps (tests do).
//
// # Concurrency
//
// The current language is guarded by a mutex; T and the other helpers may be called from any
// goroutine. Catalogs are immutable after init.
package i18n

import (
	"fmt"
	"strings"
	"sync"
)

// Lang is a supported UI language, an ISO 639-1 code.
type Lang string

const (
	// English is the source language and the fallback for every missing key.
	English Lang = "en"
	// Italian is the second shipped language.
	Italian Lang = "it"
)

// Langs lists the shipped languages, English first.
var Langs = []Lang{English, Italian}

// Valid reports whether l is a shipped language.
func (l Lang) Valid() bool {
	for _, x := range Langs {
		if x == l {
			return true
		}
	}
	return false
}

// Name returns the language's own name, for a language picker ("English", "Italiano").
func (l Lang) Name() string {
	switch l {
	case Italian:
		return "Italiano"
	case English:
		return "English"
	}
	return string(l)
}

// ParseLang interprets a locale string such as "it", "it_IT.UTF-8", "it-IT", "it_IT@euro" or
// "English". It reports false for an empty string, "auto", "C", "POSIX" and unsupported
// languages.
func ParseLang(s string) (Lang, bool) {
	s = strings.ToLower(strings.TrimSpace(s))
	if i := strings.IndexAny(s, ".@"); i >= 0 {
		s = s[:i]
	}
	if i := strings.IndexAny(s, "_-:"); i >= 0 {
		s = s[:i]
	}
	switch s {
	case "en", "english":
		return English, true
	case "it", "italiano", "italian":
		return Italian, true
	}
	return "", false
}

var (
	mu  sync.RWMutex
	cur = English
)

// MissingHandler, when non-nil, is called for every key that is missing from lang's catalog
// (before the English fallback). Set it once at start-up; it is not synchronised.
var MissingHandler func(lang Lang, key string)

// Set makes l the current language; an unsupported value selects English.
func Set(l Lang) {
	if !l.Valid() {
		l = English
	}
	mu.Lock()
	cur = l
	mu.Unlock()
}

// Current returns the current language.
func Current() Lang {
	mu.RLock()
	defer mu.RUnlock()
	return cur
}

// FromSetting resolves a stored preference: "", "auto" or an unknown value means Detect(); a
// supported code selects that language. The settings panel stores "auto", "en" or "it".
func FromSetting(s string) Lang {
	if l, ok := ParseLang(s); ok {
		return l
	}
	return Detect()
}

// Translator is a language-bound view of the catalogs, for code that must not follow the global
// language, e.g. text meant for an AI agent (use For(English)).
type Translator struct{ lang Lang }

// For returns the Translator for l (English when l is unsupported).
func For(l Lang) Translator {
	if !l.Valid() {
		l = English
	}
	return Translator{lang: l}
}

// Lang returns the translator's language.
func (t Translator) Lang() Lang { return t.lang }

// lookup finds key for lang, falling back to English.
func lookup(lang Lang, key string) (string, bool) {
	if s, ok := catalogs[lang].msgs[key]; ok {
		return s, true
	}
	if MissingHandler != nil {
		MissingHandler(lang, key)
	}
	if lang != English {
		if s, ok := catalogs[English].msgs[key]; ok {
			return s, true
		}
	}
	return "", false
}

// T returns the message for key in the translator's language, formatted with args when there are
// any. A missing key yields the English text, then the key itself.
func (t Translator) T(key string, args ...any) string {
	s, ok := lookup(t.lang, key)
	if !ok {
		return key
	}
	if len(args) == 0 {
		return s
	}
	return fmt.Sprintf(s, args...)
}

// Plural picks the "<key>_one" form when n is 1 and "<key>_other" otherwise (the rule English and
// Italian share; 0 and fractions use _other) and formats it with n as the first argument followed
// by args: Plural("cmd.join.msg.joined", 3) yields "3 objects joined.".
func (t Translator) Plural(key string, n int, args ...any) string {
	form := "_other"
	if n == 1 {
		form = "_one"
	}
	return t.T(key+form, append([]any{n}, args...)...)
}

// List returns the string array stored under key (e.g. "cmd.line.aliases"), or nil.
func (t Translator) List(key string) []string { return catalogs[t.lang].lists[key] }

// T translates key in the current language; see Translator.T.
func T(key string, args ...any) string { return For(Current()).T(key, args...) }

// Plural translates a counted message in the current language; see Translator.Plural.
func Plural(key string, n int, args ...any) string { return For(Current()).Plural(key, n, args...) }

// List returns the current language's string array for key; see Translator.List.
func List(key string) []string { return For(Current()).List(key) }

// Has reports whether lang's own catalog (without fallback) defines key.
func Has(lang Lang, key string) bool {
	_, ok := catalogs[lang].msgs[key]
	return ok
}

// Keys returns lang's message keys (not the arrays), unsorted.
func Keys(lang Lang) []string {
	out := make([]string, 0, len(catalogs[lang].msgs))
	for k := range catalogs[lang].msgs {
		out = append(out, k)
	}
	return out
}

// Raw returns lang's unformatted message for key, without fallback.
func Raw(lang Lang, key string) (string, bool) {
	s, ok := catalogs[lang].msgs[key]
	return s, ok
}
