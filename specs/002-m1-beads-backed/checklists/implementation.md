# Implementation Checklist: M1 — Beads-Backed Store

**Purpose**: Step-by-step verification for every deliverable in M1.
**Created**: 2026-05-24
**Feature**: [spec.md](../spec.md) | [plan.md](../plan.md)

---

## 1. Repository Setup

- [ ] CHK001 `cmd/muster/` directory exists (rename from `cmd/musterd/` if needed)
- [ ] CHK002 `Makefile` `PKG := ./cmd/muster/` and `BINARY := bin/muster` are correct
- [ ] CHK003 `go get github.com/go-sql-driver/mysql@v1.8.x` added to `go.mod`
- [ ] CHK004 `go get github.com/fsnotify/fsnotify@v1.7.x` added to `go.mod`
- [ ] CHK005 `go get gopkg.in/yaml.v3@v3.0.x` added to `go.mod`
- [ ] CHK006 `go mod tidy` passes cleanly
- [ ] CHK007 `go build ./...` succeeds with no errors
- [ ] CHK008 `idPattern` in `internal/api/beads/handlers.go` updated from `^bd-[0-9a-f]{4}$` to accept real beads IDs (e.g. `mp-kbj`, `muster-xyz`)
- [ ] CHK009 Request body size middleware: `http.MaxBytesReader(w, r.Body, 1<<20)` wraps all handlers; oversized bodies return 413 `PAYLOAD_TOO_LARGE`

---

## 2. Config Layer (`internal/config/`)

- [ ] CHK010 `loader.go`: `ResolveBeadsDir(flagVal, envVal string) (string, error)` — flag → `BEADS_DIR` env → `./.beads/` cwd fallback
- [ ] CHK011 `loader.go`: Returns `"beads-dir not found: set --beads-dir or BEADS_DIR"` when none found
- [ ] CHK012 `metadata.go`: `LoadMetadata(dir string) (*Metadata, error)` parses `<dir>/metadata.json`
- [ ] CHK013 `metadata.go`: Validates `database == "dolt"` and `backend == "dolt"`; returns error on mismatch
- [ ] CHK014 `metadata.go`: Validates `dolt_mode` is `"embedded"` or `"remote"`
- [ ] CHK015 `metadata.go`: Validates `dolt_database` is non-empty
- [ ] CHK016 `metadata.go`: Defaults `schema_version` to `1` when absent
- [ ] CHK017 `metadata.go`: Returns error with exact message `"invalid beads-dir: metadata.json not found at <path>"` when file missing
- [ ] CHK018 `loader.go`: `LoadBackendConfig(dir string) (BackendConfig, error)` composes resolved config
- [ ] CHK019 `loader.go`: Validates `schema_version` in `[1, 2]`; returns `"beads schema v<N> not supported by muster (need 1..2)"` on mismatch
- [ ] CHK020 `loader_test.go`: Tests for flag/env/cwd fallback resolution
- [ ] CHK021 `metadata_test.go`: Tests for each failure mode (missing, unparseable, bad database, bad mode, empty database)
- [ ] CHK022 Coverage: `internal/config/` ≥ 85%

---

## 3. Store Backend Interface (`internal/store/`)

- [ ] CHK030 `store.go` (or `backend.go`): `Backend` interface defined with `List`, `Get`, `Close`
- [ ] CHK031 `store.go`: `Filter` struct: `Status []string`, `IDs []string`, `Limit int`
- [ ] CHK032 `store.go`: `Issue` struct with all fields matching `issues.jsonl` schema (see data-model.md)
- [ ] CHK033 `errors.go`: `ErrNotFound`, `ErrStoreUnavailable`, `ErrStoreReadOnly`, `ErrSchemaMismatch` defined
- [ ] CHK034 `memstore.go`: Existing in-memory backend still compiles and satisfies `Backend` interface
- [ ] CHK035 All M0 tests still pass after interface refactor
- [ ] CHK036 Coverage: `internal/store/` ≥ 80%
- [ ] CHK037 `internal/services/mapper.go`: `IssueToBeads` and `IssueToBead` functions implement full data-model.md mapping table (Status→Column, DeriveAssignee, History, zero-value fields for VCS/Branch/etc.)
- [ ] CHK038 `internal/services/mapper_test.go`: Tests for all status→column mappings, assignee fallback, history derivation, unknown status default
- [ ] CHK039 `internal/services/beads.go`: `BeadService` accepts `store.Backend` (reads) + `bdshell.CLI` (writes), not old `store.Store`
- [ ] CHK039a `internal/api/beads/handlers.go`: `Handlers` struct uses refactored `BeadService`, not old `store.Store` directly

---

## 4. JSONL Backend (`internal/store/jsonl/`)

- [ ] CHK040 `backend.go`: `NewJSONL(beadsDir string) (store.Backend, error)` constructor
- [ ] CHK041 `backend.go`: Reads `<beadsDir>/issues.jsonl` — returns clear error if file absent or > 64 MB
- [ ] CHK042 `backend.go`: Parses each line as a JSON object into `store.Issue`; `bufio.Scanner` with `Buffer(make([]byte, 0, 64<<10), 4<<20)` (4 MB max line); skips blank lines, logs unparseable lines
- [ ] CHK043 `backend.go`: `List(ctx, Filter)` returns all issues matching filter; returns `[]Issue{}` (non-nil) when none match
- [ ] CHK044 `backend.go`: `List` respects `Filter.Limit > 0`
- [ ] CHK045 `backend.go`: `List` respects `Filter.Status` (empty = all)
- [ ] CHK045a `backend.go`: `List` respects `Filter.TruncateDesc > 0` — caps `Issue.Description` to N bytes
- [ ] CHK046 `backend.go`: `Get(ctx, id)` returns `ErrNotFound` for unknown IDs and for `id == ""`
- [ ] CHK047 `backend.go`: `Close()` is a no-op; safe to call multiple times
- [ ] CHK047a `backend.go`: `Ping(ctx)` returns nil if `issues.jsonl` is still stat-readable; non-nil otherwise
- [ ] CHK048 `backend.go`: In-memory snapshot cache, refreshed when mtime is newer than cached version
- [ ] CHK048a `backend.go`: API-triggered read retries up to 3× with 100 ms backoff on JSON parse error (atomic-rename race window)
- [ ] CHK049 `backend.go`: All methods respect `ctx.Err()` on entry
- [ ] CHK050 `backend_test.go`: Tests with a fixture `issues.jsonl` containing ≥3 diverse records
- [ ] CHK051 `backend_test.go`: Test: file absent → error
- [ ] CHK052 `backend_test.go`: Test: filter by status works
- [ ] CHK053 `backend_test.go`: Test: filter by IDs works
- [ ] CHK054 `backend_test.go`: Test: `Get` for existing and missing IDs
- [ ] CHK055 Coverage: `internal/store/jsonl/` ≥ 85%

---

## 5. Dolt SQL Backend (`internal/store/dolt/`)

- [ ] CHK060 `backend.go`: `NewDolt(ctx context.Context, dsn string) (store.Backend, error)` constructor
- [ ] CHK061 `backend.go`: Opens connection with `database/sql.Open("mysql", dsn)` and `parseTime=true`
- [ ] CHK062 `backend.go`: Pings the connection at startup; returns wrapped `ErrStoreUnavailable` on failure
- [ ] CHK063 `query.go`: `SELECT id, title, description, status, priority, issue_type, assignee, owner, created_at, updated_at, started_at, closed_at, close_reason, dependency_count, dependent_count, comment_count FROM issues`
- [ ] CHK064 `query.go`: Parameterized `Get` query: `WHERE id = ?`
- [ ] CHK065 `backend.go`: `List` respects `Filter.Status` via SQL `WHERE status IN (...)`
- [ ] CHK066 `backend.go`: `List` respects `Filter.Limit` via SQL `LIMIT ?`
- [ ] CHK067 `backend.go`: `Close()` closes the `*sql.DB`; safe to call multiple times
- [ ] CHK068 `backend.go`: All methods respect `ctx.Err()`; queries use context-aware variants
- [ ] CHK069 `backend_test.go`: Skips when `DOLT_TEST_DSN` env var not set
- [ ] CHK070 `backend_test.go`: Integration test against real Dolt (when env set): List, Get, filter
- [ ] CHK071 Coverage: `internal/store/dolt/` ≥ 75%

---

## 6. Watcher (`internal/store/watcher.go`)

- [ ] CHK080 `Watcher` struct: `backend Backend`, `path string`, `snapshot map[string]Issue`, `out chan<- WatcherEvent`, `debounce time.Duration`, `pollEvery time.Duration`
- [ ] CHK081 `NewWatcher(backend Backend, jsonlPath string, out chan<- WatcherEvent) *Watcher`
- [ ] CHK082 `Watcher.Run(ctx context.Context)`: starts fsnotify watch on `jsonlPath`
- [ ] CHK083 fsnotify watches for `Write | Create | Rename` events; re-adds watch after `Rename` (atomic write)
- [ ] CHK084 Debounce: 500ms trailing window — first event starts timer, subsequent events reset it
- [ ] CHK084a `Watcher.Run(ctx)` populates `snapshot` SYNCHRONOUSLY via `backend.List` BEFORE starting fsnotify (eliminates startup `bead.created` flood)
- [ ] CHK085 On debounce fire: calls `backend.List(ctx, Filter{})`, diffs against `snapshot` using deep-equality per Issue, emits `WatcherEvent`
- [ ] CHK085a Empty deltas (no actual content changes) are suppressed (no event emitted)
- [ ] CHK086 `WatcherEvent.ChangedIDs`, `CreatedIDs`, `DeletedIDs` correctly populated from diff
- [ ] CHK087 Polling fallback: when `fsnotify.NewWatcher()` or `Add()` fails, falls back to 5s mtime polling
- [ ] CHK088 `WatcherEvent.Source`: `SourceFSEvent` vs `SourcePoll` correctly set
- [ ] CHK089 `snapshot` only accessed from watcher goroutine (no data race)
- [ ] CHK090 `watcher_test.go`: tmpdir with `issues.jsonl`; trigger write; assert event within 2s
- [ ] CHK091 `watcher_test.go`: Bulk write (multiple bd-like mutations) → single debounced event
- [ ] CHK091a `watcher_test.go`: Identical-content rewrite → NO event emitted (deep-equality diff)
- [ ] CHK091b `watcher_test.go`: Empty initial state + first fsnotify event → no flood of `bead.created` events (snapshot bootstrap works)
- [ ] CHK092 `watcher_test.go`: go test -race passes
- [ ] CHK093 Coverage: `internal/store/watcher.go` ≥ 80%

---

## 7. `bd` Write Bridge (`internal/store/bdshell/`)

- [ ] CHK100 `exec.go`: `type CLI struct { Path string; BeadsDir string; Timeout time.Duration }`
- [ ] CHK101 `exec.go`: `CLI.Run(ctx context.Context, args ...string) (Result, error)` — core execution
- [ ] CHK102 `exec.go`: Subprocess env = `BEADS_DIR`, `PATH`, `HOME` only (minimal env — no other vars inherited)
- [ ] CHK103 `exec.go`: Timeout via `exec.CommandContext`; on timeout: SIGTERM, 1s grace, SIGKILL
- [ ] CHK104 `exec.go`: Exit code 0 → `nil` error
- [ ] CHK105 `exec.go`: Exit code 1 → `&CLIError{ExitCode: 1, Stderr: ...}` mapped to HTTP 422
- [ ] CHK106 `exec.go`: Exit code 2 → `&CLIError{ExitCode: 2}` mapped to HTTP 404
- [ ] CHK107 `exec.go`: Exit code 3 → wrapped `ErrStoreUnavailable` mapped to HTTP 503
- [ ] CHK108 `exec.go`: Timeout → `context.DeadlineExceeded` mapped to HTTP 504
- [ ] CHK109 `exec.go`: Stderr truncated to 512 bytes; ANSI color codes stripped
- [ ] CHK110 `exec.go`: `ErrCLIMissing` returned by `NewCLI` when `bd` not found on PATH
- [ ] CHK111 `exec.go`: `--bd-bin` flag / `BD_BIN` env override the PATH lookup
- [ ] CHK111a `verbs.go`: Typed helpers `Create`, `Update`, `Close`, `Dispatch`, `AppendNote`, `DoltStart` — each always passes `--json --dolt-auto-commit=on`
- [ ] CHK111b `verbs.go`: All user-supplied values passed as `--flag=value` form (single argv element); `--` separator inserted before positionals
- [ ] CHK111c `verbs.go`: `AppendNote` uses `--append-notes=<v>` (NOT `--notes`); actor prepended into note text since `bd` has no `--actor` flag
- [ ] CHK111d `verbs.go`: `Create` unmarshals `bd create --json` output (single JSON object) into `store.Issue`
- [ ] CHK111e `verbs.go`: `Update`/`Close`/`Dispatch`/`AppendNote` unmarshal `bd update --json` output (JSON array) and return first element
- [ ] CHK112 `exec_test.go`: Uses a fake `bd` shell script on PATH that echoes args and exits with controlled codes
- [ ] CHK113 `exec_test.go`: Tests for exit 0, 1, 2, 3, timeout
- [ ] CHK114 `exec_test.go`: Tests ANSI stripping and 512-byte truncation
- [ ] CHK114a `verbs_test.go`: argv injection test — value `"--priority=0"` passed as title does NOT alter priority
- [ ] CHK114b `verbs_test.go`: `--dolt-auto-commit=on` is always present in every invocation
- [ ] CHK115 Coverage: `internal/store/bdshell/` ≥ 85%

---

## 8. Wire-up: `cmd/muster/main.go`

- [ ] CHK120 `--beads-dir` flag wired to `config.ResolveBeadsDir`
- [ ] CHK121 `--bd-bin` flag / `BD_BIN` env wired to `bdshell.NewCLI`
- [ ] CHK122 `metadata.json` loaded at startup; exits 1 with clear error on any validation failure
- [ ] CHK123 Backend constructed from `BackendConfig.Mode`: `jsonl.NewJSONL` for embedded, `dolt.NewDolt` after `bd dolt start` for server
- [ ] CHK124 Server mode: `bd dolt start` called (via `bdshell`) before `dolt.NewDolt`; exits 1 on failure
- [ ] CHK125 `bd` CLI: startup warning logged if not found; write endpoints return `501 NOT_IMPLEMENTED`
- [ ] CHK126 Watcher started after Backend opens; watches `<beadsDir>/issues.jsonl`
- [ ] CHK127 WatcherEvents fanned to WebSocket hub (bead.updated / bead.created / bead.deleted)
- [ ] CHK128 Startup banner includes: `beadsDir`, `doltDatabase`, `doltMode`, `readSource`, `bdCLI`
- [ ] CHK129 `X-Beads-Dir` and `X-Beads-Database` response headers added to all API responses
- [ ] CHK130 Graceful shutdown: Backend.Close() called on SIGINT/SIGTERM
- [ ] CHK131 `--store=memory` escape hatch still works (uses M0 in-memory backend)

---

## 9. API Handler Updates

- [ ] CHK140 `PATCH /api/v1/beads/{id}`: calls `bdshell.CLI.Update`; the `bd update --json` response IS the updated issue; returns 200 (no re-read needed)
- [ ] CHK141 `PATCH /api/v1/beads/{id}`: empty body (after stripping unknown fields) → 400 `INVALID_REQUEST` without calling `bd`
- [ ] CHK142 `POST /api/v1/beads`: calls `bdshell.CLI.Create`; unmarshals `bd create --json` output directly into `store.Issue`; returns 201 (no stdout text parsing, no re-read)
- [ ] CHK143 `POST /api/v1/beads/{id}/move {"toColumn":"done"}`: calls `bdshell.CLI.Close`
- [ ] CHK144 `POST /api/v1/beads/{id}/move {"toColumn":"running"}`: calls `bdshell.CLI.Update` with `--claim`
- [ ] CHK145 `POST /api/v1/beads/{id}/move` (other columns): calls `bdshell.CLI.Update` with `--status=<state>`
- [ ] CHK146 `POST /api/v1/beads/{id}/dispatch`: calls `bdshell.CLI.Dispatch` (`--claim --json`)
- [ ] CHK147 `POST /api/v1/beads/{id}/comments`: calls `bdshell.CLI.AppendNote` (uses `--append-notes`, NOT `--notes`); actor prepended into note text
- [ ] CHK148 All write endpoints: `bd` exit 2 → 404; exit 1 → 422; exit 3 → 503; timeout → 504; other non-zero → 500
- [ ] CHK148a All write endpoints: field values containing NUL byte or > 64 KB rejected with 422
- [ ] CHK148b `GET /api/v1/beads` (list) truncates `description` at 2 KB (FR-016)
- [ ] CHK148c `GET /api/v1/beads/{id}` returns full description (no truncation)

---

## 10. WebSocket Events

- [ ] CHK150 `bead.updated` event emitted when `WatcherEvent.ChangedIDs` is non-empty (with existing ID)
- [ ] CHK151 `bead.created` event emitted for IDs in `WatcherEvent.CreatedIDs`
- [ ] CHK152 `bead.deleted` event emitted for IDs in `WatcherEvent.DeletedIDs`
- [ ] CHK153 WS event payload matches M0 shape — no breaking changes (FR-014)
- [ ] CHK154 WS events arrive within 2s of `bd` mutation (SC-002)

---

## 11. `GET /api/v1/orchestrator/status`

- [ ] CHK160 Response body includes `beadsDir`, `doltDatabase`, `doltMode`, `schemaVersion`, `projectID`
- [ ] CHK161 Response body includes `readSource` (`"issues.jsonl"` or `"dolt-sql"`)
- [ ] CHK162 Response body includes `bdCLI` (path or `"(missing)"`)
- [ ] CHK163 `Online` field reflects `backend.Ping(ctx) == nil` (not hardcoded `true`)
- [ ] CHK164 `OrchestratorStatusHandler` constructor accepts `BackendConfig` and `Backend`, not the M0 `seedDolt`

---

## 12. Integration Tests

- [ ] CHK170 Test binary boots against tmpdir `.beads/` with fixture `issues.jsonl` (≥5 issues)
- [ ] CHK171 `GET /api/v1/beads` returns issues matching fixture (not seed data)
- [ ] CHK172 Mutate `issues.jsonl` in tmpdir → assert WS `bead.updated` event within 2s
- [ ] CHK173 `PATCH /api/v1/beads/{id}` with fake `bd` script → 200 OK; re-read returns updated
- [ ] CHK174 `muster serve` with missing `--beads-dir` → exits 1 with expected error message
- [ ] CHK175 `muster serve` with absent `issues.jsonl` → exits 1 with expected error message
- [ ] CHK176 `muster serve` with bad `metadata.json` → exits 1 with expected error message

---

## 13. Coverage Gates

- [ ] CHK180 `internal/config/` ≥ 85%
- [ ] CHK181 `internal/store/` (interface pkg) ≥ 80%
- [ ] CHK182 `internal/store/jsonl/` ≥ 85%
- [ ] CHK183 `internal/store/dolt/` ≥ 75%
- [ ] CHK184 `internal/store/bdshell/` ≥ 85%
- [ ] CHK185 `internal/store/watcher.go` ≥ 80%
- [ ] CHK186 `internal/services/` ≥ 80% (unchanged from M0)
- [ ] CHK187 `internal/core/` ≥ 80% (unchanged from M0)
- [ ] CHK188 `go test -race ./...` passes — zero data races

---

## 14. Documentation

- [ ] CHK190 `quickstart.md` step-by-step works against `~/repos/beads-central/muster/.beads`
- [ ] CHK191 Startup banner matches `quickstart.md` expected output
- [ ] CHK192 `README.md` (if exists): updated with new `--beads-dir` and `--bd-bin` flags
- [ ] CHK193 `make help` (if exists): updated for new flags

---

## Notes

- Check items off as completed: `[x]`
- CHK items reference spec FRs: CHK01x → FR-001/002, CHK04x → FR-003, CHK06x → FR-004, CHK08x → FR-006/007, CHK10x → FR-009/010/011
- Integration tests (CHK17x) are the final gate before Phase 9 Verify
