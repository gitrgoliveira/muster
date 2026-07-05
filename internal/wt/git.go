package wt

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/gitrgoliveira/muster/internal/worktree"
)

// gitBackend implements Backend using the git VCS. It optionally stores a
// worktreesDir so that the interface methods Status/DiffSummary/Diff can resolve
// the worktree path without requiring the caller to pass it on every call.
//
// When constructed via For(VCSGit), worktreesDir is empty and the interface-level
// Status/DiffSummary/Diff methods will return an error. Callers that need those
// methods must use NewGitBackend(worktreesDir) to construct a backend with the
// worktrees root baked in.
//
// The exported helpers GitStatusAt/GitDiffSummaryAt/GitDiffAt bypass the Backend
// interface and accept worktreesDir explicitly — they are used by the orchestrator
// and the service layer when worktreesDir comes from config.
type gitBackend struct {
	worktreesDir string
}

// NewGitBackend returns a git Backend with worktreesDir baked in. The resulting
// backend's Status/DiffSummary/Diff methods resolve paths from worktreesDir
// without an extra parameter. Use this from the orchestrator after reading config.
// Push uses the default "origin" remote.
func NewGitBackend(worktreesDir string) Backend {
	return &gitBackend{worktreesDir: worktreesDir}
}

// Create ensures the per-bead git worktree exists by delegating to
// [worktree.Ensure]. All M2 guards (path-safety, symlink rejection,
// repo-match, branch-match) are preserved exactly — this is a thin adapter.
func (g *gitBackend) Create(ctx context.Context, worktreesDir, srcRepo, beadID string) (string, error) {
	wt, err := worktree.Ensure(ctx, worktreesDir, srcRepo, beadID)
	if err != nil {
		return "", err
	}
	return wt.Path, nil
}

// Status returns the worktree's state. Requires the backend was constructed with
// NewGitBackend(worktreesDir). Returns an error if worktreesDir is empty.
func (g *gitBackend) Status(ctx context.Context, beadID string) (WorktreeStatus, error) {
	if g.worktreesDir == "" {
		return WorktreeStatus{}, fmt.Errorf("wt git: Status requires worktreesDir — use NewGitBackend or GitStatusAt")
	}
	return GitStatusAt(ctx, g.worktreesDir, beadID)
}

// DiffSummary returns the set of changed files. Requires the backend was
// constructed with NewGitBackend(worktreesDir).
func (g *gitBackend) DiffSummary(ctx context.Context, beadID string) ([]FileChange, error) {
	if g.worktreesDir == "" {
		return nil, fmt.Errorf("wt git: DiffSummary requires worktreesDir — use NewGitBackend or GitDiffSummaryAt")
	}
	return GitDiffSummaryAt(ctx, g.worktreesDir, beadID)
}

// Diff returns a streaming unified diff. Requires the backend was constructed
// with NewGitBackend(worktreesDir).
func (g *gitBackend) Diff(ctx context.Context, beadID, path string) (io.ReadCloser, error) {
	if g.worktreesDir == "" {
		return nil, fmt.Errorf("wt git: Diff requires worktreesDir — use NewGitBackend or GitDiffAt")
	}
	return GitDiffAt(ctx, g.worktreesDir, beadID, path)
}

// Finalize commits all changes in the bead's worktree with the given message.
// If the worktree has no changes (git status --porcelain is empty), this is a
// no-op and returns (false, nil) — no commit is created (FR-010).
// On non-empty changes: git add -A + git commit -m <message>; returns (true, nil).
// Requires the backend was constructed with NewGitBackend(worktreesDir).
func (g *gitBackend) Finalize(ctx context.Context, beadID, message string) (bool, error) {
	if g.worktreesDir == "" {
		return false, fmt.Errorf("wt git: Finalize requires worktreesDir — use NewGitBackend")
	}
	path := filepath.Join(g.worktreesDir, beadID)

	// Verify the worktree exists.
	info, err := os.Lstat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return false, ErrWorktreeNotFound
		}
		return false, fmt.Errorf("wt git: lstat worktree %q: %w", path, err)
	}
	if !info.IsDir() {
		return false, ErrWorktreeNotFound
	}

	// Check for changes using `git status --porcelain`.
	statusCmd := exec.CommandContext(ctx, "git", "status", "--porcelain")
	statusCmd.Dir = path
	out, err := statusCmd.CombinedOutput()
	if err != nil {
		if ctx.Err() != nil {
			return false, fmt.Errorf("wt git: finalize status aborted: %w", ctx.Err())
		}
		return false, fmt.Errorf("wt git: git status in %q: %w\n%s", path, err, out)
	}

	// No changes — no-op success (no commit created).
	if strings.TrimSpace(string(out)) == "" {
		return false, nil
	}

	// Stage all changes.
	addCmd := exec.CommandContext(ctx, "git", "add", "-A")
	addCmd.Dir = path
	if addOut, err := addCmd.CombinedOutput(); err != nil {
		if ctx.Err() != nil {
			return false, fmt.Errorf("wt git: finalize add aborted: %w", ctx.Err())
		}
		return false, fmt.Errorf("wt git: git add -A in %q: %w\n%s", path, err, addOut)
	}

	// Commit with the provided message.
	commitCmd := exec.CommandContext(ctx, "git", "commit", "-m", message)
	commitCmd.Dir = path
	if commitOut, err := commitCmd.CombinedOutput(); err != nil {
		if ctx.Err() != nil {
			return false, fmt.Errorf("wt git: finalize commit aborted: %w", ctx.Err())
		}
		return false, fmt.Errorf("wt git: git commit in %q: %w\n%s", path, err, commitOut)
	}

	return true, nil
}

// Push pushes the bead's branch (muster/<beadID>) to the "origin" remote.
// A non-zero git exit (no remote, auth failure, rejected) returns an explicit
// typed error — never silent success (FR-007).
// Requires the backend was constructed with NewGitBackend(worktreesDir).
func (g *gitBackend) Push(ctx context.Context, beadID string) error {
	if g.worktreesDir == "" {
		return fmt.Errorf("wt git: Push requires worktreesDir — use NewGitBackend")
	}
	path := filepath.Join(g.worktreesDir, beadID)

	// Verify the worktree exists.
	info, err := os.Lstat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return ErrWorktreeNotFound
		}
		return fmt.Errorf("wt git: lstat worktree %q: %w", path, err)
	}
	if !info.IsDir() {
		return ErrWorktreeNotFound
	}

	remote := ResolveRemote("")
	branch := BranchName(beadID)

	cmd := exec.CommandContext(ctx, "git", "push", remote, branch)
	cmd.Dir = path
	out, err := cmd.CombinedOutput()
	if err != nil {
		if ctx.Err() != nil {
			return fmt.Errorf("wt git: push aborted: %w", ctx.Err())
		}
		return fmt.Errorf("wt git: git push %s %s in %q: %w\n%s", remote, branch, path, err, out)
	}
	return nil
}

// Remove tears down the per-bead git worktree. It runs
// `git worktree remove <path>` (which handles the git metadata cleanup) and
// then `git worktree prune` to remove stale refs. After Remove, a subsequent
// Status call will report the worktree as absent.
// Requires the backend was constructed with NewGitBackend(worktreesDir).
func (g *gitBackend) Remove(ctx context.Context, beadID string) error {
	if g.worktreesDir == "" {
		return fmt.Errorf("wt git: Remove requires worktreesDir — use NewGitBackend")
	}
	path := filepath.Join(g.worktreesDir, beadID)

	// Verify the worktree exists.
	info, err := os.Lstat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return ErrWorktreeNotFound
		}
		return fmt.Errorf("wt git: lstat worktree %q: %w", path, err)
	}
	if !info.IsDir() {
		return ErrWorktreeNotFound
	}

	// `git worktree remove` needs to run from within any git-associated directory.
	// Running it with the worktree path as an argument from the worktree itself is
	// valid; git resolves paths relative to the main repo.
	removeCmd := exec.CommandContext(ctx, "git", "worktree", "remove", "--force", path)
	removeCmd.Dir = path
	if removeOut, err := removeCmd.CombinedOutput(); err != nil {
		if ctx.Err() != nil {
			return fmt.Errorf("wt git: remove aborted: %w", ctx.Err())
		}
		return fmt.Errorf("wt git: git worktree remove %q: %w\n%s", path, err, removeOut)
	}

	// Prune stale worktree refs from the main repo's metadata.
	// We need to run this from somewhere with access to the git repo; since the
	// worktree is now removed, find the main repo via the git-common-dir. We run
	// prune from the srcRepo by resolving it from the common dir.
	// Best-effort: prune failure is non-fatal (stale refs are cosmetic).
	if mainRepo := gitMainRepoDir(ctx, path); mainRepo != "" {
		pruneCmd := exec.CommandContext(ctx, "git", "worktree", "prune")
		pruneCmd.Dir = mainRepo
		_ = pruneCmd.Run() // best-effort
	}

	return nil
}

// gitMainRepoDir returns the path to the main repository (where .git lives)
// given a worktree path. It reads the worktree's .git file (which contains the
// gitdir pointing back to the main repo's worktrees/<name> directory). Returns
// empty string on any error (used for best-effort prune only).
func gitMainRepoDir(ctx context.Context, wtPath string) string {
	// git rev-parse --git-common-dir returns the common git directory
	// (e.g. /path/to/main/.git) from within any worktree.
	// Note: this is called after the worktree directory is already gone, so we
	// pass the path but it may fail — in which case we just skip the prune.
	cmd := exec.CommandContext(ctx, "git", "rev-parse", "--git-common-dir")
	cmd.Dir = filepath.Dir(wtPath) // run from parent dir since wtPath is removed
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	commonDir := strings.TrimSpace(string(out))
	if commonDir == "" {
		return ""
	}
	// commonDir is .git itself or a path inside it; go up to the repo root.
	if !filepath.IsAbs(commonDir) {
		commonDir = filepath.Join(filepath.Dir(wtPath), commonDir)
	}
	// Walk up from .git to find the repo root.
	for {
		parent := filepath.Dir(commonDir)
		if parent == commonDir {
			break
		}
		if filepath.Base(commonDir) == ".git" {
			return parent
		}
		commonDir = parent
	}
	return ""
}

// ── Package-level helpers (bypass Backend interface) ──────────────────────

// GitStatusAt checks whether the per-bead worktree exists and is clean.
// It uses `git status --porcelain` — empty output means clean. If the worktree
// directory does not exist, returns WorktreeStatus{Exists: false} and
// ErrWorktreeNotFound.
func GitStatusAt(ctx context.Context, worktreesDir, beadID string) (WorktreeStatus, error) {
	path := filepath.Join(worktreesDir, beadID)

	info, err := os.Lstat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return WorktreeStatus{Exists: false}, ErrWorktreeNotFound
		}
		return WorktreeStatus{}, fmt.Errorf("wt git: lstat worktree %q: %w", path, err)
	}
	if !info.IsDir() {
		return WorktreeStatus{Exists: false}, ErrWorktreeNotFound
	}

	// Check for changes using `git status --porcelain`.
	cmd := exec.CommandContext(ctx, "git", "status", "--porcelain")
	cmd.Dir = path
	out, err := cmd.CombinedOutput()
	if err != nil {
		if ctx.Err() != nil {
			return WorktreeStatus{}, fmt.Errorf("wt git: status aborted: %w", ctx.Err())
		}
		return WorktreeStatus{}, fmt.Errorf("wt git: git status in %q: %w\n%s", path, err, out)
	}

	clean := strings.TrimSpace(string(out)) == ""

	// Best-effort ahead/behind via `git rev-list --count @{u}...HEAD`.
	// Agent worktree branches (muster/<id>) typically have no upstream, so
	// 0 is the common case. Gracefully return 0 on any error (no upstream,
	// detached HEAD, etc.) rather than failing the Status call entirely.
	ahead, behind := gitAheadBehind(ctx, path)

	return WorktreeStatus{Exists: true, Clean: clean, Ahead: ahead, Behind: behind}, nil
}

// gitAheadBehind returns the (ahead, behind) counts vs the upstream branch.
// Returns (0, 0) gracefully when there is no upstream or on any error.
// Uses `git rev-list --count --left-right @{u}...HEAD` which emits
// "<behind>\t<ahead>" (left = upstream, right = HEAD).
func gitAheadBehind(ctx context.Context, wtPath string) (ahead, behind int) {
	cmd := exec.CommandContext(ctx, "git", "rev-list", "--count", "--left-right", "@{u}...HEAD")
	cmd.Dir = wtPath
	out, err := cmd.Output()
	if err != nil {
		// No upstream or other error — silently return 0.
		return 0, 0
	}
	// Output is "<left-count>\t<right-count>\n" where left=upstream (behind),
	// right=HEAD (ahead).
	parts := strings.Fields(strings.TrimSpace(string(out)))
	if len(parts) != 2 {
		return 0, 0
	}
	var b, a int
	if _, err := fmt.Sscanf(parts[0], "%d", &b); err != nil {
		return 0, 0
	}
	if _, err := fmt.Sscanf(parts[1], "%d", &a); err != nil {
		return 0, 0
	}
	return a, b
}

// GitDiffSummaryAt parses `git status --porcelain=v1 -z` in the bead's worktree.
// The NUL-delimited output format per entry:
//   - "XY PATH\0"           — non-rename entry
//   - "XY NEWPATH\0OLDPATH\0" — rename/copy entry (XY[0] = R or C)
//
// Untracked files (`??`) are reported as Added. This is non-mutating.
func GitDiffSummaryAt(ctx context.Context, worktreesDir, beadID string) ([]FileChange, error) {
	path := filepath.Join(worktreesDir, beadID)

	// Verify the worktree exists and is a directory. A non-dir path maps to
	// ErrWorktreeNotFound (404), matching GitStatusAt, rather than letting the
	// git command fail and surface as a generic 500.
	info, err := os.Lstat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, ErrWorktreeNotFound
		}
		return nil, fmt.Errorf("wt git: lstat worktree %q: %w", path, err)
	}
	if !info.IsDir() {
		return nil, ErrWorktreeNotFound
	}

	cmd := exec.CommandContext(ctx, "git", "status", "--porcelain=v1", "-z")
	cmd.Dir = path
	out, err := cmd.CombinedOutput()
	if err != nil {
		if ctx.Err() != nil {
			return nil, fmt.Errorf("wt git: diff summary aborted: %w", ctx.Err())
		}
		return nil, fmt.Errorf("wt git: git status in %q: %w\n%s", path, err, out)
	}

	return parsePorcelainV1Z(out), nil
}

// GitDiffAt returns a streaming unified diff for the bead's worktree.
// When path is empty, the diff covers all files (git diff HEAD + per-untracked
// --no-index). When path is non-empty it covers only that single file.
// path MUST already be validated by SafeRelPath before calling.
// The returned ReadCloser wraps child stdout; Close reaps the child (no zombies).
func GitDiffAt(ctx context.Context, worktreesDir, beadID, path string) (io.ReadCloser, error) {
	wtPath := filepath.Join(worktreesDir, beadID)

	// Verify the worktree exists and is a directory (non-dir → 404, not 500).
	info, err := os.Lstat(wtPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, ErrWorktreeNotFound
		}
		return nil, fmt.Errorf("wt git: lstat worktree %q: %w", wtPath, err)
	}
	if !info.IsDir() {
		return nil, ErrWorktreeNotFound
	}

	if path != "" {
		// Single-file diff.
		return diffSingleFile(ctx, wtPath, path)
	}
	// Whole-worktree diff.
	return diffAll(ctx, wtPath)
}

// parsePorcelainV1Z parses the NUL-delimited output of `git status --porcelain=v1 -z`.
//
// Each record is one of:
//   - "XY PATH\0"           — non-rename entry
//   - "XY NEWPATH\0OLDPATH\0" — rename/copy entry (XY[0] = R or C)
//
// The two-character XY prefix encodes index (X) and worktree (Y) state.
// We prioritise the worktree column (Y) for working-tree diffs, falling back
// to the index column (X) for staged-only changes.
func parsePorcelainV1Z(data []byte) []FileChange {
	// Split on NUL bytes. The output ends with a NUL after the last entry.
	parts := bytes.Split(data, []byte{0})
	var changes []FileChange

	for i := 0; i < len(parts); {
		entry := parts[i]
		if len(entry) == 0 {
			i++
			continue
		}
		if len(entry) < 4 {
			// Malformed entry; skip with warning.
			slog.Warn("wt git: skipping malformed porcelain entry", "entry", string(entry))
			i++
			continue
		}

		xy := string(entry[:2])
		filePart := string(entry[3:]) // skip "XY " (first 3 chars)

		x := rune(xy[0])
		y := rune(xy[1])

		// Rename/copy entries consume an extra NUL-delimited token for the old path.
		if x == 'R' || x == 'C' || y == 'R' || y == 'C' {
			kind := Renamed
			if x == 'C' || y == 'C' {
				kind = Copied
			}
			oldPath := ""
			if i+1 < len(parts) {
				oldPath = string(parts[i+1])
				i += 2 // consume both tokens
			} else {
				i++
			}
			changes = append(changes, FileChange{
				Path:    filePart,
				OldPath: oldPath,
				Kind:    kind,
			})
			continue
		}

		i++

		// Map XY to ChangeKind. Prioritise the worktree column (Y), fall back
		// to the index column (X) for fully staged changes.
		kind, ok := porcelainKind(x, y)
		if !ok {
			slog.Warn("wt git: unrecognized porcelain status; skipping", "xy", xy)
			continue
		}
		changes = append(changes, FileChange{
			Path: filePart,
			Kind: kind,
		})
	}
	return changes
}

// porcelainKind maps a two-character XY status code to a ChangeKind.
// Returns (kind, true) on success or ("", false) for unrecognized codes.
func porcelainKind(x, y rune) (ChangeKind, bool) {
	// Untracked files.
	if x == '?' && y == '?' {
		return Added, true
	}

	// Worktree state takes priority.
	switch y {
	case 'M':
		return Modified, true
	case 'D':
		return Deleted, true
	case 'A':
		return Added, true
	}

	// Fall back to index state.
	switch x {
	case 'A', 'C':
		return Added, true
	case 'M':
		return Modified, true
	case 'D':
		return Deleted, true
	}

	return "", false
}

// diffSingleFile returns the diff for one validated worktree-relative path.
// If the file is untracked, uses `git diff --no-index -- /dev/null <path>`.
// Otherwise uses `git diff HEAD -- <path>`.
func diffSingleFile(ctx context.Context, wtPath, relPath string) (io.ReadCloser, error) {
	// Determine if the file is untracked (status == "??").
	statusCmd := exec.CommandContext(ctx, "git", "status", "--porcelain=v1", "-z", "--", relPath)
	statusCmd.Dir = wtPath
	statusOut, err := statusCmd.CombinedOutput()
	if err != nil && ctx.Err() != nil {
		return nil, fmt.Errorf("wt git: diff single status aborted: %w", ctx.Err())
	}

	untracked := false
	for _, entry := range bytes.Split(statusOut, []byte{0}) {
		if len(entry) >= 2 && entry[0] == '?' && entry[1] == '?' {
			untracked = true
			break
		}
	}

	var cmd *exec.Cmd
	if untracked {
		// `git diff --no-index` follows symlinks, so an untracked symlink could
		// leak files outside the worktree. Refuse to diff an unsafe path; an
		// empty diff is the safe, non-erroring response.
		if !untrackedDiffSafe(wtPath, relPath) {
			return io.NopCloser(strings.NewReader("")), nil
		}
		cmd = exec.CommandContext(ctx, "git", "diff", "--no-index", "--", "/dev/null", relPath)
	} else {
		cmd = exec.CommandContext(ctx, "git", "diff", "HEAD", "--", relPath)
	}
	cmd.Dir = wtPath

	return startStreamingCmd(cmd, true)
}

// untrackedDiffSafe reports whether an untracked worktree-relative path is safe
// to feed to `git diff --no-index`, which follows symlinks. It rejects a leaf
// symlink and any path whose symlink-resolved location escapes the worktree,
// preventing exfiltration of files outside the worktree via a malicious symlink
// (e.g. an agent creating `leak -> /etc/passwd` inside the worktree).
func untrackedDiffSafe(wtPath, relPath string) bool {
	full := filepath.Join(wtPath, relPath)
	fi, err := os.Lstat(full)
	if err != nil {
		return false
	}
	if fi.Mode()&os.ModeSymlink != 0 {
		return false // leaf is a symlink — refuse to follow it
	}
	real, err := filepath.EvalSymlinks(full)
	if err != nil {
		return false
	}
	base, err := filepath.EvalSymlinks(wtPath)
	if err != nil {
		return false
	}
	return real == base || strings.HasPrefix(real, base+string(filepath.Separator))
}

// diffAll returns the diff for the whole worktree: `git diff HEAD` for tracked
// changes, then `git diff --no-index -- /dev/null <f>` appended per untracked
// file. This is non-mutating (no git add -N).
func diffAll(ctx context.Context, wtPath string) (io.ReadCloser, error) {
	// Collect untracked files first.
	summaryCmd := exec.CommandContext(ctx, "git", "status", "--porcelain=v1", "-z")
	summaryCmd.Dir = wtPath
	summaryOut, err := summaryCmd.CombinedOutput()
	if err != nil {
		if ctx.Err() != nil {
			return nil, fmt.Errorf("wt git: diff all status aborted: %w", ctx.Err())
		}
		return nil, fmt.Errorf("wt git: git status in %q: %w\n%s", wtPath, err, summaryOut)
	}

	var untracked []string
	for _, entry := range bytes.Split(summaryOut, []byte{0}) {
		if len(entry) >= 4 && entry[0] == '?' && entry[1] == '?' {
			relPath := string(entry[3:])
			// Skip symlinked/escaping untracked entries: `git diff --no-index`
			// follows symlinks and would leak files outside the worktree.
			if !untrackedDiffSafe(wtPath, relPath) {
				continue
			}
			untracked = append(untracked, relPath)
		}
	}

	// Build a multi-reader: first git diff HEAD, then one --no-index per untracked.
	readers := make([]io.Reader, 0, 1+len(untracked))

	// Tracked diff (may be empty if there are only untracked files).
	headCmd := exec.CommandContext(ctx, "git", "diff", "HEAD")
	headCmd.Dir = wtPath
	headRC, err := startStreamingCmd(headCmd, true)
	if err != nil {
		return nil, err
	}
	readers = append(readers, headRC)

	// Per-untracked-file diff.
	untrackedRCs := make([]io.ReadCloser, 0, len(untracked))
	for _, f := range untracked {
		niCmd := exec.CommandContext(ctx, "git", "diff", "--no-index", "--", "/dev/null", f)
		niCmd.Dir = wtPath
		rc, err := startStreamingCmd(niCmd, true)
		if err != nil {
			// Clean up already-opened readers.
			_ = headRC.Close()
			for _, prev := range untrackedRCs {
				_ = prev.Close()
			}
			return nil, fmt.Errorf("wt git: diff --no-index for %q: %w", f, err)
		}
		untrackedRCs = append(untrackedRCs, rc)
		readers = append(readers, rc)
	}

	return &multiReadCloser{
		r:       io.MultiReader(readers...),
		closers: append([]io.ReadCloser{headRC}, untrackedRCs...),
	}, nil
}

// multiReadCloser combines multiple ReadClosers into a single ReadCloser.
// Reads are multiplexed via an io.MultiReader; Close closes all children.
type multiReadCloser struct {
	r       io.Reader
	closers []io.ReadCloser
}

func (m *multiReadCloser) Read(p []byte) (int, error) {
	return m.r.Read(p)
}

func (m *multiReadCloser) Close() error {
	var last error
	for _, c := range m.closers {
		if err := c.Close(); err != nil {
			last = err
		}
	}
	return last
}

// streamingCmd is a ReadCloser that wraps a running child process.
// Read reads from the child's stdout; Close closes stdout (unblocking any Read)
// and calls Wait to reap the child (no zombies), surfacing a genuine non-zero
// exit. Close does not itself signal the child — cancelling the ctx passed to
// exec.CommandContext is what kills a still-running process.
type streamingCmd struct {
	cmd    *exec.Cmd
	stdout io.ReadCloser
	// tolerateExit1 is true for `git diff` commands, which exit 1 when there ARE
	// differences (expected, not a failure). jj diff exits 0 on success, so its
	// exit 1 is a real error and must not be tolerated.
	tolerateExit1 bool
}

func (s *streamingCmd) Read(p []byte) (int, error) {
	return s.stdout.Read(p)
}

func (s *streamingCmd) Close() error {
	// Close stdout first so any blocked Read unblocks.
	_ = s.stdout.Close()
	// Reap the child. Surface a genuine failure so callers/tests can detect a
	// broken worktree — but tolerate `git diff`'s exit 1 (differences found),
	// which is expected. The HTTP handler ignores this (output already streamed);
	// tests and multiReadCloser aggregation rely on it.
	err := s.cmd.Wait()
	if err == nil {
		return nil
	}
	var exitErr *exec.ExitError
	if s.tolerateExit1 && errors.As(err, &exitErr) && exitErr.ExitCode() == 1 {
		return nil
	}
	return err
}

// startStreamingCmd starts cmd, pipes its stdout, and returns a ReadCloser.
// The caller must close the returned ReadCloser to reap the child. Set
// tolerateExit1 for `git diff` commands (exit 1 = differences found, expected).
func startStreamingCmd(cmd *exec.Cmd, tolerateExit1 bool) (io.ReadCloser, error) {
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("wt: stdout pipe: %w", err)
	}
	// Discard stderr so it doesn't mix into the diff stream.
	cmd.Stderr = nil

	if err := cmd.Start(); err != nil {
		_ = stdout.Close()
		return nil, fmt.Errorf("wt: start %v: %w", cmd.Args, err)
	}
	return &streamingCmd{cmd: cmd, stdout: stdout, tolerateExit1: tolerateExit1}, nil
}
