package wt

import (
	"testing"
)

// TestSafeRelPath exercises the safeRelPath function.
// Tests call the unexported function directly (same package).
func TestSafeRelPath(t *testing.T) {
	const wt = "/tmp/worktree"

	cases := []struct {
		name    string
		path    string
		wantErr bool
		wantOut string
	}{
		// Valid cases
		{
			name:    "empty path allowed (whole worktree)",
			path:    "",
			wantErr: false,
			wantOut: "",
		},
		{
			name:    "simple filename",
			path:    "file.go",
			wantErr: false,
			wantOut: "file.go",
		},
		{
			name:    "nested path",
			path:    "internal/wt/git.go",
			wantErr: false,
			wantOut: "internal/wt/git.go",
		},
		{
			name:    "path with redundant dot",
			path:    "./file.go",
			wantErr: false,
			wantOut: "file.go",
		},

		// Rejection cases
		{
			name:    "absolute path rejected",
			path:    "/etc/passwd",
			wantErr: true,
		},
		{
			name:    "dotdot component rejected",
			path:    "../etc/passwd",
			wantErr: true,
		},
		{
			name:    "double dotdot traversal",
			path:    "../../etc/passwd",
			wantErr: true,
		},
		{
			name:    "dotdot hidden in nested path",
			path:    "foo/../../etc/passwd",
			wantErr: true,
		},
		{
			name:    "dotdot at start with depth",
			path:    "../sibling/secret",
			wantErr: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := safeRelPath(wt, tc.path)
			if tc.wantErr {
				if err == nil {
					t.Errorf("safeRelPath(%q): expected error, got %q", tc.path, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("safeRelPath(%q): unexpected error: %v", tc.path, err)
			}
			if got != tc.wantOut {
				t.Errorf("safeRelPath(%q) = %q, want %q", tc.path, got, tc.wantOut)
			}
		})
	}
}
