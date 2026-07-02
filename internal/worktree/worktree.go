package worktree

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// Worktree describes a per-bead git worktree.
type Worktree struct {
	BeadID string
	// Path is <worktreesDir>/<beadID>; RepoPath echoes the repoPath passed to
	// Ensure. Both are absolute when the caller passes absolute worktreesDir /
	// repoPath — which the production caller (cmd/muster: --worktrees-dir
	// defaulting under the home dir, and repo paths resolved via filepath.Abs
	// in config.ParseRepoFlag) always does. Ensure does not itself re-absolutize
	// them.
	Path     string // worktree directory (<worktreesDir>/<beadID>)
	Branch   string // branch name: "muster/<beadID>"
	RepoPath string // source repository (as passed to Ensure)
}

// branchName returns the branch name for a given bead ID.
func branchName(beadID string) string {
	return "muster/" + beadID
}

// worktreePath returns the absolute path where the worktree will be created.
func worktreePath(worktreesDir, beadID string) string {
	return filepath.Join(worktreesDir, beadID)
}

// Ensure creates or reuses the per-bead worktree.
//
//   - If a worktree at <worktreesDir>/<beadID> already exists, it is reused —
//     but only after verifying it's a linked git worktree, belongs to repoPath,
//     and is checked out on the expected muster/<beadID> branch; a mismatch (or
//     a symlink at the path, or a non-worktree directory) is refused, not
//     silently accepted.
//   - Otherwise, it is created via `git worktree add -b muster/<beadID> <path>`.
//   - Returns an error if repoPath is not a git repository.
//
// ctx bounds every git subprocess Ensure spawns; cancellation propagates
// through exec.CommandContext to kill the child git process. Ensure does not
// enforce a deadline itself, so callers are free to pass context.Background()
// (several tests do). In production, however, callers SHOULD supply a
// deadline-carrying context: Ensure is called from Dispatch while the run
// reservation (State=StepActive) is already held, and a hung git call on an
// unbounded context would pin the reservation indefinitely, making the bead
// undispatchable until the server restarts.
func Ensure(ctx context.Context, worktreesDir, repoPath, beadID string) (Worktree, error) {
	// Defense in depth: beadID becomes both a filesystem path segment
	// (filepath.Join(worktreesDir, beadID)) and a git branch name
	// (muster/<beadID>). The canonical bead-ID grammar is enforced upstream by
	// the HTTP handler and the orchestrator via core.ValidBeadID; this guard is
	// narrower and purely about path safety, since Ensure is a public function
	// and must not trust its caller. It rejects only what could escape
	// worktreesDir — a value like "../x", an absolute path, or an embedded
	// separator ("a/b") — and deliberately does NOT re-impose the full grammar
	// (a path-safe but non-canonical ID like "Foo" is left for the caller's
	// validation to reject). filepath.IsLocal rejects "..", absolute paths, and
	// empty/reserved names; Base==beadID rejects any embedded separator so the
	// join stays a direct child of worktreesDir.
	if !filepath.IsLocal(beadID) || beadID != filepath.Base(beadID) {
		return Worktree{}, fmt.Errorf("worktree: refusing unsafe beadID %q (must be a single local path segment)", beadID)
	}

	// Validate that repoPath is a git repo.
	if err := validateGitRepo(ctx, repoPath); err != nil {
		return Worktree{}, err
	}

	path := worktreePath(worktreesDir, beadID)
	branch := branchName(beadID)

	// If the path already exists as a git worktree directory, reuse it.
	// Only os.IsNotExist is acceptable to fall through to creation; other stat
	// errors (permission denied, IO error, …) would otherwise reach
	// `git worktree add` and surface a confusing downstream message — return
	// the real cause instead. Use os.Lstat (not os.Stat) so a pre-planted
	// SYMLINK at <worktreesDir>/<beadID> is detected rather than followed: a
	// local attacker could otherwise redirect the reused worktree to a target
	// outside worktreesDir (the create-path symlink hardening below only guards
	// worktreesDir itself, not this per-bead entry).
	if fi, err := os.Lstat(path); err == nil {
		if fi.Mode()&os.ModeSymlink != 0 {
			return Worktree{}, fmt.Errorf("worktree: path %q is a symlink; refusing to reuse it", path)
		}
		// Directory exists — verify it's a worktree.
		if isWorktreeDir(path) {
			// Also verify the existing worktree belongs to repoPath. Without
			// this check, if worktreesDir is reused across a repo-mapping
			// change (or any other config drift), Ensure could silently
			// return a worktree linked to a different repository — and the
			// agent would then run in an unexpected checkout.
			if err := existingWorktreeMatchesRepo(ctx, path, repoPath); err != nil {
				return Worktree{}, err
			}
			// And verify it's still checked out on the expected per-bead
			// branch. A user may have manually switched branches or detached
			// HEAD inside the worktree; reusing it as-is would silently run the
			// agent against an unexpected revision, breaking the per-bead
			// branch invariant. Refuse rather than run on the wrong revision.
			if err := worktreeOnExpectedBranch(ctx, path, branch); err != nil {
				return Worktree{}, err
			}
			return Worktree{
				BeadID:   beadID,
				Path:     path,
				Branch:   branch,
				RepoPath: repoPath,
			}, nil
		}
		// Path exists but is not a linked worktree. Falling through to
		// `git worktree add` would either fail with a less clear error or
		// risk operating on an unrelated pre-existing directory. Refuse.
		return Worktree{}, fmt.Errorf("worktree: path %q exists but is not a git worktree (refusing to overwrite)", path)
	} else if !os.IsNotExist(err) {
		return Worktree{}, fmt.Errorf("worktree: lstat %q: %w", path, err)
	}

	// Create parent directory if needed. 0o700 keeps per-bead worktrees (which
	// may hold prompt files with sensitive task context, and the agent's
	// working checkout) unreadable by other local users.
	// Refuse a pre-planted symlink BEFORE creating anything. If a hostile local
	// user pre-planted worktreesDir as a symlink to an arbitrary target, both
	// os.MkdirAll and os.Chmod would follow it — MkdirAll would create/reuse the
	// target directory and later git-worktree paths would resolve under it, and
	// Chmod would change the target's mode to 0o700. Lstat (which does not
	// follow) lets us catch this first. A not-yet-existing worktreesDir is fine
	// — MkdirAll creates it fresh (and a freshly-created dir is never a symlink)
	// — so ENOENT is expected, not an error.
	if fi, err := os.Lstat(worktreesDir); err == nil {
		if fi.Mode()&os.ModeSymlink != 0 {
			return Worktree{}, fmt.Errorf("worktree: worktrees dir %q is a symlink; refusing to operate through it", worktreesDir)
		}
	} else if !os.IsNotExist(err) {
		return Worktree{}, fmt.Errorf("worktree: lstat worktrees dir: %w", err)
	}
	if err := os.MkdirAll(worktreesDir, 0o700); err != nil {
		return Worktree{}, fmt.Errorf("worktree: create worktrees dir: %w", err)
	}
	// MkdirAll only applies the mode to directories it actually creates — if
	// worktreesDir already existed (e.g. a shared default like
	// <os.TempDir()>/muster/worktrees pre-planted by another local user, or
	// created earlier under a looser umask), it's silently reused as-is, so we
	// tighten it here. Safe to Chmod directly now: the Lstat above already
	// refused a symlink at this path.
	if err := os.Chmod(worktreesDir, 0o700); err != nil {
		return Worktree{}, fmt.Errorf("worktree: chmod worktrees dir: %w", err)
	}

	// Check if branch already exists (e.g., worktree was deleted manually but
	// branch remains). If so, use --track form without -b.
	if branchExists(ctx, repoPath, branch) {
		// Recreate the worktree on the existing branch.
		cmd := exec.CommandContext(ctx, "git", "worktree", "add", path, branch)
		cmd.Dir = repoPath
		if out, err := cmd.CombinedOutput(); err != nil {
			cleanupFailedCreate(repoPath, path)
			return Worktree{}, fmt.Errorf("worktree: git worktree add (existing branch): %w\n%s", err, out)
		}
	} else {
		// Create a new branch and worktree.
		cmd := exec.CommandContext(ctx, "git", "worktree", "add", "-b", branch, path)
		cmd.Dir = repoPath
		if out, err := cmd.CombinedOutput(); err != nil {
			cleanupFailedCreate(repoPath, path)
			return Worktree{}, fmt.Errorf("worktree: git worktree add -b %s: %w\n%s", branch, err, out)
		}
	}

	return Worktree{
		BeadID:   beadID,
		Path:     path,
		Branch:   branch,
		RepoPath: repoPath,
	}, nil
}

// cleanupFailedCreate best-effort removes a half-created worktree directory
// after `git worktree add` fails or is cancelled/killed mid-checkout. git
// writes the worktree's `.git` gitlink file (and registers it in
// `.git/worktrees/<name>`) BEFORE performing the file checkout, so a process
// killed at the wrong moment — e.g. by ctx's own deadline via
// exec.CommandContext's SIGKILL — can leave a directory that looks like a
// complete worktree (isWorktreeDir would return true) but has an incomplete
// checkout. Without this cleanup, a later Ensure call for the same beadID
// would either silently reuse the corrupted directory as if it were valid,
// or (if the gitlink itself never got written) permanently refuse to reuse
// OR recreate it — both are worse than a best-effort removal here.
//
// cleanupTimeout bounds each best-effort git subprocess cleanupFailedCreate
// runs. Decoupled from the caller's ctx (which may already be the thing that
// expired), but NOT unbounded either — a hung `git worktree remove`/`prune`
// (e.g. a stale NFS mount or a stuck lock file) must not block Ensure's
// error-return path indefinitely, since Ensure runs while the orchestrator's
// run reservation is held (see the ctx-decoupling rationale on Ensure above).
const cleanupTimeout = 10 * time.Second

// Uses a context decoupled from the caller's ctx (not context.Background()
// forever — see cleanupTimeout): cleanup must still run when the failure IS
// the caller's ctx expiring/being cancelled, but each subprocess gets its own
// bounded deadline so a hung cleanup can't wedge the caller forever either.
func cleanupFailedCreate(repoPath, path string) {
	removeCtx, removeCancel := context.WithTimeout(context.Background(), cleanupTimeout)
	defer removeCancel()
	removeCmd := exec.CommandContext(removeCtx, "git", "worktree", "remove", "--force", path)
	removeCmd.Dir = repoPath
	if removeCmd.Run() == nil {
		return
	}
	// `git worktree remove` itself can fail on a sufficiently mangled
	// half-created directory (e.g. no gitlink was ever written). Fall back to
	// a raw removal plus prune to drop the stale admin entry.
	_ = os.RemoveAll(path)
	pruneCtx, pruneCancel := context.WithTimeout(context.Background(), cleanupTimeout)
	defer pruneCancel()
	pruneCmd := exec.CommandContext(pruneCtx, "git", "worktree", "prune")
	pruneCmd.Dir = repoPath
	_ = pruneCmd.Run()
}

// validateGitRepo returns an error if path is not a git repository.
func validateGitRepo(ctx context.Context, path string) error {
	cmd := exec.CommandContext(ctx, "git", "rev-parse", "--git-dir")
	cmd.Dir = path
	out, err := cmd.CombinedOutput()
	if err != nil {
		// Don't mislabel a cancelled/timed-out context as "not a git repo":
		// exec.CommandContext fails when ctx is done, and that's an aborted
		// probe, not a verdict about the directory. Surface ctx.Err() instead.
		if ctx.Err() != nil {
			return fmt.Errorf("worktree: git repo check aborted for %q: %w", path, ctx.Err())
		}
		return fmt.Errorf("worktree: not a git repo at %q: %w\n%s", path, err, out)
	}
	return nil
}

// existingWorktreeMatchesRepo verifies an already-existing linked worktree at
// wtPath was created from repoPath, by comparing their shared "common git
// directory" (the .git dir of the main checkout, which all linked worktrees
// reference). Returns an error if they differ — the safe default in that case
// is to refuse rather than reuse a worktree pointing at the wrong repo.
func existingWorktreeMatchesRepo(ctx context.Context, wtPath, repoPath string) error {
	repoCD, err := gitCommonDir(ctx, repoPath)
	if err != nil {
		return fmt.Errorf("worktree: resolve repo git-common-dir: %w", err)
	}
	wtCD, err := gitCommonDir(ctx, wtPath)
	if err != nil {
		return fmt.Errorf("worktree: resolve worktree git-common-dir: %w", err)
	}
	if repoCD != wtCD {
		return fmt.Errorf("worktree: existing path %q is linked to repo (common-dir %q), not %q (common-dir %q) — refusing to reuse", wtPath, wtCD, repoPath, repoCD)
	}
	return nil
}

// worktreeOnExpectedBranch verifies the worktree at wtPath currently has the
// branch checked out (not a different branch, not a detached HEAD). It compares
// the full symbolic ref of HEAD against refs/heads/<branch>: `git symbolic-ref
// HEAD` exits non-zero on a detached HEAD, and the full-ref form avoids the
// ambiguity `--short` introduces when a same-named tag also exists. Returns an
// error if HEAD is detached or on a different branch — Ensure refuses to reuse
// such a worktree rather than silently run the agent on an unexpected revision.
func worktreeOnExpectedBranch(ctx context.Context, wtPath, branch string) error {
	cmd := exec.CommandContext(ctx, "git", "symbolic-ref", "HEAD")
	cmd.Dir = wtPath
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("worktree: existing path %q is not on a branch (detached HEAD?); expected branch %q — refusing to reuse: %w\n%s", wtPath, branch, err, out)
	}
	got := strings.TrimSpace(string(out))
	want := "refs/heads/" + branch
	if got != want {
		return fmt.Errorf("worktree: existing path %q is on %q, expected %q — refusing to reuse (per-bead branch invariant)", wtPath, got, want)
	}
	return nil
}

// gitCommonDir returns the absolute, cleaned path of `git rev-parse
// --git-common-dir` run inside dir. The common-dir is the .git directory of
// the main worktree; linked worktrees share the same value.
func gitCommonDir(ctx context.Context, dir string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", "rev-parse", "--git-common-dir")
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("git rev-parse --git-common-dir in %q: %w\n%s", dir, err, out)
	}
	p := strings.TrimSpace(string(out))
	if !filepath.IsAbs(p) {
		// git returns a path relative to the cwd it was invoked from.
		p = filepath.Join(dir, p)
	}
	// Resolve symlinks so e.g. /var/folders vs /private/var/folders on macOS
	// don't false-mismatch.
	if resolved, err := filepath.EvalSymlinks(p); err == nil {
		p = resolved
	}
	return filepath.Clean(p), nil
}

// isWorktreeDir returns true if path contains a .git file (indicating it is a
// linked worktree rather than the main checkout).
func isWorktreeDir(path string) bool {
	// A linked worktree has a .git file (not a directory).
	info, err := os.Stat(filepath.Join(path, ".git"))
	if err != nil {
		return false
	}
	// Main worktrees have .git as a directory; linked worktrees have it as a file.
	return !info.IsDir()
}

// branchExists returns true if a local branch named branch exists in the repo.
//
// It verifies specifically under refs/heads/. A bare `git rev-parse --verify
// <branch>` resolves ANY ref matching the name — including a tag — so if a tag
// named muster/<beadID> existed, Ensure would take the "reuse existing branch"
// path and `git worktree add <path> <tag>` would check out the tag in detached
// HEAD, breaking the invariant that each bead gets a dedicated branch.
func branchExists(ctx context.Context, repoPath, branch string) bool {
	cmd := exec.CommandContext(ctx, "git", "show-ref", "--verify", "--quiet", "refs/heads/"+branch)
	cmd.Dir = repoPath
	return cmd.Run() == nil
}
