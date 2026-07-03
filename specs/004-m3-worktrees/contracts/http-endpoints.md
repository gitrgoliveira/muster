# Contract: HTTP endpoints (M3, additive over M2)

Two new read endpoints + additive status DTO fields. **No M0/M1/M2 endpoint changes**
(Constitution V, SC-007). Both new routes reuse M1 middleware (ID validation, body limits
where relevant).

## `GET /api/v1/beads/{id}/worktree`

File list + change summary for the bead's worktree.

- **200** — worktree exists:
  ```json
  {
    "beadID": "mp-abc",
    "vcs": "git",
    "clean": false,
    "files": [
      {"path": "internal/wt/git.go", "kind": "added"},
      {"path": "README.md", "kind": "modified"},
      {"path": "old.txt", "kind": "deleted"},
      {"path": "new.go", "oldPath": "prev.go", "kind": "renamed"}
    ]
  }
  ```
- **404** `WORKTREE_NOT_FOUND` — bead never dispatched / no worktree (FR-009).
- **412** `VCS_UNAVAILABLE` — the bead's backend binary isn't installed (rare on a read;
  possible if `jj` was removed after dispatch).
- `{id}` validated by the existing `core.ValidBeadID` handler guard.

## `GET /api/v1/beads/{id}/diff[?path=<file>]`

Unified diff (git-format) for the whole worktree or a single file.

- **200** — `Content-Type: text/x-diff; charset=utf-8`, streamed body (chunked). Empty body
  when the worktree exists but is clean.
- **400** `INVALID_PATH` — `?path=` is absolute, contains `..`, or escapes the worktree
  (FR-007). No file outside the worktree is ever read.
- **404** `WORKTREE_NOT_FOUND` — no worktree (FR-009).
- **412** `VCS_UNAVAILABLE` — backend binary absent.
- Streaming: server pipes the `git`/`jj` child stdout to the response writer, flushing;
  it does not buffer the full diff (FR-008).

## `GET /api/v1/orchestrator/status` — additive fields

M2 fields (`tmuxAvailable`, `tmuxVersion`, `runningCount`, `adapters`, …) unchanged.
**Add**:

```json
{
  "vcs": {
    "defaultVCS": "git",
    "git": {"available": true},
    "jj":  {"available": true, "version": "0.42.0"}
  },
  "worktreeCount": 3
}
```

- `defaultVCS` echoes `--default-vcs`.
- `worktreeCount` = number of per-bead worktrees currently on disk under `--worktrees-dir`.
- Availability from `wt.Detect` at startup (FR-010, FR-012).

## Dispatch error (existing endpoint, additive error code)

`POST /api/v1/beads/{id}/dispatch` gains one refusal path:

- **412** `VCS_UNAVAILABLE` — the effective VCS (`--default-vcs`) backend isn't installed,
  or (`vcs=jj`) the mapped source repo is not jj-native (FR-011, FR-004a). No silent
  fallback to git. Existing M2 dispatch responses (202/409/422/501/unauth) are unchanged.

## Additive-surface checklist (verified in tests, SC-007)

- [ ] All M0/M1/M2 routes still registered and behaviorally unchanged.
- [ ] All M2 status DTO fields still present with same shapes.
- [ ] New routes are the only additions; new status fields are additive keys.
- [ ] No M2 WS event type changed (M3 adds none required; `worktree.changed` is out of scope).
