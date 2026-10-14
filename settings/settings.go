// Package settings holds Cad-lite's user preferences and their TOML file.
//
// The package is pure Go and knows nothing about the UI. A Settings value is
// plain data: Load reads it (a missing file yields defaults, invalid values are
// clamped with warnings, unknown keys are kept in Extra) and Save writes it
// atomically. The file lives in the OS user config folder (see Path); on js
// there is no file system, Path fails and the application keeps its settings in
// memory.
package settings

import (
	"math"
	"strings"
	"time"

	"cad-lite/i18n"
)

// Limits and defaults of the persisted values.
const (
	// MinUIScale and MaxUIScale bound Settings.UIScale.
	MinUIScale = 0.75
	// MaxUIScale is the largest UI scale.
	MaxUIScale = 2.0
	// UIScaleStep is the granularity the UI scale snaps to.
	UIScaleStep = 0.05
	// DefaultTheme is the theme a fresh install uses.
	DefaultTheme = "dark"
	// MinWindowSize and MaxWindowSize bound each window dimension in logical
	// pixels; zero means "not stored" and is kept as is.
	MinWindowSize = 320
	// MaxWindowSize is the largest stored window dimension.
	MaxWindowSize = 16384
)

// Languages for Settings.Language.
const (
	// LangAuto follows the operating system's language.
	LangAuto = "auto"
	// LangEnglish selects English.
	LangEnglish = "en"
	// LangItalian selects Italian.
	LangItalian = "it"
)

// Languages lists the valid Settings.Language values.
var Languages = []string{LangAuto, LangEnglish, LangItalian}

// Wheel behaviours for Settings.Wheel.
const (
	// WheelAuto leaves the choice to the canvas (wheel zooms, Shift pans).
	WheelAuto = "auto"
	// WheelZoom makes the wheel zoom and Shift+wheel pan.
	WheelZoom = "zoom"
	// WheelPan makes the wheel pan and Ctrl/Cmd+wheel zoom.
	WheelPan = "pan"
)

// WheelModes lists the valid Settings.Wheel values.
var WheelModes = []string{WheelAuto, WheelZoom, WheelPan}

// Window is the last window size, in logical pixels. Zero means unset.
type Window struct {
	// Width is the window width.
	Width int
	// Height is the window height.
	Height int
}

// MaxLayoutSize bounds each stored panel size in logical pixels; zero means
// "automatic" and is kept as is.
const MaxLayoutSize = 4096

// Layout is the workspace layout the user arranged: panel sizes in logical
// pixels (zero means automatic) and which panels are collapsed. Collapsing
// keeps the size, so expanding restores it.
type Layout struct {
	// SideWidth is the Layers and Properties column's width.
	SideWidth int
	// AgentWidth is the agent column's width.
	AgentWidth int
	// LayersHeight caps the Layers section's height; the section never grows past its rows.
	LayersHeight int
	// HistoryHeight is the command history's height.
	HistoryHeight int
	// SideCollapsed folds the Layers and Properties column to a thin strip.
	SideCollapsed bool
	// LayersCollapsed and PropsCollapsed fold one section to its header.
	LayersCollapsed, PropsCollapsed bool
	// HistoryCollapsed hides the command history, leaving the input row.
	HistoryCollapsed bool
	// RibbonCollapsed hides the ribbon's button groups.
	RibbonCollapsed bool
}

// Settings are the user's persisted preferences.
type Settings struct {
	// Theme names a registered UI theme ("dark", "light", ...). Unknown names
	// are kept; the UI falls back to the default theme.
	Theme string
	// Language is the interface language: LangAuto, LangEnglish or LangItalian.
	Language string
	// UIScale is the whole-UI zoom, MinUIScale..MaxUIScale in UIScaleStep steps.
	UIScale float64
	// Grid, OSnap and Ortho are the drawing aids a new session starts with.
	Grid, OSnap, Ortho bool
	// Wheel is the mouse wheel behaviour: WheelAuto, WheelZoom or WheelPan.
	Wheel string
	// DefaultAgent names the harness the agent panel selects; empty means the
	// first one installed.
	DefaultAgent string
	// AgentBinaries maps a harness name ("opencode", "claude", "codex") to the
	// executable the user chose for it, which wins over the PATH lookup. A
	// harness without an entry is looked up on the PATH. Nil when empty.
	AgentBinaries map[string]string
	// AutoUpdate turns the periodic background check for a new release on or off.
	AutoUpdate bool
	// LastUpdateCheck is when the last check for a release finished; zero means never.
	LastUpdateCheck time.Time
	// Window is the size to open the window at.
	Window Window
	// Layout is the arrangement of the workspace panels.
	Layout Layout
	// Extra round-trips keys this version does not know. Top-level keys are
	// stored under their name, keys inside a known table as "table.key".
	Extra map[string]any
}

// Default returns the settings of a fresh install.
func Default() Settings {
	return Settings{
		Theme:      DefaultTheme,
		Language:   LangAuto,
		UIScale:    1,
		Grid:       true,
		OSnap:      true,
		Wheel:      WheelAuto,
		AutoUpdate: true,
		Window:     Window{Width: 1280, Height: 800},
	}
}

// ClampUIScale limits s to the valid range and snaps it to UIScaleStep.
func ClampUIScale(s float64) float64 {
	if math.IsNaN(s) {
		return 1
	}
	s = math.Min(math.Max(s, MinUIScale), MaxUIScale)
	n := math.Round((s - MinUIScale) / UIScaleStep)
	return math.Round((MinUIScale+n*UIScaleStep)*100) / 100
}

// Normalize clamps every value into its valid range and returns one warning
// per value it had to change.
func (s *Settings) Normalize() []string {
	var w []string
	if strings.TrimSpace(s.Theme) == "" {
		w = append(w, i18n.T("settings.warn.theme_empty", DefaultTheme))
		s.Theme = DefaultTheme
	}
	if c := ClampUIScale(s.UIScale); c != s.UIScale {
		w = append(w, i18n.T("settings.warn.ui_scale", s.UIScale, c))
		s.UIScale = c
	}
	if !contains(Languages, s.Language) {
		w = append(w, i18n.T("settings.warn.language", s.Language, Languages, LangAuto))
		s.Language = LangAuto
	}
	ok := false
	for _, m := range WheelModes {
		ok = ok || m == s.Wheel
	}
	if !ok {
		w = append(w, i18n.T("settings.warn.wheel", s.Wheel, WheelModes, WheelAuto))
		s.Wheel = WheelAuto
	}
	for _, d := range []struct {
		name string
		v    *int
	}{{"layout.side_width", &s.Layout.SideWidth}, {"layout.agent_width", &s.Layout.AgentWidth},
		{"layout.layers_height", &s.Layout.LayersHeight}, {"layout.history_height", &s.Layout.HistoryHeight}} {
		if c := min(max(*d.v, 0), MaxLayoutSize); c != *d.v {
			w = append(w, i18n.T("settings.warn.out_of_range", d.name, *d.v, c))
			*d.v = c
		}
	}
	for _, d := range []struct {
		name string
		v    *int
	}{{"window.width", &s.Window.Width}, {"window.height", &s.Window.Height}} {
		if *d.v == 0 {
			continue
		}
		if c := min(max(*d.v, MinWindowSize), MaxWindowSize); c != *d.v {
			w = append(w, i18n.T("settings.warn.out_of_range", d.name, *d.v, c))
			*d.v = c
		}
	}
	s.AgentBinaries = cleanBinaries(s.AgentBinaries)
	return w
}

// cleanBinaries trims every path and drops empty ones; it returns nil for an
// empty result. It never changes m itself.
func cleanBinaries(m map[string]string) map[string]string {
	var out map[string]string
	for h, p := range m {
		if p = strings.TrimSpace(p); p != "" && strings.TrimSpace(h) != "" {
			if out == nil {
				out = map[string]string{}
			}
			out[h] = p
		}
	}
	return out
}

func contains(l []string, v string) bool {
	for _, x := range l {
		if x == v {
			return true
		}
	}
	return false
}

// Clone returns a copy that shares no map with s.
func (s Settings) Clone() Settings {
	if s.AgentBinaries != nil {
		b := make(map[string]string, len(s.AgentBinaries))
		for k, v := range s.AgentBinaries {
			b[k] = v
		}
		s.AgentBinaries = b
	}
	if s.Extra != nil {
		e := make(map[string]any, len(s.Extra))
		for k, v := range s.Extra {
			e[k] = v
		}
		s.Extra = e
	}
	return s
}
