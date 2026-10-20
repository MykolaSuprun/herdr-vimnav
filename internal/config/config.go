// Package config loads the plugin's user config: a flat key = value file at
// $HERDR_PLUGIN_CONFIG_DIR/config.toml. Only the keys this plugin owns are
// read; unknown keys are ignored so the file can grow without breaking.
package config

import (
	"bufio"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// Config is the resolved plugin configuration.
type Config struct {
	Charset           string // jump-code letters
	MasterWidth       int    // master-list width in columns; 0 = auto (~1/3, clamp 28-42)
	SqueezeBlankLines bool   // collapse blank-line runs in previews to fit more content
	RefreshMS         int    // snapshot refresh interval
	Sort              string // "priority" (default) | "usage" | "natural"
	Preview           bool   // show live pane previews
	Theme             string // "follow" (default: mirror herdr's theme) | "latte" | "mocha"
}

// Default returns the defaults used when no config file exists.
func Default() Config {
	return Config{
		Charset:           "jklfds",
		MasterWidth:       0,
		SqueezeBlankLines: true,
		RefreshMS:         1000,
		Sort:              "priority",
		Preview:           true,
		Theme:             "follow",
	}
}

// pluginID is this plugin's id in herdr's registry (config/state dir naming).
const pluginID = "preview-grid"

// DefaultDir returns the standard plugin config dir
// (~/.config/herdr/plugins/config/preview-grid). Hooks and panes receive
// HERDR_PLUGIN_CONFIG_DIR; this fallback covers CLI runs.
func DefaultDir() string {
	base := os.Getenv("XDG_CONFIG_HOME")
	if base == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return ""
		}
		base = filepath.Join(home, ".config")
	}
	return filepath.Join(base, "herdr", "plugins", "config", pluginID)
}

// Path returns the config file location for a plugin config dir.
func Path(configDir string) string { return filepath.Join(configDir, "config.toml") }

// Load reads the config file, applying defaults for missing keys.
func Load(configDir string) Config {
	cfg := Default()
	if configDir == "" {
		return cfg
	}
	f, err := os.Open(Path(configDir))
	if err != nil {
		return cfg
	}
	defer func() { _ = f.Close() }() // read-only path

	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := sc.Text()
		if trimmed := strings.TrimSpace(line); strings.HasPrefix(trimmed, "[") {
			break // all owned keys are top-level; sections belong to other tools
		}
		key, value, ok := parseLine(line)
		if !ok {
			continue
		}
		// Tile-era keys (preview_lines, preview_scale, tile_min_width,
		// code_timeout_ms) deliberately have no arms: unknown keys fall
		// through, so old config files keep parsing without dormant fields.
		switch key {
		case "charset":
			if value != "" {
				cfg.Charset = value
			}
		case "master_width":
			cfg.MasterWidth = atoi(value, cfg.MasterWidth)
		case "squeeze_blank_lines":
			cfg.SqueezeBlankLines = value == "true" || value == "1"
		case "refresh_ms":
			cfg.RefreshMS = atoi(value, cfg.RefreshMS)
		case "sort":
			switch value {
			case "priority", "usage", "natural":
				cfg.Sort = value
			}
		case "preview":
			cfg.Preview = value == "true" || value == "1"
		case "theme":
			switch value {
			case "follow", "herdr", "latte", "mocha":
				cfg.Theme = value
			}
		}
	}
	if cfg.Charset == "" {
		cfg.Charset = Default().Charset
	}
	cfg.Charset = dedupeCharset(cfg.Charset)
	// A zero/negative refresh re-ticks instantly (busy loop); anything under
	// 100ms is one too. Fall back to the default / floor the value.
	if cfg.RefreshMS <= 0 {
		cfg.RefreshMS = Default().RefreshMS
	} else if cfg.RefreshMS < 100 {
		cfg.RefreshMS = 100
	}
	return cfg
}

// parseLine parses `key = value` with optional quotes and # comments. A '#'
// inside quotes is part of the value, not a comment. All keys this plugin
// owns are top-level; section headers end parsing.
func parseLine(line string) (key, value string, ok bool) {
	if i := commentIndex(line); i >= 0 {
		line = line[:i]
	}
	line = strings.TrimSpace(line)
	if line == "" || line[0] == '[' {
		return "", "", false
	}
	eq := strings.IndexByte(line, '=')
	if eq < 0 {
		return "", "", false
	}
	key = strings.TrimSpace(line[:eq])
	value = strings.TrimSpace(line[eq+1:])
	value = strings.Trim(value, `"'`)
	return key, value, key != ""
}

// commentIndex finds the start of a # comment, ignoring '#' inside double
// or single quotes (a value may legitimately contain one).
func commentIndex(line string) int {
	var quote rune
	for i, r := range line {
		switch {
		case quote != 0:
			if r == quote {
				quote = 0
			}
		case r == '"' || r == '\'':
			quote = r
		case r == '#':
			return i
		}
	}
	return -1
}

// dedupeCharset keeps the first occurrence of each rune, order preserved.
// Duplicate letters would assign the same jump code to two rows (the second
// unreachable) — the prefix-free table assumes a unique letter set.
func dedupeCharset(charset string) string {
	seen := map[rune]bool{}
	var b strings.Builder
	for _, r := range charset {
		if seen[r] {
			continue
		}
		seen[r] = true
		b.WriteRune(r)
	}
	return b.String()
}

func atoi(s string, def int) int {
	n, err := strconv.Atoi(s)
	if err != nil {
		return def
	}
	return n
}
