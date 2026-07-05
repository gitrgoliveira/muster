package wt_test

import (
	"errors"
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

// TestResolveRemote verifies the remote-name resolution and validation logic.
// Empty → ("origin", nil); valid names pass through; invalid names return ErrInvalidRemote.
func TestResolveRemote(t *testing.T) {
	cases := []struct {
		name    string
		remote  string
		want    string
		wantErr bool
	}{
		// Valid inputs.
		{"empty uses origin", "", "origin", false},
		{"explicit remote", "upstream", "upstream", false},
		{"explicit origin", "origin", "origin", false},
		{"custom remote name", "my-fork", "my-fork", false},
		{"alphanumeric", "my-remote", "my-remote", false},
		{"dots and dashes", "r2.d2", "r2.d2", false},
		// Invalid inputs — leading '-' is an option, not a name.
		{"leading dash long", "--force", "", true},
		{"leading dash short", "-f", "", true},
		{"receive-pack injection", "--receive-pack=/bin/sh", "", true},
		{"space in name", "a b", "", true},
		// Non-ASCII characters are rejected.
		{"non-ascii", "über", "", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := wt.ResolveRemote(tc.remote)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("ResolveRemote(%q): want error, got nil (resolved to %q)", tc.remote, got)
				}
				if !errors.Is(err, wt.ErrInvalidRemote) {
					t.Errorf("ResolveRemote(%q): error want ErrInvalidRemote, got %v", tc.remote, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("ResolveRemote(%q): unexpected error: %v", tc.remote, err)
			}
			if got != tc.want {
				t.Errorf("ResolveRemote(%q) = %q, want %q", tc.remote, got, tc.want)
			}
		})
	}
}
