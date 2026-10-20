package app

import (
	"image/color"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/MykolaSuprun/herdr-preview-grid/internal/config"
	"github.com/MykolaSuprun/herdr-preview-grid/internal/mru"
	"github.com/MykolaSuprun/herdr-preview-grid/internal/theme"
)

// Stub the herdr-config loader for the WHOLE package: pinned-colour tests
// must never depend on the developer's own herdr config (a dark-themed
// machine would flip every hex assertion). Zero Spec + Unknown appearance
// resolves to latte — the historical palette.
func init() {
	loadHerdrSpec = func(string) theme.Spec { return theme.Spec{} }
}

// latteBase/darkMocha as color.Color values for BackgroundColorMsg.
var (
	latteBG = color.RGBA{R: 0xef, G: 0xf1, B: 0xf5, A: 0xff}
	mochaBG = color.RGBA{R: 0x1e, G: 0x1e, B: 0x2e, A: 0xff}
)

func newTestModel(themeSetting string) Model {
	cfg := config.Default()
	cfg.Theme = themeSetting
	return New(ModeSpaces, cfg, nil, mru.Load(""), "", "", "")
}

func TestNewResolvesLatteByDefault(t *testing.T) {
	m := newTestModel("follow")
	if m.pal.Base != "#eff1f5" {
		t.Errorf("zero herdr spec + unknown appearance must keep latte, got %s", m.pal.Base)
	}
	if m.pal.ChipFg != "#1e1e2e" {
		t.Errorf("latte chips must carry dark text, got %s", m.pal.ChipFg)
	}
}

func TestBackgroundColorMsgFlipsPalette(t *testing.T) {
	// herdr config: auto_switch pair latte↔mocha, exactly like the user's.
	restore := loadHerdrSpec
	loadHerdrSpec = func(string) theme.Spec {
		return theme.Spec{
			Name: "catppuccin-latte", AutoSwitch: true,
			LightName: "catppuccin-latte", DarkName: "catppuccin",
		}
	}
	defer func() { loadHerdrSpec = restore }()

	m := newTestModel("follow")
	if m.pal.Base != "#eff1f5" {
		t.Fatalf("startup must keep the `name` theme (latte), got %s", m.pal.Base)
	}
	after, _ := m.Update(tea.BackgroundColorMsg{Color: mochaBG})
	flipped := after.(Model)
	if flipped.pal.Base != "#1e1e2e" {
		t.Errorf("dark appearance must flip to mocha, got %s", flipped.pal.Base)
	}
	if flipped.pal.ChipFg != "#eff1f5" {
		t.Errorf("dark chips must carry light text, got %s", flipped.pal.ChipFg)
	}
	// A dark accent keeps its hue family: same index in both palettes.
	if got := flipped.s().accentFor(dummyItem("x")); got == "" {
		t.Error("accent resolution broke after flip")
	}

	// And back to light: idempotent flips both ways.
	after2, _ := flipped.Update(tea.BackgroundColorMsg{Color: latteBG})
	if m2 := after2.(Model); m2.pal.Base != "#eff1f5" {
		t.Errorf("light appearance must flip back to latte, got %s", m2.pal.Base)
	}
}

func TestPluginForcedThemeIgnoresAppearance(t *testing.T) {
	restore := loadHerdrSpec
	loadHerdrSpec = func(string) theme.Spec {
		return theme.Spec{Name: "catppuccin", AutoSwitch: true, DarkName: "catppuccin"}
	}
	defer func() { loadHerdrSpec = restore }()

	m := newTestModel("latte")
	if m.pal.Base != "#eff1f5" {
		t.Fatalf("forced latte must win at startup, got %s", m.pal.Base)
	}
	after, _ := m.Update(tea.BackgroundColorMsg{Color: mochaBG})
	if m2 := after.(Model); m2.pal.Base != "#eff1f5" {
		t.Errorf("forced latte must ignore dark appearance, got %s", m2.pal.Base)
	}
}

func dummyItem(key string) Item { return Item{Key: key, Label: key} }
