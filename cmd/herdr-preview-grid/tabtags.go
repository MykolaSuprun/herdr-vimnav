// tab tags: while the jump popup is open, the tabs of the ACTIVE space carry
// their jump code in their label ("uj 1", "uk logs"); the moment any code
// fires (or the popup closes), the original labels are restored.
//
// Why renaming and not the tab_bar_right command segment: herdr 0.9.5 has no
// tab-metadata surface (tab.* is create/list/get/focus/rename/move/close),
// so the tab bar can only render the tab's own label. A right-aligned
// command segment is polled on an interval (>=1s) and can never catch a
// popup that lives for a few hundred milliseconds — the user would have to
// leave the popup open just to see it. Renaming is immediate and puts the
// code exactly where the tabs are.
//
// Labels are restored from a saved map, so this is non-destructive: the
// original label text is what comes back, including tabs the user renamed
// themselves. State lives in $HERDR_PLUGIN_STATE_DIR/tab-tags.json so that a
// crashed/aborted popup can still be unwound by the next open or a
// `tab-tags off` run.
package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"github.com/MykolaSuprun/herdr-preview-grid/internal/herdr"
	"github.com/MykolaSuprun/herdr-preview-grid/internal/jump"
)

// tagStateFile mirrors which tab labels were rewritten and to what.
const tagStateFile = "tab-tags.json"

// tagState is the restore ledger: tab id -> original label.
type tagState struct {
	Labels map[string]string `json:"labels"`
	path   string
}

func loadTagState(dir string) *tagState {
	st := &tagState{Labels: map[string]string{}}
	if dir != "" {
		st.path = filepath.Join(dir, tagStateFile)
		if data, err := os.ReadFile(st.path); err == nil {
			_ = json.Unmarshal(data, st)
		}
	}
	if st.Labels == nil {
		st.Labels = map[string]string{}
	}
	return st
}

func (s *tagState) save() {
	if s.path == "" {
		return
	}
	if data, err := json.MarshalIndent(s, "", "  "); err == nil {
		_ = os.MkdirAll(filepath.Dir(s.path), 0o755)
		_ = os.WriteFile(s.path, data, 0o644)
	}
}

// tabTagger is the socket surface tagging needs.
type tabTagger interface {
	Workspaces() ([]herdr.Workspace, error)
	Agents() ([]herdr.Agent, error)
	Tabs() ([]herdr.Tab, error)
	RenameTab(tabID, label string) error
}

// applyTags rewrites the active workspace's tab labels to "<code> <label>".
// Tabs that already carry their code (a second popup open) are left alone;
// originals are recorded first so a failed rename never loses the label.
func applyTags(client tabTagger, st *tagState, charset string) error {
	spaces, err := client.Workspaces()
	if err != nil {
		return err
	}
	agents, err := client.Agents()
	if err != nil {
		return err
	}
	tabs, err := client.Tabs()
	if err != nil {
		return err
	}

	active := activeWorkspaceID(spaces)
	if active == "" {
		return nil // no focused space: nothing to tag
	}

	// Codes come from the same table the popup uses, so a tag can never
	// disagree with what the popup lists.
	entries := jump.EntriesFor(spaces, agents, tabs, nil, charset)
	codeOf := make(map[string]string, len(entries))
	for _, e := range entries {
		if e.Kind == jump.KindTab {
			codeOf[e.ID] = e.Code
		}
	}

	for _, tb := range tabs {
		if tb.WorkspaceID != active {
			continue
		}
		code, ok := codeOf[tb.TabID]
		if !ok {
			continue
		}
		if _, done := st.Labels[tb.TabID]; done {
			continue // already tagged by an earlier open
		}
		original := tb.Label
		if original == "" {
			original = formatNumber(tb.Number)
		}
		if strings.HasPrefix(tb.Label, code+" ") {
			continue
		}
		// Record BEFORE renaming: a crash between the two steps must still
		// leave enough information for the next open (or `codes off`) to
		// restore the label. This is the durability contract.
		st.Labels[tb.TabID] = original
		st.save()
		if err := client.RenameTab(tb.TabID, code+" "+original); err != nil {
			delete(st.Labels, tb.TabID) // rename failed: nothing to restore
			st.save()
			continue
		}
	}
	st.save()
	return nil
}

// clearTags restores every recorded label and empties the ledger. Missing
// tabs (closed while tagged) are simply dropped.
func clearTags(client tabTagger, st *tagState) {
	if len(st.Labels) == 0 {
		_ = os.Remove(st.path)
		return
	}
	for tabID, original := range st.Labels {
		if err := client.RenameTab(tabID, original); err != nil {
			continue // tab gone; restore nothing to lose
		}
		delete(st.Labels, tabID)
	}
	st.Labels = map[string]string{}
	st.save()
}

// activeWorkspaceID is the focused space, or "" when none is focused.
func activeWorkspaceID(spaces []herdr.Workspace) string {
	for _, ws := range spaces {
		if ws.Focused {
			return ws.WorkspaceID
		}
	}
	return ""
}

func formatNumber(n int) string {
	if n <= 0 {
		return "?"
	}
	digits := []byte{}
	for n > 0 {
		digits = append([]byte{byte('0' + n%10)}, digits...)
		n /= 10
	}
	return string(digits)
}
