# Contract: HTTP Endpoints (M2 additions)

All additive. M1 endpoints/shapes unchanged (FR-019). Error body is the M1 shape: `{"error":{"code","message","requestID"}}`.

## `POST /api/v1/beads/{id}/dispatch` (real body — was a stub in M1)

Request:
```json
{ "agent": "claude", "mode": "agent", "permissionMode": "acceptEdits" }
```
- `agent` (required): `core.AgentID`; M2 only `claude` is registered.
- `mode` (required): `plan` | `agent` (other `core.Mode` values rejected in M2).
- `permissionMode` (required unless `--default-permission-mode` is configured): allow-listed `core.PermissionMode`. **muster never defaults autonomy** (FR-021).

Responses:
> **Note on `202` vs M1's `200`:** M1's `/dispatch` was a non-functional stub that returned `200 OK`. M2 makes it a real asynchronous launch, so it returns `202 Accepted`. This is the one intentional, documented exception to FR-019's "no breaking REST changes" — no real M1 behavior depended on the stub's status code; both are 2xx.

| Status | When |
|---|---|
| `202 Accepted` | run launched; body = the bead in `running`. Run proceeds async (intentional change from M1's stub `200` — see note above). |
| `409 CONFLICT` | a run is already active for this bead (one per bead in M2) |
| `400 BAD_REQUEST` | malformed body; or `INVALID_REQUEST`: unknown/invalid `agent`, `mode`, or `permissionMode` value; or no `permissionMode` and no `--default-permission-mode` configured |
| `422 UNPROCESSABLE_ENTITY` | `UNMAPPED_PREFIX`: bead prefix has no `--repo` mapping |
| `501 NOT_IMPLEMENTED` | `ADAPTER_NOT_FOUND`: agent not registered; or `ADAPTER_NOT_INSTALLED`: binary not found on PATH |
| `409`/error | adapter installed but `loggedIn=false` → message: run `claude auth login` |

Side effects (happy path): resolve repo (prefix map) → `git worktree add` (or reuse) → write `.muster-prompt-0.txt` → tmux `Spawn` → emit `tmux.session.opened` → bead column → `running`.

## `GET /api/v1/beads/{id}/steps/{idx}/attach`

M2: only `idx=0`. Returns the live-attach descriptor:
```json
{ "available": true, "command": "tmux attach -t muster/mp-abc/0/0", "session": "muster/mp-abc/0/0", "pane": "%3" }
```
- `available:false` + `reason` when: tmux absent (fallback), step not running, or `idx≠0`. Non-error (200) with `available:false` for the "not attachable" cases; `404` for unknown bead or `idx≠0`.

## `POST /api/v1/beads/{id}/steps/{idx}/send`

Request `{ "keys": "y\n" }` → forwards to the live pane via `send-keys`.
| Status | When |
|---|---|
| `204 No Content` | delivered |
| `409`/`404` | session already exited / not running / unknown |
| `412`/`409` | tmux unavailable (fallback) — sending unsupported |

## `GET /api/v1/orchestrator/status` (additive fields)

Adds to the M1 body:
```json
{ "tmuxAvailable": true, "tmuxVersion": "3.6b", "runningCount": 2,
  "adapters": [ { "id": "claude", "version": "2.1.145", "loggedIn": true } ] }
```
All M1 fields retained.
