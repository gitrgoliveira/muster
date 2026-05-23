# Implementation Plan: M0 — Skeleton

**Branch**: `001-m0-skeleton` | **Date**: 2026-05-22 (rev) | **Spec**: [spec.md](spec.md)

**Input**: Feature specification from `/specs/001-m0-skeleton/spec.md`

## Summary

Build `musterd` — a single Go binary that binds `127.0.0.1:7766`, serves the embedded prototype 
UI via `go:embed`, exposes a fully in-memory REST API for beads CRUD + lifecycle actions, and 
pushes optimistic mutations to connected clients over a WebSocket hub. The implementation is 
**layered** (core → store → services → transport) with a **tests-first** sequence in each layer: 
no production code lands without a failing test first.

## Technical Context

**Language/Version**: Go 1.26/3

**Primary Dependencies**:

| Package | Version | Purpose |
|---|---|---|
| `github.com/go-chi/chi/v5` | `v5.2.1` | HTTP routing, middleware chain, sub-routers |
| `github.com/coder/websocket` | `v1.8.13` | WebSocket server |
| `github.com/google/uuid` | `v1.6.0` | UUIDv4 for bead ID generation |
| `github.com/stretchr/testify` | `v1.10.0` | `assert`/`require` test helpers |

**Storage**: in-memory `[]core.Bead`, `sync.RWMutex`

**Testing**: `go test ./...`; table-driven unit tests; `httptest.NewServer` for HTTP integration; 
WS tested via `nhooyr.io/websocket`'s test dialer (now `coder/websocket.Dial`). Race detector 
required (`go test -race ./...`).

**Target Platform**: macOS/Linux, local loopback in M0

**Project Type**: CLI / web-service hybrid (single binary)

**Performance Goals**: <10 ms p95 per API call; <100 ms p95 to fan a WS event to 10 clients

**Constraints**: self-contained binary; UI embedded at build time; no external services

**Scale/Scope**: 14 seed beads, <10 concurrent connections, single process

## Constitution Check

| Gate | Status | Notes |
|---|---|---|
| Single binary | PASS | `cmd/musterd/` produces one binary |
| No external DB | PASS | Pure in-memory |
| Embedded UI | PASS | `go:embed ui/*` + `fs.Sub` |
| Module path known | PASS | `github.com/gitrgoliveira/muster` |
| Test coverage targets | PASS | 80% core/, 80% services/, 70% api/ (per spec §17.1, tightened) |
| Tests-first per layer | PASS | This plan enforces TDD ordering in every phase below |
| Handlers separated from business logic | PASS | `internal/services/` is the only layer that mutates `Store` state; handlers are translators |

## Project Structure

### Documentation (this feature)

```text
specs/001-m0-skeleton/
├── plan.md              # This file
├── spec.md              # Feature spec (fully clarified)
├── research.md          # Phase 0 output — 10 architectural decisions
├── data-model.md        # Phase 1 output — typed enums, domain types, DTOs
├── quickstart.md        # Phase 1 output — getting started
├── contracts/
│   ├── rest-api.md      # REST endpoint contracts (with error matrix)
│   └── ws-events.md     # WebSocket event protocol
└── tasks.md             # Phase 2 output (speckit-tasks)
```

### Source Code Layout (restructured)

The previous flat `internal/api/` is split into **resource subpackages** so each HTTP resource
owns its handlers + DTOs + tests. Business logic lives in `internal/services/` so handlers stay
thin (decode → call service → encode). The store is a pure CRUD interface — no domain rules
inside it.

```text
cmd/musterd/
├── main.go              # --addr flag, wires services + hub + router, starts http.Server
└── embed.go             # //go:embed ui/* + //go:generate cp prototype/ ui/

internal/
├── core/                # pure domain types, no I/O, no deps beyond uuid
│   ├── enums.go         # BeadType, Column, StepStatus, AgentID, Mode, VCS, Priority, EventKind, Estimate
│   ├── bead.go          # Bead struct
│   ├── step.go          # Step, SubBead
│   ├── history.go       # HistoryEvent
│   ├── gate.go          # Gate, GateKind, GateStatus
│   ├── auxiliary.go     # Acceptance, NowPlaying, Reviewer, LogEntry, FileChange
│   ├── id.go            # NewBeadID()
│   ├── validate.go      # DeriveEstimate, DeriveAssignee, DeriveCommentCount, error sentinels
│   ├── enums_test.go
│   ├── id_test.go
│   └── validate_test.go
│
├── store/               # state persistence interface + in-memory impl
│   ├── store.go         # type Store interface { List, Get, Create, Patch, Move, Dispatch, AddComment }
│   ├── memstore.go      # in-memory implementation; sync.RWMutex
│   ├── seed.go          # SeedBeads() returns 14 hard-coded beads from data.jsx
│   ├── memstore_test.go
│   └── seed_test.go
│
├── services/            # business logic — only layer that mutates store state
│   ├── beads.go         # BeadService.{Create,Patch,Move,Dispatch,AddComment} + input types
│   ├── defaults.go      # applyCreateDefaults() — table from data-model.md §6
│   ├── events.go        # eventPublisher interface (decouples ws.Hub from services package)
│   └── beads_test.go    # tests services in isolation with a mock publisher
│
├── ws/                  # WebSocket Hub
│   ├── event.go         # Event, EventType
│   ├── hub.go           # Hub: register/unregister/broadcast channels, Run() loop, slow-client drop policy
│   ├── client.go        # Client: writePump, readPump
│   ├── hub_test.go      # tests via httptest.NewServer + ws.Dial
│   └── client_test.go
│
└── api/                 # HTTP transport layer — handlers only
    ├── router.go        # NewRouter(svc *services.BeadService, hub *ws.Hub, ui fs.FS) http.Handler
    ├── render/
    │   ├── json.go      # WriteJSON helper
    │   ├── errors.go    # ErrorResponse, error code constants, WriteError helper
    │   └── render_test.go
    ├── middleware/
    │   ├── requestid.go # X-Request-ID middleware
    │   └── requestid_test.go
    ├── beads/
    │   ├── handlers.go  # List, Create, Get, Patch, Move, Dispatch, Comment handlers
    │   ├── dto.go       # ListResponse, CreateRequest, PatchRequest, MoveRequest, DispatchRequest, CommentRequest
    │   └── handlers_test.go  # integration via httptest.NewServer with real services + memstore
    ├── stream/
    │   ├── handler.go   # WebSocket upgrade handler that registers a new ws.Client with the Hub
    │   └── handler_test.go
    └── health/
        ├── handler.go   # Healthz, OrchestratorStatus
        ├── dto.go
        └── handler_test.go

ui/                      # go:embed target
└── .gitkeep             # ensures dir exists on fresh clone

go.mod
go.sum
Makefile                 # build, test, run, ui-copy
```

### Dependency direction

```
api/  ── depends on ──>  services/  ── depends on ──>  store/, ws/, core/
                                                    \
                                                     >  core/
ws/   ── depends on ──>  core/
store/ ── depends on ──>  core/
```

Crucially, **`store/` does not import `services/`** and **`services/` does not import `api/`**.
`ws/` exposes a publisher interface that `services/` consumes, so `services/` does not import
`ws/` directly — the wiring happens in `cmd/musterd/main.go`.

## Phase 0: Research

**Status**: complete — see [research.md](research.md) for the 10 decisions.

Revisions to research decisions to reflect the restructured layout:
- **Decision 2 (WS hub)** still stands but the Hub exposes a `Broadcast(Event)` method that
  `services/` calls via an interface — services do not depend on the concrete `ws.Hub`.
- **Decision 7 (error helper)** moves from `internal/api/helpers.go` to `internal/api/render/errors.go`.
- **Decision 8 (chi router)** is now assembled in `internal/api/router.go` by mounting sub-routers
  from each resource subpackage (`/beads`, `/healthz`, `/orchestrator/status`, `/stream`).

## Phase 1: Design & Contracts

**Status**: complete

- [data-model.md](data-model.md) — typed enums, domain types, transport DTOs, default values, edge cases
- [contracts/rest-api.md](contracts/rest-api.md) — endpoint-by-endpoint contract with error matrix
- [contracts/ws-events.md](contracts/ws-events.md) — WS protocol, concurrency model, slow-client policy
- [quickstart.md](quickstart.md) — build + run

## Implementation Phases (tests-first per layer)

Each phase below follows the same micro-pattern:

> 1. Write **failing tests** that pin the API of the package.
> 2. Write the **minimum code** to compile.
> 3. Iterate until tests pass.
> 4. Add edge-case tests, refactor.
> 5. Run `go test -race ./...` before moving on.

Phases run **in order**; layers within a phase may be split into parallel tasks where files do
not overlap.

### Phase 1 — Scaffolding

| Step | Artifact | Notes |
|---|---|---|
| 1.1 | `go.mod` | `module github.com/gitrgoliveira/muster`, Go 1.26, deps pinned |
| 1.2 | `ui/.gitkeep` | empty file so `//go:embed ui/*` succeeds on fresh clone |
| 1.3 | `cmd/musterd/embed.go` | `//go:embed ui/*` + `//go:generate cp -r ../../prototype/ ../../ui/` |
| 1.4 | `cmd/musterd/main.go` | bare skeleton: parse `--addr`, print banner to stdout, exit |
| 1.5 | `Makefile` | `build`, `test`, `run`, `ui-copy`, `cover` targets |

No tests in this phase — pure scaffolding.

### Phase 2 — `internal/core/` (tests-first)

| Step | Tests | Implementation |
|---|---|---|
| 2.1 | `enums_test.go` — `TestBeadType_Valid`, `TestColumn_Valid`, `TestPriority_Valid`, `TestStepStatus_Valid`, `TestMode_Valid`, `TestAgentID_Valid`, `TestVCS_Valid` (all table-driven) | `enums.go` |
| 2.2 | `id_test.go` — `TestNewBeadID_Format` (regex match), `TestNewBeadID_Uniqueness` (10k generations, expect <0.5% collision rate to validate retry policy) | `id.go` |
| 2.3 | `validate_test.go` — `TestDeriveEstimate` (table for XS/S/M/L thresholds), `TestDeriveAssignee` (active step, last done, none), `TestDeriveCommentCount` (history + reviewer) | `validate.go` |
| 2.4 | _no tests for_ `bead.go`, `step.go`, `history.go`, `gate.go`, `auxiliary.go` — pure data carriers; covered transitively through services tests | the struct files |

**Exit criterion**: `go test ./internal/core/...` passes with race detector; ≥80% coverage.

### Phase 3 — `internal/store/` (tests-first)

| Step | Tests | Implementation |
|---|---|---|
| 3.1 | `seed_test.go` — `TestSeedBeads_Count` (14), `TestSeedProviders_Count` (4), `TestSeedCapacity_Count` (4), `TestSeedDolt_NonZero`, `TestSeedBeads_HaveValidEnums`, `TestSeedBeads_DerivedFields` (estimate matches, assignee matches, history non-empty) | `seed.go` — populates beads, providers, capacity, dolt |
| 3.2 | `memstore_test.go` — `TestList`, `TestList_FilterByColumn`, `TestList_PreservesInsertionOrder`, `TestGet_Found`, `TestGet_Missing`, `TestCreate`, `TestCreate_IDCollisionRetry` (inject deterministic IDs; 3 attempts then `ErrIDExhausted`), `TestPatch_PartialFields`, `TestPatch_NotFound`, `TestMove_AppendsToColumn`, `TestMove_BeforeID_Reorders`, `TestMove_BeforeID_UnknownReturnsError`, `TestMove_BeforeID_DifferentColumnReturnsError`, `TestMove_BeforeID_SameAsMovedReturnsError`, `TestDispatch_AppendsStep_ChangesColumn_AppendsTwoHistoryEvents`, `TestDispatch_FromNonScheduledReturnsErrInvalidState`, `TestAddComment_AppendsHistoryAndIncrementsCount` | `store.go` (interface), `memstore.go` — store maintains slice in column display order; `Move` uses splice-out/splice-in |
| 3.3 | `memstore_concurrency_test.go` — `TestStore_ConcurrentReadsWrites` (`-race`, N goroutines hammering List + Patch + Move) | (concurrency proven by tests, no new code) |

**Exit criterion**: `go test -race ./internal/store/...` passes; ≥80% coverage.

### Phase 4 — `internal/services/` (tests-first)

| Step | Tests | Implementation |
|---|---|---|
| 4.1 | `beads_test.go` — table-driven validation tests for `Create` (missing title, invalid enums, whitespace, etc.) with a stub `Store` | `beads.go` (interfaces), `defaults.go` |
| 4.2 | `beads_test.go` — `TestPatch_AllFields` (each pointer field exercised), `TestPatch_EmptyBodyReturnsInvalidRequest`, `TestMove_ColumnChange`, `TestMove_BeforeID_Reorder`, `TestMove_BeforeID_DifferentColumnReturnsInvalidRequest`, `TestDispatch_Scheduled_Succeeds`, `TestDispatch_NonScheduled_ReturnsInvalidState`, `TestDispatch_AppendsStepAndTwoHistoryEvents`, `TestAddComment_AppendsAndCounts` | `beads.go` (implementations) |
| 4.3 | `events_test.go` — `TestService_PublishesCreatedEvent`, `TestService_PublishesUpdatedEvent_ForPatchAndDispatch`, `TestService_PublishesMovedEvent_ForMove`, `TestService_PublishesCommentAddedEvent` (mock publisher records calls + event types) | `events.go` (publisher interface) |

**Exit criterion**: `go test -race ./internal/services/...` passes; ≥80% coverage.

### Phase 5 — `internal/ws/` (tests-first)

| Step | Tests | Implementation |
|---|---|---|
| 5.1 | `hub_test.go` — `TestHub_RegisterUnregister`, `TestHub_HelloSentWithinOneSecondOfRegister`, `TestHub_BroadcastReachesClient` (via real `httptest.NewServer` and `websocket.Dial`) | `event.go` (7 event types + Frame union), `hub.go` |
| 5.2 | `hub_test.go` — `TestHub_SlowClientDropped` (fill send buffer, assert WARN-then-unregister after 3 drops within 10s) | extend `hub.go` |
| 5.3 | `client_test.go` — `TestClient_WritePumpExitsOnContextCancel`, `TestClient_ReadPumpExitsOnClose`, `TestClient_PingFrameProducesPong`, `TestClient_UnknownClientFrameLoggedAndIgnored` | `client.go` (readPump handles `{"type":"ping"}` → pong) |

**Exit criterion**: `go test -race ./internal/ws/...` passes; ≥75% coverage (lower because pump
goroutines have hard-to-cover error paths).

### Phase 6 — `internal/api/render/` + `internal/api/middleware/` (tests-first)

| Step | Tests | Implementation |
|---|---|---|
| 6.1 | `render_test.go` — `TestWriteJSON_SetsHeadersAndStatus`, `TestWriteError_AllCodes` (covers `BEAD_NOT_FOUND`, `NOT_FOUND`, `INVALID_REQUEST`, `INVALID_STATE`, `METHOD_NOT_ALLOWED`, `INTERNAL`), `TestWriteError_IncludesRequestID` | `json.go`, `errors.go` |
| 6.2 | `requestid_test.go` — `TestRequestID_EchoesSupplied`, `TestRequestID_GeneratesWhenAbsent`, `TestRequestID_AvailableInContext`, `TestRequestID_OnErrorResponses` (FR-017 acceptance) | `requestid.go` |
| 6.3 | `bodylimit_test.go` — `TestBodyLimit_RejectsOversize_400_InvalidRequest`, `TestBodyLimit_AcceptsBelowLimit`, `TestBodyLimit_MessageContainsLimit` | `bodylimit.go` (1 MiB cap middleware applied to POST/PATCH) |

**Exit criterion**: green; ≥90% coverage (small utilities — full coverage cheap).

### Phase 7 — `internal/api/health/` (tests-first)

| Step | Tests | Implementation |
|---|---|---|
| 7.1 | `handler_test.go` — `TestHealthz_Returns200_AndOK`, `TestOrchestratorStatus_ReturnsFullPayload` (asserts all 6 fields present: `build`, `schemaVersion`, `beadsVersion`, `online`, `serverTime`, `dolt`), `TestOrchestratorStatus_DoltMatchesSeed` (every dolt field equals the seeded value) | `handler.go`, `dto.go` |

### Phase 8 — `internal/api/beads/` (tests-first)

This is the largest phase — one integration test file per endpoint.

| Step | Tests | Implementation |
|---|---|---|
| 8.1 | `handlers_test.go` — `TestList_NoFilter`, `TestList_FilterByColumn`, `TestList_InvalidColumn_400` | `List` handler, `dto.go` (`ListResponse`) |
| 8.2 | `handlers_test.go` — `TestCreate_201_WithDefaults`, `TestCreate_400_MissingTitle`, `TestCreate_400_InvalidEnum`, `TestCreate_400_UnknownField`, `TestCreate_BroadcastsWSEvent` (real Hub + WS client) | `Create` handler |
| 8.3 | `handlers_test.go` — `TestGet_200`, `TestGet_404`, `TestGet_400_BadIDFormat` | `Get` handler |
| 8.4 | `handlers_test.go` — `TestPatch_PartialUpdate`, `TestPatch_404`, `TestPatch_400_NullField`, `TestPatch_400_UnknownField`, `TestPatch_EmptyBodyIsNoop`, `TestPatch_BroadcastsWSEvent` | `Patch` handler |
| 8.5 | `handlers_test.go` — `TestMove_200_ToColumn`, `TestMove_200_ToColumnAppendedToEnd`, `TestMove_200_WithBeforeID_Reorders`, `TestMove_400_MissingToColumn`, `TestMove_400_UnknownColumn`, `TestMove_400_UnknownBeforeID`, `TestMove_400_BeforeIDDifferentColumn`, `TestMove_400_BeforeIDEqualsMovedBead`, `TestMove_404`, `TestMove_EmitsBeadMovedEvent` (verify WS payload includes `fromColumn`/`toColumn`/`beforeID`) | `Move` handler |
| 8.6 | `handlers_test.go` — `TestDispatch_200_FromScheduled`, `TestDispatch_400_InvalidState_FromBacklog`, `TestDispatch_400_InvalidState_FromRunning`, `TestDispatch_AppendsStep`, `TestDispatch_AppendsClaimedAndStartedHistoryEvents`, `TestDispatch_ChangesColumnToRunning`, `TestDispatch_400_InvalidAgent`, `TestDispatch_400_InvalidMode`, `TestDispatch_404` | `Dispatch` handler |
| 8.7 | `handlers_test.go` — `TestComment_201_AppendsHistory`, `TestComment_IncrementsCount`, `TestComment_EmitsCommentAddedEvent` (verify WS payload has `event` field), `TestComment_400_MissingActor`, `TestComment_400_MissingNote`, `TestComment_404` | `Comment` handler (returns 201 per spec US9) |
| 8.8 | `handlers_test.go` — `TestPatch_400_EmptyBody` (empty `{}` → 400 INVALID_REQUEST) | covered in Patch handler |

**Exit criterion**: ≥70% coverage; every error-matrix row from `rest-api.md` has a test.

### Phase 9 — `internal/api/stream/` (tests-first)

| Step | Tests | Implementation |
|---|---|---|
| 9.1 | `handler_test.go` — `TestStream_UpgradesToWS`, `TestStream_SendsHelloWithinOneSecond` (per FR-013), `TestStream_PingProducesPong` (per FR-14), `TestStream_ReceivesBeadCreatedOnPostBeads`, `TestStream_ReceivesBeadMovedOnPostMove`, `TestStream_ReceivesBeadUpdatedOnPatch`, `TestStream_ReceivesCommentAddedOnPostComment`, `TestStream_DisconnectUnregistersClient` | `handler.go` |

### Phase 10 — `internal/api/router.go` + `cmd/musterd/main.go` (wiring)

| Step | Tests | Implementation |
|---|---|---|
| 10.1 | `router_test.go` — `TestRouter_StaticUIServed`, `TestRouter_APINotFound_ReturnsJSON`, `TestRouter_MethodNotAllowed_ReturnsJSON`, `TestRouter_PanicRecovered_Returns500JSON` | `router.go` assembling sub-routers |
| 10.2 | `cmd/musterd/main_test.go` — `TestServer_BootsAndServesUI`, `TestServer_GracefulShutdown_DrainsWithin5s_ExitCode0`, `TestServer_ParseAddr_IPv6`, `TestServer_ParseAddr_InvalidFormat_Exits1`, `TestServer_PortInUse_Exits1`, `TestNoSubcommand_PrintsUsageExits1` | finish `main.go`: **binary requires `serve` subcommand** (`os.Args[1]`; missing/unknown → print usage, exit 1); `serve` parses `--addr`; wire store/services/hub/router; start `http.Server`; install `signal.NotifyContext(SIGINT, SIGTERM)` handler with 5 s drain deadline |

**Exit criterion**: `go test -race ./...` green; `./musterd serve` boots and serves the UI;
quickstart smoke test passes.

## Test Coverage Targets

| Package | Target |
|---|---|
| `internal/core/` | ≥80% |
| `internal/store/` | ≥80% |
| `internal/services/` | ≥80% |
| `internal/ws/` | ≥75% |
| `internal/api/render/`, `internal/api/middleware/` | ≥90% |
| `internal/api/beads/`, `stream/`, `health/` | ≥70% |
| `cmd/musterd/` | smoke-tested only (boot test) |

Enforced as a **CI gate** (per spec round-3 clarifications): `go test -coverprofile=cover.out 
./...` then a small `make cover-check` script that reads `go tool cover -func=cover.out` and 
fails the build if any package is below its threshold.

## Lint & Format Gates

Per spec round-3 clarifications, the lint stack is **`gofmt + go vet + golangci-lint` with a
pinned curated config**. CI fails if any of the following produce output / non-zero exit:

```bash
gofmt -l .                  # MUST print nothing
go vet ./...                # MUST exit 0
golangci-lint run           # MUST exit 0
```

`.golangci.yml` (committed at repo root):

```yaml
run:
  timeout: 3m
  go: "1.26"

linters:
  disable-all: true
  enable:
    - errcheck
    - govet
    - gofmt
    - gosimple
    - ineffassign
    - staticcheck
    - unused

issues:
  exclude-rules:
    - path: _test\.go
      linters: [errcheck]   # tests intentionally ignore some returns
```

## Operational defaults (spec round-3 closure)

| Concern | Implementation |
|---|---|
| Graceful shutdown | `signal.NotifyContext(ctx, SIGINT, SIGTERM)`; on signal, call `srv.Shutdown(ctx)` with 5 s deadline. Hub closes all client `send` channels → clients receive WS close 1001. Exit 0 on clean drain, 1 on timeout. |
| `--addr` parsing | `net.SplitHostPort(addr)`. IPv6 supported. Bind failure logs `bind: address already in use` and exits 1. |
| Title length | 255 **runes** via `utf8.RuneCountInString`. |
| Validation order | body-size → structural (JSON, unknown field, null) → required → enum → numeric range. First fail short-circuits. |
| Request body cap | `http.MaxBytesReader(w, r.Body, 1<<20)` per POST/PATCH handler. |
| WS frame read limit | `wsConn.SetReadLimit(1 << 20)` after `Accept`. |

Tests covering these defaults are added to the relevant phases below (Phase 6, Phase 8, 
Phase 10).

## Complexity Tracking

No constitution violations. The added structure (resource subpackages, services layer) is
intentional — it isolates HTTP from business rules, which makes unit-testing services trivial
(`store` is a thin interface; no HTTP context needed) and keeps handlers small enough to be
review-by-eye.
