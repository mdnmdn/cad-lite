package cmdsys

import "strings"

// commandPrefixes are the characters AutoCAD accepts in front of a command name: ' (transparent),
// _ (the global, English name: names are English here whatever the UI language, so it changes
// nothing else) and . (the built-in command, bypassing a redefinition; go-draw has no UNDEFINE).
// A leading - is part of the name (-LAYER is its own command).
const commandPrefixes = "'_."

// StripCommandPrefixes removes any run of ' _ . from the front of a command name, in any order
// ("_.LINE", "'_ZOOM", "._ERASE"). It returns text unchanged when nothing would be left.
func StripCommandPrefixes(text string) string {
	t := strings.TrimLeft(text, commandPrefixes)
	if strings.TrimSpace(t) == "" {
		return text
	}
	return t
}

// stripNamePrefixes is the order-10 text stage: at Command: it strips the name prefixes. At a
// prompt the text is a response, not a name, so it is left alone (a transparent command typed at
// a prompt needs the transparent frames).
func stripNamePrefixes(s *Session, text string) string {
	if s.active != nil {
		return text
	}
	if t := strings.TrimSpace(text); t != "" {
		return StripCommandPrefixes(t)
	}
	return text
}
