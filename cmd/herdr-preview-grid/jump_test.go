package main

import (
	"strings"
	"testing"

	"github.com/MykolaSuprun/herdr-preview-grid/internal/codes"
	"github.com/MykolaSuprun/herdr-preview-grid/internal/jump"
)

// The popup's pure helpers (mode filtering, row formatting, clipping, the
// listing window) get direct coverage here; the Bubble Tea lifecycle itself
// is deliberately not driven — see main_test.go for the rationale.

func TestCodeFilterFollowsPrefixes(t *testing.T) {
	entries := []jump.Entry{
		{Code: "j", Kind: jump.KindWorkspace, Label: "alpha"},
		{Code: "k", Kind: jump.KindWorkspace, Label: "beta"},
		{Code: "uj", Kind: jump.KindTab, Label: "tab one"},
		{Code: "uk", Kind: jump.KindPane, Label: "pane one"},
	}
	if got := codeFilter(entries, ""); len(got) != 4 {
		t.Fatalf("empty buffer shows the full table, got %d rows", len(got))
	}
	got := codeFilter(entries, "u")
	if len(got) != 2 || got[0].Code != "uj" || got[1].Code != "uk" {
		t.Fatalf("live prefix filter = %+v", got)
	}
	if got := codeFilter(entries, "ux"); len(got) != 0 {
		t.Fatalf("dead-end filter = %+v, want empty", got)
	}
}

func TestSearchFilterMatchesLabelAndKind(t *testing.T) {
	entries := []jump.Entry{
		{Code: "j", Kind: jump.KindWorkspace, Label: "alpha"},
		{Code: "k", Kind: jump.KindTab, Label: "logs"},
		{Code: "l", Kind: jump.KindPane, Label: "tail -f"},
	}
	if got := searchFilter(entries, ""); len(got) != 3 {
		t.Fatalf("empty query shows all, got %d rows", len(got))
	}
	got := searchFilter(entries, "tab")
	if len(got) != 1 || got[0].Code != "k" {
		t.Fatalf("kind-word query \"tab\" = %+v, want the tab row", got)
	}
	got = searchFilter(entries, "tlf") // fuzzy subsequence across "tail -f"
	if len(got) != 1 || got[0].Label != "tail -f" {
		t.Fatalf("label fuzz \"tlf\" = %+v, want the pane row", got)
	}
}

func TestListRowFormatAndClip(t *testing.T) {
	e := jump.Entry{Code: "uj", Kind: jump.KindTab, Label: "w2 · logs"}
	row := listRow(e, 44, false)
	if !strings.HasPrefix(row, "  uj  tb ") || !strings.HasSuffix(row, "w2 · logs") {
		t.Fatalf("plain row = %q", row)
	}
	sel := listRow(e, 44, true)
	if !strings.HasPrefix(sel, "▌") {
		t.Fatalf("selected row should carry the cursor marker: %q", sel)
	}
	long := jump.Entry{Code: "uj", Kind: jump.KindPane, Label: strings.Repeat("x", 60)}
	if got := listRow(long, 20, false); herdrWidth(got) != 20 {
		t.Fatalf("clipped row width = %d, want 20 (%q)", herdrWidth(got), got)
	}
}

// herdrWidth is here only so the test file does not grow a second width
// helper; lipgloss.Width is the production path under test via clip.
func herdrWidth(s string) int { return len([]rune(s)) }

func TestClip(t *testing.T) {
	if got := clip("hello", 10); got != "hello" {
		t.Fatalf("fits = %q", got)
	}
	if got := clip("hello world", 10); got != "hello wor…" {
		t.Fatalf("ascii cut = %q", got)
	}
	if got := clip("日本語", 5); got != "日本…" { // CJK is two cells per rune
		t.Fatalf("wide-rune cut = %q", got)
	}
	if got := clip("x", 0); got != "" {
		t.Fatalf("zero budget = %q", got)
	}
}

func TestBodyRowsAndSelectionWindow(t *testing.T) {
	entries := make([]jump.Entry, 12)
	table := codes.Table(len(entries), "sdfjklerui")
	for i := range entries {
		entries[i] = jump.Entry{Code: string(table[i]), Kind: jump.KindPane, ID: string(table[i])}
	}
	m := &jumpModel{entries: entries, search: true, width: 44, height: 8}
	if m.bodyRows() != 6 {
		t.Fatalf("bodyRows = %d, want 6 (height 8 − header − footer)", m.bodyRows())
	}
	old := &jumpModel{height: 3}
	if old.bodyRows() != 1 {
		t.Fatalf("legacy 44x3 binding should still show one row, got %d", old.bodyRows())
	}

	// Cursor inside the window: no jump.
	m.sel = 5
	m.ensureVisible()
	if m.scroll != 0 {
		t.Fatalf("scroll moved for a visible row: %d", m.scroll)
	}
	// Cursor past the window: the window follows.
	m.sel = 10
	m.ensureVisible()
	if m.scroll != 5 {
		t.Fatalf("scroll = %d, want 5 (sel 10 + body 6 → first 5)", m.scroll)
	}
	m.sel = 1
	m.ensureVisible()
	if m.scroll != 1 {
		t.Fatalf("scroll = %d, want 1 (follow the cursor back)", m.scroll)
	}
}

func TestKindTag(t *testing.T) {
	for k, want := range map[jump.Kind]string{
		jump.KindWorkspace: "sp",
		jump.KindAgent:     "ag",
		jump.KindTab:       "tb",
		jump.KindPane:      "pn",
	} {
		if got := kindTag(k); got != want {
			t.Fatalf("kindTag(%s) = %q, want %q", k, got, want)
		}
	}
}
