package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/MykolaSuprun/herdr-preview-grid/internal/jump"
	"github.com/MykolaSuprun/herdr-preview-grid/internal/mru"
)

// jumpMode (the popup TUI) is deliberately not covered here: it drives a
// Bubble Tea program through the TTY lifecycle, and its decision logic
// (Match/Entries) lives in — and is covered by — internal/jump. The
// env-driven non-TUI modes get full coverage below.

func TestRecordModeTouchesLedger(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HERDR_PLUGIN_STATE_DIR", dir)
	t.Setenv("HERDR_PLUGIN_EVENT_JSON", `{"type":"workspace_focused","workspace_id":"w9"}`)

	if err := recordMode(); err != nil {
		t.Fatalf("recordMode: %v", err)
	}
	if got := mru.Load(dir).Use["workspace:w9"]; got == 0 {
		t.Fatal("ledger not touched for workspace_focused")
	}
}

func TestRecordModeIgnoresUnusablePayloads(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HERDR_PLUGIN_STATE_DIR", dir)
	for _, raw := range []string{
		"",                                      // hook fired without payload
		`{"type":"pane_closed","pane_id":"p1"}`, // unrelated event
		"{not json",                             // malformed
		`{"type":"pane_focused"}`,               // missing id
	} {
		t.Setenv("HERDR_PLUGIN_EVENT_JSON", raw)
		if err := recordMode(); err != nil {
			t.Fatalf("recordMode(%q): %v", raw, err)
		}
	}
	if _, err := os.Stat(filepath.Join(dir, "mru.json")); !os.IsNotExist(err) {
		t.Fatal("ledger written for unusable payloads")
	}
}

// sync-codes end to end: one workspace + one agent over a stub socket, the
// canonical codes land in the state file, and a second pass is diff-silent.
func TestSyncCodesModeEndToEnd(t *testing.T) {
	sock := filepath.Join(t.TempDir(), "herdr.sock")
	ln, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	t.Cleanup(func() { _ = ln.Close() })

	var mu sync.Mutex
	var reports []string
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go func() {
				defer func() { _ = conn.Close() }()
				line, err := bufio.NewReader(conn).ReadBytes('\n')
				if err != nil {
					return
				}
				var req struct {
					ID     string         `json:"id"`
					Method string         `json:"method"`
					Params map[string]any `json:"params"`
				}
				if err := json.Unmarshal(line, &req); err != nil {
					return
				}
				var result any
				switch req.Method {
				case "workspace.list":
					result = map[string]any{"workspaces": []map[string]any{
						{"workspace_id": "w9", "number": 9},
					}}
				case "agent.list":
					result = map[string]any{"agents": []map[string]any{
						{"pane_id": "p1", "agent": "oc"},
					}}
				case "workspace.report_metadata", "pane.report_metadata":
					id, _ := req.Params["workspace_id"].(string)
					if id == "" {
						id, _ = req.Params["pane_id"].(string)
					}
					mu.Lock()
					reports = append(reports, fmt.Sprintf("%s:%s=%v", req.Method, id, req.Params["tokens"].(map[string]any)["jump"]))
					mu.Unlock()
				}
				out, err := json.Marshal(map[string]any{"id": req.ID, "result": result})
				if err != nil {
					return
				}
				_, _ = conn.Write(append(out, '\n'))
			}()
		}
	}()

	stateDir := t.TempDir()
	t.Setenv("HERDR_SOCKET_PATH", sock)
	t.Setenv("HERDR_PLUGIN_STATE_DIR", stateDir)
	t.Setenv("XDG_CONFIG_HOME", t.TempDir()) // config fallback: no file → defaults

	if err := syncCodesMode(); err != nil {
		t.Fatalf("syncCodesMode pass 1: %v", err)
	}
	st := jump.LoadState(stateDir)
	if st.Spaces["w9"] != "j" || st.Agents["p1"] != "k" {
		t.Fatalf("state wrong: %+v", st)
	}
	mu.Lock()
	firstPass := len(reports)
	mu.Unlock()
	if firstPass != 2 {
		t.Fatalf("pass 1 reports = %d, want 2 (%v)", firstPass, reports)
	}

	// Second pass: diff-based, nothing changed → no re-reports.
	if err := syncCodesMode(); err != nil {
		t.Fatalf("syncCodesMode pass 2: %v", err)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(reports) != firstPass {
		t.Fatalf("pass 2 re-reported %d unchanged tokens", len(reports)-firstPass)
	}
}
