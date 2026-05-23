# REST API Contract: M0 — Skeleton

**Base URL**: `http://127.0.0.1:7766/api/v1`
**Content-Type**: `application/json; charset=utf-8` (all requests and responses)
**Auth**: None (M0 is local loopback only)

## Cross-cutting behaviour

| Concern | Behaviour |
|---|---|
| **`X-Request-ID`** | If supplied on request, echoed in response. If absent, server generates a UUIDv4. Always present in `error.requestID`. |
| **`Content-Type`** | All requests with bodies MUST send `application/json`. Other types → 415 UNSUPPORTED_MEDIA_TYPE (chi auto-handles) → wrap as `INVALID_REQUEST`. |
| **Body size limit** | POST/PATCH handlers wrapped with `http.MaxBytesReader(w, r.Body, 1<<20)`. Exceeded → 400 with JSON `INVALID_REQUEST` body (`request body exceeds 1 MiB limit`). |
| **Unknown JSON fields** | Decoded with `DisallowUnknownFields` → 400 INVALID_REQUEST. |
| **`null` for required field** | 400 INVALID_REQUEST (`field is required`). |
| **Trailing slash** | `/api/v1/beads/` is identical to `/api/v1/beads` (chi default). |
| **Method not allowed** | 405 METHOD_NOT_ALLOWED with JSON error body (override chi's default plain text). |
| **Unhandled panic** | chi recoverer → 500 INTERNAL with masked message and the request ID. |
| **Validation order** | Each handler validates in this order; first failure short-circuits and returns 400: (1) body size, (2) structural (malformed JSON, unknown field, null for required), (3) required-field presence, (4) enum membership, (5) numeric range / format. |
| **Timestamp fields** | All `at`/`createdAt`/`openedAt`/`closedAt`/`lastActivity`/`T` fields are opaque strings — handler-generated events use RFC3339; seed data may use the prototype's calendar form. |

### Standard error response

```json
{
  "error": {
    "code": "BEAD_NOT_FOUND",
    "message": "no such bead: bd-xxxx",
    "requestID": "5b1c..."
  }
}
```

### Error code matrix

| Code | HTTP | When |
|---|---|---|
| `BEAD_NOT_FOUND` | 404 | Path `/beads/{id}` references missing ID |
| `NOT_FOUND` | 404 | API path under `/api/v1` not matched (non-bead 404; static catch-all handles `/`) |
| `INVALID_STATE` | 400 | Lifecycle precondition not met (e.g., dispatch from non-`scheduled` column) |
| `INVALID_REQUEST` | 400 | Malformed JSON, missing required field, invalid enum, null where forbidden, empty PATCH body, unknown `beforeID` |
| `METHOD_NOT_ALLOWED` | 405 | Wrong HTTP verb on a known path |
| `INTERNAL` | 500 | Panic, ID-generation exhausted after 3 retries, unexpected store error |

---

## Endpoints

### `GET /api/v1/healthz`

Liveness probe — does not touch the store.

**Response 200**:
```json
{ "ok": true }
```

**No error cases.**

---

### `GET /api/v1/orchestrator/status`

Returns daemon status + the seeded DOLT object. In M0 there is no real orchestrator loop; the
`online` field reports whether the WS hub is running.

**Response 200**:
```json
{
  "build": "dev",
  "schemaVersion": 1,
  "beadsVersion": "0.9.1",
  "online": true,
  "serverTime": "2026-05-22T17:42:11Z",
  "dolt": {
    "branch": "main",
    "remote": "origin",
    "ahead": 0,
    "behind": 0,
    "lastSync": "2m ago",
    "status": "clean",
    "server": "running",
    "port": 3306,
    "writers": 4
  }
}
```

`dolt` is sourced from `prototype/data.jsx` DOLT object — populated as part of seed data.
`beadsVersion` comes from the seed's `repos[0].detected.beadsVersion` (default repo).

---

### `GET /api/v1/beads`

List all beads.

**Query parameters**:

| Param | Type | Required | Notes |
|---|---|---|---|
| `column` | string | no | Filter to one of `backlog\|scheduled\|running\|review\|done`. Invalid value → 400. |

**Response 200**:
```json
{
  "items": [ /* Bead, Bead, ... */ ],
  "nextCursor": null,
  "total": 14
}
```

**Error cases**:

| Case | Status | Code |
|---|---|---|
| `column=unknown` | 400 | `INVALID_REQUEST` (`invalid column: unknown`) |

**Notes**:
- `items` is never `null` — `[]` if filter matches no beads.
- `nextCursor` is reserved for M1+ pagination; always `null` in M0.
- `total` reports the **post-filter** count.

---

### `POST /api/v1/beads`

Create a new bead.

**Request body**:
```json
{
  "title": "Implement audit log",
  "desc": "Append-only table",
  "type": "feature",
  "column": "backlog",
  "priority": 2,
  "labels": ["admin", "security"],
  "vcs": "git",
  "tokensBudget": 300000
}
```

**Required**: `title` (non-empty after trim).

**Defaulted if absent**:

| Field | Default |
|---|---|
| `type` | `"task"` |
| `column` | `"backlog"` |
| `priority` | `2` |
| `labels` | `[]` |
| `vcs` | `"git"` |
| `tokensBudget` | `0` |
| `desc` | `""` |

**Response 201** (full bead):
```json
{
  "id": "bd-7f3a",
  "title": "Implement audit log",
  "desc": "Append-only table",
  "type": "feature",
  "column": "backlog",
  "priority": 2,
  "labels": ["admin", "security"],
  "vcs": "git",
  "ready": false,
  "repo": "main",
  "skills": [],
  "steps": [],
  "subBeads": [],
  "history": [
    { "at": "2026-05-22T17:42:11Z", "kind": "opened", "actor": "user" }
  ],
  "acceptance": [],
  "tokensUsed": 0,
  "tokensBudget": 300000,
  "estimate": "M",
  "comments": 0,
  "createdAt": "2026-05-22T17:42:11Z",
  "openedAt": "2026-05-22T17:42:11Z",
  "lastActivity": "2026-05-22T17:42:11Z",
  "log": [],
  "files": [],
  "blocks": [],
  "blockedBy": []
}
```

**Headers**: `Location: /api/v1/beads/{id}`

**Error cases**:

| Case | Status | Code | Message |
|---|---|---|---|
| Missing `title` | 400 | `INVALID_REQUEST` | `title is required` |
| `title` is whitespace only | 400 | `INVALID_REQUEST` | `title is required` |
| `title` > 255 chars | 400 | `INVALID_REQUEST` | `title exceeds 255 chars` |
| `type` not in valid set | 400 | `INVALID_REQUEST` | `invalid type: foo` |
| `column` not in valid set | 400 | `INVALID_REQUEST` | `invalid column: foo` |
| `priority` outside 0..4 | 400 | `INVALID_REQUEST` | `invalid priority: 7` |
| `vcs` not in `git`/`jj` | 400 | `INVALID_REQUEST` | `invalid vcs: bzr` |
| `tokensBudget` negative | 400 | `INVALID_REQUEST` | `tokensBudget must be >= 0` |
| Malformed JSON | 400 | `INVALID_REQUEST` | `malformed JSON: ...` |
| Unknown field | 400 | `INVALID_REQUEST` | `unknown field: foo` |
| ID collision exhausted (3 retries) | 500 | `INTERNAL` | `failed to generate unique ID` |

**Side effects**: emits `bead.created` WS event with the new bead as payload.

---

### `GET /api/v1/beads/{id}`

Get one bead by ID.

**Path params**:
- `id`: bead ID, format `bd-XXXX`. Invalid format → 400; well-formed but missing → 404.

**Response 200**: full `Bead` object.

**Error cases**:

| Case | Status | Code |
|---|---|---|
| `id` does not match `^bd-[0-9a-f]{4}$` | 400 | `INVALID_REQUEST` |
| `id` not in store | 404 | `BEAD_NOT_FOUND` |

---

### `PATCH /api/v1/beads/{id}`

Sparse update — only fields **present** (and non-null) are modified.

**Request body** (all fields optional):
```json
{
  "title": "New title",
  "priority": 0,
  "labels": ["urgent", "regression"],
  "ready": true,
  "tokensBudget": 500000
}
```

**Semantics**:
- Body MUST contain at least one field — an empty body `{}` returns `400 INVALID_REQUEST`.
- Field **absent** from body → not modified.
- Field present with valid value → set to that value.
- For `labels`: pointer-to-slice. Present as `[]` → cleared. Absent → unchanged.
- `null` value for any field → 400 INVALID_REQUEST.

**Response 200**: full `Bead` after update.

**Error cases**:

| Case | Status | Code | Message |
|---|---|---|---|
| Empty body `{}` | 400 | `INVALID_REQUEST` | `patch body must contain at least one field` |
| ID not found | 404 | `BEAD_NOT_FOUND` | `no such bead: bd-xxxx` |
| Invalid enum value | 400 | `INVALID_REQUEST` | (per-field) |
| `priority` outside 0..4 | 400 | `INVALID_REQUEST` | `invalid priority: 7` |
| `title` is empty/whitespace-only | 400 | `INVALID_REQUEST` | `title is required` |
| `null` for any field | 400 | `INVALID_REQUEST` | `field foo cannot be null` |
| Unknown field | 400 | `INVALID_REQUEST` | `unknown field: foo` |

**Side effects**: emits `bead.updated` WS event after a successful 200 response.

---

### `POST /api/v1/beads/{id}/move`

Move a bead to a different column (or reorder within the same column). M0 allows any-to-any
column transition. Position within the destination column is controlled by the optional
`beforeID` field.

**Request body**:
```json
{ "toColumn": "running", "beforeID": "bd-c411" }
```

**Required**: `toColumn`.
**Optional**: `beforeID` — when supplied, the moved bead is inserted **immediately before** the
referenced bead within `toColumn`. When absent or `""`, the moved bead is appended at the end of
`toColumn`.

**Response 200**: full `Bead` after the move. The move is reflected in the order returned by
subsequent `GET /api/v1/beads` calls.

**Error cases**:

| Case | Status | Code | Message |
|---|---|---|---|
| ID not found | 404 | `BEAD_NOT_FOUND` | `no such bead: bd-xxxx` |
| Missing `toColumn` | 400 | `INVALID_REQUEST` | `toColumn is required` |
| Unknown column | 400 | `INVALID_REQUEST` | `invalid column: foo` |
| `beforeID` is the same as `{id}` | 400 | `INVALID_REQUEST` | `beforeID cannot equal the moved bead` |
| `beforeID` does not exist | 400 | `INVALID_REQUEST` | `no such beforeID: bd-xxxx` |
| `beforeID` is in a different column than `toColumn` | 400 | `INVALID_REQUEST` | `beforeID must be in toColumn` |

**Side effects**: emits `bead.moved` WS event with payload `{id, fromColumn, toColumn, beforeID?}`.
**Same-column reorders still emit** `bead.moved` (idempotent broadcast — clients can dedupe).

---

### `POST /api/v1/beads/{id}/dispatch`

Append a new step to the bead's `steps` array with status `active`.

**Request body**:
```json
{ "agent": "claude", "mode": "plan" }
```

**Required**: `agent`, `mode`.

Dispatch is the only state transition that is **gated** in M0: it requires the bead to be in
the `scheduled` column. Per spec FR-010/US8.

**Side effects** (executed atomically under the store's write lock, in order):
1. `column` is set to `running`.
2. A `Step{Agent, Mode, Skills:[], Status:"active"}` is appended to `steps`.
3. Two `HistoryEvent` records are appended:
   - `{kind:"claimed", actor:"dispatcher", agent:<agent>, at: now}`
   - `{kind:"started", actor:<agent>, at: now}`
4. `Assignee` is recomputed (will equal `<agent>`).
5. `LastActivity` is set to `now`.
6. `bead.updated` WS event is emitted.

**Response 200**: full `Bead`.

**Error cases**:

| Case | Status | Code | Message |
|---|---|---|---|
| ID not found | 404 | `BEAD_NOT_FOUND` | `no such bead: bd-xxxx` |
| Missing `agent` | 400 | `INVALID_REQUEST` | `agent is required` |
| Missing `mode` | 400 | `INVALID_REQUEST` | `mode is required` |
| Invalid `agent` | 400 | `INVALID_REQUEST` | `invalid agent: foo` |
| Invalid `mode` | 400 | `INVALID_REQUEST` | `invalid mode: foo` |
| Bead is not in `scheduled` column | 400 | `INVALID_STATE` | `cannot dispatch bead in column <col>` |

---

### `POST /api/v1/beads/{id}/comments`

Append a `comment` lifecycle event.

**Request body**:
```json
{ "actor": "claude", "note": "requested deterministic clock test" }
```

**Required**: `actor`, `note` (both non-empty after trim).

**Side effects**:
- Appends `HistoryEvent{Kind: "comment", Actor, Note, At: now}`.
- `Comments` count incremented (derived).
- `LastActivity` updated.
- Emits `comment.added` WS event with payload `{id, event: <HistoryEvent>}`.

**Response 201** (full `Bead` after the comment is appended): per spec US9.

**Error cases**:

| Case | Status | Code | Message |
|---|---|---|---|
| ID not found | 404 | `BEAD_NOT_FOUND` | `no such bead: bd-xxxx` |
| Missing/empty `actor` | 400 | `INVALID_REQUEST` | `actor is required` |
| Missing/empty `note` | 400 | `INVALID_REQUEST` | `note is required` |

---

### `GET /api/v1/stream`

WebSocket upgrade — see [ws-events.md](ws-events.md) for the full event protocol.

---

## Static UI

### `GET /`

Serves `Muster.html` from the embedded `ui/` directory (via `go:embed ui/*` + `fs.Sub`).

### `GET /{any-non-api-path}`

Catch-all `http.FileServer(http.FS(uiFS))` mounted at `/*`. Falls through to 404 if the file
doesn't exist in the embedded FS.

Requests starting with `/api/v1/` are routed to API handlers and **never** fall through to the
file server — an unknown path under `/api/v1` returns JSON `NOT_FOUND`.
