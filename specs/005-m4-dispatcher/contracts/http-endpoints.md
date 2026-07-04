# Contract — HTTP Endpoints (M4)

All routes under `/api/v1`. **Additive** except the one documented change to `POST .../dispatch`'s duplicate response (409 → 200+join). Handlers stay thin (parse → service → render). Bead IDs validated by the existing allow-list.

## Changed (documented migration)

### `POST /beads/{id}/dispatch`
- **New behavior on in-flight duplicate**: returns **200 OK** with the existing run and `"joined": true` (was **409 Conflict** / `ErrRunAlreadyActive` in M2). First dispatch of an idle bead is unchanged (launches a run; may be `"queued": true` if at capacity).
- Request body unchanged, plus **optional** additive field `chain` (ordered step list) to override the default chain.
- Response (additive fields): `{ "bead": {...}, "joined": false, "queued": false }`.
- **Migration note**: M2 tests `TestDispatch_409_RunAlreadyActive` / `TestDispatch_409_DuplicateRun` are rewritten to assert this idempotent contract (Constitution V versioned migration; see plan Complexity Tracking).

## New — Scheduler / capacity

### `PUT /orchestrator/capacity`
- Body: `{ "capacity": <int > 0> }`.
- `200` `{ "capacity": N, "activeCount": A, "waiting": ["<beadID>", ...] }` on success.
- `400` typed error on `capacity ≤ 0` or non-integer (no silent default).
- Lowering below `activeCount` drains (does not kill running agents).

### `GET /orchestrator/status` (additive fields)
- Existing DTO **plus**: `"capacity": N`, `"activeCount": A`, `"waiting": ["<beadID>", ...]`, and per-run `"stepIdx"`, `"chainLen"`, `"quota": { "known": bool, "inputTokens": .., "outputTokens": .., "costUSD": .. }`.

## New — Worktree write-side

### `POST /beads/{id}/worktree/finalize`
- Body: `{ "message": "<commit message>" }`.
- `200` `{ "committed": true|false, "message": "..." }` — `committed:false` on a no-change worktree (no-op success, FR-010).
- `409`/typed error if the step's agent is still active (must not race a live agent).
- `VCS_UNAVAILABLE` if the bead's backend binary is absent.

### `POST /beads/{id}/worktree/push`
- Body: optional `{ "remote": "<name>" }` (defaults to configured remote / `origin`).
- `200` `{ "pushed": true, "branch": "muster/<id>", "remote": "origin" }`.
- Explicit typed error on missing remote / auth failure / rejected push (never silent success, FR-007).

### `DELETE /beads/{id}/worktree`
- Tears down the per-bead worktree; `200` `{ "removed": true }`; subsequent `GET /beads/{id}/worktree` reports absent.
- `VCS_UNAVAILABLE` if backend binary absent.

## New — Step chain (operator-driven)

### `POST /beads/{id}/steps/advance`
- Advances the step pointer by 1 and starts the next step's run over the same worktree.
- `200` `{ "stepIdx": N+1, "chainLen": L }`; `400`/typed `ErrStepOutOfRange` if already at the last step or no chain.

### `POST /beads/{id}/steps/loopback`
- Body: `{ "toIdx": <int> }` (must be `≥0` and `< current stepIdx`).
- Moves the pointer back and starts a run for that step; `200` `{ "stepIdx": toIdx, "chainLen": L }`; `400` on out-of-range.

### `GET /beads/{id}/steps/{idx}/attach` and `POST /beads/{id}/steps/{idx}/send` (widened)
- `parseStepIdx` now accepts **`idx ≥ 0`** (M2 accepted only `"0"`), still rejecting negative / non-canonical / out-of-chain-range indices with a clear error. All M2 `idx=0` behavior is unchanged.

## Error mapping (additive service codes)

| Condition | Service code | HTTP |
|---|---|---|
| in-flight duplicate dispatch | (none — success) | 200 + `joined:true` |
| step index out of range | `CodeStepOutOfRange` | 400 |
| finalize/remove while agent active | `CodeRunAlreadyActive` (reused) | 409 |
| backend VCS binary absent | `CodeVCSUnavailable` (reused, M3) | 409/424 as M3 |
| capacity ≤ 0 (runtime) | `CodeInvalidCapacity` | 400 |
