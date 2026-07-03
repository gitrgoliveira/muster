# Contract: `wt.Backend` interface

The single seam the orchestrator and diff endpoints depend on. Two concrete impls:
`gitBackend` (wraps M2 `internal/worktree`) and `jjBackend`. `wt.For(vcs)` returns one.

## Method contracts

### `Create(ctx, worktreesDir, srcRepo, beadID) (path, error)`

| | git | jj |
|---|---|---|
| Command | `git worktree add -b muster/<beadID> <path>` (delegates to `worktree.Ensure` — all M2 guards apply) | `jj workspace add <path>` run in `srcRepo` |
| Pre-check | `srcRepo` is a git repo (M2 error otherwise) | `jj root` in `srcRepo` succeeds (jj-native); else `ErrVCSUnavailable` |
| Reuse | existing valid worktree reused after repo-match/branch-match/symlink guards (M2) | existing workspace reused; path/name = `<worktreesDir>/<beadID>` |
| Path | `<worktreesDir>/<beadID>` | `<worktreesDir>/<beadID>` (same layout) |

- **Behavior-preserving for git (SC-001)**: `gitBackend.Create` is a thin adapter over
  `worktree.Ensure(ctx, worktreesDir, srcRepo, beadID)`; its error set and reuse semantics
  are identical to M2.
- beadID path-safety guard (single local segment) applies to both backends.

### `Status(ctx, beadID) (WorktreeStatus, error)`

- `Exists=false` when no worktree directory for the bead → callers return `404`.
- git: `git status --porcelain` (empty ⇒ `Clean`); jj: `jj status`.
- Ahead/behind is best-effort (US4/P3); `0` acceptable in M3.

### `DiffSummary(ctx, beadID) ([]FileChange, error)`

| git | jj |
|---|---|
| `git status --porcelain=v1 -z` in the worktree (includes untracked `??`; **non-mutating**) | `jj diff --summary` in the workspace |

- Parse per the change-kind table (data-model.md). Returns `ErrWorktreeNotFound` if the
  worktree doesn't exist.

### `Diff(ctx, beadID, path) (io.ReadCloser, error)`

| | git | jj |
|---|---|---|
| whole (`path==""`) | `git diff HEAD` **+** `git diff --no-index -- /dev/null <f>` appended per untracked file | `jj diff --git` |
| single file | `git diff HEAD -- <path>` (or `--no-index` if the file is untracked) | `jj diff --git <path>` |

- Output is **git-format unified diff** for both (spike-verified byte-compatible).
- Streamed: the `ReadCloser` wraps child stdout; `Close()` reaps the child. Never buffer
  the whole diff.
- `path` MUST already be validated by `safeRelPath` (FR-007) before reaching the backend.
- `ErrWorktreeNotFound` if no worktree.

### `Finalize` / `Push` / `Remove` — M3: return `ErrNotImplemented`

Declared to match roadmap §8; implemented in M4. Callers must not depend on them in M3.

## Invariants

1. No method mutates the agent's working tree or index as a side effect of a **read**
   (`Status`/`DiffSummary`/`Diff`). (This is why git uses `status --porcelain`, not
   `add -N`.)
2. No silent cross-backend fallback: a `jj` selection never runs git, and vice versa
   (FR-011/FR-004a).
3. Both backends share the `<worktreesDir>/<beadID>` on-disk layout and the
   `muster/<beadID>` naming convention so recovery/GC (later milestones) stay uniform.
4. Every subprocess is `exec.CommandContext`-bound (cancellation kills the child), matching
   M2's `worktree.Ensure` discipline.
