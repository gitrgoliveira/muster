# Data Model: M0 — Skeleton

**Feature**: M0 — Skeleton
**Date**: 2026-05-22
**Source**: `prototype/data.jsx`, `handoff/spec.md §3`

This file defines (1) the **domain types** in `internal/core/` (pure, no I/O, no JSON-tag-specific
DTOs), (2) the **transport DTOs** in `internal/api/<resource>/dto.go` (request/response shapes),
and (3) the **WebSocket event envelope** in `internal/ws/`.

---

## 1. Typed Enums (`internal/core/enums.go`)

All string-valued fields with closed sets use named types to make invalid values impossible to
construct without going through a validator.

```go
package core

type BeadType string

const (
    TypeFeature BeadType = "feature"
    TypeBug     BeadType = "bug"
    TypeTask    BeadType = "task"
    TypeEpic    BeadType = "epic"
    TypeChore   BeadType = "chore"
)

func (t BeadType) Valid() bool {
    switch t {
    case TypeFeature, TypeBug, TypeTask, TypeEpic, TypeChore:
        return true
    }
    return false
}

type Column string

const (
    ColBacklog   Column = "backlog"
    ColScheduled Column = "scheduled"
    ColRunning   Column = "running"
    ColReview    Column = "review"
    ColDone      Column = "done"
)

func (c Column) Valid() bool {
    switch c {
    case ColBacklog, ColScheduled, ColRunning, ColReview, ColDone:
        return true
    }
    return false
}

type StepStatus string

const (
    StepPending StepStatus = "pending"
    StepActive  StepStatus = "active"
    StepDone    StepStatus = "done"
    StepFailed  StepStatus = "failed"
)

func (s StepStatus) Valid() bool {
    switch s {
    case StepPending, StepActive, StepDone, StepFailed:
        return true
    }
    return false
}

type AgentID string

const (
    AgentClaude   AgentID = "claude"
    AgentGemini   AgentID = "gemini"
    AgentOpenCode AgentID = "opencode"
    AgentCodex    AgentID = "codex"
)

func (a AgentID) Valid() bool {
    switch a {
    case AgentClaude, AgentGemini, AgentOpenCode, AgentCodex:
        return true
    }
    return false
}

type Mode string

const (
    ModePlan   Mode = "plan"
    ModeBuild  Mode = "build"
    ModeReview Mode = "review"
    ModeAgent  Mode = "agent"
    ModeApply  Mode = "apply"
    ModeYolo   Mode = "yolo"
)

func (m Mode) Valid() bool {
    switch m {
    case ModePlan, ModeBuild, ModeReview, ModeAgent, ModeApply, ModeYolo:
        return true
    }
    return false
}

type VCS string

const (
    VCSGit VCS = "git"
    VCSJJ  VCS = "jj"
)

func (v VCS) Valid() bool {
    return v == "" || v == VCSGit || v == VCSJJ
}

type Priority int // 0..4, 0 = critical, 4 = icebox

func (p Priority) Valid() bool { return p >= 0 && p <= 4 }

type Estimate string

const (
    EstXS Estimate = "XS"
    EstS  Estimate = "S"
    EstM  Estimate = "M"
    EstL  Estimate = "L"
)

type EventKind string

const (
    EvOpened     EventKind = "opened"
    EvScheduled  EventKind = "scheduled"
    EvClaimed    EventKind = "claimed"
    EvStarted    EventKind = "started"
    EvPaused     EventKind = "paused"
    EvSplit      EventKind = "split"
    EvReview     EventKind = "review"
    EvComment    EventKind = "comment"
    EvApproved   EventKind = "approved"
    EvClosed     EventKind = "closed"
    EvReopened   EventKind = "reopened"
    EvRequeued   EventKind = "requeued"
    EvBlocked    EventKind = "blocked"
    EvUnblocked  EventKind = "unblocked"
    EvFailed     EventKind = "failed"
    EvDiscovered EventKind = "discovered"
)
```

---

## 2. Domain Types (`internal/core/`)

Split across files to keep packages small. Each file's content below.

### 2.1 `internal/core/bead.go`

```go
package core

// Bead is the primary domain entity. JSON tags follow the prototype's data.jsx
// shape verbatim — short names (`desc`, `vcs`) are deliberate to keep the
// UI's wire format unchanged from the mock.
type Bead struct {
    ID           string         `json:"id"`
    Title        string         `json:"title"`
    Desc         string         `json:"desc"`
    Type         BeadType       `json:"type"`
    Column       Column         `json:"column"`
    Priority     Priority       `json:"priority"`
    Labels       []string       `json:"labels"`
    VCS          VCS            `json:"vcs"`
    Branch       string         `json:"branch,omitempty"`
    Worktree     string         `json:"worktree,omitempty"`
    Ready        bool           `json:"ready"`
    Repo         string         `json:"repo"`
    Formula      string         `json:"formula,omitempty"`
    Skills       []string       `json:"skills"`
    Steps        []Step         `json:"steps"`
    SubBeads     []SubBead      `json:"subBeads"`
    Gates        []Gate         `json:"gates,omitempty"`
    History      []HistoryEvent `json:"history"`
    Acceptance   []Acceptance   `json:"acceptance"`
    TokensUsed   int            `json:"tokensUsed"`
    TokensBudget int            `json:"tokensBudget"`
    Estimate     Estimate       `json:"estimate"`
    Assignee     AgentID        `json:"assignee,omitempty"`
    Comments     int            `json:"comments"`
    Requeued     bool           `json:"requeued,omitempty"`
    NowPlaying   *NowPlaying    `json:"nowPlaying,omitempty"`
    Reviewer     *Reviewer      `json:"reviewer,omitempty"`
    CreatedAt    string         `json:"createdAt"`
    OpenedAt     string         `json:"openedAt"`
    ClosedAt     string         `json:"closedAt,omitempty"`
    LastActivity string         `json:"lastActivity"`
    Log          []LogEntry     `json:"log"`
    Files        []FileChange   `json:"files"`
    DiffPreview  string         `json:"diffPreview,omitempty"`
    Blocks       []string       `json:"blocks"`
    BlockedBy    []string       `json:"blockedBy"`
}
```

**Field rules**:

| Field | Required | Notes |
|---|---|---|
| `ID` | yes | Generated by `core.NewBeadID()`; format `^bd-[0-9a-f]{4}$` |
| `Title` | yes | 1–255 chars; whitespace-trimmed |
| `Type` | yes | Validated via `BeadType.Valid()`; default `TypeTask` on create |
| `Column` | yes | Validated via `Column.Valid()`; default `ColBacklog` on create |
| `Priority` | yes | 0–4 inclusive; default `2` on create |
| `Labels`, `Skills`, `Blocks`, `BlockedBy`, `Steps`, `SubBeads`, `History`, `Acceptance`, `Log`, `Files` | yes | **Never `null`** — always serialise as `[]` (use empty slice, not nil) |
| `Estimate` | derived | Computed by `DeriveEstimate(TokensBudget)`; never set by caller |
| `Assignee` | derived | Active step's agent → last done step's agent → `""` |
| `LastActivity` | derived | Last `History` entry's `at` field; falls back to `OpenedAt` |
| `Comments` | derived | Count of `EvComment` events + `Reviewer.Comments` (if set) |

### 2.2 `internal/core/step.go`

```go
package core

type Step struct {
    Agent  AgentID    `json:"agent"`
    Mode   Mode       `json:"mode"`
    Skills []string   `json:"skills"`
    Status StepStatus `json:"status"`
    Note   string     `json:"note,omitempty"`
}

type SubBead struct {
    ID        string     `json:"id"`        // "<parent>.N" form, e.g. "bd-a1f2.1"
    Title     string     `json:"title"`
    Status    StepStatus `json:"status"`    // reuses pending|active|done
    Agent     AgentID    `json:"agent,omitempty"`
    AutoSplit bool       `json:"autoSplit,omitempty"`
}
```

### 2.3 `internal/core/history.go`

```go
package core

type HistoryEvent struct {
    At    string    `json:"at"`
    Kind  EventKind `json:"kind"`
    Actor string    `json:"actor"`
    Agent AgentID   `json:"agent,omitempty"`
    Note  string    `json:"note,omitempty"`
}
```

### 2.4 `internal/core/gate.go`

```go
package core

type GateKind string

const (
    GateHuman  GateKind = "human"
    GateTimer  GateKind = "timer"
    GateGitHub GateKind = "github"
)

type GateStatus string

const (
    GateWaiting GateStatus = "waiting"
    GatePassed  GateStatus = "passed"
    GateFailed  GateStatus = "failed"
)

type Gate struct {
    Kind   GateKind   `json:"kind"`
    Label  string     `json:"label"`
    Status GateStatus `json:"status"`
}
```

### 2.4b `internal/core/reference.go` — provider / capacity / dolt seed datasets

Per spec FR-005 the store holds four reference datasets at startup: 14 beads, 4 providers,
4 capacity entries, and the DOLT object. Providers and capacity are not exposed via M0
endpoints; the DOLT object is surfaced through `/orchestrator/status`.

```go
package core

type Provider struct {
    ID       string `json:"id"`        // "claude"|"gemini"|"opencode"|"codex"
    Name     string `json:"name"`
    Mono     string `json:"mono"`      // 2-letter mono code (CC/GM/OC/CX)
    Color    string `json:"color"`     // hex
    Parallel int    `json:"parallel"`  // max parallel sessions
    Kind     string `json:"kind"`      // "cli"|"sdk"
    Plan     string `json:"plan,omitempty"`
}

type Capacity struct {
    Agent   AgentID `json:"agent"`
    Running int     `json:"running"`
    Queued  int     `json:"queued"`
    Limit   int     `json:"limit"`
}

// DoltStatus is the canonical shape; api/health's OrchestratorStatusResponse references it.
type DoltStatus struct {
    Branch   string `json:"branch"`
    Remote   string `json:"remote"`
    Ahead    int    `json:"ahead"`
    Behind   int    `json:"behind"`
    LastSync string `json:"lastSync"`
    Status   string `json:"status"`
    Server   string `json:"server"`
    Port     int    `json:"port"`
    Writers  int    `json:"writers"`
}
```

### 2.5 `internal/core/auxiliary.go`

```go
package core

type Acceptance struct {
    Text string `json:"text"`
    Done bool   `json:"done"`
}

type NowPlayingKind string

const (
    NPKTool    NowPlayingKind = "tool"
    NPKThought NowPlayingKind = "thought"
    NPKOutput  NowPlayingKind = "output"
)

func (k NowPlayingKind) Valid() bool {
    switch k {
    case NPKTool, NPKThought, NPKOutput:
        return true
    }
    return false
}

type NowPlaying struct {
    Action string         `json:"action"`
    Since  int            `json:"since"`  // seconds elapsed
    Kind   NowPlayingKind `json:"kind"`
}

type Reviewer struct {
    Agent    AgentID `json:"agent"`
    Comments int     `json:"comments"`
}

type LogKind string

const (
    LogSystem  LogKind = "system"
    LogTool    LogKind = "tool"
    LogThought LogKind = "thought"
    LogOutput  LogKind = "output"
)

func (k LogKind) Valid() bool {
    switch k {
    case LogSystem, LogTool, LogThought, LogOutput:
        return true
    }
    return false
}

type LogEntry struct {
    T    string  `json:"t"`     // opaque timestamp string (e.g., "09:14:02" or RFC3339)
    Kind LogKind `json:"kind"`
    Msg  string  `json:"msg"`
}

type FileStatus string

const (
    FileAdded    FileStatus = "A"
    FileModified FileStatus = "M"
    FileDeleted  FileStatus = "D"
)

func (s FileStatus) Valid() bool {
    switch s {
    case FileAdded, FileModified, FileDeleted:
        return true
    }
    return false
}

type FileChange struct {
    Path   string     `json:"path"`
    Status FileStatus `json:"status"`
    Adds   int        `json:"adds"`
    Dels   int        `json:"dels"`
}
```

### 2.6 `internal/core/id.go`

```go
package core

import (
    "strings"
    "github.com/google/uuid"
)

// NewBeadID generates "bd-XXXX" where XXXX is the first 4 hex chars (lowercase)
// of a UUIDv4. Callers must check store-level uniqueness and retry on collision.
func NewBeadID() string {
    return "bd-" + strings.ToLower(uuid.NewString()[:4])
}
```

### 2.7 `internal/core/validate.go`

```go
package core

import "errors"

var (
    ErrTitleRequired   = errors.New("title is required")
    ErrTitleTooLong    = errors.New("title exceeds 255 chars")
    ErrInvalidType     = errors.New("invalid type")
    ErrInvalidColumn   = errors.New("invalid column")
    ErrInvalidPriority = errors.New("invalid priority")
    ErrInvalidVCS      = errors.New("invalid vcs")
    ErrInvalidAgent    = errors.New("invalid agent")
    ErrInvalidMode     = errors.New("invalid mode")
    ErrInvalidID       = errors.New("invalid bead id")
)

// DeriveEstimate maps a token budget to an XS/S/M/L size.
func DeriveEstimate(tokensBudget int) Estimate {
    switch {
    case tokensBudget >= 350_000:
        return EstL
    case tokensBudget >= 180_000:
        return EstM
    case tokensBudget >= 90_000:
        return EstS
    default:
        return EstXS
    }
}

// DeriveAssignee returns the active step's agent, falling back to the last
// step whose status is StepDone, falling back to "".
func DeriveAssignee(steps []Step) AgentID {
    for _, s := range steps {
        if s.Status == StepActive {
            return s.Agent
        }
    }
    for i := len(steps) - 1; i >= 0; i-- {
        if steps[i].Status == StepDone {
            return steps[i].Agent
        }
    }
    return ""
}

// DeriveCommentCount counts comment events in history plus reviewer comments.
func DeriveCommentCount(history []HistoryEvent, reviewer *Reviewer) int {
    n := 0
    for _, h := range history {
        if h.Kind == EvComment {
            n++
        }
    }
    if reviewer != nil {
        n += reviewer.Comments
    }
    return n
}
```

---

## 3. Transport DTOs (`internal/api/<resource>/dto.go`)

DTOs live next to their handlers, **not** in `internal/core`. Domain types are returned verbatim
on the wire, but request bodies are transport-specific shapes that get translated into either
a `core.Bead` (for Create) or a `BeadPatch` value object (for Patch).

### 3.1 `internal/api/beads/dto.go`

```go
package beads

import "github.com/gitrgoliveira/muster/internal/core"

type ListResponse struct {
    Items      []core.Bead `json:"items"`
    NextCursor *string     `json:"nextCursor"` // always null in M0
    Total      int         `json:"total"`
}

type CreateRequest struct {
    Title        string         `json:"title"`                  // required
    Desc         string         `json:"desc,omitempty"`
    Type         core.BeadType  `json:"type,omitempty"`         // default TypeTask
    Column       core.Column    `json:"column,omitempty"`       // default ColBacklog
    Priority     *core.Priority `json:"priority,omitempty"`     // default 2 — pointer so 0 is distinguishable
    Labels       []string       `json:"labels,omitempty"`
    VCS          core.VCS       `json:"vcs,omitempty"`          // default VCSGit
    TokensBudget int            `json:"tokensBudget,omitempty"` // default 0
}

// PatchRequest uses pointer fields so absence (nil) is distinguishable from
// zero-value updates ("" or 0). Slice fields treat nil as "no change", non-nil
// (including empty) as "set to this value".
type PatchRequest struct {
    Title        *string         `json:"title,omitempty"`
    Desc         *string         `json:"desc,omitempty"`
    Type         *core.BeadType  `json:"type,omitempty"`
    Column       *core.Column    `json:"column,omitempty"`
    Priority     *core.Priority  `json:"priority,omitempty"`
    Labels       *[]string       `json:"labels,omitempty"`       // **pointer to slice**: nil = no change
    Ready        *bool           `json:"ready,omitempty"`
    TokensBudget *int            `json:"tokensBudget,omitempty"`
}

type MoveRequest struct {
    ToColumn core.Column `json:"toColumn"`           // required
    BeforeID string      `json:"beforeID,omitempty"` // optional; insert before this bead in toColumn
}
// BeforeID semantics:
//   "" or absent  → append at end of toColumn
//   present       → bead is inserted immediately before the referenced bead in toColumn;
//                   referenced bead MUST exist, MUST be in toColumn, MUST NOT equal {id}.

type DispatchRequest struct {
    Agent core.AgentID `json:"agent"` // required
    Mode  core.Mode    `json:"mode"`  // required
}

type CommentRequest struct {
    Actor string `json:"actor"` // required
    Note  string `json:"note"`  // required, non-empty after trim
}
```

### 3.2 `internal/api/render/errors.go`

```go
package render

type ErrorResponse struct {
    Error ErrorDetail `json:"error"`
}

type ErrorDetail struct {
    Code      string `json:"code"`
    Message   string `json:"message"`
    RequestID string `json:"requestID"`
}

// Error codes — exhaustively enumerated for M0. Matches spec.md §Error Codes.
const (
    CodeBeadNotFound     = "BEAD_NOT_FOUND"     // 404 — known path, missing ID
    CodeNotFound         = "NOT_FOUND"          // 404 — unknown /api/v1/* path
    CodeInvalidRequest   = "INVALID_REQUEST"    // 400 — body validation, malformed JSON, unknown field, null, empty PATCH, invalid enum, unknown beforeID
    CodeInvalidState     = "INVALID_STATE"      // 400 — lifecycle precondition (e.g., dispatch from non-scheduled)
    CodeMethodNotAllowed = "METHOD_NOT_ALLOWED" // 405 — wrong HTTP verb on a known path
    CodeInternal         = "INTERNAL"           // 500 — panic, ID generation exhausted after 3 retries
)
```

### 3.3 `internal/api/health/dto.go`

```go
package health

type HealthResponse struct {
    OK bool `json:"ok"`
}

// OrchestratorStatusResponse matches spec.md §US10 acceptance scenario 2.
// The `dolt` field is populated from the DOLT seed object in prototype/data.jsx.
// DoltStatus lives in core/reference.go (canonical source).
type OrchestratorStatusResponse struct {
    Build         string          `json:"build"`         // "dev" in M0
    SchemaVersion int             `json:"schemaVersion"` // 1 in M0
    BeadsVersion  string          `json:"beadsVersion"`  // from seed repos[0].detected.beadsVersion
    Online        bool            `json:"online"`        // true while WS hub is running
    ServerTime    string          `json:"serverTime"`    // RFC3339 timestamp
    Dolt          core.DoltStatus `json:"dolt"`          // from seed DOLT object
}
```

---

## 4. Service Value Objects (`internal/services/`)

Services consume DTOs at the API boundary and convert them into normalised value objects.
This keeps validation in one place and lets the store remain a thin CRUD layer.

### 4.1 `internal/services/beads.go` (input shapes)

```go
package services

import "github.com/gitrgoliveira/muster/internal/core"

// CreateBeadInput is the post-validation, fully-defaulted form of a create
// request. Built by BeadService.Create from the API DTO.
type CreateBeadInput struct {
    Title        string
    Desc         string
    Type         core.BeadType
    Column       core.Column
    Priority     core.Priority
    Labels       []string
    VCS          core.VCS
    TokensBudget int
}

// PatchBeadInput is the post-validation patch shape. Each field is a pointer
// matching the DTO; the service inspects each and mutates the bead accordingly.
type PatchBeadInput struct {
    Title        *string
    Desc         *string
    Type         *core.BeadType
    Column       *core.Column
    Priority     *core.Priority
    Labels       *[]string
    Ready        *bool
    TokensBudget *int
}
```

---

## 5. WebSocket Events (`internal/ws/`)

Seven event types per spec FR-012. Each has a distinct payload; `Event` is the marshalled
union. See `contracts/ws-events.md` for example payloads.

### 5.1 `internal/ws/event.go`

```go
package ws

import "github.com/gitrgoliveira/muster/internal/core"

type EventType string

const (
    EventHello        EventType = "hello"         // server → client, sent within 1s of connect (FR-013)
    EventBeadCreated  EventType = "bead.created"  // after POST /beads 201
    EventBeadUpdated  EventType = "bead.updated"  // after PATCH /beads, POST /dispatch
    EventBeadMoved    EventType = "bead.moved"    // after POST /move
    EventBeadDeleted  EventType = "bead.deleted"  // reserved — not emitted in M0
    EventCommentAdded EventType = "comment.added" // after POST /comments 201
    EventPong         EventType = "pong"          // response to client ping (FR-014)
)

// Frame is the marshalled union. Each event type uses a subset of fields per the contract.
type Frame struct {
    Type EventType `json:"type"`

    // hello fields
    Build         string `json:"build,omitempty"`
    SchemaVersion int    `json:"schemaVersion,omitempty"`
    ServerTime    string `json:"serverTime,omitempty"`
    BeadsVersion  string `json:"beadsVersion,omitempty"`

    // bead.created / bead.updated / bead.moved / comment.added — full bead
    Bead *core.Bead `json:"bead,omitempty"`

    // bead.moved / bead.deleted / comment.added — id reference
    ID string `json:"id,omitempty"`

    // bead.moved
    FromColumn core.Column `json:"fromColumn,omitempty"`
    ToColumn   core.Column `json:"toColumn,omitempty"`
    BeforeID   string      `json:"beforeID,omitempty"`

    // comment.added
    Event *core.HistoryEvent `json:"event,omitempty"`

    // pong
    At string `json:"at,omitempty"`
}
```

Client → server frame (the only one accepted):

```go
type ClientFrame struct {
    Type string `json:"type"` // only "ping" is handled in M0; others log WARN
}
```

---

## 6. Default Values (`internal/services/defaults.go`)

Centralised defaults applied on create when a field is absent.

| Field | Default |
|---|---|
| `Type` | `core.TypeTask` |
| `Column` | `core.ColBacklog` |
| `Priority` | `2` |
| `VCS` | `core.VCSGit` |
| `Repo` | `"main"` |
| `Ready` | `false` |
| `TokensUsed` | `0` |
| `TokensBudget` | `0` |
| `Labels`, `Skills`, `Blocks`, `BlockedBy`, `Steps`, `SubBeads`, `Acceptance`, `Log`, `Files` | `[]` (empty slice, not nil) |
| `History` | one `EvOpened` event with actor `"user"` |
| `CreatedAt`, `OpenedAt`, `LastActivity` | RFC3339 timestamp of creation |

---

## 7. State Transition Tables

### 7.1 Column transitions

**`POST /move` (unrestricted)**: any column → any column. Validator only checks `toColumn` is
one of the five valid values. Position within `toColumn` is controlled by `beforeID`.

**`POST /dispatch` (gated)**: only `scheduled` → `running` is accepted. Dispatch from any
other column returns `400 INVALID_STATE`. Per spec FR-010/US8.

Other future lifecycle gating (e.g., disallowing `done → running` without a `reopened` event)
is deferred to M1.

### 7.2 Step status

Step status transitions are not enforced at the API layer in M0. The store accepts whatever 
`status` value the API supplies and validates against `StepStatus.Valid()`.

---

## 7.3 Timestamp handling

All `at`, `createdAt`, `openedAt`, `closedAt`, `lastActivity`, `LogEntry.T`, and 
`HistoryEvent.At` fields are **opaque strings**. The server does not parse them. Seed data 
retains the prototype's calendar form (`"Mon 09:14"`); new events generated by handlers use 
RFC3339 (`"2026-05-22T17:42:11Z"`). Per spec round-3 clarifications.

`OrchestratorStatusResponse.ServerTime` is the only field guaranteed to be RFC3339 (since it's
freshly generated on each request).

## 7.4 Size limits

- **Request body**: `http.MaxBytesReader(w, r.Body, 1<<20)` wraps every POST/PATCH handler. 
  Exceeding 1 MiB → 400, JSON error body with code `INVALID_REQUEST` and message 
  `request body exceeds 1 MiB limit`.
- **WebSocket frame**: `websocket.Conn.SetReadLimit(1 << 20)` after `Accept`. Exceeding → 
  library closes the connection with status 1009 (message too big).
- **Title length**: 255 **runes** (`utf8.RuneCountInString`), not bytes.

## 8. Edge Cases

| Case | Behaviour |
|---|---|
| `PATCH` with empty body `{}` | **400 INVALID_REQUEST** — `patch body must contain at least one field` (per spec §Edge Cases) |
| `PATCH` with `"labels": []` | Labels set to empty slice (slice cleared) |
| `PATCH` with `"labels": null` | 400 INVALID_REQUEST (null not accepted for slice fields) |
| `PATCH` with unknown field | 400 INVALID_REQUEST (json decoder uses `DisallowUnknownFields`) |
| `POST /beads` with `"title": "   "` | 400 INVALID_REQUEST (whitespace-only after trim) |
| `POST /move` same column without `beforeID` | 200 OK, bead appended to end of that column (effective no-op if already last); `bead.moved` WS event fires |
| `POST /move` same column with `beforeID` | 200 OK, bead reordered; `bead.moved` WS event fires |
| `POST /move` with `beforeID == {id}` | 400 INVALID_REQUEST — bead cannot insert before itself |
| `POST /move` with unknown `beforeID` | 400 INVALID_REQUEST — `no such beforeID: bd-xxxx` |
| `POST /move` with `beforeID` in a different column than `toColumn` | 400 INVALID_REQUEST — `beforeID must be in toColumn` |
| `POST /dispatch` from non-`scheduled` column | 400 INVALID_STATE — `cannot dispatch bead in column <col>` |
| ID collision on create | Store retries up to 3 times; if all collide, returns 500 INTERNAL |
| Concurrent reads + write | Reads use `RLock`; writes use `Lock`; reads see snapshot-consistent state |
| WS client slow to drain | Client's `send` channel is buffered (16); on overflow, message dropped for that client; after 3 drops in 10 s the client is unregistered |
| Multiple WS broadcasts during single mutation | Hub serialises events via internal channel; per-client ordering preserved |
| WS client connects mid-mutation | Receives `hello` only; missed event recovered via `GET /beads` refetch |
| WS client sends `{"type":"ping"}` | Server responds with `{"type":"pong","at":...}` |
| WS client sends other application frame | Logged at WARN, discarded |
