package app

import (
	"testing"
	"unicode/utf8"

	tea "charm.land/bubbletea/v2"

	"github.com/MykolaSuprun/herdr-preview-grid/internal/config"
	"github.com/MykolaSuprun/herdr-preview-grid/internal/mru"
)

// Regression for the review finding: ctrl+c used to be treated like esc and
// only cleared an active search, contradicting the documented "ctrl+c closes".
func TestCtrlCAlwaysQuits(t *testing.T) {
	m := New(ModeSpaces, config.Default(), nil, mru.Load(""), "", "", "")
	m.filterActive = true

	_, cmd := m.handleKey(tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl})
	if cmd == nil {
		t.Fatal("ctrl+c mid-search returned no command")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Fatalf("cmd = %T, want tea.QuitMsg", cmd())
	}
}

// esc keeps the documented clear-search-first behavior before closing.
func TestEscClearsSearchThenCloses(t *testing.T) {
	m := New(ModeSpaces, config.Default(), nil, mru.Load(""), "", "", "")
	m.filterActive = true
	m.search = "x"

	m2, cmd := m.handleKey(tea.KeyPressMsg{Code: tea.KeyEsc})
	if cmd != nil {
		t.Fatal("esc with an active filter should clear it, not close")
	}
	next := m2.(Model)
	if next.filterActive || next.search != "" {
		t.Fatalf("esc should clear filter + search, got filterActive=%v search=%q", next.filterActive, next.search)
	}

	_, cmd = next.handleKey(tea.KeyPressMsg{Code: tea.KeyEsc})
	if cmd == nil || cmd() != (tea.QuitMsg{}) {
		t.Fatal("esc without a filter should close")
	}
}

// Backspace must drop one whole rune, not one byte: multi-byte search input
// used to leave invalid UTF-8 behind.
func TestBackspaceDropsWholeRune(t *testing.T) {
	m := New(ModeSpaces, config.Default(), nil, mru.Load(""), "", "", "")
	m.filterActive = true
	m.search = "выход"

	m2, _ := m.handleKey(tea.KeyPressMsg{Code: tea.KeyBackspace})
	next := m2.(Model)
	if !utf8.ValidString(next.search) {
		t.Fatalf("search buffer corrupted: %q", next.search)
	}
	if next.search != "выхо" {
		t.Fatalf("search = %q, want %q", next.search, "выхо")
	}
}
