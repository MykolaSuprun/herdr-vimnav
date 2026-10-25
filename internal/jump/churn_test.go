package jump

import (
	"fmt"
	"testing"

	"github.com/MykolaSuprun/herdr-preview-grid/internal/herdr"
)

// Churn: codes are positional, so add/remove has to behave predictably in
// every direction — appends leave existing codes alone, a removal frees that
// slot and the tail re-flows, and the pair→triple boundary at 90 behaves
// symmetrically when crossed in either direction.

func wsN(n int) []herdr.Workspace {
	out := make([]herdr.Workspace, n)
	for i := range out {
		out[i] = herdr.Workspace{WorkspaceID: fmt.Sprintf("w%d", i)}
	}
	return out
}

func codesByID(entries []Entry) map[string]string {
	m := make(map[string]string, len(entries))
	for _, e := range entries {
		m[e.ID] = e.Code
	}
	return m
}

func TestChurnAppendKeepsExistingCodes(t *testing.T) {
	before := codesByID(Entries(wsN(5), nil, "sdfjklerui"))
	after := codesByID(Entries(wsN(6), nil, "sdfjklerui"))

	for id, code := range before {
		if after[id] != code {
			t.Fatalf("append renumbered %s: %q -> %q", id, code, after[id])
		}
	}
	if after["w5"] != "se" {
		t.Fatalf("new item code = %q, want se (next free pair)", after["w5"])
	}
	if before["w5"] != "" {
		t.Fatal("pre-append map already had w5")
	}
}

func TestChurnRemoveLastLeavesOthersUntouched(t *testing.T) {
	before := codesByID(Entries(wsN(6), nil, "sdfjklerui"))
	after := codesByID(Entries(wsN(5), nil, "sdfjklerui"))

	for id, code := range after {
		if before[id] != code {
			t.Fatalf("removing the tail changed %s: %q -> %q", id, before[id], code)
		}
	}
	if _, ok := after["w4"]; !ok {
		t.Fatal("w4 vanished")
	}
}

// Removing a middle item frees its code and the tail re-flows one slot:
// this is the documented positional behavior. What must NOT happen is a
// duplicate or a hole before the tail.
func TestChurnRemoveMiddleReflowsTail(t *testing.T) {
	spaces := wsN(6)
	after := codesByID(Entries(append(append([]herdr.Workspace{}, spaces[0:2]...), spaces[3:]...), nil, "sdfjklerui"))

	if _, gone := after["w2"]; gone {
		t.Fatal("removed workspace still has a code")
	}
	// w3 takes w2's freed slot, and everything after shifts by one.
	want := map[string]string{"w0": "sd", "w1": "sf", "w3": "sj", "w4": "sk", "w5": "sl"}
	for id, code := range want {
		if after[id] != code {
			t.Fatalf("%s = %q, want %q (freed slot reused, tail re-flowed)", id, after[id], code)
		}
	}
	seen := map[string]bool{}
	for _, code := range after {
		if seen[code] {
			t.Fatalf("duplicate code %q after middle removal", code)
		}
		seen[code] = true
	}
}

// The 90/91 boundary: appending the 91st target must not disturb the 90
// pairs already assigned; removing it must hand the pair back.
func TestChurnPairToTripleBoundary(t *testing.T) {
	at90 := Entries(wsN(90), nil, "sdfjklerui")
	if len(at90) != 90 {
		t.Fatalf("got %d entries, want 90", len(at90))
	}
	if at90[89].Code != "iu" {
		t.Fatalf("90th pair = %q, want iu", at90[89].Code)
	}

	at91 := Entries(wsN(91), nil, "sdfjklerui")
	for i := 0; i < 90; i++ {
		if at91[i].Code != at90[i].Code {
			t.Fatalf("crossing into triples renumbered item %d: %q -> %q", i, at90[i].Code, at91[i].Code)
		}
	}
	if at91[90].Code != "ssd" {
		t.Fatalf("91st code = %q, want ssd (first triple)", at91[90].Code)
	}

	// Remove it again: the pair tier is restored exactly.
	back := Entries(wsN(90), nil, "sdfjklerui")
	if back[89].Code != "iu" {
		t.Fatalf("after removing the 91st, 90th = %q, want iu", back[89].Code)
	}
}

func TestChurnTripleTierKeepsGrowing(t *testing.T) {
	// 92 targets: pairs, then ssd, ssf — appended in order, no renumbering.
	entries := Entries(wsN(92), nil, "sdfjklerui")
	if got := entries[89].Code; got != "iu" {
		t.Fatalf("last pair = %q", got)
	}
	if got := entries[90].Code; got != "ssd" {
		t.Fatalf("first triple = %q, want ssd", got)
	}
	if got := entries[91].Code; got != "ssf" {
		t.Fatalf("second triple = %q, want ssf", got)
	}
}

// Cross-kind churn: spaces and agents share the badge tier; tabs and panes
// continue it. Adding an AGENT must not move space codes but does shift the
// tabs/panes that follow; removing a TAB only re-flows later panes.
func TestChurnAcrossKinds(t *testing.T) {
	spaces := wsN(3)
	agents := []herdr.Agent{{PaneID: "p1"}}
	tabs := []herdr.Tab{
		{TabID: "t1", WorkspaceID: "w0"},
		{TabID: "t2", WorkspaceID: "w0"},
	}
	panes := []herdr.Pane{
		{PaneID: "pa", WorkspaceID: "w0", TabID: "t1"},
		{PaneID: "pb", WorkspaceID: "w0", TabID: "t2"},
	}
	base := EntriesFor(spaces, agents, tabs, panes, "sdfjklerui")
	baseByID := codesByID(base)
	// 3 spaces + 1 agent = 4 pairs, then tabs, then panes.
	if baseByID["w0"] != "sd" || baseByID["p1"] != "sk" ||
		baseByID["t1"] != "sl" || baseByID["t2"] != "se" ||
		baseByID["pa"] != "sr" || baseByID["pb"] != "su" {
		t.Fatalf("base assignment = %+v", baseByID)
	}

	// Add an agent: space codes stay, agent slots and everything after shift.
	withAgent := codesByID(EntriesFor(spaces, append(agents, herdr.Agent{PaneID: "p2"}), tabs, panes, "sdfjklerui"))
	for _, id := range []string{"w0", "w1", "w2"} {
		if withAgent[id] != baseByID[id] {
			t.Fatalf("adding an agent moved space %s: %q -> %q", id, baseByID[id], withAgent[id])
		}
	}
	if withAgent["p1"] != "sk" || withAgent["p2"] != "sl" {
		t.Fatalf("agent codes after append = p1:%q p2:%q, want sk sl", withAgent["p1"], withAgent["p2"])
	}
	if withAgent["t1"] != "se" {
		t.Fatalf("tab should re-flow into the next slot: t1 = %q, want se", withAgent["t1"])
	}

	// Removing a TAB: earlier kinds are untouched, and only the pane whose
	// tab disappeared re-flows to the end. pa lives in the surviving t1, so
	// it takes the freed slot; pb's tab is gone from the snapshot, so it
	// sorts last.
	oneTab := codesByID(EntriesFor(spaces, agents, tabs[:1], panes, "sdfjklerui"))
	if oneTab["w0"] != "sd" || oneTab["p1"] != "sk" {
		t.Fatalf("removing a tab disturbed the badge tier: %+v", oneTab)
	}
	if oneTab["t2"] != "" {
		t.Fatal("removed tab still coded")
	}
	if oneTab["pa"] != "se" || oneTab["pb"] != "sr" {
		t.Fatalf("panes after tab removal = pa:%q pb:%q, want pa:se pb:sr", oneTab["pa"], oneTab["pb"])
	}
}

// The mirror image: removing the FIRST item frees the very first code, and
// every other target shifts up by one — no holes, no duplicates.
func TestChurnRemoveFirst(t *testing.T) {
	all := wsN(4)
	after := codesByID(Entries(all[1:], nil, "sdfjklerui"))
	want := map[string]string{"w1": "sd", "w2": "sf", "w3": "sj"}
	for id, code := range want {
		if after[id] != code {
			t.Fatalf("%s = %q, want %q", id, after[id], code)
		}
	}
}

// Sync must forget removed ids: the state file is a mirror of the live
// layout, so a closed space must not keep a stale code forever.
func TestChurnSyncDropsRemovedAndReassigns(t *testing.T) {
	layout := stubAPI{workspaces: wsN(4)}
	st := &State{Spaces: map[string]string{}, Agents: map[string]string{}}
	if _, err := Sync(&layout, st, "sdfjklerui"); err != nil {
		t.Fatalf("initial sync: %v", err)
	}
	if st.Spaces["w3"] != "sk" {
		t.Fatalf("initial w3 = %q", st.Spaces["w3"])
	}

	// Close w1: its state entry must disappear and the tail must re-flow.
	layout.workspaces = append(append([]herdr.Workspace{}, wsN(4)[0:1]...), wsN(4)[2:]...)
	layout.accepted = nil
	if _, err := Sync(&layout, st, "sdfjklerui"); err != nil {
		t.Fatalf("sync after removal: %v", err)
	}
	if _, stale := st.Spaces["w1"]; stale {
		t.Fatalf("removed workspace kept its code: %+v", st.Spaces)
	}
	want := map[string]string{"w0": "sd", "w2": "sf", "w3": "sj"}
	for id, code := range want {
		if st.Spaces[id] != code {
			t.Fatalf("state %s = %q, want %q", id, st.Spaces[id], code)
		}
	}
	// The re-flowed codes are what got re-reported to the sidebar.
	reported := map[string]bool{}
	for _, r := range layout.accepted {
		reported[r] = true
	}
	for _, w := range want {
		_ = w
	}
	for _, id := range []string{"w0", "w2", "w3"} {
		if !reported[id+"="+want[id]] {
			t.Fatalf("sidebar not re-reported %s=%s (got %v)", id, want[id], layout.accepted)
		}
	}
}
