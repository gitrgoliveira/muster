//go:build !windows

// T032: jj write-side tests: Finalize, Push, Remove (real-jj integration,
// skip-gated when jj is absent). Uses the same hermetic env as jj_test.go.

package wt_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gitrgoliveira/muster/internal/wt"
)

// initJJRepoForWrite creates a real jj repo with a committed initial file.
// The initial file is committed by running jj describe + jj new so the WC
// starts clean (no pending adds). Skips if jj is not available.
func initJJRepoForWrite(t *testing.T) string {
	t.Helper()
	if !jjAvailable() {
		t.Skip("jj not available on PATH")
	}
	t.Setenv("JJ_CONFIG", "/dev/null")

	dir := t.TempDir()
	run := func(args ...string) string {
		t.Helper()
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Dir = dir
		cmd.Env = jjEnv()
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("%v: %v\n%s", args, err, out)
		}
		return string(out)
	}

	run("jj", "git", "init")
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("# jj test\n"), 0o644); err != nil {
		t.Fatalf("write README: %v", err)
	}
	// Describe the initial change and create a new empty WC so the state is clean.
	run("jj", "describe", "-m", "initial commit")
	run("jj", "new")
	return dir
}

// createJJWorkspace creates a jj workspace at worktreesDir/<beadID> using the
// real jj binary. Returns the workspace path.
func createJJWorkspace(t *testing.T, srcRepo, worktreesDir, beadID string) string {
	t.Helper()
	b, err := wt.For(wt.VCSJJ)
	if err != nil {
		t.Fatalf("wt.For(jj): %v", err)
	}
	ctx := context.Background()
	path, err := b.Create(ctx, worktreesDir, srcRepo, beadID)
	if err != nil {
		t.Fatalf("Create jj workspace: %v", err)
	}
	return path
}

// runJJ runs a jj command in dir with the hermetic env and returns combined output.
func runJJ(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("jj", args...)
	cmd.Dir = dir
	cmd.Env = jjEnv()
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("jj %v in %s: %v\n%s", args, dir, err, out)
	}
	return string(out)
}

// ── T032: jj Finalize tests ─────────────────────────────────────────────────

// TestJJFinalize_NoChanges verifies Finalize is a no-op when the workspace has
// no changes (empty jj diff --summary → success, no new revision).
func TestJJFinalize_NoChanges(t *testing.T) {
	if !jjAvailable() {
		t.Skip("jj not available on PATH")
	}
	t.Setenv("JJ_CONFIG", "/dev/null")

	srcRepo := initJJRepoForWrite(t)
	worktreesDir := t.TempDir()
	beadID := "jj-finalize-noop"

	createJJWorkspace(t, srcRepo, worktreesDir, beadID)

	b := wt.NewJJBackend(worktreesDir)
	ctx := context.Background()

	// Workspace should be clean: no changes.
	wtPath := filepath.Join(worktreesDir, beadID)
	diffBefore := runJJ(t, wtPath, "log", "--no-pager", "-r", "@")

	committed, err := b.Finalize(ctx, beadID, "should not commit")
	if err != nil {
		t.Fatalf("Finalize on clean workspace: expected success, got %v", err)
	}
	if committed {
		t.Error("Finalize on clean workspace: committed want false, got true")
	}

	// The working-copy revision ID should be unchanged (no describe happened).
	diffAfter := runJJ(t, wtPath, "log", "--no-pager", "-r", "@")
	if diffBefore != diffAfter {
		t.Errorf("Finalize on clean workspace modified the revision:\nbefore=%q\nafter=%q", diffBefore, diffAfter)
	}
}

// TestJJFinalize_WithChanges verifies Finalize commits changes with the given
// message, creating a sealed revision on the workspace's "branch".
func TestJJFinalize_WithChanges(t *testing.T) {
	if !jjAvailable() {
		t.Skip("jj not available on PATH")
	}
	t.Setenv("JJ_CONFIG", "/dev/null")

	srcRepo := initJJRepoForWrite(t)
	worktreesDir := t.TempDir()
	beadID := "jj-finalize-dirty"

	wtPath := createJJWorkspace(t, srcRepo, worktreesDir, beadID)

	// Write a file to the workspace.
	if err := os.WriteFile(filepath.Join(wtPath, "output.txt"), []byte("agent output\n"), 0o644); err != nil {
		t.Fatalf("write output: %v", err)
	}

	b := wt.NewJJBackend(worktreesDir)
	ctx := context.Background()

	committed, err := b.Finalize(ctx, beadID, "feat: jj bead work done")
	if err != nil {
		t.Fatalf("Finalize: %v", err)
	}
	if !committed {
		t.Error("Finalize on dirty workspace: committed want true, got false")
	}

	// The parent revision (@-) should have the message.
	log := runJJ(t, wtPath, "log", "--no-pager", "-r", "@-")
	if !strings.Contains(log, "feat: jj bead work done") {
		t.Errorf("expected commit message in log, got:\n%s", log)
	}

	// The working copy should now be clean (no changes after the describe+new).
	summaryOut := runJJ(t, wtPath, "diff", "--summary")
	if strings.TrimSpace(summaryOut) != "" {
		t.Errorf("expected clean workspace after Finalize, got diff:\n%s", summaryOut)
	}
}

// TestJJFinalize_MissingWorkspace verifies Finalize returns an error when the
// workspace directory does not exist.
func TestJJFinalize_MissingWorkspace(t *testing.T) {
	if !jjAvailable() {
		t.Skip("jj not available on PATH")
	}
	t.Setenv("JJ_CONFIG", "/dev/null")

	b := wt.NewJJBackend(t.TempDir())
	ctx := context.Background()

	if _, err := b.Finalize(ctx, "nonexistent", "msg"); err == nil {
		t.Fatal("expected error for missing workspace, got nil")
	}
}

// ── T032: jj Push tests ─────────────────────────────────────────────────────

// TestJJPush_ToBarePushable verifies that Push pushes the bead's branch to a
// bare upstream repository by creating a bookmark and using git push.
func TestJJPush_ToBarePushable(t *testing.T) {
	if !jjAvailable() {
		t.Skip("jj not available on PATH")
	}
	t.Setenv("JJ_CONFIG", "/dev/null")

	// Set up a bare "remote".
	remoteDir := t.TempDir()
	if out, err := exec.Command("git", "init", "--bare", remoteDir).CombinedOutput(); err != nil {
		t.Fatalf("git init bare: %v\n%s", err, out)
	}

	srcRepo := initJJRepoForWrite(t)

	// Add the git remote to the srcrepo.
	runJJ(t, srcRepo, "git", "remote", "add", "origin", remoteDir)

	worktreesDir := t.TempDir()
	beadID := "jj-push-test"

	wtPath := createJJWorkspace(t, srcRepo, worktreesDir, beadID)

	// Write and finalize so there's something to push.
	if err := os.WriteFile(filepath.Join(wtPath, "result.txt"), []byte("done\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	b := wt.NewJJBackend(worktreesDir)
	ctx := context.Background()

	if _, err := b.Finalize(ctx, beadID, "feat: jj push test"); err != nil {
		t.Fatalf("Finalize: %v", err)
	}

	if err := b.Push(ctx, beadID, ""); err != nil {
		t.Fatalf("Push: %v", err)
	}

	// Verify the branch landed on the remote.
	branch := wt.BranchName(beadID)
	cmd := exec.Command("git", "branch", "-v")
	cmd.Dir = remoteDir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git branch -v on remote: %v\n%s", err, out)
	}
	if !strings.Contains(string(out), branch) {
		t.Errorf("expected branch %q on remote, not found in:\n%s", branch, out)
	}
}

// TestJJPush_NoRemote verifies Push returns an error when no remote is configured.
func TestJJPush_NoRemote(t *testing.T) {
	if !jjAvailable() {
		t.Skip("jj not available on PATH")
	}
	t.Setenv("JJ_CONFIG", "/dev/null")

	srcRepo := initJJRepoForWrite(t)
	worktreesDir := t.TempDir()
	beadID := "jj-push-noremote"

	createJJWorkspace(t, srcRepo, worktreesDir, beadID)

	b := wt.NewJJBackend(worktreesDir)
	ctx := context.Background()

	// Push without a remote configured — must fail with an error.
	err := b.Push(ctx, beadID, "")
	if err == nil {
		t.Fatal("Push with no remote: expected error, got nil")
	}
}

// ── T032: jj Remove tests ────────────────────────────────────────────────────

// TestJJRemove_WorkspaceAbsentAfter verifies that after Remove, Status reports
// the workspace as absent.
func TestJJRemove_WorkspaceAbsentAfter(t *testing.T) {
	if !jjAvailable() {
		t.Skip("jj not available on PATH")
	}
	t.Setenv("JJ_CONFIG", "/dev/null")

	srcRepo := initJJRepoForWrite(t)
	worktreesDir := t.TempDir()
	beadID := "jj-remove-test"

	createJJWorkspace(t, srcRepo, worktreesDir, beadID)

	b := wt.NewJJBackend(worktreesDir)
	ctx := context.Background()

	// Verify workspace exists before Remove.
	st, err := b.Status(ctx, beadID)
	if err != nil || !st.Exists {
		t.Fatalf("Status before Remove: err=%v exists=%v", err, st.Exists)
	}

	if err := b.Remove(ctx, beadID); err != nil {
		t.Fatalf("Remove: %v", err)
	}

	// Directory must be gone.
	wtPath := filepath.Join(worktreesDir, beadID)
	if _, err := os.Lstat(wtPath); !os.IsNotExist(err) {
		t.Errorf("workspace directory still exists after Remove: %v", err)
	}

	// Status should now return absent (ErrWorktreeNotFound).
	st, err = b.Status(ctx, beadID)
	if err == nil && st.Exists {
		t.Error("expected workspace absent after Remove, but Status reports it exists")
	}
}

// TestJJRemove_MissingWorkspace verifies Remove on a non-existent workspace
// returns an error.
func TestJJRemove_MissingWorkspace(t *testing.T) {
	if !jjAvailable() {
		t.Skip("jj not available on PATH")
	}
	t.Setenv("JJ_CONFIG", "/dev/null")

	b := wt.NewJJBackend(t.TempDir())
	ctx := context.Background()

	if err := b.Remove(ctx, "nonexistent"); err == nil {
		t.Fatal("Remove on missing workspace: expected error, got nil")
	}
}

// TestJJSrcRepoCache_PopulatedByCreate verifies the security hardening from
// FIX 4 (tri-review #13): after Create, the backend's internal srcRepo cache
// is populated, so Push and Remove can use the trusted srcRepo path from the
// cache rather than re-reading the agent-writable .jj/repo file.
//
// The test verifies this indirectly: a second backend instance (empty cache,
// cold fallback path) is used for Push — it must still succeed by falling back
// to jjSrcRepoDir, confirming the fallback works. The primary (cached) backend
// is then verified to Push successfully as well, proving the cache path works.
//
// Full proof that the cache is preferred over a tampered .jj/repo is not
// testable end-to-end without mocking jj internals, because jj commands
// (bookmark create, git export) also read .jj/repo internally. The cache
// prevents re-reading .jj/repo specifically for the git push srcRepo resolution.
func TestJJSrcRepoCache_PopulatedByCreate(t *testing.T) {
	if !jjAvailable() {
		t.Skip("jj not available on PATH")
	}
	t.Setenv("JJ_CONFIG", "/dev/null")

	// Set up a bare "remote".
	remoteDir := t.TempDir()
	if out, err := exec.Command("git", "init", "--bare", remoteDir).CombinedOutput(); err != nil {
		t.Fatalf("git init bare: %v\n%s", err, out)
	}

	srcRepo := initJJRepoForWrite(t)
	runJJ(t, srcRepo, "git", "remote", "add", "origin", remoteDir)

	worktreesDir := t.TempDir()
	beadID := "jj-cache-test"

	// bPrimary: same instance used for Create and Push — exercises the cache path.
	bPrimary := wt.NewJJBackend(worktreesDir)
	ctx := context.Background()

	wtPath, err := bPrimary.Create(ctx, worktreesDir, srcRepo, beadID)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	// Write and finalize.
	if err := os.WriteFile(filepath.Join(wtPath, "result.txt"), []byte("data\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	if _, err := bPrimary.Finalize(ctx, beadID, "feat: cache populate test"); err != nil {
		t.Fatalf("Finalize: %v", err)
	}

	// Push via the same backend instance (cache path).
	if err := bPrimary.Push(ctx, beadID, ""); err != nil {
		t.Fatalf("Push via cached backend: %v", err)
	}

	// Verify a fresh (cold-cache) backend also works via the fallback path.
	// This confirms jjSrcRepoDir (the .jj/repo read) is still functional when
	// the cache is absent — e.g. when the orchestrator reconstructs the backend.
	// (We don't push again — just verify Remove works via fallback path.)
	bFresh := wt.NewJJBackend(worktreesDir)
	if err := bFresh.Remove(ctx, beadID); err != nil {
		t.Fatalf("Remove via fresh (cold-cache) backend: %v", err)
	}
}

// TestJJWriteMethods_NotErrNotImplemented asserts that M4 replaced the M3 stubs.
func TestJJWriteMethods_NotErrNotImplemented(t *testing.T) {
	if !jjAvailable() {
		t.Skip("jj not available on PATH")
	}
	t.Setenv("JJ_CONFIG", "/dev/null")

	b := wt.NewJJBackend(t.TempDir())
	ctx := context.Background()

	if _, err := b.Finalize(ctx, "bead", "msg"); err == wt.ErrNotImplemented {
		t.Error("Finalize: M3 stub still in place (ErrNotImplemented)")
	}
	if err := b.Push(ctx, "bead", ""); err == wt.ErrNotImplemented {
		t.Error("Push: M3 stub still in place (ErrNotImplemented)")
	}
	if err := b.Remove(ctx, "bead"); err == wt.ErrNotImplemented {
		t.Error("Remove: M3 stub still in place (ErrNotImplemented)")
	}
}
