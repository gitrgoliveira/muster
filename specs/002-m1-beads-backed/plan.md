# Implementation Plan: M1 — Beads-Backed Store

**Branch**: `002-m1-beads-backed` | **Date**: 2026-05-24 | **Spec**: [spec.md](spec.md)

**Input**: Feature specification from `/specs/002-m1-beads-backed/spec.md`

## Summary

Replace M0's in-memory `[]core.Bead` store with a real **beads database backend**. `muster` reads
issues from the user's `.beads/` directory using a **hybrid strategy**: embedded mode parses
`issues.jsonl` directly (no Dolt library needed); server mode connects to a Dolt SQL server
(started via `bd dolt start`) over MySQL wire protocol. Writes shell out to `bd` in both modes.
External mutations (made by `bd` in a terminal) are detected via `fsnotify` on `.beads/issues.jsonl`
and broadcast over the existing WebSocket hub. The M0 REST + WS surface, services layer, HTTP handlers,
and embedded UI are **unchanged**; only the `store` package is swapped and a small `config` package is
added.

## Technical Context

**Language/Version**: Go 1.26/3 (unchanged from M0)

**Primary Dependencies** (M0 deps retained; new entries below):

| Package | Version | Purpose |
|---|---|---|
| `github.com/go-chi/chi/v5` | `v5.2.1` | HTTP routing (carried from M0) |
| `github.com/coder/websocket` | `v1.8.13` | WebSocket server (carried from M0) |
| `github.com/google/uuid` | `v1.6.0` | Request ID generation (carried from M0) |
| `github.com/stretchr/testify` | `v1.10.0` | Test helpers (carried from M0) |
| **`github.com/go-sql-driver/mysql`** | `v1.8.x` | MySQL wire client for server-mode Dolt SQL reads |
| **`github.com/fsnotify/fsnotify`** | `v1.7.x` | File-system event watching for `issues.jsonl` |
| **`gopkg.in/yaml.v3`** | `v3.0.x` | Parse `.beads/config.yaml` |

> **Note on Dolt deps**: muster does **not** import Dolt Go libraries. Embedded mode reads `issues.jsonl`
> directly. Server mode connects to a Dolt SQL server (started via `bd dolt start`) using the standard
> MySQL driver. This keeps the binary small and avoids a multi-hundred-MB transitive dependency graph.

**Storage**: external beads database managed by `bd`. muster owns **no** persistent state —
all state lives in the user's `.beads/` directory.

**Testing**: `go test ./...`; race detector required (`go test -race ./...`). Integration tests use a
tmpdir `.beads/` populated via `bd init` (vendored binary or shell-out). Coverage targets defined below.

**Target Platform**: macOS/Linux, local loopback (unchanged from M0)

**Project Type**: CLI / web-service hybrid — single Go binary (unchanged from M0)

**Performance Goals**:
- `GET /api/v1/beads` p95 ≤ 50 ms with up to 1,000 issues (in-memory snapshot cache makes this trivial)
- Change detection latency: 95% of `bd close` invocations produce a WS event within 2 s when fsnotify
  is available, within 6 s on polling fallback (SC-002)
- WS fan-out: ≤ 100 ms p95 to 10 clients (unchanged from M0)
- List response with 5,000 issues at 2 KB description cap: ≤ 10 MB body (vs ~50 MB without cap)

**Constraints**:
- Single binary, no daemons spawned by muster (server-mode Dolt is managed by `bd dolt start`, not muster)
- Multi-repo aggregation deferred to M2 (one `.beads/` directory per muster instance in M1)
- No write path bypassing `bd` — direct Dolt writes deferred to M2+

**Scale/Scope**: 1 beads directory per instance, up to ~5,000 issues, <10 concurrent WS clients.

## Constitution Check

| Gate | Status | Notes |
|---|---|---|
| Single binary | PASS | `cmd/muster/` still produces one binary |
| External DB allowed? | PASS-CONDITIONAL | M1 introduces Dolt as the source of truth — by design. The constraint was always "no DB muster owns"; the user's Dolt DB is configuration, not muster state |
| Embedded UI | PASS | Unchanged — `go:embed ui/*` |
| Module path known | PASS | `github.com/gitrgoliveira/muster` (unchanged) |
| Test coverage targets | PASS | core/ unchanged, services/ unchanged, **store/jsonl/ ≥85%**, **store/dolt/ ≥75%** (server mode requires Dolt; integration tests cover the rest), **config/ ≥85%**, **watcher ≥80%** |
| Tests-first per layer | PASS | TDD ordering enforced in every phase below |
| Handlers separated from business logic | PASS | Backend swap is transparent to handlers |
| No breaking REST/WS changes | PASS | M0 endpoints and event types preserved (FR-014) |

## Project Structure

### Documentation (this feature)

```text
specs/002-m1-beads-backed/
├── plan.md              # This file
├── spec.md              # Feature spec (fully clarified, 6 Q&A pairs)
├── research.md          # Phase 0 output — backend choice, Dolt embedding strategy, fsnotify semantics
├── data-model.md        # Phase 1 output — Issue, BackendConfig, Filter, watcher events
├── quickstart.md        # Phase 1 output — running muster against ~/repos/beads-central
├── contracts/
│   ├── backend-interface.md   # store.Backend Go interface contract
│   ├── config-file.md         # metadata.json + config.yaml schema muster consumes
│   └── bd-cli-bridge.md       # commands muster shells out to (verbs, env, exit codes)
├── checklists/          # Phase 4 output
└── tasks.md             # Phase 5 output (speckit-tasks)
```

### Source Code Layout (extends M0)

The M0 layout is preserved. M1 adds three packages (`internal/store/jsonl/`, `internal/store/dolt/`,
`internal/config/`) and one cross-cutting file (`internal/store/watcher.go`). The existing
`internal/store/memstore.go` is retained for tests and the `--store=memory` escape hatch.

```text
cmd/muster/
├── main.go              # CHANGED: parse --beads-dir, --bd-bin; build Backend from config; wire watcher
└── embed.go             # unchanged

internal/
├── core/                # unchanged — pure domain types
│
├── config/              # NEW — locate and parse the beads directory
│   ├── loader.go        # ResolveBeadsDir() (flag → env → cwd); LoadBackendConfig(dir) → BackendConfig
│   ├── metadata.go      # parse metadata.json (dolt_mode, dolt_database, project_id, schema_version)
│   ├── yaml.go          # parse config.yaml (server mode connection details)
│   ├── loader_test.go
│   ├── metadata_test.go
│   └── yaml_test.go
│
├── store/
│   ├── store.go         # CHANGED: type Backend interface — adds Backend; existing Store now uses Backend
│   ├── memstore.go        # existing in-memory store (kept; used by tests and --store=memory escape hatch)
│   ├── watcher.go       # NEW — fsnotify wrapper, 500ms debounce, 5s polling fallback
│   ├── watcher_test.go
│   ├── jsonl/
│   │   ├── backend.go      # NEW — embedded mode: reads and parses .beads/issues.jsonl
│   │   └── backend_test.go
│   ├── dolt/
│   │   ├── backend.go      # NEW — server mode: connects to Dolt SQL via MySQL wire
│   │   ├── query.go        # NEW — SQL: SELECT * FROM issues, parameterized Get(id)
│   │   ├── backend_test.go
│   │   └── query_test.go
│   └── bdshell/         # NEW — write-path: shell out to `bd` CLI
│       ├── exec.go      # type CLI struct {Path string}; Update, Create, Move, Close, Comment
│       ├── exec_test.go # uses a fake `bd` script in $PATH
│       └── timeout.go   # 5s default; context-driven cancel
│
├── services/            # unchanged — handlers still call services; services call Backend
│
├── ws/                  # unchanged
│
└── api/                 # unchanged — all M0 endpoints unchanged
```

**Structure Decision**: extend the M0 tree; do not refactor. The Backend interface absorbs reads
(via JSONL or Dolt SQL) and writes go through `bdshell`, keeping `internal/services/` callers unchanged.
`internal/store/memstore.go` stays because (a) it makes unit tests for services trivial, and (b) we want
a `--store=memory` flag for demos and tests.

## Phase 0 — Research (research.md)

The plan command will produce `research.md` with at least these 8 decisions:

1. **Embedded mode read strategy** — read `issues.jsonl` (JSONL parsing) rather than importing Dolt
   libraries or spawning a subprocess. `bd` manages the database; muster reads the export.
2. **Server mode read strategy** — use `bd dolt start` to ensure the Dolt server is running, then
   connect via `go-sql-driver/mysql`. Connection params from `metadata.json` (`dolt_host`/`dolt_port`/`dolt_user`) + `BEADS_DOLT_PASSWORD` env var.
3. **`issues.jsonl` change-detection strategy** — fsnotify event types (Write vs. Create vs. Rename),
   atomic write semantics on macOS/Linux, debounce window justification.
4. **Polling fallback trigger** — exactly when do we drop to polling? (fsnotify init error, NFS, etc.)
5. **`bd` CLI surface for writes** — exact verbs and flags for: update title, update description, update status,
   add comment, create, move/close. Confirmed against `bd v1.0+` CLI.
6. **Schema version probe** — `metadata.json` carries `schema_version`. Hard-code supported range.
7. **Server mode lifecycle** — `bd dolt start` is idempotent; muster calls it at startup. `bd dolt stop`
   is NOT called on shutdown (other `bd` commands may need the server).
8. **Delta computation** — on fsnotify fire: embedded mode re-reads `issues.jsonl` and diffs against
   snapshot. Server mode re-queries SQL and diffs. Both emit the same `WatcherEvent`.

Each entry follows the Decision / Rationale / Alternatives format from `research.md`.

## Phase 1 — Design

### data-model.md (highlights)

- **`Issue`**: the beads issue record — `id`, `title`, `description`, `status`, `priority`, `issue_type`,
  `assignee`, `owner`, `created_at`, `updated_at`, `started_at`, `closed_at`, `close_reason`,
  `dependency_count`, `dependent_count`, `comment_count`. Maps 1:1 to `issues.jsonl` lines and
  to the Dolt `issues` table.
- **`BackendConfig`**: `{Mode: "embedded" | "remote", DoltDatabase string, SchemaVersion int}`.
- **`Filter`**: `{Status string, IDs []string}` — passed to `Backend.List`.
- **`WatcherEvent`**: `{IssueIDs []string, ChangedAt time.Time}` — emitted by the watcher after debounce.
- **`Backend` interface**: `List(ctx, Filter) ([]Issue, error)`, `Get(ctx, id) (*Issue, error)`,
  `Close() error`. Two impls: `jsonl.Backend` (embedded) and `dolt.Backend` (server). Writes happen
  via `bdshell.CLI` and are **not** part of `Backend`.

### contracts/

- **`backend-interface.md`** — Go interface signatures, error sentinels (`ErrStoreReadOnly`,
  `ErrStoreUnavailable`, `ErrNotFound`), context cancellation contract.
- **`config-file.md`** — full schema for `metadata.json` (with required vs. optional fields)
  and `config.yaml` (server-mode connection details consumed by muster in M1).
- **`bd-cli-bridge.md`** — table of API endpoint → `bd` command(s), with environment (`BEADS_DIR`),
  stdin/stdout expectations, exit-code handling, and the 5-second timeout policy.

### quickstart.md (highlights)

End-to-end walk-through:
1. `make build` → produces `./muster`
2. `./muster serve --beads-dir ~/repos/beads-central/.beads`
3. `curl http://localhost:7766/api/v1/beads | jq '.total'` — confirms live count
4. In another terminal: `bd close mp-abc` — observe WS event arrive within 2 s
5. `curl -X PATCH http://localhost:7766/api/v1/beads/mp-abc -d '{"title":"new"}'` — observe `bd show mp-abc` reflect the change

### Agent context update

The plan-script step updates the `<!-- SPECKIT START -->` / `<!-- SPECKIT END -->` block in
`CLAUDE.md` to point at this plan file.

## Phase 2 — Tasks (`speckit-tasks`, not this phase)

`tasks.md` will be generated by `speckit-tasks` after the plan is approved. Expected task ordering
(authoritative once tasks.md exists):

1. **Setup / dependencies** — `go get` new deps; create empty package skeletons; ensure go.mod tidies.
2. **Config layer (TDD)** — `internal/config/` tests first, then implementations.
3. **Store backend interface (TDD)** — `internal/store/store.go` Backend type; rewire memstore.go to satisfy it; keep all M0 tests green.
4. **JSONL backend (TDD)** — backend_test.go uses a tmpdir with a fixture `issues.jsonl`; backend.go parses JSONL, implements Backend.
5. **Dolt SQL backend (TDD)** — backend_test.go skips when `DOLT_TEST_DSN` env not set; backend.go connects via MySQL wire.
6. **Watcher (TDD)** — watcher_test.go drives fsnotify with a tmpdir; production code implements debounce + polling fallback.
7. **bdshell write bridge (TDD)** — exec_test.go uses a fake `bd` binary on PATH; exec.go shells out and maps errors to API codes.
8. **Wire-up in `cmd/muster/main.go`** — parse flags, build Backend (JSONL or Dolt), build CLI bridge, attach watcher to services, start http.Server.
9. **End-to-end integration tests** — bring up muster against a tmpdir `.beads/`, perform full lifecycle, assert WS events.
10. **Docs & quickstart polish** — update README, ensure `make help` and banners are accurate.

Coverage gates (CI):
- `internal/core/` ≥80% (unchanged)
- `internal/store/` ≥80% (the interface package)
- `internal/store/jsonl/` ≥85%
- `internal/store/dolt/` ≥75%
- `internal/store/bdshell/` ≥85%
- `internal/config/` ≥85%
- `internal/services/` ≥80% (unchanged)
- `internal/api/...` ≥70% (unchanged)
- `internal/ws/` ≥75% (unchanged)
- `internal/store/watcher.go` ≥80%
- `go test -race ./...` clean

## Complexity Tracking

| Violation | Why Needed | Simpler Alternative Rejected Because |
|---|---|---|
| Two backends (JSONL + Dolt SQL) | spec FR-003, FR-004 — both `dolt_mode` values must work | JSONL-only rejected: blocks server-mode users. SQL-only rejected: requires Dolt server even for local embedded databases |
| Retain `internal/store/memstore.go` | Keeps M0 service tests trivial and provides a `--store=memory` escape hatch | Deleting it would force every service test to spin up Dolt; net negative for iteration speed |
| Shell-out to `bd` for writes | Avoids re-implementing Dolt write semantics & beads' invariants inside muster in M1 | Direct Dolt writes from muster: doable but doubles M1 scope; deferred to M2 (acknowledged in spec assumptions) |

---

**Plan status**: ready for `speckit-tasks` once research.md, data-model.md, quickstart.md, and contracts/ are written by Phase 0/1 sub-steps below.
