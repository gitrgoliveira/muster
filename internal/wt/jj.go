// jj backend for the wt package. It only shells out to the `jj` binary, so it
// is platform-agnostic and compiles everywhere; when jj is absent (e.g. on
// Windows, which is not part of the supported toolchain) Detect reports it
// unavailable and operations surface ErrVCSUnavailable. The jj *tests* are
// Unix-only (they pin JJ_CONFIG=/dev/null for hermeticity) — see jj_test.go.

package wt

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"sync"
)

// jjSrcRepoDir resolves the shared jj source-repo directory for a given workspace.
//
// In a jj colocated repo, the workspace's .jj/repo file contains a relative path
// to the shared repo's .jj/repo directory (e.g. "../../../srcrepo/.jj/repo").
// Resolving that path and stripping the "/.jj/repo" suffix yields the srcrepo root,
// where the colocated .git directory lives and where `git push` must run.
//
// This is intentionally a pure file-read — no jj invocation — so it works even
// when jj is not installed (the workspace directory was created by one that was).
//
// It always returns a filepath.Clean-ed path (Dir of a Clean path), and absolute
// whenever wtPath is absolute.
func jjSrcRepoDir(wtPath string) (string, error) {
	repoFile := filepath.Join(wtPath, ".jj", "repo")
	raw, err := os.ReadFile(repoFile)
	if err != nil {
		return "", fmt.Errorf("wt jj: read .jj/repo in %q: %w", wtPath, err)
	}
	// The file contains a relative path from the workspace's .jj directory
	// to the shared repo's .jj/repo directory, e.g. "../../../srcrepo/.jj/repo".
	// Resolve it relative to the workspace's own .jj directory.
	rel := strings.TrimSpace(string(raw))
	// sharedJJRepoDir is the .jj/repo directory of the srcrepo.
	sharedJJRepoDir := filepath.Clean(filepath.Join(wtPath, ".jj", rel))
	// srcrepo root = sharedJJRepoDir/../../ (strip the .jj/repo suffix).
	// filepath.Dir twice: .../srcrepo/.jj/repo → .../srcrepo/.jj → .../srcrepo
	srcRepo := filepath.Dir(filepath.Dir(sharedJJRepoDir))
	return srcRepo, nil
}

// checkJJWorkspaceDir verifies the workspace directory exists and is a
// directory, returning ErrWorktreeNotFound (wrapped with the bead ID) for
// absent or non-directory paths. jj sibling of git.go's checkWorktreeDir.
func checkJJWorkspaceDir(wtPath, beadID string) error {
	info, err := os.Lstat(wtPath)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("wt jj: workspace %q not found: %w", beadID, ErrWorktreeNotFound)
		}
		return fmt.Errorf("wt jj: lstat workspace %q: %w", wtPath, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("wt jj: workspace %q not found: %w", beadID, ErrWorktreeNotFound)
	}
	return nil
}

// jjBackend implements Backend using the jj VCS. The write-side (Finalize,
// Push, Remove) is implemented in M4 (was ErrNotImplemented in M3).
//
// The backend is jj-source-repos-only (FR-004a): Create probes `jj root` in
// the source repo; if it is not jj-native it returns ErrVCSUnavailable
// (no colocate, no silent fallback to git).
//
// Like gitBackend, the interface-level Status/DiffSummary/Diff methods require
// worktreesDir to be baked in via NewJJBackend(worktreesDir). The exported
// helpers JJStatusAt/JJDiffSummaryAt/JJDiffAt accept worktreesDir explicitly
// and are used by tests and the service layer.
//
// Security: Push and Remove resolve the srcRepo path via the srcRepos cache,
// which is populated by Create before the agent runs. The fallback to
// jjSrcRepoDir (reading the agent-writable .jj/repo file) is used only when
// the cache has no entry (e.g. the backend was reconstructed after the fact).
// When allowedRepos is non-nil (populated from the orchestrator's repo-map
// values), the fallback is further restricted to paths in the allow-list.
type jjBackend struct {
	worktreesDir string
	// srcRepos caches beadID → srcRepo path as populated by Create.
	// Push and Remove prefer this over re-reading the agent-writable .jj/repo.
	srcRepos sync.Map
	// allowedRepos is an optional allow-list of absolute source-repo paths.
	// When non-nil (length > 0), the resolveSrcRepo fallback (reading the
	// agent-writable .jj/repo) rejects any path not in the list. A nil or
	// empty slice disables the allow-list check (open set). The cache hit
	// path is never restricted — the orchestrator controls what goes into
	// the cache via Create.
	allowedRepos []string
}

// NewJJBackend returns a jj Backend with worktreesDir baked in and no
// allow-list restriction (any valid, non-traversal srcRepo is permitted).
func NewJJBackend(worktreesDir string) Backend {
	return NewJJBackendAllowed(worktreesDir, nil)
}

// NewJJBackendAllowed returns a jj Backend with worktreesDir baked in and an
// explicit allow-list for the resolveSrcRepo fallback path. When allowedRepos
// is non-nil and non-empty, any srcRepo resolved from the agent-writable
// .jj/repo file that is not in the list is rejected with an error. Pass nil
// to disable the allow-list (equivalent to NewJJBackend).
//
// Callers should pass the values of the orchestrator's RepoMap so the fallback
// is restricted to operator-declared source repos — this prevents a compromised
// agent from redirecting Push/Remove to an arbitrary directory by tampering with
// .jj/repo.
func NewJJBackendAllowed(worktreesDir string, allowedRepos []string) Backend {
	return &jjBackend{worktreesDir: worktreesDir, allowedRepos: allowedRepos}
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

	// If the workspace already exists, return it (reuse) and refresh the cache.
	if info, err := os.Lstat(wtPath); err == nil {
		if info.IsDir() {
			j.srcRepos.Store(beadID, srcRepo)
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

	// Cache srcRepo before the agent starts so Push/Remove can use the trusted
	// value rather than re-reading the agent-writable .jj/repo file (FIX 4).
	j.srcRepos.Store(beadID, srcRepo)

	return wtPath, nil
}

// resolveSrcRepo returns the srcRepo path for beadID. It checks the in-memory
// cache (populated by Create) first — this is the security-preferred path that
// avoids reading the agent-writable .jj/repo file. Falls back to jjSrcRepoDir
// only when no cached entry exists (e.g. backend was reconstructed).
//
// Fallback security: after jjSrcRepoDir resolves a path from the agent-writable
// .jj/repo file, we validate that the result is absolute, has no remaining ".."
// segments after filepath.Clean, and is NOT inside the worktrees directory
// (a workspace is never a source repo). Any violation returns an error to
// prevent path-traversal attacks through a tampered .jj/repo.
//
// Additionally, when j.allowedRepos is non-empty (populated from the
// orchestrator's repo-map values), the fallback is restricted to paths in the
// allow-list. The cache hit path is exempt because the orchestrator controls
// the cache via Create (only operator-declared repos ever enter it).
func (j *jjBackend) resolveSrcRepo(beadID, wtPath string) (string, error) {
	if v, ok := j.srcRepos.Load(beadID); ok {
		return v.(string), nil
	}
	// Fallback: read the agent-writable .jj/repo file.
	srcRepo, err := jjSrcRepoDir(wtPath)
	if err != nil {
		return "", err
	}
	// Validate the resolved path: must be absolute. A non-absolute result would
	// indicate a bug in jjSrcRepoDir (which always returns a Clean, absolute path
	// when wtPath is absolute). The real defences are the not-under-worktreesDir
	// check and the allow-list below.
	cleaned := filepath.Clean(srcRepo)
	if !filepath.IsAbs(cleaned) {
		return "", fmt.Errorf("wt jj: resolved srcRepo %q failed validation (possible path traversal)", srcRepo)
	}
	// Must not be under the worktrees directory (a workspace is not a source repo).
	if j.worktreesDir != "" {
		wtDir := filepath.Clean(j.worktreesDir)
		if strings.HasPrefix(cleaned+string(filepath.Separator), wtDir+string(filepath.Separator)) || cleaned == wtDir {
			return "", fmt.Errorf("wt jj: resolved srcRepo %q failed validation (possible path traversal)", srcRepo)
		}
	}
	// Allow-list check: when the allow-list is non-empty, reject any srcRepo that
	// is not in it. The allow-list contains operator-declared repo paths (RepoMap
	// values); an agent cannot add to it. Cache hits (above) bypass this check.
	if len(j.allowedRepos) > 0 && !slices.Contains(j.allowedRepos, cleaned) {
		return "", fmt.Errorf("wt jj: resolved srcRepo %q is not in allowed repos list", cleaned)
	}
	return cleaned, nil
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

// Finalize seals the working-copy changes with message and starts a new empty WC.
//
// Algorithm (pinned in research.md R6):
//  1. Check `jj diff --summary` — empty output means no changes → no-op (false, nil).
//  2. `jj describe -m <message>` to set the description on the current WC change.
//  3. `jj new` to advance the WC past the now-sealed revision.
//
// Returns (true, nil) when a revision was sealed; (false, nil) when the
// workspace had no changes (no-op).
func (j *jjBackend) Finalize(ctx context.Context, beadID, message string) (bool, error) {
	wtPath := filepath.Join(j.worktreesDir, beadID)

	// Verify workspace exists and is a directory.
	if err := checkJJWorkspaceDir(wtPath, beadID); err != nil {
		return false, err
	}

	// No-op detection: jj diff --summary empty → nothing to commit.
	diffCmd := exec.CommandContext(ctx, "jj", "diff", "--summary")
	diffCmd.Dir = wtPath
	out, err := diffCmd.CombinedOutput()
	if err != nil {
		if ctx.Err() != nil {
			return false, fmt.Errorf("wt jj: diff --summary aborted: %w", ctx.Err())
		}
		return false, fmt.Errorf("wt jj: jj diff --summary in %q: %w\n%s", wtPath, err, out)
	}
	if strings.TrimSpace(string(out)) == "" {
		// No changes — Finalize is a no-op (success, no revision sealed).
		return false, nil
	}

	// Seal: describe the WC change with the message.
	descCmd := exec.CommandContext(ctx, "jj", "describe", "-m", message)
	descCmd.Dir = wtPath
	if out, err := descCmd.CombinedOutput(); err != nil {
		if ctx.Err() != nil {
			return false, fmt.Errorf("wt jj: describe aborted: %w", ctx.Err())
		}
		return false, fmt.Errorf("wt jj: jj describe in %q: %w\n%s", wtPath, err, out)
	}

	// Advance past the sealed revision.
	newCmd := exec.CommandContext(ctx, "jj", "new")
	newCmd.Dir = wtPath
	if out, err := newCmd.CombinedOutput(); err != nil {
		if ctx.Err() != nil {
			return false, fmt.Errorf("wt jj: new aborted: %w", ctx.Err())
		}
		return false, fmt.Errorf("wt jj: jj new in %q: %w\n%s", wtPath, err, out)
	}
	return true, nil
}

// Push pushes the bead's sealed revision to remote (resolved via
// ResolveRemote — empty defaults to "origin").
//
// Algorithm (pinned in research.md R6):
//  1. Set bookmark muster/<beadID> at the sealed parent revision (@-).
//  2. Export jj bookmarks to git refs via `jj git export`.
//  3. Resolve the shared srcrepo and push the branch via `git push <remote> <branch>`.
//
// Step 1 uses `jj bookmark set` (not `create`) so Push is idempotent: `set`
// moves an existing bookmark to the new revision, whereas `create` would fail
// if the bookmark already exists. Note: `jj bookmark set` may refuse a backward
// move without --allow-backwards; that is acceptable because we only call Push
// at the same or a newer revision (never moving a bookmark backward intentionally).
//
// jj's `jj git push --bookmark` is avoided because it requires a user identity
// in the jj config (fails with no-author error when JJ_CONFIG=/dev/null or
// in environments without jj user config). Doing the export+git push directly
// sidesteps this requirement.
func (j *jjBackend) Push(ctx context.Context, beadID, remote string) error {
	wtPath := filepath.Join(j.worktreesDir, beadID)

	// Verify workspace exists and is a directory.
	if err := checkJJWorkspaceDir(wtPath, beadID); err != nil {
		return err
	}

	branch := BranchName(beadID)

	// 1. Set jj bookmark at @- (the sealed revision, not the empty WC).
	// Use `bookmark set` instead of `bookmark create` for idempotency: `set`
	// creates the bookmark when absent and moves it when present, so a second
	// Push call for the same bead succeeds rather than hard-failing on a
	// "bookmark already exists" error.
	bmCmd := exec.CommandContext(ctx, "jj", "bookmark", "set", branch, "-r", "@-")
	bmCmd.Dir = wtPath
	if out, err := bmCmd.CombinedOutput(); err != nil {
		if ctx.Err() != nil {
			return fmt.Errorf("wt jj: bookmark set aborted: %w", ctx.Err())
		}
		return fmt.Errorf("wt jj: jj bookmark set %q: %w\n%s", branch, err, out)
	}

	// 2. Export jj bookmarks to git refs so the branch is visible to git push.
	exportCmd := exec.CommandContext(ctx, "jj", "git", "export")
	exportCmd.Dir = wtPath
	if out, err := exportCmd.CombinedOutput(); err != nil {
		if ctx.Err() != nil {
			return fmt.Errorf("wt jj: git export aborted: %w", ctx.Err())
		}
		return fmt.Errorf("wt jj: jj git export: %w\n%s", err, out)
	}

	// 3. Resolve the shared srcrepo: use the cache (populated by Create) to
	// avoid reading the agent-writable .jj/repo file (FIX 4, tri-review #13).
	srcRepo, err := j.resolveSrcRepo(beadID, wtPath)
	if err != nil {
		return fmt.Errorf("wt jj: resolving srcrepo for push: %w", err)
	}

	// 4. Use `git push` from the srcrepo (colocated .git is there).
	resolvedRemote, err := ResolveRemote(remote)
	if err != nil {
		return err
	}
	pushCmd := exec.CommandContext(ctx, "git", "push", resolvedRemote, branch)
	pushCmd.Dir = srcRepo
	if out, err := pushCmd.CombinedOutput(); err != nil {
		if ctx.Err() != nil {
			return fmt.Errorf("wt jj: git push aborted: %w", ctx.Err())
		}
		return fmt.Errorf("wt jj: git push %s %s: %w\n%s", resolvedRemote, branch, err, out)
	}
	return nil
}

// Remove forgets the jj workspace and deletes its directory.
//
// Algorithm (pinned in research.md R6):
//  1. Verify the workspace directory exists.
//  2. Resolve the srcrepo via `jj root` (needed for `jj workspace forget`).
//  3. Run `jj workspace forget <beadID>` from the srcrepo to deregister it.
//  4. Remove the directory with os.RemoveAll.
//
// Note: unlike the git backend, jj Remove does NOT check for uncommitted changes
// before removing. jj auto-snapshots all file changes into a "working copy commit"
// on every command, so there is no uncommitted-index concept in jj workspaces.
// ErrWorktreeDirty is only applicable to git worktrees.
func (j *jjBackend) Remove(ctx context.Context, beadID string) error {
	wtPath := filepath.Join(j.worktreesDir, beadID)

	// Verify workspace exists and is a directory.
	if err := checkJJWorkspaceDir(wtPath, beadID); err != nil {
		return err
	}

	// Resolve srcrepo via cache (preferred) or .jj/repo fallback (see FIX 4).
	srcRepo, err := j.resolveSrcRepo(beadID, wtPath)
	if err != nil {
		return fmt.Errorf("wt jj: resolving srcrepo for remove: %w", err)
	}

	// Forget the workspace: beadID is the workspace name (basename of wtPath).
	forgetCmd := exec.CommandContext(ctx, "jj", "workspace", "forget", beadID)
	forgetCmd.Dir = srcRepo
	if out, err := forgetCmd.CombinedOutput(); err != nil {
		if ctx.Err() != nil {
			return fmt.Errorf("wt jj: workspace forget aborted: %w", ctx.Err())
		}
		return fmt.Errorf("wt jj: jj workspace forget %q: %w\n%s", beadID, err, out)
	}

	// Remove the workspace directory.
	if err := os.RemoveAll(wtPath); err != nil {
		return fmt.Errorf("wt jj: remove workspace dir %q: %w", wtPath, err)
	}
	return nil
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
		// "--" terminates option parsing so a path like "-x"/"--help" can't be
		// read as a jj flag (argument injection). SafeRelPath also rejects a
		// leading "-" as defense in depth.
		args = []string{"diff", "--git", "--", path}
	} else {
		args = []string{"diff", "--git"}
	}

	cmd := exec.CommandContext(ctx, "jj", args...)
	cmd.Dir = wtPath

	return startStreamingCmd(cmd, false)
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
