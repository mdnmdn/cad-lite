package i18n

import "strings"

// CanonicalSnapName resolves an exact catalog-owned snap input alias from any shipped language
// to its English name. Matching ignores case and repeated whitespace. A leading underscore
// excludes localized aliases; the caller remains responsible for English names and abbreviations.
func CanonicalSnapName(text string) (string, bool) {
	name := strings.Join(strings.Fields(text), " ")
	if strings.HasPrefix(name, "_") {
		return "", false
	}
	for _, lang := range Langs {
		for _, snap := range []string{"endpoint", "midpoint", "intersection", "nearest"} {
			for _, alias := range For(lang).List("snap.aliases." + snap) {
				if strings.EqualFold(alias, name) {
					return strings.ToUpper(snap), true
				}
			}
		}
	}
	return "", false
}
