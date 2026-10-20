// Package theme resolves the picker's palette so it FOLLOWS herdr's theme.
//
// herdr (0.9.x) exposes no theme env vars and no theme socket RPC; the two
// readable signals are:
//
//   - herdr's config.toml [theme] tables (name / dark_name / light_name /
//     auto_switch, plus [theme.custom] with optional .light/.dark variants),
//   - the hosting terminal's light/dark appearance, which herdr itself uses
//     for auto_switch (and sets on our pane — the picker learns it via
//     bubbletea's BackgroundColorMsg).
//
// Resolution precedence (lowest → highest):
//
//  1. plugin config `theme = "follow" | "latte" | "mocha"` (default follow)
//  2. the catppuccin pair keyed by herdr's effective theme name;
//     unknown names fall back on the appearance pair
//  3. herdr [theme.custom] (+ .light/.dark variant when auto_switch is on),
//     mapped onto our tokens
//
// The picker is transient: it re-resolves on every launch, so runtime
// appearance flips are picked up on the next open (and mid-session via
// tea.BackgroundColorMsg).
package theme

import (
	"os"
	"path/filepath"
	"strings"
)

// Appearance is the host terminal's light/dark state, the same signal
// herdr's auto_switch keys off.
type Appearance int

const (
	// Unknown mirrors herdr's own startup behaviour: until the terminal
	// reports, the effective theme stays at `theme.name`.
	Unknown Appearance = iota
	Light
	Dark
)

// Palette is every colour the picker renders with. All "#rrggbb".
type Palette struct {
	// Base is the panel colour washes and chips blend toward.
	Base string
	// ChipFg is the text drawn ON the accent-tinted badges (dark text on
	// light themes, light on dark ones).
	ChipFg string
	// Text is the dim (subtext) colour.
	Text string
	// NeutralAccent is the "no item selected" placeholder accent, and the
	// unknown-status dot.
	NeutralAccent string

	Err        string
	On         string
	Marker     string
	Focused    string
	Attention  string
	Working    string
	Blocked    string
	Done       string
	IdleStatus string

	// Accents are the per-item identity hues, in a STABLE order: the list
	// index comes from a hash of the item key, so a space keeps its hue
	// across opens — and the light/dark pairs below keep the same hue
	// FAMILY at the same index across appearance flips.
	Accents []string
}

// Latte is Catppuccin Latte (herdr's "catppuccin-latte"): the palette the
// picker historically hardcoded. The accent list reproduces the original
// order exactly — view tests pin its blended output.
func Latte() Palette {
	return Palette{
		Base:          "#eff1f5",
		ChipFg:        "#1e1e2e",
		Text:          "#6c6f85",
		NeutralAccent: "#6c6f85",

		Err:        "#f38ba8",
		On:         "#89b4fa",
		Marker:     "#1e66f5",
		Focused:    "#40a02b",
		Attention:  "#d20f39",
		Working:    "#fe640b",
		Blocked:    "#d20f39",
		Done:       "#40a02b",
		IdleStatus: "#1e66f5",

		Accents: []string{
			// rosewater, flamingo, pink, mauve, red, maroon,
			"#dc8a78", "#dd7878", "#ea76cb", "#8839ef", "#d20f39", "#e64553",
			// peach, yellow, green, teal, sky, sapphire,
			"#fe640b", "#df8e1d", "#40a02b", "#179299", "#04a5e5", "#209fb5",
			// blue, lavender
			"#1e66f5", "#7287fd",
		},
	}
}

// Mocha is Catppuccin Mocha (herdr's "catppuccin"): the dark counterpart,
// with pastel hues at the SAME list positions so item identities survive
// light/dark flips.
func Mocha() Palette {
	return Palette{
		Base:          "#1e1e2e",
		ChipFg:        "#eff1f5",
		Text:          "#6c7086",
		NeutralAccent: "#6c7086",

		Err:        "#f5c2e7",
		On:         "#89b4fa",
		Marker:     "#89b4fa",
		Focused:    "#a6e3a1",
		Attention:  "#f38ba8",
		Working:    "#fab387",
		Blocked:    "#f38ba8",
		Done:       "#a6e3a1",
		IdleStatus: "#89b4fa",

		Accents: []string{
			// rosewater, flamingo, pink, mauve, red, maroon,
			"#f5e0dc", "#f2cdcd", "#f5c2e7", "#cba6f7", "#f38ba8", "#eba0ac",
			// peach, yellow, green, teal, sky, sapphire,
			"#fab387", "#f9e2af", "#a6e3a1", "#94e2d5", "#89dceb", "#74c7ec",
			// blue, lavender
			"#89b4fa", "#b4befe",
		},
	}
}

// Spec is everything theme resolution consumes: the plugin's own `theme`
// setting plus the parsed [theme] tables from herdr's config.
type Spec struct {
	// PluginTheme is the plugin config's `theme` value:
	// "follow" (default), "latte" or "mocha".
	PluginTheme string

	// herdr [theme] values (zero values when herdr's config is unreadable).
	Name       string
	AutoSwitch bool
	DarkName   string
	LightName  string

	// Custom is [theme.custom]; CustomLight/CustomDark its .light/.dark
	// variants (only layered when AutoSwitch is on, matching herdr).
	Custom      map[string]string
	CustomLight map[string]string
	CustomDark  map[string]string
}

// Follow reports whether the plugin should derive its palette from herdr.
func (s Spec) Follow() bool {
	switch strings.ToLower(strings.TrimSpace(s.PluginTheme)) {
	case "", "follow", "herdr":
		return true
	}
	return false
}

// effectiveName resolves the theme herdr is actually displaying: with
// auto_switch the appearance picks the half; Unknown keeps the startup
// theme (herdr starts at `name`, so do we).
func (s Spec) effectiveName(app Appearance) string {
	if !s.AutoSwitch {
		return s.Name
	}
	switch app {
	case Light:
		if s.LightName != "" {
			return s.LightName
		}
	case Dark:
		if s.DarkName != "" {
			return s.DarkName
		}
	}
	return s.Name
}

// family maps a herdr theme name onto our pair. Only the catppuccin family
// is embedded; other named themes (tokyo-night, gruvbox, …) fall back on the
// appearance-driven catppuccin pair — honestly documented in the README.
func family(name string, app Appearance) Palette {
	switch canonical(name) {
	case "catppuccin-latte":
		return Latte()
	case "catppuccin":
		return Mocha()
	}
	// Unknown/other named theme: keep the hue-stable catppuccin pair keyed by
	// the appearance; herdr assumed Dark when its host probe had no answer,
	// but Unknown here means "desktop light is the status quo of this UI".
	if app == Dark {
		return Mocha()
	}
	return Latte()
}

// canonical lowercases and resolves herdr's theme aliases
// (config/theme.rs::canonical_theme_name, the subset relevant to us).
func canonical(name string) string {
	n := strings.ToLower(strings.TrimSpace(name))
	switch n {
	case "catppuccin-mocha", "light":
		if n == "light" {
			return "catppuccin-latte"
		}
		return "catppuccin"
	case "dawn":
		return "catppuccin-latte"
	}
	return n
}

// Resolve builds the render palette from a Spec and the current appearance.
func Resolve(s Spec, app Appearance) Palette {
	var p Palette
	switch strings.ToLower(strings.TrimSpace(s.PluginTheme)) {
	case "latte", "catppuccin-latte", "light":
		p = Latte()
	case "mocha", "catppuccin", "catppuccin-mocha", "dark":
		p = Mocha()
	default: // "follow" (and any typo: never brick the UI over config)
		p = family(s.effectiveName(app), app)
	}

	// Layer herdr's custom tokens — [theme.custom] always, plus the light or
	// dark variant when auto_switch is on (exactly herdr's own layering).
	apply := func(m map[string]string) {
		for k, v := range m {
			applyToken(&p, k, v)
		}
	}
	apply(s.Custom)
	if s.AutoSwitch {
		switch app {
		case Light:
			apply(s.CustomLight)
		case Dark:
			apply(s.CustomDark)
		}
	}
	return p
}

// applyToken maps ONE herdr [theme.custom] token onto our palette.
// Unmapped tokens and unparseable colours are ignored (never brick the UI).
func applyToken(p *Palette, token, value string) {
	v, ok := hexColor(value)
	if !ok {
		return
	}
	switch strings.ToLower(strings.TrimSpace(token)) {
	case "panel_bg":
		p.Base = v
	case "subtext0", "overlay1":
		p.Text = v
	case "accent":
		p.Marker = v
	case "red":
		p.Attention = v
		p.Blocked = v
	case "green":
		p.Done = v
		p.Focused = v
	case "blue":
		p.On = v
		p.IdleStatus = v
	case "peach":
		p.Working = v
	}
}

// HerdrConfigPath returns herdr's config.toml location (XDG-aware), the
// same path herdr itself uses.
func HerdrConfigPath() string {
	if x := os.Getenv("XDG_CONFIG_HOME"); x != "" {
		return filepath.Join(x, "herdr", "config.toml")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".config", "herdr", "config.toml")
}

// LoadSpec reads herdr's config.toml and extracts its [theme] tables.
// A missing/unreadable config returns a zero Spec (follow falls back to the
// catppuccin pair by appearance).
func LoadSpec(path string) Spec {
	if path == "" {
		return Spec{}
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return Spec{}
	}
	return ParseSpec(data)
}

// ParseSpec extracts the [theme] tables from herdr's config bytes using a
// deliberately bounded TOML subset: section headers, `key = "string"` and
// `key = true/false` inside [theme], [theme.custom],
// [theme.custom.light] and [theme.custom.dark]. Everything else — arrays,
// other tables, unknown syntax — is skipped, so a config herdr accepts can
// never break us. (No TOML dependency: the vendor set stays frozen.)
func ParseSpec(data []byte) Spec {
	var s Spec
	section := ""
	for _, raw := range strings.Split(string(data), "\n") {
		line := strings.TrimSpace(raw)
		if line == "" || line[0] == '#' {
			continue
		}
		// Table headers (single brackets only; [[array]] tables are not
		// part of the theme surface — skip without switching sections).
		if strings.HasPrefix(line, "[[") {
			section = ""
			continue
		}
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			section = strings.ToLower(strings.TrimSpace(line[1 : len(line)-1]))
			continue
		}
		key, val, ok := parseKV(line)
		if !ok {
			continue
		}
		switch section {
		case "theme":
			switch key {
			case "name":
				s.Name = val
			case "auto_switch":
				s.AutoSwitch = val == "true"
			case "dark_name":
				s.DarkName = val
			case "light_name":
				s.LightName = val
			}
		case "theme.custom":
			setToken(&s.Custom, key, val)
		case "theme.custom.light":
			setToken(&s.CustomLight, key, val)
		case "theme.custom.dark":
			setToken(&s.CustomDark, key, val)
		}
	}
	return s
}

// setToken records a custom colour token, dropping non-colour values so the
// maps carry only what applyToken can consume.
func setToken(m *map[string]string, key, val string) {
	if _, ok := hexColor(val); !ok {
		return
	}
	if *m == nil {
		*m = map[string]string{}
	}
	(*m)[key] = val
}

// parseKV splits a `key = value` line, honouring quoted strings and `#`
// comments outside quotes.
func parseKV(line string) (string, string, bool) {
	eq := strings.IndexByte(line, '=')
	if eq <= 0 {
		return "", "", false
	}
	key := strings.ToLower(strings.TrimSpace(line[:eq]))
	if key == "" {
		return "", "", false
	}
	rest := strings.TrimSpace(line[eq+1:])
	switch {
	case strings.HasPrefix(rest, "\""):
		sb := strings.Builder{}
		i := 1
		for i < len(rest) {
			switch rest[i] {
			case '\\':
				if i+1 < len(rest) {
					sb.WriteByte(rest[i+1])
					i += 2
					continue
				}
				return "", "", false
			case '"':
				return key, sb.String(), true
			default:
				sb.WriteByte(rest[i])
				i++
			}
		}
		return "", "", false // unterminated string
	default:
		// Bare token: true/false (or anything else — take the token, strip
		// trailing comments/whitespace, and let the caller decide).
		v := rest
		if c := strings.IndexByte(v, '#'); c >= 0 {
			v = v[:c]
		}
		return key, strings.TrimSpace(v), true
	}
}

// hexColor validates and normalises a colour value to "#rrggbb" form.
// Accepts herdr's #rgb shorthand; anything else fails (callers ignore).
func hexColor(v string) (string, bool) {
	v = strings.TrimSpace(v)
	switch len(v) {
	case 4:
		if v[0] != '#' {
			return "", false
		}
		for _, c := range v[1:] {
			if !isHex(c) {
				return "", false
			}
		}
		return "#" + string([]byte{v[1], v[1], v[2], v[2], v[3], v[3]}), true
	case 7:
		if v[0] != '#' {
			return "", false
		}
		for _, c := range v[1:] {
			if !isHex(c) {
				return "", false
			}
		}
		return strings.ToLower(v), true
	}
	return "", false
}

func isHex(c rune) bool {
	return (c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')
}
