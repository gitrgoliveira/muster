# Quickstart: musterd (M0)

**Goal**: Get `musterd` running locally in under 5 minutes.

## Prerequisites

- Go 1.26+
- `git clone https://github.com/gitrgoliveira/muster` (or your fork)

## Build and run

```bash
# 1. Copy the prototype UI files to the embed target.
#    (Or run: make ui-copy)
cp prototype/* ui/

# 2. Build — the //go:embed ui/* directive requires ui/ to be populated.
go build ./cmd/musterd

# 3. Run
./musterd serve
# or: go run ./cmd/musterd serve

# Expected stdout (banner — matches spec §Assumptions):
# musterd listening on http://127.0.0.1:7766 (build=dev schemaVersion=1)
#
# Expected stderr (slog text handler):
# time=2026-05-22T17:42:11Z level=INFO msg="listening" addr=127.0.0.1:7766
```

## Open the UI

Navigate to <http://localhost:7766> — the prototype Kanban board loads with 14 seed beads.

## Try the API — happy path

```bash
# List all beads
curl -s http://localhost:7766/api/v1/beads | jq '.total'
# → 14

# Filter by column
curl -s "http://localhost:7766/api/v1/beads?column=running" | jq '[.items[].id]'
# → ["bd-a1f2", "bd-c411"]

# Get a single bead
curl -s http://localhost:7766/api/v1/beads/bd-a1f2 | jq '.title'
# → "Refactor auth middleware for OAuth refresh tokens"

# Create a bead (only `title` is required)
curl -s -X POST http://localhost:7766/api/v1/beads \
  -H 'Content-Type: application/json' \
  -d '{"title":"My new bead"}' | jq '{id, type, column, priority}'
# → { "id": "bd-xxxx", "type": "task", "column": "backlog", "priority": 2 }

# Patch a bead (sparse — only fields you supply are updated)
curl -s -X PATCH http://localhost:7766/api/v1/beads/bd-a1f2 \
  -H 'Content-Type: application/json' \
  -d '{"priority":0,"labels":["urgent","regression"]}' | jq '.priority'
# → 0

# Move a bead (M0 allows any-to-any column transition; beforeID controls position)
curl -s -X POST http://localhost:7766/api/v1/beads/bd-a1f2/move \
  -H 'Content-Type: application/json' \
  -d '{"toColumn":"review"}' | jq '.column'
# → "review"

# Move with positional insertion
curl -s -X POST http://localhost:7766/api/v1/beads/bd-b210/move \
  -H 'Content-Type: application/json' \
  -d '{"toColumn":"backlog","beforeID":"bd-2d55"}' | jq '.column'
# → "backlog"  (bd-b210 now sits immediately before bd-2d55 within backlog)

# Dispatch a step
curl -s -X POST http://localhost:7766/api/v1/beads/bd-a1f2/dispatch \
  -H 'Content-Type: application/json' \
  -d '{"agent":"claude","mode":"plan"}' | jq '.steps[-1]'
# → { "agent": "claude", "mode": "plan", "status": "active", "skills": [] }

# Add a comment (returns 201 per spec US9)
curl -s -X POST http://localhost:7766/api/v1/beads/bd-a1f2/comments \
  -H 'Content-Type: application/json' \
  -d '{"actor":"you@yours.dev","note":"LGTM modulo migration"}' | jq '.comments'
# → (incremented count)

# Health
curl -s http://localhost:7766/api/v1/healthz
# → {"ok":true}

# Orchestrator status (full payload per spec US10)
curl -s http://localhost:7766/api/v1/orchestrator/status | jq .
# → {
#     "build": "dev",
#     "schemaVersion": 1,
#     "beadsVersion": "0.9.1",
#     "online": true,
#     "serverTime": "2026-05-22T17:42:11Z",
#     "dolt": { "branch": "main", "remote": "origin", "ahead": 0, ... }
#   }
```

## Try the API — error paths

```bash
# Missing title → 400 INVALID_REQUEST
curl -s -X POST http://localhost:7766/api/v1/beads \
  -H 'Content-Type: application/json' -d '{}' | jq .
# → { "error": { "code": "INVALID_REQUEST", "message": "title is required", "requestID": "..." } }

# Unknown bead → 404 BEAD_NOT_FOUND
curl -s http://localhost:7766/api/v1/beads/bd-zzzz | jq .
# → { "error": { "code": "BEAD_NOT_FOUND", "message": "no such bead: bd-zzzz", "requestID": "..." } }

# Invalid enum → 400
curl -s -X POST http://localhost:7766/api/v1/beads/bd-a1f2/move \
  -H 'Content-Type: application/json' -d '{"toColumn":"nope"}' | jq .
# → { "error": { "code": "INVALID_REQUEST", "message": "invalid column: nope", "requestID": "..." } }

# Empty PATCH body → 400 (per spec §Edge Cases)
curl -s -X PATCH http://localhost:7766/api/v1/beads/bd-a1f2 \
  -H 'Content-Type: application/json' -d '{}' | jq .
# → { "error": { "code": "INVALID_REQUEST", "message": "patch body must contain at least one field", ... } }

# Dispatch from non-scheduled column → 400 INVALID_STATE (per spec FR-010)
curl -s -X POST http://localhost:7766/api/v1/beads/bd-b210/dispatch \
  -H 'Content-Type: application/json' -d '{"agent":"claude","mode":"plan"}' | jq .
# → { "error": { "code": "INVALID_STATE", "message": "cannot dispatch bead in column backlog", ... } }

# Custom request ID is echoed in the error
curl -s -X POST http://localhost:7766/api/v1/beads \
  -H 'Content-Type: application/json' \
  -H 'X-Request-ID: my-trace-123' \
  -d '{}' | jq '.error.requestID'
# → "my-trace-123"
```

## WebSocket stream

```bash
# Using websocat (brew install websocat)
websocat ws://localhost:7766/api/v1/stream
# → JSON events for every mutation

# First frame (within 1s of connect, per spec FR-013):
# {"type":"hello","build":"dev","schemaVersion":1,"serverTime":"...","beadsVersion":"0.9.1"}

# Send a ping (per spec FR-14):
echo '{"type":"ping"}' | websocat ws://localhost:7766/api/v1/stream
# → {"type":"hello",...} then {"type":"pong","at":"..."}

# In another terminal, trigger a move:
curl -s -X POST http://localhost:7766/api/v1/beads/bd-a1f2/move \
  -H 'Content-Type: application/json' -d '{"toColumn":"done"}'

# websocat output:
# {"type":"bead.moved","id":"bd-a1f2","fromColumn":"running","toColumn":"done","bead":{...}}
```

## Custom bind address

```bash
./musterd serve --addr 0.0.0.0:8080
```

## Run tests

```bash
# Everything, with race detector (required for ws/, store/ concurrency)
go test -race ./...

# Just one package
go test -race ./internal/core/...

# With coverage
go test -race -coverprofile=cover.out ./...
go tool cover -func=cover.out
```

## Flags

| Flag | Default | Description |
|---|---|---|
| `--addr` | `127.0.0.1:7766` | Bind address (host:port) |

## Development workflow

During development, regenerate the embedded UI from the prototype:

```bash
make ui-copy    # cp -r prototype/* ui/
# or:
go generate ./cmd/musterd/...
```

The `ui/` directory is **kept under .gitignore** (except `.gitkeep`) so prototype changes flow
through a single source of truth (`prototype/`).
