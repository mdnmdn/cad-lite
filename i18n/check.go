package i18n

import (
	"fmt"
	"sort"
	"strings"
)

// Placeholders returns the sorted fmt placeholders of s as "argIndex:verb" tokens, e.g.
// "%s of %d" gives ["1:s" "2:d"] and "%[2]s %[1]s" gives ["1:s" "2:s"]. "%%" is not a
// placeholder. Two strings are interchangeable translations when their lists are equal.
func Placeholders(s string) []string {
	var out []string
	arg := 0
	for i := 0; i < len(s); i++ {
		if s[i] != '%' {
			continue
		}
		i++
		if i >= len(s) {
			break
		}
		if s[i] == '%' {
			continue
		}
		for i < len(s) && strings.IndexByte("+-# 0123456789.*", s[i]) >= 0 {
			i++
		}
		if i < len(s) && s[i] == '[' {
			j := strings.IndexByte(s[i:], ']')
			if j < 0 {
				break
			}
			n := 0
			fmt.Sscanf(s[i+1:i+j], "%d", &n)
			arg = n - 1
			i += j + 1
		}
		if i >= len(s) {
			break
		}
		arg++
		out = append(out, fmt.Sprintf("%d:%c", arg, s[i]))
	}
	sort.Strings(out)
	return out
}

// Verify checks the catalogs and returns one line per problem (nil when they are sound):
//
//   - the English and Italian key sets, array keys included, are identical;
//   - every key has the same placeholders in every language, and no value is empty;
//   - "_one" and "_other" plural forms come in pairs;
//   - every command in Commands has name, desc and aliases, and aliases collide neither
//     with a canonical command name nor with another command's alias;
//   - every keyword in KeywordGroups exists in every language and its shortcuts and names do not
//     collide with a sibling's.
func Verify() []string {
	var p []string
	en, it := catalogs[English], catalogs[Italian]
	for _, l := range Langs {
		other := English
		if l == English {
			other = Italian
		}
		for k := range catalogs[l].msgs {
			if _, ok := catalogs[other].msgs[k]; !ok {
				p = append(p, fmt.Sprintf("key %q is in %s.toml but not in %s.toml", k, l, other))
			}
		}
		for k := range catalogs[l].lists {
			if _, ok := catalogs[other].lists[k]; !ok {
				p = append(p, fmt.Sprintf("array %q is in %s.toml but not in %s.toml", k, l, other))
			}
		}
	}
	for k, e := range en.msgs {
		t, ok := it.msgs[k]
		if !ok {
			continue
		}
		if strings.TrimSpace(e) == "" || strings.TrimSpace(t) == "" {
			p = append(p, fmt.Sprintf("key %q has an empty value", k))
		}
		if a, b := Placeholders(e), Placeholders(t); strings.Join(a, " ") != strings.Join(b, " ") {
			p = append(p, fmt.Sprintf("key %q: placeholders differ: en %v, it %v", k, a, b))
		}
	}
	for _, l := range Langs {
		for k := range catalogs[l].msgs {
			if base, ok := strings.CutSuffix(k, "_one"); ok && !Has(l, base+"_other") {
				p = append(p, fmt.Sprintf("%s: %q has no _other form", l, k))
			}
			if base, ok := strings.CutSuffix(k, "_other"); ok && !Has(l, base+"_one") {
				p = append(p, fmt.Sprintf("%s: %q has no _one form", l, k))
			}
		}
	}
	p = append(p, verifyCommands()...)
	p = append(p, verifyKeywords()...)
	sort.Strings(p)
	return p
}

func verifyCommands() []string {
	var p []string
	canon := map[string]bool{}
	for _, c := range Commands {
		canon[c] = true
	}
	owner := map[string]string{}
	for _, l := range Langs {
		for _, c := range Commands {
			for _, f := range []string{"name", "desc"} {
				if !Has(l, cmdKey(c, f)) {
					p = append(p, fmt.Sprintf("%s: missing %s", l, cmdKey(c, f)))
				}
			}
			if _, ok := catalogs[l].lists[cmdKey(c, "aliases")]; !ok {
				p = append(p, fmt.Sprintf("%s: missing array %s", l, cmdKey(c, "aliases")))
			}
			for _, a := range CommandIn(l, c).Aliases {
				if canon[a] && a != c {
					p = append(p, fmt.Sprintf("%s: alias %q of %s shadows the command %s", l, a, c, a))
				}
				if o, ok := owner[a]; ok && o != c {
					p = append(p, fmt.Sprintf("alias %q maps to both %s and %s", a, o, c))
				}
				owner[a] = c
			}
		}
	}
	return p
}

func verifyKeywords() []string {
	var p []string
	for group, ids := range KeywordGroups {
		for _, l := range Langs {
			seen := map[string]string{}
			for _, id := range ids {
				if _, ok := keywords[l][slug(id)]; !ok {
					p = append(p, fmt.Sprintf("%s: keyword %q (group %s) has no kw.%s", l, id, group, slug(id)))
					continue
				}
				e := keywordIn(l, id)
				forms := append([]string{strings.ToLower(e.shortcut)}, e.names...)
				for _, f := range forms {
					if o, ok := seen[f]; ok && o != id {
						p = append(p, fmt.Sprintf("%s: group %s: %q and %q both answer to %q", l, group, o, id, f))
					}
					seen[f] = id
				}
			}
		}
	}
	return p
}
