# Quickstart: M2 — Dispatch a bead to Claude Code

End-to-end walkthrough of the M2 happy path. Assumes M1 is built and a beads dir exists.

## Prerequisites

muster probes `tmux` (>= 3.2) and the `claude` adapter at startup. `git` is
required at run time (worktree creation shells out to it) but is **not** probed
or version-checked at startup.

```bash
tmux -V                 # >= 3.2  (probed at startup)
claude --version        # the adapter (probed at startup)
claude auth status --json   # {"loggedIn": true, ...}
git --version           # >= 2.40 recommended (used at run time, not probed)
```

## 1. Start muster with a repo mapping

```bash
make build
bin/muster serve \
  --beads-dir ~/repos/beads-central/.beads \
  --repo mp=~/repos/bracket-creator \
  --worktrees-dir ~/.muster/worktrees \
  --default-permission-mode acceptEdits     # optional; else pass permissionMode per dispatch
```
Startup banner reports `tmuxAvailable`, tmux version, and the detected `claude` adapter (version + loggedIn). Confirm:
```bash
curl -s localhost:7766/api/v1/orchestrator/status | jq '{tmuxAvailable,tmuxVersion,runningCount,adapters}'
```

## 2. Watch the stream, then dispatch

```bash
# Terminal A — watch WS
websocat ws://localhost:7766/api/v1/stream | jq -c 'select(.type | (startswith("runlog") or startswith("tmux")))'

# Terminal B — dispatch bead mp-abc to claude in agent mode
curl -s -X POST localhost:7766/api/v1/beads/mp-abc/dispatch \
  -H 'content-type: application/json' \
  -d '{"agent":"claude","mode":"agent","permissionMode":"acceptEdits"}' | jq
# -> 202; bead now in "running"
```
Terminal A shows `tmux.session.opened` then a stream of `runlog.line` frames.

## 3. Attach to the live agent

```bash
curl -s localhost:7766/api/v1/beads/mp-abc/steps/0/attach | jq -r .command
# -> tmux attach -t muster/mp-abc/0/0
tmux attach -t muster/mp-abc/0/0          # watch the live TUI; Ctrl-b d to detach
```
Or forward a keystroke without attaching:
```bash
curl -s -X POST localhost:7766/api/v1/beads/mp-abc/steps/0/send \
  -H 'content-type: application/json' \
  -d '{"keys":"y\n"}'
```

## 4. Completion

When the agent exits, Terminal A shows `tmux.session.closed` (with `exitCode`) followed by a `bead.updated` frame. **M2 limitation**: beads has no distinct "review" status (it folds to `in_progress`), so the bead's column does not change — completion is recorded as a note on the bead ("agent run completed (exit 0) — awaiting review" or the failure equivalent) plus the `bead.updated` frame announcing that note. The worktree at `~/.muster/worktrees/mp-abc` holds the agent's changes on branch `muster/mp-abc`.

## 5. Restart recovery

```bash
# While an agent runs, kill and restart muster:
kill <muster-pid> ; bin/muster serve --beads-dir … --repo mp=…
```
On startup muster lists `muster/*` tmux sessions, re-associates `mp-abc`/step 0, marks it `running`, and resumes `runlog.line` streaming — the agent never stopped.

## 6. Automated end-to-end test

The M2 implementation ships an automated e2e test that exercises this exact flow against your real claude and tmux:

```bash
make test-e2e
```

> The `test-e2e` Makefile target and the build-tagged e2e test it runs ship in the M2 orchestrator stack (stack 3 of the M2 PR series). On a tree that has only stack 1 / stack 2 merged, the target does not exist yet and `go test -tags=e2e -run TestE2E ./internal/orchestrator/` is the manual equivalent.

**Requirements**: `claude` installed and logged in (Max plan) + `tmux` ≥ 3.2. The test skips gracefully if either is missing. It uses a trivial one-word prompt to minimize usage. Uses Max plan usage allowance (not per-call billing).

## Notes / current limits (by design in M2)

- One active run per bead (`409` on duplicate dispatch).
- No run timeout by default → a stuck agent runs until you `tmux kill-session` (or set `--run-timeout`).
- Multi-day/durable runlog history is **not** kept (M9); catch-up works only while the session lives.
- Worktrees are not garbage-collected (M9).
- Without tmux, the agent still runs (direct exec) and streams, but attach/send are unavailable.
- The `--repo` flag (repeatable) and `MUSTER_REPO` env map bead-ID prefixes to repo paths.
- `--worktrees-dir` sets the per-bead worktree root (default: `~/.muster/worktrees`).
- `tmux` must be on the user's default socket for `tmux attach -t muster/<bead>/0/0` to work.
