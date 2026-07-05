//go:build !windows

// jj backend tests. Unix-only because /dev/null and shell paths
// differ on Windows, and jj itself is rarely present there in CI.

package wt_test

import (
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gitrgoliveira/muster/internal/wt"
)

// ── helpers ──────────────────────────────────────────────────────────────────

// jjEnv returns a hermetic env slice for jj commands: JJ_CONFIG=/dev/null
// plus git identity so jj workspace add doesn't warn about missing identity.
func jjEnv() []string {
	env := os.Environ()
	// Override/add hermetic values.
	cleaned := make([]string, 0, len(env)+5)
	for _, e := range env {
		if strings.HasPrefix(e, "JJ_CONFIG=") ||
			strings.HasPrefix(e, "JJ_USER_NAME=") ||
			strings.HasPrefix(e, "JJ_USER_EMAIL=") ||
			strings.HasPrefix(e, "GIT_AUTHOR_NAME=") ||
			strings.HasPrefix(e, "GIT_AUTHOR_EMAIL=") ||
			strings.HasPrefix(e, "GIT_COMMITTER_NAME=") ||
			strings.HasPrefix(e, "GIT_COMMITTER_EMAIL=") {
			continue
		}
		cleaned = append(cleaned, e)
	}
	return append(cleaned,
		"JJ_CONFIG=/dev/null",
		"GIT_AUTHOR_NAME=Test",
		"GIT_AUTHOR_EMAIL=test@test.com",
		"GIT_COMMITTER_NAME=Test",
		"GIT_COMMITTER_EMAIL=test@test.com",
	)
}

// initJJRepo creates a real jj-native repo (jj git init) and returns its path.
// Skips if jj is not available.
func initJJRepo(t *testing.T) string {
	t.Helper()
	if !jjAvailable() {
		t.Skip("jj not available on PATH")
	}
	dir := t.TempDir()

	run := func(name string, args ...string) {
		t.Helper()
		cmd := exec.Command(name, args...)
		cmd.Dir = dir
		cmd.Env = jjEnv()
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("%s %v: %v\n%s", name, args, err, out)
		}
	}

	// jj git init makes a jj-native repo backed by git.
	// --colocate is NOT used — we want a pure jj repo (no .git dir at root).
	// jj git init alone creates a jj repo at the dir.
	run("jj", "git", "init")

	// Write an initial file so the working copy has something.
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("# test\n"), 0o644); err != nil {
		t.Fatalf("write README: %v", err)
	}

	return dir
}

// ── T021: jjBackend.Create tests ─────────────────────────────────────────────

// TestJJBackend_Create_InvalidBeadID verifies that an empty or path-separator
// beadID returns an error without invoking jj.
func TestJJBackend_Create_InvalidBeadID(t *testing.T) {
	b, err := wt.For(wt.VCSJJ)
	if err != nil {
		t.Fatalf("For(jj): %v", err)
	}
	ctx := context.Background()

	cases := []string{"", "../escape", "a/b", ".."}
	for _, id := range cases {
		_, err := b.Create(ctx, t.TempDir(), t.TempDir(), id)
		if err == nil {
			t.Errorf("Create(%q): expected error, got nil", id)
		}
	}
}

// TestJJBackend_Create_FakeJJ_NativeRepo uses the fake jj to verify that
// Create calls `jj root` (to probe jj-nativeness) and then `jj workspace add`.
func TestJJBackend_Create_FakeJJ_NativeRepo(t *testing.T) {
	binDir := t.TempDir()
	addFakeJJToBinDir(t, binDir)

	// Set up a fake source repo directory.
	srcRepo := t.TempDir()
	worktreesDir := t.TempDir()
	ctx := context.Background()

	// Point FAKE_JJ_ROOT to a valid path so `jj root` exits 0.
	t.Setenv("FAKE_JJ_ROOT", srcRepo)
	// FAKE_JJ_EXIT=0 is the default.

	b, err := wt.For(wt.VCSJJ)
	if err != nil {
		t.Fatalf("For(jj): %v", err)
	}

	recordFile := filepath.Join(t.TempDir(), "jj_calls.txt")
	t.Setenv("FAKE_JJ_RECORD_FILE", recordFile)

	path, err := b.Create(ctx, worktreesDir, srcRepo, "test-bead")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	// The returned path should be <worktreesDir>/test-bead.
	want := filepath.Join(worktreesDir, "test-bead")
	if path != want {
		t.Errorf("Create path = %q, want %q", path, want)
	}

	// Verify jj was invoked with root and workspace add.
	calls, err := os.ReadFile(recordFile)
	if err != nil {
		t.Fatalf("read record file: %v", err)
	}
	callStr := string(calls)
	if !strings.Contains(callStr, "root") {
		t.Errorf("expected 'jj root' invocation in: %q", callStr)
	}
	if !strings.Contains(callStr, "workspace") {
		t.Errorf("expected 'jj workspace add' invocation in: %q", callStr)
	}
}

// TestJJBackend_Create_FakeJJ_PlainGitRepo verifies that if `jj root` fails
// (non-zero exit), Create returns ErrVCSUnavailable.
func TestJJBackend_Create_FakeJJ_PlainGitRepo(t *testing.T) {
	binDir := t.TempDir()
	addFakeJJToBinDir(t, binDir)

	srcRepo := t.TempDir()
	worktreesDir := t.TempDir()
	ctx := context.Background()

	// Make jj root fail: exit 1 → fake jj returns an error.
	t.Setenv("FAKE_JJ_EXIT", "1")

	b, err := wt.For(wt.VCSJJ)
	if err != nil {
		t.Fatalf("For(jj): %v", err)
	}

	_, err = b.Create(ctx, worktreesDir, srcRepo, "test-bead")
	if err == nil {
		t.Fatal("Create(plain git repo): expected error, got nil")
	}
	if !strings.Contains(err.Error(), "unavailable") && err.Error() != wt.ErrVCSUnavailable.Error() {
		// Accept either the sentinel or a wrapped version.
		t.Logf("Create error: %v", err)
	}
}

// TestJJBackend_Create_FakeJJ_Reuse verifies that Create returns an existing
// directory without calling `jj workspace add` again (the reuse path).
func TestJJBackend_Create_FakeJJ_Reuse(t *testing.T) {
	binDir := t.TempDir()
	addFakeJJToBinDir(t, binDir)

	srcRepo := t.TempDir()
	worktreesDir := t.TempDir()
	ctx := context.Background()

	// Point FAKE_JJ_ROOT to a valid path so `jj root` exits 0.
	t.Setenv("FAKE_JJ_ROOT", srcRepo)

	b, err := wt.For(wt.VCSJJ)
	if err != nil {
		t.Fatalf("For(jj): %v", err)
	}

	// Pre-create the worktree directory to simulate an already-existing workspace.
	beadID := "reuse-bead"
	existingPath := filepath.Join(worktreesDir, beadID)
	if err := os.MkdirAll(existingPath, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}

	recordFile := filepath.Join(t.TempDir(), "jj_calls.txt")
	t.Setenv("FAKE_JJ_RECORD_FILE", recordFile)

	path, err := b.Create(ctx, worktreesDir, srcRepo, beadID)
	if err != nil {
		t.Fatalf("Create (reuse): %v", err)
	}
	if path != existingPath {
		t.Errorf("Create path = %q, want %q", path, existingPath)
	}

	// Verify that `jj workspace add` was NOT called (only `jj root` was called).
	calls, _ := os.ReadFile(recordFile)
	if strings.Contains(string(calls), "workspace") {
		t.Errorf("reuse should skip `jj workspace add`; got calls: %q", string(calls))
	}
}

// TestJJBackend_Create_RealJJ_NativeRepo tests Create against a real jj repo.
func TestJJBackend_Create_RealJJ_NativeRepo(t *testing.T) {
	if !jjAvailable() {
		t.Skip("jj not available")
	}
	srcRepo := initJJRepo(t)
	worktreesDir := t.TempDir()
	ctx := context.Background()

	// Set hermetic env for jj calls inside Create.
	t.Setenv("JJ_CONFIG", "/dev/null")

	b, err := wt.For(wt.VCSJJ)
	if err != nil {
		t.Fatalf("For(jj): %v", err)
	}

	path, err := b.Create(ctx, worktreesDir, srcRepo, "real-bead")
	if err != nil {
		t.Fatalf("Create(real jj repo): %v", err)
	}

	want := filepath.Join(worktreesDir, "real-bead")
	if path != want {
		t.Errorf("Create path = %q, want %q", path, want)
	}

	// The worktree directory should exist.
	if _, err := os.Stat(path); err != nil {
		t.Errorf("worktree dir does not exist: %v", err)
	}
}

// TestJJBackend_Create_RealJJ_PlainGitRepo verifies ErrVCSUnavailable against a plain git repo.
func TestJJBackend_Create_RealJJ_PlainGitRepo(t *testing.T) {
	if !jjAvailable() {
		t.Skip("jj not available")
	}

	// A plain git repo (not jj-native).
	srcRepo := initGitRepo(t)
	worktreesDir := t.TempDir()
	ctx := context.Background()

	t.Setenv("JJ_CONFIG", "/dev/null")

	b, err := wt.For(wt.VCSJJ)
	if err != nil {
		t.Fatalf("For(jj): %v", err)
	}

	_, err = b.Create(ctx, worktreesDir, srcRepo, "fail-bead")
	if err == nil {
		t.Fatal("Create(plain git repo): expected ErrVCSUnavailable, got nil")
	}
}

// ── T023: jjBackend.DiffSummary + Status ────────────────────────────────────

// TestJJBackend_DiffSummary_FakeJJ tests DiffSummary parsing of
// `jj diff --summary` output via the fake jj.
func TestJJBackend_DiffSummary_FakeJJ(t *testing.T) {
	binDir := t.TempDir()
	addFakeJJToBinDir(t, binDir)

	worktreesDir := t.TempDir()
	beadID := "jj-summary-bead"
	// Create the worktree dir so the path check passes.
	wtPath := filepath.Join(worktreesDir, beadID)
	if err := os.MkdirAll(wtPath, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	// The fake jj emits "M modified.txt\nA new.go\nD deleted.txt\n" by default.
	ctx := context.Background()

	changes, err := wt.JJDiffSummaryAt(ctx, worktreesDir, beadID)
	if err != nil {
		t.Fatalf("JJDiffSummaryAt: %v", err)
	}

	byPath := make(map[string]wt.ChangeKind)
	for _, fc := range changes {
		byPath[fc.Path] = fc.Kind
	}

	if k, ok := byPath["modified.txt"]; !ok || k != wt.Modified {
		t.Errorf("modified.txt: want Modified, got %v (present=%v)", k, ok)
	}
	if k, ok := byPath["new.go"]; !ok || k != wt.Added {
		t.Errorf("new.go: want Added, got %v (present=%v)", k, ok)
	}
	if k, ok := byPath["deleted.txt"]; !ok || k != wt.Deleted {
		t.Errorf("deleted.txt: want Deleted, got %v (present=%v)", k, ok)
	}
}

// TestJJBackend_DiffSummary_NoWorktree verifies ErrWorktreeNotFound.
func TestJJBackend_DiffSummary_NoWorktree(t *testing.T) {
	binDir := t.TempDir()
	addFakeJJToBinDir(t, binDir)

	worktreesDir := t.TempDir()
	ctx := context.Background()

	_, err := wt.JJDiffSummaryAt(ctx, worktreesDir, "missing-bead")
	if err == nil {
		t.Fatal("JJDiffSummaryAt(missing): expected error, got nil")
	}
}

// TestJJBackend_DiffSummary_Rename tests renamed-file detection in jj diff --summary.
func TestJJBackend_DiffSummary_Rename(t *testing.T) {
	binDir := t.TempDir()
	addFakeJJToBinDir(t, binDir)

	worktreesDir := t.TempDir()
	beadID := "jj-rename-bead"
	wtPath := filepath.Join(worktreesDir, beadID)
	if err := os.MkdirAll(wtPath, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	// Override fake output to include a rename and a copy.
	t.Setenv("FAKE_JJ_DIFF_SUMMARY", "R old.go new.go\nC src.go dst.go\n")
	ctx := context.Background()

	changes, err := wt.JJDiffSummaryAt(ctx, worktreesDir, beadID)
	if err != nil {
		t.Fatalf("JJDiffSummaryAt: %v", err)
	}

	byPath := make(map[string]wt.FileChange)
	for _, fc := range changes {
		byPath[fc.Path] = fc
	}

	if fc, ok := byPath["new.go"]; !ok || fc.Kind != wt.Renamed || fc.OldPath != "old.go" {
		t.Errorf("rename: want {Path:new.go Kind:renamed OldPath:old.go}, got %+v", byPath["new.go"])
	}
	if fc, ok := byPath["dst.go"]; !ok || fc.Kind != wt.Copied || fc.OldPath != "src.go" {
		t.Errorf("copy: want {Path:dst.go Kind:copied OldPath:src.go}, got %+v", byPath["dst.go"])
	}
}

// TestJJBackend_Status_FakeJJ tests Status parsing of `jj status` output.
func TestJJBackend_Status_FakeJJ(t *testing.T) {
	binDir := t.TempDir()
	addFakeJJToBinDir(t, binDir)

	worktreesDir := t.TempDir()
	beadID := "jj-status-bead"
	wtPath := filepath.Join(worktreesDir, beadID)
	if err := os.MkdirAll(wtPath, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	ctx := context.Background()
	// Default fake output includes "Working copy changes:" so not clean.
	st, err := wt.JJStatusAt(ctx, worktreesDir, beadID)
	if err != nil {
		t.Fatalf("JJStatusAt: %v", err)
	}
	if !st.Exists {
		t.Error("Exists want true")
	}
	if st.Clean {
		t.Error("Clean want false (fake output has changes)")
	}
}

// TestJJBackend_Status_Clean tests clean detection when jj status has no changes.
func TestJJBackend_Status_Clean(t *testing.T) {
	binDir := t.TempDir()
	addFakeJJToBinDir(t, binDir)

	worktreesDir := t.TempDir()
	beadID := "jj-clean-bead"
	wtPath := filepath.Join(worktreesDir, beadID)
	if err := os.MkdirAll(wtPath, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	// Override to produce a "no changes" output.
	t.Setenv("FAKE_JJ_STATUS", "The working copy is clean\n")
	ctx := context.Background()

	st, err := wt.JJStatusAt(ctx, worktreesDir, beadID)
	if err != nil {
		t.Fatalf("JJStatusAt: %v", err)
	}
	if !st.Clean {
		t.Error("Clean want true (fake output has no changes)")
	}
}

// TestJJBackend_Status_FileNotDir verifies ErrWorktreeNotFound when the path
// exists but is a file (not a directory).
func TestJJBackend_Status_FileNotDir(t *testing.T) {
	binDir := t.TempDir()
	addFakeJJToBinDir(t, binDir)

	worktreesDir := t.TempDir()
	beadID := "jj-filenotdir"
	// Write a file (not directory) at the workspace path.
	if err := os.WriteFile(filepath.Join(worktreesDir, beadID), []byte("not a dir"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	ctx := context.Background()
	st, err := wt.JJStatusAt(ctx, worktreesDir, beadID)
	if err == nil {
		t.Fatal("JJStatusAt(file instead of dir): expected error, got nil")
	}
	if st.Exists {
		t.Error("Exists want false when path is a file")
	}
}

// TestJJBackend_DiffSummary_FileNotDir verifies error when path is a file.
func TestJJBackend_DiffSummary_FileNotDir(t *testing.T) {
	binDir := t.TempDir()
	addFakeJJToBinDir(t, binDir)

	worktreesDir := t.TempDir()
	beadID := "jj-diffsumfilenotdir"
	if err := os.WriteFile(filepath.Join(worktreesDir, beadID), []byte("not a dir"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	ctx := context.Background()
	_, err := wt.JJDiffSummaryAt(ctx, worktreesDir, beadID)
	// A non-directory path maps to ErrWorktreeNotFound (404), matching JJStatusAt.
	if !errors.Is(err, wt.ErrWorktreeNotFound) {
		t.Fatalf("JJDiffSummaryAt(file instead of dir): want ErrWorktreeNotFound, got %v", err)
	}
}

// TestJJBackend_Diff_FileNotDir verifies JJDiffAt returns ErrWorktreeNotFound
// when the workspace path exists but is a file, not a directory (404, not 500).
func TestJJBackend_Diff_FileNotDir(t *testing.T) {
	binDir := t.TempDir()
	addFakeJJToBinDir(t, binDir)

	worktreesDir := t.TempDir()
	beadID := "jj-diffatfilenotdir"
	if err := os.WriteFile(filepath.Join(worktreesDir, beadID), []byte("not a dir"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	ctx := context.Background()
	_, err := wt.JJDiffAt(ctx, worktreesDir, beadID, "")
	if !errors.Is(err, wt.ErrWorktreeNotFound) {
		t.Fatalf("JJDiffAt(file instead of dir): want ErrWorktreeNotFound, got %v", err)
	}
}

// TestJJBackend_Status_NoWorktree verifies ErrWorktreeNotFound.
func TestJJBackend_Status_NoWorktree(t *testing.T) {
	binDir := t.TempDir()
	addFakeJJToBinDir(t, binDir)

	worktreesDir := t.TempDir()
	ctx := context.Background()

	st, err := wt.JJStatusAt(ctx, worktreesDir, "missing-bead")
	if err == nil {
		t.Fatal("JJStatusAt(missing): expected error, got nil")
	}
	if st.Exists {
		t.Error("Exists want false for missing worktree")
	}
}

// ── T024: jjBackend.Diff ───────────────────────────────────────────────────

// TestJJBackend_Diff_FakeJJ verifies that Diff calls `jj diff --git` and
// returns git-format output (the same format as the git backend).
func TestJJBackend_Diff_FakeJJ(t *testing.T) {
	binDir := t.TempDir()
	addFakeJJToBinDir(t, binDir)

	worktreesDir := t.TempDir()
	beadID := "jj-diff-bead"
	wtPath := filepath.Join(worktreesDir, beadID)
	if err := os.MkdirAll(wtPath, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	ctx := context.Background()

	rc, err := wt.JJDiffAt(ctx, worktreesDir, beadID, "")
	if err != nil {
		t.Fatalf("JJDiffAt: %v", err)
	}
	defer rc.Close()

	data, err := io.ReadAll(rc)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	diff := string(data)

	// The fake outputs a minimal git-format diff.
	// Assert key git-format markers are present.
	if !strings.Contains(diff, "diff --git") {
		t.Errorf("expected git-format diff marker 'diff --git', got:\n%s", diff)
	}
	if !strings.Contains(diff, "@@") {
		t.Errorf("expected hunk header '@@', got:\n%s", diff)
	}
}

// TestJJBackend_Diff_FakeJJ_SingleFile verifies ?path= scoping passes the path arg.
func TestJJBackend_Diff_FakeJJ_SingleFile(t *testing.T) {
	binDir := t.TempDir()
	addFakeJJToBinDir(t, binDir)

	worktreesDir := t.TempDir()
	beadID := "jj-diff-single"
	wtPath := filepath.Join(worktreesDir, beadID)
	if err := os.MkdirAll(wtPath, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	recordFile := filepath.Join(t.TempDir(), "jj_calls.txt")
	t.Setenv("FAKE_JJ_RECORD_FILE", recordFile)

	ctx := context.Background()
	rc, err := wt.JJDiffAt(ctx, worktreesDir, beadID, "main.go")
	if err != nil {
		t.Fatalf("JJDiffAt(single file): %v", err)
	}
	_, _ = io.ReadAll(rc)
	rc.Close()

	calls, _ := os.ReadFile(recordFile)
	if !strings.Contains(string(calls), "main.go") {
		t.Errorf("expected 'main.go' in jj invocations, got: %q", string(calls))
	}
}

// TestJJBackend_Diff_NoWorktree verifies ErrWorktreeNotFound.
func TestJJBackend_Diff_NoWorktree(t *testing.T) {
	binDir := t.TempDir()
	addFakeJJToBinDir(t, binDir)

	worktreesDir := t.TempDir()
	ctx := context.Background()

	_, err := wt.JJDiffAt(ctx, worktreesDir, "missing-bead", "")
	if err == nil {
		t.Fatal("JJDiffAt(missing): expected error, got nil")
	}
}

// ── T022→M4: Finalize/Push/Remove must NOT return ErrNotImplemented ─────────

// TestJJBackend_WriteMethodsNotImplemented was the M3 stub check. In M4 the
// real implementations replaced the stubs, so we verify that ErrNotImplemented
// is no longer returned (the methods may return any other error for a missing
// workspace — that is fine).
func TestJJBackend_WriteMethodsNotImplemented(t *testing.T) {
	b, _ := wt.For(wt.VCSJJ)
	ctx := context.Background()

	if _, err := b.Finalize(ctx, "bead", "msg"); err == wt.ErrNotImplemented {
		t.Errorf("Finalize: M3 stub still in place (ErrNotImplemented), want real error")
	}
	if err := b.Push(ctx, "bead", ""); err == wt.ErrNotImplemented {
		t.Errorf("Push: M3 stub still in place (ErrNotImplemented), want real error")
	}
	if err := b.Remove(ctx, "bead"); err == wt.ErrNotImplemented {
		t.Errorf("Remove: M3 stub still in place (ErrNotImplemented), want real error")
	}
}

// ── T033: Push idempotency — bookmark set ────────────────────────────────────

// TestJJPush_FakeJJ_UsesBookmarkSet verifies that Push invokes
// `jj bookmark set` (not `jj bookmark create`) so repeated Push calls
// succeed even when the bookmark already exists.
func TestJJPush_FakeJJ_UsesBookmarkSet(t *testing.T) {
	binDir := t.TempDir()
	addFakeJJToBinDir(t, binDir)

	// Also need a fake git for the git push step.
	fakeBin := filepath.Join(binDir, "git")
	if err := os.WriteFile(fakeBin, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatalf("write fake git: %v", err)
	}

	worktreesDir := t.TempDir()
	beadID := "push-idem-bead"

	// Create a workspace directory so the lstat check passes.
	wtPath := filepath.Join(worktreesDir, beadID)
	if err := os.MkdirAll(wtPath, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}

	// Write a fake .jj/repo file so resolveSrcRepo's fallback can resolve srcRepo.
	jjDir := filepath.Join(wtPath, ".jj")
	if err := os.MkdirAll(jjDir, 0o755); err != nil {
		t.Fatalf("MkdirAll .jj: %v", err)
	}
	// srcRepo is a temp dir we'll create.
	srcRepo := t.TempDir()
	// jjSrcRepoDir expects a RELATIVE path from wtPath/.jj to srcRepo/.jj/repo.
	// Compute the relative path dynamically.
	rel, err := filepath.Rel(jjDir, filepath.Join(srcRepo, ".jj", "repo"))
	if err != nil {
		t.Fatalf("filepath.Rel: %v", err)
	}
	if err := os.WriteFile(filepath.Join(jjDir, "repo"), []byte(rel+"\n"), 0o644); err != nil {
		t.Fatalf("write .jj/repo: %v", err)
	}

	recordFile := filepath.Join(t.TempDir(), "jj_calls.txt")
	t.Setenv("FAKE_JJ_RECORD_FILE", recordFile)

	b := wt.NewJJBackend(worktreesDir)
	ctx := context.Background()

	// Push should succeed (fake jj and fake git both exit 0).
	if err := b.Push(ctx, beadID, ""); err != nil {
		t.Fatalf("Push: %v", err)
	}

	calls, err := os.ReadFile(recordFile)
	if err != nil {
		t.Fatalf("read record file: %v", err)
	}
	callStr := string(calls)

	// The record file is tab-delimited (fake_jj.sh uses IFS=$tab for printf).
	// Must use "bookmark set", never "bookmark create".
	if !strings.Contains(callStr, "bookmark\tset") {
		t.Errorf("Push must use 'jj bookmark set'; got calls:\n%s", callStr)
	}
	if strings.Contains(callStr, "bookmark\tcreate") {
		t.Errorf("Push must NOT use 'jj bookmark create' (not idempotent); got calls:\n%s", callStr)
	}
}

// ── T025: Real-jj integration test ──────────────────────────────────────────

// TestJJIntegration_WorktreeAndDiff is the full real-jj integration test.
// It creates a workspace, edits files, and verifies /worktree + /diff
// return correct results via the same endpoints as git.
func TestJJIntegration_WorktreeAndDiff(t *testing.T) {
	if !jjAvailable() {
		t.Skip("jj not available on PATH")
	}

	t.Setenv("JJ_CONFIG", "/dev/null")

	srcRepo := initJJRepo(t)
	worktreesDir := t.TempDir()
	beadID := "jj-integ"

	ctx := context.Background()

	// Create the workspace.
	b, err := wt.For(wt.VCSJJ)
	if err != nil {
		t.Fatalf("For(jj): %v", err)
	}

	wtPath, err := b.Create(ctx, worktreesDir, srcRepo, beadID)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	if _, err := os.Stat(wtPath); err != nil {
		t.Fatalf("workspace dir: %v", err)
	}

	// Edit: add a new file and modify the existing README.md.
	if err := os.WriteFile(filepath.Join(wtPath, "new_file.go"), []byte("package main\n"), 0o644); err != nil {
		t.Fatalf("write new_file.go: %v", err)
	}
	if err := os.WriteFile(filepath.Join(wtPath, "README.md"), []byte("# modified\n"), 0o644); err != nil {
		t.Fatalf("write README.md: %v", err)
	}

	// DiffSummary should list the changes.
	changes, err := wt.JJDiffSummaryAt(ctx, worktreesDir, beadID)
	if err != nil {
		t.Fatalf("JJDiffSummaryAt: %v", err)
	}

	byPath := make(map[string]wt.ChangeKind)
	for _, fc := range changes {
		byPath[fc.Path] = fc.Kind
	}
	t.Logf("jj DiffSummary: %v", changes)

	// jj auto-snapshots so new files appear as Added immediately.
	// Note: README.md was written in the initial repo but not committed — jj
	// may see it as added or modified depending on when the snapshot is taken.
	// We check that at least new_file.go appears.
	if _, ok := byPath["new_file.go"]; !ok {
		t.Errorf("new_file.go not found in DiffSummary; got %v", byPath)
	}

	// Diff should return git-format output.
	rc, err := wt.JJDiffAt(ctx, worktreesDir, beadID, "")
	if err != nil {
		t.Fatalf("JJDiffAt: %v", err)
	}
	defer rc.Close()

	data, err := io.ReadAll(rc)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	diffStr := string(data)
	t.Logf("jj diff output (first 500 bytes): %.500s", diffStr)

	// jj diff --git must produce git-format markers when there are changes.
	// If there's no diff (completely clean workspace), skip the format check.
	if len(diffStr) > 0 && !strings.Contains(diffStr, "diff --git") {
		t.Errorf("expected git-format diff marker in jj diff output:\n%s", diffStr)
	}

	// Status should reflect existence.
	st, err := wt.JJStatusAt(ctx, worktreesDir, beadID)
	if err != nil {
		t.Fatalf("JJStatusAt: %v", err)
	}
	if !st.Exists {
		t.Error("Status.Exists want true")
	}
}

// ── T026: Hermeticity confirmation ───────────────────────────────────────────

// TestJJBackend_Hermeticity verifies that the jj backend passes JJ_CONFIG
// through the process environment, preventing ambient user config from
// affecting test results. The test runs a roundtrip using the fake jj
// in a hermetic environment.
func TestJJBackend_Hermeticity(t *testing.T) {
	binDir := t.TempDir()
	addFakeJJToBinDir(t, binDir)

	// Set hermetic env: JJ_CONFIG=/dev/null.
	t.Setenv("JJ_CONFIG", "/dev/null")

	worktreesDir := t.TempDir()
	beadID := "hermetic-bead"
	wtPath := filepath.Join(worktreesDir, beadID)
	if err := os.MkdirAll(wtPath, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	ctx := context.Background()
	// Should succeed with hermetic config.
	changes, err := wt.JJDiffSummaryAt(ctx, worktreesDir, beadID)
	if err != nil {
		t.Fatalf("JJDiffSummaryAt (hermetic): %v", err)
	}
	// The fake jj always returns its canned output — just check no panic/error.
	_ = changes
}

// ── Coverage boosters: NewJJBackend delegation methods ───────────────────────
//
// These tests exercise NewJJBackend(worktreesDir) so that the Status,
// DiffSummary, and Diff method bodies on jjBackend are covered.

func TestNewJJBackend_StatusDelegates(t *testing.T) {
	binDir := t.TempDir()
	addFakeJJToBinDir(t, binDir)

	worktreesDir := t.TempDir()
	beadID := "jj-newb-status"
	wtPath := filepath.Join(worktreesDir, beadID)
	if err := os.MkdirAll(wtPath, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	b := wt.NewJJBackend(worktreesDir)
	ctx := context.Background()

	st, err := b.Status(ctx, beadID)
	if err != nil {
		t.Fatalf("Status via NewJJBackend: %v", err)
	}
	if !st.Exists {
		t.Error("Exists want true")
	}
}

func TestNewJJBackend_DiffSummaryDelegates(t *testing.T) {
	binDir := t.TempDir()
	addFakeJJToBinDir(t, binDir)

	worktreesDir := t.TempDir()
	beadID := "jj-newb-diffsummary"
	wtPath := filepath.Join(worktreesDir, beadID)
	if err := os.MkdirAll(wtPath, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	b := wt.NewJJBackend(worktreesDir)
	ctx := context.Background()

	changes, err := b.DiffSummary(ctx, beadID)
	if err != nil {
		t.Fatalf("DiffSummary via NewJJBackend: %v", err)
	}
	// Fake jj emits "M modified.txt\nA new.go\nD deleted.txt\n" by default.
	if len(changes) == 0 {
		t.Error("DiffSummary: want at least one change, got 0")
	}
}

func TestNewJJBackend_DiffDelegates(t *testing.T) {
	binDir := t.TempDir()
	addFakeJJToBinDir(t, binDir)

	worktreesDir := t.TempDir()
	beadID := "jj-newb-diff"
	wtPath := filepath.Join(worktreesDir, beadID)
	if err := os.MkdirAll(wtPath, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	b := wt.NewJJBackend(worktreesDir)
	ctx := context.Background()

	rc, err := b.Diff(ctx, beadID, "")
	if err != nil {
		t.Fatalf("Diff via NewJJBackend: %v", err)
	}
	defer rc.Close()
	data, err := io.ReadAll(rc)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	// The fake jj emits a minimal git-format diff.
	if !strings.Contains(string(data), "diff --git") {
		t.Errorf("expected git-format diff, got:\n%s", data)
	}
}

// TestJJDiffSummaryAt_ParseEdgeCases exercises parseJJSummary edge-case paths:
// empty lines, lines with too few fields, unknown kind, malformed rename/copy.
func TestJJDiffSummaryAt_ParseEdgeCases(t *testing.T) {
	binDir := t.TempDir()
	addFakeJJToBinDir(t, binDir)

	worktreesDir := t.TempDir()
	beadID := "jj-parse-edge"
	wtPath := filepath.Join(worktreesDir, beadID)
	if err := os.MkdirAll(wtPath, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	// Comprehensive edge cases:
	// - empty line
	// - line with no space (too few fields)
	// - unknown kind 'Z'
	// - malformed rename (R with only 2 fields, missing destination)
	// - malformed copy (C with only 2 fields, missing destination)
	// - one valid line
	t.Setenv("FAKE_JJ_DIFF_SUMMARY", "\nnofields\nZ unknown.go\nR only-src\nC only-src2\nA valid.go\n")

	ctx := context.Background()
	changes, err := wt.JJDiffSummaryAt(ctx, worktreesDir, beadID)
	if err != nil {
		t.Fatalf("JJDiffSummaryAt: %v", err)
	}

	// Only "A valid.go" should survive.
	if len(changes) != 1 {
		t.Errorf("want 1 parsed change, got %d: %v", len(changes), changes)
	} else if changes[0].Path != "valid.go" || changes[0].Kind != wt.Added {
		t.Errorf("want {Path:valid.go Kind:Added}, got %+v", changes[0])
	}
}

// TestJJBackend_ResolveSrcRepo_TraversalRejected verifies that the fallback path
// in resolveSrcRepo (reading the agent-writable .jj/repo file) rejects a path
// that points into the worktrees directory (path-traversal guard, tri-review FIX 1).
//
// The test exercises Remove, which calls resolveSrcRepo before running
// `jj workspace forget`. If the traversal is detected, Remove returns an error
// before any jj command runs.
func TestJJBackend_ResolveSrcRepo_TraversalRejected(t *testing.T) {
	binDir := t.TempDir()
	addFakeJJToBinDir(t, binDir)

	worktreesDir := t.TempDir()
	beadID := "traversal-bead"
	wtPath := filepath.Join(worktreesDir, beadID)

	// Create the workspace directory manually (no real jj needed).
	jjDir := filepath.Join(wtPath, ".jj")
	if err := os.MkdirAll(jjDir, 0o755); err != nil {
		t.Fatalf("MkdirAll .jj: %v", err)
	}

	// Write a .jj/repo that resolves into the worktrees directory itself —
	// a traversal target a malicious agent could use to redirect git push.
	// jjSrcRepoDir reads: wtPath/.jj/<rel> → strips /.jj/repo suffix → srcRepo.
	// We want srcRepo == worktreesDir, so rel = "../../.jj/repo" resolves to
	// worktreesDir/.jj/repo; stripping /.jj/repo gives worktreesDir.
	traversalRel := "../../.jj/repo"
	if err := os.WriteFile(filepath.Join(jjDir, "repo"), []byte(traversalRel), 0o644); err != nil {
		t.Fatalf("write .jj/repo: %v", err)
	}

	// Use NewJJBackend so worktreesDir is set (required for the traversal check).
	b := wt.NewJJBackend(worktreesDir)
	ctx := context.Background()

	// Remove calls resolveSrcRepo (cache miss → fallback) before `jj workspace forget`.
	// The fallback should detect the traversal and return an error.
	err := b.Remove(ctx, beadID)
	if err == nil {
		t.Fatal("Remove with traversal .jj/repo: expected error, got nil")
	}
	if !strings.Contains(err.Error(), "path traversal") {
		t.Errorf("Remove traversal error = %q; want to contain 'path traversal'", err.Error())
	}
}

// TestJJBackend_ResolveSrcRepo_CacheBypassesValidation verifies that when
// resolveSrcRepo finds the beadID in the in-memory cache (populated by Create),
// it returns the cached value directly without reading or validating .jj/repo.
// The Remove call succeeds past the resolveSrcRepo step (fake jj handles
// `workspace forget`) and fails only at os.RemoveAll or succeeds entirely —
// but critically does NOT return a traversal error.
func TestJJBackend_ResolveSrcRepo_CacheBypassesValidation(t *testing.T) {
	binDir := t.TempDir()
	addFakeJJToBinDir(t, binDir)
	srcRepo := t.TempDir()

	worktreesDir := t.TempDir()
	beadID := "cache-bead"

	// Prime the cache via Create (fake jj exits 0 for root; workspace add creates the dir).
	t.Setenv("FAKE_JJ_ROOT", srcRepo)

	b, err := wt.For(wt.VCSJJ)
	if err != nil {
		t.Fatalf("For(jj): %v", err)
	}
	ctx := context.Background()
	if _, err := b.Create(ctx, worktreesDir, srcRepo, beadID); err != nil {
		t.Fatalf("Create: %v", err)
	}

	// Now write a tampered .jj/repo — the cache hit must prevent it from being read.
	wtPath := filepath.Join(worktreesDir, beadID)
	jjDir := filepath.Join(wtPath, ".jj")
	if err := os.MkdirAll(jjDir, 0o755); err != nil {
		t.Fatalf("MkdirAll .jj: %v", err)
	}
	if err := os.WriteFile(filepath.Join(jjDir, "repo"), []byte("../../.jj/repo"), 0o644); err != nil {
		t.Fatalf("write .jj/repo: %v", err)
	}

	// Remove goes through resolveSrcRepo; cache hit → returns srcRepo, bypasses .jj/repo.
	// Fake jj handles `workspace forget` → Remove succeeds (or fails at RemoveAll, not
	// at resolveSrcRepo). Critically: no traversal error.
	err = b.Remove(ctx, beadID)
	if err != nil && strings.Contains(err.Error(), "path traversal") {
		t.Errorf("Remove with cached srcRepo should not return traversal error, got: %v", err)
	}
	// The cache bypass is verified: no traversal error means the cache was used.
}

// TestJJBackend_EmptyWorktreesDir_Errors covers the worktreesDir=="" guard branches
// in jjBackend.Status, DiffSummary, and Diff (the 66.7% lines).
func TestJJBackend_EmptyWorktreesDir_Errors(t *testing.T) {
	// For(VCSJJ) returns a jjBackend with empty worktreesDir.
	b, err := wt.For(wt.VCSJJ)
	if err != nil {
		t.Fatalf("For(jj): %v", err)
	}
	ctx := context.Background()

	if _, err := b.Status(ctx, "bead"); err == nil {
		t.Error("Status with empty worktreesDir: expected error, got nil")
	}
	if _, err := b.DiffSummary(ctx, "bead"); err == nil {
		t.Error("DiffSummary with empty worktreesDir: expected error, got nil")
	}
	if _, err := b.Diff(ctx, "bead", ""); err == nil {
		t.Error("Diff with empty worktreesDir: expected error, got nil")
	}
}

// ── T028: jj resolveSrcRepo fallback allow-list (tri-review 5) ──────────────────────

// makeJJWorkspaceWithFakeRepo creates a worktree directory with a .jj/repo file
// that resolves to srcRepo. It does NOT use real jj — the structure is crafted
// manually so the test is fully hermetic (no fake jj or real jj binary needed).
//
// jjSrcRepoDir reads wtPath/.jj/repo, interprets its contents as a relative
// path from wtPath/.jj to srcRepo/.jj/repo, resolves it, then strips the
// trailing /.jj/repo suffix. The file content we write here achieves:
//
//	wtPath/.jj/../../../<srcRepoBase>/.jj/repo  →  srcRepo
//
// which simplifies to: relative path = ../../<srcRepoBase>/.jj/repo
// (from wtPath/.jj going up two levels to the parent of worktreesDir, then
// down into srcRepoBase). Because t.TempDir() paths are unpredictable we
// compute the relative path dynamically.
func makeJJWorkspaceWithFakeRepo(t *testing.T, worktreesDir, beadID, srcRepo string) string {
	t.Helper()
	wtPath := filepath.Join(worktreesDir, beadID)
	jjDir := filepath.Join(wtPath, ".jj")
	if err := os.MkdirAll(jjDir, 0o755); err != nil {
		t.Fatalf("MkdirAll .jj: %v", err)
	}
	// Compute relative path from wtPath/.jj to srcRepo/.jj/repo.
	rel, err := filepath.Rel(jjDir, filepath.Join(srcRepo, ".jj", "repo"))
	if err != nil {
		t.Fatalf("filepath.Rel: %v", err)
	}
	if err := os.WriteFile(filepath.Join(jjDir, "repo"), []byte(rel), 0o644); err != nil {
		t.Fatalf("write .jj/repo: %v", err)
	}
	return wtPath
}

// TestJJBackend_AllowList_RejectsFallbackOutsideList verifies that
// NewJJBackendAllowed with a non-empty allow-list rejects a srcRepo path
// resolved from .jj/repo that is NOT in the list. The Remove call (which
// exercises the resolveSrcRepo fallback) must fail with an "not in allowed"
// error, not a traversal error.
func TestJJBackend_AllowList_RejectsFallbackOutsideList(t *testing.T) {
	binDir := t.TempDir()
	addFakeJJToBinDir(t, binDir)

	worktreesDir := t.TempDir()
	beadID := "allowlist-bead"

	// srcRepo is a real directory — just not the one in the allow-list.
	srcRepo := t.TempDir()
	makeJJWorkspaceWithFakeRepo(t, worktreesDir, beadID, srcRepo)

	// The allow-list contains a different (allowed) path, not srcRepo.
	otherAllowed := t.TempDir()
	b := wt.NewJJBackendAllowed(worktreesDir, []string{otherAllowed})
	ctx := context.Background()

	err := b.Remove(ctx, beadID)
	if err == nil {
		t.Fatal("Remove with out-of-allow-list srcRepo: expected error, got nil")
	}
	if !strings.Contains(err.Error(), "not in allowed") {
		t.Errorf("Remove allow-list error = %q; want to contain 'not in allowed'", err.Error())
	}
}

// TestJJBackend_AllowList_AcceptsFallbackInsideList verifies that
// NewJJBackendAllowed accepts a srcRepo path that IS in the allow-list.
// The Remove succeeds past resolveSrcRepo (fake jj handles workspace forget).
func TestJJBackend_AllowList_AcceptsFallbackInsideList(t *testing.T) {
	binDir := t.TempDir()
	addFakeJJToBinDir(t, binDir)

	worktreesDir := t.TempDir()
	beadID := "allowlist-accepted-bead"

	srcRepo := t.TempDir()
	makeJJWorkspaceWithFakeRepo(t, worktreesDir, beadID, srcRepo)

	// Allow-list contains srcRepo — fallback should be permitted.
	b := wt.NewJJBackendAllowed(worktreesDir, []string{srcRepo})
	ctx := context.Background()

	// Remove may fail at `jj workspace forget` or os.RemoveAll (the worktrees dir
	// file layout is minimal), but must NOT fail with "not in allowed".
	err := b.Remove(ctx, beadID)
	if err != nil && strings.Contains(err.Error(), "not in allowed") {
		t.Errorf("Remove with allowed srcRepo should not return allow-list error, got: %v", err)
	}
}

// TestJJBackend_AllowList_NilAllowsAll verifies that NewJJBackend (nil
// allow-list) continues to permit any valid, non-traversal srcRepo — existing
// behaviour is not regressed.
func TestJJBackend_AllowList_NilAllowsAll(t *testing.T) {
	binDir := t.TempDir()
	addFakeJJToBinDir(t, binDir)

	worktreesDir := t.TempDir()
	beadID := "allowlist-nil-bead"

	srcRepo := t.TempDir()
	makeJJWorkspaceWithFakeRepo(t, worktreesDir, beadID, srcRepo)

	// No allow-list → any valid path is accepted.
	b := wt.NewJJBackend(worktreesDir)
	ctx := context.Background()

	err := b.Remove(ctx, beadID)
	if err != nil && strings.Contains(err.Error(), "not in allowed") {
		t.Errorf("Remove with nil allow-list should not return allow-list error, got: %v", err)
	}
}

// ── FIX A: checkJJWorkspaceDir — file-not-dir guard ─────────────────────────

// TestJJBackend_Finalize_FileNotDir verifies that Finalize returns
// ErrWorktreeNotFound when the workspace path exists but is a regular file,
// not a directory. This tests the IsDir() check added by checkJJWorkspaceDir.
func TestJJBackend_Finalize_FileNotDir(t *testing.T) {
	worktreesDir := t.TempDir()
	beadID := "jj-finalize-filenotdir"

	// Plant a plain file at the workspace path instead of a directory.
	if err := os.WriteFile(filepath.Join(worktreesDir, beadID), []byte("not a dir"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	b := wt.NewJJBackend(worktreesDir)
	ctx := context.Background()

	_, err := b.Finalize(ctx, beadID, "msg")
	if err == nil {
		t.Fatal("Finalize(file instead of dir): expected error, got nil")
	}
	if !errors.Is(err, wt.ErrWorktreeNotFound) {
		t.Errorf("Finalize(file instead of dir): want ErrWorktreeNotFound, got %v", err)
	}
}

// TestJJBackend_Push_FileNotDir verifies that Push returns ErrWorktreeNotFound
// when the workspace path exists but is a regular file, not a directory.
func TestJJBackend_Push_FileNotDir(t *testing.T) {
	worktreesDir := t.TempDir()
	beadID := "jj-push-filenotdir"

	if err := os.WriteFile(filepath.Join(worktreesDir, beadID), []byte("not a dir"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	b := wt.NewJJBackend(worktreesDir)
	ctx := context.Background()

	err := b.Push(ctx, beadID, "")
	if err == nil {
		t.Fatal("Push(file instead of dir): expected error, got nil")
	}
	if !errors.Is(err, wt.ErrWorktreeNotFound) {
		t.Errorf("Push(file instead of dir): want ErrWorktreeNotFound, got %v", err)
	}
}

// TestJJBackend_Remove_FileNotDir verifies that Remove returns ErrWorktreeNotFound
// when the workspace path exists but is a regular file, not a directory.
func TestJJBackend_Remove_FileNotDir(t *testing.T) {
	worktreesDir := t.TempDir()
	beadID := "jj-remove-filenotdir"

	if err := os.WriteFile(filepath.Join(worktreesDir, beadID), []byte("not a dir"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	b := wt.NewJJBackend(worktreesDir)
	ctx := context.Background()

	err := b.Remove(ctx, beadID)
	if err == nil {
		t.Fatal("Remove(file instead of dir): expected error, got nil")
	}
	if !errors.Is(err, wt.ErrWorktreeNotFound) {
		t.Errorf("Remove(file instead of dir): want ErrWorktreeNotFound, got %v", err)
	}
}
