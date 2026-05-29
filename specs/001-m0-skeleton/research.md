# Research: M0 — Skeleton

**Feature**: M0 — Skeleton (muster binary + in-memory API + WS)
**Date**: 2026-05-22

## Decision 1: go:embed strategy for prototype UI files

**Decision**: Use `//go:embed ui/*` in `cmd/muster/embed.go`. The `ui/` directory at the repo root is a verbatim copy of `prototype/` (all 12 files: Muster.html, styles.css, 10 JSX files). Use `fs.Sub(embeddedFS, "ui")` to strip the `ui/` prefix before serving with `http.FileServer`.

**Rationale**: `go:embed` with a wildcard embeds all files in the directory without needing to list them. `fs.Sub` allows serving at `/` rather than `/ui/`. The prototype uses bare relative `src=` paths (e.g., `src="data.jsx"`) so the server must serve files at root-level paths.

**Alternatives considered**:
- Embedding each file separately: rejected — fragile, requires updating embed directives whenever a file is added.
- Serving `prototype/` directly from disk at runtime: rejected — binary must be self-contained.

**Important**: The `ui/` directory must be populated before `go build`. A `Makefile` target or `go generate` comment will copy `prototype/*` → `ui/*`. The `ui/` dir must exist (with at least a `.gitkeep`) so `go:embed` doesn't fail on a fresh clone.

---

## Decision 2: WebSocket hub architecture (coder/websocket)

**Decision**: Single `Hub` struct in `internal/ws/hub.go` with:
- `clients map[*Client]struct{}` guarded by `sync.Mutex`
- `broadcast chan []byte` for outbound messages
- `register/unregister chan *Client` for lifecycle
- Hub runs in a single goroutine (the `Run()` loop)

Each `Client` wraps a `*websocket.Conn` and has a `send chan []byte`. The `writePump` goroutine drains the channel and writes to the WS connection. The HTTP handler registers the client, then starts `readPump` (handles ping frames) and `writePump` goroutines.

**Rationale**: The canonical Go WebSocket hub pattern. `coder/websocket` v1 uses `conn.Write(ctx, websocket.MessageText, data)` and `conn.Read(ctx)`. The hub goroutine model avoids concurrent writes to the same connection (which `coder/websocket` prohibits).

**Alternatives considered**:
- Per-connection goroutine writing directly: rejected — race condition on concurrent broadcasts.
- Using channels without a hub goroutine: rejected — requires locking on client map during broadcast.

**Note**: `coder/websocket` does NOT support `SetReadDeadline` like `gorilla/websocket`. Use context cancellation for timeouts: derive a context with `context.WithTimeout` for each read.

---

## Decision 3: In-memory store concurrency

**Decision**: `sync.RWMutex` in `internal/store/store.go`. All reads use `RLock/RUnlock`. All writes (create, patch, move, dispatch, comment) use `Lock/Unlock`. The store holds `[]core.Bead` as a slice; lookups are O(n) linear scan (acceptable for 14–1000 beads in M0).

**Rationale**: `sync.RWMutex` allows concurrent reads while serialising writes. For M0's scale (tens of beads, <10 concurrent HTTP connections), this is correct and simple. No need for a concurrent map or copy-on-write.

**Alternatives considered**:
- `sync.Map`: rejected — worse ergonomics for a slice-based store with ordering.
- Channel-based serialisation: rejected — more complex than a mutex for this scale.

---

## Decision 4: Bead ID generation

**Decision**: `"bd-" + strings.ToLower(uuid.New().String()[:4])` using `github.com/google/uuid`.

`uuid.New()` returns a UUIDv4 in the format `xxxxxxxx-xxxx-4xxx-yxxx-xxxxxxxxxxxx`. Taking `[:4]` gives the first 4 hex chars (e.g., `550e` from `550e8400-...`). Result: `bd-550e`.

**Collision risk**: With 14 seed beads and M0's expected usage (tens of new beads), the probability of collision is negligible. The store's `Create` function checks for an existing ID and retries up to a maximum of **3 attempts total** (1 initial + 2 retries). If all three collide, the create returns `500 INTERNAL` with message `failed to generate unique ID`. This matches the error matrix in `contracts/rest-api.md`.

**Rationale**: Matches exactly the format used in seed data (`bd-a1f2`, `bd-c411`, etc.) and what the beads CLI assigns.

---

## Decision 5: Seed data strategy

**Decision**: Hard-code seed data as Go literals in `internal/store/seed.go`. The function `SeedBeads() []core.Bead` returns the 14 beads from `prototype/data.jsx` TASKS array, with all derived fields computed:
- `Estimate`: derived from `TokensBudget` (≥350k→L, ≥180k→M, ≥90k→S, else→XS)
- `Assignee`: derived from Steps (active step's agent, else last done step's agent)
- `History`: from HISTORY_BY_BEAD map
- `Acceptance`: from ACCEPTANCE_BY_BEAD map
- `Comments`: count of `comment` history events + reviewer.comments
- `Repo`: from REPO_OF_BEAD map (default "main")

**Rationale**: The UI's `data.jsx` is the reference; the Go seed data must produce identical JSON output to `window.MUSTER_DATA.TASKS`. Keeping it as Go literals (not parsing the JSX at runtime) avoids a JS runtime dependency.

---

## Decision 6: X-Request-ID middleware

**Decision**: chi middleware in `internal/api/middleware/requestid.go` that:
1. Reads `X-Request-ID` request header; if empty, generates `uuid.New().String()`
2. Stores in context via a typed key (`requestIDKey struct{}`)
3. Sets `X-Request-ID` response header on all responses

**Rationale**: Spec §4.3 requires this header on all responses. chi's middleware chain makes
this trivial. Living in its own subpackage keeps the middleware reusable across resource
sub-routers without sucking in handler code.

---

## Decision 7: Error response format

**Decision**: All error responses use the shape from spec §4.3:
```json
{"error": {"code": "BEAD_NOT_FOUND", "message": "no such bead", "requestID": "..."}}
```
A helper `render.WriteError(w, r, statusCode, code, message string)` in
`internal/api/render/errors.go` extracts the request ID from context and renders the JSON.
Companion `render.WriteJSON(w, statusCode, body any)` for success responses.

Error code constants (`render.CodeBeadNotFound`, etc.) live in the same package so handlers
import a single symbol set.

---

## Decision 8: chi router layout

**Decision**: Resource sub-routers, assembled in `internal/api/router.go`. Each resource
package (`beads`, `health`, `stream`) exports a `Mount(r chi.Router, deps ...)` function that
registers its own routes on the supplied router.

```go
// internal/api/router.go
func NewRouter(svc *services.BeadService, hub *ws.Hub, ui fs.FS) http.Handler {
    r := chi.NewRouter()
    r.Use(chimw.RequestID, chimw.Logger, chimw.Recoverer, middleware.RequestID)

    r.Route("/api/v1", func(api chi.Router) {
        health.Mount(api)
        beads.Mount(api, svc)
        stream.Mount(api, hub)
    })

    // Catch-all static UI mounted last
    r.Handle("/*", http.FileServer(http.FS(ui)))
    return r
}
```

Each resource package owns its routes:
```go
// internal/api/beads/handlers.go
func Mount(r chi.Router, svc *services.BeadService) {
    h := &handlers{svc: svc}
    r.Route("/beads", func(b chi.Router) {
        b.Get("/", h.List)
        b.Post("/", h.Create)
        b.Get("/{id}", h.Get)
        b.Patch("/{id}", h.Patch)
        b.Post("/{id}/move", h.Move)
        b.Post("/{id}/dispatch", h.Dispatch)
        b.Post("/{id}/comments", h.Comment)
    })
}
```

**Rationale**: This isolates each resource's HTTP wiring next to its DTOs and tests. Adding a
new resource (e.g., `/api/v1/agents` in M1) is a localised change.

---

## Decision 9: Module dependencies (go.mod)

```
module github.com/gitrgoliveira/muster

go 1.26

require (
    github.com/go-chi/chi/v5 v5.2.1
    github.com/coder/websocket v1.8.13
    github.com/google/uuid v1.6.0
    github.com/stretchr/testify v1.10.0
)
```

**Note**: `coder/websocket` was formerly `nhooyr.io/websocket` — the module path changed. Use `github.com/coder/websocket`.

---

## Decision 10: Test approach

**Decision**: 
- `internal/core/` — pure unit tests, no I/O. Table-driven.
- `internal/store/` — unit tests with a fresh `NewStore(SeedBeads())` per test.
- `internal/ws/` — unit tests with a mock conn or `httptest`+`websocket.Dial`.
- `internal/api/` — integration tests using `httptest.NewServer` with a real store. No mocking.

**Rationale**: spec §17.1 sets 80% coverage on core/, 70% on api/. The in-memory store makes full integration tests cheap — no database setup needed.
