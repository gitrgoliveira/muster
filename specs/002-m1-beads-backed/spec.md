# Feature Specification: M1 — Beads-Backed Store

**Feature Branch**: `002-m1-beads-backed`

**Created**: 2026-05-24

**Status**: Draft

**Input**: Replace the in-memory seed-data store from M0 with a real beads database. `muster` reads and writes live issues from whatever beads database the user has — embedded Dolt or remote Dolt. The REST API and WebSocket surface from M0 are unchanged; only the storage layer swaps out.

**Canonical Sources**:
- `handoff/spec.md` §20 M1 definition ("Replace in-memory store with Dolt. `bd init` integration. Persistence across restarts.")
- `BEADS_MULTIREPO_SETUP_GUIDE.md` — multi-repo routing, `BEADS_DIR` convention, `.beads/metadata.json` schema
- `.beads/metadata.json` — runtime config consumed by muster to select backend and database name

---

## Product Context

Muster is **beads-central**: a server that manages beads. The beads database is the source of truth — not an internal model that muster owns. Everything else (the Go server, REST API, WebSocket hub, UI) is infrastructure for interacting with that database.

M0 faked it with seed data. M1 makes it real: `muster serve` reads from whichever `.beads/` directory the user has configured, reflecting the actual live issues across restarts. muster must support both embedded Dolt databases (`.beads/embeddeddolt/`) and remote Dolt servers.

---

## Clarifications

### Session 2026-05-24

- Q: Should muster write directly to Dolt, or shell out to the `bd` CLI for mutations? → A: **Shell out to `bd` CLI** in M1. Direct Dolt write support deferred to M2.
- Q: Is `issues.jsonl` sufficient for reads, or does muster need direct Dolt SQL access? → A: **Hybrid**. Embedded mode reads `issues.jsonl` directly (no Dolt library). Server mode connects via MySQL after `bd dolt start`. `issues.jsonl` also serves as the fsnotify change-detection trigger in both modes.
- Q: What is the watch/polling interval for detecting changes made by `bd` CLI outside muster? → A: **fsnotify** on `issues.jsonl` with a **500ms debounce**. Fallback polling every 5s if fsnotify is unavailable (e.g., network FS).
- Q: Should `GET /api/v1/beads` support pagination in M1? → A: No — same envelope as M0 (`{"items":[], "nextCursor":null, "total":N}`). Pagination deferred to M2.
- Q: Should muster expose write endpoints in M1? → A: **Yes** — PATCH, POST, move, close. All delegate to `bd` CLI.
- Q: Both embedded and remote Dolt must work — how does muster select? → A: **Read `metadata.json`** from `beads-dir`. `dolt_mode: "embedded"` → read `.beads/issues.jsonl` (no Dolt server needed). `dolt_mode: "remote"` → run `bd dolt start` then connect via MySQL. Server-mode connection params (host, port, user, database) come from `metadata.json` itself (written by `bd init --server` or `bd dolt set`); password comes from `BEADS_DOLT_PASSWORD` env var.
- Q: How does muster invoke `bd` to keep stdout parsing robust? → A: **Always pass `--json`** to `bd create`, `bd update`, `bd show`. JSON output is structured, stable, and avoids ANSI/banner noise. Use `--append-notes=<v>` (NOT `--notes=<v>`) for the comments endpoint so existing notes are preserved.

---

## User Scenarios & Testing *(mandatory)*

### User Story 1 — Start muster against a real beads directory (Priority: P1)

A developer runs `muster serve --beads-dir ~/repos/beads-central/.beads` (or sets `BEADS_DIR`). The server starts, reads the live issues from that database, and `GET /api/v1/beads` returns the actual issues rather than seed data.

**Why this priority**: This is the entire point of M1. Nothing else works until real data loads.

**Independent Test**: `curl http://localhost:7766/api/v1/beads | jq '.total'` returns the actual issue count from the user's beads database, not 14.

**Acceptance Scenarios**:

1. **Given** `BEADS_DIR=~/repos/beads-central/.beads`, **When** `muster serve` starts, **Then** the startup banner includes the resolved beads-dir and the database name read from `metadata.json`.
2. **Given** the server is running, **When** `GET /api/v1/beads`, **Then** the response reflects the live issue list from Dolt (not seed data).
3. **Given** no `--beads-dir` and no `BEADS_DIR`, **When** no `.beads/` exists in cwd, **Then** exit 1 with `beads-dir not found: set --beads-dir or BEADS_DIR`.
4. **Given** `--beads-dir` points to a path that doesn't exist, **Then** exit 1 with `beads-dir not found: <path>`.
5. **Given** `metadata.json` is absent or unparseable, **Then** exit 1 with `invalid beads-dir: cannot read metadata.json: <err>`.

---

### User Story 2 — Embedded mode: JSONL backend (Priority: P1)

`muster` detects `dolt_mode: "embedded"` in `metadata.json` and reads issues from `.beads/issues.jsonl`. No Dolt server or library is required — `bd` manages the Dolt database and exports `issues.jsonl` as a passive data file.

**Why this priority**: All current beads databases (mp, muster) use embedded mode. This is the primary path.

**Independent Test**: Run muster against `~/repos/beads-central/.beads` (embedded, `dolt_database: "mp"`). `GET /api/v1/beads` returns all `mp-*` issues.

**Acceptance Scenarios**:

1. **Given** `metadata.json` has `"dolt_mode": "embedded"`, **When** muster starts, **Then** it reads and parses `.beads/issues.jsonl` (one JSON object per line).
2. **Given** the JSONL backend is loaded, **When** `GET /api/v1/beads`, **Then** issues are served from the parsed JSONL data.
3. **Given** `.beads/issues.jsonl` is absent, **Then** exit 1 with `issues.jsonl not found at <path>`.

---

### User Story 3 — Server mode: Dolt SQL backend (Priority: P2)

`muster` detects `dolt_mode: "remote"` in `metadata.json`, ensures the Dolt server is running (via `bd dolt start`), and connects via MySQL wire protocol.

**Why this priority**: Required for multi-machine and hosted deployments, and for users who run a Dolt server.

**Independent Test**: Configure a Dolt server endpoint; muster connects and serves issues from it.

**Acceptance Scenarios**:

1. **Given** `"dolt_mode": "remote"` and `dolt_host`/`dolt_port`/`dolt_user` set in `metadata.json` (and optional `BEADS_DOLT_PASSWORD` env), **When** muster starts, **Then** it runs `bd dolt start` (idempotent) and connects to the Dolt SQL server via MySQL wire protocol.
2. **Given** the Dolt server is unreachable at startup, **Then** exit 1 with `cannot connect to dolt server: <err>`.
3. **Given** the Dolt server drops during operation, **When** a request arrives, **Then** return `503 SERVICE_UNAVAILABLE` and attempt reconnect in background with exponential backoff (cap 30s).

---

### User Story 4 — Live reload when beads change externally (Priority: P2)

When the user runs `bd close <id>` or `bd create ...` in a terminal while muster is running, connected WS clients receive the appropriate event within 2 seconds.

**Why this priority**: The UI must reflect changes made by the `bd` CLI. Otherwise Muster is a static snapshot.

**Independent Test**: Open a WS connection, run `bd close mp-abc` in terminal, confirm a `bead.updated` WS event arrives within 2 seconds.

**Acceptance Scenarios**:

1. **Given** muster is running (embedded mode), **When** `issues.jsonl` is written by `bd`, **Then** muster detects the change via `fsnotify` within 1 second.
2. **Given** a changed issue is detected, **Then** muster re-reads the data source (JSONL or SQL depending on mode), computes the delta, and broadcasts the appropriate WS event (`bead.updated`, `bead.created`, `bead.deleted`).
3. **Given** a burst of `bd` writes (bulk close), **Then** muster debounces fsnotify events with a 500ms window and emits one WS event per changed issue.
4. **Given** fsnotify is unavailable (network FS), **Then** muster falls back to polling `issues.jsonl` every 5s.

---

### User Story 5 — Write mutations back via `bd` CLI (Priority: P2)

`PATCH /api/v1/beads/{id}`, `POST /api/v1/beads`, `POST /api/v1/beads/{id}/move`, and `POST /api/v1/beads/{id}/dispatch` shell out to the `bd` CLI, then return the updated issue read back from Dolt.

**Why this priority**: Keeps muster from reimplementing Dolt write semantics in M1. `bd` is the authoritative writer.

**Independent Test**: PATCH a bead's title via the API. Verify with `bd show <id>` that the title changed.

**Acceptance Scenarios**:

1. **Given** `PATCH /api/v1/beads/mp-abc {"title":"New title"}`, **Then** muster runs `bd update mp-abc --title="New title"` (with `BEADS_DIR` set to the active beads-dir), reads back the result from Dolt, returns `200 OK` with the updated issue.
2. **Given** `POST /api/v1/beads/mp-abc/move {"toColumn":"done"}`, **Then** muster runs `bd close mp-abc` or the appropriate `bd update` command and returns `200 OK`.
3. **Given** `bd` is not on `$PATH` and `--bd-bin` is not set, **When** muster starts, **Then** log a warning: `bd CLI not found — write endpoints will return 501 NOT_IMPLEMENTED`. Read endpoints remain functional.
4. **Given** `bd` returns a non-zero exit code, **Then** map to HTTP per the exit-code table in `contracts/bd-cli-bridge.md` (exit 1→422, exit 2→404, exit 3→503, other→500), with stderr in the error body.
5. **Given** a `bd` write takes >5s, **Then** cancel it and return `504 GATEWAY_TIMEOUT`.

---

### User Story 6 — Schema version guard (Priority: P3)

`muster serve` reads `metadata.json` and refuses to start if the schema version is incompatible.

**Why this priority**: Prevents silent data corruption on version mismatch between `bd` and muster.

**Acceptance Scenarios**:

1. **Given** `metadata.json` carries a supported schema version, **Then** startup proceeds.
2. **Given** a `schema_version` newer than muster supports, **Then** exit 1 with `beads schema v<N> is newer than muster supports v<M>: upgrade muster`.

---

### Edge Cases

- `.beads/issues.jsonl` absent in embedded mode → exit 1 with `issues.jsonl not found`.
- `.beads/issues.jsonl` absent in server mode → fall back to SQL-only reads; log a warning that live-reload via fsnotify is impaired.
- `bd dolt start` fails in server mode → exit 1 with `cannot start dolt server: <err>`.
- `issues.jsonl` mid-write when fsnotify or an API request fires → retry parse up to 3 times with 100 ms delay; skip and log if still unparseable.
- `bd` write takes >5 s → cancel and return `504 GATEWAY_TIMEOUT`.
- User-supplied field value starts with `-` (looks like a flag) → muster uses `--flag=value` argv form and a `--` separator so `bd` cannot misparse it as a flag.
- Request body > 1 MB → return `413 PAYLOAD_TOO_LARGE` without invoking handler logic.
- `issues.jsonl` larger than 64 MB → exit 1 at startup (treat as misconfiguration).
- Watcher emits initial snapshot before any fsnotify events → first `Backend.List` call populates `snapshot` synchronously *during* `Watcher.Run` startup, so the first delta is computed against real data (not empty).
- Server-mode Dolt connection drops mid-request → reads return `503 SERVICE_UNAVAILABLE`; reconnect with exponential backoff (capped 30 s) in the background.

---

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: System MUST read the beads database path from `--beads-dir` flag, falling back to `BEADS_DIR` env var, then `./.beads/` in cwd.
- **FR-002**: System MUST parse `<beads-dir>/metadata.json` at startup to determine `dolt_mode`, `dolt_database`, and `project_id`.
- **FR-003**: System MUST support `dolt_mode: "embedded"` by reading and parsing `.beads/issues.jsonl`. No Dolt library or server process is needed in embedded mode.
- **FR-004**: System MUST support `dolt_mode: "remote"` by (1) reading `dolt_host`, `dolt_port`, `dolt_user`, `dolt_database` fields from `metadata.json`; (2) reading the optional password from `BEADS_DOLT_PASSWORD` env var; (3) running `bd dolt start` (idempotent) to ensure the server is up; (4) connecting via MySQL wire protocol (`go-sql-driver/mysql`).
- **FR-005**: System MUST abstract both backends behind a `store.Backend` interface (`List`, `Get`, `Close`).
- **FR-006**: System MUST watch `<beads-dir>/issues.jsonl` with `fsnotify`; on change, re-read data from the backend (JSONL re-parse or SQL re-query depending on mode), compute the delta, and broadcast WS events. Debounce: 500ms.
- **FR-007**: System MUST fall back to 5s polling if fsnotify fails to initialize.
- **FR-008**: In embedded mode, read endpoints serve data parsed from `issues.jsonl`. In server mode, read endpoints query Dolt SQL via the `store.Backend`.
- **FR-009**: Write endpoints (`POST`, `PATCH`, `POST /{id}/move`, `POST /{id}/dispatch`, `POST /{id}/comments`) MUST delegate to the `bd` CLI, passing `BEADS_DIR=<beads-dir>` in the subprocess environment. Invocations MUST pass `--json` for structured output and MUST use the `--flag=value` argv form (single argument) with a `--` argv separator preceding any user-supplied flag values to prevent argument injection. For comments, MUST use `--append-notes=<v>` (additive) rather than `--notes=<v>` (replaces).
- **FR-010**: System MUST resolve `bd` via `--bd-bin` flag, `BD_BIN` env var, then `$PATH`.
- **FR-011**: System MUST log a startup warning if `bd` is not found; write endpoints return `501 NOT_IMPLEMENTED`.
- **FR-012**: System MUST include `X-Beads-Dir` and `X-Beads-Database` response headers on all API responses.
- **FR-013**: System MUST check schema version compatibility on startup; exit 1 if the schema is incompatible.
- **FR-014**: All M0 REST endpoints and WebSocket event types MUST remain behaviorally compatible (no breaking changes). Internal store types (`memstore` return shape) may change since they are not part of the public surface; M0 store-level tests that assert on `core.Bead` shapes will be rewritten.
- **FR-015**: System MUST cap inbound request bodies at **1 MB** via `http.MaxBytesReader`; oversized bodies return `413 PAYLOAD_TOO_LARGE`.
- **FR-016**: `GET /api/v1/beads` (list) MUST return issues with `description` truncated to **2 KB** (with a `descTruncated: true` marker when truncated). `GET /api/v1/beads/{id}` returns the full description.
- **FR-017**: JSONL backend MUST cap individual line length at **4 MB**; malformed/oversized lines are skipped and logged. API-triggered reads MUST retry up to 3× with 100 ms backoff on parse failure (covers the atomic-rename race window). Cap whole-file size at **64 MB**.
- **FR-018**: `bd` invocations MUST run with `--dolt-auto-commit=on` so writes are committed to Dolt immediately; this matches the user expectation that mutations persist across `bd dolt push`.

### Key Entities

- **BeadsDir**: The `.beads/` directory — `metadata.json`, `config.yaml`, `embeddeddolt/`, `issues.jsonl`.
- **Metadata**: `dolt_mode`, `dolt_database`, `project_id` parsed from `metadata.json`.
- **Issue**: A beads issue — `id`, `title`, `description`, `status`, `priority`, `issue_type`, `assignee`, `owner`, `created_at`, `updated_at`, `started_at`, `closed_at`, `close_reason`, `dependency_count`, `dependent_count`, `comment_count`. Read from `issues.jsonl` (embedded) or Dolt SQL (server).
- **Backend** (`store.Backend`): Interface — `List(ctx, Filter) ([]Issue, error)`, `Get(ctx, id) (*Issue, error)`, `Close() error`.

### What Changes vs M0

| Aspect | M0 | M1 |
|---|---|---|
| Data source | 14 hardcoded seed beads | Live beads database (JSONL or Dolt SQL) |
| Store impl | `[]core.Bead` + `sync.RWMutex` | JSONL file (embedded) or MySQL wire (server) |
| Writes | Mutate in-memory slice | Shell out to `bd` CLI |
| Change detection | None | fsnotify on `issues.jsonl` + re-read/re-query |
| Config | `--addr` only | `--addr`, `--beads-dir`, `--bd-bin` |
| Startup | None | Validate `metadata.json`, schema version |

### What Does NOT Change vs M0

- REST endpoint paths and shapes
- WebSocket event protocol
- Error response format (`{"error":{"code","message","requestID"}}`)
- `X-Request-ID` middleware
- `GET /api/v1/healthz` and `GET /api/v1/orchestrator/status`
- Embedded UI serving
- Graceful shutdown (SIGINT/SIGTERM, 5s drain)

---

## Success Criteria *(mandatory)*

- **SC-001**: `muster serve --beads-dir ~/repos/beads-central/.beads` serves live `mp-*` issues; count matches `bd stats`.
- **SC-002**: Running `bd close <id>` in terminal causes a `bead.updated` WS event within **2 seconds** when fsnotify is available, or within **6 seconds** when running on the polling fallback (5 s poll + 1 s for diff + broadcast).
- **SC-003**: `PATCH /api/v1/beads/{id}` persists; verified by `bd show <id>` returning the new value.
- **SC-004**: `go test -race ./...` passes — no data races in the store layer.
- **SC-005**: Both JSONL (embedded) and Dolt SQL (server) paths covered by integration tests.
- **SC-006**: `muster serve` with no beads-dir and no `.beads/` in cwd exits 1 with a clear error.
- **SC-007**: Startup with `dolt_mode: "remote"` and unreachable server exits 1 within 5s.

---

## Assumptions

- The `bd` CLI is the sole writer to Dolt in M1. Direct Dolt write support deferred to M2+.
- Multi-repo aggregation (multiple `.beads/` directories in one muster instance) is deferred to M2.
- Authentication is not added in M1.
- `fsnotify` (`github.com/fsnotify/fsnotify`) is used for file watching.
- `issues.jsonl` records match the `store.Issue` struct fields 1:1 (verified against live data).
- muster owns no persistent state of its own; all state lives in the beads directory.
- In server mode, `bd dolt start` is available and works (requires `dolt` on `$PATH`).
- `bd v1.0+` provides `--json` output for `bd create`, `bd update`, `bd show` (verified empirically 2026-05-24).
- `bd update --append-notes=<v>` is additive (verified empirically); `--notes=<v>` is destructive.
- In server mode, `metadata.json` carries `dolt_host`, `dolt_port`, `dolt_user`, `dolt_database`; password (when needed) comes from `BEADS_DOLT_PASSWORD` env var.

## Technical Context

### New Dependencies

| Package | Version | Purpose |
|---|---|---|
| `github.com/go-sql-driver/mysql` | `v1.8.x` | MySQL wire client for server-mode Dolt reads |
| `github.com/fsnotify/fsnotify` | `v1.7.x` | File system event watching |
| `gopkg.in/yaml.v3` | `v3.0.x` | Parse `.beads/config.yaml` |

### New Configuration

| Flag | Env var | Default | Description |
|---|---|---|---|
| `--beads-dir` | `BEADS_DIR` | `./.beads/` | Path to the `.beads/` directory |
| `--bd-bin` | `BD_BIN` | `bd` (from PATH) | Path to the `bd` CLI binary |

### Store Interface

```go
// internal/store/backend.go
type Backend interface {
    List(ctx context.Context, filter Filter) ([]Issue, error)
    Get(ctx context.Context, id string) (*Issue, error)
    Close() error
}

type Filter struct {
    Status            []string // empty = all statuses
    IDs               []string // empty = all issues
    Limit             int      // 0 = unlimited (internal use — not exposed via API in M1)
    TruncateDesc      int      // 0 = no truncation; N > 0 = cap Description at N bytes (list path uses 2048)
}
```

Two implementations:
- `internal/store/jsonl/backend.go` — reads and parses `.beads/issues.jsonl` (embedded mode)
- `internal/store/dolt/backend.go` — connects to Dolt SQL server via MySQL wire (server mode)

### Watcher

`internal/store/watcher.go` — wraps `fsnotify`, debounces 500ms, calls a callback with the affected issue IDs. Services layer subscribes and fans to the WS hub.

### Module Layout Additions

```
internal/
  store/
    backend.go      Backend interface + Filter + Issue types
    jsonl/
      backend.go    embedded mode — reads issues.jsonl
    dolt/
      backend.go    server mode — MySQL wire to Dolt SQL server
      query.go      SQL query helpers
    watcher.go      fsnotify wrapper with debounce + fallback polling
  config/
    loader.go       parse metadata.json + config.yaml → BackendConfig
```
