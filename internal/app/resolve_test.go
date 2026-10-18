package app

import (
	"testing"

	"github.com/MykolaSuprun/herdr-preview-grid/internal/herdr"
)

func TestResolvePreviewPane(t *testing.T) {
	ws := herdr.Workspace{WorkspaceID: "w1", ActiveTabID: "w1:t1"}
	tests := []struct {
		name   string
		panes  []herdr.Pane
		self   string
		launch string
		want   string
	}{
		{
			name: "focused pane wins",
			panes: []herdr.Pane{
				{PaneID: "w1:p1", WorkspaceID: "w1", TabID: "w1:t1", Focused: true},
				{PaneID: "w1:p2", WorkspaceID: "w1", TabID: "w1:t1"},
			},
			want: "w1:p1",
		},
		{
			name: "own focused pane skipped, launch pane used",
			panes: []herdr.Pane{
				{PaneID: "w1:p1", WorkspaceID: "w1", TabID: "w1:t1"},
				{PaneID: "w1:p2", WorkspaceID: "w1", TabID: "w1:t1", Focused: true},
			},
			self:   "w1:p2",
			launch: "w1:p1",
			want:   "w1:p1",
		},
		{
			name: "own focused pane skipped, no launch context, other pane used",
			panes: []herdr.Pane{
				{PaneID: "w1:p1", WorkspaceID: "w1", TabID: "w1:t1"},
				{PaneID: "w1:p2", WorkspaceID: "w1", TabID: "w1:t1", Focused: true},
			},
			self: "w1:p2",
			want: "w1:p1",
		},
		{
			name: "launch pane in another tab ignored",
			panes: []herdr.Pane{
				{PaneID: "w1:p1", WorkspaceID: "w1", TabID: "w1:t1"},
				{PaneID: "w1:p2", WorkspaceID: "w1", TabID: "w1:t1", Focused: true},
				{PaneID: "w1:p3", WorkspaceID: "w1", TabID: "w1:t2"},
			},
			self:   "w1:p2",
			launch: "w1:p3",
			want:   "w1:p1",
		},
		{
			name: "launch context equal to self ignored",
			panes: []herdr.Pane{
				{PaneID: "w1:p1", WorkspaceID: "w1", TabID: "w1:t1"},
				{PaneID: "w1:p2", WorkspaceID: "w1", TabID: "w1:t1", Focused: true},
			},
			self:   "w1:p2",
			launch: "w1:p2",
			want:   "w1:p1",
		},
		{
			name: "only own pane in tab yields empty",
			panes: []herdr.Pane{
				{PaneID: "w1:p2", WorkspaceID: "w1", TabID: "w1:t1", Focused: true},
				{PaneID: "w2:p1", WorkspaceID: "w2", TabID: "w2:t1", Focused: true},
			},
			self: "w1:p2",
			want: "",
		},
		{
			name: "no focused pane yields first pane in tab",
			panes: []herdr.Pane{
				{PaneID: "w1:p1", WorkspaceID: "w1", TabID: "w1:t1"},
				{PaneID: "w1:p2", WorkspaceID: "w1", TabID: "w1:t1"},
			},
			want: "w1:p1",
		},
		{
			name: "other workspaces ignored",
			panes: []herdr.Pane{
				{PaneID: "w2:p1", WorkspaceID: "w2", TabID: "w2:t1", Focused: true},
			},
			want: "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := resolvePreviewPane(tt.panes, ws, tt.self, tt.launch); got != tt.want {
				t.Fatalf("resolvePreviewPane() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestLaunchFocusedPane(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want string
	}{
		{"valid context", `{"workspace_id":"w9","focused_pane_id":"w9:p1","invocation_source":"api"}`, "w9:p1"},
		{"empty", "", ""},
		{"malformed", "not json", ""},
		{"missing field", `{"workspace_id":"w9"}`, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := LaunchFocusedPane(tt.raw); got != tt.want {
				t.Fatalf("LaunchFocusedPane() = %q, want %q", got, tt.want)
			}
		})
	}
}
