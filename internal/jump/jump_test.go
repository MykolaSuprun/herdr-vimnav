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
	entries := Entries(spaces, agents, "sdfjklerui")
	// 5 items: the first five pairs of the pool — spaces first, then agents.
	want := []struct {
		code, kind, id string
	}{
		{"sd", "workspace", "w1"},
		{"sf", "workspace", "w2"},
		{"sj", "agent", "p1"},
		{"sk", "agent", "p2"},
		{"sl", "agent", "p3"},
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

func TestEntriesNoSingleLetterCodes(t *testing.T) {
	// Every code is at least two letters, so the first keystroke is always
	// a live prefix and never fires on its own.
	spaces := make([]herdr.Workspace, 12)
	for i := range spaces {
		spaces[i] = herdr.Workspace{WorkspaceID: "w"}
	}
	for _, e := range Entries(spaces, nil, "sdfjklerui") {
		if len(e.Code) < 2 {
			t.Fatalf("single-letter code %q", e.Code)
		}
	}
}

func TestEntriesBeyondPairTier(t *testing.T) {
	// 90 pair codes, then doubled-first triples.
	spaces := make([]herdr.Workspace, 93)
	for i := range spaces {
		spaces[i] = herdr.Workspace{WorkspaceID: "w"}
	}
	entries := Entries(spaces, nil, "sdfjklerui")
	if len(entries) != 93 {
		t.Fatalf("got %d entries", len(entries))
	}
	if got := entries[89].Code; got != "iu" {
		t.Fatalf("last pair = %q, want iu (i-block ends with the pool's last letter)", got)
	}
	if got := entries[90].Code; got != "ssd" {
		t.Fatalf("first triple = %q, want ssd", got)
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

func TestEntriesForAppendsTabsAndPanes(t *testing.T) {
	spaces := []herdr.Workspace{
		{WorkspaceID: "w1", Label: "alpha"},
		{WorkspaceID: "w2"},
	}
	agents := []herdr.Agent{{PaneID: "p1", Agent: "oc"}}
	tabs := []herdr.Tab{
		{TabID: "w2:t2", WorkspaceID: "w2", Label: "logs"},
	}
	panes := []herdr.Pane{
		// Agent pane: excluded — already jumpable as its agent entry.
		{PaneID: "p4", WorkspaceID: "w1", TabID: "w1:t1", Agent: "claude"},
		{PaneID: "p5", WorkspaceID: "w2", TabID: "w2:t2", TerminalTitle: "tail -f plugin.log"},
		{PaneID: "p6", WorkspaceID: "w1", TabID: "w1:t1", Cwd: "/home/dev/workspaces/src/vimnav"},
	}
	// 2 spaces + 1 agent → the first three pairs sd sf sj; the tab follows
	// with sk; the panes take sl and se.
	entries := EntriesFor(spaces, agents, tabs, panes, "sdfjklerui")
	want := []struct {
		code, kind, id, label, ws, tab string
	}{
		{"sd", "workspace", "w1", "alpha", "", ""},
		{"sf", "workspace", "w2", "w2", "", ""},
		{"sj", "agent", "p1", "oc", "", ""},
		{"sk", "tab", "w2:t2", "w2 · logs", "w2", ""},
		{"sl", "pane", "p5", "w2 · logs · tail -f plugin.log", "w2", "w2:t2"},
		// p6's tab is absent from the tabs snapshot (layout churn), and its
		// label draws from the renamed space: compound labels skip missing
		// context levels.
		{"se", "pane", "p6", "alpha · vimnav", "w1", "w1:t1"},
	}
	if len(entries) != len(want) {
		t.Fatalf("got %d entries, want %d (%+v)", len(entries), len(want), entries)
	}
	for i, w := range want {
		e := entries[i]
		if e.Code != w.code || string(e.Kind) != w.kind || e.ID != w.id ||
			e.Label != w.label || e.WorkspaceID != w.ws || e.TabID != w.tab {
			t.Fatalf("entry %d = %+v, want %+v", i, e, w)
		}
	}
}

// The defining invariant of the extended table: badges never renumber when
// tabs and panes join. codes.Table's sequence is prefix-stable, so the
// leading rows of EntriesFor are byte-identical to the badge table Entries
// builds — a popup-visible code always matches its sidebar badge.
func TestEntriesForBadgePrefixStable(t *testing.T) {
	spaces := []herdr.Workspace{{WorkspaceID: "w1"}, {WorkspaceID: "w2"}, {WorkspaceID: "w3"}}
	agents := []herdr.Agent{
		{PaneID: "p1"}, {PaneID: "p2"}, {PaneID: "p3"},
		{PaneID: "p4"}, {PaneID: "p5"},
	}
	tabs := []herdr.Tab{{TabID: "t1", WorkspaceID: "w1"}, {TabID: "t2", WorkspaceID: "w1"}}
	panes := []herdr.Pane{{PaneID: "pa", WorkspaceID: "w1", TabID: "t1"}}

	badges := Entries(spaces, agents, "sdfjklerui")
	full := EntriesFor(spaces, agents, tabs, panes, "sdfjklerui")
	if len(full) <= len(badges) {
		t.Fatalf("full table should grow: %d vs %d", len(full), len(badges))
	}
	for i := range badges {
		if full[i] != badges[i] {
			t.Fatalf("entry %d renumbered: full=%+v badge=%+v", i, full[i], badges[i])
		}
	}
}

func TestEntriesForOrdersPanesByTab(t *testing.T) {
	tabs := []herdr.Tab{
		{TabID: "w1:t2", WorkspaceID: "w1"}, // second tab first: the tabs slice rules
		{TabID: "w1:t1", WorkspaceID: "w1"},
	}
	panes := []herdr.Pane{
		{PaneID: "pA", TabID: "w1:t1"},
		{PaneID: "pB", TabID: "w1:t2"},
		{PaneID: "pC", TabID: "w9:t9"}, // tab missing from the snapshot: sorts last
		{PaneID: "pD", TabID: "w1:t1"},
	}
	// The default pool: 2 tabs then 4 panes fit well inside the pair tier.
	entries := EntriesFor(nil, nil, tabs, panes, "sdfjklerui")
	got := make([]string, len(entries))
	for i := range entries {
		got[i] = entries[i].ID
	}
	// Tabs lead the extended table; the panes follow in tab order.
	want := []string{"w1:t2", "w1:t1", "pB", "pA", "pD", "pC"}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("pane order = %v, want %v", got, want)
		}
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
