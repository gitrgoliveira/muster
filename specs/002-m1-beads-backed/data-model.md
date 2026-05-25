# Phase 1 Data Model — M1 Beads-Backed Store

Types introduced or modified for M1. M0 domain types (`core.Bead`, `core.Step`, `core.HistoryEvent`) are unchanged.

---

## `internal/config`

### `BackendConfig`

```go
type BackendConfig struct {
    Mode          Mode    // "embedded" | "remote"
    BeadsDir      string  // absolute path to the .beads/ directory
    DoltDatabase  string  // from metadata.json
    DoltHost      string  // from metadata.json (server mode only)
    DoltPort      int     // from metadata.json (server mode only)
    DoltUser      string  // from metadata.json (server mode only)
    DoltPassword  string  // from BEADS_DOLT_PASSWORD env var (server mode only)
    SchemaVersion int     // 0 = not declared in metadata.json
    ProjectID     string  // from metadata.json (UUID); not used at runtime, surfaced in /orchestrator/status
}

type Mode string

const (
    ModeEmbedded Mode = "embedded"
    ModeRemote   Mode = "remote"
)
```

**Validation**:
- `BeadsDir` must be an absolute path that exists and contains a readable `metadata.json`.
- `Mode == ModeEmbedded` ⇒ `<BeadsDir>/issues.jsonl` must exist and be readable.
- \`Mode == ModeRemote\` ⇒ \`DoltHost\` and \`DoltDatabase\` required; \`DoltPort\` defaults to 3306, \`DoltUser\` defaults to \"root\" (from `metadata.json`); `DoltPassword` read from `BEADS_DOLT_PASSWORD` env var (may be empty); `bd dolt start` must succeed.
- `SchemaVersion`, when present, must be in `[MinSchema, MaxSchema]`.

### `Metadata` (raw shape of `metadata.json`)

```go
type Metadata struct {
    Database      string `json:"database"`       // always "dolt" in M1
    Backend       string `json:"backend"`        // always "dolt" in M1
    DoltMode      string `json:"dolt_mode"`      // "embedded" | "remote"
    DoltDatabase  string `json:"dolt_database"`  // schema name
    DoltHost      string `json:"dolt_host,omitempty"` // server mode: "127.0.0.1" default
    DoltPort      int    `json:"dolt_port,omitempty"` // server mode: 3306 default
    DoltUser      string `json:"dolt_user,omitempty"` // server mode: "root" default
    ProjectID     string `json:"project_id"`     // UUID
    SchemaVersion int    `json:"schema_version,omitempty"`
}
```

---

## `internal/store`

### `Backend` interface

```go
// Backend is the read interface over a beads database.
// Writes go through bdshell.CLI, not Backend.
type Backend interface {
    List(ctx context.Context, f Filter) ([]Issue, error)
    Get(ctx context.Context, id string) (*Issue, error)
    Close() error
}

type Filter struct {
    Status       []string // empty = all statuses
    IDs          []string // empty = all issues
    Limit        int      // 0 = unlimited
    TruncateDesc int      // 0 = no truncation; N > 0 = cap Description at N bytes (list path uses 2048)
}
```

Error sentinels (defined in `internal/store/errors.go`):

```go
var (
    ErrNotFound         = errors.New("issue not found")
    ErrStoreUnavailable = errors.New("store unavailable")
    ErrStoreReadOnly    = errors.New("store is read-only")
    ErrSchemaMismatch   = errors.New("schema version mismatch")
)
```

### `Issue` (the M1 row type)

```go
type Issue struct {
    ID              string    `json:"id"`               // e.g. "mp-kbj"
    Title           string    `json:"title"`
    Description     string    `json:"description"`
    Status          string    `json:"status"`           // "open" | "in_progress" | "closed" | custom
    Priority        int       `json:"priority"`         // 0-4
    IssueType       string    `json:"issue_type"`       // "feature" | "bug" | "task" | ...
    Assignee        string    `json:"assignee"`
    Owner           string    `json:"owner"`
    CreatedAt       time.Time `json:"created_at"`
    UpdatedAt       time.Time `json:"updated_at"`
    StartedAt       *time.Time `json:"started_at,omitempty"`
    ClosedAt        *time.Time `json:"closed_at,omitempty"`
    CloseReason     string    `json:"close_reason,omitempty"`
    DependencyCount int       `json:"dependency_count"`
    DependentCount  int       `json:"dependent_count"`
    CommentCount    int       `json:"comment_count"`
    Notes           string    `json:"notes,omitempty"` // appended via --append-notes
}
```

This maps 1:1 to `issues.jsonl` records and the Dolt `issues` table — verified against
`~/repos/beads-central/.beads/issues.jsonl` (see spec.md sample).

### Mapping `Issue` → M0 `core.Bead` (presentation layer)

M0 handlers return `core.Bead` shapes; M1 keeps the same DTOs by mapping at the service boundary
(`internal/services/beads.go`):

| `core.Bead` field | Sourced from |
|---|---|
| `ID`            | `Issue.ID` |
| `Title`         | `Issue.Title` |
| `Desc`          | `Issue.Description` |
| `Type`          | `Issue.IssueType` |
| `Column`        | derived from `Issue.Status` (mapping below) |
| `Priority`      | `Issue.Priority` |
| `Assignee`      | `Issue.Assignee` (with `DeriveAssignee` fallback to `Owner`) |
| `CreatedAt`     | `Issue.CreatedAt.Format(time.RFC3339)` |
| `LastActivity`  | `Issue.UpdatedAt.Format(time.RFC3339)` |
| `History[]`     | derived: `Issue.CreatedAt`, `StartedAt`, `ClosedAt` → 3 events max in M1 |
| `Acceptance[]`  | empty in M1 (deferred to a future spec) |
| `Steps[]`       | empty in M1 |
| `Labels[]`      | empty in M1 |
| `TokensUsed/Budget` | 0 |
| `Ready`         | computed: `Issue.DependencyCount == 0 && Issue.Status == "open"` |
| `Repo`          | `BackendConfig.DoltDatabase` (e.g. `"mp"`, `"muster"`) |

### Status ↔ Column mapping

```go
var statusToColumn = map[string]core.Column{
    "open":        core.ColBacklog,
    "in_progress": core.ColRunning,
    "blocked":     core.ColBacklog,   // M1 simplification; M2 may add a Blocked column
    "closed":      core.ColDone,
    "deferred":    core.ColBacklog,
    "superseded":  core.ColDone,
}
```

A reverse map drives `POST /move`. Unknown statuses (custom states) default to `Backlog`.

---

## `internal/store/watcher`

### `WatcherEvent`

```go
type WatcherEvent struct {
    // Source describes what triggered the event.
    Source EventSource

    // ChangedIDs is the set of issue IDs that differ from the last snapshot.
    // Populated only after the watcher re-reads from Backend.
    ChangedIDs []string

    // CreatedIDs and DeletedIDs are subsets of ChangedIDs for create/delete bead events.
    CreatedIDs []string
    DeletedIDs []string

    At time.Time
}

type EventSource int

const (
    SourceFSEvent EventSource = iota
    SourcePoll
)
```

### Watcher state

```go
type Watcher struct {
    backend   Backend
    path      string
    snapshot  map[string]Issue   // last-seen state, keyed by ID
    out       chan<- WatcherEvent
    debounce  time.Duration      // 500 * time.Millisecond
    pollEvery time.Duration      // 5 * time.Second
}
```

**Invariants**:
- `snapshot` is read & rewritten only inside the watcher goroutine; never exposed outside.
- `Watcher.Run(ctx)` populates `snapshot` **synchronously** by calling `backend.List` BEFORE
  starting fsnotify and BEFORE the function returns. This eliminates the "every issue looks new"
  flood on first event.
- After each debounced event, `Watcher` calls `backend.List(ctx, Filter{})`, diffs against `snapshot`
  using deep-equality (not just ID-set), emits a single `WatcherEvent` with separate
  `ChangedIDs`/`CreatedIDs`/`DeletedIDs`, then replaces `snapshot`.
- Empty events (no actual field-level changes) are suppressed.

---

## `internal/store/bdshell`

### `CLI`

```go
type CLI struct {
    Path     string         // resolved path to the `bd` binary
    BeadsDir string         // value of BEADS_DIR for the subprocess env
    Timeout  time.Duration  // 5 * time.Second
}

// Result is the typed output of one `bd` invocation.
type Result struct {
    Stdout string
    Stderr string
}
```

Error mapping:

| `bd` exit code | Returned error | Mapped HTTP code (by handler) |
|---|---|---|
| 0 | nil | 200/201/204 |
| non-zero (validation error) | `&CLIError{ExitCode, Stderr}` | 422 UNPROCESSABLE_ENTITY |
| context timeout | `context.DeadlineExceeded` | 504 GATEWAY_TIMEOUT |
| binary not found (M0 startup) | `ErrCLIMissing` | 501 NOT_IMPLEMENTED |

---

## State transitions

```
muster serve
   │
   ▼
ResolveBeadsDir → LoadBackendConfig → ValidateSchema → OpenBackend (JSONL or Dolt SQL)
                                                            │
              ┌─────────────────────────────────────────────┘
              ▼
        Backend ready
              │
              ▼
      Spawn watcher  ──┐  ─── fsnotify event ──▶ debounce ──▶ Backend.List ──▶ diff ──▶ WS broadcast
                       │
                       └── 5 s poll (fallback) ──▶ same path
              │
              ▼
       http.Server up
              │
              ▼
     HTTP/WS requests served
```

---

**Status**: data model ready. Contracts produced next.
