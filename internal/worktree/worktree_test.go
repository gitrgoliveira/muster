package worktree

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEnsure_Create(t *testing.T) {
	repoPath := initGitRepo(t)
	worktreesDir := t.TempDir()

	wt, err := Ensure(context.Background(), worktreesDir, repoPath, "mp-abc")
	if err != nil {
		t.Fatalf("Ensure: %v", err)
	}

	if wt.BeadID != "mp-abc" {
		t.Errorf("BeadID want mp-abc got %q", wt.BeadID)
	}
	if wt.Branch != "muster/mp-abc" {
		t.Errorf("Branch want muster/mp-abc got %q", wt.Branch)
	}
	expectedPath := filepath.Join(worktreesDir, "mp-abc")
	if wt.Path != expectedPath {
		t.Errorf("Path want %q got %q", expectedPath, wt.Path)
	}
	if wt.RepoPath != repoPath {
		t.Errorf("RepoPath want %q got %q", repoPath, wt.RepoPath)
	}

	// Worktree directory must exist.
	if _, err := os.Stat(wt.Path); err != nil {
		t.Errorf("worktree path %q does not exist: %v", wt.Path, err)
	}

	// Branch should exist in the repo.
	output := runGitCmd(t, repoPath, "branch", "--list", "muster/mp-abc")
	if !strings.Contains(output, "muster/mp-abc") {
		t.Errorf("branch muster/mp-abc not found in repo; git branch output: %q", output)
	}
}

func TestEnsure_Reuse(t *testing.T) {
	repoPath := initGitRepo(t)
	worktreesDir := t.TempDir()

	// Create the worktree first.
	wt1, err := Ensure(context.Background(), worktreesDir, repoPath, "mp-abc")
	if err != nil {
		t.Fatalf("first Ensure: %v", err)
	}

	// Create a sentinel file in the worktree.
	sentinelPath := filepath.Join(wt1.Path, "sentinel.txt")
	if err := os.WriteFile(sentinelPath, []byte("hello"), 0644); err != nil {
		t.Fatalf("write sentinel: %v", err)
	}

	// Call Ensure again — should reuse the existing worktree.
	wt2, err := Ensure(context.Background(), worktreesDir, repoPath, "mp-abc")
	if err != nil {
		t.Fatalf("second Ensure: %v", err)
	}
	if wt2.Path != wt1.Path {
		t.Errorf("path want %q got %q (reuse failed)", wt1.Path, wt2.Path)
	}

	// Sentinel must still exist (worktree not recreated).
	if _, err := os.Stat(sentinelPath); err != nil {
		t.Errorf("sentinel file should still exist after reuse: %v", err)
	}
}

func TestEnsure_NonGitRepoError(t *testing.T) {
	notARepo := t.TempDir()
	worktreesDir := t.TempDir()

	_, err := Ensure(context.Background(), worktreesDir, notARepo, "mp-abc")
	if err == nil {
		t.Fatal("want error for non-git-repo, got nil")
	}
	if !strings.Contains(err.Error(), "not a git repo") && !strings.Contains(err.Error(), "git") {
		t.Errorf("error %q should mention git", err.Error())
	}
}

func TestEnsure_ManuallyDeletedWorktree(t *testing.T) {
	// If the branch exists but the worktree directory was manually deleted,
	// Ensure should recreate it on the existing branch.
	repoPath := initGitRepo(t)
	worktreesDir := t.TempDir()

	// First ensure creates the worktree.
	wt1, err := Ensure(context.Background(), worktreesDir, repoPath, "mp-regen")
	if err != nil {
		t.Fatalf("first Ensure: %v", err)
	}

	// Manually remove the worktree directory (simulating cleanup or crash).
	if err := os.RemoveAll(wt1.Path); err != nil {
		t.Fatalf("remove: %v", err)
	}
	// Remove git worktree tracking via prune.
	runGitCmd(t, repoPath, "worktree", "prune")

	// Second ensure should recreate the worktree on the existing branch.
	wt2, err := Ensure(context.Background(), worktreesDir, repoPath, "mp-regen")
	if err != nil {
		t.Fatalf("second Ensure after manual remove: %v", err)
	}
	if _, err := os.Stat(wt2.Path); err != nil {
		t.Errorf("worktree should exist after re-creation: %v", err)
	}
}

func TestEnsure_PathExistsButNotWorktree(t *testing.T) {
	// If the target path exists but is not a linked git worktree
	// (e.g. a plain directory or a stale checkout), Ensure must refuse
	// rather than fall through to `git worktree add` with a confusing error.
	repoPath := initGitRepo(t)
	worktreesDir := t.TempDir()

	// Pre-create a non-worktree directory at the path Ensure would use.
	collisionPath := filepath.Join(worktreesDir, "mp-collide")
	if err := os.MkdirAll(collisionPath, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	// Put a sentinel inside so we can check it wasn't touched.
	sentinel := filepath.Join(collisionPath, "important.txt")
	if err := os.WriteFile(sentinel, []byte("keep me"), 0644); err != nil {
		t.Fatalf("write sentinel: %v", err)
	}

	_, err := Ensure(context.Background(), worktreesDir, repoPath, "mp-collide")
	if err == nil {
		t.Fatal("want error for non-worktree pre-existing path, got nil")
	}
	if !strings.Contains(err.Error(), "not a git worktree") {
		t.Errorf("error %q should mention 'not a git worktree'", err.Error())
	}

	// Sentinel must still exist (Ensure refused to touch the directory).
	if _, err := os.Stat(sentinel); err != nil {
		t.Errorf("sentinel file should be untouched: %v", err)
	}
}

func TestEnsure_ReuseRejectsMismatchedRepo(t *testing.T) {
	// If worktreesDir is reused across a repo-mapping change, an existing
	// worktree at the same path may belong to a different repository. Ensure
	// must refuse to silently return it as if it were ours.
	repoA := initGitRepo(t)
	repoB := initGitRepo(t)
	worktreesDir := t.TempDir()

	// Create the worktree linked to repoA.
	if _, err := Ensure(context.Background(), worktreesDir, repoA, "mp-shared"); err != nil {
		t.Fatalf("first Ensure (repoA): %v", err)
	}

	// Now ask for the same beadID against repoB — should error.
	_, err := Ensure(context.Background(), worktreesDir, repoB, "mp-shared")
	if err == nil {
		t.Fatal("want error for repo-mismatched reuse, got nil")
	}
	if !strings.Contains(err.Error(), "linked to repo") || !strings.Contains(err.Error(), "refusing to reuse") {
		t.Errorf("error %q should describe a repo-mismatch reuse refusal", err.Error())
	}
}

func TestEnsure_TwoBeadIsolation(t *testing.T) {
	repoPath := initGitRepo(t)
	worktreesDir := t.TempDir()

	// Create two worktrees for two different beads.
	wt1, err := Ensure(context.Background(), worktreesDir, repoPath, "mp-aaa")
	if err != nil {
		t.Fatalf("Ensure mp-aaa: %v", err)
	}
	wt2, err := Ensure(context.Background(), worktreesDir, repoPath, "mp-bbb")
	if err != nil {
		t.Fatalf("Ensure mp-bbb: %v", err)
	}

	// Paths must be distinct.
	if wt1.Path == wt2.Path {
		t.Errorf("worktrees for different beads have same path: %q", wt1.Path)
	}

	// Write a file in wt1; it must not appear in wt2 or the main repo.
	file1 := filepath.Join(wt1.Path, "bead-aaa.txt")
	if err := os.WriteFile(file1, []byte("aaa content"), 0644); err != nil {
		t.Fatalf("write file in wt1: %v", err)
	}

	// File must not appear in wt2.
	if _, err := os.Stat(filepath.Join(wt2.Path, "bead-aaa.txt")); err == nil {
		t.Error("file written in wt1 should not be visible in wt2")
	}

	// File must not appear in main checkout.
	if _, err := os.Stat(filepath.Join(repoPath, "bead-aaa.txt")); err == nil {
		t.Error("file written in wt1 should not be visible in main checkout")
	}
}
