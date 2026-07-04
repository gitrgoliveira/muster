package wt_test

import (
	"testing"

	"github.com/gitrgoliveira/muster/internal/wt"
)

// TestBranchName verifies the branch name convention used by the wt package.
// All per-bead branches follow "muster/<beadID>" per the M3 convention
// (internal/worktree branchName function).
func TestBranchName(t *testing.T) {
	cases := []struct {
		beadID string
		want   string
	}{
		{"abc-123", "muster/abc-123"},
		{"xyz", "muster/xyz"},
		{"my-bead-1", "muster/my-bead-1"},
		{"", "muster/"},
	}
	for _, tc := range cases {
		got := wt.BranchName(tc.beadID)
		if got != tc.want {
			t.Errorf("BranchName(%q) = %q, want %q", tc.beadID, got, tc.want)
		}
	}
}

// TestResolveRemote verifies the remote-name resolution logic.
// When the remote param is empty, defaults to "origin".
// When non-empty, uses the provided value as-is.
func TestResolveRemote(t *testing.T) {
	cases := []struct {
		name   string
		remote string // empty = use default
		want   string
	}{
		{"empty uses origin", "", "origin"},
		{"explicit remote", "upstream", "upstream"},
		{"explicit origin", "origin", "origin"},
		{"custom remote name", "my-fork", "my-fork"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := wt.ResolveRemote(tc.remote)
			if got != tc.want {
				t.Errorf("ResolveRemote(%q) = %q, want %q", tc.remote, got, tc.want)
			}
		})
	}
}
