package i18n

import (
	"embed"
	"fmt"
	"sort"
	"strings"

	"github.com/BurntSushi/toml"
)

//go:embed locales/*.toml
var localeFS embed.FS

type catalog struct {
	msgs  map[string]string
	lists map[string][]string
}

var catalogs = map[Lang]*catalog{}

func init() {
	for _, l := range Langs {
		data, err := localeFS.ReadFile("locales/" + string(l) + ".toml")
		if err != nil {
			panic(fmt.Sprintf("i18n: %v", err))
		}
		c, err := parseCatalog(data)
		if err != nil {
			panic(fmt.Sprintf("i18n: locales/%s.toml: %v", l, err))
		}
		catalogs[l] = c
	}
	buildKeywords()
}

// parseCatalog decodes a catalog and flattens its tables into dotted keys.
func parseCatalog(data []byte) (*catalog, error) {
	var raw map[string]any
	if err := toml.Unmarshal(data, &raw); err != nil {
		return nil, err
	}
	c := &catalog{msgs: map[string]string{}, lists: map[string][]string{}}
	return c, flatten(c, "", raw)
}

func flatten(c *catalog, prefix string, m map[string]any) error {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		full := k
		if prefix != "" {
			full = prefix + "." + k
		}
		switch v := m[k].(type) {
		case string:
			c.msgs[full] = v
		case map[string]any:
			if err := flatten(c, full, v); err != nil {
				return err
			}
		case []any:
			list := make([]string, 0, len(v))
			for _, e := range v {
				s, ok := e.(string)
				if !ok {
					return fmt.Errorf("%s: array elements must be strings", full)
				}
				list = append(list, s)
			}
			c.lists[full] = list
		default:
			return fmt.Errorf("%s: unsupported value type %T", full, v)
		}
	}
	return nil
}

// ListKeys returns lang's array keys (e.g. "cmd.line.aliases"), unsorted.
func ListKeys(lang Lang) []string {
	out := make([]string, 0, len(catalogs[lang].lists))
	for k := range catalogs[lang].lists {
		out = append(out, k)
	}
	return out
}

// slug turns a keyword id such as "cuTting edges" into its catalog leaf "cutting_edges".
func slug(id string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(id) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
		default:
			b.WriteByte('_')
		}
	}
	return b.String()
}
