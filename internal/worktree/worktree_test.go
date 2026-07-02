package worktree

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// TestEnsure_ReuseRejectsSymlinkedPath verifies the reuse fast-path refuses a
// pre-planted SYMLINK at <worktreesDir>/<beadID> (via os.Lstat), rather than
// following it and operating on a worktree the attacker redirected outside
// worktreesDir.
func TestEnsure_ReuseRejectsSymlinkedPath(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink test not run on Windows")
	}
	repoPath := initGitRepo(t)
	worktreesDir := t.TempDir()
	target := t.TempDir() // some directory outside worktreesDir
	link := filepath.Join(worktreesDir, "mp-sym")
	if err := os.Symlink(target, link); err != nil {
		t.Fatalf("symlink: %v", err)
	}

	_, err := Ensure(context.Background(), worktreesDir, repoPath, "mp-sym")
	if err == nil {
		t.Fatal("want error for a symlinked worktree path, got nil")
	}
	if !strings.Contains(err.Error(), "symlink") {
		t.Errorf("error should mention the symlink refusal, got: %v", err)
	}
}

// TestEnsure_RejectsUnsafeBeadID verifies Ensure refuses bead IDs that would
// escape worktreesDir when used as a path segment (path traversal). The guard
// is defense-in-depth: the HTTP layer already allow-lists IDs, but Ensure must
// not trust its caller.
func TestEnsure_RejectsUnsafeBeadID(t *testing.T) {
	repoPath := initGitRepo(t)
	worktreesDir := t.TempDir()

	for _, bad := range []string{"../x", "../../etc", "a/b", "/abs", "", "."} {
		_, err := Ensure(context.Background(), worktreesDir, repoPath, bad)
		if err == nil {
			t.Errorf("Ensure(beadID=%q) = nil error, want rejection", bad)
		}
	}
}

// TestEnsure_TightensPreExistingWorktreesDirPermissions verifies that Ensure
// tightens worktreesDir's permissions to 0o700 even when the directory
// already existed with looser permissions — e.g. a shared default like
// <os.TempDir()>/muster/worktrees pre-planted by another local user, or
// created earlier under a looser umask. os.MkdirAll alone would silently
// reuse such a directory as-is, since it only applies the mode to
// directories it actually creates.
func TestEnsure_TightensPreExistingWorktreesDirPermissions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX permission bits (0o700) are not meaningful on Windows")
	}
	repoPath := initGitRepo(t)
	parent := t.TempDir()
	worktreesDir := filepath.Join(parent, "worktrees")
	if err := os.Mkdir(worktreesDir, 0o755); err != nil {
		t.Fatalf("pre-create worktreesDir: %v", err)
	}

	if _, err := Ensure(context.Background(), worktreesDir, repoPath, "mp-abc"); err != nil {
		t.Fatalf("Ensure: %v", err)
	}

	info, err := os.Stat(worktreesDir)
	if err != nil {
		t.Fatalf("stat worktreesDir: %v", err)
	}
	if got := info.Mode().Perm(); got != 0o700 {
		t.Errorf("worktreesDir perm want 0o700 got %o", got)
	}
}

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

// TestEnsure_ReuseRejectsWrongBranch verifies Ensure refuses to reuse an
// existing worktree that a user has switched to a different branch. Reusing it
// as-is would silently run the agent against an unexpected revision, breaking
// the per-bead branch invariant.
func TestEnsure_ReuseRejectsWrongBranch(t *testing.T) {
	repoPath := initGitRepo(t)
	worktreesDir := t.TempDir()

	wt, err := Ensure(context.Background(), worktreesDir, repoPath, "mp-abc")
	if err != nil {
		t.Fatalf("first Ensure: %v", err)
	}

	// Simulate a user switching the worktree to a different branch.
	runGitCmd(t, wt.Path, "checkout", "-b", "somethingelse")

	_, err = Ensure(context.Background(), worktreesDir, repoPath, "mp-abc")
	if err == nil {
		t.Fatal("want error reusing a worktree on the wrong branch, got nil")
	}
	if !strings.Contains(err.Error(), "per-bead branch invariant") {
		t.Errorf("error %q should mention the per-bead branch invariant", err.Error())
	}
}

// TestEnsure_ReuseRejectsDetachedHEAD verifies Ensure refuses to reuse an
// existing worktree whose HEAD has been detached (e.g. a manual `git checkout
// <sha>`), which similarly would run the agent against an unexpected revision.
func TestEnsure_ReuseRejectsDetachedHEAD(t *testing.T) {
	repoPath := initGitRepo(t)
	worktreesDir := t.TempDir()

	wt, err := Ensure(context.Background(), worktreesDir, repoPath, "mp-abc")
	if err != nil {
		t.Fatalf("first Ensure: %v", err)
	}

	// Detach HEAD at the current commit.
	runGitCmd(t, wt.Path, "checkout", "--detach", "HEAD")

	_, err = Ensure(context.Background(), worktreesDir, repoPath, "mp-abc")
	if err == nil {
		t.Fatal("want error reusing a worktree with a detached HEAD, got nil")
	}
	if !strings.Contains(err.Error(), "detached HEAD") {
		t.Errorf("error %q should mention detached HEAD", err.Error())
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

func TestEnsure_CancelledContext_NoLeftoverDir(t *testing.T) {
	// With an already-cancelled context, Ensure aborts early — at the
	// validateGitRepo probe (git rev-parse), before it ever reaches
	// `git worktree add` — and must leave no directory behind and not
	// permanently block the bead: a later call with a healthy context must
	// still succeed. (This asserts the clean-abort/retry contract; it does not
	// exercise the mid-`git worktree add` cleanup path, which a cancelled ctx
	// can't reach.)
	repoPath := initGitRepo(t)
	worktreesDir := t.TempDir()
	path := filepath.Join(worktreesDir, "mp-killed")

	cancelledCtx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := Ensure(cancelledCtx, worktreesDir, repoPath, "mp-killed")
	if err == nil {
		t.Fatal("want error from Ensure with an already-cancelled context, got nil")
	}

	// The half-created directory must not linger.
	if _, statErr := os.Stat(path); !os.IsNotExist(statErr) {
		t.Errorf("worktree path %q should not exist after a failed create, stat err: %v", path, statErr)
	}

	// A subsequent Ensure call with a healthy context must succeed cleanly —
	// the bead must not be permanently stuck because of the earlier failure.
	wt, err := Ensure(context.Background(), worktreesDir, repoPath, "mp-killed")
	if err != nil {
		t.Fatalf("Ensure after failed create should succeed, got: %v", err)
	}
	if _, statErr := os.Stat(wt.Path); statErr != nil {
		t.Errorf("worktree should exist after successful retry: %v", statErr)
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

// TestEnsure_TagNamedLikeBranch verifies that a tag named muster/<beadID>
// does NOT cause Ensure to treat it as an existing branch. A bare
// `git rev-parse --verify muster/<beadID>` would resolve the tag, sending
// Ensure down the "reuse existing branch" path and checking out the tag in
// detached HEAD — breaking the dedicated-branch invariant. Ensure must instead
// create a real branch refs/heads/muster/<beadID>.
func TestEnsure_TagNamedLikeBranch(t *testing.T) {
	repoPath := initGitRepo(t)
	worktreesDir := t.TempDir()

	// Create a tag that collides with the branch name Ensure will want.
	runGitCmd(t, repoPath, "tag", "muster/mp-tagcollide")

	wt, err := Ensure(context.Background(), worktreesDir, repoPath, "mp-tagcollide")
	if err != nil {
		t.Fatalf("Ensure: %v", err)
	}

	// The worktree must be on a real local branch, not a detached HEAD on the
	// tag. `git symbolic-ref HEAD` fails (non-zero) on a detached HEAD, so
	// capture the error ourselves rather than fataling with a raw git error.
	// Use the full ref form (not --short): with a same-named tag and branch,
	// --short keeps a disambiguating "heads/" prefix, but the full ref is
	// unambiguously refs/heads/... — which is exactly the invariant we assert.
	cmd := exec.Command("git", "symbolic-ref", "HEAD")
	cmd.Dir = wt.Path
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("worktree HEAD is not on a branch (detached-on-tag?): %v\n%s", err, out)
	}
	if got := strings.TrimSpace(string(out)); got != "refs/heads/muster/mp-tagcollide" {
		t.Errorf("worktree HEAD want refs/heads/muster/mp-tagcollide, got %q", got)
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
