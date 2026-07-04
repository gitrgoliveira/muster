package config_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gitrgoliveira/muster/internal/config"
)

func TestParseRepoFlag(t *testing.T) {
	t.Run("valid prefix=path", func(t *testing.T) {
		m := config.RepoMap{}
		// Use t.TempDir() so the test is portable: ParseRepoFlag stores
		// filepath.Abs(path), and on Windows Abs("/tmp/myrepo") resolves to a
		// volume-qualified path — comparing against a hard-coded Unix string
		// would fail there. Compare against Abs of the same input instead.
		repoDir := t.TempDir()
		err := config.ParseRepoFlag(m, "mp="+repoDir)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		wantAbs, err := filepath.Abs(repoDir)
		if err != nil {
			t.Fatalf("filepath.Abs(%q): %v", repoDir, err)
		}
		if m["mp"] != wantAbs {
			t.Errorf("want %q got %q", wantAbs, m["mp"])
		}
	})

	t.Run("missing equals returns error", func(t *testing.T) {
		m := config.RepoMap{}
		err := config.ParseRepoFlag(m, "noequalssign")
		if err == nil {
			t.Fatal("want error, got nil")
		}
		if !strings.Contains(err.Error(), "prefix=path") {
			t.Errorf("error %q should mention prefix=path", err.Error())
		}
	})

	t.Run("empty prefix returns error", func(t *testing.T) {
		m := config.RepoMap{}
		err := config.ParseRepoFlag(m, "=/tmp/path")
		if err == nil {
			t.Fatal("want error for empty prefix")
		}
	})

	t.Run("empty path returns error", func(t *testing.T) {
		m := config.RepoMap{}
		err := config.ParseRepoFlag(m, "mp=")
		if err == nil {
			t.Fatal("want error for empty path")
		}
	})

	t.Run("leading tilde is expanded to home", func(t *testing.T) {
		home, err := os.UserHomeDir()
		if err != nil {
			t.Skipf("no home dir: %v", err)
		}
		// A shell never expands the ~ in `--repo mp=~/x` (not at word start),
		// and MUSTER_REPO never goes through a shell, so ParseRepoFlag must do
		// it — otherwise the path resolves relative to the cwd.
		m := config.RepoMap{}
		if err := config.ParseRepoFlag(m, "mp=~/repos/foo"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		want := filepath.Join(home, "repos", "foo")
		if m["mp"] != want {
			t.Errorf("want %q got %q", want, m["mp"])
		}
		// A bare "~" expands to the home dir itself.
		m2 := config.RepoMap{}
		if err := config.ParseRepoFlag(m2, "mp=~"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if m2["mp"] != home {
			t.Errorf("bare ~: want %q got %q", home, m2["mp"])
		}
		// A ~ NOT at the start (e.g. a real dir literally named "~x") is left
		// alone — only a leading ~/~ / is treated as home.
		m3 := config.RepoMap{}
		if err := config.ParseRepoFlag(m3, "mp=/tmp/~nothome"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if strings.Contains(m3["mp"], home) && home != "/" {
			t.Errorf("non-leading ~ should not expand: got %q", m3["mp"])
		}
	})
}

func TestParseRepoEnv(t *testing.T) {
	t.Run("empty env is no-op", func(t *testing.T) {
		m := config.RepoMap{}
		if err := config.ParseRepoEnv(m, ""); err != nil {
			t.Fatal(err)
		}
		if len(m) != 0 {
			t.Errorf("want empty map, got %v", m)
		}
	})

	t.Run("single entry", func(t *testing.T) {
		m := config.RepoMap{}
		if err := config.ParseRepoEnv(m, "mp=/tmp/repo1"); err != nil {
			t.Fatal(err)
		}
		if len(m) != 1 {
			t.Errorf("want 1 entry, got %d", len(m))
		}
	})

	t.Run("multiple comma-separated entries", func(t *testing.T) {
		m := config.RepoMap{}
		if err := config.ParseRepoEnv(m, "mp=/tmp/repo1,bd=/tmp/repo2"); err != nil {
			t.Fatal(err)
		}
		if len(m) != 2 {
			t.Errorf("want 2 entries, got %d: %v", len(m), m)
		}
	})

	t.Run("invalid entry returns error", func(t *testing.T) {
		m := config.RepoMap{}
		err := config.ParseRepoEnv(m, "mp=/tmp/ok,badsyntax")
		if err == nil {
			t.Fatal("want error for bad entry")
		}
		if !strings.Contains(err.Error(), "MUSTER_REPO") {
			t.Errorf("error %q should mention MUSTER_REPO", err.Error())
		}
	})
}

func TestDefaultWorktreesDir(t *testing.T) {
	dir := config.DefaultWorktreesDir()
	if dir == "" {
		t.Error("DefaultWorktreesDir should not be empty")
	}
	// Should contain "muster" and "worktrees"
	if !strings.Contains(dir, "muster") {
		t.Errorf("DefaultWorktreesDir %q should contain 'muster'", dir)
	}
}

// ── T032: --default-vcs allow-list (FR-018) ──────────────────────────────

func TestParseDefaultVCS(t *testing.T) {
	t.Run("git is valid", func(t *testing.T) {
		v, err := config.ParseDefaultVCS("git")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if v != "git" {
			t.Errorf("want git got %q", v)
		}
	})

	t.Run("jj is valid", func(t *testing.T) {
		v, err := config.ParseDefaultVCS("jj")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if v != "jj" {
			t.Errorf("want jj got %q", v)
		}
	})

	t.Run("empty string defaults to git", func(t *testing.T) {
		v, err := config.ParseDefaultVCS("")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if v != "git" {
			t.Errorf("empty string: want git got %q", v)
		}
	})

	t.Run("invalid value returns error", func(t *testing.T) {
		_, err := config.ParseDefaultVCS("svn")
		if err == nil {
			t.Fatal("want error for 'svn'")
		}
		if !strings.Contains(err.Error(), "git") || !strings.Contains(err.Error(), "jj") {
			t.Errorf("error %q should mention allowed values git and jj", err.Error())
		}
	})

	t.Run("GIT (uppercase) is invalid", func(t *testing.T) {
		_, err := config.ParseDefaultVCS("GIT")
		if err == nil {
			t.Fatal("want error for 'GIT' (case-sensitive allow-list)")
		}
	})

	t.Run("mixed case invalid", func(t *testing.T) {
		_, err := config.ParseDefaultVCS("Git")
		if err == nil {
			t.Fatal("want error for 'Git'")
		}
	})
}
