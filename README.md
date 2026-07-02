# muster

A central hub that serves [beads](https://github.com/gastownhall/beads) issues over a REST + WebSocket API, and (M2) runs CLI coding agents against them. M1 serves a single beads repository (via `--beads-dir`); aggregating multiple repositories from one instance is planned for a later milestone.

**M2 — Claude Code adapter:** dispatching a bead launches the `claude` CLI inside a per-bead **git worktree**, hosted in a **tmux** session, streaming its output over WebSocket (`runlog.line`) with live attach/send. Runtime deps: `git`, and `tmux` ≥ 3.2 (optional — without it agents still run via direct exec, but attach/send are disabled). Agent execution is exercisable via the API today; the embedded UI is not yet wired to the live runlog/attach stream (planned follow-up). muster defaults to listening on `127.0.0.1`; non-loopback binds are not yet refused and there is no auth/Origin check, so do not expose beyond localhost (see the `--addr` row below for the hardening follow-up).

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
| `--addr` | — | `127.0.0.1:7766` | TCP address to listen on. Defaults to localhost; **non-loopback binds are not yet refused and there is no auth/Origin check** — do not expose beyond localhost (hardening is a tracked follow-up). |
| `--repo` (repeatable) | `MUSTER_REPO` | — | Map a bead-ID prefix to a source repo, e.g. `--repo mp=/path/to/repo`. Resolves which repo a dispatched bead's worktree branches from (M2). |
| `--worktrees-dir` | `MUSTER_WORKTREES_DIR` | `~/.muster/worktrees` | Root directory for per-bead git worktrees (M2). |
| `--run-timeout` | `MUSTER_RUN_TIMEOUT` | `0` (none) | Optional per-run wall-clock cap, e.g. `30m`. `0` = unbounded (M2). |
| `--default-permission-mode` | `MUSTER_DEFAULT_PERMISSION_MODE` | — | Fallback claude autonomy (`default`/`acceptEdits`/`dontAsk`/`bypassPermissions`/`auto`) applied to **agent-mode** dispatches that omit `permissionMode`. `plan` is **not** valid here — it's implicit for plan-mode dispatches and rejected at startup as a default. muster never defaults autonomy silently (M2). |

## API

| Method | Path | Description |
|---|---|---|
| `GET` | `/api/v1/beads` | List all beads (optional `?column=` filter) |
| `GET` | `/api/v1/beads/{id}` | Get a single bead |
| `POST` | `/api/v1/beads` | Create a bead (requires `bd`) |
| `PATCH` | `/api/v1/beads/{id}` | Update a bead (requires `bd`) |
| `POST` | `/api/v1/beads/{id}/move` | Move to column (requires `bd`) |
| `POST` | `/api/v1/beads/{id}/dispatch` | Run a CLI agent on the bead — body `{agent, mode, permissionMode}` (M2) |
| `GET` | `/api/v1/beads/{id}/steps/{idx}/attach` | tmux attach command + pane for a running step (M2; `idx=0`) |
| `POST` | `/api/v1/beads/{id}/steps/{idx}/send` | Forward keystrokes to the live agent pane (M2; `idx=0`) |
| `POST` | `/api/v1/beads/{id}/comments` | Add comment (requires `bd`) |
| `GET` | `/api/v1/orchestrator/status` | Backend health, config, tmux + adapter availability |
| `GET` | `/api/v1/stream` | WebSocket event stream (`bead.*`, `runlog.line`, `tmux.session.*`) |

## Build & test

```bash
make build        # produces bin/muster
make test         # go test ./... (claude always faked; real-tmux tests run if tmux is present, else skip)
make cover-check  # coverage gates
make lint         # gofmt + go vet + golangci-lint
make test-e2e     # real end-to-end M2 run (needs claude logged in + tmux)
```

`make test` never *requires* `claude` or `tmux` to be installed: `claude` is always faked, and the handful of integration tests that exercise the real `tmux` transport skip automatically when a supported `tmux` (>= 3.2) isn't present — but they **do** run against a real `tmux` when one is available, so they aren't fakes-only. `make test-e2e` (build-tagged, excluded from the default suite) drives a real dispatch against `claude` + `tmux` — see the [M2 quickstart](specs/003-m2-cli-adapter/quickstart.md). The `test-e2e` target and the underlying `//go:build e2e` test ship with the M2 orchestrator stack (stack 3 of the M2 PR series); on a tree that has only the docs/foundation stacks merged the target is absent, and `go test -tags=e2e -run TestE2E ./internal/orchestrator/` is the manual equivalent.

## Multi-repo setup

See [`BEADS_MULTIREPO_SETUP_GUIDE.md`](BEADS_MULTIREPO_SETUP_GUIDE.md).
