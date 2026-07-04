//go:build !windows

// jj backend for the wt package. Unix-only: jj is not supported on Windows
// in this codebase (JJ_CONFIG=/dev/null uses /dev/null which is Unix-only).

package wt

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// jjBackend implements Backend using the jj VCS. Finalize, Push, and Remove
// return ErrNotImplemented in M3 — the write-side is deferred to M4.
//
// The backend is jj-source-repos-only (FR-004a): Create probes `jj root` in
// the source repo; if it is not jj-native it returns ErrVCSUnavailable
// (no colocate, no silent fallback to git).
//
// Like gitBackend, the interface-level Status/DiffSummary/Diff methods require
// worktreesDir to be baked in via NewJJBackend(worktreesDir). The exported
// helpers JJStatusAt/JJDiffSummaryAt/JJDiffAt accept worktreesDir explicitly
// and are used by tests and the service layer.
type jjBackend struct {
	worktreesDir string
}

// NewJJBackend returns a jj Backend with worktreesDir baked in.
func NewJJBackend(worktreesDir string) Backend {
	return &jjBackend{worktreesDir: worktreesDir}
}

// Create ensures the per-bead jj workspace exists. It first probes `jj root`
// in srcRepo — if the repo is not jj-native it returns ErrVCSUnavailable.
// On a jj-native repo it runs `jj workspace add <path>`.
func (j *jjBackend) Create(ctx context.Context, worktreesDir, srcRepo, beadID string) (string, error) {
	// Sanitise beadID: must be non-empty and not contain path-separator chars.
	if beadID == "" || strings.ContainsAny(beadID, "/\\") || beadID == ".." {
		return "", fmt.Errorf("wt jj: invalid beadID %q", beadID)
	}

	wtPath := filepath.Join(worktreesDir, beadID)

	// Probe jj-nativeness: `jj root` exits 0 in a jj repo, non-zero otherwise.
	rootCmd := exec.CommandContext(ctx, "jj", "root")
	rootCmd.Dir = srcRepo
	if out, err := rootCmd.CombinedOutput(); err != nil {
		if ctx.Err() != nil {
			return "", fmt.Errorf("wt jj: root probe aborted: %w", ctx.Err())
		}
		return "", fmt.Errorf("%w: jj root failed in %q: %s", ErrVCSUnavailable, srcRepo, strings.TrimSpace(string(out)))
	}

	// If the workspace already exists, return it (reuse).
	if info, err := os.Lstat(wtPath); err == nil {
		if info.IsDir() {
			return wtPath, nil
		}
	}

	// Create the workspace: `jj workspace add <path>` run from srcRepo.
	addCmd := exec.CommandContext(ctx, "jj", "workspace", "add", wtPath)
	addCmd.Dir = srcRepo
	if out, err := addCmd.CombinedOutput(); err != nil {
		if ctx.Err() != nil {
			return "", fmt.Errorf("wt jj: workspace add aborted: %w", ctx.Err())
		}
		return "", fmt.Errorf("wt jj: workspace add %q: %w\n%s", wtPath, err, out)
	}

	return wtPath, nil
}

// Status returns the worktree's state. Requires NewJJBackend(worktreesDir).
func (j *jjBackend) Status(ctx context.Context, beadID string) (WorktreeStatus, error) {
	if j.worktreesDir == "" {
		return WorktreeStatus{}, fmt.Errorf("wt jj: Status requires worktreesDir — use NewJJBackend or JJStatusAt")
	}
	return JJStatusAt(ctx, j.worktreesDir, beadID)
}

// DiffSummary returns the set of changed files. Requires NewJJBackend(worktreesDir).
func (j *jjBackend) DiffSummary(ctx context.Context, beadID string) ([]FileChange, error) {
	if j.worktreesDir == "" {
		return nil, fmt.Errorf("wt jj: DiffSummary requires worktreesDir — use NewJJBackend or JJDiffSummaryAt")
	}
	return JJDiffSummaryAt(ctx, j.worktreesDir, beadID)
}

// Diff returns a streaming unified diff. Requires NewJJBackend(worktreesDir).
func (j *jjBackend) Diff(ctx context.Context, beadID, path string) (io.ReadCloser, error) {
	if j.worktreesDir == "" {
		return nil, fmt.Errorf("wt jj: Diff requires worktreesDir — use NewJJBackend or JJDiffAt")
	}
	return JJDiffAt(ctx, j.worktreesDir, beadID, path)
}

// Finalize returns ErrNotImplemented in M3.
func (j *jjBackend) Finalize(_ context.Context, _, _ string) error {
	return ErrNotImplemented
}

// Push returns ErrNotImplemented in M3.
func (j *jjBackend) Push(_ context.Context, _ string) error {
	return ErrNotImplemented
}

// Remove returns ErrNotImplemented in M3.
func (j *jjBackend) Remove(_ context.Context, _ string) error {
	return ErrNotImplemented
}

// ── Package-level helpers (bypass Backend interface) ──────────────────────

// JJStatusAt checks whether the per-bead workspace exists and is clean.
// It uses `jj status` — "The working copy is clean" means clean.
// If the workspace directory does not exist, returns ErrWorktreeNotFound.
func JJStatusAt(ctx context.Context, worktreesDir, beadID string) (WorktreeStatus, error) {
	path := filepath.Join(worktreesDir, beadID)

	info, err := os.Lstat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return WorktreeStatus{Exists: false}, ErrWorktreeNotFound
		}
		return WorktreeStatus{}, fmt.Errorf("wt jj: lstat workspace %q: %w", path, err)
	}
	if !info.IsDir() {
		return WorktreeStatus{Exists: false}, ErrWorktreeNotFound
	}

	cmd := exec.CommandContext(ctx, "jj", "status")
	cmd.Dir = path
	out, err := cmd.CombinedOutput()
	if err != nil {
		if ctx.Err() != nil {
			return WorktreeStatus{}, fmt.Errorf("wt jj: status aborted: %w", ctx.Err())
		}
		return WorktreeStatus{}, fmt.Errorf("wt jj: jj status in %q: %w\n%s", path, err, out)
	}

	// jj status says "The working copy is clean" when there are no changes.
	outStr := strings.TrimSpace(string(out))
	clean := strings.Contains(outStr, "clean") && !strings.Contains(outStr, "Working copy changes")
	return WorktreeStatus{Exists: true, Clean: clean}, nil
}

// JJDiffSummaryAt parses `jj diff --summary` output in the bead's workspace.
// The output format is space-delimited: "<KIND> <path>" per line, with
// renames as "R <oldpath> <newpath>" and copies as "C <oldpath> <newpath>".
// This parser is separate from the git porcelain parser (research §3).
func JJDiffSummaryAt(ctx context.Context, worktreesDir, beadID string) ([]FileChange, error) {
	path := filepath.Join(worktreesDir, beadID)

	// Verify the workspace exists and is a directory. A non-dir path maps to
	// ErrWorktreeNotFound (404), matching JJStatusAt, rather than letting the
	// jj command fail (chdir error) and surface as a generic 500.
	info, err := os.Lstat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, ErrWorktreeNotFound
		}
		return nil, fmt.Errorf("wt jj: lstat workspace %q: %w", path, err)
	}
	if !info.IsDir() {
		return nil, ErrWorktreeNotFound
	}

	cmd := exec.CommandContext(ctx, "jj", "diff", "--summary")
	cmd.Dir = path
	out, err := cmd.CombinedOutput()
	if err != nil {
		if ctx.Err() != nil {
			return nil, fmt.Errorf("wt jj: diff summary aborted: %w", ctx.Err())
		}
		return nil, fmt.Errorf("wt jj: jj diff --summary in %q: %w\n%s", path, err, out)
	}

	return parseJJSummary(out), nil
}

// JJDiffAt returns a streaming unified diff (git format) for the bead's workspace.
// When path is empty, the diff covers all files (`jj diff --git`).
// When path is non-empty, it covers the single file (`jj diff --git <path>`).
// path MUST already be validated by SafeRelPath before calling.
// The returned ReadCloser wraps child stdout; Close reaps the child.
// ctx cancellation kills the child via exec.CommandContext (review note W2).
func JJDiffAt(ctx context.Context, worktreesDir, beadID, path string) (io.ReadCloser, error) {
	wtPath := filepath.Join(worktreesDir, beadID)

	// Verify the workspace exists and is a directory (non-dir → 404, not 500).
	info, err := os.Lstat(wtPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, ErrWorktreeNotFound
		}
		return nil, fmt.Errorf("wt jj: lstat workspace %q: %w", wtPath, err)
	}
	if !info.IsDir() {
		return nil, ErrWorktreeNotFound
	}

	var args []string
	if path != "" {
		args = []string{"diff", "--git", path}
	} else {
		args = []string{"diff", "--git"}
	}

	cmd := exec.CommandContext(ctx, "jj", args...)
	cmd.Dir = wtPath

	return startStreamingCmd(cmd)
}

// parseJJSummary parses `jj diff --summary` output into FileChange entries.
//
// Line format:
//   - "M <path>"   → Modified
//   - "A <path>"   → Added
//   - "D <path>"   → Deleted
//   - "R <old> <new>" → Renamed (OldPath set)
//   - "C <old> <new>" → Copied (OldPath set)
//
// Unknown leading letters are skipped with a warning (defensive, per spec).
func parseJJSummary(data []byte) []FileChange {
	lines := strings.Split(strings.TrimRight(string(data), "\n"), "\n")
	var changes []FileChange

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		// Lines are space-delimited.
		parts := strings.Fields(line)
		if len(parts) < 2 {
			slog.Warn("wt jj: skipping malformed summary line", "line", line)
			continue
		}

		kind := parts[0]
		switch kind {
		case "M":
			changes = append(changes, FileChange{Path: parts[1], Kind: Modified})
		case "A":
			changes = append(changes, FileChange{Path: parts[1], Kind: Added})
		case "D":
			changes = append(changes, FileChange{Path: parts[1], Kind: Deleted})
		case "R":
			if len(parts) < 3 {
				slog.Warn("wt jj: rename entry missing new path", "line", line)
				continue
			}
			changes = append(changes, FileChange{
				Path:    parts[2],
				OldPath: parts[1],
				Kind:    Renamed,
			})
		case "C":
			if len(parts) < 3 {
				slog.Warn("wt jj: copy entry missing new path", "line", line)
				continue
			}
			changes = append(changes, FileChange{
				Path:    parts[2],
				OldPath: parts[1],
				Kind:    Copied,
			})
		default:
			slog.Warn("wt jj: unrecognized diff summary kind; skipping", "kind", kind, "line", line)
		}
	}
	return changes
}
