# muster

A central hub that aggregates and serves [beads](https://github.com/gastownhall/beads) issues from one or more repositories via a REST + WebSocket API.

## Quick start

```bash
make build
bin/muster serve --beads-dir /path/to/.beads
```

See [`specs/002-m1-beads-backed/quickstart.md`](specs/002-m1-beads-backed/quickstart.md) for a full walkthrough.

## Flags

| Flag | Env | Default | Description |
|---|---|---|---|
| `--beads-dir` | `BEADS_DIR` | `./.beads` if present | Path to the beads directory containing `metadata.json` and `issues.jsonl`. Falls back to `./.beads` only when it exists; otherwise the flag/env is required. |
| `--bd-bin` | `BD_BIN` | `bd` (from PATH) | Path to the `bd` CLI binary. Write endpoints are disabled if `bd` is not found. |
| `--addr` | — | `127.0.0.1:7766` | TCP address to listen on |

## API

| Method | Path | Description |
|---|---|---|
| `GET` | `/api/v1/beads` | List all beads (optional `?column=` filter) |
| `GET` | `/api/v1/beads/{id}` | Get a single bead |
| `POST` | `/api/v1/beads` | Create a bead (requires `bd`) |
| `PATCH` | `/api/v1/beads/{id}` | Update a bead (requires `bd`) |
| `POST` | `/api/v1/beads/{id}/move` | Move to column (requires `bd`) |
| `POST` | `/api/v1/beads/{id}/dispatch` | Dispatch to agent (requires `bd`) |
| `POST` | `/api/v1/beads/{id}/comments` | Add comment (requires `bd`) |
| `GET` | `/api/v1/orchestrator/status` | Backend health and config |
| `GET` | `/api/v1/stream` | WebSocket event stream |

## Build & test

```bash
make build        # produces bin/muster
make test         # go test ./...
make cover-check  # coverage gates
make lint         # gofmt + go vet + golangci-lint
```

## Multi-repo setup

See [`BEADS_MULTIREPO_SETUP_GUIDE.md`](BEADS_MULTIREPO_SETUP_GUIDE.md).
