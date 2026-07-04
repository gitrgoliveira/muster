# Data Model: M3 — Worktrees (`wt.Backend`)

Types introduced in `internal/wt`. Pure domain types (Constitution III: `core`-like layer);
no persistence — all reconstructable from the VCS on disk.

## `VCS` (backend selector)

```go
type VCS string

const (
    VCSGit VCS = "git"
    VCSJJ  VCS = "jj"
)

func (v VCS) Valid() bool // allow-list: git | jj
```

- Source in M3: the `--default-vcs` config value (default `VCSGit`). No per-bead field.
- `config` rejects any non-allow-listed value at startup (FR-018), mirroring M2's
  `--default-permission-mode` allow-list.

## `Backend` (the interface — full roadmap §8 surface)

```go
type Backend interface {
    // Implemented + tested in M3 (read + create):
    Status(ctx context.Context, beadID string) (WorktreeStatus, error)
    Create(ctx context.Context, worktreesDir, srcRepo, beadID string) (path string, err error)
    DiffSummary(ctx context.Context, beadID string) ([]FileChange, error)
    Diff(ctx context.Context, beadID, path string) (io.ReadCloser, error) // path == "" ⇒ whole worktree

    // Declared in M3, return ErrNotImplemented; filled in M4 (write side):
    Finalize(ctx context.Context, beadID, msg string) error
    Push(ctx context.Context, beadID string) error
    Remove(ctx context.Context, beadID string) error
}
```

- Every method takes `context.Context` (consistent with M2's `worktree.Ensure` — bounds
  the shelled-out subprocess; cancellation kills the child).
- `Diff` returns an `io.ReadCloser` streamed straight to the HTTP response (FR-008); the
  caller closes it. Reader wraps the child process stdout; closing waits/reaps the child.

### Resolver

```go
func For(v VCS) (Backend, error) // VCSGit → gitBackend{}, VCSJJ → jjBackend{}, else error
```

### Availability probe (startup)

```go
type Availability struct {
    Git     bool
    JJ      bool
    JJVer   string // best-effort, for status DTO
}
func Detect(ctx context.Context) Availability // `git --version`, `jj --version`
```

- Per-repo jj-native check is separate (jj backend's `Create`/`Status` run `jj root` in the
  source repo; FR-004a). `Detect` only reports binary presence for the status DTO/picker.

## `WorktreeStatus`

```go
type WorktreeStatus struct {
    Exists bool
    Clean  bool  // no changes vs base
    Ahead  int   // best-effort; 0 if not computed in M3
    Behind int   // best-effort; 0 if not computed in M3
}
```

- `Exists=false` ⇒ endpoints return `404 WORKTREE_NOT_FOUND` (FR-009).
- git: `git status --porcelain` empty ⇒ `Clean`; ahead/behind via `git rev-list --count`
  (best-effort, US4/P3 — may be left 0 in M3).
- jj: `jj status` "no changes" ⇒ `Clean`.

## `FileChange` + `ChangeKind`

```go
type ChangeKind string

const (
    Added    ChangeKind = "added"
    Modified ChangeKind = "modified"
    Deleted  ChangeKind = "deleted"
    Renamed  ChangeKind = "renamed"
    Copied   ChangeKind = "copied"
)

type FileChange struct {
    Path    string     `json:"path"`
    OldPath string     `json:"oldPath,omitempty"` // set for Renamed/Copied
    Kind    ChangeKind `json:"kind"`
}
```

### Change-kind mapping (from research.md §3)

| Native | git source (`git status --porcelain=v1 -z`) | jj source (`jj diff --summary`) | → `ChangeKind` |
|---|---|---|---|
| added | `A `, `??` (untracked), `AM` | `A ` | `Added` |
| modified | ` M`, `M `, `MM` | `M ` | `Modified` |
| deleted | ` D`, `D ` | `D ` | `Deleted` |
| renamed | `R  old -> new` | `R old new` | `Renamed` (set `OldPath`) |
| copied | `C  old -> new` | `C old new` | `Copied` (set `OldPath`) |

- git `porcelain` two-char `XY` status: treat `??` as `Added`; take the worktree column
  (`Y`) for working-tree state, falling back to the index column (`X`). Defensive default
  for an unrecognized code: skip with a logged warning, never panic.
- Separate tokenizers per backend (git uses NUL-delimited `-z`, TAB in rename arrows; jj
  uses space-delimited) — do **not** share the parser (research.md §3).

## Errors (sentinels)

```go
var (
    ErrNotImplemented   = errors.New("wt: not implemented in M3")   // Finalize/Push/Remove
    ErrWorktreeNotFound = errors.New("wt: worktree does not exist") // → HTTP 404
    ErrVCSUnavailable   = errors.New("wt: selected VCS backend unavailable") // → HTTP 412
)
```

- Handlers map `ErrWorktreeNotFound` → `404 {"error":{"code":"WORKTREE_NOT_FOUND"}}`.
- Dispatch maps `ErrVCSUnavailable` → `412 {"error":{"code":"VCS_UNAVAILABLE"}}`
  (roadmap error table; joins M2's dispatch error mapping).
- `?path=` validation failure → `400 {"error":{"code":"INVALID_PATH"}}`.

## Path safety (`?path=`, FR-007)

```go
func safeRelPath(worktree, path string) (string, error)
```

- Reject if `path` is absolute, not `filepath.IsLocal`, or resolves (via `filepath.Join` +
  `filepath.Clean`) outside `worktree`. Return the cleaned worktree-relative path; the
  backend passes only this validated value to `git`/`jj` (never the raw query value).
