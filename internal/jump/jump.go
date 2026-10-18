// Package jump assigns sidebar jump codes ($jump metadata tokens) to spaces
// and agents, keeps them in sync with the live layout, and resolves typed
// codes back to focus targets.
package jump

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"github.com/MykolaSuprun/herdr-preview-grid/internal/codes"
	"github.com/MykolaSuprun/herdr-preview-grid/internal/herdr"
)

// Source is the metadata source id used for $jump tokens. Re-reporting with
// the same source replaces that source's earlier tokens.
const Source = "user:jumpd"

// pluginID is this plugin's id in herdr's registry (state dir naming).
const pluginID = "preview-grid"

// Kind discriminates workspace and agent assignments.
type Kind string

const (
	KindWorkspace Kind = "workspace"
	KindAgent     Kind = "agent"
)

// Entry is one code assignment: a space or an agent pane.
type Entry struct {
	Code  string
	Kind  Kind
	ID    string // workspace id, or pane id for agents
	Label string // display label for the popup
}

// Entries computes the canonical assignment: spaces first (herdr's order),
// then agents, coded with a prefix-free table from charset.
func Entries(spaces []herdr.Workspace, agents []herdr.Agent, charset string) []Entry {
	table := codes.Table(len(spaces)+len(agents), charset)
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

// State is the last reported mapping, used to report only changed tokens.
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

// api is the herdr surface Sync needs: layout reads + metadata reports.
// *herdr.Client implements it; it is an interface so Sync can be tested
// against a stub (including failing reports).
type api interface {
	Workspaces() ([]herdr.Workspace, error)
	Agents() ([]herdr.Agent, error)
	ReportWorkspaceMetadata(workspaceID, source string, tokens map[string]string) error
	ReportPaneMetadata(paneID, source string, tokens map[string]string) error
}

// Sync (re)assigns codes for the live layout and reports changed tokens.
// Individual report failures don't abort the pass; the first error is
// returned so callers can surface it in the plugin log. A code is recorded
// in the state only once its report SUCCEEDED — failed reports stay
// unreported and are retried by the next diff pass.
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
			if st.Spaces[e.ID] == e.Code {
				continue
			}
			if err := client.ReportWorkspaceMetadata(e.ID, Source, tokens); err != nil {
				note(err)
				continue
			}
			st.Spaces[e.ID] = e.Code
		case KindAgent:
			seenAgents[e.ID] = true
			if st.Agents[e.ID] == e.Code {
				continue
			}
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
	return entries, firstErr
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
