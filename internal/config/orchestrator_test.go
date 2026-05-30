package config_test

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/gitrgoliveira/muster/internal/config"
)

func TestParseRepoFlag(t *testing.T) {
	t.Run("valid prefix=path", func(t *testing.T) {
		m := config.RepoMap{}
		// Use a real-path-like string; Abs will just canonicalize it.
		err := config.ParseRepoFlag(m, "mp=/tmp/myrepo")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if m["mp"] != filepath.Clean("/tmp/myrepo") {
			t.Errorf("want /tmp/myrepo got %q", m["mp"])
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
