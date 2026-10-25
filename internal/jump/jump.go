// Package jump assigns jump codes ($jump metadata tokens) to spaces and
// agents — the badge kinds — and extends the popup's table to tabs and
// plain panes, keeps the sidebar badges in sync with the live layout, and
// resolves typed codes back to focus targets.
package jump

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/MykolaSuprun/herdr-preview-grid/internal/codes"
	"github.com/MykolaSuprun/herdr-preview-grid/internal/herdr"
)

// Source is the metadata source id used for $jump tokens. Re-reporting with
// the same source replaces that source's earlier tokens.
const Source = "user:jumpd"

// pluginID is this plugin's id in herdr's registry (state dir naming).
const pluginID = "preview-grid"

// Kind discriminates focus targets. Workspaces and agents are the badge
// kinds — the only kinds with sidebar rows; tabs and plain panes carry codes
// in the popup's table only (no badgeable row exists for them).
type Kind string

const (
	KindWorkspace Kind = "workspace"
	KindAgent     Kind = "agent"
	KindTab       Kind = "tab"
	KindPane      Kind = "pane"
)

// Entry is one code assignment: a space, an agent pane, a tab, or a plain
// (non-agent) pane. WorkspaceID/TabID carry the surrounding hierarchy for
// compound labels and pane-focus degradation; empty means "is that level"
// (spaces have no parent, tabs have no tab prefix).
type Entry struct {
	Code        string
	Kind        Kind
	ID          string // workspace id for spaces; pane id for agents/panes; tab id for tabs
	Label       string // display label (popup listing + fire confirmation)
	WorkspaceID string // space context of tabs/panes ("" for spaces)
	TabID       string // tab context of panes; fallback focus target
}

// Entries computes the canonical assignment: spaces first (herdr's order),
// then agents, coded with a prefix-free table from charset. A charset too
// small to form any pair (fewer than two distinct letters) yields an empty
// table and therefore no entries — degraded, never a panic.
func Entries(spaces []herdr.Workspace, agents []herdr.Agent, charset string) []Entry {
	table := codes.Table(len(spaces)+len(agents), charset)
	if len(table) == 0 {
		return nil
	}
	out := make([]Entry, 0, len(table))
	for i, ws := range spaces {
		label := ws.Label
		if label == "" {
			label = ws.WorkspaceID
		}
		out = append(out, Entry{Code: string(table[i]), Kind: KindWorkspace, ID: ws.WorkspaceID, Label: label})
	}
	for i, ag := range agents {
		label := ag.DisplayAgent
		if label == "" {
			label = ag.Agent
		}
		out = append(out, Entry{Code: string(table[len(spaces)+i]), Kind: KindAgent, ID: ag.PaneID, Label: label})
	}
	return out
}

// EntriesFor builds the popup's full code table: spaces and agents first —
// with EXACTLY the codes Entries() (and therefore the sidebar badges) uses,
// because codes.Table's sequence is prefix-stable (pinned by test) — then
// every tab, then every plain pane. Tabs keep tab.list order (workspace-
// major); panes follow their tab. Agent-hosting panes are excluded: those
// panes are already jumpable, with stronger semantics, through their agent
// entry (agent.focus also marks the agent seen).
func EntriesFor(spaces []herdr.Workspace, agents []herdr.Agent, tabs []herdr.Tab, panes []herdr.Pane, charset string) []Entry {
	plain := make([]herdr.Pane, 0, len(panes))
	for _, p := range panes {
		if p.Agent == "" {
			plain = append(plain, p)
		}
	}
	plain = orderPanesByTab(plain, tabs)

	out := Entries(spaces, agents, charset)
	n := len(out)
	if n == 0 && len(tabs) == 0 && len(plain) == 0 {
		return nil // no targets at all
	}
	table := codes.Table(n+len(tabs)+len(plain), charset)
	if len(table) == 0 {
		return nil // unusable charset (fewer than two distinct letters)
	}
	// More codable targets than codewords only happens past ~258 items
	// with the default charset (6 + 36 + 216). Rather than emitting
	// code-less entries (which would break Match's empty-buffer contract),
	// the tail sections shrink to fit; badges are always covered because
	// their codes come first in the same prefix-stable sequence.
	fitTabs := min(len(tabs), max(0, len(table)-n))
	fitPanes := min(len(plain), max(0, len(table)-n-fitTabs))

	spaceOf := spaceLabels(spaces)
	tabOf := make(map[string]string, len(tabs))
	for i, tb := range tabs {
		if i >= fitTabs {
			break
		}
		tabOf[tb.TabID] = tabName(tb)
	}
	for i := 0; i < fitTabs; i++ {
		tb := tabs[i]
		out = append(out, Entry{
			Code:        string(table[n+i]),
			Kind:        KindTab,
			ID:          tb.TabID,
			Label:       joinLabel(spaceOf[tb.WorkspaceID], tabName(tb)),
			WorkspaceID: tb.WorkspaceID,
		})
	}
	base := n + fitTabs
	for i := 0; i < fitPanes; i++ {
		p := plain[i]
		out = append(out, Entry{
			Code:        string(table[base+i]),
			Kind:        KindPane,
			ID:          p.PaneID,
			Label:       joinLabel(spaceOf[p.WorkspaceID], tabOf[p.TabID], paneTitle(p)),
			WorkspaceID: p.WorkspaceID,
			TabID:       p.TabID,
		})
	}
	return out
}

// spaceLabels indexes every space's display label (label, else id).
func spaceLabels(spaces []herdr.Workspace) map[string]string {
	m := make(map[string]string, len(spaces))
	for _, ws := range spaces {
		label := ws.Label
		if label == "" {
			label = ws.WorkspaceID
		}
		m[ws.WorkspaceID] = label
	}
	return m
}

// tabName returns a tab's own label (empty label falls back to its id —
// live herdr serves the tab number as the label for unnamed tabs).
func tabName(tb herdr.Tab) string {
	if tb.Label == "" {
		return tb.TabID
	}
	return tb.Label
}

// joinLabel composes a compound label from non-empty parts, " · "-separated.
func joinLabel(parts ...string) string {
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p != "" {
			out = append(out, p)
		}
	}
	return strings.Join(out, " · ")
}

// paneTitle picks a pane's display title: its terminal title, the basename
// of its (foreground) working directory, or the pane id as last resort.
func paneTitle(p herdr.Pane) string {
	if t := strings.TrimSpace(p.TerminalTitle); t != "" {
		return t
	}
	dir := p.ForegroundCwd
	if dir == "" {
		dir = p.Cwd
	}
	if dir != "" {
		if base := filepath.Base(dir); base != "" && base != "." && base != "/" {
			return base
		}
	}
	return p.PaneID
}

// orderPanesByTab lays panes out in the tabs-slice order (tab.list is
// workspace-major, so panes end up workspace-major too), each tab's panes in
// server order. Panes whose tab is missing from the snapshot (layout churn
// mid-list) sort last in input order, keeping the table deterministic.
func orderPanesByTab(panes []herdr.Pane, tabs []herdr.Tab) []herdr.Pane {
	seq := make(map[string]int, len(tabs))
	for i, tb := range tabs {
		seq[tb.TabID] = i
	}
	out := append([]herdr.Pane(nil), panes...)
	sort.SliceStable(out, func(a, b int) bool {
		sa, oka := seq[out[a].TabID]
		sb, okb := seq[out[b].TabID]
		switch {
		case oka && okb:
			return sa < sb
		case oka:
			return true
		default:
			return false
		}
	})
	return out
}

// State is the audit mirror of the last successfully reported mapping.
// Entries() assigns codes deterministically from the layout — this file
// exists so operators can see what the sidebar should carry, not to gate
// reporting (see Sync for why reporting is unconditional).
type State struct {
	Spaces map[string]string `json:"spaces"`
	Agents map[string]string `json:"agents"`
	path   string
}

const stateFile = "jumpcodes.json"

// DefaultStateDir returns the standard plugin state dir
// (~/.local/state/herdr/plugins/preview-grid), used when
// HERDR_PLUGIN_STATE_DIR is absent (CLI runs).
func DefaultStateDir() string {
	base := os.Getenv("XDG_STATE_HOME")
	if base == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return ""
		}
		base = filepath.Join(home, ".local", "state")
	}
	return filepath.Join(base, "herdr", "plugins", pluginID)
}

// LoadState reads the persisted mapping; a missing or corrupt file is a fresh state.
func LoadState(stateDir string) *State {
	st := &State{Spaces: map[string]string{}, Agents: map[string]string{}}
	if stateDir == "" {
		return st
	}
	st.path = filepath.Join(stateDir, stateFile)
	if data, err := os.ReadFile(st.path); err == nil {
		_ = json.Unmarshal(data, st)
	}
	if st.Spaces == nil {
		st.Spaces = map[string]string{}
	}
	if st.Agents == nil {
		st.Agents = map[string]string{}
	}
	return st
}

// save persists the mapping atomically (temp file + rename), so a crash
// mid-write can never leave a truncated state file behind.
func (s *State) save() {
	if s.path == "" {
		return
	}
	if data, err := json.MarshalIndent(s, "", "  "); err == nil {
		writeAtomic(s.path, data)
	}
}

// writeAtomic replaces path with data via a temp file + rename. Twin of the
// helper in internal/mru (kept local so packages stay self-contained).
func writeAtomic(path string, data []byte) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return
	}
	_ = os.Rename(tmp, path)
}

// api is the herdr surface badge syncing needs: layout reads + metadata
// reports. *herdr.Client implements it; it is an interface so Sync can be
// tested against a stub (including failing reports).
type api interface {
	Workspaces() ([]herdr.Workspace, error)
	Agents() ([]herdr.Agent, error)
	ReportWorkspaceMetadata(workspaceID, source string, tokens map[string]string) error
	ReportPaneMetadata(paneID, source string, tokens map[string]string) error
}

// lister is the extended surface the popup needs: the badge reads/reports
// plus the tab/pane enumerations for its full code table.
// *herdr.Client implements it.
type lister interface {
	api
	Tabs() ([]herdr.Tab, error)
	Panes() ([]herdr.Pane, error)
}

// Sync (re)assigns badges for the live layout and re-reports EVERY token on
// every pass. herdr does not persist plugin metadata across server
// restarts, but this state file (and the codes) survive them — a diff-based
// "report only changes" pass therefore skipped exactly the stable
// workspace ids whose server-side badges a restart had wiped, and they
// never came back until a code happened to change. Tokens are idempotent
// (single Source, replace semantics), so unconditionally re-reporting
// self-heals the sidebar within one sync of any restart. The first report
// error is returned so callers can surface it; failed reports stay out of
// the state so the mirror shows only confirmed tokens.
func Sync(client api, st *State, charset string) ([]Entry, error) {
	spaces, err := client.Workspaces()
	if err != nil {
		return nil, err
	}
	agents, err := client.Agents()
	if err != nil {
		return nil, err
	}
	entries := Entries(spaces, agents, charset)
	return entries, applyBadges(client, st, entries)
}

// Collect builds the popup's full table (spaces, agents, tabs, plain panes)
// and, along the way, re-asserts the sidebar badges — the same self-heal
// Sync performs, because opening the popup is a natural refresh moment.
// Spaces/agents list errors are fatal as in Sync; tab/pane list errors are
// soft: those sections just drop out (older servers without tab.list
// degrade to the badge-only table), and the first soft error is returned
// alongside the entries so callers can surface it when nothing else worked.
func Collect(client lister, st *State, charset string) ([]Entry, error) {
	spaces, err := client.Workspaces()
	if err != nil {
		return nil, err
	}
	agents, err := client.Agents()
	if err != nil {
		return nil, err
	}
	tabs, tabErr := client.Tabs()
	panes, paneErr := client.Panes()

	entries := EntriesFor(spaces, agents, tabs, panes, charset)
	firstErr := applyBadges(client, st, entries)
	switch {
	case firstErr != nil:
	case tabErr != nil:
		firstErr = tabErr
	case paneErr != nil:
		firstErr = paneErr
	}
	return entries, firstErr
}

// applyBadges re-reports the workspace/agent tokens among entries (tabs and
// plain panes have no badgeable sidebar row, so their kinds are ignored),
// prunes vanished ids from the state, and persists it. The first report
// error is returned; failed reports stay out of the state so the mirror
// shows only confirmed tokens.
func applyBadges(client api, st *State, entries []Entry) error {
	var firstErr error
	note := func(err error) {
		if err != nil && firstErr == nil {
			firstErr = err
		}
	}
	seenSpaces := map[string]bool{}
	seenAgents := map[string]bool{}
	for _, e := range entries {
		tokens := map[string]string{"jump": e.Code}
		switch e.Kind {
		case KindWorkspace:
			seenSpaces[e.ID] = true
			if err := client.ReportWorkspaceMetadata(e.ID, Source, tokens); err != nil {
				note(err)
				continue
			}
			st.Spaces[e.ID] = e.Code
		case KindAgent:
			seenAgents[e.ID] = true
			if err := client.ReportPaneMetadata(e.ID, Source, tokens); err != nil {
				note(err)
				continue
			}
			st.Agents[e.ID] = e.Code
		}
	}
	// Forgetting gone ids keeps the state file a faithful mirror.
	for id := range st.Spaces {
		if !seenSpaces[id] {
			delete(st.Spaces, id)
		}
	}
	for id := range st.Agents {
		if !seenAgents[id] {
			delete(st.Agents, id)
		}
	}
	st.save()
	return firstErr
}

// Match resolves buf against entries:
//   - entry non-nil: buf is a complete code — act now.
//   - entry nil, live true: buf is a live prefix — keep typing.
//   - entry nil, live false: dead end (an empty buffer is always live).
func Match(buf string, entries []Entry) (*Entry, bool) {
	live := false
	for i := range entries {
		c := entries[i].Code
		switch {
		case c == buf:
			return &entries[i], true
		case strings.HasPrefix(c, buf):
			live = true
		}
	}
	return nil, live
}
