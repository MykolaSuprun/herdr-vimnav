package app

import (
	"bufio"
	"encoding/json"
	"net"
	"path/filepath"
	"testing"

	"github.com/MykolaSuprun/herdr-preview-grid/internal/codes"
	"github.com/MykolaSuprun/herdr-preview-grid/internal/config"
	"github.com/MykolaSuprun/herdr-preview-grid/internal/herdr"
	"github.com/MykolaSuprun/herdr-preview-grid/internal/jump"
	"github.com/MykolaSuprun/herdr-preview-grid/internal/mru"
)

// Regression suite for GitHub issue #1: the pickers must show — and resolve —
// the canonical jump codes (the sidebar-badge/jump-popup assignment), not
// codes recomputed from displayed positions. Layout mirrors the issue:
//
//	alpha (w1, focused, idle agent A)   -> sd
//	beta  (w2, BLOCKED agent B)         -> sf
//	gamma (w3, no agent)                -> sj
//	agent A (pA, w1, idle)              -> sk   (continues after spaces)
//	agent B (pB, w2, blocked)           -> sl

const testPool = "sdfjklerui"

func testSpaces() []herdr.Workspace {
	return []herdr.Workspace{
		{WorkspaceID: "w1", Number: 1, Label: "alpha", Focused: true},
		{WorkspaceID: "w2", Number: 2, Label: "beta"},
		{WorkspaceID: "w3", Number: 3, Label: "gamma"},
	}
}

func testAgents() []herdr.Agent {
	return []herdr.Agent{
		{Agent: "A", DisplayAgent: "Agent A", AgentStatus: "idle", PaneID: "pA", WorkspaceID: "w1"},
		{Agent: "B", DisplayAgent: "Agent B", AgentStatus: "blocked", PaneID: "pB", WorkspaceID: "w2"},
	}
}

func testPanes() []herdr.Pane {
	return []herdr.Pane{
		{PaneID: "pA", WorkspaceID: "w1", Agent: "A", AgentStatus: "idle"},
		{PaneID: "pB", WorkspaceID: "w2", Agent: "B", AgentStatus: "blocked"},
	}
}

// canonicalCodes is the cross-surface truth: what the sidebar badges and jump
// popup assign for this layout. Item keys use the same derivation
// ("workspace:<id>" / "agent:<pane id>"), so a direct map lookup pins parity.
func canonicalCodes() map[string]string {
	out := map[string]string{}
	for _, e := range jump.Entries(testSpaces(), testAgents(), testPool) {
		out[string(e.Kind)+":"+e.ID] = e.Code
	}
	return out
}

// stubSocket serves one canned result per method; each connection carries
// exactly one request, like the real herdr socket.
func stubSocket(t *testing.T, results map[string]any) *herdr.Client {
	t.Helper()
	sock := filepath.Join(t.TempDir(), "herdr.sock")
	ln, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	t.Cleanup(func() { _ = ln.Close() })
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				defer func() { _ = c.Close() }()
				line, err := bufio.NewReader(c).ReadBytes('\n')
				if err != nil {
					return
				}
				var req struct {
					ID     string `json:"id"`
					Method string `json:"method"`
				}
				if err := json.Unmarshal(line, &req); err != nil {
					return
				}
				out, err := json.Marshal(map[string]any{"id": req.ID, "result": results[req.Method]})
				if err != nil {
					return
				}
				_, _ = c.Write(append(out, '\n'))
			}(conn)
		}
	}()
	return herdr.NewClient(sock)
}

func stubResults() map[string]any {
	return map[string]any{
		"workspace.list": map[string]any{"workspaces": testSpaces()},
		"agent.list":     map[string]any{"agents": testAgents()},
		"pane.list":      map[string]any{"panes": testPanes()},
	}
}

func newReloadedModel(t *testing.T, mode Mode, sort string) *Model {
	t.Helper()
	dir := t.TempDir()
	m := New(mode, config.Config{Charset: testPool, Sort: sort, Preview: true},
		stubSocket(t, stubResults()), mru.Load(dir), dir, "", "")
	m.reload()
	if m.errMsg != "" {
		t.Fatalf("reload: %s", m.errMsg)
	}
	return &m
}

func assertCanonicalParity(t *testing.T, m *Model) {
	t.Helper()
	want := canonicalCodes()
	for i, it := range m.allItems {
		if it.Code != want[it.Key] {
			t.Errorf("%s: code = %q, want canonical %q", it.Key, it.Code, want[it.Key])
		}
		if string(m.allCodes[i]) != it.Code {
			t.Errorf("allCodes[%d] = %q, not parallel to allItems (%q)", i, m.allCodes[i], it.Code)
		}
	}
}

// Under priority sorting the blocked beta rises to the top, yet every row
// still shows its canonical code — not the code its new position implies.
func TestSpacesCodesCanonicalUnderPrioritySort(t *testing.T) {
	m := newReloadedModel(t, ModeSpaces, "priority")
	if m.allItems[0].Key != "workspace:w2" {
		t.Fatalf("fixture: expected blocked beta to lead the sorted list, got %s", m.allItems[0].Key)
	}
	if m.allItems[0].Code != "sf" {
		t.Fatalf("beta moved to row 0 and was relabeled %q; must keep canonical sf", m.allItems[0].Code)
	}
	assertCanonicalParity(t, m)
}

// Searching "beta" leaves one row; filtering hides and reorders, it never
// relabels — the survivor keeps its canonical code and the resolve table
// still spans every target.
func TestSpacesCodesSurviveFilter(t *testing.T) {
	m := newReloadedModel(t, ModeSpaces, "priority")
	m.search = "beta"
	m.applyFilter()
	if len(m.items) != 1 || m.items[0].Key != "workspace:w2" {
		t.Fatalf("filter: got %d item(s), want exactly beta", len(m.items))
	}
	if m.items[0].Code != "sf" {
		t.Fatalf("filtered beta shows %q; codes must survive filtering", m.items[0].Code)
	}
	if len(m.allCodes) != 3 {
		t.Fatalf("resolve table rebuilt to %d entries; it must keep covering all targets", len(m.allCodes))
	}
	assertCanonicalParity(t, m)
}

// The agent picker continues the shared sequence AFTER the workspace codes
// (jump.Entries offset), even though no workspace is displayed there —
// and even when priority sorting reorders the agents themselves.
func TestAgentsCodesContinueWorkspaceSequence(t *testing.T) {
	m := newReloadedModel(t, ModeAgents, "priority")
	if m.nSpaces != 3 {
		t.Fatalf("agents picker did not observe the workspace count (nSpaces=%d)", m.nSpaces)
	}
	if m.allItems[0].Key != "agent:pB" {
		t.Fatalf("fixture: expected blocked agent B to lead the sorted list, got %s", m.allItems[0].Key)
	}
	if m.allItems[0].Code != "sl" {
		t.Fatalf("agent B sorted first shows %q; must keep canonical sl (not the sequence head)", m.allItems[0].Code)
	}
	if m.allItems[1].Code != "sk" {
		t.Fatalf("agent A shows %q; want canonical sk", m.allItems[1].Code)
	}
	assertCanonicalParity(t, m)
}

// Typing a code learned from the sidebar must select its target: directly
// when visible, and through an active filter by revealing the full list.
func TestTypedCodeSelectsTargetAcrossFilter(t *testing.T) {
	m := newReloadedModel(t, ModeSpaces, "priority")

	// Visible case: gamma sits under the search "ga"; sf (beta) is hidden.
	m.search = "ga"
	m.applyFilter()
	if len(m.items) != 1 || m.items[0].Key != "workspace:w3" {
		t.Fatalf("fixture: expected only gamma visible under \"ga\"")
	}
	idx, _, ok := codes.Resolve("sf", m.allCodes)
	if !ok || idx < 0 {
		t.Fatalf("Resolve(sf): ok=%v idx=%d — canonical beta code must resolve", ok, idx)
	}
	m.selectByCode(idx)
	if m.search != "" || m.filterActive {
		t.Fatalf("selecting a hidden target must clear the filter: search=%q filterActive=%v", m.search, m.filterActive)
	}
	if cur := m.items[m.cursor]; cur.Key != "workspace:w2" || cur.Code != "sf" {
		t.Fatalf("cursor landed on %s (%q); want beta (sf) after reveal", cur.Key, cur.Code)
	}

	// Visible case: same code again, now directly on screen.
	idx, _, ok = codes.Resolve("sj", m.allCodes)
	if !ok || idx < 0 {
		t.Fatalf("Resolve(sj): ok=%v idx=%d", ok, idx)
	}
	m.selectByCode(idx)
	if cur := m.items[m.cursor]; cur.Key != "workspace:w3" {
		t.Fatalf("cursor landed on %s; want gamma after direct select", cur.Key)
	}
}
