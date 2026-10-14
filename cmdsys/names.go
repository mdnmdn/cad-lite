package cmdsys

import "strings"

// commandPrefixes are the characters AutoCAD accepts in front of a command name: ' (transparent),
// _ (the global, English name: names are English here whatever the UI language, so it changes
// nothing else) and . (the built-in command, bypassing a redefinition; Cad-lite has no UNDEFINE).
// A leading - is part of the name (-LAYER is its own command).
const commandPrefixes = "'_."

// cliPrefix marks the command-line version of a dialog command: -LAYER, -OSNAP, -ARRAY.
const cliPrefix = "-"

// StripCommandPrefixes removes any run of ' _ . from the front of a command name, in any order
// ("_.LINE", "'_ZOOM", "._ERASE"), and the same run after a leading - ("-_LAYER" is "-LAYER").
// It returns text unchanged when nothing would be left.
func StripCommandPrefixes(text string) string {
	t := strings.TrimLeft(text, commandPrefixes)
	if rest, ok := strings.CutPrefix(t, cliPrefix); ok {
		if r := strings.TrimLeft(rest, commandPrefixes); r != "" {
			t = cliPrefix + r
		}
	}
	if strings.TrimSpace(t) == "" {
		return text
	}
	return t
}

// RegisterCLI registers the command-line version of a command under "-NAME" and each alias under
// "-ALIAS" (RegisterCLI(r, "LAYER", f, "LA") makes -LAYER and -LA run f). The version is its own
// command with its own prompts, which is why it takes its own factory; the factory's command
// should return "-NAME" from Name. The Italian names resolve too: "-LIVELLO" runs the command
// registered as "-LAYER" when "LIVELLO" is an alias of LAYER.
func RegisterCLI(r *Registry, name string, f func() Command, aliases ...string) {
	r.Register(cliPrefix+name, f)
	for _, a := range aliases {
		r.Alias(cliPrefix+a, cliPrefix+name)
	}
}

// IsCLIName reports whether name is typed as the command-line version of a command.
func IsCLIName(name string) bool { return strings.HasPrefix(strings.TrimSpace(name), cliPrefix) }

// lookupCommand resolves name the way the command line does: the registry (aliases first), then
// the "-NAME" form through the aliases of NAME, then a system variable typed by name (which runs
// as a one-prompt SETVAR). Commands win over variables.
func (s *Session) lookupCommand(name string) (Command, bool) {
	if c, ok := s.registry.Lookup(name); ok {
		return c, true
	}
	n := strings.ToUpper(strings.TrimSpace(name))
	if rest, ok := strings.CutPrefix(n, cliPrefix); ok {
		if canon, ok := s.registry.aliases[rest]; ok {
			if c, ok := s.registry.Lookup(cliPrefix + canon); ok {
				return c, true
			}
		}
		return nil, false
	}
	if s.vars != nil && s.vars.Has(n) {
		return newVarCommand(n), true
	}
	return nil, false
}

// stripNamePrefixes is the order-10 text stage: at Command: it strips the name prefixes. At a
// prompt the text is a response, not a name, so it is left alone (a transparent command typed at
// a prompt is the transparent stage's, transparent.go).
func stripNamePrefixes(s *Session, text string) string {
	if s.active != nil {
		return text
	}
	if t := strings.TrimSpace(text); t != "" {
		return StripCommandPrefixes(t)
	}
	return text
}
