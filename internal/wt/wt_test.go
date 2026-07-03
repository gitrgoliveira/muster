package wt_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/gitrgoliveira/muster/internal/wt"
)

// ── VCS.Valid ─────────────────────────────────────────────────────────────

func TestVCS_Valid(t *testing.T) {
	cases := []struct {
		vcs  wt.VCS
		want bool
	}{
		{wt.VCSGit, true},
		{wt.VCSJJ, true},
		{"", false},
		{"svn", false},
		{"Git", false},
		{"GIT", false},
	}
	for _, tc := range cases {
		if got := tc.vcs.Valid(); got != tc.want {
			t.Errorf("VCS(%q).Valid() = %v, want %v", tc.vcs, got, tc.want)
		}
	}
}

// ── ChangeKind zero-value ─────────────────────────────────────────────────

func TestChangeKind_Values(t *testing.T) {
	// Enumerate all exported constants to ensure they are distinct.
	kinds := []wt.ChangeKind{wt.Added, wt.Modified, wt.Deleted, wt.Renamed, wt.Copied}
	seen := make(map[wt.ChangeKind]bool)
	for _, k := range kinds {
		if seen[k] {
			t.Errorf("duplicate ChangeKind value %q", k)
		}
		seen[k] = true
		if string(k) == "" {
			t.Errorf("ChangeKind has empty string value")
		}
	}
}

// ── FileChange zero-value ─────────────────────────────────────────────────

func TestFileChange_ZeroValue(t *testing.T) {
	var fc wt.FileChange
	if fc.Path != "" || fc.OldPath != "" || fc.Kind != "" {
		t.Errorf("unexpected non-zero FileChange zero value: %+v", fc)
	}
}

// ── WorktreeStatus zero-value ──────────────────────────────────────────────

func TestWorktreeStatus_ZeroValue(t *testing.T) {
	var s wt.WorktreeStatus
	if s.Exists || s.Clean || s.Ahead != 0 || s.Behind != 0 {
		t.Errorf("unexpected WorktreeStatus zero value: %+v", s)
	}
}

// ── Sentinel errors ────────────────────────────────────────────────────────

func TestSentinelErrors_Distinct(t *testing.T) {
	errs := []error{
		wt.ErrNotImplemented,
		wt.ErrWorktreeNotFound,
		wt.ErrVCSUnavailable,
	}
	for i, a := range errs {
		for j, b := range errs {
			if i != j && errors.Is(a, b) {
				t.Errorf("sentinel[%d] equals sentinel[%d]", i, j)
			}
		}
	}
}

// ── For ───────────────────────────────────────────────────────────────────

func TestFor_Git(t *testing.T) {
	b, err := wt.For(wt.VCSGit)
	if err != nil {
		t.Fatalf("For(git): unexpected error: %v", err)
	}
	if b == nil {
		t.Fatal("For(git): nil backend")
	}
}

func TestFor_JJ(t *testing.T) {
	b, err := wt.For(wt.VCSJJ)
	if err != nil {
		t.Fatalf("For(jj): unexpected error: %v", err)
	}
	if b == nil {
		t.Fatal("For(jj): nil backend")
	}
}

func TestFor_Unknown(t *testing.T) {
	_, err := wt.For("svn")
	if err == nil {
		t.Fatal("For(svn): expected error, got nil")
	}
}

// ── Detect ────────────────────────────────────────────────────────────────

func TestDetect_Git(t *testing.T) {
	// git is expected to be available in all CI/dev environments.
	a := wt.Detect(context.Background())
	if !a.Git {
		t.Error("Detect: expected Git=true (git should be on PATH)")
	}
}

func TestDetect_FakeJJ(t *testing.T) {
	// Point PATH at the fake jj to confirm Detect picks it up.
	fakeJJ := filepath.Join("..", "..", "internal", "wt", "testdata", "fake_jj.sh")
	abs, err := filepath.Abs(fakeJJ)
	if err != nil {
		t.Fatalf("abs fake_jj: %v", err)
	}
	if _, err := os.Stat(abs); err != nil {
		t.Skipf("fake_jj.sh not found: %v", err)
	}
	// Create a temp bin dir with "jj" symlinked to fake_jj.sh.
	binDir := t.TempDir()
	dest := filepath.Join(binDir, "jj")
	if err := os.Symlink(abs, dest); err != nil {
		t.Fatalf("symlink: %v", err)
	}
	oldPath := os.Getenv("PATH")
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+oldPath)

	a := wt.Detect(context.Background())
	if !a.JJ {
		t.Error("Detect: expected JJ=true with fake jj on PATH")
	}
	if a.JJVer == "" {
		t.Error("Detect: expected JJVer non-empty with fake jj on PATH")
	}
}

// ── SafeRelPath ────────────────────────────────────────────────────────────

func TestSafeRelPath_Exported(t *testing.T) {
	t.Run("empty path is allowed", func(t *testing.T) {
		got, err := wt.SafeRelPath("")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != "" {
			t.Errorf("want empty, got %q", got)
		}
	})
	t.Run("simple file allowed", func(t *testing.T) {
		got, err := wt.SafeRelPath("foo.go")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != "foo.go" {
			t.Errorf("want foo.go, got %q", got)
		}
	})
	t.Run("absolute path rejected", func(t *testing.T) {
		_, err := wt.SafeRelPath("/etc/passwd")
		if err == nil {
			t.Fatal("expected error for absolute path, got nil")
		}
	})
	t.Run("dotdot rejected", func(t *testing.T) {
		_, err := wt.SafeRelPath("../secret")
		if err == nil {
			t.Fatal("expected error for dotdot path, got nil")
		}
	})
}

func TestDetect_NoJJ(t *testing.T) {
	// If jj is not on PATH at all, JJ should be false.
	t.Setenv("PATH", "/nonexistent")
	a := wt.Detect(context.Background())
	if a.Git {
		t.Error("Detect: expected Git=false with empty PATH")
	}
	if a.JJ {
		t.Error("Detect: expected JJ=false with empty PATH")
	}
}
