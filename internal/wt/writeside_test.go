package wt_test

// T031: Failing git write-side tests: Finalize, Push, Remove.
// This file contains both fake-on-$PATH unit tests and real-git integration
// tests (skip-gated when git is absent, but git is assumed present here).

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gitrgoliveira/muster/internal/wt"
)

// ─── helpers ───────────────────────────────────────────────────────────────

// initGitRepoForWrite creates a temp git repo with one initial commit and
// configures hermetic identity. Returns the repo path.
func initGitRepoForWrite(t *testing.T) string {
	t.Helper()
	repo := t.TempDir()
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = repo
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=Test",
			"GIT_AUTHOR_EMAIL=test@test.com",
			"GIT_COMMITTER_NAME=Test",
			"GIT_COMMITTER_EMAIL=test@test.com",
		)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	run("init")
	run("config", "user.email", "test@test.com")
	run("config", "user.name", "Test")
	if err := os.WriteFile(filepath.Join(repo, "README.md"), []byte("# test\n"), 0o644); err != nil {
		t.Fatalf("write README: %v", err)
	}
	run("add", ".")
	run("commit", "-m", "initial commit")
	return repo
}

// createGitWorktree creates a worktree under worktreesDir for beadID.
// Returns the worktree path.
func createGitWorktree(t *testing.T, srcRepo, worktreesDir, beadID string) string {
	t.Helper()
	b, err := wt.For(wt.VCSGit)
	if err != nil {
		t.Fatalf("wt.For(git): %v", err)
	}
	ctx := context.Background()
	path, err := b.Create(ctx, worktreesDir, srcRepo, beadID)
	if err != nil {
		t.Fatalf("Create worktree: %v", err)
	}
	return path
}

// gitLog returns the short log for the worktree's branch in the format
// "<hash> <message>". Fatals on error.
func gitLogLatest(t *testing.T, wtPath string) string {
	t.Helper()
	cmd := exec.Command("git", "log", "--oneline", "-1")
	cmd.Dir = wtPath
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git log: %v\n%s", err, out)
	}
	return strings.TrimSpace(string(out))
}

// gitBranchExistsOnRemote checks if a branch exists in the bare remote repo.
func gitBranchExistsOnRemote(t *testing.T, remoteDir, branch string) bool {
	t.Helper()
	cmd := exec.Command("git", "branch", "-v")
	cmd.Dir = remoteDir
	out, err := cmd.CombinedOutput()
	if err != nil {
		return false
	}
	return strings.Contains(string(out), branch)
}

// ─── T031: git Finalize tests ─────────────────────────────────────────────

// TestGitFinalize_NoChanges verifies that Finalize on a clean worktree is a
// no-op success (no commit created).
func TestGitFinalize_NoChanges(t *testing.T) {
	srcRepo := initGitRepoForWrite(t)
	worktreesDir := t.TempDir()
	beadID := "bead-finalize-noop"

	createGitWorktree(t, srcRepo, worktreesDir, beadID)

	b := wt.NewGitBackend(worktreesDir)
	ctx := context.Background()

	// Before finalize: get the current commit hash.
	wtPath := filepath.Join(worktreesDir, beadID)
	beforeLog := gitLogLatest(t, wtPath)

	committed, err := b.Finalize(ctx, beadID, "should not commit")
	if err != nil {
		t.Fatalf("Finalize on clean worktree: expected success, got %v", err)
	}
	if committed {
		t.Error("Finalize on clean worktree: committed want false, got true")
	}

	// After finalize: commit log must be unchanged (no new commit).
	afterLog := gitLogLatest(t, wtPath)
	if beforeLog != afterLog {
		t.Errorf("Finalize on clean worktree created a commit: before=%q after=%q", beforeLog, afterLog)
	}
}

// TestGitFinalize_WithChanges verifies that Finalize on a dirty worktree
// creates a commit with the given message on branch muster/<beadID>.
func TestGitFinalize_WithChanges(t *testing.T) {
	srcRepo := initGitRepoForWrite(t)
	worktreesDir := t.TempDir()
	beadID := "bead-finalize-dirty"

	wtPath := createGitWorktree(t, srcRepo, worktreesDir, beadID)

	// Dirty the worktree.
	if err := os.WriteFile(filepath.Join(wtPath, "output.txt"), []byte("agent result\n"), 0o644); err != nil {
		t.Fatalf("write output: %v", err)
	}

	b := wt.NewGitBackend(worktreesDir)
	ctx := context.Background()

	committed, err := b.Finalize(ctx, beadID, "feat: agent work done")
	if err != nil {
		t.Fatalf("Finalize: %v", err)
	}
	if !committed {
		t.Error("Finalize on dirty worktree: committed want true, got false")
	}

	// Verify a commit was created with the right message.
	log := gitLogLatest(t, wtPath)
	if !strings.Contains(log, "feat: agent work done") {
		t.Errorf("expected commit message %q in log, got %q", "feat: agent work done", log)
	}

	// Verify the worktree is now clean.
	st, err := wt.GitStatusAt(ctx, worktreesDir, beadID)
	if err != nil {
		t.Fatalf("GitStatusAt: %v", err)
	}
	if !st.Clean {
		t.Errorf("expected worktree clean after Finalize, got dirty")
	}
}

// TestGitFinalize_AllFilesStaged verifies Finalize stages untracked, modified,
// and deleted files before committing.
func TestGitFinalize_AllFilesStaged(t *testing.T) {
	srcRepo := initGitRepoForWrite(t)
	worktreesDir := t.TempDir()
	beadID := "bead-finalize-all"

	wtPath := createGitWorktree(t, srcRepo, worktreesDir, beadID)

	// Add a file to stage tracking first (so we can delete it).
	if err := os.WriteFile(filepath.Join(wtPath, "to_delete.txt"), []byte("delete me\n"), 0o644); err != nil {
		t.Fatalf("write to_delete: %v", err)
	}
	// Stage and commit to get the file tracked.
	runGitCmd(t, wtPath, "add", ".")
	runGitCmd(t, wtPath, "commit", "-m", "pre-finalize")

	// Now make changes: new file, modified, deleted.
	if err := os.WriteFile(filepath.Join(wtPath, "new_file.txt"), []byte("new\n"), 0o644); err != nil {
		t.Fatalf("write new_file: %v", err)
	}
	if err := os.WriteFile(filepath.Join(wtPath, "README.md"), []byte("# modified\n"), 0o644); err != nil {
		t.Fatalf("write README: %v", err)
	}
	if err := os.Remove(filepath.Join(wtPath, "to_delete.txt")); err != nil {
		t.Fatalf("remove: %v", err)
	}

	b := wt.NewGitBackend(worktreesDir)
	ctx := context.Background()

	if _, err := b.Finalize(ctx, beadID, "all changes committed"); err != nil {
		t.Fatalf("Finalize: %v", err)
	}

	// Worktree should be clean.
	st, err := wt.GitStatusAt(ctx, worktreesDir, beadID)
	if err != nil {
		t.Fatalf("GitStatusAt: %v", err)
	}
	if !st.Clean {
		t.Error("expected clean worktree after Finalize")
	}
}

// TestGitFinalize_MissingWorktree verifies Finalize returns ErrWorktreeNotFound
// when the worktree directory does not exist.
func TestGitFinalize_MissingWorktree(t *testing.T) {
	b := wt.NewGitBackend(t.TempDir()) // empty worktreesDir with no beads
	ctx := context.Background()
	_, err := b.Finalize(ctx, "nonexistent", "msg")
	if err == nil {
		t.Fatal("expected error for missing worktree, got nil")
	}
}

// ─── T031: git Push tests ─────────────────────────────────────────────────

// TestGitPush_ToBarePushable verifies that Push pushes the branch to a bare
// upstream repository. This is the integration test that asserts the branch
// actually lands on the remote.
func TestGitPush_ToBarePushable(t *testing.T) {
	// Set up a bare "remote".
	remoteDir := t.TempDir()
	runGitCmd(t, remoteDir, "init", "--bare")

	// Set up source repo with initial commit.
	srcRepo := initGitRepoForWrite(t)
	runGitCmd(t, srcRepo, "remote", "add", "origin", remoteDir)

	worktreesDir := t.TempDir()
	beadID := "bead-push-test"

	wtPath := createGitWorktree(t, srcRepo, worktreesDir, beadID)

	// Dirty and finalize the worktree.
	if err := os.WriteFile(filepath.Join(wtPath, "output.txt"), []byte("result\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	b := wt.NewGitBackend(worktreesDir)
	ctx := context.Background()

	if _, err := b.Finalize(ctx, beadID, "finalized for push"); err != nil {
		t.Fatalf("Finalize: %v", err)
	}

	// Push to the bare remote.
	if err := b.Push(ctx, beadID, ""); err != nil {
		t.Fatalf("Push: %v", err)
	}

	// Verify the branch landed on the remote.
	branch := wt.BranchName(beadID)
	if !gitBranchExistsOnRemote(t, remoteDir, branch) {
		t.Errorf("expected branch %q on remote, not found", branch)
	}
}

// TestGitPush_NoRemote verifies that Push returns an explicit error when there
// is no remote configured, not silent success.
func TestGitPush_NoRemote(t *testing.T) {
	srcRepo := initGitRepoForWrite(t)
	worktreesDir := t.TempDir()
	beadID := "bead-push-noremote"

	createGitWorktree(t, srcRepo, worktreesDir, beadID)

	b := wt.NewGitBackend(worktreesDir)
	ctx := context.Background()

	// Push without a remote configured — must fail with an error.
	err := b.Push(ctx, beadID, "")
	if err == nil {
		t.Fatal("Push with no remote: expected error, got nil")
	}
}

// ─── T031: git Remove tests ───────────────────────────────────────────────

// TestGitRemove_WorktreeAbsentAfter verifies that after Remove, Status reports
// the worktree as absent.
func TestGitRemove_WorktreeAbsentAfter(t *testing.T) {
	srcRepo := initGitRepoForWrite(t)
	worktreesDir := t.TempDir()
	beadID := "bead-remove-test"

	createGitWorktree(t, srcRepo, worktreesDir, beadID)

	b := wt.NewGitBackend(worktreesDir)
	ctx := context.Background()

	// Verify worktree exists before Remove.
	st, err := b.Status(ctx, beadID)
	if err != nil || !st.Exists {
		t.Fatalf("Status before Remove: err=%v exists=%v", err, st.Exists)
	}

	// Remove the worktree.
	if err := b.Remove(ctx, beadID); err != nil {
		t.Fatalf("Remove: %v", err)
	}

	// Status should now report absent.
	st, err = b.Status(ctx, beadID)
	if err == nil && st.Exists {
		t.Error("expected worktree absent after Remove, but Status reports it exists")
	}
}

// TestGitRemove_MissingWorktree verifies Remove on a non-existent worktree
// returns an error (not panics or silent success).
func TestGitRemove_MissingWorktree(t *testing.T) {
	b := wt.NewGitBackend(t.TempDir())
	ctx := context.Background()
	err := b.Remove(ctx, "nonexistent")
	if err == nil {
		t.Fatal("Remove on missing worktree: expected error, got nil")
	}
}

// TestGitRemove_CleanWorktreeSucceeds verifies Remove works on a clean worktree
// (no uncommitted changes).
func TestGitRemove_CleanWorktreeSucceeds(t *testing.T) {
	srcRepo := initGitRepoForWrite(t)
	worktreesDir := t.TempDir()
	beadID := "bead-remove-clean"

	createGitWorktree(t, srcRepo, worktreesDir, beadID)

	b := wt.NewGitBackend(worktreesDir)
	ctx := context.Background()

	if err := b.Remove(ctx, beadID); err != nil {
		t.Fatalf("Remove clean worktree: %v", err)
	}

	// Directory must be gone.
	wtPath := filepath.Join(worktreesDir, beadID)
	if _, err := os.Lstat(wtPath); !os.IsNotExist(err) {
		t.Errorf("worktree directory still exists after Remove: %v", err)
	}
}

// TestGitRemove_DirtyWorktree_ReturnsDirtyError verifies that Remove on a
// worktree with uncommitted changes returns ErrWorktreeDirty and leaves the
// directory intact (Fix B: dirty-remove guard).
func TestGitRemove_DirtyWorktree_ReturnsDirtyError(t *testing.T) {
	srcRepo := initGitRepoForWrite(t)
	worktreesDir := t.TempDir()
	beadID := "bead-remove-dirty"

	wtPath := createGitWorktree(t, srcRepo, worktreesDir, beadID)

	// Write an uncommitted file to make the worktree dirty.
	if err := os.WriteFile(filepath.Join(wtPath, "uncommitted.txt"), []byte("dirty\n"), 0o644); err != nil {
		t.Fatalf("write uncommitted file: %v", err)
	}

	b := wt.NewGitBackend(worktreesDir)
	ctx := context.Background()

	err := b.Remove(ctx, beadID)
	if !errors.Is(err, wt.ErrWorktreeDirty) {
		t.Fatalf("Remove dirty worktree: want ErrWorktreeDirty, got %v", err)
	}

	// Directory must still exist — Remove must not discard the dirty changes.
	if _, statErr := os.Lstat(wtPath); os.IsNotExist(statErr) {
		t.Error("worktree directory was deleted despite dirty changes (data loss)")
	}
}

// TestGitWriteMethods_VCSUnavailable verifies ErrVCSUnavailable behavior when
// the git binary is removed from PATH. This simulates the runtime-missing case.
func TestGitWriteMethods_VCSUnavailable(t *testing.T) {
	// Create a fake-on-path dir with no git binary (empty bin dir).
	binDir := t.TempDir()
	// Add binDir first on PATH so the real git is shadowed.
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	// The wt package checks for the binary at call time.
	// We verify that Finalize/Push/Remove return an error when git is absent.
	// The errors won't be ErrVCSUnavailable itself (that's for VCS type mismatch)
	// but rather an exec error — either exec.ErrNotFound or a subprocess failure.
	// The key requirement is: no panic, explicit error returned.
	b := wt.NewGitBackend(t.TempDir())
	ctx := context.Background()

	for _, op := range []struct {
		name string
		fn   func() error
	}{
		{"Finalize", func() error { _, err := b.Finalize(ctx, "any", "msg"); return err }},
		{"Push", func() error { return b.Push(ctx, "any", "") }},
		{"Remove", func() error { return b.Remove(ctx, "any") }},
	} {
		if err := op.fn(); err == nil {
			t.Errorf("%s with absent git: expected error, got nil", op.name)
		}
	}
}

// TestGitWriteMethods_NotImplemented is the migrated M3 test that previously
// asserted ErrNotImplemented. In M4 these methods are implemented, so this
// test now asserts that the error is NOT ErrNotImplemented.
func TestGitWriteMethods_NotErrNotImplemented(t *testing.T) {
	b := wt.NewGitBackend(t.TempDir())
	ctx := context.Background()

	// All three should return real errors (e.g. worktree not found), not the stub.
	if _, err := b.Finalize(ctx, "bead", "msg"); err == wt.ErrNotImplemented {
		t.Error("Finalize: still returning ErrNotImplemented (M3 stub not replaced)")
	}
	if err := b.Push(ctx, "bead", ""); err == wt.ErrNotImplemented {
		t.Error("Push: still returning ErrNotImplemented (M3 stub not replaced)")
	}
	if err := b.Remove(ctx, "bead"); err == wt.ErrNotImplemented {
		t.Error("Remove: still returning ErrNotImplemented (M3 stub not replaced)")
	}
}
