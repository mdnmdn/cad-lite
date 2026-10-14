package i18n

import "os"

// PlatformLang is an optional hook for platforms whose language is not in the environment
// (Windows, a macOS app started from Finder). cmd/ may set it before calling Detect; it is
// consulted only when none of LC_ALL, LC_MESSAGES and LANG is set. i18n itself reads the
// environment only, which keeps it dependency-free and wasm-safe.
var PlatformLang func() Lang

// Detect returns the language the environment asks for: the first non-empty of LC_ALL,
// LC_MESSAGES and LANG decides (POSIX precedence), so LC_ALL=C gives English even when LANG is
// Italian. An unsupported language gives English. With none of them set, PlatformLang is
// consulted; otherwise the result is English. On js/wasm there is no environment, so the result
// is English unless PlatformLang says otherwise.
func Detect() Lang { return DetectWith(os.Getenv) }

// DetectWith is Detect with an explicit environment lookup, for tests.
func DetectWith(getenv func(string) string) Lang {
	for _, name := range []string{"LC_ALL", "LC_MESSAGES", "LANG"} {
		if v := getenv(name); v != "" {
			if l, ok := ParseLang(v); ok {
				return l
			}
			return English
		}
	}
	if PlatformLang != nil {
		if l := PlatformLang(); l.Valid() {
			return l
		}
	}
	return English
}
