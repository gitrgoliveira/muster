# Contract: WebSocket Events (M2 additions)

Additive to the M1 `bead.*` protocol. Same `ws.Frame` envelope, same hub. M1 event types unchanged (FR-019).

## New event types

| `type` | Emitted when | Frame fields |
|---|---|---|
| `runlog.line` | agent produces pane output | `beadID`, `stepIdx`, `seq` (monotonic per run), `data` (**base64-encoded** raw pane bytes — terminal output is not guaranteed UTF-8, so clients MUST base64-decode) |
| `tmux.session.opened` | a run's session is spawned | `beadID`, `stepIdx`, `session` (name) |
| `tmux.session.closed` | a run ends (exit/timeout/cancel) | `beadID`, `stepIdx`, `session`, `exitCode` |

## Ordering & semantics

- `runlog.line.seq` is monotonic within a (bead, step) run; clients render in `seq` order into a terminal emulator. `data` is **raw** (ANSI preserved) — see plan D1.
- Lifecycle order per run: `tmux.session.opened` → `runlog.line`* → `tmux.session.closed`. A `bead.updated` (M1 event) accompanies close, recording the run's outcome as an appended note — M2 cannot persist a distinct `review` state, so the bead's column does not move on completion (see data-model.md).
- **Catch-up is NOT via replayed WS frames**: a late-joining client fetches scrollback via the attach/`capture-pane` path (REST), then consumes live `runlog.line` from connect time. (No durable runlog in M2 — M9.)
- Fallback (tmux absent): `runlog.line` still flows from the child's stdout/stderr; `tmux.session.*` still bracket the run (name empty/synthetic); no catch-up.

## Non-goals (later milestones)

`runlog` compaction/persistence (M9), `worktree.changed`/diff events (M3), `step.*` chain events and `tmux` send-echo (M4/M8) are **not** emitted in M2.
