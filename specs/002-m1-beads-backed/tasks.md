# Tasks: M1 — Beads-Backed Store

**Input**: Design documents from `specs/002-m1-beads-backed/`

**Prerequisites**: plan.md ✓, spec.md ✓, research.md ✓, data-model.md ✓, contracts/ ✓

**TDD Policy**: Write tests first; confirm they FAIL; then implement until green.

---

## Phase 1: Setup

**Purpose**: Dependencies, package skeletons, rename cmd dir if needed.

- [ ] T001 ⚠️ BLOCKER: Rename `cmd/musterd/` → `cmd/muster/` — `make build` is currently broken because Makefile says `PKG := ./cmd/muster/` but dir is `cmd/musterd/`. This MUST be done first.
- [ ] T002 `go get github.com/go-sql-driver/mysql@v1.8.1` and add to `go.mod`
- [ ] T003 [P] `go get github.com/fsnotify/fsnotify@v1.7.0` and add to `go.mod`
- [ ] T004 [P] Promote `gopkg.in/yaml.v3` from indirect to direct in `go.mod` (already present via testify)
- [ ] T005 Create empty package dirs: `internal/config/`, `internal/store/jsonl/`, `internal/store/dolt/`, `internal/store/bdshell/`
- [ ] T006 `go mod tidy && go build ./...` — must succeed with no errors
- [ ] T007 Update `idPattern` in `internal/api/beads/handlers.go` from `^bd-[0-9a-f]{4}$` to accept real beads IDs (`<prefix>-<suffix>` format, e.g. `mp-kbj`, `muster-xyz`). Regex: `^[a-z]+-[0-9a-z]+$` or similar. All handler `validateID` calls use this.
- [ ] T008 Add request body size limit middleware in `internal/api/`: wrap all handlers with `http.MaxBytesReader(w, r.Body, 1<<20)`. Oversized bodies return `413 PAYLOAD_TOO_LARGE` with `{"error":{"code":"PAYLOAD_TOO_LARGE","message":"request body exceeds 1 MB limit"}}`.

**Checkpoint**: `go build ./...` green; all M0 tests still pass (after T007 regex change, some M0 ID-validation tests may need fixture IDs updated).

---

## Phase 2: Foundational — Backend Interface + Config (Blocks All Stories)

**Purpose**: Shared types and config loader that every user story depends on.

⚠️ **CRITICAL**: No user story work can begin until this phase is complete.

### 2a — Store interface and types

- [ ] T010 Write `internal/store/errors.go`: define `ErrNotFound`, `ErrStoreUnavailable`, `ErrStoreReadOnly`, `ErrSchemaMismatch` using `errors.New`
- [ ] T011 Write `internal/store/issue.go`: define `Issue` struct with all fields from data-model.md (id, title, description, status, priority, issue_type, assignee, owner, created_at, updated_at, started_at, closed_at, close_reason, dependency_count, dependent_count, comment_count, **notes** with `json:"notes,omitempty"`) — JSON tags matching `issues.jsonl`
- [ ] T012 Write `internal/store/filter.go`: define `Filter` struct (`Status []string`, `IDs []string`, `Limit int`, `TruncateDesc int`). `TruncateDesc > 0` caps the `Description` field to N bytes; backends MUST honor this for list responses (FR-016)
- [ ] T013 Write `internal/store/backend.go`: define `Backend` interface (`List`, `Get`, `Close`); `memory.go` must satisfy it — run `go build ./...` to confirm
- [ ] T014 Rewrite `internal/store/memstore.go` to satisfy the new `Backend` interface (`List`/`Get`/`Close` returning `store.Issue`). M0 store-level tests that assert on `core.Bead`-shaped store outputs MUST be rewritten or deleted — they're not part of the public API (FR-014 carve-out). Keep memstore minimal: in-memory `[]Issue`, `sync.RWMutex`. Service-layer tests (which assert on `core.Bead` shapes via the mapper) remain valid.

### 2b — Issue→Bead mapping + service layer refactoring

- [ ] T015 Write `internal/services/mapper.go`: `IssueToBeads(issues []store.Issue, repo string) []core.Bead` and `IssueToBead(issue *store.Issue, repo string) core.Bead` — implements the mapping table from data-model.md (Status→Column, DeriveAssignee, History from timestamps, zero-value fields for VCS/Branch/Worktree/etc.)
- [ ] T016 Write `internal/services/mapper_test.go`: test all status→column mappings, assignee fallback to owner, history derivation from created_at/started_at/closed_at, unknown status defaults to Backlog
- [ ] T017 Refactor `internal/services/beads.go`: change `BeadService` to accept `store.Backend` (reads) + `bdshell.CLI` (writes) instead of the M0 `store.Store`. Read methods call `Backend.List`/`Get` then `IssueToBeads`/`IssueToBead`. Write methods call `CLI.Run` then re-read from Backend.
- [ ] T018 Update `internal/api/beads/handlers.go`: `Handlers` struct accepts the refactored `BeadService` (not the old `store.Store` directly). Handlers call service methods, not store directly.

### 2c — Config layer (TDD)

- [ ] T020 Write `internal/config/metadata_test.go`: table-driven tests for `LoadMetadata` covering — file missing, file unparseable, `database` != "dolt", `dolt_mode` invalid, `dolt_database` empty, valid embedded, valid remote (with dolt_host/port/user), schema_version absent (defaults to 1), schema_version=99 (error), missing `dolt_host` in remote mode (error)
- [ ] T021 Run `go test ./internal/config/...` — confirm tests FAIL (no implementation yet)
- [ ] T022 Write `internal/config/metadata.go`: `Metadata` struct (JSON tags per data-model.md, including `dolt_host`, `dolt_port`, `dolt_user`); `LoadMetadata(dir string) (*Metadata, error)` — reads `<dir>/metadata.json`, validates per contracts/config-file.md, returns exact error messages
- [ ] T023 Run `go test ./internal/config/...` — all metadata tests pass
- [ ] T024 Write `internal/config/loader_test.go`: tests for `ResolveBeadsDir` (flag wins over env, env wins over cwd, all absent → error), `LoadBackendConfig` (schema_version range validation, server-mode validations, `BEADS_DOLT_PASSWORD` env reading)
- [ ] T025 Run `go test ./internal/config/...` — confirm new tests FAIL
- [ ] T026 Write `internal/config/loader.go`: `BackendConfig` struct (per data-model.md, including `DoltHost`/`DoltPort`/`DoltUser`/`DoltPassword`); `ResolveBeadsDir(flag, env string) (string, error)`; `LoadBackendConfig(dir string) (BackendConfig, error)` — composes metadata + env-var password
- [ ] T027 Run `go test ./internal/config/...` — all pass; coverage ≥ 85%

**Checkpoint**: Foundation ready. `internal/config/` and `internal/store/` types compile; all tests green.

---

## Phase 3: User Story 1 — Start muster against a real beads directory (P1) 🎯 MVP

**Goal**: `muster serve --beads-dir <path>` loads a real `.beads/` directory and fails clearly on bad input.

**Independent Test**: `muster serve --beads-dir ~/repos/beads-central/muster/.beads` starts and prints the startup banner.

### Implementation

- [ ] T030 Update `cmd/muster/main.go`: add `--beads-dir` (flag → `BEADS_DIR` env → `./.beads/`) and `--bd-bin` (flag → `BD_BIN` env → PATH) flags
- [ ] T031 `cmd/muster/main.go`: call `config.ResolveBeadsDir` and `config.LoadBackendConfig` at startup; `log.Fatal` with error message on failure
- [ ] T032 `cmd/muster/main.go`: print startup banner with `beadsDir`, `doltDatabase`, `doltMode`, `readSource`, `bdCLI` (or `(missing)`)
- [ ] T033 Add `X-Beads-Dir` and `X-Beads-Database` middleware in `internal/api/` (or in main router setup) — sets headers on all responses
- [ ] T034 Update `GET /api/v1/orchestrator/status` response to include `beadsDir`, `doltDatabase`, `doltMode`, `schemaVersion`, `projectID`, `readSource`, `bdCLI`. Refactor `OrchestratorStatusHandler` constructor to accept `BackendConfig` (not the M0 hardcoded `seedDolt`)
- [ ] T034b Add `Backend.Ping(ctx) error` method (small extension to the interface) and wire `OrchestratorStatusResponse.Online` to `backend.Ping(ctx) == nil`. JSONL backend implements `Ping` by checking `issues.jsonl` is still readable; Dolt backend implements `Ping` via `db.PingContext(ctx)`; memstore returns nil. Update `contracts/backend-interface.md` accordingly

### Integration test

- [ ] T035 Write `cmd/muster/main_test.go` (or `internal/integration/startup_test.go`): build binary; run with missing `--beads-dir` → exit 1 with expected message; run with bad metadata.json → exit 1; run with valid tmpdir → exit 0 and banner printed

**Checkpoint**: `muster serve --beads-dir <valid>` starts and logs banner. Bad inputs exit 1 clearly.

---

## Phase 4: User Story 2 — Embedded mode: JSONL backend (P1) 🎯 MVP

**Goal**: `muster serve` in embedded mode reads live issues from `issues.jsonl`.

**Independent Test**: `GET /api/v1/beads` returns issues from a fixture `issues.jsonl`, not seed data.

### Tests first

- [ ] T040 Write `internal/store/jsonl/backend_test.go`: fixture `issues.jsonl` with ≥5 diverse records (nil started_at, nil closed_at, multi-line descriptions, unicode, custom statuses); tests for — `List` all, `List` by status, `List` by IDs, `List` with Limit, `List` with TruncateDesc=100 truncates Description, `Get` existing, `Get` missing → ErrNotFound, `Get` empty ID → ErrNotFound, `Close` no-op, file absent → error on construction, unparseable line skipped with log, retry-on-partial-read (write mid-rename → 3× retry succeeds), oversize line (>4 MB) skipped, oversize file (>64 MB) → error
- [ ] T041 Run `go test ./internal/store/jsonl/...` — confirm tests FAIL

### Implementation

- [ ] T042 Write `internal/store/jsonl/backend.go`: `type Backend struct { path string; cache *cachedSnapshot; mu sync.RWMutex }`; `NewJSONL(beadsDir string) (store.Backend, error)` — validates `issues.jsonl` exists and size ≤ 64 MB; `parseFile` uses `bufio.Scanner` with `Buffer(make([]byte, 0, 64<<10), 4<<20)` for 4 MB max line; on JSON parse error of trailing partial line, retry up to 3× with 100 ms backoff; refresh cache when mtime is newer than cached version; `List` applies filter + `TruncateDesc`; `Get` searches cache; `Close` no-op
- [ ] T042b Implement in-memory snapshot cache in `internal/store/jsonl/backend.go`: shared with watcher via package-level `Refresh()` hook so watcher's re-read and API request's re-read use the same parse pass; `sync.RWMutex` protects the cache
- [ ] T043 Run `go test ./internal/store/jsonl/...` — all pass; coverage ≥ 85%

### Wire-up for embedded mode

- [ ] T044 `cmd/muster/main.go`: when `BackendConfig.Mode == "embedded"`, construct `jsonl.NewJSONL(beadsDir)` and use as the Backend passed to services
- [ ] T045 Wire Backend into existing services (swap out `store.NewStore(seed)` with the real Backend); `internal/services/beads.go` must call `backend.List` / `backend.Get` instead of the old in-memory store
- [ ] T046 Run `go test ./...` — all M0 tests green; no data races (`-race`)

### Integration test

- [ ] T047 Add integration test: start muster against tmpdir with fixture `issues.jsonl`; `GET /api/v1/beads` returns fixture issues (count and IDs match); `GET /api/v1/beads/{id}` returns specific issue

**Checkpoint**: `muster serve --beads-dir ~/repos/beads-central/muster/.beads` serves live `muster-*` issues.

---

## Phase 5: User Story 4 — Live reload when beads change externally (P2)

**Goal**: WS clients receive `bead.updated` within 2s of `bd` mutating `issues.jsonl`.

**Independent Test**: Modify `issues.jsonl` in a tmpdir; assert WS event arrives within 2s.

### Tests first

- [ ] T050 Write `internal/store/watcher_test.go`: tmpdir with `issues.jsonl`; subscribe to watcher channel; assert `Run()` populates initial snapshot synchronously (no flood of `bead.created` events on first real change); write updated `issues.jsonl` via atomic rename; assert `WatcherEvent` received within 2s with correct `ChangedIDs`; test bulk write → single debounced event; test deep-equality diff (identical content rewrite → no event emitted); test go test -race clean
- [ ] T051 Run `go test ./internal/store/...` — confirm watcher tests FAIL

### Implementation

- [ ] T052 Write `internal/store/watcher.go`: `Watcher` struct (backend, path, snapshot, out, debounce=500ms, pollEvery=5s); `NewWatcher(backend Backend, jsonlPath string, out chan<- WatcherEvent) *Watcher`
- [ ] T053 `watcher.go`: `Run(ctx)` — **first** calls `backend.List(ctx, Filter{})` synchronously and populates `snapshot` BEFORE starting fsnotify (eliminates startup flood); then starts fsnotify watch on `jsonlPath`; handles Write/Create/Rename events; re-adds watch after Rename (atomic write); 500ms trailing debounce
- [ ] T054 `watcher.go`: on debounce fire — calls `backend.List(ctx, Filter{})`, diffs against `snapshot` with **deep equality** per Issue (use `reflect.DeepEqual` or struct comparison); compute `ChangedIDs` (existing IDs whose content differs), `CreatedIDs` (new IDs), `DeletedIDs` (vanished IDs); skip emit when all three are empty; updates snapshot
- [ ] T055 `watcher.go`: polling fallback — if `fsnotify.NewWatcher()` or `Add()` fails, fall back to 5s mtime polling loop; same initial-snapshot rule applies
- [ ] T056 `watcher.go`: `WatcherEvent` struct with `Source`, `ChangedIDs`, `CreatedIDs`, `DeletedIDs`, `At`
- [ ] T057 Run `go test -race ./internal/store/...` — all pass; coverage ≥ 80%

### Wire-up

- [ ] T058 `cmd/muster/main.go`: start `Watcher` after Backend opens; fan `WatcherEvent.ChangedIDs` into WS hub as `bead.updated`; fan `CreatedIDs` as `bead.created`; fan `DeletedIDs` as `bead.deleted`
- [ ] T059 Integration test: write to `issues.jsonl` in tmpdir → assert WS event within 2s

**Checkpoint**: `bd close <id>` → WS `bead.updated` event appears within 2s.

---

## Phase 6: User Story 5 — Write mutations via `bd` CLI (P2)

**Goal**: PATCH/POST/move/dispatch shell out to `bd` and return the updated issue.

**Independent Test**: `PATCH /api/v1/beads/{id} {"title":"new"}` → `bd show <id>` reflects the change.

### Tests first

- [ ] T060 Write `internal/store/bdshell/exec_test.go`: fake `bd` shell script in tmpdir added to PATH; tests for — exit 0, exit 1 (→ CLIError 422), exit 2 (→ 404), exit 3 (→ 503), timeout (→ 504), ANSI stripping, 512-byte truncation, BEADS_DIR set correctly in subprocess env, PATH and HOME inherited only, `Update`/`Create`/`Close`/`Dispatch`/`AppendNote` helpers always pass `--json --dolt-auto-commit=on`, user value starting with `-` is passed safely as `--flag=value` argv form
- [ ] T061 Run `go test ./internal/store/bdshell/...` — confirm tests FAIL

### Implementation

- [ ] T062 Write `internal/store/bdshell/exec.go`: `type CLI struct { Path, BeadsDir string; Timeout time.Duration }`; `type CLIError struct { ExitCode int; Stderr string }`; `type Result struct { Stdout, Stderr string }`
- [ ] T063 `exec.go`: `NewCLI(bdBin, beadsDir string) (*CLI, error)` — `exec.LookPath` if bdBin empty; returns `ErrCLIMissing` if not found
- [ ] T064 `exec.go`: `CLI.Run(ctx, args ...string) (Result, error)` — minimal env (BEADS_DIR, PATH, HOME only); `exec.CommandContext` with derived timeout context; SIGTERM on timeout, 1s grace, SIGKILL; exit-code → error mapping; strip ANSI, truncate stderr to 512 bytes
- [ ] T064b Write `internal/store/bdshell/verbs.go`: typed helpers — `Create(ctx, in CreateInput) (store.Issue, error)`, `Update(ctx, id string, p UpdatePatch) (store.Issue, error)`, `Close(ctx, id string) error`, `Dispatch(ctx, id string) (store.Issue, error)`, `AppendNote(ctx, id, text string) (store.Issue, error)`, `DoltStart(ctx) error`. Each builds argv using `--flag=value` form, prepends `--json --dolt-auto-commit=on`, unmarshals JSON output into `store.Issue` (or `[]store.Issue` for update). User-supplied strings flow through as opaque `--flag=value` argv elements
- [ ] T065 Run `go test -race ./internal/store/bdshell/...` — all pass; coverage ≥ 85%

### API handler updates

- [ ] T066 Update `PATCH /api/v1/beads/{id}` handler: build `UpdatePatch` from request body fields; empty patch → 400 INVALID_REQUEST; call `bdshell.CLI.Update(ctx, id, patch)`; return 200 with the `store.Issue` (mapped through `IssueToBead`)
- [ ] T067 Update `POST /api/v1/beads` handler: validate `title` and `type` present; call `bdshell.CLI.Create(ctx, input)` which uses `bd create --json`; the response IS the new issue (no separate re-read needed); return 201 with the mapped Bead
- [ ] T068 Update `POST /api/v1/beads/{id}/move` handler: switch on `toColumn` — `done` → `CLI.Close`; `running` → `CLI.Update --claim`; other → `CLI.Update --status=<state>`; return 200
- [ ] T069 Update `POST /api/v1/beads/{id}/dispatch` handler: call `bdshell.CLI.Dispatch(ctx, id)` (which is `bd update <id> --claim --json`)
- [ ] T070 Update `POST /api/v1/beads/{id}/comments` handler: call `bdshell.CLI.AppendNote(ctx, id, text)` (which uses `bd update <id> --append-notes=<text> --json`). The `actor` field from the request body is prepended into the note text (e.g., `<actor>: <text>`) since `bd` v1.0 has no `--actor` flag
- [ ] T071 All write handlers: when `CLI == nil` (bd not found at startup), return `501 NOT_IMPLEMENTED` with `BD_CLI_MISSING` code
- [ ] T072 All write handlers: map `CLIError.ExitCode` to HTTP (1→422, 2→404, 3→503); `DeadlineExceeded`→504; other non-zero→500 INTERNAL
- [ ] T072b All write handlers: reject request body field values containing the NUL byte or longer than 64 KB per field (defensive — `bd` may behave unexpectedly with embedded NUL)

### Integration test

- [ ] T073 Write integration test with fake `bd` script: `PATCH /{id}` → script writes updated `issues.jsonl` → 200 OK with updated fields; `POST /beads` → script echoes new ID → 201 with full DTO; bad `bd` exit → correct HTTP code

**Checkpoint**: All write endpoints delegate to `bd`; correct HTTP codes on every `bd` exit.

---

## Phase 7: User Story 3 — Server mode: Dolt SQL backend (P2)

**Goal**: `muster serve` with `dolt_mode: remote` connects to Dolt via MySQL wire.

**Independent Test**: Connect muster against a live Dolt server (when `DOLT_TEST_DSN` set); `GET /api/v1/beads` returns real issues.

### Tests first

- [ ] T080 Write `internal/store/dolt/backend_test.go`: skip entire file when `DOLT_TEST_DSN` env not set; tests for — `List` all, `List` by status, `Get` existing, `Get` missing → ErrNotFound, `Close` idempotent, unreachable DSN → error on construction
- [ ] T081 Write `internal/store/dolt/query_test.go`: unit tests for query building (parameterized WHERE clauses)
- [ ] T082 Run `go test ./internal/store/dolt/...` — confirm tests FAIL (or skip if no DSN)

### Implementation

- [ ] T083 Write `internal/store/dolt/query.go`: `const listSQL` (SELECT with all Issue columns); `buildListArgs(f Filter) (where string, args []interface{})`; `const getSQL`
- [ ] T084 Write `internal/store/dolt/backend.go`: `NewDolt(ctx context.Context, dsn string) (store.Backend, error)` — `sql.Open("mysql", dsn)`, ping with ctx, return wrapped `ErrStoreUnavailable` on failure; `List` uses `listSQL`; `Get` uses `getSQL`; `Close` closes pool
- [ ] T085 Run `go test ./internal/store/dolt/...` — pass (or skip cleanly); coverage ≥ 75%

### Wire-up for server mode

- [ ] T086 `cmd/muster/main.go`: when `BackendConfig.Mode == "remote"`, call `bdshell.CLI.DoltStart(ctx)` (idempotent); build DSN from `BackendConfig.{DoltUser,DoltPassword,DoltHost,DoltPort,DoltDatabase}` — password from `BEADS_DOLT_PASSWORD` env (loaded into BackendConfig in T026); construct `dolt.NewDolt(ctx, dsn)` with `parseTime=true&collation=utf8mb4_0900_ai_ci`; exit 1 on any failure with message `cannot connect to dolt server: <err>`
- [ ] T087 Integration test (skipped without Dolt): start muster in server mode; `GET /api/v1/beads` returns issues from Dolt

**Checkpoint**: Both backends (JSONL and Dolt SQL) select correctly based on `dolt_mode`.

---

## Phase 8: User Story 6 — Schema version guard (P3)

**Goal**: Startup rejects databases with incompatible schema versions.

**Independent Test**: `metadata.json` with `schema_version: 99` → exit 1 with clear message.

### Implementation (covered by config layer tests — extend them)

- [ ] T090 Extend `internal/config/metadata_test.go`: test schema_version 1 (pass), 2 (pass), 3 (fail with correct message), absent (defaults to 1, passes)
- [ ] T091 Verify `LoadBackendConfig` enforces `schema_version` in `[1, 2]`; error message: `"beads schema v<N> not supported by muster (need 1..2)"`
- [ ] T092 Integration test: start muster with `schema_version: 99` → exit 1 with schema error message

**Checkpoint**: Invalid schema version causes clean exit 1 with actionable error.

---

## Phase 9: Polish & Cross-Cutting Concerns

**Purpose**: Documentation, coverage gates, final integration pass.

- [ ] T100 Run `make cover-check` — all coverage gates pass (see checklist CHK180–CHK188)
- [ ] T101 Run `go test -race ./...` — zero data races
- [ ] T102 Verify startup banner output matches `quickstart.md` expected output (section 2)
- [ ] T103 Walk through `quickstart.md` steps 1–5 manually (or via integration test) against `~/repos/beads-central/muster/.beads`
- [ ] T104 `SC-001`: `GET /api/v1/beads` total matches `bd stats` count
- [ ] T105 `SC-002`: `bd close <id>` → WS event within 2s (manual or automated test)
- [ ] T106 `SC-003`: `PATCH /api/v1/beads/{id}` persists; `bd show <id>` confirms
- [ ] T107 Update `README.md` with `--beads-dir` and `--bd-bin` flags
- [ ] T108 `go vet ./...` and `golangci-lint run` — clean

---

## Dependencies & Execution Order

### Phase Dependencies

- **Phase 1 (Setup)**: No dependencies — start immediately
- **Phase 2 (Foundational)**: Depends on Phase 1 — **BLOCKS all user stories**
- **Phases 3–4 (US1, US2)**: Both depend on Phase 2; can run in parallel; both P1 — do together
- **Phase 5 (US4 — Watcher)**: Depends on Phase 4 (needs a Backend to diff against)
- **Phase 6 (US5 — Writes)**: Depends on Phase 3 wire-up (needs `cmd/muster/main.go` skeleton)
- **Phase 7 (US3 — Dolt SQL)**: Depends on Phase 2; independent of Phases 3–6
- **Phase 8 (US6 — Schema guard)**: Already covered in Phase 2 config; T090–T092 are extensions
- **Phase 9 (Polish)**: Depends on all implementation phases

### Critical Path

```
Phase 1 → Phase 2 → Phase 3+4 (parallel) → Phase 5 → Phase 6 → Phase 9
                  ↘ Phase 7 (independent, can run in parallel with 3–6)
```

### Parallel Opportunities

- T002, T003, T004: go get commands — parallel
- T010, T011, T012: defining types — parallel (different files)
- T020–T027 (config TDD) and T010–T014 (interface types) — parallel within Phase 2
- Phase 7 (Dolt SQL backend) — fully independent, can run in parallel with Phases 5–6
- T060–T065 (bdshell) — independent from T050–T057 (watcher); run in parallel

---

## Notes

- `[P]` tasks have no intra-phase dependencies — safe to parallelize
- Tests MUST fail before implementation (TDD)
- Commit after each checkpoint
- CHK items in `checklists/implementation.md` map to these tasks for final verification
- All `go test` runs include `-race` flag
