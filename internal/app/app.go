// Package app implements the preview-grid switcher: a master list of herdr
// workspaces or agents with a live pane preview for the selected item,
// ctrl-chord navigation, fuzzy search, and select-only jump codes.
package app

import (
	"encoding/json"
	"strings"
	"time"
	"unicode/utf8"

	tea "charm.land/bubbletea/v2"

	"github.com/MykolaSuprun/herdr-preview-grid/internal/codes"
	"github.com/MykolaSuprun/herdr-preview-grid/internal/config"
	"github.com/MykolaSuprun/herdr-preview-grid/internal/herdr"
	"github.com/MykolaSuprun/herdr-preview-grid/internal/mru"
	"github.com/MykolaSuprun/herdr-preview-grid/internal/theme"

	"github.com/sahilm/fuzzy"
)

// Mode selects what the picker lists.
type Mode int

// Modes.
const (
	ModeSpaces Mode = iota
	ModeAgents
)

// Item is one list entry.
type Item struct {
	Key           string // stable identity + MRU key, e.g. "workspace:w15"
	Label         string
	Number        int    // herdr row number (0 = none); its own list column
	Cwd           string // display-ready (shortened) working dir; "" = none
	Status        string
	AwaitingInput bool   // agent blocked / space has a blocked agent
	PaneID        string // pane whose content is previewed ("" = none)
	Target        string // focus target (workspace id / agent pane id)
	Focused       bool
	Kind          string // "space" | "agent"
	Code          string // canonical jump code ("" past table capacity)
}

// Model is the Bubble Tea model.
type Model struct {
	mode     Mode
	cfg      config.Config
	client   *herdr.Client
	ledger   *mru.Ledger
	stateDir string

	// Herdr context for preview resolution: the picker's own pane and the
	// pane it was launched over (underneath the overlay). Both "" when the
	// binary runs outside herdr (plain CLI, tests).
	selfPane   string
	launchPane string

	width, height int
	ready         bool

	items    []Item // ordered as displayed
	allItems []Item // unfiltered, in base order
	// allCodes holds each allItems entry's canonical jump code, index-
	// parallel to allItems ("" for entries past the table's capacity). It
	// is rebuilt only on reload, from codes assigned in the server's
	// NATURAL order — never from the displayed order — so sorting and
	// filtering reorder rows without ever relabeling a target. The codes
	// are exactly the sidebar-badge/jump-popup codes (jump.Entries
	// semantics: spaces from the sequence start, agents offset by the
	// workspace count).
	allCodes []codes.Code
	// nSpaces is the workspace count of the current layout, whatever the
	// mode — loadAgents' workspace fetch sets it; it is the agents-mode
	// code offset.
	nSpaces int

	cursor       int
	search       string
	filterActive bool
	codeBuf      string

	previews   map[string]string // pane id -> preview text
	scroll     int               // first visible list row (item index)
	errMsg     string
	seeded     bool // ledger focus-seed done once per open
	spec       theme.Spec
	pal        theme.Palette // resolved at New + on every BackgroundColorMsg
	cursorInit bool          // initial cursor placement done once per open
}

// New constructs the model. selfPane is this process's own herdr pane
// (HERDR_PANE_ID) and launchPane the pane the picker pane was launched over
// (HERDR_PLUGIN_CONTEXT_JSON); both may be empty outside herdr.
func New(mode Mode, cfg config.Config, client *herdr.Client, ledger *mru.Ledger, stateDir, selfPane, launchPane string) Model {
	// Theming: read herdr's [theme] tables once; appearance is Unknown until
	// the hosting terminal answers tea.BackgroundColorMsg (Init requests it).
	// Indirection exists for tests: pinned-colour tests stub the loader so
	// they never depend on the developer's own herdr config.
	spec := loadHerdrSpec(theme.HerdrConfigPath())
	spec.PluginTheme = cfg.Theme
	return Model{
		mode:       mode,
		cfg:        cfg,
		client:     client,
		ledger:     ledger,
		stateDir:   stateDir,
		selfPane:   selfPane,
		launchPane: launchPane,
		previews:   map[string]string{},
		spec:       spec,
		pal:        theme.Resolve(spec, theme.Unknown),
	}
}

// loadHerdrSpec is theme.LoadSpec, indirected for deterministic tests.
var loadHerdrSpec = theme.LoadSpec

// Init loads the first snapshot and asks the terminal for its background so
// the palette can follow herdr's light/dark appearance.
func (m Model) Init() tea.Cmd {
	return tea.Batch(
		func() tea.Msg { return refreshMsg{} },
		func() tea.Msg { return tea.RequestBackgroundColor() },
	)
}

type refreshMsg struct{}

// Update implements tea.Model.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.ready = true
		m.clampScroll()
		return m, nil

	case refreshMsg:
		m.reload()
		m.refreshPreviews()
		return m, tea.Tick(time.Duration(m.cfg.RefreshMS)*time.Millisecond, func(time.Time) tea.Msg { return refreshMsg{} })

	case tea.BackgroundColorMsg:
		// herdr keys its auto_switch off the host terminal's appearance and
		// sets our pane's colour scheme accordingly, so this flips the
		// palette in lockstep with herdr. Washes/styles derive from m.pal at
		// render time — nothing else to recompute.
		app := theme.Light
		if msg.IsDark() {
			app = theme.Dark
		}
		m.pal = theme.Resolve(m.spec, app)
		return m, nil

	case tea.KeyPressMsg:
		return m.handleKey(msg)
	}
	return m, nil
}

func (m Model) handleKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	key := msg.String()

	// Global keys.
	switch key {
	case "ctrl+c": // unconditional quit, even mid-search
		return m, tea.Quit
	case "esc":
		if m.filterActive {
			m.filterActive = false
			m.search = ""
			m.applyFilter()
			return m, nil
		}
		return m, tea.Quit
	case "enter":
		if len(m.items) > 0 && m.cursor < len(m.items) {
			return m.jump(m.cursor)
		}
		return m, nil
	}

	// List navigation (ctrl chords + arrows; bare letters stay code input,
	// because the code letters themselves are j/k/l/f/d/s).
	switch key {
	case "ctrl+j", "down":
		m.moveSelection(+1)
		return m, nil
	case "ctrl+k", "up":
		m.moveSelection(-1)
		return m, nil
	case "pgdown", "ctrl+d":
		m.moveSelection(m.listRows())
		return m, nil
	case "pgup", "ctrl+u":
		m.moveSelection(-m.listRows())
		return m, nil
	case "home":
		m.setCursor(0)
		return m, nil
	case "end":
		m.setCursor(len(m.items) - 1)
		return m, nil
	}

	if key == "/" && !m.filterActive {
		m.filterActive = true
		m.search = ""
		m.codeBuf = ""
		m.applyFilter()
		return m, nil
	}

	if m.filterActive {
		return m, m.filterKey(key, msg)
	}
	return m, m.codeKey(key, msg)
}

// filterKey handles keys while search is active: backspace edits the query
// (rune-aware — the buffer holds decoded multi-byte input) and printable
// runes extend it. Navigation stays in handleKey so ctrl+j/k work mid-search.
func (m *Model) filterKey(key string, msg tea.KeyPressMsg) tea.Cmd {
	m.codeBuf = ""
	switch key {
	case "backspace":
		r := []rune(m.search)
		if len(r) > 0 {
			m.search = string(r[:len(r)-1])
			m.applyFilter()
		}
	default:
		if txt := printable(msg); txt != "" {
			m.search += txt
			m.applyFilter()
		}
	}
	return nil
}

// codeKey handles keys outside search. Code input is SELECT-ONLY: a complete
// code moves the highlight (the detail preview follows); enter commits the
// switch. The table is prefix-free, so a code fires the moment it completes —
// no timers. codeBuf only ever holds single runes (consumeCode guarantees
// it), so a byte-wise backspace trim is safe here.
func (m *Model) codeKey(key string, msg tea.KeyPressMsg) tea.Cmd {
	if key == "backspace" {
		if m.codeBuf != "" {
			m.codeBuf = m.codeBuf[:len(m.codeBuf)-1]
		}
		return nil
	}
	if m.consumeCode(msg) {
		idx, _, ok := codes.Resolve(m.codeBuf, m.allCodes)
		switch {
		case ok && idx >= 0: // complete: select and consume the buffer
			m.codeBuf = ""
			m.selectByCode(idx)
		case !ok: // dead end: start over
			m.codeBuf = ""
		}
	}
	return nil
}

// selectByCode moves the cursor to the target the resolved index points at
// in allItems. The target is normally on screen; when the active filter
// hides it, the filter is cleared first — a code learned from the sidebar
// or the jump popup always selects its target, whatever is displayed.
func (m *Model) selectByCode(allIdx int) {
	key := m.allItems[allIdx].Key
	for i := range m.items {
		if m.items[i].Key == key {
			m.setCursor(i)
			return
		}
	}
	m.search = ""
	m.filterActive = false
	m.applyFilterFrom(key)
}

// consumeCode appends a code letter to the buffer. Every code is drawn from
// the pool itself (2 letters, then doubled-first triples), so the charset IS
// the full valid input alphabet — no separate initiator set exists.
func (m *Model) consumeCode(msg tea.KeyPressMsg) bool {
	txt := printable(msg)
	if txt == "" || utf8.RuneCountInString(txt) != 1 {
		return false
	}
	if !strings.ContainsRune(m.cfg.Charset, []rune(txt)[0]) {
		return false
	}
	m.codeBuf += txt
	return true
}

func printable(msg tea.KeyPressMsg) string {
	if msg.Mod.Contains(tea.ModCtrl) || msg.Mod.Contains(tea.ModAlt) {
		return ""
	}
	return msg.Text
}

// --- data -------------------------------------------------------------------

func (m *Model) reload() {
	var items []Item
	var err error
	switch m.mode {
	case ModeSpaces:
		items, err = m.loadSpaces()
	case ModeAgents:
		items, err = m.loadAgents()
	}
	if err != nil {
		m.errMsg = err.Error()
		return
	}
	m.errMsg = ""

	// Codes BEFORE ordering: each item gets its canonical code in the
	// server's natural order — exactly what the sidebar badges and the
	// jump popup assign, so one code means one target on every surface.
	// (jump.Entries semantics: spaces start the shared sequence, agents
	// continue after the workspace codes. codes' sequence is prefix-
	// stable, so Table(N)[i] equals the shared table's slot i regardless
	// of N — the spaces picker needs no agent knowledge.) Everything below
	// (priority/MRU sort, fuzzy filter) reorders rows but never relabels.
	start := 0
	if m.mode == ModeAgents {
		start = m.nSpaces // authoritative count, set by loadAgents
	}
	table := codes.Table(start+len(items), m.cfg.Charset)
	for i := range items {
		if start+i < len(table) {
			items[i].Code = string(table[start+i])
		}
	}

	// Seed the ledger once per open: the currently focused workspace/agent is
	// "where the user is right now" — belt-and-braces for the [[events]]
	// hooks (covers server restarts that emit no initial focus event). Runs
	// before ordering so the focused item leads its recency group.
	if !m.seeded {
		m.seeded = true
		for _, it := range items {
			if it.Focused {
				m.ledger.Touch(it.Key)
			}
		}
	}

	// Base order: attention first (blocked/done, then MRU), usage (MRU),
	// or natural.
	switch m.cfg.Sort {
	case "priority":
		items = mru.Order(m.ledger, items, func(it Item) string { return it.Key })
		items = partitionAwaiting(items)
	case "usage":
		items = mru.Order(m.ledger, items, func(it Item) string { return it.Key })
	}
	m.allItems = items
	// Resolve table parallel to allItems — re-read from the (now sorted)
	// items' own codes, so a Resolve hit can be mapped straight back to
	// its target. Empty codes (past capacity) never match a typed key.
	m.allCodes = make([]codes.Code, len(items))
	for i := range items {
		m.allCodes[i] = codes.Code(items[i].Code)
	}

	// Cursor: on first load start on the currently focused item, so the
	// picker opens showing where you are; afterwards restore the selection
	// across refreshes by identity key.
	selKey := ""
	if !m.cursorInit {
		m.cursorInit = true
		for _, it := range items {
			if it.Focused {
				selKey = it.Key
				break
			}
		}
	} else if m.cursor < len(m.items) {
		selKey = m.items[m.cursor].Key
	}
	m.applyFilterFrom(selKey)
}

func (m *Model) loadSpaces() ([]Item, error) {
	spaces, err := m.client.Workspaces()
	if err != nil {
		return nil, err
	}
	panes, err := m.client.Panes()
	if err != nil {
		return nil, err
	}
	// Spaces owing ATTENTION rise to the top under the priority sort:
	// blocked (approval/question dialog) or done (finished but unviewed).
	// Idle is the majority state and stays in the recency stream. Derived
	// from panes directly — more robust than the rolled-up workspace
	// agent_status.
	readyWS := map[string]bool{}
	for _, p := range panes {
		if isReadyForInput(p.AgentStatus) {
			readyWS[p.WorkspaceID] = true
		}
	}
	out := make([]Item, 0, len(spaces))
	for _, ws := range spaces {
		previewPane := resolvePreviewPane(panes, ws, m.selfPane, m.launchPane)
		out = append(out, Item{
			Key:           "workspace:" + ws.WorkspaceID,
			Label:         ws.Label,
			Number:        ws.Number,
			Cwd:           paneDisplayCwd(panes, previewPane),
			Status:        ws.AgentStatus,
			AwaitingInput: readyWS[ws.WorkspaceID],
			PaneID:        previewPane,
			Target:        ws.WorkspaceID,
			Focused:       ws.Focused,
			Kind:          "space",
		})
	}
	return out, nil
}

// paneDisplayCwd is the shortened working directory shown on a list row: the
// pane's foreground cwd (what the shell is actually looking at), falling back
// to the pane cwd.
func paneDisplayCwd(panes []herdr.Pane, paneID string) string {
	if paneID == "" {
		return ""
	}
	for _, p := range panes {
		if p.PaneID == paneID {
			if p.ForegroundCwd != "" {
				return shortPath(p.ForegroundCwd)
			}
			if p.Cwd != "" {
				return shortPath(p.Cwd)
			}
			return ""
		}
	}
	return ""
}

// resolvePreviewPane picks the pane whose content stands in for a workspace
// tab in the detail panel. Preference order:
//
//  1. the tab's focused pane (the normal case);
//  2. the pane the picker was launched over (launchPane) — while the picker
//     overlay is open, herdr focuses the overlay pane itself (it is a normal
//     pane), so step 1 resolves to our own pane; the launch context still
//     points at the pane underneath;
//  3. any other pane in the tab.
//
// selfPane (this process's HERDR_PANE_ID; "" outside herdr) is never
// previewed: the picker inside the picker is a stale self-echo.
func resolvePreviewPane(panes []herdr.Pane, ws herdr.Workspace, selfPane, launchPane string) string {
	inTab := func(p herdr.Pane) bool {
		return p.WorkspaceID == ws.WorkspaceID && p.TabID == ws.ActiveTabID && p.PaneID != selfPane
	}
	for _, p := range panes {
		if inTab(p) && p.Focused {
			return p.PaneID
		}
	}
	if launchPane != "" && launchPane != selfPane {
		for _, p := range panes {
			if p.PaneID == launchPane && inTab(p) {
				return p.PaneID
			}
		}
	}
	for _, p := range panes {
		if inTab(p) {
			return p.PaneID
		}
	}
	return ""
}

// LaunchFocusedPane extracts focused_pane_id from HERDR_PLUGIN_CONTEXT_JSON —
// herdr injects this context into every plugin pane process, and the field
// names the pane the pane was launched over. Returns "" when absent or
// unparseable.
func LaunchFocusedPane(raw string) string {
	if raw == "" {
		return ""
	}
	var ctx struct {
		FocusedPaneID string `json:"focused_pane_id"`
	}
	if err := json.Unmarshal([]byte(raw), &ctx); err != nil {
		return ""
	}
	return ctx.FocusedPaneID
}

func (m *Model) loadAgents() ([]Item, error) {
	agents, err := m.client.Agents()
	if err != nil {
		return nil, err
	}
	out := make([]Item, 0, len(agents))
	// The workspace count is authoritative here, not best-effort: agent
	// codes continue the shared sequence AFTER the workspace codes (the
	// same offset jump.Entries uses), so a count we don't have would give
	// every agent a wrong code. Blank numbers were an acceptable failure;
	// wrong codes are not.
	workspaces, err := m.client.Workspaces()
	if err != nil {
		return nil, err
	}
	m.nSpaces = len(workspaces)
	// The number column mirrors each agent's member-workspace row number.
	numbers := map[string]int{}
	for _, ws := range workspaces {
		numbers[ws.WorkspaceID] = ws.Number
	}
	for _, a := range agents {
		label := a.DisplayAgent
		if label == "" {
			label = a.Agent
		}
		cwd := ""
		if a.Cwd != "" {
			cwd = shortPath(a.Cwd)
		}
		out = append(out, Item{
			Key:           "agent:" + a.PaneID,
			Label:         label,
			Number:        numbers[a.WorkspaceID],
			Cwd:           cwd,
			Status:        a.AgentStatus,
			AwaitingInput: isReadyForInput(a.AgentStatus),
			PaneID:        a.PaneID,
			Target:        a.PaneID,
			Focused:       a.Focused,
			Kind:          "agent",
		})
	}
	return out, nil
}

// ParseEventKey maps a herdr event JSON payload to the ledger item key it
// represents, if any. Event type values are snake_case (schema: EventKind).
// Used by the record subcommand (herdr [[events]] hooks) to feed the MRU
// ledger with real navigation activity from any client.
func ParseEventKey(raw string) (string, bool) {
	var ev struct {
		Type        string `json:"type"`
		WorkspaceID string `json:"workspace_id"`
		PaneID      string `json:"pane_id"`
	}
	if err := json.Unmarshal([]byte(raw), &ev); err != nil {
		return "", false
	}
	switch ev.Type {
	case "workspace_focused":
		if ev.WorkspaceID == "" {
			return "", false
		}
		return "workspace:" + ev.WorkspaceID, true
	case "pane_focused":
		if ev.PaneID == "" {
			return "", false
		}
		// Agent list items key on the pane id; non-agent panes record keys
		// that simply never match a grid item — harmless ledger noise.
		return "agent:" + ev.PaneID, true
	default:
		return "", false
	}
}

// isReadyForInput reports whether a herdr agent_status marks ATTENTION the
// user owes the agent (2026-10-10 decision): `blocked` (approval/question UI)
// or `done` (finished but not yet seen). Idle is NOT special — idle is the
// majority state on this setup, and floating it drowns MRU recency — so idle
// and working stay interleaved in the recency stream. (Readiness details
// remain visible via the status dots.)
func isReadyForInput(status string) bool {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "blocked", "done":
		return true
	default:
		return false
	}
}

// partitionAwaiting stably moves awaiting-input items to the front,
// preserving the relative (MRU) order within both groups.
func partitionAwaiting(items []Item) []Item {
	front := make([]Item, 0, len(items))
	rest := make([]Item, 0, len(items))
	for _, it := range items {
		if it.AwaitingInput {
			front = append(front, it)
		} else {
			rest = append(rest, it)
		}
	}
	return append(front, rest...)
}

func shortPath(p string) string {
	parts := strings.Split(strings.TrimRight(p, "/"), "/")
	if len(parts) <= 2 {
		return p
	}
	return "…/" + strings.Join(parts[len(parts)-2:], "/")
}

// applyFilter recomputes items from allItems using the current search.
func (m *Model) applyFilter() { m.applyFilterFrom("") }

func (m *Model) applyFilterFrom(selKey string) {
	if m.search == "" {
		m.items = append([]Item(nil), m.allItems...)
	} else {
		labels := make([]string, len(m.allItems))
		for i, it := range m.allItems {
			labels[i] = it.Label + " " + it.Status
		}
		matches := fuzzy.FindFrom(m.search, stringSource(labels))
		m.items = make([]Item, 0, len(matches))
		for _, mt := range matches {
			m.items = append(m.items, m.allItems[mt.Index])
		}
	}
	// Codes travel with their items: rows filtered or reordered above keep
	// the codes reload() assigned. No table is rebuilt here.

	// Restore cursor on the selected key when possible.
	m.cursor = 0
	if selKey != "" {
		for i, it := range m.items {
			if it.Key == selKey {
				m.cursor = i
				break
			}
		}
	}
	m.cursor = clamp(m.cursor, 0, max(0, len(m.items)-1))
	m.clampScroll()
	m.ensureVisible()
}

type stringSource []string

func (s stringSource) String(i int) string { return s[i] }
func (s stringSource) Len() int            { return len(s) }

// refreshPreviews fetches live output for ALL items, not just the visible
// rows — moving the selection then shows fresh content with zero fetch
// delay (moves additionally re-read immediately; see
// refreshSelectedPreview). Item counts are small (dozens), each read is one
// short socket round-trip, so this stays cheap.
func (m *Model) refreshPreviews() {
	if !m.cfg.Preview {
		return
	}
	live := map[string]bool{}
	for _, it := range m.items {
		if it.PaneID == "" {
			continue
		}
		live[it.PaneID] = true
		text, err := m.client.ReadPane(it.PaneID, "recent_unwrapped", m.previewReadLines(), true)
		if err != nil {
			m.previews[it.PaneID] = ""
			continue
		}
		m.previews[it.PaneID] = text
	}
	// Drop cached previews for panes that no longer back any item.
	for paneID := range m.previews {
		if !live[paneID] {
			delete(m.previews, paneID)
		}
	}
}

// --- navigation -------------------------------------------------------------

// setCursor moves the selection to idx, scrolls it into view, and re-reads
// the newly selected pane immediately (so the preview never lags a tick).
func (m *Model) setCursor(idx int) {
	if len(m.items) == 0 {
		m.cursor = 0
		return
	}
	idx = clamp(idx, 0, len(m.items)-1)
	if idx == m.cursor {
		return
	}
	m.cursor = idx
	m.ensureVisible()
	m.refreshSelectedPreview()
}

// moveSelection steps the cursor by delta rows (clamped).
func (m *Model) moveSelection(delta int) {
	m.setCursor(m.cursor + delta)
}

// refreshSelectedPreview re-reads the selected item's pane right away so a
// cursor move shows fresh output without waiting for the next refresh tick.
func (m *Model) refreshSelectedPreview() {
	if !m.cfg.Preview || len(m.items) == 0 || m.cursor >= len(m.items) {
		return
	}
	it := m.items[m.cursor]
	if it.PaneID == "" {
		return
	}
	text, err := m.client.ReadPane(it.PaneID, "recent_unwrapped", m.previewReadLines(), true)
	if err != nil {
		m.previews[it.PaneID] = ""
		return
	}
	m.previews[it.PaneID] = text
}

func (m *Model) jump(idx int) (tea.Model, tea.Cmd) {
	if idx < 0 || idx >= len(m.items) {
		return m, nil
	}
	it := m.items[idx]
	var err error
	switch it.Kind {
	case "space":
		err = m.client.FocusWorkspace(it.Target)
	case "agent":
		err = m.client.FocusAgent(it.Target)
	}
	if err != nil {
		m.errMsg = err.Error()
		return m, nil
	}
	m.ledger.Touch(it.Key)
	return m, tea.Quit
}
