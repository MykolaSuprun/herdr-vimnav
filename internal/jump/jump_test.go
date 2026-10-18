package jump

import (
	"testing"

	"github.com/MykolaSuprun/herdr-preview-grid/internal/herdr"
)

func TestEntriesOrderAndCodes(t *testing.T) {
	spaces := []herdr.Workspace{{WorkspaceID: "w1", Label: "one"}, {WorkspaceID: "w2"}}
	agents := []herdr.Agent{
		{PaneID: "p1", DisplayAgent: "oc"},
		{PaneID: "p2", Agent: "claude"},
		{PaneID: "p3", DisplayAgent: "z"},
	}
	entries := Entries(spaces, agents, "jklfds")
	// 5 items: singles j k l f d — spaces first, then agents.
	want := []struct {
		code, kind, id string
	}{
		{"j", "workspace", "w1"},
		{"k", "workspace", "w2"},
		{"l", "agent", "p1"},
		{"f", "agent", "p2"},
		{"d", "agent", "p3"},
	}
	if len(entries) != len(want) {
		t.Fatalf("got %d entries, want %d", len(entries), len(want))
	}
	for i, w := range want {
		e := entries[i]
		if e.Code != w.code || string(e.Kind) != w.kind || e.ID != w.id {
			t.Fatalf("entry %d = %+v, want code=%s kind=%s id=%s", i, e, w.code, w.kind, w.id)
		}
	}
	if entries[1].Label != "w2" { // label falls back to the id
		t.Fatalf("label fallback: got %q", entries[1].Label)
	}
	if entries[3].Label != "claude" { // display_agent empty → agent kind
		t.Fatalf("agent fallback: got %q", entries[3].Label)
	}
}

func TestEntriesPairTier(t *testing.T) {
	spaces := make([]herdr.Workspace, 7)
	for i := range spaces {
		spaces[i] = herdr.Workspace{WorkspaceID: "w"}
	}
	entries := Entries(spaces, nil, "jklfds")
	if entries[6].Code != "uj" {
		t.Fatalf("7th code: got %q want uj", entries[6].Code)
	}
}

func TestMatch(t *testing.T) {
	entries := []Entry{{Code: "j"}, {Code: "ui"}}
	if e, _ := Match("j", entries); e == nil || e.Code != "j" {
		t.Fatal("j should fire")
	}
	if e, live := Match("u", entries); e != nil || !live {
		t.Fatal("u should be a live prefix")
	}
	if e, live := Match("ux", entries); e != nil || live {
		t.Fatal("ux should be a dead end")
	}
	if e, live := Match("", entries); e != nil || !live {
		t.Fatal("empty buffer is live")
	}
}

func TestStateRoundTrip(t *testing.T) {
	dir := t.TempDir()
	st := LoadState(dir)
	st.Spaces["w1"] = "j"
	st.save()
	re := LoadState(dir)
	if re.Spaces["w1"] != "j" {
		t.Fatalf("round trip lost data: %+v", re)
	}
}
