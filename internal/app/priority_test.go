package app

import (
	"testing"
)

func TestParseEventKey(t *testing.T) {
	cases := []struct {
		raw  string
		key  string
		ok   bool
		fail bool // malformed payload
	}{
		{`{"type":"workspace_focused","workspace_id":"wA"}`, "workspace:wA", true, false},
		{`{"type":"pane_focused","pane_id":"wA:p2T","workspace_id":"wA"}`, "agent:wA:p2T", true, false},
		{`{"type":"pane_closed","pane_id":"wA:p2T"}`, "", false, false},
		{`{"type":"workspace_focused"}`, "", false, false},                   // no id
		{`{"type":"pane_focused"}`, "", false, false},                        // no id
		{`{not json`, "", false, true},                                       // malformed
		{`{"type":"workspace_focused","workspace_id":""}`, "", false, false}, // empty id
	}
	for _, c := range cases {
		key, ok := ParseEventKey(c.raw)
		if c.fail && ok {
			t.Errorf("parseEventKey(%q) ok=true, want failure", c.raw)
			continue
		}
		if !c.fail && ok != c.ok {
			t.Errorf("parseEventKey(%q) ok = %v, want %v", c.raw, ok, c.ok)
			continue
		}
		if key != c.key {
			t.Errorf("parseEventKey(%q) key = %q, want %q", c.raw, key, c.key)
		}
	}
}

func TestPartitionAwaiting(t *testing.T) {
	items := []Item{
		{Key: "a", AwaitingInput: false},
		{Key: "b", AwaitingInput: true},
		{Key: "c", AwaitingInput: false},
		{Key: "d", AwaitingInput: true},
	}
	got := partitionAwaiting(items)
	want := []string{"b", "d", "a", "c"}
	for i, key := range want {
		if got[i].Key != key {
			t.Fatalf("got %v at %d, want %q", got[i].Key, i, key)
		}
	}
	// All-awaiting and none-awaiting stay stable.
	all := []Item{{Key: "x", AwaitingInput: true}, {Key: "y", AwaitingInput: true}}
	got = partitionAwaiting(all)
	if got[0].Key != "x" || got[1].Key != "y" {
		t.Errorf("all-awaiting order changed: %v", got)
	}
}

func TestIsReadyForInput(t *testing.T) {
	// Semantics v2 (user decision 2026-10-10): attention float = blocked +
	// done ONLY; idle is majority-state and stays in the MRU stream.
	for _, s := range []string{"blocked", "done", "Blocked", " done ", "DONE"} {
		if !isReadyForInput(s) {
			t.Errorf("isReadyForInput(%q) = false", s)
		}
	}
	for _, s := range []string{"", "idle", "working", "unknown", "Idle"} {
		if isReadyForInput(s) {
			t.Errorf("isReadyForInput(%q) = true", s)
		}
	}
}
