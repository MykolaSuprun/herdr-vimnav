// Command demo-server is a fake herdr socket server for screenshots, demos,
// and manual UI checks without touching a live session. It speaks the same
// newline-delimited JSON protocol the plugin's client expects (one request per
// connection) and serves a curated layout: 10 spaces (so jump codes show both
// pair codes and the triple tier), a 4-digit number, duplicate labels, two multi-tab spaces
// (tabs carry codes too), two attention items, and ANSI-rich pane content.
//
// Usage:
//
//	go run ./tools/demo-server /tmp/herdr-demo.sock
//	HERDR_SOCKET_PATH=/tmp/herdr-demo.sock ./bin/herdr-preview-grid spaces
package main

import (
	"bufio"
	"encoding/json"
	"log"
	"net"
	"os"
)

// --- fixture ---------------------------------------------------------------

type ws struct {
	id     string
	num    int
	label  string
	status string
	focus  bool
}

var spaces = []ws{
	{"w1", 12, "nixvim-config", "working", false},
	{"w2", 7, "herdr-rework", "idle", true},
	{"w3", 9, "docs-site", "", false},
	{"w4", 1042, "archive", "", false},
	{"w5", 4, "claude-scratch", "idle", false},
	{"w6", 6, "openclaw-tui", "", false},
	{"w7", 11, "research-openclaw", "working", false},
	{"w8", 2, "qs-flow", "", false},
	{"w9", 13, "nixvim-config", "", false},
	{"w10", 5, "general-1", "", false},
}

type pane struct {
	id, ws, tab, title string
	cwd, fgCwd         string
	agent, status      string
	focus              bool
}

// Tabs exist as a hierarchy level (workspace → tab → pane); live herdr
// serves unnamed tabs with their number as the label, so the fixture mixes
// named and unnamed tabs across two multi-tab spaces.
type demoTab struct {
	id, ws, label string
	num           int
}

var tabs = []demoTab{
	{"w1:t1", "w1", "editor", 21},
	{"w2:t1", "w2", "main", 22},
	{"w2:t2", "w2", "logs", 23},
	{"w3:t1", "w3", "1", 24},
	{"w4:t1", "w4", "1", 25},
	{"w5:t1", "w5", "1", 26},
	{"w6:t1", "w6", "dev", 27},
	{"w7:t1", "w7", "1", 28},
	{"w8:t1", "w8", "1", 29},
	{"w9:t1", "w9", "1", 30},
	{"w10:t1", "w10", "obsidian", 31},
	{"w10:t2", "w10", "raw", 32},
}

// Pane ids are workspace-scoped (w2:p3 — never tab-qualified), matching
// herdr's public id grammar; the tab field carries the hierarchy level.
// Agent fields mirror agent.list so the popup's pane section (plain panes
// only) has an honest fixture to exclude.
var panes = []pane{
	{"w1:p1", "w1", "w1:t1", "vim src/config.go", "/home/dev/archives/old/nixvim", "/home/dev/workspaces/src/nixvim", "", "", true},
	{"w1:p2", "w1", "w1:t1", "aider", "/home/dev/workspaces/src/nixvim", "", "aider", "idle", false},
	{"w2:p1", "w2", "w2:t1", "herdr ▸ plugin pane open …", "/home/dev/workspaces/src/herdr", "", "", "", true},
	{"w2:p2", "w2", "w2:t1", "golangci-lint run ./…", "/home/dev/workspaces/src/herdr-preview-grid", "/home/dev/workspaces/src/herdr", "opencode", "working", false},
	{"w2:p3", "w2", "w2:t2", "tail -f plugin.log", "/home/dev/workspaces/src/herdr-preview-grid", "", "", "", false},
	{"w3:p1", "w3", "w3:t1", "npm run dev", "/home/dev/workspaces/src/docs-site", "", "", "", true},
	{"w3:p2", "w3", "w3:t1", "npm test", "/home/dev/workspaces/src/docs-site", "", "gemini-cli", "idle", false},
	{"w4:p1", "w4", "w4:t1", "zsh", "/home/dev/archive", "", "", "", true},
	{"w5:p1", "w5", "w5:t1", "claude ▸ refactor parser", "/home/dev/workspaces/src/nixvim", "", "claude-code", "blocked", true},
	{"w6:p1", "w6", "w6:t1", "openclaw ▸ run", "/home/dev/workspaces/src/openclaw", "", "opencode", "done", true},
	{"w7:p1", "w7", "w7:t1", "rg -n 'agent.detected' ~/workspaces", "/home/dev/workspaces/src/herdr-research", "", "claude-code", "working", true},
	{"w8:p1", "w8", "w8:t1", "quickshell", "/home/dev/workspaces/src/qs-flow", "", "", "", true},
	{"w8:p2", "w8", "w8:t1", "bun run dev", "/home/dev/workspaces/src/qs-flow", "", "crush", "", false},
	{"w9:p1", "w9", "w9:t1", "git rebase -i main", "/home/dev/dotfiles/nixvim", "", "", "", true},
	{"w9:p2", "w9", "w9:t1", "droid", "/home/dev/dotfiles/nixvim", "", "droid", "working", false},
	{"w10:p1", "w10", "w10:t1", "obsidian ▸ task_backlog", "/home/dev/Documents/vaults/general-1", "", "", "", true},
	{"w10:p2", "w10", "w10:t2", "tail -f plugin.log", "/home/dev/Documents/vaults/general-1", "", "", "", false},
}

type agent struct {
	paneID, wsID, tab, name, display, status, cwd, title string
	focus                                                bool
}

var agents = []agent{
	{"w5:p1", "w5", "w5:t1", "claude-code", "claude", "blocked", "/home/dev/workspaces/src/nixvim", "refactor parser ▸ 3 tests left", false},
	{"w6:p1", "w6", "w6:t1", "opencode", "", "done", "/home/dev/workspaces/src/openclaw", "session summary ready", false},
	{"w7:p1", "w7", "w7:t1", "claude-code", "", "working", "/home/dev/workspaces/src/herdr-research", "rg agent.detected …", false},
	{"w2:p2", "w2", "w2:t1", "opencode", "oc", "working", "/home/dev/workspaces/src/herdr-preview-grid", "golangci-lint run ./…", true},
	{"w1:p2", "w1", "w1:t1", "aider", "", "idle", "/home/dev/workspaces/src/nixvim", "", false},
	{"w3:p2", "w3", "w3:t1", "gemini-cli", "", "idle", "/home/dev/workspaces/src/docs-site", "", false},
	{"w8:p2", "w8", "w8:t1", "crush", "", "", "/home/dev/workspaces/src/qs-flow", "", false},
	{"w9:p2", "w9", "w9:t1", "droid", "factory", "working", "/home/dev/dotfiles/nixvim", "rebase -i main", false},
}

// previewText is the pane content returned by pane.read. The first block is
// rich ANSI (a test run + git summary) for whichever pane leads the demo;
// everything else gets a short plausible transcript.
var previewText = map[string]string{
	"w2:p2": "\x1b[1;34m❯\x1b[0m \x1b[1mgolangci-lint run ./...\x1b[0m\r" + "\n" +
		"0 issues.\r\x1b[1;32mLINT_CLEAN\x1b[0m\r" + "\n" +
		"\x1b[2mreviewed 612 statements in 8 packages\x1b[0m",
	"w2:p1": "\x1b[1;34m❯\x1b[0m \x1b[1mgo test -race ./...\x1b[0m\r" + "\n" +
		"\x1b[32mok\x1b[0m  herdr-preview-grid/internal/app       \x1b[2m0.021s\x1b[0m\r" + "\n" +
		"\x1b[32mok\x1b[0m  herdr-preview-grid/internal/codes     \x1b[2m0.002s\x1b[0m\r" + "\n" +
		"\x1b[32mok\x1b[0m  herdr-preview-grid/internal/config    \x1b[2m0.004s\x1b[0m\r" + "\n" +
		"\x1b[32mok\x1b[0m  herdr-preview-grid/internal/herdr     \x1b[2m0.307s\x1b[0m\r" + "\n" +
		"\x1b[32mok\x1b[0m  herdr-preview-grid/internal/jump      \x1b[2m0.003s\x1b[0m\r" + "\n" +
		"\x1b[32mok\x1b[0m  herdr-preview-grid/internal/mru       \x1b[2m0.003s\x1b[0m\r" + "\n" +
		"\r" + "\n" +
		"\x1b[1;32mPASS\x1b[0m 6 packages · coverage \x1b[1m87.4%\x1b[0m\r" + "\n" +
		"\r" + "\n" +
		"\x1b[33mcommit\x1b[0m \x1b[1m4c81673\x1b[0m  pair badges color through the gap\r" + "\n" +
		" \x1b[1mAuthor:\x1b[0m Mykola Suprun <mykola.suprun@protonmail.com>\r" + "\n" +
		" \x1b[1mDate:\x1b[0m   Sun Oct 18 04:12:07 2026 +0200\r" + "\n" +
		"\r" + "\n" +
		" \x1b[32m 3 files changed\x1b[0m, 24 insertions(+), 26 deletions(-)",
	"w7:p1": "\x1b[1;34m❯\x1b[0m rg -n agent.detected ~/workspaces\r" + "\n" +
		"src/herdr/src/agent.rs:412:pub fn agent_detected(ev: &Event) {\r" + "\n" +
		"src/plugin/mod.rs:88:    on: \"pane.agent_detected\",\r" + "\n" +
		"\x1b[2m12 files changed\x1b[0m",
}

var defaultText = "\x1b[2m$ session idle — no recent output\x1b[0m"

// --- server ----------------------------------------------------------------

type request struct {
	ID     string         `json:"id"`
	Method string         `json:"method"`
	Params map[string]any `json:"params"`
}

func handle(line []byte) map[string]any {
	var req request
	if err := json.Unmarshal(line, &req); err != nil {
		return map[string]any{"id": "", "error": map[string]string{"code": "bad_request", "message": err.Error()}}
	}
	switch req.Method {
	case "workspace.list":
		list := make([]map[string]any, 0, len(spaces))
		for _, w := range spaces {
			list = append(list, map[string]any{
				"workspace_id": w.id, "number": w.num, "label": w.label,
				"focused":       w.focus,
				"pane_count":    countPanes(w.id),
				"tab_count":     countTabs(w.id),
				"active_tab_id": activeTab(w.id), "agent_status": w.status,
			})
		}
		return map[string]any{"id": req.ID, "result": map[string]any{"workspaces": list}}
	case "tab.list":
		list := make([]map[string]any, 0, len(tabs))
		for _, t := range tabs {
			list = append(list, map[string]any{
				"tab_id": t.id, "workspace_id": t.ws, "label": t.label,
				"number": t.num, "pane_count": countPanesInTab(t.id),
				"agent_status": tabStatus(t.id), "focused": t.id == "w2:t1",
			})
		}
		return map[string]any{"id": req.ID, "result": map[string]any{"tabs": list}}
	case "pane.list":
		list := make([]map[string]any, 0, len(panes))
		for _, p := range panes {
			list = append(list, map[string]any{
				"pane_id": p.id, "workspace_id": p.ws, "tab_id": p.tab,
				"focused": p.focus, "cwd": p.cwd, "foreground_cwd": p.fgCwd,
				"agent": p.agent, "agent_status": p.status,
				"terminal_title_stripped": p.title,
			})
		}
		return map[string]any{"id": req.ID, "result": map[string]any{"panes": list}}
	case "agent.list":
		list := make([]map[string]any, 0, len(agents))
		for _, a := range agents {
			list = append(list, map[string]any{
				"agent": a.name, "display_agent": a.display, "agent_status": a.status,
				"pane_id": a.paneID, "workspace_id": a.wsID, "tab_id": a.tab,
				"cwd": a.cwd, "focused": a.focus, "terminal_title_stripped": a.title,
			})
		}
		return map[string]any{"id": req.ID, "result": map[string]any{"agents": list}}
	case "pane.read":
		id, _ := req.Params["pane_id"].(string)
		text, ok := previewText[id]
		if !ok {
			text = defaultText
		}
		return map[string]any{"id": req.ID, "result": map[string]any{"read": map[string]string{"text": text}}}
	default: // focus/report calls: accept and ignore (read-only demo)
		return map[string]any{"id": req.ID, "result": map[string]any{}}
	}
}

// --- fixture roll-ups (one source of truth per fact) ------------------------

func countPanes(ws string) int {
	n := 0
	for _, p := range panes {
		if p.ws == ws {
			n++
		}
	}
	return n
}

func countTabs(ws string) int {
	n := 0
	for _, t := range tabs {
		if t.ws == ws {
			n++
		}
	}
	return n
}

// activeTab mirrors herdr: the workspace's first tab (fallback synthetic id
// for spaces the tabs fixture somehow missed).
func activeTab(ws string) string {
	for _, t := range tabs {
		if t.ws == ws {
			return t.id
		}
	}
	return ws + ":t1"
}

func countPanesInTab(tabID string) int {
	n := 0
	for _, p := range panes {
		if p.tab == tabID {
			n++
		}
	}
	return n
}

// tabStatus rolls a tab's panes up like herdr does: the most demanding
// member state wins, plain-shell tabs read unknown.
func tabStatus(tabID string) string {
	statuses := make([]string, 0, 2)
	for _, p := range panes {
		if p.tab == tabID && p.status != "" {
			statuses = append(statuses, p.status)
		}
	}
	for _, want := range []string{"blocked", "done", "working", "idle"} {
		for _, s := range statuses {
			if s == want {
				return s
			}
		}
	}
	return "unknown"
}

func serve(conn net.Conn) {
	defer func() { _ = conn.Close() }()
	line, err := bufio.NewReader(conn).ReadBytes('\n')
	if err != nil {
		return
	}
	out, err := json.Marshal(handle(line))
	if err != nil {
		return
	}
	_, _ = conn.Write(append(out, '\n'))
}

func main() {
	if len(os.Args) < 2 {
		log.Fatal("usage: demo-server <socket-path>")
	}
	sock := os.Args[1]
	_ = os.Remove(sock)
	ln, err := net.Listen("unix", sock)
	if err != nil {
		log.Fatalf("listen %s: %v", sock, err)
	}
	defer func() { _ = ln.Close() }()
	log.Printf("demo herdr listening on %s", sock)
	for {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		go serve(conn)
	}
}
