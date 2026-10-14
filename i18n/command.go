package i18n

import "strings"

// Commands lists the canonical (English, upper-case) names of every command that has catalog
// metadata. Verify requires name, desc and aliases for each, in every language.
var Commands = []string{
	"LINE", "PLINE", "CIRCLE", "ARC", "RECTANG", "POLYGON", "ERASE", "MOVE", "COPY", "ROTATE", "SCALE",
	"UNDO", "U", "REDO", "ZOOM", "LAYER", "DIST", "AREA", "MEASUREGEOM", "ID", "LIST", "UNITS", "NEW", "OPEN",
	"SAVE", "MIRROR", "OFFSET", "FILLET", "CHAMFER", "TRIM", "EXTEND", "BREAK", "BREAKATPOINT", "JOIN", "EXPLODE", "STRETCH",
	"AI", "OPTIONS",
}

// CommandInfo is a command's localized metadata.
type CommandInfo struct {
	// Name is the display name ("Linea"); the typed form is strings.ToUpper(Name).
	Name string
	// Desc is a one-line description ("Disegna segmenti di retta ...").
	Desc string
	// Aliases are the extra names typed to run the command in this language ("LINEA").
	Aliases []string
}

func cmdKey(name, field string) string { return "cmd." + strings.ToLower(name) + "." + field }

// CommandName returns the display name of command name (any case), in the current language.
func CommandName(name string) string { return T(cmdKey(name, "name")) }

// CommandName returns the display name of command name in the translator's language, or name
// itself when the command has no catalog entry (a command added by a host or a test).
func (t Translator) CommandName(name string) string {
	k := cmdKey(name, "name")
	if !Has(English, k) {
		return name
	}
	return t.T(k)
}

// Command returns the current language's metadata for command name (any case).
func Command(name string) CommandInfo { return CommandIn(Current(), name) }

// CommandIn returns lang's metadata for command name.
func CommandIn(lang Lang, name string) CommandInfo {
	t := For(lang)
	return CommandInfo{
		Name:    t.T(cmdKey(name, "name")),
		Desc:    t.T(cmdKey(name, "desc")),
		Aliases: upper(t.List(cmdKey(name, "aliases"))),
	}
}

// Alias maps a typed name to the canonical command it runs.
type Alias struct {
	// Alias is the upper-case typed name.
	Alias string
	// Command is the canonical upper-case command name.
	Command string
}

// Aliases returns lang's aliases for every command, in Commands order.
func Aliases(lang Lang) []Alias {
	var out []Alias
	for _, c := range Commands {
		for _, a := range CommandIn(lang, c).Aliases {
			out = append(out, Alias{Alias: a, Command: c})
		}
	}
	return out
}

// AllAliases returns the aliases of every shipped language, English first, without duplicates.
// Register all of them whatever the UI language: a drafter who switches to Italian keeps typing
// L, and an Italian drafter pasting an English script still works.
func AllAliases() []Alias {
	var out []Alias
	seen := map[Alias]bool{}
	for _, l := range Langs {
		for _, a := range Aliases(l) {
			if !seen[a] {
				seen[a] = true
				out = append(out, a)
			}
		}
	}
	return out
}

// RegisterAliases calls add(alias, command) for every alias of every language; pass
// Registry.Alias. It keeps cmdsys free of any i18n type.
func RegisterAliases(add func(alias, command string)) {
	for _, a := range AllAliases() {
		add(a.Alias, a.Command)
	}
}

func upper(in []string) []string {
	out := make([]string, len(in))
	for i, s := range in {
		out[i] = strings.ToUpper(s)
	}
	return out
}
