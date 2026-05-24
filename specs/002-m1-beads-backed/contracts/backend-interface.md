# Contract: `internal/store.Backend`

The read interface over a beads database. Two production implementations: `jsonl.Backend` (embedded mode, reads `issues.jsonl`) and `dolt.Backend` (server mode, MySQL wire). One test implementation: `memory.Backend` (carried from M0).

---

## Go interface

```go
package store

import (
    "context"
    "errors"
)

type Backend interface {
    // List returns issues matching f. Order is undefined unless caller sorts.
    // ctx cancellation MUST be respected — implementations cancel the underlying SQL query.
    List(ctx context.Context, f Filter) ([]Issue, error)

    // Get returns a single issue by ID. Returns ErrNotFound when no row exists.
    Get(ctx context.Context, id string) (*Issue, error)

    // Ping verifies the backend is reachable. Returns nil if healthy.
    // Used by GET /api/v1/orchestrator/status to populate the `online` field.
    // Cheap operation: JSONL stats the file; Dolt SQL runs `SELECT 1`.
    Ping(ctx context.Context) error

    // Close releases backend resources (SQL conn pool, spawned subprocess, file handles).
    // Safe to call multiple times.
    Close() error
}

type Filter struct {
    Status       []string // nil/empty = no filter on status
    IDs          []string // nil/empty = no filter on ID
    Limit        int      // 0 = unlimited
    TruncateDesc int      // 0 = no truncation; N > 0 = cap Description at N bytes
}
```

## Error sentinels

```go
var (
    ErrNotFound         = errors.New("issue not found")
    ErrStoreUnavailable = errors.New("store unavailable")    // Dolt server down, lock contention
    ErrStoreReadOnly    = errors.New("store is read-only")   // opened with --readonly
    ErrSchemaMismatch   = errors.New("schema version mismatch")
)
```

Implementations MUST wrap underlying errors with `fmt.Errorf("backend: %w", err)` so callers can
`errors.Is(err, store.ErrStoreUnavailable)`.

---

## Behavioral contract

| Method | MUST | MUST NOT |
|---|---|---|
| `List` | Return `[]Issue{}` (empty, non-nil) when no rows match | Block longer than the ctx deadline |
| `List` | Respect `Filter.Limit > 0` | Allocate beyond `Limit` rows |
| `Get` | Return `ErrNotFound` for missing IDs | Return `nil, nil` |
| `Get` | Treat `id == ""` as `ErrNotFound` | Panic on empty input |
| `Close` | Be idempotent | Block longer than 5 seconds |

## Concurrency

- All methods are safe for concurrent use.
- `List` and `Get` may share a connection pool internally; callers do not coordinate.

## Cancellation

- If `ctx.Err() != nil` on entry, return `ctx.Err()` immediately without I/O.
- If cancellation arrives mid-query, the underlying SQL driver receives `KILL QUERY`; the method returns `ctx.Err()`.

---

## Implementations

### `internal/store/jsonl.Backend` (embedded mode)

- Constructor: `NewJSONL(path string) (Backend, error)`
- Reads and parses `<path>/issues.jsonl` — one JSON object per line
- `List` re-reads the file on each call (the file is small, ≤5000 issues)
- `Close()` is a no-op (no resources to release)
- No Dolt dependency — pure Go JSON parsing

### `internal/store/dolt.Backend` (server mode)

- Constructor: `NewDolt(ctx, dsn string) (Backend, error)`
- Connects via `database/sql.Open("mysql", dsn)` with `parseTime=true`
- DSN built from `BackendConfig.{DoltUser,DoltPassword,DoltHost,DoltPort,DoltDatabase}` (`metadata.json` + `BEADS_DOLT_PASSWORD` env); muster calls `bd dolt start` (idempotent) before opening the connection
- `Close()` closes the SQL connection pool; does NOT stop the Dolt server

### `internal/store/memory.Backend` (test-only)

- Constructor: `NewMemory(seed []Issue) Backend`
- In-memory slice; `Close()` is a no-op
- Useful for service-layer unit tests that don't need Dolt or JSONL files
