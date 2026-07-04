# Contract — `wt.Backend` write-side (M4)

Fills the three methods M3 declared and stubbed with `ErrNotImplemented`. **No interface change** — the M3 signatures are honored verbatim, so no caller changes (M3 designed for this). Both git and jj implement all three. Each behavior is TDD'd with a fake-on-`$PATH` unit test **and** a skip-gated real-binary integration test.

```go
Finalize(ctx context.Context, beadID, message string) error
Push(ctx context.Context, beadID string) error   // remote resolved from config (default origin)
Remove(ctx context.Context, beadID string) error
```

## Finalize(beadID, message)

| Backend | Behavior |
|---|---|
| **git** | `git status --porcelain` in the worktree; **empty ⇒ no-op success** (no commit). Else `git add -A` + `git commit -m <message>` on branch `muster/<beadID>`. |
| **jj** | Working copy auto-snapshots; **empty diff (`jj diff --summary`) ⇒ no-op success**. Else `jj describe -m <message>` (+ new working revision) so the change is a committed revision on `muster/<beadID>`. *(exact incantation pinned by the R6 spike)* |

- Postcondition (non-empty): the worktree's VCS log shows a commit/revision with `message`.
- No-change postcondition: no new commit; success reported (idempotent, retry-safe).
- `VCS_UNAVAILABLE` if the backend binary is absent (FR-009).

## Push(beadID)

| Backend | Behavior |
|---|---|
| **git** | `git push <remote> muster/<beadID>` (remote default `origin`, configurable). |
| **jj** | `jj git push --branch muster/<beadID>` (or `git push <remote> muster/<beadID>` on the colocated repo) *(pinned by R6 spike)*. |

- Success ⇒ branch `muster/<beadID>` exists on the remote (integration test pushes to a **bare** upstream and asserts).
- Missing remote / auth failure / rejected ⇒ **explicit typed error**, never silent success (FR-007).
- muster never creates or authenticates a remote and stores no credentials (Constitution — no credential handling).

## Remove(beadID)

| Backend | Behavior |
|---|---|
| **git** | `git worktree remove <path>` (+ prune); working tree gone, metadata cleaned. |
| **jj** | `jj workspace forget <name>` + remove the workspace directory *(pinned by R6 spike)*. |

- Postcondition: a subsequent `Status(beadID)` reports the worktree **absent**.
- `VCS_UNAVAILABLE` if the backend binary is absent.
- On-demand only — this is **not** scheduled GC (that is M9).

## Safety

- Finalize/Remove must not run while the bead's step agent is **active** — the service guards on run state and returns `CodeRunAlreadyActive` rather than racing a live agent (consistent with M3's non-mutating-read rule). This is the write-side analogue of M3's "don't race the agent" invariant.
