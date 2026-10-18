// Popup "jump" mode: opens from the prefix+f [[keys.command]] popup binding.
// Type a sidebar code; a complete code focuses its target and the popup
// closes. Backspace edits, Esc (or ctrl+c) cancels.
package main

import (
	"fmt"
	"os"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/MykolaSuprun/herdr-preview-grid/internal/config"
	"github.com/MykolaSuprun/herdr-preview-grid/internal/herdr"
	"github.com/MykolaSuprun/herdr-preview-grid/internal/jump"
)

// fireDelay shows the resolved target for a beat before closing, so the
// jump reads as intentional.
const fireDelay = 180 * time.Millisecond

type firedMsg struct{}

type jumpModel struct {
	client  *herdr.Client
	entries []jump.Entry
	buf     string
	target  *jump.Entry
	fireErr error
}

func jumpMode() error {
	socket := os.Getenv("HERDR_SOCKET_PATH")
	if socket == "" {
		return herdr.ErrNotInHerdr
	}
	client := herdr.NewClient(socket)
	cfg := config.Load(configDir())

	// Opening the popup is a natural moment to re-assert the badges
	// (self-healing: covers any layout event the server never emitted).
	// Best-effort — resolution works even if reporting fails.
	entries, syncErr := jump.Sync(client, jump.LoadState(stateDir()), cfg.Charset)
	if len(entries) == 0 {
		if syncErr != nil {
			return syncErr
		}
		return nil
	}

	m := &jumpModel{client: client, entries: entries}
	if _, err := tea.NewProgram(m).Run(); err != nil {
		return err
	}
	return m.fireErr
}

func (m *jumpModel) Init() tea.Cmd { return nil }

func (m *jumpModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case firedMsg:
		return m, tea.Quit
	case tea.KeyPressMsg:
		switch msg.String() {
		case "esc", "ctrl+c":
			return m, tea.Quit
		case "backspace":
			if len(m.buf) > 0 {
				m.buf = m.buf[:len(m.buf)-1]
			}
			return m, nil
		}
		txt := printable(msg)
		if txt == "" {
			return m, nil
		}
		next := m.buf + txt
		entry, live := jump.Match(next, m.entries)
		switch {
		case entry != nil:
			m.buf = next
			m.target = entry
			m.fire()
			return m, tea.Tick(fireDelay, func(time.Time) tea.Msg { return firedMsg{} })
		case live:
			m.buf = next
		}
		return m, nil
	}
	return m, nil
}

// fire focuses the resolved target; a focus failure surfaces as the popup's
// exit error (visible in the plugin log when launched from the key binding).
func (m *jumpModel) fire() {
	switch m.target.Kind {
	case jump.KindWorkspace:
		m.fireErr = m.client.FocusWorkspace(m.target.ID)
	case jump.KindAgent:
		m.fireErr = m.client.FocusAgent(m.target.ID)
	}
}

func (m *jumpModel) View() tea.View {
	if m.target != nil {
		return tea.NewView(fmt.Sprintf("jump ▸ %s → %s", m.buf, m.target.Label))
	}
	return tea.NewView(fmt.Sprintf("jump ▸ %s\ncode from the sidebar · esc cancels", m.buf))
}

// printable returns the text a key press contributes, ignoring modified
// chords. (Twin of the same helper in internal/app; kept local so the popup
// stays self-contained.)
func printable(msg tea.KeyPressMsg) string {
	if msg.Mod.Contains(tea.ModCtrl) || msg.Mod.Contains(tea.ModAlt) {
		return ""
	}
	return msg.Text
}
