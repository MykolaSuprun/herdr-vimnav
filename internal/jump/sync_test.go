package jump

import (
	"errors"
	"testing"

	"github.com/MykolaSuprun/herdr-preview-grid/internal/herdr"
)

// stubAPI feeds a fixed layout and fails the first n metadata reports.
type stubAPI struct {
	workspaces []herdr.Workspace
	agents     []herdr.Agent
	fails      int
	accepted   []string
}

func (s *stubAPI) Workspaces() ([]herdr.Workspace, error) { return s.workspaces, nil }
func (s *stubAPI) Agents() ([]herdr.Agent, error)         { return s.agents, nil }

func (s *stubAPI) report(id string, tokens map[string]string) error {
	if s.fails > 0 {
		s.fails--
		return errors.New("report failed")
	}
	s.accepted = append(s.accepted, id+"="+tokens["jump"])
	return nil
}

func (s *stubAPI) ReportWorkspaceMetadata(workspaceID, _ string, tokens map[string]string) error {
	return s.report(workspaceID, tokens)
}

func (s *stubAPI) ReportPaneMetadata(paneID, _ string, tokens map[string]string) error {
	return s.report(paneID, tokens)
}

// Regression for the review finding: Sync used to record a code in the state
// even when its report failed, so the diff-based pass never retried and the
// sidebar badge stayed missing until that item's code happened to change.
func TestSyncRetriesFailedReports(t *testing.T) {
	layout := stubAPI{
		workspaces: []herdr.Workspace{
			{WorkspaceID: "w1"}, {WorkspaceID: "w2"}, {WorkspaceID: "w3"},
			{WorkspaceID: "w4"}, {WorkspaceID: "w5"}, {WorkspaceID: "w6"},
		},
		agents: []herdr.Agent{{PaneID: "p1"}, {PaneID: "p2"}},
	}
	const charset = "jklfds" // 6 singles: w1=j w2=k w3=l w4=f w5=d w6=s, then agents

	st := &State{Spaces: map[string]string{}, Agents: map[string]string{}}

	// Pass 1: the first two workspace reports fail (Sync walks spaces before
	// agents); the rest succeed. The failed codes must NOT enter the state.
	first := layout
	first.fails = 2
	if _, err := Sync(&first, st, charset); err == nil {
		t.Fatal("expected the first report error to be surfaced")
	}
	if st.Spaces["w1"] != "" || st.Spaces["w2"] != "" {
		t.Fatalf("failed reports were committed to state: %+v", st.Spaces)
	}
	if st.Spaces["w3"] != "l" {
		t.Fatalf("successful reports missing from state: %+v", st.Spaces)
	}
	if st.Agents["p1"] == "" || st.Agents["p2"] == "" {
		t.Fatalf("agent reports missing from state: %+v", st.Agents)
	}

	// Pass 2: every report succeeds. Only the two failed workspaces are
	// re-reported (diff-based); already-known codes are skipped.
	second := layout
	if _, err := Sync(&second, st, charset); err != nil {
		t.Fatalf("pass 2 failed: %v", err)
	}
	if want := []string{"w1=j", "w2=k"}; len(second.accepted) != len(want) ||
		second.accepted[0] != want[0] || second.accepted[1] != want[1] {
		t.Fatalf("pass 2 re-reports = %v, want %v", second.accepted, want)
	}

	// Final state must carry the full canonical assignment.
	spaces, agents := layout.workspaces, layout.agents
	for _, e := range Entries(spaces, agents, charset) {
		var got string
		if e.Kind == KindWorkspace {
			got = st.Spaces[e.ID]
		} else {
			got = st.Agents[e.ID]
		}
		if got != e.Code {
			t.Fatalf("state %s = %q, want %q", e.ID, got, e.Code)
		}
	}
}
