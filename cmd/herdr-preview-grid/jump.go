// Popup "jump" mode: opens from a [[keys.command]] popup binding (prefix+f,
// or a direct chord such as ctrl+shift+e — see README Keybindings). The
// popup lists every jumpable target — spaces, agents, tabs, and plain
// (non-agent) panes — with its prefix-free code; a complete code fires
// immediately. '/' opens a fuzzy label search (↑/↓ or pgup/pgdn to walk,
// enter to commit); backspace edits; esc (or ctrl+c) cancels, clearing the
// search first.
//
// Spaces and agents carry exactly the codes their sidebar badges show;
// tabs and panes continue the same prefix-free sequence (the table only
// grows after the badge rows, so badges are stable across popup opens).
package main

import (
	"fmt"
	"os"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/sahilm/fuzzy"

	"github.com/MykolaSuprun/herdr-preview-grid/internal/config"
	"github.com/MykolaSuprun/herdr-preview-grid/internal/herdr"
	"github.com/MykolaSuprun/herdr-preview-grid/internal/jump"
)

// fireDelay shows the resolved target for a beat before closing, so the
// jump reads as intentional.
const fireDelay = 180 * time.Millisecond

// Popup size until the terminal reports its real dimensions (herdr derives
// these from the binding's width/height): the historically documented box.
const defaultWidth, defaultHeight = 44, 3

type firedMsg struct{}

type jumpModel struct {
	client  *herdr.Client
	entries []jump.Entry
	tags    *tagState

	buf    string // code buffer (code mode)
	query  string // search text (search mode)
	search bool

	sel    int  // selected row in search mode (index into the filtered rows)
	scroll int  // first visible listing row (filtered-row index)
	start  bool // first WindowSizeMsg seen

	width, height int

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
	// (self-healing: covers any layout event the server never emitted) and
	// to rebuild the full table — spaces, agents, tabs, and panes — from
	// the live layout. Badge/table failures degrade to whatever sections
	// did resolve: the popup only dies when there is nothing to jump to.
	entries, err := jump.Collect(client, jump.LoadState(stateDir()), cfg.Charset)
	if len(entries) == 0 {
		if err != nil {
			return err
		}
		return nil
	}

	// Tag the active space's tabs with their codes while the popup is open:
	// the jump chord IS the trigger (ctrl+shift+e), so the codes appear on
	// the tabs the moment you open the picker. Any fired code restores the
	// labels (see fire), and closing without firing restores them too —
	// herdr 0.9.5 has no tab-metadata surface, so a label rewrite is the
	// only way a code can live on a tab (see tabtags.go).
	//
	// Unwind first: a previous popup that was killed (no chance to restore)
	// leaves its ledger on disk, and tagging over a stale tag would bury
	// the user's real label permanently.
	tags := loadTagState(stateDir())
	clearTags(client, tags)
	// Re-read the table AFTER unwinding so labels are the user's originals,
	// never the previous open's tags ("wl 1" must not become "wl wl 1").
	if fresh, ferr := jump.Collect(client, jump.LoadState(stateDir()), cfg.Charset); ferr == nil || len(fresh) > 0 {
		entries = fresh
	}
	_ = applyTags(client, tags, cfg.Charset)
	defer clearTags(client, tags) // survives panics and error returns

	m := &jumpModel{client: client, entries: entries, tags: tags, width: defaultWidth, height: defaultHeight}
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
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.start = true
		m.ensureVisible()
		return m, nil
	case tea.KeyPressMsg:
		return m.key(msg)
	}
	return m, nil
}

func (m *jumpModel) key(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c": // unconditional quit, even mid-search
		return m, tea.Quit
	case "esc":
		if m.search {
			m.search = false
			m.query = ""
			m.reset()
			return m, nil
		}
		return m, tea.Quit
	case "backspace":
		if m.search {
			r := []rune(m.query)
			if len(r) > 0 {
				m.query = string(r[:len(r)-1])
				m.reset()
			}
		} else if len(m.buf) > 0 {
			m.buf = m.buf[:len(m.buf)-1]
			m.reset()
		}
		return m, nil
	case "/":
		if !m.search {
			m.search = true
			m.query = ""
			m.buf = "" // modes are exclusive: entering search abandons the buffer
			m.reset()
		}
		return m, nil
	case "enter":
		if m.search {
			rows := m.rows()
			if m.sel < len(rows) {
				return m.fireEntry(rows[m.sel])
			}
		}
		return m, nil
	case "up":
		return m.step(-1)
	case "down":
		return m.step(1)
	case "pgup", "ctrl+u":
		return m.step(-m.bodyRows())
	case "pgdn", "ctrl+d":
		return m.step(m.bodyRows())
	case "home":
		if m.search {
			m.sel = 0
			m.ensureVisible()
		}
		return m, nil
	case "end":
		if m.search {
			if n := len(m.rows()); n > 0 {
				m.sel = n - 1
				m.ensureVisible()
			}
		}
		return m, nil
	}

	txt := printable(msg)
	if txt == "" {
		return m, nil
	}
	if m.search {
		m.query += txt
		m.reset()
		return m, nil
	}
	next := m.buf + txt
	entry, live := jump.Match(next, m.entries)
	switch {
	case entry != nil:
		m.buf = next
		return m.fireEntry(*entry)
	case live:
		m.buf = next
		m.reset()
	}
	return m, nil
}

// step moves the search cursor (search mode) or scrolls the listing window
// (code mode — the buffer follows typing instead of a cursor).
func (m *jumpModel) step(delta int) (tea.Model, tea.Cmd) {
	rows := m.rows()
	if len(rows) == 0 {
		m.sel, m.scroll = 0, 0
		return m, nil
	}
	if m.search {
		m.sel = clamp(m.sel+delta, 0, len(rows)-1)
	} else {
		m.scroll = clamp(m.scroll+delta, 0, len(rows)-1)
	}
	m.ensureVisible()
	return m, nil
}

// reset re-collapses the selection after the row set changed (edit, mode
// switch) — the top match is the natural new focus — and clamps the window.
func (m *jumpModel) reset() {
	m.sel = 0
	m.scroll = 0
	m.ensureVisible()
}

// ensureVisible keeps the selected row inside the listing window and the
// window inside the row set. Code mode has no cursor; the scroll stays
// clamped so the window can never fall off the filtered rows.
func (m *jumpModel) ensureVisible() {
	body := m.bodyRows()
	rows := m.rows()
	maxFirst := max(0, len(rows)-1)
	if m.search {
		maxFirst = max(0, len(rows)-body)
		if m.sel < m.scroll {
			m.scroll = m.sel
		}
		if m.sel >= m.scroll+body {
			m.scroll = m.sel - body + 1
		}
	}
	m.scroll = clamp(m.scroll, 0, maxFirst)
}

// rows returns the entries visible in the current mode: code mode narrows
// to live-prefix continuations of the code buffer (all of them when empty),
// search mode fuzzy-matches label+kind+code.
func (m *jumpModel) rows() []jump.Entry {
	if m.search {
		return searchFilter(m.entries, m.query)
	}
	return codeFilter(m.entries, m.buf)
}

// codeFilter narrows entries to the code-continuations of buf (every entry
// when the buffer is empty).
func codeFilter(entries []jump.Entry, buf string) []jump.Entry {
	if buf == "" {
		return entries
	}
	out := make([]jump.Entry, 0, len(entries))
	for i := range entries {
		if strings.HasPrefix(entries[i].Code, buf) {
			out = append(out, entries[i])
		}
	}
	return out
}

// searchFilter fuzzy-matches the query against each entry's label, kind,
// and code (kind words like "tab"/"pane" make kind-only queries work).
func searchFilter(entries []jump.Entry, query string) []jump.Entry {
	if query == "" {
		return entries
	}
	labels := make([]string, len(entries))
	for i := range entries {
		e := &entries[i]
		labels[i] = strings.Join([]string{e.Label, string(e.Kind), e.Code}, " ")
	}
	matches := fuzzy.FindFrom(query, labelSource(labels))
	out := make([]jump.Entry, 0, len(matches))
	for _, mt := range matches {
		out = append(out, entries[mt.Index])
	}
	return out
}

type labelSource []string

func (s labelSource) String(i int) string { return s[i] }
func (s labelSource) Len() int            { return len(s) }

func clamp(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// bodyRows is the listing window height: the popup box minus the header and
// footer lines. One row minimum — even the old 44x3 binding shows the top
// of the table.
func (m *jumpModel) bodyRows() int {
	body := m.height - 2
	if body < 1 {
		body = 1
	}
	return body
}

// fireEntry resolves and focuses the target, starting the close delay.
func (m *jumpModel) fireEntry(e jump.Entry) (tea.Model, tea.Cmd) {
	m.target = &e
	m.fire()
	return m, tea.Tick(fireDelay, func(time.Time) tea.Msg { return firedMsg{} })
}

// fire focuses the resolved target; a focus failure surfaces as the popup's
// exit error (visible in the plugin log when launched from the key
// binding). Plain panes focus exactly via pane.focus (in herdr 0.9.5's
// schema even though the online method table omits it); a rejection on an
// older server degrades to the pane's tab — one level coarser, not nowhere.
// Agent panes stay on agent.focus: it pins the pane AND marks the agent
// seen, which pane.focus cannot do.
//
// Any activation also restores the tab labels (the codes were only on the
// tabs for this one jump decision) before moving focus.
func (m *jumpModel) fire() {
	clearTags(m.client, m.tags)
	switch m.target.Kind {
	case jump.KindWorkspace:
		m.fireErr = m.client.FocusWorkspace(m.target.ID)
	case jump.KindAgent:
		m.fireErr = m.client.FocusAgent(m.target.ID)
	case jump.KindTab:
		m.fireErr = m.client.FocusTab(m.target.ID)
	case jump.KindPane:
		err := m.client.FocusPane(m.target.ID)
		if err != nil && m.target.TabID != "" {
			if tabErr := m.client.FocusTab(m.target.TabID); tabErr == nil {
				err = nil // degradation succeeded; the pane error is moot
			}
		}
		m.fireErr = err
	}
}

func (m *jumpModel) View() tea.View {
	if m.target != nil {
		return tea.NewView(fmt.Sprintf("jump ▸ %s → %s", m.target.Code, m.target.Label))
	}

	rows := m.rows()
	lines := make([]string, 0, m.height)
	head := m.buf
	if m.search {
		head = "/" + m.query
	}
	label := "jump"
	if m.search {
		label = "search"
	}
	lines = append(lines, clip(fmt.Sprintf("%s ▸ %s", label, head), m.width))

	body := m.bodyRows()
	for i := 0; i < body; i++ {
		idx := m.scroll + i
		if idx >= len(rows) {
			lines = append(lines, "")
			continue
		}
		lines = append(lines, listRow(rows[idx], m.width, m.search && idx == m.sel))
	}

	if m.height >= 3 {
		hint := fmt.Sprintf("%d", len(rows))
		if m.search {
			hint += " match"
			if len(rows) != 1 {
				hint += "es"
			}
		} else {
			hint += " targets"
		}
		hint += " · / search · esc cancels"
		lines = append(lines, clip(hint, m.width))
	}
	return tea.NewView(strings.Join(lines, "\n"))
}

// listRow renders one listing line: cursor marker, code (badges column-
// aligned at three cells), kind tag, then the label. Plain text only —
// label sources are server-sanitized titles/cwds/ids, so the display-width
// clip is the only care the line needs.
func listRow(e jump.Entry, width int, selected bool) string {
	marker := " "
	if selected {
		marker = "▌"
	}
	return clip(fmt.Sprintf("%s %-3s %-2s %s", marker, e.Code, kindTag(e.Kind), e.Label), width)
}

// kindTag is the two-letter kind column in the listing.
func kindTag(k jump.Kind) string {
	switch k {
	case jump.KindWorkspace:
		return "sp"
	case jump.KindAgent:
		return "ag"
	case jump.KindTab:
		return "tb"
	default:
		return "pn"
	}
}

// clip cuts a plain text to at most w display cells, adding an ellipsis.
// Twin of the same helper in internal/app (view.go's truncateANSI without
// the ANSI passthrough — popup rows are unstyled); kept local so the popup
// stays self-contained, like printable below.
func clip(s string, w int) string {
	if w <= 0 {
		return ""
	}
	if lipgloss.Width(s) <= w {
		return s
	}
	var b strings.Builder
	used := 0
	for _, r := range s {
		rw := lipgloss.Width(string(r))
		if used+rw > w-1 { // leave room for the ellipsis
			break
		}
		b.WriteRune(r)
		used += rw
	}
	return b.String() + "…"
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
