package app

import (
	"fmt"
	"strings"
	"testing"

	"charm.land/lipgloss/v2"

	"github.com/MykolaSuprun/herdr-preview-grid/internal/codes"
	"github.com/MykolaSuprun/herdr-preview-grid/internal/config"
	"github.com/MykolaSuprun/herdr-preview-grid/internal/theme"
)

// latte is the pinned palette for helper-level assertions: the package-wide
// loader stub resolves Unknown appearance to latte, so every rendered hex
// below must match the historical values exactly.
var latte = newStyles(theme.Latte())

func TestStripANSI(t *testing.T) {
	in := "\x1b[31mred\x1b[0m plain \x1b]0;title\x07after"
	got := stripANSI(in)
	if strings.ContainsAny(got, "\x1b]") {
		t.Fatalf("escapes remain: %q", got)
	}
	if !strings.Contains(got, "red") || !strings.Contains(got, "plain") || !strings.Contains(got, "after") {
		t.Fatalf("text lost: %q", got)
	}
}

func TestStripNonSGR(t *testing.T) {
	in := "\x1b[31mred\x1b[0m \x1b[2J\x1b[Hmove \x1b]0;title\x07 \x1b[38;5;123m256\x1b[0m \x1b[?25l"
	got := stripNonSGR(in)
	// SGR sequences survive.
	if !strings.Contains(got, "\x1b[31m") || !strings.Contains(got, "\x1b[38;5;123m") || !strings.Contains(got, "\x1b[0m") {
		t.Fatalf("SGR lost: %q", got)
	}
	// Cursor/erase/OSC/private-mode sequences are dropped.
	for _, banned := range []string{"\x1b[2J", "\x1b[H", "\x1b]0;", "\x1b[?25l"} {
		if strings.Contains(got, banned) {
			t.Fatalf("non-SGR escape kept: %q in %q", banned, got)
		}
	}
	if !strings.Contains(got, "red") || !strings.Contains(got, "move") || !strings.Contains(got, "256") {
		t.Fatalf("text lost: %q", got)
	}
}

func TestTruncateANSIPlainInput(t *testing.T) {
	// truncateANSI is now the single truncator: escape-free input must keep
	// the plain-truncation semantics.
	if got := truncateANSI("hello", 10); got != "hello" {
		t.Errorf("no-op: %q", got)
	}
	got := truncateANSI("hello world", 6)
	if got != "hello…" {
		t.Errorf("got %q", got)
	}
	if truncateANSI("abc", 0) != "" {
		t.Errorf("zero width should be empty")
	}
}

func TestTruncateANSI(t *testing.T) {
	// Fits: sequence passes through untouched.
	in := "\x1b[31mhi\x1b[0m"
	if got := truncateANSI(in, 10); got != in {
		t.Errorf("no-op cut changed content: %q", got)
	}
	// Cut: printable budget counts text only; escapes are never split and
	// display width stays within budget.
	in = "\x1b[31mhello\x1b[0m world"
	got := truncateANSI(in, 6)
	if !strings.Contains(got, "hello") || !strings.HasSuffix(got, "…") {
		t.Fatalf("cut content wrong: %q", got)
	}
	if w := lipgloss.Width(got); w != 6 {
		t.Errorf("display width = %d, want 6", w)
	}
	// An escape right at the cut boundary is not truncated mid-sequence.
	in = "ab\x1b[31mcd"
	got = truncateANSI(in, 3)
	if strings.Count(got, "\x1b[") != 1 || !strings.HasSuffix(got, "…") {
		t.Fatalf("escape atomicity broken: %q", got)
	}
}

func TestStatusColor(t *testing.T) {
	// Latte-family semantic colors (2026-10-09 swap; mocha hexes washed out
	// on the user's light terminal).
	cases := map[string]string{
		"working": "#fe640b", "WORKING": "#fe640b", " working ": "#fe640b",
		"blocked": "#d20f39", "done": "#40a02b", "idle": "#1e66f5",
		"": "#6c6f85", "unknown": "#6c6f85", "zzz": "#6c6f85",
	}
	for in, want := range cases {
		if got := latte.statusColor(in); got != want {
			t.Errorf("latte.statusColor(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestAccentForDeterminism(t *testing.T) {
	// Palette sanity: unique, saturated latte-family hues.
	seen := map[string]bool{}
	for _, c := range theme.Latte().Accents {
		if seen[c] {
			t.Errorf("duplicate accent hue %q", c)
		}
		seen[c] = true
	}
	a := Item{Key: "workspace:w15"}
	b := Item{Key: "workspace:w15"}
	c := Item{Key: "workspace:w16"}
	if latte.accentFor(a) != latte.accentFor(b) {
		t.Errorf("same key must map to same accent")
	}
	if latte.accentFor(a) == latte.accentFor(c) {
		// Not a hard requirement (hash collision), but 14 hues make it
		// vanishingly unlikely for different keys; guards accidental constant.
		t.Logf("w15 and w16 share accent %q", latte.accentFor(a))
	}
	for _, k := range []string{"workspace:w1", "agent:wA:p2T", "workspace:wZ"} {
		if !strings.HasPrefix(latte.accentFor(Item{Key: k}), "#") {
			t.Errorf("accent not a hex color for %q", k)
		}
	}
}

func TestAnsiSafe(t *testing.T) {
	if got := ansiSafe("plain"); got != "plain" {
		t.Errorf("plain changed: %q", got)
	}
	got := ansiSafe("\x1b[31mred\x1b[0m")
	if !strings.HasSuffix(got, "\x1b[0m") || strings.Count(got, "\x1b[0m") != 1 {
		t.Errorf("reset not appended exactly once: %q", got)
	}
	// A line ending in color WITHOUT reset gets one.
	got = ansiSafe("\x1b[31mred")
	if !strings.HasSuffix(got, "\x1b[0m") {
		t.Errorf("unterminated color not reset: %q", got)
	}
}

func TestTailLines(t *testing.T) {
	in := "a\nb\nc\nd\n"
	got := tailLines(in, 2)
	if len(got) != 2 || got[0] != "c" || got[1] != "d" {
		t.Fatalf("got %v", got)
	}
	if tailLines(in, 0) != nil {
		t.Fatalf("zero should be nil")
	}
}

func TestPreviewBodySqueeze(t *testing.T) {
	in := "\n\n\nhello\n\n\n\nworld\n\n\n"
	// Squeeze: leading/trailing blanks trimmed, blank runs collapsed to one.
	got := previewBody(in, 10, true)
	want := []string{"hello", "", "world"}
	if len(got) != len(want) {
		t.Fatalf("got %q, want %q", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("line %d: got %q, want %q", i, got[i], want[i])
		}
	}
	// Content beyond n is dropped from the top (tail wins, opening blanks
	// re-trimmed so the tile starts on content).
	got = previewBody("x\n\ny\n\nz\n", 2, true)
	if len(got) != 1 || got[0] != "z" {
		t.Fatalf("tail-squeeze: got %q", got)
	}
	// ANSI-only "blank" lines count as blank.
	got = previewBody("\x1b[31m   \x1b[0m\na\n", 5, true)
	if len(got) != 1 || got[0] != "a" {
		t.Fatalf("ansi blank: got %q", got)
	}
	// Colored content lines keep their sequences through the squeeze;
	// interior blank separators stay (one per run).
	got = previewBody("\x1b[32mgreen\x1b[0m\n\n\nb\n", 5, true)
	if len(got) != 3 || got[0] != "\x1b[32mgreen\x1b[0m" || got[1] != "" || got[2] != "b" {
		t.Fatalf("color lost in squeeze: %q", got)
	}
	// Without squeeze it is the plain tail (trailing blank lines are
	// trimmed upstream by TrimRight; interior blanks stay).
	got = previewBody("a\n\n\nb\n\n", 3, false)
	if len(got) != 3 || got[0] != "" || got[1] != "" || got[2] != "b" {
		t.Fatalf("no-squeeze tail: got %q", got)
	}
}

// --- master-detail layout/render ---------------------------------------------

func testModel() Model {
	return Model{
		mode: ModeSpaces,
		cfg: config.Config{
			Charset:           "sdfjklerui",
			SqueezeBlankLines: true,
			Sort:              "priority",
			Preview:           true,
		},
		width:  120,
		height: 30,
		ready:  true,
		items: []Item{
			{Key: "workspace:wA", Label: "n1-build", Number: 1, Cwd: "~/src/build", Status: "idle", PaneID: "wA:p1", Target: "wA", Kind: "space", Focused: true},
			{Key: "workspace:wN", Label: "nixvim-config", Number: 2, Cwd: "~/src/nixvim", Status: "working", PaneID: "wN:p1", Target: "wN", Kind: "space"},
		},
		table:    []codes.Code{"j", "k"},
		previews: map[string]string{"wA:p1": "line1\nline2\nline3"},
	}
}

func TestLayoutSplit(t *testing.T) {
	m := testModel()
	l := m.layout()
	if l.listW+1+l.detailW != m.width {
		t.Fatalf("columns do not fill width: %d+1+%d != %d", l.listW+1+l.detailW, m.width, m.width)
	}
	if l.bodyH != m.height-2 {
		t.Fatalf("bodyH = %d, want %d", l.bodyH, m.height-2)
	}
	// Auto width: clamped thirds of the terminal.
	if got := (Model{width: 200, height: 30}).layout().listW; got != listMaxW {
		t.Errorf("200 cols: listW = %d, want %d", got, listMaxW)
	}
	if got := (Model{width: 60, height: 30}).layout().listW; got != listMinW {
		t.Errorf("60 cols: listW = %d, want %d", got, listMinW)
	}
	// Explicit master_width wins.
	m.cfg.MasterWidth = 30
	if got := m.layout().listW; got != 30 {
		t.Errorf("explicit: listW = %d, want 30", got)
	}
}

func TestDetailSizing(t *testing.T) {
	m := testModel()
	if got, want := m.detailLines(), m.height-2-3; got != want {
		t.Errorf("detailLines = %d, want %d", got, want)
	}
	if got, want := m.previewReadLines(), m.detailLines()+8; got != want {
		t.Errorf("previewReadLines = %d, want %d (squeeze headroom)", got, want)
	}
}

func TestRenderListRow(t *testing.T) {
	m := testModel()
	l := m.layout()
	row := m.renderListRow(0, l.listW)
	if lipgloss.Width(row) != l.listW {
		t.Fatalf("selected row width = %d, want %d", lipgloss.Width(row), l.listW)
	}
	if !strings.Contains(row, "▌") {
		t.Errorf("selected row missing marker: %q", row)
	}
	// The number lives in its own column; the label stands alone.
	if !strings.Contains(row, "n1-build") || strings.Contains(row, "1 n1-build") {
		t.Errorf("row label wrong: %q", row)
	}
	if !strings.Contains(row, "~/src/build") {
		t.Errorf("cwd segment missing: %q", row)
	}
	// The wash rides on the selected row only, as a truecolor background
	// fragment (lipgloss rewrites the hex into SGR parameters).
	w0, w1 := latte.washColor(latte.accentFor(m.items[0])), latte.washColor(latte.accentFor(m.items[1]))
	r, g, b, _ := splitHexColor(w0)
	frag := fmt.Sprintf("48;2;%d;%d;%d", r, g, b)
	if !strings.Contains(row, frag) {
		t.Errorf("selected row missing wash background: %q", row)
	}
	row2 := m.renderListRow(1, l.listW)
	if lipgloss.Width(row2) != l.listW {
		t.Fatalf("unselected row width = %d, want %d", lipgloss.Width(row2), l.listW)
	}
	if strings.Contains(row2, "▌") {
		t.Errorf("unselected row has marker: %q", row2)
	}
	if w0 != w1 && strings.Contains(row2, frag) {
		t.Errorf("wash leaked to unselected row: %q", row2)
	}
}

func TestRenderDetail(t *testing.T) {
	m := testModel()
	l := m.layout()
	out := m.renderDetail(l)
	if got := strings.Count(out, "\n") + 1; got != l.bodyH {
		t.Fatalf("detail height = %d lines, want %d", got, l.bodyH)
	}
	if !strings.Contains(out, "n1-build") {
		t.Errorf("title missing label: %q", out)
	}
	if !strings.Contains(out, "line3") {
		t.Errorf("preview content missing: %q", out)
	}
}

func TestRenderDetailNoPane(t *testing.T) {
	m := testModel()
	m.items = m.items[1:] // single item, no pane
	m.items[0].PaneID = ""
	m.table = []codes.Code{"k"}
	out := m.renderDetail(m.layout())
	if !strings.Contains(out, "no pane") {
		t.Errorf("missing no-pane hint: %q", out)
	}
}

// --- master-row columns ------------------------------------------------------

// Column order is number < label < cwd, and the pinned rail renders as
// status dot + gap + focused flag with a blank flag slot when unfocused.
func TestRowColumns(t *testing.T) {
	m := testModel()
	l := m.layout()

	plain := stripANSI(m.renderListRow(0, l.listW))
	pi, li, ci := strings.Index(plain, "1 "), strings.Index(plain, "n1-build"), strings.Index(plain, "~/src/build")
	if pi < 0 || li < 0 || ci < 0 || pi >= li || li >= ci {
		t.Fatalf("column order broken (num < label < cwd): %q", plain)
	}
	if !strings.HasSuffix(plain, "● ●") {
		t.Fatalf("focused rail wrong: %q", plain)
	}

	plain1 := stripANSI(m.renderListRow(1, l.listW))
	if !strings.HasSuffix(plain1, "●  ") {
		t.Fatalf("unfocused rail wrong (blank flag slot expected): %q", plain1)
	}
}

// The cwd segment yields to the label when the row is too narrow for both.
func TestRowCwdDropsWhenCramped(t *testing.T) {
	m := testModel()
	l := m.layout()
	m.items[0].Label = strings.Repeat("labelish", 6) // wider than the label zone
	m.items[0].Cwd = "~/very/long/path"
	plain := stripANSI(m.renderListRow(0, l.listW))
	if strings.Contains(plain, "~/very/long/path") {
		t.Fatalf("cwd kept beside an overlong label: %q", plain)
	}
	if !strings.Contains(plain, "…") {
		t.Fatalf("visible label not truncated: %q", plain)
	}
}

// Header shows the attention tally only when there is one.
func TestHeaderAttentionCount(t *testing.T) {
	m := testModel()
	m.items[0].AwaitingInput = true
	if out := m.render(); !strings.Contains(out, "1 attention") {
		t.Fatalf("attention count missing: %q", out)
	}
	if out := testModel().render(); strings.Contains(out, "attention") {
		t.Fatalf("zero attention shown: %q", out)
	}
}

// A 4-digit live number widens the number column instead of spilling into
// the rail; the row still pads to exactly listW.
func TestNumberColumnDataSized(t *testing.T) {
	m := testModel()
	m.items[0].Number = 1234
	l := m.layout()
	if lipgloss.Width(m.renderListRow(0, l.listW)) != l.listW {
		t.Fatal("row not exact width with a 4-digit number")
	}
	if plain := stripANSI(m.renderListRow(0, l.listW)); !strings.Contains(plain, "1234") {
		t.Fatalf("4-digit number missing: %q", plain)
	}
}

// The wash hex is stable, distinct from base and accent, and falls back.
func TestWashColor(t *testing.T) {
	c := latte.washColor("#8839ef")
	if len(c) != 7 || c[0] != '#' {
		t.Fatalf("not hex: %q", c)
	}
	if c == "#8839ef" || c == "#eff1f5" {
		t.Fatalf("degenerate wash: %q", c)
	}
	if again := latte.washColor("#8839ef"); again != c {
		t.Fatalf("unstable wash: %q vs %q", again, c)
	}
	// Exact pin: latte base with 12% of #209fb5 — guards the blend
	// direction (an inverted helper renders a saturated row).
	if got := latte.washColor("#209fb5"); got != "#d6e7ed" {
		t.Fatalf("latte.washColor(#209fb5) = %q, want #d6e7ed", got)
	}
	if got := latte.washColor("zzz"); got != "#eff1f5" {
		t.Fatalf("bad accent should fall back to the base: %q", got)
	}
}

// The badge background keeps its hue at ~45% intensity: latte base with 55%
// of #209fb5 — readable, and pinned exactly so the blend can't flip.
func TestChipBgSoftensAccent(t *testing.T) {
	if got := latte.chipBg("#209fb5"); got != "#92ccd8" {
		t.Fatalf("latte.chipBg(#209fb5) = %q, want #92ccd8", got)
	}
	if got := latte.chipBg("zzz"); got != "#eff1f5" {
		t.Fatalf("bad accent = %q, want base", got)
	}
}

// The badge is exactly as wide as the code — every glyph in its own cell,
// no lopsided colored blanks — emitted as one uninterrupted color run.
func TestChipInkTight(t *testing.T) {
	const accent = "#209fb5"
	cr, cg, cb, _ := splitHexColor(latte.chipBg(accent))
	chipFrag := fmt.Sprintf("48;2;%d;%d;%d", cr, cg, cb)

	for _, tt := range []struct {
		code  string
		wantW int
	}{
		{"j", 1},
		{"uj", 2},
		{"uuu", 3},
	} {
		got := latte.chips(tt.code, accent)
		if lipgloss.Width(got) != tt.wantW {
			t.Fatalf("latte.chips(%q) width = %d, want %d", tt.code, lipgloss.Width(got), tt.wantW)
		}
		if n := strings.Count(got, chipFrag); n != 1 {
			t.Fatalf("latte.chips(%q) = %d color runs, want 1 (one uninterrupted block)", tt.code, n)
		}
	}
}

// C0 controls (BEL/BS/VT/FF/DEL) corrupt the fixed panel like cursor moves
// do: dropped; tab expands; SGR sequences and their text survive; \r drops.
func TestCleanLineANSIDropsC0(t *testing.T) {
	in := "a\x07b\x08c\td\x0be\x0cf\x7fg\x1b[31mred\x1b[0m\rh"
	want := "abc    defg\x1b[31mred\x1b[0mh"
	if got := cleanLineANSI(in); got != want {
		t.Fatalf("cleanLineANSI = %q, want %q", got, want)
	}
}
