package worktree

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
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
func Ensure(worktreesDir, repoPath, beadID string) (Worktree, error) {
	// Validate that repoPath is a git repo.
	if err := validateGitRepo(repoPath); err != nil {
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
			return Worktree{
				BeadID:   beadID,
				Path:     path,
				Branch:   branch,
				RepoPath: repoPath,
			}, nil
		}
	} else if !os.IsNotExist(err) {
		return Worktree{}, fmt.Errorf("worktree: stat %q: %w", path, err)
	}

	// Create parent directory if needed.
	if err := os.MkdirAll(worktreesDir, 0o755); err != nil {
		return Worktree{}, fmt.Errorf("worktree: create worktrees dir: %w", err)
	}

	// Check if branch already exists (e.g., worktree was deleted manually but
	// branch remains). If so, use --track form without -b.
	if branchExists(repoPath, branch) {
		// Recreate the worktree on the existing branch.
		cmd := exec.Command("git", "worktree", "add", path, branch)
		cmd.Dir = repoPath
		if out, err := cmd.CombinedOutput(); err != nil {
			return Worktree{}, fmt.Errorf("worktree: git worktree add (existing branch): %w\n%s", err, out)
		}
	} else {
		// Create a new branch and worktree.
		cmd := exec.Command("git", "worktree", "add", "-b", branch, path)
		cmd.Dir = repoPath
		if out, err := cmd.CombinedOutput(); err != nil {
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

// validateGitRepo returns an error if path is not a git repository.
func validateGitRepo(path string) error {
	cmd := exec.Command("git", "rev-parse", "--git-dir")
	cmd.Dir = path
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("worktree: not a git repo at %q: %w\n%s", path, err, out)
	}
	return nil
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
func branchExists(repoPath, branch string) bool {
	cmd := exec.Command("git", "rev-parse", "--verify", branch)
	cmd.Dir = repoPath
	return cmd.Run() == nil
}
