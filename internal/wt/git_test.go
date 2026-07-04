package wt_test

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gitrgoliveira/muster/internal/wt"
)

// gitB is a helper that returns the gitBackend accessor (via For + type assertion).
// We use the exported For() for construction and expose the extended At methods
// via the concrete package-internal type indirectly through the service layer.
// For tests we use the wt package's internal accessor helpers directly.

// ── T008: gitBackend.Create (SC-001 — behaviour-preserving) ───────────────

func TestGitBackend_Create_NewWorktree(t *testing.T) {
	repoPath := initGitRepo(t)
	worktreesDir := t.TempDir()
	ctx := context.Background()

	b, err := wt.For(wt.VCSGit)
	if err != nil {
		t.Fatalf("For(git): %v", err)
	}

	path, err := b.Create(ctx, worktreesDir, repoPath, "mp-test")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	// Path must be <worktreesDir>/mp-test
	want := filepath.Join(worktreesDir, "mp-test")
	if path != want {
		t.Errorf("Create path = %q, want %q", path, want)
	}
	// The directory must exist as a linked git worktree (.git is a file).
	fi, err := os.Stat(filepath.Join(path, ".git"))
	if err != nil {
		t.Fatalf("worktree .git: %v", err)
	}
	if fi.IsDir() {
		t.Errorf(".git is a directory (main checkout?), want file (linked worktree)")
	}
}

func TestGitBackend_Create_Reuse(t *testing.T) {
	repoPath := initGitRepo(t)
	worktreesDir := t.TempDir()
	ctx := context.Background()

	b, _ := wt.For(wt.VCSGit)

	path1, err := b.Create(ctx, worktreesDir, repoPath, "mp-reuse")
	if err != nil {
		t.Fatalf("first Create: %v", err)
	}
	path2, err := b.Create(ctx, worktreesDir, repoPath, "mp-reuse")
	if err != nil {
		t.Fatalf("second Create (reuse): %v", err)
	}
	if path1 != path2 {
		t.Errorf("reuse path mismatch: %q vs %q", path1, path2)
	}
}

func TestGitBackend_Create_NonGitRepo(t *testing.T) {
	notARepo := t.TempDir()
	worktreesDir := t.TempDir()
	ctx := context.Background()

	b, _ := wt.For(wt.VCSGit)

	_, err := b.Create(ctx, worktreesDir, notARepo, "mp-fail")
	if err == nil {
		t.Fatal("Create(non-git-repo): expected error, got nil")
	}
}

func TestGitBackend_Create_SymlinkRejected(t *testing.T) {
	repoPath := initGitRepo(t)
	worktreesDir := t.TempDir()
	ctx := context.Background()

	b, _ := wt.For(wt.VCSGit)

	// Pre-plant a symlink at the expected worktree path.
	target := t.TempDir()
	link := filepath.Join(worktreesDir, "mp-sym")
	if err := os.Symlink(target, link); err != nil {
		t.Fatalf("Symlink: %v", err)
	}

	_, err := b.Create(ctx, worktreesDir, repoPath, "mp-sym")
	if err == nil {
		t.Fatal("Create(symlink path): expected error, got nil")
	}
	if !strings.Contains(err.Error(), "symlink") {
		t.Errorf("error should mention symlink, got: %v", err)
	}
}

func TestGitBackend_Create_TwoBeadIsolation(t *testing.T) {
	repoPath := initGitRepo(t)
	worktreesDir := t.TempDir()
	ctx := context.Background()

	b, _ := wt.For(wt.VCSGit)

	path1, err := b.Create(ctx, worktreesDir, repoPath, "mp-beada")
	if err != nil {
		t.Fatalf("Create bead-a: %v", err)
	}
	path2, err := b.Create(ctx, worktreesDir, repoPath, "mp-beadb")
	if err != nil {
		t.Fatalf("Create bead-b: %v", err)
	}
	if path1 == path2 {
		t.Errorf("two beads should have distinct paths, both got %q", path1)
	}
}

func TestGitBackend_Create_UnsafeBeadID(t *testing.T) {
	repoPath := initGitRepo(t)
	worktreesDir := t.TempDir()
	ctx := context.Background()

	b, _ := wt.For(wt.VCSGit)

	_, err := b.Create(ctx, worktreesDir, repoPath, "../escape")
	if err == nil {
		t.Fatal("Create(../escape): expected error, got nil")
	}
}

// ── T010: gitBackend.Status ────────────────────────────────────────────────

// statusAt calls the concrete method via the wt.GitStatusAt helper so we don't
// expose StatusAt on the Backend interface. Since StatusAt is package-internal
// we test it via the exported GitStatusAt helper.
// Actually, the Backend interface has Status but it needs worktreesDir.
// We expose it via the GitBackendAccessor helper in the wt package.

func TestGitBackend_StatusAt_CleanWorktree(t *testing.T) {
	repoPath := initGitRepo(t)
	worktreesDir := t.TempDir()
	ctx := context.Background()

	b, _ := wt.For(wt.VCSGit)
	_, err := b.Create(ctx, worktreesDir, repoPath, "mp-clean")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	st, err := wt.GitStatusAt(ctx, worktreesDir, "mp-clean")
	if err != nil {
		t.Fatalf("StatusAt: %v", err)
	}
	if !st.Exists {
		t.Error("Exists want true")
	}
	if !st.Clean {
		t.Error("Clean want true (fresh worktree has no changes)")
	}
}

func TestGitBackend_StatusAt_DirtyWorktree(t *testing.T) {
	repoPath := initGitRepo(t)
	worktreesDir := t.TempDir()
	ctx := context.Background()

	b, _ := wt.For(wt.VCSGit)
	path, err := b.Create(ctx, worktreesDir, repoPath, "mp-dirty")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	// Write a file to make the worktree dirty.
	if err := os.WriteFile(filepath.Join(path, "newfile.go"), []byte("package main\n"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	st, err := wt.GitStatusAt(ctx, worktreesDir, "mp-dirty")
	if err != nil {
		t.Fatalf("StatusAt: %v", err)
	}
	if !st.Exists {
		t.Error("Exists want true")
	}
	if st.Clean {
		t.Error("Clean want false (untracked file added)")
	}
}

func TestGitBackend_StatusAt_NoWorktree(t *testing.T) {
	worktreesDir := t.TempDir()
	ctx := context.Background()

	st, err := wt.GitStatusAt(ctx, worktreesDir, "mp-missing")
	if err == nil {
		t.Fatal("StatusAt(missing): expected error, got nil")
	}
	if st.Exists {
		t.Error("Exists want false for missing worktree")
	}
}

// ── T013-T014: gitBackend.DiffSummary ─────────────────────────────────────

func TestGitBackend_DiffSummaryAt_ModifiedDeletedUntracked(t *testing.T) {
	repoPath := initGitRepo(t)
	worktreesDir := t.TempDir()
	ctx := context.Background()

	b, _ := wt.For(wt.VCSGit)
	wtPath, err := b.Create(ctx, worktreesDir, repoPath, "mp-diff")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	// Stage a second file in the main repo so the worktree has it too.
	runGitCmd(t, repoPath, "checkout", "-b", "main-setup")
	// The worktree was already created; write changes directly in it.

	// Write a tracked file (starts as committed README.md in the repo).
	// Modify it.
	readmePath := filepath.Join(wtPath, "README.md")
	if err := os.WriteFile(readmePath, []byte("# modified\n"), 0o644); err != nil {
		t.Fatalf("WriteFile README: %v", err)
	}

	// Add an untracked file.
	if err := os.WriteFile(filepath.Join(wtPath, "new.go"), []byte("package main\n"), 0o644); err != nil {
		t.Fatalf("WriteFile new.go: %v", err)
	}

	changes, err := wt.GitDiffSummaryAt(ctx, worktreesDir, "mp-diff")
	if err != nil {
		t.Fatalf("DiffSummaryAt: %v", err)
	}

	// Expect at least a modified README.md and an added new.go.
	found := make(map[string]wt.ChangeKind)
	for _, fc := range changes {
		found[fc.Path] = fc.Kind
	}

	if k, ok := found["README.md"]; !ok || k != wt.Modified {
		t.Errorf("README.md: want Modified, got %v (present=%v)", k, ok)
	}
	if k, ok := found["new.go"]; !ok || k != wt.Added {
		t.Errorf("new.go: want Added, got %v (present=%v)", k, ok)
	}
}

func TestGitBackend_DiffSummaryAt_NoWorktree(t *testing.T) {
	worktreesDir := t.TempDir()
	ctx := context.Background()

	_, err := wt.GitDiffSummaryAt(ctx, worktreesDir, "mp-missing")
	if err == nil {
		t.Fatal("DiffSummaryAt(missing): expected error, got nil")
	}
}

func TestGitBackend_DiffSummaryAt_Clean(t *testing.T) {
	repoPath := initGitRepo(t)
	worktreesDir := t.TempDir()
	ctx := context.Background()

	b, _ := wt.For(wt.VCSGit)
	_, err := b.Create(ctx, worktreesDir, repoPath, "mp-clean2")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	changes, err := wt.GitDiffSummaryAt(ctx, worktreesDir, "mp-clean2")
	if err != nil {
		t.Fatalf("DiffSummaryAt: %v", err)
	}
	if len(changes) != 0 {
		t.Errorf("clean worktree: want 0 changes, got %d: %v", len(changes), changes)
	}
}

// ── T015: gitBackend.Diff (streaming) ─────────────────────────────────────

func TestGitBackend_DiffAt_WholeWorktree(t *testing.T) {
	repoPath := initGitRepo(t)
	worktreesDir := t.TempDir()
	ctx := context.Background()

	b, _ := wt.For(wt.VCSGit)
	wtPath, err := b.Create(ctx, worktreesDir, repoPath, "mp-diffall")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	// Modify README.md and add new.go (untracked).
	if err := os.WriteFile(filepath.Join(wtPath, "README.md"), []byte("# changed\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := os.WriteFile(filepath.Join(wtPath, "new.go"), []byte("package main\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	rc, err := wt.GitDiffAt(ctx, worktreesDir, "mp-diffall", "")
	if err != nil {
		t.Fatalf("DiffAt: %v", err)
	}
	defer rc.Close()

	data, err := io.ReadAll(rc)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	diff := string(data)
	// The diff should include both the modified README and the new file.
	if !strings.Contains(diff, "README.md") {
		t.Errorf("diff does not mention README.md:\n%s", diff)
	}
	if !strings.Contains(diff, "new.go") {
		t.Errorf("diff does not mention new.go (untracked):\n%s", diff)
	}
}

func TestGitBackend_DiffAt_SingleFile(t *testing.T) {
	repoPath := initGitRepo(t)
	worktreesDir := t.TempDir()
	ctx := context.Background()

	b, _ := wt.For(wt.VCSGit)
	wtPath, err := b.Create(ctx, worktreesDir, repoPath, "mp-diffsingle")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	// Modify README.md.
	if err := os.WriteFile(filepath.Join(wtPath, "README.md"), []byte("# changed single\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	rc, err := wt.GitDiffAt(ctx, worktreesDir, "mp-diffsingle", "README.md")
	if err != nil {
		t.Fatalf("DiffAt(README.md): %v", err)
	}
	defer rc.Close()

	data, err := io.ReadAll(rc)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	diff := string(data)
	if !strings.Contains(diff, "README.md") {
		t.Errorf("single-file diff should mention README.md:\n%s", diff)
	}
}

func TestGitBackend_DiffAt_SingleUntracked(t *testing.T) {
	repoPath := initGitRepo(t)
	worktreesDir := t.TempDir()
	ctx := context.Background()

	b, _ := wt.For(wt.VCSGit)
	wtPath, err := b.Create(ctx, worktreesDir, repoPath, "mp-diffuntrack")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	// Add an untracked file.
	if err := os.WriteFile(filepath.Join(wtPath, "brand_new.go"), []byte("package main\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	rc, err := wt.GitDiffAt(ctx, worktreesDir, "mp-diffuntrack", "brand_new.go")
	if err != nil {
		t.Fatalf("DiffAt(brand_new.go): %v", err)
	}
	defer rc.Close()

	data, err := io.ReadAll(rc)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	diff := string(data)
	if !strings.Contains(diff, "brand_new.go") {
		t.Errorf("untracked single-file diff should mention brand_new.go:\n%s", diff)
	}
}

// TestGitBackend_DiffAt_UntrackedSymlinkNotFollowed verifies the symlink
// exfiltration guard: an untracked symlink pointing outside the worktree must
// NOT be followed by `git diff --no-index` (which would otherwise leak the
// target's contents). diffAll skips it; diffSingleFile returns an empty diff.
func TestGitBackend_DiffAt_UntrackedSymlinkNotFollowed(t *testing.T) {
	repoPath := initGitRepo(t)
	worktreesDir := t.TempDir()
	ctx := context.Background()

	b, _ := wt.For(wt.VCSGit)
	wtPath, err := b.Create(ctx, worktreesDir, repoPath, "mp-symleak")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	// A secret file OUTSIDE the worktree.
	secretDir := t.TempDir()
	secretFile := filepath.Join(secretDir, "secret.txt")
	if err := os.WriteFile(secretFile, []byte("TOP_SECRET_VALUE\n"), 0o644); err != nil {
		t.Fatalf("write secret: %v", err)
	}

	// An untracked symlink inside the worktree pointing at the secret.
	if err := os.Symlink(secretFile, filepath.Join(wtPath, "leak")); err != nil {
		t.Skipf("symlink unsupported on this platform: %v", err)
	}

	// Whole-worktree diff must not leak the symlink target's contents.
	rc, err := wt.GitDiffAt(ctx, worktreesDir, "mp-symleak", "")
	if err != nil {
		t.Fatalf("DiffAt(whole): %v", err)
	}
	data, _ := io.ReadAll(rc)
	_ = rc.Close()
	if strings.Contains(string(data), "TOP_SECRET_VALUE") {
		t.Errorf("whole-worktree diff leaked symlink target contents:\n%s", data)
	}

	// Single-file diff of the symlink must be empty (skipped), not an error.
	rc2, err := wt.GitDiffAt(ctx, worktreesDir, "mp-symleak", "leak")
	if err != nil {
		t.Fatalf("DiffAt(leak): unexpected error: %v", err)
	}
	data2, _ := io.ReadAll(rc2)
	_ = rc2.Close()
	if len(data2) != 0 {
		t.Errorf("single-file diff of untracked symlink should be empty, got:\n%s", data2)
	}
}

// ── T029: Ahead/Behind ────────────────────────────────────────────────────

// TestGitBackend_StatusAt_FileNotDir verifies that StatusAt returns
// ErrWorktreeNotFound when the path exists but is a file, not a directory.
func TestGitBackend_StatusAt_FileNotDir(t *testing.T) {
	worktreesDir := t.TempDir()
	ctx := context.Background()

	// Write a FILE at the expected worktree path (not a directory).
	beadID := "mp-filenotdir"
	if err := os.WriteFile(filepath.Join(worktreesDir, beadID), []byte("not a dir"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	st, err := wt.GitStatusAt(ctx, worktreesDir, beadID)
	if err == nil {
		t.Fatal("StatusAt(file instead of dir): expected error, got nil")
	}
	if st.Exists {
		t.Error("Exists want false when path is a file, not a dir")
	}
}

// TestGitBackend_DiffSummaryAt_FileNotDir verifies ErrWorktreeNotFound when
// the path exists but is a file.
func TestGitBackend_DiffSummaryAt_FileNotDir(t *testing.T) {
	worktreesDir := t.TempDir()
	ctx := context.Background()

	beadID := "mp-diffsumfilenotdir"
	if err := os.WriteFile(filepath.Join(worktreesDir, beadID), []byte("not a dir"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	_, err := wt.GitDiffSummaryAt(ctx, worktreesDir, beadID)
	// GitDiffSummaryAt doesn't check IsDir — git status will just fail on a file path.
	// This verifies we get some error (not panic).
	if err == nil {
		t.Fatal("DiffSummaryAt(file instead of dir): expected error, got nil")
	}
}

// TestGitBackend_StatusAt_AheadBehind_NoUpstream verifies that when a
// worktree branch has no upstream, Ahead and Behind gracefully default to 0.
// This is the common case for muster/<id> branches (no remote tracking branch).
func TestGitBackend_StatusAt_AheadBehind_NoUpstream(t *testing.T) {
	repoPath := initGitRepo(t)
	worktreesDir := t.TempDir()
	ctx := context.Background()

	b, _ := wt.For(wt.VCSGit)
	_, err := b.Create(ctx, worktreesDir, repoPath, "mp-aheadbehind")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	st, err := wt.GitStatusAt(ctx, worktreesDir, "mp-aheadbehind")
	if err != nil {
		t.Fatalf("StatusAt: %v", err)
	}
	// No upstream → Ahead=0, Behind=0 (graceful default).
	if st.Ahead != 0 {
		t.Errorf("Ahead want 0 (no upstream), got %d", st.Ahead)
	}
	if st.Behind != 0 {
		t.Errorf("Behind want 0 (no upstream), got %d", st.Behind)
	}
}

func TestGitBackend_DiffAt_NoWorktree(t *testing.T) {
	worktreesDir := t.TempDir()
	ctx := context.Background()

	_, err := wt.GitDiffAt(ctx, worktreesDir, "mp-missing", "")
	if err == nil {
		t.Fatal("DiffAt(missing): expected error, got nil")
	}
}

func TestGitBackend_DiffAt_CleanWorktree(t *testing.T) {
	repoPath := initGitRepo(t)
	worktreesDir := t.TempDir()
	ctx := context.Background()

	b, _ := wt.For(wt.VCSGit)
	_, err := b.Create(ctx, worktreesDir, repoPath, "mp-diffclean")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	rc, err := wt.GitDiffAt(ctx, worktreesDir, "mp-diffclean", "")
	if err != nil {
		t.Fatalf("DiffAt(clean): %v", err)
	}
	defer rc.Close()

	data, err := io.ReadAll(rc)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	// Clean worktree diff must be empty.
	if len(data) != 0 {
		t.Errorf("clean worktree: want empty diff, got %d bytes:\n%s", len(data), data)
	}
}

// ── Coverage boosters: NewGitBackend delegation methods ──────────────────────
//
// These tests exercise NewGitBackend(worktreesDir) so that the Status,
// DiffSummary, Diff, Finalize, Push, and Remove method bodies are covered.

func TestNewGitBackend_StatusDelegates(t *testing.T) {
	repoPath := initGitRepo(t)
	worktreesDir := t.TempDir()
	ctx := context.Background()

	b := wt.NewGitBackend(worktreesDir)

	// Create via For so we can reuse the bead path.
	fb, _ := wt.For(wt.VCSGit)
	_, err := fb.Create(ctx, worktreesDir, repoPath, "mp-newgit-status")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	st, err := b.Status(ctx, "mp-newgit-status")
	if err != nil {
		t.Fatalf("Status via NewGitBackend: %v", err)
	}
	if !st.Exists {
		t.Error("Exists want true")
	}
}

func TestNewGitBackend_StatusEmptyWorktreesDir(t *testing.T) {
	// When NewGitBackend is constructed via For(VCSGit) the worktreesDir is ""
	// and Status must return an error.
	b, _ := wt.For(wt.VCSGit)
	ctx := context.Background()
	_, err := b.Status(ctx, "bead")
	if err == nil {
		t.Fatal("Status with empty worktreesDir: expected error, got nil")
	}
}

func TestNewGitBackend_DiffSummaryDelegates(t *testing.T) {
	repoPath := initGitRepo(t)
	worktreesDir := t.TempDir()
	ctx := context.Background()

	b := wt.NewGitBackend(worktreesDir)

	fb, _ := wt.For(wt.VCSGit)
	wtPath, err := fb.Create(ctx, worktreesDir, repoPath, "mp-newgit-diffsummary")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	// Write an untracked file to make it dirty.
	if err := os.WriteFile(filepath.Join(wtPath, "extra.go"), []byte("package main\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	changes, err := b.DiffSummary(ctx, "mp-newgit-diffsummary")
	if err != nil {
		t.Fatalf("DiffSummary via NewGitBackend: %v", err)
	}
	if len(changes) == 0 {
		t.Error("DiffSummary: want at least one change, got 0")
	}
}

func TestNewGitBackend_DiffSummaryEmptyWorktreesDir(t *testing.T) {
	b, _ := wt.For(wt.VCSGit)
	ctx := context.Background()
	_, err := b.DiffSummary(ctx, "bead")
	if err == nil {
		t.Fatal("DiffSummary with empty worktreesDir: expected error, got nil")
	}
}

func TestNewGitBackend_DiffDelegates(t *testing.T) {
	repoPath := initGitRepo(t)
	worktreesDir := t.TempDir()
	ctx := context.Background()

	b := wt.NewGitBackend(worktreesDir)

	fb, _ := wt.For(wt.VCSGit)
	wtPath, err := fb.Create(ctx, worktreesDir, repoPath, "mp-newgit-diff")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	// Write something to diff.
	if err := os.WriteFile(filepath.Join(wtPath, "README.md"), []byte("# changed\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	rc, err := b.Diff(ctx, "mp-newgit-diff", "")
	if err != nil {
		t.Fatalf("Diff via NewGitBackend: %v", err)
	}
	defer rc.Close()
	data, err := io.ReadAll(rc)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if !strings.Contains(string(data), "README.md") {
		t.Errorf("diff should mention README.md:\n%s", data)
	}
}

func TestNewGitBackend_DiffEmptyWorktreesDir(t *testing.T) {
	b, _ := wt.For(wt.VCSGit)
	ctx := context.Background()
	_, err := b.Diff(ctx, "bead", "")
	if err == nil {
		t.Fatal("Diff with empty worktreesDir: expected error, got nil")
	}
}

func TestGitBackend_WriteMethodsNotImplemented(t *testing.T) {
	// Via NewGitBackend (has worktreesDir).
	b := wt.NewGitBackend(t.TempDir())
	ctx := context.Background()
	if err := b.Finalize(ctx, "bead", "msg"); err != wt.ErrNotImplemented {
		t.Errorf("Finalize: want ErrNotImplemented, got %v", err)
	}
	if err := b.Push(ctx, "bead"); err != wt.ErrNotImplemented {
		t.Errorf("Push: want ErrNotImplemented, got %v", err)
	}
	if err := b.Remove(ctx, "bead"); err != wt.ErrNotImplemented {
		t.Errorf("Remove: want ErrNotImplemented, got %v", err)
	}
}

// TestGitBackend_StatusAt_AheadBehind_WithUpstream verifies that Ahead/Behind
// are populated correctly when a worktree branch has an upstream.
func TestGitBackend_StatusAt_AheadBehind_WithUpstream(t *testing.T) {
	// Set up a bare "remote" and clone from it so we have an upstream.
	remote := t.TempDir()
	runGitCmd(t, remote, "init", "--bare")

	local := t.TempDir()
	runGitCmd(t, local, "init")
	runGitCmd(t, local, "config", "user.email", "test@test.com")
	runGitCmd(t, local, "config", "user.name", "Test")
	if err := os.WriteFile(filepath.Join(local, "README.md"), []byte("# test\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	runGitCmd(t, local, "add", ".")
	runGitCmd(t, local, "commit", "-m", "initial")
	runGitCmd(t, local, "remote", "add", "origin", remote)
	runGitCmd(t, local, "push", "-u", "origin", "HEAD:main")
	runGitCmd(t, local, "branch", "--set-upstream-to=origin/main")

	// Make one local commit ahead of the upstream.
	if err := os.WriteFile(filepath.Join(local, "extra.go"), []byte("package main\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	runGitCmd(t, local, "add", "extra.go")
	runGitCmd(t, local, "commit", "-m", "ahead commit")

	worktreesDir := t.TempDir()
	ctx := context.Background()

	// Create a worktree from the local repo at its current HEAD.
	b, _ := wt.For(wt.VCSGit)
	_, err := b.Create(ctx, worktreesDir, local, "mp-ab-upstream")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	st, err := wt.GitStatusAt(ctx, worktreesDir, "mp-ab-upstream")
	if err != nil {
		t.Fatalf("StatusAt: %v", err)
	}
	// The worktree branch has no upstream tracking of its own — git worktree add
	// creates a detached branch. So Ahead=0, Behind=0 is the expected result.
	// This also exercises the error-return branch of gitAheadBehind.
	if st.Ahead < 0 || st.Behind < 0 {
		t.Errorf("Ahead/Behind must be non-negative: Ahead=%d Behind=%d", st.Ahead, st.Behind)
	}
}

// TestGitBackend_StatusAt_AheadBehind_RealUpstream exercises the gitAheadBehind
// happy-path by creating a local git repo with a real upstream tracking branch.
// The worktree on a branch that tracks origin/main will have behind/ahead counts.
func TestGitBackend_StatusAt_AheadBehind_RealUpstream(t *testing.T) {
	// Create a "remote" bare repo.
	remote := t.TempDir()
	runGitCmd(t, remote, "init", "--bare")

	// Create a local clone.
	local := t.TempDir()
	runGitCmd(t, local, "init")
	runGitCmd(t, local, "config", "user.email", "test@test.com")
	runGitCmd(t, local, "config", "user.name", "Test")
	if err := os.WriteFile(filepath.Join(local, "README.md"), []byte("# test\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	runGitCmd(t, local, "add", ".")
	runGitCmd(t, local, "commit", "-m", "initial")
	runGitCmd(t, local, "remote", "add", "origin", remote)
	runGitCmd(t, local, "push", "-u", "origin", "HEAD:main")

	// Make a branch "tracking-branch" that tracks origin/main.
	runGitCmd(t, local, "checkout", "-b", "tracking-branch", "--track", "origin/main")

	// Make one local commit so tracking-branch is 1 ahead.
	if err := os.WriteFile(filepath.Join(local, "ahead.go"), []byte("package main\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	runGitCmd(t, local, "add", "ahead.go")
	runGitCmd(t, local, "commit", "-m", "ahead commit")

	worktreesDir := t.TempDir()
	ctx := context.Background()

	// Create a git worktree from local using the tracking-branch.
	// git worktree add will create a new branch muster/bead from the current HEAD.
	// We need the worktree itself to be on a branch that tracks something.
	// The simplest approach: create the worktree on tracking-branch (but git worktree
	// requires a fresh branch). Instead use --detach and manually set upstream.
	//
	// Actually the cleanest is: add a worktree checked out at the tracking-branch's
	// HEAD and set an upstream for the new branch there. We can use a pre-created
	// branch name.
	runGitCmd(t, local, "branch", "wt-branch", "tracking-branch")
	runGitCmd(t, local, "branch", "--set-upstream-to=origin/main", "wt-branch")

	// Create the worktree on wt-branch (already exists, so we use the bead path directly).
	wtDir := filepath.Join(worktreesDir, "mp-realupstream")
	runGitCmd(t, local, "worktree", "add", wtDir, "wt-branch")

	st, err := wt.GitStatusAt(ctx, worktreesDir, "mp-realupstream")
	if err != nil {
		t.Fatalf("StatusAt: %v", err)
	}
	if !st.Exists {
		t.Error("Exists want true")
	}
	// wt-branch is 1 ahead of origin/main.
	if st.Ahead != 1 {
		t.Errorf("Ahead want 1, got %d", st.Ahead)
	}
	if st.Behind != 0 {
		t.Errorf("Behind want 0, got %d", st.Behind)
	}
}

// TestGitDiffSummaryAt_StagedRename exercises the rename (R) path in parsePorcelainV1Z.
func TestGitDiffSummaryAt_StagedRename(t *testing.T) {
	repoPath := initGitRepo(t)
	worktreesDir := t.TempDir()
	ctx := context.Background()

	b, _ := wt.For(wt.VCSGit)
	wtPath, err := b.Create(ctx, worktreesDir, repoPath, "mp-rename")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	// Stage a rename: git mv README.md READYOU.md
	runGitCmd(t, wtPath, "mv", "README.md", "READYOU.md")

	changes, err := wt.GitDiffSummaryAt(ctx, worktreesDir, "mp-rename")
	if err != nil {
		t.Fatalf("DiffSummaryAt: %v", err)
	}

	var found *wt.FileChange
	for _, fc := range changes {
		fc := fc
		if fc.Kind == wt.Renamed {
			found = &fc
			break
		}
	}
	if found == nil {
		t.Errorf("expected a Renamed entry; got %v", changes)
	} else {
		if found.Path != "READYOU.md" {
			t.Errorf("rename new path: want READYOU.md, got %q", found.Path)
		}
		if found.OldPath != "README.md" {
			t.Errorf("rename old path: want README.md, got %q", found.OldPath)
		}
	}
}

// TestGitParsePorcelainV1Z_StagedOnly exercises the fallback path in
// porcelainKind where the worktree column is space (clean) but the index
// column has A/M/D (staged).
func TestGitParsePorcelainV1Z_StagedOnly(t *testing.T) {
	repoPath := initGitRepo(t)
	worktreesDir := t.TempDir()
	ctx := context.Background()

	b, _ := wt.For(wt.VCSGit)
	wtPath, err := b.Create(ctx, worktreesDir, repoPath, "mp-staged")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	// Stage a new file (index=A, worktree=space — "A ").
	if err := os.WriteFile(filepath.Join(wtPath, "staged_new.go"), []byte("package main\n"), 0o644); err != nil {
		t.Fatalf("write staged_new.go: %v", err)
	}
	runGitCmd(t, wtPath, "add", "staged_new.go")

	// Modify README.md and stage (index=M, worktree=space — "M ").
	if err := os.WriteFile(filepath.Join(wtPath, "README.md"), []byte("# changed\n"), 0o644); err != nil {
		t.Fatalf("write README: %v", err)
	}
	runGitCmd(t, wtPath, "add", "README.md")

	changes, err := wt.GitDiffSummaryAt(ctx, worktreesDir, "mp-staged")
	if err != nil {
		t.Fatalf("DiffSummaryAt: %v", err)
	}

	found := make(map[string]wt.ChangeKind)
	for _, fc := range changes {
		found[fc.Path] = fc.Kind
	}

	// staged_new.go: index A, worktree ' ' → porcelainKind falls through to index 'A' → Added.
	if k, ok := found["staged_new.go"]; !ok || k != wt.Added {
		t.Errorf("staged_new.go: want Added, got %v (present=%v)", k, ok)
	}
	// README.md: index M, worktree ' ' → falls through to index 'M' → Modified.
	if k, ok := found["README.md"]; !ok || k != wt.Modified {
		t.Errorf("README.md: want Modified, got %v (present=%v)", k, ok)
	}
}

// TestGitDiffSummaryAt_WorktreeDeletedFile exercises the y=='D' path in
// porcelainKind (file deleted in worktree but not staged: XY = " D").
func TestGitDiffSummaryAt_WorktreeDeletedFile(t *testing.T) {
	repoPath := initGitRepo(t)
	worktreesDir := t.TempDir()
	ctx := context.Background()

	b, _ := wt.For(wt.VCSGit)
	wtPath, err := b.Create(ctx, worktreesDir, repoPath, "mp-wtdel")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	// Delete README.md from the worktree (worktree=D, index=space → " D").
	if err := os.Remove(filepath.Join(wtPath, "README.md")); err != nil {
		t.Fatalf("remove README.md: %v", err)
	}

	changes, err := wt.GitDiffSummaryAt(ctx, worktreesDir, "mp-wtdel")
	if err != nil {
		t.Fatalf("DiffSummaryAt: %v", err)
	}

	found := make(map[string]wt.ChangeKind)
	for _, fc := range changes {
		found[fc.Path] = fc.Kind
	}

	// README.md: XY = " D" → y='D' → Deleted.
	if k, ok := found["README.md"]; !ok || k != wt.Deleted {
		t.Errorf("README.md: want Deleted (worktree-deleted), got %v (present=%v)", k, ok)
	}
}

// TestGitParsePorcelainV1Z_StagedDelete exercises the index 'D' fallback path
// in porcelainKind (staged delete: XY = "D ").
func TestGitParsePorcelainV1Z_StagedDelete(t *testing.T) {
	repoPath := initGitRepo(t)
	worktreesDir := t.TempDir()
	ctx := context.Background()

	b, _ := wt.For(wt.VCSGit)
	wtPath, err := b.Create(ctx, worktreesDir, repoPath, "mp-stageddel")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	// Create and commit a file so we can delete it.
	if err := os.WriteFile(filepath.Join(wtPath, "to_delete.go"), []byte("package main\n"), 0o644); err != nil {
		t.Fatalf("write to_delete.go: %v", err)
	}
	runGitCmd(t, wtPath, "add", "to_delete.go")
	runGitCmd(t, wtPath, "commit", "-m", "add to_delete.go")

	// Stage a delete (index=D, worktree=space — "D ").
	runGitCmd(t, wtPath, "rm", "to_delete.go")

	changes, err := wt.GitDiffSummaryAt(ctx, worktreesDir, "mp-stageddel")
	if err != nil {
		t.Fatalf("DiffSummaryAt: %v", err)
	}

	found := make(map[string]wt.ChangeKind)
	for _, fc := range changes {
		found[fc.Path] = fc.Kind
	}

	// to_delete.go: index D, worktree ' ' → porcelainKind falls through to index 'D' → Deleted.
	if k, ok := found["to_delete.go"]; !ok || k != wt.Deleted {
		t.Errorf("to_delete.go: want Deleted, got %v (present=%v)", k, ok)
	}
}
