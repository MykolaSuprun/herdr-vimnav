package herdr

import (
	"bufio"
	"encoding/json"
	"net"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// reply is one canned stub-server response. raw is sent verbatim when set
// (malformed-response cases); delay stalls before replying (deadline cases).
type reply struct {
	result any
	err    *apiError
	raw    string
	delay  time.Duration
}

// stub is a one-shot herdr socket. Each Call dials a fresh connection and
// the server handles exactly one, recording the request before replying —
// reading stub.req after <-done is race-free.
type stub struct {
	client *Client
	req    *request
	done   <-chan struct{}
}

// newStub serves fn's canned reply to every request the client sends.
func newStub(t *testing.T, fn func(request) reply) *stub {
	t.Helper()
	dir := t.TempDir()
	sock := filepath.Join(dir, "herdr.sock")
	ln, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	t.Cleanup(func() { _ = ln.Close() })

	s := &stub{req: &request{}, client: &Client{socketPath: sock, timeout: 5 * time.Second}}
	done := make(chan struct{})
	s.done = done
	go func() {
		defer close(done)
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer func() { _ = conn.Close() }()
		line, err := bufio.NewReader(conn).ReadBytes('\n')
		if err != nil {
			return
		}
		if err := json.Unmarshal(line, s.req); err != nil {
			return
		}
		r := fn(*s.req)
		if r.delay > 0 {
			time.Sleep(r.delay)
		}
		if r.raw != "" {
			_, _ = conn.Write([]byte(r.raw))
			return
		}
		resp := response{ID: s.req.ID, Error: r.err}
		if r.result != nil {
			raw, err := json.Marshal(r.result)
			if err != nil {
				return
			}
			resp.Result = raw
		}
		out, err := json.Marshal(resp)
		if err != nil {
			return
		}
		_, _ = conn.Write(append(out, '\n'))
	}()
	return s
}

func TestCallDecodesTypedResult(t *testing.T) {
	s := newStub(t, func(request) reply {
		return reply{result: workspaceList{Workspaces: []Workspace{
			{WorkspaceID: "w1", Number: 3, Label: "one"},
			{WorkspaceID: "w2", Number: 1, Label: "two", Focused: true},
		}}}
	})
	got, err := s.client.Workspaces()
	<-s.done
	if err != nil {
		t.Fatalf("Workspaces: %v", err)
	}
	if len(got) != 2 || got[0].Number != 3 || got[1].WorkspaceID != "w2" || !got[1].Focused {
		t.Fatalf("decoded wrong: %+v", got)
	}
}

func TestCallSurfacesAPIError(t *testing.T) {
	s := newStub(t, func(request) reply {
		return reply{err: &apiError{
			Code:    "invalid_params",
			Message: "overlay and popup plugin panes target the active pane",
		}}
	})
	err := s.client.FocusWorkspace("w1")
	<-s.done
	if err == nil || !strings.Contains(err.Error(), "invalid_params") ||
		!strings.Contains(err.Error(), "active pane") {
		t.Fatalf("err = %v", err)
	}
}

func TestCallConnectFailureIsWrapped(t *testing.T) {
	c := &Client{socketPath: filepath.Join(t.TempDir(), "absent.sock"), timeout: time.Second}
	err := c.FocusWorkspace("w1")
	if err == nil || !strings.Contains(err.Error(), "connect to herdr socket") {
		t.Fatalf("err = %v", err)
	}
}

func TestCallMalformedResponseIsWrapped(t *testing.T) {
	s := newStub(t, func(request) reply { return reply{raw: "not json at all\n"} })
	err := s.client.FocusWorkspace("w1")
	<-s.done
	if err == nil || !strings.Contains(err.Error(), "decode workspace.focus response") {
		t.Fatalf("err = %v", err)
	}
}

func TestCallPayloadTypeMismatchIsWrapped(t *testing.T) {
	s := newStub(t, func(request) reply { return reply{result: "a string, not an object"} })
	_, err := s.client.Workspaces()
	<-s.done
	if err == nil || !strings.Contains(err.Error(), "decode workspace.list payload") {
		t.Fatalf("err = %v", err)
	}
}

func TestCallReadTimeoutIsWrapped(t *testing.T) {
	s := newStub(t, func(request) reply {
		return reply{result: paneList{}, delay: 300 * time.Millisecond}
	})
	s.client.timeout = 50 * time.Millisecond // stall the server past the deadline
	_, err := s.client.Panes()
	<-s.done
	if err == nil || !strings.Contains(err.Error(), "read pane.list response") {
		t.Fatalf("err = %v", err)
	}
}

func TestTabsDecodeLiveShape(t *testing.T) {
	s := newStub(t, func(request) reply {
		return reply{result: tabList{Tabs: []Tab{
			{TabID: "wA:tC", WorkspaceID: "wA", Label: "embeddings", Number: 12, PaneCount: 2},
			{TabID: "wA:tK", WorkspaceID: "wA", Label: "3", Number: 19, PaneCount: 2, Focused: true},
		}}}
	})
	got, err := s.client.Tabs()
	<-s.done
	if err != nil {
		t.Fatalf("Tabs: %v", err)
	}
	if s.req.Method != "tab.list" {
		t.Fatalf("method = %q", s.req.Method)
	}
	// Unnamed tabs carry their number as the label, like herdr's own UI.
	if len(got) != 2 || got[0].Label != "embeddings" || got[0].Number != 12 ||
		got[1].Label != "3" || !got[1].Focused {
		t.Fatalf("decoded wrong: %+v", got)
	}
}

func TestFocusTabSendsTabID(t *testing.T) {
	s := newStub(t, func(request) reply { return reply{result: map[string]any{}} })
	if err := s.client.FocusTab("wA:tC"); err != nil {
		t.Fatalf("FocusTab: %v", err)
	}
	<-s.done
	if s.req.Method != "tab.focus" {
		t.Fatalf("method = %q", s.req.Method)
	}
	if params, _ := s.req.Params.(map[string]any); params["tab_id"] != "wA:tC" {
		t.Fatalf("params = %v, want tab_id wA:tC", s.req.Params)
	}
}

func TestFocusPaneSendsPaneID(t *testing.T) {
	s := newStub(t, func(request) reply { return reply{result: map[string]any{}} })
	if err := s.client.FocusPane("w1:p2"); err != nil {
		t.Fatalf("FocusPane: %v", err)
	}
	<-s.done
	// pane.focus is the by-id method (PaneTarget in the bundled 0.9.5
	// schema, absent from the online method table) — distinct from
	// pane.focus_direction.
	if s.req.Method != "pane.focus" {
		t.Fatalf("method = %q", s.req.Method)
	}
	if params, _ := s.req.Params.(map[string]any); params["pane_id"] != "w1:p2" {
		t.Fatalf("params = %v, want pane_id w1:p2", s.req.Params)
	}
}

func TestReadPaneSendsRequestParams(t *testing.T) {
	s := newStub(t, func(request) reply {
		return reply{result: map[string]any{"read": map[string]any{"text": "$ hello"}}}
	})
	text, err := s.client.ReadPane("w1:p1", "recent_unwrapped", 12, true)
	<-s.done
	if err != nil {
		t.Fatalf("ReadPane: %v", err)
	}
	if text != "$ hello" {
		t.Fatalf("text = %q", text)
	}
	if s.req.Method != "pane.read" {
		t.Fatalf("method = %q", s.req.Method)
	}
	params, ok := s.req.Params.(map[string]any)
	if !ok {
		t.Fatalf("params = %T, want an object", s.req.Params)
	}
	for k, want := range map[string]any{
		"pane_id": "w1:p1",
		"source":  "recent_unwrapped",
		"format":  "ansi",
	} {
		if params[k] != want {
			t.Fatalf("param %s = %v, want %v", k, params[k], want)
		}
	}
	if n, ok := params["lines"].(float64); !ok || n != 12 {
		t.Fatalf("lines = %v, want 12", params["lines"])
	}
}
