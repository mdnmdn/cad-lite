package settings

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/BurntSushi/toml"

	"cad-lite/appenv"
	"cad-lite/i18n"
)

// EnvDir names an environment variable that replaces the config directory
// (the directory holding settings.toml itself), for tests and portable setups.
const EnvDir = "CONFIG_DIR"

// FileName is the settings file's name inside the config directory.
const FileName = "settings.toml"

// Path returns the settings file: $CADLITE_CONFIG_DIR/settings.toml when that
// variable is set, else <os.UserConfigDir()>/cad-lite/settings.toml
// (~/Library/Application Support on macOS, %AppData% on Windows,
// $XDG_CONFIG_HOME or ~/.config on Linux). On js there is no file system and
// Path returns an error: keep the settings in memory.
func Path() (string, error) {
	if d := appenv.Get(EnvDir); d != "" {
		return filepath.Join(d, FileName), nil
	}
	if runtime.GOOS == "js" {
		return "", errors.New("settings: no config directory on js")
	}
	d, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("settings: %w", err)
	}
	path := filepath.Join(d, DirName, FileName)
	migrateLegacy(path, filepath.Join(d, LegacyDirName, FileName))
	return path, nil
}

// DirName is the settings folder's name inside the OS config directory.
const DirName = "cad-lite"

// LegacyDirName is the folder the settings had while the project was called
// Cad-lite. Path copies its settings file into DirName on first start.
const LegacyDirName = "go-draw"

// migrateLegacy copies the old settings file to path when path does not exist
// yet and old does. The old file is left in place; every failure is ignored
// (the app then starts with the defaults).
func migrateLegacy(path, old string) {
	if _, err := os.Stat(path); err == nil || !errors.Is(err, fs.ErrNotExist) {
		return
	}
	data, err := os.ReadFile(old)
	if err != nil {
		return
	}
	if os.MkdirAll(filepath.Dir(path), 0o755) != nil {
		return
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		os.Remove(path)
	}
}

// Load reads path. A missing file yields Default and no error. A file that
// cannot be read or parsed yields Default and the error; invalid values are
// clamped silently (LoadWarn reports them).
func Load(path string) (Settings, error) {
	s, _, err := LoadWarn(path)
	return s, err
}

// LoadWarn is Load that also returns one warning per value it clamped or
// ignored.
func LoadWarn(path string) (Settings, []string, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return Default(), nil, nil
	}
	if err != nil {
		return Default(), nil, fmt.Errorf("settings: %w", err)
	}
	s, w, err := Parse(data)
	if err != nil {
		return Default(), nil, fmt.Errorf("settings: %s: %w", path, err)
	}
	return s, w, nil
}

var tables = map[string]map[string]bool{
	"appearance": {"theme": true, "ui_scale": true, "language": true},
	"drawing":    {"grid": true, "osnap": true, "ortho": true, "wheel": true},
	"agent":      {"default": true},
	"update":     {"auto_check": true, "last_check": true},
	"window":     {"width": true, "height": true},
	"layout": {"side_width": true, "agent_width": true, "layers_height": true, "history_height": true,
		"side_collapsed": true, "layers_collapsed": true, "properties_collapsed": true, "history_collapsed": true, "ribbon_collapsed": true},
}

// Parse decodes TOML text into Settings: absent keys keep their defaults,
// values of the wrong type or range are replaced (one warning each) and
// unknown keys go to Extra.
func Parse(data []byte) (Settings, []string, error) {
	var raw map[string]any
	if _, err := toml.NewDecoder(bytes.NewReader(data)).Decode(&raw); err != nil {
		return Settings{}, nil, err
	}
	s := Default()
	var warn []string
	extra := map[string]any{}
	bad := func(key, want string, v any) {
		warn = append(warn, i18n.T("settings.warn.wrong_type", key, want, v))
	}
	for name, v := range raw {
		keys, known := tables[name]
		tbl, isTbl := v.(map[string]any)
		if !known {
			extra[name] = v
			continue
		}
		if !isTbl {
			bad(name, "a table", v)
			continue
		}
		for k, v := range tbl {
			if !keys[k] {
				extra[name+"."+k] = v
				continue
			}
			full := name + "." + k
			switch full {
			case "appearance.theme":
				if x, ok := v.(string); ok {
					s.Theme = x
				} else {
					bad(full, "a string", v)
				}
			case "appearance.language":
				if x, ok := v.(string); ok {
					s.Language = x
				} else {
					bad(full, "a string", v)
				}
			case "appearance.ui_scale":
				switch x := v.(type) {
				case float64:
					s.UIScale = x
				case int64:
					s.UIScale = float64(x)
				default:
					bad(full, "a number", v)
				}
			case "drawing.grid", "drawing.osnap", "drawing.ortho":
				x, ok := v.(bool)
				if !ok {
					bad(full, "a boolean", v)
					break
				}
				switch k {
				case "grid":
					s.Grid = x
				case "osnap":
					s.OSnap = x
				default:
					s.Ortho = x
				}
			case "drawing.wheel":
				if x, ok := v.(string); ok {
					s.Wheel = x
				} else {
					bad(full, "a string", v)
				}
			case "agent.default":
				if x, ok := v.(string); ok {
					s.DefaultAgent = x
				} else {
					bad(full, "a string", v)
				}
			case "update.auto_check":
				if x, ok := v.(bool); ok {
					s.AutoUpdate = x
				} else {
					bad(full, "a boolean", v)
				}
			case "update.last_check":
				if x, ok := v.(time.Time); ok {
					s.LastUpdateCheck = x
				} else {
					bad(full, "a date and time", v)
				}
			case "layout.side_width", "layout.agent_width", "layout.layers_height", "layout.history_height":
				x, ok := v.(int64)
				if !ok {
					bad(full, "an integer", v)
					break
				}
				x = min(max(x, 0), math.MaxInt32)
				switch k {
				case "side_width":
					s.Layout.SideWidth = int(x)
				case "agent_width":
					s.Layout.AgentWidth = int(x)
				case "layers_height":
					s.Layout.LayersHeight = int(x)
				default:
					s.Layout.HistoryHeight = int(x)
				}
			case "layout.side_collapsed", "layout.layers_collapsed", "layout.properties_collapsed", "layout.history_collapsed", "layout.ribbon_collapsed":
				x, ok := v.(bool)
				if !ok {
					bad(full, "a boolean", v)
					break
				}
				switch k {
				case "side_collapsed":
					s.Layout.SideCollapsed = x
				case "layers_collapsed":
					s.Layout.LayersCollapsed = x
				case "properties_collapsed":
					s.Layout.PropsCollapsed = x
				case "history_collapsed":
					s.Layout.HistoryCollapsed = x
				default:
					s.Layout.RibbonCollapsed = x
				}
			case "window.width", "window.height":
				x, ok := v.(int64)
				if !ok {
					bad(full, "an integer", v)
					break
				}
				if x > math.MaxInt32 || x < 0 {
					x = 0
				}
				if k == "width" {
					s.Window.Width = int(x)
				} else {
					s.Window.Height = int(x)
				}
			}
		}
	}
	if len(extra) > 0 {
		s.Extra = extra
	}
	return s, append(warn, s.Normalize()...), nil
}

// Marshal renders s as the settings file's text: a comment header, then the
// known tables in a fixed order, then Extra. Values are written as they are;
// call Normalize first to persist clamped ones.
func Marshal(s Settings) []byte {
	var b bytes.Buffer
	b.WriteString("# Cad-lite settings. Rewritten when you change a setting in the app; comments you add are not kept.\n")
	b.WriteString("# Unknown keys are preserved. Invalid values are clamped when loaded.\n\n")
	top := map[string]any{}
	sub := map[string]map[string]any{}
	for k, v := range s.Extra {
		if t, key, ok := strings.Cut(k, "."); ok && tables[t] != nil && !tables[t][key] {
			if sub[t] == nil {
				sub[t] = map[string]any{}
			}
			sub[t][key] = v
		} else {
			top[k] = v
		}
	}
	writeKVs(&b, top)
	if len(top) > 0 {
		b.WriteByte('\n')
	}
	section := func(name, comment string, lines ...string) {
		b.WriteString("[" + name + "]\n")
		if comment != "" {
			b.WriteString("# " + comment + "\n")
		}
		for _, l := range lines {
			b.WriteString(l + "\n")
		}
		writeKVs(&b, sub[name])
		b.WriteByte('\n')
	}
	section("appearance", "theme: dark, light, classic or high-contrast. language: auto, en or it. ui_scale: 0.75 to 2.0 in steps of 0.05.",
		"theme = "+quote(s.Theme),
		"language = "+quote(s.Language),
		"ui_scale = "+formatValue(s.UIScale))
	section("drawing", "wheel: auto, zoom (wheel zooms, Shift pans) or pan (wheel pans, Ctrl/Cmd zooms).",
		"grid = "+strconv.FormatBool(s.Grid),
		"osnap = "+strconv.FormatBool(s.OSnap),
		"ortho = "+strconv.FormatBool(s.Ortho),
		"wheel = "+quote(s.Wheel))
	section("agent", "default: the harness the agent panel selects (empty: first installed).",
		"default = "+quote(s.DefaultAgent))
	update := []string{"auto_check = " + strconv.FormatBool(s.AutoUpdate)}
	if !s.LastUpdateCheck.IsZero() {
		update = append(update, "last_check = "+s.LastUpdateCheck.UTC().Format(time.RFC3339))
	}
	section("update", "auto_check: look for a new release now and then. last_check: when the last check finished (set by the app).", update...)
	section("window", "last window size in logical pixels; 0 means unset.",
		"width = "+strconv.Itoa(s.Window.Width),
		"height = "+strconv.Itoa(s.Window.Height))
	l := s.Layout
	section("layout", "panel sizes in logical pixels (0: automatic) and which panels are collapsed; collapsing keeps the size.",
		"side_width = "+strconv.Itoa(l.SideWidth),
		"agent_width = "+strconv.Itoa(l.AgentWidth),
		"layers_height = "+strconv.Itoa(l.LayersHeight),
		"history_height = "+strconv.Itoa(l.HistoryHeight),
		"side_collapsed = "+strconv.FormatBool(l.SideCollapsed),
		"layers_collapsed = "+strconv.FormatBool(l.LayersCollapsed),
		"properties_collapsed = "+strconv.FormatBool(l.PropsCollapsed),
		"history_collapsed = "+strconv.FormatBool(l.HistoryCollapsed),
		"ribbon_collapsed = "+strconv.FormatBool(l.RibbonCollapsed))
	return append(bytes.TrimRight(b.Bytes(), "\n"), '\n')
}

func sortedKeys[V any](m map[string]V) []string {
	ks := make([]string, 0, len(m))
	for k := range m {
		ks = append(ks, k)
	}
	sort.Strings(ks)
	return ks
}

func writeKVs(b *bytes.Buffer, m map[string]any) {
	for _, k := range sortedKeys(m) {
		b.WriteString(formatKey(k) + " = " + formatValue(m[k]) + "\n")
	}
}

func formatKey(k string) string {
	if k != "" && strings.IndexFunc(k, func(r rune) bool {
		return !(r == '_' || r == '-' || r >= '0' && r <= '9' || r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z')
	}) < 0 {
		return k
	}
	return quote(k)
}

func quote(s string) string { return strconv.Quote(s) }

// formatValue writes any value TOML decoding can produce as an inline value.
func formatValue(v any) string {
	switch x := v.(type) {
	case string:
		return quote(x)
	case bool:
		return strconv.FormatBool(x)
	case int64:
		return strconv.FormatInt(x, 10)
	case int:
		return strconv.Itoa(x)
	case float64:
		switch {
		case math.IsNaN(x):
			return "nan"
		case math.IsInf(x, 1):
			return "inf"
		case math.IsInf(x, -1):
			return "-inf"
		}
		t := strconv.FormatFloat(x, 'f', -1, 64)
		if !strings.ContainsAny(t, ".") {
			t += ".0"
		}
		return t
	case time.Time:
		return x.Format(time.RFC3339Nano)
	case []any:
		parts := make([]string, len(x))
		for i, e := range x {
			parts[i] = formatValue(e)
		}
		return "[" + strings.Join(parts, ", ") + "]"
	case []map[string]any:
		parts := make([]string, len(x))
		for i, e := range x {
			parts[i] = formatValue(e)
		}
		return "[" + strings.Join(parts, ", ") + "]"
	case map[string]any:
		parts := make([]string, 0, len(x))
		for _, k := range sortedKeys(x) {
			parts = append(parts, formatKey(k)+" = "+formatValue(x[k]))
		}
		return "{ " + strings.Join(parts, ", ") + " }"
	}
	return quote(fmt.Sprint(v))
}

// Save writes s to path atomically: the text goes to a temporary file in the
// same directory (mode 0600), which is then renamed over path. The directory
// is created with mode 0700. s is normalized first, so the file never holds an
// invalid value.
func Save(path string, s Settings) error {
	s = s.Clone()
	s.Normalize()
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("settings: %w", err)
	}
	f, err := os.CreateTemp(dir, ".settings-*.tmp")
	if err != nil {
		return fmt.Errorf("settings: %w", err)
	}
	tmp := f.Name()
	fail := func(err error) error {
		f.Close()
		os.Remove(tmp)
		return fmt.Errorf("settings: %w", err)
	}
	if err := f.Chmod(0o600); err != nil && runtime.GOOS != "windows" {
		return fail(err)
	}
	if _, err := f.Write(Marshal(s)); err != nil {
		return fail(err)
	}
	if err := f.Sync(); err != nil {
		return fail(err)
	}
	if err := f.Close(); err != nil {
		os.Remove(tmp)
		return fmt.Errorf("settings: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		os.Remove(tmp)
		return fmt.Errorf("settings: %w", err)
	}
	return nil
}
