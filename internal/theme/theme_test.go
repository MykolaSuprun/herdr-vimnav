package theme

import (
	"reflect"
	"testing"
)

// realisticHerdrConfig mirrors the shape of a real user config: comments,
// [theme] plus custom variants, arrays, and unrelated tables.
const realisticHerdrConfig = `
onboarding = false

# Theme: herdr follows the terminal's light/dark at runtime.
[theme]
name = "catppuccin-latte"        # static startup theme
auto_switch = true
light_name = "catppuccin-latte"
dark_name = "catppuccin"

[theme.custom]
panel_bg = "#eff1f5"
red = "#d20f39"

[theme.custom.dark]
panel_bg = "#11111b"
subtext0 = "#7f849c"

[theme.custom.light]
blue = "#209fb5"

[keys]
prefix = "ctrl+e"

[[keys.command]]
key = "prefix+w"
type = "plugin_action"
command = "preview-grid.spaces"

[other]
unlikely = "part of the theme surface"
`

func TestResolveFollowLatte(t *testing.T) {
	s := Spec{Name: "catppuccin-latte", AutoSwitch: true, LightName: "catppuccin-latte", DarkName: "catppuccin"}
	if got := Resolve(s, Light); got.Base != "#eff1f5" {
		t.Errorf("light: got base %s, want latte base", got.Base)
	}
	if got := Resolve(s, Unknown); got.Base != "#eff1f5" {
		t.Errorf("unknown appearance must keep the startup (name) theme: got %s", got.Base)
	}
}

func TestResolveFollowMocha(t *testing.T) {
	s := Spec{Name: "catppuccin-latte", AutoSwitch: true, LightName: "catppuccin-latte", DarkName: "catppuccin"}
	p := Resolve(s, Dark)
	if p.Base != "#1e1e2e" {
		t.Errorf("dark: got base %s, want mocha base", p.Base)
	}
	if p.ChipFg != "#eff1f5" {
		t.Errorf("dark chips must carry light text, got %s", p.ChipFg)
	}
}

// The light/dark accent lists must stay hue-aligned at every index: an
// item's identity comes from a hash of its key, so index N must mean the
// same hue family in both palettes.
func TestAccentsHueStableAcrossFlip(t *testing.T) {
	l, m := Latte(), Mocha()
	if len(l.Accents) != len(m.Accents) {
		t.Fatalf("accent list lengths differ: %d vs %d", len(l.Accents), len(m.Accents))
	}
	sameHue := []int{2, 3, 4, 6, 7, 12} // pink, mauve, red, peach, yellow, blue
	for _, i := range sameHue {
		if l.Accents[i] == m.Accents[i] {
			t.Errorf("accent %d identical across palettes (bad copy?): %s", i, l.Accents[i])
		}
	}
}

func TestResolvePluginForces(t *testing.T) {
	s := Spec{PluginTheme: "latte", Name: "catppuccin", AutoSwitch: true, DarkName: "catppuccin"}
	if p := Resolve(s, Dark); p.Base != "#eff1f5" {
		t.Errorf("plugin 'latte' must win over herdr dark: got %s", p.Base)
	}
	s.PluginTheme = "mocha"
	if p := Resolve(s, Light); p.Base != "#1e1e2e" {
		t.Errorf("plugin 'mocha' must win over herdr light: got %s", p.Base)
	}
}

func TestResolveCustomLayering(t *testing.T) {
	s := Spec{
		Name: "catppuccin-latte", AutoSwitch: true,
		LightName: "catppuccin-latte", DarkName: "catppuccin",
		Custom:      map[string]string{"panel_bg": "#aaaaaa"},
		CustomLight: map[string]string{"blue": "#209fb5"},
		CustomDark:  map[string]string{"panel_bg": "#11111b", "subtext0": "#7f849c"},
	}
	light := Resolve(s, Light)
	if light.Base != "#aaaaaa" {
		t.Errorf("custom panel_bg must apply always: got %s", light.Base)
	}
	if light.IdleStatus != "#209fb5" {
		t.Errorf("custom.light blue must map to idle: got %s", light.IdleStatus)
	}
	dark := Resolve(s, Dark)
	if dark.Base != "#11111b" {
		t.Errorf("custom.dark panel_bg must override base custom: got %s", dark.Base)
	}
	if dark.Text != "#7f849c" {
		t.Errorf("custom.dark subtext0 must map to Text: got %s", dark.Text)
	}
	// The light variant must not leak in: idle keeps the MOCHA blue, not
	// custom.light's #209fb5.
	if dark.IdleStatus != "#89b4fa" {
		t.Errorf("custom.light must NOT apply when dark: got %s", dark.IdleStatus)
	}
}

func TestResolveUnknownThemesFallBackByAppearance(t *testing.T) {
	s := Spec{Name: "tokyo-night"}
	if p := Resolve(s, Dark); p.Base != "#1e1e2e" {
		t.Errorf("unknown theme + dark host must fall back to the dark pair, got %s", p.Base)
	}
	if p := Resolve(s, Unknown); p.Base != "#eff1f5" {
		t.Errorf("unknown theme + unknown appearance must fall back to light (status quo), got %s", p.Base)
	}
}

func TestResolveBadValuesNeverBrick(t *testing.T) {
	s := Spec{
		Name:   "catppuccin",
		Custom: map[string]string{"panel_bg": "reset", "red": "not-a-color", "blue": "#zzzzzz"},
	}
	p := Resolve(s, Unknown)
	if p.Base != "#1e1e2e" {
		t.Errorf("unparseable custom tokens must be ignored, got base %s", p.Base)
	}
	if p.Blocked != "#d20f39" && p.Blocked != "#f38ba8" {
		t.Errorf("unexpected blocked: %s", p.Blocked)
	}
}

func TestParseSpecRealisticConfig(t *testing.T) {
	s := ParseSpec([]byte(realisticHerdrConfig))
	if s.Name != "catppuccin-latte" {
		t.Errorf("name: got %q", s.Name)
	}
	if !s.AutoSwitch {
		t.Error("auto_switch not parsed")
	}
	if s.DarkName != "catppuccin" || s.LightName != "catppuccin-latte" {
		t.Errorf("names: dark=%q light=%q", s.DarkName, s.LightName)
	}
	if s.Custom["panel_bg"] != "#eff1f5" || s.Custom["red"] != "#d20f39" {
		t.Errorf("custom: %v", s.Custom)
	}
	if s.CustomDark["panel_bg"] != "#11111b" {
		t.Errorf("custom.dark: %v", s.CustomDark)
	}
	if s.CustomLight["blue"] != "#209fb5" {
		t.Errorf("custom.light: %v", s.CustomLight)
	}
	// keys/other sections must not leak in
	if len(s.Custom) != 2 || len(s.CustomLight) != 1 || len(s.CustomDark) != 2 {
		t.Errorf("section leakage: %v %v %v", s.Custom, s.CustomLight, s.CustomDark)
	}
}

func TestParseSpecGuards(t *testing.T) {
	cases := []struct {
		name string
		in   string
	}{
		{"array of tables only", "[[keys.command]]\nkey=\"w\"\n[theme]\nname = \"catppuccin\""},
		{"quoted value with trailing comment", "[theme.custom]\npanel_bg = \"#eff1f5\" # panel colour"},
		{"quoted non-colour is dropped", "[theme.custom]\npanel_bg = \"reset\""},
		{"unterminated string", "[theme]\nname = \"catppuccin"},
		{"empty", ""},
		{"bare non-bool", "[theme]\nauto_switch = maybe"},
		{"escaped quote in value", "[theme]\nname = \"cat\\\"ppuccin\""},
	}
	for _, c := range cases {
		s := ParseSpec([]byte(c.in))
		if c.name == "escaped quote in value" && s.Name != "cat\"ppuccin" {
			t.Errorf("%s: got %q", c.name, s.Name)
		}
		if c.name == "quoted value with trailing comment" {
			if s.Custom["panel_bg"] != "#eff1f5" {
				t.Errorf("%s: got %q (want the quoted colour, hash included)", c.name, s.Custom["panel_bg"])
			}
		}
		if c.name == "quoted non-colour is dropped" && len(s.Custom) != 0 {
			t.Errorf("%s: non-colour values must be dropped, got %v", c.name, s.Custom)
		}
		if c.name == "bare non-bool" && s.AutoSwitch {
			t.Errorf("%s: auto_switch must stay false", c.name)
		}
	}
}

func TestHexColor(t *testing.T) {
	for _, c := range []struct {
		in, want string
		ok       bool
	}{
		{"#eff1f5", "#eff1f5", true},
		{"#EFF1F5", "#eff1f5", true},
		{"#abc", "#aabbcc", true},
		{"eff1f5", "", false},
		{"reset", "", false},
		{"#eff1f", "", false},
	} {
		got, ok := hexColor(c.in)
		if ok != c.ok || got != c.want {
			t.Errorf("hexColor(%q) = %q,%v want %q,%v", c.in, got, ok, c.want, c.ok)
		}
	}
}

func TestCanonicalAliases(t *testing.T) {
	for _, c := range []struct{ in, want string }{
		{"catppuccin-mocha", "catppuccin"},
		{"Catppuccin-Latte", "catppuccin-latte"},
		{"LIGHT", "catppuccin-latte"},
	} {
		if got := canonical(c.in); got != c.want {
			t.Errorf("canonical(%q) = %q want %q", c.in, got, c.want)
		}
	}
}

func TestLoadSpecMissingFile(t *testing.T) {
	if s := LoadSpec("/nonexistent/herdr/config.toml"); !reflect.DeepEqual(s, Spec{}) {
		t.Errorf("missing file must give a zero Spec, got %+v", s)
	}
}

// LoadSpec must pick up the real user config on this machine (the gate that
// matters for "follows herdr").
func TestLoadSpecFindsUserConfig(t *testing.T) {
	path := HerdrConfigPath()
	if path == "" {
		t.Skip("no home to resolve")
	}
	s := LoadSpec(path)
	if s.Name == "" && s.Custom == nil {
		t.Skipf("no readable herdr config at %s", path)
	}
	if s.Name != "catppuccin-latte" {
		t.Errorf("expected the user's catppuccin-latte, got name=%q", s.Name)
	}
}
