package cmdsys

import (
	"sort"
	"strings"
)

// Registry maps command names and aliases to command factories.
type Registry struct {
	factories map[string]func() Command
	aliases   map[string]string
}

// NewRegistry returns an empty registry.
func NewRegistry() *Registry {
	return &Registry{factories: map[string]func() Command{}, aliases: map[string]string{}}
}

// Register adds a command under its upper-case name.
func (r *Registry) Register(name string, f func() Command) {
	r.factories[strings.ToUpper(name)] = f
}

// Alias makes alias run name (e.g. "L" runs "LINE").
func (r *Registry) Alias(alias, name string) {
	r.aliases[strings.ToUpper(alias)] = strings.ToUpper(name)
}

// LoadAliases reads "ALIAS,*COMMAND" lines in the acad.pgp format; other
// lines and ";" comments are ignored.
func (r *Registry) LoadAliases(pgp string) {
	for _, line := range strings.Split(pgp, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, ";") {
			continue
		}
		a, n, ok := strings.Cut(line, ",")
		if !ok {
			continue
		}
		r.Alias(strings.TrimSpace(a), strings.TrimPrefix(strings.TrimSpace(n), "*"))
	}
}

// Lookup returns a new instance of the command name or alias.
func (r *Registry) Lookup(name string) (Command, bool) {
	n := strings.ToUpper(strings.TrimSpace(name))
	if a, ok := r.aliases[n]; ok {
		n = a
	}
	f, ok := r.factories[n]
	if !ok {
		return nil, false
	}
	return f(), true
}

// Names returns every command name, sorted.
func (r *Registry) Names() []string {
	out := make([]string, 0, len(r.factories))
	for n := range r.factories {
		out = append(out, n)
	}
	sort.Strings(out)
	return out
}

// AliasesOf returns the aliases that run command name, shortest first and alphabetical within a
// length. It is the table a keyboard hint is read from, so a hint never repeats an alias by hand.
func (r *Registry) AliasesOf(name string) []string {
	n := strings.ToUpper(name)
	var out []string
	for a, c := range r.aliases {
		if c == n {
			out = append(out, a)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if len(out[i]) != len(out[j]) {
			return len(out[i]) < len(out[j])
		}
		return out[i] < out[j]
	})
	return out
}

// Complete returns the command names and aliases starting with prefix.
func (r *Registry) Complete(prefix string) []string {
	p := strings.ToUpper(prefix)
	var out []string
	for _, n := range r.Names() {
		if strings.HasPrefix(n, p) {
			out = append(out, n)
		}
	}
	return out
}
