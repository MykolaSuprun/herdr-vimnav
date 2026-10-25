// Package herdr is a minimal client for herdr's newline-delimited JSON
// socket API. One request per connection, mirroring the server's model.
package herdr

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"time"
)

// Client talks to a running herdr server over its Unix socket.
type Client struct {
	socketPath string
	timeout    time.Duration
}

// NewClient returns a client for the given socket path.
func NewClient(socketPath string) *Client {
	return &Client{socketPath: socketPath, timeout: 5 * time.Second}
}

type request struct {
	ID     string `json:"id"`
	Method string `json:"method"`
	Params any    `json:"params"`
}

type response struct {
	ID     string          `json:"id"`
	Result json.RawMessage `json:"result"`
	Error  *apiError       `json:"error"`
}

type apiError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func (e *apiError) Error() string {
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

// Call sends one request and decodes its result into out (when non-nil).
func (c *Client) Call(method string, params map[string]any, out any) error {
	if params == nil {
		params = map[string]any{}
	}

	payload, err := json.Marshal(request{ID: "preview-grid", Method: method, Params: params})
	if err != nil {
		return err
	}

	conn, err := net.DialTimeout("unix", c.socketPath, c.timeout)
	if err != nil {
		return fmt.Errorf("connect to herdr socket: %w", err)
	}
	defer func() { _ = conn.Close() }() // read-only path; close errors are irrelevant

	_ = conn.SetDeadline(time.Now().Add(c.timeout))
	if _, err := conn.Write(append(payload, '\n')); err != nil {
		return fmt.Errorf("send %s: %w", method, err)
	}

	line, err := bufio.NewReader(conn).ReadBytes('\n')
	if err != nil && len(line) == 0 {
		return fmt.Errorf("read %s response: %w", method, err)
	}

	var resp response
	if err := json.Unmarshal(line, &resp); err != nil {
		return fmt.Errorf("decode %s response: %w", method, err)
	}
	if resp.Error != nil {
		return resp.Error
	}
	if out == nil {
		return nil
	}
	if err := json.Unmarshal(resp.Result, out); err != nil {
		return fmt.Errorf("decode %s payload: %w", method, err)
	}
	return nil
}

// --- Typed API surface -----------------------------------------------------

// Workspace mirrors one entry of workspace.list.
type Workspace struct {
	WorkspaceID string `json:"workspace_id"`
	Number      int    `json:"number"`
	Label       string `json:"label"`
	Focused     bool   `json:"focused"`
	PaneCount   int    `json:"pane_count"`
	TabCount    int    `json:"tab_count"`
	ActiveTabID string `json:"active_tab_id"`
	AgentStatus string `json:"agent_status"`
}

// Pane mirrors one entry of pane.list.
type Pane struct {
	PaneID        string `json:"pane_id"`
	TerminalID    string `json:"terminal_id"`
	WorkspaceID   string `json:"workspace_id"`
	TabID         string `json:"tab_id"`
	Focused       bool   `json:"focused"`
	Cwd           string `json:"cwd"`
	ForegroundCwd string `json:"foreground_cwd"`
	Agent         string `json:"agent"`
	AgentStatus   string `json:"agent_status"`
	TerminalTitle string `json:"terminal_title_stripped"`
}

// Agent mirrors one entry of agent.list.
type Agent struct {
	Agent         string `json:"agent"`
	DisplayAgent  string `json:"display_agent"` // rename-aware name from the herdr panel
	AgentStatus   string `json:"agent_status"`
	PaneID        string `json:"pane_id"`
	WorkspaceID   string `json:"workspace_id"`
	TabID         string `json:"tab_id"`
	Cwd           string `json:"cwd"`
	Focused       bool   `json:"focused"`
	TerminalTitle string `json:"terminal_title_stripped"`
}

// Tab mirrors one entry of tab.list (workspace-major; unnamed tabs carry
// their number as the label, like herdr's own UI).
type Tab struct {
	TabID       string `json:"tab_id"`
	WorkspaceID string `json:"workspace_id"`
	Label       string `json:"label"`
	Number      int    `json:"number"`
	PaneCount   int    `json:"pane_count"`
	AgentStatus string `json:"agent_status"`
	Focused     bool   `json:"focused"`
}

type workspaceList struct {
	Workspaces []Workspace `json:"workspaces"`
}

type paneList struct {
	Panes []Pane `json:"panes"`
}

type agentList struct {
	Agents []Agent `json:"agents"`
}

type tabList struct {
	Tabs []Tab `json:"tabs"`
}

type paneRead struct {
	Read struct {
		Text string `json:"text"`
	} `json:"read"`
}

// Workspaces returns all workspaces.
func (c *Client) Workspaces() ([]Workspace, error) {
	var out workspaceList
	if err := c.Call("workspace.list", nil, &out); err != nil {
		return nil, err
	}
	return out.Workspaces, nil
}

// Panes returns all panes.
func (c *Client) Panes() ([]Pane, error) {
	var out paneList
	if err := c.Call("pane.list", nil, &out); err != nil {
		return nil, err
	}
	return out.Panes, nil
}

// Agents returns all detected agents.
func (c *Client) Agents() ([]Agent, error) {
	var out agentList
	if err := c.Call("agent.list", nil, &out); err != nil {
		return nil, err
	}
	return out.Agents, nil
}

// Tabs returns all tabs across workspaces (tab.list is workspace-major).
func (c *Client) Tabs() ([]Tab, error) {
	var out tabList
	if err := c.Call("tab.list", nil, &out); err != nil {
		return nil, err
	}
	return out.Tabs, nil
}

// FocusTab focuses a tab.
func (c *Client) FocusTab(id string) error {
	return c.Call("tab.focus", map[string]any{"tab_id": id}, nil)
}

// RenameTab sets a tab's label. Used by the jump popup to carry codes on the
// tabs while it is open (herdr 0.9.5 has no tab-metadata surface); the
// original labels are restored on the way out.
func (c *Client) RenameTab(id, label string) error {
	return c.Call("tab.rename", map[string]any{"tab_id": id, "label": label}, nil)
}

// FocusPane focuses a pane directly (schema: PaneTarget{pane_id}; present in
// herdr 0.9.5's bundled API even though the online method table omits it).
// Callers on older servers should degrade on error, e.g. to FocusTab of the
// pane's tab.
func (c *Client) FocusPane(id string) error {
	return c.Call("pane.focus", map[string]any{"pane_id": id}, nil)
}

// ReadPane returns the pane's rendered output (optionally ANSI).
func (c *Client) ReadPane(paneID, source string, lines int, ansi bool) (string, error) {
	format := "text"
	if ansi {
		format = "ansi"
	}
	var out paneRead
	err := c.Call("pane.read", map[string]any{
		"pane_id": paneID,
		"source":  source,
		"lines":   lines,
		"format":  format,
	}, &out)
	if err != nil {
		return "", err
	}
	return out.Read.Text, nil
}

// FocusWorkspace focuses a workspace.
func (c *Client) FocusWorkspace(id string) error {
	return c.Call("workspace.focus", map[string]any{"workspace_id": id}, nil)
}

// FocusAgent focuses the pane hosting an agent (pane id or live name).
func (c *Client) FocusAgent(target string) error {
	return c.Call("agent.focus", map[string]any{"target": target}, nil)
}

// ReportWorkspaceMetadata sets display-only metadata tokens on a workspace
// (the sidebar renders them through $name rows). Re-reporting with the same
// source replaces that source's earlier tokens.
func (c *Client) ReportWorkspaceMetadata(workspaceID, source string, tokens map[string]string) error {
	return c.Call("workspace.report_metadata", map[string]any{
		"workspace_id": workspaceID,
		"source":       source,
		"tokens":       tokens,
	}, nil)
}

// ReportPaneMetadata sets display-only metadata tokens on a pane.
func (c *Client) ReportPaneMetadata(paneID, source string, tokens map[string]string) error {
	return c.Call("pane.report_metadata", map[string]any{
		"pane_id": paneID,
		"source":  source,
		"tokens":  tokens,
	}, nil)
}

// ErrNotInHerdr reports a missing plugin runtime environment.
var ErrNotInHerdr = errors.New("not running inside herdr (HERDR_SOCKET_PATH unset)")
