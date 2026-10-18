package config

import (
	"os"
	"path/filepath"
	"testing"
)

func write(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "config.toml"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestDefaultsWhenMissing(t *testing.T) {
	cfg := Load(t.TempDir())
	def := Default()
	if cfg != def {
		t.Fatalf("got %+v want %+v", cfg, def)
	}
}

func TestParsesValuesAndIgnoresUnknown(t *testing.T) {
	dir := write(t, `
# comment
charset = "abc"
preview_lines = 3
master_width = 34
preview = false
sort = "natural"
unknown_key = "whatever"
[section]
charset = "zzz"
`)
	cfg := Load(dir)
	if cfg.Charset != "abc" {
		t.Errorf("charset = %q", cfg.Charset)
	}
	if cfg.MasterWidth != 34 {
		t.Errorf("master_width = %d", cfg.MasterWidth)
	}
	if cfg.Preview {
		t.Errorf("preview should be false")
	}
	if cfg.Sort != "natural" {
		t.Errorf("sort = %q", cfg.Sort)
	}
}

func TestInvalidSortFallsBack(t *testing.T) {
	dir := write(t, "sort = \"bogus\"\n")
	if cfg := Load(dir); cfg.Sort != "priority" {
		t.Errorf("sort = %q, want default priority", cfg.Sort)
	}
}

// A zero/negative refresh_ms would re-tick instantly (busy loop) and anything
// under the floor is the same in practice; Load must normalize it.
func TestRefreshMSFlooredOrDefault(t *testing.T) {
	cases := map[string]int{
		"0":   1000, // would busy-loop → default
		"-5":  1000, // negative → default
		"50":  100,  // sub-floor → clamped
		"250": 250,  // valid → kept
	}
	for raw, want := range cases {
		dir := write(t, "refresh_ms = "+raw+"\n")
		if cfg := Load(dir); cfg.RefreshMS != want {
			t.Errorf("refresh_ms %s: got %d, want %d", raw, cfg.RefreshMS, want)
		}
	}
}

// A '#' inside a quoted value is part of the value, not a comment.
func TestParsesQuotedHashValue(t *testing.T) {
	dir := write(t, "charset = \"jk#l\"\n")
	if cfg := Load(dir); cfg.Charset != "jk#l" {
		t.Fatalf("charset = %q, want %q (quoted # truncated the value)", cfg.Charset, "jk#l")
	}
}

// Duplicate charset letters would assign one jump code to two rows; the
// config boundary dedupes (order preserved) before the table is built.
func TestCharsetDedupes(t *testing.T) {
	dir := write(t, "charset = \"jjkll\"\n")
	if cfg := Load(dir); cfg.Charset != "jkl" {
		t.Fatalf("charset = %q, want %q", cfg.Charset, "jkl")
	}
}
