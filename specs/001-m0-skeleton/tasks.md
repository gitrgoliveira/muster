# Tasks: M0 — Skeleton (musterd binary + in-memory API + WS)

**Input**: Design documents from `specs/001-m0-skeleton/`

**Prerequisites**: plan.md ✓, spec.md ✓, data-model.md ✓, contracts/rest-api.md ✓, contracts/ws-events.md ✓, research.md ✓

**TDD approach**: Within each phase, write failing tests first, then implement to pass. Run `go test -race ./...` before marking a phase done.

**Go module**: `github.com/gitrgoliveira/muster`  
**Binary**: `musterd` at `cmd/musterd/`  
**Dependencies**: go-chi/chi/v5@v5.2.1, coder/websocket@v1.8.13, google/uuid@v1.6.0, stretchr/testify@v1.10.0

## Format: `[ID] [P?] [Story?] Description`

- **[P]**: Can run in parallel (different files, no dependencies on incomplete tasks in same phase)
- **[Story]**: Which user story this task belongs to (US1–US10 maps to spec.md user stories)

---

## Phase 1: Scaffolding

**Purpose**: Project initialization — go.mod, module layout, embed skeleton, Makefile. No tests in this phase.

<!-- sequential -->
- [ ] T001 Initialize `go.mod` with module `github.com/gitrgoliveira/muster`, Go 1.26, add dependencies: `github.com/go-chi/chi/v5@v5.2.1`, `github.com/coder/websocket@v1.8.13`, `github.com/google/uuid@v1.6.0`, `github.com/stretchr/testify@v1.10.0`; run `go mod tidy` to generate `go.sum`; create all empty package directories: `cmd/musterd/`, `internal/core/`, `internal/store/`, `internal/services/`, `internal/ws/`, `internal/api/render/`, `internal/api/middleware/`, `internal/api/beads/`, `internal/api/stream/`, `internal/api/health/`, `ui/`

<!-- parallel-group: 1 (max 3 concurrent) -->
- [ ] T002 [P] Create `ui/.gitkeep` (empty file so `//go:embed ui/*` compiles on fresh clone); create placeholder `ui/index.html` with `<!DOCTYPE html><html><body>placeholder</body></html>` so the embed is non-empty at compile time (real UI copied in Phase 10)
- [ ] T003 [P] Create `cmd/musterd/embed.go` with `//go:embed ui/*` directive and exported `var UI embed.FS`; create `cmd/musterd/main.go` as bare skeleton: declare `--addr` flag defaulting to `127.0.0.1:7766`, print banner `musterd listening on http://127.0.0.1:7766 (build=dev schemaVersion=1)` to stdout, then `os.Exit(0)`; must compile with `go build ./cmd/musterd/`
- [ ] T004 [P] Create `Makefile` at repo root with targets: `build` (`go build -o bin/musterd ./cmd/musterd/`), `test` (`go test -race ./...`), `run` (`go run ./cmd/musterd/ serve --addr 127.0.0.1:7766`), `ui-copy` (`cp -r prototype/ ui/`), `cover` (`go test -coverprofile=cover.out ./... && go tool cover -func=cover.out`), `cover-check` (implement as inline awk in Makefile: run `go tool cover -func=cover.out`, pipe to awk that checks each package total against a hardcoded threshold map; the awk script should: for each line matching `total:`, extract package path and statement percentage; compare against thresholds: `internal/core` ≥80, `internal/store` ≥80, `internal/services` ≥80, `internal/ws` ≥75, `internal/api/render` ≥90, `internal/api/middleware` ≥90, `internal/api/beads` ≥70, `internal/api/stream` ≥70, `internal/api/health` ≥70; exit 1 if any package is below threshold, printing which package failed), `lint` (`gofmt -l . && go vet ./... && golangci-lint run`)

<!-- sequential -->
- [ ] T005 Create `.golangci.yml` at repo root with config: `run.timeout: 3m`, `run.go: "1.26"`, `linters.disable-all: true`, `linters.enable: [errcheck, govet, gofmt, gosimple, ineffassign, staticcheck, unused]`, `issues.exclude-rules: [{path: _test\.go, linters: [errcheck]}]` (tests intentionally ignore some returns)

**Checkpoint**: `go build ./cmd/musterd/` succeeds; binary prints banner and exits.

---

## Phase 2: Core Types (`internal/core/`)

**Purpose**: Pure domain types — enums, ID generation, derived-field helpers. No I/O, no deps beyond `uuid`. All other layers import this package.

**Exit criterion**: `go test -race ./internal/core/...` passes; ≥80% statement coverage.

<!-- parallel-group: 1 (max 3 concurrent) -->
- [ ] T006 [P] TDD: Write `internal/core/enums_test.go` with table-driven tests: `TestBeadType_Valid`, `TestColumn_Valid`, `TestPriority_Valid`, `TestStepStatus_Valid`, `TestMode_Valid`, `TestAgentID_Valid`, `TestVCS_Valid`, `TestNowPlayingKind_Valid`, `TestLogKind_Valid`, `TestFileStatus_Valid` — each test verifies every defined constant returns `Valid()==true`, an unknown value returns `Valid()==false`; also add `TestVCS_EmptyStringIsValid` (empty VCS string is valid per data-model.md §1); then write `internal/core/enums.go` implementing all types **exactly as specified in `data-model.md §1`** (copy the Go source verbatim): `BeadType string` (TypeFeature/TypeBug/TypeTask/TypeEpic/TypeChore — 5 values), `Column string` (5 columns), `Priority int` (0–4 range; `Valid() bool { return p >= 0 && p <= 4 }`), `StepStatus string` (`pending|active|done|failed`), `AgentID string` (`claude|gemini|opencode|codex`), `Mode string` (`plan|build|review|agent|apply|yolo`), `VCS string` (`git|jj`; empty string also valid), `EventKind string` (full 15-value set: `opened|scheduled|claimed|started|paused|split|review|comment|approved|closed|reopened|requeued|blocked|unblocked|failed|discovered`), `Estimate string` (`XS|S|M|L` — 4 values, **no XL**), `NowPlayingKind string` (`tool|thought|output`), `LogKind string` (`system|tool|thought|output`), `FileStatus string` (`A|M|D`)
- [ ] T007 [P] TDD: Write `internal/core/id_test.go` with: `TestNewBeadID_Format` (1000 generated IDs all match regex `^bd-[0-9a-f]{4}$`), `TestNewBeadID_Uniqueness` (10k generations have collision rate <0.5%); then write `internal/core/id.go` implementing `NewBeadID() string` — generates UUIDv4 via `github.com/google/uuid`, takes the first 4 hex chars (positions 0-3 of the UUID's hex without dashes), prepends `bd-`, returns the ID string; **no retry or existsFn logic** — retry and collision detection belong in `memstore.Create` per `data-model.md §2.6`
- [ ] T008 [P] TDD: Write `internal/core/validate_test.go` with: `TestDeriveEstimate` (table: 0→`XS`, 89999→`XS`, 90000→`S`, 179999→`S`, 180000→`M`, 349999→`M`, 350000→`L`), `TestDeriveAssignee` (step with `Status=="active"` wins; fallback to last step's agent; empty string if no steps), `TestDeriveCommentCount` (count history events with `kind=="comment"`; add reviewer.Comments if reviewer non-nil; nil reviewer is safe — no panic); then write `internal/core/validate.go` implementing `DeriveEstimate(tokensBudget int) Estimate` (thresholds per `data-model.md §2.7`: <90000→XS, 90000–179999→S, 180000–349999→M, ≥350000→L; **no XL**), `DeriveAssignee(steps []Step) AgentID`, `DeriveCommentCount(history []HistoryEvent, reviewer *Reviewer) int` (pointer, nil-safe)

<!-- sequential -->
- [ ] T009 Write the data carrier structs (no tests — covered transitively): `internal/core/bead.go` (full Bead struct with all fields from data-model.md §2.1, `json:` tags matching field names exactly), `internal/core/step.go` (Step, SubBead — copy from **data-model.md §2.2** verbatim), `internal/core/history.go` (HistoryEvent — copy from **data-model.md §2.3** verbatim), `internal/core/gate.go` (Gate, GateKind, GateStatus — copy from **data-model.md §2.4a** verbatim), `internal/core/auxiliary.go` (Acceptance, NowPlaying, Reviewer, LogEntry, FileChange — implement **exactly as in data-model.md §2.5**, copy Go source verbatim; do NOT infer field names or types), `internal/core/reference.go` (Provider, Capacity, DoltStatus — implement **exactly as in data-model.md §2.4b**, copy Go source verbatim; do NOT infer field names or types)

**Checkpoint**: `go test -race ./internal/core/...` passes; ≥80% coverage.

---

## Phase 3: In-Memory Store (`internal/store/`)

**Purpose**: `Store` interface + `MemStore` implementation + seed data from prototype/data.jsx. Exit: `go test -race ./internal/store/...`; ≥80% coverage.

<!-- sequential -->
- [ ] T010 Write `internal/store/store.go` defining the `Store` interface with methods: `List(ctx context.Context, column string) ([]core.Bead, error)` (empty string = no filter), `Get(ctx context.Context, id string) (*core.Bead, error)`, `Create(ctx context.Context, b core.Bead) (*core.Bead, error)`, `Patch(ctx context.Context, id string, patch PatchBeadInput) (*core.Bead, error)` (**typed value object, not map** — avoids string-keyed switch and preserves type safety; `PatchBeadInput` is defined in this file, not in services/ to avoid circular import), `Move(ctx context.Context, id, toColumn, beforeID string) (*core.Bead, error)` (empty beforeID = append), `Dispatch(ctx context.Context, id string, req DispatchRequest) (*core.Bead, error)`, `AddComment(ctx context.Context, id string, req CommentRequest) (*core.Bead, error)`; define value types in this file: `PatchBeadInput{Title *string, Desc *string, Type *core.BeadType, Column *core.Column, Priority *core.Priority, Ready *bool, Labels *[]string, TokensBudget *int}` (mirrors services.PatchBeadInput; services.Patch translates from its own input type to this), `DispatchRequest{Agent core.AgentID, Mode core.Mode}` (**typed**, matching API layer), `CommentRequest{Actor, Note string}`; define error sentinels: `ErrNotFound`, `ErrInvalidState`, `ErrIDExhausted`, `ErrBeforeIDNotFound`, `ErrBeforeIDDifferentColumn`, `ErrBeforeIDSameAsMoved`; also define `NewMemStore(seeds []core.Bead) *MemStore` constructor signature here as a comment (implemented in memstore.go)

<!-- parallel-group: 1 (max 2 concurrent) -->
- [ ] T011 [P] TDD: Write `internal/store/seed_test.go` with: `TestSeedBeads_Count` (len==14), `TestSeedProviders_Count` (len==4), `TestSeedCapacity_Count` (len==4), `TestSeedDolt_NonZero` (Commit non-empty, Branch non-empty, Tables>0), `TestSeedBeads_HaveValidEnums` (every bead's Column/Type/Priority all pass Valid()), `TestSeedBeads_DerivedFields` (estimate equals `DeriveEstimate(bead.TokensBudget)`, assignee equals `DeriveAssignee(bead.Steps)`, comments equals `DeriveCommentCount(bead.History, bead.Reviewer)`); then write `internal/store/seed.go` implementing `SeedBeads() []core.Bead` returning exactly the 14 beads from `prototype/data.jsx` TASKS array transcribed field-by-field (timestamps kept as prototype calendar strings e.g. `"Mon 09:14"` — opaque), `SeedProviders() []core.Provider` (4 from AGENTS array), `SeedCapacity() []core.Capacity` (4 entries), `SeedDolt() core.DoltStatus`
- [ ] T012 [P] TDD: Write `internal/store/memstore_test.go` covering: `TestList_NoFilter` (14 items), `TestList_FilterByColumn` (only matching column), `TestList_PreservesInsertionOrder`, `TestGet_Found`, `TestGet_Missing` (ErrNotFound), `TestCreate` (ID generated via core.NewBeadID, fields stored), `TestCreate_IDCollisionRetry` (give MemStore a `generateID func() string` field defaulting to `core.NewBeadID`; in test inject a func that returns the same ID twice then a unique one — succeeds on 3rd attempt), `TestCreate_IDExhausted` (inject a func that always returns the same already-existing ID → ErrIDExhausted after 3 attempts), `TestPatch_PartialFields` (only supplied fields updated — pass `PatchBeadInput{Title: ptr("new")}`, verify only Title changed), `TestPatch_NotFound`, `TestMove_AppendsToColumn` (appends at end when beforeID empty), `TestMove_BeforeID_Reorders` (splice-out from source position, splice-in before target), `TestMove_BeforeID_UnknownReturnsError` (ErrBeforeIDNotFound), `TestMove_BeforeID_DifferentColumnReturnsError` (ErrBeforeIDDifferentColumn), `TestMove_BeforeID_SameAsMovedReturnsError` (ErrBeforeIDSameAsMoved), `TestDispatch_AppendsStep_ChangesColumn_AppendsTwoHistoryEvents` (assert: column changed to running; `len(bead.Steps)==N+1`; new step has `Agent==req.Agent, Mode==req.Mode, Skills==[], Status==core.StepActive`; history gains `EvClaimed` event with `actor:"dispatcher", agent:<agent>` AND `EvStarted` event with `actor:<agent>` both with RFC3339 At), `TestDispatch_FromNonScheduledReturnsErrInvalidState`, `TestAddComment_AppendsHistoryAndIncrementsCount`; then write `internal/store/memstore.go` implementing: `NewMemStore(seeds []core.Bead) *MemStore` constructor (seeds the slice), all Store methods with `sync.RWMutex` (RLock for reads, Lock for writes), beads stored as slice preserving insertion order, Move using slice splice-out-then-splice-in for beforeID reordering, Dispatch gated on `bead.Column == core.ColScheduled`, Patch iterating over non-nil fields of `store.PatchBeadInput`

<!-- sequential -->
- [ ] T013 Write `internal/store/memstore_concurrency_test.go` with `TestStore_ConcurrentReadsWrites`: spawn 20 goroutines, each executing 50 iterations of randomly interleaved List + Patch + Move calls against the same MemStore; run with `-race` flag; this test requires T012 to be complete first

**Checkpoint**: `go test -race ./internal/store/...` passes; ≥80% coverage.

---

## Phase 4: Business Logic (`internal/services/`)

**Purpose**: Validation + defaults + store mutations + event publishing. Only this layer mutates store state. Exit: `go test -race ./internal/services/...`; ≥80% coverage.

<!-- sequential -->
- [ ] T014 Write `internal/services/events.go` defining `Publisher` as `type Publisher func(frame interface{})` (or a minimal interface with `Publish(v interface{})`) to decouple services from `ws` package — services call this to broadcast events; write `internal/services/defaults.go` with `applyCreateDefaults(b *core.Bead)` setting: Column=`core.ColBacklog` (if zero), Type=`core.TypeTask` (if zero), Priority=`2` (int, not "P2" — Priority is `type Priority int`), VCS=`core.VCSGit` (if zero), Repo=`"main"` (if empty), TokensUsed=0, now=`time.Now().UTC().Format(time.RFC3339)`, CreatedAt=now, OpenedAt=now, LastActivity=now, Labels=`[]string{}` (not nil), Skills=`[]string{}`, Steps=`[]core.Step{}`, SubBeads=`[]core.SubBead{}`, History=`[]core.HistoryEvent{{Kind: core.EvOpened, Actor: "user", At: now}}` (**one initial event, not empty slice** per data-model.md §6), Acceptance=`[]core.Acceptance{}` (not nil), Log=`[]core.LogEntry{}`, Files=`[]core.FileChange{}`, Blocks=`[]string{}`, BlockedBy=`[]string{}`, Comments=0; write `internal/services/beads.go` skeleton declaring `BeadService` struct with `store store.Store` and `publish Publisher` fields and empty method signatures for Create, Patch, Move, Dispatch, AddComment

<!-- sequential -->
- [ ] T015 TDD: Write `internal/services/beads_test.go` using a `mockStore` struct implementing `store.Store`; table-driven tests: `TestCreate_MissingTitle` (→ error code INVALID_REQUEST), `TestCreate_TitleTooLong` (>255 runes → INVALID_REQUEST), `TestCreate_InvalidType` (unknown type string → INVALID_REQUEST), `TestCreate_InvalidPriority` (unknown priority → INVALID_REQUEST), `TestCreate_WhitespaceTitleTrimmed` (leading/trailing whitespace stripped), `TestPatch_EmptyBodyReturnsInvalidRequest` (empty map → error), `TestPatch_AllPointerFields` (title, desc, type, priority, labels, tokensBudget each exercised individually), `TestMove_ColumnChange`, `TestMove_BeforeID_Reorder`, `TestDispatch_Scheduled_Succeeds`, `TestDispatch_NonScheduled_ReturnsInvalidState` (INVALID_STATE code), `TestDispatch_AppendsStepAndTwoHistoryEvents` (history has EventKindClaimed + EventKindStarted), `TestAddComment_AppendsAndCounts`; then implement all BeadService methods in `internal/services/beads.go`: validation → call store method → **call publish** with correct Frame type (publish calls go here, not in a later task — T016 only tests them)
- [ ] T016 TDD: Write `internal/services/events_test.go` — these tests verify the publish calls T015 already implemented; do NOT modify `beads.go` here (read-only verification phase); assert Publisher is called with correct event type on each mutating operation: `TestService_PublishesCreatedEvent` (type==`bead.created`, bead field set), `TestService_PublishesUpdatedEvent_OnPatch` (type==`bead.updated`), `TestService_PublishesUpdatedEvent_OnDispatch` (type==`bead.updated`), `TestService_PublishesMovedEvent` (type==`bead.moved`, fromColumn/toColumn/beforeID set), `TestService_PublishesCommentAddedEvent` (type==`comment.added`, event+bead fields set per `contracts/ws-events.md`); if any test fails, fix the publish call in `beads.go` (add/correct only the failing publish call — no other changes)

**Checkpoint**: `go test -race ./internal/services/...` passes; ≥80% coverage.

---

## Phase 5: WebSocket Hub (`internal/ws/`)

**Purpose**: Frame types, Hub broadcast loop, Client read/write pumps, hello + ping/pong protocol. Exit: `go test -race ./internal/ws/...`; ≥75% coverage.

<!-- sequential -->
- [ ] T017 Write `internal/ws/event.go` defining: `EventType string` with constants `EventHello="hello"`, `EventBeadCreated="bead.created"`, `EventBeadUpdated="bead.updated"`, `EventBeadMoved="bead.moved"`, `EventBeadDeleted="bead.deleted"`, `EventCommentAdded="comment.added"`, `EventPong="pong"`; `Frame` union struct with all fields optional (`json:",omitempty"`): `Type EventType`, `Build string`, `SchemaVersion int`, `BeadsVersion string`, `ServerTime string` (hello fields), `Bead *core.Bead` (bead events), `ID string`, `FromColumn core.Column`, `ToColumn core.Column`, `BeforeID string` (bead.moved), `Event *core.HistoryEvent` (comment.added), `At string` (pong); `ClientFrame` struct `{Type string}` for parsing incoming ping frames; write `internal/ws/hub.go` defining `Hub` struct with `register chan *Client`, `unregister chan *Client`, `broadcast chan Frame`, `clients map[*Client]bool`; `NewHub() *Hub` constructor; `Run() loop` that on register: sends hello Frame (build="dev", schemaVersion=1, serverTime=RFC3339 now, beadsVersion=injected string — **not hardcoded**; `NewHub` must accept `beadsVersion string` param so main.go passes the seed value "0.9.1") within 1s; on unregister: closes client.send and deletes from map; on broadcast: sends to each client's send channel, increments drop counter if full (buffer 16), unregisters client after 3 drops within 10s; `Broadcast(f Frame)` method that sends to hub.broadcast channel

<!-- parallel-group: 1 (max 2 concurrent) -->
- [ ] T018 [P] TDD: Write `internal/ws/hub_test.go` using `httptest.NewServer` + `coder/websocket` Dial: `TestHub_RegisterUnregister` (connect, verify client in map; disconnect, verify removed), `TestHub_HelloSentWithinOneSecondOfRegister` (connect, set 1s deadline, read first message, verify type=="hello" with non-empty build/schemaVersion/serverTime/beadsVersion), `TestHub_BroadcastReachesClient` (call hub.Broadcast with bead.created frame, connected client receives it within 100ms), `TestHub_SlowClientDropped` (fill client's send channel buffer, call Broadcast 3+ times within 10s, verify client is eventually unregistered); implement all Hub behavior to make tests pass
- [ ] T019 [P] TDD: Write `internal/ws/client_test.go`: `TestClient_PingFrameProducesPong` (send `{"type":"ping"}`, receive `{"type":"pong","at":"..."}` within 500ms), `TestClient_WritePumpExitsOnContextCancel` (cancel context, writePump goroutine exits), `TestClient_ReadPumpExitsOnClose` (close WS conn server-side, readPump goroutine exits), `TestClient_UnknownClientFrameLoggedAndIgnored` (send `{"type":"unknown"}`, no crash, connection stays open); then write `internal/ws/client.go` with `Client` struct holding `hub *Hub`, `conn *websocket.Conn`, `send chan Frame`; `writePump(ctx)` reads from send channel and writes JSON to conn; `readPump(ctx)` calls `conn.SetReadLimit(1<<20)`, reads frames, handles type=="ping" by responding with pong, logs unknown types and continues; `ServeWS(hub *Hub, w http.ResponseWriter, r *http.Request)` function that accepts upgrade, creates Client, starts pumps in goroutines, registers with hub

**Checkpoint**: `go test -race ./internal/ws/...` passes; ≥75% coverage.

---

## Phase 6: HTTP Utilities (`internal/api/render/` + `internal/api/middleware/`)

**Purpose**: JSON helpers, error code constants, X-Request-ID middleware, body-size cap middleware. Exit: ≥90% coverage.

<!-- parallel-group: 1 (max 3 concurrent) -->
- [ ] T020 [P] TDD: Write `internal/api/render/render_test.go` with: `TestWriteJSON_SetsHeadersAndStatus` (Content-Type: application/json, correct status, body is valid JSON), `TestWriteError_AllCodes` (for each code: BEAD_NOT_FOUND→404, NOT_FOUND→404, INVALID_REQUEST→400, INVALID_STATE→400, METHOD_NOT_ALLOWED→405, INTERNAL→500 — verify HTTP status and JSON body shape `{"error":{"code":"...","message":"...","requestID":"..."}}`), `TestWriteError_IncludesRequestID` (requestID value comes from request context via middleware key); then write `internal/api/render/json.go` with `WriteJSON(w http.ResponseWriter, status int, v interface{})` and `internal/api/render/errors.go` with `type ErrorResponse struct{Code,Message,RequestID string}` wrapped in `{"error":{}}`, string constants `CodeBeadNotFound="BEAD_NOT_FOUND"`, `CodeNotFound="NOT_FOUND"`, `CodeInvalidRequest="INVALID_REQUEST"`, `CodeInvalidState="INVALID_STATE"`, `CodeMethodNotAllowed="METHOD_NOT_ALLOWED"`, `CodeInternal="INTERNAL"`, `WriteError(w http.ResponseWriter, r *http.Request, httpStatus int, code, message string)` helper
- [ ] T021 [P] TDD: Write `internal/api/middleware/requestid_test.go` with: `TestRequestID_EchoesSupplied` (X-Request-ID on request echoed verbatim in response header), `TestRequestID_GeneratesWhenAbsent` (response has X-Request-ID matching UUID format), `TestRequestID_AvailableInContext` (downstream handler can retrieve via typed context key), `TestRequestID_OnErrorResponses` (X-Request-ID present even in 404/500 responses — FR-017 acceptance); then write `internal/api/middleware/requestid.go` as a chi middleware: read `X-Request-ID` from request (generate `uuid.NewString()` if absent), store in context with typed key `type contextKey string; const reqIDKey contextKey = "requestID"`, set on response writer header before serving next handler; export `GetRequestID(ctx context.Context) string` helper
- [ ] T022 [P] TDD: Write `internal/api/middleware/bodylimit_test.go` with: `TestBodyLimit_RejectsOversize_400_InvalidRequest` (POST body of 1 MiB+1 byte → 400 INVALID_REQUEST, body contains message "request body exceeds 1 MiB limit"), `TestBodyLimit_AcceptsBelowLimit` (1 MiB-1 byte body → handler receives it and returns 200), `TestBodyLimit_MessageContainsLimit` (error message mentions "1 MiB"); then write `internal/api/middleware/bodylimit.go` as chi middleware that wraps `r.Body` with `http.MaxBytesReader(w, r.Body, 1<<20)`; when the handler reads the body and triggers the limit, intercept `*http.MaxBytesError` (or detect `io.EOF` after limit) and call `render.WriteError` with 400 + CodeInvalidRequest + "request body exceeds 1 MiB limit"; apply only to POST/PATCH methods

**Checkpoint**: `go test -race ./internal/api/render/... ./internal/api/middleware/...` passes; ≥90% coverage.

---

## Phase 7: Health Endpoints (`internal/api/health/`) [US10]

**User Story**: US10 — Orchestrator status & healthz

**Goal**: `GET /api/v1/healthz` → 200 `{"ok":true}`; `GET /api/v1/orchestrator/status` → full 6-field payload.

**Independent Test**: `curl http://localhost:7766/api/v1/healthz | jq '.ok'` = `true`; `curl .../orchestrator/status | jq '.build'` = `"dev"`.

<!-- sequential -->
- [ ] T023 [US10] Write `internal/api/health/dto.go` with `HealthzResponse{Ok bool \`json:"ok"\`}` and `OrchestratorStatusResponse{Build string, SchemaVersion int, BeadsVersion string, Online bool, ServerTime string, Dolt core.DoltStatus}` — field names and json tags matching contracts/rest-api.md exactly

- [ ] T024 [US10] TDD: Write `internal/api/health/handler_test.go` using `httptest.NewServer`: `TestHealthz_Returns200_AndOK` (GET /healthz → 200, `{"ok":true}`), `TestOrchestratorStatus_ReturnsFullPayload` (all 6 top-level fields present and non-zero: build, schemaVersion, beadsVersion, online, serverTime, dolt), `TestOrchestratorStatus_BeadsVersionMatchesSeed` (beadsVersion == "0.9.1" — the value from `prototype/data.jsx` repos[0].detected.beadsVersion, passed in via constructor, **not hardcoded**), `TestOrchestratorStatus_DoltMatchesSeed` (every dolt sub-field equals the value from `store.SeedDolt()`), `TestOrchestratorStatus_ServerTimeIsRFC3339` (serverTime parses as RFC3339); then write `internal/api/health/handler.go` with `HealthzHandler(w, r)` returning 200 + HealthzResponse{Ok:true}, and `OrchestratorStatusHandler(beadsVersion string, seedDolt core.DoltStatus) http.HandlerFunc` closure returning 200 + OrchestratorStatusResponse{Build:"dev", SchemaVersion:1, BeadsVersion:beadsVersion, Online:true, ServerTime:RFC3339 now, Dolt:seedDolt}

**Checkpoint**: `go test -race ./internal/api/health/...` passes; ≥70% coverage.

---

## Phase 8: Beads CRUD Handlers (`internal/api/beads/`) [US2–US6, US8, US9]

**Purpose**: All 7 bead handlers + DTOs. Handlers share `handlers.go` so tasks are sequential. Each step: write tests first (failing), then implement handler.

**Exit criterion**: ≥70% coverage; every error-matrix row in `contracts/rest-api.md` has a corresponding test.

<!-- sequential -->
- [ ] T025 Write `internal/api/beads/dto.go` with all request/response types — implement **exactly as in data-model.md §3.1** (copy Go source verbatim for each struct); key points: `ListResponse{Items []core.Bead, NextCursor *string, Total int}`, `CreateRequest{Title string, Desc string, Type core.BeadType, Column core.Column, Priority core.Priority, Labels []string, VCS core.VCS, TokensBudget int}` (Title is the only required field), `PatchRequest` with typed pointer fields: `Title *string, Desc *string, Type *core.BeadType, Column *core.Column, Priority *core.Priority, Ready *bool, Labels *[]string, TokensBudget *int` (all nullable-aware; **no** untyped `*string` for enum fields), `MoveRequest{ToColumn core.Column, BeforeID string}` (**string, not *string** — absent means "" which means append), `DispatchRequest{Agent core.AgentID, Mode core.Mode}` (typed, not plain strings), `CommentRequest{Actor string, Note string}`; json tags must match field names in contracts/rest-api.md exactly

- [ ] T026 [US2] TDD: Add to `internal/api/beads/handlers_test.go`: `TestList_NoFilter` (GET /api/v1/beads → 200, body has items array len==14, nextCursor==null, total==14), `TestList_FilterByColumn` (GET /api/v1/beads?column=running → only running-column beads), `TestList_InvalidColumn_400` (GET /api/v1/beads?column=invalid → 400 INVALID_REQUEST); test setup: `store.NewMemStore(store.SeedBeads())` seeded with 14 beads, `services.NewBeadService(memStore, mockPublisher)`, wired into router via `httptest.NewServer` — **the test helper must seed the store or len==14 assertion will fail**; then write `List` handler in `internal/api/beads/handlers.go`

- [ ] T027 [US4] TDD: Add to `handlers_test.go`: `TestCreate_201_WithDefaults` (POST with `{"title":"Test"}` → 201, body has bd-XXXX id, column==backlog, type==task, priority==2; response has `Location: /api/v1/beads/{id}` header per contracts/rest-api.md), `TestCreate_LocationHeader` (Location header value matches the returned bead ID exactly), `TestCreate_400_MissingTitle` (empty title → 400 INVALID_REQUEST), `TestCreate_400_InvalidEnum` (bad type or priority value → 400), `TestCreate_400_UnknownField` (body contains unknown key → 400 INVALID_REQUEST with message `unknown field: foo` — per contracts/rest-api.md cross-cutting `DisallowUnknownFields` requirement), `TestCreate_400_OversizeBody` (body >1 MiB → 400 INVALID_REQUEST via body-limit middleware), `TestCreate_BroadcastsWSEvent` (mock publisher receives bead.created frame); then write `Create` handler — **must** use `json.NewDecoder` with `DisallowUnknownFields()` for body decoding, and **must** call `w.Header().Set("Location", "/api/v1/beads/"+bead.ID)` before writing 201 response

- [ ] T028 [US3] TDD: Add to `handlers_test.go`: `TestGet_200` (GET /api/v1/beads/{id} → 200, full bead body with all fields), `TestGet_404` (nonexistent ID → 404 BEAD_NOT_FOUND), `TestGet_400_BadIDFormat` (malformed id pattern → 400 INVALID_REQUEST); then write `Get` handler

- [ ] T029 [US5] TDD: Add to `handlers_test.go`: `TestPatch_PartialUpdate` (PATCH `{"title":"X"}` → 200, only title changed), `TestPatch_404` (nonexistent ID → 404), `TestPatch_400_EmptyBody` (empty `{}` → 400 INVALID_REQUEST), `TestPatch_400_NullField` (explicit null where forbidden → 400), `TestPatch_400_UnknownField` (unknown key in body → 400), `TestPatch_BroadcastsWSEvent` (bead.updated event emitted); then write `Patch` handler

- [ ] T030 [US6] TDD: Add to `handlers_test.go`: `TestMove_200_ToColumn` (POST move → 200, bead.column changed, appended at end of target column), `TestMove_200_WithBeforeID_Reorders` (beforeID supplied → bead inserted immediately before target), `TestMove_400_MissingToColumn` (no toColumn → 400), `TestMove_400_UnknownColumn` (invalid column value → 400), `TestMove_400_UnknownBeforeID` (beforeID not found → 400 with message), `TestMove_400_BeforeIDDifferentColumn` (beforeID in different column than toColumn → 400), `TestMove_400_BeforeIDEqualsMovedBead` (beforeID == moved bead's own ID → 400), `TestMove_404` (bead not found), `TestMove_EmitsBeadMovedEvent` (WS payload includes fromColumn, toColumn, beforeID); then write `Move` handler

- [ ] T031 [US8] TDD: Add to `handlers_test.go`: `TestDispatch_200_FromScheduled` (POST dispatch on scheduled bead → 200, column==running, history includes claimed+started events with RFC3339 At), `TestDispatch_400_InvalidState_FromBacklog` (backlog bead → 400 INVALID_STATE), `TestDispatch_400_InvalidState_FromRunning` (already-running bead → 400 INVALID_STATE), `TestDispatch_400_InvalidAgent` (unknown agent value → 400 INVALID_REQUEST), `TestDispatch_400_InvalidMode` (unknown mode value → 400 INVALID_REQUEST), `TestDispatch_404` (nonexistent bead); then write `Dispatch` handler

- [ ] T032 [US9] TDD: Add to `handlers_test.go`: `TestComment_201_AppendsHistory` (POST comment → 201, history contains new comment event), `TestComment_IncrementsCount` (comments field on bead incremented by 1), `TestComment_EmitsCommentAddedEvent` (WS payload has {id, event, bead} per contracts/ws-events.md), `TestComment_400_MissingActor` (empty actor → 400), `TestComment_400_MissingNote` (empty note → 400), `TestComment_404` (nonexistent bead → 404); then write `Comment` handler (returns 201 per spec US9)

**Checkpoint**: `go test -race ./internal/api/beads/...` passes; ≥70% coverage; every error-matrix row in contracts/rest-api.md has a test.

---

## Phase 9: WebSocket Stream Handler (`internal/api/stream/`) [US7]

**User Story**: US7 — WebSocket event stream

**Goal**: Upgrade GET /api/v1/stream to WS, register with Hub, receive hello within 1s, relay all bead mutation events.

**Independent Test**: Connect WS client, POST /api/v1/beads, assert WS client receives `bead.created` event.

<!-- sequential -->
- [ ] T033 [US7] TDD: Write `internal/api/stream/handler_test.go` using `httptest.NewServer` with full router (store + services + hub): `TestStream_UpgradesToWS` (GET /api/v1/stream with Upgrade: websocket → 101), `TestStream_SendsHelloWithinOneSecond` (first WS message has type=="hello" with build+schemaVersion+serverTime+beadsVersion per FR-013), `TestStream_PingProducesPong` (send `{"type":"ping"}`, receive `{"type":"pong","at":"..."}` within 500ms per FR-014), `TestStream_ReceivesBeadCreatedOnPostBeads` (POST /api/v1/beads → WS receives bead.created frame), `TestStream_ReceivesBeadMovedOnPostMove` (POST .../move → bead.moved frame), `TestStream_ReceivesBeadUpdatedOnPatch` (PATCH → bead.updated frame), `TestStream_ReceivesCommentAddedOnPostComment` (POST .../comments → comment.added frame with event+bead fields), `TestStream_DisconnectUnregistersClient` (close WS client → hub no longer holds reference); then write `internal/api/stream/handler.go` with `StreamHandler(hub *ws.Hub) http.HandlerFunc` that calls `ws.ServeWS(hub, w, r)`

**Checkpoint**: `go test -race ./internal/api/stream/...` passes; ≥70% coverage.

---

## Phase 10: Router + Main (Wiring) [US1]

**User Story**: US1 — Start musterd and view the UI

**Goal**: Assemble all components into a running server; embed real prototype UI; graceful shutdown.

**Independent Test**: `./bin/musterd` starts, `curl http://localhost:7766/` returns 200 with HTML, `curl http://localhost:7766/api/v1/beads` returns 14 beads.

<!-- sequential -->
- [ ] T034 [US1] TDD: Write `internal/api/router_test.go` with: `TestRouter_StaticUIServed` (GET / → 200 with HTML content), `TestRouter_APINotFound_ReturnsJSON` (GET /api/v1/nonexistent → 404 with JSON body `{"error":{"code":"NOT_FOUND",...}}`), `TestRouter_MethodNotAllowed_ReturnsJSON` (DELETE /api/v1/beads → 405 with JSON body `{"error":{"code":"METHOD_NOT_ALLOWED",...}}`), `TestRouter_PanicRecovered_Returns500JSON` (inject handler that panics → 500 with JSON body `{"error":{"code":"INTERNAL",...}}`); then write `internal/api/router.go` with `NewRouter(svc *services.BeadService, hub *ws.Hub, uiFS fs.FS) http.Handler` that creates a chi router, installs X-Request-ID middleware globally, installs body-limit middleware on /api/v1/ POST+PATCH routes, mounts beads sub-router at /api/v1/beads, mounts stream handler at /api/v1/stream, mounts health handlers at /api/v1/healthz and /api/v1/orchestrator/status, serves `uiFS` at / (with fallback to index.html for SPA), sets chi.NotFound/MethodNotAllowed handlers that call render.WriteError

- [ ] T035 [US1] TDD: Write `cmd/musterd/main_test.go` with (using `exec.Command` or in-process test server): `TestServer_BootsAndServesUI` (run `musterd serve`, GET /, confirm 200 with HTML), `TestServer_GracefulShutdown_DrainsWithin5s_ExitCode0` (send os.Interrupt, verify server stops cleanly within 5s), `TestServer_ParseAddr_IPv6` (`serve --addr [::1]:7777` → binds successfully), `TestServer_ParseAddr_InvalidFormat_Exits1` (`serve --addr notvalid` → process exits 1), `TestServer_PortInUse_Exits1` (pre-bind port then `serve` → exits 1), `TestNoSubcommand_PrintsUsageExits1` (no args → prints usage, exits 1); then complete `cmd/musterd/main.go`: **require `serve` subcommand** (parse `os.Args[1]` — if missing or unknown, print usage and exit 1; `serve` registers `--addr` flag defaulting to `127.0.0.1:7766`), get seedDolt=`store.SeedDolt()`, seedBeadsVersion=seedDolt.BeadsVersion (or from repos[0] seed — "0.9.1"), create `ws.NewHub(seedBeadsVersion)` (beadsVersion injected per T017) + `go hub.Run()`, create `store.NewMemStore()` seeded with SeedBeads/SeedProviders/SeedCapacity/SeedDolt, create `services.NewBeadService(store, hub.Broadcast)` (**`hub.Broadcast` as function value**, not `hub.BroadcastFunc()`), call `NewRouter(svc, hub, uiFS)`, create `http.Server{Addr, Handler, ReadHeaderTimeout: 5*time.Second}`, install `signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)` with 5s drain deadline, print banner `musterd listening on http://ADDR (build=dev schemaVersion=1)` to stdout, start `srv.ListenAndServe()`, on signal call `srv.Shutdown(drainCtx)`, exit 0 on clean shutdown or 1 on timeout/bind failure

- [ ] T036 [US1] Copy prototype UI files and verify embed: run `make ui-copy` to copy `prototype/` → `ui/`; run `go build -o bin/musterd ./cmd/musterd/`; run quickstart smoke test: start `./bin/musterd &`, verify `curl -sf http://localhost:7766/ | grep -i html` matches, `curl -sf http://localhost:7766/api/v1/beads | jq '.total'` returns 14, `curl -sf http://localhost:7766/api/v1/healthz | jq '.ok'` returns true, connect WS to `ws://localhost:7766/api/v1/stream` and receive hello frame, kill server; fix any deviations from quickstart.md examples

**Checkpoint**: `go test -race ./...` fully green; binary boots and serves UI with seed data; all acceptance criteria in spec.md met.

---

## Phase 11: Polish & Cross-Cutting

**Purpose**: Coverage gates, lint pass, full integration gate before shipping M0.

<!-- parallel-group: 1 (max 3 concurrent) -->
- [ ] T037 [P] Run coverage gate: `make cover` then verify per-package statement coverage meets thresholds from plan.md §Test Coverage Targets: `internal/core/` ≥80%, `internal/store/` ≥80%, `internal/services/` ≥80%, `internal/ws/` ≥75%, `internal/api/render/` ≥90%, `internal/api/middleware/` ≥90%, `internal/api/beads/` ≥70%, `internal/api/stream/` ≥70%, `internal/api/health/` ≥70%; for any package below threshold add missing test cases until it passes; run `make cover-check` (should exit 0)
- [ ] T038 [P] Run lint gate: `gofmt -l .` (must print nothing — fix any formatting), `go vet ./...` (must exit 0 — fix all vet warnings), `golangci-lint run` (must exit 0 — fix all findings from the 7 enabled linters in .golangci.yml); all three must pass before this task is complete
- [ ] T039 [P] Run full integration verification per `quickstart.md`: execute every curl example in quickstart.md in sequence (list, get, create, patch, move, dispatch, comment, healthz, orchestrator/status); connect WS and verify all 7 event types are receivable; verify startup banner format matches spec exactly; verify X-Request-ID present on every response; document any deviations and fix them

**Checkpoint**: All gates pass. M0 implementation complete and shippable.

---

## Dependencies & Execution Order

### Phase Dependencies

| Phase | Depends On | Can Parallelize With |
|---|---|---|
| Phase 1 (Scaffolding) | — | — |
| Phase 2 (core/) | Phase 1 | — |
| Phase 3 (store/) | Phase 2 | — |
| Phase 4 (services/) | Phase 3 | — |
| Phase 5 (ws/) | Phase 2 | Phase 4 (different packages) |
| Phase 6 (render/middleware/) | Phase 1 | Phases 2–5 (different packages) |
| Phase 7 (health/) | Phases 3, 4, 6 | — |
| Phase 8 (beads/) | Phases 4, 5, 6 | Phase 7 (different package) |
| Phase 9 (stream/) | Phases 5, 6 | Phase 8 (different package) |
| Phase 10 (router+main) | Phases 7, 8, 9 | — |
| Phase 11 (polish) | Phase 10 | — |

### User Story → Phase Mapping

| User Story | Priority | Plan Phase(s) |
|---|---|---|
| US1 — Start musterd & view UI | P1 | Phase 10 (T034–T036) |
| US2 — List beads via REST | P1 | Phase 8 (T026) |
| US7 — WebSocket event stream | P1 | Phase 5 + Phase 9 (T017–T019, T033) |
| US3 — Get single bead | P2 | Phase 8 (T028) |
| US4 — Create bead | P2 | Phase 8 (T027) |
| US5 — Patch bead | P2 | Phase 8 (T029) |
| US6 — Move bead | P2 | Phase 8 (T030) |
| US10 — Health & status | P3 | Phase 7 (T023–T024) |
| US8 — Dispatch | P3 | Phase 8 (T031) |
| US9 — Comments | P3 | Phase 8 (T032) |

### Within-Phase Parallel Groups

| Phase | Parallel Group | Tasks | Constraint |
|---|---|---|---|
| Phase 1 | Group 1 | T002, T003, T004 | After T001 |
| Phase 2 | Group 1 | T006, T007, T008 | All independent files |
| Phase 3 | Group 1 | T011, T012 | After T010 |
| Phase 4 | Group 1 | T015, T016 | After T014 |
| Phase 5 | Group 1 | T018, T019 | After T017 |
| Phase 6 | Group 1 | T020, T021, T022 | All independent subpackages |
| Phase 11 | Group 1 | T037, T038, T039 | All independent |

---

## Implementation Strategy

### MVP First (P1 Stories: US1 + US2 + US7)

The three P1 stories deliver the full working product:

1. Phase 1: Scaffolding
2. Phase 2: Core types (foundation)
3. Phase 3: Store + seed data
4. Phase 4: Services (business logic)
5. Phase 5: WS Hub
6. Phase 6: HTTP utilities
7. Phase 8 T025–T026: DTO + List handler → **US2 done**
8. Phase 9: Stream handler → **US7 done**
9. Phase 10: Router + main wiring → **US1 done** (binary serves UI + beads + WS)
10. Remaining Phase 8 tasks: US3–US6, US8, US9

### Incremental Delivery

After Phase 10, all stories are complete. Phase 11 gates quality for shipping. Each phase builds on the previous with no circular dependencies.

---

## Notes

- `[P]` tasks have different files and no incomplete dependencies — safe to parallelize
- `[Story]` label maps to spec.md user story for traceability
- TDD within each phase: write tests first (verify they fail), then implement
- All test files use `httptest.NewServer` for HTTP integration; real MemStore + real services (no HTTP mocks)
- Run `go test -race ./...` before closing any phase — race detector is a hard gate
- Phase 8 tasks are sequential: handlers.go and handlers_test.go are shared files
- Timestamps in seed data are opaque strings (prototype format); new events from handlers use RFC3339
- Body size cap (1 MiB) is enforced by middleware in Phase 6, not by individual handlers
