package worktree

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Worktree describes a per-bead git worktree.
type Worktree struct {
	BeadID   string
	Path     string // absolute path to the worktree directory
	Branch   string // branch name: "muster/<beadID>"
	RepoPath string // absolute path to the source repository
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
//   - If the worktree at <worktreesDir>/<beadID> already exists, it is returned as-is.
//   - Otherwise, it is created via `git worktree add -b muster/<beadID> <path>`.
//   - Returns an error if repoPath is not a git repository.
//
// ctx bounds every git subprocess Ensure spawns. Ensure is called from Dispatch
// while the run reservation (State=StepActive) is already held; a hung git
// call would otherwise pin the reservation indefinitely and make the bead
// undispatchable until the server restarts, so the caller MUST supply a
// deadline-carrying context. Cancellation propagates through
// exec.CommandContext to kill the child git process.
func Ensure(ctx context.Context, worktreesDir, repoPath, beadID string) (Worktree, error) {
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
	// the real cause instead.
	if _, err := os.Stat(path); err == nil {
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
		return Worktree{}, fmt.Errorf("worktree: stat %q: %w", path, err)
	}

	// Create parent directory if needed. 0o700 keeps per-bead worktrees (which
	// may hold prompt files with sensitive task context, and the agent's
	// working checkout) unreadable by other local users.
	if err := os.MkdirAll(worktreesDir, 0o700); err != nil {
		return Worktree{}, fmt.Errorf("worktree: create worktrees dir: %w", err)
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
// Uses context.Background(), not the caller's ctx: cleanup must still run
// when the failure IS the caller's ctx expiring/being cancelled.
func cleanupFailedCreate(repoPath, path string) {
	removeCmd := exec.CommandContext(context.Background(), "git", "worktree", "remove", "--force", path)
	removeCmd.Dir = repoPath
	if removeCmd.Run() == nil {
		return
	}
	// `git worktree remove` itself can fail on a sufficiently mangled
	// half-created directory (e.g. no gitlink was ever written). Fall back to
	// a raw removal plus prune to drop the stale admin entry.
	_ = os.RemoveAll(path)
	pruneCmd := exec.CommandContext(context.Background(), "git", "worktree", "prune")
	pruneCmd.Dir = repoPath
	_ = pruneCmd.Run()
}

// validateGitRepo returns an error if path is not a git repository.
func validateGitRepo(ctx context.Context, path string) error {
	cmd := exec.CommandContext(ctx, "git", "rev-parse", "--git-dir")
	cmd.Dir = path
	out, err := cmd.CombinedOutput()
	if err != nil {
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

// branchExists returns true if branch exists in the repo.
func branchExists(ctx context.Context, repoPath, branch string) bool {
	cmd := exec.CommandContext(ctx, "git", "rev-parse", "--verify", branch)
	cmd.Dir = repoPath
	return cmd.Run() == nil
}
